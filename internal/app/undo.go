package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const maxUndo = 50

type undoEntry struct {
	label string
	run   func() error
}

// remember records how to reverse a user action. Changes made while undoing
// (and by automation such as chutes) are never recorded.
func (a *App) remember(label string, run func() error) {
	a.undoMu.Lock()
	defer a.undoMu.Unlock()
	if a.undoing {
		return
	}
	a.undo = append(a.undo, undoEntry{label, run})
	if len(a.undo) > maxUndo {
		a.undo = a.undo[len(a.undo)-maxUndo:]
	}
}

// UndoLabel describes what Undo would reverse, or "" when there's nothing.
func (a *App) UndoLabel() string {
	a.undoMu.Lock()
	defer a.undoMu.Unlock()
	if len(a.undo) == 0 {
		return ""
	}
	return a.undo[len(a.undo)-1].label
}

// Undo reverses the most recent user action and returns its label.
func (a *App) Undo() (string, error) {
	a.undoMu.Lock()
	if len(a.undo) == 0 {
		a.undoMu.Unlock()
		return "", fmt.Errorf("nothing to undo")
	}
	e := a.undo[len(a.undo)-1]
	a.undo = a.undo[:len(a.undo)-1]
	a.undoing = true
	a.undoMu.Unlock()
	defer func() {
		a.undoMu.Lock()
		a.undoing = false
		a.undoMu.Unlock()
	}()
	if err := e.run(); err != nil {
		return e.label, err
	}
	return e.label, a.Reload()
}

// untrash moves a transcript back from kennel's trash.
func (a *App) untrash(trashed, original string) error {
	if _, err := os.Stat(original); err == nil {
		return fmt.Errorf("%s exists again; leaving the trashed copy at %s", original, trashed)
	}
	if err := os.MkdirAll(filepath.Dir(original), 0o700); err != nil {
		return err
	}
	if err := os.Rename(trashed, original); err != nil {
		return err
	}
	_, err := a.Sync(context.Background(), nil)
	return err
}
