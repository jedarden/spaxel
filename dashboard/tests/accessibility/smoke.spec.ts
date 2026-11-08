import { test, expect } from '@playwright/test';
import { scanAccessibility, expectNoAccessibilityViolations } from './helper';

test.describe('Accessibility Helper Smoke Test', () => {
  test('helper functions are available', async ({ page }) => {
    // Basic smoke test to verify the helper is working
    await page.goto('/index.html', { waitUntil: 'load' });

    // Test the scanAccessibility function
    const results = await scanAccessibility(page);
    expect(results).toHaveProperty('violations');
    expect(results).toHaveProperty('passes');
    expect(Array.isArray(results.violations)).toBe(true);
    expect(Array.isArray(results.passes)).toBe(true);

    const scannedRules = [
      ...results.passes,
      ...results.violations,
      ...results.incomplete,
      ...results.inapplicable,
    ].map((result) => result.id);
    expect(scannedRules).toEqual(
      expect.arrayContaining([
        'autocomplete-valid',
        'avoid-inline-spacing',
      ])
    );
  });

  test('index page has no WCAG AA violations (using helper)', async ({ page }) => {
    await page.goto('/index.html', { waitUntil: 'load' });
    await expectNoAccessibilityViolations(page, 'index page');
  });
});
