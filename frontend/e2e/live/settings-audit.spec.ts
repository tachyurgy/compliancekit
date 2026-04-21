import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: audit log.
 * We emit signup + login audit rows on every test, so the list should be
 * non-empty immediately after `loginAs`.
 */

test.describe('LIVE settings-audit', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('audit log API returns recent rows for this tenant', async ({ page }) => {
    await loginAs(page);

    const resp = await page.request.get(`${BACKEND_URL}/api/audit-log`);
    expect(resp.status()).toBe(200);
    const data = await resp.json();
    const rows: any[] = Array.isArray(data) ? data : (data.items ?? data.rows ?? []);
    expect(rows.length).toBeGreaterThan(0);
    // Should include a signup or login action.
    expect(
      rows.some((r: any) => /signup|login/i.test(r.action ?? '')),
    ).toBeTruthy();
  });
});
