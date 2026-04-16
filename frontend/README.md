# ComplianceKit Frontend

React + TypeScript single-page app for ComplianceKit — daycare compliance SaaS.

## Stack

- Vite + React 18 + TypeScript (strict mode)
- React Router v6 for routing
- Zustand for local/global client state
- React Query (`@tanstack/react-query`) for server state
- Tailwind CSS v3 (custom minimal UI kit, no component library)
- `react-hook-form` + `zod` for forms and validation
- `lucide-react` for icons
- `react-pdf` + `pdf-lib` + `signature_pad` for in-browser PDF signing

## Quickstart

```bash
cd frontend
cp .env.example .env
npm install
npm run dev
```

App runs at http://localhost:5173.

## Environment variables

| Var                            | Notes                                              |
| ------------------------------ | -------------------------------------------------- |
| `VITE_API_BASE_URL`            | Backend API origin. Defaults to `http://localhost:8080`. |
| `VITE_STRIPE_PUBLISHABLE_KEY`  | For Stripe Customer Portal redirects.              |
| `VITE_SENTRY_DSN`              | Optional client error reporting.                   |
| `VITE_BASE_PATH`               | `/` for custom domain, `/repo/` for GH Pages projects. |

## Scripts

| Command            | What it does                    |
| ------------------ | ------------------------------- |
| `npm run dev`      | Vite dev server.                |
| `npm run build`    | Type-check + production build. |
| `npm run preview`  | Preview built app locally.      |
| `npm run typecheck`| Type check only.                |

## Deploy to GitHub Pages

The app is a static SPA and deploys cleanly to GitHub Pages.

1. Set `VITE_BASE_PATH` to your repo path (`/compliancekit/` for project pages, `/` for a custom domain).
2. `npm run build` — output lands in `dist/`.
3. Copy `public/404.html` to `dist/404.html` (Vite already does this). This enables deep-link reloads on Pages.
4. Push `dist/` to the `gh-pages` branch (or wire a GitHub Action).

### GitHub Pages caveats

- Client routing needs the `404.html` SPA redirect trick (included). Pages serves `404.html` for unknown paths, which then bounces the browser to `index.html` with the original path preserved.
- Ensure the Pages domain matches `VITE_API_BASE_URL`'s CORS allowlist on the backend.

## Boundary with PDF signing

Routes `/sign/:token` and `/templates` are lazy-loaded from
`src/pages/SignDocument.tsx` and `src/pages/DocumentTemplates.tsx`, which are
owned by a separate agent. The main app only wires those routes and provides
shared dependencies (`react-pdf`, `pdf-lib`, `signature_pad`, `pdfjs-dist`).
