import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: admin-facing parent + staff portal link generation.
 *
 * Creates a child and a staff member, then hits the portal-link endpoints
 * and asserts the response shape. The URL points at /portal/parent or
 * /portal/staff and contains a fresh magic-link token.
 */

test.describe('LIVE portal-link', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('POST /api/children/{id}/portal-link mints a parent invite URL', async ({ page }) => {
    await loginAs(page);

    const child = await page.request.post(`${BACKEND_URL}/api/children`, {
      data: {
        first_name: 'Leo',
        last_name: 'Lionheart',
        date_of_birth: '2022-03-10T00:00:00Z',
        parent_email: 'leoparent@example.com',
      },
      headers: { 'Content-Type': 'application/json' },
    });
    expect(child.status(), await child.text()).toBe(201);
    const childBody = await child.json();
    const childId = childBody.id as string;

    const link = await page.request.post(
      `${BACKEND_URL}/api/children/${childId}/portal-link`,
      { data: {}, headers: { 'Content-Type': 'application/json' } },
    );
    expect(link.status(), await link.text()).toBe(200);
    const body = await link.json();
    expect(body.subject_id).toBe(childId);
    expect(body.subject_kind).toBe('child');
    expect(body.url).toMatch(/\/portal\/parent\?t=/);
    expect(typeof body.expires_at).toBe('string');
  });

  test('POST /api/staff/{id}/portal-link mints a staff invite URL', async ({ page }) => {
    await loginAs(page);

    const staff = await page.request.post(`${BACKEND_URL}/api/staff`, {
      data: {
        first_name: 'Morgan',
        last_name: 'Teacher',
        email: 'morgan@example.com',
        role: 'lead_teacher',
      },
      headers: { 'Content-Type': 'application/json' },
    });
    expect(staff.status(), await staff.text()).toBe(201);
    const staffBody = await staff.json();
    const staffId = staffBody.id as string;

    const link = await page.request.post(
      `${BACKEND_URL}/api/staff/${staffId}/portal-link`,
      { data: {}, headers: { 'Content-Type': 'application/json' } },
    );
    expect(link.status(), await link.text()).toBe(200);
    const body = await link.json();
    expect(body.subject_id).toBe(staffId);
    expect(body.subject_kind).toBe('staff');
    expect(body.url).toMatch(/\/portal\/staff\?t=/);
  });

  test('portal-link is admin-only (unknown child id returns 404 for admin)', async ({ page }) => {
    await loginAs(page);
    const resp = await page.request.post(
      `${BACKEND_URL}/api/children/does-not-exist/portal-link`,
      { data: {}, headers: { 'Content-Type': 'application/json' } },
    );
    expect(resp.status()).toBe(404);
  });
});
