# Releases

## 2026-10-28 — Dashboard loads again (API matches the frontend contract)
- **What deployed:** https://compliancekit.levelbrook.com (same single container).
- **Changed:**
  - `GET /api/dashboard` now returns the shape the SPA's zod `DashboardSchema` parses: `complianceScore` (same synthesized score as the score hero), `scoreDelta` (vs the newest snapshot at least 7 days old, 0 without history), `updatedAt`, `alerts` (one per violation, with UI severity/category/href), `timeline` (upcoming deadlines in 90 days) and `counts.documents/criticalAlerts/warningAlerts`. The raw engine keys stay for API clients. Before this the page showed "We couldn't load your dashboard" with a zod error list.
  - Engine violations and deadlines carry their rule `category`.
  - New integration test `TestDashboard_MatchesFrontendContract`.
- **How:** `HOST=root@<demo-host> deploy/deploy-demo.sh`.
- **Verified:** see below.

## 2026-10-15 — Demo login shown on sign-in, site-wide demo notice
- **What deployed:** https://compliancekit.levelbrook.com (same single container, Go API + React build).
- **Changed:**
  - Sign-in page (demo build only): a bordered "Demo login" callout with `demo@example.com` / `inspection-ready` sits directly above the password field, and both inputs are pre-filled so one click signs in.
  - New `DemoDeploymentNotice` strip at the top of the landing page, every auth page (sign-in, signup, password reset) and the signed-in app shell: "Demo deployment: fictional data, sandboxes reset after 24h. Sign in with demo@example.com / inspection-ready." Renders nothing unless `VITE_DEMO_MODE=1`.
- **How:** `HOST=root@<demo-host> deploy/deploy-demo.sh`.
- **Verified:** curl `POST /api/auth/signin` with the demo login 200 and `/api/me` 200 before and after the deploy; headless Chromium (desktop 1366px and mobile 390px) shows the notice on landing and sign-in, the callout between the email and password inputs with both pre-filled, and clicking Sign in lands on `/dashboard` with the notice in the app shell and the dashboard populated. tsc and Vitest (72 tests) pass.

## 2026-10-14 — Public demo at compliancekit.levelbrook.com
- **What deployed:** https://compliancekit.levelbrook.com, a single container (Go API + React build) on a Hetzner VM behind kamal-proxy (Let's Encrypt TLS). Demo mode: a shared login (`demo@example.com` / `inspection-ready`) and one-click private sandboxes, all fictional data; email, SMS and Stripe disabled.
- **Changed:**
  - Single-host mode: embedded migrations applied on boot, local-disk storage with HMAC-signed presigned URLs, log-only email/SMS, SPA served by the API, demo tenants and a sandbox janitor.
  - E-signature now works end to end: schema the pdfsign store actually uses (migration 000044), signer routes reachable without a login, template upload/read/PDF endpoints, signature list with downloads, tenant checks on templates (fixes a cross-tenant field-layout overwrite), `sha256Before` bound to the uploaded template, adapter bucket fix, pdf.js worker version fix.
  - Fixed TEXT-timestamp scans that returned 500 on the dashboard, document detail and staff/child document lists, and made every subscription read as inactive.
  - Document presign writes the canonical NOT NULL columns, so operator uploads work.
  - CI replaced with working backend (gofmt, vet, build, unit tests under `-race`, integration suite) and frontend (tsc, Vitest, build) workflows; stale deploy workflows removed; backend gofmt'd.
  - ARCHITECTURE.md added.
- **How:** `HOST=root@<demo-host> deploy/deploy-demo.sh` (local cross-compile + Vite build, runtime-only image on the host, `kamal-proxy deploy compliancekit --host compliancekit.levelbrook.com --tls --health-check-path /healthz`).
- **Verified:** `/healthz` 200 over TLS; password sign-in and `/api/me`, `/api/compliance/score`, `/api/pdfsign/templates` 200 with curl; headless-Chromium walk of landing → one-click sandbox → dashboard, children, staff, documents, inspections, operations, chase, templates, settings, Inspector View with no 5xx; full e-signature on the live site (create link, sign four fields, finalize, signed PDF 200 `application/pdf`, audit JSON with server-side hashes; tampered blob signature returns 403). Container RSS about 16-28 MiB.
