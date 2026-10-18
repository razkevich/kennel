"use strict";
// kennel map — a Gather-style office where Claude sessions are pups and other
// terminals are things. Everything is drawn procedurally; no image assets.

const TOKEN = new URLSearchParams(location.search).get("t") || "";
const TILE = 32;
const SLOT = 3.6; // tiles per pup slot, wide enough for name tags
const MIN_W = 6, MIN_H = 6;

const api = async (path, opts = {}) => {
  const res = await fetch(path, { ...opts, headers: { "X-Kennel-Token": TOKEN, "Content-Type": "application/json", ...(opts.headers || {}) } });
  if (!res.ok) {
    let msg = res.statusText;
    try { msg = (await res.json()).error || msg; } catch (_) {}
    throw new Error(msg);
  }
  return res;
};

const $ = (s) => document.querySelector(s);
const el = (tag, cls, text) => { const e = document.createElement(tag); if (cls) e.className = cls; if (text != null) e.textContent = text; return e; };

const STATUS = {
  working: { label: "working", color: "#43b86e" },
  waiting: { label: "needs you", color: "#e8a417" },
  done: { label: "done", color: "#4f8de0" },
  idle: { label: "idle", color: "#2fb3a4" },
  offline: { label: "asleep", color: "#8a87a6" },
};

// ---------- color helpers ----------
const hex = (h) => [parseInt(h.slice(1, 3), 16), parseInt(h.slice(3, 5), 16), parseInt(h.slice(5, 7), 16)];
const rgb = ([r, g, b]) => `rgb(${r | 0},${g | 0},${b | 0})`;
const mix = (a, b, t) => { const x = hex(a), y = hex(b); return rgb(x.map((v, i) => v + (y[i] - v) * t)); };
const mixHex = (a, b, t) => { const x = hex(a), y = hex(b); return "#" + x.map((v, i) => Math.round(v + (y[i] - v) * t).toString(16).padStart(2, "0")).join(""); };

// ---------- pixel art ----------
const DOG = [
  "..DD......DD..",
  ".DFFD....DFFD.",
  ".DFFFFFFFFFFD.",
  "..FFFFFFFFFF..",
  "..FFEFFFFEFF..",
  "..FFFFNNFFFF..",
  "..FFFWWWWFFF..",
  "...FFFTTFFF...",
  "..FFFFFFFFFF..",
  ".FFFFFFFFFFFFR",
  ".FFFFFFFFFFFF.",
  ".FF.FF..FF.FF.",
];
const HUSKY = [
  "..D........D..",
  ".DFD......DFD.",
  ".DFFFFFFFFFFD.",
  "..FWWFFFFWWF..",
  "..FWEWFFWEWF..",
  "..FWWWNNWWWF..",
  "..FFWWWWWWFF..",
  "...FFWTTWFF...",
  "..FFFFFFFFFF..",
  ".FFFFWWWWFFFFR",
  ".FFFWWWWWWFFF.",
  ".FF.FF..FF.FF.",
];
const DALMATIAN = [
  "..DD......DD..",
  ".DDDD....DDDD.",
  ".DFFFFFSFFFFD.",
  "..FFSFFFFFFF..",
  "..FFEFFFFEFF..",
  "..FFFFNNFFSF..",
  "..FFFWWWWFFF..",
  "...FFFTTFFF...",
  "..FSFFFFFSFF..",
  ".FFFFFSFFFFFFR",
  ".FFSFFFFFFSFF.",
  ".FF.FF..FF.FF.",
];
const BREEDS = { claude: DOG, codex: HUSKY, cursor: DALMATIAN };
const walkOf = (g) => g.slice(0, 11).concat([".F.FF....FF.F."]);
const wagOf = (g) => g.slice(0, 8).concat(["..FFFFFFFFFF.R", g[9].slice(0, 13) + ".", g[10], ".FF.FF..FF.FF."]);
const DOG_WALK = DOG.slice(0, 11).concat([".F.FF....FF.F."]);
const DOG_WAG = DOG.slice(0, 8).concat(["..FFFFFFFFFF.R", ".FFFFFFFFFFFF.", ".FFFFFFFFFFFF.", ".FF.FF..FF.FF."]);
const DOG_SLEEP = [
  "..............",
  "..............",
  "..............",
  "..DD......DD..",
  ".DFFFFFFFFFFD.",
  ".FFFFFFFFFFFF.",
  ".FF--FFFF--FF.",
  ".FFFFFNNFFFFFR",
  "FFFFFFWWFFFFFF",
  "FFFFFFFFFFFFFF",
  ".FFFFFFFFFFFF.",
  "..............",
];
const HUMAN = [
  "....HHHH....",
  "...HHHHHH...",
  "..HHSSSSHH..",
  "...SESSES...",
  "...SSSSSS...",
  "....SMMS....",
  "..CCCCCCCC..",
  ".SCCCCCCCCS.",
  ".SCCCCCCCCS.",
  "..CCCCCCCC..",
  "..PPPPPPPP..",
  "..PP....PP..",
  "..BB....BB..",
];
const HUMAN_WALK = HUMAN.slice(0, 11).concat(["..PP.....PP.", ".BB......BB."]);

function drawPixels(ctx, grid, pal, x, y, s) {
  for (let r = 0; r < grid.length; r++) {
    const row = grid[r];
    for (let c = 0; c < row.length; c++) {
      const col = pal[row[c]];
      if (!col) continue;
      ctx.fillStyle = col;
      ctx.fillRect(Math.round(x + c * s), Math.round(y + r * s), Math.ceil(s), Math.ceil(s));
    }
  }
}

function dogPalette(fur, asleep, agent) {
  if (agent === "codex") fur = "#9db4cf";
  if (agent === "cursor") fur = "#f6f6f2";
  const f = asleep ? mixHex(fur, "#6f6c8f", 0.35) : fur;
  const dark = agent === "cursor" ? "#2a2a2a" : agent === "codex" ? "#4d5f78" : mixHex(f, "#3a2a1a", 0.35);
  return { F: f, D: dark, S: "#2a2a2a", R: f, E: "#1e1e2e", N: "#3b2f2f", W: "#fff4e0", T: "#ff8fa3", "-": "#45475a" };
}
const HUMAN_PAL = { H: "#5b3a29", S: "#f6c9a4", E: "#1e1e2e", M: "#d4776b", C: "#f5a3c7", P: "#4c5a9c", B: "#2a2840" };

// ---------- state ----------
const app = {
  state: null,
  floor: null,
  rooms: [],
  world: { w: 40, h: 30 },
  ents: new Map(), // id -> {kind, id, x, y, tx, ty, home, room, phase}
  sel: null,
  cam: { x: 0, y: 0, z: 1, follow: false },
  you: { x: 4, y: 4, dir: 1, moving: false, t: 0 },
  keys: new Set(),
  t: 0,
  lastClick: { id: null, at: 0 },
};

// ---------- layout ----------
function layout(state) {
  const viewW = Math.max(18, Math.floor(($("#stage").clientWidth || 1000) / (TILE * 0.85)) - 2);
  const sized = state.rooms.map((r) => {
    const n = r.pups.length + r.things.length;
    const maxCols = r.look.Wide ? 6 : 4;
    const cols = Math.max(2, r.minCols || 0, Math.min(maxCols, Math.ceil(Math.sqrt(n * 1.6)) || 2));
    const rows = Math.max(1, r.minRows || 0, Math.ceil(n / cols));
    const out = { ...r, cols, rows, w: Math.ceil(cols * SLOT) + 2, h: rows * 3 + 4 };
    const size = r.style && r.style.pos && r.style.pos[String(state.floor)];
    if (size && size.w) {
      out.w = Math.max(MIN_W, size.w);
      out.cols = Math.max(1, Math.floor((out.w - 2) / SLOT));
      out.rows = Math.max(1, Math.ceil(n / out.cols));
      out.h = Math.max(MIN_H, size.h || 0, out.rows * 3 + 4);
      out.sized = true;
    }
    return out;
  });
  // Rooms the user placed keep their spot; the rest flow into the free space around them.
  const placed = [];
  const overlaps = (x, y, w, h) => placed.some((o) => x < o.x + o.w + 2 && x + w + 2 > o.x && y < o.y + o.h + 2 && y + h + 2 > o.y);
  for (const r of sized) {
    const pos = r.style && r.style.pos && r.style.pos[String(state.floor)];
    if (pos) { r.x = Math.max(1, pos.x); r.y = Math.max(2, pos.y); r.pinned = true; placed.push(r); }
  }
  let x = 2, y = 2, rowH = 0;
  for (const r of sized) {
    if (r.pinned) continue;
    for (let guard = 0; guard < 400; guard++) {
      if (x + r.w > viewW && x > 2) { x = 2; y += Math.max(rowH, 3) + 2; rowH = 0; }
      if (!overlaps(x, y, r.w, r.h)) break;
      x += 2;
    }
    r.x = x; r.y = y;
    placed.push(r);
    x += r.w + 2;
    rowH = Math.max(rowH, r.h);
  }
  const rooms = sized;
  let worldW = 0, worldH = 0;
  for (const r of rooms) { worldW = Math.max(worldW, r.x + r.w + 2); worldH = Math.max(worldH, r.y + r.h + 3); }
  app.rooms = rooms;
  app.world = { w: Math.max(worldW + 1, viewW + 2), h: worldH };

  const seen = new Set();
  for (const r of rooms) {
    const items = r.pups.map((id) => ({ kind: "pup", id })).concat(r.things.map((id) => ({ kind: "thing", id })));
    items.forEach((it, i) => {
      let cx = r.x + 1 + (i % r.cols) * SLOT + SLOT / 2;
      let cy = r.y + 3 + Math.floor(i / r.cols) * 3 + 1.4;
      const spot = r.style && r.style.spots && r.style.spots[it.id];
      if (spot) { const box = interior(r); cx = box.x + spot.x * box.w; cy = box.y + spot.y * box.h; }
      seen.add(it.id);
      let e = app.ents.get(it.id);
      if (!e) {
        e = { kind: it.kind, id: it.id, x: cx, y: cy, tx: cx, ty: cy, phase: Math.random() * 10 };
        app.ents.set(it.id, e);
      }
      e.kind = it.kind;
      e.home = { x: cx, y: cy };
      e.room = r;
      if (e.dragging) return;
      if (Math.hypot(e.x - cx, e.y - cy) > 2.5) { e.tx = cx; e.ty = cy; }
      // Far moves (a room was rebuilt or moved) teleport instead of a long walk.
      if (it.kind === "thing" || Math.hypot(e.x - cx, e.y - cy) > 6) { e.x = e.tx = cx; e.y = e.ty = cy; }
    });
  }
  for (const id of [...app.ents.keys()]) if (!seen.has(id)) app.ents.delete(id);
}

const pupOf = (id) => app.state && app.state.pups[id];
// Your own kennels hold the pups you file; smart ones fill themselves.
const isOwnKennel = (r) => !!(r && r.kennel && !r.smart);
const ownKennels = () => (app.state.kennels || []).filter((k) => !k.smart);
const dimmed = (p) => !!(app.project && p && p.project !== app.project);
const thingOf = (id) => app.state && app.state.things[id];

// ---------- data ----------
// ---------- appearance: "" follows the system, or pinned "light" / "dark" ----------
const systemDark = matchMedia("(prefers-color-scheme: dark)");
let appearance = "";
try { appearance = localStorage.getItem("kennel.appearance") || ""; } catch (_) {}
const isDark = () => appearance === "dark" || (appearance === "" && systemDark.matches);
const APPEARANCE_LABEL = { "": "🖥 Auto", light: "☀️ Light", dark: "🌙 Dark" };

function applyAppearance(a) {
  appearance = a || "";
  if (appearance) document.documentElement.dataset.theme = appearance;
  else delete document.documentElement.dataset.theme;
  PAL = isDark() ? NIGHT : DAY;
  const b = $("#themebtn");
  b.textContent = APPEARANCE_LABEL[appearance];
  b.title = `Appearance: ${APPEARANCE_LABEL[appearance].slice(3)} — click to switch (Shift+T)`;
  try { localStorage.setItem("kennel.appearance", appearance); } catch (_) {}
  if (native) native.postMessage({ type: "appearance", dark: isDark() });
}

async function cycleAppearance() {
  const next = { "": "light", light: "dark", dark: "" }[appearance];
  applyAppearance(next);
  try { await api("/api/appearance", { method: "POST", body: JSON.stringify({ appearance: next }) }); } catch (e) { toast(e.message, true); }
}

async function loadState() {
  const res = await api(app.floor === null ? "/api/state" : `/api/state?floor=${app.floor}`);
  app.state = await res.json();
  app.floor = app.state.floor;
  if ((app.state.appearance || "") !== appearance) applyAppearance(app.state.appearance);
  layout(app.state);
  renderFloors();
  renderProjects();
  renderCounts();
  renderPack();
  renderBuilder();
  renderThemeStatus();
  if (native) native.postMessage({ type: "counts", counts: app.state.counts });
  if (lib.open) refreshLibrary();
  if (app.tab === "jobs") renderJobs();
  if (app.sel) renderCard();
  if (!app.placedYou && app.rooms.length) {
    const r = app.rooms[0];
    app.you.x = r.x + r.w / 2;
    app.you.y = r.y + r.h + 0.8;
    app.placedYou = true;
    frameWorld();
  }
}

// native is the Kennel Mac app's bridge (absent in a normal browser).
const native = window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.kennel;

function listen() {
  const es = new EventSource(`/api/events?t=${TOKEN}`);
  es.addEventListener("change", () => loadState().catch(console.error));
  es.addEventListener("run", (e) => { if (native) native.postMessage({ type: "run", run: JSON.parse(e.data) }); });
  es.onerror = () => { es.close(); setTimeout(listen, 2000); };
}

// ---------- UI chrome ----------
function renderFloors() {
  const nav = $("#floors");
  nav.innerHTML = "";
  for (const f of app.state.floors) {
    const b = el("button", f.id === app.state.floor ? "on" : "", `${f.icon || "🏢"} ${f.name}`);
    b.dataset.floor = f.id;
    b.title = "right-click to rename or remove · drop a kennel here to move it";
    b.onclick = () => goFloor(f.id);
    b.oncontextmenu = (e) => { e.preventDefault(); showMenu(e, floorMenu(f)); };
    nav.appendChild(b);
  }
  const add = el("button", "addfloor", "＋");
  add.title = "new floor";
  add.onclick = newFloor;
  nav.appendChild(add);
}

function goFloor(id) {
  if (id === app.state.floor) return;
  app.floor = id; app.sel = null; app.multi.clear();
  $("#card").hidden = true;
  app.placedYou = false; app.ents.clear();
  loadState();
}

async function newFloor() {
  const name = prompt("Name the new floor", `Floor ${app.state.floors.length + 1}`);
  if (!name || !name.trim()) return;
  try {
    const f = await (await api("/api/floors", { method: "POST", body: JSON.stringify({ Name: name.trim(), Icon: "🏢" }) })).json();
    goFloor(f.ID);
    toast(`🏢 built ${f.Name}`, false, true);
  } catch (e) { toast(e.message, true); }
}

function floorMenu(f) {
  const post = async (action, body) => {
    try { await api(`/api/floors/${f.id}/${action}`, { method: "POST", body: JSON.stringify(body || {}) }); await loadState(); return true; } catch (e) { toast(e.message, true); return false; }
  };
  return [
    ["✎ Rename…", async () => { const n = prompt("Rename floor", f.name); if (n && n.trim() && await post("rename", { name: n.trim() })) toast(`✎ ${n.trim()}`, false, true); }],
    ["🗑 Remove floor", async () => {
      if (app.state.floors.length < 2) { toast("the last floor stays", true); return; }
      if (await post("delete")) { app.ents.clear(); toast(`🗑 removed ${f.name}; its kennels moved to another floor`, false, true); }
    }],
  ];
}

async function moveKennelToFloor(r, floorID) {
  const f = app.state.floors.find((x) => x.id === floorID);
  try {
    await api(`/api/kennels/${r.kennel}/floor`, { method: "POST", body: JSON.stringify({ floor: floorID }) });
    await loadState();
    toast(`🏢 moved ${r.title} to ${f ? f.name : "another floor"}`, false, true);
  } catch (e) { toast(e.message, true); }
}

function floorTabAt(x, y) {
  const b = document.elementFromPoint(x, y);
  return b && b.closest ? b.closest("#floors button[data-floor]") : null;
}

function renderProjects() {
  const sel = $("#projectsel");
  const v = app.project;
  sel.innerHTML = "";
  sel.appendChild(new Option("All projects", ""));
  for (const pr of app.state.projects) sel.appendChild(new Option(`${pr.name} · ${pr.count}`, pr.root));
  if (v && !app.state.projects.some((pr) => pr.root === v)) sel.appendChild(new Option(v.split("/").pop(), v));
  sel.value = v;
  sel.classList.toggle("on", !!v);
}

function renderCounts() {
  const c = app.state.counts;
  const box = $("#counts");
  box.innerHTML = "";
  if (app.state.indexing) box.appendChild(el("span", "pill", `🦴 fetching sessions ${app.state.indexing}`));
  const add = (k, text) => { if (c[k]) box.appendChild(el("span", "pill " + k, text)); };
  add("waiting", `❗ ${c.waiting} needs you`);
  add("working", `⚡ ${c.working} working`);
  add("done", `✓ ${c.done} done`);
  add("idle", `• ${c.idle} idle`);
}

