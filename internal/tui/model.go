// Package tui is kennel's Bubble Tea interface.
package tui

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
	"github.com/razkevich/kennel/internal/app"
	"github.com/razkevich/kennel/internal/ask"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/floor"
	"github.com/razkevich/kennel/internal/gear"
	"github.com/razkevich/kennel/internal/indexer"
	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/search"
)

type Tab int

const (
	TabMap Tab = iota
	TabPups
	TabAsk
	TabJobs
	TabArtifacts
	TabGear
)

var tabNames = []string{"🧭 Map", "🐾 Pups", "💬 Ask", "⏰ Jobs", "🎨 Artifacts", "🔩 Gear"}

type Options struct {
	// Static disables background timers; tests drive the model explicitly.
	Static bool
	// NoHerdr skips polling herdr for pane status.
	NoHerdr bool
	// Clipboard copies text; defaults to pbcopy.
	Clipboard func(string) error
	// OpenURL opens a link; defaults to `open`.
	OpenURL  func(string) error
	StartTab Tab
}

type Model struct {
	app  *app.App
	opts Options
	w, h int

	tab   Tab
	frame int

	scopes       []scopeItem
	scopeIdx     int
	focusSidebar bool

	query     textinput.Model
	searching bool
	results   []search.Result
	cursor    int
	offset    int
	searchSeq int
	contentQ  string

	floor     int
	mapSel    string
	mapLayout []roomLayout

	askInput  textinput.Model
	askEv     ask.Event
	askQ      string
	askCh     chan ask.Event
	askCancel context.CancelFunc
	askCursor int

	jobsCursor int
	artCursor  int

	gearScopes []gear.Scope
	gearScope  int
	gearCursor int

	modal      *modal
	transcript *transcriptView
	help       bool

	toast      string
	toastErr   bool
	toastUntil time.Time

	syncing   bool
	syncDone  *atomic.Int64
	syncTotal *atomic.Int64

	liveN int

	lastClickID string
	lastClickAt time.Time
}

func New(a *app.App, opts Options) Model {
	q := textinput.New()
	q.Prompt = "🔎 "
	q.Placeholder = "search names, prompts, transcripts…  is:live repo:x since:7d"
	q.PromptStyle = sAccent
	q.TextStyle = sText
	q.PlaceholderStyle = sDim
	ai := textinput.New()
	ai.Prompt = "💬 "
	ai.Placeholder = "ask your pack anything… e.g. where did we fix the flaky login test?"
	ai.PromptStyle = sAccent
	ai.TextStyle = sText
	ai.PlaceholderStyle = sDim
	ai.CharLimit = 500
	if opts.Clipboard == nil {
		opts.Clipboard = func(s string) error {
			_, err := runWithInput("pbcopy", s)
			return err
		}
	}
	if opts.OpenURL == nil {
		opts.OpenURL = func(u string) error {
			_, err := a.Cfg.Runner.Run("open", u)
			return err
		}
	}
	m := Model{app: a, opts: opts, query: q, askInput: ai, tab: opts.StartTab,
		syncDone: &atomic.Int64{}, syncTotal: &atomic.Int64{}, w: 120, h: 40}
	m.floor = a.DefaultFloor("tui_floor")
	m.rebuildScopes()
	m.refreshResults()
	return m
}

// ---- messages ----

type animMsg struct{}
type liveTickMsg struct{}
type liveMsg struct{ transitions []app.Transition }
type syncMsg struct {
	p   indexer.Progress
	err error
}
type searchTickMsg struct{ seq int }
type searchResMsg struct {
	seq     int
	results []search.Result
	err     error
}
type askEventMsg struct {
	ev ask.Event
	ch chan ask.Event
}
type toastMsg struct {
	text string
	err  bool
}
type nameMsg struct {
	pup  *pack.Pup
	name string
	err  error
}
type openedMsg struct {
	text string
	err  error
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.syncCmd()}
	if !m.opts.Static {
		cmds = append(cmds, animTick(), liveTick())
	}
	return tea.Batch(cmds...)
}

func animTick() tea.Cmd {
	return tea.Tick(220*time.Millisecond, func(time.Time) tea.Msg { return animMsg{} })
}

func liveTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return liveTickMsg{} })
}

func (m *Model) syncCmd() tea.Cmd {
	m.syncing = true
	a, done, total := m.app, m.syncDone, m.syncTotal
	return func() tea.Msg {
		p, err := a.Sync(context.Background(), func(p indexer.Progress) {
			done.Store(int64(p.Done))
			total.Store(int64(p.Total))
		})
		if n := floor.Sweep(a); n > 0 {
			p.Changed += n
		}
		return syncMsg{p: p, err: err}
	}
}

