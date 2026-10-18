package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/pack"
)

var AgentNames = map[string]string{"claude": "Claude Code", "codex": "Codex", "cursor": "Cursor"}

var slugRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// Markdown renders a session's indexed conversation as a readable document any agent can load.
func (a *App) Markdown(p *pack.Pup) (string, error) {
	chunks, err := a.Store.Chunks(p.ID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", p.Title())
	fmt.Fprintf(&b, "- agent: %s\n- folder: %s\n", AgentNames[p.AgentKind()], p.Cwd)
	if len(p.Branches) > 0 {
		fmt.Fprintf(&b, "- branches: %s\n", strings.Join(p.Branches, ", "))
	}
	fmt.Fprintf(&b, "- active: %s → %s\n", p.Started.Format("2006-01-02 15:04"), p.Updated.Format("2006-01-02 15:04"))
	for _, pr := range p.PRs {
		fmt.Fprintf(&b, "- PR: %s\n", pr)
	}
	fmt.Fprintf(&b, "- session id: %s\n", p.ID)
	if p.Recap != "" {
		fmt.Fprintf(&b, "\n## Where it stands\n\n%s\n", p.Recap)
	}
	b.WriteString("\n## Conversation\n")
	who := map[claude.Role]string{claude.RoleUser: "🧑 You", claude.RoleAssistant: "🤖 " + AgentNames[p.AgentKind()], claude.RoleRecap: "✦ Recap"}
	prev := claude.Role("")
	for _, c := range chunks {
		if c.Role == claude.RoleTool {
			fmt.Fprintf(&b, "\n> ⚙ `%s`\n", strings.ReplaceAll(c.Text, "`", "'"))
			prev = c.Role
			continue
		}
		if c.Role != prev {
			fmt.Fprintf(&b, "\n### %s — %s\n\n", who[c.Role], c.TS.Format("Jan 2 15:04"))
		}
		b.WriteString(c.Text + "\n")
		prev = c.Role
	}
	return b.String(), nil
}

// Export writes the Markdown to kennel's handoffs folder and returns its path.
func (a *App) Export(p *pack.Pup) (string, error) {
	md, err := a.Markdown(p)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(a.Cfg.Home, "handoffs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(p.Title()), "-"), "-")
	if len(slug) > 40 {
		slug = slug[:40]
	}
	path := filepath.Join(dir, slug+"-"+p.ID[:8]+".md")
	return path, os.WriteFile(path, []byte(md), 0o600)
}

// ConvertTarget starts the same work in another agent: it exports the
// conversation to Markdown and opens the target agent with a brief pointing at it.
func (a *App) ConvertTarget(id, agent, title string) (opener.Target, string, error) {
	p := a.Pup(id)
	if p == nil {
		return opener.Target{}, "", fmt.Errorf("unknown session %s", id)
	}
	if _, ok := AgentNames[agent]; !ok {
		return opener.Target{}, "", fmt.Errorf("unknown agent %q", agent)
	}
	path, err := a.Export(p)
	if err != nil {
		return opener.Target{}, "", err
	}
	if title == "" {
		title = p.Title()
	}
	brief := fmt.Sprintf("We're continuing work that started in %s (session %q). The full conversation is in %s — read it, then give me a short status (what was decided, what's open, next steps) and wait.",
		AgentNames[p.AgentKind()], p.Title(), path)
	t := opener.Target{Agent: agent, New: true, Cwd: a.Target(p, false, false).Cwd, Title: title, Prompt: brief}
	if agent == "cursor" {
		t.Prompt = ""
		t.Files = []string{path}
	}
	return t, path, nil
}
