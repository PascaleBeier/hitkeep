const path = require("node:path");
const { defineConfig } = require("playwright/test");

const port = Number(process.env.HITKEEP_E2E_PORT || 8098);
const baseURL = process.env.HITKEEP_BASE_URL || `http://127.0.0.1:${port}`;
const repoRoot = path.resolve(__dirname, "../..");

module.exports = defineConfig({
    testDir: "./e2e",
    timeout: 60_000,
    expect: {
        timeout: 10_000
    },
    fullyParallel: false,
    workers: 1,
    retries: process.env.CI ? 1 : 0,
    reporter: [["list"], ["html", { open: "never" }]],
    use: {
        baseURL,
        trace: "retain-on-failure",
        screenshot: "only-on-failure",
        video: "retain-on-failure"
    },
    webServer: process.env.HITKEEP_BASE_URL
        ? undefined
        : {
              command: "bash tests/e2e/serve-dashboard.sh",
              cwd: repoRoot,
              url: `${baseURL}/healthz`,
              timeout: 300_000,
              reuseExistingServer: !process.env.CI,
              stdout: "pipe",
              stderr: "pipe"
          }
});
