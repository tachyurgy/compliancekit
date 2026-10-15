# ComplianceKit

Licensing-compliance software for child care centers. It tracks every state requirement, deadline and document an inspector will ask for, scores how inspection-ready a center is today, chases parents and staff for expiring paperwork, and collects signatures on forms with a self-hosted PDF e-signature flow.

**Go backend** (single static binary, SQLite, `net/http` + chi) and a **React + TypeScript** frontend.

- **Live demo:** https://compliancekit.levelbrook.com
  - Click **Explore the live demo** for a private, pre-seeded sandbox (deleted after 24 hours), or
  - sign in with **`demo@example.com`** / **`inspection-ready`** (shared account; its sample data is restored hourly if someone deletes it).
  - All data is fictional. Email, SMS and payments are switched off; signing links are shown on screen instead of emailed.
- **Architecture, design and why Go:** [ARCHITECTURE.md](ARCHITECTURE.md)
- **Release history:** [RELEASES.md](RELEASES.md)

## Things to try in the demo

1. **Dashboard:** the readiness score and top gaps for a fictional Florida center (Sunshine Kids of Hialeah).
2. **Staff / Children:** per-person required documents, with what is missing and what is expiring.
3. **Inspections:** run a mock inspection against Florida's CF-FSP 5316 checklist. **Open Inspector View** shows the read-only screen a center hands to the inspector.
4. **Templates → Send for signature:** create a signing link for the sample permission slip, open it, fill and sign it, then download the signed PDF (with its certificate page) and the JSON audit record.
5. **Chase Mode / Send a message:** reminder history and manual nudges. They are recorded but not delivered in the demo.

## Layout

```
backend/    Go API, rules engine, pdfsign, workers, migrations (cmd/server is the entry point)
frontend/   React 18 + Vite + TypeScript app, Vitest + Playwright tests
docs/       pdf-signature-spec.md (the e-signature design)
deploy/     demo runtime image, env template, deploy script
Dockerfile  multi-stage from-source build of the whole thing
```

## Run it locally

Requires Go 1.25+ and Node 20+.

```bash
# 1. Build the frontend in demo mode (same-origin API)
cd frontend
npm ci
VITE_DEMO_MODE=1 VITE_API_BASE_URL= npm run build
cd ../backend

# 2. Run the API + UI on :8080 with local storage and no outbound email/SMS
export APP_BASE_URL=http://localhost:8080 FRONTEND_BASE_URL=http://localhost:8080 \
  SESSION_COOKIE_DOMAIN=localhost DATABASE_URL=./ck.db AUTO_MIGRATE=true \
  STATIC_DIR=../frontend/dist STORAGE_LOCAL_DIR=./blobs OUTBOUND_DISABLED=true \
  DEMO_MODE=true DEMO_ACCOUNT_EMAIL=demo@example.com DEMO_ACCOUNT_PASSWORD=inspection-ready \
  S3_BUCKET_DOCUMENTS=documents S3_BUCKET_SIGNED_PDFS=signed-pdfs \
  S3_BUCKET_AUDIT_TRAIL=audit-trail S3_BUCKET_RAW_UPLOADS=raw-uploads \
  SES_FROM_EMAIL=no-reply@example.com STRIPE_SECRET_KEY=sk_test_disabled \
  STRIPE_WEBHOOK_SECRET=whsec_disabled STRIPE_PRICE_PRO=price_disabled \
  MAGIC_LINK_SIGNING_KEY=$(openssl rand -hex 32)
go run ./cmd/server
```

Open http://localhost:8080. For frontend development with hot reload, run `npm run dev` in `frontend/` with `VITE_API_BASE_URL=http://localhost:8080`.

Or with Docker: `docker build -t compliancekit .`, then run it with the variables from `deploy/demo.env.example` (set `APP_BASE_URL`, `FRONTEND_BASE_URL` and `SESSION_COOKIE_DOMAIN` for your host).

## Tests

```bash
cd backend && go test ./...          # 400 Go tests incl. HTTP integration tests on a fresh SQLite per test
cd frontend && npx tsc -b && npx vitest run
```

CI runs both on every push (`.github/workflows/`).

## License

Proprietary. All rights reserved. The source is published for review. It is not open source, and it may not be used, copied or distributed without permission.

Copyright © 2026 ComplianceKit (Washington LLC).
