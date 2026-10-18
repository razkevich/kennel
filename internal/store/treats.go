package store

import (
	"time"

	"github.com/razkevich/kennel/internal/treats"
)

func (s *Store) Treats() ([]treats.Treat, error) {
	rows, err := s.DB.Query(`SELECT id, name, kind, value, coalesce(kennel_id, 0) FROM treats ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []treats.Treat
	for rows.Next() {
		var t treats.Treat
		var kind string
		if err := rows.Scan(&t.ID, &t.Name, &kind, &t.Value, &t.Kennel); err != nil {
			return nil, err
		}
		t.Kind = treats.Kind(kind)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) SaveTreat(t *treats.Treat) error {
	res, err := s.DB.Exec(`INSERT INTO treats(name, kind, value, created, kennel_id) VALUES(?,?,?,?,?)`, t.Name, string(t.Kind), t.Value, time.Now().UnixMilli(), nullID(t.Kennel))
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
	_, err := s.DB.Exec(`INSERT INTO treats(id, name, kind, value, created, kennel_id) VALUES(?,?,?,?,?,?)`, t.ID, t.Name, string(t.Kind), t.Value, time.Now().UnixMilli(), nullID(t.Kennel))
	return err
}

// ShelveTreat puts a treat on a kennel's shelf; kennel 0 leaves it loose.
func (s *Store) ShelveTreat(id, kennel int64) error {
	_, err := s.DB.Exec(`UPDATE treats SET kennel_id=? WHERE id=?`, nullID(kennel), id)
	return err
}

func (s *Store) HiddenArtifacts() (map[string]bool, error) {
	rows, err := s.DB.Query(`SELECT url FROM hidden_artifacts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out[u] = true
	}
	return out, rows.Err()
}

func (s *Store) HideArtifact(url string, hidden bool) error {
	q := `INSERT OR IGNORE INTO hidden_artifacts(url) VALUES(?)`
	if !hidden {
		q = `DELETE FROM hidden_artifacts WHERE url=?`
	}
	_, err := s.DB.Exec(q, url)
	return err
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
