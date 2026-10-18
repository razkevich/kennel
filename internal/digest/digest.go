// Package digest summarizes what happened across sessions in a time window.
package digest

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/pack"
)

type Digest struct {
	Since     time.Time
	Touched   []*pack.Pup
	Started   []*pack.Pup
	Shipped   []*pack.Pup
	Waiting   []*pack.Pup
	Working   []*pack.Pup
	Rugged    []*pack.Pup
	Artifacts int
	Cost      float64
	Prompts   int
	Repos     []RepoCount
}

type RepoCount struct {
	Repo  string
	Count int
}

func Build(pups []*pack.Pup, now time.Time, window time.Duration) Digest {
	d := Digest{Since: now.Add(-window)}
	repos := map[string]int{}
	rug := pack.SmartByKey("rug")
	for _, p := range pups {
		if p.Empty() {
			continue
		}
		switch p.Status {
		case claude.Waiting:
			d.Waiting = append(d.Waiting, p)
		case claude.Working:
			d.Working = append(d.Working, p)
		}
		if p.Updated.Before(d.Since) {
			// Fell under the rug during this window: last touched just past the rug's threshold.
			if rug.Match(p, now) && now.Sub(p.Updated) < 7*24*time.Hour+window {
				d.Rugged = append(d.Rugged, p)
			}
			continue
		}
		d.Touched = append(d.Touched, p)
		if !p.Started.Before(d.Since) {
			d.Started = append(d.Started, p)
		}
		if len(p.PRs) > 0 {
			d.Shipped = append(d.Shipped, p)
		}
		for _, a := range p.Artifacts {
			if !a.At.Before(d.Since) {
				d.Artifacts++
			}
		}
		d.Cost += p.Cost
		d.Prompts += p.NUser
		repos[p.Repo()]++
	}
	for r, c := range repos {
		d.Repos = append(d.Repos, RepoCount{r, c})
	}
	sort.Slice(d.Repos, func(i, j int) bool {
		if d.Repos[i].Count != d.Repos[j].Count {
			return d.Repos[i].Count > d.Repos[j].Count
		}
		return d.Repos[i].Repo < d.Repos[j].Repo
	})
	return d
}

func titles(ps []*pack.Pup, max int) string {
	var t []string
	for i, p := range ps {
		if i == max {
			t = append(t, fmt.Sprintf("+%d more", len(ps)-max))
			break
		}
		t = append(t, p.Title())
	}
	return strings.Join(t, ", ")
}

// Lines renders the digest as short, plain lines.
func (d Digest) Lines() []string {
	var out []string
	add := func(icon, format string, args ...any) { out = append(out, icon+" "+fmt.Sprintf(format, args...)) }
	add("🐾", "%d pups touched, %d new, %d prompts", len(d.Touched), len(d.Started), d.Prompts)
	if len(d.Repos) > 0 {
		var r []string
		for i, rc := range d.Repos {
			if i == 4 {
				break
			}
			r = append(r, fmt.Sprintf("%s (%d)", rc.Repo, rc.Count))
		}
		add("📁", "busiest: %s", strings.Join(r, ", "))
	}
	if len(d.Shipped) > 0 {
		add("🚀", "with PRs: %s", titles(d.Shipped, 6))
	}
	if len(d.Waiting) > 0 {
		add("❗", "waiting on you: %s", titles(d.Waiting, 6))
	}
	if len(d.Working) > 0 {
		add("⚡", "still working: %s", titles(d.Working, 6))
	}
	if len(d.Rugged) > 0 {
		add("🧹", "slid under the rug: %s", titles(d.Rugged, 6))
	}
	if d.Artifacts > 0 {
		add("🎨", "%d artifacts published", d.Artifacts)
	}
	if d.Cost > 0 {
		add("💸", "about $%.2f across touched sessions", d.Cost)
	}
	return out
}
