// Package ask answers natural-language questions over past sessions:
// expand the question into search terms, retrieve chunks, let Claude answer with citations.
package ask

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/razkevich/kennel/internal/search"
	"github.com/razkevich/kennel/internal/store"
)

// LLM streams a completion for prompt; onText receives incremental text.
type LLM interface {
	Complete(ctx context.Context, model, prompt string, onText func(string)) (string, error)
}

// ClaudeCLI runs the user's own `claude -p`, isolated from settings, tools,
// MCP servers and session persistence so it never shows up as a session.
type ClaudeCLI struct {
	Bin string
}

func (c ClaudeCLI) Complete(ctx context.Context, model, prompt string, onText func(string)) (string, error) {
	bin := c.Bin
	if bin == "" {
		bin = "claude"
	}
	cmd := exec.CommandContext(ctx, bin, "-p", "--no-session-persistence", "--model", model,
		"--tools", "", "--strict-mcp-config", "--setting-sources", "",
		"--output-format", "stream-json", "--include-partial-messages", "--verbose")
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = os.TempDir()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("starting claude: %w", err)
	}
	var full strings.Builder
	final := ""
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var ev struct {
			Type    string `json:"type"`
			Result  string `json:"result"`
			IsError bool   `json:"is_error"`
			Event   struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			} `json:"event"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch {
		case ev.Type == "stream_event" && ev.Event.Type == "content_block_delta" && ev.Event.Delta.Type == "text_delta":
			full.WriteString(ev.Event.Delta.Text)
			if onText != nil {
				onText(ev.Event.Delta.Text)
			}
		case ev.Type == "result":
			if ev.IsError {
				return "", fmt.Errorf("claude: %s", ev.Result)
			}
			final = ev.Result
		}
	}
	if err := cmd.Wait(); err != nil && final == "" && full.Len() == 0 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("claude: %s", msg)
	}
	if final != "" {
		return final, nil
	}
	return full.String(), nil
}

type Source struct {
	N         int
	SessionID string
	Title     string
	Repo      string
	When      time.Time
	Hit       store.Hit
}

type Event struct {
	Stage   string
	Terms   []string
	Sources []Source
	Text    string
	Err     error
	Done    bool
}

type Asker struct {
	Store       *store.Store
	LLM         LLM
	FastModel   string
	AnswerModel string
	// Describe returns a session's title and repo for prompt context.
	Describe func(sessionID string) (title, repo string)
}

const maxSources = 18

// Ask runs the pipeline, reporting progress through emit. It returns when done.
func (a *Asker) Ask(ctx context.Context, question string, emit func(Event)) {
	emit(Event{Stage: "thinking of search terms"})
	terms := a.expand(ctx, question)
	emit(Event{Stage: "searching", Terms: terms})

	hits, err := a.Store.Search(search.FTSAny(terms), 200)
	if err != nil || len(hits) == 0 {
		hits, _ = a.Store.Search(search.FTSAny(strings.Fields(question)), 200)
	}
	sources := diversify(hits, a.Describe)
	emit(Event{Stage: "reading", Terms: terms, Sources: sources})
	if len(sources) == 0 {
		emit(Event{Text: "Nothing in your sessions matches that. Try different words, or plain search with /.", Done: true, Sources: sources, Terms: terms})
		return
	}

	var text strings.Builder
	_, err = a.LLM.Complete(ctx, a.answerModel(), answerPrompt(question, sources), func(t string) {
		text.WriteString(t)
		emit(Event{Stage: "answering", Text: text.String(), Sources: sources, Terms: terms})
	})
	if err != nil {
		emit(Event{Err: err, Done: true, Sources: sources, Terms: terms, Text: text.String()})
		return
	}
	emit(Event{Text: text.String(), Sources: sources, Terms: terms, Done: true})
}

func (a *Asker) answerModel() string {
	if a.AnswerModel != "" {
		return a.AnswerModel
	}
	return "sonnet"
}

func (a *Asker) expand(ctx context.Context, question string) []string {
	base := strings.Fields(question)
	model := a.FastModel
	if model == "" {
		model = "haiku"
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := a.LLM.Complete(cctx, model, expandPrompt(question), nil)
	if err != nil {
		return base
	}
	var words []string
	for _, line := range strings.Split(out, "\n") {
		for _, w := range strings.Split(line, ",") {
			w = strings.Trim(strings.TrimSpace(w), "-*•\"'`")
			if w != "" && len(w) < 40 {
				words = append(words, w)
			}
		}
	}
	if len(words) == 0 {
		return base
	}
	return append(base, words...)
}

func expandPrompt(q string) string {
	return `You turn a question about past coding-assistant conversations into search keywords.
Output 8-15 comma-separated keywords and short phrases likely to appear verbatim in those conversations:
synonyms, technical terms, file/tool names, error words. No explanation, keywords only.

Question: ` + q
}

func answerPrompt(q string, sources []Source) string {
	var b strings.Builder
	b.WriteString(`You answer questions about the user's past Claude Code sessions using only the excerpts below.
Cite excerpts inline as [n]. Be concise (under 200 words), lead with the direct answer, and name sessions by title.
If the excerpts do not contain the answer, say so plainly and mention the closest related sessions.

`)
	for _, s := range sources {
		fmt.Fprintf(&b, "[%d] session %q (%s, %s, %s)\n%s\n\n", s.N, s.Title, s.Repo, s.When.Format("2006-01-02"), s.Hit.Role, s.Hit.Text)
	}
	b.WriteString("Question: " + q + "\nAnswer:")
	return b.String()
}

// diversify keeps the best few chunks per session so one huge session can't crowd out the rest.
func diversify(hits []store.Hit, describe func(string) (string, string)) []Source {
	perSession := map[string]int{}
	var picked []store.Hit
	for _, h := range hits {
		if perSession[h.SessionID] >= 3 {
			continue
		}
		perSession[h.SessionID]++
		picked = append(picked, h)
		if len(picked) == maxSources {
			break
		}
	}
	order := map[string]int{}
	for _, h := range picked {
		if _, ok := order[h.SessionID]; !ok {
			order[h.SessionID] = len(order)
		}
	}
	sort.SliceStable(picked, func(i, j int) bool { return order[picked[i].SessionID] < order[picked[j].SessionID] })
	out := make([]Source, len(picked))
	for i, h := range picked {
		title, repo := h.SessionID[:min(8, len(h.SessionID))], ""
		if describe != nil {
			title, repo = describe(h.SessionID)
		}
		out[i] = Source{N: i + 1, SessionID: h.SessionID, Title: title, Repo: repo, When: h.TS, Hit: h}
	}
	return out
}
