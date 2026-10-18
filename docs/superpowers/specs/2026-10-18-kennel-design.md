# Kennel — design

Date: 2026-10-18. Written autonomously overnight (user asleep, asked for a finished product);
decisions below were made without a review round and are open to revision.

## What it is

A fast terminal app for finding, organizing and jumping between Claude Code sessions.
Sessions are "pups", user-defined groups are "kennels" (spaces). It sits next to the tools
already in use — Ghostty and herdr — rather than replacing them: Kennel never emulates a
terminal, it opens/focuses real ones.

## Evidence from actual usage (229 local sessions, Sep–Oct 2026)

- 10–16 new sessions a day, many in parallel, ~85% in one repo (`~/code/chat`) plus worktrees.
- Long-lived workstreams get an UPPERCASE `/rename` (`KALSHI`, `C1VEGA`, `DRIFT`, …): 44 titled,
  `/rename` is the most used slash command, `/fork` second. Short one-offs stay untitled.
- 48 sessions produced PRs; 52 span more than one git branch; worktrees under `.claude/worktrees/`.
- Many sessions start from a pasted Slack message; finding "the session where …" later is hard
  (one session literally asked "what is claude session e2817dad", another swept all past sessions).
- herdr is the multiplexer; it tracks agent state per pane and exposes it via `herdr agent list`.
- Claude Code keeps a live registry in `~/.claude/sessions/<pid>.json` with sessionId, name, cwd
  and status (`idle`/`busy`/…), so live status needs no hooks.

## Decisions

1. **Go TUI (Bubble Tea v1 + Lip Gloss), single static binary.** Fits a terminal-first workflow,
   runs inside a herdr tab or Ghostty, testable headlessly, easy releases via GoReleaser.
   A native/web "Gather map" was considered; the Pack view (below) gives that feel without a
   second runtime.
2. **SQLite (modernc, pure Go, FTS5) at `~/.kennel/kennel.db`.** Index tables are rebuildable;
   user metadata (spaces, stars, aliases) lives in separate tables that reindex never touches.
3. **Incremental indexer.** Session files are append-only JSONL; store byte offset per file and
   parse only the tail. Parallel full index on first run, in the background — the UI is usable
   immediately and fills in.
4. **Search is three-tier and always live as you type:**
   - fuzzy over title/alias/first prompt/branch/cwd/id/PRs (in-memory, every keystroke);
   - full-text over message content via FTS5 bm25 with snippets (debounced);
   - **Ask** (RAG): Claude rewrites the question into search terms, Kennel retrieves top chunks
     (diversified per session), Claude answers with numbered citations that jump to sessions.
     Optional semantic vectors via a local Ollama embedding model when it is running.
   Filters: `is:live`, `is:starred`, `space:x`, `repo:x`, `since:7d`.
5. **Opening sessions** (Enter): live + known herdr pane → focus that pane; otherwise resume via
   the configured opener: `herdr` (new tab, `herdr agent start … -- --resume ID`), `ghostty`
   (AppleScript new tab), or `inline` (suspend Kennel, run claude in this terminal, return).
   Default `auto` picks herdr inside herdr, Ghostty in Ghostty, inline otherwise.
6. **LLM calls** shell out to the user's `claude -p` (already authenticated) with
   `--no-session-persistence` so Kennel never pollutes the session list.
7. **Never destructive:** delete moves the JSONL into `~/.kennel/trash/`; rename appends a
   standard `custom-title` record (what `/rename` writes) only when the session is not live,
   and always stores a Kennel alias.

## Features (v0.1)

- Sidebar: smart spaces (All, Live, Starred, Today, per-repo) + user kennels with icon/colour.
- Session list: status dot (working / needs you / idle / offline), title, repo, branch, age, size.
- Preview: ids, cwd, branches, PRs, cost, first prompt, last prompts, matching snippets.
- Transcript viewer with in-transcript search.
- Pack view (`tab`): Gather-style rooms per kennel; pups wag while working, bark when waiting.
- Actions: new session in kennel, resume, fork, rename, star, move to kennel, open PR,
  copy id / resume command, trash, auto-title suggestion via Claude.
