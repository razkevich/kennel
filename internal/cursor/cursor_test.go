package cursor

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/razkevich/kennel/internal/demo"
)

func TestReadDemoComposers(t *testing.T) {
	dir := t.TempDir()
	cdir := filepath.Join(dir, "claude")
	if err := demo.Generate(cdir, time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	convs, all, err := Read(demo.CursorDir(cdir), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(convs) != 2 || len(all) != 2 {
		t.Fatalf("convs=%d all=%d", len(convs), len(all))
	}
	var polish *Conversation
	for i := range convs {
		if convs[i].Session.CustomTitle == "Onboarding copy polish" {
			polish = &convs[i]
		}
	}
	s := polish.Session
	if s.Agent != "cursor" || s.Cwd != "/Users/demo/code/atlas" || s.NUser != 2 || s.NAssistant != 2 || len(polish.Chunks) != 4 {
		t.Fatalf("session: %+v chunks=%d", s, len(polish.Chunks))
	}
	if s.FirstPrompt != "tighten the onboarding tour copy, it's too wordy" {
		t.Fatalf("first prompt %q", s.FirstPrompt)
	}
	again, _, _ := Read(demo.CursorDir(cdir), map[string]int64{s.ID: s.Updated.UnixMilli()})
	if len(again) != 1 {
		t.Fatalf("unchanged composers should be skipped, got %d", len(again))
	}
}
