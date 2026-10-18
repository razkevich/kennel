import { test, expect } from "@playwright/test";
import { open, post, pupId, thingKey, clickEntity } from "./helpers.mjs";

test("shows the office with live counts and the session tray", async ({ page }) => {
  await open(page);
  await expect(page.locator("#counts")).toContainText("1 needs you");
  await expect(page.locator("#counts")).toContainText("2 working");
  await expect(page.locator("#floors button.on")).toHaveText("🏢 Main");
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

test("project filter dims other projects and narrows the tray", async ({ page }) => {
  await open(page);
  await page.locator("#projectsel").selectOption("/Users/demo/code/infra");
  await expect(page.locator("#projectsel")).toHaveClass(/on/);
  const shown = await page.locator("#pack .member").count();
  expect(shown).toBeGreaterThan(0);
  const projects = await page.evaluate(() => [...document.querySelectorAll("#pack .member")].map((m) => window.kennel.app.state.pups[m.dataset.id].project));
  expect(new Set(projects)).toEqual(new Set(["/Users/demo/code/infra"]));
  await page.locator("#projectsel").selectOption("");
  await expect.poll(() => page.locator("#pack .member").count()).toBeGreaterThan(shown);
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
  await expect(page.locator("#pack .member", { hasText: "★ CHECKOUT-V2" })).toBeVisible();
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
  page.once("dialog", (d) => d.accept("Side projects"));
  await page.locator("#floors button.addfloor").click();
  await expect(page.locator("#floors button.on")).toHaveText("🏢 Side projects");
  await expect.poll(() => page.evaluate(() => window.kennel.app.rooms.length)).toBe(0);
  // The floor is remembered across reloads.
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.state);
  await expect(page.locator("#floors button.on")).toHaveText("🏢 Side projects");

  // Back on Main, drag Payments by its wall onto the new floor's tab.
  await page.locator("#floors button[data-floor]").first().click();
  await page.waitForFunction(() => window.kennel.app.rooms.some((r) => r.title === "Payments"));
  const key = await page.evaluate(() => window.kennel.app.rooms.find((r) => r.title === "Payments").key);
  await page.evaluate((k) => {
    const app = window.kennel.app, st = document.querySelector("#stage"), r = app.rooms.find((x) => x.key === k);
    app.cam.x = (r.x + r.w / 2) * 32 - st.clientWidth / app.cam.z / 2;
    app.cam.y = (r.y + r.h / 2) * 32 - st.clientHeight / app.cam.z / 2;
  }, key);
  const wall = await page.evaluate((k) => window.kennel.wallPos(k), key);
  const tab = await page.locator("#floors button", { hasText: "Side projects" }).boundingBox();
  await page.mouse.move(wall.x, wall.y);
  await page.mouse.down();
  for (let i = 1; i <= 10; i++) await page.mouse.move(wall.x + ((tab.x + tab.width / 2 - wall.x) * i) / 10, wall.y + ((tab.y + tab.height / 2 - wall.y) * i) / 10);
  await expect(page.locator("#floors button.droptarget")).toHaveText("🏢 Side projects");
  await page.mouse.up();
  await expect(page.locator("#toast")).toContainText("moved Payments to Side projects");
  await expect.poll(() => page.evaluate(() => window.kennel.app.rooms.some((r) => r.title === "Payments"))).toBe(false);

  // Rename and remove from the tab's right-click menu; undo brings it back.
  page.once("dialog", (d) => d.accept("Garden"));
  await page.locator("#floors button", { hasText: "Side projects" }).click({ button: "right" });
  await page.locator("#ctxmenu button", { hasText: "Rename" }).click();
  await expect(page.locator("#floors button", { hasText: "Garden" })).toBeVisible();
  await page.locator("#floors button", { hasText: "Garden" }).click({ button: "right" });
  await page.locator("#ctxmenu button", { hasText: "Remove floor" }).click();
  await expect(page.locator("#floors button", { hasText: "Garden" })).toHaveCount(0);
  await expect.poll(() => page.evaluate(() => window.kennel.app.rooms.some((r) => r.title === "Payments"))).toBe(true);
  await page.keyboard.press("Meta+z");
  await expect(page.locator("#floors button", { hasText: "Garden" })).toBeVisible();
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
  await expect.poll(() => page.evaluate((k) => (window.kennel.app.rooms.find((r) => r.key === k).shelf || []).length, key)).toBe(1);
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

test("quick tell: one line to a pup without leaving the map", async ({ page }) => {
  await open(page);
  const id = await pupId(page, "RATE-LIMITS");
  await clickEntity(page, id);
  await page.keyboard.press("t");
  const tell = page.locator("#tell");
  await expect(tell).toBeVisible();
  await expect(tell).toContainText("Tell RATE-LIMITS");
  await expect(tell).toContainText("in the background");
  await tell.locator("input").fill("add per-tenant overrides to the limiter");
  await tell.locator("input").press("Enter");
  await expect(tell).toBeHidden();
  await expect(page.locator("#toast")).toContainText(`claude --resume ${id} -p`);
  await expect(page.locator("#toast")).toContainText("per-tenant overrides");
  // From the right-click menu, and Esc cancels.
  const pos = await page.evaluate((i) => window.kennel.screenPos(i), id);
  await page.mouse.click(pos.x, pos.y, { button: "right" });
  await page.locator("#ctxmenu button", { hasText: "Tell" }).click();
  await expect(tell).toBeVisible();
  await tell.locator("input").press("Escape");
  await expect(tell).toBeHidden();
  // A live pup kennel can't type into gets a way to go there instead of a dead input.
  const far = await page.evaluate(() => Object.values(window.kennel.app.state.pups).find((p) => p.status !== "offline" && !p.pane)?.id);
  expect(far).toBeTruthy();
  await page.evaluate((i) => { window.kennel.app.sel = i; }, far);
  await page.keyboard.press("t");
  await expect(tell.locator(".row")).toBeHidden();
  await expect(tell.locator("button.go")).toBeVisible();
  await expect(tell).toContainText("can't type into");
  await tell.locator("button.go").click();
  await expect(tell).toBeHidden();
});
