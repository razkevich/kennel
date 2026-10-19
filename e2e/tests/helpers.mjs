import { expect } from "@playwright/test";

// Every test starts on the first floor; the server remembers the last floor between tests.
export const open = async (page) => {
  await page.goto("/?t=e2e");
  await post(page, "/api/prefs", { key: "tray", value: "null" });
  await page.reload();
  // A cold server indexes in the background; rooms reflow until it's done.
  await page.waitForFunction(() => window.kennel && window.kennel.app.state && !window.kennel.app.state.indexing, null, { timeout: 20000 });
  const first = await page.evaluate(() => window.kennel.app.state.floors[0].id);
  if (await page.evaluate(() => window.kennel.app.state.floor) !== first) {
    await page.locator("#floors .floor[data-floor]").first().click();
  }
  await page.waitForFunction((f) => window.kennel.app.state.floor === f && window.kennel.app.ents.size > 0, first);
};

export const post = (page, path, body) => page.evaluate(([p, b]) =>
  fetch(p, { method: "POST", headers: { "X-Kennel-Token": "e2e" }, body: JSON.stringify(b) }).then((r) => r.json()), [path, body]);

export const pupId = (page, title) =>
  page.evaluate((t) => Object.values(window.kennel.app.state.pups).find((p) => p.title === t)?.id, title);

export const thingKey = (page, name) =>
  page.evaluate((n) => Object.values(window.kennel.app.state.things).find((t) => t.name === n)?.key, name);

export const room = (page, key) => page.evaluate((k) => window.kennel.app.rooms.find((r) => r.key === k), key);
export const roomKey = (page, title) => page.evaluate((t) => window.kennel.app.rooms.find((r) => r.title === t)?.key, title);

// Centers the camera on a room so its parts are on screen.
export const center = (page, key) => page.evaluate((k) => {
  const app = window.kennel.app, st = document.querySelector("#stage"), r = app.rooms.find((x) => x.key === k);
  app.flyTo = null; app.cam.follow = false;
  app.cam.x = (r.x + r.w / 2) * 32 - st.clientWidth / app.cam.z / 2;
  app.cam.y = (r.y + r.h / 2) * 32 - st.clientHeight / app.cam.z / 2;
}, key);

// Click where an entity is drawn on the canvas, like a user would.
export const clickEntity = async (page, id, opts = {}) => {
  await page.evaluate((i) => window.kennel.reveal(i), id);
  const pos = await page.evaluate((i) => window.kennel.screenPos(i), id);
  expect(pos, `entity ${id} on screen`).toBeTruthy();
  await page.mouse.click(pos.x, pos.y, opts);
};

export async function dragTo(page, from, to) {
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  for (let i = 1; i <= 8; i++) await page.mouse.move(from.x + ((to.x - from.x) * i) / 8, from.y + ((to.y - from.y) * i) / 8);
  await page.mouse.up();
}

// HTML5 drag of a session from the tray (or anything carrying a pup id) onto the map.
export const dropPup = (page, id, at) => page.evaluate(({ x, y, id }) => {
  const dt = new DataTransfer();
  dt.setData("application/x-kennel-pup", id);
  const c = document.querySelector("#map");
  for (const type of ["dragover", "drop"]) c.dispatchEvent(new DragEvent(type, { clientX: x, clientY: y, dataTransfer: dt, bubbles: true, cancelable: true }));
}, { ...at, id });

export const newKennel = async (page, name) => {
  const k = await post(page, "/api/kennels", { Name: name, Icon: "🧺" });
  await page.waitForFunction((key) => window.kennel.app.rooms.some((r) => r.key === key), "kennel:" + k.ID);
  return k;
};
