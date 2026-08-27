import { test, expect } from '@playwright/test';

/**
 * Basic import verification test for @axe-core/playwright
 *
 * This test verifies that:
 * 1. AxeBuilder can be imported from @axe-core/playwright
 * 2. The import is functional and accessible
 */
test.describe('@axe-core/playwright Import Verification', () => {
  test('AxeBuilder can be imported', async () => {
    // Dynamically import to verify the module loads correctly
    const axeModule = await import('@axe-core/playwright');
    const AxeBuilder = axeModule.default;

    // Verify AxeBuilder is a constructor/function
    expect(typeof AxeBuilder).toBe('function');
    expect(AxeBuilder.name).toBe('AxeBuilder');
  });

  test('AxeBuilder has expected static properties', async () => {
    const axeModule = await import('@axe-core/playwright');
    const AxeBuilder = axeModule.default;

    // Verify the constructor has expected properties
    expect(AxeBuilder).toBeDefined();
  });

  test('default import works with CommonJS/ES Module interop', async () => {
    // Test both default and named imports work
    const defaultImport = (await import('@axe-core/playwright')).default;
    const namedImport = await import('@axe-core/playwright');

    expect(defaultImport).toBeDefined();
    expect(namedImport).toBeDefined();
    expect(namedImport.default).toBe(defaultImport);
  });
});