function miniDog(fur, status, agent) {
  const cv = document.createElement("canvas");
  cv.width = 28; cv.height = 24;
  const ctx = cv.getContext("2d");
  drawPixels(ctx, status === "offline" ? DOG_SLEEP : BREEDS[agent] || DOG, dogPalette(fur, status === "offline", agent), 0, 0, 2);
  return cv;
}

// The session tray: everything recent, grouped by what it needs from you. Drag any
// row onto one of your kennels to file it.
const tray = { q: "", closed: {}, more: {} };

function renderPack() {
  const box = $("#pack");
  if (!box.querySelector(".traylist")) {
    box.innerHTML = "";
    const q = el("input");
    q.type = "search";
    q.className = "trayq";
    q.placeholder = "Filter sessions…";
    q.oninput = () => { tray.q = q.value; renderTray(); };
    box.append(q, el("div", "traylist"));
  }
  renderTray();
}

function renderTray() {
  const list = $("#pack .traylist");
  const q = tray.q.trim().toLowerCase();
  const pups = app.state.tray.map(pupOf).filter(Boolean)
    .filter((p) => !dimmed(p))
    .filter((p) => !q || `${p.title} ${p.repo} ${p.kennel} ${p.snippet} ${p.branch}`.toLowerCase().includes(q));
  const used = new Set();
  const take = (f) => pups.filter((p) => !used.has(p.id) && f(p)).map((p) => (used.add(p.id), p));
  const groups = [
    ["waiting", "❗ Needs you", take((p) => p.status === "waiting")],
    ["new", "📥 New, not in a kennel", take((p) => !p.kennel)],
    ["working", "⚡ Working", take((p) => p.status === "working")],
    ["recent", "🕘 Recent", take(() => true)],
  ];
  // Rebuilding the list under the cursor would break hovers and drags, so only redraw on change.
  const sig = JSON.stringify([q, app.project, app.sel, tray.closed, tray.more, groups.map(([k, , items]) => [k, items.map((p) => [p.id, p.title, p.status, p.kennel, p.updated, p.starred])])]);
  if (sig === tray.sig) return;
  tray.sig = sig;
  list.innerHTML = "";
  for (const [key, label, items] of groups) {
    if (!items.length) continue;
    const g = el("div", "group" + (tray.closed[key] ? " closed" : ""));
    const h = el("h4", "", `${label} · ${items.length}`);
    h.onclick = () => { tray.closed[key] = !tray.closed[key]; renderTray(); };
    g.appendChild(h);
    if (!tray.closed[key]) {
      const limit = tray.more[key] ? items.length : 25;
      for (const p of items.slice(0, limit)) g.appendChild(memberRow(p));
      if (items.length > limit) {
        const more = el("button", "more", `show ${items.length - limit} more`);
        more.onclick = () => { tray.more[key] = true; renderTray(); };
        g.appendChild(more);
      }
    }
    list.appendChild(g);
  }
  if (!list.children.length) list.appendChild(el("div", "empty", q || app.project ? "Nothing matches." : "No sessions in the last two weeks."));
}

function memberRow(p) {
  const row = el("div", "member" + (app.sel === p.id ? " on" : ""));
  row.dataset.id = p.id;
  const who = el("div", "who");
  who.appendChild(el("div", "name", (p.starred ? "★ " : "") + p.title));
  who.appendChild(el("div", "what", p.snippet || p.repo));
  const side = el("div", "side");
  if (p.kennel) side.appendChild(el("span", "pill kennel", p.kennel));
  side.appendChild(el("span", "age", relAge(p.updated)));
  const dot = el("span", "dot");
  dot.style.background = STATUS[p.status].color;
  row.append(miniDog(p.fur, p.status, p.agent), who, side, dot);
  row.title = "click to see it · double-click to open · drag onto a kennel to file";
  row.onclick = () => select(p.id, app.ents.has(p.id));
  row.ondblclick = () => act("open", p.id);
  draggablePup(row, p);
  return row;
}

function ago(ms) {
  const d = (app.state.now - ms) / 1000;
  if (d < 60) return "now";
  if (d < 3600) return `${Math.floor(d / 60)}m ago`;
  if (d < 86400) return `${Math.floor(d / 3600)}h ago`;
  return `${Math.floor(d / 86400)}d ago`;
}

// frameWorld fits the office in view (or shows its top-left when it's too big).
function frameWorld() {
  const st = $("#stage");
  const vw = st.clientWidth, vh = st.clientHeight;
  const W = app.world.w * TILE, H = app.world.h * TILE;
  const gutter = 56; // keeps the tool palette off the leftmost rooms
  app.cam.z = Math.max(0.75, Math.min(1.25, (vw - gutter) / W));
  app.cam.x = W * app.cam.z < vw - gutter ? -(vw / app.cam.z - W) / 2 : -gutter / app.cam.z;
  app.cam.y = H * app.cam.z < vh ? -(vh / app.cam.z - H) / 2 : 0;
  app.cam.follow = false;
  app.flyTo = null;
}

function select(id, fly) {
  app.sel = id;
  const e = app.ents.get(id);
  if (fly && e) {
    app.cam.follow = false;
    app.flyTo = { x: e.x, y: e.y };
    app.you.x = e.x; app.you.y = e.y + 1.2;
  }
  renderCard();
  for (const m of document.querySelectorAll(".member")) m.classList.toggle("on", m.dataset.id === id);
}

function renderCard() {
  const card = $("#card");
  if (app.sel && app.sel.startsWith("room:")) { renderRoomCard(app.sel.slice(5)); return; }
  const p = pupOf(app.sel), t = thingOf(app.sel);
  if (!p && !t) { card.hidden = true; return; }
  card.innerHTML = "";
  card.hidden = false;
  const close = el("button", "x", "✕");
  close.onclick = () => { app.sel = null; card.hidden = true; };
  if (p) {
    const st = STATUS[p.status];
    const row = el("div", "row");
    row.appendChild(miniDog(p.fur, p.status, p.agent));
    row.appendChild(el("span", "title", p.title));
    const pill = el("span", "status", st.label);
    pill.style.background = st.color;
    row.append(pill, close);
    const meta = [{ claude: "Claude Code", codex: "Codex", cursor: "Cursor" }[p.agent] || "Claude Code", p.repo, p.branch, `${p.messages} msgs`, ago(p.updated)];
    if (p.prs.length) meta.push("PR #" + p.prs[p.prs.length - 1].split("/").pop());
    if (p.kennel) meta.push("🏠 " + p.kennel);
    const actions = el("div", "actions");
    const btn = (label, fn, primary) => { const b = el("button", primary ? "primary" : "", label); b.onclick = fn; actions.appendChild(b); };
    btn(p.status === "offline" ? "▶ Resume" : "↗ Go to", () => act("open", p.id), true);
    btn("⑂ Fork", () => act("fork", p.id));
    btn("⧉ Clone", () => { const cwd = prompt(`Clone ${p.title} into which folder?`, p.cwd); if (cwd) act("clone", p.id, { cwd }); });
    btn("⇄ Merge", () => { const other = prompt(`Merge ${p.title} with which pup? (name or id)`); if (other) act("merge", p.id, { with: [other] }); });
    btn("✨ Distill", () => act("distill", p.id));
    btn("↔ Convert", () => {
      const to = prompt(`Continue ${p.title} in which agent? (claude, codex, cursor)`, p.agent === "claude" ? "codex" : "claude");
      if (to) act("convert", p.id, { agent: to.trim().toLowerCase() });
    });
    btn("⬇ Export", () => act("export", p.id));
    btn("📜 Read", () => openTranscript(p.id));
    btn(p.starred ? "★ Unstar" : "☆ Star", () => act("star", p.id));
    if (p.kennel) btn("✕ Remove from " + p.kennel, () => unfile([p.id]));
    btn(p.archived ? "↩ Restore" : "🗑 Archive", () => act("archive", p.id));
    if (p.pane) btn("💬 Reply", () => { const text = prompt(`Reply to ${p.title}`); if (text) act("reply", p.id, { text }); });
    const move = el("label", "move", "🏠 kennel");
    const sel = el("select");
    sel.appendChild(new Option("— none —", "0"));
    for (const k of ownKennels()) sel.appendChild(new Option(`${k.icon} ${k.name}`, String(k.id), false, k.name === p.kennel));
    sel.onchange = () => act("move", p.id, { kennel: Number(sel.value) });
    move.appendChild(sel);
    card.append(row, el("div", "meta", meta.filter(Boolean).join(" · ")), el("div", "snip", p.snippet), actions, move);
  } else {
    const row = el("div", "row");
    const ic = el("span", "", t.icon); ic.style.fontSize = "22px";
    row.append(ic, el("span", "title", t.name));
    const pill = el("span", "status", t.running ? "running" : "stopped");
    pill.style.background = t.running ? STATUS.working.color : STATUS.offline.color;
    row.append(pill, close);
    const actions = el("div", "actions");
    const b1 = el("button", "primary", t.running ? "↗ Focus" : "▶ Start");
    b1.onclick = () => thingAct("start", t.key);
    actions.appendChild(b1);
    if (t.running) { const b2 = el("button", "", "✋ Stop"); b2.onclick = () => { if (confirm(`Stop ${t.name}?`)) thingAct("stop", t.key); }; actions.appendChild(b2); }
    card.append(row, el("div", "meta", `${t.kind} · ${t.cwd}`), el("div", "snip", "$ " + t.command), actions);
  }
}

// renderRoomCard lets you start a pup in a room, or pull any session into it.
function renderRoomCard(key) {
  const card = $("#card");
  const r = roomByKey(key);
  if (!r) { card.hidden = true; return; }
  card.innerHTML = "";
  card.hidden = false;
  const row = el("div", "row");
  row.append(el("span", "title", `${r.icon} ${r.title}`), el("span", "meta", `${r.pups.length} pups`));
  const close = el("button", "x", "✕");
  close.onclick = () => { app.sel = null; card.hidden = true; };
  row.appendChild(close);
  const actions = el("div", "actions");
  const isKennel = r.key.startsWith("kennel:");
  const b1 = el("button", "primary", "🐣 New pup here");
  b1.onclick = async () => {
    const title = prompt(`Name the new pup in ${r.title} (optional)`, "");
    if (title === null) return;
    const first = prompt("First thing to ask it (optional)", "") || "";
    try {
      const res = await api("/api/room/new", { method: "POST", body: JSON.stringify({ key: r.key, title, prompt: first }) });
      toast((await res.json()).message || "🐣 started");
    } catch (e) { toast(e.message, true); }
  };
  actions.appendChild(b1);
  const b3 = el("button", "", "🔨 Shape");
  b3.onclick = () => { setBuild(true); build.room = r.key; renderBuilder(); };
  actions.appendChild(b3);
  if (r.kennel) {
    const rn = el("button", "", "✎ Rename");
    rn.onclick = () => renameKennel(r);
    const del = el("button", "", "🗑 Delete");
    del.onclick = () => deleteKennel(r);
    actions.append(rn, del);
  }
  card.append(row, el("div", "snip", r.smart ? `Fills itself: ${r.blurb}.` : r.blurb && !r.blurb.startsWith("/") ? r.blurb : "Your kennel. Drag sessions in from the tray."), actions);
  if (r.kennel && app.state.floors.length > 1) {
    const mv = el("label", "move", "🏢 floor");
    const sel = el("select");
    for (const f of app.state.floors) sel.appendChild(new Option(`${f.icon} ${f.name}`, String(f.id), false, f.id === app.state.floor));
    sel.onchange = () => moveKennelToFloor(r, Number(sel.value));
    mv.appendChild(sel);
    card.appendChild(mv);
  }
  if (isKennel && !r.smart) {
    const kid = Number(r.key.slice(7));
    const box = el("div", "addpups");
    const q = el("input");
    q.placeholder = "＋ add pups: search any session…";
    const list = el("div", "addlist");
    let seq = 0;
    q.oninput = async () => {
      const mine = ++seq;
      if (!q.value.trim()) { list.innerHTML = ""; return; }
      const items = await (await api(`/api/search?q=${encodeURIComponent(q.value)}`)).json();
      if (mine !== seq) return;
      list.innerHTML = "";
      for (const it of items.slice(0, 8)) {
        const p = it.pup;
        const rowp = el("div", "addrow");
        rowp.append(miniDog(p.fur, p.status, p.agent), el("span", "", p.title), el("span", "m", p.kennel ? "🏠 " + p.kennel : p.repo));
        const add = el("button", "", p.kennel === r.title ? "✓" : "＋");
        add.disabled = p.kennel === r.title;
        add.onclick = async () => { await act("move", p.id, { kennel: kid }); q.oninput(); };
        rowp.appendChild(add);
        list.appendChild(rowp);
      }
    };
    box.append(q, list);
    card.appendChild(box);
  }
}

async function act(action, id, body) {
  try {
    const res = await api(`/api/pup/${id}/${action}`, { method: "POST", body: JSON.stringify(body || {}) });
    const out = await res.json();
    await loadState();
    const verbs = { star: "★ starred", archive: "🗑 archived", rename: "✏️ renamed", clone: "", note: "📝 noted", move: "" };
    const msg = out.message || verbs[action] || "";
    if (msg) toast(msg, false, action in verbs);
  } catch (e) { toast(e.message, true); }
}

async function thingAct(action, key) {
  try {
    const res = await api(`/api/thing/${action}`, { method: "POST", body: JSON.stringify({ key }) });
    const out = await res.json();
    if (out.message) toast(out.message);
    await loadState();
  } catch (e) { toast(e.message, true); }
}

let toastTimer;
function toast(text, err, undoable) {
  const t = $("#toast");
  t.textContent = text;
  t.className = err ? "err" : "";
  if (undoable && app.state && app.state.undo) {
    const b = el("button", "undo", "↶ Undo");
    b.title = "undo " + app.state.undo + " (⌘Z)";
    b.onclick = (e) => { e.stopPropagation(); undo(); };
    t.appendChild(b);
  }
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), undoable ? 6000 : 3500);
}

async function undo() {
  try {
    const res = await api("/api/undo", { method: "POST" });
    const out = await res.json();
    await Promise.all([loadState(), loadTreats()]);
    toast(out.message, false, true);
  } catch (e) { toast(e.message, true); }
}

// ---------- search ----------
function marked(snippet) {
  const frag = document.createDocumentFragment();
  const parts = snippet.split(/(\x01[^\x02]*\x02)/);
  for (const part of parts) {
    if (part.startsWith("\x01")) frag.appendChild(el("mark", "", part.slice(1, -1)));
    else if (part) frag.appendChild(document.createTextNode(part));
  }
  return frag;
}

let searchTimer, searchSeq = 0;
$("#q").addEventListener("input", () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(runSearch, 120);
});
$("#q").addEventListener("keydown", (e) => {
  const items = [...document.querySelectorAll(".result")];
  let i = items.findIndex((x) => x.classList.contains("on"));
  if (e.key === "ArrowDown" || e.key === "ArrowUp") {
    e.preventDefault();
    i = e.key === "ArrowDown" ? Math.min(items.length - 1, i + 1) : Math.max(0, i - 1);
    items.forEach((x, j) => x.classList.toggle("on", j === i));
  } else if (e.key === "Enter" && items.length) {
    (items[Math.max(0, i)]).click();
  } else if (e.key === "Escape") {
    $("#q").value = ""; $("#results").hidden = true; $("#q").blur();
  }
});

async function runSearch() {
  const q = $("#q").value.trim();
  const box = $("#results");
  if (!q) { box.hidden = true; return; }
  const seq = ++searchSeq;
  const res = await api(`/api/search?q=${encodeURIComponent(q)}`);
  const items = await res.json();
  if (seq !== searchSeq) return;
  box.innerHTML = "";
  box.hidden = false;
  if (!items.length) { box.appendChild(el("div", "result", "No pups match — try Ask in the sidebar")); return; }
  items.forEach((it, i) => {
    const p = it.pup;
    const r = el("div", "result" + (i === 0 ? " on" : ""));
    const t = el("div", "t");
    t.append(miniDog(p.fur, p.status, p.agent), el("span", "", p.title));
    const dot = el("span", "dot"); dot.style.background = STATUS[p.status].color; t.appendChild(dot);
    r.appendChild(t);
    r.appendChild(el("div", "m", [p.repo, p.branch, ago(p.updated)].filter(Boolean).join(" · ")));
    for (const h of it.hits.slice(0, 2)) { const hd = el("div", "h"); hd.appendChild(marked(h.snippet)); r.appendChild(hd); }
    r.onclick = () => {
      box.hidden = true;
      if (!pupOf(p.id)) { app.state.pups[p.id] = p; }
      select(p.id, true);
    };
    box.appendChild(r);
  });
}

// ---------- ask ----------
const PANELS = ["pack", "ask", "build", "jobs", "art"];
function showTab(tab) {
  if (tab === "build") { setBuild(true); return; }
  if (build.on) setBuild(false);
  for (const x of document.querySelectorAll("#side .tabs button")) x.classList.toggle("on", x.dataset.tab === tab);
  for (const p of PANELS) $("#" + p).hidden = p !== tab;
  $("#treats").hidden = tab !== "pack";
  if (tab === "ask") $("#askq").focus();
  if (tab === "jobs") renderJobs();
  if (tab === "art") renderArt();
  app.tab = tab;
}
for (const b of document.querySelectorAll("#side .tabs button")) b.onclick = () => showTab(b.dataset.tab);

