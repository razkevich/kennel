package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/razkevich/kennel/internal/claude"
)

func (m Model) View() string {
	if m.w < 40 || m.h < 12 {
		return "kennel needs a bigger window 🐕"
	}
	header := m.header()
	footer := m.footer()
	bodyH := m.h - lipgloss.Height(header) - lipgloss.Height(footer)

	var body string
	switch {
	case m.transcript != nil:
		body = m.transcript.view(m.w, bodyH)
	default:
		switch m.tab {
		case TabMap:
			body = m.mapView(m.w, bodyH)
		case TabPups:
			body = m.pupsView(m.w, bodyH)
		case TabAsk:
			body = m.askView(m.w, bodyH)
		case TabJobs:
			body = m.jobsView(m.w, bodyH)
		case TabArtifacts:
			body = m.artView(m.w, bodyH)
		case TabGear:
			body = m.gearView(m.w, bodyH)
		}
	}
	body = lipgloss.NewStyle().Width(m.w).Height(bodyH).MaxHeight(bodyH).Render(body)
	screen := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	if m.help {
		screen = overlay(screen, m.helpView(), m.w, m.h)
	}
	if m.modal != nil {
		screen = overlay(screen, m.modal.view(m.w), m.w, m.h)
	}
	return zone.Scan(screen)
}

func (m Model) header() string {
	logo := lipgloss.NewStyle().Foreground(cPink).Bold(true).Render("▼•ᴥ•▼ kennel")
	var tabs []string
	for i, name := range tabNames {
		st := lipgloss.NewStyle().Foreground(cSub).Padding(0, 1)
		if Tab(i) == m.tab {
			st = lipgloss.NewStyle().Foreground(cBase).Background(cPink).Bold(true).Padding(0, 1)
		}
		label := name
		if m.w < 120 {
			label = strings.Fields(name)[0]
		}
		tabs = append(tabs, zone.Mark(ZoneTab(Tab(i)), st.Render(label)))
	}
	left := logo + "  " + strings.Join(tabs, " ")

	var working, waiting, idle, done int
	for _, p := range m.app.Pups() {
		switch p.Status {
		case claude.Working:
			working++
		case claude.Waiting:
			waiting++
		case claude.Idle:
			idle++
		case claude.Done:
			done++
		}
	}
	var stats []string
	if waiting > 0 {
		st := lipgloss.NewStyle().Foreground(cBase).Background(cYellow).Bold(true).Padding(0, 1)
		if m.frame%4 >= 2 {
			st = st.Background(cPeach)
		}
		stats = append(stats, st.Render(fmt.Sprintf("❗%d needs you", waiting)))
	}
	if working > 0 {
		stats = append(stats, lipgloss.NewStyle().Foreground(cGreen).Render(fmt.Sprintf("⚡%d working", working)))
	}
	if done > 0 {
		stats = append(stats, lipgloss.NewStyle().Foreground(cBlue).Render(fmt.Sprintf("✓%d done", done)))
	}
	if idle > 0 {
		stats = append(stats, lipgloss.NewStyle().Foreground(cTeal).Render(fmt.Sprintf("•%d idle", idle)))
	}
	right := strings.Join(stats, "  ")
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		right, gap = "", m.w-lipgloss.Width(left)-2
	}
	line := " " + left + strings.Repeat(" ", max(gap, 1)) + right + " "
	rule := sFaint.Render(strings.Repeat("─", m.w))
	return line + "\n" + rule
}

func (m Model) footer() string {
	var left string
	switch {
	case m.toast != "" && time.Now().Before(m.toastUntil):
		st := sOK
		if m.toastErr {
			st = sErr
		}
		left = st.Render(m.toast)
	case m.syncing && m.syncTotal.Load() > 0:
		left = sDim.Render(fmt.Sprintf("🦴 fetching sessions %d/%d…", m.syncDone.Load(), m.syncTotal.Load()))
	default:
		left = m.hints()
	}
	n, _ := m.app.Store.Count()
	right := sFaint.Render(fmt.Sprintf("%d pups", n))
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		return " " + truncate(left, m.w-2)
	}
	return " " + left + strings.Repeat(" ", gap) + right + " "
}

func hint(k, label string) string {
	return lipgloss.NewStyle().Foreground(cMauve).Bold(true).Render(k) + sDim.Render(" "+label)
}

