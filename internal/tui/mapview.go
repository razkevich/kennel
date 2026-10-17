package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/razkevich/kennel/internal/app"
	"github.com/razkevich/kennel/internal/pack"
)

var floors = []string{"🏡 Campus", "📁 Projects", "🏠 Kennels"}

type room struct {
	key   string
	title string
	icon  string
	color lipgloss.Color
	look  pack.Look
	blurb string
	pups  []*pack.Pup
}

type roomLayout struct {
	key  string
	pups []*pack.Pup
	cols int
}

// campusOrder decides which room a pup stands in when several smart kennels match.
var campusOrder = []string{"yard", "best", "fire", "helping", "court", "yak", "water", "marathon", "shipped", "rug", "dumpster"}

const recentWindow = 21 * 24 * time.Hour

func (m Model) rooms() []room {
	now := m.app.Now()
	pups := m.app.Pups()
	var rooms []room
	switch m.floor {
	case 0:
		placed := map[string]bool{}
		for _, key := range campusOrder {
			sm := pack.SmartByKey(key)
			r := room{key: key, title: sm.Name, icon: sm.Icon, color: lipgloss.Color(sm.Color), look: sm.Look, blurb: sm.Blurb}
			for _, p := range pups {
				if placed[p.ID] || !sm.Match(p, now) {
					continue
				}
				if key != "rug" && key != "dumpster" && !p.Status.Live() && now.Sub(p.Updated) > recentWindow {
					continue
				}
				placed[p.ID] = true
				r.pups = append(r.pups, p)
			}
			rooms = append(rooms, r)
		}
		lobby := room{key: "lobby", title: "Lobby", icon: "🛋️", color: cLav, look: pack.Look{Border: "rounded", Floor: " "}, blurb: "everyone else this month"}
		for _, p := range pups {
			if !placed[p.ID] && !p.Meta.Archived && !p.Empty() && now.Sub(p.Updated) < recentWindow {
				lobby.pups = append(lobby.pups, p)
			}
		}
		rooms = append(rooms[:1], append([]room{lobby}, rooms[1:]...)...)
	case 1:
		floorsChars := []string{"·", "⠂", "∴", "⁘", "~", " "}
		borders := []string{"rounded", "double", "thick", "normal"}
		palette := []lipgloss.Color{cSky, cPeach, cMauve, cGreen, cYellow, cPink, cTeal, cLav, cRed, cBlue}
		for i, pr := range m.app.Projects() {
			r := room{key: "project:" + pr.Root, title: pr.Name, icon: "📁", color: palette[i%len(palette)],
				look: pack.Look{Border: borders[i%len(borders)], Floor: floorsChars[i%len(floorsChars)], Wide: pr.Count > 12}, blurb: shortPath(pr.Root)}
			for _, p := range pups {
				if app.ProjectRoot(p.Cwd) == pr.Root && !p.Meta.Archived && !p.Empty() && (p.Status.Live() || now.Sub(p.Updated) < recentWindow) {
					r.pups = append(r.pups, p)
				}
			}
			rooms = append(rooms, r)
		}
	case 2:
		for _, k := range m.app.Kennels() {
			icon := k.Icon
			if icon == "" {
				icon = "🏠"
			}
			color := lipgloss.Color(k.Color)
			if k.Color == "" {
				color = cPeach
			}
			r := room{key: fmt.Sprintf("kennel:%d", k.ID), title: k.Name, icon: icon, color: color, look: pack.Look{Border: "rounded", Floor: "·"}}
			for _, p := range pups {
				if p.Kennel != nil && p.Kennel.ID == k.ID && !p.Meta.Archived {
					r.pups = append(r.pups, p)
				}
			}
			rooms = append(rooms, r)
		}
		if len(m.app.Kennels()) == 0 {
			rooms = append(rooms, room{key: "new-kennel", title: "Your first kennel", icon: "＋", color: cPeach,
				look: pack.Look{Border: "hidden", Floor: " ", Decor: "🔨"}, blurb: "No kennels yet — press K to build one"})
		}
		strays := room{key: "strays", title: "Strays", icon: "🐾", color: cDim, look: pack.Look{Border: "hidden", Floor: "⠂", Wide: true}, blurb: "recent pups without a kennel — m to move one in"}
		for _, p := range pups {
			if p.Kennel == nil && !p.Meta.Archived && !p.Empty() && (p.Status.Live() || now.Sub(p.Updated) < 7*24*time.Hour) {
				strays.pups = append(strays.pups, p)
			}
		}
		rooms = append(rooms, strays)
	}
	return rooms
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
		for _, p := range layout[i].pups {
			if p.ID == sel {
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

func geometry(r room, maxW, maxRows int) (cols, rows int, shown []*pack.Pup, more int) {
	n := len(r.pups)
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
	shown = r.pups
	if n > cols*rows {
		shown = r.pups[:cols*rows-1]
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
		out = append(out, roomLayout{key: r.key, pups: shown, cols: cols})
	}
	return out
}

// mapSelection is the selected pup if it's on this floor, else the first pup shown.
func mapSelection(sel string, layout []roomLayout) string {
	first := ""
	for _, l := range layout {
		for _, p := range l.pups {
			if p.ID == sel {
				return sel
			}
			if first == "" {
				first = p.ID
			}
		}
	}
	return first
}

func (m Model) renderRoom(r room, maxW, maxRows int, sel string) string {
	n := len(r.pups)
	cols, rows, shown, more := geometry(r, maxW, maxRows)

	innerW := cols * spriteW
	color := r.color
	selectedHere := false
	for _, p := range shown {
		if p.ID == sel {
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
				p := shown[i]
				cells = append(cells, zone.Mark(ZonePup(p.ID), pupSprite(p, m.frame+i, p.ID == sel)))
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
	bc := lipgloss.Color(string(color))
	st := lipgloss.NewStyle().Border(border).BorderForeground(bc).Padding(0, 1)
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
	p := m.app.Pup(sel)
	if p == nil {
		msg := "No pups on this floor yet."
		if m.floor == 2 {
			msg = "No kennels yet — press K to build one, then m on a pup to move it in."
		}
		return lipgloss.NewStyle().Width(w-2).Height(3).Border(lipgloss.RoundedBorder()).BorderForeground(cFaint).Render(sDim.Render(msg))
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
		for i, p := range l.pups {
			if p.ID == m.mapSel {
				ri, pi = r, i
			}
		}
	}
	if ri < 0 {
		for _, l := range m.mapLayout {
			if len(l.pups) > 0 {
				m.mapSel = l.pups[0].ID
				return
			}
		}
		return
	}
	l := m.mapLayout[ri]
	next := pi + dx + dy*l.cols
	if next >= 0 && next < len(l.pups) && (dy != 0 || (pi/l.cols == next/l.cols)) {
		m.mapSel = l.pups[next].ID
		return
	}
	dir := 1
	if dx < 0 || dy < 0 {
		dir = -1
	}
	for r := ri + dir; r >= 0 && r < len(m.mapLayout); r += dir {
		if ps := m.mapLayout[r].pups; len(ps) > 0 {
			if dir > 0 {
				m.mapSel = ps[0].ID
			} else {
				m.mapSel = ps[len(ps)-1].ID
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
		for _, p := range l.pups {
			if p.ID == m.mapSel {
				ri = r
			}
		}
	}
	n := len(m.mapLayout)
	for step := 1; step <= n; step++ {
		r := ((ri+dir*step)%n + n) % n
		if ps := m.mapLayout[r].pups; len(ps) > 0 {
			m.mapSel = ps[0].ID
			return
		}
	}
}
