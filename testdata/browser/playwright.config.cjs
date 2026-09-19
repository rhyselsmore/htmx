const { defineConfig } = require('@playwright/test');

// One Go fixture process serves all workers. Each test gets a fresh page;
// polling cases additionally use unique tokens for their server-side counters.
module.exports = defineConfig({
  globalSetup: require.resolve('./setup.cjs'),
  testDir: '.',
  testMatch: '*.spec.cjs',
  fullyParallel: true,
  timeout: 30000,
  retries: 0,
  workers: 3,
  use: { headless: true, trace: 'retain-on-failure' },
  projects: ['chromium', 'firefox', 'webkit'].map((browserName) => ({
    name: browserName,
    use: { browserName },
  })),
});
