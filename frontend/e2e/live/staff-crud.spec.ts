import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: staff CRUD (API-level mirror of children-crud).
 */

test.describe('LIVE staff-crud', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('create + list staff member', async ({ page }) => {
    await loginAs(page);

    const create = await page.request.post(`${BACKEND_URL}/api/staff`, {
      data: {
        first_name: 'Grace',
        last_name: 'Hopper',
        email: `grace-${Date.now()}@ck.local`,
        role: 'director',
        hire_date: '2026-01-01T00:00:00Z',
        status: 'active',
      },
      headers: { 'Content-Type': 'application/json' },
    });
    // Accept both 201 and 200 — create handlers vary.
    expect([200, 201]).toContain(create.status());

    const list = await page.request.get(`${BACKEND_URL}/api/staff`);
    expect(list.status()).toBe(200);
    const staff = await list.json();
    expect(Array.isArray(staff)).toBeTruthy();
    expect(staff.find((s: any) => s.first_name === 'Grace')).toBeDefined();
  });

  test('staff page loads or redirects without crashing', async ({ page }) => {
    await loginAs(page);
    await page.goto('/staff');
    await expect(page).toHaveURL(/\/(staff|onboarding|login)/);
  });
});
