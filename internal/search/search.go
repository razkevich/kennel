// Package search parses kennel queries and ranks pups by metadata and content.
package search

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/razkevich/kennel/internal/claude"
	"github.com/razkevich/kennel/internal/pack"
	"github.com/razkevich/kennel/internal/store"
	"github.com/sahilm/fuzzy"
)

type Query struct {
	Terms   []string
	Is      []string
	Repo    string
	Kennel  string
	Branch  string
	HasPR   bool
	Since   time.Duration
	SinceAt time.Time
	Raw     string
}

var sinceRe = regexp.MustCompile(`^(\d+)([hdwm])$`)

func Parse(raw string) Query {
	q := Query{Raw: raw}
	for _, f := range splitFields(raw) {
		key, val, ok := strings.Cut(f, ":")
		if ok && val != "" {
			switch strings.ToLower(key) {
			case "is":
				q.Is = append(q.Is, strings.ToLower(val))
				continue
			case "repo":
				q.Repo = strings.ToLower(val)
				continue
			case "kennel", "k":
				q.Kennel = strings.ToLower(val)
				continue
			case "branch", "b":
				q.Branch = strings.ToLower(val)
				continue
			case "has":
				if strings.EqualFold(val, "pr") {
					q.HasPR = true
					continue
				}
			case "since":
				if m := sinceRe.FindStringSubmatch(strings.ToLower(val)); m != nil {
					n, _ := strconv.Atoi(m[1])
					unit := map[string]time.Duration{"h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour, "m": 30 * 24 * time.Hour}[m[2]]
					q.Since = time.Duration(n) * unit
					continue
				}
				if t, err := time.Parse("2006-01-02", val); err == nil {
					q.SinceAt = t
					continue
				}
			}
		}
		q.Terms = append(q.Terms, strings.Trim(f, `"`))
	}
	return q
}

func splitFields(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			cur.WriteRune(r)
		case unicode.IsSpace(r) && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func (q Query) Text() string { return strings.Join(q.Terms, " ") }

func (q Query) Empty() bool {
	return len(q.Terms) == 0 && len(q.Is) == 0 && q.Repo == "" && q.Kennel == "" && q.Branch == "" && !q.HasPR && q.Since == 0 && q.SinceAt.IsZero()
}

// Filter applies the non-text constraints.
func (q Query) Filter(p *pack.Pup, now time.Time) bool {
	for _, is := range q.Is {
		switch is {
		case "live":
			if !p.Status.Live() {
				return false
			}
		case "working":
			if p.Status != claude.Working {
				return false
			}
		case "waiting", "blocked":
			if p.Status != claude.Waiting {
				return false
			}
		case "starred", "star":
			if !p.Meta.Starred {
				return false
			}
		case "archived":
			if !p.Meta.Archived {
				return false
			}
		case "unread":
			if !p.Unread() {
				return false
			}
		case "titled", "named":
			if p.CustomTitle == "" && p.Meta.Alias == "" {
				return false
			}
		}
	}
	if q.Repo != "" && !strings.Contains(strings.ToLower(p.Repo()), q.Repo) {
		return false
	}
	if q.Kennel != "" && (p.Kennel == nil || !strings.Contains(strings.ToLower(p.Kennel.Name), q.Kennel)) {
		return false
	}
	if q.Branch != "" && !containsAny(p.Branches, q.Branch) {
		return false
	}
	if q.HasPR && len(p.PRs) == 0 {
		return false
	}
	if q.Since > 0 && now.Sub(p.Updated) > q.Since {
		return false
	}
	if !q.SinceAt.IsZero() && p.Updated.Before(q.SinceAt) {
		return false
	}
	return true
}

func containsAny(xs []string, sub string) bool {
	for _, x := range xs {
		if strings.Contains(strings.ToLower(x), sub) {
			return true
		}
	}
	return false
}

// FTSMatch turns free-text terms into a safe FTS5 expression (prefix AND).
func FTSMatch(terms []string) string {
	var parts []string
	for _, t := range terms {
		for _, w := range tokenize(t) {
			parts = append(parts, `"`+w+`"*`)
		}
	}
	return strings.Join(parts, " AND ")
}

// FTSAny is like FTSMatch but ORs the words; used by Ask for recall.
func FTSAny(words []string) string {
	var parts []string
	seen := map[string]bool{}
	for _, t := range words {
		for _, w := range tokenize(t) {
			if !seen[w] && !stop[w] {
				seen[w] = true
				parts = append(parts, `"`+w+`"*`)
			}
		}
	}
	return strings.Join(parts, " OR ")
}

func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
}

var stop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an the and or but if of to in on at for with from by about into over is are was were be been being
		do does did done have has had i me my we our you your it its this that these those there here what which who whom when where why how
		can could should would will shall may might must not no yes so than then too very just also any some all each every
		session sessions claude did we us let lets let's please thing things stuff get got make made`) {
		stop[w] = true
	}
}

