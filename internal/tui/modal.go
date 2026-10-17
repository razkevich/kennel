package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/razkevich/kennel/internal/gear"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/store"
	"github.com/razkevich/kennel/internal/things"
	"github.com/sahilm/fuzzy"
)

type choice struct {
	label  string
	detail string
	value  string
}

// modal is a small dialog: a text input, a filterable picker, or a yes/no confirm.
type modal struct {
	title   string
	help    string
	input   *textinput.Model
	choices []choice
	cursor  int
	confirm bool
	// onSubmit receives the input text and the chosen value (if any).
	onSubmit func(m *Model, text, value string) tea.Cmd
}

func textModal(title, help, value, placeholder string, submit func(m *Model, text, value string) tea.Cmd) *modal {
	ti := textinput.New()
	ti.SetValue(value)
	ti.Placeholder = placeholder
	ti.Prompt = "› "
	ti.PromptStyle = sAccent
	ti.Width = 50
	ti.CharLimit = 2000
	ti.Focus()
	return &modal{title: title, help: help, input: &ti, onSubmit: submit}
}

func pickerModal(title string, choices []choice, submit func(m *Model, text, value string) tea.Cmd) *modal {
	md := textModal(title, "type to filter · ⏎ pick · esc cancel", "", "filter…", submit)
	md.choices = choices
	return md
}

func confirmModal(title, help string, submit func(m *Model, text, value string) tea.Cmd) *modal {
	return &modal{title: title, help: help, confirm: true, onSubmit: submit}
}

func (md *modal) filtered() []choice {
	if md.input == nil || md.input.Value() == "" {
		return md.choices
	}
	labels := make([]string, len(md.choices))
	for i, c := range md.choices {
		labels[i] = c.label + " " + c.detail
	}
	var out []choice
	for _, r := range fuzzy.Find(md.input.Value(), labels) {
		out = append(out, md.choices[r.Index])
	}
	return out
}

func (m Model) updateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	md := m.modal
	switch msg.String() {
	case "esc":
		m.modal = nil
		return m, nil
	case "enter", "y":
		if msg.String() == "y" && !md.confirm {
			break
		}
		m.modal = nil
		text, value := "", ""
		if md.input != nil {
			text = strings.TrimSpace(md.input.Value())
		}
		if md.choices != nil {
			f := md.filtered()
			if len(f) == 0 {
				return m, nil
			}
			value = f[min(md.cursor, len(f)-1)].value
		}
		return m, md.onSubmit(&m, text, value)
	case "n":
		if md.confirm {
			m.modal = nil
			return m, nil
		}
	case "up", "ctrl+p":
		if md.choices != nil {
			md.cursor = max(0, md.cursor-1)
			return m, nil
		}
	case "down", "ctrl+n":
		if md.choices != nil {
			md.cursor = min(len(md.filtered())-1, md.cursor+1)
			return m, nil
		}
	}
	if md.input != nil {
		ti, cmd := md.input.Update(msg)
		md.input = &ti
		md.cursor = 0
		return m, cmd
	}
	return m, nil
}

func (md *modal) view(w int) string {
	width := min(72, w-6)
	var b strings.Builder
	b.WriteString(sAccent.Render(md.title) + "\n")
	if md.help != "" {
		b.WriteString(sDim.Render(md.help) + "\n")
	}
	b.WriteString("\n")
	if md.input != nil {
		b.WriteString(md.input.View() + "\n")
	}
	if md.confirm {
		b.WriteString(hint("y", "yes") + "   " + hint("n/esc", "no"))
	}
	if md.choices != nil {
		f := md.filtered()
		start := max(0, md.cursor-9)
		for i := start; i < len(f) && i < start+10; i++ {
			c := f[i]
			line := "  " + c.label
			if c.detail != "" {
				line += sDim.Render("  " + c.detail)
			}
			line = truncateANSI(line, width-4)
			if i == md.cursor {
				line = lipgloss.NewStyle().Foreground(cBase).Background(cPink).Bold(true).Width(width - 4).Render(truncate(stripSpaces(c.label), width-6))
			}
			b.WriteString("\n" + line)
		}
		if len(f) == 0 {
			b.WriteString("\n" + sDim.Render("  nothing matches"))
		}
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cPink).Padding(1, 2).Width(width).Render(b.String())
}

