package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/razkevich/kennel/internal/ask"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/digest"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/things"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.modal != nil {
		return m.updateModal(msg)
	}
	if m.transcript != nil {
		return m.updateTranscript(msg)
	}
	if m.help {
		m.help = false
		return m, nil
	}

	if m.searching {
		switch key {
		case "esc":
			m.searching = false
			m.query.Blur()
			if m.query.Value() != "" {
				m.query.SetValue("")
				return m, m.refreshResults()
			}
			return m, nil
		case "enter", "down", "up", "ctrl+n", "ctrl+p":
			m.searching = false
			m.query.Blur()
			if key == "enter" {
				return m, nil
			}
			m.moveCursor(map[string]int{"down": 1, "ctrl+n": 1, "up": -1, "ctrl+p": -1}[key])
			return m, nil
		}
		var cmd tea.Cmd
		before := m.query.Value()
		m.query, cmd = m.query.Update(msg)
		if m.query.Value() != before {
			m.cursor, m.offset = 0, 0
			return m, tea.Batch(cmd, m.refreshResults())
		}
		return m, cmd
	}

	if m.tab == TabAsk && m.askInput.Focused() {
		switch key {
		case "esc":
			m.askInput.Blur()
			return m, nil
		case "enter":
			q := m.askInput.Value()
			m.askInput.Blur()
			return m, m.startAsk(q)
		case "down":
			m.askInput.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.askInput, cmd = m.askInput.Update(msg)
		return m, cmd
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "1", "2", "3", "4", "5", "6":
		if m.tab == TabAsk && len(m.askEv.Sources) > 0 && m.askEv.Done {
			// Number keys jump to answer citations while reading an answer.
			n := int(key[0] - '0')
			for _, s := range m.askEv.Sources {
				if s.N == n {
					return m.jumpTo(s.SessionID)
				}
			}
		}
		m.tab = Tab(key[0] - '1')
		return m, m.enterTab()
	case "]", "L":
		m.tab = (m.tab + 1) % Tab(len(tabNames))
		return m, m.enterTab()
	case "[", "H":
		m.tab = (m.tab + Tab(len(tabNames)) - 1) % Tab(len(tabNames))
		return m, m.enterTab()
	case "?", "f1":
		m.help = true
		return m, nil
	case "a":
		m.tab = TabAsk
		return m, m.askInput.Focus()
	case "/":
		if m.tab == TabAsk {
			return m, m.askInput.Focus()
		}
		if m.tab != TabPups {
			m.tab = TabPups
		}
		m.searching = true
		m.focusSidebar = false
		return m, m.query.Focus()
	case "ctrl+k", "ctrl+p":
		m.modal = m.switcherModal()
		return m, nil
	case "u":
		return m.jumpUnread()
	case "d":
		d := digest.Build(m.app.Pups(), m.app.Now(), 24*time.Hour)
		m.modal = &modal{title: "📰 the last 24 hours", help: "kennel digest -since 168h for a week", confirm: false, info: d.Lines(),
			onSubmit: func(*Model, string, string) tea.Cmd { return nil }}
		return m, nil
	case "B":
		if md := m.buildModal(); md != nil {
			m.modal = md
			return m, nil
		}
		return m, toast("pick a room first (a smart kennel, project or kennel)", true)
	case "esc":
		if m.query.Value() != "" {
			m.query.SetValue("")
			return m, m.refreshResults()
		}
		return m, nil
	}

	switch m.tab {
	case TabMap:
		return m.mapKeys(key)
	case TabPups:
		return m.pupKeys(key)
	case TabAsk:
		return m.askKeys(key)
	case TabJobs:
		return m.jobKeys(key)
	case TabArtifacts:
		return m.artKeys(key)
	case TabGear:
		return m.gearKeys(key)
	}
	return m, nil
}

func (m Model) jumpTo(id string) (tea.Model, tea.Cmd) {
	m.tab = TabPups
	m.focusSidebar = false
	m.selectPup(id)
	return m, nil
}

