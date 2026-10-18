// Package floor lays out kennel's map: which rooms exist on each floor and who stands where.
package floor

import (
	"fmt"
	"strings"
	"time"

	"github.com/razkevich/kennel/internal/app"
	"github.com/razkevich/kennel/internal/blocks"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/store"
	"github.com/razkevich/kennel/internal/things"
)

var Names = []string{"🏡 Campus", "📁 Projects", "🏠 Kennels"}

// KennelsFloor is the curated floor: only rooms you built, holding only pups you chose.
const KennelsFloor = 2

const (
	peach    = "#fab387"
	lavender = "#b4befe"
)

type Room struct {
	Key     string
	Style   store.Style
	MinCols int
	MinRows int
	Title   string
	Icon    string
	Color   string
	Look    pack.Look
	Blurb   string
	Pups    []*pack.Pup
	Things  []things.Thing
}

// styled applies the user's building blocks (floor, walls, shape) over a room's defaults.
func styled(a *app.App, rooms []Room) []Room {
	styles := a.Styles()
	for i := range rooms {
		st, ok := styles[rooms[i].Key]
		if !ok {
			continue
		}
		rooms[i].Style = st
		if g, ok := blocks.FloorGlyph(st.Floor); ok {
			rooms[i].Look.Floor = g
		}
		if b, ok := blocks.WallBorder(st.Wall); ok {
			rooms[i].Look.Border = b
		}
		if sh := blocks.ShapeByKey(st.Shape); sh != nil {
			rooms[i].MinCols, rooms[i].MinRows = sh.Cols, sh.Rows
			rooms[i].Look.Wide = sh.Cols > 4
		}
	}
	return rooms
}

// campusOrder decides which room a pup stands in when several smart kennels match.
var campusOrder = []string{"yard", "best", "fire", "helping", "court", "yak", "water", "marathon", "shipped", "rug", "dumpster"}

const recentWindow = 21 * 24 * time.Hour

func workshop() Room {
	return Room{Key: "workshop", Title: "Workshop", Icon: "🔧", Color: peach, Look: pack.Look{Border: "thick", Floor: "▪", Decor: "🧰"},
		Blurb: "monitors, servers and long jobs — N to add one"}
}

// Rooms lays out one floor of the map. Both the TUI and the web map draw these.
func Rooms(a *app.App, floor int) []Room {
	return styled(a, rooms(a, floor))
}

func rooms(a *app.App, floor int) []Room {
	now := a.Now()
	pups := a.Pups()
	allThings := a.Things()
	var rooms []Room
	switch floor {
	case 0:
		placed := map[string]bool{}
		for _, key := range campusOrder {
			sm := pack.SmartByKey(key)
			r := Room{Key: key, Title: sm.Name, Icon: sm.Icon, Color: sm.Color, Look: sm.Look, Blurb: sm.Blurb}
			for _, p := range pups {
				if placed[p.ID] || !sm.Match(p, now) {
					continue
				}
				if key != "rug" && key != "dumpster" && !p.Status.Live() && now.Sub(p.Updated) > recentWindow {
					continue
				}
				placed[p.ID] = true
				r.Pups = append(r.Pups, p)
			}
			rooms = append(rooms, r)
		}
		lobby := Room{Key: "lobby", Title: "Lobby", Icon: "🪑", Color: lavender, Look: pack.Look{Border: "rounded", Floor: " "}, Blurb: "everyone else this month"}
		for _, p := range pups {
			if !placed[p.ID] && !p.Meta.Archived && !p.Empty() && now.Sub(p.Updated) < recentWindow {
				lobby.Pups = append(lobby.Pups, p)
			}
		}
		ws := workshop()
		ws.Things = allThings
		rooms = append(rooms[:1], append([]Room{lobby, ws}, rooms[1:]...)...)
	case 1:
		floorsChars := []string{"·", "⠂", "∴", "⁘", "~", " "}
		borders := []string{"rounded", "double", "thick", "normal"}
		palette := []string{"#89dceb", peach, "#cba6f7", "#a6e3a1", "#f9e2af", "#f5c2e7", "#94e2d5", lavender, "#f38ba8", "#89b4fa"}
		homed := map[string]bool{}
		for i, pr := range a.Projects() {
			r := Room{Key: "project:" + pr.Root, Title: pr.Name, Icon: "📁", Color: palette[i%len(palette)],
				Look: pack.Look{Border: borders[i%len(borders)], Floor: floorsChars[i%len(floorsChars)], Wide: pr.Count > 12}, Blurb: pr.Root}
			for _, p := range pups {
				if app.ProjectRoot(p.Cwd) == pr.Root && !p.Meta.Archived && !p.Empty() && (p.Status.Live() || now.Sub(p.Updated) < recentWindow) {
					r.Pups = append(r.Pups, p)
				}
			}
			for _, t := range allThings {
				if t.Cwd != "" && strings.HasPrefix(t.Cwd, pr.Root) {
					r.Things = append(r.Things, t)
					homed[t.Key()] = true
				}
			}
			rooms = append(rooms, r)
		}
		if ws := homeless(allThings, homed); len(ws.Things) > 0 {
			rooms = append(rooms, ws)
		}
	case 2:
		homed := map[string]bool{}
		for _, k := range a.Kennels() {
			icon := k.Icon
			if icon == "" {
				icon = "🏠"
			}
			color := k.Color
			if color == "" {
				color = peach
			}
			r := Room{Key: fmt.Sprintf("kennel:%d", k.ID), Title: k.Name, Icon: icon, Color: color, Look: pack.Look{Border: "rounded", Floor: "·"}}
			for _, p := range pups {
				if p.Kennel != nil && p.Kennel.ID == k.ID && !p.Meta.Archived {
					r.Pups = append(r.Pups, p)
				}
			}
			for _, t := range allThings {
				if t.KennelID == k.ID {
					r.Things = append(r.Things, t)
					homed[t.Key()] = true
				}
			}
			rooms = append(rooms, r)
		}
		if len(a.Kennels()) == 0 {
			rooms = append(rooms, Room{Key: "new-kennel", Title: "Your first kennel", Icon: "＋", Color: peach,
				Look: pack.Look{Border: "hidden", Floor: " ", Decor: "🔨"}, Blurb: "No kennels yet — press K to build one"})
		}
		// The kennels floor shows only what you've chosen: no strays, no unassigned things.
	}
	return rooms
}

func homeless(all []things.Thing, homed map[string]bool) Room {
	ws := workshop()
	for _, t := range all {
		if !homed[t.Key()] {
			ws.Things = append(ws.Things, t)
		}
	}
	return ws
}
