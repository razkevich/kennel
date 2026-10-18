package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/razkevich/kennel/internal/opener"
	"github.com/razkevich/kennel/internal/pack"
)

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// ProjectDirName is how Claude Code names a project's folder under ~/.claude/projects.
func ProjectDirName(cwd string) string {
	return nonAlnum.ReplaceAllString(cwd, "-")
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Clone copies a session into a new, independent transcript — optionally into
// another project folder — so it can be resumed there. The original is untouched.
func (a *App) Clone(id, targetCwd, title string) (string, error) {
	p := a.Pup(id)
	if p == nil {
		return "", fmt.Errorf("unknown session %s", id)
	}
	src, err := os.ReadFile(p.Path)
	if err != nil {
		return "", err
	}
	if targetCwd == "" {
		targetCwd = p.Cwd
	}
	newID := newUUID()
	out := bytes.ReplaceAll(src, []byte(`"sessionId":"`+id+`"`), []byte(`"sessionId":"`+newID+`"`))
	out = bytes.ReplaceAll(out, []byte(`"session_id":"`+id+`"`), []byte(`"session_id":"`+newID+`"`))
	if targetCwd != p.Cwd {
		for _, old := range []string{p.Cwd, p.LastCwd} {
			if old == "" {
				continue
			}
			from, _ := json.Marshal(old)
			to, _ := json.Marshal(targetCwd)
			out = bytes.ReplaceAll(out, append([]byte(`"cwd":`), from...), append([]byte(`"cwd":`), to...))
		}
	}
	if title == "" {
		title = p.Title() + "-CLONE"
	}
	for _, rec := range []map[string]string{
		{"type": "custom-title", "customTitle": title, "sessionId": newID},
		{"type": "agent-name", "agentName": title, "sessionId": newID},
	} {
		b, _ := json.Marshal(rec)
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		out = append(append(out, b...), '\n')
	}
	dir := filepath.Join(a.Cfg.ClaudeDir, "projects", ProjectDirName(targetCwd))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, newID+".jsonl")
	if err := os.WriteFile(dst, out, 0o600); err != nil {
		return "", err
	}
	if _, err := a.Sync(context.Background(), nil); err != nil {
		return newID, err
	}
	return newID, nil
}

// Brief is a one-paragraph handoff that seeds a new session with what earlier
// sessions did, pointing at their transcripts for details.
func Brief(pups []*pack.Pup) string {
	var parts []string
	for _, p := range pups {
		var bits []string
		bits = append(bits, fmt.Sprintf("%q (%s", p.Title(), p.Repo()))
		if b := p.Branch(); b != "" {
			bits[0] += ", branch " + b
		}
		bits[0] += ", last active " + p.Updated.Format("2006-01-02") + ")"
		if p.Recap != "" {
			bits = append(bits, "status: "+p.Recap)
		}
		if p.FirstPrompt != "" {
			bits = append(bits, "started with: "+clip(p.FirstPrompt, 240))
		}
		if p.LastPrompt != "" && p.LastPrompt != p.FirstPrompt {
			bits = append(bits, "last asked: "+clip(p.LastPrompt, 240))
		}
		for _, pr := range p.PRs {
			bits = append(bits, "PR "+pr)
		}
		bits = append(bits, "transcript: "+p.Path)
		parts = append(parts, strings.Join(bits, "; "))
	}
	intro := "We're picking up from an earlier Claude Code session."
	if len(pups) > 1 {
		intro = fmt.Sprintf("We're combining %d earlier Claude Code sessions into one.", len(pups))
	}
	return intro + " " + strings.Join(parts, " || ") +
		" — The transcripts are JSONL; read only the parts you need. Start with a short combined status (decisions, open questions, next steps), then wait for me."
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// Merge starts a new session seeded with a brief of the given sessions.
// With one session this is a "distill": a fresh start without the long history.
func (a *App) MergeTarget(ids []string, cwd, title string) (opener.Target, error) {
	var pups []*pack.Pup
	for _, id := range ids {
		p := a.Pup(id)
		if p == nil {
			return opener.Target{}, fmt.Errorf("unknown session %s", id)
		}
		pups = append(pups, p)
	}
	if len(pups) == 0 {
		return opener.Target{}, fmt.Errorf("nothing to merge")
	}
	if cwd == "" {
		cwd = a.Target(pups[0], false, false).Cwd
	}
	if title == "" {
		title = pups[0].Title() + "+"
		if len(pups) == 1 {
			title = pups[0].Title() + "-FRESH"
		}
	}
	return opener.Target{Cwd: cwd, Title: title, New: true, Prompt: Brief(pups)}, nil
}
