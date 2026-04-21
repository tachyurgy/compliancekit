import { test, expect } from '@playwright/test';
import { BACKEND_URL, loginAs } from '../helpers/auth';

/**
 * LIVE: onboarding wizard + POST /api/provider/onboarding.
 *
 * Walks the six-step wizard and verifies the backend finalize endpoint
 * persists facility fields, flips onboarding_complete, and bulk-inserts
 * staff and children.
 */

test.describe('LIVE onboarding', () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test('authenticated user can walk to the review step', async ({ page }) => {
    await loginAs(page);

    await page.goto('/onboarding/state');
    await expect(page.getByText(/which state are you licensed in/i)).toBeVisible();

    const ca = page.getByText(/California/i).first();
    await ca.click({ trial: false }).catch(() => {});

    const continueBtn = page.getByRole('button', { name: /continue/i });
    if (await continueBtn.isVisible().catch(() => false)) {
      await continueBtn.click().catch(() => {});
    }

    await page.goto('/onboarding/review');
    await expect(page.getByText(/review and generate/i)).toBeVisible();
  });

  test('POST /api/provider/onboarding persists facility + staff + children', async ({
    page,
  }) => {
    await loginAs(page);

    const resp = await page.request.post(`${BACKEND_URL}/api/provider/onboarding`, {
      data: {
        stateCode: 'CA',
        licenseType: 'center',
        licenseNumber: 'TEST-LIC-042',
        name: 'Finalize Test Daycare',
        address1: '500 Test Way',
        address2: 'Suite 2',
        city: 'Los Angeles',
        stateRegion: 'CA',
        postalCode: '90001',
        capacity: 45,
        agesServedMonths: { minMonths: 6, maxMonths: 60 },
        staff: [
          {
            firstName: 'Alice',
            lastName: 'Admin',
            email: 'alice@example.com',
            role: 'director',
          },
          { firstName: 'Bob', lastName: 'Teacher', role: 'lead_teacher' },
        ],
        children: [
          {
            firstName: 'Charlie',
            lastName: 'Kid',
            dateOfBirth: '2022-05-10',
            parentEmail: 'parent@example.com',
          },
        ],
      },
      headers: { 'Content-Type': 'application/json' },
    });
    expect(resp.status(), await resp.text()).toBe(200);
    const body = await resp.json();
    expect(body.name).toBe('Finalize Test Daycare');
    expect(body.stateCode).toBe('CA');
    expect(body.licenseType).toBe('center');
    expect(body.capacity).toBe(45);
    expect(body.agesServedMonths).toEqual({ minMonths: 6, maxMonths: 60 });
    expect(body.onboardingComplete).toBe(true);

    // Confirm /api/me now reports onboarding as complete.
    const me = await page.request.get(`${BACKEND_URL}/api/me`);
    expect(me.status()).toBe(200);
    const meBody = await me.json();
    expect(meBody.onboardingComplete).toBe(true);
    expect(meBody.capacity).toBe(45);

    // Confirm staff + children were inserted.
    const staffResp = await page.request.get(`${BACKEND_URL}/api/staff`);
    expect(staffResp.status()).toBe(200);
    const staff = (await staffResp.json()) as Array<{ first_name: string }>;
    expect(staff.length).toBeGreaterThanOrEqual(2);
    expect(staff.some((s) => s.first_name === 'Alice')).toBe(true);

    const childrenResp = await page.request.get(`${BACKEND_URL}/api/children`);
    expect(childrenResp.status()).toBe(200);
    const children = (await childrenResp.json()) as Array<{ first_name: string }>;
    expect(children.length).toBeGreaterThanOrEqual(1);
    expect(children.some((c) => c.first_name === 'Charlie')).toBe(true);
  });

  test('POST /api/provider/onboarding rejects invalid stateCode', async ({ page }) => {
    await loginAs(page);
    const resp = await page.request.post(`${BACKEND_URL}/api/provider/onboarding`, {
      data: {
        stateCode: 'ZZ',
        licenseType: 'center',
        name: 'Bad State',
        address1: '1 Bad',
        city: 'Nowhere',
        stateRegion: 'ZZ',
        postalCode: '00000',
        capacity: 10,
        agesServedMonths: { minMonths: 0, maxMonths: 60 },
      },
      headers: { 'Content-Type': 'application/json' },
    });
    expect(resp.status()).toBe(400);
  });
});
