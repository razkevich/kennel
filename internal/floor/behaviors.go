package floor

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/razkevich/kennel/internal/app"
	"github.com/razkevich/kennel/internal/blocks"
	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/store"
)

// RoomsOf lists every room (on any floor) a pup currently stands in.
func RoomsOf(a *app.App, pupID string) []Room {
	var out []Room
	for f := range Names {
		for _, r := range Rooms(a, f) {
			for _, p := range r.Pups {
				if p.ID == pupID {
					out = append(out, r)
				}
			}
		}
	}
	return out
}

func blocksFor(a *app.App, pupID, kind string) []store.Block {
	var out []store.Block
	for _, r := range RoomsOf(a, pupID) {
		out = append(out, r.Style.OfKind(kind)...)
	}
	return out
}

// Ring sends a desktop notification for transitions in rooms that have a bell.
// It returns how many notifications went out.
func Ring(a *app.App, ts []app.Transition) int {
	n := 0
	for _, t := range ts {
		for _, b := range blocksFor(a, t.Pup.ID, blocks.Bell) {
			when := b.Get("when")
			if when == "" {
				when = "waiting,done"
			}
			state := "done"
			if t.To == claude.Waiting {
				state = "waiting"
			}
			if !slices.Contains(strings.Split(when, ","), state) {
				continue
			}
			msg := t.Pup.Title() + " finished"
			if state == "waiting" {
				msg = t.Pup.Title() + " needs you"
			}
			if opener.Notify(a.Cfg.Runner, "🔔 kennel", msg) == nil {
				n++
			}
			break
		}
	}
	return n
}

// Sweep archives pups that have slept past a chute's limit. Only kennel's own
// archive flag changes; transcripts are untouched. It returns how many moved.
func Sweep(a *app.App) int {
	now := a.Now()
	n := 0
	for f := range Names {
		for _, r := range Rooms(a, f) {
			for _, c := range r.Style.OfKind(blocks.Chute) {
				limit := time.Duration(c.Int("days", 14)) * 24 * time.Hour
				for _, p := range r.Pups {
					if p.Meta.Archived || p.Meta.Starred || p.Status.Live() || now.Sub(p.Updated) < limit {
						continue
					}
					if a.SetArchived(p.ID, true) == nil {
						n++
					}
				}
			}
		}
	}
	return n
}

// TargetFor is app.Target plus portal routing into a herdr workspace.
func TargetFor(a *app.App, p *pack.Pup, fork, remote bool) opener.Target {
	t := a.Target(p, fork, remote)
	if ps := blocksFor(a, p.ID, blocks.Portal); len(ps) > 0 {
		t.Workspace = strings.TrimSpace(ps[0].Get("workspace"))
	}
	return t
}

// Spawn builds a new-session target from a room's spawn door.
func Spawn(a *app.App, room Room, door store.Block) (opener.Target, error) {
	cwd := app.ExpandHome(strings.TrimSpace(door.Get("cwd")))
	if cwd == "" {
		cwd = roomCwd(a, room)
	}
	if cwd == "" {
		return opener.Target{}, fmt.Errorf("the door in %s needs a folder", room.Title)
	}
	t := opener.Target{Cwd: cwd, Title: strings.TrimSpace(door.Get("name")), New: true,
		Prompt: strings.TrimSpace(door.Get("prompt")), Model: strings.TrimSpace(door.Get("model"))}
	for _, p := range room.Style.OfKind(blocks.Portal) {
		t.Workspace = strings.TrimSpace(p.Get("workspace"))
	}
	return t, nil
}

func roomCwd(a *app.App, room Room) string {
	if root, ok := strings.CutPrefix(room.Key, "project:"); ok {
		return root
	}
	if strings.HasPrefix(room.Key, "kennel:") {
		for _, k := range a.Kennels() {
			if fmt.Sprintf("kennel:%d", k.ID) == room.Key && k.Cwd != "" {
				return k.Cwd
			}
		}
	}
	for _, p := range room.Pups {
		return app.ProjectRoot(p.Cwd)
	}
	return ""
}

// FindRoom looks a room up by key on any floor.
func FindRoom(a *app.App, key string) (Room, bool) {
	for f := range Names {
		for _, r := range Rooms(a, f) {
			if r.Key == key {
				return r, true
			}
		}
	}
	return Room{}, false
}

// ApplyTemplate restyles a room from a template, keeping its position.
func ApplyTemplate(a *app.App, key string, tp blocks.Template) error {
	st := a.Style(key)
	st.Shape, st.Floor, st.Wall = tp.Shape, tp.Floor, tp.Wall
	st.Decor = nil
	for i, d := range tp.Decor {
		st.Decor = append(st.Decor, store.Placed{Kind: d, X: 0.15 + 0.25*float64(i%4), Y: 0.85})
	}
	st.Blocks = nil
	kinds := make([]string, 0, len(tp.Blocks))
	for k := range tp.Blocks {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	for i, kind := range kinds {
		cfg := map[string]string{}
		for k, v := range tp.Blocks[kind] {
			cfg[k] = v
		}
		st.Blocks = append(st.Blocks, store.Block{Kind: kind, X: 0.9, Y: 0.25 + 0.25*float64(i), Config: cfg})
	}
	return a.SaveStyle(key, st)
}
