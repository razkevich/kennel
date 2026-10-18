import { test, expect } from "@playwright/test";
import { open, room, roomKey, center as reveal, post } from "./helpers.mjs";

test("build mode: template, floor, furniture, blocks, moving rooms", async ({ page }) => {
  await open(page);
  await page.keyboard.press("b");
  const panel = page.locator("#build");
  await expect(panel).toBeVisible();
  await expect(panel).toContainText("click a room on the map to shape it");
  const L = await roomKey(page, "Best in Show");

  // Pick the room by clicking its floor.
  await reveal(page, L);
  let p = await page.evaluate((k) => window.kennel.roomPos(k, 0.9, 0.9), L);
  await page.mouse.click(p.x, p.y);
  await expect(panel.locator(".roomname")).toContainText("Best in Show");

  await panel.getByRole("button", { name: "🚨 On-call room" }).click();
  await expect.poll(async () => (await room(page, L)).style.shape).toBe("office");
  let st = (await room(page, L)).style;
  expect(st.blocks.map((b) => b.kind)).toEqual(expect.arrayContaining(["door", "bell"]));
  expect(st.decor.length).toBe(3);

  await panel.getByRole("button", { name: "Water" }).click();
  await expect.poll(async () => (await room(page, L)).style.floor).toBe("water");

  // Carry a plant and drop it in the room.
  await panel.locator("button.decor[title='Plant']").click();
  await reveal(page, L);
  p = await page.evaluate((k) => window.kennel.roomPos(k, 0.1, 0.2), L);
  await page.mouse.click(p.x, p.y);
  await expect.poll(async () => (await room(page, L)).style.decor.length).toBe(4);
  await page.keyboard.press("Escape");

  // Configure the door.
  const doorName = panel.locator(".blockcard", { hasText: "Spawn door" }).locator("input[data-field='name']");
  await doorName.fill("PAGER");
  await doorName.press("Enter");
  await expect.poll(async () => (await room(page, L)).style.blocks.find((b) => b.kind === "door").config.name).toBe("PAGER");
  const doorCwd = panel.locator(".blockcard", { hasText: "Spawn door" }).locator("input[data-field='cwd']");
  await doorCwd.fill("/Users/demo/code/infra");
  await doorCwd.press("Enter");
  await expect.poll(async () => (await room(page, L)).style.blocks.find((b) => b.kind === "door").config.cwd).toBe("/Users/demo/code/infra");

  // Drag the room by its wall to a new spot.
  await reveal(page, L);
  const before = await room(page, L);
  const wall = await page.evaluate((k) => window.kennel.wallPos(k), L);
  await page.mouse.move(wall.x, wall.y);
  await page.mouse.down();
  await page.mouse.move(wall.x + 120, wall.y + 140, { steps: 8 });
  await page.mouse.up();
  await expect.poll(async () => JSON.stringify((await room(page, L)).style.pos || {})).not.toBe("{}");
  const after = await room(page, L);
  expect(after.x !== before.x || after.y !== before.y).toBeTruthy();

  // Right-click removes furniture.
  await reveal(page, L);
  const plant = await page.evaluate((k) => {
    const r = window.kennel.app.rooms.find((x) => x.key === k);
    const d = r.style.decor[r.style.decor.length - 1];
    return window.kennel.roomPos(k, d.x, d.y - 0.05);
  }, L);
  await page.mouse.click(plant.x, plant.y, { button: "right" });
  await expect.poll(async () => (await room(page, L)).style.decor.length).toBe(3);

  // Leave build mode and use the door: it spawns a pup with the preset name.
  await page.keyboard.press("b");
  await expect(panel).toBeHidden();
  await reveal(page, L);
  const door = await page.evaluate((k) => {
    const r = window.kennel.app.rooms.find((x) => x.key === k);
    const d = r.style.blocks.find((b) => b.kind === "door");
    return window.kennel.roomPos(k, d.x, d.y - 0.05);
  }, L);
  page.once("dialog", (d) => d.accept());
  await page.mouse.click(door.x, door.y);
  await expect(page.locator("#toast")).toContainText("claude -n PAGER");
});

