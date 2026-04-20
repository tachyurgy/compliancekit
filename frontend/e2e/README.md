# ComplianceKit frontend — end-to-end tests

Playwright tests that exercise the React app against a running backend.

## First-time setup

```bash
cd frontend
npm install
npm run test:e2e:install   # downloads Chromium for Playwright
```

## Running the tests

1. In one terminal, start the backend:
   ```bash
   cd backend
   go run ./cmd/server
   ```
2. In another terminal (from `frontend/`):
   ```bash
   npm run test:e2e        # headless
   npm run test:e2e:headed # see the browser
   npm run test:e2e:ui     # Playwright's interactive test runner
   ```

`playwright.config.ts` auto-starts the Vite dev server on `:5173` so you don't need a third terminal.

## What's covered

- **`smoke.spec.ts`** — no-backend smoke tests. Landing page renders, login page renders, unknown routes don't crash, magic-link request form submits without errors. Run these first; if they fail, nothing else will work.
- **`signup-flow.spec.ts`** — backend-dependent tests. Signup API contract (CA/TX/FL only), healthz, `/api/me` session guard, onboarding route guard, draft persistence in localStorage.

## Adding tests

One `.spec.ts` per feature. Keep them hermetic — each test should start and end in a clean state (clear cookies in `beforeEach`, use unique emails per test run with `Date.now()` suffix).

Don't rely on seeded DB rows. If you need fixtures, POST them via the API as part of the test, not via SQL.

## Cross-reference

- Manual test steps for every scenario live in `../../QA-TESTING-GUIDE.md`. That file is the human-friendly counterpart to these automated tests. When you add an e2e test, tick the matching ⚡ box in the smoke-test matrix at the bottom of QA-TESTING-GUIDE.md.
