import { test, expect } from "@playwright/test";
import { open, post, pupId, thingKey, clickEntity, dragTo } from "./helpers.mjs";

test("shows the office with live counts and the session tray", async ({ page }) => {
  await open(page);
  await expect(page.locator("#counts")).toContainText("1 needs you");
  await expect(page.locator("#counts")).toContainText("2 working");
  await expect(page.locator("#floors .floor.on .fname")).toContainText("🏢 Main");
  await expect(page.locator("#pack")).toContainText("Needs you · 1");
  await expect(page.locator("#pack")).toContainText("New, not in a kennel");
  await expect(page.locator("#pack .member", { hasText: "FLAKY-CI" })).toBeVisible();
  const rooms = await page.evaluate(() => window.kennel.app.rooms.map((r) => r.title));
  expect(rooms).toEqual(expect.arrayContaining(["The Yard", "Workshop", "Firehouse", "Helping Out", "Best in Show", "Payments"]));
  // Each pup stands in one room at most.
  const ids = await page.evaluate(() => window.kennel.app.rooms.flatMap((r) => r.pups));
  expect(new Set(ids).size).toBe(ids.length);
});

test("tray: filter, collapse groups, drag a session onto a kennel to file it", async ({ page }) => {
  await open(page);
  const tray = page.locator("#pack");
  await tray.locator(".trayq").fill("pg-upgrade");
  await expect(tray.locator(".member")).toHaveCount(1);
  await tray.locator(".trayq").fill("");
  await tray.locator("h4", { hasText: "New" }).click();
  await expect(tray.locator(".group.closed")).toHaveCount(1);
  await expect(tray.locator(".group.closed .member")).toHaveCount(0);
  await tray.locator("h4", { hasText: "New" }).click();

  const id = await pupId(page, "RATE-LIMITS");
  const payments = await page.evaluate(() => window.kennel.app.rooms.find((r) => r.title === "Payments").key);
  await page.evaluate((k) => {
    const app = window.kennel.app, st = document.querySelector("#stage"), r = app.rooms.find((x) => x.key === k);
    app.cam.x = (r.x + r.w / 2) * 32 - st.clientWidth / app.cam.z / 2;
    app.cam.y = (r.y + r.h / 2) * 32 - st.clientHeight / app.cam.z / 2;
  }, payments);
  const at = await page.evaluate((k) => window.kennel.roomPos(k, 0.5, 0.6), payments);
  await tray.locator(`.member[data-id="${id}"]`).dragTo(page.locator("#map"), { targetPosition: { x: at.x - (await page.locator("#map").boundingBox()).x, y: at.y - (await page.locator("#map").boundingBox()).y } });
  await expect(page.locator("#toast")).toContainText("filed into 💳 Payments");
  await expect(tray.locator(`.member[data-id="${id}"] .pill.kennel`)).toHaveText("Payments");
  await expect.poll(() => page.evaluate((k) => window.kennel.app.rooms.find((r) => r.key === k).pups.length, payments)).toBe(1);
  await page.keyboard.press("Meta+z");
  await expect.poll(() => page.evaluate((k) => window.kennel.app.rooms.find((r) => r.key === k).pups.length, payments)).toBe(0);
});

