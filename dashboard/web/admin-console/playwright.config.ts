import { defineConfig } from "playwright/test";

const bffPort = 28082;
const dashboardPort = 35173;
const bffURL = `http://127.0.0.1:${bffPort}`;
const dashboardURL = process.env.DASHBOARD_URL ?? `http://127.0.0.1:${dashboardPort}`;
const runSuffix = `${process.pid}-${Date.now()}`;

export default defineConfig({
  testDir: "./e2e",
  timeout: 45_000,
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: dashboardURL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure"
  },
  webServer: process.env.DASHBOARD_URL ? undefined : [
    {
      command: "go run ./cmd/gateway",
      cwd: "../..",
      url: `${bffURL}/bff/health`,
      timeout: 120_000,
      reuseExistingServer: false,
      env: {
        ...process.env,
        BFF_ADDR: `127.0.0.1:${bffPort}`,
		REDIS_ADDR: process.env.E2E_REDIS_ADDR ?? "127.0.0.1:6379",
        ADMIN_STATE_PATH: `tmp/e2e-admin-state-${runSuffix}.json`,
        EMAIL_OUTBOX_PATH: `tmp/e2e-email-outbox-${runSuffix}.log`,
        ADMIN_BOOTSTRAP_EMAIL: "e2e-admin@example.test",
        ADMIN_BOOTSTRAP_USERNAME: "e2e_admin",
        ADMIN_BOOTSTRAP_PASSWORD: "E2E-Admin-Password-1234"
      }
    },
    {
      command: `npm.cmd run dev -- --port ${dashboardPort} --strictPort`,
      cwd: ".",
      url: dashboardURL,
      timeout: 120_000,
      reuseExistingServer: false,
      env: {
        ...process.env,
        VITE_BFF_TARGET: bffURL
      }
    }
  ],
  reporter: [["list"], ["html", { outputFolder: "playwright-report", open: "never" }]]
});
