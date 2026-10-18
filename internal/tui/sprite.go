package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/things"
)

const spriteW = 13

func face(s claude.Status, frame int) string {
	switch s {
	case claude.Working:
		return [...]string{"•ᴥ•", "•ᴥ•", "-ᴥ•", "•ᴥ•"}[frame%4]
	case claude.Waiting:
		return "°ᴥ°"
	case claude.Done:
		return "^ᴥ^"
	case claude.Idle:
		return "•ᴥ•"
	}
	return "-ᴥ-"
}

func tail(s claude.Status, frame int) string {
	switch s {
	case claude.Working:
		return [...]string{"∿", "~", "≀", "~"}[frame%4]
	case claude.Waiting, claude.Done:
		return [...]string{"~", "∿"}[frame%2]
	}
	return "_"
}

func bubble(s claude.Status, frame int) string {
	switch s {
	case claude.Working:
		return [...]string{"·  ", "·· ", "···", " ··"}[frame%4]
	case claude.Waiting:
		if frame%4 < 2 {
			return " ! "
		}
		return "(!)"
	case claude.Done:
		return " ✓ "
	case claude.Idle:
		return "   "
	}
	return [...]string{" z ", " zZ", "  Z", "   "}[(frame/2)%4]
}

// ears tell agents apart at a glance: Claude corgis, Codex huskies, Cursor dalmatians.
func ears(agent string) (string, string) {
	switch agent {
	case "codex":
		return "▲", "▲"
	case "cursor":
		return "◖", "◗"
	}
	return "▼", "▼"
}

func belly(agent string) string {
	if agent == "cursor" {
		return " ▕▚▞▏"
	}
	return " ▕▁▁▏"
}

// pupSprite draws a 4-line cartoon pup: speech bubble, head, body, name tag.
func pupSprite(p *pack.Pup, frame int, selected bool) string {
	fur := furColorFor(p.ID, p.Color, p.AgentKind())
	st := statusColor(p.Status)
	dim := !p.Status.Live()

	furStyle := lipgloss.NewStyle().Foreground(fur)
	faceStyle := lipgloss.NewStyle().Foreground(cText)
	if dim {
		furStyle = furStyle.Faint(true)
		faceStyle = faceStyle.Foreground(cDim)
	}
	bub := lipgloss.NewStyle().Foreground(st).Bold(true).Render(bubble(p.Status, frame))
	l, r := ears(p.AgentKind())
	head := furStyle.Render(l) + faceStyle.Render(face(p.Status, frame)) + furStyle.Render(r)
	body := furStyle.Render(belly(p.AgentKind())) + lipgloss.NewStyle().Foreground(fur).Render(tail(p.Status, frame))

	badge := ""
	switch {
	case p.Meta.Starred:
		badge = "★"
	case p.Remote != "":
		badge = "📡"
	case len(p.Crons) > 0:
		badge = "⏰"
	}
	name := truncate(p.Title(), spriteW-2)
	nameStyle := lipgloss.NewStyle().Foreground(cSub)
	if p.Status.Live() {
		nameStyle = nameStyle.Foreground(cText)
	}
	if selected {
		nameStyle = lipgloss.NewStyle().Foreground(cBase).Background(cPink).Bold(true)
	}
	lines := []string{
		center("  "+bub, spriteW),
		center(head+" "+badgeOr(badge), spriteW),
		center(body+" ", spriteW),
		center(nameStyle.Render(name), spriteW),
	}
	return strings.Join(lines, "\n")
}

func badgeOr(b string) string {
	if b == "" {
		return " "
	}
	return lipgloss.NewStyle().Foreground(cYellow).Render(b)
}

// miniFace is the one-cell-row status marker used in lists.
func miniFace(p *pack.Pup, frame int) string {
	fur := furColorFor(p.ID, p.Color, p.AgentKind())
	fs := lipgloss.NewStyle().Foreground(fur)
	inner := lipgloss.NewStyle().Foreground(statusColor(p.Status)).Bold(p.Status.Live())
	if !p.Status.Live() {
		fs = fs.Faint(true)
	}
	l, r := ears(p.AgentKind())
	return fs.Render(l) + inner.Render(face(p.Status, frame)) + fs.Render(r)
}

func truncate(s string, w int) string {
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

func center(s string, w int) string {
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, s)
}

// thingSprite draws a non-agent terminal as an object, not a pup: a little
// device whose screen animates while it runs.
func thingSprite(t things.Thing, frame int, selected bool) string {
	on := t.Running
	frameSt := lipgloss.NewStyle().Foreground(cPeach)
	screenSt := lipgloss.NewStyle().Foreground(cGreen)
	if !on {
		frameSt = lipgloss.NewStyle().Foreground(cDim)
		screenSt = sFaint
	}
	icon := things.Icon[t.Kind]
	dot := sFaint.Render("○")
	if on {
		dot = lipgloss.NewStyle().Foreground(cGreen).Render("●")
	}
	screen := thingScreen(t.Kind, on, frame)
	nameSt := lipgloss.NewStyle().Foreground(cSub)
	if on {
		nameSt = nameSt.Foreground(cText)
	}
	if selected {
		nameSt = lipgloss.NewStyle().Foreground(cBase).Background(cPeach).Bold(true)
	}
	lines := []string{
		center(icon+" "+dot, spriteW),
		center(frameSt.Render("▐")+screenSt.Render(screen)+frameSt.Render("▌"), spriteW),
		center(frameSt.Render("▝▀▀▀▀▀▀▀▘"), spriteW),
		center(nameSt.Render(truncate(t.Name, spriteW-2)), spriteW),
	}
	return strings.Join(lines, "\n")
}

func thingScreen(k things.Kind, on bool, frame int) string {
	if !on {
		return "  off  "
	}
	switch k {
	case things.Monitor:
		bars := []rune("▁▂▃▄▅▆▇")
		out := make([]rune, 7)
		for i := range out {
			out[i] = bars[(frame*3+i*5+i*i)%len(bars)]
		}
		return string(out)
	case things.Server:
		return [...]string{" ▲ ▲▴ ", " ▴▲ ▲ ", " ▲▴▲ ▴", " ▴ ▲▲ "}[frame%4] + " "
	case things.Logs:
		return [...]string{"≡ ≡≡ ≡ ", " ≡≡ ≡≡", "≡≡ ≡ ≡", "≡ ≡ ≡≡"}[frame%4] + ""
	case things.Job:
		n := frame % 8
		return strings.Repeat("▰", min(n, 7)) + strings.Repeat("▱", 7-min(n, 7))
	}
	return [...]string{" ·     ", "  ·    ", "   ·   ", "    ·  "}[frame%4]
}
