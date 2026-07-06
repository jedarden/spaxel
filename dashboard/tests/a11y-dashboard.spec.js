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
      await pg.goto(page.path, { waitUntil: 'load' });

      // Use the shared accessibility helper
      await expectNoAccessibilityViolations(pg, `${page.name} dashboard page`);

      await context.close();
    });
  });
}
