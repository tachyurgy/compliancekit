# ComplianceKit

Compliance management SaaS for licensed child care providers. Tracks every state-specific regulatory requirement, deadline, and document against a deterministic rules engine. Replaces paper binders, spreadsheets, and pre-inspection panic.

**Tagline:** "Be inspection-ready every single day."

**Launch markets (MVP):** California, Texas, Florida.

---

## Quickstart (development)

```bash
# Clone
git clone git@github.com:compliancekit/backend.git
cd backend

# Environment
cp .env.example .env
# Edit .env — fill in:
#   CK_DB_URL (local Postgres)
#   CK_S3_*_ACCESS_KEY / SECRET_KEY (per-bucket)
#   CK_SES_*, CK_TWILIO_*, CK_STRIPE_*, CK_MISTRAL_API_KEY, CK_GEMINI_API_KEY

# Bring up local Postgres + a MinIO stand-in for S3
docker-compose up -d

# Run migrations
go run ./cmd/migrate up

# Start API
go run ./cmd/api

# In a second terminal, start the worker
go run ./cmd/worker
```

Frontend (separate repo):

```bash
git clone git@github.com:compliancekit/frontend.git
cd frontend
npm install
cp .env.local.example .env.local  # set VITE_API_BASE=http://localhost:8080
npm run dev
```

---

## Architecture (one paragraph)

Go backend running under systemd on a single DigitalOcean droplet, reverse-proxied by Caddy with auto-TLS. React (Vite + TypeScript) frontend hosted on GitHub Pages. PostgreSQL on DigitalOcean Managed. Four dedicated S3 buckets partition document storage by lifecycle and access pattern. Stripe for billing, SES for email, Twilio for SMS. Mistral OCR extracts text from uploaded documents with Gemini Flash as fallback; Gemini Flash is also used for structured metadata extraction and an onboarding chat. The compliance rules engine is a pure Go package — deterministic, fully unit-tested, no LLM at runtime. Authentication is magic-link only, two-part: owner login and per-parent/per-staff upload portals.

---

## Documentation

- [SPEC.md](SPEC.md) — product + technical specification
- [ARCHITECTURE.md](ARCHITECTURE.md) — technical architecture, data flows, security model
- [ROADMAP.md](ROADMAP.md) — week-by-week plan, MVP through post-MVP week 8
- [DECISIONS.md](DECISIONS.md) — architecture decision records (ADRs)
- [EXTERNAL_SERVICES.md](EXTERNAL_SERVICES.md) — third-party integration register
- [QUESTIONS.md](QUESTIONS.md) — open questions awaiting review

Planning research (regulatory, state-specific): [planning-docs/](planning-docs/)
Marketing pages and SEO articles: see `*.html` in repo root.

---

## Contributing

Solo for now. If that changes, this section becomes real.

---

## License

Proprietary. All rights reserved. Not open source. Unauthorized use, copying, or distribution prohibited.

Copyright © 2026 ComplianceKit (Washington LLC).
