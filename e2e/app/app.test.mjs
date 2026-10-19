// End-to-end UI tests for the Kennel Mac app. They launch the real app on synthetic
// sessions with fake runs (nothing real is started) and drive it through its
// automation port: real menu shortcuts, real clicks in the page, typing into terminals.
// usage: node --test e2e/app/app.test.mjs  (after `swift build` in macos/ and `go install ./cmd/kennel`)
import { test, before, after } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import path from "node:path";
import fs from "node:fs";
import os from "node:os";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const appBin = process.env.KENNEL_APP_BIN || path.join(root, "macos/.build/debug/Kennel");
// A bundled Kennel.app carries its own kennel server; a dev build uses the installed one.
const kennelBin = process.env.KENNEL_BIN || (appBin.includes(".app/") ? "" : path.join(os.homedir(), "go/bin/kennel"));
const port = 7400 + Math.floor(Math.random() * 400);
const shots = process.env.SHOTS ? path.join(root, "docs/img") : fs.mkdtempSync(path.join(os.tmpdir(), "kennel-app-"));
let proc;

const call = async (method, p, body) => {
  const res = await fetch(`http://127.0.0.1:${port}${p}`, { method, body: body ? JSON.stringify(body) : undefined });
  const out = await res.json();
  if (!res.ok) throw new Error(`${p}: ${out.error}`);
  return out;
};
const state = () => call("GET", "/state");
const js = async (code) => (await call("POST", "/js", { code })).value;
const key = (k, mods = ["cmd"]) => call("POST", "/key", { key: k, mods });
const text = async (i) => (await call("GET", `/text?index=${i}`)).text;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function waitFor(fn, what, ms = 8000) {
  const end = Date.now() + ms;
  let last;
  while (Date.now() < end) {
    try { last = await fn(); if (last) return last; } catch (e) { last = e.message; }
    await sleep(150);
  }
  throw new Error(`timed out waiting for ${what} (last: ${JSON.stringify(last)})`);
}
const tabs = async () => (await state()).tabs;
const selected = async () => (await tabs()).find((t) => t.selected);
const pupId = (title) => js(`Object.values(window.kennel.app.state.pups).find(p => p.title === ${JSON.stringify(title)}).id`);
const cardButton = (label) => js(`(() => { const b = [...document.querySelectorAll("#card button")].find(b => b.textContent.includes(${JSON.stringify(label)})); if (b) b.click(); return !!b })()`);
async function clickPup(title, double = false) {
  const id = await pupId(title);
  const p = await js(`(() => { window.kennel.reveal(${JSON.stringify(id)}); const p = window.kennel.screenPos(${JSON.stringify(id)}); return { x: p.x, y: p.y } })()`);
  await sleep(200);
  await call("POST", "/click", { x: p.x, y: p.y, double });
  return id;
}
async function backToKennel() {
  await key("k", ["cmd", "shift"]);
  await waitFor(async () => (await selected()).kind === "kennel", "kennel tab selected");
}

before(async () => {
  assert.ok(fs.existsSync(appBin), `build the app first: ${appBin}`);
  proc = spawn(appBin, [], {
    env: { ...process.env, ...(kennelBin ? { KENNEL_BIN: kennelBin } : {}), KENNEL_APP_DEMO: "1", KENNEL_APP_FAKE_RUN: "1", KENNEL_APP_AUTOMATION: String(port), KENNEL_FAKE_LLM: "1" },
    stdio: "ignore",
  });
  await waitFor(async () => (await state()).pageLoaded, "page loaded", 20000);
  await waitFor(() => js("!!(window.kennel && window.kennel.app.state && window.kennel.app.ents.size > 0)"), "map drawn");
  if ((await js("window.kennel.app.state.floor !== window.kennel.app.state.floors[0].id"))) {
    await js(`document.querySelector("#floors .floor[data-floor]").click()`);
    await waitFor(() => js("window.kennel.app.state.floor === window.kennel.app.state.floors[0].id && window.kennel.app.ents.size > 0"), "first floor");
  }
});

after(async () => {
  try { await call("POST", "/quit"); } catch (_) {}
  await sleep(400);
  proc?.kill();
});

