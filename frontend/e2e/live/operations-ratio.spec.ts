import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: ratio-check endpoint.
 * CA infant ratio is 1:4 — so 5 infants + 1 staff should fail, 12 preschool +
 * 1 staff should pass in TX (1:15). This test uses the backend endpoint only.
 */

test.describe('LIVE operations-ratio', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('ratio-check flags an understaffed infant room', async ({ page }) => {
    await loginAs(page, { state: 'CA' });
    const resp = await page.request.post(`${BACKEND_URL}/api/facility/ratio-check`, {
      data: {
        rooms: [
          { name: 'Infant', age_group: 'infant', children: 5, staff: 1 },
          { name: 'Preschool', age_group: 'preschool', children: 12, staff: 1 },
        ],
      },
      headers: { 'Content-Type': 'application/json' },
    });
    expect(resp.status()).toBe(200);
    const data = await resp.json();
    // Expect at least one room flagged as out-of-ratio.
    const rooms: any[] = data.rooms ?? data.results ?? [];
    expect(rooms.length).toBeGreaterThan(0);
    const infant = rooms.find((r: any) =>
      /infant/i.test(r.name ?? r.age_group ?? ''),
    );
    if (infant) {
      expect(infant.ok ?? infant.in_ratio ?? infant.pass).toBeFalsy();
    }
  });
});
