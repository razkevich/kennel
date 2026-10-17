// Package things tracks non-agent terminals — monitors, dev servers, log tails and
// long-running commands — shown in kennel as objects in the rooms, not as pups.
package things

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Kind string

const (
	Monitor Kind = "monitor"
	Server  Kind = "server"
	Logs    Kind = "logs"
	Job     Kind = "job"
	Other   Kind = "other"
)

var Icon = map[Kind]string{Monitor: "📺", Server: "🔥", Logs: "📜", Job: "⏳", Other: "🔧"}

type Thing struct {
	ID       int64
	Name     string
	Command  string
	Cwd      string
	Kind     Kind
	KennelID int64
	// Saved is false for things discovered in herdr but never saved.
	Saved   bool
	Running bool
	Pane    string
	Tab     string
	PID     int
}

// Key identifies a thing across refreshes.
func (t Thing) Key() string {
	if t.Saved {
		return fmt.Sprintf("thing:%d", t.ID)
	}
	return "pane:" + t.Pane
}

var (
	monitorRe = regexp.MustCompile(`^(btop|htop|top|k9s|lazygit|lazydocker|glances|grafatui|watch|nvtop|bottom|btm|gitui|tig|ctop|bandwhich|harlequin|mtr)\b`)
	serverRe  = regexp.MustCompile(`\b(dev|serve|server|start|vite|next dev|rails s|runserver|uvicorn|gunicorn|flask run|air|nodemon|docker compose up|docker-compose up|caddy|ngrok|port-forward)\b`)
	logsRe    = regexp.MustCompile(`(^|\s)(tail\s+-[fF]|kubectl\s+logs|stern|docker\s+logs|journalctl\s+-f|lnav)\b|\s-f\b.*\blogs?\b`)
	jobRe     = regexp.MustCompile(`\b(test|build|terraform|tofu|make|bazel|cargo build|go test|pytest|rsync|backup|migrate|deploy|apply|sync)\b`)
)

// Infer guesses what kind of thing a command is, for its icon and drawing.
func Infer(command string) Kind {
	c := strings.TrimSpace(strings.ToLower(command))
	c = strings.TrimPrefix(c, "sudo ")
	switch {
	case monitorRe.MatchString(c):
		return Monitor
	case logsRe.MatchString(c):
		return Logs
	case serverRe.MatchString(c):
		return Server
	case jobRe.MatchString(c):
		return Job
	}
	return Other
}

var shells = map[string]bool{"zsh": true, "-zsh": true, "bash": true, "-bash": true, "fish": true, "sh": true, "nu": true, "login": true}

// IsShellTitle reports whether a terminal title just names an idle shell or a path.
func IsShellTitle(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" || shells[t] || strings.HasPrefix(t, "~") || strings.HasPrefix(t, "/") || strings.HasPrefix(t, "…") {
		return true
	}
	if i := strings.Index(t, ":"); i > 0 && strings.Contains(t[:i], "@") {
		return true
	}
	return false
}

// HerdrPane is the subset of `herdr pane list` kennel uses.
type HerdrPane struct {
	PaneID  string `json:"pane_id"`
	TabID   string `json:"tab_id"`
	Agent   string `json:"agent"`
	Title   string `json:"terminal_title_stripped"`
	Cwd     string `json:"foreground_cwd"`
	Focused bool   `json:"focused"`
}

func ParseHerdrPanes(out []byte) []HerdrPane {
	var resp struct {
		Result struct {
			Panes []HerdrPane `json:"panes"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &resp) != nil {
		return nil
	}
	return resp.Result.Panes
}

type Proc struct {
	PID     int
	Command string
}

// ParsePS reads `ps -axo pid=,command=` output.
func ParsePS(out []byte) []Proc {
	var procs []Proc
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		pidStr, cmd, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		procs = append(procs, Proc{PID: pid, Command: strings.TrimSpace(cmd)})
	}
	return procs
}

// Resolve marks saved things as running (via herdr panes or processes) and
// adds non-agent herdr panes that don't belong to any saved thing.
func Resolve(saved []Thing, panes []HerdrPane, procs []Proc) []Thing {
	used := map[string]bool{}
	out := make([]Thing, 0, len(saved))
	for _, t := range saved {
		t.Saved = true
		t.Running, t.Pane, t.Tab, t.PID = false, "", "", 0
		for _, p := range panes {
			if p.Agent != "" || used[p.PaneID] || IsShellTitle(p.Title) {
				continue
			}
			if matches(t, p.Title) {
				t.Running, t.Pane, t.Tab = true, p.PaneID, p.TabID
				used[p.PaneID] = true
				break
			}
		}
		if !t.Running {
			for _, pr := range procs {
				if matches(t, pr.Command) {
					t.Running, t.PID = true, pr.PID
					break
				}
			}
		}
		out = append(out, t)
	}
	for _, p := range panes {
		if p.Agent != "" || used[p.PaneID] || IsShellTitle(p.Title) {
			continue
		}
		out = append(out, Thing{Name: p.Title, Command: p.Title, Cwd: p.Cwd, Kind: Infer(p.Title), Running: true, Pane: p.PaneID, Tab: p.TabID})
	}
	return out
}

func matches(t Thing, text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	cmd := strings.TrimSpace(t.Command)
	if cmd != "" && (strings.Contains(text, cmd) || strings.HasPrefix(cmd, text)) {
		return true
	}
	// Terminal titles often show just the program name.
	first := strings.Fields(cmd)
	return len(first) > 0 && filepath.Base(first[0]) == text
}
