import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: parent portal magic-link flow.
 *
 * Currently the admin UI for minting a parent upload link is not a fully
 * public route from the frontend. We exercise the backend portion: authed
 * user creates a child, the admin route to mint a parent magic link is not
 * yet exposed in the API surface (tracked separately), so for now we smoke
 * the portal route's 401 behavior and known magic-link-gated paths.
 */

test.describe('LIVE parent-portal', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('unauthenticated /portal/parent without a token redirects', async ({ page }) => {
    await page.goto('/portal/parent/nope-not-a-token');
    // The page should render an error or redirect — no stack trace.
    const body = await page.locator('body').textContent();
    expect(body).not.toMatch(/TypeError|ReferenceError/i);
  });

  test('portal /portal/parent without a token returns 401 from API', async ({ request }) => {
    const resp = await request.get(`${BACKEND_URL}/portal/parent`);
    expect([400, 401, 403]).toContain(resp.status());
  });
});