$("#askform").addEventListener("submit", async (e) => {
  e.preventDefault();
  const q = $("#askq").value.trim();
  if (!q) return;
  $("#answer").textContent = "";
  $("#sources").innerHTML = "";
  $("#askstage").textContent = "▼•ᴥ•▼ sniffing around…";
  try {
    const res = await api("/api/ask", { method: "POST", body: JSON.stringify({ q }) });
    const reader = res.body.getReader();
    const dec = new TextDecoder();
    let buf = "";
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += dec.decode(value, { stream: true });
      let i;
      while ((i = buf.indexOf("\n\n")) >= 0) {
        const chunk = buf.slice(0, i);
        buf = buf.slice(i + 2);
        if (chunk.startsWith("data: ")) showAsk(JSON.parse(chunk.slice(6)));
      }
    }
  } catch (err) { $("#askstage").textContent = "✗ " + err.message; }
});

function showAsk(ev) {
  $("#askstage").textContent = ev.error ? "✗ " + ev.error : ev.done ? (ev.sources && ev.sources.length ? `answered from ${new Set(ev.sources.map((s) => s.id)).size} sessions` : "") : "▼•ᴥ•▼ " + ev.stage + "…";
  const ans = $("#answer");
  ans.innerHTML = "";
  const parts = (ev.text || "").split(/(\[\d+\])/);
  for (const part of parts) {
    const m = part.match(/^\[(\d+)\]$/);
    if (m) {
      const c = el("span", "cite", part);
      const src = (ev.sources || []).find((s) => s.n === +m[1]);
      if (src) c.onclick = () => jumpToSession(src.id);
      ans.appendChild(c);
    } else ans.appendChild(document.createTextNode(part));
  }
  const box = $("#sources");
  box.innerHTML = "";
  for (const s of ev.sources || []) {
    const d = el("div", "source");
    d.appendChild(el("b", "", `[${s.n}] `));
    d.appendChild(document.createTextNode(`${s.title} · ${s.repo}`));
    d.appendChild(el("div", "ex", s.excerpt));
    d.onclick = () => jumpToSession(s.id);
    box.appendChild(d);
  }
}

function jumpToSession(id) {
  if (app.ents.has(id)) select(id, true);
  else openTranscript(id);
}

// ---------- transcript ----------
async function openTranscript(id) {
  try {
    const res = await api(`/api/transcript/${id}`);
    const data = await res.json();
    $("#tr-title").textContent = "📜 " + data.pup.title;
    const body = $("#tr-body");
    body.innerHTML = "";
    for (const c of data.chunks) {
      const m = el("div", "msg " + c.role);
      if (c.role === "tool") { m.textContent = "⚙ " + c.text; body.appendChild(m); continue; }
      m.appendChild(el("span", "who", { user: "you", assistant: "▼•ᴥ•▼ claude", recap: "✦ recap" }[c.role] || c.role));
      m.appendChild(el("div", "body", c.text));
      body.appendChild(m);
    }
    $("#transcript").hidden = false;
    $("#tr-find").value = "";
  } catch (e) { toast(e.message, true); }
}
$("#tr-close").onclick = () => ($("#transcript").hidden = true);
$("#transcript").addEventListener("click", (e) => { if (e.target.id === "transcript") $("#transcript").hidden = true; });
$("#tr-find").addEventListener("input", () => {
  const q = $("#tr-find").value.toLowerCase();
  let first = null;
  for (const m of document.querySelectorAll("#tr-body .msg")) {
    const hit = q && m.textContent.toLowerCase().includes(q);
    m.style.outline = hit ? "3px solid #ffd66b" : "";
    if (hit && !first) first = m;
  }
  if (first) first.scrollIntoView({ block: "center" });
});

// ---------- input ----------
const canvas = $("#map");
const ctx = canvas.getContext("2d");

function screenToWorld(px, py) {
  return { x: (px / app.cam.z + app.cam.x) / TILE, y: (py / app.cam.z + app.cam.y) / TILE };
}

function entityAt(px, py) {
  const w = screenToWorld(px, py);
  let best = null, bd = 1.3;
  for (const e of app.ents.values()) {
    const d = Math.hypot(e.x - w.x, e.y - 0.4 - w.y);
    if (d < bd) { bd = d; best = e; }
  }
  return best;
}

// Pointer model, after Excalidraw: a select tool that drags pups and moves/resizes rooms,
// a hand tool (or held space / middle button) that pans, and a room tool that draws kennels.
app.tool = "select";
app.multi = new Set();
let drag = null;
let spaceHeld = false;

const HANDLES = ["nw", "n", "ne", "e", "se", "s", "sw", "w"];
function handlePoints(r) {
  const x0 = r.x, y0 = r.y, x1 = r.x + r.w, y1 = r.y + r.h, mx = (x0 + x1) / 2, my = (y0 + y1) / 2;
  return { nw: [x0, y0], n: [mx, y0], ne: [x1, y0], e: [x1, my], se: [x1, y1], s: [mx, y1], sw: [x0, y1], w: [x0, my] };
}
function handleAt(r, w) {
  const pts = handlePoints(r), tol = 0.45 / Math.max(0.6, app.cam.z);
  return HANDLES.find((h) => Math.abs(pts[h][0] - w.x) < tol && Math.abs(pts[h][1] - w.y) < tol) || null;
}
const selectedRoom = () => (app.sel && app.sel.startsWith("room:") ? roomByKey(app.sel.slice(5)) : null);
const onWall = (r, w) => w.y < r.y + WALL + 0.2;
const canvasPoint = (e) => { const rc = canvas.getBoundingClientRect(); return { sx: e.clientX - rc.left, sy: e.clientY - rc.top }; };

function setTool(t) {
  app.tool = t;
  document.querySelectorAll("#tools [data-tool]").forEach((b) => b.classList.toggle("on", b.dataset.tool === t));
  canvas.dataset.tool = t;
}

canvas.addEventListener("mousedown", (e) => {
  hideMenu();
  if (e.button !== 0 && e.button !== 1) return;
  const { sx, sy } = canvasPoint(e);
  const w = screenToWorld(sx, sy);
  const pan = e.button === 1 || spaceHeld || app.tool === "hand";
  if (!pan && build.on && !build.carry) {
    const f = fixtureAt(w.x, w.y);
    if (f) { build.drag = { fixture: f, moved: false }; return; }
  }
  if (!pan && app.tool === "room") {
    drag = { kind: "create", x0: Math.round(w.x), y0: Math.round(w.y), x1: Math.round(w.x), y1: Math.round(w.y), sx: e.clientX, sy: e.clientY, moved: false };
    return;
  }
  if (!pan && !build.carry) {
    const sr = selectedRoom();
    const h = sr && handleAt(sr, w);
    if (h) { drag = { kind: "resize", room: sr, handle: h, orig: { x: sr.x, y: sr.y, w: sr.w, h: sr.h }, sx: e.clientX, sy: e.clientY, moved: false }; return; }
    const hit = entityAt(sx, sy);
    if (hit) {
      const group = app.multi.has(hit.id) ? [...app.multi].map((id) => app.ents.get(id)).filter(Boolean) : [hit];
      drag = { kind: "pup", ent: hit, group: group.map((g) => ({ ent: g, ox: g.x - w.x, oy: g.y - w.y })), sx: e.clientX, sy: e.clientY, moved: false };
      return;
    }
    const r = roomAt(w.x, w.y);
    if (r && (onWall(r, w) || (sr === r && !e.shiftKey))) {
      drag = { kind: "room", room: r, dx: w.x - r.x, dy: w.y - r.y, sx: e.clientX, sy: e.clientY, moved: false };
      return;
    }
    if (r || e.shiftKey) {
      drag = { kind: "marquee", x0: w.x, y0: w.y, x1: w.x, y1: w.y, room: r, sx: e.clientX, sy: e.clientY, moved: false, add: e.shiftKey };
      return;
    }
  }
  drag = { kind: "pan", sx: e.clientX, sy: e.clientY, cx: app.cam.x, cy: app.cam.y, moved: false };
});

window.addEventListener("mousemove", (e) => {
  const { sx, sy } = canvasPoint(e);
  const w = screenToWorld(sx, sy);
  if (build.drag) {
    const d = build.drag;
    d.moved = true;
    const box = interior(d.fixture.room);
    d.fixture.item.x = Math.max(0, Math.min(1, (w.x - box.x) / box.w));
    d.fixture.item.y = Math.max(0, Math.min(1, (w.y + 0.4 - box.y) / box.h));
    return;
  }
  if (!drag) { if (e.target === canvas) hover(w, sx, sy); return; }
  if (!drag.moved && Math.abs(e.clientX - drag.sx) + Math.abs(e.clientY - drag.sy) > 4) {
    drag.moved = true;
    app.cam.follow = false; app.flyTo = null;
    canvas.classList.add("dragging");
  }
  if (!drag.moved) return;
  switch (drag.kind) {
    case "pan":
      app.cam.x = drag.cx - (e.clientX - drag.sx) / app.cam.z;
      app.cam.y = drag.cy - (e.clientY - drag.sy) / app.cam.z;
      break;
    case "pup":
      for (const g of drag.group) { g.ent.dragging = true; g.ent.x = g.ent.tx = w.x + g.ox; g.ent.y = g.ent.ty = w.y + g.oy; }
      { const r = roomAt(w.x, w.y); app.dropRoom = r && r !== drag.ent.room ? r.key : null; }
      break;
    case "room": {
      const r = drag.room;
      const tab = r.kennel && floorTabAt(e.clientX, e.clientY);
      document.querySelectorAll("#floors button").forEach((b) => b.classList.toggle("droptarget", b === tab));
      r.x = Math.max(1, Math.round(w.x - drag.dx));
      r.y = Math.max(2, Math.round(w.y - drag.dy));
      snapGuides(r);
      break;
    }
    case "resize": {
      const r = drag.room, o = drag.orig, h = drag.handle;
      const gx = Math.round(w.x), gy = Math.round(w.y);
      if (h.includes("e")) r.w = Math.max(MIN_W, gx - o.x);
      if (h.includes("s")) r.h = Math.max(MIN_H, gy - o.y);
      if (h.includes("w")) { const nx = Math.min(gx, o.x + o.w - MIN_W); r.x = Math.max(1, nx); r.w = o.x + o.w - r.x; }
      if (h.includes("n")) { const ny = Math.min(gy, o.y + o.h - MIN_H); r.y = Math.max(2, ny); r.h = o.y + o.h - r.y; }
      snapGuides(r);
      break;
    }
    case "marquee":
      drag.x1 = w.x; drag.y1 = w.y;
      app.multi = new Set(drag.add ? app.multi : []);
      for (const ent of app.ents.values()) if (ent.kind === "pup" && inRect(ent, drag)) app.multi.add(ent.id);
      break;
    case "create":
      drag.x1 = Math.round(w.x); drag.y1 = Math.round(w.y);
      break;
  }
});

window.addEventListener("mouseup", async (e) => {
  if (build.drag) {
    const d = build.drag;
    build.drag = null;
    if (d.moved) saveStyle(d.fixture.room);
    else { build.room = d.fixture.room.key; renderBuilder(); }
    return;
  }
  const d = drag;
  drag = null;
  app.guides = null;
  canvas.classList.remove("dragging");
  if (!d) return;
  const { sx, sy } = canvasPoint(e);
  const w = screenToWorld(sx, sy);
  if (!d.moved) {
    if (e.target === canvas && d.kind !== "create") click(sx, sy, w, e);
    return;
  }
  switch (d.kind) {
    case "pup": await dropPups(d, w); break;
    case "room": {
      const tab = d.room.kennel && floorTabAt(e.clientX, e.clientY);
      document.querySelectorAll("#floors button.droptarget").forEach((b) => b.classList.remove("droptarget"));
      if (tab && Number(tab.dataset.floor) !== app.state.floor) await moveKennelToFloor(d.room, Number(tab.dataset.floor));
      else if (tab) await loadState();
      else await saveRoomBox(d.room, false);
      break;
    }
    case "resize": await saveRoomBox(d.room, true); break;
    case "marquee":
      if (app.multi.size === 1) { select([...app.multi][0], false); app.multi.clear(); }
      else if (app.multi.size) { app.sel = null; renderCard(); }
      break;
    case "create": await createKennel(d); break;
  }
});

function click(sx, sy, w, e) {
  if (build.on) {
    const r = roomAt(w.x, w.y);
    const hit = !build.carry && entityAt(sx, sy);
    if (hit) { select(hit.id, false); return; }
    if (r && build.carry) placeIn(r, build.carry, w);
    else if (r) { build.room = r.key; app.sel = "room:" + r.key; renderBuilder(); renderCard(); }
    return;
  }
  const hit = entityAt(sx, sy);
  const fx = !hit && fixtureAt(w.x, w.y);
  if (fx) { useFixture(fx); return; }
  if (hit) {
    if (e.shiftKey && hit.kind === "pup") {
      if (app.sel && app.ents.has(app.sel)) app.multi.add(app.sel);
      app.multi.has(hit.id) ? app.multi.delete(hit.id) : app.multi.add(hit.id);
      app.sel = null; renderCard();
      return;
    }
    app.multi.clear();
    const now = performance.now();
    const dbl = app.lastClick.id === hit.id && now - app.lastClick.at < 450;
    app.lastClick = { id: hit.id, at: now };
    select(hit.id, false);
    if (dbl) interact(hit);
    return;
  }
  app.multi.clear();
  const room = roomAt(w.x, w.y);
  app.sel = room ? "room:" + room.key : null;
  renderCard();
}

const inRect = (p, d) => p.x >= Math.min(d.x0, d.x1) && p.x <= Math.max(d.x0, d.x1) && p.y - 0.4 >= Math.min(d.y0, d.y1) && p.y - 0.4 <= Math.max(d.y0, d.y1);

// Snap a moving room's edges to other rooms' edges, and remember the guides to draw.
function snapGuides(r) {
  const guides = [];
  for (const o of app.rooms) {
    if (o === r) continue;
    for (const [a, b] of [[r.x, o.x], [r.x + r.w, o.x + o.w], [r.x, o.x + o.w], [r.x + r.w, o.x]]) if (a === b) guides.push({ x: a, y0: Math.min(r.y, o.y), y1: Math.max(r.y + r.h, o.y + o.h) });
    for (const [a, b] of [[r.y, o.y], [r.y + r.h, o.y + o.h]]) if (a === b) guides.push({ y: a, x0: Math.min(r.x, o.x), x1: Math.max(r.x + r.w, o.x + o.w) });
  }
  app.guides = guides;
}

async function saveRoomBox(r, sized) {
  const st = styleOf(r);
  st.pos = st.pos || {};
  const prev = st.pos[String(app.state.floor)] || {};
  const pos = { x: r.x, y: r.y };
  if (sized) { pos.w = r.w; pos.h = r.h; } else if (prev.w) { pos.w = prev.w; pos.h = prev.h; }
  st.pos[String(app.state.floor)] = pos;
  await saveStyle(r);
  if (build.on) { build.room = r.key; renderBuilder(); }
}

function spotIn(r, ent) {
  const box = interior(r);
  return { x: Math.max(0.03, Math.min(0.97, (ent.x - box.x) / box.w)), y: Math.max(0.05, Math.min(1, (ent.y - box.y) / box.h)) };
}

// Dropping pups: inside their own room they stay where you put them; on another kennel
// they're filed there; anywhere else they walk back.
async function dropPups(d, w) {
  app.dropRoom = null;
  const target = roomAt(w.x, w.y);
  const from = d.ent.room;
  const back = () => { for (const g of d.group) { g.ent.dragging = false; if (g.ent.home) { g.ent.tx = g.ent.home.x; g.ent.ty = g.ent.home.y; } } };
  if (!target) {
    const out = d.group.map((g) => g.ent.id).filter(inKennel);
    if (out.length) { for (const g of d.group) g.ent.dragging = false; await unfile(out); return; }
    back(); toast("drop pups inside a room", true); return;
  }
  if (target === from) {
    const st = styleOf(target);
    st.spots = st.spots || {};
    for (const g of d.group) if (g.ent.room === target) st.spots[g.ent.id] = spotIn(target, g.ent);
    for (const g of d.group) g.ent.dragging = false;
    await saveStyle(target, true);
    return;
  }
  if (!isOwnKennel(target)) {
    back();
    toast(`${target.title} fills itself; drop pups into one of your own kennels`, true);
    return;
  }
  const st = styleOf(target);
  st.spots = st.spots || {};
  for (const g of d.group) st.spots[g.ent.id] = spotIn(target, g.ent);
  try {
    await api("/api/room/style", { method: "POST", body: JSON.stringify({ key: target.key, style: st }) });
    for (const g of d.group) await act("move", g.ent.id, { kennel: Number(target.key.slice(7)) });
  } catch (err) { toast(err.message, true); }
  for (const g of d.group) g.ent.dragging = false;
  toast(`🏠 ${d.group.length > 1 ? d.group.length + " pups" : "filed"} into ${target.icon} ${target.title}`, false, true);
  app.multi.clear();
}

