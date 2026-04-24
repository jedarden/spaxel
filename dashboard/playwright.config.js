const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: './tests',
  timeout: 30000,
  retries: 0,
  use: {
    baseURL: 'http://localhost:3210',
    headless: true,
  },
  webServer: {
    command: 'npx http-server . -p 3210 --silent',
    port: 3210,
    reuseExistingServer: true,
  },
});
