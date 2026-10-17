// Package pack joins indexed sessions, user metadata and live status into pups,
// and defines the built-in smart kennels.
package pack

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/store"
)

type Pup struct {
	*claude.Session
	Meta   store.PupMeta
	Live   claude.Live
	Status claude.Status
	Pane   string
	Kennel *store.Kennel
}

func (p *Pup) Title() string {
	if p.Meta.Alias != "" {
		return p.Meta.Alias
	}
	return p.DisplayTitle()
}

// Repo is the project folder name, with worktrees folded into their parent repo.
func (p *Pup) Repo() string {
	cwd := p.Cwd
	if i := strings.Index(cwd, "/.claude/worktrees/"); i >= 0 {
		cwd = cwd[:i]
	}
	if cwd == "" {
		return p.Project
	}
	return filepath.Base(cwd)
}

func (p *Pup) Branch() string {
	if len(p.Branches) == 0 {
		return ""
	}
	return p.Branches[len(p.Branches)-1]
}

// Unread means the session finished or asked for input after the user last looked.
func (p *Pup) Unread() bool {
	if p.Status == claude.Waiting {
		return true
	}
	return p.Status == claude.Done && p.Live.Since.After(p.Meta.SeenAt)
}

func (p *Pup) Empty() bool { return p.NUser == 0 }

// Snippet is the single most useful line about where the session stands.
func (p *Pup) Snippet() string {
	if p.Recap != "" {
		return p.Recap
	}
	if p.LastPrompt != "" {
		return "› " + p.LastPrompt
	}
	return p.FirstPrompt
}

// Build joins everything into pups, newest first.
func Build(sessions []*claude.Session, meta map[string]store.PupMeta, kennels []store.Kennel, live map[string]claude.Live) []*Pup {
	byID := map[int64]*store.Kennel{}
	for i := range kennels {
		byID[kennels[i].ID] = &kennels[i]
	}
	pups := make([]*Pup, 0, len(sessions))
	for _, s := range sessions {
		p := &Pup{Session: s, Meta: meta[s.ID]}
		if l, ok := live[s.ID]; ok {
			p.Live = l
			p.Status = l.Status
		}
		if k := byID[p.Meta.KennelID]; k != nil {
			p.Kennel = k
		} else {
			for i := range kennels {
				if kennels[i].Rules.Matches(s.Cwd, p.Title(), strings.Join(s.Branches, " ")) {
					p.Kennel = &kennels[i]
					break
				}
			}
		}
		pups = append(pups, p)
	}
	sort.SliceStable(pups, func(i, j int) bool { return pups[i].Updated.After(pups[j].Updated) })
	return pups
}

// Look is how a smart kennel's room is drawn in the Pack view.
type Look struct {
	Border string
	Floor  string
	Decor  string
	Wide   bool
}

type Smart struct {
	Key   string
	Name  string
	Icon  string
	Color string
	Blurb string
	Look  Look
	Match func(p *Pup, now time.Time) bool
}

var (
	slackPaste = regexp.MustCompile(`(?i)(\[\d{1,2}:\d{2}\s?[AP]M\]|\b\d{1,2}:\d{2}\s?[AP]M\b|an ask from|asked me|#[a-z0-9][a-z0-9_-]+ channel|slack thread|from [A-Z][a-z]+ to me|@[a-z]+\.[a-z]+|\bhey team\b|\bhi team\b)`)
	reviewRe   = regexp.MustCompile(`(?i)^\s*(please\s+)?(code\s+)?revie+w+\b|/pull/\d+`)
	yakRe      = regexp.MustCompile(`(?i)\b(install|set ?up|configure|config|dotfiles|zsh|nvim|neovim|vim|tmux|ghostty|herdr|iterm|sublime|vscode|plugin|theme|keybind|statusline|brew)\b`)
	fireRe     = regexp.MustCompile(`(?i)\b(incident|outage|p99|p95|pager|on-?call|sev[0-9]|alert(s|ing)?|crash(es|ing)?|down|degraded|hotfix|rollback|postmortem|5\d\d errors?)\b`)
	questionRe = regexp.MustCompile(`(?i)^\s*(what|why|how|is|are|can|does|do|where|which|whats|what's|explain|wh?at)\b`)
)

