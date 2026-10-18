import { test } from "@playwright/test";

// Regenerates README images: SHOTS=1 npx playwright test screenshots
test.skip(!process.env.SHOTS, "set SHOTS=1 to refresh README screenshots");

const out = (name) => `../docs/img/${name}.png`;

test("web screenshots", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 860 });
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.ents.size > 0);
  for (const [name, value] of [["runbook", "~/docs/oncall-runbook.md"], ["", "#qa-alerts"], ["", "INF-2141"], ["design doc", "https://docs.acme.dev/checkout-v2"]]) {
    await page.evaluate(([n, v]) => fetch("/api/treats", { method: "POST", headers: { "X-Kennel-Token": "e2e" }, body: JSON.stringify({ name: n, value: v }) }), [name, value]);
  }
  await page.reload();
  await page.waitForFunction(() => window.kennel && window.kennel.app.ents.size > 0);
  await page.waitForTimeout(1200);
  await page.screenshot({ path: out("web-map") });

  await page.locator("#floors button", { hasText: "Projects" }).click();
  await page.waitForFunction(() => window.kennel.app.state.floor === 1);
  await page.waitForTimeout(1200);
  await page.screenshot({ path: out("web-projects") });
  await page.locator("#floors button", { hasText: "Campus" }).click();
  await page.waitForFunction(() => window.kennel.app.state.floor === 0);
  await page.waitForTimeout(800);

  const id = await page.evaluate(() => Object.values(window.kennel.app.state.pups).find((p) => p.title === "FLAKY-CI").id);
  const pos = await page.evaluate((i) => window.kennel.screenPos(i), id);
  await page.mouse.click(pos.x, pos.y);
  await page.waitForTimeout(600);
  await page.screenshot({ path: out("web-card") });

  await page.keyboard.press("Escape");
  await page.locator("#q").click();
  await page.keyboard.type("postgres");
  await page.waitForTimeout(700);
  await page.screenshot({ path: out("web-search") });
  await page.keyboard.press("Escape");

  await page.locator("#side .tabs button[data-tab=ask]").click();
  await page.locator("#askq").fill("what did we decide about the postgres upgrade?");
  await page.locator("#askform button").click();
  await page.locator("#sources .source").first().waitFor();
  await page.waitForTimeout(400);
  await page.screenshot({ path: out("web-ask") });
});

test("editing screenshot", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 860 });
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.ents.size > 0);
  await page.waitForTimeout(800);
  const wall = await page.evaluate(() => window.kennel.wallPos("lobby"));
  await page.mouse.click(wall.x, wall.y);
  const r = await page.evaluate(() => window.kennel.app.rooms.find((x) => x.key === "lobby"));
  const se = await page.evaluate(([x, y]) => window.kennel.worldPos(x, y), [r.x + r.w, r.y + r.h]);
  await page.mouse.move(se.x, se.y);
  await page.mouse.down();
  for (let i = 1; i <= 6; i++) await page.mouse.move(se.x + i * 14, se.y + i * 9);
  await page.waitForTimeout(300);
  await page.screenshot({ path: out("web-edit") });
  await page.mouse.up();
  await page.keyboard.press("Meta+z");
});

test("build screenshot", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 860 });
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.ents.size > 0);
  const tok = { "X-Kennel-Token": "e2e", "Content-Type": "application/json" };
  const post = (path, body) => page.evaluate(([p, b, h]) => fetch(p, { method: "POST", headers: h, body: JSON.stringify(b) }), [path, body, tok]);
  await post("/api/room/template", { key: "fire", template: "oncall" });
  await post("/api/room/template", { key: "helping", template: "corner" });
  await post("/api/room/template", { key: "lobby", template: "lounge" });
  await page.waitForTimeout(1500);
  await page.keyboard.press("b");
  const p = await page.evaluate(() => window.kennel.roomPos("lobby", 0.95, 0.95));
  await page.mouse.click(p.x, p.y);
  await page.waitForTimeout(800);
  await page.screenshot({ path: out("web-build") });
});

test("library and panels screenshots", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 860 });
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.state && window.kennel.app.ents.size > 0);
  await page.evaluate(() => fetch("/api/themes/run", { method: "POST", headers: { "X-Kennel-Token": "e2e" }, body: "{}" }));
  await page.waitForFunction(() => window.kennel.app.state.themes && !window.kennel.app.state.themes.running && window.kennel.app.state.themes.done > 0);
  await page.waitForTimeout(800);
  await page.screenshot({ path: out("web-inbox") });
  await page.keyboard.press("l");
  await page.locator("#libwhen").selectOption("0");
  await page.locator("#libgroup").selectOption("theme");
  await page.waitForTimeout(500);
  await page.screenshot({ path: out("web-library") });
  await page.keyboard.press("Escape");
  await page.locator("#side .tabs button[data-tab=gear]").click();
  await page.waitForTimeout(500);
  await page.screenshot({ path: out("web-gear") });
  await page.locator("#side .tabs button[data-tab=jobs]").click();
  await page.waitForTimeout(500);
  await page.screenshot({ path: out("web-jobs") });
});