func (m Model) jumpUnread() (tea.Model, tea.Cmd) {
	var best *pack.Pup
	for _, p := range m.app.Pups() {
		if !p.Unread() {
			continue
		}
		if best == nil || (p.Status == claude.Waiting && best.Status != claude.Waiting) ||
			(p.Status == best.Status && p.Live.Since.Before(best.Live.Since)) {
			best = p
		}
	}
	if best == nil {
		m.setToast("no pups need you right now 🐾", false)
		return m, nil
	}
	if m.tab != TabMap {
		m.tab = TabPups
	}
	m.selectPup(best.ID)
	return m, nil
}

// pupActions are shared between the map and the list.
func (m Model) pupActions(key string, p *pack.Pup) (tea.Model, tea.Cmd, bool) {
	if p == nil {
		return m, nil, false
	}
	switch key {
	case "enter", "o":
		return m, m.open(p, false, false), true
	case "f":
		return m, m.open(p, true, false), true
	case "R":
		return m, m.open(p, false, true), true
	case "C":
		if p.AgentKind() != "claude" {
			return m, toast("clone works on Claude sessions; use c to convert", true), true
		}
		m.modal = m.cloneModal(p)
		return m, nil, true
	case "c":
		m.modal = m.convertModal(p)
		return m, nil, true
	case "+":
		m.modal = m.feedModal(p)
		return m, nil, true
	case "E":
		path, err := m.app.Export(p)
		if err != nil {
			return m, toast(err.Error(), true), true
		}
		return m, m.copy(path, "exported to "+path+" (path copied)"), true
	case "M":
		m.modal = m.mergeModal(p)
		return m, nil, true
	case "Z":
		m.modal = textModal("✨ distill "+p.Title(), "a fresh pup that starts from a brief of this one (recap, asks, PRs, transcript path)", p.Title()+"-FRESH", "NAME",
			func(m *Model, text, _ string) tea.Cmd { return m.startMerged([]string{p.ID}, text) })
		return m, nil, true
	case "s":
		if err := m.app.ToggleStar(p.ID); err != nil {
			return m, toast(err.Error(), true), true
		}
		m.rebuildScopes()
		verb := "unstarred"
		if m.app.Pup(p.ID).Meta.Starred {
			verb = "★ starred"
		}
		return m, tea.Batch(m.refreshResults(), toast(verb+" "+p.Title(), false)), true
	case "x":
		if err := m.app.ToggleArchive(p.ID); err != nil {
			return m, toast(err.Error(), true), true
		}
		m.rebuildScopes()
		verb := "restored"
		if m.app.Pup(p.ID).Meta.Archived {
			verb = "🚮 archived"
		}
		return m, tea.Batch(m.refreshResults(), toast(verb+" "+p.Title(), false)), true
	case "r":
		m.modal = renameModal(p)
		return m, nil, true
	case "m":
		m.modal = m.moveModal(p)
		return m, nil, true
	case "t":
		if !p.Status.Live() || p.Pane == "" {
			return m, toast("quick reply needs a running pup in a herdr pane", true), true
		}
		m.modal = replyModal(p)
		return m, nil, true
	case "N":
		m.modal = noteModal(p)
		return m, nil, true
	case "T":
		llm := m.app.Cfg.LLM
		return m, tea.Batch(toast("🐕 sniffing out a name…", false), func() tea.Msg {
			name, err := ask.SuggestName(context.Background(), llm, p.FirstPrompt, p.Recap, p.LastPrompt)
			return nameMsg{pup: p, name: name, err: err}
		}), true
	case "y":
		return m, m.copy(p.ID, "copied session id"), true
	case "Y":
		return m, m.copy(m.app.Target(p, false, false).ShellCommand(), "copied resume command"), true
	case "p":
		if len(p.PRs) == 0 {
			return m, toast("no PR linked to "+p.Title(), true), true
		}
		return m, m.openURL(p.PRs[len(p.PRs)-1]), true
	case "A":
		if len(p.Artifacts) == 0 {
			return m, toast("no artifacts in "+p.Title(), true), true
		}
		return m, m.openURL(p.Artifacts[len(p.Artifacts)-1].URL), true
	case "v", " ":
		m.transcript = newTranscript(p, m.contentQ)
		return m, nil, true
	case "D":
		m.modal = trashModal(p)
		return m, nil, true
	case "n":
		if cmd, ok := m.spawnFromDoor(); ok {
			return m, cmd, true
		}
		m.modal = newSessionModal(m.app.Target(p, false, false).Cwd)
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) copy(text, label string) tea.Cmd {
	cp := m.opts.Clipboard
	return func() tea.Msg {
		if err := cp(text); err != nil {
			return toastMsg{text: "clipboard: " + err.Error(), err: true}
		}
		return toastMsg{text: "📋 " + label}
	}
}

func (m *Model) openURL(u string) tea.Cmd {
	open := m.opts.OpenURL
	return func() tea.Msg {
		if err := open(u); err != nil {
			return toastMsg{text: err.Error(), err: true}
		}
		return toastMsg{text: "🔗 opened " + u}
	}
}

func (m Model) mapKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h":
		m.moveMap(-1, 0)
	case "right", "l":
		m.moveMap(1, 0)
	case "up", "k":
		m.moveMap(0, -1)
	case "down", "j":
		m.moveMap(0, 1)
	case "tab":
		m.jumpRoom(1)
	case "shift+tab":
		m.jumpRoom(-1)
	case "F":
		m.floor = (m.floor + 1) % len(floors)
		m.mapSel = ""
	case "N":
		m.modal = newThingModal("")
	default:
		if t := m.selectedThing(); t != nil {
			if mm, cmd, ok := m.thingActions(key, *t); ok {
				return mm, cmd
			}
			return m, nil
		}
		if mm, cmd, ok := m.pupActions(key, m.selected()); ok {
			return mm, cmd
		}
	}
	return m, nil
}

