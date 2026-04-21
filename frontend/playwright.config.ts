import { defineConfig, devices } from '@playwright/test';

// Playwright config for ComplianceKit — LIVE end-to-end harness.
//
// "LIVE" means: the test runner boots BOTH servers (Go backend + Vite dev
// server), applies migrations into a fresh SQLite file, and exercises real
// user flows through the real UI. No mocks. See ./e2e/globalSetup.ts for the
// DB bootstrap.
//
// To run:
//   1. Install deps (once): `npm install && npx playwright install chromium`
//   2. Run tests:           `npm run test:e2e`
//   3. Headed / debug:      `npm run test:e2e:headed`
//
// Env knobs:
//   CI=1                             Strict mode — no reuseExistingServer, retries=2.
//   PW_FRONTEND_URL=http://...       Override the Vite URL (otherwise http://localhost:5173).
//   PW_BACKEND_URL=http://...        Override the backend URL (otherwise http://localhost:8080).

const BACKEND_DB = '/tmp/ck-e2e.db';
const BACKEND_PORT = 8080;
const FRONTEND_PORT = 5173;

// Env block for the backend server, flattened onto one line so `sh -c` picks
// it up as in-line vars for `go run`. Kept as an array so future additions
// are easy to spot.
const backendEnvInline = [
  `DATABASE_URL=${BACKEND_DB}`,
  'MAGIC_LINK_SIGNING_KEY=test-signing-key-ee2e-xxxxxxxxxxxxxx',
  `FRONTEND_BASE_URL=http://localhost:${FRONTEND_PORT}`,
  `APP_BASE_URL=http://localhost:${BACKEND_PORT}`,
  `PORT=${BACKEND_PORT}`,
  'SESSION_COOKIE_DOMAIN=localhost',
  'APP_ENV=test',
  'S3_BUCKET_DOCUMENTS=ck-docs-e2e',
  'S3_BUCKET_SIGNED_PDFS=ck-signed-e2e',
  'S3_BUCKET_AUDIT_TRAIL=ck-audit-e2e',
  'S3_BUCKET_RAW_UPLOADS=ck-raw-e2e',
  'SES_FROM_EMAIL=noreply@ck.local',
  'STRIPE_SECRET_KEY=sk_test_stub',
  'STRIPE_WEBHOOK_SECRET=whsec_stub',
  'STRIPE_PRICE_PRO=price_stub',
].join(' ');

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI ? 'github' : [['list'], ['html', { open: 'never' }]],
  timeout: 60_000,
  expect: { timeout: 10_000 },

  globalSetup: './e2e/globalSetup.ts',

  use: {
    baseURL: process.env.PW_FRONTEND_URL ?? `http://localhost:${FRONTEND_PORT}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },

  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],

  // The `sh -c` wrapper below exports env inline so the Go server picks it up.
  // We intentionally DO NOT use `go run` here — the spec says `go build` then
  // run the binary; but `go run` is fine for local dev and picks up code
  // changes between runs. reuseExistingServer=true (non-CI) means Playwright
  // will attach to an already-running server; this is what you want when
  // iterating locally with `go run ./cmd/server` in another terminal.
  webServer: [
    {
      // Start sequence: nuke previous DB files, pipe every migration through
      // sqlite3 CLI to recreate the schema, then exec the server. This runs
      // inline with the webServer command so we don't race Playwright's
      // globalSetup ordering (which is not guaranteed to precede webServer).
      command: `sh -c 'rm -f ${BACKEND_DB} ${BACKEND_DB}-wal ${BACKEND_DB}-shm ${BACKEND_DB}-journal && cat ../backend/migrations/*.up.sql | sqlite3 ${BACKEND_DB} && cd ../backend && ${backendEnvInline} go run ./cmd/server'`,
      url: `http://localhost:${BACKEND_PORT}/healthz`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      stdout: 'pipe',
      stderr: 'pipe',
    },
    {
      command: `npm run dev -- --port ${FRONTEND_PORT} --strictPort`,
      url: `http://localhost:${FRONTEND_PORT}`,
      reuseExistingServer: !process.env.CI,
      timeout: 60_000,
    },
  ],
});
