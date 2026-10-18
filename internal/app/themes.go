package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/razkevich/kennel/internal/ask"
	"github.com/razkevich/kennel/internal/pack"
)

// ThemeLLM picks the model used for categorizing: Cloudflare Workers AI when
// configured, otherwise the same claude -p used for Ask (fast model).
func (a *App) ThemeLLM() (ask.LLM, string) {
	if a.Cfg.ThemeLLM != nil {
		return a.Cfg.ThemeLLM, "custom"
	}
	if cf, ok := ask.CloudflareFromEnv(); ok {
		return cf, "cloudflare " + cf.Model
	}
	return a.Cfg.LLM, "claude haiku"
}

type ThemeProgress struct {
	Done, Total int
	Themes      []string
	Err         error
}

const themeBatch = 40

func themeLine(p *pack.Pup) string {
	parts := []string{p.ID[:8], clip(p.Title(), 60), clip(p.FirstPrompt, 140)}
	if p.Recap != "" {
		parts = append(parts, clip(p.Recap, 110))
	}
	return strings.Join(parts, " | ")
}

// Categorize assigns a theme to each pup that has none (or to all when redo is set).
// Only titles, first asks and recaps are sent — never whole transcripts.
func (a *App) Categorize(ctx context.Context, ids []string, redo bool, progress func(ThemeProgress)) error {
	llm, _ := a.ThemeLLM()
	var todo []*pack.Pup
	known := map[string]int{}
	for _, p := range a.Pups() {
		if p.Meta.Theme != "" {
			known[p.Meta.Theme]++
		}
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for _, p := range a.Pups() {
		if p.Empty() || (len(ids) > 0 && !want[p.ID]) || (p.Meta.Theme != "" && !redo) {
			continue
		}
		todo = append(todo, p)
	}
	if len(todo) == 0 {
		return nil
	}
	themes := themeList(known, a)
	if len(known) == 0 {
		var err error
		if themes, err = proposeThemes(ctx, llm, todo, themes); err != nil {
			return err
		}
	}
	pr := ThemeProgress{Total: len(todo), Themes: themes}
	report := func() {
		if progress != nil {
			progress(pr)
		}
	}
	report()
	for start := 0; start < len(todo); start += themeBatch {
		batch := todo[start:min(start+themeBatch, len(todo))]
		var lines []string
		for _, p := range batch {
			lines = append(lines, themeLine(p))
		}
		prompt := "Assign each coding-assistant session below to exactly one theme.\n" +
			"Prefer these themes: " + strings.Join(themes, ", ") + ". Only invent a new short theme (1-3 words) if none fits.\n" +
			"Reply with one line per session, formatted exactly `id: Theme`, nothing else.\n\n" + strings.Join(lines, "\n")
		out, err := llm.Complete(ctx, "haiku", prompt, nil)
		if err != nil {
			pr.Err = err
			report()
			return err
		}
		byPrefix := map[string]*pack.Pup{}
		for _, p := range batch {
			byPrefix[p.ID[:8]] = p
		}
		for _, line := range strings.Split(out, "\n") {
			id, theme, ok := strings.Cut(strings.Trim(strings.TrimSpace(line), "-*` "), ":")
			p := byPrefix[strings.TrimSpace(id)]
			theme = strings.Trim(strings.TrimSpace(theme), "`*\"'.")
			if !ok || p == nil || theme == "" || len(theme) > 40 {
				continue
			}
			if err := a.Store.SetMetaField(p.ID, "theme", theme); err != nil {
				return err
			}
			if !contains(themes, theme) {
				themes = append(themes, theme)
				pr.Themes = themes
			}
		}
		pr.Done += len(batch)
		report()
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return a.Reload()
}

func themeList(known map[string]int, a *App) []string {
	var out []string
	for t := range known {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return known[out[i]] > known[out[j]] })
	for _, k := range a.Kennels() {
		if !contains(out, k.Name) {
			out = append(out, k.Name)
		}
	}
	return out
}

func proposeThemes(ctx context.Context, llm ask.LLM, pups []*pack.Pup, seed []string) ([]string, error) {
	sample := pups
	if len(sample) > 120 {
		sample = sample[:120]
	}
	var lines []string
	for _, p := range sample {
		lines = append(lines, "- "+clip(p.Title(), 60)+" — "+clip(p.FirstPrompt, 100))
	}
	prompt := "Here are coding-assistant sessions. Propose 8-14 short themes (1-3 words each) that cover them, " +
		"like 'Cost & FinOps', 'Incidents', 'Code review', 'Tooling'.\n"
	if len(seed) > 0 {
		prompt += "Keep these existing themes and add more only where needed: " + strings.Join(seed, ", ") + "\n"
	}
	prompt += "Reply with the themes only, comma-separated.\n\n" + strings.Join(lines, "\n")
	out, err := llm.Complete(ctx, "haiku", prompt, nil)
	if err != nil {
		return nil, fmt.Errorf("proposing themes: %w", err)
	}
	themes := append([]string{}, seed...)
	for _, t := range strings.Split(strings.ReplaceAll(out, "\n", ","), ",") {
		t = strings.Trim(strings.TrimSpace(t), "-*`\"'.0123456789) ")
		if t != "" && len(t) <= 40 && !contains(themes, t) {
			themes = append(themes, t)
		}
	}
	return themes, nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if strings.EqualFold(y, x) {
			return true
		}
	}
	return false
}
