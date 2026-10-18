package store

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Style is how a user has shaped one room: looks, where it sits on the map,
// decoration, and purpose blocks. Keyed by room key ("yard", "project:/x", "kennel:3").
type Style struct {
	Shape  string         `json:"shape,omitempty"`
	Floor  string         `json:"floor,omitempty"`
	Wall   string         `json:"wall,omitempty"`
	Pos    map[string]Pos `json:"pos,omitempty"`
	Decor  []Placed       `json:"decor,omitempty"`
	Blocks []Block        `json:"blocks,omitempty"`
	// Spots are where the user dragged pups inside the room, keyed by pup id,
	// in the same 0..1 interior coordinates as decor.
	Spots map[string]Placed `json:"spots,omitempty"`
}

// Pos is a room's spot on one floor, in tiles; W and H are a user-set size (0 = automatic).
type Pos struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w,omitempty"`
	H int `json:"h,omitempty"`
}

type Placed struct {
	Kind string  `json:"kind"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

// Block is a purpose block placed in a room; Config holds its settings.
type Block struct {
	Kind   string            `json:"kind"`
	X      float64           `json:"x"`
	Y      float64           `json:"y"`
	Config map[string]string `json:"config,omitempty"`
}

func (b Block) Get(k string) string { return b.Config[k] }

func (b Block) Int(k string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(b.Config[k])); err == nil {
		return v
	}
	return def
}

func (s Style) OfKind(kind string) []Block {
	var out []Block
	for _, b := range s.Blocks {
		if b.Kind == kind {
			out = append(out, b)
		}
	}
	return out
}

func (s *Store) Styles() (map[string]Style, error) {
	rows, err := s.DB.Query(`SELECT key, style FROM room_style`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Style{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		var st Style
		if json.Unmarshal([]byte(v), &st) == nil {
			out[k] = st
		}
	}
	return out, rows.Err()
}

func (s *Store) SaveStyle(key string, st Style) error {
	if key == "" {
		return fmt.Errorf("room key required")
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO room_style(key, style) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET style=excluded.style`, key, string(b))
	return err
}

func (s *Store) DeleteStyle(key string) error {
	_, err := s.DB.Exec(`DELETE FROM room_style WHERE key=?`, key)
	return err
}
