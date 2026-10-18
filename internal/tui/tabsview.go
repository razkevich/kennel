package tui

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/razkevich/kennel/internal/gear"
	"github.com/razkevich/kennel/internal/things"
)

var citeRe = regexp.MustCompile(`\[(\d+)\]`)

func (m Model) askView(w, h int) string {
	inner := w - 4
	var lines []string
	input := m.askInput.View()
	if !m.askInput.Focused() && m.askInput.Value() == "" {
		input = sAccent.Render("💬 ") + sDim.Render("press a, / or i to ask your pack anything…")
	}
	inputBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cPink).Padding(0, 1).Width(inner).Render(input)
	lines = append(lines, inputBox)

	ev := m.askEv
	if m.askQ == "" {
		intro := []string{
			"",
			sAccent.Render("Ask your pack") + sSub.Render(" — questions answered from your past sessions, with sources."),
			"",
			sDim.Render("  try:  what did we decide about the postgres upgrade?"),
			sDim.Render("        which session touched the stripe payment element?"),
			sDim.Render("        what was the root cause of the last incident?"),
			"",
			sDim.Render("  kennel finds the most relevant excerpts, then your own `claude -p` answers using only them."),
		}
		lines = append(lines, intro...)
		return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
	}

	stage := ev.Stage
	if !ev.Done && stage != "" {
		dots := strings.Repeat("·", m.frame%4)
		lines = append(lines, lipgloss.NewStyle().Foreground(cGreen).Render("▼•ᴥ•▼ "+stage+dots))
	}
	if len(ev.Terms) > 0 {
		lines = append(lines, sDim.Render(truncate("sniffed for: "+strings.Join(ev.Terms, ", "), inner)))
	}
	if ev.Err != nil {
		lines = append(lines, sErr.Render("✗ "+ev.Err.Error()))
	}

	answerH := h - len(lines) - 2
	srcH := 0
	if len(ev.Sources) > 0 {
		srcH = min(len(ev.Sources)+2, max(6, answerH/2))
		answerH -= srcH
	}
	if ev.Text != "" {
		text := citeRe.ReplaceAllStringFunc(ev.Text, func(c string) string {
			return lipgloss.NewStyle().Foreground(cMauve).Bold(true).Render(c)
		})
		ans := lipgloss.NewStyle().Width(inner).Foreground(cText).Render(text)
		al := strings.Split(ans, "\n")
		if len(al) > answerH {
			al = al[len(al)-answerH:]
		}
		lines = append(lines, "")
		lines = append(lines, al...)
	}
	if len(ev.Sources) > 0 {
		lines = append(lines, "", sAccent.Render("sources")+sDim.Render("  (↑↓ select · ⏎ go · 1-9 jump)"))
		start := 0
		if m.askCursor >= srcH-2 {
			start = m.askCursor - (srcH - 3)
		}
		for i := start; i < len(ev.Sources) && i < start+srcH-2; i++ {
			s := ev.Sources[i]
			sel := i == m.askCursor
			bar := "  "
			if sel {
				bar = lipgloss.NewStyle().Foreground(cPink).Render("▌ ")
			}
			n := lipgloss.NewStyle().Foreground(cMauve).Bold(true).Render(fmt.Sprintf("[%d]", s.N))
			title := sText.Bold(true).Render(s.Title)
			meta := sDim.Render(fmt.Sprintf(" %s · %s · %s  ", s.Repo, s.When.Format("Jan 2"), roleIcon(s.Hit.Role)))
			snippet := strings.Join(strings.Fields(s.Hit.Text), " ")
			line := bar + n + " " + title + meta
			line += sSub.Render(truncate(snippet, max(10, inner-lipgloss.Width(line))))
			if sel {
				line = lipgloss.NewStyle().Background(cSurface).Width(inner).Render(line)
			}
			lines = append(lines, line)
		}
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

type jobRow struct {
	thing     string
	kind      string
	icon      string
	title     string
	detail    string
	state     string
	when      time.Time
	sessionID string
	live      bool
}

func (m Model) jobRows() []jobRow {
	var rows []jobRow
	for _, t := range m.app.Things() {
		state := "stopped"
		if t.Running {
			state = "running"
			if t.Pane != "" {
				state += " · herdr " + t.Pane
			}
		}
		if !t.Saved {
			state += " · found in herdr (S to save)"
		}
		rows = append(rows, jobRow{thing: t.Key(), kind: "thing", icon: things.Icon[t.Kind], title: t.Name, detail: "$ " + t.Command + "   " + shortPath(t.Cwd), state: state, live: t.Running})
	}
	for _, j := range m.app.Jobs() {
		title := orDefault(strings.TrimSpace(strings.ReplaceAll(j.Name, "⑂", "")), j.Short)
		if p := m.app.Pup(j.SessionID); p != nil {
			title = p.Title()
		}
		icon := "🐕"
		if j.Running() {
			icon = "🏃"
		}
		rows = append(rows, jobRow{kind: "background", icon: icon, title: title, detail: orDefault(j.Detail, j.Intent), state: j.State, when: j.UpdatedAt, sessionID: j.SessionID, live: j.Running()})
	}
	for _, p := range m.app.Pups() {
		for _, c := range p.Crons {
			state := "scheduled"
			if !p.Status.Live() {
				state = "stopped (session closed)"
			}
			rows = append(rows, jobRow{kind: "cron", icon: "⏰", title: p.Title() + "  " + c.Expr, detail: c.Prompt,
				state: fmt.Sprintf("%s · fired %d×", state, c.Fires), when: c.LastFire, sessionID: p.ID, live: p.Status.Live()})
		}
	}
	return rows
}

func (m Model) jobsView(w, h int) string {
	rows := m.jobRows()
	now := m.app.Now()
	inner := w - 4
	lines := []string{sAccent.Render("⏰ Jobs & Things") + sSub.Render(" — terminals that aren't agents, background agents, and cron tasks inside sessions"), ""}
	if len(rows) == 0 {
		lines = append(lines, sDim.Render("  no background agents or scheduled tasks yet — they show up here when you run claude in the background or /loop"))
	}
	kind := ""
	visible := (h - 4) / 3
	start := 0
	if m.jobsCursor >= visible {
		start = m.jobsCursor - visible + 1
	}
	for i := start; i < len(rows) && i < start+visible; i++ {
		r := rows[i]
		if r.kind != kind {
			kind = r.kind
			label := map[string]string{"thing": "THINGS  (N new · ⏎ start/focus · X stop · S save · D forget)", "background": "BACKGROUND AGENTS", "cron": "SCHEDULED TASKS"}[kind]
			lines = append(lines, sDim.Bold(true).Render(label))
		}
		sel := i == m.jobsCursor
		bar := "  "
		if sel {
			bar = lipgloss.NewStyle().Foreground(cPink).Render("▌ ")
		}
		stColor := cDim
		if r.live {
			stColor = cGreen
		}
		state := lipgloss.NewStyle().Foreground(stColor).Render(r.state)
		when := ""
		if !r.when.IsZero() {
			when = "  " + ago(now, r.when)
		}
		l1 := bar + r.icon + " " + sText.Bold(true).Render(truncate(r.title, inner/2)) + "  " + state + sDim.Render(when)
		l2 := "     " + sSub.Render(truncate(r.detail, inner-6))
		block := l1 + "\n" + l2
		if sel {
			block = lipgloss.NewStyle().Background(cSurface).Width(inner).Render(block)
		}
		lines = append(lines, block)
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func (m Model) artView(w, h int) string {
	arts := m.app.Artifacts()
	now := m.app.Now()
	inner := w - 4
	lines := []string{sAccent.Render("🎨 Artifacts") + sSub.Render(fmt.Sprintf(" — %d pages your pups published", len(arts))), ""}
	if len(arts) == 0 {
		lines = append(lines, sDim.Render("  no artifacts yet"))
	}
	visible := (h - 3) / 2
	start := 0
	if m.artCursor >= visible {
		start = m.artCursor - visible + 1
	}
	for i := start; i < len(arts) && i < start+visible; i++ {
		a := arts[i]
		sel := i == m.artCursor
		bar := "  "
		if sel {
			bar = lipgloss.NewStyle().Foreground(cPink).Render("▌ ")
		}
		star := "  "
		if m.app.Store.Get("art-star:"+a.URL) == "1" {
			star = lipgloss.NewStyle().Foreground(cYellow).Render("★ ")
		}
		title := orDefault(a.Title, a.URL)
		l1 := bar + star + "🎨 " + sText.Bold(true).Render(truncate(title, inner/2)) + sDim.Render("  from ") +
			lipgloss.NewStyle().Foreground(furColorFor(a.Pup.ID, a.Pup.Color, a.Pup.AgentKind())).Render(a.Pup.Title()) + sDim.Render("  "+ago(now, a.At))
		l2 := "       " + sDim.Render(truncate(a.URL, inner-8))
		block := l1 + "\n" + l2
		if sel {
			block = lipgloss.NewStyle().Background(cSurface).Width(inner).Render(block)
		}
		lines = append(lines, block)
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func (m Model) gearItems() []gear.Item {
	if m.gearScope < len(m.gearScopes) {
		return m.gearScopes[m.gearScope].Items
	}
	return nil
}

func (m Model) gearView(w, h int) string {
	inner := w - 4
	var tabs []string
	for i, s := range m.gearScopes {
		st := sSub.Padding(0, 1)
		if i == m.gearScope {
			st = lipgloss.NewStyle().Foreground(cBase).Background(cMauve).Bold(true).Padding(0, 1)
		}
		icon := "📁"
		switch s.Name {
		case "User":
			icon = "👤"
		case "Plugins":
			icon = "🧩"
		}
		tabs = append(tabs, st.Render(icon+" "+s.Name))
	}
	lines := []string{sAccent.Render("🔩 Gear") + sSub.Render(" — MCP servers, skills, plugins, subagents, commands and hooks"), "", truncateANSI(strings.Join(tabs, " "), inner), ""}
	items := m.gearItems()
	if len(m.gearScopes) > 0 {
		lines = append(lines, sDim.Render(shortPath(m.gearScopes[m.gearScope].Root)))
	}
	if len(items) == 0 {
		lines = append(lines, sDim.Render("  nothing configured here"))
	}
	visible := h - len(lines) - 2
	start := 0
	if m.gearCursor >= visible-4 {
		start = m.gearCursor - visible + 5
	}
	kind := gear.Kind("")
	for i := start; i < len(items) && len(lines) < h-1; i++ {
		it := items[i]
		if it.Kind != kind {
			kind = it.Kind
			lines = append(lines, sDim.Bold(true).Render(strings.ToUpper(string(kind))+"S"))
		}
		sel := i == m.gearCursor
		bar := "  "
		if sel {
			bar = lipgloss.NewStyle().Foreground(cPink).Render("▌ ")
		}
		dot := lipgloss.NewStyle().Foreground(cGreen).Render("●")
		if !it.Enabled {
			dot = sDim.Render("○")
		}
		lock := ""
		if it.Toggle == nil {
			lock = sFaint.Render(" 🔒")
		}
		name := sText.Bold(true).Render(it.Name)
		if !it.Enabled {
			name = sDim.Render(it.Name)
		}
		left := bar + dot + " " + gear.KindIcon[it.Kind] + " " + name + lock + sDim.Render("  "+it.Level+"  ")
		line := left + sSub.Render(truncate(it.Detail, max(8, inner-lipgloss.Width(left))))
		if sel {
			line = lipgloss.NewStyle().Background(cSurface).Width(inner).Render(line)
		}
		lines = append(lines, line)
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func runWithInput(name, input string) ([]byte, error) {
	c := exec.Command(name)
	c.Stdin = strings.NewReader(input)
	return c.CombinedOutput()
}
