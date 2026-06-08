import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';

const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/pc-cookie-regression';
const pcURL = process.env.LIVE_AUCTION_PC_URL || 'http://127.0.0.1:5277';
const h5URL = process.env.LIVE_AUCTION_H5_URL || 'http://127.0.0.1:5276/rooms/room_main';

async function main() {
  await fs.mkdir(outDir, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 950 } });
  const page = await context.newPage();
  const network = [];
  page.on('response', (response) => {
    const url = response.url();
    if (url.includes('/api/')) network.push(`${response.status()} ${response.request().method()} ${url}`);
  });

  await page.goto(h5URL, { waitUntil: 'networkidle' });
  await page.evaluate(async () => {
    await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ account: 'user' })
    });
  });
  await page.screenshot({ path: `${outDir}/01-h5-user-session.png`, fullPage: true });

  await page.goto(pcURL, { waitUntil: 'networkidle' });
  await page.getByTestId('pc-workbench-status').getByText(/工作台已刷新|工作台已就绪/).waitFor({ timeout: 20_000 });
  await page.getByRole('heading', { name: '开播中控' }).waitFor({ timeout: 10_000 });
  await page.screenshot({ path: `${outDir}/02-pc-recovers-host-session.png`, fullPage: true });

  await fs.writeFile(`${outDir}/network.json`, JSON.stringify(network, null, 2));
  await browser.close();
  console.log(JSON.stringify({ outDir, requests: network.length }, null, 2));
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
