import { test, expect } from "@playwright/test";
import { open, post, center, room, dragTo } from "./helpers.mjs";

test("drag a pup to a new spot in its room; it stays there", async ({ page }) => {
  await open(page);
  const id = await page.evaluate(() => [...window.kennel.app.ents.values()].find((e) => e.kind === "pup" && e.room.w >= 8).id);
  const key = await page.evaluate((i) => window.kennel.app.ents.get(i).room.key, id);
  await center(page, key);
  const from = await page.evaluate((i) => window.kennel.screenPos(i), id);
  const to = await page.evaluate((k) => window.kennel.roomPos(k, 0.85, 0.85), key);
  await dragTo(page, from, to);
  await expect.poll(async () => ((await room(page, key)).style.spots || {})[id]).toBeTruthy();
  const spot = (await room(page, key)).style.spots[id];
  expect(spot.x).toBeGreaterThan(0.7);
  expect(spot.y).toBeGreaterThan(0.7);
  await page.reload();
  await page.waitForFunction((i) => window.kennel && window.kennel.app.ents.get(i), id);
  const home = await page.evaluate((i) => window.kennel.app.ents.get(i).home, id);
  const r = await room(page, key);
  expect(home.x).toBeGreaterThan(r.x + r.w * 0.6);
  // Right-click puts it back into the grid.
  await center(page, key);
  const at = await page.evaluate((i) => window.kennel.screenPos(i), id);
  await page.mouse.click(at.x, at.y, { button: "right" });
  await page.locator("#ctxmenu button", { hasText: "Back to its slot" }).click();
  await expect.poll(async () => ((await room(page, key)).style.spots || {})[id]).toBeFalsy();
});

test("select a room, move it by its wall, resize it by a handle", async ({ page }) => {
  await open(page);
  const L = await page.evaluate(() => window.kennel.app.rooms.find((r) => r.title === "Best in Show").key);
  const F = String(await page.evaluate(() => window.kennel.app.state.floor));
  await center(page, L);
  const before = await room(page, L);
  const wall = await page.evaluate((k) => window.kennel.wallPos(k), L);
  await page.mouse.click(wall.x, wall.y);
  await expect(page.locator("#card")).toContainText("Best in Show");
  // Move: drag the wall 3 tiles right and 2 down.
  const z = await page.evaluate(() => window.kennel.app.cam.z);
  await dragTo(page, wall, { x: wall.x + 3 * 32 * z, y: wall.y + 2 * 32 * z });
  await expect.poll(async () => ((await room(page, L)).style.pos || {})[F]).toEqual({ x: before.x + 3, y: before.y + 2 });
  // Resize from the south-east handle.
  await center(page, L);
  const moved = await room(page, L);
  const se = await page.evaluate(([x, y]) => window.kennel.worldPos(x, y), [moved.x + moved.w, moved.y + moved.h]);
  await dragTo(page, se, { x: se.x + 4 * 32 * z, y: se.y + 3 * 32 * z });
  await expect.poll(async () => (await room(page, L)).style.pos[F].w).toBe(moved.w + 4);
  await expect.poll(async () => (await room(page, L)).w).toBe(moved.w + 4);
  const sized = await room(page, L);
  expect(sized.w).toBe(moved.w + 4);
  expect(sized.h).toBe(moved.h + 3);
  // Undo puts the size back.
  await page.keyboard.press("Meta+z");
  await expect.poll(async () => ((await room(page, L)).style.pos || {})[F]).toEqual({ x: before.x + 3, y: before.y + 2 });
  await page.locator("#zoomfit").click();
  await page.mouse.click(10, 300);
  // Back to the automatic layout from the right-click menu.
  await center(page, L);
  const w2 = await page.evaluate((k) => window.kennel.wallPos(k), L);
  await page.mouse.click(w2.x, w2.y, { button: "right" });
  await page.locator("#ctxmenu button", { hasText: "Back to auto layout" }).click();
  await expect.poll(async () => ((await room(page, L)).style.pos || {})[F]).toBeFalsy();
});

