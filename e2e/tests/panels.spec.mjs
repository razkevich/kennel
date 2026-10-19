import { test, expect } from "@playwright/test";

import { open, post, center } from "./helpers.mjs";

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

test("jobs and artifacts panels", async ({ page }) => {
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
});


test("sort new sessions into kennels: suggestions, adjust, file all, one undo", async ({ page }) => {
  await open(page);
  const newCount = async () => (await page.locator("#chatsortinto").isVisible()) ? Number((await page.locator("#chatsortinto").textContent()).match(/Sort (\d+)/)[1]) : 0;
  const before = await newCount();
  const kennelsBefore = await page.evaluate(() => window.kennel.app.state.kennels.length);
  await page.locator("#chatsortinto").click();
  const sheet = page.locator("#sort");
  await expect(sheet).toBeVisible();
  await expect(sheet.locator(".sortrow").first()).toBeVisible();
  const rows = await sheet.locator(".sortrow").count();
  expect(rows).toBe(before);
  // Suggestions reuse existing kennels and propose new ones.
  const values = await sheet.locator(".sortrow select").evaluateAll((l) => l.map((x) => x.value));
  expect(values.some((v) => v.startsWith("k:"))).toBe(true);
  expect(values.some((v) => v.startsWith("n:"))).toBe(true);
  // Keep one where it is.
  await sheet.locator(".sortrow select").first().selectOption("");
  await expect(sheet.locator(".apply")).toHaveText(`File ${rows - 1}`);
  await sheet.locator(".apply").click();
  await expect(sheet).toBeHidden();
  await expect(page.locator("#toast")).toContainText(`filed ${rows - 1} sessions`);
  await expect.poll(newCount).toBe(1);
  expect(await page.evaluate(() => window.kennel.app.state.kennels.length)).toBeGreaterThan(kennelsBefore);
  await page.keyboard.press("Meta+z");
  await expect.poll(newCount).toBe(before);
  await expect.poll(() => page.evaluate(() => window.kennel.app.state.kennels.length)).toBe(kennelsBefore);
});

test("kennels show the last two weeks; older filed sessions are one click away", async ({ page }) => {
  await open(page);
  const old = await page.evaluate(async () => {
    const lib = await (await fetch("/api/library", { headers: { "X-Kennel-Token": "e2e" } })).json();
    return lib.find((p) => Date.now() - p.updated > 15 * 86400e3 && p.status === "offline");
  });
  expect(old).toBeTruthy();
  const payments = await page.evaluate(() => window.kennel.app.rooms.find((r) => r.title === "Payments"));
  await post(page, `/api/pup/${old.id}/move`, { kennel: payments.kennel });
  await expect.poll(() => page.evaluate(() => window.kennel.app.rooms.find((r) => r.title === "Payments").older)).toBe(1);
  expect(await page.evaluate((i) => window.kennel.app.rooms.some((r) => r.pups.includes(i)), old.id)).toBe(false);

  await center(page, payments.key);
  const wall = await page.evaluate((k) => window.kennel.wallPos(k), payments.key);
  await page.mouse.click(wall.x, wall.y);
  await page.locator("#card button.older").click();
  await expect(page.locator("#library")).toBeVisible();
  await expect(page.locator("#libkennel")).toHaveValue("Payments");
  await expect(page.locator("#library .librow", { hasText: old.title })).toBeVisible();
  await page.keyboard.press("Escape");
  await post(page, `/api/pup/${old.id}/move`, { kennel: 0 });
});
