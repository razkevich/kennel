// Package app is the shared core behind the TUI, the web map and the CLI.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/razkevich/kennel/internal/ask"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/demo"
	"github.com/razkevich/kennel/internal/indexer"
	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/search"
	"github.com/razkevich/kennel/internal/store"
)

type Config struct {
	ClaudeDir string
	Home      string
	Opener    opener.Kind
	Runner    opener.Runner
	LLM       ask.LLM
	Now       func() time.Time
	// Alive overrides pid liveness checks (demo data and tests).
	Alive func(pid int) bool
	// Demo is set for synthetic data: project folders needn't exist.
	Demo bool
}

type App struct {
	Cfg     Config
	Store   *store.Store
	Indexer *indexer.Indexer

	mu      sync.RWMutex
	pups    []*pack.Pup
	kennels []store.Kennel
	jobs    []claude.Job
	herdr   []opener.HerdrAgent
	live    map[string]claude.Live
}

func DefaultConfig() Config {
	cfg := Config{
		ClaudeDir: claude.Dir(),
		Home:      store.Home(),
		Opener:    opener.Kind(os.Getenv("KENNEL_OPENER")),
		Runner:    opener.ExecRunner{},
		LLM:       ask.ClaudeCLI{},
		Now:       time.Now,
	}
	if cfg.Opener == "" {
		cfg.Opener = opener.Auto
	}
	return cfg
}

