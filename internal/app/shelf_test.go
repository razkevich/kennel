package app

import (
	"strings"
	"testing"

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

func TestTellAsleepClaudeRunsHeadless(t *testing.T) {
	a := demoApp(t)
	a.Cfg.Demo = true
	p := a.Find("RATE-LIMITS")
	msg, err := a.Tell(p, "  add overrides  ")
	if err != nil || !strings.Contains(msg, "claude --resume "+p.ID+" -p add overrides") {
		t.Fatalf("%q %v", msg, err)
	}
	if _, err := a.Tell(p, " "); err == nil {
		t.Fatal("empty message")
	}
}
