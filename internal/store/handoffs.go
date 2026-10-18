package store

import "time"

// Handoff is one session passing something to another, drawn as a line between them.
type Handoff struct {
	From string
	To   string
	At   time.Time
}

func (s *Store) SaveHandoff(h Handoff) error {
	_, err := s.DB.Exec(`INSERT INTO handoffs(from_id, to_id, at) VALUES(?,?,?)`, h.From, h.To, h.At.UnixMilli())
	return err
}

func (s *Store) Handoffs(since time.Time) ([]Handoff, error) {
	rows, err := s.DB.Query(`SELECT from_id, to_id, at FROM handoffs WHERE at >= ? ORDER BY at`, since.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Handoff
	for rows.Next() {
		var h Handoff
		var at int64
		if err := rows.Scan(&h.From, &h.To, &at); err != nil {
			return nil, err
		}
		h.At = time.UnixMilli(at)
		out = append(out, h)
	}
	return out, rows.Err()
}
