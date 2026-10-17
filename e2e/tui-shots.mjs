// Renders kennel's TUI frames (ANSI dumps from `go test -run TestShots`) through
// xterm.js in Chrome, so README screenshots match a real terminal cell grid.
// usage: node tui-shots.mjs <ans-dir> <out-dir>
import { chromium } from "playwright";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const [ansDir, outDir] = process.argv.slice(2);
const cols = 160, rows = 46;
const css = fs.readFileSync(path.join(here, "node_modules/@xterm/xterm/css/xterm.css"), "utf8");
const xterm = fs.readFileSync(path.join(here, "node_modules/@xterm/xterm/lib/xterm.js"), "utf8");
const uni = fs.readFileSync(path.join(here, "node_modules/@xterm/addon-unicode11/lib/addon-unicode11.js"), "utf8");

const browser = await chromium.launch({ channel: process.env.PW_CHANNEL || "chrome" });
const page = await browser.newPage({ deviceScaleFactor: 2, viewport: { width: 1700, height: 1000 } });
await page.setContent(`<!doctype html><html><head><style>${css}
  body { margin: 0; background: #11111b; }
  #win { display: inline-block; margin: 24px; border-radius: 12px; overflow: hidden; box-shadow: 0 20px 60px #000a; background: #1e1e2e; }
  #bar { height: 30px; background: #181825; display: flex; align-items: center; gap: 8px; padding: 0 12px; }
  #bar i { width: 12px; height: 12px; border-radius: 50%; display: inline-block; }
  #bar span { color: #6c7086; font: 12px -apple-system, system-ui; margin-left: 10px; }
  #term { padding: 10px 12px 12px; }
</style></head><body><div id="win"><div id="bar"><i style="background:#f38ba8"></i><i style="background:#f9e2af"></i><i style="background:#a6e3a1"></i><span>kennel</span></div><div id="term"></div></div>
<script>${xterm}</script><script>${uni}</script></body></html>`);
await page.evaluate(({ cols, rows }) => {
  const term = new window.Terminal({ cols, rows, fontFamily: "Menlo, 'DejaVu Sans Mono', monospace", fontSize: 13, lineHeight: 1.12,
    allowProposedApi: true, theme: { background: "#1e1e2e", foreground: "#cdd6f4" } });
  term.loadAddon(new window.Unicode11Addon.Unicode11Addon());
  term.unicode.activeVersion = "11";
  term.open(document.getElementById("term"));
  window.term = term;
}, { cols, rows });

fs.mkdirSync(outDir, { recursive: true });
for (const f of fs.readdirSync(ansDir).filter((f) => f.endsWith(".ans")).sort()) {
  const data = fs.readFileSync(path.join(ansDir, f), "utf8").replace(/\n/g, "\r\n");
  await page.evaluate((d) => new Promise((res) => { window.term.reset(); window.term.write("\x1b[?25l" + d, res); }), data);
  await page.waitForTimeout(150);
  const out = path.join(outDir, "tui-" + f.replace(".ans", ".png"));
  await page.locator("#win").screenshot({ path: out });
  console.log("wrote", out);
}
await browser.close();
