var _a;
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';
import fs from 'node:fs';
// Copy the pdf.js worker into /public on startup so react-pdf can load it from the same origin.
function copyPdfjsWorker() {
    try {
        var workerSrc = path.resolve(__dirname, 'node_modules/pdfjs-dist/build/pdf.worker.min.mjs');
        var destDir = path.resolve(__dirname, 'public');
        var destPath = path.join(destDir, 'pdf.worker.min.mjs');
        if (fs.existsSync(workerSrc)) {
            if (!fs.existsSync(destDir))
                fs.mkdirSync(destDir, { recursive: true });
            fs.copyFileSync(workerSrc, destPath);
        }
    }
    catch (_a) {
        // Best-effort; the worker can also be loaded from a CDN at runtime.
    }
}
copyPdfjsWorker();
export default defineConfig({
    plugins: [react()],
    base: (_a = process.env.VITE_BASE_PATH) !== null && _a !== void 0 ? _a : '/',
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