func (m Model) selectedThing() *things.Thing {
	switch m.tab {
	case TabMap:
		return m.app.Thing(mapSelection(m.mapSel, m.mapGeometry()))
	case TabJobs:
		rows := m.jobRows()
		if m.jobsCursor < len(rows) && rows[m.jobsCursor].thing != "" {
			return m.app.Thing(rows[m.jobsCursor].thing)
		}
	}
	return nil
}

func (m Model) thingActions(key string, t things.Thing) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "enter", "o":
		a := m.app
		return m, func() tea.Msg {
			text, err := a.StartOrFocus(t)
			return openedMsg{text: text, err: err}
		}, true
	case "X":
		if !t.Running {
			return m, toast(t.Name+" isn't running", true), true
		}
		m.modal = stopThingModal(t)
		return m, nil, true
	case "S":
		if t.Saved {
			return m, toast(t.Name+" is already saved", false), true
		}
		m.modal = newThingModal(t.Name + " | " + t.Command + " | " + t.Cwd)
		return m, nil, true
	case "D":
		if !t.Saved {
			return m, toast("only saved things can be forgotten", true), true
		}
		m.modal = forgetThingModal(t)
		return m, nil, true
	case "m":
		if !t.Saved {
			return m, toast("save it first with S", true), true
		}
		m.modal = m.moveThingModal(t)
		return m, nil, true
	case "N":
		m.modal = newThingModal("")
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) pupKeys(key string) (tea.Model, tea.Cmd) {
	if m.focusSidebar {
		switch key {
		case "tab", "right", "l", "enter":
			if key == "enter" && m.scope().kind == scopeNewKennel {
				m.modal = newKennelModal()
				return m, nil
			}
			m.focusSidebar = false
			return m, nil
		case "up", "k":
			m.moveCursor(-1)
			return m, nil
		case "down", "j":
			m.moveCursor(1)
			return m, nil
		case "e":
			if m.scope().kind == scopeKennel {
				m.modal = m.editKennelModal(m.scope().id)
			}
			return m, nil
		case "D":
			if m.scope().kind == scopeKennel {
				m.modal = deleteKennelModal(m.scope().id, m.scope().label)
			}
			return m, nil
		case "n":
			if m.scope().kind == scopeProject {
				m.modal = newSessionModal(strings.TrimPrefix(m.scope().key, "project:"))
			}
			return m, nil
		}
		return m, nil
	}
	switch key {
	case "tab", "left":
		m.focusSidebar = true
		return m, nil
	case "up", "k":
		m.moveCursor(-1)
		return m, nil
	case "down", "j":
		m.moveCursor(1)
		return m, nil
	case "pgdown", "ctrl+d":
		m.moveCursor(m.listPage())
		return m, nil
	case "pgup", "ctrl+u":
		m.moveCursor(-m.listPage())
		return m, nil
	case "g", "home":
		m.cursor = 0
		return m, nil
	case "G", "end":
		m.cursor = max(0, len(m.results)-1)
		return m, nil
	case "K":
		m.modal = newKennelModal()
		return m, nil
	}
	if mm, cmd, ok := m.pupActions(key, m.selected()); ok {
		return mm, cmd
	}
	return m, nil
}

