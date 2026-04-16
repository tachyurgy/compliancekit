import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';
import fs from 'node:fs';

// Copy the pdf.js worker into /public on startup so react-pdf can load it from the same origin.
function copyPdfjsWorker(): void {
  try {
    const workerSrc = path.resolve(
      __dirname,
      'node_modules/pdfjs-dist/build/pdf.worker.min.mjs',
    );
    const destDir = path.resolve(__dirname, 'public');
    const destPath = path.join(destDir, 'pdf.worker.min.mjs');
    if (fs.existsSync(workerSrc)) {
      if (!fs.existsSync(destDir)) fs.mkdirSync(destDir, { recursive: true });
      fs.copyFileSync(workerSrc, destPath);
    }
  } catch {
    // Best-effort; the worker can also be loaded from a CDN at runtime.
  }
}

copyPdfjsWorker();

export default defineConfig({
  plugins: [react()],
  base: process.env.VITE_BASE_PATH ?? '/',
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    host: true,
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
    target: 'es2020',
  },
});
