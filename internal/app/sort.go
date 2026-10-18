package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/store"
)

// Suggestion is where a session could be filed: an existing kennel (KennelID set)
// or a new one to build (Kennel name only).
type Suggestion struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Kennel   string `json:"kennel"`
	KennelID int64  `json:"kennelId"`
}

const sortLimit = 80

// SuggestKennels asks the theme model where each session belongs, preferring your
// own kennels in the space. Like Categorize, only titles, first asks and recaps are sent.
func (a *App) SuggestKennels(ctx context.Context, ids []string, space string) ([]Suggestion, error) {
	var todo []*pack.Pup
	for _, id := range ids {
		if p := a.Pup(id); p != nil && !p.Empty() {
			todo = append(todo, p)
		}
	}
	if len(todo) == 0 {
		return nil, errors.New("nothing to sort")
	}
	if len(todo) > sortLimit {
		todo = todo[:sortLimit]
	}
	own := map[string]store.Kennel{}
	var names []string
	for _, k := range a.Kennels() {
		if k.Smart != "" || a.SpaceOf(k) != space {
			continue
		}
		own[strings.ToLower(k.Name)] = k
		var examples []string
		for _, p := range a.Pups() {
			if p.Kennel != nil && p.Kennel.ID == k.ID && len(examples) < 3 {
				examples = append(examples, clip(p.Title(), 40))
			}
		}
		if len(examples) > 0 {
			names = append(names, fmt.Sprintf("%s (e.g. %s)", k.Name, strings.Join(examples, "; ")))
		} else {
			names = append(names, k.Name)
		}
	}
	llm, _ := a.ThemeLLM()
	var out []Suggestion
	for start := 0; start < len(todo); start += themeBatch {
		batch := todo[start:min(start+themeBatch, len(todo))]
		var lines []string
		byPrefix := map[string]*pack.Pup{}
		for _, p := range batch {
			lines = append(lines, themeLine(p))
			byPrefix[p.ID[:8]] = p
		}
		prompt := "Assign each coding-assistant session below to exactly one kennel (a folder of related work).\n"
		if len(names) > 0 {
			prompt += "Existing kennels: " + strings.Join(names, ", ") + ".\nUse an existing kennel whenever it fits; "
		}
		prompt += "otherwise propose a new short name (1-3 words).\n" +
			"Reply with one line per session, formatted exactly `id: Kennel`, nothing else.\n\n" + strings.Join(lines, "\n")
		reply, err := llm.Complete(ctx, "haiku", prompt, nil)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(reply, "\n") {
			id, name, ok := strings.Cut(strings.Trim(strings.TrimSpace(line), "-*` "), ":")
			p := byPrefix[strings.TrimSpace(id)]
			name = strings.Trim(strings.TrimSpace(name), "`*\"'.")
			if !ok || p == nil || name == "" || len(name) > 40 {
				continue
			}
			s := Suggestion{ID: p.ID, Title: p.Title(), Kennel: name}
			if k, ok := own[strings.ToLower(name)]; ok {
				s.Kennel, s.KennelID = k.Name, k.ID
			}
			out = append(out, s)
			delete(byPrefix, p.ID[:8])
		}
	}
	return out, nil
}

// FileAll files sessions into kennels, building the new ones on floorID, as one undo step.
func (a *App) FileAll(moves []Suggestion, floorID int64) (int, error) {
	type prev struct {
		id      string
		kennel  int64
		unfiled bool
	}
	var undo []prev
	var built []int64
	byName := map[string]int64{}
	for _, k := range a.Kennels() {
		if k.Smart == "" {
			byName[strings.ToLower(k.Name)] = k.ID
		}
	}
	n := 0
	for _, m := range moves {
		p := a.Pup(m.ID)
		if p == nil {
			continue
		}
		kid := m.KennelID
		if kid == 0 {
			name := strings.TrimSpace(m.Kennel)
			if name == "" {
				continue
			}
			if id, ok := byName[strings.ToLower(name)]; ok {
				kid = id
			} else {
				k := store.Kennel{Name: name, Icon: "🏠", Floor: floorID}
				if err := a.Store.SaveKennel(&k); err != nil {
					return n, err
				}
				kid = k.ID
				byName[strings.ToLower(name)] = kid
				built = append(built, kid)
			}
		}
		undo = append(undo, prev{p.ID, p.Meta.KennelID, p.Meta.Unfiled})
		if err := a.setKennel(p.ID, kid, false); err != nil {
			return n, err
		}
		n++
	}
	if n > 0 {
		a.remember(fmt.Sprintf("sort %d sessions", n), func() error {
			for _, u := range undo {
				if err := a.setKennel(u.id, u.kennel, u.unfiled); err != nil {
					return err
				}
			}
			for _, id := range built {
				if err := a.Store.DeleteKennel(id); err != nil {
					return err
				}
			}
			return a.Reload()
		})
	}
	return n, a.Reload()
}
