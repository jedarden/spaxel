const { test, expect } = require('@playwright/test');
const { expectNoAccessibilityViolations } = require('./accessibility/helper');

/**
 * Accessibility tests for the Spaxel Onboarding Wizard.
 *
 * These tests walk through the onboarding UI flow and scan for WCAG 2.1 AA
 * violations at each step. Tests mock hardware-dependent APIs (Web Serial,
 * WebSocket) to focus on UI accessibility.
 */

test.describe('Onboarding Wizard Accessibility', () => {
  test.beforeEach(async ({ page, context }) => {
    // Mock Web Serial API to prevent hardware requirement errors
    await context.addInitScript(() => {
      if (!window.navigator.serial) {
        window.navigator.serial = {
          requestPort: async () => ({
            open: async () => {},
            close: async () => {},
            writable: { getWriter: () => ({ write: async () => {}, close: async () => {} }) },
            readable: { pipeTo: async () => {}, getReader: () => ({ read: async () => ({ done: true }) }) },
          }),
          getPorts: async () => [],
        };
      }
    });

    // Mock WebSocket to prevent network connection errors
    await context.addInitScript(() => {
      window.WebSocket = class MockWebSocket {
        constructor(url) {
          this.url = url;
          this.readyState = 1;
          setTimeout(() => {
            if (this.onopen) this.onopen({});
          }, 0);
        }
        close() {}
        send() {}
      };
    });

    // Mock crypto.randomUUID if not available
    await context.addInitScript(() => {
      if (!window.crypto?.randomUUID) {
        window.crypto = window.crypto || {};
        window.crypto.randomUUID = () => 'test-uuid-' + Math.random().toString(36).substr(2, 9);
      }
    });

    // Mock fetch API for provisioning endpoints
    await context.addInitScript(() => {
      const originalFetch = window.fetch;
      window.fetch = async (url, options) => {
        if (url.includes('/api/provision') || url.includes('/api/nodes') || url.includes('/api/firmware')) {
          return {
            ok: true,
            json: async () => [],
            status: 200,
          };
        }
        return originalFetch(url, options);
      };
    });
  });

  test('onboarding wizard can be started', async ({ page }) => {
    await page.goto('/live.html', { waitUntil: 'load' });

    // Start the onboarding wizard
    await page.evaluate(() => {
      if (window.SpaxelOnboard) {
        window.SpaxelOnboard.start();
      }
    });

    // Wait for wizard overlay to appear
    await page.waitForSelector('#wizard-overlay', { timeout: 5000 });

    // Verify wizard structure exists
    await expect(page.locator('#wizard-card')).toBeVisible();
    await expect(page.locator('#wizard-steps')).toBeVisible();
    await expect(page.locator('#wizard-content')).toBeVisible();
  });

  test('connect device step has no WCAG AA violations', async ({ page }) => {
    await page.goto('/live.html', { waitUntil: 'load' });

    // Start the wizard and wait for it to navigate past browser_check (auto-advance)
    await page.evaluate(() => {
      if (window.SpaxelOnboard) {
        window.SpaxelOnboard.start();
      }
    });

    // Wait for connect device step
    await page.waitForSelector('#wizard-overlay', { timeout: 5000 });
    await page.waitForTimeout(500); // Allow auto-advance from browser_check

    // Verify we're on connect device step by checking for key content
    await expect(page.locator('#wizard-content')).toContainText('Connect Your ESP32-S3', { timeout: 5000 });

    // Scan for accessibility violations
    await expectNoAccessibilityViolations(page, 'onboarding connect device step');
  });

  test('WiFi configuration step has no WCAG AA violations', async ({ page }) => {
    await page.goto('/live.html', { waitUntil: 'load' });

    // Start wizard and programmatically advance to WiFi step
    await page.evaluate(() => {
      if (window.SpaxelOnboard) {
        // Set state to go directly to provision_wifi step (step index 3)
        const state = window.SpaxelOnboard._state;
        state.currentStepIndex = 2; // provision_wifi
        state.port = null;
        state.nodeMAC = null;
        state.knownMACs = [];
        window.SpaxelOnboard.start();
      }
    });

    // Wait for wizard and WiFi step
    await page.waitForSelector('#wizard-overlay', { timeout: 5000 });
    await expect(page.locator('#wizard-content')).toContainText('Configure WiFi', { timeout: 5000 });

    // Scan for accessibility violations
    await expectNoAccessibilityViolations(page, 'onboarding WiFi configuration step');
  });

  test('node placement step has no WCAG AA violations', async ({ page }) => {
    await page.goto('/live.html', { waitUntil: 'load' });

    // Start wizard and advance to placement step
    await page.evaluate(() => {
      if (window.SpaxelOnboard) {
        const state = window.SpaxelOnboard._state;
        state.currentStepIndex = 6; // placement
        state.nodeMAC = 'AA:BB:CC:DD:EE:FF';
        state.knownMACs = ['AA:BB:CC:DD:EE:FF'];
        state.wifiSSID = 'TestWiFi';
        state.wifiPass = 'testpass';
        window.SpaxelOnboard.start();
      }
    });

    // Wait for wizard and placement step
    await page.waitForSelector('#wizard-overlay', { timeout: 5000 });
    await expect(page.locator('#wizard-content')).toContainText('Node Placement', { timeout: 5000 });

    // Scan for accessibility violations
    await expectNoAccessibilityViolations(page, 'onboarding node placement step');
  });

  test('complete step has no WCAG AA violations', async ({ page }) => {
    await page.goto('/live.html', { waitUntil: 'load' });

    // Start wizard and advance to complete step
    await page.evaluate(() => {
      if (window.SpaxelOnboard) {
        const state = window.SpaxelOnboard._state;
        state.currentStepIndex = 7; // complete
        state.nodeMAC = 'AA:BB:CC:DD:EE:FF';
        state.knownMACs = ['AA:BB:CC:DD:EE:FF'];
        state.wifiSSID = 'TestWiFi';
        state.wifiPass = 'testpass';
        window.SpaxelOnboard.start();
      }
    });

    // Wait for wizard and complete step
    await page.waitForSelector('#wizard-overlay', { timeout: 5000 });
    await expect(page.locator('#wizard-content')).toContainText('Setup Complete', { timeout: 5000 });

    // Scan for accessibility violations
    await expectNoAccessibilityViolations(page, 'onboarding complete step');
  });

  test('re-provision mode connect step has no WCAG AA violations', async ({ page }) => {
    await page.goto('/live.html', { waitUntil: 'load' });

    // Start wizard in re-provision mode
    await page.evaluate(() => {
      if (window.SpaxelOnboard) {
        window.SpaxelOnboard.reprove('AA:BB:CC:DD:EE:FF');
      }
    });

    // Wait for wizard
    await page.waitForSelector('#wizard-overlay', { timeout: 5000 });

    // Verify re-provision banner is shown
    await expect(page.locator('.wizard-reprove-banner')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.wizard-reprove-banner')).toContainText('Re-provisioning AA:BB:CC:DD:EE:FF');

    // Scan for accessibility violations
    await expectNoAccessibilityViolations(page, 'onboarding re-provision mode connect step');
  });

  test('wizard step indicator has no WCAG AA violations', async ({ page }) => {
    await page.goto('/live.html', { waitUntil: 'load' });

    // Start wizard
    await page.evaluate(() => {
      if (window.SpaxelOnboard) {
        window.SpaxelOnboard.start();
      }
    });

    // Wait for wizard
    await page.waitForSelector('#wizard-overlay', { timeout: 5000 });
    await page.waitForSelector('#wizard-steps', { timeout: 5000 });

    // Verify step dots are rendered (8 steps total)
    const stepDots = page.locator('.wizard-step-dot');
    await expect(stepDots).toHaveCount(8);

    // Scan for accessibility violations
    await expectNoAccessibilityViolations(page, 'onboarding step indicator');
  });

  test('wizard close button has no WCAG AA violations', async ({ page }) => {
    await page.goto('/live.html', { waitUntil: 'load' });

    // Start wizard
    await page.evaluate(() => {
      if (window.SpaxelOnboard) {
        window.SpaxelOnboard.start();
      }
    });

    // Wait for wizard and close button
    await page.waitForSelector('#wizard-overlay', { timeout: 5000 });
    await page.waitForSelector('#wizard-close-btn', { timeout: 5000 });

    // Verify close button exists and is accessible
    const closeBtn = page.locator('#wizard-close-btn');
    await expect(closeBtn).toBeVisible();
    await expect(closeBtn).toHaveAttribute('title', 'Close');
    await expect(closeBtn).toHaveText('×');

    // Scan for accessibility violations
    await expectNoAccessibilityViolations(page, 'onboarding close button');
  });

  test('wizard error states have no WCAG AA violations', async ({ page }) => {
    await page.goto('/live.html', { waitUntil: 'load' });

    // Start wizard and inject error state
    await page.evaluate(() => {
      if (window.SpaxelOnboard) {
        const state = window.SpaxelOnboard._state;
        state.currentStepIndex = 1; // connect_device
        window.SpaxelOnboard.start();

        // Manually inject an error message
        setTimeout(() => {
          const errorEl = document.getElementById('connect-error');
          if (errorEl) {
            errorEl.style.display = 'block';
            errorEl.textContent = 'No device detected. Did you hold the BOOT button while plugging in?';
          }
        }, 100);
      }
    });

    // Wait for wizard
    await page.waitForSelector('#wizard-overlay', { timeout: 5000 });

    // Wait for error to be injected
    await page.waitForTimeout(200);

    // Verify error element is visible
    const errorEl = page.locator('#connect-error');
    await expect(errorEl).toBeVisible();

    // Scan for accessibility violations
    await expectNoAccessibilityViolations(page, 'onboarding error state');
  });
});
