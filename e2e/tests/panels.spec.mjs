import { test, expect } from "@playwright/test";

import { open, post } from "./helpers.mjs";

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

