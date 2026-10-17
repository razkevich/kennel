package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/razkevich/kennel/internal/claude"
	_ "modernc.org/sqlite"
)

// indexVersion bumps whenever parsing changes; index tables are then rebuilt
// while user metadata (kennels, pup_meta) is kept.
const indexVersion = "5"

type Store struct {
	DB *sql.DB
}

func Home() string {
	if d := os.Getenv("KENNEL_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kennel")
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, err
	}
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

const metaSchema = `
CREATE TABLE IF NOT EXISTS kv(k TEXT PRIMARY KEY, v TEXT);
CREATE TABLE IF NOT EXISTS kennels(
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	icon TEXT NOT NULL DEFAULT '',
	color TEXT NOT NULL DEFAULT '',
	cwd TEXT NOT NULL DEFAULT '',
	rules TEXT NOT NULL DEFAULT '',
	sort INTEGER NOT NULL DEFAULT 0,
	created INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS things(
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	command TEXT NOT NULL,
	cwd TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL DEFAULT 'other',
	kennel_id INTEGER REFERENCES kennels(id) ON DELETE SET NULL,
	created INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS pup_meta(
	session_id TEXT PRIMARY KEY,
	kennel_id INTEGER REFERENCES kennels(id) ON DELETE SET NULL,
	alias TEXT NOT NULL DEFAULT '',
	starred INTEGER NOT NULL DEFAULT 0,
	archived INTEGER NOT NULL DEFAULT 0,
	seen_at INTEGER NOT NULL DEFAULT 0,
	note TEXT NOT NULL DEFAULT ''
);`

const indexSchema = `
CREATE TABLE IF NOT EXISTS sessions(
	id TEXT PRIMARY KEY,
	project TEXT, path TEXT UNIQUE, cwd TEXT, last_cwd TEXT,
	custom_title TEXT, agent_name TEXT, ai_title TEXT,
	first_prompt TEXT, last_prompt TEXT, recap TEXT,
	color TEXT, continued_in TEXT, worktree TEXT, version TEXT, remote TEXT,
	branches TEXT, prs TEXT, artifacts TEXT, crons TEXT, models TEXT,
	started INTEGER, updated INTEGER,
	n_user INTEGER, n_asst INTEGER, n_tools INTEGER, cost REAL,
	file_size INTEGER, file_mtime INTEGER, file_offset INTEGER, chunk_seq INTEGER
);
CREATE INDEX IF NOT EXISTS sessions_updated ON sessions(updated DESC);
CREATE VIRTUAL TABLE IF NOT EXISTS chunks USING fts5(
	text, session_id UNINDEXED, role UNINDEXED, ts UNINDEXED, seq UNINDEXED,
	tokenize='unicode61 remove_diacritics 2', prefix='2 3'
);`

func (s *Store) migrate() error {
	if _, err := s.DB.Exec(metaSchema); err != nil {
		return err
	}
	var v string
	_ = s.DB.QueryRow(`SELECT v FROM kv WHERE k='index_version'`).Scan(&v)
	if v != indexVersion {
		if _, err := s.DB.Exec(`DROP TABLE IF EXISTS sessions; DROP TABLE IF EXISTS chunks;`); err != nil {
			return err
		}
	}
	if _, err := s.DB.Exec(indexSchema); err != nil {
		return err
	}
	_, err := s.DB.Exec(`INSERT INTO kv(k,v) VALUES('index_version',?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, indexVersion)
	return err
}

// FileState is what the indexer needs to resume parsing a transcript.
type FileState struct {
	Size, Mtime, Offset int64
	Seq                 int
	Session             *claude.Session
}

func (s *Store) FileStates() (map[string]FileState, error) {
	rows, err := s.DB.Query(`SELECT ` + sessionCols + `, file_size, file_mtime, file_offset, chunk_seq FROM sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]FileState{}
	for rows.Next() {
		var fs FileState
		sess, err := scanSession(rows, &fs.Size, &fs.Mtime, &fs.Offset, &fs.Seq)
		if err != nil {
			return nil, err
		}
		fs.Session = sess
		out[sess.Path] = fs
	}
	return out, rows.Err()
}

const sessionCols = `id, project, path, cwd, last_cwd, custom_title, agent_name, ai_title,
	first_prompt, last_prompt, recap, color, continued_in, worktree, version, remote,
	branches, prs, artifacts, crons, models, started, updated, n_user, n_asst, n_tools, cost`

func scanSession(r interface{ Scan(...any) error }, extra ...any) (*claude.Session, error) {
	var s claude.Session
	var branches, prs, artifacts, crons, models string
	var started, updated int64
	dest := []any{&s.ID, &s.Project, &s.Path, &s.Cwd, &s.LastCwd, &s.CustomTitle, &s.AgentName, &s.AITitle,
		&s.FirstPrompt, &s.LastPrompt, &s.Recap, &s.Color, &s.ContinuedIn, &s.Worktree, &s.Version, &s.Remote,
		&branches, &prs, &artifacts, &crons, &models, &started, &updated, &s.NUser, &s.NAssistant, &s.NTools, &s.Cost}
	if err := r.Scan(append(dest, extra...)...); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(branches), &s.Branches)
	_ = json.Unmarshal([]byte(prs), &s.PRs)
	_ = json.Unmarshal([]byte(artifacts), &s.Artifacts)
	_ = json.Unmarshal([]byte(crons), &s.Crons)
	_ = json.Unmarshal([]byte(models), &s.Models)
	if started > 0 {
		s.Started = time.UnixMilli(started)
	}
	if updated > 0 {
		s.Updated = time.UnixMilli(updated)
	}
	return &s, nil
}

