<div align="center">

# ▼•ᴥ•▼ kennel

**A cozy, Gather-style home for your Claude Code, Codex and Cursor sessions.**
Find any session by name, content or meaning, file them into rooms, see who needs you, and open them as terminal tabs.

[![CI](https://github.com/razkevich/kennel/actions/workflows/ci.yml/badge.svg)](https://github.com/razkevich/kennel/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/razkevich/kennel?include_prereleases)](https://github.com/razkevich/kennel/releases)

<img src="docs/img/app-kennel.png" alt="Kennel for Mac" width="900">

</div>

Running ten-plus agent sessions a day means losing track of them: which one needs an approval, where you fixed that flaky test last week, what came of a colleague's Slack question. Kennel indexes every transcript, watches which sessions are live, and puts them in a pixel-art office.

- **Kennel for Mac** has a pinned 🐾 Kennel tab holding the office. Every session you open becomes a normal terminal tab next to it, rendered with your Ghostty font and colors, and a menu-bar pup shows who needs you.
- **The same app in your browser:** `kennel` serves the identical UI, and sessions open in herdr or Ghostty instead.

Sessions are **pups**: Claude Code corgis ▼•ᴥ•▼, Codex huskies ▲•ᴥ•▲ and Cursor dalmatians ◖•ᴥ•◗. Rooms are **kennels**. Other terminals, such as monitors, dev servers, log tails and long jobs, are **things** 📺🔥📜⏳.

## What's in it

| | |
|---|---|
| 🗺 **The office** | Organize kennels by **floors** instead of one giant area: the floor navigator on the left lists each floor with its kennels, pups and who needs you; ＋ adds one, right-click renames or removes it, drag a kennel onto a floor to move it, and **✨ Organize into floors** has the model propose floors for your kennels (you review, one ⌘Z undoes it). Each floor holds **your kennels**, which show what you filed, and optional **smart rooms** that fill themselves: Live now, Firehouse, Helping Out, Best in Show, Lobby, Workshop and more, added from 🔨 Build. Pups wander, sit at desks while working, and bounce with `!` when they need you. Click to inspect, double-click to open. |
| 📨 **Pups talk** | Drag one pup onto another (or right-click → Pass latest reply to…) to hand it the first one's latest reply plus your note: it's typed into the receiver's terminal or herdr pane, or an asleep session wakes up in the panel with it. The sender walks over, an envelope flies, and the two stay linked by a dotted line for a day. |
| 🛋 **Room objects** | In Build's Shape menu: a room color, and objects you place, move and resize: a rug (pups dropped on it go under, with a count), a doghouse or basket (they peek out), a couch, chair, desk or piano (they sit on it), dividers, and all the furniture. |
| 💬 **Chats** | The navigator lists active chats first, then recent ones; picking one is the same as clicking it on the map (it hops floors), and ☆ stars it everywhere. |
| ✏️ **Rename** | Renaming a pup renames the Claude session too: an asleep one gets the same title record `/rename` writes, and one running in kennel's terminal panel or a herdr pane gets `/rename` typed in. |
| ✋ **Arrange it by hand** | Drag pups anywhere in their room, or onto another kennel to file them. Marquee-select several and move them together. Select a room to get handles: drag its wall to move it and its handles to resize it, with snap guides as you go. Draw a new kennel with the ▭ tool. Right-click anything for its actions. Take pups out of a kennel by dragging them out, with Delete, or from the right-click menu or their card. That only unfiles them: the session goes back to 📥 New in the tray, and rule-based kennels won't pull it back. Tools: `V` select, `H` hand (or hold Space), `R` kennel. |
| 🌗 **Light and dark** | A day office and a night office. The 🖥 Auto / ☀️ Light / 🌙 Dark switch in the top bar (or `Shift+T`, or View → Appearance in the Mac app) is shared by every open window. |
| 🐾 **Sessions** | The side panel lists the last week: ❗ needs you, ⚡ working, 📥 new (not in a kennel), 🕘 recent. Group by status, kennel, project or role; sort by most recent, most active, cost or name; filter by text. Drag any row onto a kennel to file it, or press **✨ Sort these into kennels**: the model suggests a kennel for each new session (yours first), you adjust, and everything is filed in one undoable step. Kennels on the map show the last week too; older sessions stay filed and are one click away in the Library. Your filters and the panel's width are remembered. |
| 🖥 **Terminal panel** | Click a pup and the panel below its card becomes its terminal: an asleep session resumes right there (▶ Resume here), one already running here re-attaches with its scrollback, and one running elsewhere shows its latest messages with ↗ Go to. Move it to a dedicated tab whenever you like. Drag the panel's left edge to make it wider. |
| 📁 **Projects** | The navigator's top section lists your projects; each has its own floors, kennels and smart rooms. ＋ starts one, double-click renames, ⋯ sets its icon or links a folder (then its sessions and smart rooms show only that folder's sessions; without one it sees everything), or removes it (its floors move to another project; undoable). |
| 🎩 **Roles** | Pups wear what they do (or what you pick: 🎨 Look on a pup's card sets its hat and fur): a firefighter's helmet for incidents, glasses for reviews, a hard hat for infra, a top hat for cost work, a beret for UI, a headset for colleagues' asks, a magnifier for tests, and more. |
| 📚 **Library** | Every past session: filter by date, agent, kennel or theme, group by day, week, theme, kennel, repo or agent, sort, and move many at once. |
| ✨ **Themes** | Automatic categories from an LLM, Cloudflare Workers AI or your `claude` haiku. Only titles, first asks and recaps are sent. |
| 🔎 **Search** | Fuzzy titles, branches and PRs, plus full-text over every prompt, reply and tool call, with highlighted snippets. Filters: `is:live` `repo:x` `agent:codex` `since:7d` `has:pr`. |
| 💬 **Ask** | "What did we decide about the Postgres upgrade?" Answered from your sessions with numbered citations you can jump to. |
| ❗ **Live status** | Working, needs you, done, idle or asleep, from Claude Code's session registry and herdr. No hooks to install. |
| 🗂 **Organize** | Star, rename (written like `/rename`), notes, archive, recoverable trash, rooms with auto-adopt magnets. **Undo** everything with ⌘Z. |
| ⑂ **Fork, clone, merge, convert** | Fork; clone into another project; merge several sessions into a new pup from a brief; distill a long one; continue any session in another agent (Claude ↔ Codex ↔ Cursor) through a Markdown handoff. The original is never touched. |
| 📌 **Kennel items** | Keep files, links, `#slack-channels`, tickets and your sessions' artifacts in a kennel, like pups: each stands in the room as a little prop (a poster, a page, a sign, a ticket stub, an easel). Click for its card, double-click to open, drag it to another kennel or out of the room, right-click for its menu. The 📎 panel lists everything, including artifacts from sessions (pin or hide them). Drag any item onto a kennel to pin it, or onto a pup to feed it: a live pup gets it as a prompt, an asleep one wakes up with it. |
| 🔨 **Build** | Room templates, shapes, floors, walls and furniture, plus purpose blocks: 🚪 spawn door, 🧲 magnet, 🔔 bell, 🌀 portal to a herdr workspace, 📌 pin board, 🧹 archive chute. |
| ⏰ **Jobs** | Background agents, in-session cron tasks, monitors and servers. |

## Screens

<table>
<tr><td><img src="docs/img/web-map.png" alt="the office"></td><td><img src="docs/img/web-library.png" alt="library grouped by theme"></td></tr>
<tr><td align="center">the office, with the session tray</td><td align="center">📚 library, grouped by ✨ theme</td></tr>
<tr><td><img src="docs/img/web-edit.png" alt="resizing a room"></td><td><img src="docs/img/web-card.png" alt="pup card"></td></tr>
<tr><td align="center">✋ select a room, drag handles to resize</td><td align="center">a pup's card: state and every action</td></tr>
<tr><td><img src="docs/img/web-build.png" alt="build mode"></td><td><img src="docs/img/web-dark.png" alt="dark mode"></td></tr>
<tr><td align="center">🔨 build mode</td><td align="center">🌙 the night office</td></tr>
<tr><td><img src="docs/img/web-ask.png" alt="ask"></td><td><img src="docs/img/web-projects.png" alt="project filter"></td></tr>
<tr><td align="center">💬 Ask, with citations</td><td align="center">📁 one project in focus</td></tr>
<tr><td colspan="2"><img src="docs/img/web-jobs.png" alt="jobs and things"></td></tr>
<tr><td colspan="2" align="center">⏰ jobs and things</td></tr>
</table>

Every screenshot comes from built-in synthetic sessions (`kennel -demo`), not real ones.

## Install

**Mac app:** download `Kennel.app.zip` from [Releases](https://github.com/razkevich/kennel/releases), unzip it into Applications, and open it. The first time, right-click and choose Open, because the app is ad-hoc signed.

**Browser only:**
```sh
go install github.com/razkevich/kennel/cmd/kennel@latest   # or grab a binary from Releases
kennel                                                    # opens the office in your browser
```

## Use

```sh
kennel                              # the office (in your browser)
kennel search "flaky login"         # names + transcripts (-json)
kennel ask "why did checkout flicker on safari?"
kennel open KALSHI                  # resume by name or id (-fork, -remote, -opener herdr|ghostty|inline)
kennel convert KALSHI -to codex     # continue in another agent
kennel merge KALSHI C1VEGA -name RELOC
kennel clone KALSHI -to ~/code/other
kennel export KALSHI -o kalshi.md
kennel -demo                        # try it on synthetic sessions
```

**Keys in the office:** `/` search · `V` `H` `R` tools · Space-drag pan · `+` `-` `Shift+1` zoom · `Delete` remove from kennel (archive elsewhere) · `Shift+T` light/dark · `Enter` open · arrows pan · `l` library · `b` build · `⌘Z` undo · `Esc` close.
**Keys in the Mac app** follow Ghostty's defaults:

| | |
|---|---|
| Tabs | `⌘T` new (in the current directory) · `⌘⌥W` close tab · `⌘1–9` by position (`⌘1` is Kennel) · `⌘⇧[ ]` previous/next · `⌘⇧K` Kennel |
| Splits | `⌘D` split right · `⌘⇧D` split down · `⌘W` close split · `⌘[ ]` previous/next split · `⌘⌥←↑↓→` move between splits · `⌘⌃←↑↓→` resize · `⌘⌃=` equalize · `⌘⇧↩` zoom a split. Drag a divider to resize. Unfocused splits dim (`unfocused-split-opacity`). |
| Terminal | `⌘K` clear · `⌘F` find, `⌘G` / `⌘⇧G` next/previous, `⌘E` use selection · `⌘=` `⌘-` `⌘0` font size per tab (page zoom in the Kennel tab) · `⌘↩` full screen |
| Config | `⌘,` opens your Ghostty config · `⌘⇧,` reloads it: font, colors, palette, `split-divider-color`, `unfocused-split-opacity` |

**Themes with Cloudflare:** put `CLOUDFLARE_AI_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` in the environment or in `~/cloudflare_ai_token.env`. Optionally set `KENNEL_CF_MODEL` (default `@cf/meta/llama-3.1-8b-instruct-fast`). Without them, kennel uses `claude` haiku.

## How it works

- **Index.** Claude Code transcripts (`~/.claude/projects`), Codex rollouts (`~/.codex/sessions`) and Cursor composer chats, read-only from its state database, go into SQLite with FTS5. Kennel parses only the bytes appended since the last pass, and the first index runs in the background.
- **Live status** comes from `~/.claude/sessions/<pid>.json` and `herdr agent list`.
- **Opening.** In the Mac app, the page hands launches to the app over a WebKit message bridge and they become tabs. In a browser, kennel focuses the herdr pane or opens herdr or Ghostty tabs.
- **Safety.** Everything kennel sends to a shell or terminal has control bytes removed. Only Claude transcripts are ever written to, and only by rename (the same record `/rename` writes). Trash moves files and never deletes them. The page listens on `127.0.0.1` with a per-run token and a Host check.

Kennel's own data lives in `~/.kennel`. Override paths with `KENNEL_CLAUDE_DIR`, `KENNEL_CODEX_DIR`, `KENNEL_CURSOR_DIR` and `KENNEL_HOME`.

## Develop

```sh
go test -race ./...                         # core, store, indexer, search, agents, blocks, undo
(cd e2e && npm ci && npx playwright test)   # browser UI: clicks, drags, build mode, library, panels
(cd macos && ./test.sh)                     # Swift unit tests
(cd macos && swift build) && node --test e2e/app/app.test.mjs   # Mac app UI: tabs, shortcuts, terminals, clicks, dialogs
macos/build-app.sh                          # dist/Kennel.app
```

The Mac app UI tests launch the real app on synthetic sessions with fake runs, so no real agent is started. They drive it through an automation port that is enabled only by `KENNEL_APP_AUTOMATION`: real menu shortcuts, real clicks in the page, typing into terminals, and reading terminal text. Every test uses synthetic data, with recorders standing in for herdr, osascript and the browser.

## Roadmap

- **Pups working together:** standing "pairs" (a reviewer pup watches a coder pup and comments on each turn), a relay that passes a reply on automatically when a session finishes, and broadcasting one message to a whole kennel.
- **Lineage leashes:** show forks, clones, merges and handoffs as a family tree, and replay how a piece of work moved between sessions.
- **Kennel routines:** a ⏲ timer block that sends a room's pups a recurring prompt (stand-up summaries, "rebase on main"), and a 🚦 gate that holds a pup until another one finishes.
- **Budgets and costs:** per-kennel spend with a soft limit that taps a pup on the shoulder before it burns through it.
- **Sharing:** export a floor as a read-only page for teammates, and room templates others can import.
- **Search over meaning:** local embeddings alongside full-text, and "find the session where we decided X" straight from the map.
- **More agents:** Gemini CLI, Aider and opencode sessions; remote herdr machines as their own project.

## License

MIT
