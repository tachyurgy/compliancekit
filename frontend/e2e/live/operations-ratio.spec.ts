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
          {
            label: 'Infant',
            age_months_low: 0,
            age_months_high: 12,
            children_present: 5,
            staff_present: 1,
          },
          {
            label: 'Preschool',
            age_months_low: 36,
            age_months_high: 60,
            children_present: 12,
            staff_present: 1,
          },
        ],
      },
      headers: { 'Content-Type': 'application/json' },
    });
    expect(resp.status()).toBe(200);
    const data = await resp.json();
    const rooms: any[] = data.rooms ?? [];
    expect(rooms.length).toBe(2);
    const infant = rooms.find((r: any) => /infant/i.test(r.label ?? ''));
    expect(infant?.in_ratio).toBe(false);
  });
});
