package claude

import (
	"strings"
	"testing"
)

const fixture = `{"type":"permission-mode","permissionMode":"auto","sessionId":"s1"}
{"type":"user","sessionId":"s1","timestamp":"2026-10-01T10:00:00Z","cwd":"/r","gitBranch":"main","isMeta":true,"message":{"role":"user","content":"<system-reminder>ignore</system-reminder>"}}
{"type":"user","sessionId":"s1","timestamp":"2026-10-01T10:00:01Z","cwd":"/r","gitBranch":"main","message":{"role":"user","content":"<command-name>/rename</command-name>"}}
{"type":"user","sessionId":"s1","timestamp":"2026-10-01T10:00:02Z","cwd":"/r","gitBranch":"main","message":{"role":"user","content":"fix the <pasted_content id=\"x\">flaky</pasted_content> test"}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-10-01T10:00:05Z","cwd":"/r","gitBranch":"feat/x","message":{"role":"assistant","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"hmm"},{"type":"tool_use","name":"Bash","input":{"command":"go test ./..."}},{"type":"text","text":"Fixed it."}]}}
{"type":"user","sessionId":"s1","timestamp":"2026-10-01T10:00:06Z","isSidechain":true,"message":{"role":"user","content":"subagent prompt"}}
{"type":"user","sessionId":"s1","timestamp":"2026-10-01T10:00:07Z","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}
{"type":"system","subtype":"away_summary","sessionId":"s1","timestamp":"2026-10-01T11:00:00Z","content":"All green now. (disable recaps in /config)"}
{"type":"ai-title","aiTitle":"Flaky ⑂","sessionId":"s1"}
{"type":"pr-link","prUrl":"https://github.com/a/b/pull/1","sessionId":"s1"}
{"type":"bridge-session","bridgeSessionId":"br1","sessionId":"s1"}
not json
`

func TestParserFixture(t *testing.T) {
	p := NewParser(&Session{}, 0)
	n, err := p.Feed(strings.NewReader(fixture + `{"type":"user","incomplete`))
	if err != nil {
		t.Fatal(err)
	}
	if int(n) != len(fixture) {
		t.Fatalf("consumed %d want %d (partial line must be left)", n, len(fixture))
	}
	s := p.S
	if s.ID != "s1" || s.NUser != 1 || s.NAssistant != 1 || s.NTools != 1 {
		t.Fatalf("counts: %+v", s)
	}
	if s.FirstPrompt != "fix the flaky test" {
		t.Errorf("first prompt %q", s.FirstPrompt)
	}
	if s.Recap != "All green now." {
		t.Errorf("recap %q", s.Recap)
	}
	if got := s.DisplayTitle(); got != "Flaky" {
		t.Errorf("title %q", got)
	}
	if strings.Join(s.Branches, ",") != "main,feat/x" {
		t.Errorf("branches %v", s.Branches)
	}
	if s.Remote != "br1" || len(s.PRs) != 1 || s.Models[0] != "claude-opus-5-5" {
		t.Errorf("meta %+v", s)
	}
	roles := ""
	for _, c := range p.Chunks {
		roles += string(c.Role[0])
	}
	if roles != "utar" {
		t.Errorf("chunk roles %q", roles)
	}
}

func TestChunkSplitsLongText(t *testing.T) {
	p := NewParser(&Session{}, 0)
	long := strings.Repeat("word ", 1000)
	p.emit(RoleAssistant, p.S.Started, long)
	if len(p.Chunks) < 3 {
		t.Fatalf("chunks=%d", len(p.Chunks))
	}
	total := 0
	for i, c := range p.Chunks {
		if len(c.Text) > maxChunk {
			t.Errorf("chunk %d too long: %d", i, len(c.Text))
		}
		if c.Seq != i {
			t.Errorf("seq %d at %d", c.Seq, i)
		}
		total += len(strings.Fields(c.Text))
	}
	if total != 1000 {
		t.Errorf("lost words: %d", total)
	}
}

func TestReadLive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/sessions/1.json", `{"pid":1,"sessionId":"a","status":"busy","updatedAt":5}`)
	writeFile(t, dir+"/sessions/2.json", `{"pid":2,"sessionId":"b","status":"idle"}`)
	writeFile(t, dir+"/sessions/3.json", `{"pid":3,"sessionId":"c","status":"busy"}`)
	live := ReadLive(dir, func(pid int) bool { return pid != 3 })
	if len(live) != 2 || live["a"].Status != Working || live["b"].Status != Idle {
		t.Fatalf("%+v", live)
	}
}

func TestAutoTitle(t *testing.T) {
	cases := map[string]string{
		"Priya Patel  [10:14 AM]\nhey team — the invoice":    "ask from Priya",
		"review https://github.com/acme/infra/pull/91":       "review infra#91",
		"can you make my zsh startup faster, it takes 900ms": "make my zsh startup faster, it",
		"why does `pnpm build` warn":                         "why does `pnpm build` warn",
	}
	for in, want := range cases {
		if got := AutoTitle(in); got != want {
			t.Errorf("AutoTitle(%q)=%q want %q", in, got, want)
		}
	}
}

const codexFixture = `{"timestamp":"2026-10-18T09:00:00Z","type":"session_meta","payload":{"id":"0199c1a2-7e3b-7d40-9a51-6f2b1c0e4a11","cwd":"/r","git":{"branch":"perf/x"}}}
{"timestamp":"2026-10-18T09:00:01Z","type":"turn_context","payload":{"cwd":"/r","model":"gpt-5-codex"}}
{"timestamp":"2026-10-18T09:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>x</environment_context>"}]}}
{"timestamp":"2026-10-18T09:00:03Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"speed up the export"}]}}
{"timestamp":"2026-10-18T09:00:04Z","type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"command\":[\"bash\",\"-lc\",\"rg -n export\"]}"}}
{"timestamp":"2026-10-18T09:00:05Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Streaming fixes it."}]}}
`

func TestCodexParser(t *testing.T) {
	p := NewParser(&Session{}, 0)
	p.Codex = true
	if _, err := p.Feed(strings.NewReader(codexFixture)); err != nil {
		t.Fatal(err)
	}
	s := p.S
	if s.Agent != "codex" || s.ID != "0199c1a2-7e3b-7d40-9a51-6f2b1c0e4a11" || s.Cwd != "/r" || s.NUser != 1 || s.NAssistant != 1 || s.NTools != 1 {
		t.Fatalf("%+v", s)
	}
	if s.Branches[0] != "perf/x" || s.Models[0] != "gpt-5-codex" || s.FirstPrompt != "speed up the export" {
		t.Fatalf("%+v", s)
	}
	if len(p.Chunks) != 3 || p.Chunks[1].Text != "shell: rg -n export" {
		t.Fatalf("chunks %+v", p.Chunks)
	}
}
