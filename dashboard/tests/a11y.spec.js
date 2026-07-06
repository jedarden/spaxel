const { test } = require('@playwright/test');
const { expectNoAccessibilityViolations } = require('./accessibility/helper');

const pages = [
  { name: 'index', path: '/index.html' },
  { name: 'live', path: '/live.html' },
  { name: 'fleet', path: '/fleet.html' },
  { name: 'setup', path: '/setup.html' },
  { name: 'integrations', path: '/integrations.html' },
];

for (const page of pages) {
  test.describe(`${page.name} page`, () => {
    test('has no WCAG AA violations', async ({ browser }) => {
      const context = await browser.newContext();
      const pg = await context.newPage();
      await pg.goto(page.path, { waitUntil: 'load' });

      // Use the shared accessibility helper
      await expectNoAccessibilityViolations(pg, page.path);

      await context.close();
    });
  });
}