test("starts with one pinned Kennel tab and a menu-bar pup counting who needs you", async () => {
  const t = await tabs();
  assert.equal(t.length, 1);
  assert.equal(t[0].kind, "kennel");
  assert.match(t[0].title, /Kennel/);
  await waitFor(async () => (await state()).status.includes("▼°ᴥ°▼ 1"), "status item shows 1 waiting");
  assert.match((await state()).status, /⚡2/);
  await call("POST", "/snapshot", { index: 0, path: path.join(shots, "app-kennel.png") });
});

test("⌘T opens a shell tab you can type into; ⌘W closes it; the Kennel tab can't be closed", async () => {
  await key("t");
  await waitFor(async () => (await tabs()).length === 2, "second tab");
  const t = await selected();
  assert.equal(t.kind, "terminal");
  await sleep(600);
  await call("POST", "/type", { text: "echo hello-$((40+2))-kennel\n" });
  await waitFor(async () => (await text(1)).includes("hello-42-kennel"), "shell output");
  await key("w");
  await waitFor(async () => (await tabs()).length === 1, "tab closed");
  await key("w");
  await sleep(300);
  assert.equal((await tabs()).length, 1, "kennel tab is pinned");
});

test("Shift+Return in a terminal is a new line for the agent, not a submit", async () => {
  await key("t");
  await waitFor(async () => (await tabs()).length === 2, "shell tab");
  await sleep(600);
  await call("POST", "/type", { text: "cat -v\n" });
  await sleep(400);
  await call("POST", "/type", { text: "first" });
  await call("POST", "/keydown", { key: "return", mods: ["shift"] });
  await call("POST", "/type", { text: "second\n" });
  // cat -v shows the ESC that comes before the Return as ^[.
  await waitFor(async () => (await text(1)).includes("first^["), "ESC+Return sent");
  await call("POST", "/type", { text: "\u0004" });
  await key("w");
  await waitFor(async () => (await tabs()).length === 1, "closed");
});

test("Ghostty splits: ⌘D / ⌘⇧D split, ⌘⌥arrows and ⌘[ ] move focus, ⌘⌃ resize and equalize, ⌘⇧↩ zoom, ⌘W closes a split", async () => {
  await key("t");
  await waitFor(async () => (await tabs()).length === 2, "shell tab");
  const panes = async () => (await selected()).panes;
  const focusedAt = async () => (await panes()).findIndex((p) => p.focused);
  await key("d");
  await waitFor(async () => (await panes()).length === 2, "split right");
  let ps = await panes();
  assert.ok(ps[1].frame[0] > ps[0].frame[0] + 100, "side by side");
  assert.equal(ps[0].frame[1], ps[1].frame[1]);
  assert.equal(await focusedAt(), 1, "the new split has focus");
  await sleep(600);
  await call("POST", "/type", { text: "echo right-$((1+1))\n" });
  await waitFor(async () => (await call("GET", "/text?index=1&pane=1")).text.includes("right-2"), "typed into the right split");
  assert.ok(!(await call("GET", "/text?index=1&pane=0")).text.includes("right-2"), "left split untouched");

  await key("d", ["cmd", "shift"]);
  await waitFor(async () => (await panes()).length === 3, "split down");
  ps = await panes();
  assert.ok(ps[2].frame[1] > ps[1].frame[1] + 50 && ps[2].frame[0] === ps[1].frame[0], "stacked under the right split");

  await key("left", ["cmd", "alt"]);
  await waitFor(async () => (await focusedAt()) === 0, "⌘⌥← to the left split");
  await key("[");
  await waitFor(async () => (await focusedAt()) === 2, "⌘[ wraps to the last split");
  await key("]");
  await waitFor(async () => (await focusedAt()) === 0, "⌘] wraps to the first");

  const w0 = (await panes())[0].frame[2];
  await key("right", ["cmd", "ctrl"]);
  await key("right", ["cmd", "ctrl"]);
  await waitFor(async () => (await panes())[0].frame[2] >= w0 + 15, "⌘⌃→ widens the split");
  await key("=", ["cmd", "ctrl"]);
  await waitFor(async () => { const p = await panes(); return Math.abs(p[0].frame[2] - p[1].frame[2]) <= 2; }, "equalized");

  await key("enter", ["cmd", "shift"]);
  await waitFor(async () => (await selected()).zoomed, "zoomed");
  ps = await panes();
  assert.ok(ps[1].hidden && ps[2].hidden && !ps[0].hidden);
  await call("POST", "/snapshot", { index: 1, path: path.join(os.tmpdir(), "kennel-app-zoom.png") });
  await key("enter", ["cmd", "shift"]);
  await waitFor(async () => !(await selected()).zoomed, "unzoomed");

  const f0 = (await panes())[0].font;
  await key("=");
  await key("=");
  await waitFor(async () => (await panes()).every((p) => p.font === f0 + 2), "⌘= grows every split's font");
  await key("0");
  await waitFor(async () => (await panes()).every((p) => p.font === f0), "⌘0 resets");
  await call("POST", "/snapshot", { index: 1, path: path.join(os.tmpdir(), "kennel-app-splits.png") });

  await key("w");
  await waitFor(async () => (await panes()).length === 2, "⌘W closes just the focused split");
  await call("POST", "/type", { text: "exit\n" });
  await waitFor(async () => (await panes()).length === 1, "exiting a shell closes its split");
  await key("w");
  await waitFor(async () => (await tabs()).length === 1, "last split closes the tab");
});

