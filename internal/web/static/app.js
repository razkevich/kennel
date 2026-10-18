"use strict";
// kennel map — a Gather-style office where Claude sessions are pups and other
// terminals are things. Everything is drawn procedurally; no image assets.

const TOKEN = new URLSearchParams(location.search).get("t") || "";
const TILE = 32;
const SLOT = 3.6; // tiles per pup slot, wide enough for name tags

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
    return { ...r, cols, rows, w: Math.ceil(cols * SLOT) + 2, h: rows * 3 + 4 };
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
      const cx = r.x + 1 + (i % r.cols) * SLOT + SLOT / 2;
      const cy = r.y + 3 + Math.floor(i / r.cols) * 3 + 1.4;
      seen.add(it.id);
      let e = app.ents.get(it.id);
      if (!e) {
        e = { kind: it.kind, id: it.id, x: cx, y: cy, tx: cx, ty: cy, phase: Math.random() * 10 };
        app.ents.set(it.id, e);
      }
      e.kind = it.kind;
      e.home = { x: cx, y: cy };
      e.room = r;
      if (Math.hypot(e.x - cx, e.y - cy) > 2.5) { e.tx = cx; e.ty = cy; }
      // Far moves (a room was rebuilt or moved) teleport instead of a long walk.
      if (it.kind === "thing" || Math.hypot(e.x - cx, e.y - cy) > 6) { e.x = e.tx = cx; e.y = e.ty = cy; }
    });
  }
  for (const id of [...app.ents.keys()]) if (!seen.has(id)) app.ents.delete(id);
}

const pupOf = (id) => app.state && app.state.pups[id];
const thingOf = (id) => app.state && app.state.things[id];

