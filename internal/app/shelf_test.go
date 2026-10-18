package app

import (
	"context"
	"strings"
	"testing"

	"github.com/razkevich/kennel/internal/ask"
	"github.com/razkevich/kennel/internal/store"
)

func TestShelvesFollowTheirKennel(t *testing.T) {
	a := demoApp(t)
	k := store.Kennel{Name: "Shelf test"}
	a.SaveKennel(&k)
	tr, err := a.AddTreat("", "https://docs.acme.dev/x", k.ID)
	if err != nil || tr.Kennel != k.ID {
		t.Fatalf("add: %+v %v", tr, err)
	}
	if err := a.ShelveTreat(tr.ID, 0); err != nil {
		t.Fatal(err)
	}
	a.Undo()
	if got := shelfOf(a, k.ID); len(got) != 1 {
		t.Fatalf("undo should put it back on the shelf, got %v", got)
	}
	a.DeleteKennel(k.ID)
	if got := shelfOf(a, k.ID); len(got) != 0 {
		t.Fatal("a deleted kennel's items go loose")
	}
	a.Undo()
	if got := shelfOf(a, k.ID); len(got) != 1 {
		t.Fatal("undoing the delete puts the shelf back")
	}
}

func shelfOf(a *App, kennel int64) []string {
	var out []string
	for _, t := range a.Treats() {
		if t.Kennel == kennel {
			out = append(out, t.Name)
		}
	}
	return out
}

func TestHideArtifact(t *testing.T) {
	a := demoApp(t)
	all := a.Artifacts()
	if len(all) == 0 {
		t.Fatal("demo has artifacts")
	}
	a.HideArtifact(all[0].URL)
	if len(a.Artifacts()) != len(all)-1 {
		t.Fatal("hidden artifact still listed")
	}
	a.Undo()
	if len(a.Artifacts()) != len(all) {
		t.Fatal("undo shows it again")
	}
}

func TestSortIntoKennels(t *testing.T) {
	a := demoApp(t)
	a.Cfg.ThemeLLM = ask.Fake{}
	var k store.Kennel
	for _, x := range a.Kennels() {
		if x.Name == "Payments" {
			k = x
		}
	}
	var ids []string
	for _, p := range a.Pups() {
		if p.Kennel == nil && !p.Empty() {
			ids = append(ids, p.ID)
		}
	}
	sugg, err := a.SuggestKennels(context.Background(), ids, "")
	if err != nil || len(sugg) == 0 {
		t.Fatalf("%v %v", sugg, err)
	}
	var existing, fresh int
	for _, s := range sugg {
		if s.KennelID == k.ID {
			existing++
		} else if s.KennelID == 0 {
			fresh++
		}
	}
	if existing == 0 || fresh == 0 {
		t.Fatalf("want some into Payments and some new kennels: %+v", sugg)
	}
	before := len(a.Kennels())
	n, err := a.FileAll(sugg, a.Floors()[0].ID)
	if err != nil || n != len(sugg) {
		t.Fatalf("filed %d of %d: %v", n, len(sugg), err)
	}
	if len(a.Kennels()) <= before {
		t.Fatal("new kennels should be built")
	}
	if label := a.UndoLabel(); !strings.HasPrefix(label, "sort ") {
		t.Fatalf("one undo step, got %q", label)
	}
	a.Undo()
	if len(a.Kennels()) != before {
		t.Fatal("undo removes the kennels it built")
	}
	for _, id := range ids {
		if a.Pup(id).Kennel != nil {
			t.Fatal("undo unfiles everything")
		}
	}
}