test("projects: start one, link a folder, its own floors and sessions, remembered, removable", async ({ page }) => {
  await open(page);
  await expect(page.locator("#spaces .space.on .fname")).toContainText("My work");
  // A project is a folder: start one from a folder your sessions used.
  const all = await page.locator("#pack .member").count();
  await page.locator("#addspacebtn").click();
  await page.locator("#ctxmenu button", { hasText: "📁 infra" }).click();
  await expect(page.locator("#spaces .space.on .fname")).toContainText("infra");
  await expect(page.locator("#spaces .space.on .fcount")).toContainText("/code/infra");
  // A fresh project starts with one floor and no rooms, and only that folder's sessions.
  await expect(page.locator("#floors .floor.on .fname")).toHaveText("🏢 Main");
  await expect.poll(() => page.evaluate(() => window.kennel.app.rooms.length)).toBe(0);
  await expect.poll(() => page.locator("#pack .member").count()).toBeLessThan(all);
  const projects = await page.evaluate(() => [...document.querySelectorAll("#pack .member")].map((m) => window.kennel.app.state.pups[m.dataset.id].project));
  expect(new Set(projects)).toEqual(new Set(["/Users/demo/code/infra"]));
  // Smart rooms here only pick from that folder.
  await page.keyboard.press("b");
  await page.locator("#build .smartgrid .smart", { hasText: "Lobby" }).click();
  await page.waitForFunction(() => window.kennel.app.rooms.some((r) => r.smart === "lobby"));
  const lobby = await page.evaluate(() => [...new Set(window.kennel.app.rooms.find((x) => x.smart === "lobby").pups.map((id) => window.kennel.app.state.pups[id].project))]);
  expect(lobby).toEqual(["/Users/demo/code/infra"]);
  await page.keyboard.press("b");
  // Reload: still in Infra.
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.state && window.kennel.app.state.spaces.length);
  await expect(page.locator("#spaces .space.on .fname")).toContainText("infra");
  // Remove it (it has a kennel, so it asks): we land in My work; undo brings it back.
  page.once("dialog", (d) => { expect(d.message()).toContain("move to My work"); d.accept(); });
  await page.locator("#spaces .space.on button.dots").click();
  await page.locator("#ctxmenu button", { hasText: "Remove project" }).click();
  await expect(page.locator("#spaces .space.on .fname")).toContainText("My work");
  await expect(page.locator("#spaces .space", { hasText: "infra" })).toHaveCount(0);
  await page.keyboard.press("Meta+z");
  await expect(page.locator("#spaces .space", { hasText: "infra" })).toHaveCount(1);
  // Leave things as they were for the other tests: no Infra, no extra floor or kennel.
  const id = await page.evaluate(() => window.kennel.app.state.spaces.find((x) => x.name === "infra").id);
  await page.locator("#spaces .space", { hasText: "infra" }).click();
  await page.waitForFunction(() => window.kennel.app.state.spaces.find((x) => x.key === window.kennel.app.state.space)?.name === "infra");
  const lobbyId = await page.evaluate(() => window.kennel.app.rooms.find((r) => r.smart === "lobby").kennel);
  const floorId = await page.evaluate(() => window.kennel.app.state.floor);
  await post(page, `/api/kennels/${lobbyId}/delete`, {});
  await post(page, `/api/spaces/${id}/delete`, {});
  await post(page, `/api/floors/${floorId}/delete`, {});
});
test("filters are remembered across reloads: list text, collapsed group, library filters", async ({ page }) => {
  await open(page);
  await page.locator("#pack .trayq").fill("pg");
  await page.locator("#pack h4", { hasText: "New" }).click();
  await page.keyboard.press("l");
  await page.locator("#libagent").selectOption("codex");
  await page.locator("#libgroup").selectOption("agent");
  await page.keyboard.press("Escape");
  await page.waitForTimeout(500);
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.state && document.querySelector("#pack .trayq"));
  await expect(page.locator("#pack .trayq")).toHaveValue("pg");
  await expect(page.locator("#pack .group.closed")).toHaveCount(1);
  await page.keyboard.press("l");
  await expect(page.locator("#libagent")).toHaveValue("codex");
  await expect(page.locator("#libgroup")).toHaveValue("agent");
  // Put things back for the other tests.
  await page.locator("#libagent").selectOption("");
  await page.locator("#libgroup").selectOption("day");
  await page.keyboard.press("Escape");
  await page.locator("#pack h4", { hasText: "New" }).click();
  await page.locator("#pack .trayq").fill("");
  await page.waitForTimeout(500);
});
test("clicking a pup opens its card, double-click resumes it", async ({ page }) => {
  await open(page);
  const id = await pupId(page, "PG-UPGRADE");
  await clickEntity(page, id);
  const card = page.locator("#card");
  await expect(card).toBeVisible();
  await expect(card).toContainText("PG-UPGRADE");
  await expect(card).toContainText("idle");
  await expect(card).toContainText("Staging is on Postgres 16");
  await page.waitForTimeout(500);
  await clickEntity(page, id, { clickCount: 2 });
  await expect(page.locator("#toast")).toContainText(`claude --resume ${id}`);
});

test("card buttons: star shows up in the pack, read opens the transcript", async ({ page }) => {
  await open(page);
  await page.locator("#pack .member", { hasText: "CHECKOUT-V2" }).click();
  const card = page.locator("#card");
  await expect(card).toContainText("CHECKOUT-V2");
  await card.getByRole("button", { name: "☆ Star" }).click();
  await expect(page.locator("#pack .member", { hasText: /★.*CHECKOUT-V2/ })).toBeVisible();
  await expect(card.getByRole("button", { name: "★ Unstar" })).toBeVisible();

  await card.getByRole("button", { name: "📜 Read" }).click();
  const tr = page.locator("#transcript");
  await expect(tr).toBeVisible();
  await expect(tr).toContainText("rebuild the checkout flow");
  await expect(tr).toContainText("PaymentElement");
  await page.locator("#tr-find").fill("apple pay");
  await expect(page.locator("#tr-body .msg[style*='outline']").first()).toBeVisible();
  await page.locator("#tr-close").click();
  await expect(tr).toBeHidden();
});

test("search finds pups by content with highlighted snippets", async ({ page }) => {
  await open(page);
  await page.keyboard.press("/");
  await expect(page.locator("#q")).toBeFocused();
  await page.keyboard.type("paymentelement");
  const results = page.locator("#results");
  await expect(results).toBeVisible();
  await expect(results.locator(".result").first()).toContainText("CHECKOUT-V2");
  await expect(results.locator("mark").first()).toHaveText(/PaymentElement/i);
  await page.keyboard.press("Enter");
  await expect(page.locator("#card")).toContainText("CHECKOUT-V2");
});

test("ask answers with clickable citations", async ({ page }) => {
  await open(page);
  await page.locator("#side .tabs button[data-tab=ask]").click();
  await page.locator("#askq").fill("why did the payment form flicker?");
  await page.locator("#askform button").click();
  await expect(page.locator("#answer")).toContainText("memoizing its options", { timeout: 15000 });
  await expect(page.locator("#sources .source").first()).toContainText("CHECKOUT-V2");
  await page.locator("#answer .cite", { hasText: "[1]" }).click();
  await expect(page.locator("#card")).toContainText("CHECKOUT-V2");
});

