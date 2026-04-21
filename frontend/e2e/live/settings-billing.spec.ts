import { test, expect } from '@playwright/test';
import { loginAs } from '../helpers/auth';

/**
 * LIVE: settings/billing page renders.
 * Stripe is stubbed; we only assert the page doesn't crash.
 */

test.describe('LIVE settings-billing', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('settings page loads without stack trace', async ({ page }) => {
    await loginAs(page);
    await page.goto('/settings');
    await expect(page).toHaveURL(/\/(settings|onboarding|login)/);
    const body = await page.locator('body').textContent();
    expect(body).not.toMatch(/TypeError|ReferenceError/i);
  });

  test('billing sub-page loads without stack trace', async ({ page }) => {
    await loginAs(page);
    await page.goto('/settings/billing');
    await expect(page).toHaveURL(/\/(settings|onboarding|login)/);
    const body = await page.locator('body').textContent();
    expect(body).not.toMatch(/TypeError|ReferenceError/i);
  });
});