test("⌘K clears the screen and scrollback", async () => {
  await key("t");
  await waitFor(async () => (await tabs()).length === 2, "shell tab");
  await sleep(600);
  await call("POST", "/type", { text: "echo clear-$((6*7))\n" });
  await waitFor(async () => (await text(1)).includes("clear-42"), "output");
  await key("k");
  await waitFor(async () => !(await text(1)).includes("clear-42"), "cleared");
  await key("w");
  await waitFor(async () => (await tabs()).length === 1, "closed");
});

test("dark mode: the Kennel tab's window chrome follows the page", async () => {
  await backToKennel();
  await call("POST", "/jsasync", { code: `await window.kennel.setAppearance("dark"); return true` });
  await waitFor(async () => (await state()).kennelDark === true, "dark chrome");
  assert.equal(await js(`document.documentElement.dataset.theme`), "dark");
  await call("POST", "/jsasync", { code: `await window.kennel.setAppearance("light"); return true` });
  await waitFor(async () => (await state()).kennelDark === false, "light chrome");
  await call("POST", "/jsasync", { code: `await window.kennel.setAppearance(""); return true` });
});

test("the terminal panel runs a pup inside the Kennel tab", async () => {
  await backToKennel();
  const id = await js(`Object.values(window.kennel.app.state.pups).find((p) => p.status === "offline" && p.agent === "claude").id`);
  await call("POST", "/jsasync", { code: `
    window.kennel.app.sel = ${JSON.stringify(id)};
    document.querySelector('#side .tabs button[data-tab="term"]').click();
    await new Promise((r) => setTimeout(r, 200));
    [...document.querySelectorAll("#term button")].find((b) => /Resume here|Continue here/.test(b.textContent)).click();
    return true` });
  await waitFor(() => js(`window.kennel.termText().includes("would run: claude --resume")`), "terminal output");
  await js(`(() => { const t = document.querySelector("#term textarea"); t && t.focus(); return true })()`);
  await call("POST", "/jsasync", { code: `await fetch("/api/term/${id}/input", { method: "POST", headers: { "X-Kennel-Token": new URLSearchParams(location.search).get("t") }, body: JSON.stringify({ data: "from the app\\r" }) }); return true` });
  await waitFor(() => js(`window.kennel.termText().includes("from the app")`), "echo");
  assert.equal((await tabs()).length, 1, "no extra tab opened");
  await call("POST", "/jsasync", { code: `await fetch("/api/term/${id}/stop", { method: "POST", headers: { "X-Kennel-Token": new URLSearchParams(location.search).get("t") }, body: "{}" }); return true` });
});

test("in the Kennel tab, ⌘= / ⌘0 zoom the page and ⌘D does nothing", async () => {
  await backToKennel();
  await key("=");
  await waitFor(async () => (await selected()).pageZoom > 1.05, "page zoomed in");
  await key("0");
  await waitFor(async () => (await selected()).pageZoom === 1, "page zoom reset");
  await key("d");
  await sleep(300);
  assert.equal((await tabs()).length, 1);
});

test("resuming a pup opens it as a tab; resuming again focuses that tab", async () => {
  await backToKennel();
  const id = await clickPup("RATE-LIMITS");
  await waitFor(() => js(`window.kennel.app.sel === ${JSON.stringify(id)}`), "pup selected by a real click");
  assert.ok(await js(`document.querySelector("#card").innerText.includes("RATE-LIMITS")`));
  assert.ok(await cardButton("Resume"));
  await waitFor(async () => (await tabs()).length === 2, "session tab");
  const t = await selected();
  assert.equal(t.session, id);
  await waitFor(async () => (await text(1)).includes(`would run: claude --resume ${id}`), "claude --resume");
  await call("POST", "/snapshot", { index: 1, path: path.join(shots, "app-terminal.png") });

  await backToKennel();
  assert.ok(await cardButton("Resume"));
  await waitFor(async () => (await selected()).session === id, "existing tab focused");
  assert.equal((await tabs()).length, 2, "no duplicate tab");
});

