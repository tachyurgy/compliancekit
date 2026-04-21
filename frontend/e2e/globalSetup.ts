/**
 * globalSetup — no-op in the LIVE harness.
 *
 * Why: Playwright's ordering between globalSetup and webServer is not strictly
 * guaranteed (observed: webServer can START before globalSetup finishes). To
 * avoid that race, the DB reset + migration step is embedded inside the
 * webServer shell command in playwright.config.ts. This file remains as the
 * wiring point for future global test setup needs — e.g. seeding a fixtures
 * directory, warming an OCR cache, etc.
 */
export default async function globalSetup(): Promise<void> {
  // Intentionally empty — see webServer command in playwright.config.ts.
}