test("kennel tool draws a new kennel; pups dragged onto it are filed; marquee moves many", async ({ page }) => {
  await open(page);
  page.on("dialog", (d) => d.accept("Drawn kennel"));
  await page.keyboard.press("r");
  await expect(page.locator('#tools [data-tool="room"]')).toHaveClass(/on/);
  const app = await page.evaluate(() => ({ w: window.kennel.app.world.w, h: window.kennel.app.world.h }));
  await page.evaluate(() => { const a = window.kennel.app; a.cam.x = 0; a.cam.y = 0; a.cam.z = 1; });
  // Draw it to the right of everything, then pan there.
  await page.evaluate(([x]) => { window.kennel.app.cam.x = (x - 10) * 32; }, [app.w]);
  const from = await page.evaluate(([x, y]) => window.kennel.worldPos(x, y), [app.w + 2, 4]);
  await dragTo(page, from, { x: from.x + 12 * 32, y: from.y + 8 * 32 });
  await expect(page.locator("#toast")).toContainText("built Drawn kennel");
  const k = await page.evaluate(() => window.kennel.app.rooms.find((r) => r.title === "Drawn kennel"));
  expect(k.w).toBe(12);
  expect(k.h).toBe(8);
  await expect(page.locator('#tools [data-tool="select"]')).toHaveClass(/on/);

  // File two pups from another kennel with a marquee drag + group drag.
  const other = await post(page, "/api/kennels", { Name: "Marquee source", Icon: "🧺" });
  const ids = await page.evaluate(() => window.kennel.app.state.tray.filter((id) => !window.kennel.app.state.pups[id].kennel).slice(-2));
  expect(ids.length).toBe(2);
  for (const id of ids) await post(page, `/api/pup/${id}/move`, { kennel: other.ID });
  await page.reload();
  await page.waitForFunction((key) => window.kennel && window.kennel.app.rooms.some((r) => r.key === key && r.pups.length === 2), "kennel:" + other.ID);
  await center(page, "kennel:" + other.ID);
  const src = await room(page, "kennel:" + other.ID);
  const tl = await page.evaluate(([x, y]) => window.kennel.worldPos(x, y), [src.x + 0.3, src.y + 2.2]);
  const br = await page.evaluate(([x, y]) => window.kennel.worldPos(x, y), [src.x + src.w - 0.3, src.y + src.h - 0.2]);
  await dragTo(page, tl, br);
  await expect.poll(() => page.evaluate(() => window.kennel.app.multi.size)).toBe(2);
  // Drag the group onto the drawn kennel (pan so both are on screen first).
  await page.evaluate(() => { const a = window.kennel.app; a.cam.z = 0.5; });
  await page.evaluate(([s, d]) => {
    const a = window.kennel.app, st = document.querySelector("#stage");
    a.cam.x = ((s.x + d.x + d.w) / 2) * 32 - st.clientWidth / a.cam.z / 2;
    a.cam.y = ((Math.min(s.y, d.y) + Math.max(s.y + s.h, d.y + d.h)) / 2) * 32 - st.clientHeight / a.cam.z / 2;
  }, [src, k]);
  const pupAt = await page.evaluate((i) => window.kennel.screenPos(i), ids[0]);
  const dest = await page.evaluate(() => window.kennel.roomPos(window.kennel.app.rooms.find((r) => r.title === "Drawn kennel").key, 0.5, 0.5));
  await dragTo(page, pupAt, dest);
  await expect(page.locator("#toast")).toContainText("2 pups into");
  await expect.poll(() => page.evaluate((l) => l.map((i) => window.kennel.app.state.pups[i].kennel), ids)).toEqual(["Drawn kennel", "Drawn kennel"]);
});