test("things: running ones focus, stopped ones start", async ({ page }) => {
  await open(page);
  const dev = await thingKey(page, "atlas dev");
  await clickEntity(page, dev);
  const card = page.locator("#card");
  await expect(card).toContainText("atlas dev");
  await expect(card).toContainText("running");
  await expect(card).toContainText("$ pnpm dev");
  await expect(card.getByRole("button", { name: "✋ Stop" })).toBeVisible();

  const tf = await thingKey(page, "tf plan");
  await clickEntity(page, tf);
  await expect(card).toContainText("stopped");
  await card.getByRole("button", { name: "▶ Start" }).click();
  await expect(page.locator("#toast")).toContainText("terraform plan -out tf.plan");
});

test("floors: add, switch, move a kennel by dragging it onto a floor tab, rename, remove with undo", async ({ page }) => {
  await open(page);
  await page.locator("#addfloorbtn").click();
  await page.locator("#floors .fedit").fill("Side projects");
  await page.locator("#floors .fedit").press("Enter");
  await expect(page.locator("#floors .floor.on .fname")).toHaveText("🏢 Side projects");
  await expect(page.locator("#floors .floor.on .fcount")).toHaveText("0 kennels · 0 pups");
  await expect.poll(() => page.evaluate(() => window.kennel.app.rooms.length)).toBe(0);
  // The floor is remembered across reloads.
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.state);
  await expect(page.locator("#floors .floor.on .fname")).toHaveText("🏢 Side projects");

  // Back on Main, drag Payments by its wall onto the new floor's tab.
  await page.locator("#floors .floor[data-floor]").first().click();
  await page.waitForFunction(() => window.kennel.app.rooms.some((r) => r.title === "Payments"));
  const key = await page.evaluate(() => window.kennel.app.rooms.find((r) => r.title === "Payments").key);
  await page.evaluate((k) => {
    const app = window.kennel.app, st = document.querySelector("#stage"), r = app.rooms.find((x) => x.key === k);
    app.cam.x = (r.x + r.w / 2) * 32 - st.clientWidth / app.cam.z / 2;
    app.cam.y = (r.y + r.h / 2) * 32 - st.clientHeight / app.cam.z / 2;
  }, key);
  const wall = await page.evaluate((k) => window.kennel.wallPos(k), key);
  const tab = await page.locator("#floors .floor", { hasText: "Side projects" }).boundingBox();
  await page.mouse.move(wall.x, wall.y);
  await page.mouse.down();
  for (let i = 1; i <= 10; i++) await page.mouse.move(wall.x + ((tab.x + tab.width / 2 - wall.x) * i) / 10, wall.y + ((tab.y + tab.height / 2 - wall.y) * i) / 10);
  await expect(page.locator("#floors .floor.droptarget .fname")).toHaveText("🏢 Side projects");
  await page.mouse.up();
  await expect(page.locator("#toast")).toContainText("moved Payments to Side projects");
  await expect.poll(() => page.evaluate(() => window.kennel.app.rooms.some((r) => r.title === "Payments"))).toBe(false);

  // Rename and remove from the tab's right-click menu; undo brings it back.
  // Rename in place (double-click), set an icon and reorder from the ⋯ menu.
  await page.locator("#floors .floor", { hasText: "Side projects" }).dblclick();
  await page.locator("#floors .fedit").fill("Garden");
  await page.locator("#floors .fedit").press("Enter");
  await expect(page.locator("#floors .floor", { hasText: "Garden" })).toBeVisible();
  await page.locator("#floors .floor", { hasText: "Garden" }).locator("button.dots").click();
  await page.locator("#ctxmenu button", { hasText: "Icon" }).click();
  await page.locator("#ctxmenu button", { hasText: "🌱" }).click();
  await expect(page.locator("#floors .floor .fname", { hasText: "Garden" })).toContainText("🌱");
  await page.locator("#floors .floor", { hasText: "Garden" }).locator("button.dots").click();
  await page.locator("#ctxmenu button", { hasText: "Move up" }).click();
  await expect(page.locator("#floors .floor .fname").first()).toContainText("Garden");
  const lastBox = await page.locator("#floors .floor").last().boundingBox();
  await page.locator("#floors .floor", { hasText: "Garden" }).dragTo(page.locator("#floors .floor").last(), { targetPosition: { x: 20, y: lastBox.height - 4 } });
  await expect.poll(() => page.locator("#floors .floor .fname").last().textContent()).toContain("Garden");
  // Remove asks first when the floor holds kennels, and says where they go.
  page.once("dialog", (d) => { expect(d.message()).toContain("move to Main"); d.accept(); });
  await page.locator("#floors .floor", { hasText: "Garden" }).locator("button.dots").click();
  await page.locator("#ctxmenu button", { hasText: "Remove floor" }).click();
  await expect(page.locator("#floors .floor", { hasText: "Garden" })).toHaveCount(0);
  await expect.poll(() => page.evaluate(() => window.kennel.app.rooms.some((r) => r.title === "Payments"))).toBe(true);
  await page.keyboard.press("Meta+z");
  await expect(page.locator("#floors .floor", { hasText: "Garden" })).toBeVisible();
  // Leave things as they were: Payments back on Main, the extra floor gone.
  const garden = await page.evaluate(() => window.kennel.app.state.floors.find((f) => f.name === "Garden").id);
  const main = await page.evaluate(() => window.kennel.app.state.floors[0].id);
  await post(page, `/api/kennels/${key.slice(7)}/floor`, { floor: main });
  await post(page, `/api/floors/${garden}/delete`, {});
});

