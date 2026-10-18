package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/pack"
)

type transcriptView struct {
	pup     *pack.Pup
	chunks  []claude.Chunk
	err     error
	top     int
	find    textinput.Model
	finding bool
	matches []int
	cur     int
	lineOf  []int
	width   int
}

func newTranscript(p *pack.Pup, query string) *transcriptView {
	_, chunks, err := claude.ParseFile(p.Path)
	ti := textinput.New()
	ti.Prompt = "/"
	ti.PromptStyle = sAccent
	t := &transcriptView{pup: p, chunks: chunks, err: err, find: ti}
	if q := strings.TrimSpace(query); q != "" {
		t.find.SetValue(q)
		t.search()
		if len(t.matches) > 0 {
			t.top = -1
		}
	}
	return t
}

func (t *transcriptView) search() {
	t.matches = nil
	q := strings.ToLower(strings.TrimSpace(t.find.Value()))
	if q == "" {
		return
	}
	words := strings.Fields(q)
	for i, c := range t.chunks {
		lc := strings.ToLower(c.Text)
		all := true
		for _, w := range words {
			if !strings.Contains(lc, w) {
				all = false
				break
			}
		}
		if all {
			t.matches = append(t.matches, i)
		}
	}
	t.cur = 0
}

func (t *transcriptView) scroll(d int) { t.top = max(0, t.top+d) }

func (m Model) updateTranscript(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t := m.transcript
	if t.finding {
		switch msg.String() {
		case "esc":
			t.finding = false
			t.find.Blur()
		case "enter":
			t.finding = false
			t.find.Blur()
			t.search()
			t.top = -1
		default:
			var cmd tea.Cmd
			t.find, cmd = t.find.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	switch msg.String() {
	case "esc", "q", "v":
		m.transcript = nil
	case "j", "down":
		t.scroll(1)
	case "k", "up":
		t.scroll(-1)
	case "d", "pgdown", " ":
		t.scroll(m.h / 2)
	case "u", "pgup":
		t.scroll(-m.h / 2)
	case "g", "home":
		t.top = 0
	case "G", "end":
		t.top = 1 << 30
	case "/":
		t.finding = true
		t.find.SetValue("")
		return m, t.find.Focus()
	case "n":
		if len(t.matches) > 0 {
			t.cur = (t.cur + 1) % len(t.matches)
			t.top = -1
		}
	case "N":
		if len(t.matches) > 0 {
			t.cur = (t.cur + len(t.matches) - 1) % len(t.matches)
			t.top = -1
		}
	case "enter", "o":
		p := t.pup
		m.transcript = nil
		return m, m.open(p, false, false)
	}
	return m, nil
}

func (t *transcriptView) view(w, h int) string {
	inner := w - 6
	title := lipgloss.NewStyle().Foreground(furColorFor(t.pup.ID, t.pup.Color, t.pup.AgentKind())).Bold(true).Render("📜 " + t.pup.Title())
	info := sDim.Render("  " + t.pup.Repo() + " · " + t.pup.ID)
	if len(t.matches) > 0 {
		info += lipgloss.NewStyle().Foreground(cYellow).Render("  match " + itoa(t.cur+1) + "/" + itoa(len(t.matches)))
	}
	header := title + info
	if t.finding {
		header = t.find.View()
	}
	if t.err != nil {
		return header + "\n\n" + sErr.Render(t.err.Error())
	}

	q := strings.Fields(strings.ToLower(t.find.Value()))
	var lines []string
	t.lineOf = make([]int, len(t.chunks))
	for i, c := range t.chunks {
		t.lineOf[i] = len(lines)
		var label string
		var body lipgloss.Style
		switch c.Role {
		case claude.RoleUser:
			label = lipgloss.NewStyle().Foreground(cBase).Background(cPink).Bold(true).Render(" you ")
			body = lipgloss.NewStyle().Foreground(cText)
		case claude.RoleAssistant:
			label = lipgloss.NewStyle().Foreground(cBase).Background(furColorFor(t.pup.ID, t.pup.Color, t.pup.AgentKind())).Bold(true).Render(" ▼•ᴥ•▼ ")
			body = lipgloss.NewStyle().Foreground(cSub)
		case claude.RoleTool:
			label = sDim.Render(" ⚙ ")
			body = sDim
		case claude.RoleRecap:
			label = lipgloss.NewStyle().Foreground(cYellow).Render(" ✦ recap ")
			body = lipgloss.NewStyle().Foreground(cYellow)
		}
		text := c.Text
		if c.Role == claude.RoleTool {
			text = truncate(strings.Join(strings.Fields(text), " "), inner-6)
		}
		rendered := body.Width(inner).Render(highlightWords(text, q, body))
		if c.Role == claude.RoleTool {
			lines = append(lines, label+" "+rendered)
			continue
		}
		lines = append(lines, "", label+sFaint.Render("  "+c.TS.Local().Format("Jan 2 15:04")))
		lines = append(lines, strings.Split(rendered, "\n")...)
	}
	bodyH := h - 2
	if t.top < 0 && len(t.matches) > 0 {
		t.top = max(0, t.lineOf[t.matches[t.cur]]-2)
	}
	t.top = min(max(0, t.top), max(0, len(lines)-bodyH))
	view := lines[t.top:min(len(lines), t.top+bodyH)]
	pct := 100
	if len(lines) > bodyH {
		pct = (t.top + bodyH) * 100 / len(lines)
	}
	header += sFaint.Render("  " + itoa(min(pct, 100)) + "%")
	return lipgloss.NewStyle().Padding(0, 2).Render(header + "\n" + strings.Join(view, "\n"))
}

func highlightWords(text string, words []string, base lipgloss.Style) string {
	if len(words) == 0 {
		return text
	}
	lower := strings.ToLower(text)
	var b strings.Builder
	i := 0
	for i < len(text) {
		hit := 0
		for _, w := range words {
			if w != "" && strings.HasPrefix(lower[i:], w) {
				hit = len(w)
				break
			}
		}
		if hit > 0 {
			b.WriteString(sMatch.Render(text[i : i+hit]))
			i += hit
			continue
		}
		j := i + 1
		for j < len(text) && text[j]&0xC0 == 0x80 {
			j++
		}
		b.WriteString(text[i:j])
		i = j
	}
	return b.String()
}

func itoa(n int) string { return strconv.Itoa(n) }
