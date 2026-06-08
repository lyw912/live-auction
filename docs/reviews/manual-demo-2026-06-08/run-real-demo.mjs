import { chromium } from '@playwright/test';

const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence';
const pcURL = process.env.LIVE_AUCTION_PC_URL || 'http://127.0.0.1:5277';
const h5URL = process.env.LIVE_AUCTION_H5_URL || 'http://127.0.0.1:5276/rooms/room_main';

async function shot(page, name, fullPage = true) {
  await page.screenshot({ path: `${outDir}/${name}.png`, fullPage });
}

async function clickVisible(page, name, timeout = 10_000) {
  const button = page.getByRole('button', { name });
  await button.waitFor({ state: 'visible', timeout });
  await button.click();
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const pcContext = await browser.newContext({ viewport: { width: 1440, height: 950 } });
  const h5Context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true });
  const pc = await pcContext.newPage();
  const h5 = await h5Context.newPage();

  const network = [];
  for (const page of [pc, h5]) {
    page.on('response', (response) => {
      const url = response.url();
      if (url.includes('/api/')) network.push(`${response.status()} ${response.request().method()} ${url}`);
    });
  }

  await pc.goto(pcURL, { waitUntil: 'networkidle' });
  await pc.getByRole('heading', { name: '开播中控' }).waitFor({ timeout: 15_000 });
  await shot(pc, '01-pc-initial');

  await clickVisible(pc, '重置演示环境');
  await clickVisible(pc, '确认重置');
  await pc.waitForTimeout(1500);
  await shot(pc, '02-pc-reset');

  await h5.goto(h5URL, { waitUntil: 'networkidle' });
  await h5.getByRole('button', { name: '进入竞拍面板' }).waitFor({ timeout: 15_000 });
  await shot(h5, '03-h5-room-entry');

  await clickVisible(h5, '进入竞拍面板');
  await shot(h5, '04-h5-bid-panel');

  await h5.getByRole('button', { name: /滑动出一手/ }).click();
  await h5.waitForTimeout(1800);
  await shot(h5, '05-h5-leading-after-bid');

  await clickVisible(pc, '打开竞价演示');
  await shot(pc, '06-pc-demo-assistant');

  await clickVisible(pc, '模拟无效出价');
  await pc.waitForTimeout(1000);
  await shot(pc, '07-pc-invalid-bid');

  await clickVisible(pc, '对手压过买家');
  await pc.waitForTimeout(1500);
  await h5.bringToFront();
  await h5.waitForTimeout(1500);
  await shot(h5, '08-h5-outbid');

  await h5.getByRole('button', { name: /滑动出一手/ }).click();
  await h5.waitForTimeout(1500);
  await shot(h5, '09-h5-leading-again');

  await pc.bringToFront();
  await clickVisible(pc, '倒计时缩到 15 秒');
  await pc.waitForTimeout(1200);
  await shot(pc, '10-pc-countdown-shortened');
  await h5.bringToFront();
  await h5.waitForTimeout(1200);
  await shot(h5, '11-h5-countdown-shortened');

  await pc.bringToFront();
  await pc.waitForTimeout(7000);
  await clickVisible(pc, '触发末段延时');
  await pc.waitForTimeout(1500);
  await shot(pc, '12-pc-extension-triggered');
  await h5.bringToFront();
  await h5.waitForTimeout(1500);
  await shot(h5, '13-h5-extension');

  await pc.bringToFront();
  await clickVisible(pc, '触发封顶成交');
  await pc.waitForTimeout(2200);
  await shot(pc, '14-pc-sold');
  await h5.bringToFront();
  await h5.waitForTimeout(2200);
  await shot(h5, '15-h5-sold-result');

  await pc.bringToFront();
  await pc.keyboard.press('Escape');
  await pc.waitForTimeout(500);
  await clickVisible(pc, '生成复盘');
  await pc.waitForTimeout(1800);
  await shot(pc, '16-pc-recap');

  await pc.getByRole('button', { name: '运行监控' }).click();
  await pc.waitForTimeout(1200);
  await shot(pc, '17-pc-diagnostics');

  await pc.getByRole('button', { name: /auc_live|事件回放|回放/ }).first().click().catch(() => {});
  await pc.waitForTimeout(1000);
  await shot(pc, '18-pc-final-evidence');

  await browser.close();
  console.log(JSON.stringify({ screenshots: 18, network }, null, 2));
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
