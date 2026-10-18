// Package floor lays out kennel's map: which rooms stand on each floor and who is in them.
package floor

import (
	"fmt"
	"time"

	"github.com/razkevich/kennel/internal/app"
	"github.com/razkevich/kennel/internal/blocks"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/store"
	"github.com/razkevich/kennel/internal/things"
)

const peach = "#fab387"

type Room struct {
	Key      string
	KennelID int64
	// Smart is the self-filling kind of a smart kennel; empty for your own kennels.
	Smart   string
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
	// Older counts pups filed here that haven't been touched within Recent.
	Older int
}

// SmartKind is a kind of room that fills itself; you place one on a floor from Build.
type SmartKind struct {
	Key   string
	Name  string
	Icon  string
	Color string
	Blurb string
	Look  pack.Look
}

const (
	Lobby    = "lobby"
	Workshop = "workshop"
)

// SmartKinds lists what Build offers: the pack's smart kennels plus the lobby and workshop.
func SmartKinds() []SmartKind {
	var out []SmartKind
	for _, sm := range pack.Smarts {
		if sm.Key == "all" {
			continue
		}
		out = append(out, SmartKind{sm.Key, sm.Name, sm.Icon, sm.Color, sm.Blurb, sm.Look})
	}
	return append(out,
		SmartKind{Lobby, "Lobby", "🪑", "#b4befe", "everyone else from this month", pack.Look{Border: "rounded", Floor: " "}},
		SmartKind{Workshop, "Workshop", "🔧", peach, "monitors, servers and jobs not in a kennel", pack.Look{Border: "thick", Floor: "▪", Decor: "🧰"}})
}

func SmartKindByKey(key string) (SmartKind, bool) {
	for _, k := range SmartKinds() {
		if k.Key == key {
			return k, true
		}
	}
	return SmartKind{}, false
}

// Recent is how far back the map and the session tray reach: kennels show what you
// worked on in this window (plus anything live) and count the rest as older.
const Recent = 14 * 24 * time.Hour

var keepsOld = map[string]bool{"rug": true, "dumpster": true, "best": true, "shipped": true}

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

// Rooms lays out one floor's kennels. A pup stands in at most one room per floor:
// the kennel it's filed in, else the first smart kennel that wants it. Unfiled pups
// wait in the page's session tray until you drag them somewhere.
func Rooms(a *app.App, floorID int64) []Room {
	now := a.Now()
	pups := a.Pups()
	allThings := a.Things()
	placed := map[string]bool{}

	var rooms []Room
	var smart []int
	homed := map[string]bool{}
	for _, t := range allThings {
		if t.KennelID != 0 {
			homed[t.Key()] = true
		}
	}
	for _, k := range a.Kennels() {
		if a.FloorOf(k) != floorID {
			continue
		}
		r := Room{Key: fmt.Sprintf("kennel:%d", k.ID), KennelID: k.ID, Smart: k.Smart, Title: k.Name, Icon: k.Icon, Color: k.Color,
			Look: pack.Look{Border: "rounded", Floor: "·"}}
		if kind, ok := SmartKindByKey(k.Smart); ok {
			r.Look, r.Blurb = kind.Look, kind.Blurb
			if r.Color == "" {
				r.Color = kind.Color
			}
			if r.Icon == "" {
				r.Icon = kind.Icon
			}
			smart = append(smart, len(rooms))
		} else {
			for _, p := range pups {
				if p.Kennel == nil || p.Kennel.ID != k.ID || p.Meta.Archived {
					continue
				}
				if !p.Status.Live() && now.Sub(p.Updated) > Recent {
					r.Older++
					continue
				}
				r.Pups = append(r.Pups, p)
				placed[p.ID] = true
			}
			for _, t := range allThings {
				if t.KennelID == k.ID {
					r.Things = append(r.Things, t)
				}
			}
		}
		if r.Icon == "" {
			r.Icon = "🏠"
		}
		if r.Color == "" {
			r.Color = peach
		}
		rooms = append(rooms, r)
	}

	// Smart kennels fill in order; the lobby takes whatever is left, so it goes last.
	var order, lobbies []int
	for _, i := range smart {
		if rooms[i].Smart == Lobby {
			lobbies = append(lobbies, i)
		} else {
			order = append(order, i)
		}
	}
	for _, i := range append(order, lobbies...) {
		r := &rooms[i]
		switch r.Smart {
		case Workshop:
			for _, t := range allThings {
				if !homed[t.Key()] {
					r.Things = append(r.Things, t)
				}
			}
		case Lobby:
			for _, p := range pups {
				if !placed[p.ID] && !p.Meta.Archived && !p.Empty() && now.Sub(p.Updated) < Recent {
					r.Pups = append(r.Pups, p)
					placed[p.ID] = true
				}
			}
		default:
			sm := pack.SmartByKey(r.Smart)
			if sm == nil {
				continue
			}
			for _, p := range pups {
				if placed[p.ID] || !sm.Match(p, now) {
					continue
				}
				if !keepsOld[r.Smart] && !p.Status.Live() && now.Sub(p.Updated) > Recent {
					continue
				}
				r.Pups = append(r.Pups, p)
				placed[p.ID] = true
			}
		}
	}
	return styled(a, rooms)
}