func (m Model) refreshLiveCmd(withHerdr bool) tea.Cmd {
	a := m.app
	return func() tea.Msg {
		ts := a.RefreshLive(withHerdr)
		floor.Ring(a, ts)
		return liveMsg{transitions: ts}
	}
}

func (m *Model) setToast(text string, isErr bool) {
	m.toast, m.toastErr, m.toastUntil = text, isErr, time.Now().Add(4*time.Second)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case animMsg:
		m.frame++
		if !m.toastUntil.IsZero() && time.Now().After(m.toastUntil) {
			m.toast = ""
		}
		return m, animTick()
	case liveTickMsg:
		m.liveN++
		cmds := []tea.Cmd{m.refreshLiveCmd(!m.opts.NoHerdr && m.liveN%2 == 0)}
		if !m.syncing && m.liveN%3 == 0 {
			cmds = append(cmds, m.syncCmd())
		}
		return m, tea.Batch(cmds...)
	case liveMsg:
		for _, t := range msg.transitions {
			switch t.To {
			case claude.Waiting:
				m.setToast(fmt.Sprintf("🐕 %s needs you", t.Pup.Title()), false)
			default:
				m.setToast(fmt.Sprintf("🦴 %s finished", t.Pup.Title()), false)
			}
		}
		m.rebuildScopes()
		cmd := m.refreshResults()
		if m.opts.Static {
			return m, cmd
		}
		return m, tea.Batch(cmd, liveTick())
	case syncMsg:
		m.syncing = false
		if msg.err != nil {
			m.setToast("index: "+msg.err.Error(), true)
		}
		if msg.p.Changed > 0 {
			m.rebuildScopes()
			m.refreshResults()
		}
		return m, nil
	case searchTickMsg:
		if msg.seq != m.searchSeq {
			return m, nil
		}
		a, raw, seq := m.app, m.query.Value(), msg.seq
		return m, func() tea.Msg {
			res, err := a.Search(raw, true)
			return searchResMsg{seq: seq, results: res, err: err}
		}
	case searchResMsg:
		if msg.seq == m.searchSeq {
			m.results = m.scopeFilter(msg.results)
			m.contentQ = m.query.Value()
			m.clampCursor()
		}
		return m, nil
	case askEventMsg:
		if msg.ch != m.askCh {
			return m, nil
		}
		m.askEv = msg.ev
		if msg.ev.Done {
			m.askCh = nil
			return m, nil
		}
		return m, waitAsk(msg.ch)
	case toastMsg:
		m.setToast(msg.text, msg.err)
		return m, nil
	case nameMsg:
		if msg.err != nil {
			m.setToast("naming: "+msg.err.Error(), true)
			return m, nil
		}
		md := renameModal(msg.pup)
		md.input.SetValue(msg.name)
		md.input.CursorEnd()
		md.title = "✨ suggested name for " + msg.pup.Title()
		m.modal = md
		return m, nil
	case openedMsg:
		if msg.err != nil {
			m.setToast(msg.err.Error(), true)
		} else if msg.text != "" {
			m.setToast(msg.text, false)
		}
		m.app.Reload()
		m.refreshResults()
		return m, nil
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// ---- scopes (sidebar) ----

type scopeKind int

const (
	scopeSmart scopeKind = iota
	scopeProject
	scopeKennel
	scopeNewKennel
)

type scopeItem struct {
	kind   scopeKind
	key    string
	label  string
	icon   string
	color  string
	id     int64
	count  int
	unread int
	match  func(p *pack.Pup) bool
}

func (m *Model) rebuildScopes() {
	now := m.app.Now()
	pups := m.app.Pups()
	var scopes []scopeItem
	for i := range pack.Smarts {
		sm := &pack.Smarts[i]
		scopes = append(scopes, scopeItem{kind: scopeSmart, key: "smart:" + sm.Key, label: sm.Name, icon: sm.Icon, color: sm.Color,
			match: func(p *pack.Pup) bool { return sm.Match(p, now) }})
	}
	for _, pr := range m.app.Projects() {
		root := pr.Root
		scopes = append(scopes, scopeItem{kind: scopeProject, key: "project:" + root, label: pr.Name, icon: "📁", color: string(cSky),
			match: func(p *pack.Pup) bool { return app.ProjectRoot(p.Cwd) == root && !p.Meta.Archived && !p.Empty() }})
	}
	for _, k := range m.app.Kennels() {
		id := k.ID
		icon := k.Icon
		if icon == "" {
			icon = "🏠"
		}
		scopes = append(scopes, scopeItem{kind: scopeKennel, key: fmt.Sprintf("kennel:%d", id), label: k.Name, icon: icon, color: k.Color, id: id,
			match: func(p *pack.Pup) bool { return p.Kennel != nil && p.Kennel.ID == id && !p.Meta.Archived }})
	}
	scopes = append(scopes, scopeItem{kind: scopeNewKennel, key: "new-kennel", label: "new kennel", icon: "＋", match: func(*pack.Pup) bool { return false }})
	for i := range scopes {
		for _, p := range pups {
			if scopes[i].match(p) {
				scopes[i].count++
				if p.Unread() {
					scopes[i].unread++
				}
			}
		}
	}
	prev := ""
	if m.scopeIdx < len(m.scopes) {
		prev = m.scopes[m.scopeIdx].key
	}
	m.scopes = scopes
	m.scopeIdx = 0
	for i, s := range scopes {
		if s.key == prev {
			m.scopeIdx = i
		}
	}
}

func (m *Model) scope() scopeItem {
	if m.scopeIdx < len(m.scopes) {
		return m.scopes[m.scopeIdx]
	}
	return m.scopes[0]
}

func (m *Model) scopeFilter(rs []search.Result) []search.Result {
	sc := m.scope()
	q := search.Parse(m.query.Value())
	out := rs[:0:0]
	for _, r := range rs {
		inScope := sc.match(r.Pup)
		// An explicit search reaches archived/empty pups too, but stays inside a project/kennel scope.
		if !inScope && !(sc.key == "smart:all" && !q.Empty()) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// refreshResults re-ranks with metadata only (instant), then schedules a content search.
func (m *Model) refreshResults() tea.Cmd {
	selID := ""
	if m.cursor >= 0 && m.cursor < len(m.results) {
		selID = m.results[m.cursor].Pup.ID
	}
	defer func() {
		for i, r := range m.results {
			if r.Pup.ID == selID {
				m.cursor = i
			}
		}
	}()
	res, _ := m.app.Search(m.query.Value(), false)
	keep := map[string][]search.Result{}
	if m.contentQ == m.query.Value() {
		for _, r := range m.results {
			keep[r.Pup.ID] = append(keep[r.Pup.ID], r)
		}
	}
	m.results = m.scopeFilter(res)
	for i, r := range m.results {
		if prev, ok := keep[r.Pup.ID]; ok && len(prev[0].Hits) > 0 {
			m.results[i].Hits = prev[0].Hits
		}
	}
	m.clampCursor()
	if len(search.Parse(m.query.Value()).Terms) == 0 {
		m.contentQ = m.query.Value()
		return nil
	}
	m.searchSeq++
	seq := m.searchSeq
	if m.opts.Static {
		return func() tea.Msg { return searchTickMsg{seq: seq} }
	}
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return searchTickMsg{seq: seq} })
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.results) {
		m.cursor = len(m.results) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) selected() *pack.Pup {
	if m.tab == TabMap {
		return m.app.Pup(mapSelection(m.mapSel, m.mapGeometry()))
	}
	if m.cursor < len(m.results) && m.cursor >= 0 {
		return m.results[m.cursor].Pup
	}
	return nil
}

func (m *Model) selectPup(id string) {
	m.mapSel = id
	for i, r := range m.results {
		if r.Pup.ID == id {
			m.cursor = i
			return
		}
	}
	// Not visible in this scope: widen to All and clear the query.
	m.scopeIdx = 0
	m.query.SetValue("")
	m.refreshResults()
	for i, r := range m.results {
		if r.Pup.ID == id {
			m.cursor = i
		}
	}
}

// ---- actions ----

func (m *Model) open(p *pack.Pup, fork, remote bool) tea.Cmd {
	if p == nil {
		return nil
	}
	_ = m.app.MarkSeen(p.ID)
	t := floor.TargetFor(m.app, p, fork, remote)
	if err := m.app.CheckCwd(t.Cwd); err != nil {
		return toast(p.Title()+": "+err.Error(), true)
	}
	if opener.Resolve(m.app.Cfg.Opener) == opener.Inline {
		if p.Status.Live() && !fork {
			return toast(p.Title()+" is already running in another terminal", true)
		}
		return tea.ExecProcess(opener.InlineCmd(t), func(err error) tea.Msg {
			return openedMsg{text: "back in the kennel 🐾", err: err}
		})
	}
	o := m.app.Opener()
	return func() tea.Msg {
		text, err := o.Open(t)
		return openedMsg{text: text, err: err}
	}
}

func (m *Model) newSession(cwd, title string) tea.Cmd {
	t := opener.Target{Cwd: cwd, Title: title, New: true}
	if opener.Resolve(m.app.Cfg.Opener) == opener.Inline {
		return tea.ExecProcess(opener.InlineCmd(t), func(err error) tea.Msg {
			return openedMsg{text: "back in the kennel 🐾", err: err}
		})
	}
	o := m.app.Opener()
	return func() tea.Msg {
		text, err := o.Open(t)
		return openedMsg{text: text, err: err}
	}
}

func toast(text string, isErr bool) tea.Cmd {
	return func() tea.Msg { return toastMsg{text: text, err: isErr} }
}

func (m *Model) startAsk(q string) tea.Cmd {
	if strings.TrimSpace(q) == "" {
		return nil
	}
	if m.askCancel != nil {
		m.askCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.askCancel = cancel
	ch := make(chan ask.Event, 64)
	m.askCh = ch
	m.askQ = q
	m.askEv = ask.Event{Stage: "sniffing around"}
	m.askCursor = 0
	asker := m.app.Asker()
	go func() {
		asker.Ask(ctx, q, func(ev ask.Event) {
			select {
			case ch <- ev:
			case <-ctx.Done():
			}
		})
	}()
	return waitAsk(ch)
}

func waitAsk(ch chan ask.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return askEventMsg{ev: ev, ch: ch}
	}
}

// Zone ids are stable strings so tests can click on them.
func ZonePup(id string) string    { return "pup:" + id }
func ZoneRow(id string) string    { return "row:" + id }
func ZoneScope(key string) string { return "scope:" + key }
func ZoneTab(t Tab) string        { return fmt.Sprintf("tab:%d", t) }
func ZoneThing(key string) string { return "thing:" + key }

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.modal != nil || m.transcript != nil {
		if m.transcript != nil && msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.transcript.scroll(-3)
			case tea.MouseButtonWheelDown:
				m.transcript.scroll(3)
			}
		}
		return m, nil
	}
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.moveCursor(-1)
		return m, nil
	case tea.MouseButtonWheelDown:
		m.moveCursor(1)
		return m, nil
	case tea.MouseButtonLeft:
	default:
		return m, nil
	}
	for t := range tabNames {
		if zone.Get(ZoneTab(Tab(t))).InBounds(msg) {
			m.tab = Tab(t)
			return m, m.enterTab()
		}
	}
	for i, s := range m.scopes {
		if zone.Get(ZoneScope(s.key)).InBounds(msg) {
			return m.pickScope(i)
		}
	}
	for _, t := range m.app.Things() {
		if zone.Get(ZoneThing(t.Key())).InBounds(msg) {
			double := m.lastClickID == t.Key() && time.Since(m.lastClickAt) < 450*time.Millisecond
			m.lastClickID, m.lastClickAt = t.Key(), time.Now()
			m.mapSel = t.Key()
			if double {
				mm, cmd, _ := m.thingActions("enter", t)
				return mm, cmd
			}
			return m, nil
		}
	}
	for _, p := range m.app.Pups() {
		if zone.Get(ZonePup(p.ID)).InBounds(msg) || zone.Get(ZoneRow(p.ID)).InBounds(msg) {
			double := m.lastClickID == p.ID && time.Since(m.lastClickAt) < 450*time.Millisecond
			m.lastClickID, m.lastClickAt = p.ID, time.Now()
			m.selectPup(p.ID)
			if double {
				return m, m.open(p, false, false)
			}
			return m, nil
		}
	}
	return m, nil
}

