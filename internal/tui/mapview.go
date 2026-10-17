package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/razkevich/kennel/internal/floor"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/things"
)

var floors = floor.Names

type room struct {
	key    string
	title  string
	icon   string
	color  lipgloss.Color
	look   pack.Look
	blurb  string
	pups   []*pack.Pup
	things []things.Thing
}

// mapItem is whatever stands on a tile: a pup (agent session) or a thing (any other terminal).
type mapItem struct {
	pup   *pack.Pup
	thing *things.Thing
}

func (it mapItem) id() string {
	if it.pup != nil {
		return it.pup.ID
	}
	return it.thing.Key()
}

func (r room) items() []mapItem {
	out := make([]mapItem, 0, len(r.pups)+len(r.things))
	for _, p := range r.pups {
		out = append(out, mapItem{pup: p})
	}
	for i := range r.things {
		out = append(out, mapItem{thing: &r.things[i]})
	}
	return out
}

type roomLayout struct {
	key   string
	items []mapItem
	cols  int
}

const recentWindow = 21 * 24 * time.Hour

func (m Model) rooms() []room {
	var out []room
	for _, r := range floor.Rooms(m.app, m.floor) {
		blurb := r.Blurb
		if strings.HasPrefix(blurb, "/") {
			blurb = shortPath(blurb)
		}
		out = append(out, room{key: r.Key, title: r.Title, icon: r.Icon, color: lipgloss.Color(r.Color), look: r.Look, blurb: blurb, pups: r.Pups, things: r.Things})
	}
	return out
}

func shortPath(p string) string {
	parts := strings.Split(p, "/")
	if len(parts) > 3 {
		return "…/" + strings.Join(parts[len(parts)-2:], "/")
	}
	return p
}

func (m Model) mapView(w, h int) string {
	floorBar := m.floorBar()
	cardH := 5
	gridH := h - 2 - cardH
	rooms := m.rooms()
	maxRows := m.mapMaxRows()
	layout := m.mapGeometry()
	sel := mapSelection(m.mapSel, layout)

	// Flow rooms left to right, wrapping at the window edge.
	var lines []string
	var row []string
	rowW := 0
	selLine := 0
	flush := func() {
		if len(row) == 0 {
			return
		}
		lines = append(lines, strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, row...), "\n")...)
		row, rowW = nil, 0
	}
	for i, r := range rooms {
		b := m.renderRoom(r, w-2, maxRows, sel)
		bw := lipgloss.Width(b)
		if rowW+bw > w-1 && len(row) > 0 {
			flush()
		}
		for _, it := range layout[i].items {
			if it.id() == sel {
				selLine = len(lines)
			}
		}
		row = append(row, b)
		rowW += bw
	}
	flush()

	scroll := 0
	if selLine+4*maxRows+3 > gridH {
		scroll = min(selLine, max(0, len(lines)-gridH))
	}
	visible := lines[scroll:min(len(lines), scroll+gridH)]
	grid := strings.Join(visible, "\n")
	card := m.hoverCard(w, sel)
	return floorBar + "\n" + lipgloss.NewStyle().Height(gridH).MaxHeight(gridH).Render(grid) + "\n" + card
}

func (m Model) floorBar() string {
	var parts []string
	for i, f := range floors {
		st := sDim
		if i == m.floor {
			st = lipgloss.NewStyle().Foreground(cPink).Bold(true).Underline(true)
		}
		parts = append(parts, st.Render(f))
	}
	return " " + strings.Join(parts, sFaint.Render("  ·  ")) + sFaint.Render("   (F to change floor)")
}

func geometry(r room, maxW, maxRows int) (cols, rows int, shown []mapItem, more int) {
	items := r.items()
	n := len(items)
	maxCols := 4
	if r.look.Wide {
		maxCols = 6
	}
	maxCols = max(1, min(maxCols, (maxW-4)/spriteW))
	cols = max(1, min(maxCols, int(math.Ceil(math.Sqrt(float64(n)*1.6)))))
	if n == 0 {
		cols = 2
	}
	rows = max(1, min(maxRows, (n+cols-1)/cols))
	shown = items
	if n > cols*rows {
		shown = items[:cols*rows-1]
		more = n - len(shown)
	}
	return
}

func (m Model) mapMaxRows() int {
	if m.h > 46 {
		return 3
	}
	return 2
}

func (m Model) mapGeometry() []roomLayout {
	var out []roomLayout
	for _, r := range m.rooms() {
		cols, _, shown, _ := geometry(r, m.w-2, m.mapMaxRows())
		out = append(out, roomLayout{key: r.key, items: shown, cols: cols})
	}
	return out
}

// mapSelection is the selected item if it's on this floor, else the first one shown.
func mapSelection(sel string, layout []roomLayout) string {
	first := ""
	for _, l := range layout {
		for _, it := range l.items {
			if it.id() == sel {
				return sel
			}
			if first == "" {
				first = it.id()
			}
		}
	}
	return first
}

