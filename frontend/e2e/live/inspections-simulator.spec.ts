import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: inspection simulator — start, answer items, finalize.
 */

test.describe('LIVE inspections-simulator', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('start + respond + finalize an inspection', async ({ page }) => {
    await loginAs(page, { state: 'CA' });

    // Start a mock inspection.
    const start = await page.request.post(`${BACKEND_URL}/api/inspections`, {
      data: { kind: 'self', state_code: 'CA' },
      headers: { 'Content-Type': 'application/json' },
    });
    expect([200, 201]).toContain(start.status());
    const run = await start.json();
    expect(run.id).toBeTruthy();

    // Load the run — it should include a set of items.
    const get = await page.request.get(`${BACKEND_URL}/api/inspections/${run.id}`);
    expect(get.status()).toBe(200);
    const full = await get.json();
    const items: any[] = full.items ?? [];

    // Answer the first few items (3 pass, 2 fail).
    for (let i = 0; i < Math.min(items.length, 5); i++) {
      const status = i < 3 ? 'pass' : 'fail';
      const patch = await page.request.patch(
        `${BACKEND_URL}/api/inspections/${run.id}/items/${items[i].id}`,
        {
          data: { status, notes: status === 'fail' ? 'seeded by e2e' : '' },
          headers: { 'Content-Type': 'application/json' },
        },
      );
      expect([200, 204]).toContain(patch.status());
    }

    // Finalize.
    const fin = await page.request.post(
      `${BACKEND_URL}/api/inspections/${run.id}/finalize`,
      { data: {}, headers: { 'Content-Type': 'application/json' } },
    );
    expect([200, 201]).toContain(fin.status());
  });
});
