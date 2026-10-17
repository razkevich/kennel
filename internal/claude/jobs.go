package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Job is a background agent run by the Claude daemon (<claudeDir>/jobs/<short>/state.json).
type Job struct {
	Short      string    `json:"-"`
	SessionID  string    `json:"sessionId"`
	Name       string    `json:"name"`
	State      string    `json:"state"`
	Tempo      string    `json:"tempo"`
	Detail     string    `json:"detail"`
	Intent     string    `json:"intent"`
	Cwd        string    `json:"cwd"`
	Template   string    `json:"template"`
	ForkParent string    `json:"forkParentSessionId"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (j Job) Running() bool { return j.State == "running" || j.Tempo == "working" }

func ReadJobs(claudeDir string) []Job {
	files, _ := filepath.Glob(filepath.Join(claudeDir, "jobs", "*", "state.json"))
	var out []Job
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var j Job
		if json.Unmarshal(b, &j) != nil {
			continue
		}
		j.Short = filepath.Base(filepath.Dir(f))
		out = append(out, j)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].UpdatedAt.After(out[b].UpdatedAt) })
	return out
}