func (m Model) pickScope(i int) (tea.Model, tea.Cmd) {
	if m.scopes[i].kind == scopeNewKennel {
		m.modal = newKennelModal()
		return m, nil
	}
	m.scopeIdx = i
	m.cursor, m.offset = 0, 0
	return m, m.refreshResults()
}

func (m *Model) enterTab() tea.Cmd {
	switch m.tab {
	case TabGear:
		var roots []string
		for _, pr := range m.app.Projects() {
			roots = append(roots, pr.Root)
		}
		m.gearScopes = gear.Inventory(m.app.Cfg.ClaudeDir, roots)
	}
	return nil
}

func (m *Model) moveCursor(d int) {
	switch m.tab {
	case TabMap:
		m.moveMap(d, 0)
	case TabPups:
		if m.focusSidebar {
			m.scopeIdx = clamp(m.scopeIdx+d, 0, len(m.scopes)-1)
			m.cursor, m.offset = 0, 0
			m.refreshResults()
			return
		}
		m.cursor = clamp(m.cursor+d, 0, len(m.results)-1)
	case TabAsk:
		m.askCursor = clamp(m.askCursor+d, 0, max(0, len(m.askEv.Sources)-1))
	case TabJobs:
		m.jobsCursor = clamp(m.jobsCursor+d, 0, max(0, len(m.jobRows())-1))
	case TabArtifacts:
		m.artCursor = clamp(m.artCursor+d, 0, max(0, len(m.app.Artifacts())-1))
	case TabGear:
		items := m.gearItems()
		m.gearCursor = clamp(m.gearCursor+d, 0, max(0, len(items)-1))
	}
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return max(lo, min(v, hi))
}
