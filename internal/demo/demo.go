// Package demo writes a synthetic ~/.claude tree so kennel can be developed,
// tested and screenshotted without touching anyone's real sessions.
package demo

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/razkevich/kennel/internal/things"
)

// Marker is created in generated dirs; kennel treats every registered pid there as alive.
const Marker = "kennel-demo"

// PanesFile stands in for `herdr pane list` in demo mode.
const PanesFile = "kennel-demo-panes.json"

// Things are saved demo things; the first three appear running via PanesFile.
func Things() []things.Thing {
	return []things.Thing{
		{Name: "atlas dev", Command: "pnpm dev", Cwd: "/Users/demo/code/atlas", Kind: things.Server},
		{Name: "prod pods", Command: "k9s --context prod", Cwd: "/Users/demo/code/infra", Kind: things.Monitor},
		{Name: "api logs", Command: "kubectl logs -f deploy/api", Cwd: "/Users/demo/code/infra", Kind: things.Logs},
		{Name: "tf plan", Command: "terraform plan -out tf.plan", Cwd: "/Users/demo/code/infra", Kind: things.Job},
		{Name: "storybook", Command: "pnpm storybook", Cwd: "/Users/demo/code/mobile", Kind: things.Server},
	}
}

const demoPanes = `{"result":{"panes":[
 {"pane_id":"w1:p2","tab_id":"w1:t2","terminal_title_stripped":"pnpm dev","foreground_cwd":"/Users/demo/code/atlas"},
 {"pane_id":"w1:p3","tab_id":"w1:t3","terminal_title_stripped":"k9s","foreground_cwd":"/Users/demo/code/infra"},
 {"pane_id":"w1:p4","tab_id":"w1:t4","terminal_title_stripped":"kubectl logs -f deploy/api","foreground_cwd":"/Users/demo/code/infra"},
 {"pane_id":"w1:p5","tab_id":"w1:t5","terminal_title_stripped":"btop","foreground_cwd":"/Users/demo"}]}}`

type Turn struct {
	User      string
	Assistant string
	Tools     []string
}

type Spec struct {
	ID        string
	Title     string
	Repo      string
	Branches  []string
	PR        string
	Color     string
	Recap     string
	Ago       time.Duration
	Span      time.Duration
	Turns     []Turn
	Live      string
	Worktree  string
	AITitle   string
	Model     string
	CostUSD   float64
	Artifacts []string
	Crons     []string
	Remote    bool
	Job       string
}

