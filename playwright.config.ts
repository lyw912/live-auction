import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 30_000,
  expect: {
    timeout: 5_000
  },
  webServer: [
    {
      command: 'node node_modules/vite/bin/vite.js frontend/mobile-h5 --host 127.0.0.1 --port 5173',
      url: 'http://127.0.0.1:5173',
      reuseExistingServer: true
    },
    {
      command: 'node node_modules/vite/bin/vite.js frontend/pc-console --host 127.0.0.1 --port 5174',
      url: 'http://127.0.0.1:5174',
      reuseExistingServer: true
    }
  ],
  projects: [
    {
      name: 'mobile-h5',
      use: {
        ...devices['Pixel 5'],
        baseURL: 'http://127.0.0.1:5173'
      },
      testMatch: /mobile-h5\.spec\.ts/
    },
    {
      name: 'pc-console',
      use: {
        ...devices['Desktop Chrome'],
        baseURL: 'http://127.0.0.1:5174',
        viewport: { width: 1440, height: 900 }
      },
      testMatch: /pc-console\.spec\.ts/
    }
  ]
});
