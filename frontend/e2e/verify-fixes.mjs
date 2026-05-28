// Targeted prod verifier for the bugs reported on 2026-05-23.
// Uses an existing QA account (no Stripe Checkout).
//
// Run: cd frontend && node e2e/verify-fixes.mjs
//
// Each "check" hits one URL, waits for it to settle, and asserts that the
// page does NOT contain raw Zod JSON or other diagnostic markers from the
// reported issues. Failures are recorded with a screenshot at /tmp.
import { chromium } from 'playwright';
import path from 'node:path';

const APP = 'https://app.levelbrook.com';
const EMAIL = process.env.QA_EMAIL || 'qa-verify-1779575973813@levelbrook.test';
const PASSWORD = process.env.QA_PASSWORD || 'correct horse staple battery';
const TMPDIR = '/tmp';

const results = [];
function record(name, pass, observation) {
  results.push({ name, pass, observation });
  const tag = pass ? 'PASS' : 'FAIL';
  console.log(`[${tag}] ${name} — ${observation}`);
}

async function shot(page, slug) {
  const p = path.join(TMPDIR, `verify-${slug}.png`);
  await page.screenshot({ path: p, fullPage: true }).catch(() => {});
  return p;
}

function looksLikeZodDump(text) {
  if (!text) return false;
  // The user pasted: `[ { "code": "invalid_type", "expected": "string", ...`
  // Catch both the bracketed array and the field-name signatures.
  return (
    /"code"\s*:\s*"invalid_type"/.test(text) ||
    /"path"\s*:\s*\[\s*\d+\s*,\s*"\w+"\s*\]/.test(text) ||
    /ZodError/.test(text)
  );
}

async function loginAndAssertHome(page) {
  await page.goto(`${APP}/login`, { waitUntil: 'domcontentloaded' });
  await page.locator('input[name="email"]').fill(EMAIL);
  await page.locator('input[name="password"]').fill(PASSWORD);
  await page.locator('button[type="submit"]').first().click();
  await page.waitForURL((u) => !/\/login/.test(u.toString()), { timeout: 20000 });
  record('login', true, `landed at ${page.url()}`);
}

async function checkRoute(page, route, slug, label) {
  await page.goto(`${APP}${route}`, { waitUntil: 'domcontentloaded' });
  // Give React/queries time to settle.
  await page.waitForTimeout(2500);
  const body = await page.locator('body').innerText().catch(() => '');
  const zod = looksLikeZodDump(body);
  if (zod) {
    await shot(page, `zod-${slug}`);
    record(label, false, `Zod-dump in body at ${route}`);
    return;
  }
  // Generic "Couldn't load" indicators that should not appear.
  if (/couldn't load your roster|couldn't load your facility|too many requests/i.test(body)) {
    await shot(page, `loadfail-${slug}`);
    const m = body.match(/(Couldn't load[^\n]{0,80}|too many requests)/i);
    record(label, false, `load failure: ${m?.[0]}`);
    return;
  }
  record(label, true, `${route} clean`);
}

async function checkTemplates(page) {
  await page.goto(`${APP}/templates`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(2500);
  const body = await page.locator('body').innerText().catch(() => '');
  // The original bug: page rendered a Cloudflare Pages 404 HTML inline
  // because DocumentTemplates was hitting the wrong env var (VITE_API_BASE
  // instead of VITE_API_BASE_URL). After the fix it should render the
  // "Document templates" header and either an empty state or a list.
  if (body.includes('<!doctype html>') || body.includes('GitHub Pages SPA')) {
    await shot(page, 'templates-404');
    record('/templates', false, '404 HTML leaking into page body');
    return;
  }
  if (!/Document templates/i.test(body)) {
    await shot(page, 'templates-empty');
    record('/templates', false, 'header missing — page did not mount');
    return;
  }
  record('/templates', true, 'header present, no 404 leak');
}

async function checkAddChildModalWidth(page) {
  await page.goto(`${APP}/children`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(2000);
  const trigger = page.getByRole('button', { name: /add a child/i }).first();
  if (await trigger.count() === 0) {
    record('add-child-modal width', false, 'no Add a child button found');
    return;
  }
  await trigger.click();
  await page.waitForTimeout(800);
  const dialog = page.locator('[role="dialog"]').first();
  const box = await dialog.boundingBox();
  const viewport = page.viewportSize();
  if (!box || !viewport) {
    record('add-child-modal width', false, 'no dialog rendered');
    return;
  }
  const overflowsRight = box.x + box.width > viewport.width + 1;
  const overflowsLeft = box.x < 0;
  if (overflowsRight || overflowsLeft) {
    await shot(page, 'addchild-overflow');
    record(
      'add-child-modal width',
      false,
      `modal at (${box.x},${box.y}) ${box.width}x${box.height} in viewport ${viewport.width}x${viewport.height}`,
    );
  } else {
    record('add-child-modal width', true, `${Math.round(box.width)}px wide in ${viewport.width}px viewport`);
  }
  // Close the modal so subsequent checks aren't blocked.
  await page.keyboard.press('Escape').catch(() => {});
}

async function checkNavMessageLink(page) {
  await page.goto(`${APP}/`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1500);
  const link = page.locator('a[href="/messages/new"]').first();
  const has = (await link.count()) > 0;
  record('nav /messages/new', has, has ? 'present in sidebar' : 'missing — not linked');
}

async function checkChasePage(page) {
  await page.goto(`${APP}/chase`, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(2000);
  const body = await page.locator('body').innerText().catch(() => '');
  // Must render the page (not redirect to login or show a 404)
  if (/login|404|page not found/i.test(body) && !/Chase/i.test(body)) {
    await shot(page, 'chase-missing');
    record('/chase mounts', false, 'chase did not render');
    return;
  }
  record('/chase mounts', true, '/chase rendered');
}

(async () => {
  const browser = await chromium.launch({ headless: true });
  const ctx = await browser.newContext({
    viewport: { width: 1280, height: 900 },
  });
  const page = await ctx.newPage();

  // Capture console errors for diagnostics
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const text = msg.text();
      if (looksLikeZodDump(text)) {
        console.log(`  ! console Zod error: ${text.slice(0, 200)}`);
      }
    }
  });

  try {
    await loginAndAssertHome(page);
    await checkNavMessageLink(page);
    await checkRoute(page, '/children', 'children', '/children no Zod dump');
    await checkRoute(page, '/settings', 'settings', '/settings facility loads');
    await checkRoute(page, '/operations', 'operations', '/operations renders');
    await checkRoute(page, '/staff', 'staff', '/staff renders');
    await checkTemplates(page);
    await checkChasePage(page);
    await checkAddChildModalWidth(page);
  } catch (e) {
    console.log('fatal:', e.message);
  } finally {
    await browser.close();
  }

  console.log('\n--- SUMMARY ---');
  const passes = results.filter((r) => r.pass).length;
  const fails = results.length - passes;
  for (const r of results) {
    console.log(`${r.pass ? 'PASS' : 'FAIL'}: ${r.name} — ${r.observation}`);
  }
  console.log(`\n${passes}/${results.length} passing, ${fails} failing`);
  process.exit(fails ? 1 : 0);
})();
