import { execSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// __dirname shim for ESM — Playwright's config loader runs this as a module,
// so CommonJS globals aren't available.
const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

/**
 * globalSetup — runs once before all tests.
 *
 * 1. Nukes any previous /tmp/ck-e2e.db and its WAL/SHM sidecars.
 * 2. Applies every migration from backend/migrations into a fresh SQLite DB
 *    so the backend server can start without its own migrate step.
 *
 * Uses `migrate` CLI if present (fast, proper versioning); otherwise falls
 * back to piping every *.up.sql into the `sqlite3` CLI. Both achieve the
 * same end state for a fresh DB.
 */
export default async function globalSetup(): Promise<void> {
  const dbPath = '/tmp/ck-e2e.db';
  // Nuke DB + WAL/SHM sidecars. The Go server opens with WAL so these files
  // are written alongside the main DB file.
  for (const suffix of ['', '-wal', '-shm', '-journal']) {
    try {
      fs.unlinkSync(dbPath + suffix);
    } catch {
      /* ok */
    }
  }

  const migrationsDir = path.resolve(__dirname, '../../backend/migrations');
  if (!fs.existsSync(migrationsDir)) {
    throw new Error(`globalSetup: migrations dir not found at ${migrationsDir}`);
  }

  // Try `migrate` CLI first (golang-migrate). If not installed, fall back to
  // sqlite3 CLI piping each *.up.sql. Both work for a fresh DB.
  let used = 'migrate';
  try {
    execSync(
      `migrate -path ${migrationsDir} -database "sqlite3://${dbPath}" up`,
      { stdio: 'inherit' },
    );
  } catch {
    used = 'sqlite3';
    const files = fs
      .readdirSync(migrationsDir)
      .filter((f) => f.endsWith('.up.sql'))
      .sort();
    const combined = files
      .map((f) => fs.readFileSync(path.join(migrationsDir, f), 'utf8'))
      .join('\n;\n');
    // Pipe SQL into sqlite3 via stdin so we don't hit ARG_MAX on big inputs.
    execSync(`sqlite3 ${dbPath}`, { input: combined, stdio: ['pipe', 'inherit', 'inherit'] });
  }

  // Sanity-check: the providers table should exist now.
  const tables = execSync(`sqlite3 ${dbPath} ".tables"`, { encoding: 'utf8' });
  if (!/\bproviders\b/.test(tables)) {
    throw new Error(
      `globalSetup: providers table not created (used ${used}). Tables: ${tables}`,
    );
  }

  // Expose the path to the rest of the suite if helpers want to poke the DB
  // directly — but we prefer HTTP test endpoints to raw DB access.
  process.env.CK_E2E_DB = dbPath;
}
