// Package opener resumes or focuses Claude sessions in herdr, Ghostty or the current terminal.
package opener

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/razkevich/kennel/internal/claude"
)

// Runner executes external commands; tests substitute a recorder.
type Runner interface {
	Run(name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) ([]byte, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// Target says what to open.
type Target struct {
	SessionID string
	Cwd       string
	Title     string
	Fork      bool
	New       bool
	Pane      string
	Live      bool
	Remote    bool
	// Prompt and Model seed a new session (spawn doors).
	Prompt string
	Model  string
	// Workspace routes herdr tabs into a workspace with this label (portals).
	Workspace string
}

// ClaudeArgs are the arguments passed to claude for this target.
func (t Target) ClaudeArgs() []string {
	var args []string
	if t.New {
		if t.Title != "" {
			args = []string{"-n", t.Title}
		}
		if t.Model != "" {
			args = append(args, "--model", t.Model)
		}
	} else {
		args = claude.ResumeArgs(t.SessionID, t.Fork)
	}
	if t.Remote {
		args = append(args, "--remote-control")
	}
	if t.New && t.Prompt != "" {
		args = append(args, t.Prompt)
	}
	return args
}

// ShellCommand is a copy-pasteable command line for the target.
func (t Target) ShellCommand() string {
	parts := []string{"cd", shellQuote(t.Cwd), "&&", "claude"}
	for _, a := range t.ClaudeArgs() {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

type Kind string

const (
	Auto    Kind = "auto"
	Herdr   Kind = "herdr"
	Ghostty Kind = "ghostty"
	Inline  Kind = "inline"
	Print   Kind = "print"
)

// Resolve picks a concrete opener for auto, based on where kennel runs.
func Resolve(k Kind) Kind {
	if k != Auto && k != "" {
		return k
	}
	if os.Getenv("HERDR_ENV") == "1" {
		return Herdr
	}
	if os.Getenv("TERM_PROGRAM") == "ghostty" {
		return Ghostty
	}
	return Inline
}

type Opener struct {
	Kind   Kind
	Runner Runner
}

// Open performs the action and returns a short human description.
// Inline is handled by the caller (it needs to hand over the terminal).
func (o Opener) Open(t Target) (string, error) {
	switch Resolve(o.Kind) {
	case Herdr:
		return o.herdr(t)
	case Ghostty:
		return o.ghostty(t)
	case Print:
		return t.ShellCommand(), nil
	default:
		return "", fmt.Errorf("opener %q must be run inline", o.Kind)
	}
}

func (o Opener) herdr(t Target) (string, error) {
	if t.Live && t.Pane != "" && t.Remote {
		if err := Prompt(o.Runner, t.Pane, "/remote-control"); err != nil {
			return "", err
		}
		return "remote control requested in " + t.Pane, nil
	}
	if t.Live && t.Pane != "" && !t.Fork && !t.New {
		if _, err := o.Runner.Run("herdr", "agent", "focus", t.Pane); err != nil {
			return "", err
		}
		return "focused " + t.Pane, nil
	}
	args := []string{"tab", "create", "--cwd", t.Cwd, "--focus"}
	if t.Title != "" {
		args = append(args, "--label", t.Title)
	}
	if t.Workspace != "" {
		ws, err := o.workspace(t.Workspace, t.Cwd)
		if err != nil {
			return "", err
		}
		args = append(args, "--workspace", ws)
	}
	out, err := o.Runner.Run("herdr", args...)
	if err != nil {
		return "", err
	}
	var resp struct {
		Result struct {
			RootPane struct {
				PaneID string `json:"pane_id"`
			} `json:"root_pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil || resp.Result.RootPane.PaneID == "" {
		return "", fmt.Errorf("herdr tab create: unexpected response: %s", strings.TrimSpace(string(out)))
	}
	pane := resp.Result.RootPane.PaneID
	start := []string{"agent", "start", AgentName(t.Title, t.SessionID), "--kind", "claude", "--pane", pane}
	if ca := t.ClaudeArgs(); len(ca) > 0 {
		start = append(append(start, "--"), ca...)
	}
	if _, err := o.Runner.Run("herdr", start...); err != nil {
		return "", err
	}
	return "opened in herdr " + pane, nil
}

var nameClean = regexp.MustCompile(`[^a-z0-9_-]+`)

// AgentName derives a herdr agent name ([a-z][a-z0-9_-]{0,31}).
func AgentName(title, id string) string {
	n := nameClean.ReplaceAllString(strings.ToLower(title), "-")
	n = strings.Trim(n, "-_")
	if n == "" || n[0] < 'a' || n[0] > 'z' {
		n = "pup-" + n
	}
	if len(id) >= 4 {
		n = strings.TrimRight(n[:min(len(n), 26)], "-_") + "-" + id[:4]
	}
	return n[:min(len(n), 32)]
}

func (o Opener) ghostty(t Target) (string, error) {
	if t.Live && t.Title != "" && !t.Fork && !t.New {
		if _, err := o.Runner.Run("osascript", "-e", ghosttyFocusScript(t.Title)); err == nil {
			return "focused in Ghostty", nil
		}
	}
	if _, err := o.Runner.Run("osascript", "-e", ghosttyOpenScript(t)); err != nil {
		return "", err
	}
	return "opened in Ghostty", nil
}

func ghosttyOpenScript(t Target) string {
	cmd := "claude"
	for _, a := range t.ClaudeArgs() {
		cmd += " " + shellQuote(a)
	}
	return fmt.Sprintf(`tell application "Ghostty"
	set cfg to new surface configuration
	set initial working directory of cfg to %s
	set initial input of cfg to %s
	if (count of windows) > 0 then
		new tab in front window with configuration cfg
	else
		new window with configuration cfg
	end if
	activate
end tell`, asQuote(t.Cwd), asQuote(cmd+"\n"))
}

func ghosttyFocusScript(title string) string {
	return fmt.Sprintf(`tell application "Ghostty"
	repeat with w in windows
		repeat with tb in tabs of w
			set term to focused terminal of tb
			if name of term contains %s then
				focus term
				return "ok"
			end if
		end repeat
	end repeat
	error "not found"
end tell`, asQuote(title))
}

// InlineCmd is the process to run in kennel's own terminal.
func InlineCmd(t Target) *exec.Cmd {
	c := exec.Command("claude", t.ClaudeArgs()...)
	c.Dir = t.Cwd
	return c
}

// Prompt sends text to a live herdr agent without switching to it.
func Prompt(r Runner, pane, text string) error {
	_, err := r.Run("herdr", "agent", "prompt", pane, text)
	return err
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`!*?[]{}()<>|&;#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func asQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// RunCommand opens a terminal running an arbitrary command (a "thing").
func (o Opener) RunCommand(cwd, title, command string) (string, error) {
	switch Resolve(o.Kind) {
	case Herdr:
		out, err := o.Runner.Run("herdr", "tab", "create", "--cwd", cwd, "--focus", "--label", title)
		if err != nil {
			return "", err
		}
		var resp struct {
			Result struct {
				RootPane struct {
					PaneID string `json:"pane_id"`
				} `json:"root_pane"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &resp); err != nil || resp.Result.RootPane.PaneID == "" {
			return "", fmt.Errorf("herdr tab create: unexpected response: %s", strings.TrimSpace(string(out)))
		}
		if _, err := o.Runner.Run("herdr", "pane", "run", resp.Result.RootPane.PaneID, command); err != nil {
			return "", err
		}
		return "started " + title + " in herdr", nil
	case Ghostty:
		script := fmt.Sprintf(`tell application "Ghostty"
	set cfg to new surface configuration
	set initial working directory of cfg to %s
	set initial input of cfg to %s
	if (count of windows) > 0 then
		new tab in front window with configuration cfg
	else
		new window with configuration cfg
	end if
	activate
end tell`, asQuote(cwd), asQuote(command+"\n"))
		if _, err := o.Runner.Run("osascript", "-e", script); err != nil {
			return "", err
		}
		return "started " + title + " in Ghostty", nil
	case Print:
		return "cd " + shellQuote(cwd) + " && " + command, nil
	}
	return "", fmt.Errorf("opener %q must be run inline", o.Kind)
}

// InlineShell runs a command through the user's shell in kennel's terminal.
func InlineShell(cwd, command string) *exec.Cmd {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	c := exec.Command(sh, "-lc", command)
	c.Dir = cwd
	return c
}

// workspace finds a herdr workspace by label, creating it when missing.
func (o Opener) workspace(label, cwd string) (string, error) {
	out, err := o.Runner.Run("herdr", "workspace", "list")
	if err != nil {
		return "", err
	}
	var list struct {
		Result struct {
			Workspaces []struct {
				ID    string `json:"workspace_id"`
				Label string `json:"label"`
			} `json:"workspaces"`
		} `json:"result"`
	}
	_ = json.Unmarshal(out, &list)
	for _, w := range list.Result.Workspaces {
		if strings.EqualFold(w.Label, label) {
			return w.ID, nil
		}
	}
	out, err = o.Runner.Run("herdr", "workspace", "create", "--label", label, "--cwd", cwd, "--no-focus")
	if err != nil {
		return "", err
	}
	var created struct {
		Result struct {
			Workspace struct {
				ID string `json:"workspace_id"`
			} `json:"workspace"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &created); err != nil || created.Result.Workspace.ID == "" {
		return "", fmt.Errorf("herdr workspace create: unexpected response: %s", strings.TrimSpace(string(out)))
	}
	return created.Result.Workspace.ID, nil
}

// Notify shows a desktop notification (macOS Notification Center, or notify-send on Linux).
func Notify(r Runner, title, body string) error {
	if runtime.GOOS == "darwin" {
		_, err := r.Run("osascript", "-e", fmt.Sprintf("display notification %s with title %s sound name \"Glass\"", asQuote(body), asQuote(title)))
		return err
	}
	_, err := r.Run("notify-send", title, body)
	return err
}
