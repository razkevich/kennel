import { test, expect } from "@playwright/test";

const open = async (page) => {
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.state && window.kennel.app.ents.size > 0);
};
const bg = (page) => page.evaluate(() => getComputedStyle(document.body).backgroundColor);
// The map's top-left corner is hallway or stage: dark at night, light by day.
const mapIsDark = (page) => page.evaluate(() => {
  const c = document.querySelector("#map");
  const [r, g, b] = c.getContext("2d").getImageData(4, c.height - 4, 1, 1).data;
  return (r + g + b) / 3 < 90;
});

test.afterEach(async ({ request }) => {
  await request.post("/api/appearance", { headers: { "X-Kennel-Token": "e2e" }, data: { appearance: "" } });
});

test("appearance switch cycles Auto → Light → Dark, restyles page and map, and is shared", async ({ page, browser }) => {
  await open(page);
  const btn = page.locator("#themebtn");
  await expect(btn).toHaveText("🖥 Auto");
  expect(await mapIsDark(page)).toBe(false);

  await btn.click();
  await expect(btn).toHaveText("☀️ Light");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");

  await btn.click();
  await expect(btn).toHaveText("🌙 Dark");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  expect(await bg(page)).toBe("rgb(27, 30, 49)");
  await expect.poll(() => mapIsDark(page)).toBe(true);

  // A second page (the Mac app, another browser tab) picks it up live.
  const other = await browser.newPage();
  await open(other);
  await expect(other.locator("html")).toHaveAttribute("data-theme", "dark");
  await page.keyboard.press("Shift+T");
  await expect(btn).toHaveText("🖥 Auto");
  await expect.poll(() => other.evaluate(() => document.documentElement.dataset.theme || "")).toBe("");
  await other.close();
});

test("Auto follows the system setting", async ({ browser }) => {
  const ctx = await browser.newContext({ colorScheme: "dark" });
  const page = await ctx.newPage();
  await open(page);
  await expect(page.locator("#themebtn")).toHaveText("🖥 Auto");
  expect(await bg(page)).toBe("rgb(27, 30, 49)");
  await expect.poll(() => mapIsDark(page)).toBe(true);
  await page.emulateMedia({ colorScheme: "light" });
  await expect.poll(() => bg(page)).toBe("rgb(245, 244, 241)");
  await expect.poll(() => mapIsDark(page)).toBe(false);
  await ctx.close();
});
