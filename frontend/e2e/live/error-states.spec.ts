import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: error-path coverage.
 */

test.describe('LIVE error-states', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('invalid signup payload surfaces a 400', async ({ request }) => {
    const resp = await request.post(`${BACKEND_URL}/api/auth/signup`, {
      data: { name: '', owner_email: 'not-an-email', state_code: 'ZZ' },
    });
    expect(resp.status()).toBe(400);
  });

  test('unauthenticated /api/me returns 401 JSON', async ({ request }) => {
    const resp = await request.get(`${BACKEND_URL}/api/me`);
    expect(resp.status()).toBe(401);
    const body = await resp.json();
    expect(body.error).toBeDefined();
  });

  test('offline navigation shows no stack trace', async ({ page, context }) => {
    await loginAs(page);
    await context.setOffline(true);
    const resp = await page.goto('/dashboard').catch(() => null);
    // The goto may fail outright, which is fine — we just assert no crash UI.
    if (resp) {
      const body = await page.locator('body').textContent();
      expect(body).not.toMatch(/TypeError|ReferenceError/i);
    }
    await context.setOffline(false);
  });
});
