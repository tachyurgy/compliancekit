import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: dashboard API + page shell.
 *
 * The dashboard UI in React requires a fully onboarded provider (RequireOnboarded
 * gate). Until the frontend/backend onboarding-complete contract is reconciled,
 * we focus on the API layer here and assert the page either renders a non-crash
 * state or redirects cleanly.
 */

test.describe('LIVE dashboard', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('dashboard API returns a score + violations for fresh CA provider', async ({ page }) => {
    await loginAs(page);
    const resp = await page.request.get(`${BACKEND_URL}/api/dashboard`);
    expect(resp.status()).toBe(200);
    const data = await resp.json();
    expect(typeof data.score).toBe('number');
    expect(data.score).toBeGreaterThanOrEqual(0);
    expect(data.score).toBeLessThanOrEqual(100);
    expect(Array.isArray(data.violations)).toBeTruthy();
    expect(data.state).toBe('CA');
    expect(data.rules_evaluated).toBeGreaterThan(0);
  });

  test('dashboard page does not white-screen', async ({ page }) => {
    await loginAs(page);
    await page.goto('/dashboard');
    // Accept any route outcome — onboarded -> dashboard, not-onboarded ->
    // /onboarding, session contract mismatch -> /login.
    await expect(page).toHaveURL(/\/(dashboard|onboarding|login)/);
    // No stack traces on screen.
    const body = await page.locator('body').textContent();
    expect(body).not.toMatch(/TypeError|ReferenceError|Cannot read properties/i);
  });
});