const inKennel = (id) => { const e = app.ents.get(id); return isOwnKennel(e && e.room); };

// Removing a pup from a kennel only unfiles it: the session goes back to 📥 New.
async function unfile(ids) {
  const names = [...new Set(ids.map((id) => pupOf(id) && pupOf(id).kennel).filter(Boolean))];
  const one = ids.length === 1 && pupOf(ids[0]) ? pupOf(ids[0]).title : `${ids.length} pups`;
  for (const id of ids) await act("move", id, { kennel: 0 });
  app.multi.clear();
  toast(`✕ removed ${one} from ${names.join(", ") || "its kennel"}`, false, true);
}

// Delete takes pups out of the kennel they're in; anywhere else it archives them.
async function deletePups(ids) {
  const filed = ids.filter(inKennel), loose = ids.filter((id) => !inKennel(id));
  if (filed.length) await unfile(filed);
  for (const id of loose) await act("archive", id);
  app.multi.clear();
}

async function createKennel(d) {
  setTool("select");
  const x = Math.min(d.x0, d.x1), y = Math.max(2, Math.min(d.y0, d.y1));
  const w = Math.max(MIN_W, Math.abs(d.x1 - d.x0)), h = Math.max(MIN_H, Math.abs(d.y1 - d.y0));
  const name = prompt("Name the new kennel", "New kennel");
  if (!name || !name.trim()) return;
  try {
    const k = await (await api("/api/kennels", { method: "POST", body: JSON.stringify({ Name: name.trim(), Icon: "🏠" }) })).json();
    const pos = {};
    pos[String(app.state.floor)] = { x, y, w, h };
    await api("/api/room/style", { method: "POST", body: JSON.stringify({ key: "kennel:" + k.ID, style: { pos } }) });
    await loadState();
    app.sel = "room:kennel:" + k.ID;
    renderCard();
    toast(`🏠 built ${name.trim()}`, false, true);
  } catch (err) { toast(err.message, true); }
}

const CURSORS = { nw: "nwse-resize", se: "nwse-resize", ne: "nesw-resize", sw: "nesw-resize", n: "ns-resize", s: "ns-resize", e: "ew-resize", w: "ew-resize" };
function hover(w, sx, sy) {
  let cur = "";
  if (spaceHeld || app.tool === "hand") cur = "grab";
  else if (app.tool === "room") cur = "crosshair";
  else {
    const sr = selectedRoom();
    const h = sr && handleAt(sr, w);
    const r = roomAt(w.x, w.y);
    if (h) cur = CURSORS[h];
    else if (entityAt(sx, sy)) cur = "grab";
    else if (r && (onWall(r, w) || sr === r)) cur = "move";
    app.hoverRoom = r ? r.key : null;
  }
  canvas.style.cursor = cur;
}

canvas.addEventListener("contextmenu", (e) => {
  e.preventDefault();
  const { sx, sy } = canvasPoint(e);
  const w = screenToWorld(sx, sy);
  if (build.on) {
    const f = fixtureAt(w.x, w.y);
    if (f) {
      const st = styleOf(f.room);
      (f.type === "decor" ? st.decor : st.blocks).splice(f.idx, 1);
      saveStyle(f.room).then(renderBuilder);
      return;
    }
  }
  const hit = entityAt(sx, sy);
  const r = roomAt(w.x, w.y);
  if (hit && hit.kind === "pup") { select(hit.id, false); showMenu(e, pupMenu(hit)); }
  else if (hit) { select(hit.id, false); showMenu(e, [["▶ Start", () => thingAct("start", hit.id)], ["■ Stop", () => thingAct("stop", hit.id)]]); }
  else if (r) { app.sel = "room:" + r.key; renderCard(); showMenu(e, roomMenu(r)); }
  else showMenu(e, [
    ["🏠 New kennel here", () => createKennel({ x0: Math.round(w.x), y0: Math.round(w.y), x1: Math.round(w.x) + 12, y1: Math.round(w.y) + 8 })],
    ["⤢ Zoom to fit", () => zoomFit()],
    [build.on ? "✓ Done building" : "🔨 Build mode", () => setBuild(!build.on)],
  ]);
});

function pupMenu(ent) {
  const p = pupOf(ent.id);
  const ids = app.multi.size > 1 && app.multi.has(ent.id) ? [...app.multi] : [ent.id];
  const items = [];
  if (ids.length === 1) {
    items.push(["↗ Open", () => act("open", ent.id)], ["⑂ Fork", () => act("fork", ent.id)],
      [p && p.starred ? "☆ Unstar" : "★ Star", () => act("star", ent.id)],
      ["✎ Rename…", () => { const t = prompt("Rename", p ? p.title : ""); if (t) act("rename", ent.id, { title: t }); }]);
  }
  const kennels = ownKennels();
  if (kennels.length) items.push(["—"], ...kennels.slice(0, 12).map((k) => [`→ ${k.icon || "🏠"} ${k.name}`, async () => { for (const id of ids) await act("move", id, { kennel: k.id }); toast(`🏠 filed ${ids.length > 1 ? ids.length + " pups " : ""}into ${k.name}`, false, true); app.multi.clear(); }]));
  const r = ent.room;
  if (r && r.style && r.style.spots && ids.some((id) => r.style.spots[id])) items.push(["—"], ["↺ Back to its slot", () => { for (const id of ids) delete r.style.spots[id]; saveStyle(r, true); }]);
  const filed = ids.filter(inKennel);
  if (filed.length) {
    const names = [...new Set(filed.map((id) => pupOf(id) && pupOf(id).kennel).filter(Boolean))];
    items.push(["—"], [`✕ Remove from ${names.length === 1 ? names[0] : "kennels"}${filed.length > 1 ? " (" + filed.length + ")" : ""}`, () => unfile(filed)]);
  }
  items.push(["—"], [`🧹 Archive${ids.length > 1 ? " " + ids.length : ""}`, async () => { for (const id of ids) await act("archive", id); app.multi.clear(); }]);
  return items;
}

function roomMenu(r) {
  const st = r.style || {};
  const pos = st.pos && st.pos[String(app.state.floor)];
  const items = [["🐣 New pup here", () => newPupIn(r)], ["🔨 Customize", () => { setBuild(true); build.room = r.key; renderBuilder(); }]];
  if (r.kennel) {
    items.push(["✎ Rename…", () => renameKennel(r)]);
    for (const f of app.state.floors) if (f.id !== app.state.floor) items.push([`🏢 Move to ${f.name}`, () => moveKennelToFloor(r, f.id)]);
  }
  if (pos && pos.w) items.push(["⤡ Fit to pups", () => { delete pos.w; delete pos.h; saveStyle(r); }]);
  if (st.spots && Object.keys(st.spots).length) items.push(["↺ Tidy pups", () => { delete st.spots; saveStyle(r); }]);
  if (pos) items.push(["⌂ Back to auto layout", () => { delete st.pos[String(app.state.floor)]; saveStyle(r); }]);
  if (r.kennel) items.push(["—"], ["🗑 Delete kennel", () => deleteKennel(r)]);
  return items;
}

async function renameKennel(r) {
  const name = prompt("Rename kennel", r.title);
  if (!name || !name.trim() || name.trim() === r.title) return;
  try {
    await api(`/api/kennels/${r.kennel}/rename`, { method: "POST", body: JSON.stringify({ name: name.trim() }) });
    await loadState();
    toast(`✎ ${name.trim()}`, false, true);
  } catch (e) { toast(e.message, true); }
}

async function deleteKennel(r) {
  const n = r.pups.length;
  if (!r.smart && n && !confirm(`Delete ${r.title}? Its ${n} pups go back to 📥 New; the sessions themselves are kept.`)) return;
  try {
    await api(`/api/kennels/${r.kennel}/delete`, { method: "POST", body: "{}" });
    app.sel = null; $("#card").hidden = true;
    await loadState();
    toast(`🗑 deleted ${r.title}`, false, true);
  } catch (e) { toast(e.message, true); }
}

async function newPupIn(r) {
  const title = prompt(`New pup in ${r.title} — what should it do?`, "");
  if (title === null) return;
  try { toast((await (await api("/api/room/new", { method: "POST", body: JSON.stringify({ key: r.key, title, prompt: title }) })).json()).message || "🐣 started"); }
  catch (err) { toast(err.message, true); }
}

function showMenu(e, items) {
  const m = $("#ctxmenu");
  m.innerHTML = "";
  for (const [label, fn] of items) {
    if (label === "—") { m.appendChild(el("div", "sep")); continue; }
    const b = el("button", "", label);
    b.onclick = () => { hideMenu(); fn(); };
    m.appendChild(b);
  }
  m.hidden = false;
  const st = $("#stage").getBoundingClientRect();
  const x = Math.min(e.clientX - st.left, st.width - m.offsetWidth - 8), y = Math.min(e.clientY - st.top, st.height - m.offsetHeight - 8);
  m.style.left = Math.max(8, x) + "px"; m.style.top = Math.max(8, y) + "px";
}
function hideMenu() { const m = $("#ctxmenu"); if (m) m.hidden = true; }
window.addEventListener("blur", hideMenu);

canvas.addEventListener("wheel", (e) => {
  e.preventDefault();
  hideMenu();
  app.cam.follow = false;
  app.flyTo = null;
  if (!e.ctrlKey && !e.metaKey) {
    app.cam.x += e.deltaX / app.cam.z;
    app.cam.y += e.deltaY / app.cam.z;
    return;
  }
  const { sx, sy } = canvasPoint(e);
  zoomAt(app.cam.z * Math.exp(-e.deltaY * 0.01), sx, sy);
}, { passive: false });

function zoomAt(z, sx, sy) {
  if (sx === undefined) { sx = canvas.width / devicePixelRatio / 2; sy = canvas.height / devicePixelRatio / 2; }
  const before = screenToWorld(sx, sy);
  app.cam.z = Math.max(0.4, Math.min(2.5, z));
  const after = screenToWorld(sx, sy);
  app.cam.x += (before.x - after.x) * TILE;
  app.cam.y += (before.y - after.y) * TILE;
  app.cam.follow = false; app.flyTo = null;
  $("#zoomval").textContent = Math.round(app.cam.z * 100) + "%";
}

function zoomFit() {
  const vw = canvas.width / devicePixelRatio, vh = canvas.height / devicePixelRatio;
  const z = Math.max(0.4, Math.min(1.4, Math.min(vw / (app.world.w * TILE), vh / (app.world.h * TILE))));
  app.cam.z = z;
  app.cam.x = (app.world.w * TILE - vw / z) / 2;
  app.cam.y = (app.world.h * TILE - vh / z) / 2 - 30 / z;
  app.cam.follow = false; app.flyTo = null;
  $("#zoomval").textContent = Math.round(z * 100) + "%";
}

document.querySelectorAll("#tools [data-tool]").forEach((b) => (b.onclick = () => setTool(b.dataset.tool)));
$("#zoomin").onclick = () => zoomAt(app.cam.z * 1.2);
$("#zoomout").onclick = () => zoomAt(app.cam.z / 1.2);
$("#zoomval").onclick = () => zoomAt(1);
$("#zoomfit").onclick = () => zoomFit();
setTool("select");

function interact(e) {
  if (e.kind === "pup") act("open", e.id);
  else thingAct("start", e.id);
}

window.addEventListener("keydown", (e) => {
  const typing = ["INPUT", "TEXTAREA"].includes(document.activeElement.tagName);
  if ((e.key === "/" && !typing) || ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "f")) { e.preventDefault(); $("#q").focus(); $("#q").select(); return; }
  if (typing) return;
  if (e.key === "Escape" && lib.open) { closeLibrary(); return; }
  if (e.key === "Escape") { hideMenu(); app.multi.clear(); setTool("select"); $("#digest").hidden = true; $("#transcript").hidden = true; app.sel = null; $("#card").hidden = true; if (build.carry) { build.carry = null; renderBuilder(); } return; }
  if (e.key === " " && !e.repeat) { e.preventDefault(); spaceHeld = true; canvas.style.cursor = "grab"; return; }
  if (e.key === "v" || e.key === "1") { setTool("select"); return; }
  if (e.key === "h" || e.key === "2") { setTool("hand"); return; }
  if (e.key === "r" || e.key === "3") { setTool("room"); return; }
  if (e.key === "!" || (e.shiftKey && e.code === "Digit1")) { zoomFit(); return; }
  if ((e.key === "=" || e.key === "+") && !e.metaKey) { zoomAt(app.cam.z * 1.2); return; }
  if (e.key === "-" && !e.metaKey) { zoomAt(app.cam.z / 1.2); return; }
  if ((e.key === "Delete" || e.key === "Backspace") && (app.multi.size || (app.sel && pupOf(app.sel)))) {
    const ids = app.multi.size ? [...app.multi] : [app.sel];
    deletePups(ids);
    return;
  }
  if (e.key === "T") { cycleAppearance(); return; }
  if (e.key === "b") { setBuild(!build.on); return; }
  if (e.key === "l" || e.key === "L") { lib.open ? closeLibrary() : openLibrary(); return; }
  if (e.key === "D") { openDigest(); return; }
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "z") { e.preventDefault(); undo(); return; }
  const k = e.key.toLowerCase();
  if (["arrowup", "arrowdown", "arrowleft", "arrowright", "w", "a", "s", "d"].includes(k)) {
    e.preventDefault();
    app.keys.add(k);
    app.cam.follow = true;
    app.flyTo = null;
  }
  if (k === "x" || k === "enter") {
    const near = nearest();
    if (near) { select(near.id, false); interact(near); }
    else if (app.sel && app.ents.has(app.sel)) interact(app.ents.get(app.sel));
  }
});
window.addEventListener("keyup", (e) => { app.keys.delete(e.key.toLowerCase()); if (e.key === " ") { spaceHeld = false; canvas.style.cursor = ""; } });

function nearest() {
  let best = null, bd = 1.7;
  for (const e of app.ents.values()) {
    const d = Math.hypot(e.x - app.you.x, e.y - app.you.y);
    if (d < bd) { bd = d; best = e; }
  }
  return best;
}

// ---------- simulation ----------
function step(dt) {
  app.t += dt;
  const you = app.you;
  let vx = 0, vy = 0;
  if (app.keys.has("arrowleft") || app.keys.has("a")) vx -= 1;
  if (app.keys.has("arrowright") || app.keys.has("d")) vx += 1;
  if (app.keys.has("arrowup") || app.keys.has("w")) vy -= 1;
  if (app.keys.has("arrowdown") || app.keys.has("s")) vy += 1;
  you.moving = !!(vx || vy);
  if (you.moving) {
    const len = Math.hypot(vx, vy);
    you.x = Math.max(0.5, Math.min(app.world.w - 0.5, you.x + (vx / len) * 5 * dt));
    you.y = Math.max(0.5, Math.min(app.world.h - 0.5, you.y + (vy / len) * 5 * dt));
    if (vx) you.dir = vx;
  }

  for (const e of app.ents.values()) {
    if (e.kind !== "pup") continue;
    const p = pupOf(e.id);
    if (!p || e.dragging) continue;
    const wanders = p.status === "idle" || p.status === "done";
    if (wanders && Math.hypot(e.tx - e.x, e.ty - e.y) < 0.05 && Math.random() < dt * 0.5) {
      e.tx = e.home.x + (Math.random() - 0.5) * 1.8;
      e.ty = e.home.y + (Math.random() - 0.5) * 1.0;
    }
    if (!wanders) { e.tx = e.home.x; e.ty = e.home.y; }
    const dx = e.tx - e.x, dy = e.ty - e.y, d = Math.hypot(dx, dy);
    e.moving = d > 0.05;
    if (e.moving) {
      const sp = Math.min(d, 1.6 * dt);
      e.x += (dx / d) * sp; e.y += (dy / d) * sp;
      if (Math.abs(dx) > 0.01) e.dir = Math.sign(dx);
    }
  }

  const vw = canvas.width / devicePixelRatio / app.cam.z, vh = canvas.height / devicePixelRatio / app.cam.z;
  let target = null;
  if (app.flyTo) target = app.flyTo;
  else if (app.cam.follow) target = you;
  if (target) {
    const gx = target.x * TILE - vw / 2, gy = target.y * TILE - vh / 2;
    app.cam.x += (gx - app.cam.x) * Math.min(1, dt * 4);
    app.cam.y += (gy - app.cam.y) * Math.min(1, dt * 4);
  }
}

// ---------- drawing ----------
function resize() {
  const r = canvas.getBoundingClientRect();
  canvas.width = Math.max(1, r.width * devicePixelRatio);
  canvas.height = Math.max(1, r.height * devicePixelRatio);
}
window.addEventListener("resize", () => { resize(); if (app.state) { layout(app.state); frameWorld(); } });

// ---------- the office, drawn in Gather's style ----------
// Light walls, carpets, small pixel furniture, translucent room names, dark name tags.

