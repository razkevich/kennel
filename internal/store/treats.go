package store

import (
	"time"

	"github.com/razkevich/kennel/internal/treats"
)

func (s *Store) Treats() ([]treats.Treat, error) {
	rows, err := s.DB.Query(`SELECT id, name, kind, value FROM treats ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []treats.Treat
	for rows.Next() {
		var t treats.Treat
		var kind string
		if err := rows.Scan(&t.ID, &t.Name, &kind, &t.Value); err != nil {
			return nil, err
		}
		t.Kind = treats.Kind(kind)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) SaveTreat(t *treats.Treat) error {
	res, err := s.DB.Exec(`INSERT INTO treats(name, kind, value, created) VALUES(?,?,?,?)`, t.Name, string(t.Kind), t.Value, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) DeleteTreat(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM treats WHERE id=?`, id)
	return err
}

// RestoreTreat puts back a removed treat under its original id.
func (s *Store) RestoreTreat(t treats.Treat) error {
	_, err := s.DB.Exec(`INSERT INTO treats(id, name, kind, value, created) VALUES(?,?,?,?,?)`, t.ID, t.Name, string(t.Kind), t.Value, time.Now().UnixMilli())
	return err
}
