package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/store"
	"github.com/razkevich/kennel/internal/things"
)

// Spaces are your projects, each with its own floors and kennels.
func (a *App) Spaces() []store.Space {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.spaces
}

// Space finds a project by key; ok is false for an unknown key.
func (a *App) Space(key string) (store.Space, bool) {
	for _, sp := range a.Spaces() {
		if sp.Key() == key {
			return sp, true
		}
	}
	return store.Space{}, false
}

// FirstSpace is where things go when no project is named.
func (a *App) FirstSpace() string {
	if sps := a.Spaces(); len(sps) > 0 {
		return sps[0].Key()
	}
	return ""
}

// InSpace says whether a session belongs in a project: all of them, unless the
// project is linked to a folder.
func (a *App) InSpace(p *pack.Pup, key string) bool {
	sp, _ := a.Space(key)
	return sp.Root == "" || ProjectRoot(p.Cwd) == sp.Root
}

func (a *App) ThingInSpace(t things.Thing, key string) bool {
	sp, _ := a.Space(key)
	return sp.Root == "" || strings.HasPrefix(t.Cwd, sp.Root)
}

// OpenSpace resolves a project key (falling back to the first project) and makes
// sure it has a floor to stand on.
func (a *App) OpenSpace(key string) (string, []store.Floor, error) {
	if _, ok := a.Space(key); !ok {
		key = a.FirstSpace()
	}
	if fs := a.FloorsIn(key); len(fs) > 0 {
		return key, fs, nil
	}
	f := store.Floor{Name: "Main", Icon: "🏢", Space: key}
	if err := a.Store.SaveFloor(&f); err != nil {
		return key, nil, err
	}
	if err := a.Reload(); err != nil {
		return key, nil, err
	}
	return key, a.FloorsIn(key), nil
}

// SaveSpace starts or edits a project.
func (a *App) SaveSpace(sp *store.Space) error {
	prev, existed := a.Space(sp.Key())
	if !existed {
		sp.ID = 0
	}
	if err := a.Store.SaveSpace(sp); err != nil {
		return err
	}
	if existed {
		a.remember("edit project "+prev.Name, func() error { return a.SaveSpace(&prev) })
	} else {
		id := sp.ID
		a.remember("start project "+sp.Name, func() error { return a.DeleteSpace(id) })
	}
	return a.Reload()
}

// DeleteSpace removes a project; its floors (and their kennels) move to another one.
func (a *App) DeleteSpace(id int64) error {
	gone, ok := a.Space(fmt.Sprintf("space:%d", id))
	if !ok {
		return fmt.Errorf("no project %d", id)
	}
	var to string
	for _, sp := range a.Spaces() {
		if sp.ID != id {
			to = sp.Key()
			break
		}
	}
	if to == "" {
		return errors.New("keep at least one project")
	}
	// Floors that move keep their kennels; one named like a floor already there gets
	// the project's name added ("Main (Infra)") so the two stay tellable apart.
	taken := map[string]bool{}
	for _, f := range a.FloorsIn(to) {
		taken[strings.ToLower(f.Name)] = true
	}
	moved := a.FloorsIn(gone.Key())
	for _, f := range moved {
		if taken[strings.ToLower(f.Name)] {
			if _, err := a.Store.DB.Exec(`UPDATE floors SET name=? WHERE id=?`, f.Name+" ("+gone.Name+")", f.ID); err != nil {
				return err
			}
		}
	}
	if err := a.Store.DeleteSpace(id, to); err != nil {
		return err
	}
	a.remember("remove project "+gone.Name, func() error {
		if err := a.Store.SaveSpace(&gone); err != nil {
			return err
		}
		for _, f := range moved {
			if _, err := a.Store.DB.Exec(`UPDATE floors SET space=?, name=? WHERE id=?`, gone.Key(), f.Name, f.ID); err != nil {
				return err
			}
		}
		return a.Reload()
	})
	return a.Reload()
}