func Specs() []Spec {
	return []Spec{
		{Title: "CHECKOUT-V2", Repo: "atlas", Branches: []string{"feat/checkout-v2"}, PR: "https://github.com/acme/atlas/pull/412", Color: "green",
			Recap:     "Checkout v2 is behind the flag and passing e2e. Next: wire Apple Pay sheet and ask design for the empty-cart state.",
			Artifacts: []string{"https://claude.ai/artifact/demo-checkout-flow Checkout v2 flow map"},
			Ago:       4 * time.Minute, Span: 5 * time.Hour, Live: "busy", Model: "claude-opus-5-5", CostUSD: 14.20,
			Turns: []Turn{
				{"let's rebuild the checkout flow as a single page, keep the old one behind a flag", "I'll split this into three steps: a `CheckoutV2` route behind `flags.checkoutV2`, a shared cart summary, and the payment step. Starting with the route and flag plumbing.", []string{"Bash: rg -n \"checkout\" src/routes", "Edit: src/routes/checkout/v2.tsx"}},
				{"the stripe elements iframe flickers on mobile safari", "The flicker comes from remounting `<PaymentElement>` whenever the cart total changes. Memoizing the `options` object and keying the element by client secret fixes it.", []string{"Edit: src/checkout/PaymentStep.tsx", "Bash: pnpm test checkout"}},
				{"add apple pay too", "Apple Pay needs the domain association file served from `/.well-known/`. I added it and the payment request button; it shows only when `canMakePayment()` resolves.", nil},
			}},
		{Title: "FLAKY-CI", Remote: true, Repo: "atlas", Branches: []string{"fix/flaky-playwright"}, PR: "https://github.com/acme/atlas/pull/409", Color: "yellow",
			Recap: "Found the race: the login fixture reuses storage state across workers. Waiting on you to approve the retry-budget change.",
			Ago:   9 * time.Minute, Span: 2 * time.Hour, Live: "waiting", Model: "claude-sonnet-5-5", CostUSD: 3.10,
			Turns: []Turn{
				{"playwright tests are flaky again on CI, ~1 in 5 runs fail in auth.spec", "Looking at the last 20 runs: failures cluster on worker 3 and always in tests that run after `logout.spec`. That points at shared storage state.", []string{"Bash: gh run list --workflow e2e --limit 20", "Read: tests/fixtures/auth.ts"}},
				{"can we just retry?", "Retries would hide it. Better: give each worker its own storage state file keyed by `workerIndex`. I can also cap retries at 1 as a safety net — want me to apply both?", nil},
			}},
		{Title: "PG-UPGRADE", Repo: "infra", Branches: []string{"ops/pg16"}, PR: "https://github.com/acme/infra/pull/88", Color: "blue",
			Recap: "Staging is on Postgres 16 with logical replication healthy. Prod cutover is scheduled; runbook is in docs/pg16.md.",
			Ago:   2 * time.Hour, Span: 26 * time.Hour, Live: "idle", Model: "claude-opus-5-5", CostUSD: 22.75, Artifacts: []string{"https://claude.ai/artifact/demo-pg16-runbook PG16 cutover runbook", "https://claude.ai/artifact/demo-pg16-lag Replication lag dashboard"}, Remote: true,
			Turns: []Turn{
				{"plan the postgres 15 -> 16 upgrade with minimal downtime", "Plan: logical replication to a fresh 16 cluster, verify row counts and lag, then a 30-second write freeze for the cutover. Rollback keeps the 15 primary warm for 24h.", []string{"Read: terraform/rds.tf", "Write: docs/pg16.md"}},
				{"what about extensions? we use pg_partman and pgvector", "Both have 16-compatible releases. pg_partman needs its background worker re-registered after the upgrade; pgvector indexes must be rebuilt (`REINDEX INDEX CONCURRENTLY`).", nil},
				{"ok run the staging dry run", "Staging cutover done in 41s of write freeze. Replication lag peaked at 3s. Row counts match on all 212 tables.", []string{"Bash: ./scripts/pg16-cutover.sh staging"}},
			}},
		{Title: "RATE-LIMITS", Repo: "atlas", Branches: []string{"feat/rate-limits"}, Color: "purple",
			Artifacts: []string{"https://claude.ai/artifact/demo-ratelimit-design Rate limiter design"},
			Recap:     "Token-bucket limiter merged for the public API. Follow-up: per-tenant overrides from the admin panel.",
			Ago:       26 * time.Hour, Span: 3 * time.Hour, Model: "claude-sonnet-5-5", CostUSD: 4.80,
			Turns: []Turn{
				{"add rate limiting to the public api, redis backed", "Using a token bucket in Redis via a single Lua script so check-and-decrement is atomic. Limits come from the API key's plan.", []string{"Write: src/api/ratelimit.ts", "Write: src/api/ratelimit.lua"}},
				{"return proper headers", "Responses now include `RateLimit-Limit`, `RateLimit-Remaining` and `RateLimit-Reset`, plus `Retry-After` on 429.", nil},
			}},
		{Title: "DARK-MODE", Repo: "mobile", Branches: []string{"feat/dark-mode", "fix/dark-mode-contrast"}, PR: "https://github.com/acme/mobile/pull/131", Color: "cyan",
			Ago: 50 * time.Hour, Span: 4 * time.Hour, Model: "claude-sonnet-5-5", CostUSD: 2.30,
			Turns: []Turn{
				{"implement dark mode using the design tokens from figma", "I mapped the Figma tokens to a `ThemeProvider` with light and dark palettes; components read colors through `useTheme()`.", []string{"Write: src/theme/tokens.ts"}},
				{"contrast on the secondary buttons is too low", "Bumped `secondary.fg` in dark mode from #8A8F98 to #B4BAC4 — contrast is now 7.1:1 on the card background.", nil},
			}},
		{Title: "ONBOARDING", Repo: "atlas", Branches: []string{"feat/onboarding-tour"}, Color: "orange", Live: "idle",
			Recap: "Tour steps are done; copy review pending from Priya.",
			Ago:   35 * time.Minute, Span: 90 * time.Minute, Model: "claude-opus-5-5", CostUSD: 1.95,
			Turns: []Turn{
				{"build a 4-step onboarding tour for new workspaces", "Built with a small `Tour` component and a persisted `seenTourAt` on the user. Steps: create project, invite team, connect repo, first deploy.", nil},
			}},
		{Title: "INCIDENT-0612", Repo: "infra", Branches: []string{"main"}, Color: "red",
			Recap:     "Root cause: connection pool exhaustion after the 2.14 deploy doubled worker count. Postmortem drafted.",
			Artifacts: []string{"https://claude.ai/artifact/demo-0612-timeline Incident 0612 timeline", "https://claude.ai/artifact/demo-0612-pool Pool saturation chart"},
			Ago:       6 * 24 * time.Hour, Span: 7 * time.Hour, Model: "claude-opus-5-5", CostUSD: 9.40,
			Turns: []Turn{
				{"api p99 is at 8s and climbing, help me find out why", "Pool wait time spikes at 14:02, right after the 2.14 rollout. Worker count went from 8 to 16 per pod but `max_connections` per pod stayed at 20 — workers now queue for connections.", []string{"Bash: kubectl top pods -n api", "Bash: kubectl logs deploy/api --since=30m | rg 'pool timeout'"}},
				{"write the postmortem", "Drafted in docs/postmortems/0612.md with timeline, impact (38 min degraded), root cause and three action items.", []string{"Write: docs/postmortems/0612.md"}},
			}},
		{Repo: "atlas", Branches: []string{"main"}, Ago: 3 * time.Hour, Span: 10 * time.Minute, Model: "claude-haiku-5-5", CostUSD: 0.05,
			Turns: []Turn{{"why does `pnpm build` warn about duplicate react versions", "Two copies: 18.3.1 from the app and 18.2.0 pulled in by `@acme/ui`. Adding a `pnpm.overrides` entry dedupes it.", nil}}},
		{Repo: "infra", Branches: []string{"main"}, Ago: 5 * time.Hour, Span: 20 * time.Minute, Model: "claude-sonnet-5-5", CostUSD: 0.40,
			Turns: []Turn{{"review https://github.com/acme/infra/pull/91", "Mostly good. One issue: the new IAM role grants `s3:*` on the bucket; scope it to `GetObject`/`PutObject` on the `exports/` prefix.", []string{"Bash: gh pr diff 91"}}}},
		{Title: "SEARCH-RELEVANCE", Repo: "atlas", Branches: []string{"exp/hybrid-search"}, Color: "pink", AITitle: "Hybrid search experiment",
			Job:       "done | Hybrid retrieval beats BM25 by 0.13 NDCG@10; report written to eval/REPORT.md.",
			Recap:     "BM25 + embeddings hybrid improves NDCG@10 from 0.61 to 0.74 on the eval set. Needs a latency budget decision.",
			Artifacts: []string{"https://claude.ai/artifact/demo-search-eval Search relevance eval report"},
			Ago:       3 * 24 * time.Hour, Span: 6 * time.Hour, Model: "claude-opus-5-5", CostUSD: 11.00,
			Turns: []Turn{
				{"our product search is bad for typos and synonyms. try hybrid bm25 + embeddings", "Built an offline eval with 300 labeled queries. BM25 alone: NDCG@10 0.61. Hybrid with reciprocal rank fusion: 0.74. Typos improve the most.", []string{"Bash: python eval/run.py --mode hybrid"}},
				{"what's the latency cost", "p50 +18ms, p95 +41ms with the embedding call cached per query. Most of it is the vector search; HNSW with ef=64 keeps recall at 0.97.", nil},
			}},
		{Repo: "dotfiles", Branches: []string{"main"}, Ago: 8 * 24 * time.Hour, Span: 15 * time.Minute, Model: "claude-haiku-5-5", CostUSD: 0.08,
			Turns: []Turn{{"make my zsh startup faster, it takes 900ms", "nvm was 610ms of it. Lazy-loading nvm and caching `compinit` brings startup to 120ms.", []string{"Edit: ~/.zshrc"}}}},
		{Title: "IOS-CRASH", Repo: "mobile", Branches: []string{"fix/ios-crash-on-launch"}, PR: "https://github.com/acme/mobile/pull/135", Color: "red",
			Ago: 30 * time.Hour, Span: 2 * time.Hour, Model: "claude-opus-5-5", CostUSD: 3.60,
			Turns: []Turn{
				{"ios app crashes on launch for some users after 4.2, here's the crashlytics trace", "The crash is a force-unwrap of `UserDefaults` migration data that only exists for users upgrading from 3.x. Replaced it with a guarded migration and added a test.", []string{"Edit: ios/App/Migrations.swift"}},
			}},
		{Title: "COST-REPORT", Repo: "infra", Branches: []string{"main"}, Color: "green", Live: "busy",
			Recap: "Pulling last month's cloud bill by service; S3 egress looks 3x higher than expected.",
			Ago:   1 * time.Minute, Span: 40 * time.Minute, Model: "claude-sonnet-5-5", CostUSD: 1.10,
			Job:   "running | Pulling the September bill by cost-allocation tag; 14 of 22 services done.",
			Crons: []string{"*/30 * * * * | Re-check S3 egress for the exports bucket and post a delta if it grew more than 10%"},
			Turns: []Turn{
				{"break down last month's cloud cost by team and flag anything weird", "Total $48.2k (+12%). Biggest jump: S3 egress at $6.1k vs $2.0k average, almost all from the `exports` bucket after the CSV export launch.", []string{"Bash: ./scripts/cost-by-tag.sh 2026-09"}},
			}},
		{Repo: "atlas", Branches: []string{"main"}, Ago: 12 * 24 * time.Hour, Span: 30 * time.Minute, Model: "claude-sonnet-5-5", CostUSD: 0.30,
			Turns: []Turn{{"explain how our feature flag evaluation works", "Flags are evaluated server-side in `flags.ts`: user overrides first, then tenant rules, then percentage rollout hashed on user id.", nil}}},
		{Repo: "atlas", Branches: []string{"main"}, Ago: 7 * time.Hour, Span: 45 * time.Minute, Model: "claude-sonnet-5-5", CostUSD: 0.90, Live: "idle",
			Turns: []Turn{
				{"Priya Patel  [10:14 AM]\nhey team — the invoice PDF export shows the wrong currency symbol for CHF customers, can someone take a look?", "Found it: `formatMoney` falls back to `$` when the locale has no symbol override. CHF needs `currencyDisplay: 'code'`. Patch is ready.", []string{"Bash: rg -n formatMoney src"}},
				{"reply to Priya with what we found", "Drafted: \"Fixed in #418 — CHF now renders as 'CHF 1,200.00'. It was a fallback in formatMoney.\"", nil},
			}},
		{Repo: "infra", Branches: []string{"main"}, Ago: 4 * 24 * time.Hour, Span: 30 * time.Minute, Model: "claude-sonnet-5-5", CostUSD: 0.60,
			Turns: []Turn{
				{"Marco Rossi  [3:02 PM]\ncan we give the data team read access to the analytics replica?", "Added a `data-readonly` role scoped to the `analytics` schema and a Terraform PR for the IAM binding. Marco needs to confirm the group email.", []string{"Edit: terraform/iam.tf"}},
			}},
		{Title: "SEARCH-INDEX-REWRITE", Repo: "atlas", Branches: []string{"exp/search-index"}, Color: "purple", Ago: 16 * 24 * time.Hour, Span: 8 * time.Hour, Model: "claude-opus-5-5", CostUSD: 6.30,
			Turns: []Turn{
				{"let's rewrite the search indexer as a streaming pipeline", "Sketched a pipeline: change-data-capture → normalizer → batcher → index writer, with backpressure via bounded channels.", nil},
				{"how do we handle deletes", "Tombstones flow through the same pipeline; the writer issues delete-by-id and the batcher coalesces delete+insert pairs.", nil},
				{"benchmark it against the current batch job", "Streaming keeps the index 4s behind writes vs 15 minutes for the batch job, at similar CPU.", []string{"Bash: go test -bench . ./indexer/..."}},
				{"what about reindexing from scratch", "Added a backfill mode that reads snapshots in parallel ranges and switches to CDC at the snapshot LSN.", nil},
				{"ok let's park this until after the launch", "Parked. Notes are in docs/search-streaming.md; the branch is pushed.", nil},
			}},
		{Repo: "mobile", Branches: []string{"main"}, Ago: 20 * time.Hour, Span: 2 * time.Minute, Model: "claude-haiku-5-5"},
		{Title: "I18N", Repo: "mobile", Branches: []string{"feat/i18n"}, Color: "blue",
			Ago: 9 * 24 * time.Hour, Span: 5 * time.Hour, Model: "claude-sonnet-5-5", CostUSD: 5.20,
			Turns: []Turn{
				{"add german and japanese, extract all strings", "Extracted 1,284 strings into `locales/en.json`, added `de` and `ja` with machine translations flagged for review.", []string{"Bash: pnpm i18n:extract"}},
			}},
	}
}