func (m Model) renderRoom(r room, maxW, maxRows int, sel string) string {
	n := len(r.pups) + len(r.things)
	cols, rows, shown, more := geometry(r, maxW, maxRows)

	innerW := cols * spriteW
	color := r.color
	selectedHere := false
	for _, it := range shown {
		if it.id() == sel {
			selectedHere = true
		}
	}

	title := lipgloss.NewStyle().Foreground(color).Bold(true).Render(r.icon + " " + r.title)
	count := sDim.Render(fmt.Sprintf(" %d", n))
	unread := 0
	for _, p := range r.pups {
		if p.Unread() {
			unread++
		}
	}
	if unread > 0 {
		count += " " + lipgloss.NewStyle().Foreground(cBase).Background(cYellow).Bold(true).Render(fmt.Sprintf(" %d! ", unread))
	}
	headerLine := title + count
	if lipgloss.Width(headerLine) > innerW {
		innerW = lipgloss.Width(headerLine)
	}

	floorCell := func(width int) string {
		ch := r.look.Floor
		if ch == "" || ch == " " {
			return strings.Repeat(" ", width)
		}
		var b strings.Builder
		for x := 0; x < width; x++ {
			if x%3 == 1 {
				b.WriteString(ch)
			} else {
				b.WriteByte(' ')
			}
		}
		return sFaint.Render(b.String())
	}
	emptySlot := func() string {
		lines := make([]string, 4)
		for i := range lines {
			lines[i] = floorCell(spriteW)
		}
		return strings.Join(lines, "\n")
	}

	var gridRows []string
	for rIdx := 0; rIdx < rows; rIdx++ {
		var cells []string
		for c := 0; c < cols; c++ {
			i := rIdx*cols + c
			switch {
			case i < len(shown):
				it := shown[i]
				if it.pup != nil {
					cells = append(cells, zone.Mark(ZonePup(it.pup.ID), pupSprite(it.pup, m.frame+i, it.id() == sel)))
				} else {
					cells = append(cells, zone.Mark(ZoneThing(it.thing.Key()), thingSprite(*it.thing, m.frame+i, it.id() == sel)))
				}
			case i == len(shown) && more > 0:
				moreCell := lipgloss.NewStyle().Width(spriteW).Height(4).Align(lipgloss.Center, lipgloss.Center).
					Foreground(cSub).Render(fmt.Sprintf("+%d\nmore", more))
				cells = append(cells, moreCell)
			default:
				cells = append(cells, emptySlot())
			}
		}
		gridRows = append(gridRows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	grid := strings.Join(gridRows, "\n")
	if n == 0 {
		msg := sDim.Italic(true).Render(r.blurb)
		if r.look.Decor != "" {
			msg = r.look.Decor + "  " + msg
		}
		grid = lipgloss.NewStyle().Width(innerW).Height(4).Align(lipgloss.Center, lipgloss.Center).Render(truncate(msg, innerW))
	}
	gw := lipgloss.Width(grid)
	if gw < innerW {
		grid = lipgloss.JoinHorizontal(lipgloss.Top, grid, lipgloss.NewStyle().Width(innerW-gw).Render(""))
	}

	decor := ""
	if r.look.Decor != "" && n > 0 {
		decor = r.look.Decor
	}
	hdrGap := innerW - lipgloss.Width(headerLine) - lipgloss.Width(decor)
	header := headerLine + strings.Repeat(" ", max(0, hdrGap)) + decor
	floorStrip := floorCell(innerW)

	border := borderFor(r.look.Border)
	st := lipgloss.NewStyle().Border(border).BorderForeground(color).Padding(0, 1)
	if !selectedHere {
		st = st.BorderForeground(dimmed(color))
	}
	return st.Render(header + "\n" + grid + "\n" + floorStrip)
}

// dimmed fades a room's border when it isn't the one you're standing in.
func dimmed(c lipgloss.Color) lipgloss.TerminalColor {
	return lipgloss.AdaptiveColor{Dark: blend(string(c), "#1e1e2e", 0.45), Light: string(c)}
}

func blend(a, b string, t float64) string {
	var r1, g1, b1, r2, g2, b2 int
	if _, err := fmt.Sscanf(a, "#%02x%02x%02x", &r1, &g1, &b1); err != nil {
		return a
	}
	if _, err := fmt.Sscanf(b, "#%02x%02x%02x", &r2, &g2, &b2); err != nil {
		return a
	}
	mix := func(x, y int) int { return int(float64(x)*(1-t) + float64(y)*t) }
	return fmt.Sprintf("#%02x%02x%02x", mix(r1, r2), mix(g1, g2), mix(b1, b2))
}

func (m Model) hoverCard(w int, sel string) string {
	if t := m.app.Thing(sel); t != nil {
		return m.thingCard(w, *t)
	}
	p := m.app.Pup(sel)
	if p == nil {
		msg := "No pups on this floor yet."
		if m.floor == 2 {
			msg = "No kennels yet — press K to build one, then m on a pup to move it in."
		}
		return lipgloss.NewStyle().Width(w - 2).Height(3).Border(lipgloss.RoundedBorder()).BorderForeground(cFaint).Render(sDim.Render(msg))
	}
	now := m.app.Now()
	st := statusColor(p.Status)
	title := lipgloss.NewStyle().Foreground(furColor(p.ID, p.Color)).Bold(true).Render(p.Title())
	status := lipgloss.NewStyle().Foreground(st).Render("● " + statusLabel(p.Status))
	if p.Status.Live() && !p.Live.Since.IsZero() {
		status += sDim.Render(" for " + strings.TrimSuffix(ago(now, p.Live.Since), "now"))
	}
	meta := []string{p.Repo()}
	if b := p.Branch(); b != "" {
		meta = append(meta, b)
	}
	meta = append(meta, fmt.Sprintf("%d msgs", p.NUser), ago(now, p.Updated)+" ago")
	if len(p.PRs) > 0 {
		meta = append(meta, "PR "+prNumber(p.PRs[len(p.PRs)-1]))
	}
	line1 := title + "  " + status + "  " + sDim.Render(strings.Join(meta, " · "))
	snippet := sSub.Render(truncate(p.Snippet(), w-6))
	actions := hint("⏎", "open") + "  " + hint("v", "read") + "  " + hint("s", "star") + "  " + hint("m", "move") + "  " + hint("t", "reply")
	return lipgloss.NewStyle().Width(w-2).Border(lipgloss.RoundedBorder()).BorderForeground(st).Padding(0, 1).
		Render(truncate(line1, w-6) + "\n" + snippet + "\n" + actions)
}

func (m Model) thingCard(w int, t things.Thing) string {
	color := cDim
	state := "stopped"
	if t.Running {
		color, state = cGreen, "running"
		if t.Pane != "" {
			state += " in herdr " + t.Pane
		} else if t.PID > 0 {
			state += fmt.Sprintf(" (pid %d)", t.PID)
		}
	}
	title := lipgloss.NewStyle().Foreground(cPeach).Bold(true).Render(things.Icon[t.Kind] + " " + t.Name)
	line1 := title + "  " + lipgloss.NewStyle().Foreground(color).Render("● "+state) + sDim.Render("  "+string(t.Kind)+" · "+shortPath(t.Cwd))
	line2 := sSub.Render("$ " + truncate(t.Command, w-8))
	var acts []string
	if t.Running {
		acts = append(acts, hint("⏎", "focus"), hint("X", "stop"))
	} else {
		acts = append(acts, hint("⏎", "start"))
	}
	if t.Saved {
		acts = append(acts, hint("m", "move"), hint("D", "forget"))
	} else {
		acts = append(acts, hint("S", "save as a thing"))
	}
	return lipgloss.NewStyle().Width(w-2).Border(lipgloss.RoundedBorder()).BorderForeground(color).Padding(0, 1).
		Render(truncate(line1, w-6) + "\n" + line2 + "\n" + strings.Join(acts, "  "))
}

func prNumber(url string) string {
	if i := strings.LastIndex(url, "/"); i >= 0 {
		return "#" + url[i+1:]
	}
	return url
}

// moveMap walks the selection through rooms: dx within a row, dy across rows/rooms.
func (m *Model) moveMap(dx, dy int) {
	m.mapLayout = m.mapGeometry()
	m.mapSel = mapSelection(m.mapSel, m.mapLayout)
	ri, pi := -1, -1
	for r, l := range m.mapLayout {
		for i, it := range l.items {
			if it.id() == m.mapSel {
				ri, pi = r, i
			}
		}
	}
	if ri < 0 {
		return
	}
	l := m.mapLayout[ri]
	next := pi + dx + dy*l.cols
	if next >= 0 && next < len(l.items) && (dy != 0 || (pi/l.cols == next/l.cols)) {
		m.mapSel = l.items[next].id()
		return
	}
	dir := 1
	if dx < 0 || dy < 0 {
		dir = -1
	}
	for r := ri + dir; r >= 0 && r < len(m.mapLayout); r += dir {
		if its := m.mapLayout[r].items; len(its) > 0 {
			if dir > 0 {
				m.mapSel = its[0].id()
			} else {
				m.mapSel = its[len(its)-1].id()
			}
			return
		}
	}
}

func (m *Model) jumpRoom(dir int) {
	m.mapLayout = m.mapGeometry()
	m.mapSel = mapSelection(m.mapSel, m.mapLayout)
	ri := 0
	for r, l := range m.mapLayout {
		for _, it := range l.items {
			if it.id() == m.mapSel {
				ri = r
			}
		}
	}
	n := len(m.mapLayout)
	for step := 1; step <= n; step++ {
		r := ((ri+dir*step)%n + n) % n
		if its := m.mapLayout[r].items; len(its) > 0 {
			m.mapSel = its[0].id()
			return
		}
	}
}
