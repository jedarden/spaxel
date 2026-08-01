const { test } = require('@playwright/test');
const { expectNoAccessibilityViolations } = require('./accessibility/helper');

const dashboardPages = [
  { name: 'ambient', path: '/ambient.html' },
  { name: 'live', path: '/live.html' },
  { name: 'simple', path: '/simple.html' },
  { name: 'simulator', path: '/simulator.html' },
];

for (const page of dashboardPages) {
  test.describe(`${page.name} page`, () => {
    test('has no WCAG AA violations', async ({ browser }) => {
      const context = await browser.newContext();
      const pg = await context.newPage();

      // This suite runs against a static file server with no mothership API
      // behind it. Every fetch fails, and ambient.html's bootstrap treats a
      // failed /api/auth/status check as "auth required" (correct fail-closed
      // behavior for real deployments — see js/auth.js's checkAuthStatus) and
      // redirects to '/', which tears down axe's execution context mid-scan.
      // Mock the endpoint to report the "no PIN configured" case that
      // ambient.html's own bootstrap comment names as its no-backend carve-out
      // (hardware-free sim), rather than weakening the app's real fail-closed
      // default.
      await pg.route('**/api/auth/status', (route) =>
        route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ pin_configured: false }),
        })
      );

      await pg.goto(page.path, { waitUntil: 'load' });

      // Use the shared accessibility helper
      await expectNoAccessibilityViolations(pg, `${page.name} dashboard page`);

      await context.close();
    });
  });
}
