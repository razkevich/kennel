package app

import (
	"errors"
	"fmt"

	"github.com/razkevich/kennel/internal/store"
)

func (a *App) Floors() []store.Floor {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.floors
}

// FloorOf is the floor a kennel stands on; kennels without one belong to the first floor.
func (a *App) FloorOf(k store.Kennel) int64 {
	fs := a.Floors()
	for _, f := range fs {
		if f.ID == k.Floor {
			return f.ID
		}
	}
	if len(fs) > 0 {
		return fs[0].ID
	}
	return 0
}

func (a *App) floor(id int64) (store.Floor, bool) {
	for _, f := range a.Floors() {
		if f.ID == id {
			return f, true
		}
	}
	return store.Floor{}, false
}

// SaveFloor creates or renames a floor.
func (a *App) SaveFloor(f *store.Floor) error {
	prev, existed := a.floor(f.ID)
	if err := a.Store.SaveFloor(f); err != nil {
		return err
	}
	id := f.ID
	if existed {
		a.remember("rename floor "+prev.Name, func() error { return a.SaveFloor(&prev) })
	} else {
		a.remember("build floor "+f.Name, func() error { return a.DeleteFloor(id) })
	}
	return a.Reload()
}

// DeleteFloor removes a floor; its kennels move to the first other floor.
func (a *App) DeleteFloor(id int64) error {
	gone, ok := a.floor(id)
	if !ok {
		return fmt.Errorf("no floor %d", id)
	}
	var to int64
	for _, f := range a.Floors() {
		if f.ID != id {
			to = f.ID
			break
		}
	}
	if to == 0 {
		return errors.New("the last floor can't be removed")
	}
	var moved []int64
	for _, k := range a.Kennels() {
		if a.FloorOf(k) == id {
			moved = append(moved, k.ID)
		}
	}
	if err := a.Store.DeleteFloor(id, to); err != nil {
		return err
	}
	a.remember("remove floor "+gone.Name, func() error {
		if err := a.Store.SaveFloor(&gone); err != nil {
			return err
		}
		for _, kid := range moved {
			if err := a.setKennelFloor(kid, id); err != nil {
				return err
			}
		}
		return a.Reload()
	})
	return a.Reload()
}

// MoveKennelToFloor carries a kennel's room, pups and shelf to another floor.
func (a *App) MoveKennelToFloor(kennelID, floorID int64) error {
	if _, ok := a.floor(floorID); !ok {
		return fmt.Errorf("no floor %d", floorID)
	}
	for _, k := range a.Kennels() {
		if k.ID != kennelID {
			continue
		}
		prev := a.FloorOf(k)
		if prev == floorID {
			return nil
		}
		if err := a.setKennelFloor(kennelID, floorID); err != nil {
			return err
		}
		a.remember("move "+k.Name+" to another floor", func() error {
			if err := a.setKennelFloor(kennelID, prev); err != nil {
				return err
			}
			return a.Reload()
		})
		return a.Reload()
	}
	return fmt.Errorf("no kennel %d", kennelID)
}

func (a *App) setKennelFloor(kennelID, floorID int64) error {
	_, err := a.Store.DB.Exec(`UPDATE kennels SET floor_id=? WHERE id=?`, floorID, kennelID)
	return err
}