test("api rejects requests without the token", async ({ request }) => {
  const res = await request.get("/api/state");
  expect(res.status()).toBe(401);
});

test("renders without console errors and draws the map", async ({ page }) => {
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
  await open(page);
  await page.waitForTimeout(800);
  const painted = await page.evaluate(() => {
    const c = document.querySelector("#map");
    const d = c.getContext("2d").getImageData(c.width / 2, c.height / 2, 40, 40).data;
    const colors = new Set();
    for (let i = 0; i < d.length; i += 4) colors.add(`${d[i]},${d[i + 1]},${d[i + 2]}`);
    return colors.size;
  });
  expect(painted).toBeGreaterThan(3);
  expect(errors).toEqual([]);
});

test("codex and cursor pups: breeds, convert and export", async ({ page }) => {
  await open(page);
  const pups = await page.evaluate(() => Object.values(window.kennel.app.state.pups).map((p) => p.agent));
  expect(pups).toEqual(expect.arrayContaining(["claude", "codex", "cursor"]));
  await page.keyboard.press("/");
  await page.keyboard.type("agent:codex csv");
  await expect(page.locator("#results .result").first()).toContainText("speed up the CSV export");
  await page.keyboard.press("Enter");
  const card = page.locator("#card");
  await expect(card).toContainText("Codex");
  page.once("dialog", (d) => d.accept("claude"));
  await card.locator("button.dots").click();
  await page.locator("#ctxmenu button", { hasText: "Convert" }).click();
  await expect(page.locator("#toast")).toContainText("claude -n");
  await card.locator("button.dots").click();
  await page.locator("#ctxmenu button", { hasText: "Export" }).click();
  await expect(page.locator("#toast")).toContainText("exported to");
});

test("shelves: pin links to kennels from the panel, see them on the wall and the card, ⋯ to move or remove", async ({ page }) => {
  await open(page);
  await page.locator("#side .tabs button[data-tab=art]").click();
  const panel = page.locator("#art");
  const payments = await page.evaluate(() => window.kennel.app.state.kennels.find((k) => k.name === "Payments"));
  await panel.locator(".addshelf input").fill("https://docs.acme.dev/checkout-v2");
  await panel.locator(".addshelf select").selectOption(String(payments.id));
  await panel.locator(".addshelf button").click();
  await expect(panel.locator("h4", { hasText: "Payments · 1" })).toBeVisible();
  const row = panel.locator(".item", { hasText: "docs.acme.dev" }).first();
  await expect(row).toBeVisible();

  // It sits on the kennel's back wall; clicking the room lists it on the card.
  const key = "kennel:" + payments.id;
  await page.evaluate((k) => {
    const app = window.kennel.app, st = document.querySelector("#stage"), r = app.rooms.find((x) => x.key === k);
    app.cam.x = (r.x + r.w / 2) * 32 - st.clientWidth / app.cam.z / 2;
    app.cam.y = (r.y + r.h / 2) * 32 - st.clientHeight / app.cam.z / 2;
  }, key);
  await expect.poll(() => page.evaluate((k) => [...window.kennel.app.ents.values()].filter((e) => e.kind === "item" && e.room.key === k).length, key)).toBe(1);
  const spot = await page.evaluate((k) => window.kennel.roomPos(k, 0.9, 0.9), key);
  await page.mouse.click(spot.x, spot.y);
  await expect(page.locator("#card .cardshelf")).toContainText("docs.acme.dev");

  // Pin from the card too, then ⋯ → take it off the shelf, and remove with undo.
  await page.locator("#card .cardshelf input").fill("#payments-oncall");
  await page.locator("#card .cardshelf input").press("Enter");
  await expect(page.locator("#card .cardshelf .item")).toHaveCount(2);
  await page.locator("#card .cardshelf .item", { hasText: "payments-oncall" }).locator("button.dots").click();
  await page.locator("#ctxmenu button", { hasText: "Take off the shelf" }).click();
  await expect(page.locator("#card .cardshelf .item")).toHaveCount(1);
  await expect(panel.locator("h4", { hasText: "Loose" })).toBeVisible();
  await panel.locator(".item", { hasText: "payments-oncall" }).locator("button.dots").click();
  await page.locator("#ctxmenu button", { hasText: "Remove" }).click();
  await expect(panel.locator(".item", { hasText: "payments-oncall" })).toHaveCount(0);
  await page.keyboard.press("Meta+z");
  await expect(panel.locator(".item", { hasText: "payments-oncall" })).toHaveCount(1);
});

