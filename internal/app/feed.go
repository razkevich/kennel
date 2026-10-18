package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/treats"
)

func (a *App) Treats() []treats.Treat {
	ts, _ := a.Store.Treats()
	return ts
}

func (a *App) AddTreat(name, value string) (treats.Treat, error) {
	if strings.TrimSpace(value) == "" {
		return treats.Treat{}, fmt.Errorf("a treat needs a value: a path, link, #channel, ticket or note")
	}
	t := treats.New(name, ExpandHome(strings.TrimSpace(value)))
	if err := a.Store.SaveTreat(&t); err != nil {
		return t, err
	}
	id := t.ID
	a.remember("add treat "+t.Name, func() error { return a.Store.DeleteTreat(id) })
	return t, nil
}

func (a *App) DeleteTreat(id int64) error {
	for _, t := range a.Treats() {
		if t.ID == id {
			t := t
			a.remember("remove treat "+t.Name, func() error { return a.Store.RestoreTreat(t) })
		}
	}
	return a.Store.DeleteTreat(id)
}

// FeedLive hands a message to a running pup through its herdr pane. ok is false
// when the pup is asleep and has to be woken up with FeedTarget instead.
func (a *App) FeedLive(p *pack.Pup, msg string) (ok bool, err error) {
	if !p.Status.Live() {
		return false, nil
	}
	if p.Pane == "" {
		return true, fmt.Errorf("%s is running outside herdr, so kennel can't hand it a treat — paste this instead: %s", p.Title(), msg)
	}
	return true, opener.Prompt(a.Cfg.Runner, p.Pane, msg)
}

// FeedTarget wakes an asleep pup with the message as its first prompt.
func (a *App) FeedTarget(p *pack.Pup, msg string, files []string) opener.Target {
	t := a.Target(p, false, false)
	if t.Agent == "cursor" {
		t.Files = files
		return t
	}
	t.Prompt = msg
	return t
}

// SaveDrop stores a file dropped onto the map so agents can read it from disk.
func (a *App) SaveDrop(name string, data []byte) (string, error) {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == "/" {
		name = "drop"
	}
	dir := filepath.Join(a.Cfg.Home, "drops", time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			break
		}
		ext := filepath.Ext(name)
		path = filepath.Join(dir, fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), i, ext))
	}
	return path, os.WriteFile(path, data, 0o600)
}
