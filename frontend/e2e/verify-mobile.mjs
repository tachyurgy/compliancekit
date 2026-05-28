// Mobile/tablet viewport check for Add Child modal width — the original
// user report was that the modal "stretches off of the page". Verify the
// dialog stays within the viewport at 375px (iPhone) and 768px (iPad).
import { chromium } from 'playwright';
import path from 'node:path';

const APP = 'https://app.levelbrook.com';
const EMAIL = process.env.QA_EMAIL || 'qa-verify-1779575973813@levelbrook.test';
const PASSWORD = process.env.QA_PASSWORD || 'correct horse staple battery';

const results = [];
function record(name, pass, observation) {
  results.push({ name, pass, observation });
  console.log(`[${pass ? 'PASS' : 'FAIL'}] ${name} — ${observation}`);
}

async function check(viewport) {
  const browser = await chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport });
  const page = await ctx.newPage();
  try {
    await page.goto(`${APP}/login`, { waitUntil: 'domcontentloaded' });
    await page.locator('input[name="email"]').fill(EMAIL);
    await page.locator('input[name="password"]').fill(PASSWORD);
    await page.locator('button[type="submit"]').first().click();
    await page.waitForURL((u) => !/\/login/.test(u.toString()), { timeout: 20000 });

    await page.goto(`${APP}/children`, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(2500);

    // Open the side nav on mobile if needed (the button moves into a hamburger).
    const trigger = page.getByRole('button', { name: /add a child/i }).first();
    if ((await trigger.count()) === 0) {
      record(`add-child modal @ ${viewport.width}px`, false, 'no Add a child button visible');
      return;
    }
    await trigger.click();
    await page.waitForTimeout(800);
    const dialog = page.locator('[role="dialog"]').first();
    const box = await dialog.boundingBox();
    if (!box) {
      record(`add-child modal @ ${viewport.width}px`, false, 'dialog not rendered');
      return;
    }
    const ok = box.x >= 0 && box.x + box.width <= viewport.width + 0.5;
    if (!ok) {
      const p = `/tmp/verify-modal-${viewport.width}.png`;
      await page.screenshot({ path: p, fullPage: true });
      record(
        `add-child modal @ ${viewport.width}px`,
        false,
        `dialog at x=${box.x.toFixed(0)} w=${box.width.toFixed(0)} (viewport ${viewport.width}) — screenshot ${p}`,
      );
    } else {
      record(
        `add-child modal @ ${viewport.width}px`,
        true,
        `dialog ${box.width.toFixed(0)}px in ${viewport.width}px viewport`,
      );
    }
    // Also check no horizontal scroll on the page itself
    const scrollLeftMax = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    if (scrollLeftMax > 1) {
      record(
        `no h-scroll @ ${viewport.width}px`,
        false,
        `body overflows by ${scrollLeftMax}px`,
      );
    } else {
      record(`no h-scroll @ ${viewport.width}px`, true, 'body fits viewport');
    }
  } finally {
    await browser.close();
  }
}

(async () => {
  await check({ width: 375, height: 812 });   // iPhone X
  await check({ width: 768, height: 1024 });  // iPad portrait
  await check({ width: 1024, height: 1366 }); // iPad Pro portrait
  const fails = results.filter((r) => !r.pass).length;
  console.log(`\n${results.length - fails}/${results.length} passing`);
  process.exit(fails ? 1 : 0);
})();
