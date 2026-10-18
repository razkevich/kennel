package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/razkevich/kennel/internal/store"
)

// FloorPlan puts kennels (by id) on a floor (by name; an existing floor of the space is reused).
type FloorPlan struct {
	Floor   string  `json:"floor"`
	Kennels []int64 `json:"kennels"`
}

// SuggestFloors asks the theme model to group a space's kennels into a few floors.
// Only kennel names and a few session titles are sent.
func (a *App) SuggestFloors(ctx context.Context, space string) ([]FloorPlan, error) {
	var lines []string
	byName := map[string]int64{}
	for _, k := range a.Kennels() {
		if a.SpaceOf(k) != space {
			continue
		}
		byName[strings.ToLower(k.Name)] = k.ID
		var ex []string
		for _, p := range a.Pups() {
			if p.Kennel != nil && p.Kennel.ID == k.ID && len(ex) < 3 {
				ex = append(ex, clip(p.Title(), 40))
			}
		}
		lines = append(lines, "- "+k.Name+" (e.g. "+strings.Join(ex, "; ")+")")
	}
	if len(lines) < 2 {
		return nil, errors.New("build a few kennels first")
	}
	var existing []string
	for _, f := range a.FloorsIn(space) {
		existing = append(existing, f.Name)
	}
	prompt := "Group these kennels (folders of coding-assistant sessions) into 2-5 floors of a building, " +
		"so related work sits together (for example Work, Ops, Side projects, Personal).\n"
	if len(existing) > 0 {
		prompt += "Existing floors you may reuse: " + strings.Join(existing, ", ") + ".\n"
	}
	prompt += "Reply with one line per floor, formatted exactly `Floor name: kennel, kennel`, using the kennel names as given, nothing else.\n\n" +
		strings.Join(lines, "\n")
	llm, _ := a.ThemeLLM()
	reply, err := llm.Complete(ctx, "haiku", prompt, nil)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	var out []FloorPlan
	for _, line := range strings.Split(reply, "\n") {
		name, list, ok := strings.Cut(strings.Trim(strings.TrimSpace(line), "-*` "), ":")
		name = strings.Trim(strings.TrimSpace(name), "`*\"'")
		if !ok || name == "" || len(name) > 30 {
			continue
		}
		plan := FloorPlan{Floor: name}
		for _, k := range strings.Split(list, ",") {
			if id, ok := byName[strings.ToLower(strings.Trim(strings.TrimSpace(k), "`*\"'."))]; ok && !seen[id] {
				seen[id] = true
				plan.Kennels = append(plan.Kennels, id)
			}
		}
		if len(plan.Kennels) > 0 {
			out = append(out, plan)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("the model didn't propose any floors; try again")
	}
	return out, nil
}

// OrganizeFloors moves kennels onto floors by name, building missing floors in the
// space, as one undo step.
func (a *App) OrganizeFloors(space string, plan []FloorPlan) (int, error) {
	floorID := map[string]int64{}
	for _, f := range a.FloorsIn(space) {
		floorID[strings.ToLower(f.Name)] = f.ID
	}
	prev := map[int64]int64{}
	for _, k := range a.Kennels() {
		prev[k.ID] = a.FloorOf(k)
	}
	var built []int64
	moved := 0
	for _, p := range plan {
		name := strings.TrimSpace(p.Floor)
		if name == "" || len(p.Kennels) == 0 {
			continue
		}
		id, ok := floorID[strings.ToLower(name)]
		if !ok {
			f := store.Floor{Name: name, Icon: "🏢", Space: space}
			if err := a.Store.SaveFloor(&f); err != nil {
				return moved, err
			}
			id, floorID[strings.ToLower(name)] = f.ID, f.ID
			built = append(built, f.ID)
		}
		for _, kid := range p.Kennels {
			if _, known := prev[kid]; !known || prev[kid] == id {
				continue
			}
			if err := a.setKennelFloor(kid, id); err != nil {
				return moved, err
			}
			moved++
		}
	}
	a.remember(fmt.Sprintf("organize %d kennels into floors", moved), func() error {
		for kid, fid := range prev {
			if err := a.setKennelFloor(kid, fid); err != nil {
				return err
			}
		}
		for _, id := range built {
			if _, err := a.Store.DB.Exec(`DELETE FROM floors WHERE id=?`, id); err != nil {
				return err
			}
		}
		return a.Reload()
	})
	return moved, a.Reload()
}