func stripSpaces(s string) string { return "  " + strings.TrimSpace(s) }

// ---- specific dialogs ----

func renameModal(p *pack.Pup) *modal {
	help := "kennel remembers it; when the pup is asleep it's also written to the session like /rename"
	return textModal("📝 rename "+p.Title(), help, p.Title(), "NEW-NAME", func(m *Model, text, _ string) tea.Cmd {
		if err := m.app.Rename(p.ID, text); err != nil {
			return toast(err.Error(), true)
		}
		m.rebuildScopes()
		return tea.Batch(m.refreshResults(), toast("📝 renamed to "+text, false))
	})
}

func noteModal(p *pack.Pup) *modal {
	return textModal("📝 note on "+p.Title(), "a reminder for future you (shown in the list)", p.Meta.Note, "e.g. waiting on Priya's review", func(m *Model, text, _ string) tea.Cmd {
		if err := m.app.SetNote(p.ID, text); err != nil {
			return toast(err.Error(), true)
		}
		return m.refreshResults()
	})
}

func replyModal(p *pack.Pup) *modal {
	return textModal("💬 quick reply to "+p.Title(), "sent to the running session through herdr, without switching to it", "", "yes, go ahead", func(m *Model, text, _ string) tea.Cmd {
		if text == "" {
			return nil
		}
		a := m.app
		return func() tea.Msg {
			if err := a.Prompt(p, text); err != nil {
				return toastMsg{text: err.Error(), err: true}
			}
			return toastMsg{text: "💬 sent to " + p.Title()}
		}
	})
}

func trashModal(p *pack.Pup) *modal {
	return confirmModal("🚮 move "+p.Title()+" to kennel's trash?", "the transcript is moved to ~/.kennel/trash, never deleted", func(m *Model, _, _ string) tea.Cmd {
		dst, err := m.app.Trash(p.ID)
		if err != nil {
			return toast(err.Error(), true)
		}
		m.rebuildScopes()
		return tea.Batch(m.refreshResults(), toast("🚮 moved to "+dst, false))
	})
}

func newSessionModal(cwd string) *modal {
	return textModal("🐣 new pup in "+shortPath(cwd), "name it (optional) — opens claude there", "", "NAME", func(m *Model, text, _ string) tea.Cmd {
		if err := m.app.CheckCwd(cwd); err != nil {
			return toast(err.Error(), true)
		}
		return m.newSession(cwd, text)
	})
}

var kennelIcons = []string{"🏠", "🦴", "🎾", "🐾", "🌭", "🧸", "🛁", "⛺", "🚀", "🧪", "💼", "🔧"}
var kennelColors = []string{"#fab387", "#a6e3a1", "#89b4fa", "#cba6f7", "#f5c2e7", "#f9e2af", "#94e2d5", "#f38ba8"}

func newKennelModal() *modal {
	return textModal("🏠 new kennel", "name · optional rules after '|':  cwd=~/code/atlas  match=^INF-|checkout", "", "Payments | match=checkout|stripe", func(m *Model, text, _ string) tea.Cmd {
		if text == "" {
			return nil
		}
		k := parseKennel(text, len(m.app.Kennels()))
		if err := m.app.SaveKennel(&k); err != nil {
			return toast(err.Error(), true)
		}
		m.rebuildScopes()
		return tea.Batch(m.refreshResults(), toast(k.Icon+" kennel "+k.Name+" built", false))
	})
}