- Kennel rules: auto-assign sessions by cwd prefix or title/branch regex.
- Status-bar toasts when a live session finishes or needs input.
- CLI: `kennel`, `kennel search`, `kennel ask`, `kennel index`, `kennel ls`, `kennel open`.
- `KENNEL_CLAUDE_DIR` points at another `~/.claude` (used for demo data / screenshots).

## Layout

```
cmd/kennel            entry, CLI subcommands
internal/claude       JSONL parsing, live registry, launching claude
internal/store        SQLite schema, index + metadata queries, FTS
internal/indexer      incremental background indexing
internal/search       query parsing, fuzzy + FTS merge
internal/ask          RAG pipeline over claude -p
internal/opener       herdr / ghostty / inline
internal/tui          Bubble Tea UI
internal/demo         synthetic sessions for screenshots/tests
```

## Testing

Unit tests for the parser (fixtures from real record shapes), store/FTS, query parser,
search merge and incremental offsets; a smoke test runs the TUI model headless over demo data.
CI: `go vet`, `go test -race`, build on macOS + Linux. Releases: GoReleaser on tags
(darwin/linux, amd64/arm64).

## Status after the overnight build (2026-10-18)

Everything above shipped, plus requests that arrived during the night:

| Area | What landed | Where |
|---|---|---|
| Web map | Gather-style pixel office: rooms, pups wander, desks for working pups, status bubbles, your avatar (WASD + X), cards, search, Ask, transcripts | `internal/web` |
| Smart kennels | Yard, Best in Show, Firehouse, Helping Out, Courtroom, Yak Shaving, Water Bowl, Marathons, Shipped, Under the Rug, Dumpster | `internal/pack` |
| Things | non-agent terminals (monitors, servers, log tails, jobs) as objects; herdr panes + `ps` | `internal/things` |
| Jobs | Claude daemon background agents + in-session cron tasks | `internal/claude/jobs.go`, TUI Jobs tab |
| Artifacts | gallery from `frame-link` records | TUI Artifacts tab |
| Gear | MCP/skills/plugins/agents/commands/hooks at user/project/plugin level, safe toggles | `internal/gear` |
| Projects | one room per repo (Projects floor) + sidebar section | `internal/floor` |
| Remote control | 📡 badge from `bridge-session`, `R` resumes with `--remote-control` or sends `/remote-control` via herdr | opener |
| Building blocks | templates, shapes, floors, walls, furniture; spawn door, magnet, bell, portal, pin board, chute | `internal/blocks`, `internal/floor/behaviors.go`, web build mode, TUI `B` |
| Fork/clone/merge | fork, clone into another project, merge N sessions via brief, distill | `internal/app/lineage.go` |
| Agents | Codex rollouts and Cursor composer chats; breeds; convert between agents via Markdown handoff | `internal/claude/codex.go`, `internal/cursor`, `internal/app/export.go` |
| Treats | drag files/links/#channels/tickets/notes onto pups (live → herdr prompt, asleep → resume with prompt) | `internal/treats`, `internal/app/feed.go` |
| Digest | touched/shipped/waiting/under-the-rug summary | `internal/digest` |

Testing: Go unit tests per package; TUI tests drive the real Bubble Tea model with keys and mouse
clicks (bubblezone) and assert on rendered frames; Playwright tests click and drag on the canvas;
screenshots are rendered from real frames (xterm.js for the TUI). All against `internal/demo`
synthetic data with a recorder replacing herdr/osascript/open.

Known gaps (not verifiable without touching real data, by design):
- herdr and Ghostty integrations are exercised only against recorded command shapes, not live.
- Codex and Cursor formats are implemented from their documented/observed shapes and tested with
  synthetic fixtures; unknown variants are skipped rather than failing.
- Cold indexing of a 1.4 GB history takes ~40 s in the background on first run.