// Smarts are the built-in kennels; a pup can live in several.
var Smarts = []Smart{
	{Key: "all", Name: "All Pups", Icon: "🐾", Color: "#a6adc8", Blurb: "every session",
		Look: Look{Border: "rounded", Floor: " "}, Match: func(p *Pup, _ time.Time) bool { return !p.Meta.Archived && !p.Empty() }},
	{Key: "yard", Name: "The Yard", Icon: "🌳", Color: "#a6e3a1", Blurb: "running right now",
		Look: Look{Border: "rounded", Floor: "⠂", Decor: "🌳", Wide: true}, Match: func(p *Pup, _ time.Time) bool { return p.Status.Live() }},
	{Key: "best", Name: "Best in Show", Icon: "🏆", Color: "#f9e2af", Blurb: "starred",
		Look: Look{Border: "double", Floor: " ", Decor: "🏆"}, Match: func(p *Pup, _ time.Time) bool { return p.Meta.Starred }},
	{Key: "today", Name: "Today", Icon: "☀️", Color: "#fab387", Blurb: "touched in the last 24h",
		Look: Look{Border: "rounded", Floor: " "}, Match: func(p *Pup, now time.Time) bool {
			return !p.Meta.Archived && !p.Empty() && now.Sub(p.Updated) < 24*time.Hour
		}},
	{Key: "helping", Name: "Helping Out", Icon: "🤝", Color: "#89dceb", Blurb: "started from a colleague's ask",
		Look: Look{Border: "rounded", Floor: "·", Decor: "☕"}, Match: func(p *Pup, _ time.Time) bool {
			return !p.Meta.Archived && slackPaste.MatchString(p.FirstPrompt)
		}},
	{Key: "court", Name: "Courtroom", Icon: "🔍", Color: "#cba6f7", Blurb: "PR reviews",
		Look: Look{Border: "thick", Floor: " ", Decor: "⚖️"}, Match: func(p *Pup, _ time.Time) bool {
			return !p.Meta.Archived && reviewRe.MatchString(p.FirstPrompt) && strings.Contains(strings.ToLower(p.FirstPrompt), "review")
		}},
	{Key: "fire", Name: "Firehouse", Icon: "🔥", Color: "#f38ba8", Blurb: "incidents, prod, things on fire",
		Look: Look{Border: "thick", Floor: "░", Decor: "🚒"}, Match: func(p *Pup, _ time.Time) bool {
			return !p.Meta.Archived && (fireRe.MatchString(p.FirstPrompt) || fireRe.MatchString(p.Title()))
		}},
	{Key: "yak", Name: "Yak Shaving", Icon: "🐃", Color: "#f5c2e7", Blurb: "tools, setup, dotfiles",
		Look: Look{Border: "normal", Floor: "⁘", Decor: "✂️"}, Match: func(p *Pup, _ time.Time) bool {
			return !p.Meta.Archived && yakRe.MatchString(p.FirstPrompt) && len(p.PRs) == 0 && p.NUser < 40
		}},
	{Key: "marathon", Name: "Marathons", Icon: "🏔️", Color: "#94e2d5", Blurb: "huge, long-running sessions",
		Look: Look{Border: "double", Floor: "∴", Decor: "🏔️", Wide: true}, Match: func(p *Pup, _ time.Time) bool {
			return !p.Meta.Archived && (p.NUser >= 60 || p.Updated.Sub(p.Started) > 24*time.Hour)
		}},
	{Key: "shipped", Name: "Shipped", Icon: "🚀", Color: "#b4befe", Blurb: "opened a PR",
		Look: Look{Border: "rounded", Floor: " ", Decor: "🚀"}, Match: func(p *Pup, _ time.Time) bool { return len(p.PRs) > 0 }},
	{Key: "water", Name: "Water Bowl", Icon: "💧", Color: "#74c7ec", Blurb: "quick questions",
		Look: Look{Border: "rounded", Floor: "~", Decor: "💧"}, Match: func(p *Pup, _ time.Time) bool {
			return !p.Meta.Archived && p.NUser > 0 && p.NUser <= 3 && len(p.PRs) == 0 && questionRe.MatchString(p.FirstPrompt)
		}},
	{Key: "rug", Name: "Under the Rug", Icon: "🧹", Color: "#9399b2", Blurb: "real work, abandoned a week+, no PR",
		Look: Look{Border: "hidden", Floor: "▒", Decor: "🧹"}, Match: func(p *Pup, now time.Time) bool {
			return !p.Meta.Archived && !p.Status.Live() && p.NUser >= 5 && len(p.PRs) == 0 && now.Sub(p.Updated) > 7*24*time.Hour
		}},
	{Key: "dumpster", Name: "Dumpster", Icon: "🗑️", Color: "#6c7086", Blurb: "empty or archived",
		Look: Look{Border: "normal", Floor: "▚", Decor: "🗑️"}, Match: func(p *Pup, _ time.Time) bool { return p.Meta.Archived || p.Empty() }},
}

func SmartByKey(key string) *Smart {
	for i := range Smarts {
		if Smarts[i].Key == key {
			return &Smarts[i]
		}
	}
	return nil
}
