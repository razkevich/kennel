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
	sugg, err := a.SuggestKennels(context.Background(), ids, a.FirstSpace())
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

func TestOrganizeFloors(t *testing.T) {
	a := demoApp(t)
	a.Cfg.ThemeLLM = ask.Fake{}
	plan, err := a.SuggestFloors(context.Background(), a.FirstSpace())
	if err != nil || len(plan) < 2 {
		t.Fatalf("%+v %v", plan, err)
	}
	before := len(a.Floors())
	if _, err := a.OrganizeFloors(a.FirstSpace(), plan); err != nil {
		t.Fatal(err)
	}
	if len(a.Floors()) != before+len(plan) {
		t.Fatalf("want %d new floors, have %d", len(plan), len(a.Floors())-before)
	}
	for _, p := range plan {
		for _, kid := range p.Kennels {
			for _, k := range a.Kennels() {
				if k.ID == kid {
					if f, _ := a.floor(a.FloorOf(k)); f.Name != p.Floor {
						t.Fatalf("%s on %s, want %s", k.Name, f.Name, p.Floor)
					}
				}
			}
		}
	}
	a.Undo()
	if len(a.Floors()) != before {
		t.Fatal("undo removes the floors it built")
	}
	for _, k := range a.Kennels() {
		if f, _ := a.floor(a.FloorOf(k)); f.Name != "Main" {
			t.Fatalf("%s should be back on Main", k.Name)
		}
	}
}

func TestProjects(t *testing.T) {
	a := demoApp(t)
	first := a.FirstSpace()
	if sp, ok := a.Space(first); !ok || sp.Name != "My work" {
		t.Fatalf("a fresh kennel starts with My work, got %+v", a.Spaces())
	}
	sp := store.Space{Name: "Infra", Root: "/Users/demo/code/infra"}
	if err := a.SaveSpace(&sp); err != nil {
		t.Fatal(err)
	}
	key, floors, err := a.OpenSpace(sp.Key())
	if err != nil || key != sp.Key() || len(floors) != 1 || floors[0].Name != "Main" {
		t.Fatalf("opening a project gives it a floor: %v %+v %v", key, floors, err)
	}
	if a.InSpace(a.Find("CHECKOUT-V2"), sp.Key()) || !a.InSpace(a.Find("PG-UPGRADE"), sp.Key()) {
		t.Fatal("a project linked to a folder holds only that folder's sessions")
	}
	if !a.InSpace(a.Find("CHECKOUT-V2"), first) {
		t.Fatal("a project without a folder holds every session")
	}
	k := store.Kennel{Name: "Pipelines", Floor: floors[0].ID}
	a.SaveKennel(&k)
	if err := a.DeleteSpace(sp.ID); err != nil {
		t.Fatal(err)
	}
	for _, x := range a.Kennels() {
		if x.ID == k.ID && a.SpaceOf(x) != first {
			t.Fatal("a removed project's kennels move to another project")
		}
	}
	names := map[string]bool{}
	for _, f := range a.FloorsIn(first) {
		names[f.Name] = true
	}
	if !names["Main"] || !names["Main (Infra)"] {
		t.Fatalf("a moved floor named like one already there gets the project's name: %v", names)
	}
	a.Undo()
	if _, ok := a.Space(sp.Key()); !ok {
		t.Fatal("undo brings the project back")
	}
	if fs := a.FloorsIn(sp.Key()); len(fs) != 1 || fs[0].Name != "Main" {
		t.Fatalf("…with its floor's own name: %+v", fs)
	}
	for _, x := range a.Kennels() {
		if x.ID == k.ID && a.SpaceOf(x) != sp.Key() {
			t.Fatal("…with its floors and kennels")
		}
	}
	if err := a.DeleteSpace(sp.ID); err != nil {
		t.Fatal(err)
	}
	f, _ := a.Space(first)
	if err := a.DeleteSpace(f.ID); err == nil {
		t.Fatal("the last project stays")
	}
}