func js(v []string) string {
	if v == nil {
		return "[]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func jsonOf(v any) string {
	b, _ := json.Marshal(v)
	if string(b) == "null" {
		return "[]"
	}
	return string(b)
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// Save upserts a session and appends its new chunks in one transaction.
// When reset is true, previously indexed chunks for the session are dropped first.
func (s *Store) Save(sess *claude.Session, fs FileState, chunks []claude.Chunk, reset bool) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if reset {
		if _, err := tx.Exec(`DELETE FROM chunks WHERE session_id=?`, sess.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM sessions WHERE path=? AND id<>?`, sess.Path, sess.ID); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`INSERT INTO sessions(`+sessionCols+`, file_size, file_mtime, file_offset, chunk_seq)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET project=excluded.project, path=excluded.path, cwd=excluded.cwd, last_cwd=excluded.last_cwd,
		custom_title=excluded.custom_title, agent_name=excluded.agent_name, ai_title=excluded.ai_title,
		first_prompt=excluded.first_prompt, last_prompt=excluded.last_prompt, recap=excluded.recap, color=excluded.color,
		continued_in=excluded.continued_in, worktree=excluded.worktree, version=excluded.version, remote=excluded.remote,
		branches=excluded.branches, prs=excluded.prs, artifacts=excluded.artifacts, crons=excluded.crons, models=excluded.models,
		started=excluded.started, updated=excluded.updated, n_user=excluded.n_user, n_asst=excluded.n_asst,
		n_tools=excluded.n_tools, cost=excluded.cost, file_size=excluded.file_size, file_mtime=excluded.file_mtime,
		file_offset=excluded.file_offset, chunk_seq=excluded.chunk_seq`,
		sess.ID, sess.Project, sess.Path, sess.Cwd, sess.LastCwd, sess.CustomTitle, sess.AgentName, sess.AITitle,
		sess.FirstPrompt, sess.LastPrompt, sess.Recap, sess.Color, sess.ContinuedIn, sess.Worktree, sess.Version, sess.Remote,
		js(sess.Branches), js(sess.PRs), jsonOf(sess.Artifacts), jsonOf(sess.Crons), js(sess.Models), ms(sess.Started), ms(sess.Updated),
		sess.NUser, sess.NAssistant, sess.NTools, sess.Cost, fs.Size, fs.Mtime, fs.Offset, fs.Seq)
	if err != nil {
		return err
	}
	if len(chunks) > 0 {
		st, err := tx.Prepare(`INSERT INTO chunks(text, session_id, role, ts, seq) VALUES(?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, c := range chunks {
			if _, err := st.Exec(c.Text, sess.ID, string(c.Role), ms(c.TS), c.Seq); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) RemovePath(path string) error {
	var id string
	if err := s.DB.QueryRow(`SELECT id FROM sessions WHERE path=?`, path).Scan(&id); err != nil {
		return nil
	}
	_, err := s.DB.Exec(`DELETE FROM chunks WHERE session_id=?; DELETE FROM sessions WHERE id=?`, id, id)
	return err
}

func (s *Store) Sessions() ([]*claude.Session, error) {
	rows, err := s.DB.Query(`SELECT ` + sessionCols + ` FROM sessions ORDER BY updated DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*claude.Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func (s *Store) Count() (sessions, chunks int) {
	_ = s.DB.QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessions)
	_ = s.DB.QueryRow(`SELECT count(*) FROM chunks`).Scan(&chunks)
	return
}

// Hit is one full-text match.
type Hit struct {
	SessionID string
	Role      claude.Role
	Seq       int
	TS        time.Time
	Snippet   string
	Text      string
	Score     float64
}

// SnipOpen and SnipClose delimit matched terms inside Hit.Snippet.
const (
	SnipOpen  = "\x01"
	SnipClose = "\x02"
)

// Search runs an FTS5 MATCH query. match must already be valid FTS5 syntax.
func (s *Store) Search(match string, limit int) ([]Hit, error) {
	rows, err := s.DB.Query(`SELECT session_id, role, seq, ts,
		snippet(chunks, 0, ?, ?, '…', 14), text, bm25(chunks)
		FROM chunks WHERE chunks MATCH ? ORDER BY bm25(chunks) LIMIT ?`, SnipOpen, SnipClose, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search %q: %w", match, err)
	}
	defer rows.Close()
	var out []Hit
	for rows.Next() {
		var h Hit
		var role string
		var ts int64
		if err := rows.Scan(&h.SessionID, &role, &h.Seq, &ts, &h.Snippet, &h.Text, &h.Score); err != nil {
			return nil, err
		}
		h.Role = claude.Role(role)
		h.TS = time.UnixMilli(ts)
		h.Snippet = strings.ReplaceAll(h.Snippet, "\n", " ")
		out = append(out, h)
	}
	return out, rows.Err()
}

// Chunks returns a session's indexed chunks in order (used for transcript context).
func (s *Store) Chunks(sessionID string) ([]claude.Chunk, error) {
	rows, err := s.DB.Query(`SELECT seq, role, ts, text FROM chunks WHERE session_id=? ORDER BY seq`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []claude.Chunk
	for rows.Next() {
		var c claude.Chunk
		var role string
		var ts int64
		if err := rows.Scan(&c.Seq, &role, &ts, &c.Text); err != nil {
			return nil, err
		}
		c.Role = claude.Role(role)
		c.TS = time.UnixMilli(ts)
		out = append(out, c)
	}
	return out, rows.Err()
}
