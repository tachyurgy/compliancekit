# PdfSigner package dependencies

Add to `frontend/package.json`:

```json
{
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "react-router-dom": "^6.26.0",
    "react-pdf": "^9.1.1",
    "pdfjs-dist": "4.4.168",
    "pdf-lib": "^1.17.1",
    "signature_pad": "^5.0.2"
  },
  "devDependencies": {
    "typescript": "^5.5.4",
    "vite": "^5.4.1",
    "@vitejs/plugin-react": "^4.3.1",
    "vite-plugin-static-copy": "^1.0.6",
    "@types/react": "^18.3.3",
    "@types/react-dom": "^18.3.0"
  }
}
```

## Version compatibility notes

- **`react-pdf@9.x` pairs with `pdfjs-dist@4.4.168` exactly.** react-pdf imports
  the pdf.js API; minor pdfjs bumps break the import shape. Pin the version.
- `pdf-lib@1.17.1` is the latest published release as of this writing; it is
  pure ES5, has no WebAssembly dependency, and works in all evergreen browsers.
- `signature_pad@5.x` requires `Pointer Events`, which is fine in every
  browser ComplianceKit supports (Chrome, Safari, Edge, Firefox — last 2
  major versions).

## Vite config adjustments

`vite-plugin-static-copy` is used to publish the pdf.js worker. Without this
step, `pdfjs.GlobalWorkerOptions.workerSrc = "/pdf.worker.min.mjs"` returns 404
and the PDF never renders.

```ts
// frontend/vite.config.ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { viteStaticCopy } from "vite-plugin-static-copy";

export default defineConfig({
  plugins: [
    react(),
    viteStaticCopy({
      targets: [
        {
          src: "node_modules/pdfjs-dist/build/pdf.worker.min.mjs",
          dest: "",
        },
      ],
    }),
  ],
  optimizeDeps: {
    // pdf-lib ships large; exclude from the dev pre-bundle to avoid OOM on
    // first boot. It still gets bundled in prod.
    exclude: ["pdf-lib"],
  },
  server: {
    // Required for crypto.subtle on some configurations.
    headers: {
      "Cross-Origin-Opener-Policy": "same-origin",
      "Cross-Origin-Embedder-Policy": "credentialless",
    },
  },
});
```

## Content Security Policy

If CSP is enforced, add:

```
worker-src 'self' blob:;
script-src 'self';
style-src 'self' 'unsafe-inline';  // react-pdf inlines a tiny style
connect-src 'self' https://*.amazonaws.com;  // for S3 pre-signed PDF GETs
```

## Install

```sh
cd frontend
npm install
```

## Verify

```sh
npx tsc --noEmit
npm run dev
# then open http://localhost:5173/sign/<token>
```