test("artifacts from sessions: pin to a kennel with ⋯, or hide them (undoable)", async ({ page }) => {
  await open(page);
  await page.locator("#side .tabs button[data-tab=art]").click();
  const panel = page.locator("#art");
  const art = panel.locator(".item", { hasText: "PG16 cutover runbook" });
  await expect(art).toHaveCount(1);
  await art.locator("button.dots").click();
  await page.locator("#ctxmenu button", { hasText: "Pin to 💳 Payments" }).click();
  await expect(page.locator("#toast")).toContainText("pinned to 💳 Payments");
  await expect(panel.locator(".item", { hasText: "PG16 cutover runbook" })).toHaveCount(2);
  await panel.locator(".item", { hasText: "PG16 cutover runbook" }).last().locator("button.dots").click();
  await page.locator("#ctxmenu button", { hasText: "Hide" }).click();
  await expect(panel.locator(".item", { hasText: "PG16 cutover runbook" })).toHaveCount(1);
  await page.keyboard.press("Meta+z");
  await page.locator("#side .tabs button[data-tab=art]").click();
  await expect(panel.locator(".item", { hasText: "PG16 cutover runbook" })).toHaveCount(2);
});

test("feeding: drag a shelf item or a desktop file onto a pup", async ({ page }) => {
  await open(page);
  await post(page, "/api/treats", { name: "runbook", value: "/Users/demo/docs/runbook.md" });
  await page.locator("#side .tabs button[data-tab=art]").click();
  const item = page.locator("#art .item", { hasText: "runbook" }).first();
  await expect(item).toBeVisible();
  const id = await pupId(page, "PG-UPGRADE");
  await page.evaluate((i) => window.kennel.reveal(i), id);
  const pos = await page.evaluate((i) => window.kennel.screenPos(i), id);
  const box = await page.locator("#map").boundingBox();
  await item.dragTo(page.locator("#map"), { targetPosition: { x: pos.x - box.x, y: pos.y - box.y } });
  await expect(page.locator("#toast")).toContainText("runbook");

  await page.evaluate(({ x, y }) => {
    const dt = new DataTransfer();
    dt.items.add(new File(["# notes\nremember the rollback plan"], "notes.md", { type: "text/markdown" }));
    const c = document.querySelector("#map");
    for (const type of ["dragover", "drop"]) c.dispatchEvent(new DragEvent(type, { clientX: x, clientY: y, dataTransfer: dt, bubbles: true, cancelable: true }));
  }, pos);
  await expect(page.locator("#toast")).toContainText("notes.md");
});

test("the terminal panel: messages, resume here, type into it, move to a tab", async ({ page }) => {
  await open(page);
  const id = await pupId(page, "RATE-LIMITS");
  await clickEntity(page, id);
  // Clicking a pup on the map opens its terminal panel; asleep, it shows the latest messages.
  await expect(page.locator('#side .tabs button[data-tab="term"]')).toHaveClass(/on/);
  const term = page.locator("#term");
  await expect.poll(() => term.locator(".m.user, .m.assistant").count()).toBeGreaterThan(0);
  await term.locator("button", { hasText: "Resume here" }).click();
  const screen = term.locator(".xterm-rows");
  await expect(screen).toContainText(`would run: claude --resume ${id}`);
  await page.keyboard.type("hello pty");
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("hello pty");
  await page.locator('#side .tabs button[data-tab="pack"]').click();
  await expect(page.locator(`#pack .member[data-id="${id}"] .pill.here`)).toBeVisible();
  // Back to it: it re-attaches with the scrollback.
  await page.locator(`#pack .member[data-id="${id}"]`).click();
  await page.locator('#side .tabs button[data-tab="term"]').click();
  await expect(screen).toContainText("hello pty");
  await term.locator("button", { hasText: "Stop" }).click();
  await expect.poll(() => page.evaluate((i) => window.kennel.app.state.pups[i].term, id)).toBe(false);
  // A pup running in another terminal can still continue here (a second copy), or Go to.
  const far = await page.evaluate(() => Object.values(window.kennel.app.state.pups).find((p) => p.status !== "offline" && !p.pane)?.id);
  await clickEntity(page, far);
  await expect(term.locator("button", { hasText: "Go to it" })).toBeVisible();
  await term.locator("button", { hasText: "Continue here" }).click();
  await expect(term.locator(".xterm-rows")).toContainText(`would run: claude --resume ${far}`);
  await term.locator("button", { hasText: "Stop" }).click();
});

test("sessions list: group and sort", async ({ page }) => {
  await open(page);
  const tray = page.locator("#pack");
  await tray.locator('select[data-key="group"]').selectOption("role");
  await expect(tray.locator("h4", { hasText: "Firefighter" })).toBeVisible();
  await tray.locator('select[data-key="group"]').selectOption("project");
  await expect(tray.locator("h4", { hasText: "📁 infra" })).toBeVisible();
  await tray.locator('select[data-key="group"]').selectOption("none");
  await tray.locator('select[data-key="sort"]').selectOption("name");
  const names = await tray.locator(".member .name").allTextContents();
  const plain = names.map((n) => n.replace(/^[^A-Za-z0-9]+/, ""));
  expect(plain).toEqual([...plain].sort((a, b) => a.localeCompare(b)));
  await tray.locator('select[data-key="group"]').selectOption("status");
  await tray.locator('select[data-key="sort"]').selectOption("recent");
});
test("pups wear their role: a role icon on the card and in the New list", async ({ page }) => {
  await open(page);
  const roles = await page.evaluate(() => Object.fromEntries(Object.values(window.kennel.app.state.pups).map((p) => [p.title, p.role])));
  expect(roles["INCIDENT-0612"]).toBe("firefighter");
  expect(roles["review infra#91"]).toBe("reviewer");
  const id = await pupId(page, "INCIDENT-0612");
  await clickEntity(page, id);
  await expect(page.locator("#card .role")).toHaveText("🧑‍🚒");
  await expect(page.locator("#pack .member", { hasText: "review infra#91" })).toContainText("🧐");
});

