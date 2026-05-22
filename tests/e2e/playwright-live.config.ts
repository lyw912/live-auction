import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: '.',
  workers: 1,
  timeout: 45_000,
  expect: {
    timeout: 10_000
  },
  projects: [
    {
      name: 'mobile-h5-live',
      use: {
        ...devices['Pixel 5'],
        baseURL: process.env.LIVE_AUCTION_H5_URL || 'http://127.0.0.1:5175'
      },
      testMatch: /mobile-h5-live\.spec\.ts/
    },
    {
      name: 'pc-console-live',
      use: {
        ...devices['Desktop Chrome'],
        baseURL: process.env.LIVE_AUCTION_PC_URL || 'http://127.0.0.1:5177'
      },
      testMatch: /pc-console-live\.spec\.ts/
    }
  ]
});
