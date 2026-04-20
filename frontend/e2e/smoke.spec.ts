import { test, expect } from '@playwright/test';

/**
 * Smoke tests — run first, no backend dependency.
 * If these fail, the frontend build is broken (Vite / React / routing).
 * Everything else in e2e/ assumes these pass.
 */

test.describe('Smoke — no backend required', () => {
  test('landing page renders', async ({ page }) => {
    await page.goto('/');
    // The landing page is either the marketing Landing.tsx or a redirect to /login.
    // Both acceptable — just confirm the DOM loaded without white-screen.
    await expect(page.locator('body')).toBeVisible();
    // Expect a recognizable piece of the app brand.
    await expect(page).toHaveTitle(/ComplianceKit|Compliance/i);
  });

  test('login page loads', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('body')).toBeVisible();
    // Should have an email input and a submit button for the magic link.
    await expect(page.getByRole('textbox', { name: /email/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /send|magic|sign/i })).toBeVisible();
  });

  test('unknown route renders without crashing', async ({ page }) => {
    await page.goto('/this-route-does-not-exist');
    // Either renders a 404 or redirects; never a stack trace.
    const body = await page.locator('body').textContent();
    expect(body).not.toMatch(/TypeError|ReferenceError|Cannot read/i);
  });

  test('can request a magic link (happy path)', async ({ page }) => {
    await page.goto('/login');
    await page.getByRole('textbox', { name: /email/i }).fill('smoke-test@example.com');
    await page.getByRole('button', { name: /send|magic|sign/i }).click();

    // The app should either show a "check your email" message, or navigate
    // to a confirmation screen. Either is fine; we just don't want an error toast.
    await page.waitForTimeout(1500);
    const body = await page.locator('body').textContent();
    expect(body).not.toMatch(/error|failed|something went wrong/i);
  });
});
