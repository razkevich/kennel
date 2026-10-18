package store

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestStyleKeepsSizeAndSpots(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "kennel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	want := Style{
		Pos:   map[string]Pos{"2": {X: 4, Y: 6, W: 14, H: 9}, "0": {X: 1, Y: 2}},
		Spots: map[string]Placed{"abc": {X: 0.25, Y: 0.8}},
	}
	if err := s.SaveStyle("kennel:1", want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Styles()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got["kennel:1"], want) {
		t.Fatalf("got %+v, want %+v", got["kennel:1"], want)
	}
}
