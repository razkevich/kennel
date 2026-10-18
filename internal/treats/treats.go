// Package treats turns sources — files, links, Slack channels, tickets, notes —
// into a prompt that feeds them to an agent session.
package treats

import (
	"fmt"
	"regexp"
	"strings"
)

type Kind string

const (
	File   Kind = "file"
	URL    Kind = "url"
	Slack  Kind = "slack"
	Ticket Kind = "ticket"
	Text   Kind = "text"
)

var Icon = map[Kind]string{File: "📄", URL: "🔗", Slack: "💬", Ticket: "🎫", Text: "📝"}

type Treat struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Kind  Kind   `json:"kind"`
	Value string `json:"value"`
}

var ticketRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}-\d+$`)

// Infer guesses a treat's kind from what was typed or dropped.
func Infer(value string) Kind {
	v := strings.TrimSpace(value)
	switch {
	case strings.HasPrefix(v, "http://"), strings.HasPrefix(v, "https://"):
		if strings.Contains(v, ".slack.com/archives/") {
			return Slack
		}
		return URL
	case strings.HasPrefix(v, "#") && !strings.Contains(v, " "):
		return Slack
	case strings.HasPrefix(v, "/"), strings.HasPrefix(v, "~/"), strings.HasPrefix(v, "./"):
		return File
	case ticketRe.MatchString(v):
		return Ticket
	}
	return Text
}

// New builds a treat, naming it from the value when no name is given.
func New(name, value string) Treat {
	value = strings.TrimSpace(value)
	t := Treat{Name: strings.TrimSpace(name), Kind: Infer(value), Value: value}
	if t.Name == "" {
		t.Name = value
		if t.Kind == File {
			if i := strings.LastIndex(value, "/"); i >= 0 && i < len(value)-1 {
				t.Name = value[i+1:]
			}
		}
		if len([]rune(t.Name)) > 40 {
			t.Name = string([]rune(t.Name)[:39]) + "…"
		}
	}
	return t
}

// Prompt is what the pup is told when it gets this treat.
func (t Treat) Prompt(note string) string {
	var msg string
	switch t.Kind {
	case File:
		msg = fmt.Sprintf("Please read %s and keep it in mind for what we're doing here.", t.Value)
	case URL:
		msg = fmt.Sprintf("Please take a look at %s and use it as context.", t.Value)
	case Slack:
		msg = fmt.Sprintf("Please read the Slack channel %s (recent messages and threads) with your Slack tools and pull out what's relevant here.", t.Value)
	case Ticket:
		msg = fmt.Sprintf("Please pull up ticket %s (with your Linear/Jira tools) and use it as context.", t.Value)
	default:
		msg = t.Value
	}
	if note = strings.TrimSpace(note); note != "" {
		msg += " " + note
	}
	return msg
}
