import { test, expect } from "@playwright/test";

const open = async (page) => {
  await page.goto("/?t=e2e");
  await page.waitForFunction(() => window.kennel && window.kennel.app.state && window.kennel.app.ents.size > 0);
};

const room = (page, key) => page.evaluate((k) => window.kennel.app.rooms.find((r) => r.key === k), key);

const reveal = async (page, key) => {
  await page.evaluate((k) => {
    const r = window.kennel.app.rooms.find((x) => x.key === k);
    const st = document.querySelector("#stage");
    const app = window.kennel.app;
    app.cam.x = (r.x + r.w / 2) * 32 - st.clientWidth / app.cam.z / 2;
    app.cam.y = (r.y + r.h / 2) * 32 - st.clientHeight / app.cam.z / 2;
  }, key);
};

test("build mode: template, floor, furniture, blocks, moving rooms", async ({ page }) => {
  await open(page);
  await page.keyboard.press("b");
  const panel = page.locator("#build");
  await expect(panel).toBeVisible();
  await expect(panel).toContainText("Click any room to shape it");

  // Pick the Lobby by clicking its floor.
  await reveal(page, "lobby");
  let p = await page.evaluate(() => window.kennel.roomPos("lobby", 0.9, 0.9));
  await page.mouse.click(p.x, p.y);
  await expect(panel.locator(".roomname")).toContainText("Lobby");

  await panel.getByRole("button", { name: "🚨 On-call room" }).click();
  await expect.poll(async () => (await room(page, "lobby")).style.shape).toBe("office");
  let st = (await room(page, "lobby")).style;
  expect(st.blocks.map((b) => b.kind)).toEqual(expect.arrayContaining(["door", "bell"]));
  expect(st.decor.length).toBe(3);

  await panel.getByRole("button", { name: "Water" }).click();
  await expect.poll(async () => (await room(page, "lobby")).style.floor).toBe("water");

  // Carry a plant and drop it in the room.
  await panel.locator("button.decor[title='Plant']").click();
  await reveal(page, "lobby");
  p = await page.evaluate(() => window.kennel.roomPos("lobby", 0.1, 0.2));
  await page.mouse.click(p.x, p.y);
  await expect.poll(async () => (await room(page, "lobby")).style.decor.length).toBe(4);
  await page.keyboard.press("Escape");

  // Configure the door.
  const doorName = panel.locator(".blockcard", { hasText: "Spawn door" }).locator("input[data-field='name']");
  await doorName.fill("PAGER");
  await doorName.press("Enter");
  await expect.poll(async () => (await room(page, "lobby")).style.blocks.find((b) => b.kind === "door").config.name).toBe("PAGER");

  // Drag the room by its wall to a new spot.
  await reveal(page, "lobby");
  const before = await room(page, "lobby");
  const wall = await page.evaluate(() => window.kennel.wallPos("lobby"));
  await page.mouse.move(wall.x, wall.y);
  await page.mouse.down();
  await page.mouse.move(wall.x + 120, wall.y + 140, { steps: 8 });
  await page.mouse.up();
  await expect.poll(async () => JSON.stringify((await room(page, "lobby")).style.pos || {})).not.toBe("{}");
  const after = await room(page, "lobby");
  expect(after.x !== before.x || after.y !== before.y).toBeTruthy();

  // Right-click removes furniture.
  await reveal(page, "lobby");
  const plant = await page.evaluate(() => {
    const r = window.kennel.app.rooms.find((x) => x.key === "lobby");
    const d = r.style.decor[r.style.decor.length - 1];
    return window.kennel.roomPos("lobby", d.x, d.y - 0.05);
  });
  await page.mouse.click(plant.x, plant.y, { button: "right" });
  await expect.poll(async () => (await room(page, "lobby")).style.decor.length).toBe(3);

  // Leave build mode and use the door: it spawns a pup with the preset name.
  await page.keyboard.press("b");
  await expect(panel).toBeHidden();
  await reveal(page, "lobby");
  const door = await page.evaluate(() => {
    const r = window.kennel.app.rooms.find((x) => x.key === "lobby");
    const d = r.style.blocks.find((b) => b.kind === "door");
    return window.kennel.roomPos("lobby", d.x, d.y - 0.05);
  });
  page.once("dialog", (d) => d.accept());
  await page.mouse.click(door.x, door.y);
  await expect(page.locator("#toast")).toContainText("claude -n PAGER");
});