func parseKennel(text string, n int) store.Kennel {
	name, rules, _ := strings.Cut(text, "|")
	k := store.Kennel{Name: strings.TrimSpace(name), Icon: kennelIcons[n%len(kennelIcons)], Color: kennelColors[n%len(kennelColors)]}
	for _, f := range strings.Fields(rules) {
		key, val, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		switch key {
		case "cwd":
			if strings.HasPrefix(val, "~/") {
				home, _ := os.UserHomeDir()
				val = home + val[1:]
			}
			k.Rules.CwdPrefix = append(k.Rules.CwdPrefix, val)
			if k.Cwd == "" {
				k.Cwd = val
			}
		case "match":
			k.Rules.Match = val
		case "icon":
			k.Icon = val
		case "color":
			k.Color = val
		}
	}
	return k
}

func kennelText(k store.Kennel) string {
	var parts []string
	for _, c := range k.Rules.CwdPrefix {
		parts = append(parts, "cwd="+c)
	}
	if k.Rules.Match != "" {
		parts = append(parts, "match="+k.Rules.Match)
	}
	parts = append(parts, "icon="+k.Icon, "color="+k.Color)
	return k.Name + " | " + strings.Join(parts, " ")
}

func (m *Model) editKennelModal(id int64) *modal {
	var k store.Kennel
	for _, kk := range m.app.Kennels() {
		if kk.ID == id {
			k = kk
		}
	}
	return textModal("🏠 edit kennel", "name | cwd=… match=… icon=… color=…", kennelText(k), "", func(m *Model, text, _ string) tea.Cmd {
		nk := parseKennel(text, 0)
		nk.ID, nk.Sort = k.ID, k.Sort
		if err := m.app.SaveKennel(&nk); err != nil {
			return toast(err.Error(), true)
		}
		m.rebuildScopes()
		return m.refreshResults()
	})
}

func deleteKennelModal(id int64, name string) *modal {
	return confirmModal("tear down kennel "+name+"?", "its pups just become strays; no sessions are touched", func(m *Model, _, _ string) tea.Cmd {
		if err := m.app.DeleteKennel(id); err != nil {
			return toast(err.Error(), true)
		}
		m.scopeIdx = 0
		m.rebuildScopes()
		return m.refreshResults()
	})
}

func (m *Model) moveModal(p *pack.Pup) *modal {
	choices := []choice{{label: "🐾 no kennel (stray)", value: "0"}}
	for _, k := range m.app.Kennels() {
		choices = append(choices, choice{label: k.Icon + " " + k.Name, value: fmt.Sprint(k.ID)})
	}
	choices = append(choices, choice{label: "＋ new kennel…", value: "new"})
	return pickerModal("🚚 move "+p.Title()+" to…", choices, func(m *Model, _, value string) tea.Cmd {
		if value == "new" {
			m.modal = newKennelModal()
			return nil
		}
		var id int64
		fmt.Sscan(value, &id)
		if err := m.app.MoveTo(p.ID, id); err != nil {
			return toast(err.Error(), true)
		}
		m.rebuildScopes()
		return tea.Batch(m.refreshResults(), toast("🚚 moved "+p.Title(), false))
	})
}

func gearToggleModal(it gear.Item) *modal {
	verb := "disable"
	if !it.Enabled {
		verb = "enable"
	}
	return confirmModal(fmt.Sprintf("%s %s %s?", verb, it.Kind, it.Name), "takes effect in new Claude sessions · "+shortPath(it.Path), func(m *Model, _, _ string) tea.Cmd {
		if err := it.Toggle(); err != nil {
			return toast(err.Error(), true)
		}
		m.enterTab()
		return toast(fmt.Sprintf("%sd %s", verb, it.Name), false)
	})
}