test("double-clicking a pup on the map opens it", async () => {
  await backToKennel();
  const id = await clickPup("DARK-MODE", true);
  await waitFor(async () => (await tabs()).some((t) => t.session === id), "double-click opened a tab");
});

test("a pup running in another terminal still opens in a tab here, and only once", async () => {
  await backToKennel();
  const before = (await tabs()).length;
  const id = await clickPup("PG-UPGRADE");
  assert.ok(await cardButton("Go to"));
  await waitFor(async () => (await tabs()).some((t) => t.session === id), "opened as a tab");
  assert.equal((await tabs()).length, before + 1);
  await backToKennel();
  assert.ok(await cardButton("Go to"));
  await waitFor(async () => (await selected()).session === id, "the same tab is focused again");
  assert.equal((await tabs()).length, before + 1, "no second tab");
});

test("tab shortcuts: ⌘⇧K Kennel, ⌘1..9 by position, ⌘⇧] next", async () => {
  const n = (await tabs()).length;
  assert.ok(n >= 3);
  await key("2");
  await waitFor(async () => (await selected()).index === 1, "tab 2");
  await key("]", ["cmd", "shift"]);
  await waitFor(async () => (await selected()).index === 2, "next tab");
  await key("9");
  await waitFor(async () => (await selected()).index === n - 1, "last tab");
  await key("1");
  await waitFor(async () => (await selected()).kind === "kennel", "tab 1 is kennel");
});

test("when the program exits, its tab closes", async () => {
  const id = await pupId("RATE-LIMITS");
  const i = (await tabs()).find((t) => t.session === id).index;
  await call("POST", "/select", { index: i });
  await call("POST", "/type", { text: "\u0004" });
  await waitFor(async () => !(await tabs()).some((t) => t.session === id), "tab closed after exit");
});

test("things, spawn doors, convert and new pups all open as tabs", async () => {
  await backToKennel();
  let count = (await tabs()).length;
  // a stopped thing starts in a tab
  const tf = await js(`Object.values(window.kennel.app.state.things).find(t => t.name === "tf plan").key`);
  await js(`window.kennel.reveal(${JSON.stringify(tf)}); window.kennel.app.sel = ${JSON.stringify(tf)}; null`);
  await js(`(() => { const p = window.kennel.screenPos(${JSON.stringify(tf)}); return p })()`).then((p) => call("POST", "/click", { x: p.x, y: p.y }));
  assert.ok(await cardButton("Start"));
  await waitFor(async () => (await tabs()).length === count + 1, "thing tab");
  await waitFor(async () => (await text((await selected()).index)).includes("would run: terraform plan -out tf.plan"), "terraform");
  count++;

  // convert answers the page's prompt through the app's native dialog
  await backToKennel();
  await clickPup("IOS-CRASH");
  await call("POST", "/dialogs", { answers: ["codex"] });
  assert.ok(await cardButton("⋯"));
  assert.ok(await js(`(() => { const b = [...document.querySelectorAll("#ctxmenu button")].find(b => b.textContent.includes("Convert")); if (b) b.click(); return !!b })()`));
  await waitFor(async () => (await tabs()).length === count + 1, "convert tab");
  await waitFor(async () => /would run: codex .*continuing work that started in Claude Code/.test(await text((await selected()).index)), "codex handoff");
  count++;

  // a new pup in one of your kennels gets a pre-assigned session id
  await backToKennel();
  const k = await call("POST", "/jsasync", { code: `return (await fetch("/api/kennels", { method: "POST", headers: { "X-Kennel-Token": new URLSearchParams(location.search).get("t") }, body: JSON.stringify({ Name: "App test", Icon: "🧪", Cwd: "/Users/demo/code/atlas" }) })).json()` });
  await waitFor(() => js(`window.kennel.app.rooms.some(r => r.key === "kennel:${k.value.ID}")`), "kennel on the map");
  await js(`(() => { window.kennel.app.sel = "room:kennel:${k.value.ID}"; document.querySelector("#card").hidden = false; })()`);
  await js(`window.kennel.app.sel = "room:kennel:${k.value.ID}"; null`);
  await js(`(() => { const r = window.kennel.app.rooms.find(r => r.key === "kennel:${k.value.ID}"); const st = document.querySelector("#stage"); const a = window.kennel.app; a.cam.x = (r.x + r.w/2)*32 - st.clientWidth/a.cam.z/2; a.cam.y = (r.y + r.h/2)*32 - st.clientHeight/a.cam.z/2; })()`);
  await sleep(200);
  const spot = await js(`window.kennel.roomPos("kennel:${k.value.ID}", 0.9, 0.9)`);
  await call("POST", "/click", { x: spot.x, y: spot.y });
  await waitFor(() => js(`document.querySelector("#card").innerText.includes("App test")`), "room card");
  await call("POST", "/dialogs", { answers: ["NIGHTLY", "summarize overnight alerts"] });
  assert.ok(await cardButton("New pup here"));
  await waitFor(async () => (await tabs()).length === count + 1, "new pup tab");
  const out = await text((await selected()).index);
  assert.match(out, /would run: claude --session-id [0-9a-f-]{36} -n NIGHTLY 'summarize overnight alerts'/);
});

