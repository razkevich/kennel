package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Floor is one level of a space's map; each kennel stands on exactly one.
// Space is a project root, or "" for the space that spans all projects.
type Floor struct {
	ID    int64
	Name  string
	Icon  string
	Sort  int
	Space string
}

func (s *Store) Floors() ([]Floor, error) {
	rows, err := s.DB.Query(`SELECT id, name, icon, sort, space FROM floors ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Floor
	for rows.Next() {
		var f Floor
		if err := rows.Scan(&f.ID, &f.Name, &f.Icon, &f.Sort, &f.Space); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) SaveFloor(f *Floor) error {
	if strings.TrimSpace(f.Name) == "" {
		return errors.New("a floor needs a name")
	}
	if f.ID == 0 {
		_ = s.DB.QueryRow(`SELECT coalesce(max(sort), 0) + 1 FROM floors`).Scan(&f.Sort)
		res, err := s.DB.Exec(`INSERT INTO floors(name, icon, sort, space) VALUES(?,?,?,?)`, f.Name, f.Icon, f.Sort, f.Space)
		if err != nil {
			return err
		}
		f.ID, _ = res.LastInsertId()
		return nil
	}
	_, err := s.DB.Exec(`INSERT INTO floors(id, name, icon, sort, space) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, icon=excluded.icon, sort=excluded.sort, space=excluded.space`, f.ID, f.Name, f.Icon, f.Sort, f.Space)
	return err
}

// DeleteFloor removes a floor, moving its kennels to `to`.
func (s *Store) DeleteFloor(id, to int64) error {
	if id == to {
		return errors.New("can't move a floor's kennels onto itself")
	}
	// One statement per Exec: the driver binds every statement of a batch from the first argument.
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE kennels SET floor_id=? WHERE floor_id=?`, to, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM floors WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// firstFloor makes sure there is a floor. Kennels from before floors existed move
// onto it, keeping the spots they had on the old curated floor ("2").
func (s *Store) firstFloor() error {
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM floors`).Scan(&n); err != nil || n > 0 {
		return err
	}
	f := Floor{Name: "Main", Icon: "🏢"}
	if err := s.SaveFloor(&f); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`UPDATE kennels SET floor_id=? WHERE floor_id=0`, f.ID); err != nil {
		return err
	}
	styles, err := s.Styles()
	if err != nil {
		return err
	}
	for key, st := range styles {
		if !strings.HasPrefix(key, "kennel:") || st.Pos == nil {
			continue
		}
		old, ok := st.Pos["2"]
		if !ok {
			continue
		}
		st.Pos[fmt.Sprint(f.ID)] = old
		b, _ := json.Marshal(st)
		if _, err := s.DB.Exec(`UPDATE room_style SET style=? WHERE key=?`, string(b), key); err != nil {
			return err
		}
	}
	return nil
}