// switcherModal jumps anywhere: pups, projects, kennels, tabs.
func (m *Model) switcherModal() *modal {
	var choices []choice
	for i, name := range tabNames {
		choices = append(choices, choice{label: name, detail: "tab", value: fmt.Sprintf("tab:%d", i)})
	}
	for i, s := range m.scopes {
		if s.kind == scopeNewKennel {
			continue
		}
		choices = append(choices, choice{label: s.icon + " " + s.label, detail: fmt.Sprintf("%d pups", s.count), value: fmt.Sprintf("scope:%d", i)})
	}
	for _, p := range m.app.Pups() {
		if p.Empty() {
			continue
		}
		choices = append(choices, choice{label: "▼•ᴥ•▼ " + p.Title(), detail: p.Repo() + " · " + ago(m.app.Now(), p.Updated), value: "pup:" + p.ID})
	}
	return pickerModal("🧭 jump to…", choices, func(m *Model, _, value string) tea.Cmd {
		kind, v, _ := strings.Cut(value, ":")
		switch kind {
		case "tab":
			var t int
			fmt.Sscan(v, &t)
			m.tab = Tab(t)
			return m.enterTab()
		case "scope":
			var i int
			fmt.Sscan(v, &i)
			m.tab = TabPups
			m.focusSidebar = false
			m.scopeIdx = i
			m.cursor = 0
			return m.refreshResults()
		case "pup":
			m.tab = TabPups
			m.selectPup(v)
		}
		return nil
	})
}

func newThingModal(prefill string) *modal {
	help := "name | command | folder   — e.g.  api logs | kubectl logs -f deploy/api | ~/code/infra"
	return textModal("🔧 new thing", help, prefill, "dev server | pnpm dev | ~/code/app", func(m *Model, text, _ string) tea.Cmd {
		parts := strings.Split(text, "|")
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return toast("need at least: name | command", true)
		}
		t := things.Thing{Name: strings.TrimSpace(parts[0]), Command: strings.TrimSpace(parts[1])}
		if len(parts) > 2 {
			t.Cwd = expandHome(strings.TrimSpace(parts[2]))
		}
		if t.Cwd == "" {
			t.Cwd, _ = os.UserHomeDir()
		}
		t.Kind = things.Infer(t.Command)
		if err := m.app.SaveThing(&t); err != nil {
			return toast(err.Error(), true)
		}
		return toast(things.Icon[t.Kind]+" saved "+t.Name, false)
	})
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, _ := os.UserHomeDir()
		return home + p[1:]
	}
	return p
}

func stopThingModal(t things.Thing) *modal {
	how := "Ctrl-C in its herdr pane"
	if t.Pane == "" {
		how = fmt.Sprintf("SIGINT to pid %d", t.PID)
	}
	return confirmModal("✋ stop "+t.Name+"?", how, func(m *Model, _, _ string) tea.Cmd {
		a := m.app
		return func() tea.Msg {
			if err := a.Stop(t); err != nil {
				return toastMsg{text: err.Error(), err: true}
			}
			return toastMsg{text: "✋ stopped " + t.Name}
		}
	})
}

func forgetThingModal(t things.Thing) *modal {
	return confirmModal("forget "+t.Name+"?", "removes it from kennel; nothing running is touched", func(m *Model, _, _ string) tea.Cmd {
		if err := m.app.DeleteThing(t.ID); err != nil {
			return toast(err.Error(), true)
		}
		return toast("forgot "+t.Name, false)
	})
}

func (m *Model) moveThingModal(t things.Thing) *modal {
	choices := []choice{{label: "🔧 Workshop (no kennel)", value: "0"}}
	for _, k := range m.app.Kennels() {
		choices = append(choices, choice{label: k.Icon + " " + k.Name, value: fmt.Sprint(k.ID)})
	}
	return pickerModal("🚚 move "+t.Name+" to…", choices, func(m *Model, _, value string) tea.Cmd {
		var id int64
		fmt.Sscan(value, &id)
		t.KennelID = id
		if err := m.app.SaveThing(&t); err != nil {
			return toast(err.Error(), true)
		}
		return toast("🚚 moved "+t.Name, false)
	})
}
