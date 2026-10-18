import { test, expect } from "@playwright/test";

const open = async (page) => {
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.state && window.kennel.app.ents.size > 0);
};

const pupId = (page, title) =>
  page.evaluate((t) => Object.values(window.kennel.app.state.pups).find((p) => p.title === t)?.id, title);

const thingKey = (page, name) =>
  page.evaluate((n) => Object.values(window.kennel.app.state.things).find((t) => t.name === n)?.key, name);

// Click where an entity is drawn on the canvas, like a user would.
const clickEntity = async (page, id, opts = {}) => {
  await page.evaluate((i) => window.kennel.reveal(i), id);
  const pos = await page.evaluate((i) => window.kennel.screenPos(i), id);
  expect(pos, `entity ${id} on screen`).toBeTruthy();
  await page.mouse.click(pos.x, pos.y, opts);
};

test("shows the office with live counts and the pack", async ({ page }) => {
  await open(page);
  await expect(page.locator("#counts")).toContainText("1 needs you");
  await expect(page.locator("#counts")).toContainText("2 working");
  await expect(page.locator("#floors button.on")).toHaveText("🏡 Campus");
  await expect(page.locator("#pack")).toContainText("Needs you · 1");
  await expect(page.locator("#pack .member", { hasText: "FLAKY-CI" })).toBeVisible();
  await expect(page.locator("#pack")).toContainText("Things");
  const rooms = await page.evaluate(() => window.kennel.app.rooms.map((r) => r.title));
  expect(rooms).toEqual(expect.arrayContaining(["The Yard", "Lobby", "Workshop", "Firehouse", "Helping Out", "Dumpster"]));
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
  await page.locator("#side .tabs button", { hasText: "Ask" }).click();
  await page.locator("#askq").fill("why did the payment form flicker?");
  await page.locator("#askform button").click();
  await expect(page.locator("#answer")).toContainText("memoizing its options", { timeout: 15000 });
  await expect(page.locator("#sources .source").first()).toContainText("CHECKOUT-V2");
  await page.locator("#answer .cite", { hasText: "[1]" }).click();
  await expect(page.locator("#card")).toContainText("CHECKOUT-V2");
});

test("walking up to a pup and pressing X interacts", async ({ page }) => {
  await open(page);
  const id = await pupId(page, "FLAKY-CI");
  await page.evaluate((i) => {
    const e = window.kennel.app.ents.get(i);
    window.kennel.app.you.x = e.x;
    window.kennel.app.you.y = e.y + 0.8;
  }, id);
  await expect(page.locator("#hint")).toContainText("visit FLAKY-CI");
  await page.keyboard.press("x");
  await expect(page.locator("#toast")).toContainText(`claude --resume ${id}`);
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

test("floors switch between campus, projects and kennels", async ({ page }) => {
  await open(page);
  await page.locator("#floors button", { hasText: "Projects" }).click();
  await expect(page.locator("#floors button.on")).toHaveText("📁 Projects");
  await page.waitForFunction(() => window.kennel.app.state.floor === 1);
  const projects = await page.evaluate(() => window.kennel.app.rooms.map((r) => r.title));
  expect(projects).toEqual(expect.arrayContaining(["atlas", "infra", "mobile", "dotfiles"]));
  await page.locator("#floors button", { hasText: "Kennels" }).click();
  await page.waitForFunction(() => window.kennel.app.state.floor === 2);
  const kennels = await page.evaluate(() => window.kennel.app.rooms.map((r) => r.title));
  expect(kennels).toEqual(expect.arrayContaining(["Your first kennel", "Strays"]));
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
  await card.getByRole("button", { name: "↔ Convert" }).click();
  await expect(page.locator("#toast")).toContainText("claude -n");
  await card.getByRole("button", { name: "⬇ Export" }).click();
  await expect(page.locator("#toast")).toContainText("exported to");
});
