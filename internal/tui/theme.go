package tui

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"
	"github.com/razkevich/kennel/internal/claude"
)

// Palette loosely follows Catppuccin Mocha: soft, saturated, cartoon-friendly.
var (
	cText    = lipgloss.Color("#cdd6f4")
	cSub     = lipgloss.Color("#a6adc8")
	cDim     = lipgloss.Color("#6c7086")
	cFaint   = lipgloss.Color("#45475a")
	cSurface = lipgloss.Color("#313244")
	cBase    = lipgloss.Color("#1e1e2e")
	cPink    = lipgloss.Color("#f5c2e7")
	cMauve   = lipgloss.Color("#cba6f7")
	cRed     = lipgloss.Color("#f38ba8")
	cPeach   = lipgloss.Color("#fab387")
	cYellow  = lipgloss.Color("#f9e2af")
	cGreen   = lipgloss.Color("#a6e3a1")
	cTeal    = lipgloss.Color("#94e2d5")
	cSky     = lipgloss.Color("#89dceb")
	cBlue    = lipgloss.Color("#89b4fa")
	cLav     = lipgloss.Color("#b4befe")
)

var furs = []lipgloss.Color{"#fab387", "#f9e2af", "#e6c9a8", "#c9a27e", "#f5c2e7", "#b4befe", "#94e2d5", "#eba0ac", "#d9d9d9", "#a6adc8"}

var namedColors = map[string]lipgloss.Color{
	"red": cRed, "blue": cBlue, "green": cGreen, "yellow": cYellow, "purple": cMauve,
	"orange": cPeach, "pink": cPink, "cyan": cSky, "teal": cTeal, "white": cText,
}

// furColor is a pup's own colour: its /color if set, else a stable pick from its id.
func furColor(id, named string) lipgloss.Color {
	if c, ok := namedColors[named]; ok {
		return c
	}
	h := fnv.New32a()
	h.Write([]byte(id))
	return furs[h.Sum32()%uint32(len(furs))]
}

func statusColor(s claude.Status) lipgloss.Color {
	switch s {
	case claude.Working:
		return cGreen
	case claude.Waiting:
		return cYellow
	case claude.Done:
		return cBlue
	case claude.Idle:
		return cTeal
	}
	return cDim
}

func statusLabel(s claude.Status) string {
	switch s {
	case claude.Working:
		return "working"
	case claude.Waiting:
		return "needs you"
	case claude.Done:
		return "done"
	case claude.Idle:
		return "idle"
	}
	return "asleep"
}

var (
	sText   = lipgloss.NewStyle().Foreground(cText)
	sSub    = lipgloss.NewStyle().Foreground(cSub)
	sDim    = lipgloss.NewStyle().Foreground(cDim)
	sFaint  = lipgloss.NewStyle().Foreground(cFaint)
	sBold   = lipgloss.NewStyle().Foreground(cText).Bold(true)
	sAccent = lipgloss.NewStyle().Foreground(cPink).Bold(true)
	sKey    = lipgloss.NewStyle().Foreground(cBase).Background(cMauve).Bold(true).Padding(0, 1)
	sHint   = lipgloss.NewStyle().Foreground(cSub)
	sMatch  = lipgloss.NewStyle().Foreground(cBase).Background(cYellow)
	sErr    = lipgloss.NewStyle().Foreground(cRed).Bold(true)
	sOK     = lipgloss.NewStyle().Foreground(cGreen).Bold(true)
)

func borderFor(name string) lipgloss.Border {
	switch name {
	case "double":
		return lipgloss.DoubleBorder()
	case "thick":
		return lipgloss.ThickBorder()
	case "normal":
		return lipgloss.NormalBorder()
	case "hidden":
		return lipgloss.Border{Top: "┄", Bottom: "┄", Left: "┆", Right: "┆", TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯"}
	}
	return lipgloss.RoundedBorder()
}
