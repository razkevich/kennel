import { test, expect } from "@playwright/test";

const open = async (page) => {
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.state);
  if (await page.evaluate(() => window.kennel.app.state.floor) !== 0) {
    await page.locator("#floors button", { hasText: "Campus" }).click();
  }
  await page.waitForFunction(() => window.kennel.app.state.floor === 0 && window.kennel.app.ents.size > 0);
};
const post = (page, path, body) => page.evaluate(([p, b]) => fetch(p, { method: "POST", headers: { "X-Kennel-Token": "e2e" }, body: JSON.stringify(b) }).then((r) => r.json()), [path, body]);

test("inbox lists unfiled pups; dragging one onto a kennel files it", async ({ page }) => {
  await open(page);
  const k = await post(page, "/api/kennels", { Name: "Inbox target", Icon: "📦" });
  const chips = page.locator("#inboxlist .inpup");
  await expect(chips.first()).toBeVisible();
  const first = await chips.first().getAttribute("data-id");
  expect(first).toBeTruthy();

  await page.locator("#floors button", { hasText: "Kennels" }).click();
  await page.waitForFunction((key) => window.kennel.app.state.floor === 2 && window.kennel.app.rooms.some((r) => r.key === key), "kennel:" + k.ID);
  const spot = await page.evaluate((key) => {
    const app = window.kennel.app, st = document.querySelector("#stage"), r = app.rooms.find((x) => x.key === key);
    app.cam.x = (r.x + r.w / 2) * 32 - st.clientWidth / app.cam.z / 2;
    app.cam.y = (r.y + r.h / 2) * 32 - st.clientHeight / app.cam.z / 2;
    return window.kennel.roomPos(key, 0.5, 0.6);
  }, "kennel:" + k.ID);
  await page.evaluate(({ x, y, id }) => {
    const dt = new DataTransfer();
    dt.setData("application/x-kennel-pup", id);
    const c = document.querySelector("#map");
    for (const type of ["dragover", "drop"]) c.dispatchEvent(new DragEvent(type, { clientX: x, clientY: y, dataTransfer: dt, bubbles: true, cancelable: true }));
  }, { x: spot.x, y: spot.y, id: first });
  await expect(page.locator("#toast")).toContainText("filed into 📦 Inbox target");
  await expect(page.locator(`#inboxlist .inpup[data-id="${first}"]`)).toHaveCount(0);
  await expect.poll(() => page.evaluate((id) => window.kennel.app.state.pups[id] && window.kennel.app.state.pups[id].kennel, first)).toBe("Inbox target");
  await page.locator("#floors button", { hasText: "Campus" }).click();
});

test("library: filter, group, sort, bulk move and themes", async ({ page }) => {
  await open(page);
  const k = await post(page, "/api/kennels", { Name: "Library shelf", Icon: "📚" });
  await page.reload();
  await open(page);
  await page.keyboard.press("l");
  const libr = page.locator("#library");
  await expect(libr).toBeVisible();
  await page.locator("#libwhen").selectOption("0");
  await expect(libr.locator(".librow").first()).toBeVisible();
  const all = await libr.locator(".librow").count();
  expect(all).toBeGreaterThan(15);

  await page.locator("#libagent").selectOption("codex");
  await expect(libr.locator(".librow")).toHaveCount(2);
  await page.locator("#libagent").selectOption("");
  await page.locator("#libq").fill("pg-upgrade");
  await expect(libr.locator(".librow")).toHaveCount(1);
  await page.locator("#libq").fill("");

  await page.locator("#libgroup").selectOption("repo");
  await expect(libr.locator(".libgroup h4", { hasText: "atlas" })).toBeVisible();
  await expect(libr.locator(".libgroup h4", { hasText: "infra" })).toBeVisible();
  await page.locator("#libsort").selectOption("msgs");

  // select two and move them
  const rows = libr.locator(".librow");
  await rows.nth(0).locator("input[type=checkbox]").check();
  await rows.nth(1).locator("input[type=checkbox]").check();
  await expect(page.locator("#libselcount")).toHaveText("2 selected");
  await page.locator("#libmove").selectOption(String(k.ID));
  await page.locator("#libdo").click();
  await expect(page.locator("#toast")).toContainText("moved 2 pups");
  await page.locator("#libkennel").selectOption("Library shelf");
  await expect(libr.locator(".librow")).toHaveCount(2);
  await page.locator("#libkennel").selectOption("");

  // themes via the (fake) LLM
  page.once("dialog", (d) => d.dismiss());
  await page.locator("#libthemes").click();
  await expect.poll(() => page.evaluate(() => window.kennel.app.state.themes.running === false && window.kennel.app.state.themes.done > 0), { timeout: 15000 }).toBeTruthy();
  await page.locator("#libgroup").selectOption("theme");
  await expect(libr.locator(".libgroup h4", { hasText: "Payments" })).toBeVisible();
  await expect(libr.locator(".pill.theme").first()).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(libr).toBeHidden();
});

test("jobs, artifacts and gear panels; gear toggles undo", async ({ page }) => {
  await open(page);
  await page.locator("#side .tabs button[data-tab=jobs]").click();
  const jobs = page.locator("#jobs");
  await expect(jobs).toContainText("Things");
  await expect(jobs).toContainText("atlas dev");
  await expect(jobs).toContainText("Background agents");
  await expect(jobs).toContainText("14 of 22 services done");
  await expect(jobs).toContainText("*/30 * * * *");

  await page.locator("#side .tabs button[data-tab=art]").click();
  await expect(page.locator("#art")).toContainText("PG16 cutover runbook");

  await page.locator("#side .tabs button[data-tab=gear]").click();
  const gear = page.locator("#gear");
  await expect(gear).toContainText("linear");
  await expect(gear).toContainText("deploy-checklist");
  const row = gear.locator(".gitem", { hasText: "weekly-report" });
  await expect(row.locator(".switch")).toHaveClass(/on/);
  await row.locator(".switch").click();
  await expect(page.locator("#toast")).toContainText("disabled weekly-report");
  await expect(gear.locator(".gitem", { hasText: "weekly-report" }).locator(".switch")).not.toHaveClass(/on/);
  await page.locator("#toast button.undo").click();
  await expect(page.locator("#toast")).toContainText("undid");
  await page.locator("#side .tabs button[data-tab=gear]").click();
  await expect(gear.locator(".gitem", { hasText: "weekly-report" }).locator(".switch")).toHaveClass(/on/);
});

test("digest modal", async ({ page }) => {
  await open(page);
  await page.locator("#digestbtn").click();
  await expect(page.locator("#digestbody")).toContainText("pups touched");
  await page.locator("#digestdays").selectOption("7");
  await expect(page.locator("#digestbody")).toContainText("busiest");
  await page.keyboard.press("Escape");
  await expect(page.locator("#digest")).toBeHidden();
});