// The office by day and by night; PAL switches with the appearance.
const DAY = {
  stage: "#e9e1d4", hall: ["#efe6d8", "#eadfcf"], hallLine: "#dfd2bf", backWall: "#e4e2ec", backCap: "#8f8ca6",
  window: "#b9dcf2", shine: "#ffffff", carpet: "#ffffff",
  grass: "#bfe6a6", blades: ["#9fd486", "#a9da90"], wood: "#ead4b4", woodLine: "#dcc19c", pool: "#cfe7f6", poolLine: "#b5d9ef",
  rug: "#d8b8c0", rugLine: "#c39aa5", stone: "#ddd8cf", stoneLine: "#cbc4b8", metal: "#d7dae2", metalLine: "#c3c7d2",
  label: "#ffffffd9", labelInk: "#5f6480", labelLine: "#282d4f14", empty: "#9a9db3", picket: "#ffffff", picketShade: "#d9d4c8",
  desk: "#f3f2f6", deskEdge: "#c9c7d4",
  walls: {
    rounded: { face: "#efe7da", cap: "#a08a70", trim: "#d9c9b0" },
    thick: { face: "#efd9d0", cap: "#a8655a", trim: "#d9b0a5" },
    double: { face: "#f4efe0", cap: "#b39443", trim: "#e6cd7d" },
    normal: { face: "#e8e7ef", cap: "#8f8ca6", trim: "#cfcddc" },
  },
};
const NIGHT = {
  stage: "#141728", hall: ["#24273d", "#212438"], hallLine: "#2b2f4c", backWall: "#2b2f4a", backCap: "#12152a",
  window: "#1d3152", shine: "#3c5a8a", carpet: "#1b1e31",
  grass: "#2d5338", blades: ["#3a6a45", "#346140"], wood: "#4a3a2c", woodLine: "#3c2f23", pool: "#1e3850", poolLine: "#294a66",
  rug: "#55384a", rugLine: "#6c4a5c", stone: "#383640", stoneLine: "#45434e", metal: "#343846", metalLine: "#414657",
  label: "#1b1e31d9", labelInk: "#c4c7e3", labelLine: "#ffffff14", empty: "#6f7496", picket: "#b9bcd6", picketShade: "#7f83a1",
  desk: "#c9cbe0", deskEdge: "#8d90a8",
  walls: {
    rounded: { face: "#3a3342", cap: "#18131e", trim: "#4b4253" },
    thick: { face: "#41303a", cap: "#271318", trim: "#563b46" },
    double: { face: "#3c392f", cap: "#5c4d22", trim: "#7a6830" },
    normal: { face: "#30344d", cap: "#12152a", trim: "#3d4262" },
  },
};
let PAL = DAY;

const STATUS_DOT = { working: "#2fb36b", waiting: "#f2b01e", done: "#4f8de0", idle: "#2fb3a4", offline: "#9a9db3" };

function px(x, y, w, h, c) { ctx.fillStyle = c; ctx.fillRect(Math.round(x), Math.round(y), Math.round(w), Math.round(h)); }

function drawHallway() {
  const { w, h } = app.world;
  // Warm checker tiles like Gather's office floor.
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
    px(x * TILE, y * TILE, TILE, TILE, PAL.hall[(x + y) % 2]);
  }
  ctx.strokeStyle = PAL.hallLine; ctx.lineWidth = 1;
  for (let x = 0; x <= w; x++) { ctx.beginPath(); ctx.moveTo(x * TILE + .5, 0); ctx.lineTo(x * TILE + .5, h * TILE); ctx.stroke(); }
  for (let y = 0; y <= h; y++) { ctx.beginPath(); ctx.moveTo(0, y * TILE + .5); ctx.lineTo(w * TILE, y * TILE + .5); ctx.stroke(); }
  // The building's back wall, with windows.
  px(0, 0, w * TILE, TILE * 1.2, PAL.backWall);
  px(0, 0, w * TILE, 6, PAL.backCap);
  for (let x = 2; x < w - 2; x += 6) {
    px(x * TILE, 10, TILE * 2, TILE * 0.8, PAL.window);
    px(x * TILE, 10, TILE * 2, 3, PAL.shine);
    if (PAL === NIGHT) { px(x * TILE + 12, 18, 2, 2, "#fff6c8"); px(x * TILE + 44, 24, 2, 2, "#ffffff"); }
    px(x * TILE + TILE - 1, 10, 2, TILE * 0.8, PAL.backWall);
  }
  // Plants along the hallway.
  for (let x = 4; x < w - 1; x += 9) plant(x * TILE + 16, (h - 0.6) * TILE, 0.9);
}

// Carpets are soft and light; the room's color only tints them.
function floorPattern(r, x0, y0) {
  const W = r.w * TILE, H = (r.h - WALL) * TILE;
  const f = r.look.Floor || " ";
  const tint = (t) => mixHex(r.color, PAL.carpet, t);
  ctx.save();
  ctx.beginPath(); ctx.rect(x0, y0, W, H); ctx.clip();
  switch (f) {
    case "⠂": { // grass, with paths and flowers
      px(x0, y0, W, H, PAL.grass);
      for (let i = 0; i < W * H / 700; i++) {
        const gx = x0 + ((i * 137) % W), gy = y0 + ((i * 251 + i * i * 7) % H);
        px(gx, gy, 2, 4, PAL.blades[0]); px(gx + 3, gy + 1, 2, 3, PAL.blades[1]);
      }
      for (let i = 0; i < W * H / 9000; i++) { const fx = x0 + ((i * 331) % W), fy = y0 + ((i * 173) % H); px(fx, fy, 3, 3, ["#ffffff", "#ffd36b", "#f5a3c7"][i % 3]); }
      break;
    }
    case " ": { // light wood planks
      px(x0, y0, W, H, PAL.wood);
      for (let y = y0, i = 0; y < y0 + H; y += 16, i++) {
        px(x0, y, W, 1, PAL.woodLine);
        for (let x = x0 + (i % 3) * 22; x < x0 + W; x += 64) px(x, y, 1, 16, PAL.woodLine);
      }
      break;
    }
    case "░": case "⁘": case "▚": { // tiles
      px(x0, y0, W, H, tint(0.78));
      for (let y = 0; y < H / 16; y++) for (let x = 0; x < W / 16; x++) if ((x + y) % 2) px(x0 + x * 16, y0 + y * 16, 16, 16, tint(0.7));
      if (f === "▚") for (let i = 0; i < 7; i++) { ctx.globalAlpha = 0.18; ctx.fillStyle = "#7a6650"; ctx.beginPath(); ctx.ellipse(x0 + ((i * 97) % W), y0 + ((i * 53) % H), 10, 6, 0, 0, 7); ctx.fill(); ctx.globalAlpha = 1; }
      break;
    }
    case "~": { // pool-blue carpet
      px(x0, y0, W, H, PAL.pool);
      ctx.strokeStyle = PAL.poolLine; ctx.lineWidth = 2;
      for (let y = y0 + 8; y < y0 + H; y += 14) { ctx.beginPath(); for (let x = x0; x <= x0 + W; x += 8) ctx.lineTo(x, y + Math.sin((x + app.t * 30) / 10) * 2); ctx.stroke(); }
      break;
    }
    case "▒": { // a rug in the middle — things get swept under it
      px(x0, y0, W, H, tint(0.8));
      px(x0 + 20, y0 + 14, W - 40, H - 28, PAL.rug);
      ctx.strokeStyle = PAL.rugLine; ctx.lineWidth = 3; ctx.strokeRect(x0 + 26, y0 + 20, W - 52, H - 40);
      break;
    }
    case "∴": case "▪": { // stone / metal
      px(x0, y0, W, H, f === "▪" ? PAL.metal : PAL.stone);
      ctx.strokeStyle = f === "▪" ? PAL.metalLine : PAL.stoneLine; ctx.lineWidth = 1;
      for (let y = y0; y < y0 + H; y += 32) for (let x = x0; x < x0 + W; x += 32) ctx.strokeRect(x + .5, y + .5, 31, 31);
      break;
    }
    default: { // soft dotted carpet in the room's tint
      px(x0, y0, W, H, tint(0.74));
      for (let y = y0 + 6; y < y0 + H; y += 12) for (let x = x0 + 6 + ((y / 12) % 2) * 6; x < x0 + W; x += 12) px(x, y, 2, 2, tint(0.6));
    }
  }
  ctx.restore();
}

const WALL = 1.6; // tiles of back wall above each room's floor



function drawRoom(r) {
  ctx.save();
  if (app.project && !r.pups.some((id) => !dimmed(pupOf(id)))) ctx.globalAlpha = 0.45;
  drawRoomBody(r);
  ctx.restore();
}

function drawRoomBody(r) {
  const x0 = r.x * TILE, y0 = r.y * TILE, W = r.w * TILE, H = r.h * TILE;
  const wallH = WALL * TILE;
  const style = r.look.Border;
  floorPattern(r, x0, y0 + wallH);
  const wl = PAL.walls[style] || PAL.walls.normal;
  if (style !== "hidden") {
    // Back wall: a pale face with a dark cap, like Gather's top-down walls.
    px(x0 - 6, y0, W + 12, wallH, wl.face);
    px(x0 - 6, y0, W + 12, 7, wl.cap);
    px(x0 - 6, y0 + wallH - 6, W + 12, 6, wl.trim);
    if (style === "thick") for (let y = 12; y < wallH - 8; y += 9) for (let x = (Math.floor(y / 9) % 2) ? 0 : 10; x < W; x += 20) px(x0 + x, y0 + y, Math.min(18, W - x), 1, "#e2c2b8");
    if (style === "double") px(x0 - 6, y0 + wallH - 14, W + 12, 3, "#e6cd7d");
    // A framed picture or window on the wall.
    const fx = x0 + W - 70;
    px(fx, y0 + 14, 40, 22, wl.cap);
    px(fx + 3, y0 + 17, 34, 16, style === "thick" ? "#f6d38b" : PAL.window);
    // Thin side and front walls with a door gap in front.
    px(x0 - 6, y0, 6, H, wl.cap);
    px(x0 + W, y0, 6, H, wl.cap);
    px(x0 - 6, y0 + H, W / 2 - TILE + 6, 6, wl.cap);
    px(x0 + W / 2 + TILE, y0 + H, W / 2 - TILE + 6, 6, wl.cap);
  } else {
    // A white picket fence for gardens.
    for (let x = 2; x < W; x += 12) { px(x0 + x, y0 + wallH - 22, 6, 22, PAL.picket); px(x0 + x, y0 + wallH - 22, 6, 2, PAL.picketShade); }
    px(x0, y0 + wallH - 14, W, 3, PAL.picket);
    for (const [x, y] of [[x0 - 4, y0], [x0 + W, y0]]) px(x, y + wallH - 24, 4, H - wallH + 24, PAL.picket);
  }
  autoDecor(r, x0, y0 + wallH, W, H - wallH);
  // Room name: a translucent pill on the back wall, clear of the pups' name tags.
  const label = (r.icon ? r.icon + " " : "") + r.title;
  ctx.font = "600 14px Inter, system-ui, sans-serif";
  const tw = Math.min(W - 90, ctx.measureText(label).width + 24);
  const lx = x0 + 14, ly = y0 + (wallH - 24) / 2 + 2;
  ctx.fillStyle = PAL.label; roundRect(lx, ly, tw, 24, 12); ctx.fill();
  ctx.strokeStyle = PAL.labelLine; ctx.lineWidth = 1; ctx.stroke();
  ctx.save(); ctx.beginPath(); ctx.rect(lx, ly, tw - 8, 24); ctx.clip();
  ctx.fillStyle = PAL.labelInk; ctx.fillText(label, lx + 12, ly + 17); ctx.restore();
  const unread = r.pups.filter((id) => pupOf(id) && pupOf(id).unread).length;
  if (unread) {
    ctx.font = "700 12px Inter, system-ui";
    ctx.fillStyle = "#f2b01e"; roundRect(lx + tw + 6, ly + 2, 24, 20, 10); ctx.fill();
    ctx.fillStyle = "#fff"; ctx.textAlign = "center"; ctx.fillText(`${unread}`, lx + tw + 18, ly + 16); ctx.textAlign = "left";
  }
  const n = r.pups.length + r.things.length;
  if (!n) {
    ctx.fillStyle = PAL.empty; ctx.font = "italic 13px Inter, system-ui";
    ctx.textAlign = "center"; ctx.fillText(isOwnKennel(r) ? "drag sessions here from the tray →" : r.blurb || "empty", x0 + W / 2, y0 + wallH + 62); ctx.textAlign = "left";
  }
  if (app.dropRoom === r.key) {
    ctx.fillStyle = "#3d47db1f"; ctx.fillRect(x0, y0, W, H);
    ctx.strokeStyle = "#3d47db"; ctx.lineWidth = 4; ctx.strokeRect(x0 - 8, y0 - 6, W + 16, H + 14);
  }
  if (r.pups.includes(app.sel) || r.things.includes(app.sel)) {
    ctx.strokeStyle = "#3d47db66"; ctx.lineWidth = 3; ctx.setLineDash([8, 6]);
    ctx.strokeRect(x0 - 10, y0 - 4, W + 20, H + 12); ctx.setLineDash([]);
  }
  if (build.on && build.room === r.key) {
    ctx.strokeStyle = "#f2b01e"; ctx.lineWidth = 4; ctx.setLineDash([10, 6]);
    ctx.strokeRect(x0 - 10, y0 - 6, W + 20, H + 16); ctx.setLineDash([]);
  }
  drawFixtures(r);
}

// Every room gets a little furniture so the office feels lived in; user decor comes on top.
function autoDecor(r, x0, y0, W, H) {
  if ((r.style && (r.style.decor || []).length) || build.on && build.room === r.key && false) return;
  const f = r.look.Floor || " ";
  if (f === "⠂") {
    tree(x0 + 28, y0 + 40); tree(x0 + W - 28, y0 + 46);
    if (H > 150) bench(x0 + W - 70, y0 + H - 18);
    return;
  }
  plant(x0 + 16, y0 + 30, 1); plant(x0 + W - 16, y0 + 30, 1);
  if (W >= 260) bookshelf(x0 + 40, y0 + 4, Math.min(84, W / 4));
  if (f === "▒") return;
  if (r.smart === "lobby" || f === "~") { sofa(x0 + W / 2 - 40, y0 + H - 26); rug(x0 + W / 2, y0 + H - 46, 110, 36, PAL === NIGHT ? "#3a355e" : "#e7e2f7"); }
  if (H > 160 && W >= 220) plant(x0 + W - 18, y0 + H - 14, 0.8);
}

function plant(x, y, s = 1) {
  px(x - 7 * s, y - 2 * s, 14 * s, 11 * s, "#d1834f");
  px(x - 7 * s, y - 2 * s, 14 * s, 3 * s, "#b56b3b");
  ctx.fillStyle = "#4caf6a";
  for (const [dx, dy, rr] of [[-6, -10, 7], [6, -11, 7], [0, -17, 8], [-2, -7, 6], [4, -6, 6]]) { ctx.beginPath(); ctx.arc(x + dx * s, y + dy * s, rr * s, 0, 7); ctx.fill(); }
  ctx.fillStyle = "#6cc785";
  for (const [dx, dy, rr] of [[-4, -13, 3], [5, -15, 3], [1, -19, 3]]) { ctx.beginPath(); ctx.arc(x + dx * s, y + dy * s, rr * s, 0, 7); ctx.fill(); }
}

function tree(x, y) {
  px(x - 4, y - 6, 8, 16, "#8a5a36");
  ctx.fillStyle = "#4f9f5b";
  for (const [dx, dy, rr] of [[-12, -18, 13], [12, -18, 13], [0, -30, 15], [0, -14, 14]]) { ctx.beginPath(); ctx.arc(x + dx, y + dy, rr, 0, 7); ctx.fill(); }
  ctx.fillStyle = "#6dbb6f";
  for (const [dx, dy, rr] of [[-8, -26, 5], [8, -32, 5], [2, -20, 4]]) { ctx.beginPath(); ctx.arc(x + dx, y + dy, rr, 0, 7); ctx.fill(); }
}

function bookshelf(x, y, w) {
  px(x, y, w, 30, "#8b5e3c");
  px(x + 3, y + 3, w - 6, 11, "#6e4a2f"); px(x + 3, y + 16, w - 6, 11, "#6e4a2f");
  const colors = ["#e76f51", "#2a9d8f", "#e9c46a", "#457b9d", "#f4a261", "#8e7dbe"];
  for (let row = 0; row < 2; row++) for (let i = 0, bx = x + 5; bx < x + w - 8; i++, bx += 5 + (i % 3)) px(bx, y + 4 + row * 13, 4, 10 - (i % 2) * 2, colors[(i + row) % colors.length]);
}

function sofa(x, y) {
  px(x, y - 14, 80, 12, "#f0a35e"); px(x - 4, y - 10, 8, 18, "#e08e48"); px(x + 76, y - 10, 8, 18, "#e08e48");
  px(x + 2, y - 4, 76, 10, "#f4b57a"); px(x + 39, y - 4, 2, 10, "#e08e48");
}

function rug(cx, cy, w, h, c) { ctx.fillStyle = c; ctx.beginPath(); ctx.ellipse(cx, cy, w / 2, h / 2, 0, 0, 7); ctx.fill(); }

function bench(x, y) { px(x, y - 8, 48, 6, "#a87a52"); px(x + 2, y - 2, 4, 8, "#7a5134"); px(x + 42, y - 2, 4, 8, "#7a5134"); }

