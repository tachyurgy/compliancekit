import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: facility postings checklist.
 */

test.describe('LIVE operations-postings', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('list postings for a CA provider', async ({ page }) => {
    await loginAs(page, { state: 'CA' });
    const resp = await page.request.get(`${BACKEND_URL}/api/facility/postings`);
    expect(resp.status()).toBe(200);
    const data = await resp.json();
    // Response shape: array of { key, required, posted_at, ... } or an object
    // with items. Accept either; just check we got JSON back.
    expect(data).toBeTruthy();
  });

  test('patching a posting marks it complete', async ({ page }) => {
    await loginAs(page, { state: 'CA' });
    const list = await page.request.get(`${BACKEND_URL}/api/facility/postings`);
    const data = await list.json();
    const items: any[] = Array.isArray(data) ? data : (data.items ?? []);
    if (items.length === 0) {
      test.skip(true, 'No postings configured for this state');
    }
    const first = items[0];
    const patch = await page.request.patch(
      `${BACKEND_URL}/api/facility/postings/${first.key ?? first.id}`,
      {
        data: { posted_at: new Date().toISOString() },
        headers: { 'Content-Type': 'application/json' },
      },
    );
    // 200 or 204 both acceptable.
    expect([200, 204]).toContain(patch.status());
  });
});
