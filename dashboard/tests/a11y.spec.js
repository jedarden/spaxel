const { test, expect } = require('@playwright/test');
const AxeBuilder = require('@axe-core/playwright').default;

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

      const results = await new AxeBuilder({ page: pg })
        .withTags(['wcag2a', 'wcag2aa'])
        .analyze();

      const violations = results.violations;
      if (violations.length > 0) {
        const details = violations
          .map((v) => `  ${v.id}: ${v.nodes.length} node(s) — ${v.description}`)
          .join('\n');
        // Attach full report for debugging
        await test.info().attach('axe-violations', {
          body: JSON.stringify(violations, null, 2),
          contentType: 'application/json',
        });
      }

      await expect(violations, `${violations.length} WCAG AA violation(s) on ${page.path}:\n${
        violations.map((v) => `  ${v.id}: ${v.description}`).join('\n')
      }`).toEqual([]);

      await context.close();
    });
  });
}