func New(cfg Config) (*App, error) {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Runner == nil {
		cfg.Runner = opener.ExecRunner{}
	}
	if cfg.Alive == nil {
		if _, err := os.Stat(filepath.Join(cfg.ClaudeDir, demo.Marker)); err == nil {
			cfg.Alive = func(int) bool { return true }
			cfg.Demo = true
		}
	}
	st, err := store.Open(filepath.Join(cfg.Home, "kennel.db"))
	if err != nil {
		return nil, err
	}
	a := &App{Cfg: cfg, Store: st, Indexer: &indexer.Indexer{Store: st, ClaudeDir: cfg.ClaudeDir}}
	if err := a.Reload(); err != nil {
		st.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) Close() error { return a.Store.Close() }

func (a *App) Now() time.Time { return a.Cfg.Now() }

// Sync indexes changed transcripts and reloads the in-memory snapshot if anything changed.
func (a *App) Sync(ctx context.Context, progress func(indexer.Progress)) (indexer.Progress, error) {
	p, err := a.Indexer.Sync(ctx, progress)
	if p.Changed > 0 || p.Total > 0 {
		if rerr := a.Reload(); rerr != nil && err == nil {
			err = rerr
		}
	}
	return p, err
}

// Reload rebuilds pups from the store plus current live state.
func (a *App) Reload() error {
	sessions, err := a.Store.Sessions()
	if err != nil {
		return err
	}
	meta, err := a.Store.Meta()
	if err != nil {
		return err
	}
	kennels, err := a.Store.Kennels()
	if err != nil {
		return err
	}
	live := claude.ReadLive(a.Cfg.ClaudeDir, a.Cfg.Alive)
	jobs := claude.ReadJobs(a.Cfg.ClaudeDir)
	a.mu.RLock()
	herdr := a.herdr
	a.mu.RUnlock()
	pups := pack.Build(sessions, meta, kennels, live)
	applyHerdr(pups, herdr)
	a.mu.Lock()
	a.pups, a.kennels, a.live, a.jobs = pups, kennels, live, jobs
	a.mu.Unlock()
	return nil
}

// RefreshLive re-reads the live registry and herdr state; cheap enough to poll every second.
// It returns status transitions worth telling the user about.
func (a *App) RefreshLive(withHerdr bool) []Transition {
	live := claude.ReadLive(a.Cfg.ClaudeDir, a.Cfg.Alive)
	var herdr []opener.HerdrAgent
	if withHerdr {
		herdr = opener.HerdrAgents(a.Cfg.Runner)
	}
	jobs := claude.ReadJobs(a.Cfg.ClaudeDir)
	a.mu.Lock()
	defer a.mu.Unlock()
	if !withHerdr {
		herdr = a.herdr
	}
	var out []Transition
	for _, p := range a.pups {
		prev := p.Status
		p.Live, p.Status, p.Pane = live[p.ID], live[p.ID].Status, ""
		if _, ok := live[p.ID]; !ok {
			p.Status = claude.Offline
		}
		applyHerdrOne(p, herdr)
		if prev != p.Status && prev != claude.Offline && (p.Status == claude.Waiting || p.Status == claude.Done || (prev == claude.Working && p.Status == claude.Idle)) {
			out = append(out, Transition{Pup: p, From: prev, To: p.Status})
		}
	}
	a.live, a.herdr, a.jobs = live, herdr, jobs
	return out
}

type Transition struct {
	Pup      *pack.Pup
	From, To claude.Status
}

func applyHerdr(pups []*pack.Pup, agents []opener.HerdrAgent) {
	for _, p := range pups {
		applyHerdrOne(p, agents)
	}
}

func applyHerdrOne(p *pack.Pup, agents []opener.HerdrAgent) {
	for _, h := range agents {
		match := h.SessionID == p.ID
		if !match && h.SessionID == "" && p.Status.Live() && h.Title != "" {
			match = strings.EqualFold(h.Title, p.Title()) && (h.Cwd == p.Live.Cwd || h.Cwd == p.LastCwd || h.Cwd == p.Cwd)
		}
		if !match {
			continue
		}
		p.Pane = h.Pane
		if h.Status != claude.Idle || p.Status == claude.Offline {
			p.Status = h.Status
		}
		return
	}
}

func (a *App) Pups() []*pack.Pup {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.pups
}

func (a *App) Kennels() []store.Kennel {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.kennels
}

func (a *App) Jobs() []claude.Job {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.jobs
}

func (a *App) Pup(id string) *pack.Pup {
	for _, p := range a.Pups() {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Find resolves a session by id, id prefix, or exact title.
func (a *App) Find(ref string) *pack.Pup {
	ref = strings.TrimSpace(ref)
	var prefix *pack.Pup
	for _, p := range a.Pups() {
		if p.ID == ref || strings.EqualFold(p.Title(), ref) {
			return p
		}
		if len(ref) >= 4 && strings.HasPrefix(p.ID, ref) && prefix == nil {
			prefix = p
		}
	}
	return prefix
}

// Search ranks pups for a query, including full-text hits.
func (a *App) Search(raw string, withContent bool) ([]search.Result, error) {
	q := search.Parse(raw)
	var hits []store.Hit
	var err error
	if withContent && len(q.Terms) > 0 {
		if m := search.FTSMatch(q.Terms); m != "" {
			hits, err = a.Store.Search(m, 300)
		}
	}
	return search.Rank(a.Pups(), q, hits, a.Now()), err
}

func (a *App) Asker() *ask.Asker {
	return &ask.Asker{Store: a.Store, LLM: a.Cfg.LLM, Describe: func(id string) (string, string) {
		if p := a.Pup(id); p != nil {
			return p.Title(), p.Repo()
		}
		return id[:min(8, len(id))], ""
	}}
}

// Projects are the distinct repo roots sessions ran in, most recently used first.
func (a *App) Projects() []Project {
	seen := map[string]*Project{}
	var out []*Project
	for _, p := range a.Pups() {
		root := ProjectRoot(p.Cwd)
		if root == "" {
			continue
		}
		pr := seen[root]
		if pr == nil {
			pr = &Project{Root: root, Name: filepath.Base(root)}
			seen[root] = pr
			out = append(out, pr)
		}
		pr.Count++
		if p.Status.Live() {
			pr.Live++
		}
		if p.Updated.After(pr.Updated) {
			pr.Updated = p.Updated
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	res := make([]Project, len(out))
	for i, p := range out {
		res[i] = *p
	}
	return res
}

type Project struct {
	Root    string
	Name    string
	Count   int
	Live    int
	Updated time.Time
}

func ProjectRoot(cwd string) string {
	if i := strings.Index(cwd, "/.claude/worktrees/"); i >= 0 {
		return cwd[:i]
	}
	return cwd
}

// ---- mutations ----

func (a *App) updateMeta(id string, f func(*store.PupMeta)) error {
	p := a.Pup(id)
	if p == nil {
		return fmt.Errorf("unknown session %s", id)
	}
	m := p.Meta
	m.SessionID = id
	f(&m)
	if err := a.Store.SaveMeta(m); err != nil {
		return err
	}
	a.mu.Lock()
	p.Meta = m
	a.mu.Unlock()
	return nil
}

func (a *App) ToggleStar(id string) error {
	return a.updateMeta(id, func(m *store.PupMeta) { m.Starred = !m.Starred })
}

func (a *App) ToggleArchive(id string) error {
	return a.updateMeta(id, func(m *store.PupMeta) { m.Archived = !m.Archived })
}

func (a *App) MarkSeen(id string) error {
	return a.updateMeta(id, func(m *store.PupMeta) { m.SeenAt = a.Now() })
}

func (a *App) SetNote(id, note string) error {
	return a.updateMeta(id, func(m *store.PupMeta) { m.Note = note })
}

func (a *App) MoveTo(id string, kennelID int64) error {
	if err := a.updateMeta(id, func(m *store.PupMeta) { m.KennelID = kennelID }); err != nil {
		return err
	}
	return a.Reload()
}

// Rename stores a kennel alias and, when the session isn't running, also appends
// the same custom-title record /rename writes so `claude --resume` shows it.
func (a *App) Rename(id, title string) error {
	p := a.Pup(id)
	if p == nil {
		return fmt.Errorf("unknown session %s", id)
	}
	if err := a.updateMeta(id, func(m *store.PupMeta) { m.Alias = title }); err != nil {
		return err
	}
	if p.Status.Live() || title == "" {
		return nil
	}
	f, err := os.OpenFile(p.Path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, rec := range []map[string]string{
		{"type": "custom-title", "customTitle": title, "sessionId": id},
		{"type": "agent-name", "agentName": title, "sessionId": id},
	} {
		b, _ := json.Marshal(rec)
		if _, err := f.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// Trash moves a transcript into kennel's trash; nothing is deleted.
func (a *App) Trash(id string) (string, error) {
	p := a.Pup(id)
	if p == nil {
		return "", fmt.Errorf("unknown session %s", id)
	}
	if p.Status.Live() {
		return "", fmt.Errorf("%s is running; stop it first", p.Title())
	}
	dst := filepath.Join(a.Cfg.Home, "trash", p.Project, filepath.Base(p.Path))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(p.Path, dst); err != nil {
		return "", err
	}
	if err := a.Store.RemovePath(p.Path); err != nil {
		return dst, err
	}
	return dst, a.Reload()
}

func (a *App) SaveKennel(k *store.Kennel) error {
	if err := a.Store.SaveKennel(k); err != nil {
		return err
	}
	return a.Reload()
}

func (a *App) DeleteKennel(id int64) error {
	if err := a.Store.DeleteKennel(id); err != nil {
		return err
	}
	return a.Reload()
}

// CheckCwd reports whether a session's folder still exists.
func (a *App) CheckCwd(cwd string) error {
	if a.Cfg.Demo {
		return nil
	}
	if _, err := os.Stat(cwd); err != nil {
		return fmt.Errorf("folder %s is gone", cwd)
	}
	return nil
}

// Target builds what the opener needs for a pup.
func (a *App) Target(p *pack.Pup, fork, remote bool) opener.Target {
	cwd := p.Cwd
	if _, err := os.Stat(cwd); err != nil && p.LastCwd != "" {
		cwd = p.LastCwd
	}
	return opener.Target{SessionID: p.ID, Cwd: cwd, Title: p.Title(), Fork: fork, Pane: p.Pane, Live: p.Status.Live(), Remote: remote}
}

func (a *App) Opener() opener.Opener {
	return opener.Opener{Kind: a.Cfg.Opener, Runner: a.Cfg.Runner}
}

// Prompt sends text to a live pup through herdr.
func (a *App) Prompt(p *pack.Pup, text string) error {
	if p.Pane == "" {
		return fmt.Errorf("%s isn't in a herdr pane kennel can reach", p.Title())
	}
	return opener.Prompt(a.Cfg.Runner, p.Pane, text)
}

// Artifacts across all sessions, newest first.
func (a *App) Artifacts() []ArtifactRef {
	var out []ArtifactRef
	for _, p := range a.Pups() {
		for _, art := range p.Artifacts {
			out = append(out, ArtifactRef{Artifact: art, Pup: p})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

type ArtifactRef struct {
	claude.Artifact
	Pup *pack.Pup
}
