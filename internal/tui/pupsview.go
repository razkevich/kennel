package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/razkevich/kennel/internal/app"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/search"
	"github.com/razkevich/kennel/internal/store"
)

const rowH = 3

func (m Model) listPage() int { return max(1, (m.h-8)/rowH) }

func (m Model) pupsView(w, h int) string {
	sideW := 0
	if w >= 90 {
		sideW = 26
	}
	prevW := 0
	if w >= 130 {
		prevW = min(64, w*2/5)
	}
	listW := w - sideW - prevW
	var cols []string
	if sideW > 0 {
		cols = append(cols, m.sidebar(sideW, h))
	}
	cols = append(cols, m.list(listW, h))
	if prevW > 0 {
		cols = append(cols, m.preview(prevW, h))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

func (m Model) sidebar(w, h int) string {
	var lines []string
	section := func(title string) {
		lines = append(lines, "", sDim.Bold(true).Render(" "+strings.ToUpper(title)))
	}
	lastKind := scopeKind(-1)
	selLine := 0
	for i, s := range m.scopes {
		if s.kind != lastKind {
			switch s.kind {
			case scopeSmart:
				section("smart kennels")
			case scopeProject:
				section("projects")
			case scopeKennel:
				section("your kennels")
			case scopeNewKennel:
				if lastKind != scopeKennel {
					section("your kennels")
				}
			}
			lastKind = s.kind
		}
		label := truncate(s.label, w-11)
		count := ""
		if s.kind != scopeNewKennel {
			count = fmt.Sprintf("%d", s.count)
		}
		badge := ""
		if s.unread > 0 {
			badge = lipgloss.NewStyle().Foreground(cBase).Background(cYellow).Bold(true).Render(fmt.Sprintf("%d", s.unread))
		}
		left := " " + s.icon + " " + label
		right := badge + " " + count + " "
		gap := w - lipgloss.Width(left) - lipgloss.Width(right)
		line := left + strings.Repeat(" ", max(1, gap)) + right
		st := sSub
		if s.count == 0 && s.kind != scopeNewKennel {
			st = sDim
		}
		if i == m.scopeIdx {
			if m.focusSidebar {
				st = lipgloss.NewStyle().Foreground(cBase).Background(cPink).Bold(true)
			} else {
				st = lipgloss.NewStyle().Foreground(cText).Background(cSurface).Bold(true)
			}
		}
		if i == m.scopeIdx {
			selLine = len(lines)
		}
		lines = append(lines, zone.Mark(ZoneScope(s.key), st.Width(w).Render(line)))
	}
	// Keep the selected scope visible when the list of projects is long.
	start := 0
	if len(lines) > h {
		start = min(max(0, selLine-h+3), len(lines)-h)
	}
	end := min(len(lines), start+h)
	return lipgloss.NewStyle().Width(w).Height(h).BorderRight(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(cFaint).
		Render(strings.Join(lines[start:end], "\n"))
}

func (m Model) list(w, h int) string {
	sc := m.scope()
	searchLine := " " + m.query.View()
	if !m.searching && m.query.Value() == "" {
		searchLine = " " + sDim.Render("🔎 press / to search · a to ask")
	}
	title := lipgloss.NewStyle().Foreground(lipgloss.Color(orDefault(sc.color, string(cPink)))).Bold(true).Render(sc.icon + " " + sc.label)
	if sm := pack.SmartByKey(strings.TrimPrefix(sc.key, "smart:")); sm != nil {
		title += sDim.Render("  " + sm.Blurb)
	}
	countTxt := sDim.Render(fmt.Sprintf("%d", len(m.results)))
	head := " " + title + strings.Repeat(" ", max(1, w-lipgloss.Width(title)-lipgloss.Width(countTxt)-3)) + countTxt
	lines := []string{truncateANSI(searchLine, w), head, sFaint.Render(" " + strings.Repeat("─", max(0, w-2)))}

	avail := h - len(lines)
	visible := max(1, avail/rowH)
	off := 0
	if m.cursor >= visible {
		off = m.cursor - visible + 1
	}
	if len(m.results) == 0 {
		empty := "no pups here yet 🐾"
		if m.query.Value() != "" {
			empty = "no pups match — try fewer words, or press a to ask instead"
		}
		lines = append(lines, "", "  "+sDim.Render(empty))
	}
	for i := off; i < len(m.results) && i < off+visible; i++ {
		lines = append(lines, m.row(m.results[i], i == m.cursor, w))
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.Join(lines, "\n"))
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func (m Model) row(r search.Result, selected bool, w int) string {
	p := r.Pup
	now := m.app.Now()
	bar := "  "
	if selected {
		bar = lipgloss.NewStyle().Foreground(cPink).Render("▌ ")
	}
	titleSt := lipgloss.NewStyle().Foreground(cText).Bold(p.Status.Live() || p.CustomTitle != "" || p.Meta.Alias != "")
	if !p.Status.Live() && p.CustomTitle == "" && p.Meta.Alias == "" {
		titleSt = titleSt.Foreground(cSub)
	}
	badges := ""
	if p.Meta.Starred {
		badges += lipgloss.NewStyle().Foreground(cYellow).Render(" ★")
	}
	if p.Remote != "" {
		badges += " 📡"
	}
	if a := p.AgentKind(); a != "claude" {
		badges += lipgloss.NewStyle().Foreground(cSky).Render(" " + a)
	}
	if len(p.Crons) > 0 {
		badges += " ⏰"
	}
	if len(p.PRs) > 0 {
		badges += lipgloss.NewStyle().Foreground(cMauve).Render(" " + prNumber(p.PRs[len(p.PRs)-1]))
	}
	if p.Kennel != nil {
		badges += sDim.Render(" " + p.Kennel.Icon + p.Kennel.Name)
	}
	status := ""
	if p.Status.Live() {
		label := statusLabel(p.Status)
		if !p.Live.Since.IsZero() && p.Status != claude.Working {
			label += " " + ago(now, p.Live.Since)
		}
		status = lipgloss.NewStyle().Foreground(statusColor(p.Status)).Render(label) + "  "
	}
	age := sDim.Render(ago(now, p.Updated))
	left := bar + miniFace(p, m.frame) + " " + titleSt.Render(truncate(p.Title(), w/2)) + badges
	right := status + age + " "
	line1 := left + strings.Repeat(" ", max(1, w-lipgloss.Width(left)-lipgloss.Width(right))) + right
	line1 = truncateANSI(line1, w)

	indent := "        "
	var line2 string
	if len(r.Hits) > 0 {
		line2 = indent + highlightSnippet(r.Hits[0], w-len(indent)-1)
	} else {
		line2 = indent + sSub.Render(truncate(p.Snippet(), w-len(indent)-1))
	}
	meta := []string{p.Repo()}
	if b := p.Branch(); b != "" {
		meta = append(meta, "⎇ "+b)
	}
	meta = append(meta, fmt.Sprintf("%d msgs", p.NUser))
	if p.Cost >= 0.01 {
		meta = append(meta, fmt.Sprintf("$%.2f", p.Cost))
	}
	if p.Meta.Note != "" {
		meta = append(meta, "📝 "+p.Meta.Note)
	}
	line3 := indent + sDim.Render(truncate(strings.Join(meta, " · "), w-len(indent)-1))
	out := line1 + "\n" + line2 + "\n" + line3
	if selected {
		out = lipgloss.NewStyle().Background(cSurface).Width(w).Render(out)
	}
	return zone.Mark(ZoneRow(p.ID), out)
}

func roleIcon(r claude.Role) string {
	switch r {
	case claude.RoleUser:
		return "you›"
	case claude.RoleAssistant:
		return "🐕"
	case claude.RoleTool:
		return "⚙"
	case claude.RoleRecap:
		return "✦"
	}
	return "·"
}

func highlightSnippet(h store.Hit, w int) string {
	prefix := sDim.Render(roleIcon(h.Role) + " ")
	text := h.Snippet
	var b strings.Builder
	width := lipgloss.Width(prefix)
	in := false
	for _, r := range text {
		switch string(r) {
		case store.SnipOpen:
			in = true
			continue
		case store.SnipClose:
			in = false
			continue
		}
		if width >= w-1 {
			b.WriteString(sDim.Render("…"))
			break
		}
		ch := string(r)
		if in {
			b.WriteString(sMatch.Render(ch))
		} else {
			b.WriteString(sSub.Render(ch))
		}
		width += lipgloss.Width(ch)
	}
	return prefix + b.String()
}

func truncateANSI(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

func (m Model) preview(w, h int) string {
	p := m.selected()
	box := lipgloss.NewStyle().Width(w - 3).Height(h).MaxHeight(h).PaddingLeft(1).PaddingRight(1).
		BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(cFaint)
	if p == nil {
		return box.Render("\n\n" + center(sDim.Render("select a pup"), w-4))
	}
	now := m.app.Now()
	inner := w - 6
	sprite := pupSprite(p, m.frame, false)
	title := lipgloss.NewStyle().Foreground(furColorFor(p.ID, p.Color, p.AgentKind())).Bold(true).Render(truncate(p.Title(), inner-14))
	st := lipgloss.NewStyle().Foreground(statusColor(p.Status)).Render("● " + statusLabel(p.Status))
	if p.Pane != "" {
		st += sDim.Render("  herdr " + p.Pane)
	}
	idLine := sDim.Render(p.ID)
	headRight := lipgloss.JoinVertical(lipgloss.Left, title, st, idLine)
	head := lipgloss.JoinHorizontal(lipgloss.Top, sprite, "  ", headRight)

	kv := func(k, v string) string {
		if v == "" {
			return ""
		}
		return sDim.Width(10).Render(k) + sText.Render(truncate(v, inner-10))
	}
	var details []string
	add := func(s string) {
		if s != "" {
			details = append(details, s)
		}
	}
	add(kv("agent", app.AgentNames[p.AgentKind()]))
	add(kv("project", shortPath(p.Cwd)))
	if p.Worktree != "" {
		add(kv("worktree", p.Worktree))
	}
	add(kv("branch", strings.Join(lastN(p.Branches, 3), ", ")))
	add(kv("when", fmt.Sprintf("%s → %s ago", p.Started.Local().Format("Jan 2 15:04"), ago(now, p.Updated))))
	add(kv("size", fmt.Sprintf("%d prompts · %d replies · %d tools", p.NUser, p.NAssistant, p.NTools)))
	if p.Cost > 0 {
		add(kv("cost", fmt.Sprintf("$%.2f", p.Cost)))
	}
	if len(p.Models) > 0 {
		add(kv("model", strings.Join(p.Models, ", ")))
	}
	for _, pr := range lastN(p.PRs, 3) {
		add(kv("PR", pr))
	}
	if p.Remote != "" {
		add(kv("remote", "📡 remote control session"))
	}
	if p.ContinuedIn != "" {
		add(kv("continued", p.ContinuedIn[:8]))
	}
	if p.Kennel != nil {
		add(kv("kennel", p.Kennel.Icon+" "+p.Kennel.Name))
	}

	section := func(t string) string { return "\n" + sAccent.Render(t) }
	body := []string{head, "", strings.Join(details, "\n")}
	if p.Meta.Note != "" {
		body = append(body, section("📝 note"), wrap(p.Meta.Note, inner))
	}
	if p.Recap != "" {
		body = append(body, section("✦ recap"), sText.Render(wrap(p.Recap, inner)))
	}
	if r := m.currentResult(); r != nil && len(r.Hits) > 0 {
		body = append(body, section("🔎 matches"))
		for _, hit := range r.Hits {
			body = append(body, highlightSnippet(hit, inner))
		}
	}
	if p.FirstPrompt != "" {
		body = append(body, section("› first ask"), sSub.Render(wrap(truncate(p.FirstPrompt, inner*3), inner)))
	}
	if p.LastPrompt != "" && p.LastPrompt != p.FirstPrompt {
		body = append(body, section("› last ask"), sSub.Render(wrap(truncate(p.LastPrompt, inner*2), inner)))
	}
	if len(p.Crons) > 0 {
		body = append(body, section("⏰ scheduled"))
		for _, c := range p.Crons {
			body = append(body, sText.Render(c.Expr)+sDim.Render(fmt.Sprintf("  fired %d× · last %s ago", c.Fires, ago(now, c.LastFire))))
		}
	}
	if len(p.Artifacts) > 0 {
		body = append(body, section("🎨 artifacts"))
		for _, a := range lastN(p.Artifacts, 4) {
			body = append(body, sText.Render(truncate(orDefault(a.Title, a.URL), inner)))
		}
	}
	return box.Render(strings.Join(body, "\n"))
}

func (m Model) currentResult() *search.Result {
	if m.tab == TabPups && m.cursor < len(m.results) {
		return &m.results[m.cursor]
	}
	return nil
}

func lastN[T any](xs []T, n int) []T {
	if len(xs) <= n {
		return xs
	}
	return xs[len(xs)-n:]
}

func wrap(s string, w int) string {
	return lipgloss.NewStyle().Width(w).Render(s)
}
