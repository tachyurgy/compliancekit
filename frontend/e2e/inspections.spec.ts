import { test, expect } from '@playwright/test';

/**
 * Inspections page smoke tests.
 *
 * UI-only smoke layer — no seeded backend DB, no real session. A full E2E
 * (start a mock inspection, walk through the wizard, finalize, view the
 * report) is a follow-up that requires either a test-only session endpoint
 * or a helper that completes the magic-link flow before the test.
 *
 * For now we verify:
 *   - /inspections redirects to /login when unauthenticated
 *   - /inspections/:id also redirects to /login when unauthenticated
 *
 * The "Start a mock inspection" CTA on the empty-state of the authed page
 * is asserted in signup-flow.spec.ts once that test adds a post-onboarding
 * navigation step.
 */

test.describe('Inspections page — smoke', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('/inspections redirects to /login when unauthenticated', async ({ page }) => {
    await page.goto('/inspections');
    await page.waitForURL(/\/login/, { timeout: 5_000 });
    await expect(page).toHaveURL(/\/login/);
  });

  test('/inspections/:id redirects to /login when unauthenticated', async ({ page }) => {
    await page.goto('/inspections/some-fake-run-id');
    await page.waitForURL(/\/login/, { timeout: 5_000 });
    await expect(page).toHaveURL(/\/login/);
  });
});
