package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/pack"
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

// pupSprite draws a 4-line cartoon pup: speech bubble, head, body, name tag.
func pupSprite(p *pack.Pup, frame int, selected bool) string {
	fur := furColor(p.ID, p.Color)
	st := statusColor(p.Status)
	dim := !p.Status.Live()

	furStyle := lipgloss.NewStyle().Foreground(fur)
	faceStyle := lipgloss.NewStyle().Foreground(cText)
	if dim {
		furStyle = furStyle.Faint(true)
		faceStyle = faceStyle.Foreground(cDim)
	}
	bub := lipgloss.NewStyle().Foreground(st).Bold(true).Render(bubble(p.Status, frame))
	head := furStyle.Render("▼") + faceStyle.Render(face(p.Status, frame)) + furStyle.Render("▼")
	body := furStyle.Render(" ▕▁▁▏") + lipgloss.NewStyle().Foreground(fur).Render(tail(p.Status, frame))
	if p.Status == claude.Working && frame%2 == 1 {
		body = furStyle.Render(" ▕▁▁▏") + lipgloss.NewStyle().Foreground(fur).Render(tail(p.Status, frame))
	}

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
	fur := furColor(p.ID, p.Color)
	fs := lipgloss.NewStyle().Foreground(fur)
	inner := lipgloss.NewStyle().Foreground(statusColor(p.Status)).Bold(p.Status.Live())
	if !p.Status.Live() {
		fs = fs.Faint(true)
	}
	return fs.Render("▼") + inner.Render(face(p.Status, frame)) + fs.Render("▼")
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
