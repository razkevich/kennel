package things

import "testing"

func TestInfer(t *testing.T) {
	cases := map[string]Kind{
		"btop":                                  Monitor,
		"k9s --context prod":                    Monitor,
		"pnpm dev":                              Server,
		"kubectl logs -f deploy/api":            Logs,
		"tail -f /var/log/system.log":           Logs,
		"terraform plan -out tf.plan":           Job,
		"go test ./...":                         Job,
		"kubectl port-forward svc/grafana 3000": Server,
		"python notebook.py":                    Other,
	}
	for cmd, want := range cases {
		if got := Infer(cmd); got != want {
			t.Errorf("Infer(%q)=%s want %s", cmd, got, want)
		}
	}
}

func TestResolve(t *testing.T) {
	saved := []Thing{
		{ID: 1, Name: "dev server", Command: "pnpm dev", Cwd: "/code/atlas"},
		{ID: 2, Name: "cluster", Command: "k9s --context prod"},
		{ID: 3, Name: "nightly sync", Command: "rsync -a /data /backup"},
	}
	panes := ParseHerdrPanes([]byte(`{"result":{"panes":[
		{"pane_id":"w1:p1","tab_id":"w1:t1","agent":"claude","terminal_title_stripped":"KALSHI"},
		{"pane_id":"w1:p2","tab_id":"w1:t2","terminal_title_stripped":"pnpm dev","foreground_cwd":"/code/atlas"},
		{"pane_id":"w1:p3","tab_id":"w1:t3","terminal_title_stripped":"btop"},
		{"pane_id":"w1:p4","tab_id":"w1:t4","terminal_title_stripped":"zsh"},
		{"pane_id":"w1:p5","tab_id":"w1:t5","terminal_title_stripped":"~/code/atlas"},
		{"pane_id":"w1:p6","tab_id":"w1:t6","terminal_title_stripped":"k9s"}]}}`))
	procs := ParsePS([]byte("  101 /usr/bin/rsync -a /data /backup\n  202 /bin/zsh\n"))
	got := Resolve(saved, panes, procs)
	if len(got) != 4 {
		t.Fatalf("want 3 saved + btop, got %+v", got)
	}
	if !got[0].Running || got[0].Pane != "w1:p2" {
		t.Errorf("dev server: %+v", got[0])
	}
	if !got[1].Running || got[1].Pane != "w1:p6" {
		t.Errorf("k9s should match by program name: %+v", got[1])
	}
	if !got[2].Running || got[2].PID != 101 {
		t.Errorf("rsync should match a process: %+v", got[2])
	}
	if got[3].Saved || got[3].Name != "btop" || got[3].Kind != Monitor || got[3].Pane != "w1:p3" {
		t.Errorf("discovered: %+v", got[3])
	}
}
