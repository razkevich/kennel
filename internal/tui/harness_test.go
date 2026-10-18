package tui

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/razkevich/kennel/internal/app"
	"github.com/razkevich/kennel/internal/demo"
	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/pack"
)

var testNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func init() { zone.NewGlobal() }

// recorder stands in for herdr/osascript/open so tests never touch real terminals or sessions.
type recorder struct {
	mu     sync.Mutex
	calls  []string
	agents string
}

func (r *recorder) Run(name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	if name == "herdr" && len(args) >= 2 {
		switch args[0] + " " + args[1] {
		case "tab create":
			return []byte(`{"result":{"root_pane":{"pane_id":"w1:p9"}}}`), nil
		case "agent list":
			return []byte(r.agents), nil
		}
	}
	return []byte("{}"), nil
}

func (r *recorder) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

type fakeLLM struct{}

func (fakeLLM) Complete(_ context.Context, _ string, prompt string, onText func(string)) (string, error) {
	if strings.Contains(prompt, "names workstreams") {
		return "pg upgrade", nil
	}
	if strings.Contains(prompt, "search keywords") {
		return "stripe, PaymentElement, flicker, safari", nil
	}
	answer := "The flicker came from remounting the payment element; memoizing options fixed it [1]."
	for _, part := range strings.SplitAfter(answer, " ") {
		if onText != nil {
			onText(part)
		}
	}
	return answer, nil
}

type harness struct {
	t     *testing.T
	app   *app.App
	m     tea.Model
	rec   *recorder
	dir   string
	clip  []string
	urls  []string
	queue []tea.Cmd
}

func newHarness(t *testing.T, kind opener.Kind) *harness {
	t.Helper()
	dir := t.TempDir()
	cdir := filepath.Join(dir, "claude")
	if err := demo.Generate(cdir, testNow); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	a, err := app.New(app.Config{ClaudeDir: cdir, Home: filepath.Join(dir, "home"), Opener: kind, Runner: rec, LLM: fakeLLM{},
		Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	if _, err := a.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, app: a, rec: rec, dir: dir}
	h.m = New(a, Options{Static: true, NoHerdr: true,
		Clipboard: func(s string) error { h.clip = append(h.clip, s); return nil },
		OpenURL:   func(u string) error { h.urls = append(h.urls, u); return nil }})
	h.send(tea.WindowSizeMsg{Width: 150, Height: 46})
	return h
}

// send delivers a message and synchronously drains resulting commands (except
// timers and process handoffs), like a real program loop would.
func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	var cmd tea.Cmd
	h.m, cmd = h.m.Update(msg)
	h.run(cmd, 0)
}

func (h *harness) run(cmd tea.Cmd, depth int) {
	if cmd == nil || depth > 500 {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(40 * time.Millisecond):
		// Timers (search debounce) are skipped; tests trigger them explicitly.
		return
	}
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c, depth+1)
		}
	case searchTickMsg:
		h.send(msg)
	default:
		if isExec(msg) {
			h.queue = append(h.queue, cmd)
			return
		}
		var next tea.Cmd
		h.m, next = h.m.Update(msg)
		h.run(next, depth+1)
	}
}

func isExec(msg tea.Msg) bool {
	return strings.Contains(strings.ToLower(typeName(msg)), "exec")
}

func typeName(v any) string { return strings.TrimPrefix(strings.TrimPrefix(fmtType(v), "*"), "tea.") }

func (h *harness) key(keys ...string) {
	h.t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		case "space":
			msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		case "ctrl+k":
			msg = tea.KeyMsg{Type: tea.KeyCtrlK}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		h.send(msg)
	}
}

func (h *harness) typeText(s string) {
	h.t.Helper()
	for _, r := range s {
		if r == ' ' {
			h.send(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;:]*[a-zA-Z]`)

// screen is the rendered frame with styling stripped, as a user would read it.
func (h *harness) screen() string {
	return ansiRe.ReplaceAllString(h.m.View(), "")
}

func (h *harness) model() Model { return h.m.(Model) }

func (h *harness) contains(want ...string) {
	h.t.Helper()
	s := h.screen()
	for _, w := range want {
		if !strings.Contains(s, w) {
			h.t.Fatalf("screen missing %q:\n%s", w, s)
		}
	}
}

func (h *harness) lacks(bad ...string) {
	h.t.Helper()
	s := h.screen()
	for _, b := range bad {
		if strings.Contains(s, b) {
			h.t.Fatalf("screen unexpectedly has %q:\n%s", b, s)
		}
	}
}

// click renders the frame, waits for bubblezone to register the zone, then clicks its center.
func (h *harness) click(id string) {
	h.t.Helper()
	h.m.View()
	deadline := time.Now().Add(2 * time.Second)
	for {
		z := zone.Get(id)
		if z != nil && !z.IsZero() {
			x := (z.StartX + z.EndX) / 2
			y := (z.StartY + z.EndY) / 2
			h.send(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("zone %q never appeared", id)
		}
		time.Sleep(5 * time.Millisecond)
		h.m.View()
	}
}

func (h *harness) pup(title string) *pack.Pup {
	h.t.Helper()
	for _, p := range h.app.Pups() {
		if p.Title() == title {
			return p
		}
	}
	h.t.Fatalf("no pup %q", title)
	return nil
}

func (h *harness) selectedTitle() string {
	m := h.model()
	if p := m.selected(); p != nil {
		return p.Title()
	}
	return ""
}

func readFile(t *testing.T, path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
