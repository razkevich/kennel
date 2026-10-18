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
	// Agent is "claude" (default), "codex" or "cursor".
	Agent string
	// Files are opened alongside (Cursor gets the handoff document).
	Files     []string
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
		if t.SessionID != "" {
			args = append(args, "--session-id", t.SessionID)
		}
		if t.Title != "" {
			args = append(args, "-n", t.Title)
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
	if t.Prompt != "" {
		args = append(args, t.Prompt)
	}
	return args
}

// SafeArg strips control characters (which a terminal would act on when the
// command is typed into a shell) and keeps free text from being read as a flag.
func SafeArg(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' {
			return ' '
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
	if strings.HasPrefix(s, "-") {
		s = " " + s
	}
	return s
}

// Command is the program and arguments that open this target.
func (t Target) Command() (string, []string) {
	t.Prompt = SafeArg(t.Prompt)
	t.Title = SafeArg(t.Title)
	t.Model = strings.TrimSpace(SafeArg(t.Model))
	switch t.Agent {
	case "codex":
		var args []string
		if !t.New {
			args = append(args, "resume", t.SessionID)
		}
		if t.Model != "" && t.New {
			args = append(args, "-m", t.Model)
		}
		if t.Prompt != "" {
			args = append(args, t.Prompt)
		}
		return "codex", args
	case "cursor":
		if runtime.GOOS == "darwin" {
			return "open", append([]string{"-a", "Cursor", t.Cwd}, t.Files...)
		}
		return "cursor", append([]string{t.Cwd}, t.Files...)
	}
	return "claude", t.ClaudeArgs()
}

// ShellCommand is a copy-pasteable command line for the target.
func (t Target) ShellCommand() string {
	bin, args := t.Command()
	parts := []string{"cd", shellQuote(t.Cwd), "&&", bin}
	for _, a := range args {
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
	// App hands launches to the Kennel Mac app, which opens them as tabs.
	App Kind = "app"
)

// Run is a launch handed to the Mac app: a command line to run in a new tab
// (or the tab already showing Session, when it has one).
type Run struct {
	Cwd     string `json:"cwd"`
	Title   string `json:"title"`
	Command string `json:"command"`
	Session string `json:"session,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Focus   bool   `json:"focus,omitempty"`
}

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
	// Handoff delivers launches in App mode.
	Handoff func(Run) error
}

func commandLine(bin string, args []string) string {
	parts := []string{bin}
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func (o Opener) handoff(r Run) (string, error) {
	if o.Handoff == nil {
		return "", fmt.Errorf("no Kennel app is connected")
	}
	if err := o.Handoff(r); err != nil {
		return "", err
	}
	if r.Focus {
		// The app answers itself: it focuses the tab, or says the session runs elsewhere.
		return "", nil
	}
	return "opened " + r.Title + " in a new tab", nil
}

// Open performs the action and returns a short human description.
// Inline is handled by the caller (it needs to hand over the terminal).
func (o Opener) Open(t Target) (string, error) {
	if t.Agent == "cursor" {
		if Resolve(o.Kind) == Print {
			return t.ShellCommand(), nil
		}
		// Cursor is an editor, not a terminal agent: open it on the folder.
		bin, args := t.Command()
		if _, err := o.Runner.Run(bin, args...); err != nil {
			return "", err
		}
		return "opened Cursor in " + t.Cwd, nil
	}
	switch Resolve(o.Kind) {
	case App:
		bin, args := t.Command()
		return o.handoff(Run{Cwd: t.Cwd, Title: SafeArg(t.Title), Command: commandLine(bin, args), Session: t.SessionID, Agent: t.Agent,
			Focus: t.Live && !t.Fork && !t.New && !t.Remote && t.Prompt == ""})
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
		args = append(args, "--label", SafeArg(t.Title))
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
	bin, cargs := t.Command()
	start := []string{"agent", "start", AgentName(t.Title, t.SessionID), "--kind", bin, "--pane", pane}
	if len(cargs) > 0 {
		start = append(append(start, "--"), cargs...)
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
	bin, args := t.Command()
	cmd := bin
	for _, a := range args {
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
	title = SafeArg(title)
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
	bin, args := t.Command()
	c := exec.Command(bin, args...)
	c.Dir = t.Cwd
	return c
}

// Prompt sends text to a live herdr agent without switching to it.
func Prompt(r Runner, pane, text string) error {
	_, err := r.Run("herdr", "agent", "prompt", pane, SafeArg(text))
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
	title, command = SafeArg(title), SafeArg(command)
	switch Resolve(o.Kind) {
	case App:
		return o.handoff(Run{Cwd: cwd, Title: title, Command: command})
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
