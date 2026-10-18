package store

import (
	"database/sql"
	"fmt"

	"encoding/json"
	"github.com/razkevich/kennel/internal/things"
	"regexp"
	"strings"
	"time"
)

type Kennel struct {
	ID    int64
	Name  string
	Icon  string
	Color string
	Cwd   string
	Rules Rules
	Sort  int
	// Floor is where the kennel's room stands on the map.
	Floor int64
	// Smart names a self-filling kind ("yard", "fire", "workshop"…); such a kennel
	// shows matching pups instead of the ones filed into it.
	Smart string
}

// Rules auto-assign pups to a kennel when they have no explicit kennel.
type Rules struct {
	CwdPrefix []string `json:"cwd_prefix,omitempty"`
	Match     string   `json:"match,omitempty"`
}

func (r Rules) Empty() bool { return len(r.CwdPrefix) == 0 && r.Match == "" }

// Matches tests cwd prefixes and a case-insensitive regex against title/branches.
func (r Rules) Matches(cwd string, texts ...string) bool {
	for _, p := range r.CwdPrefix {
		if p != "" && strings.HasPrefix(cwd, p) {
			return true
		}
	}
	if r.Match == "" {
		return false
	}
	re, err := regexp.Compile("(?i)" + r.Match)
	if err != nil {
		return false
	}
	for _, t := range texts {
		if re.MatchString(t) {
			return true
		}
	}
	return false
}

type PupMeta struct {
	SessionID string
	KennelID  int64
	Alias     string
	Starred   bool
	Archived  bool
	SeenAt    time.Time
	Note      string
	// Theme is an automatic category (set by kennel's categorizer).
	Theme string
	// Unfiled means the user took the pup out of its kennel, so kennel rules don't adopt it again.
	Unfiled bool
	// LookRole and LookFur are the hat and fur the user picked: "" means automatic,
	// and LookRole "none" means no hat.
	LookRole string
	LookFur  string
}

