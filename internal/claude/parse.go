package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type record struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	SessionID        string          `json:"sessionId"`
	Timestamp        string          `json:"timestamp"`
	Cwd              string          `json:"cwd"`
	GitBranch        string          `json:"gitBranch"`
	Version          string          `json:"version"`
	IsMeta           bool            `json:"isMeta"`
	IsSidechain      bool            `json:"isSidechain"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	Message          *message        `json:"message"`
	Content          json.RawMessage `json:"content"`
	CustomTitle      string          `json:"customTitle"`
	AgentName        string          `json:"agentName"`
	AITitle          string          `json:"aiTitle"`
	AgentColor       string          `json:"agentColor"`
	PrURL            string          `json:"prUrl"`
	RelocatedCwd     string          `json:"relocatedCwd"`
	ContinuedIn      string          `json:"continuedInSessionId"`
	FrameURL         string          `json:"frameUrl"`
	Title            string          `json:"title"`
	TotalCostUSD     float64         `json:"totalCostUSD"`
	BridgeSessionID  string          `json:"bridgeSessionId"`
	TaskID           string          `json:"taskId"`
	Cron             string          `json:"cron"`
	Prompt           string          `json:"prompt"`
	WorktreeSession  *struct {
		WorktreeName string `json:"worktreeName"`
	} `json:"worktreeSession"`
}

type message struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// Session is the accumulated, index-friendly view of one transcript.
type Session struct {
	ID          string
	Project     string
	Path        string
	Cwd         string
	LastCwd     string
	CustomTitle string
	AgentName   string
	AITitle     string
	FirstPrompt string
	LastPrompt  string
	Recap       string
	Color       string
	ContinuedIn string
	Worktree    string
	Version     string
	Remote      string
	Branches    []string
	PRs         []string
	Artifacts   []Artifact
	Crons       []Cron
	Models      []string
	Started     time.Time
	Updated     time.Time
	NUser       int
	NAssistant  int
	NTools      int
	Cost        float64
}

type Artifact struct {
	URL   string    `json:"url"`
	Title string    `json:"title"`
	At    time.Time `json:"at"`
}

// Cron is a scheduled task created inside a session (CronCreate / /loop).
type Cron struct {
	ID       string    `json:"id"`
	Expr     string    `json:"expr"`
	Prompt   string    `json:"prompt"`
	LastFire time.Time `json:"last_fire"`
	Fires    int       `json:"fires"`
}

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleRecap     Role = "recap"
)

// Chunk is a searchable piece of conversation text.
type Chunk struct {
	Seq  int
	Role Role
	TS   time.Time
	Text string
}

const maxChunk = 1500

// Parser folds transcript lines into a Session, producing chunks as it goes.
// It is resumable: feed it more bytes later and it continues where it left off.
type Parser struct {
	S      *Session
	Seq    int
	Chunks []Chunk
}

func NewParser(s *Session, seq int) *Parser {
	return &Parser{S: s, Seq: seq}
}

// Feed consumes complete lines from r and returns how many bytes were consumed.
// A trailing partial line (a write in progress) is left for the next call.
func (p *Parser) Feed(r io.Reader) (int64, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	var consumed int64
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			consumed += int64(len(line))
			p.line(line)
		}
		if err == io.EOF {
			return consumed, nil
		}
		if err != nil {
			return consumed, err
		}
	}
}

func (p *Parser) line(b []byte) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || b[0] != '{' {
		return
	}
	var r record
	if json.Unmarshal(b, &r) != nil {
		return
	}
	s := p.S
	if s.ID == "" && r.SessionID != "" {
		s.ID = r.SessionID
	}
	ts := parseTime(r.Timestamp)
	if !ts.IsZero() {
		if s.Started.IsZero() || ts.Before(s.Started) {
			s.Started = ts
		}
		if ts.After(s.Updated) {
			s.Updated = ts
		}
	}
	if r.IsSidechain {
		return
	}
	if r.Cwd != "" {
		if s.Cwd == "" {
			s.Cwd = r.Cwd
		}
		s.LastCwd = r.Cwd
	}
	if r.GitBranch != "" && r.GitBranch != "HEAD" {
		s.Branches = appendUnique(s.Branches, r.GitBranch)
	}
	if r.Version != "" {
		s.Version = r.Version
	}

	switch r.Type {
	case "custom-title":
		s.CustomTitle = r.CustomTitle
	case "agent-name":
		s.AgentName = r.AgentName
	case "ai-title":
		s.AITitle = r.AITitle
	case "agent-color":
		s.Color = r.AgentColor
	case "pr-link":
		s.PRs = appendUnique(s.PRs, r.PrURL)
	case "frame-link":
		if r.FrameURL != "" {
			s.addArtifact(Artifact{URL: r.FrameURL, Title: r.Title, At: ts})
		}
	case "relocated":
		if r.RelocatedCwd != "" {
			s.LastCwd = r.RelocatedCwd
		}
	case "continued-in":
		s.ContinuedIn = r.ContinuedIn
	case "worktree-state":
		if r.WorktreeSession != nil {
			s.Worktree = r.WorktreeSession.WorktreeName
		}
	case "bridge-session":
		s.Remote = r.BridgeSessionID
	case "cost-state":
		if r.TotalCostUSD > s.Cost {
			s.Cost = r.TotalCostUSD
		}
	case "system":
		if r.Subtype == "scheduled_task_fire" && r.TaskID != "" {
			s.fireCron(r.TaskID, r.Cron, r.Prompt, ts)
		}
		if r.Subtype == "away_summary" {
			if txt := rawString(r.Content); txt != "" {
				txt = strings.TrimSpace(strings.TrimSuffix(txt, "(disable recaps in /config)"))
				s.Recap = txt
				p.emit(RoleRecap, ts, txt)
			}
		}
	case "user":
		if r.IsMeta || r.IsCompactSummary || r.Message == nil {
			return
		}
		for _, txt := range userTexts(r.Message.Content) {
			s.NUser++
			if s.FirstPrompt == "" {
				s.FirstPrompt = oneLine(txt, 300)
			}
			s.LastPrompt = oneLine(txt, 300)
			p.emit(RoleUser, ts, txt)
		}
	case "assistant":
		if r.Message == nil {
			return
		}
		if m := r.Message.Model; m != "" && m != "<synthetic>" {
			s.Models = appendUnique(s.Models, m)
		}
		var blocks []block
		if json.Unmarshal(r.Message.Content, &blocks) != nil {
			return
		}
		counted := false
		for _, bl := range blocks {
			switch bl.Type {
			case "text":
				if strings.TrimSpace(bl.Text) == "" {
					continue
				}
				if !counted {
					s.NAssistant++
					counted = true
				}
				p.emit(RoleAssistant, ts, bl.Text)
			case "tool_use":
				s.NTools++
				if sum := toolSummary(bl.Name, bl.Input); sum != "" {
					p.emit(RoleTool, ts, sum)
				}
			}
		}
	}
}

func (p *Parser) emit(role Role, ts time.Time, text string) {
	text = strings.TrimSpace(text)
	for len(text) > 0 {
		part := text
		if len(part) > maxChunk {
			cut := strings.LastIndexAny(part[:maxChunk], "\n. ")
			if cut < maxChunk/2 {
				cut = maxChunk
			}
			for cut < len(part) && !utf8Start(part[cut]) {
				cut++
			}
			part = part[:cut]
		}
		p.Chunks = append(p.Chunks, Chunk{Seq: p.Seq, Role: role, TS: ts, Text: part})
		p.Seq++
		text = strings.TrimSpace(text[len(part):])
	}
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

var noiseRe = regexp.MustCompile(`(?s)^\s*(<command-name>|<local-command-|<command-message>|<system-reminder>|<bash-|<task-notification>|\[Request interrupted|Caveat: The messages below)`)

func userTexts(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var out []string
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil && keepUser(s) {
			out = append(out, s)
		}
		return out
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	for _, b := range blocks {
		if b.Type == "text" && keepUser(b.Text) {
			out = append(out, b.Text)
		}
	}
	return out
}

func keepUser(s string) bool {
	return strings.TrimSpace(s) != "" && !noiseRe.MatchString(s)
}

func toolSummary(name string, input json.RawMessage) string {
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := in[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	arg := pick("command", "file_path", "path", "pattern", "url", "query", "description", "skill", "prompt")
	if arg == "" {
		return name
	}
	return name + ": " + oneLine(arg, 400)
}

func rawString(raw json.RawMessage) string {
	var s string
	if len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

var pasteRe = regexp.MustCompile(`(?s)<pasted_content[^>]*>|</pasted_content[^>]*>`)

func oneLine(s string, max int) string {
	s = pasteRe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > max {
		s = string([]rune(s)[:max-1]) + "…"
	}
	return s
}

func (s *Session) addArtifact(a Artifact) {
	for i := range s.Artifacts {
		if s.Artifacts[i].URL == a.URL {
			if a.Title != "" {
				s.Artifacts[i].Title = a.Title
			}
			if a.At.After(s.Artifacts[i].At) {
				s.Artifacts[i].At = a.At
			}
			return
		}
	}
	s.Artifacts = append(s.Artifacts, a)
}

func (s *Session) fireCron(id, expr, prompt string, at time.Time) {
	for i := range s.Crons {
		if s.Crons[i].ID == id {
			s.Crons[i].Fires++
			if at.After(s.Crons[i].LastFire) {
				s.Crons[i].LastFire = at
			}
			return
		}
	}
	s.Crons = append(s.Crons, Cron{ID: id, Expr: expr, Prompt: oneLine(prompt, 500), LastFire: at, Fires: 1})
}

func appendUnique(xs []string, x string) []string {
	if x == "" {
		return xs
	}
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// SessionFiles lists top-level transcripts under <claudeDir>/projects.
// Subagent transcripts live in per-session subdirectories and are skipped.
func SessionFiles(claudeDir string) ([]string, error) {
	return filepath.Glob(filepath.Join(claudeDir, "projects", "*", "*.jsonl"))
}

// ParseFile reads a whole transcript; used by the transcript viewer and tests.
func ParseFile(path string) (*Session, []Chunk, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	p := NewParser(&Session{Path: path, Project: filepath.Base(filepath.Dir(path))}, 0)
	if _, err := p.Feed(f); err != nil {
		return nil, nil, err
	}
	if p.S.ID == "" {
		p.S.ID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	return p.S, p.Chunks, nil
}

// DisplayTitle picks the most human-chosen name available.
func (s *Session) DisplayTitle() string {
	for _, t := range []string{s.CustomTitle, s.AgentName, cleanAITitle(s.AITitle)} {
		if strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
	}
	if s.FirstPrompt != "" {
		return AutoTitle(s.FirstPrompt)
	}
	return "(empty)"
}

var (
	slackHeadRe = regexp.MustCompile(`^\s*([A-Z][\p{L}'-]+(?:\s+[A-Z][\p{L}.'-]+){0,2})\s+\[?\d{1,2}:\d{2}\s?[AP]M\]?`)
	prURLRe     = regexp.MustCompile(`github\.com/([^/\s]+)/([^/\s]+)/pull/(\d+)`)
	fillerRe    = regexp.MustCompile(`(?i)^(please|pls|can you|could you|can we|let's|lets|i want to|i need to|help me|hey|hi|ok|so)\s+`)
)

// AutoTitle makes a short label for sessions nobody named.
func AutoTitle(prompt string) string {
	p := oneLine(prompt, 300)
	if m := slackHeadRe.FindStringSubmatch(p); m != nil {
		return "ask from " + strings.Fields(m[1])[0]
	}
	if m := prURLRe.FindStringSubmatch(p); m != nil && regexp.MustCompile(`(?i)\brevie+w`).MatchString(p) {
		return "review " + m[2] + "#" + m[3]
	}
	for i := 0; i < 3; i++ {
		p = fillerRe.ReplaceAllString(p, "")
	}
	words := strings.Fields(p)
	if len(words) > 6 {
		words = words[:6]
	}
	t := strings.TrimRight(strings.Join(words, " "), ",.:;!?")
	return oneLine(t, 40)
}

func cleanAITitle(t string) string {
	return strings.TrimSpace(strings.ReplaceAll(t, "⑂", ""))
}
