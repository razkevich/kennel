package app

import (
	"os"
	"strings"
	"testing"

	"github.com/razkevich/kennel/internal/store"
)

func TestUndo(t *testing.T) {
	a := demoApp(t)
	p := a.Find("RATE-LIMITS")

	a.ToggleStar(p.ID)
	a.SetNote(p.ID, "ping Priya")
	k := store.Kennel{Name: "Payments"}
	a.SaveKennel(&k)
	a.MoveTo(p.ID, k.ID)
	if got := a.UndoLabel(); got != "move RATE-LIMITS" {
		t.Fatalf("label %q", got)
	}
	for _, want := range []string{"move RATE-LIMITS", "build kennel Payments", "note on RATE-LIMITS", "star RATE-LIMITS"} {
		label, err := a.Undo()
		if err != nil || label != want {
			t.Fatalf("undo %q: got %q %v", want, label, err)
		}
	}
	q := a.Pup(p.ID)
	if q.Meta.Starred || q.Meta.Note != "" || q.Meta.KennelID != 0 || len(a.Kennels()) != 0 {
		t.Fatalf("state not restored: %+v kennels=%d", q.Meta, len(a.Kennels()))
	}
	if _, err := a.Undo(); err == nil {
		t.Fatal("nothing left to undo")
	}

	// Rename writes a title record; undo writes the old one back.
	a.Rename(p.ID, "THROTTLE")
	a.Undo()
	if a.Pup(p.ID).Title() != "RATE-LIMITS" {
		t.Fatalf("title after undo: %q", a.Pup(p.ID).Title())
	}
	b, _ := os.ReadFile(p.Path)
	if !strings.HasSuffix(strings.TrimSpace(string(b)), `"agentName":"RATE-LIMITS","sessionId":"`+p.ID+`","type":"agent-name"}`) &&
		!strings.Contains(string(b)[strings.LastIndex(string(b), "THROTTLE"):], `"customTitle":"RATE-LIMITS"`) {
		t.Fatalf("old title should be written back after THROTTLE")
	}

	// Trash moves the transcript back.
	if _, err := a.Trash(p.ID); err != nil {
		t.Fatal(err)
	}
	if a.Pup(p.ID) != nil {
		t.Fatal("trashed pup should disappear")
	}
	if _, err := a.Undo(); err != nil {
		t.Fatal(err)
	}
	if a.Pup(p.ID) == nil {
		t.Fatal("untrash should bring the pup back")
	}

	// Undo of a deleted kennel restores it with its members.
	k2 := store.Kennel{Name: "Infra"}
	a.SaveKennel(&k2)
	a.MoveTo(p.ID, k2.ID)
	a.DeleteKennel(k2.ID)
	a.Undo()
	if len(a.Kennels()) != 1 || a.Pup(p.ID).Meta.KennelID != k2.ID {
		t.Fatalf("kennel and membership not restored")
	}
}
