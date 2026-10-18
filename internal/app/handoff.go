package app

import (
	"fmt"
	"strings"

	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/store"
)

// HandoffText is what one session tells another: your note, then the sender's
// latest reply (trimmed) so the receiver has the context.
func (a *App) HandoffText(from *pack.Pup, note string) string {
	var last string
	if chunks, err := a.Store.Chunks(from.ID); err == nil {
		for i := len(chunks) - 1; i >= 0; i-- {
			if chunks[i].Role == claude.RoleAssistant && strings.TrimSpace(chunks[i].Text) != "" {
				last = strings.TrimSpace(chunks[i].Text)
				break
			}
		}
	}
	if last == "" {
		last = from.Snippet()
	}
	last = clip(last, 1500)
	note = strings.TrimSpace(note)
	if note == "" {
		note = "Here's what another session found; take it into account."
	}
	return fmt.Sprintf("%s\n\nFrom the session \"%s\":\n%s", note, from.Title(), last)
}

func (a *App) RecordHandoff(from, to string) error {
	return a.Store.SaveHandoff(store.Handoff{From: from, To: to, At: a.Now()})
}
