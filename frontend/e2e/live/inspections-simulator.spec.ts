import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: inspection simulator — start, answer items, finalize.
 */

test.describe('LIVE inspections-simulator', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('start + respond + finalize an inspection', async ({ page }) => {
    await loginAs(page, { state: 'CA' });

    // Start a mock inspection. No body required; state is inferred from the
    // provider. Response is { run, checklist: { items }, responses }.
    const start = await page.request.post(`${BACKEND_URL}/api/inspections`, {
      data: {},
      headers: { 'Content-Type': 'application/json' },
    });
    expect([200, 201]).toContain(start.status());
    const detail = await start.json();
    const runID: string = detail.run.id;
    const items: any[] = detail.checklist?.items ?? [];
    expect(items.length).toBeGreaterThan(0);

    // Answer the first few items (3 pass, 2 fail).
    for (let i = 0; i < Math.min(items.length, 5); i++) {
      const answer = i < 3 ? 'pass' : 'fail';
      const patch = await page.request.patch(
        `${BACKEND_URL}/api/inspections/${runID}/items/${items[i].id}`,
        {
          data: { answer, note: answer === 'fail' ? 'seeded by e2e' : '' },
          headers: { 'Content-Type': 'application/json' },
        },
      );
      expect([200, 204]).toContain(patch.status());
    }

    // Finalize.
    const fin = await page.request.post(
      `${BACKEND_URL}/api/inspections/${runID}/finalize`,
      { data: {}, headers: { 'Content-Type': 'application/json' } },
    );
    expect([200, 201]).toContain(fin.status());
  });
});