// ---------- data ----------
async function loadState() {
  const res = await api(app.floor === null ? "/api/state" : `/api/state?floor=${app.floor}`);
  app.state = await res.json();
  app.floor = app.state.floor;
  layout(app.state);
  renderFloors();
  renderCounts();
  renderPack();
  renderBuilder();
  renderThemeStatus();
  if (native) native.postMessage({ type: "counts", counts: app.state.counts });
  loadInbox();
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
  app.state.floors.forEach((f, i) => {
    const b = el("button", i === app.floor ? "on" : "", f);
    b.onclick = () => { app.floor = i; app.sel = null; $("#card").hidden = true; app.placedYou = false; app.ents.clear(); loadState(); };
    nav.appendChild(b);
  });
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

function renderPack() {
  const box = $("#pack");
  box.innerHTML = "";
  const pups = Object.values(app.state.pups);
  const groups = [
    ["waiting", "Needs you"], ["working", "Working"], ["done", "Done"], ["idle", "Idle"], ["offline", "Asleep"],
  ];
  for (const [st, label] of groups) {
    const list = pups.filter((p) => p.status === st).sort((a, b) => b.updated - a.updated);
    if (!list.length) continue;
    const g = el("div", "group");
    g.appendChild(el("h4", "", `${label} · ${list.length}`));
    for (const p of list) g.appendChild(memberRow(p));
    box.appendChild(g);
  }
  const things = Object.values(app.state.things);
  if (things.length) {
    const g = el("div", "group");
    g.appendChild(el("h4", "", `Things · ${things.length}`));
    for (const t of things) {
      const row = el("div", "member" + (app.sel === t.key ? " on" : ""));
      const icon = el("div", "", t.icon);
      icon.style.width = "28px"; icon.style.textAlign = "center"; icon.style.fontSize = "18px";
      const who = el("div", "who");
      who.appendChild(el("div", "name", t.name));
      who.appendChild(el("div", "what", "$ " + t.command));
      const dot = el("span", "dot");
      dot.style.background = t.running ? STATUS.working.color : STATUS.offline.color;
      row.append(icon, who, dot);
      row.onclick = () => select(t.key, true);
      g.appendChild(row);
    }
    box.appendChild(g);
  }
}

function memberRow(p) {
  const row = el("div", "member" + (app.sel === p.id ? " on" : ""));
  row.dataset.id = p.id;
  const who = el("div", "who");
  who.appendChild(el("div", "name", (p.starred ? "★ " : "") + p.title));
  who.appendChild(el("div", "what", p.snippet || p.repo));
  const dot = el("span", "dot");
  dot.style.background = STATUS[p.status].color;
  row.append(miniDog(p.fur, p.status, p.agent), who, dot);
  row.onclick = () => select(p.id, true);
  row.ondblclick = () => act("open", p.id);
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
  app.cam.z = Math.max(0.75, Math.min(1.25, vw / W));
  app.cam.x = W * app.cam.z < vw ? -(vw / app.cam.z - W) / 2 : 0;
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
    btn(p.archived ? "↩ Restore" : "🗑 Archive", () => act("archive", p.id));
    if (p.pane) btn("💬 Reply", () => { const text = prompt(`Reply to ${p.title}`); if (text) act("reply", p.id, { text }); });
    const move = el("label", "move", "🏠 kennel");
    const sel = el("select");
    sel.appendChild(new Option("— none —", "0"));
    for (const k of app.state.kennels || []) sel.appendChild(new Option(`${k.icon} ${k.name}`, String(k.id), false, k.name === p.kennel));
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
  card.append(row, el("div", "snip", r.blurb && !r.blurb.startsWith("/") ? r.blurb : (isKennel ? "Your kennel." : "")), actions);
  if (isKennel) {
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
const PANELS = ["pack", "ask", "build", "jobs", "art", "gear"];
function showTab(tab) {
  if (tab === "build") { setBuild(true); return; }
  if (build.on) setBuild(false);
  for (const x of document.querySelectorAll("#side .tabs button")) x.classList.toggle("on", x.dataset.tab === tab);
  for (const p of PANELS) $("#" + p).hidden = p !== tab;
  $("#treats").hidden = tab !== "pack";
  if (tab === "ask") $("#askq").focus();
  if (tab === "jobs") renderJobs();
  if (tab === "art") renderArt();
  if (tab === "gear") renderGear();
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

let drag = null;
canvas.addEventListener("mousedown", (e) => {
  if (e.button !== 0) return;
  const rc = canvas.getBoundingClientRect();
  const w = screenToWorld(e.clientX - rc.left, e.clientY - rc.top);
  if (build.on && !build.carry) {
    const f = fixtureAt(w.x, w.y);
    if (f) { build.drag = { fixture: f, moved: false }; return; }
    const r = roomAt(w.x, w.y);
    if (r && w.y < r.y + 2) { build.drag = { room: r, dx: w.x - r.x, dy: w.y - r.y, moved: false }; return; }
  }
  drag = { x: e.clientX, y: e.clientY, cx: app.cam.x, cy: app.cam.y, moved: false };
});
canvas.addEventListener("contextmenu", (e) => {
  if (!build.on) return;
  e.preventDefault();
  const rc = canvas.getBoundingClientRect();
  const w = screenToWorld(e.clientX - rc.left, e.clientY - rc.top);
  const f = fixtureAt(w.x, w.y);
  if (!f) return;
  const st = styleOf(f.room);
  (f.type === "decor" ? st.decor : st.blocks).splice(f.idx, 1);
  saveStyle(f.room).then(renderBuilder);
});
window.addEventListener("mousemove", (e) => {
  if (build.drag) {
    const rc = canvas.getBoundingClientRect();
    const w = screenToWorld(e.clientX - rc.left, e.clientY - rc.top);
    const d = build.drag;
    d.moved = true;
    if (d.fixture) {
      const box = interior(d.fixture.room);
      d.fixture.item.x = Math.max(0, Math.min(1, (w.x - box.x) / box.w));
      d.fixture.item.y = Math.max(0, Math.min(1, (w.y + 0.4 - box.y) / box.h));
    } else if (d.room) {
      d.room.x = Math.max(1, Math.round(w.x - d.dx));
      d.room.y = Math.max(2, Math.round(w.y - d.dy));
    }
    return;
  }
  if (!drag) return;
  const dx = e.clientX - drag.x, dy = e.clientY - drag.y;
  if (Math.abs(dx) + Math.abs(dy) > 4) { drag.moved = true; canvas.classList.add("dragging"); app.cam.follow = false; app.flyTo = null; }
  if (drag.moved) { app.cam.x = drag.cx - dx / app.cam.z; app.cam.y = drag.cy - dy / app.cam.z; }
});
window.addEventListener("mouseup", (e) => {
  if (build.drag) {
    const d = build.drag;
    build.drag = null;
    if (d.room && d.moved) {
      const st = styleOf(d.room);
      st.pos = st.pos || {};
      st.pos[String(app.state.floor)] = { x: d.room.x, y: d.room.y };
      build.room = d.room.key;
      saveStyle(d.room).then(renderBuilder);
    } else if (d.fixture && d.moved) saveStyle(d.fixture.room);
    else { build.room = (d.room || d.fixture.room).key; renderBuilder(); }
    return;
  }
  if (build.on && drag && !drag.moved && e.target === canvas) {
    const rc = canvas.getBoundingClientRect();
    const w = screenToWorld(e.clientX - rc.left, e.clientY - rc.top);
    const hit = !build.carry && entityAt(e.clientX - rc.left, e.clientY - rc.top);
    if (hit) { select(hit.id, false); drag = null; return; }
    const r = roomAt(w.x, w.y);
    if (r && build.carry && build.room === r.key) {
      const box = interior(r);
      const item = { kind: build.carry.kind, x: Math.max(0, Math.min(1, (w.x - box.x) / box.w)), y: Math.max(0, Math.min(1, (w.y + 0.4 - box.y) / box.h)) };
      const st = styleOf(r);
      if (build.carry.type === "decor") (st.decor = st.decor || []).push(item);
      else { item.config = {}; (st.blocks = st.blocks || []).push(item); build.carry = null; }
      saveStyle(r).then(renderBuilder);
    } else if (r) { build.room = r.key; renderBuilder(); }
    drag = null;
    return;
  }
  if (drag && !drag.moved && e.target === canvas) {
    const rcf = canvas.getBoundingClientRect();
    const wf = screenToWorld(e.clientX - rcf.left, e.clientY - rcf.top);
    const fx = !entityAt(e.clientX - rcf.left, e.clientY - rcf.top) && fixtureAt(wf.x, wf.y);
    if (fx) { useFixture(fx); drag = null; return; }
    const r = canvas.getBoundingClientRect();
    const hit = entityAt(e.clientX - r.left, e.clientY - r.top);
    if (hit) {
      const now = performance.now();
      const dbl = app.lastClick.id === hit.id && now - app.lastClick.at < 450;
      app.lastClick = { id: hit.id, at: now };
      select(hit.id, false);
      if (dbl) interact(hit);
    } else {
      const w = screenToWorld(e.clientX - r.left, e.clientY - r.top);
      const room = roomAt(w.x, w.y);
      app.sel = room ? "room:" + room.key : null;
      renderCard();
    }
  }
  drag = null;
  canvas.classList.remove("dragging");
});
canvas.addEventListener("wheel", (e) => {
  e.preventDefault();
  app.cam.follow = false;
  app.flyTo = null;
  if (!e.ctrlKey && !e.metaKey) {
    app.cam.x += e.deltaX / app.cam.z;
    app.cam.y += e.deltaY / app.cam.z;
    return;
  }
  const r = canvas.getBoundingClientRect();
  const before = screenToWorld(e.clientX - r.left, e.clientY - r.top);
  app.cam.z = Math.max(0.5, Math.min(2.2, app.cam.z * Math.exp(-e.deltaY * 0.01)));
  const after = screenToWorld(e.clientX - r.left, e.clientY - r.top);
  app.cam.x += (before.x - after.x) * TILE;
  app.cam.y += (before.y - after.y) * TILE;
  app.cam.follow = false;
}, { passive: false });

function interact(e) {
  if (e.kind === "pup") act("open", e.id);
  else thingAct("start", e.id);
}

window.addEventListener("keydown", (e) => {
  const typing = ["INPUT", "TEXTAREA"].includes(document.activeElement.tagName);
  if (e.key === "/" && !typing) { e.preventDefault(); $("#q").focus(); return; }
  if (typing) return;
  if (e.key === "Escape" && lib.open) { closeLibrary(); return; }
  if (e.key === "Escape") { $("#digest").hidden = true; $("#transcript").hidden = true; app.sel = null; $("#card").hidden = true; if (build.carry) { build.carry = null; renderBuilder(); } return; }
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
window.addEventListener("keyup", (e) => app.keys.delete(e.key.toLowerCase()));

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
    if (!p) continue;
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

function drawHallway() {
  const { w, h } = app.world;
  for (let y = 0; y < h; y++) for (let x = 0; x < w; x++) {
    ctx.fillStyle = (x + y) % 2 ? "#e9dfc9" : "#e3d7bd";
    ctx.fillRect(x * TILE, y * TILE, TILE, TILE);
  }
  // Outer wall of the office.
  ctx.fillStyle = "#3a3566";
  ctx.fillRect(0, 0, w * TILE, TILE);
  ctx.fillStyle = "#4b4580";
  ctx.fillRect(0, TILE - 6, w * TILE, 6);
  // A few plants in the hallway, Gather style.
  ctx.font = "22px system-ui";
  for (let x = 3; x < w; x += 11) ctx.fillText("🪴", x * TILE + 4, (h - 1) * TILE + 24);
}

function floorPattern(r, x0, y0) {
  const base = mixHex(r.color, "#fffaf0", 0.55);
  const alt = mixHex(r.color, "#fffaf0", 0.42);
  const line = mixHex(r.color, "#3a2a1a", 0.25);
  const W = r.w * TILE, H = (r.h - 2) * TILE;
  ctx.fillStyle = base;
  ctx.fillRect(x0, y0, W, H);
  const f = r.look.Floor || " ";
  ctx.save();
  ctx.beginPath(); ctx.rect(x0, y0, W, H); ctx.clip();
  switch (f) {
    case " ": // wooden planks
      ctx.strokeStyle = alt; ctx.lineWidth = 2;
      for (let y = y0; y < y0 + H; y += 12) { ctx.beginPath(); ctx.moveTo(x0, y); ctx.lineTo(x0 + W, y); ctx.stroke(); }
      for (let y = y0, i = 0; y < y0 + H; y += 12, i++) for (let x = x0 + (i % 2) * 24; x < x0 + W; x += 48) { ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x, y + 12); ctx.stroke(); }
      break;
    case "░": case "⁘": case "▚": // tiles
      for (let y = 0; y < H / 16; y++) for (let x = 0; x < W / 16; x++) if ((x + y) % 2) { ctx.fillStyle = alt; ctx.fillRect(x0 + x * 16, y0 + y * 16, 16, 16); }
      if (f === "▚") { ctx.fillStyle = mixHex(r.color, "#5a4a3a", 0.5); for (let i = 0; i < 8; i++) { ctx.globalAlpha = 0.25; ctx.beginPath(); ctx.ellipse(x0 + ((i * 97) % W), y0 + ((i * 53) % H), 10, 6, 0, 0, 7); ctx.fill(); } ctx.globalAlpha = 1; }
      break;
    case "⠂": // grass
      ctx.fillStyle = mixHex("#7fd8a0", "#fffaf0", 0.35); ctx.fillRect(x0, y0, W, H);
      ctx.fillStyle = "#5fbf7f";
      for (let i = 0; i < W * H / 900; i++) { const gx = x0 + ((i * 137) % W), gy = y0 + ((i * 251 + i * i * 7) % H); ctx.fillRect(gx, gy, 2, 5); ctx.fillRect(gx + 3, gy + 1, 2, 4); }
      break;
    case "~": // water-ish blue carpet
      ctx.strokeStyle = alt; ctx.lineWidth = 2;
      for (let y = y0 + 8; y < y0 + H; y += 14) { ctx.beginPath(); for (let x = x0; x <= x0 + W; x += 8) ctx.lineTo(x, y + Math.sin((x + app.t * 30) / 10) * 2); ctx.stroke(); }
      break;
    case "▒": // a big rug in the middle (things swept under it)
      ctx.fillStyle = mixHex("#b5838d", "#fffaf0", 0.2);
      ctx.fillRect(x0 + 18, y0 + 14, W - 36, H - 28);
      ctx.strokeStyle = "#8e5b66"; ctx.lineWidth = 3; ctx.strokeRect(x0 + 24, y0 + 20, W - 48, H - 40);
      break;
    case "∴": case "▪": // stone / metal plates
      ctx.strokeStyle = line; ctx.globalAlpha = 0.25; ctx.lineWidth = 1;
      for (let y = y0; y < y0 + H; y += 32) for (let x = x0; x < x0 + W; x += 32) { ctx.strokeRect(x + 1, y + 1, 30, 30); if (f === "▪") { ctx.fillStyle = line; ctx.fillRect(x + 4, y + 4, 2, 2); ctx.fillRect(x + 26, y + 26, 2, 2); } }
      ctx.globalAlpha = 1;
      break;
    default: // dotted carpet
      ctx.fillStyle = alt;
      for (let y = y0 + 6; y < y0 + H; y += 12) for (let x = x0 + 6 + ((y / 12) % 2) * 6; x < x0 + W; x += 12) ctx.fillRect(x, y, 2, 2);
  }
  ctx.restore();
}

function drawRoom(r) {
  const x0 = r.x * TILE, y0 = r.y * TILE, W = r.w * TILE;
  const wallH = 2 * TILE;
  const selected = r.pups.includes(app.sel) || r.things.includes(app.sel);
  const style = r.look.Border;
  // Floor below the wall.
  floorPattern(r, x0, y0 + wallH);
  const H = r.h * TILE;
  // Back wall.
  const wall = mixHex(r.color, "#2a2840", 0.45);
  if (style !== "hidden") {
    ctx.fillStyle = wall;
    ctx.fillRect(x0, y0, W, wallH);
    ctx.fillStyle = mixHex(r.color, "#2a2840", 0.25);
    ctx.fillRect(x0, y0, W, 8);
    if (style === "thick") { // bricks
      ctx.fillStyle = mixHex(r.color, "#2a2840", 0.6);
      for (let y = 12; y < wallH; y += 10) for (let x = (y / 10) % 2 ? 0 : 12; x < W; x += 24) ctx.fillRect(x0 + x, y0 + y, Math.min(20, W - x), 2);
    } else if (style === "double") { // wainscot with a gold stripe
      ctx.fillStyle = "#e6c35c"; ctx.fillRect(x0, y0 + wallH - 18, W, 4);
    } else if (style === "rounded") { // wood panels
      ctx.fillStyle = mixHex(r.color, "#2a2840", 0.55);
      for (let x = 16; x < W; x += 28) ctx.fillRect(x0 + x, y0 + 12, 3, wallH - 14);
    }
    ctx.fillStyle = "#0002";
    ctx.fillRect(x0, y0 + wallH, W, 6);
  } else {
    // A low picket fence instead of a wall.
    ctx.fillStyle = "#fffaf0";
    for (let x = 4; x < W; x += 14) ctx.fillRect(x0 + x, y0 + wallH - 22, 6, 22);
    ctx.fillRect(x0, y0 + wallH - 14, W, 4);
  }
  // Side walls.
  ctx.fillStyle = selected ? "#ff8fb8" : mixHex(r.color, "#2a2840", 0.55);
  const lw = selected ? 5 : 4;
  ctx.fillRect(x0 - lw, y0, lw, H);
  ctx.fillRect(x0 + W, y0, lw, H);
  ctx.fillRect(x0 - lw, y0 + H, W / 2 - TILE + lw, lw);
  ctx.fillRect(x0 + W / 2 + TILE, y0 + H, W / 2 - TILE + lw, lw);
  // Sign.
  const label = `${r.icon} ${r.title}`;
  ctx.font = "bold 14px ui-rounded, system-ui, sans-serif";
  const tw = ctx.measureText(label).width + 18;
  const sx = x0 + 10, sy = y0 + 14;
  ctx.fillStyle = "#fffdf7"; roundRect(sx, sy, tw, 26, 7); ctx.fill();
  ctx.strokeStyle = "#2a2840"; ctx.lineWidth = 2; ctx.stroke();
  ctx.fillStyle = "#2a2840"; ctx.fillText(label, sx + 9, sy + 18);
  const n = r.pups.length + r.things.length;
  const unread = r.pups.filter((id) => pupOf(id) && pupOf(id).unread).length;
  if (unread) {
    ctx.fillStyle = "#ffd66b"; roundRect(sx + tw + 6, sy + 3, 28, 20, 10); ctx.fill();
    ctx.fillStyle = "#2b2205"; ctx.fillText(`${unread}!`, sx + tw + 12, sy + 18);
  }
  if (r.look.Decor) { ctx.font = "26px system-ui"; ctx.fillText(r.look.Decor, x0 + W - 36, y0 + 44); }
  if (!n && !((r.style || {}).decor || []).length) {
    ctx.fillStyle = "#6b6890"; ctx.font = "italic 13px ui-rounded, system-ui";
    ctx.fillText(r.blurb || "empty", x0 + 14, y0 + wallH + 34);
  }
  if (app.dropRoom === r.key) {
    ctx.fillStyle = "#ffd66b33"; ctx.fillRect(x0, y0, W, H);
    ctx.strokeStyle = "#ffd66b"; ctx.lineWidth = 5; ctx.strokeRect(x0 - 6, y0 - 4, W + 12, H + 10);
  }
  if (build.on && build.room === r.key) {
    ctx.strokeStyle = "#ffd66b"; ctx.lineWidth = 4; ctx.setLineDash([10, 6]);
    ctx.strokeRect(x0 - 8, y0 - 6, W + 16, H + 14); ctx.setLineDash([]);
  }
  drawFixtures(r);
}

function roundRect(x, y, w, h, r) {
  ctx.beginPath();
  ctx.moveTo(x + r, y); ctx.arcTo(x + w, y, x + w, y + h, r); ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r); ctx.arcTo(x, y, x + w, y, r); ctx.closePath();
}

function bubble(x, y, text, bg, fg) {
  ctx.font = "bold 13px ui-rounded, system-ui";
  const w = Math.max(22, ctx.measureText(text).width + 12);
  ctx.fillStyle = bg; roundRect(x - w / 2, y - 22, w, 20, 9); ctx.fill();
  ctx.beginPath(); ctx.moveTo(x - 4, y - 2); ctx.lineTo(x + 4, y - 2); ctx.lineTo(x, y + 4); ctx.fill();
  ctx.fillStyle = fg; ctx.textAlign = "center"; ctx.fillText(text, x, y - 7); ctx.textAlign = "left";
}

function nameTag(x, y, text, selected) {
  ctx.font = "bold 11px ui-rounded, system-ui";
  const t = text.length > 14 ? text.slice(0, 13) + "…" : text;
  const w = ctx.measureText(t).width + 10;
  ctx.fillStyle = selected ? "#f5a3c7" : "#2a2840cc"; roundRect(x - w / 2, y, w, 16, 6); ctx.fill();
  ctx.fillStyle = selected ? "#2a1730" : "#fff"; ctx.textAlign = "center"; ctx.fillText(t, x, y + 12); ctx.textAlign = "left";
}

function drawPup(e) {
  const p = pupOf(e.id);
  if (!p) return;
  const s = 3;
  const px = e.x * TILE, py = e.y * TILE;
  const selected = app.sel === e.id;
  // Desk for working pups.
  if (p.status === "working") {
    ctx.fillStyle = "#9c6b45"; ctx.fillRect(px - 26, py + 4, 52, 10);
    ctx.fillStyle = "#7a5134"; ctx.fillRect(px - 22, py + 14, 4, 10); ctx.fillRect(px + 18, py + 14, 4, 10);
    ctx.fillStyle = "#2a2840"; ctx.fillRect(px - 10, py - 8, 20, 12);
    ctx.fillStyle = Math.floor(app.t * 6 + e.phase) % 2 ? "#a6e3a1" : "#89dceb"; ctx.fillRect(px - 8, py - 6, 16, 8);
  }
  // Shadow.
  ctx.fillStyle = "#0002"; ctx.beginPath(); ctx.ellipse(px, py + 2, 18, 5, 0, 0, 7); ctx.fill();
  const breed = BREEDS[p.agent] || DOG;
  let grid = breed, dy = 0;
  const frame = Math.floor(app.t * 6 + e.phase);
  if (p.status === "offline") grid = DOG_SLEEP;
  else if (e.moving) grid = frame % 2 ? walkOf(breed) : breed;
  else if (p.status === "working" || p.status === "done") grid = frame % 2 ? wagOf(breed) : breed;
  if (p.status === "waiting") dy = -Math.abs(Math.sin(app.t * 7 + e.phase)) * 8;
  const w = grid[0].length * s, h = grid.length * s;
  ctx.save();
  if (e.dir < 0) { ctx.translate(px * 2, 0); ctx.scale(-1, 1); }
  drawPixels(ctx, grid, dogPalette(p.fur, p.status === "offline", p.agent), px - w / 2, py - h + dy, s);
  ctx.restore();
  if (selected) { ctx.strokeStyle = "#f5a3c7"; ctx.lineWidth = 3; ctx.beginPath(); ctx.ellipse(px, py + 2, 24, 8, 0, 0, 7); ctx.stroke(); }
  // Status bubble.
  const top = py - h + dy - 6;
  if (app.fed && app.fed[e.id] && app.t - app.fed[e.id] < 3) bubble(px, top, "🦴 nom", "#ffe9b8", "#3a2a10");
  else if (p.status === "waiting") bubble(px, top, Math.floor(app.t * 3) % 2 ? "!" : "!!", "#ffd66b", "#2b2205");
  else if (p.status === "working") bubble(px, top, ["·", "··", "···"][frame % 3], "#a6e3a1", "#14381f");
  else if (p.status === "done") bubble(px, top, "✓", "#89b4fa", "#0d2347");
  else if (p.status === "offline") { ctx.fillStyle = "#6b6890"; ctx.font = "bold 12px system-ui"; ctx.fillText("z", px + 14, top + 4 - (app.t * 6 % 6)); ctx.fillText("Z", px + 20, top - 6 - (app.t * 4 % 6)); }
  if (p.starred) { ctx.font = "13px system-ui"; ctx.fillText("⭐", px + 16, py - h + 10); }
  if (p.remote) { ctx.font = "12px system-ui"; ctx.fillText("📡", px - 30, py - h + 12); }
  nameTag(px, py + 8, p.title, selected);
}

function drawThing(e) {
  const t = thingOf(e.id);
  if (!t) return;
  const px = e.x * TILE, py = e.y * TILE;
  const on = t.running;
  const selected = app.sel === e.id;
  ctx.fillStyle = "#0002"; ctx.beginPath(); ctx.ellipse(px, py + 2, 22, 5, 0, 0, 7); ctx.fill();
  switch (t.kind) {
    case "monitor": { // a TV on a stand with live bars
      ctx.fillStyle = "#2a2840"; ctx.fillRect(px - 24, py - 40, 48, 34); ctx.fillRect(px - 4, py - 6, 8, 6); ctx.fillRect(px - 14, py - 1, 28, 3);
      ctx.fillStyle = on ? "#14381f" : "#45475a"; ctx.fillRect(px - 20, py - 36, 40, 26);
      if (on) { ctx.fillStyle = "#a6e3a1"; for (let i = 0; i < 6; i++) { const bh = 4 + ((Math.sin(app.t * 4 + i * 1.7 + e.phase) + 1) * 9); ctx.fillRect(px - 18 + i * 6.5, py - 11 - bh, 5, bh); } }
      break;
    }
    case "server": { // campfire
      ctx.fillStyle = "#7a5134"; ctx.fillRect(px - 18, py - 6, 36, 6); ctx.fillRect(px - 14, py - 10, 28, 5);
      if (on) for (let i = 0; i < 3; i++) {
        const fh = 16 + Math.sin(app.t * 9 + i * 2 + e.phase) * 5;
        ctx.fillStyle = ["#f38ba8", "#fab387", "#f9e2af"][i];
        ctx.beginPath(); ctx.moveTo(px - 12 + i * 4, py - 10); ctx.quadraticCurveTo(px - 6 + i * 6, py - 10 - fh * 1.4, px + 2 + i * 4, py - 10); ctx.fill();
      } else { ctx.fillStyle = "#6c7086"; ctx.fillRect(px - 6, py - 16, 3, 6); ctx.fillRect(px + 2, py - 20, 3, 8); }
      break;
    }
    case "logs": { // a scroll of paper with scrolling lines
      ctx.fillStyle = "#fff4e0"; ctx.fillRect(px - 18, py - 40, 36, 38);
      ctx.fillStyle = "#c9a27e"; ctx.fillRect(px - 20, py - 42, 40, 5); ctx.fillRect(px - 20, py - 4, 40, 5);
      ctx.fillStyle = on ? "#585b70" : "#bac2de";
      const off = on ? (app.t * 12) % 6 : 0;
      for (let y = 0; y < 5; y++) ctx.fillRect(px - 14, py - 34 + y * 6 + off, 8 + ((y * 7 + 3) % 20), 2);
      break;
    }
    case "job": { // hourglass
      ctx.fillStyle = "#9c6b45"; ctx.fillRect(px - 14, py - 42, 28, 4); ctx.fillRect(px - 14, py - 4, 28, 4);
      ctx.fillStyle = "#cdd6f4"; ctx.beginPath(); ctx.moveTo(px - 10, py - 38); ctx.lineTo(px + 10, py - 38); ctx.lineTo(px, py - 21); ctx.lineTo(px + 10, py - 4); ctx.lineTo(px - 10, py - 4); ctx.lineTo(px, py - 21); ctx.fill();
      ctx.fillStyle = "#f9e2af"; const sand = on ? (app.t * 0.3 + e.phase) % 1 : 1;
      ctx.beginPath(); ctx.moveTo(px - 8 * (1 - sand), py - 36 + 14 * sand); ctx.lineTo(px + 8 * (1 - sand), py - 36 + 14 * sand); ctx.lineTo(px, py - 22); ctx.fill();
      ctx.fillRect(px - 8 * sand, py - 6 - 10 * sand, 16 * sand, 10 * sand);
      if (on) ctx.fillRect(px - 1, py - 21, 2, 15);
      break;
    }
    default: { // toolbox
      ctx.fillStyle = "#f38ba8"; ctx.fillRect(px - 18, py - 22, 36, 20);
      ctx.fillStyle = "#2a2840"; ctx.fillRect(px - 8, py - 28, 16, 4);
    }
  }
  if (!on) { ctx.font = "11px system-ui"; ctx.fillStyle = "#6b6890"; ctx.fillText("off", px + 16, py - 30); }
  if (selected) { ctx.strokeStyle = "#f5a3c7"; ctx.lineWidth = 3; ctx.beginPath(); ctx.ellipse(px, py + 2, 28, 8, 0, 0, 7); ctx.stroke(); }
  nameTag(px, py + 8, `${t.icon} ${t.name}`, selected);
}

function drawYou() {
  const you = app.you;
  const px = you.x * TILE, py = you.y * TILE, s = 3;
  const grid = you.moving && Math.floor(app.t * 8) % 2 ? HUMAN_WALK : HUMAN;
  ctx.fillStyle = "#0002"; ctx.beginPath(); ctx.ellipse(px, py + 2, 16, 5, 0, 0, 7); ctx.fill();
  const w = grid[0].length * s, h = grid.length * s;
  ctx.save();
  if (you.dir < 0) { ctx.translate(px * 2, 0); ctx.scale(-1, 1); }
  drawPixels(ctx, grid, HUMAN_PAL, px - w / 2, py - h, s);
  ctx.restore();
  nameTag(px, py + 8, "you", false);
}

function render() {
  const dpr = devicePixelRatio;
  ctx.setTransform(1, 0, 0, 1, 0, 0);
  ctx.fillStyle = "#2b2f55";
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

function renderBuilder() {
  const box = $("#build");
  if (!box || box.hidden) return;
  box.innerHTML = "";
  const r = build.room && roomByKey(build.room);
  if (!r) {
    box.appendChild(el("div", "hint", "Click any room to shape it: size, floor, walls, furniture, and blocks that do things. Your own kennels can also pull pups in with a 🧲 magnet."));
    box.appendChild(el("h3", "", "blocks you can place"));
    for (const k of catalog.kinds) {
      const c = el("div", "blockcard");
      c.appendChild(el("div", "head", `${k.icon} ${k.name}`));
      c.appendChild(el("div", "hint", k.blurb));
      box.appendChild(c);
    }
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
    dc.appendChild(b);
  }
  box.appendChild(dc);

  box.appendChild(el("h3", "", "blocks — pick one, then click in the room"));
  const bk = el("div", "chips");
  for (const k of catalog.kinds) bk.appendChild(chip(`${k.icon} ${k.name}`, false, () => {
    build.carry = build.carry && build.carry.kind === k.key ? null : { type: "block", kind: k.key }; renderBuilder();
  }, build.carry && build.carry.kind === k.key ? "carry" : ""));
  box.appendChild(bk);

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
  $("#stage").classList.remove("dropping");
  $("#stage").classList.remove("pupdrag");
  const pupId = e.dataTransfer.getData("application/x-kennel-pup");
  if (pupId) {
    app.dropHover = null;
    const r = kennelRoomAt(e.clientX, e.clientY);
    if (r) await fileInto(r, pupId);
    else toast(app.state.floor === 2 ? "drop it inside one of your kennels" : "switch to 🏠 Kennels to file pups", true);
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


// ---------- inbox: newest sessions not in a kennel yet ----------
let inboxN = 12;
try { inboxN = Number(localStorage.getItem("kennel.inboxN")) || 12; } catch (_) {}
$("#inboxn").value = String(inboxN);
$("#inboxn").onchange = () => { inboxN = Number($("#inboxn").value); try { localStorage.setItem("kennel.inboxN", String(inboxN)); } catch (_) {} loadInbox(); };
$("#inboxtoggle").onclick = () => {
  const box = $("#inbox");
  box.classList.toggle("collapsed");
  $("#inboxtoggle").textContent = box.classList.contains("collapsed") ? "+" : "–";
  try { localStorage.setItem("kennel.inboxCollapsed", box.classList.contains("collapsed") ? "1" : ""); } catch (_) {}
};
try { if (localStorage.getItem("kennel.inboxCollapsed")) { $("#inbox").classList.add("collapsed"); $("#inboxtoggle").textContent = "+"; } } catch (_) {}

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

let inboxItems = [];
async function loadInbox() {
  try { inboxItems = await (await api(`/api/inbox?n=${inboxN}`)).json(); } catch (_) { inboxItems = []; }
  const list = $("#inboxlist");
  list.innerHTML = "";
  if (!inboxItems.length) list.appendChild(el("div", "inpup", "✨ everything is filed"));
  for (const p of inboxItems) {
    const chip = el("div", "inpup");
    chip.title = `${p.title}\n${p.snippet || ""}`;
    chip.dataset.id = p.id;
    chip.append(miniDog(p.fur, p.status, p.agent), el("span", "", p.title), el("span", "age", relAge(p.updated)));
    chip.onclick = () => { app.state.pups[p.id] = app.state.pups[p.id] || p; select(p.id, false); };
    draggablePup(chip, p);
    list.appendChild(chip);
  }
}

// A pup dropped on one of your kennels on the map is filed there.
function kennelRoomAt(clientX, clientY) {
  const rc = canvas.getBoundingClientRect();
  const w = screenToWorld(clientX - rc.left, clientY - rc.top);
  const r = roomAt(w.x, w.y);
  return r && r.key.startsWith("kennel:") ? r : null;
}

async function fileInto(r, pupId) {
  await act("move", pupId, { kennel: Number(r.key.slice(7)) });
  toast(`🏠 filed into ${r.icon} ${r.title}`, false, true);
  loadInbox();
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
  for (const x of app.state.kennels || []) mv.appendChild(new Option(`${x.icon} ${x.name}`, String(x.id)));
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
  loadInbox();
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

// ---------- side panels: jobs & things, artifacts, gear ----------
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

let gearScope = 0;
async function renderGear() {
  const box = $("#gear");
  box.innerHTML = "";
  box.className = "panel";
  let scopes;
  try { scopes = await (await api("/api/gear")).json(); } catch (e) { box.textContent = e.message; return; }
  gearScope = Math.min(gearScope, scopes.length - 1);
  const tabs = el("div", "gearscopes");
  scopes.forEach((sc, i) => {
    const b = el("button", i === gearScope ? "on" : "", (sc.name === "User" ? "👤 " : sc.name === "Plugins" ? "🧩 " : "📁 ") + sc.name);
    b.onclick = () => { gearScope = i; renderGear(); };
    tabs.appendChild(b);
  });
  box.appendChild(tabs);
  const sc = scopes[gearScope];
  if (!sc) return;
  let kind = "";
  for (const it of sc.items) {
    if (it.kind !== kind) { kind = it.kind; box.appendChild(el("h4", "", kind + "s")); }
    const row = el("div", "gitem");
    const w = el("div", "w");
    w.append(el("div", "n", `${it.icon} ${it.name}`), el("div", "d", `${it.level} · ${it.detail}`));
    const sw = el("button", "switch" + (it.enabled ? " on" : ""));
    sw.title = it.toggleable ? (it.enabled ? "disable" : "enable") : "managed outside kennel";
    sw.disabled = !it.toggleable;
    sw.onclick = async () => {
      try {
        const res = await api("/api/gear/toggle", { method: "POST", body: JSON.stringify({ Scope: sc.name, Kind: it.kind, Name: it.name, Level: it.level }) });
        await loadState();
        toast((await res.json()).message, false, true);
      } catch (e) { toast(e.message, true); }
      renderGear();
    };
    row.append(w, sw);
    box.appendChild(row);
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
  screenPos(id) {
    const e = app.ents.get(id);
    if (!e) return null;
    const r = canvas.getBoundingClientRect();
    return { x: r.left + (e.x * TILE - app.cam.x) * app.cam.z, y: r.top + ((e.y - 0.5) * TILE - app.cam.y) * app.cam.z };
  },
};

setTimeout(() => $("#help").classList.add("faded"), 9000);
resize();
Promise.all([loadCatalog(), loadState(), loadTreats()]).then(() => { listen(); requestAnimationFrame(frame); }).catch((e) => toast(e.message, true));