test("toolbar, zoom controls, space-pan and context menu on empty floor", async ({ page }) => {
  await open(page);
  await page.locator("#zoomin").click();
  await expect(page.locator("#zoomval")).toHaveText(/\d+%/);
  const z1 = await page.evaluate(() => window.kennel.app.cam.z);
  await page.locator("#zoomout").click();
  expect(await page.evaluate(() => window.kennel.app.cam.z)).toBeLessThan(z1);
  await page.locator("#zoomval").click();
  await expect(page.locator("#zoomval")).toHaveText("100%");
  await page.locator('#tools [data-tool="hand"]').click();
  const cam = await page.evaluate(() => ({ ...window.kennel.app.cam }));
  const box = await page.locator("#map").boundingBox();
  const p = await page.evaluate(() => { const r = window.kennel.app.rooms[0]; return window.kennel.roomPos(r.key, 0.5, 0.5); });
  await dragTo(page, p, { x: p.x - 120, y: p.y - 60 });
  const cam2 = await page.evaluate(() => ({ ...window.kennel.app.cam }));
  expect(cam2.x).toBeGreaterThan(cam.x + 50);
  await page.keyboard.press("v");
  // Right-click empty hallway offers building a kennel.
  await page.evaluate(() => { const a = window.kennel.app; a.cam.x = -400; a.cam.y = -300; });
  await page.mouse.click(box.x + 320, box.y + 120, { button: "right" });
  await expect(page.locator("#ctxmenu")).toBeVisible();
  await expect(page.locator("#ctxmenu")).toContainText("New kennel here");
  await page.keyboard.press("Escape");
  await expect(page.locator("#ctxmenu")).toBeHidden();
});

test("remove pups from a kennel: right-click, Delete, drag out, card button; undo puts them back", async ({ page }) => {
  await open(page);
  const k = await post(page, "/api/kennels", { Name: "Removal yard", Icon: "🧺" });
  const key = "kennel:" + k.ID;
  const ids = await page.evaluate(() => window.kennel.app.state.tray.filter((id) => !window.kennel.app.state.pups[id].kennel).slice(-4));
  expect(ids.length).toBe(4);
  for (const id of ids) await post(page, `/api/pup/${id}/move`, { kennel: k.ID });
  await page.reload();
  await page.waitForFunction((kk) => window.kennel && window.kennel.app.rooms.some((r) => r.key === kk && r.pups.length === 4), key);
  const members = () => page.evaluate((kk) => window.kennel.app.rooms.find((r) => r.key === kk)?.pups.length ?? 0, key);
  const at = async (id) => { await center(page, key); return page.evaluate((i) => window.kennel.screenPos(i), id); };

  // Right-click → Remove from …
  let p = await at(ids[0]);
  await page.mouse.click(p.x, p.y, { button: "right" });
  await page.locator("#ctxmenu button", { hasText: "Remove from Removal yard" }).click();
  await expect(page.locator("#toast")).toContainText("removed");
  await expect.poll(members).toBe(3);
  await page.keyboard.press("Meta+z");
  await expect.poll(members).toBe(4);

  // Select + Delete.
  p = await at(ids[1]);
  await page.mouse.click(p.x, p.y);
  await page.keyboard.press("Delete");
  await expect.poll(members).toBe(3);
  expect(await page.evaluate((i) => window.kennel.app.state.pups[i]?.archived ?? false, ids[1])).toBe(false);

  // Drag out onto the empty floor.
  p = await at(ids[2]);
  const r = await page.evaluate((kk) => window.kennel.app.rooms.find((x) => x.key === kk), key);
  const outside = await page.evaluate(([x, y]) => window.kennel.worldPos(x, y), [r.x + r.w + 1.2, r.y + r.h / 2]);
  await dragTo(page, p, outside);
  await expect.poll(members).toBe(2);

  // The card's button.
  p = await at(ids[3]);
  await page.mouse.click(p.x, p.y);
  await page.locator("#card button.dots").click();
  await page.locator("#ctxmenu button", { hasText: "Remove from Removal yard" }).click();
  await expect.poll(members).toBe(1);
  await expect(page.locator("#pack .group", { hasText: "New" }).locator(`.member[data-id="${ids[3]}"]`)).toHaveCount(1);
});
