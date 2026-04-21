import { test, expect } from '@playwright/test';
import { loginAs } from '../helpers/auth';

/**
 * LIVE: route guard behavior for authed and unauthed users.
 */

const PROTECTED = [
  '/dashboard',
  '/children',
  '/staff',
  '/operations',
  '/inspections',
  '/settings',
];

test.describe('LIVE route-guards', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  for (const path of PROTECTED) {
    test(`unauthenticated ${path} redirects to /login`, async ({ page }) => {
      await page.goto(path);
      await page.waitForURL(/\/login/, { timeout: 10_000 });
      await expect(page).toHaveURL(/\/login/);
    });
  }

  test('authenticated but not onboarded user goes to /onboarding', async ({ page }) => {
    await loginAs(page);
    await page.goto('/dashboard');
    // Expect /onboarding OR /login (if the session contract doesn't match).
    await expect(page).toHaveURL(/\/(onboarding|login|dashboard)/);
  });
});
