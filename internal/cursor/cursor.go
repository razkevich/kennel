// Package cursor reads Cursor's composer (agent chat) history, read-only, from
// its VS Code-style state databases.
package cursor

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/razkevich/kennel/internal/claude"
	_ "modernc.org/sqlite"
)

// Dir is Cursor's User folder.
func Dir() string {
	if d := os.Getenv("KENNEL_CURSOR_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "Cursor", "User")
	}
	return filepath.Join(home, ".config", "Cursor", "User")
}

func GlobalDB(dir string) string { return filepath.Join(dir, "globalStorage", "state.vscdb") }

type Conversation struct {
	Session *claude.Session
	Chunks  []claude.Chunk
}

type composer struct {
	ID        string `json:"composerId"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"lastUpdatedAt"`
	Headers   []struct {
		BubbleID string `json:"bubbleId"`
		Type     int    `json:"type"`
	} `json:"fullConversationHeadersOnly"`
	Conversation []bubble `json:"conversation"`
}

type bubble struct {
	BubbleID  string `json:"bubbleId"`
	Type      int    `json:"type"`
	Text      string `json:"text"`
	CreatedAt any    `json:"createdAt"`
}

func open(path string) (*sql.DB, error) {
	// Read-only: Cursor owns these files and may be writing to them.
	return sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(3000)")
}

// Read returns every composer conversation updated after `since` (ms), plus the
// ids of all composers that exist (so callers can drop deleted ones).
func Read(dir string, since map[string]int64) ([]Conversation, map[string]bool, error) {
	db, err := open(GlobalDB(dir))
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	folders := workspaceFolders(dir)

	// Look at timestamps first; full JSON is loaded only for composers that changed.
	all := map[string]bool{}
	var changed []string
	stamps, err := db.Query(`SELECT key, json_extract(value, '$.lastUpdatedAt') FROM cursorDiskKV WHERE key LIKE 'composerData:%'`)
	if err == nil {
		for stamps.Next() {
			var key string
			var updated sql.NullInt64
			if stamps.Scan(&key, &updated) != nil {
				continue
			}
			id := strings.TrimPrefix(key, "composerData:")
			all[id] = true
			if prev, ok := since[id]; ok && updated.Valid && prev >= updated.Int64 {
				continue
			}
			changed = append(changed, key)
		}
		err = stamps.Err()
		stamps.Close()
	}
	var comps []composer
	load := func(query string, args ...any) error {
		rows, err := db.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var val []byte
			if err := rows.Scan(&key, &val); err != nil {
				return err
			}
			var c composer
			if json.Unmarshal(val, &c) != nil {
				continue
			}
			if c.ID == "" {
				c.ID = strings.TrimPrefix(key, "composerData:")
			}
			comps = append(comps, c)
		}
		return rows.Err()
	}
	if err != nil {
		// Older or binary-encoded values: fall back to reading everything.
		all = map[string]bool{}
		if err := load(`SELECT key, value FROM cursorDiskKV WHERE key LIKE 'composerData:%'`); err != nil {
			return nil, nil, err
		}
		for _, c := range comps {
			all[c.ID] = true
		}
	} else {
		for _, key := range changed {
			if err := load(`SELECT key, value FROM cursorDiskKV WHERE key = ?`, key); err != nil {
				return nil, nil, err
			}
		}
	}

	var out []Conversation
	for _, c := range comps {
		all[c.ID] = true
		if prev, ok := since[c.ID]; ok && prev >= c.UpdatedAt {
			continue
		}
		bubbles := c.Conversation
		if len(bubbles) == 0 && len(c.Headers) > 0 {
			bubbles = loadBubbles(db, c)
		}
		if len(bubbles) == 0 {
			continue
		}
		out = append(out, convert(c, bubbles, folders[c.ID]))
	}
	return out, all, nil
}

func loadBubbles(db *sql.DB, c composer) []bubble {
	rows, err := db.Query(`SELECT key, value FROM cursorDiskKV WHERE key LIKE ?`, "bubbleId:"+c.ID+":%")
	if err != nil {
		return nil
	}
	defer rows.Close()
	byID := map[string]bubble{}
	for rows.Next() {
		var key string
		var val []byte
		if rows.Scan(&key, &val) != nil {
			continue
		}
		var b bubble
		if json.Unmarshal(val, &b) == nil {
			b.BubbleID = key[strings.LastIndex(key, ":")+1:]
			byID[b.BubbleID] = b
		}
	}
	var out []bubble
	for _, h := range c.Headers {
		if b, ok := byID[h.BubbleID]; ok {
			if b.Type == 0 {
				b.Type = h.Type
			}
			out = append(out, b)
		}
	}
	return out
}

func convert(c composer, bubbles []bubble, folder string) Conversation {
	s := &claude.Session{Agent: "cursor", ID: c.ID, Path: "cursor:" + c.ID, Project: "cursor", Cwd: folder, LastCwd: folder, CustomTitle: c.Name}
	if c.CreatedAt > 0 {
		s.Started = time.UnixMilli(c.CreatedAt)
	}
	if c.UpdatedAt > 0 {
		s.Updated = time.UnixMilli(c.UpdatedAt)
	}
	var chunks []claude.Chunk
	seq := 0
	for _, b := range bubbles {
		text := strings.TrimSpace(b.Text)
		if text == "" {
			continue
		}
		role := claude.RoleAssistant
		if b.Type == 1 {
			role = claude.RoleUser
			s.NUser++
			if s.FirstPrompt == "" {
				s.FirstPrompt = clip(text, 300)
			}
			s.LastPrompt = clip(text, 300)
		} else {
			s.NAssistant++
		}
		for len(text) > 0 {
			part := text
			if len(part) > 1500 {
				cut := strings.LastIndexAny(part[:1500], "\n. ")
				if cut < 750 {
					cut = 1500
				}
				for cut < len(part) && part[cut]&0xC0 == 0x80 {
					cut++
				}
				part = part[:cut]
			}
			chunks = append(chunks, claude.Chunk{Seq: seq, Role: role, TS: s.Updated, Text: part})
			seq++
			text = strings.TrimSpace(text[len(part):])
		}
	}
	return Conversation{Session: s, Chunks: chunks}
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// workspaceFolders maps composer ids to the folder of the workspace they were created in.
func workspaceFolders(dir string) map[string]string {
	out := map[string]string{}
	metas, _ := filepath.Glob(filepath.Join(dir, "workspaceStorage", "*", "workspace.json"))
	sort.Strings(metas)
	for _, meta := range metas {
		b, err := os.ReadFile(meta)
		if err != nil {
			continue
		}
		var ws struct {
			Folder string `json:"folder"`
		}
		if json.Unmarshal(b, &ws) != nil || ws.Folder == "" {
			continue
		}
		u, err := url.Parse(ws.Folder)
		if err != nil || u.Scheme != "file" {
			continue
		}
		db, err := open(filepath.Join(filepath.Dir(meta), "state.vscdb"))
		if err != nil {
			continue
		}
		var val []byte
		err = db.QueryRow(`SELECT value FROM ItemTable WHERE key = 'composer.composerData'`).Scan(&val)
		db.Close()
		if err != nil {
			continue
		}
		var data struct {
			All []struct {
				ID string `json:"composerId"`
			} `json:"allComposers"`
		}
		if json.Unmarshal(val, &data) == nil {
			for _, c := range data.All {
				out[c.ID] = u.Path
			}
		}
	}
	return out
}
