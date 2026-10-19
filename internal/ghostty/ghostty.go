// Package ghostty reads the user's Ghostty config so kennel's terminals look and
// feel like Ghostty: font, colors, palette, cursor and padding.
package ghostty

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Theme is the part of a Ghostty config a terminal needs.
type Theme struct {
	FontFamily    string            `json:"fontFamily"`
	FontSize      float64           `json:"fontSize"`
	Background    string            `json:"background"`
	Foreground    string            `json:"foreground"`
	Cursor        string            `json:"cursor"`
	CursorStyle   string            `json:"cursorStyle"`
	CursorBlink   bool              `json:"cursorBlink"`
	Selection     string            `json:"selection"`
	Palette       map[string]string `json:"palette"`
	PaddingX      int               `json:"paddingX"`
	PaddingY      int               `json:"paddingY"`
	OptionAsAlt   bool              `json:"optionAsAlt"`
	LineHeightPct float64           `json:"lineHeight"`
}

// Defaults are Ghostty's own when a config says nothing.
func Defaults() Theme {
	return Theme{FontSize: 13, Background: "#282c34", Foreground: "#ffffff", CursorStyle: "block", CursorBlink: true,
		Palette: map[string]string{}, PaddingX: 2, PaddingY: 2, OptionAsAlt: true, LineHeightPct: 1}
}

// Parse applies a config's text; a `theme` line loads that theme first via load.
func (t *Theme) Parse(text string, load func(name string) (string, bool)) {
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), `"`)
		switch key {
		case "font-family":
			if t.FontFamily == "" && value != "" {
				t.FontFamily = value
			}
		case "font-size":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				t.FontSize = v
			}
		case "background":
			t.Background = hex(value)
		case "foreground":
			t.Foreground = hex(value)
		case "cursor-color":
			t.Cursor = hex(value)
		case "cursor-style":
			t.CursorStyle = value
		case "cursor-style-blink":
			t.CursorBlink = value != "false"
		case "selection-background":
			t.Selection = hex(value)
		case "palette":
			if n, c, ok := strings.Cut(value, "="); ok {
				t.Palette[strings.TrimSpace(n)] = hex(strings.TrimSpace(c))
			}
		case "window-padding-x":
			if v, err := strconv.Atoi(strings.Fields(value + " 0")[0]); err == nil {
				t.PaddingX = v
			}
		case "window-padding-y":
			if v, err := strconv.Atoi(strings.Fields(value + " 0")[0]); err == nil {
				t.PaddingY = v
			}
		case "macos-option-as-alt":
			t.OptionAsAlt = value != "false"
		case "adjust-cell-height":
			if v, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64); err == nil && strings.HasSuffix(value, "%") {
				t.LineHeightPct = 1 + v/100
			}
		case "theme":
			if body, ok := load(darkVariant(value)); ok {
				t.Parse(body, func(string) (string, bool) { return "", false })
			}
		}
	}
}

// darkVariant picks the dark one of `theme = light:A,dark:B`.
func darkVariant(v string) string {
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); strings.HasPrefix(p, "dark:") {
			return strings.TrimPrefix(p, "dark:")
		}
	}
	first := strings.TrimSpace(strings.Split(v, ",")[0])
	return strings.TrimPrefix(first, "light:")
}

func hex(v string) string {
	if strings.HasPrefix(v, "#") {
		return v
	}
	return "#" + v
}

// Load reads the user's Ghostty config (and its theme from the installed app).
func Load(home string) Theme {
	t := Defaults()
	themeDirs := []string{filepath.Join(home, ".config/ghostty/themes"), "/Applications/Ghostty.app/Contents/Resources/ghostty/themes"}
	load := func(name string) (string, bool) {
		for _, d := range themeDirs {
			if b, err := os.ReadFile(filepath.Join(d, name)); err == nil {
				return string(b), true
			}
		}
		return "", false
	}
	for _, p := range []string{filepath.Join(home, ".config/ghostty/config"), filepath.Join(home, "Library/Application Support/com.mitchellh.ghostty/config")} {
		if b, err := os.ReadFile(p); err == nil {
			t.Parse(string(b), load)
		}
	}
	return t
}
