package opener

import (
	"encoding/json"
	"os/exec"

	"github.com/razkevich/kennel/internal/claude"
)

type HerdrAgent struct {
	Pane      string
	SessionID string
	Title     string
	Cwd       string
	Status    claude.Status
	Focused   bool
}

// HerdrAgents lists Claude agents known to a running herdr server. It returns
// nil when herdr isn't installed or not running.
func HerdrAgents(r Runner) []HerdrAgent {
	if _, err := exec.LookPath("herdr"); err != nil {
		if _, real := r.(ExecRunner); real {
			return nil
		}
	}
	out, err := r.Run("herdr", "agent", "list")
	if err != nil {
		return nil
	}
	var resp struct {
		Result struct {
			Agents []struct {
				Agent        string `json:"agent"`
				Status       string `json:"agent_status"`
				Cwd          string `json:"cwd"`
				Focused      bool   `json:"focused"`
				PaneID       string `json:"pane_id"`
				Title        string `json:"terminal_title_stripped"`
				AgentSession *struct {
					Value string `json:"value"`
				} `json:"agent_session"`
			} `json:"agents"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &resp) != nil {
		return nil
	}
	var agents []HerdrAgent
	for _, a := range resp.Result.Agents {
		if a.Agent != "claude" && a.Agent != "codex" && a.Agent != "cursor" {
			continue
		}
		h := HerdrAgent{Pane: a.PaneID, Title: a.Title, Cwd: a.Cwd, Focused: a.Focused}
		if a.AgentSession != nil {
			h.SessionID = a.AgentSession.Value
		}
		switch a.Status {
		case "working":
			h.Status = claude.Working
		case "blocked":
			h.Status = claude.Waiting
		case "done":
			h.Status = claude.Done
		default:
			h.Status = claude.Idle
		}
		agents = append(agents, h)
	}
	return agents
}
