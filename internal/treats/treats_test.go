package treats

import (
	"strings"
	"testing"
)

func TestInferAndPrompt(t *testing.T) {
	cases := map[string]Kind{
		"/Users/me/docs/runbook.md":                      File,
		"~/notes/plan.md":                                File,
		"https://github.com/acme/atlas/pull/412":         URL,
		"https://acme.slack.com/archives/C01ABC/p123456": Slack,
		"#care-internal":                                 Slack,
		"INF-2141":                                       Ticket,
		"remember: prod deploys freeze on Fridays":       Text,
	}
	for v, want := range cases {
		if got := Infer(v); got != want {
			t.Errorf("Infer(%q)=%s want %s", v, got, want)
		}
	}
	tr := New("", "/Users/me/docs/runbook.md")
	if tr.Name != "runbook.md" || !strings.Contains(tr.Prompt("focus on rollback"), "Please read /Users/me/docs/runbook.md") ||
		!strings.HasSuffix(tr.Prompt("focus on rollback"), "focus on rollback") {
		t.Fatalf("%+v %q", tr, tr.Prompt("focus on rollback"))
	}
	if p := New("", "#care-internal").Prompt(""); !strings.Contains(p, "Slack channel #care-internal") {
		t.Fatal(p)
	}
}