// Generate writes the demo tree into dir and returns it. now anchors all timestamps
// so tests can render deterministic relative ages.
func Generate(dir string, now time.Time) error {
	rng := rand.New(rand.NewSource(7))
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, Marker), []byte("synthetic data\n"), 0o644); err != nil {
		return err
	}
	if err := writeGear(dir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, PanesFile), []byte(demoPanes), 0o644); err != nil {
		return err
	}
	if err := writeCodex(CodexDir(dir), now); err != nil {
		return err
	}
	if err := writeCursor(CursorDir(dir), now); err != nil {
		return err
	}
	pid := 40000
	for i, sp := range Specs() {
		if sp.ID == "" {
			sp.ID = fmt.Sprintf("%08x-%04x-4%03x-a%03x-%012x", rng.Uint32(), rng.Intn(1<<16), rng.Intn(1<<12), rng.Intn(1<<12), rng.Int63n(1<<48))
		}
		cwd := "/Users/demo/code/" + sp.Repo
		proj := filepath.Join(dir, "projects", strings.ReplaceAll(cwd, "/", "-"))
		if err := os.MkdirAll(proj, 0o755); err != nil {
			return err
		}
		lines := renderSession(sp, cwd, now)
		if err := os.WriteFile(filepath.Join(proj, sp.ID+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			return err
		}
		if sp.Job != "" {
			state, detail, _ := strings.Cut(sp.Job, " | ")
			tempo := "idle"
			if state == "running" {
				tempo = "working"
			}
			job := map[string]any{"state": state, "tempo": tempo, "detail": detail, "name": sp.Title, "intent": sp.Turns[0].User,
				"sessionId": sp.ID, "cwd": cwd, "backend": "daemon", "template": "bg",
				"createdAt": now.Add(-sp.Ago - sp.Span).UTC().Format(time.RFC3339Nano), "updatedAt": now.Add(-sp.Ago).UTC().Format(time.RFC3339Nano)}
			b, _ := json.MarshalIndent(job, "", "  ")
			jdir := filepath.Join(dir, "jobs", sp.ID[:8])
			if err := os.MkdirAll(jdir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(jdir, "state.json"), b, 0o644); err != nil {
				return err
			}
		}
		if sp.Live != "" {
			pid += 17 * (i + 1)
			reg := map[string]any{"pid": pid, "sessionId": sp.ID, "cwd": cwd, "name": sp.Title, "kind": "interactive",
				"status": sp.Live, "updatedAt": now.Add(-sp.Ago).UnixMilli(), "startedAt": now.Add(-sp.Ago - sp.Span).UnixMilli()}
			b, _ := json.Marshal(reg)
			if err := os.WriteFile(filepath.Join(dir, "sessions", fmt.Sprintf("%d.json", pid)), b, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderSession(sp Spec, cwd string, now time.Time) []string {
	end := now.Add(-sp.Ago)
	start := end.Add(-sp.Span)
	n := len(sp.Turns)
	var lines []string
	add := func(v map[string]any) {
		b, _ := json.Marshal(v)
		lines = append(lines, string(b))
	}
	base := func(typ string, t time.Time, branch string) map[string]any {
		return map[string]any{"type": typ, "sessionId": sp.ID, "timestamp": t.UTC().Format(time.RFC3339Nano),
			"cwd": cwd, "gitBranch": branch, "version": "2.1.302", "isSidechain": false, "userType": "external", "entrypoint": "cli"}
	}
	meta := base("user", start, sp.Branches[0])
	meta["isMeta"] = true
	meta["message"] = map[string]any{"role": "user", "content": "<system-reminder>session start</system-reminder>"}
	add(meta)
	for i, t := range sp.Turns {
		at := start.Add(time.Duration(float64(sp.Span) * float64(i) / float64(max(n, 1))))
		branch := sp.Branches[min(i, len(sp.Branches)-1)]
		u := base("user", at, branch)
		u["message"] = map[string]any{"role": "user", "content": t.User}
		add(u)
		var blocks []any
		for _, tool := range t.Tools {
			name, arg, _ := strings.Cut(tool, ": ")
			key := "command"
			if name != "Bash" {
				key = "file_path"
			}
			blocks = append(blocks, map[string]any{"type": "tool_use", "name": name, "input": map[string]any{key: arg}})
		}
		blocks = append(blocks, map[string]any{"type": "text", "text": t.Assistant})
		a := base("assistant", at.Add(40*time.Second), branch)
		a["message"] = map[string]any{"role": "assistant", "model": sp.Model, "content": blocks}
		add(a)
	}
	last := base("system", end, sp.Branches[len(sp.Branches)-1])
	if sp.Recap != "" {
		last["subtype"] = "away_summary"
		last["content"] = sp.Recap + " (disable recaps in /config)"
		add(last)
	}
	if sp.Title != "" {
		add(map[string]any{"type": "custom-title", "customTitle": sp.Title, "sessionId": sp.ID})
		add(map[string]any{"type": "agent-name", "agentName": sp.Title, "sessionId": sp.ID})
	}
	if sp.AITitle != "" {
		add(map[string]any{"type": "ai-title", "aiTitle": sp.AITitle, "sessionId": sp.ID})
	}
	if sp.Color != "" {
		add(map[string]any{"type": "agent-color", "agentColor": sp.Color, "sessionId": sp.ID})
	}
	if sp.PR != "" {
		add(map[string]any{"type": "pr-link", "prUrl": sp.PR, "sessionId": sp.ID, "timestamp": end.UTC().Format(time.RFC3339Nano)})
	}
	for _, a := range sp.Artifacts {
		url, title, _ := strings.Cut(a, " ")
		add(map[string]any{"type": "frame-link", "frameUrl": url, "title": title, "sessionId": sp.ID, "timestamp": end.UTC().Format(time.RFC3339Nano)})
	}
	for i, c := range sp.Crons {
		expr, prompt, _ := strings.Cut(c, " | ")
		for f := range 3 {
			fire := base("system", end.Add(-time.Duration(f)*30*time.Minute), sp.Branches[0])
			fire["subtype"] = "scheduled_task_fire"
			fire["content"] = "Running scheduled task"
			fire["taskId"] = fmt.Sprintf("c%07d", i)
			fire["cron"] = expr
			fire["prompt"] = prompt
			add(fire)
		}
	}
	if sp.Remote {
		add(map[string]any{"type": "bridge-session", "bridgeSessionId": "bridge_" + sp.ID[:8], "sessionId": sp.ID})
	}
	if sp.CostUSD > 0 {
		add(map[string]any{"type": "cost-state", "totalCostUSD": sp.CostUSD, "sessionId": sp.ID})
	}
	return lines
}

func writeGear(dir string) error {
	files := map[string]string{
		filepath.Join(filepath.Dir(dir), ".claude.json"): `{"mcpServers":{
  "linear":{"type":"http","url":"https://mcp.linear.app/mcp"},
  "github":{"type":"http","url":"https://api.githubcopilot.com/mcp/"},
  "postgres":{"command":"npx","args":["-y","@modelcontextprotocol/server-postgres","postgres://localhost/atlas"]}}}`,
		filepath.Join(dir, "settings.json"): `{
  "theme": "dark",
  "enabledPlugins": {
    "superpowers@superpowers-dev": true,
    "frontend-design@claude-plugins-official": true,
    "vercel@claude-plugins-official": false
  },
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "afplay /System/Library/Sounds/Glass.aiff"}]}],
    "SessionStart": [{"matcher": "startup", "hooks": [{"type": "command", "command": "~/.claude/hooks/context.sh"}]}]
  }
}
`,
		filepath.Join(dir, "skills", "deploy-checklist", "SKILL.md"):                                                                           "---\nname: deploy-checklist\ndescription: Walk through the release checklist before any production deploy\n---\n",
		filepath.Join(dir, "skills", "grafana-dash", "SKILL.md"):                                                                               "---\nname: grafana-dash\ndescription: Open Grafana dashboards as live terminal charts\n---\n",
		filepath.Join(dir, "skills", "weekly-report", "SKILL.md"):                                                                              "---\nname: weekly-report\ndescription: Summarize the week's merged PRs and incidents for the team update\n---\n",
		filepath.Join(dir, "skills-disabled", "old-jira", "SKILL.md"):                                                                          "---\nname: old-jira\ndescription: Legacy Jira helper (replaced by Linear MCP)\n---\n",
		filepath.Join(dir, "agents", "reviewer.md"):                                                                                            "---\nname: reviewer\ndescription: Strict code reviewer for correctness and security\n---\n",
		filepath.Join(dir, "commands", "standup.md"):                                                                                           "---\ndescription: Draft my standup from yesterday's sessions\n---\n",
		filepath.Join(dir, "plugins", "cache", "superpowers-dev", "superpowers", "6.3.0", "skills", "brainstorming", "SKILL.md"):               "---\ndescription: Turn ideas into designs before writing code\n---\n",
		filepath.Join(dir, "plugins", "cache", "superpowers-dev", "superpowers", "6.3.0", "skills", "systematic-debugging", "SKILL.md"):        "---\ndescription: Find root causes before proposing fixes\n---\n",
		filepath.Join(dir, "plugins", "cache", "claude-plugins-official", "frontend-design", "1.0.0", "skills", "frontend-design", "SKILL.md"): "---\ndescription: Distinctive, intentional UI design\n---\n",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