test("agent messages render as markdown, safely", async ({ page }) => {
  await open(page);
  const id = await pupId(page, "FLAKY-CI");
  await clickEntity(page, id);
  const msgs = page.locator("#term .msgs");
  await expect(msgs.locator(".m.assistant code", { hasText: "workerIndex" })).toBeVisible();
  const html = await page.evaluate(() => {
    const n = window.kennel.markdown("It's **`gcp-us-east4`**:\n\n- **Zone:** `us-east4-b`\n- [docs](https://example.com)\n\n```\nterraform apply\n```\n<img src=x onerror=alert(1)>");
    return n.innerHTML;
  });
  expect(html).toContain("<strong><code>gcp-us-east4</code></strong>");
  expect(html).toContain("<li><strong>Zone:</strong> <code>us-east4-b</code></li>");
  expect(html).toContain('<a href="https://example.com" target="_blank" rel="noopener noreferrer">docs</a>');
  expect(html).toContain("<pre><code>terraform apply</code></pre>");
  expect(html).not.toContain("<img");
  expect(html).toContain("&lt;img src=x onerror=alert(1)&gt;");
});

test("organize kennels into floors: suggestions, review, apply, one undo", async ({ page }) => {
  await open(page);
  const floorsBefore = await page.evaluate(() => window.kennel.app.state.floors.length);
  await page.locator("#organizefloors").click();
  const sheet = page.locator("#floorsheet");
  await expect(sheet.locator(".sortrow").first()).toBeVisible();
  await expect(sheet.locator("option", { hasText: "(new)" }).first()).toBeAttached();
  const payments = sheet.locator(".sortrow", { hasText: "Payments" }).locator("select");
  await expect(payments).toHaveValue("Work");
  await sheet.locator(".apply").click();
  await expect(page.locator("#toast")).toContainText("onto their floors");
  await expect(page.locator("#floors .floor .fname", { hasText: "Work" })).toBeVisible();
  expect(await page.evaluate(() => window.kennel.app.state.floors.length)).toBeGreaterThan(floorsBefore);
  await page.locator("#floors .floor", { hasText: "Work" }).click();
  await page.waitForFunction(() => window.kennel.app.rooms.some((r) => r.title === "Payments"));
  await expect(page.locator("#floors .floor.on .fcount")).toContainText("kennel");
  await page.keyboard.press("Meta+z");
  await expect.poll(() => page.evaluate(() => window.kennel.app.state.floors.length)).toBe(floorsBefore);
  await page.locator("#floors .floor[data-floor]").first().click();
  await page.waitForFunction(() => window.kennel.app.rooms.some((r) => r.title === "Payments"));
});

test("kennel items stand in the room like pups: card, drag to another kennel, drag out", async ({ page }) => {
  await open(page);
  const payments = await page.evaluate(() => window.kennel.app.state.kennels.find((k) => k.name === "Payments"));
  const t = await post(page, "/api/treats", { value: "https://docs.acme.dev/limits", kennel: payments.id });
  const id = "item:" + t.id;
  await page.waitForFunction((i) => window.kennel.app.ents.has(i), id);
  await clickEntity(page, id);
  await expect(page.locator("#card")).toContainText("docs.acme.dev/limits");
  await expect(page.locator("#card")).toContainText("in 💳 Payments");
  // Drag it into another kennel of yours.
  const other = await post(page, "/api/kennels", { Name: "Limits", Icon: "🚦" });
  await page.waitForFunction((k) => window.kennel.app.rooms.some((r) => r.key === k), "kennel:" + other.ID);
  await page.evaluate(() => { window.kennel.app.cam.z = 0.5; });
  const from = await page.evaluate((i) => { window.kennel.reveal(i); return window.kennel.screenPos(i); }, id);
  const to = await page.evaluate((k) => window.kennel.roomPos(k, 0.5, 0.6), "kennel:" + other.ID);
  await dragTo(page, from, to);
  await expect(page.locator("#toast")).toContainText("pinned to 🚦 Limits");
  await expect.poll(() => page.evaluate((i) => window.kennel.app.ents.get(i)?.room.title, id)).toBe("Limits");
  // Right-click → take it off the shelf: it leaves the map but stays in 📎.
  const pos = await page.evaluate((i) => { window.kennel.reveal(i); return window.kennel.screenPos(i); }, id);
  await page.mouse.click(pos.x, pos.y, { button: "right" });
  await page.locator("#ctxmenu button", { hasText: "Take off the shelf" }).click();
  await expect.poll(() => page.evaluate((i) => window.kennel.app.ents.has(i), id)).toBe(false);
});

