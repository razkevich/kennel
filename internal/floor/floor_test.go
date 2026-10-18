package floor

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/razkevich/kennel/internal/app"
	"github.com/razkevich/kennel/internal/blocks"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/demo"
	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/store"
)

var now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

type recorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *recorder) Run(name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	switch {
	case name == "herdr" && args[0] == "workspace" && args[1] == "list":
		return []byte(`{"result":{"workspaces":[{"workspace_id":"w1","label":"relocations"}]}}`), nil
	case name == "herdr" && args[0] == "workspace" && args[1] == "create":
		return []byte(`{"result":{"workspace":{"workspace_id":"w7"}}}`), nil
	case name == "herdr" && args[0] == "tab":
		return []byte(`{"result":{"root_pane":{"pane_id":"w7:p1"}}}`), nil
	}
	return []byte("{}"), nil
}

func setup(t *testing.T) (*app.App, *recorder) {
	t.Helper()
	dir := t.TempDir()
	cdir := filepath.Join(dir, "claude")
	if err := demo.Generate(cdir, now); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	a, err := app.New(app.Config{ClaudeDir: cdir, Home: filepath.Join(dir, "home"), Opener: opener.Herdr, Runner: rec, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	if _, err := a.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	return a, rec
}

func pupNamed(t *testing.T, a *app.App, title string) *pack.Pup {
	for _, p := range a.Pups() {
		if p.Title() == title {
			return p
		}
	}
	t.Fatalf("no pup %s", title)
	return nil
}

func TestStyleOverridesLook(t *testing.T) {
	a, _ := setup(t)
	if err := a.SaveStyle("yard", store.Style{Shape: "tower", Floor: "water", Wall: "brick"}); err != nil {
		t.Fatal(err)
	}
	r, ok := FindRoom(a, "yard")
	if !ok || r.Look.Floor != "~" || r.Look.Border != "thick" || r.MinCols != 2 || r.MinRows != 4 {
		t.Fatalf("style not applied: %+v", r)
	}
}

func TestMagnetAdoptsIntoKennel(t *testing.T) {
	a, _ := setup(t)
	k := store.Kennel{Name: "Infra"}
	if err := a.SaveKennel(&k); err != nil {
		t.Fatal(err)
	}
	key := "kennel:" + itoa(k.ID)
	st := store.Style{Blocks: []store.Block{{Kind: blocks.Magnet, Config: map[string]string{"cwd": "/Users/demo/code/infra"}}}}
	if err := a.SaveStyle(key, st); err != nil {
		t.Fatal(err)
	}
	r, _ := FindRoom(a, key)
	titles := map[string]bool{}
	for _, p := range r.Pups {
		titles[p.Title()] = true
	}
	if !titles["PG-UPGRADE"] || !titles["COST-REPORT"] || titles["CHECKOUT-V2"] {
		t.Fatalf("magnet adopted %v", titles)
	}
}

func TestBellRingsOnlyInBelledRooms(t *testing.T) {
	a, rec := setup(t)
	a.SaveStyle("yard", store.Style{Blocks: []store.Block{{Kind: blocks.Bell, Config: map[string]string{"when": "waiting"}}}})
	flaky := pupNamed(t, a, "FLAKY-CI")
	old := pupNamed(t, a, "RATE-LIMITS")
	n := Ring(a, []app.Transition{
		{Pup: flaky, From: claude.Working, To: claude.Waiting},
		{Pup: flaky, From: claude.Working, To: claude.Done},
		{Pup: old, From: claude.Working, To: claude.Waiting},
	})
	if n != 1 {
		t.Fatalf("rang %d times, calls %v", n, rec.calls)
	}
	if !strings.Contains(rec.calls[0], "FLAKY-CI needs you") {
		t.Fatalf("notification: %v", rec.calls)
	}
}

func TestChuteArchivesSleepyPups(t *testing.T) {
	a, _ := setup(t)
	a.SaveStyle("lobby", store.Style{Blocks: []store.Block{{Kind: blocks.Chute, Config: map[string]string{"days": "2"}}}})
	before, _ := FindRoom(a, "lobby")
	moved := Sweep(a)
	if moved == 0 {
		t.Fatalf("expected archives from %d lobby pups", len(before.Pups))
	}
	for _, p := range before.Pups {
		got := a.Pup(p.ID).Meta.Archived
		want := now.Sub(p.Updated) >= 48*time.Hour
		if got != want {
			t.Errorf("%s archived=%v want %v", p.Title(), got, want)
		}
	}
}

func TestPortalRoutesIntoWorkspace(t *testing.T) {
	a, rec := setup(t)
	a.SaveStyle("yard", store.Style{Blocks: []store.Block{{Kind: blocks.Portal, Config: map[string]string{"workspace": "pack"}}}})
	p := pupNamed(t, a, "ONBOARDING")
	tgt := TargetFor(a, p, true, false)
	if tgt.Workspace != "pack" {
		t.Fatalf("workspace %q", tgt.Workspace)
	}
	if _, err := a.Opener().Open(tgt); err != nil {
		t.Fatal(err)
	}
	calls := strings.Join(rec.calls, "\n")
	if !strings.Contains(calls, "herdr workspace create --label pack") || !strings.Contains(calls, "--workspace w7") {
		t.Fatalf("calls:\n%s", calls)
	}
}

func TestDoorSpawnsWithPresets(t *testing.T) {
	a, _ := setup(t)
	r, _ := FindRoom(a, "project:/Users/demo/code/infra")
	door := store.Block{Kind: blocks.Door, Config: map[string]string{"name": "ONCALL", "model": "opus", "prompt": "check the alerts"}}
	tgt, err := Spawn(a, r, door)
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Cwd != "/Users/demo/code/infra" || strings.Join(tgt.ClaudeArgs(), " ") != "-n ONCALL --model opus check the alerts" {
		t.Fatalf("target %+v args %v", tgt, tgt.ClaudeArgs())
	}
}

func TestTemplate(t *testing.T) {
	a, _ := setup(t)
	if err := ApplyTemplate(a, "fire", blocks.Templates[0]); err != nil {
		t.Fatal(err)
	}
	st := a.Style("fire")
	if st.Shape != "office" || len(st.Decor) != 3 || len(st.OfKind(blocks.Door)) != 1 || st.OfKind(blocks.Door)[0].Get("name") != "ONCALL" {
		t.Fatalf("template: %+v", st)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestTwoChutesDontUndoEachOther(t *testing.T) {
	a, _ := setup(t)
	chute := store.Block{Kind: blocks.Chute, Config: map[string]string{"days": "2"}}
	a.SaveStyle("lobby", store.Style{Blocks: []store.Block{chute, chute}})
	Sweep(a)
	Sweep(a)
	r, _ := FindRoom(a, "dumpster")
	archived := 0
	for _, p := range r.Pups {
		if p.Meta.Archived {
			archived++
		}
	}
	if archived == 0 {
		t.Fatal("old lobby pups should end up archived")
	}
}
