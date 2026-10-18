import { test } from "@playwright/test";

// Regenerates README images: SHOTS=1 npx playwright test screenshots
test.skip(!process.env.SHOTS, "set SHOTS=1 to refresh README screenshots");

const out = (name) => `../docs/img/${name}.png`;

test("web screenshots", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 860 });
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.ents.size > 0);
  await page.waitForTimeout(1200);
  await page.screenshot({ path: out("web-map") });

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

  await page.locator("#side .tabs button", { hasText: "Ask" }).click();
  await page.locator("#askq").fill("what did we decide about the postgres upgrade?");
  await page.locator("#askform button").click();
  await page.locator("#sources .source").first().waitFor();
  await page.waitForTimeout(400);
  await page.screenshot({ path: out("web-ask") });
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
