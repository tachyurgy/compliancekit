import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: drills CRUD.
 */

test.describe('LIVE operations-drills', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('log a fire drill and see it in the list', async ({ page }) => {
    await loginAs(page);

    const today = new Date().toISOString();
    const create = await page.request.post(`${BACKEND_URL}/api/drills`, {
      data: { drill_kind: 'fire', drill_date: today, duration_seconds: 120 },
      headers: { 'Content-Type': 'application/json' },
    });
    expect([200, 201]).toContain(create.status());

    const list = await page.request.get(`${BACKEND_URL}/api/drills`);
    expect(list.status()).toBe(200);
    const drills = await list.json();
    expect(Array.isArray(drills)).toBeTruthy();
    expect(drills.length).toBeGreaterThanOrEqual(1);
    expect(drills.some((d: any) => d.drill_kind === 'fire')).toBeTruthy();
  });
});
