package digest

import (
	"strings"
	"testing"
	"time"

	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/pack"
)

func TestDigest(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	mk := func(title string, ago time.Duration, n int, st claude.Status, prs ...string) *pack.Pup {
		return &pack.Pup{Session: &claude.Session{ID: title, CustomTitle: title, Cwd: "/x/atlas", NUser: n, PRs: prs,
			Started: now.Add(-ago - time.Hour), Updated: now.Add(-ago), Cost: 1}, Status: st}
	}
	pups := []*pack.Pup{
		mk("A", time.Hour, 3, claude.Waiting),
		mk("B", 2*time.Hour, 5, claude.Working, "https://x/pull/1"),
		mk("OLD", 7*24*time.Hour+3*time.Hour, 9, claude.Offline),
		mk("ANCIENT", 40*24*time.Hour, 9, claude.Offline),
	}
	d := Build(pups, now, 24*time.Hour)
	if len(d.Touched) != 2 || len(d.Shipped) != 1 || len(d.Waiting) != 1 || len(d.Rugged) != 1 || d.Rugged[0].Title() != "OLD" {
		t.Fatalf("%+v", d)
	}
	text := strings.Join(d.Lines(), "\n")
	for _, want := range []string{"2 pups touched", "with PRs: B", "waiting on you: A", "under the rug: OLD", "busiest: atlas (2)"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}
