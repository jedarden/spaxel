const { test } = require('@playwright/test');
const { expectNoAccessibilityViolations } = require('./accessibility/helper');
const { corePages: pages } = require('./accessibility/pages');

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
