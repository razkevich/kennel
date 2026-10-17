package claude

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type Status int

const (
	Offline Status = iota
	Idle
	Done
	Working
	Waiting
)

func (s Status) String() string {
	return [...]string{"offline", "idle", "done", "working", "needs you"}[s]
}

func (s Status) Live() bool { return s != Offline }

// Live describes a running Claude Code process, as registered in <claudeDir>/sessions/<pid>.json.
type Live struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	RawStatus string `json:"status"`
	UpdatedAt int64  `json:"updatedAt"`
	Status    Status `json:"-"`
	Since     time.Time
}

// ReadLive returns live sessions keyed by session id. Stale registry files
// (process gone) are ignored, never deleted: Claude owns that directory.
func ReadLive(claudeDir string, alive func(pid int) bool) map[string]Live {
	if alive == nil {
		alive = pidAlive
	}
	out := map[string]Live{}
	files, _ := filepath.Glob(filepath.Join(claudeDir, "sessions", "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var l Live
		if json.Unmarshal(b, &l) != nil || l.SessionID == "" || l.PID <= 0 || !alive(l.PID) {
			continue
		}
		l.Status = mapStatus(l.RawStatus)
		if l.UpdatedAt > 0 {
			l.Since = time.UnixMilli(l.UpdatedAt)
		}
		if prev, ok := out[l.SessionID]; ok && prev.UpdatedAt > l.UpdatedAt {
			continue
		}
		out[l.SessionID] = l
	}
	return out
}

func mapStatus(s string) Status {
	switch s {
	case "busy", "working", "running":
		return Working
	case "waiting", "blocked", "permission", "needs_input":
		return Waiting
	default:
		return Idle
	}
}

func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
