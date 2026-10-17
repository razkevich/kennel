import { defineConfig } from "@playwright/test";

const port = 7399;

export default defineConfig({
  testDir: "./tests",
  timeout: 30_000,
  fullyParallel: false,
  workers: 1,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    viewport: { width: 1440, height: 900 },
    channel: process.env.PW_CHANNEL || (process.env.CI ? undefined : "chrome"),
    trace: "retain-on-failure",
  },
  webServer: {
    // Synthetic sessions only (-demo) and a canned LLM: tests never read or touch real sessions.
    command: `cd .. && KENNEL_FAKE_LLM=1 go run ./cmd/kennel map -demo -no-open -port ${port} -token e2e`,
    url: `http://127.0.0.1:${port}/`,
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
