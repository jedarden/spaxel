import { Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/**
 * Run an axe-core accessibility scan with WCAG 2.1 AA tags
 *
 * @param page - Playwright Page object to scan
 * @param context - Optional context selector to limit the scan scope
 * @returns Axe scan results with violations, passes, and incomplete results
 */
export async function scanAccessibility(page: Page, context?: string) {
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
 * @param violations - Array of axe violation results
 * @returns Formatted string with violation details
 */
export function formatViolations(violations: any[]): string {
  return violations
    .map((v) => `  ${v.id}: ${v.nodes.length} node(s) — ${v.description}`)
    .join('\n');
}

/**
 * Assert that a page has no accessibility violations
 *
 * @param page - Playwright Page object to scan
 * @param pageName - Name of the page for error reporting
 * @param context - Optional context selector to limit the scan scope
 */
export async function expectNoAccessibilityViolations(
  page: Page,
  pageName: string,
  context?: string
): Promise<void> {
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