// A Gather-style desk pod: white desk, monitor, chair (drawn under working pups).
function desk(x, y, phase) {
  px(x - 28, y - 4, 56, 14, PAL.desk); px(x - 28, y + 10, 56, 3, PAL.deskEdge);
  px(x - 11, y - 18, 22, 14, "#2d3142"); px(x - 9, y - 16, 18, 10, Math.floor(app.t * 4 + phase) % 2 ? "#8fd3ff" : "#a9e5c0");
  px(x - 2, y - 4, 4, 2, "#2d3142");
  px(x + 14, y - 1, 8, 4, "#9a9db3");
}

function roundRect(x, y, w, h, r) {
  ctx.beginPath();
  ctx.moveTo(x + r, y); ctx.arcTo(x + w, y, x + w, y + h, r); ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r); ctx.arcTo(x, y, x + w, y, r); ctx.closePath();
}

// Speech bubble: white with a dark outline, next to the head.
function bubble(x, y, text, bg = "#ffffff", fg = "#282d4f") {
  ctx.font = "700 12px Inter, system-ui";
  const w = Math.max(24, ctx.measureText(text).width + 14);
  ctx.fillStyle = bg; roundRect(x - w / 2, y - 22, w, 20, 10); ctx.fill();
  ctx.strokeStyle = "#282d4f"; ctx.lineWidth = 1.5; ctx.stroke();
  ctx.beginPath(); ctx.moveTo(x - 5, y - 2.5); ctx.lineTo(x + 3, y - 2.5); ctx.lineTo(x - 3, y + 4); ctx.closePath(); ctx.fillStyle = bg; ctx.fill(); ctx.stroke();
  ctx.fillStyle = bg; ctx.fillRect(x - 4, y - 4, 7, 3);
  ctx.fillStyle = fg; ctx.textAlign = "center"; ctx.fillText(text, x, y - 8); ctx.textAlign = "left";
}

// Name tag: dark pill with a status dot, above the avatar — Gather's signature look.
function nameTag(x, y, text, selected, dot) {
  ctx.font = "600 11.5px Inter, system-ui";
  const t = text.length > 14 ? text.slice(0, 13) + "…" : text;
  const tw = ctx.measureText(t).width;
  const w = tw + (dot ? 22 : 14);
  ctx.fillStyle = selected ? "#3d47db" : "#1f2233e6"; roundRect(x - w / 2, y, w, 18, 9); ctx.fill();
  let tx = x - w / 2 + 7;
  if (dot) { ctx.fillStyle = dot; ctx.beginPath(); ctx.arc(tx + 3, y + 9, 3.5, 0, 7); ctx.fill(); tx += 10; }
  ctx.fillStyle = "#fff"; ctx.fillText(t, tx, y + 13);
}

function drawPup(e) {
  const p = pupOf(e.id);
  if (!p) return;
  ctx.save();
  if (dimmed(p)) ctx.globalAlpha = 0.18;
  drawPupBody(e, p);
  ctx.restore();
}

function drawPupBody(e, p) {
  const s = 3;
  const x = e.x * TILE, y = e.y * TILE;
  const selected = app.sel === e.id;
  if (p.status === "working") desk(x, y + 6, e.phase);
  ctx.fillStyle = "#2b2f4a24"; ctx.beginPath(); ctx.ellipse(x, y + 2, 17, 5, 0, 0, 7); ctx.fill();
  const breed = BREEDS[p.agent] || DOG;
  let grid = breed, dy = 0;
  const frame = Math.floor(app.t * 6 + e.phase);
  if (p.status === "offline") grid = DOG_SLEEP;
  else if (e.moving) grid = frame % 2 ? walkOf(breed) : breed;
  else if (p.status === "working" || p.status === "done") grid = frame % 2 ? wagOf(breed) : breed;
  if (p.status === "waiting") dy = -Math.abs(Math.sin(app.t * 7 + e.phase)) * 7;
  const w = grid[0].length * s, h = grid.length * s;
  ctx.save();
  if (e.dir < 0) { ctx.translate(x * 2, 0); ctx.scale(-1, 1); }
  drawPixels(ctx, grid, dogPalette(p.fur, p.status === "offline", p.agent), x - w / 2, y - h + dy, s);
  ctx.restore();
  if (selected) { ctx.strokeStyle = "#3d47db"; ctx.lineWidth = 2.5; ctx.beginPath(); ctx.ellipse(x, y + 2, 23, 7, 0, 0, 7); ctx.stroke(); }
  const top = y - h + dy;
  nameTag(x, top - 24, (p.starred ? "★ " : "") + p.title, selected, STATUS_DOT[p.status]);
  const bx = x + 30, by = top + 12;
  if (app.fed && app.fed[e.id] && app.t - app.fed[e.id] < 3) bubble(bx, by, "🦴 nom");
  else if (p.status === "waiting") bubble(bx, by, Math.floor(app.t * 3) % 2 ? "!" : "!!", "#fff4cf");
  else if (p.status === "working") bubble(bx, by, ["•  ", "•• ", "•••"][frame % 3]);
  else if (p.status === "done") bubble(bx, by, "✓", "#e6f0ff");
  else if (p.status === "offline") { ctx.fillStyle = "#9a9db3"; ctx.font = "700 11px Inter, system-ui"; ctx.fillText("z", x + 14, top + 10 - (app.t * 6 % 6)); ctx.fillText("Z", x + 20, top + 2 - (app.t * 4 % 6)); }
  if (p.remote) { ctx.font = "12px system-ui"; ctx.fillStyle = "#000"; ctx.fillText("📡", x - 32, top + 14); }
}

function drawThing(e) {
  const t = thingOf(e.id);
  if (!t) return;
  const x = e.x * TILE, y = e.y * TILE;
  const on = t.running;
  const selected = app.sel === e.id;
  ctx.fillStyle = "#2b2f4a24"; ctx.beginPath(); ctx.ellipse(x, y + 2, 22, 5, 0, 0, 7); ctx.fill();
  switch (t.kind) {
    case "monitor": { // a wall screen on a stand with live bars
      px(x - 24, y - 40, 48, 32, "#2d3142"); px(x - 3, y - 8, 6, 6, "#2d3142"); px(x - 12, y - 2, 24, 3, "#9a9db3");
      px(x - 20, y - 36, 40, 24, on ? "#183a2a" : "#4a4d5e");
      if (on) for (let i = 0; i < 6; i++) { const bh = 3 + ((Math.sin(app.t * 4 + i * 1.7 + e.phase) + 1) * 8); px(x - 18 + i * 6.5, y - 13 - bh, 5, bh, "#7ee2a8"); }
      break;
    }
    case "server": { // campfire
      px(x - 18, y - 6, 36, 6, "#8a5a36"); px(x - 14, y - 10, 28, 5, "#a06b42");
      if (on) for (let i = 0; i < 3; i++) {
        const fh = 16 + Math.sin(app.t * 9 + i * 2 + e.phase) * 5;
        ctx.fillStyle = ["#ef6f5e", "#f6a14d", "#ffd36b"][i];
        ctx.beginPath(); ctx.moveTo(x - 12 + i * 4, y - 10); ctx.quadraticCurveTo(x - 6 + i * 6, y - 10 - fh * 1.4, x + 2 + i * 4, y - 10); ctx.fill();
      } else { px(x - 6, y - 16, 3, 6, "#b3b5c4"); px(x + 2, y - 20, 3, 8, "#b3b5c4"); }
      break;
    }
    case "logs": { // a scroll of paper with scrolling lines
      px(x - 18, y - 40, 36, 38, "#fffaf0"); px(x - 20, y - 42, 40, 5, "#c9a27e"); px(x - 20, y - 4, 40, 5, "#c9a27e");
      const off = on ? (app.t * 12) % 6 : 0;
      for (let k = 0; k < 5; k++) px(x - 14, y - 34 + k * 6 + off, 8 + ((k * 7 + 3) % 20), 2, on ? "#5f6480" : "#c4c6d3");
      break;
    }
    case "job": { // hourglass
      px(x - 14, y - 42, 28, 4, "#a06b42"); px(x - 14, y - 4, 28, 4, "#a06b42");
      ctx.fillStyle = "#e8eef8"; ctx.beginPath(); ctx.moveTo(x - 10, y - 38); ctx.lineTo(x + 10, y - 38); ctx.lineTo(x, y - 21); ctx.lineTo(x + 10, y - 4); ctx.lineTo(x - 10, y - 4); ctx.lineTo(x, y - 21); ctx.fill();
      const sand = on ? (app.t * 0.3 + e.phase) % 1 : 1;
      ctx.fillStyle = "#f2c86b"; ctx.beginPath(); ctx.moveTo(x - 8 * (1 - sand), y - 36 + 14 * sand); ctx.lineTo(x + 8 * (1 - sand), y - 36 + 14 * sand); ctx.lineTo(x, y - 22); ctx.fill();
      ctx.fillRect(x - 8 * sand, y - 6 - 10 * sand, 16 * sand, 10 * sand);
      if (on) ctx.fillRect(x - 1, y - 21, 2, 15);
      break;
    }
    default: { // toolbox
      px(x - 18, y - 22, 36, 20, "#e76f6f"); px(x - 8, y - 28, 16, 4, "#2d3142");
    }
  }
  nameTag(x, y - 64, `${t.icon} ${t.name}`, selected, on ? "#2fb36b" : "#9a9db3");
  if (selected) { ctx.strokeStyle = "#3d47db"; ctx.lineWidth = 2.5; ctx.beginPath(); ctx.ellipse(x, y + 2, 28, 8, 0, 0, 7); ctx.stroke(); }
}

function drawYou() {
  const you = app.you;
  const x = you.x * TILE, y = you.y * TILE, s = 3;
  const grid = you.moving && Math.floor(app.t * 8) % 2 ? HUMAN_WALK : HUMAN;
  ctx.fillStyle = "#2b2f4a24"; ctx.beginPath(); ctx.ellipse(x, y + 2, 15, 5, 0, 0, 7); ctx.fill();
  const w = grid[0].length * s, h = grid.length * s;
  ctx.save();
  if (you.dir < 0) { ctx.translate(x * 2, 0); ctx.scale(-1, 1); }
  drawPixels(ctx, grid, HUMAN_PAL, x - w / 2, y - h, s);
  ctx.restore();
  nameTag(x, y - h - 24, "You", false, "#2fb36b");
}

// Selection boxes, handles, marquee and snap guides, drawn above everything else.
function drawOverlays() {
  const z = app.cam.z, lw = 1.5 / z;
  for (const id of app.multi) {
    const e = app.ents.get(id);
    if (!e) continue;
    ctx.strokeStyle = "#3d47db"; ctx.lineWidth = 2.5; ctx.beginPath(); ctx.ellipse(e.x * TILE, e.y * TILE + 2, 23, 7, 0, 0, 7); ctx.stroke();
  }
  const sr = selectedRoom();
  if (sr && !build.on) {
    const x0 = sr.x * TILE, y0 = sr.y * TILE, W = sr.w * TILE, H = sr.h * TILE;
    ctx.strokeStyle = "#6965db"; ctx.lineWidth = lw; ctx.strokeRect(x0, y0, W, H);
    const hs = 9 / z, pts = handlePoints(sr);
    for (const h of HANDLES) {
      const [hx, hy] = pts[h];
      ctx.fillStyle = "#fff"; roundRect(hx * TILE - hs / 2, hy * TILE - hs / 2, hs, hs, 2 / z);
      ctx.fill(); ctx.stroke();
    }
    if (drag && drag.moved && (drag.kind === "resize" || drag.kind === "room")) {
      const label = drag.kind === "resize" ? `${sr.w} × ${sr.h}` : `${sr.x}, ${sr.y}`;
      ctx.font = `600 ${12 / z}px Inter, system-ui`;
      const tw = ctx.measureText(label).width + 12 / z;
      ctx.fillStyle = "#6965db"; roundRect(x0 + W - tw, y0 + H + 6 / z, tw, 20 / z, 5 / z); ctx.fill();
      ctx.fillStyle = "#fff"; ctx.fillText(label, x0 + W - tw + 6 / z, y0 + H + 20 / z);
    }
  } else if (app.hoverRoom && !drag && app.tool === "select") {
    const r = roomByKey(app.hoverRoom);
    if (r) { ctx.strokeStyle = "#6965db55"; ctx.lineWidth = lw; ctx.strokeRect(r.x * TILE, r.y * TILE, r.w * TILE, r.h * TILE); }
  }
  for (const g of app.guides || []) {
    ctx.strokeStyle = "#ec6ea7"; ctx.lineWidth = lw; ctx.beginPath();
    if (g.x !== undefined) { ctx.moveTo(g.x * TILE, g.y0 * TILE - 20); ctx.lineTo(g.x * TILE, g.y1 * TILE + 20); }
    else { ctx.moveTo(g.x0 * TILE - 20, g.y * TILE); ctx.lineTo(g.x1 * TILE + 20, g.y * TILE); }
    ctx.stroke();
  }
  if (drag && drag.moved && (drag.kind === "marquee" || drag.kind === "create")) {
    const x = Math.min(drag.x0, drag.x1) * TILE, y = Math.min(drag.y0, drag.y1) * TILE;
    const w = Math.abs(drag.x1 - drag.x0) * TILE, h = Math.abs(drag.y1 - drag.y0) * TILE;
    const create = drag.kind === "create";
    ctx.fillStyle = create ? "#f2b01e22" : "#6965db14"; ctx.fillRect(x, y, w, h);
    ctx.strokeStyle = create ? "#f2b01e" : "#6965db"; ctx.lineWidth = lw;
    if (create) ctx.setLineDash([8 / z, 5 / z]);
    ctx.strokeRect(x, y, w, h); ctx.setLineDash([]);
    if (create) {
      ctx.font = `600 ${13 / z}px Inter, system-ui`; ctx.fillStyle = "#b07d0a";
      ctx.fillText(`🏠 new kennel ${Math.abs(drag.x1 - drag.x0)} × ${Math.abs(drag.y1 - drag.y0)}`, x + 8 / z, y + 20 / z);
    }
  }
}

function render() {
  const dpr = devicePixelRatio;
  ctx.setTransform(1, 0, 0, 1, 0, 0);
  ctx.fillStyle = PAL.stage;
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  if (!app.state) return;
  ctx.setTransform(dpr * app.cam.z, 0, 0, dpr * app.cam.z, -app.cam.x * dpr * app.cam.z, -app.cam.y * dpr * app.cam.z);
  drawHallway();
  for (const r of app.rooms) drawRoom(r);
  const ents = [...app.ents.values()].sort((a, b) => a.y - b.y);
  let youDrawn = false;
  for (const e of ents) {
    if (!youDrawn && e.y > app.you.y) { drawYou(); youDrawn = true; }
    if (e.kind === "pup") drawPup(e); else drawThing(e);
  }
  if (!youDrawn) drawYou();
  drawDropHover();
  drawOverlays();
  if (!app.rooms.length) {
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    const cw = canvas.width / dpr, ch = canvas.height / dpr;
    ctx.textAlign = "center"; ctx.fillStyle = PAL.labelInk;
    ctx.font = "700 20px Inter, system-ui"; ctx.fillText("This floor is empty", cw / 2, ch / 2 - 14);
    ctx.font = "14px Inter, system-ui"; ctx.fillStyle = PAL.empty;
    ctx.fillText("Draw a kennel with ▭ (R), or open 🔨 Build to add rooms that fill themselves.", cw / 2, ch / 2 + 14);
    ctx.textAlign = "left";
  }
  if (app.shownZoom !== app.cam.z) { app.shownZoom = app.cam.z; $("#zoomval").textContent = Math.round(app.cam.z * 100) + "%"; }

  const near = nearest();
  const hint = $("#hint");
  if (near) {
    const sx = (near.x * TILE - app.cam.x) * app.cam.z, sy = ((near.y - 1.9) * TILE - app.cam.y) * app.cam.z;
    hint.style.left = sx + "px"; hint.style.top = sy + "px";
    const p = pupOf(near.id), t = thingOf(near.id);
    hint.textContent = p ? `X  ${p.status === "offline" ? "wake " : "visit "}${p.title}` : `X  ${t.running ? "focus" : "start"} ${t.name}`;
    hint.hidden = false;
  } else hint.hidden = true;
}

let last = performance.now();
function frame(now) {
  const dt = Math.min(0.05, (now - last) / 1000);
  last = now;
  step(dt);
  render();
  requestAnimationFrame(frame);
}


// ---------- building blocks ----------
const build = { on: false, room: null, carry: null, drag: null };
let catalog = { shapes: [], floors: [], walls: [], decor: [], kinds: [], templates: [] };
const FLOOR_SWATCH = { wood: "#e8c9a0", grass: "#9fdcae", tiles: "#f4d9c6", carpet: "#d9c9f0", water: "#a9d4f5", stone: "#d8d4cc", metal: "#c9ced8", rug: "#e3b5bd", checker: "#efe3c8" };
const WALL_SWATCH = { wood: "#8b5e3c", brick: "#a3524a", gold: "#c9a24a", plain: "#8a8fa8", fence: "#fffaf0" };

async function loadCatalog() {
  try { catalog = await (await api("/api/catalog")).json(); } catch (e) { console.error(e); }
}

const roomByKey = (k) => app.rooms.find((r) => r.key === k);
const interior = (r) => ({ x: r.x + 0.8, y: r.y + 2.5, w: r.w - 1.6, h: r.h - 3.1 });
const styleOf = (r) => { r.style = r.style || {}; return r.style; };