test("a pup's look: pick a hat and fur, Auto puts it back", async ({ page }) => {
  await open(page);
  const id = await pupId(page, "ONBOARDING");
  await clickEntity(page, id);
  await page.locator("#card button", { hasText: "Look" }).click();
  const pick = page.locator("#card .lookpick");
  await pick.locator(".hats button", { hasText: "🧑‍🚒" }).click();
  await expect.poll(() => page.evaluate((i) => window.kennel.app.state.pups[i].role, id)).toBe("firefighter");
  await pick.locator(".furs button[title='#89b4fa']").click();
  await expect.poll(() => page.evaluate((i) => window.kennel.app.state.pups[i].fur, id)).toBe("#89b4fa");
  expect(await page.evaluate((i) => window.kennel.app.state.pups[i].role, id)).toBe("firefighter");
  await pick.locator(".hats button", { hasText: "🚫" }).click();
  await expect.poll(() => page.evaluate((i) => window.kennel.app.state.pups[i].role, id)).toBe("");
  await pick.locator(".hats button", { hasText: "Auto" }).click();
  await pick.locator(".furs button", { hasText: "Auto" }).click();
  await expect.poll(() => page.evaluate((i) => window.kennel.app.state.pups[i].lookFur, id)).toBe("");
});

test("renaming a pup renames the Claude session too", async ({ page }) => {
  await open(page);
  const id = await pupId(page, "RATE-LIMITS");
  // Asleep: the transcript gets the same title record /rename writes.
  page.once("dialog", (d) => d.accept("THROTTLE"));
  const pos = await page.evaluate((i) => { window.kennel.reveal(i); return window.kennel.screenPos(i); }, id);
  await page.mouse.click(pos.x, pos.y, { button: "right" });
  await page.locator("#ctxmenu button", { hasText: "Rename" }).click();
  await expect(page.locator("#toast")).toContainText("claude --resume shows it too");
  await expect.poll(() => page.evaluate((i) => window.kennel.app.state.pups[i].title, id)).toBe("THROTTLE");
  // Running in the panel: kennel types /rename into the session.
  await clickEntity(page, id);
  await page.locator("#term button", { hasText: "Resume here" }).click();
  await expect.poll(() => page.evaluate(() => window.kennel.termText())).toContain("would run");
  page.once("dialog", (d) => d.accept("LIMITER"));
  const pos2 = await page.evaluate((i) => { window.kennel.reveal(i); return window.kennel.screenPos(i); }, id);
  await page.mouse.click(pos2.x, pos2.y, { button: "right" });
  await page.locator("#ctxmenu button", { hasText: "Rename" }).click();
  await expect(page.locator("#toast")).toContainText("in the session too");
  await expect.poll(() => page.evaluate(() => window.kennel.termText())).toContain("/rename LIMITER");
  await page.locator("#term button", { hasText: "Stop" }).click();
  page.once("dialog", (d) => d.accept("RATE-LIMITS"));
  await page.mouse.click(pos2.x, pos2.y, { button: "right" });
  await page.locator("#ctxmenu button", { hasText: "Rename" }).click();
});

test("chats in the navigator: pick one to go to it (switching floors), star it in sync", async ({ page }) => {
  await open(page);
  const chats = page.locator("#chats .chat");
  await expect(chats.first()).toBeVisible();
  // Active ones come first.
  const first = await chats.first().getAttribute("data-id");
  expect(await page.evaluate((i) => window.kennel.app.state.pups[i].status, first)).toMatch(/waiting|working/);
  // From another floor, picking a chat hops to the floor it stands on and selects it.
  const f = await post(page, "/api/floors", { Name: "Upstairs", Icon: "🏢", Space: await page.evaluate(() => window.kennel.app.state.space) });
  await page.reload();
  await page.waitForFunction(() => window.kennel && window.kennel.app.state);
  await page.locator("#floors .floor", { hasText: "Upstairs" }).click();
  await expect(page.locator("#floors .floor.on .fname")).toHaveText("🏢 Upstairs");
  const id = await pupId(page, "CHECKOUT-V2");
  await page.locator(`#chats .chat[data-id="${id}"]`).click();
  await expect(page.locator("#floors .floor.on .fname")).toContainText("Main");
  await expect.poll(() => page.evaluate(() => window.kennel.app.sel)).toBe(id);
  await expect.poll(() => page.evaluate((i) => window.kennel.app.ents.has(i), id)).toBe(true);
  await expect(page.locator("#card")).toContainText("CHECKOUT-V2");
  // Star from the list; the card and the map agree.
  const was = await page.evaluate((i) => window.kennel.app.state.pups[i].starred, id);
  await page.locator(`#chats .chat[data-id="${id}"] .star`).click();
  await expect.poll(() => page.evaluate((i) => window.kennel.app.state.pups[i].starred, id)).toBe(!was);
  await expect(page.locator(`#chats .chat[data-id="${id}"] .star`)).toHaveText(was ? "☆" : "★");
  await expect(page.locator("#card")).toContainText(was ? "☆ Star" : "★ Unstar");
  await page.locator(`#chats .chat[data-id="${id}"] .star`).click();
  await expect.poll(() => page.evaluate((i) => window.kennel.app.state.pups[i].starred, id)).toBe(was);
  await post(page, `/api/floors/${f.ID}/delete`, {});
});

