import { defineConfig, devices } from '@playwright/test'

const origin = 'http://127.0.0.1:18573'

// E2E runs the real binary, with the web build embedded, against the compose Postgres: the same
// artifact the image ships. `task e2e` builds it first.
export default defineConfig({
  testDir: 'tests/e2e',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  // A flaky test is a bug to fix, not to retry.
  retries: 0,
  reporter: process.env.CI ? [['github'], ['list']] : 'list',
  use: { baseURL: origin, trace: 'retain-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'phone', use: { ...devices['Pixel 7'] } },
  ],
  webServer: {
    command: '../bin/estate-dashboard',
    url: `${origin}/readyz`,
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
    env: {
      ADDR: new URL(origin).host,
      DATABASE_URL: process.env.DATABASE_URL ?? 'postgres://app:app@localhost:55432/app?sslmode=disable',
      ALERTMANAGER_URL: process.env.ALERTMANAGER_URL ?? 'http://localhost:59093',
      // A local run signs nobody in through auth; every request is this admin.
      DEV_USER: 'e2e',
    },
  },
})
