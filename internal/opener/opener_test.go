package opener

import (
	"strings"
	"testing"
)

func TestSafeArgNeutralizesControlBytesAndFlags(t *testing.T) {
	evil := "fix it\x03touch /tmp/pwned; : '\x15\x1b[2J"
	got := SafeArg(evil)
	if strings.ContainsAny(got, "\x03\x15\x1b") {
		t.Fatalf("control bytes survived: %q", got)
	}
	if got := SafeArg("--dangerously-skip-permissions"); !strings.HasPrefix(got, " ") {
		t.Fatalf("flag-like text must not stay a flag: %q", got)
	}
	tg := Target{Cwd: "/x", New: true, Title: "A\x03B", Prompt: "line1\nline2\x07"}
	_, args := tg.Command()
	for _, a := range args {
		if strings.ContainsAny(a, "\x03\x07\n") {
			t.Fatalf("arg %q still has control characters", a)
		}
	}
	if script := ghosttyOpenScript(tg); strings.ContainsAny(script, "\x03\x07") {
		t.Fatal("ghostty script carries control bytes")
	}
}

func TestCommandsPerAgent(t *testing.T) {
	cases := []struct {
		t    Target
		want string
	}{
		{Target{SessionID: "abc", Cwd: "/r"}, "claude --resume abc"},
		{Target{SessionID: "abc", Cwd: "/r", Fork: true}, "claude --resume abc --fork-session"},
		{Target{Agent: "codex", SessionID: "abc", Cwd: "/r"}, "codex resume abc"},
		{Target{Agent: "codex", SessionID: "abc", Cwd: "/r", Prompt: "read x"}, "codex resume abc 'read x'"},
		{Target{New: true, Title: "ONCALL", Model: "opus", Cwd: "/r", Prompt: "hi"}, "claude -n ONCALL --model opus hi"},
	}
	for _, c := range cases {
		if got := c.t.ShellCommand(); got != "cd /r && "+c.want {
			t.Errorf("got %q want %q", got, "cd /r && "+c.want)
		}
	}
}
