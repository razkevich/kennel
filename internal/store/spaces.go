package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Space is a project in kennel: its own floors and kennels. Root optionally links a
// folder; then its session list and smart rooms only show sessions from there.
type Space struct {
	ID   int64
	Name string
	Icon string
	Root string
	Sort int
}

// Key is how floors refer to the space.
func (s Space) Key() string { return fmt.Sprintf("space:%d", s.ID) }

func (s *Store) Spaces() ([]Space, error) {
	rows, err := s.DB.Query(`SELECT id, name, icon, root, sort FROM spaces ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Space
	for rows.Next() {
		var sp Space
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.Icon, &sp.Root, &sp.Sort); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *Store) SaveSpace(sp *Space) error {
	if strings.TrimSpace(sp.Name) == "" {
		return errors.New("a project needs a name")
	}
	if sp.ID == 0 {
		_ = s.DB.QueryRow(`SELECT coalesce(max(sort), 0) + 1 FROM spaces`).Scan(&sp.Sort)
		res, err := s.DB.Exec(`INSERT INTO spaces(name, icon, root, sort) VALUES(?,?,?,?)`, sp.Name, sp.Icon, sp.Root, sp.Sort)
		if err != nil {
			return err
		}
		sp.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.Exec(`INSERT INTO spaces(id, name, icon, root, sort) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, icon=excluded.icon, root=excluded.root, sort=excluded.sort`, sp.ID, sp.Name, sp.Icon, sp.Root, sp.Sort)
	return err
}

// DeleteSpace removes a project, moving its floors to the space with key `to`.
func (s *Store) DeleteSpace(id int64, to string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE floors SET space=? WHERE space=?`, to, fmt.Sprintf("space:%d", id)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM spaces WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// firstSpaces turns the floors' old spaces into projects: the all-projects map
// becomes "My work", and each per-folder map a project linked to that folder.
func (s *Store) firstSpaces() error {
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM spaces`).Scan(&n); err != nil || n > 0 {
		return err
	}
	rows, err := s.DB.Query(`SELECT DISTINCT space FROM floors ORDER BY space`)
	if err != nil {
		return err
	}
	var old []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		old = append(old, v)
	}
	rows.Close()
	if len(old) == 0 {
		old = []string{""}
	}
	for _, v := range old {
		sp := Space{Name: "My work", Icon: "💼"}
		if v != "" {
			sp = Space{Name: filepath.Base(v), Icon: "📁", Root: v}
		}
		if err := s.SaveSpace(&sp); err != nil {
			return err
		}
		if _, err := s.DB.Exec(`UPDATE floors SET space=? WHERE space=?`, sp.Key(), v); err != nil {
			return err
		}
		// The page's memory of where it was follows the map to its new key.
		if s.Get("web_space") == v {
			_ = s.Set("web_space", sp.Key())
		}
		if fl := s.Get("web_floor:" + v); fl != "" {
			_ = s.Set("web_floor:"+sp.Key(), fl)
		}
	}
	return nil
}