test("room objects: placed and resized in build mode; a pup dropped on a rug goes under it", async ({ page }) => {
  await open(page);
  const key = await page.evaluate(() => window.kennel.app.rooms.find((r) => r.title === "Payments").key);
  await page.evaluate((k) => {
    const app = window.kennel.app, st = document.querySelector("#stage"), r = app.rooms.find((x) => x.key === k);
    app.cam.x = (r.x + r.w / 2) * 32 - st.clientWidth / app.cam.z / 2;
    app.cam.y = (r.y + r.h / 2) * 32 - st.clientHeight / app.cam.z / 2;
  }, key);
  await page.keyboard.press("b");
  const wall = await page.evaluate((k) => window.kennel.wallPos(k), key);
  await page.mouse.click(wall.x, wall.y);
  const panel = page.locator("#build");
  await panel.locator(".chips button", { hasText: "🧹 Rug" }).click();
  let at = await page.evaluate((k) => window.kennel.roomPos(k, 0.3, 0.55), key);
  await page.mouse.click(at.x, at.y);
  await expect.poll(() => page.evaluate((k) => (window.kennel.app.rooms.find((r) => r.key === k).style.props || []).map((p) => p.kind), key)).toEqual(["rug"]);
  // Resize it from its corner.
  const rug = await page.evaluate((k) => window.kennel.app.rooms.find((r) => r.key === k).style.props[0], key);
  const corner = await page.evaluate(([k, p]) => window.kennel.roomPos(k, p.x + p.w, p.y + p.h), [key, rug]);
  await page.mouse.move(corner.x, corner.y);
  await page.mouse.down();
  await page.mouse.move(corner.x + 60, corner.y + 30, { steps: 6 });
  await page.mouse.up();
  await expect.poll(() => page.evaluate((k) => window.kennel.app.rooms.find((r) => r.key === k).style.props[0].w, key)).toBeGreaterThan(rug.w);
  await page.keyboard.press("b");
  // File a pup and drop it on the rug: it goes under.
  const id = await pupId(page, "ONBOARDING");
  await post(page, `/api/pup/${id}/move`, { kennel: Number(key.slice(7)) });
  await page.waitForFunction(([k, i]) => window.kennel.app.rooms.find((r) => r.key === k).pups.includes(i) && window.kennel.app.ents.has(i), [key, id]);
  const from = await page.evaluate((i) => window.kennel.screenPos(i), id);
  const big = await page.evaluate((k) => window.kennel.app.rooms.find((r) => r.key === k).style.props[0], key);
  at = await page.evaluate(([k, p]) => window.kennel.roomPos(k, p.x + p.w / 2, p.y + p.h / 2), [key, big]);
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  await page.mouse.move(at.x, at.y, { steps: 8 });
  await page.mouse.up();
  await expect.poll(() => page.evaluate((i) => window.kennel.propModeOf(i), id)).toBe("under");
  // Clean up: back to New, no objects.
  await post(page, `/api/pup/${id}/move`, { kennel: 0 });
  await post(page, "/api/room/style", { key, style: {} });
});

test("pups talk: drag one onto another to pass its latest reply; they stay linked", async ({ page }) => {
  await open(page);
  const from = await pupId(page, "PG-UPGRADE");
  const to = await pupId(page, "RATE-LIMITS");
  await page.evaluate(() => { window.kennel.app.cam.z = 0.6; });
  await page.evaluate(([a, b]) => {
    const app = window.kennel.app, st = document.querySelector("#stage"), ea = app.ents.get(a), eb = app.ents.get(b);
    app.flyTo = null;
    app.cam.x = ((ea.x + eb.x) / 2) * 32 - st.clientWidth / app.cam.z / 2;
    app.cam.y = ((ea.y + eb.y) / 2) * 32 - st.clientHeight / app.cam.z / 2;
  }, [from, to]);
  const a = await page.evaluate((i) => window.kennel.screenPos(i), from);
  const b = await page.evaluate((i) => window.kennel.screenPos(i), to);
  page.once("dialog", (d) => { expect(d.message()).toContain("Pass PG-UPGRADE's latest reply to RATE-LIMITS"); d.accept("check the limits on the new cluster"); });
  await page.mouse.move(a.x, a.y);
  await page.mouse.down();
  for (let i = 1; i <= 10; i++) await page.mouse.move(a.x + ((b.x - a.x) * i) / 10, a.y + ((b.y - a.y) * i) / 10);
  await page.mouse.up();
  // RATE-LIMITS is asleep: it wakes up in the panel with the message as its first prompt.
  await expect(page.locator("#toast")).toContainText("woke up with what PG-UPGRADE found");
  await expect.poll(() => page.evaluate(() => window.kennel.termText())).toContain("check the limits on the new cluster");
  await expect.poll(() => page.evaluate(() => window.kennel.termText())).toContain('From the session "PG-UPGRADE"');
  expect(await page.evaluate(([x, y]) => window.kennel.app.state.handoffs.some((h) => h.from === x && h.to === y), [from, to])).toBe(true);
  await page.locator("#term button", { hasText: "Stop" }).click();
});
