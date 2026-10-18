package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Codex CLI keeps rollouts at <codexDir>/sessions/YYYY/MM/DD/rollout-*.jsonl:
// {"timestamp","type":"session_meta"|"response_item"|"event_msg"|"turn_context","payload":{…}}.

type codexRecord struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexPayload struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Cwd       string `json:"cwd"`
	Role      string `json:"role"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Message   string `json:"message"`
	Model     string `json:"model"`
	Title     string `json:"thread_name"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Git *struct {
		Branch string `json:"branch"`
	} `json:"git"`
}

func CodexDir() string {
	if d := os.Getenv("KENNEL_CODEX_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

// CodexFiles lists Codex rollouts, newest layout first.
func CodexFiles(codexDir string) []string {
	files, _ := filepath.Glob(filepath.Join(codexDir, "sessions", "*", "*", "*", "rollout-*.jsonl"))
	return files
}

func (p *Parser) codexLine(b []byte) {
	var r codexRecord
	if json.Unmarshal(b, &r) != nil {
		return
	}
	var pl codexPayload
	if json.Unmarshal(r.Payload, &pl) != nil {
		return
	}
	s := p.S
	s.Agent = "codex"
	ts := parseTime(r.Timestamp)
	if !ts.IsZero() {
		if s.Started.IsZero() || ts.Before(s.Started) {
			s.Started = ts
		}
		if ts.After(s.Updated) {
			s.Updated = ts
		}
	}
	switch r.Type {
	case "session_meta":
		if pl.ID != "" {
			s.ID = pl.ID
		}
		if pl.Cwd != "" {
			if s.Cwd == "" {
				s.Cwd = pl.Cwd
			}
			s.LastCwd = pl.Cwd
		}
		if pl.Git != nil && pl.Git.Branch != "" {
			s.Branches = appendUnique(s.Branches, pl.Git.Branch)
		}
		if pl.Title != "" {
			s.CustomTitle = pl.Title
		}
	case "turn_context":
		if pl.Cwd != "" {
			s.LastCwd = pl.Cwd
		}
		if pl.Model != "" {
			s.Models = appendUnique(s.Models, pl.Model)
		}
	case "response_item":
		switch pl.Type {
		case "message":
			var texts []string
			for _, c := range pl.Content {
				if strings.TrimSpace(c.Text) != "" {
					texts = append(texts, c.Text)
				}
			}
			text := strings.Join(texts, "\n")
			if strings.TrimSpace(text) == "" {
				return
			}
			switch pl.Role {
			case "user":
				// Codex injects environment and instructions as user messages wrapped in tags.
				if t := strings.TrimSpace(text); strings.HasPrefix(t, "<") {
					return
				}
				s.NUser++
				if s.FirstPrompt == "" {
					s.FirstPrompt = oneLine(text, 300)
				}
				s.LastPrompt = oneLine(text, 300)
				p.emit(RoleUser, ts, text)
			case "assistant":
				s.NAssistant++
				p.emit(RoleAssistant, ts, text)
			}
		case "function_call", "custom_tool_call", "local_shell_call":
			s.NTools++
			arg := pl.Arguments
			var a struct {
				Command any    `json:"command"`
				Path    string `json:"path"`
			}
			if json.Unmarshal([]byte(arg), &a) == nil {
				switch c := a.Command.(type) {
				case []any:
					var parts []string
					for _, x := range c {
						if str, ok := x.(string); ok {
							parts = append(parts, str)
						}
					}
					if len(parts) >= 3 && (parts[0] == "bash" || parts[0] == "zsh") {
						parts = parts[2:]
					}
					arg = strings.Join(parts, " ")
				case string:
					arg = c
				}
				if a.Path != "" && arg == pl.Arguments {
					arg = a.Path
				}
			}
			p.emit(RoleTool, ts, pl.Name+": "+oneLine(arg, 400))
		}
	}
}