func (s *Store) Kennels() ([]Kennel, error) {
	rows, err := s.DB.Query(`SELECT id, name, icon, color, cwd, rules, sort, floor_id, smart FROM kennels ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Kennel
	for rows.Next() {
		var k Kennel
		var rules string
		if err := rows.Scan(&k.ID, &k.Name, &k.Icon, &k.Color, &k.Cwd, &rules, &k.Sort, &k.Floor, &k.Smart); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(rules), &k.Rules)
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) SaveKennel(k *Kennel) error {
	rules := ""
	if !k.Rules.Empty() {
		b, _ := json.Marshal(k.Rules)
		rules = string(b)
	}
	if k.ID == 0 {
		var maxSort sql.NullInt64
		_ = s.DB.QueryRow(`SELECT max(sort) FROM kennels`).Scan(&maxSort)
		k.Sort = int(maxSort.Int64) + 1
		res, err := s.DB.Exec(`INSERT INTO kennels(name, icon, color, cwd, rules, sort, created, floor_id, smart) VALUES(?,?,?,?,?,?,?,?,?)`,
			k.Name, k.Icon, k.Color, k.Cwd, rules, k.Sort, time.Now().UnixMilli(), k.Floor, k.Smart)
		if err != nil {
			return err
		}
		k.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.Exec(`UPDATE kennels SET name=?, icon=?, color=?, cwd=?, rules=?, sort=?, floor_id=?, smart=? WHERE id=?`,
		k.Name, k.Icon, k.Color, k.Cwd, rules, k.Sort, k.Floor, k.Smart, k.ID)
	return err
}

func (s *Store) DeleteKennel(id int64) error {
	_, err := s.DB.Exec(`UPDATE pup_meta SET kennel_id=NULL WHERE kennel_id=?; DELETE FROM kennels WHERE id=?`, id, id)
	return err
}

func (s *Store) Meta() (map[string]PupMeta, error) {
	rows, err := s.DB.Query(`SELECT session_id, coalesce(kennel_id,0), alias, starred, archived, seen_at, note, theme, unfiled, look_role, look_fur FROM pup_meta`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]PupMeta{}
	for rows.Next() {
		var m PupMeta
		var seen int64
		if err := rows.Scan(&m.SessionID, &m.KennelID, &m.Alias, &m.Starred, &m.Archived, &seen, &m.Note, &m.Theme, &m.Unfiled, &m.LookRole, &m.LookFur); err != nil {
			return nil, err
		}
		if seen > 0 {
			m.SeenAt = time.UnixMilli(seen)
		}
		out[m.SessionID] = m
	}
	return out, rows.Err()
}

func (s *Store) SaveMeta(m PupMeta) error {
	var kennel any
	if m.KennelID != 0 {
		kennel = m.KennelID
	}
	_, err := s.DB.Exec(`INSERT INTO pup_meta(session_id, kennel_id, alias, starred, archived, seen_at, note) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(session_id) DO UPDATE SET kennel_id=excluded.kennel_id, alias=excluded.alias, starred=excluded.starred,
		archived=excluded.archived, seen_at=excluded.seen_at, note=excluded.note`,
		m.SessionID, kennel, m.Alias, m.Starred, m.Archived, ms(m.SeenAt), m.Note)
	return err
}

func (s *Store) Things() ([]things.Thing, error) {
	rows, err := s.DB.Query(`SELECT id, name, command, cwd, kind, coalesce(kennel_id,0) FROM things ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []things.Thing
	for rows.Next() {
		var t things.Thing
		var kind string
		if err := rows.Scan(&t.ID, &t.Name, &t.Command, &t.Cwd, &kind, &t.KennelID); err != nil {
			return nil, err
		}
		t.Kind = things.Kind(kind)
		t.Saved = true
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) SaveThing(t *things.Thing) error {
	var kennel any
	if t.KennelID != 0 {
		kennel = t.KennelID
	}
	if t.ID == 0 {
		res, err := s.DB.Exec(`INSERT INTO things(name, command, cwd, kind, kennel_id, created) VALUES(?,?,?,?,?,?)`,
			t.Name, t.Command, t.Cwd, string(t.Kind), kennel, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		t.ID, _ = res.LastInsertId()
		t.Saved = true
		return nil
	}
	_, err := s.DB.Exec(`UPDATE things SET name=?, command=?, cwd=?, kind=?, kennel_id=? WHERE id=?`,
		t.Name, t.Command, t.Cwd, string(t.Kind), kennel, t.ID)
	return err
}

func (s *Store) DeleteThing(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM things WHERE id=?`, id)
	return err
}

var metaColumns = map[string]bool{"kennel_id": true, "alias": true, "starred": true, "archived": true, "seen_at": true, "note": true, "theme": true, "unfiled": true, "look_role": true, "look_fur": true}

// SetMetaField upserts a single pup_meta column.
func (s *Store) SetMetaField(sessionID, column string, value any) error {
	if !metaColumns[column] {
		return fmt.Errorf("unknown meta field %q", column)
	}
	_, err := s.DB.Exec(`INSERT INTO pup_meta(session_id, `+column+`) VALUES(?, ?)
		ON CONFLICT(session_id) DO UPDATE SET `+column+`=excluded.`+column, sessionID, value)
	return err
}

func (s *Store) Get(k string) string {
	var v string
	_ = s.DB.QueryRow(`SELECT v FROM kv WHERE k=?`, k).Scan(&v)
	return v
}

func (s *Store) Set(k, v string) error {
	_, err := s.DB.Exec(`INSERT INTO kv(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
	return err
}

// RestoreKennel puts back a deleted kennel under its original id.
func (s *Store) RestoreKennel(k Kennel) error {
	rules := ""
	if !k.Rules.Empty() {
		b, _ := json.Marshal(k.Rules)
		rules = string(b)
	}
	_, err := s.DB.Exec(`INSERT INTO kennels(id, name, icon, color, cwd, rules, sort, created, floor_id, smart) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		k.ID, k.Name, k.Icon, k.Color, k.Cwd, rules, k.Sort, time.Now().UnixMilli(), k.Floor, k.Smart)
	return err
}

// RestoreThing puts back a forgotten thing under its original id.
func (s *Store) RestoreThing(t things.Thing) error {
	var kennel any
	if t.KennelID != 0 {
		kennel = t.KennelID
	}
	_, err := s.DB.Exec(`INSERT INTO things(id, name, command, cwd, kind, kennel_id, created) VALUES(?,?,?,?,?,?,?)`,
		t.ID, t.Name, t.Command, t.Cwd, string(t.Kind), kennel, time.Now().UnixMilli())
	return err
}