function fixtures() {
  const out = [];
  for (const r of app.rooms) {
    const st = r.style || {};
    const box = interior(r);
    (st.decor || []).forEach((d, i) => out.push({ room: r, type: "decor", idx: i, item: d, x: box.x + d.x * box.w, y: box.y + d.y * box.h }));
    (st.blocks || []).forEach((b, i) => out.push({ room: r, type: "block", idx: i, item: b, x: box.x + b.x * box.w, y: box.y + b.y * box.h }));
  }
  return out;
}

function fixtureAt(wx, wy) {
  let best = null, bd = 0.9;
  for (const f of fixtures()) {
    const d = Math.hypot(f.x - wx, f.y - 0.5 - wy);
    if (d < bd) { bd = d; best = f; }
  }
  return best;
}

function roomAt(wx, wy) {
  return app.rooms.find((r) => wx >= r.x && wx <= r.x + r.w && wy >= r.y && wy <= r.y + r.h);
}

async function saveStyle(r, quiet) {
  try {
    await api("/api/room/style", { method: "POST", body: JSON.stringify({ key: r.key, style: r.style || {} }) });
  } catch (e) { toast(e.message, true); }
  await loadState();
  if (!quiet && app.state.undo) toast("🔨 " + r.title + " updated", false, true);
}

const decorIcon = (k) => (catalog.decor.find((d) => d.key === k) || { icon: "❓" }).icon;
const kindOf = (k) => catalog.kinds.find((x) => x.key === k) || { icon: "❓", name: k, fields: [] };

function drawFixtures(r) {
  const st = r.style || {};
  const box = interior(r);
  const waiting = r.pups.some((id) => pupOf(id) && pupOf(id).status === "waiting");
  ctx.textAlign = "center";
  for (const d of st.decor || []) {
    const px = (box.x + d.x * box.w) * TILE, py = (box.y + d.y * box.h) * TILE;
    ctx.fillStyle = "#0002"; ctx.beginPath(); ctx.ellipse(px, py + 2, 14, 4, 0, 0, 7); ctx.fill();
    ctx.fillStyle = "#000"; ctx.font = "28px system-ui"; ctx.fillText(decorIcon(d.kind), px, py);
  }
  for (const b of st.blocks || []) {
    const px = (box.x + b.x * box.w) * TILE, py = (box.y + b.y * box.h) * TILE;
    const cfg = b.config || {};
    ctx.fillStyle = "#0002"; ctx.beginPath(); ctx.ellipse(px, py + 2, 16, 5, 0, 0, 7); ctx.fill();
    ctx.fillStyle = "#000";
    switch (b.kind) {
      case "door": {
        ctx.fillStyle = "#6b4226"; ctx.fillRect(px - 15, py - 42, 30, 42);
        ctx.fillStyle = "#9c6b45"; ctx.fillRect(px - 12, py - 39, 24, 39);
        ctx.fillStyle = "#7a5134"; ctx.fillRect(px - 9, py - 35, 18, 12); ctx.fillRect(px - 9, py - 19, 18, 14);
        ctx.fillStyle = "#ffd66b"; ctx.beginPath(); ctx.arc(px + 7, py - 18, 2.5, 0, 7); ctx.fill();
        break;
      }
      case "portal": {
        for (let i = 3; i >= 0; i--) {
          ctx.strokeStyle = ["#cba6f7", "#89dceb", "#f5c2e7", "#b4befe"][i]; ctx.lineWidth = 3;
          ctx.beginPath(); ctx.ellipse(px, py - 18, 8 + i * 4 + Math.sin(app.t * 3 + i) * 1.5, 14 + i * 3, app.t * (i % 2 ? 1 : -1) * 0.6, 0, 7); ctx.stroke();
        }
        break;
      }
      case "board": {
        ctx.fillStyle = "#b07d4f"; ctx.fillRect(px - 36, py - 46, 72, 44);
        ctx.fillStyle = "#e7c08d"; ctx.fillRect(px - 33, py - 43, 66, 38);
        ctx.fillStyle = "#fff6c2"; ctx.fillRect(px - 26, py - 39, 52, 30);
        ctx.fillStyle = "#f38ba8"; ctx.beginPath(); ctx.arc(px, py - 39, 3, 0, 7); ctx.fill();
        ctx.fillStyle = "#3d3a5c"; ctx.font = "8px ui-rounded, system-ui"; ctx.textAlign = "left";
        const words = (cfg.text || "").split(/\s+/); let line = "", y = py - 29, lines = 0;
        for (const w of words) {
          if (ctx.measureText(line + w).width > 46) { ctx.fillText(line, px - 23, y); y += 9; line = ""; if (++lines === 3) break; }
          line += w + " ";
        }
        if (lines < 3) ctx.fillText(line, px - 23, y);
        ctx.textAlign = "center";
        break;
      }
      case "bell": {
        ctx.save(); ctx.translate(px, py - 18);
        if (waiting) ctx.rotate(Math.sin(app.t * 14) * 0.35);
        ctx.font = "28px system-ui"; ctx.fillText("🔔", 0, 10); ctx.restore();
        break;
      }
      case "magnet": {
        ctx.strokeStyle = "#f38ba8"; ctx.setLineDash([4, 4]); ctx.lineWidth = 2;
        ctx.beginPath(); ctx.arc(px, py - 14, 18 + Math.sin(app.t * 3) * 2, 0, 7); ctx.stroke(); ctx.setLineDash([]);
        ctx.font = "24px system-ui"; ctx.fillText("🧲", px, py - 4);
        break;
      }
      case "chute": {
        ctx.fillStyle = "#45475a"; ctx.fillRect(px - 16, py - 30, 32, 30);
        ctx.fillStyle = "#1e1e2e"; ctx.fillRect(px - 11, py - 25, 22, 12);
        ctx.font = "18px system-ui"; ctx.fillText("🧹", px, py - 2);
        break;
      }
    }
    const k = kindOf(b.kind);
    const label = b.kind === "door" && cfg.name ? `🚪 ${cfg.name}` : b.kind === "portal" && cfg.workspace ? `🌀 ${cfg.workspace}` :
      b.kind === "chute" ? `🧹 ${cfg.days || 14}d` : `${k.icon} ${k.name}`;
    ctx.textAlign = "left";
    nameTag(px, py + 6, label, build.drag && build.drag.fixture && build.drag.fixture.item === b);
    ctx.textAlign = "center";
  }
  ctx.textAlign = "left";
}

function setBuild(on) {
  build.on = on;
  $("#stage").classList.toggle("building", on);
  if (!on) { build.carry = null; build.drag = null; }
  for (const x of document.querySelectorAll("#side .tabs button")) x.classList.toggle("on", x.dataset.tab === (on ? "build" : "pack"));
  for (const p of PANELS) $("#" + p).hidden = p !== (on ? "build" : "pack");
  $("#treats").hidden = on;
  renderBuilder();
}

function chip(label, on, fn, cls) {
  const b = el("button", "chip" + (on ? " on" : "") + (cls ? " " + cls : ""), label);
  b.onclick = fn;
  return b;
}

function blockChips() {
  const bk = el("div", "chips");
  for (const k of catalog.kinds) {
    const c = chip(`${k.icon} ${k.name}`, false, () => {
      build.carry = build.carry && build.carry.kind === k.key ? null : { type: "block", kind: k.key };
      renderBuilder();
    }, build.carry && build.carry.kind === k.key ? "carry" : "");
    c.title = k.blurb;
    carriable(c, { type: "block", kind: k.key });
    bk.appendChild(c);
  }
  return bk;
}

// Chips in the builder can also be dragged straight onto a room.
function carriable(node, item) {
  node.draggable = true;
  node.addEventListener("dragstart", (e) => { e.dataTransfer.setData("application/x-kennel-block", JSON.stringify(item)); e.dataTransfer.effectAllowed = "copy"; });
}

function placeIn(r, item, w) {
  const box = interior(r);
  const placed = { kind: item.kind, x: Math.max(0, Math.min(1, (w.x - box.x) / box.w)), y: Math.max(0, Math.min(1, (w.y + 0.4 - box.y) / box.h)) };
  const st = styleOf(r);
  if (item.type === "decor") (st.decor = st.decor || []).push(placed);
  else { placed.config = {}; (st.blocks = st.blocks || []).push(placed); }
  build.room = r.key;
  if (item.type === "block") build.carry = null;
  return saveStyle(r).then(renderBuilder);
}

async function addSmart(k) {
  try {
    await api("/api/kennels", { method: "POST", body: JSON.stringify({ Name: k.Name, Icon: k.Icon, Smart: k.Key, Floor: app.state.floor }) });
    await loadState();
    toast(`${k.Icon} added ${k.Name}`, false, true);
  } catch (e) { toast(e.message, true); }
}

function renderBuilder() {
  const box = $("#build");
  if (!box || box.hidden) return;
  box.innerHTML = "";
  const r = build.room && roomByKey(build.room);
  if (!r) {
    box.appendChild(el("div", "hint", "Add rooms to this floor, or click a room on the map to shape it."));
    box.appendChild(el("h3", "", "add a kennel"));
    const own = el("div", "chips");
    own.appendChild(chip("▭ Draw a kennel", false, () => { setTool("room"); toast("drag on the map to draw it"); }));
    box.appendChild(own);
    box.appendChild(el("h3", "", "rooms that fill themselves"));
    const sm = el("div", "smartgrid");
    for (const k of catalog.smarts) {
      const c = el("button", "smart");
      c.append(el("b", "", `${k.Icon} ${k.Name}`), el("span", "", k.Blurb));
      c.onclick = () => addSmart(k);
      sm.appendChild(c);
    }
    box.appendChild(sm);
    box.appendChild(el("h3", "", "blocks: click one, then click a room (or drag it there)"));
    box.appendChild(blockChips());
    return;
  }
  const st = styleOf(r);
  box.appendChild(el("div", "roomname", `${r.icon} ${r.title}`));

  box.appendChild(el("h3", "", "templates"));
  const tp = el("div", "chips");
  for (const t of catalog.templates) tp.appendChild(chip(`${t.icon} ${t.name}`, false, async () => {
    try { const res = await api("/api/room/template", { method: "POST", body: JSON.stringify({ key: r.key, template: t.key }) }); toast((await res.json()).message); } catch (e) { toast(e.message, true); }
    await loadState(); renderBuilder();
  }));
  box.appendChild(tp);

  box.appendChild(el("h3", "", "shape"));
  const sh = el("div", "chips");
  for (const s of catalog.shapes) sh.appendChild(chip(`${s.icon} ${s.name}`, st.shape === s.key, () => { st.shape = st.shape === s.key ? "" : s.key; saveStyle(r).then(renderBuilder); }));
  box.appendChild(sh);

  box.appendChild(el("h3", "", "floor"));
  const fl = el("div", "chips");
  for (const f of catalog.floors) {
    const b = el("button", "swatch" + (st.floor === f.key ? " on" : ""), f.name);
    b.style.background = FLOOR_SWATCH[f.key] || "#ccc";
    b.onclick = () => { st.floor = f.key; saveStyle(r).then(renderBuilder); };
    fl.appendChild(b);
  }
  box.appendChild(fl);

  box.appendChild(el("h3", "", "walls"));
  const wl = el("div", "chips");
  for (const w of catalog.walls) {
    const b = el("button", "swatch" + (st.wall === w.key ? " on" : ""), w.name.split(" ")[0]);
    b.style.background = WALL_SWATCH[w.key] || "#ccc";
    if (w.key !== "fence") b.style.color = "#fff";
    b.onclick = () => { st.wall = w.key; saveStyle(r).then(renderBuilder); };
    wl.appendChild(b);
  }
  box.appendChild(wl);

  box.appendChild(el("h3", "", "furniture — pick one, then click in the room"));
  const dc = el("div", "chips");
  for (const d of catalog.decor) {
    const b = el("button", "decor" + (build.carry && build.carry.kind === d.key ? " carry" : ""), d.icon);
    b.title = d.name;
    b.onclick = () => { build.carry = build.carry && build.carry.kind === d.key ? null : { type: "decor", kind: d.key }; renderBuilder(); };
    carriable(b, { type: "decor", kind: d.key });
    dc.appendChild(b);
  }
  box.appendChild(dc);

  box.appendChild(el("h3", "", "blocks: click one, then click in the room (or drag it there)"));
  box.appendChild(blockChips());

  (st.blocks || []).forEach((b, i) => {
    const k = kindOf(b.kind);
    const c = el("div", "blockcard");
    const head = el("div", "head", `${k.icon} ${k.name}`);
    const del = el("button", "", "🗑");
    del.title = "remove";
    del.onclick = () => { st.blocks.splice(i, 1); saveStyle(r).then(renderBuilder); };
    head.appendChild(del);
    c.appendChild(head);
    c.appendChild(el("div", "hint", k.blurb));
    for (const f of k.fields) {
      c.appendChild(el("label", "", f.label));
      const inp = el("input");
      inp.value = (b.config || {})[f.key] || "";
      inp.placeholder = f.placeholder;
      inp.dataset.field = f.key;
      inp.onchange = () => { b.config = b.config || {}; b.config[f.key] = inp.value; saveStyle(r, true); };
      c.appendChild(inp);
    }
    box.appendChild(c);
  });

  const reset = el("button", "danger", "Reset this room");
  reset.onclick = async () => { await api("/api/room/reset", { method: "POST", body: JSON.stringify({ key: r.key }) }); await loadState(); renderBuilder(); };
  box.appendChild(reset);
}

// Fixture clicks outside build mode: doors spawn, boards show their note.
async function useFixture(f) {
  const b = f.item;
  if (f.type !== "block") return;
  const cfg = b.config || {};
  if (b.kind === "door") {
    const idx = (f.room.style.blocks || []).filter((x) => x.kind === "door").indexOf(b);
    if (!confirm(`Start a new pup${cfg.name ? " " + cfg.name : ""} here?`)) return;
    try {
      const res = await api("/api/room/door", { method: "POST", body: JSON.stringify({ key: f.room.key, index: idx }) });
      toast((await res.json()).message || "opened");
    } catch (e) { toast(e.message, true); }
  } else if (b.kind === "board") toast("📌 " + (cfg.text || "empty board"));
  else toast(`${kindOf(b.kind).icon} ${kindOf(b.kind).blurb}`);
}


// ---------- treats: feed sources to pups by drag and drop ----------
let treatList = [];
async function loadTreats() {
  try { treatList = await (await api("/api/treats")).json(); } catch (e) { treatList = []; }
  renderTreats();
}

function renderTreats() {
  const box = $("#treatlist");
  if (!box) return;
  box.innerHTML = "";
  if (!treatList.length) box.appendChild(el("div", "empty", "Save files, links, #channels or tickets here, or drag files straight from Finder."));
  for (const t of treatList) {
    const c = el("div", "treat", `${t.icon} ${t.name}`);
    c.title = `${t.value}\nright-click to remove`;
    c.draggable = true;
    c.dataset.id = t.id;
    c.addEventListener("dragstart", (e) => { e.dataTransfer.setData("application/x-kennel-treat", String(t.id)); e.dataTransfer.effectAllowed = "copy"; });
    c.addEventListener("contextmenu", async (e) => {
      e.preventDefault();
      if (!confirm(`Remove treat ${t.name}?`)) return;
      await api(`/api/treats/${t.id}/delete`, { method: "POST" });
      await loadTreats();
      await loadState();
      toast("removed " + t.name, false, true);
    });
    box.appendChild(c);
  }
}

$("#addtreat").onclick = async () => {
  const value = prompt("A file path, link, #slack-channel, TICKET-123 or a note:");
  if (!value) return;
  try { await api("/api/treats", { method: "POST", body: JSON.stringify({ value }) }); } catch (e) { toast(e.message, true); }
  await loadTreats();
  await loadState();
  toast("🦴 treat saved", false, true);
};

const dropTargetAt = (clientX, clientY) => {
  const rc = canvas.getBoundingClientRect();
  const px = clientX - rc.left, py = clientY - rc.top;
  const ent = entityAt(px, py);
  if (ent && ent.kind === "pup") return { kind: "pup", id: ent.id };
  const w = screenToWorld(px, py);
  const f = fixtureAt(w.x, w.y);
  if (f && f.type === "block" && f.item.kind === "door") {
    const idx = (f.room.style.blocks || []).filter((x) => x.kind === "door").indexOf(f.item);
    return { kind: "door", room: f.room.key, index: idx, fixture: f };
  }
  return null;
};

