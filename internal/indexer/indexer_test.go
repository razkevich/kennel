package indexer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/razkevich/kennel/internal/demo"
	"github.com/razkevich/kennel/internal/search"
	"github.com/razkevich/kennel/internal/store"
)

var now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func setup(t *testing.T) (*Indexer, string) {
	t.Helper()
	dir := t.TempDir()
	cdir := filepath.Join(dir, "claude")
	if err := demo.Generate(cdir, now); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "kennel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &Indexer{Store: st, ClaudeDir: cdir}, cdir
}

func TestSyncIndexesDemo(t *testing.T) {
	ix, _ := setup(t)
	p, err := ix.Sync(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Changed != len(demo.Specs()) {
		t.Fatalf("changed=%d want %d", p.Changed, len(demo.Specs()))
	}
	sessions, err := ix.Store.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	var co *struct{}
	_ = co
	found := false
	for _, s := range sessions {
		if s.CustomTitle == "CHECKOUT-V2" {
			found = true
			if s.NUser != 3 || s.NAssistant != 3 || s.NTools != 4 {
				t.Errorf("counts user=%d asst=%d tools=%d", s.NUser, s.NAssistant, s.NTools)
			}
			if len(s.PRs) != 1 || !strings.HasSuffix(s.PRs[0], "/412") {
				t.Errorf("prs=%v", s.PRs)
			}
			if s.Color != "green" || s.Recap == "" || s.Cwd != "/Users/demo/code/atlas" {
				t.Errorf("meta: %+v", s)
			}
			if !strings.Contains(s.FirstPrompt, "rebuild the checkout flow") {
				t.Errorf("first prompt %q", s.FirstPrompt)
			}
		}
	}
	if !found {
		t.Fatal("CHECKOUT-V2 not indexed")
	}

	again, err := ix.Sync(context.Background(), nil)
	if err != nil || again.Total != 0 {
		t.Fatalf("second sync should be a no-op: %+v %v", again, err)
	}
}

func TestSyncAppendsIncrementally(t *testing.T) {
	ix, cdir := setup(t)
	if _, err := ix.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(cdir, "projects", "*", "*.jsonl"))
	target := files[0]
	sess, _, _ := mustParse(t, target)
	_, before := ix.Store.Count()

	f, _ := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0)
	line := `{"type":"user","sessionId":"` + sess + `","timestamp":"2026-10-18T12:30:00Z","cwd":"/x","message":{"role":"user","content":"zebrafish migration plan please"}}`
	f.WriteString(line + "\n" + `{"type":"custom-title","customTitle":"ZEBRA","sessionId":"` + sess + `"}` + "\n" + `{"type":"user","partial`)
	f.Close()

	p, err := ix.Sync(context.Background(), nil)
	if err != nil || p.Changed != 1 {
		t.Fatalf("sync: %+v %v", p, err)
	}
	_, after := ix.Store.Count()
	if after != before+1 {
		t.Fatalf("chunks before=%d after=%d, want +1 (partial line must wait)", before, after)
	}
	hits, err := ix.Store.Search(search.FTSMatch([]string{"zebrafish"}), 5)
	if err != nil || len(hits) != 1 || hits[0].SessionID != sess {
		t.Fatalf("hits=%v err=%v", hits, err)
	}
	all, _ := ix.Store.Sessions()
	for _, s := range all {
		if s.ID == sess && (s.CustomTitle != "ZEBRA" || s.LastPrompt != "zebrafish migration plan please") {
			t.Fatalf("session not updated: %q %q", s.CustomTitle, s.LastPrompt)
		}
	}
}

func TestSyncDropsDeletedFiles(t *testing.T) {
	ix, cdir := setup(t)
	ix.Sync(context.Background(), nil)
	files, _ := filepath.Glob(filepath.Join(cdir, "projects", "*", "*.jsonl"))
	os.Remove(files[0])
	ix.Sync(context.Background(), nil)
	n, _ := ix.Store.Count()
	if n != len(demo.Specs())-1 {
		t.Fatalf("sessions=%d", n)
	}
}

func mustParse(t *testing.T, path string) (string, int, error) {
	t.Helper()
	b, _ := os.ReadFile(path)
	i := strings.Index(string(b), `"sessionId":"`)
	id := string(b[i+13 : i+13+36])
	return strings.TrimRight(id, `"`), 0, nil
}

// Two kennel processes (say the TUI and `kennel map`) syncing the same append
// must not index the new lines twice.
func TestTwoProcessesDoNotDuplicateChunks(t *testing.T) {
	ix, cdir := setup(t)
	if _, err := ix.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	other, err := store.Open(filepath.Join(filepath.Dir(cdir), "kennel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ix2 := &Indexer{Store: other, ClaudeDir: cdir}

	files, _ := filepath.Glob(filepath.Join(cdir, "projects", "*", "*.jsonl"))
	f, _ := os.OpenFile(files[0], os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"user","sessionId":"x","timestamp":"2026-10-18T12:30:00Z","message":{"role":"user","content":"walrus migration"}}` + "\n")
	f.Close()

	done := make(chan error, 2)
	go func() { _, err := ix.Sync(context.Background(), nil); done <- err }()
	go func() { _, err := ix2.Sync(context.Background(), nil); done <- err }()
	<-done
	<-done
	hits, err := ix.Store.Search(`"walrus"*`, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("appended line indexed %d times", len(hits))
	}
}

func TestSessionIDComesFromFileName(t *testing.T) {
	ix, cdir := setup(t)
	files, _ := filepath.Glob(filepath.Join(cdir, "projects", "*", "*.jsonl"))
	b, _ := os.ReadFile(files[0])
	twin := filepath.Join(filepath.Dir(files[0]), "11111111-2222-4333-8444-555555555555.jsonl")
	os.WriteFile(twin, b, 0o644) // a fork whose lines still carry the parent's sessionId
	if _, err := ix.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	all, _ := ix.Store.Sessions()
	ids := map[string]string{}
	for _, s := range all {
		ids[s.ID] = s.Path
	}
	if ids["11111111-2222-4333-8444-555555555555"] != twin || ids[strings.TrimSuffix(filepath.Base(files[0]), ".jsonl")] != files[0] {
		t.Fatalf("both files should be indexed under their own ids: %v", ids)
	}
	again, _ := ix.Sync(context.Background(), nil)
	if again.Total != 0 {
		t.Fatalf("twins must not keep re-indexing each other: %+v", again)
	}
}
