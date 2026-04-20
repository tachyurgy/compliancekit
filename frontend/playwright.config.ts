import { defineConfig, devices } from '@playwright/test';

// Playwright config for ComplianceKit frontend end-to-end tests.
//
// To run:
//   1. Install deps (once): `npm install` then `npx playwright install chromium`
//   2. Start the backend:   `cd ../backend && go run ./cmd/server`
//      (the test runner will auto-start the Vite dev server via webServer below)
//   3. Run tests:           `npm run test:e2e`
//   4. Headed/debug:        `npm run test:e2e:headed`
//
// The e2e tests assume the backend is reachable at http://localhost:8080 with
// a clean SQLite DB. If you're iterating, set PW_BACKEND_URL to point
// somewhere else (e.g. a seeded staging DB).

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false, // tests share state (auth cookies, DB rows)
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI ? 'github' : [['list'], ['html', { open: 'never' }]],
  timeout: 30_000,
  expect: { timeout: 5_000 },

  use: {
    baseURL: process.env.PW_FRONTEND_URL ?? 'http://localhost:5173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },

  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],

  webServer: [
    {
      command: 'npm run dev -- --port 5173',
      url: 'http://localhost:5173',
      reuseExistingServer: !process.env.CI,
      timeout: 30_000,
    },
  ],
});