canvas.addEventListener("dragover", (e) => {
  e.preventDefault();
  if ([...e.dataTransfer.types].includes("application/x-kennel-block")) {
    e.dataTransfer.dropEffect = "copy";
    const rc = canvas.getBoundingClientRect();
    const w = screenToWorld(e.clientX - rc.left, e.clientY - rc.top);
    const r = roomAt(w.x, w.y);
    app.dropRoom = r ? r.key : null;
    return;
  }
  if ([...e.dataTransfer.types].includes("application/x-kennel-pup")) {
    e.dataTransfer.dropEffect = "move";
    const r = kennelRoomAt(e.clientX, e.clientY);
    app.dropRoom = r ? r.key : null;
    return;
  }
  e.dataTransfer.dropEffect = "copy";
  $("#stage").classList.add("dropping");
  app.dropHover = dropTargetAt(e.clientX, e.clientY);
});
canvas.addEventListener("dragleave", () => { $("#stage").classList.remove("dropping"); app.dropHover = null; app.dropRoom = null; });
canvas.addEventListener("drop", async (e) => {
  e.preventDefault();
  const block = e.dataTransfer.getData("application/x-kennel-block");
  if (block) {
    app.dropRoom = null;
    const rc = canvas.getBoundingClientRect();
    const w = screenToWorld(e.clientX - rc.left, e.clientY - rc.top);
    const r = roomAt(w.x, w.y);
    if (r) placeIn(r, JSON.parse(block), w); else toast("drop it inside a room", true);
    return;
  }
  $("#stage").classList.remove("dropping");
  $("#stage").classList.remove("pupdrag");
  const pupId = e.dataTransfer.getData("application/x-kennel-pup");
  if (pupId) {
    app.dropHover = null;
    const r = kennelRoomAt(e.clientX, e.clientY);
    if (r) await fileInto(r, pupId);
    else toast("drop it inside one of your own kennels (smart rooms fill themselves)", true);
    return;
  }
  const target = dropTargetAt(e.clientX, e.clientY);
  app.dropHover = null;
  if (!target) { toast("drop it right on a pup (or a spawn door)", true); return; }
  let body = null;
  const treatId = e.dataTransfer.getData("application/x-kennel-treat");
  try {
    if (treatId) body = { treat: Number(treatId) };
    else if (e.dataTransfer.files && e.dataTransfer.files.length) {
      const file = e.dataTransfer.files[0];
      const data = await new Promise((res, rej) => { const r = new FileReader(); r.onload = () => res(r.result.split(",")[1] || ""); r.onerror = rej; r.readAsDataURL(file); });
      const saved = await (await api("/api/drop", { method: "POST", body: JSON.stringify({ name: file.name, data }) })).json();
      body = { value: saved.path, title: file.name };
    } else {
      const text = (e.dataTransfer.getData("text/uri-list") || e.dataTransfer.getData("text/plain") || "").split("\n")[0].trim();
      if (text) body = { value: text };
    }
    if (!body) return;
    if (target.kind === "pup") {
      const res = await api(`/api/pup/${target.id}/feed`, { method: "POST", body: JSON.stringify(body) });
      toast((await res.json()).message);
      app.fed = app.fed || {};
      app.fed[target.id] = app.t;
    } else {
      let promptText = body.value;
      if (body.treat) { const t = treatList.find((x) => x.id === body.treat); promptText = t ? t.value : ""; }
      const res = await api("/api/room/door", { method: "POST", body: JSON.stringify({ key: target.room, index: target.index, prompt: "Start by looking at " + promptText }) });
      toast("🚪 " + ((await res.json()).message || "opened"));
    }
  } catch (err) { toast(err.message, true); }
});

function drawDropHover() {
  const h = app.dropHover;
  if (!h) return;
  let x, y;
  if (h.kind === "pup") { const e = app.ents.get(h.id); if (!e) return; x = e.x * TILE; y = (e.y - 1.9) * TILE; }
  else { x = h.fixture.x * TILE; y = (h.fixture.y - 2) * TILE; }
  const bob = Math.sin(app.t * 8) * 4;
  ctx.font = "26px system-ui"; ctx.textAlign = "center"; ctx.fillStyle = "#000";
  ctx.fillText("🦴", x, y + bob);
  ctx.textAlign = "left";
}


// ---------- dragging sessions from the tray ----------

function relAge(ms) {
  const d = ((app.state ? app.state.now : Date.now()) - ms) / 1000;
  if (d < 3600) return `${Math.max(1, Math.floor(d / 60))}m`;
  if (d < 86400) return `${Math.floor(d / 3600)}h`;
  return `${Math.floor(d / 86400)}d`;
}

function draggablePup(node, p) {
  node.draggable = true;
  node.addEventListener("dragstart", (e) => {
    e.dataTransfer.setData("application/x-kennel-pup", p.id);
    e.dataTransfer.setData("text/plain", p.title);
    e.dataTransfer.effectAllowed = "move";
    $("#stage").classList.add("pupdrag");
  });
  node.addEventListener("dragend", () => $("#stage").classList.remove("pupdrag"));
}


// A pup dropped on one of your kennels on the map is filed there.
function kennelRoomAt(clientX, clientY) {
  const rc = canvas.getBoundingClientRect();
  const w = screenToWorld(clientX - rc.left, clientY - rc.top);
  const r = roomAt(w.x, w.y);
  return isOwnKennel(r) ? r : null;
}

async function fileInto(r, pupId) {
  await act("move", pupId, { kennel: Number(r.key.slice(7)) });
  toast(`🏠 filed into ${r.icon} ${r.title}`, false, true);
}

// ---------- library ----------
let library = [];
const libSelected = new Set();
const lib = { open: false };

async function openLibrary() {
  lib.open = true;
  $("#library").hidden = false;
  try { library = await (await api("/api/library")).json(); } catch (e) { toast(e.message, true); }
  fillLibFilters();
  renderLibrary();
  $("#libq").focus();
}
let libFetch = null;
function refreshLibrary() {
  if (libFetch) return;
  libFetch = api("/api/library").then((r) => r.json()).then((l) => { library = l; fillLibFilters(); renderLibrary(); })
    .catch(() => {}).finally(() => { libFetch = null; });
}
function closeLibrary() { lib.open = false; $("#library").hidden = true; }
$("#libbtn").onclick = () => (lib.open ? closeLibrary() : openLibrary());
$("#libclose").onclick = closeLibrary;
for (const id of ["#libq", "#libwhen", "#libagent", "#libkennel", "#libtheme", "#libgroup", "#libsort"]) $(id).addEventListener("input", renderLibrary);

function fillLibFilters() {
  const k = $("#libkennel"), cur = k.value;
  k.innerHTML = "";
  k.append(new Option("any kennel", ""), new Option("📥 not filed", "none"));
  for (const x of app.state.kennels || []) k.appendChild(new Option(`${x.icon} ${x.name}`, x.name));
  k.value = cur;
  const t = $("#libtheme"), curT = t.value;
  const themes = [...new Set(library.map((p) => p.theme).filter(Boolean))].sort();
  t.innerHTML = "";
  t.append(new Option("any theme", ""), new Option("no theme yet", "none"));
  for (const x of themes) t.appendChild(new Option(x, x));
  t.value = curT;
  const mv = $("#libmove");
  mv.innerHTML = "";
  mv.appendChild(new Option("move to…", ""));
  mv.appendChild(new Option("📥 no kennel", "0"));
  for (const x of ownKennels()) mv.appendChild(new Option(`${x.icon} ${x.name}`, String(x.id)));
}

function weekOf(ms) {
  const d = new Date(ms);
  const day = (d.getDay() + 6) % 7;
  d.setDate(d.getDate() - day);
  return "week of " + d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function renderLibrary() {
  if (!lib.open) return;
  const q = $("#libq").value.trim().toLowerCase();
  const days = Number($("#libwhen").value);
  const agent = $("#libagent").value, kennel = $("#libkennel").value, theme = $("#libtheme").value;
  const now = app.state ? app.state.now : Date.now();
  let rows = library.filter((p) => {
    if (days && now - p.updated > days * 86400000) return false;
    if (agent && p.agent !== agent) return false;
    if (kennel === "none" ? p.kennel : kennel && p.kennel !== kennel) return false;
    if (theme === "none" ? p.theme : theme && p.theme !== theme) return false;
    if (q && !`${p.title} ${p.firstPrompt} ${p.repo} ${p.theme} ${p.kennel} ${p.branch}`.toLowerCase().includes(q)) return false;
    return true;
  });
  const sort = $("#libsort").value;
  rows.sort((a, b) => sort === "old" ? a.updated - b.updated : sort === "msgs" ? b.messages - a.messages : sort === "cost" ? b.cost - a.cost : b.updated - a.updated);
  const group = $("#libgroup").value;
  const keyOf = (p) => group === "day" ? new Date(p.updated).toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" })
    : group === "week" ? weekOf(p.updated) : group === "theme" ? (p.theme || "no theme yet") : group === "kennel" ? (p.kennel || "📥 not filed")
    : group === "repo" ? (p.repo || "—") : group === "agent" ? p.agent : "";
  const groups = new Map();
  for (const p of rows) { const k = keyOf(p); if (!groups.has(k)) groups.set(k, []); groups.get(k).push(p); }
  const body = $("#libbody");
  body.innerHTML = "";
  if (!rows.length) body.appendChild(el("div", "libempty", "No sessions match these filters."));
  for (const [k, list] of groups) {
    const g = el("div", "libgroup");
    if (group !== "none") g.appendChild(el("h4", "", `${k} · ${list.length}`));
    for (const p of list) {
      const r = el("div", "librow" + (app.sel === p.id ? " on" : ""));
      const cb = el("input");
      cb.type = "checkbox";
      cb.checked = libSelected.has(p.id);
      cb.onclick = (e) => { e.stopPropagation(); cb.checked ? libSelected.add(p.id) : libSelected.delete(p.id); renderLibSel(); };
      r.append(cb, miniDog(p.fur, p.status, p.agent), el("span", "t", (p.starred ? "★ " : "") + p.title), el("span", "s", p.snippet || p.firstPrompt || ""));
      r.append(p.theme ? el("span", "pill theme", p.theme) : el("span", ""), p.kennel ? el("span", "pill kennel", "🏠 " + p.kennel) : el("span", ""));
      r.append(el("span", "d", [p.repo, `${p.messages} msgs`].filter(Boolean).join(" · ")), el("span", "d", relAge(p.updated)));
      r.onclick = () => { app.state.pups[p.id] = app.state.pups[p.id] || p; select(p.id, false); renderLibrary(); };
      draggablePup(r, p);
      g.appendChild(r);
    }
    body.appendChild(g);
  }
  renderLibSel();
}

function renderLibSel() {
  $("#libsel").hidden = libSelected.size === 0;
  $("#libselcount").textContent = `${libSelected.size} selected`;
}
$("#libclear").onclick = () => { libSelected.clear(); renderLibrary(); };
$("#libdo").onclick = async () => {
  const v = $("#libmove").value;
  if (v === "") return;
  for (const id of libSelected) await api(`/api/pup/${id}/move`, { method: "POST", body: JSON.stringify({ kennel: Number(v) }) });
  const n = libSelected.size;
  libSelected.clear();
  await loadState();
  library = await (await api("/api/library")).json();
  renderLibrary();
  toast(`🚚 moved ${n} pups`, false, true);
};
$("#libthemes").onclick = async () => {
  const redo = library.some((p) => p.theme) && confirm("Re-categorize sessions that already have a theme too? (Cancel = only the ones without)");
  try {
    const res = await api("/api/themes/run", { method: "POST", body: JSON.stringify({ redo }) });
    toast(`✨ categorizing with ${(await res.json()).provider}…`);
  } catch (e) { toast(e.message, true); }
};

function renderThemeStatus() {
  const t = app.state.themes || {};
  const btn = $("#libthemes");
  btn.textContent = t.running ? `✨ ${t.done}/${t.total || "…"}` : "✨ Themes";
  btn.title = `categorize with ${t.provider || "an LLM"}` + (t.error ? ` — last error: ${t.error}` : "");
  if (lib.wasRunning && !t.running) {
    if (t.error) toast("themes: " + t.error, true);
    else toast(`✨ themes ready (${(t.themes || []).length})`);
    refreshLibrary();
  }
  lib.wasRunning = t.running;
}

// ---------- side panels: jobs & things, artifacts ----------
async function renderJobs() {
  const box = $("#jobs");
  box.innerHTML = "";
  box.className = "panel";
  let d;
  try { d = await (await api("/api/jobs")).json(); } catch (e) { box.textContent = e.message; return; }
  box.appendChild(el("h4", "", `Things · ${d.things.length}`));
  for (const t of d.things) {
    const r = el("div", "prow");
    r.innerHTML = "";
    r.append(el("b", "", `${t.icon} ${t.name} `), el("span", t.running ? "on" : "off", t.running ? "● running" : "○ stopped"), el("div", "m", "$ " + t.command));
    r.onclick = () => select(t.key, true);
    box.appendChild(r);
  }
  box.appendChild(el("h4", "", `Background agents · ${d.jobs.length}`));
  for (const j of d.jobs) {
    const r = el("div", "prow");
    r.append(el("b", "", (j.running ? "🏃 " : "🐕 ") + j.title + " "), el("span", j.running ? "on" : "off", j.state), el("div", "m", j.detail));
    r.onclick = () => jumpToSession(j.session);
    box.appendChild(r);
  }
  box.appendChild(el("h4", "", `Scheduled tasks · ${d.crons.length}`));
  for (const c of d.crons) {
    const r = el("div", "prow");
    r.append(el("b", "", `⏰ ${c.title} `), el("span", c.live ? "on" : "off", `${c.expr} · fired ${c.fires}×`), el("div", "m", c.prompt));
    r.onclick = () => jumpToSession(c.session);
    box.appendChild(r);
  }
}

async function renderArt() {
  const box = $("#art");
  box.innerHTML = "";
  box.className = "panel";
  let arts;
  try { arts = await (await api("/api/artifacts")).json(); } catch (e) { box.textContent = e.message; return; }
  box.appendChild(el("h4", "", `Artifacts · ${arts.length}`));
  for (const a of arts) {
    const r = el("div", "prow");
    r.append(el("b", "", "🎨 " + (a.title || a.url)), el("div", "m", `from ${a.pup} · ${relAge(a.at)} ago`));
    r.onclick = () => window.open(a.url, "_blank", "noopener");
    box.appendChild(r);
  }
}


// ---------- digest ----------
async function openDigest() {
  $("#digest").hidden = false;
  const d = await (await api(`/api/digest?days=${$("#digestdays").value}`)).json();
  const body = $("#digestbody");
  body.innerHTML = "";
  for (const l of d.lines) body.appendChild(el("div", "", l));
}
$("#digestbtn").onclick = openDigest;
$("#digestdays").onchange = openDigest;
$("#digestclose").onclick = () => ($("#digest").hidden = true);
$("#digest").addEventListener("click", (e) => { if (e.target.id === "digest") $("#digest").hidden = true; });

// Exposed for end-to-end tests: screen positions of entities.
window.kennel = {
  app,
  setAppearance: async (a) => { applyAppearance(a); await api("/api/appearance", { method: "POST", body: JSON.stringify({ appearance: a }) }); },
  toast,
  lib,
  fileInto: (key, id) => fileInto(roomByKey(key), id),
  build,
  roomPos(key, fx = 0.5, fy = 0.5) {
    const r = roomByKey(key);
    if (!r) return null;
    const box = interior(r);
    const rc = canvas.getBoundingClientRect();
    const wx = box.x + fx * box.w, wy = box.y + fy * box.h;
    return { x: rc.left + (wx * TILE - app.cam.x) * app.cam.z, y: rc.top + (wy * TILE - app.cam.y) * app.cam.z };
  },
  wallPos(key) {
    const r = roomByKey(key);
    if (!r) return null;
    const rc = canvas.getBoundingClientRect();
    return { x: rc.left + ((r.x + r.w - 2) * TILE - app.cam.x) * app.cam.z, y: rc.top + ((r.y + 1) * TILE - app.cam.y) * app.cam.z };
  },
  reveal(id) {
    const e = app.ents.get(id);
    if (!e) return false;
    const st = $("#stage");
    app.flyTo = null;
    app.cam.follow = false;
    app.cam.x = e.x * TILE - st.clientWidth / app.cam.z / 2;
    app.cam.y = e.y * TILE - st.clientHeight / app.cam.z / 2;
    return true;
  },
  worldPos(wx, wy) {
    const r = canvas.getBoundingClientRect();
    return { x: r.left + (wx * TILE - app.cam.x) * app.cam.z, y: r.top + (wy * TILE - app.cam.y) * app.cam.z };
  },
  screenPos(id) {
    const e = app.ents.get(id);
    if (!e) return null;
    const r = canvas.getBoundingClientRect();
    return { x: r.left + (e.x * TILE - app.cam.x) * app.cam.z, y: r.top + ((e.y - 0.5) * TILE - app.cam.y) * app.cam.z };
  },
};

$("#buildbtn").onclick = () => setBuild(!build.on);
try { app.project = localStorage.getItem("kennel.project") || ""; } catch (_) { app.project = ""; }
$("#projectsel").onchange = () => {
  app.project = $("#projectsel").value;
  try { localStorage.setItem("kennel.project", app.project); } catch (_) {}
  $("#projectsel").classList.toggle("on", !!app.project);
  renderTray();
};
$("#themebtn").onclick = cycleAppearance;
systemDark.addEventListener("change", () => applyAppearance(appearance));
applyAppearance(appearance);
$("#helpbtn").onclick = () => toast("drag pups to place or file them · drag a room’s wall to move it · select a room, drag handles to resize · right-click for actions · V/H/R tools · Space-drag pans · ⌘-scroll zooms · ⌘Z undo");
resize();
Promise.all([loadCatalog(), loadState(), loadTreats()]).then(() => { listen(); requestAnimationFrame(frame); }).catch((e) => toast(e.message, true));
