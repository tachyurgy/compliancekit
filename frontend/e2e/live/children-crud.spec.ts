import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: children CRUD via API (UI requires onboarding_complete).
 *
 * The /children page is behind RequireOnboarded. Until the onboarding endpoint
 * is wired up, we exercise the backend CRUD endpoints with the real session
 * cookie — which is the ground-truth contract tests need to cover.
 */

test.describe('LIVE children-crud', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('create + list + delete a child', async ({ page }) => {
    await loginAs(page);

    // Create
    const create = await page.request.post(`${BACKEND_URL}/api/children`, {
      data: {
        first_name: 'Ada',
        last_name: 'Lovelace',
        date_of_birth: '2022-01-15T00:00:00Z',
      },
      headers: { 'Content-Type': 'application/json' },
    });
    expect(create.status(), 'create child').toBe(201);
    const created = await create.json();
    expect(created.first_name).toBe('Ada');

    // List
    const list = await page.request.get(`${BACKEND_URL}/api/children`);
    expect(list.status()).toBe(200);
    const children = await list.json();
    expect(Array.isArray(children)).toBeTruthy();
    expect(children.find((c: any) => c.first_name === 'Ada')).toBeDefined();

    // Delete (soft)
    const del = await page.request.delete(
      `${BACKEND_URL}/api/children/${created.id}`,
    );
    expect([200, 204]).toContain(del.status());
  });

  test('children page loads or redirects without crashing', async ({ page }) => {
    await loginAs(page);
    await page.goto('/children');
    await expect(page).toHaveURL(/\/(children|onboarding|login)/);
  });
});