test("clicking a pup in build mode selects the pup, and the panel can move it to a kennel", async ({ page }) => {
  await open(page);
  await page.keyboard.press("b");
  const id = await page.evaluate(() => Object.values(window.kennel.app.state.pups).find((p) => p.title === "CHECKOUT-V2").id);
  await page.evaluate((i) => window.kennel.reveal(i), id);
  const pos = await page.evaluate((i) => window.kennel.screenPos(i), id);
  await page.mouse.click(pos.x, pos.y);
  const card = page.locator("#card");
  await expect(card).toBeVisible();
  await expect(card).toContainText("CHECKOUT-V2");
  await expect(card).toContainText("working");
  await expect(card.getByRole("button", { name: "📜 Read" })).toBeVisible();
  await card.locator(".move select").selectOption({ label: "💳 Payments" });
  await expect.poll(() => page.evaluate((i) => window.kennel.app.state.pups[i] && window.kennel.app.state.pups[i].kennel, id)).toBe("Payments");
  await page.keyboard.press("Meta+z");
  await expect.poll(() => page.evaluate((i) => window.kennel.app.state.pups[i] && window.kennel.app.state.pups[i].kennel, id)).toBe("");
});

test("room card: start a new pup in a kennel and add existing pups from search", async ({ page }) => {
  await open(page);
  const k = await page.evaluate(async () => (await fetch("/api/kennels", { method: "POST", headers: { "X-Kennel-Token": "e2e" }, body: JSON.stringify({ Name: "Night shift", Icon: "🌙", Cwd: "/Users/demo/code/infra" }) })).json());
  const key = "kennel:" + k.ID;
  await page.waitForFunction((kk) => window.kennel.app.rooms.some((r) => r.key === kk), key);
  await page.evaluate((kk) => {
    const r = window.kennel.app.rooms.find((x) => x.key === kk);
    const app = window.kennel.app, st = document.querySelector("#stage");
    app.cam.x = (r.x + r.w / 2) * 32 - st.clientWidth / app.cam.z / 2;
    app.cam.y = (r.y + r.h / 2) * 32 - st.clientHeight / app.cam.z / 2;
  }, key);
  const spot = await page.evaluate((kk) => window.kennel.roomPos(kk, 0.9, 0.9), key);
  await page.mouse.click(spot.x, spot.y);
  const card = page.locator("#card");
  await expect(card).toContainText("Night shift");

  const answers = ["PAGER", "check the overnight alerts"];
  page.on("dialog", (d) => d.accept(answers.shift()));
  await card.getByRole("button", { name: "🐣 New pup here" }).click();
  await expect(page.locator("#toast")).toContainText("claude --session-id");
  await expect(page.locator("#toast")).toContainText("-n PAGER");

  await card.locator(".addpups input").fill("rate-limits");
  const row = card.locator(".addrow", { hasText: "RATE-LIMITS" });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "＋" }).click();
  await expect.poll(() => page.evaluate(() => Object.values(window.kennel.app.state.pups).some((p) => p.title === "RATE-LIMITS" && p.kennel === "Night shift"))).toBeTruthy();
});

test("builder with no room picked: add a smart room, drag a block onto a room, click-to-place", async ({ page }) => {
  await open(page);
  await page.keyboard.press("b");
  const panel = page.locator("#build");
  await panel.locator(".smartgrid .smart", { hasText: "Under the Rug" }).click();
  await expect(page.locator("#toast")).toContainText("added Under the Rug");
  await page.waitForFunction(() => window.kennel.app.rooms.some((r) => r.smart === "rug"));
  const rug = await page.evaluate(() => window.kennel.app.rooms.find((r) => r.smart === "rug").key);

  // Drag the bell chip from the panel onto the new room.
  await page.keyboard.press("Escape");
  await page.evaluate(() => { window.kennel.build.room = null; });
  await page.keyboard.press("b"); await page.keyboard.press("b");
  await reveal(page, rug);
  const at = await page.evaluate((k) => window.kennel.roomPos(k, 0.5, 0.5), rug);
  const box = await page.locator("#map").boundingBox();
  await panel.locator(".chips button", { hasText: "Bell" }).dragTo(page.locator("#map"), { targetPosition: { x: at.x - box.x, y: at.y - box.y } });
  await expect.poll(async () => ((await room(page, rug)).style.blocks || []).map((b) => b.kind)).toEqual(["bell"]);

  // Or click a block, then click a room.
  await panel.locator(".chips button", { hasText: "Portal" }).click();
  await page.mouse.click(at.x - 40, at.y);
  await expect.poll(async () => ((await room(page, rug)).style.blocks || []).map((b) => b.kind)).toEqual(["bell", "portal"]);

  // Smart rooms don't take filed pups, and can be deleted from their card.
  await page.keyboard.press("b");
  const wall = await page.evaluate((k) => window.kennel.wallPos(k), rug);
  await page.mouse.click(wall.x, wall.y);
  await expect(page.locator("#card")).toContainText("Fills itself");
  await page.locator("#card button", { hasText: "Delete" }).click();
  await expect.poll(() => page.evaluate(() => window.kennel.app.rooms.some((r) => r.smart === "rug"))).toBe(false);
});