test("the Kennel tab keeps the full web UI: library, panels and search work in the app", async () => {
  await backToKennel();
  await js(`document.querySelector("#libbtn").click()`);
  await waitFor(() => js(`!document.querySelector("#library").hidden && document.querySelectorAll(".librow").length > 0`), "library rows");
  await js(`document.querySelector("#libclose").click()`);
  await js(`document.querySelector("#side .tabs button[data-tab=art]").click()`);
  await waitFor(() => js(`document.querySelector("#art").innerText.includes("PG16 cutover runbook")`), "artifacts panel");
  await js(`document.querySelector("#side .tabs button[data-tab=term]").click()`);
  
});

test("in the app, pups drag to a new spot and rooms resize from their handles", async () => {
  await backToKennel();
  await js(`document.querySelector("#floors .floor[data-floor]").click()`);
  await waitFor(() => js(`window.kennel.app.state.floor === window.kennel.app.state.floors[0].id`), "first floor");
  const id = await pupId("RATE-LIMITS");
  const roomKey = await js(`window.kennel.app.ents.get(${JSON.stringify(id)}).room.key`);
  await js(`(() => { const a = window.kennel.app, st = document.querySelector("#stage"), r = a.rooms.find(r => r.key === ${JSON.stringify(roomKey)}); a.flyTo = null; a.cam.follow = false; a.cam.x = (r.x + r.w/2)*32 - st.clientWidth/a.cam.z/2; a.cam.y = (r.y + r.h/2)*32 - st.clientHeight/a.cam.z/2; })()`);
  await sleep(200);
  const from = await js(`window.kennel.screenPos(${JSON.stringify(id)})`);
  const to = await js(`window.kennel.roomPos(${JSON.stringify(roomKey)}, 0.9, 0.85)`);
  await call("POST", "/drag", { x: from.x, y: from.y, toX: to.x, toY: to.y });
  await waitFor(() => js(`!!((window.kennel.app.rooms.find(r => r.key === ${JSON.stringify(roomKey)}).style || {}).spots || {})[${JSON.stringify(id)}]`), "spot saved");

  const wall = await js(`window.kennel.wallPos(${JSON.stringify(roomKey)})`);
  await call("POST", "/click", { x: wall.x, y: wall.y });
  await waitFor(() => js(`window.kennel.app.sel === "room:" + ${JSON.stringify(roomKey)}`), "room selected");
  const r = await js(`window.kennel.app.rooms.find(r => r.key === ${JSON.stringify(roomKey)})`);
  const z = await js(`window.kennel.app.cam.z`);
  const se = await js(`window.kennel.worldPos(${r.x + r.w}, ${r.y + r.h})`);
  await call("POST", "/drag", { x: se.x, y: se.y, toX: se.x + 3 * 32 * z, toY: se.y + 2 * 32 * z });
  await waitFor(() => js(`(window.kennel.app.rooms.find(r => r.key === ${JSON.stringify(roomKey)}).style.pos || {})[String(window.kennel.app.state.floor)]?.w === ${r.w + 3}`), "room resized");
  await call("POST", "/snapshot", { index: 0, path: path.join(shots, "app-edit.png") });
  await key("z");
  await key("z");
});
