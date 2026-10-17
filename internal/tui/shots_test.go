package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/razkevich/kennel/internal/opener"
)

// TestShots writes ANSI frames of key screens for the README (KENNEL_SHOTS=dir).
func TestShots(t *testing.T) {
	dir := os.Getenv("KENNEL_SHOTS")
	if dir == "" {
		t.Skip("set KENNEL_SHOTS=dir to write screenshots")
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	h := newHarness(t, opener.Print)
	h.send(tea.WindowSizeMsg{Width: 160, Height: 46})
	save := func(name string) {
		m := h.model()
		m.frame = 5
		h.m = m
		if err := os.WriteFile(filepath.Join(dir, name+".ans"), []byte(h.m.View()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h.key("u")
	save("map")
	h.key("F")
	save("projects")
	h.key("F", "F", "2")
	h.key("/")
	h.typeText("pg-upgrade")
	h.key("enter")
	h.key("esc")
	save("pups")
	h.key("/")
	h.typeText("paymentelement")
	h.send(searchTickMsg{seq: h.model().searchSeq})
	h.key("enter")
	save("search")
	h.key("esc", "a")
	h.typeText("why did the payment form flicker")
	h.key("enter")
	save("ask")
	h.key("4")
	save("jobs")
	h.key("6")
	save("gear")
	h.key("2", "/")
	h.typeText("pg-upgrade")
	h.key("enter", "v")
	save("transcript")
}