type Result struct {
	Pup   *pack.Pup
	Score float64
	Hits  []store.Hit
}

// Rank scores pups against q. hits are FTS matches for q's terms (may be nil
// when only metadata search has run so far).
func Rank(pups []*pack.Pup, q Query, hits []store.Hit, now time.Time) []Result {
	byID := map[string][]store.Hit{}
	ftsRank := map[string]int{}
	for _, h := range hits {
		if _, ok := ftsRank[h.SessionID]; !ok {
			ftsRank[h.SessionID] = len(ftsRank)
		}
		if len(byID[h.SessionID]) < 3 {
			byID[h.SessionID] = append(byID[h.SessionID], h)
		}
	}
	terms := make([]string, 0, len(q.Terms))
	for _, t := range q.Terms {
		terms = append(terms, strings.ToLower(t))
	}
	var titles []string
	for _, p := range pups {
		titles = append(titles, p.Title())
	}
	fuzzyScore := map[int]int{}
	if len(terms) > 0 {
		for _, m := range fuzzy.Find(q.Text(), titles) {
			fuzzyScore[m.Index] = m.Score
		}
	}

	var out []Result
	for i, p := range pups {
		if !q.Filter(p, now) {
			continue
		}
		score := 0.0
		if len(terms) > 0 {
			ms, ok := metaScore(p, terms)
			if fs, fok := fuzzyScore[i]; fok && !ok {
				ms, ok = 3+float64(min(fs, 40))/20, true
			}
			r, inFTS := ftsRank[p.ID]
			if !ok && !inFTS {
				continue
			}
			score = ms
			if inFTS {
				score += 8 / (1 + 0.25*float64(r))
			}
		}
		if p.Status == claude.Waiting {
			score += 0.5
		}
		out = append(out, Result{Pup: p, Score: score, Hits: byID[p.ID]})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Score != out[b].Score {
			return out[a].Score > out[b].Score
		}
		return out[a].Pup.Updated.After(out[b].Pup.Updated)
	})
	return out
}

// metaScore requires every term to appear in some metadata field.
func metaScore(p *pack.Pup, terms []string) (float64, bool) {
	title := strings.ToLower(p.Title())
	fields := []struct {
		text   string
		weight float64
	}{
		{title, 10},
		{strings.ToLower(p.Repo() + " " + strings.Join(p.Branches, " ") + " " + p.Worktree), 4},
		{strings.ToLower(strings.Join(p.PRs, " ")), 4},
		{strings.ToLower(p.FirstPrompt + " " + p.LastPrompt + " " + p.Recap), 2},
	}
	total := 0.0
	for _, t := range terms {
		best := 0.0
		if strings.HasPrefix(p.ID, t) && len(t) >= 4 {
			best = 20
		}
		for _, f := range fields {
			if strings.Contains(f.text, t) && f.weight > best {
				best = f.weight
				if f.weight == 10 && strings.HasPrefix(title, t) {
					best += 3
				}
			}
		}
		if best == 0 {
			return 0, false
		}
		total += best
	}
	return total, true
}
