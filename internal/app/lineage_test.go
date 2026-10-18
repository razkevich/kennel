package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/razkevich/kennel/internal/demo"
	"github.com/razkevich/kennel/internal/opener"
)

func demoApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	if err := demo.Generate(filepath.Join(dir, "claude"), now); err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{ClaudeDir: filepath.Join(dir, "claude"), Home: filepath.Join(dir, "home"), Opener: opener.Print, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	if _, err := a.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestCloneIntoAnotherProject(t *testing.T) {
	a := demoApp(t)
	src := a.Find("PG-UPGRADE")
	before, _ := os.ReadFile(src.Path)
	id, err := a.Clone(src.ID, "/Users/demo/code/mobile", "")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(src.Path)
	if string(before) != string(after) {
		t.Fatal("the original transcript must not change")
	}
	c := a.Pup(id)
	if c == nil || c.Title() != "PG-UPGRADE-CLONE" || c.Cwd != "/Users/demo/code/mobile" || c.NUser != src.NUser {
		t.Fatalf("clone: %+v", c)
	}
	if filepath.Base(filepath.Dir(c.Path)) != "-Users-demo-code-mobile" {
		t.Fatalf("clone stored in %s", c.Path)
	}
	b, _ := os.ReadFile(c.Path)
	if strings.Contains(string(b), src.ID) {
		t.Fatal("clone still references the original session id")
	}
}

func TestMergeTargetBrief(t *testing.T) {
	a := demoApp(t)
	x, y := a.Find("PG-UPGRADE"), a.Find("INCIDENT-0612")
	tg, err := a.MergeTarget([]string{x.ID, y.ID}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if tg.Title != "PG-UPGRADE+" || tg.Cwd != "/Users/demo/code/infra" || !tg.New {
		t.Fatalf("target %+v", tg)
	}
	for _, want := range []string{"combining 2", `"PG-UPGRADE"`, `"INCIDENT-0612"`, "logical replication", "transcript: " + x.Path, "PR https://github.com/acme/infra/pull/88"} {
		if !strings.Contains(tg.Prompt, want) {
			t.Errorf("brief missing %q:\n%s", want, tg.Prompt)
		}
	}
	if strings.Contains(tg.Prompt, "\n") {
		t.Error("brief should be one line so it survives shells and terminals")
	}
	one, _ := a.MergeTarget([]string{x.ID}, "", "")
	if one.Title != "PG-UPGRADE-FRESH" || !strings.Contains(one.Prompt, "picking up from an earlier") {
		t.Fatalf("distill: %+v", one)
	}
}

func TestRenameAddsNewlineAndSkipsOtherAgents(t *testing.T) {
	a := demoApp(t)
	p := a.Find("RATE-LIMITS")
	f, _ := os.OpenFile(p.Path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"user","partial`) // a line still being written
	f.Close()
	if err := a.Rename(p.ID, "THROTTLE"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p.Path)
	if !strings.Contains(string(b), "\"partial\n{") {
		t.Fatalf("title record must start on its own line:\n%s", b[len(b)-200:])
	}

	codex := a.Find("0199c1a2")
	before, _ := os.ReadFile(codex.Path)
	if err := a.Rename(codex.ID, "CSV"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(codex.Path)
	if string(before) != string(after) || a.Pup(codex.ID).Title() != "CSV" {
		t.Fatal("codex rename should be a kennel alias only")
	}
	if _, err := a.Trash(codex.ID); err == nil {
		t.Fatal("trash must refuse non-Claude sessions")
	}
}
