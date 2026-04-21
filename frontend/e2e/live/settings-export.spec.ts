import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: data export history + create.
 */

test.describe('LIVE settings-export', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('list + create export', async ({ page }) => {
    await loginAs(page);

    // Initial list — may be empty.
    const list1 = await page.request.get(`${BACKEND_URL}/api/exports`);
    expect(list1.status()).toBe(200);

    // Kick off an export.
    const create = await page.request.post(`${BACKEND_URL}/api/exports`, {
      data: {},
      headers: { 'Content-Type': 'application/json' },
    });
    expect([200, 201, 202]).toContain(create.status());

    // Next list should have at least one row.
    const list2 = await page.request.get(`${BACKEND_URL}/api/exports`);
    const data = await list2.json();
    const rows: any[] = Array.isArray(data) ? data : (data.items ?? data.rows ?? []);
    expect(rows.length).toBeGreaterThanOrEqual(1);
    expect(rows[0]).toHaveProperty('status');
  });
});
