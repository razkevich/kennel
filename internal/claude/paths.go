package claude

import (
	"os"
	"path/filepath"
)

// Dir is the Claude Code config dir; KENNEL_CLAUDE_DIR overrides it so demos
// and tests never touch real sessions.
func Dir() string {
	if d := os.Getenv("KENNEL_CLAUDE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// ResumeArgs are the claude CLI arguments to continue (or fork) a session.
func ResumeArgs(id string, fork bool) []string {
	args := []string{"--resume", id}
	if fork {
		args = append(args, "--fork-session")
	}
	return args
}
