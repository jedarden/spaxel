const AxeBuilder = require('@axe-core/playwright').default;

/**
 * Run an axe-core accessibility scan with WCAG 2.1 AA tags
 *
 * @param {Page} page - Playwright Page object to scan
 * @param {string} context - Optional context selector to limit the scan scope
 * @returns {Promise<Object>} Axe scan results with violations, passes, and incomplete results
 */
async function scanAccessibility(page, context) {
  const builder = new AxeBuilder({ page });

  // Configure with WCAG 2.1 AA tags
  builder.withTags(['wcag2a', 'wcag2aa']);

  // Limit scan scope if context is provided
  if (context) {
    builder.include(context);
  }

  return await builder.analyze();
}

/**
 * Format accessibility violations for error reporting
 *
 * @param {Array} violations - Array of axe violation results
 * @returns {string} Formatted string with violation details
 */
function formatViolations(violations) {
  return violations
    .map((v) => `  ${v.id}: ${v.nodes.length} node(s) — ${v.description}`)
    .join('\n');
}

/**
 * Assert that a page has no accessibility violations
 *
 * @param {Page} page - Playwright Page object to scan
 * @param {string} pageName - Name of the page for error reporting
 * @param {string} context - Optional context selector to limit the scan scope
 */
async function expectNoAccessibilityViolations(page, pageName, context) {
  const results = await scanAccessibility(page, context);
  const violations = results.violations;

  if (violations.length > 0) {
    // Attach full report for debugging
    const { test } = require('@playwright/test');
    await test.info().attach('axe-violations', {
      body: JSON.stringify(violations, null, 2),
      contentType: 'application/json',
    });
  }

  const { expect } = require('@playwright/test');
  await expect(
    violations,
    `${violations.length} WCAG AA violation(s) on ${pageName}:\n${
      violations.map((v) => `  ${v.id}: ${v.description}`).join('\n')
    }`
  ).toEqual([]);
}

module.exports = {
  scanAccessibility,
  formatViolations,
  expectNoAccessibilityViolations,
};
