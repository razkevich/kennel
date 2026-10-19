package ghostty

import "testing"

func TestParse(t *testing.T) {
	th := Defaults()
	th.Parse(`
# comment
font-family = "JetBrains Mono"
font-size = 14
theme = light:Solarized,dark:Dracula
cursor-color = ffb86c
cursor-style = bar
palette = 2=50fa7b
window-padding-x = 8
adjust-cell-height = 10%
`, func(name string) (string, bool) {
		if name != "Dracula" {
			return "", false
		}
		return "background = 282a36\nforeground = f8f8f2\npalette = 1=#ff5555", true
	})
	if th.FontFamily != "JetBrains Mono" || th.FontSize != 14 || th.Background != "#282a36" || th.Foreground != "#f8f8f2" {
		t.Fatalf("%+v", th)
	}
	if th.Cursor != "#ffb86c" || th.CursorStyle != "bar" || th.Palette["1"] != "#ff5555" || th.Palette["2"] != "#50fa7b" {
		t.Fatalf("%+v", th)
	}
	if th.PaddingX != 8 || th.LineHeightPct < 1.09 || th.LineHeightPct > 1.11 {
		t.Fatalf("%+v", th)
	}
}
