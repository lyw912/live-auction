import { defineConfig, devices } from '@playwright/test';

if (!process.env.H5_PORT || !process.env.PC_PORT) {
  const portOffset = Number(process.env.PLAYWRIGHT_PORT_OFFSET ?? (process.pid % 1000));
  process.env.H5_PORT = String(20_000 + portOffset * 2);
  process.env.PC_PORT = String(Number(process.env.H5_PORT) + 1);
}

const h5Port = Number(process.env.H5_PORT);
const pcPort = Number(process.env.PC_PORT);
const h5URL = `http://127.0.0.1:${h5Port}`;
const pcURL = `http://127.0.0.1:${pcPort}`;

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 30_000,
  expect: {
    timeout: 5_000
  },
  webServer: [
    {
      command: `node node_modules/vite/bin/vite.js frontend/mobile-h5 --host 127.0.0.1 --port ${h5Port} --strictPort`,
      url: h5URL,
      reuseExistingServer: true
    },
    {
      command: `node node_modules/vite/bin/vite.js frontend/pc-console --host 127.0.0.1 --port ${pcPort} --strictPort`,
      url: pcURL,
      reuseExistingServer: true
    }
  ],
  projects: [
    {
      name: 'mobile-h5',
      use: {
        ...devices['Pixel 5'],
        baseURL: h5URL
      },
      testMatch: /(?:mobile-h5(?:-media-isolation)?|atmosphere-engine)\.spec\.ts/
    },
    {
      name: 'pc-console',
      use: {
        ...devices['Desktop Chrome'],
        baseURL: pcURL,
        viewport: { width: 1440, height: 900 }
      },
      testMatch: /pc-console\.spec\.ts/
    },
    {
      name: 'visual-mobile-h5',
      use: {
        ...devices['Pixel 5'],
        baseURL: h5URL
      },
      testMatch: /visual-regression\.spec\.ts/,
      grep: /@visual-h5/
    },
    {
      name: 'visual-pc-console',
      use: {
        ...devices['Desktop Chrome'],
        baseURL: pcURL,
        viewport: { width: 1440, height: 900 }
      },
      testMatch: /visual-regression\.spec\.ts/,
      grep: /@visual-pc/
    }
  ]
});