func (m Model) hints() string {
	var hs []string
	if m.transcript != nil {
		hs = []string{hint("j/k", "scroll"), hint("/", "find"), hint("n/N", "next/prev"), hint("esc", "close")}
	} else {
		switch m.tab {
		case TabMap:
			hs = []string{hint("←↑↓→", "walk"), hint("tab", "room"), hint("⏎", "open"), hint("F", "floor"), hint("u", "needs you"), hint("/", "search"), hint("a", "ask"), hint("?", "help")}
		case TabPups:
			if m.searching {
				hs = []string{hint("type", "to filter"), hint("⏎/↓", "to list"), hint("esc", "clear")}
			} else {
				hs = []string{hint("⏎", "open"), hint("/", "search"), hint("v", "read"), hint("s", "star"), hint("r", "rename"), hint("m", "move"), hint("f", "fork"), hint("t", "reply"), hint("?", "help")}
			}
		case TabAsk:
			hs = []string{hint("⏎", "ask"), hint("↑↓", "sources"), hint("1-9", "jump to [n]"), hint("r", "re-ask"), hint("i", "edit")}
		case TabJobs:
			hs = []string{hint("⏎", "open"), hint("g", "go to pup"), hint("t", "message")}
		case TabArtifacts:
			hs = []string{hint("⏎", "open"), hint("y", "copy link"), hint("s", "star"), hint("g", "go to pup")}
		case TabGear:
			hs = []string{hint("←→", "scope"), hint("space", "toggle"), hint("o", "open file"), hint("r", "rescan")}
		}
	}
	out := ""
	for _, h := range hs {
		if lipgloss.Width(out)+lipgloss.Width(h)+3 > m.w-12 {
			break
		}
		if out != "" {
			out += "  "
		}
		out += h
	}
	return out
}

func (m Model) helpView() string {
	sections := []struct {
		title string
		keys  [][2]string
	}{
		{"getting around", [][2]string{{"1-6 / [ ]", "switch tabs"}, {"ctrl+k", "jump anywhere"}, {"u", "next pup that needs you"}, {"/", "search (names + transcripts)"}, {"a", "ask your pack (RAG)"}, {"F", "map floor: campus / projects / kennels"}}},
		{"a pup", [][2]string{{"⏎", "open / focus in herdr or Ghostty"}, {"f", "fork session"}, {"R", "resume with remote control"}, {"t", "quick reply (herdr)"}, {"v / space", "read transcript"}, {"n", "new session here"}}},
		{"organize", [][2]string{{"s", "star (Best in Show)"}, {"r", "rename"}, {"m", "move to kennel"}, {"N", "note"}, {"x", "archive"}, {"D", "trash (recoverable)"}, {"K", "new kennel"}}},
		{"handy", [][2]string{{"y / Y", "copy id / resume command"}, {"p / A", "open PR / artifact"}, {"search", "is:live is:starred repo:x since:7d has:pr"}, {"q", "quit"}}},
	}
	var cols []string
	for _, s := range sections {
		lines := []string{sAccent.Render(s.title)}
		for _, k := range s.keys {
			lines = append(lines, lipgloss.NewStyle().Foreground(cMauve).Bold(true).Width(11).Render(k[0])+sSub.Render(k[1]))
		}
		cols = append(cols, lipgloss.NewStyle().Width(46).Render(strings.Join(lines, "\n")))
	}
	grid := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, cols[0], cols[1]),
		"",
		lipgloss.JoinHorizontal(lipgloss.Top, cols[2], cols[3]))
	title := sAccent.Render("▼•ᴥ•▼  kennel — good dog commands") + "\n\n"
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cPink).Padding(1, 2).
		Render(title + grid + "\n\n" + sDim.Render("any key to close"))
}

// overlay centers fg over bg, replacing whole lines (cheap and ANSI-safe).
func overlay(bg, fg string, w, h int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")
	fw := lipgloss.Width(fg)
	top := max(0, (h-len(fgLines))/2)
	left := max(0, (w-fw)/2)
	for i, l := range fgLines {
		y := top + i
		if y >= len(bgLines) {
			break
		}
		bgLines[y] = strings.Repeat(" ", left) + l
	}
	return strings.Join(bgLines, "\n")
}

func ago(now, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 120*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/24/7))
	}
	return t.Format("Jan 2006")
}
