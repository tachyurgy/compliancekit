import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: documents upload smoke.
 *
 * The full upload flow is S3 presigned PUT → POST /api/documents/{id}/finalize.
 * Without real AWS creds the presign step returns an error, so we only verify:
 *   - /api/documents list is reachable and returns JSON,
 *   - the documents page loads or redirects without crashing.
 * The deeper end-to-end upload is gated behind real S3 access.
 */

test.describe('LIVE documents-upload', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('documents list endpoint returns an array for a fresh provider', async ({ page }) => {
    await loginAs(page);
    const list = await page.request.get(`${BACKEND_URL}/api/documents`);
    expect(list.status()).toBe(200);
    const body = await list.json();
    expect(Array.isArray(body)).toBeTruthy();
    expect(body.length).toBe(0);
  });

  test('documents page does not white-screen', async ({ page }) => {
    await loginAs(page);
    await page.goto('/documents');
    await expect(page).toHaveURL(/\/(documents|onboarding|login)/);
    const body = await page.locator('body').textContent();
    expect(body).not.toMatch(/TypeError|ReferenceError/i);
  });
});
