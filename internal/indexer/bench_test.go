package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/razkevich/kennel/internal/store"
)

// TestBigSynthetic indexes a large synthetic corpus (KENNEL_BIG=<MB>) and reports timings.
func TestBigSynthetic(t *testing.T) {
	mb := os.Getenv("KENNEL_BIG")
	if mb == "" {
		t.Skip("set KENNEL_BIG=<total MB> to run")
	}
	var total int
	fmt.Sscan(mb, &total)
	dir := t.TempDir()
	words := strings.Fields("the deploy failed because postgres replication lag spiked while kubectl rollout restarted pods and terraform plan showed drift in the egress firewall rules we should check grafana dashboards for p99 latency and fix the flaky test in auth spec")
	rng := rand.New(rand.NewSource(1))
	sentence := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(words[rng.Intn(len(words))])
			b.WriteByte(' ')
		}
		return b.String()
	}
	proj := filepath.Join(dir, "claude", "projects", "-x-repo")
	os.MkdirAll(proj, 0o755)
	written := 0
	for s := 0; written < total<<20; s++ {
		f, _ := os.Create(filepath.Join(proj, fmt.Sprintf("%08d-0000-4000-8000-000000000000.jsonl", s)))
		id := fmt.Sprintf("%08d-0000-4000-8000-000000000000", s)
		ts := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		for turn := 0; turn < 150; turn++ {
			ts = ts.Add(time.Minute)
			u, _ := json.Marshal(map[string]any{"type": "user", "sessionId": id, "timestamp": ts.Format(time.RFC3339), "cwd": "/x/repo", "message": map[string]any{"role": "user", "content": sentence(30)}})
			big := strings.Repeat(sentence(50), 40) // tool output noise like real transcripts
			tr, _ := json.Marshal(map[string]any{"type": "user", "sessionId": id, "timestamp": ts.Format(time.RFC3339), "message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "content": big}}}})
			a, _ := json.Marshal(map[string]any{"type": "assistant", "sessionId": id, "timestamp": ts.Format(time.RFC3339), "message": map[string]any{"role": "assistant", "model": "claude-opus-5-5", "content": []any{
				map[string]any{"type": "tool_use", "name": "Bash", "input": map[string]any{"command": "kubectl get pods"}}, map[string]any{"type": "text", "text": sentence(120)}}}})
			n, _ := f.Write(append(append(append(append(append(u, '\n'), tr...), '\n'), a...), '\n'))
			written += n
		}
		f.Close()
	}
	st, err := store.Open(filepath.Join(dir, "kennel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ix := &Indexer{Store: st, ClaudeDir: filepath.Join(dir, "claude")}
	start := time.Now()
	p, err := ix.Sync(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	full := time.Since(start)
	start = time.Now()
	ix.Sync(context.Background(), nil)
	noop := time.Since(start)
	start = time.Now()
	hits, _ := st.Search(`"replication"* AND "grafana"*`, 300)
	q := time.Since(start)
	start = time.Now()
	sessions, _ := st.Sessions()
	load := time.Since(start)
	t.Logf("%d MB in %d files: full index %s, no-op sync %s, search %s (%d hits), load %d sessions %s",
		written>>20, p.Changed, full.Round(time.Millisecond), noop.Round(time.Millisecond), q.Round(time.Millisecond), len(hits), len(sessions), load.Round(time.Millisecond))
}