func (m Model) askKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "i", "e":
		return m, m.askInput.Focus()
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "enter":
		if m.askCursor < len(m.askEv.Sources) {
			return m.jumpTo(m.askEv.Sources[m.askCursor].SessionID)
		}
	case "r":
		return m, m.startAsk(m.askQ)
	}
	return m, nil
}

func (m Model) jobKeys(key string) (tea.Model, tea.Cmd) {
	rows := m.jobRows()
	if key == "N" {
		m.modal = newThingModal("")
		return m, nil
	}
	if t := m.selectedThing(); t != nil && key != "up" && key != "down" && key != "j" && key != "k" {
		mm, cmd, _ := m.thingActions(key, *t)
		return mm, cmd
	}
	switch key {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "enter", "o":
		if m.jobsCursor < len(rows) && rows[m.jobsCursor].sessionID != "" {
			if p := m.app.Pup(rows[m.jobsCursor].sessionID); p != nil {
				return m, m.open(p, false, false)
			}
		}
	case "g":
		if m.jobsCursor < len(rows) && rows[m.jobsCursor].sessionID != "" {
			return m.jumpTo(rows[m.jobsCursor].sessionID)
		}
	case "t":
		if m.jobsCursor < len(rows) {
			if p := m.app.Pup(rows[m.jobsCursor].sessionID); p != nil && p.Pane != "" {
				m.modal = replyModal(p)
			} else {
				return m, toast("that pup isn't reachable in herdr", true)
			}
		}
	}
	return m, nil
}

func (m Model) artKeys(key string) (tea.Model, tea.Cmd) {
	arts := m.app.Artifacts()
	switch key {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "enter", "o":
		if m.artCursor < len(arts) {
			return m, m.openURL(arts[m.artCursor].URL)
		}
	case "y":
		if m.artCursor < len(arts) {
			return m, m.copy(arts[m.artCursor].URL, "copied artifact link")
		}
	case "s":
		if m.artCursor < len(arts) {
			url := arts[m.artCursor].URL
			k := "art-star:" + url
			if m.app.Store.Get(k) == "1" {
				m.app.Store.Set(k, "")
			} else {
				m.app.Store.Set(k, "1")
			}
		}
	case "g":
		if m.artCursor < len(arts) {
			return m.jumpTo(arts[m.artCursor].Pup.ID)
		}
	}
	return m, nil
}

func (m Model) gearKeys(key string) (tea.Model, tea.Cmd) {
	items := m.gearItems()
	switch key {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "tab", "right", "l":
		if len(m.gearScopes) > 0 {
			m.gearScope = (m.gearScope + 1) % len(m.gearScopes)
			m.gearCursor = 0
		}
	case "shift+tab", "left":
		if len(m.gearScopes) > 0 {
			m.gearScope = (m.gearScope + len(m.gearScopes) - 1) % len(m.gearScopes)
			m.gearCursor = 0
		}
	case " ", "enter":
		if m.gearCursor < len(items) {
			it := items[m.gearCursor]
			if it.Toggle == nil {
				return m, toast(fmt.Sprintf("%s is managed outside kennel (%s)", it.Name, it.Path), true)
			}
			m.modal = gearToggleModal(it)
		}
	case "o":
		if m.gearCursor < len(items) {
			return m, m.openURL(items[m.gearCursor].Path)
		}
	case "r":
		return m, m.enterTab()
	}
	return m, nil
}
