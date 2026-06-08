import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/product-flow';
const pcURL = process.env.LIVE_AUCTION_PC_URL || 'http://127.0.0.1:5277';
const h5URL = process.env.LIVE_AUCTION_H5_URL || 'http://127.0.0.1:5276/rooms/room_main';

async function shot(page, name, fullPage = true) {
  await page.screenshot({ path: `${outDir}/${name}.png`, fullPage });
}

async function clickByText(page, text, timeout = 12_000) {
  const target = page.getByText(text, { exact: false }).first();
  await target.waitFor({ state: 'visible', timeout });
  await target.click();
}

async function clickButton(page, name, timeout = 12_000) {
  const target = page.getByRole('button', { name });
  await target.first().waitFor({ state: 'visible', timeout });
  await target.first().click();
}

async function clickControlButton(page, name, timeout = 12_000) {
  const target = page.getByTestId('auction-control-summary').getByRole('button', { name });
  await target.first().waitFor({ state: 'visible', timeout });
  await target.first().click();
}

async function selectQueueCard(page, title, status) {
  const card = page.getByRole('button', { name: new RegExp(`${title}.*${status}`) }).first();
  await card.waitFor({ state: 'visible', timeout: 12_000 });
  await card.click();
  await page.getByTestId('auction-control-summary').getByRole('heading', { name: title }).waitFor({ timeout: 12_000 });
}

async function ensureHost(page) {
  await page.evaluate(async () => {
    await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ account: 'host' })
    });
  });
}

async function cancelActiveAuctionInRoom(page, exceptTitle) {
  await page.evaluate(async ({ exceptTitle }) => {
    await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ account: 'host' })
    });
    const response = await fetch('/api/auctions?room_id=room_main');
    const auctions = await response.json();
    const active = (Array.isArray(auctions) ? auctions : []).find((auction) => auction.status === 'ACTIVE' && auction.item?.title !== exceptTitle);
    if (active?.id) {
      await fetch(`/api/auctions/${active.id}/cancel`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reason: '演示切换下一件' })
      });
    }
  }, { exceptTitle });
}

async function ensureUser(page) {
  await page.evaluate(async () => {
    await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ account: 'user' })
    });
  });
}

async function main() {
  await fs.mkdir(outDir, { recursive: true });
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
  await ensureHost(pc);
  await pc.reload({ waitUntil: 'networkidle' });
  await pc.getByRole('heading', { name: '开播中控' }).waitFor({ timeout: 15_000 });
  await clickButton(pc, '重置演示环境');
  await clickButton(pc, '确认重置');
  await pc.getByText('演示环境已重置').first().waitFor({ timeout: 20_000 });
  await shot(pc, '01-pc-reset-ready');

  await clickButton(pc, '拍品与规则');
  const title = `流程验证拍品 ${Date.now().toString().slice(-4)}`;
  await pc.getByRole('textbox', { name: 'item-title' }).fill(title);
  await pc.getByRole('textbox', { name: 'item-description' }).fill('端到端验证：新建、改规则、排期、开拍、出价、成交、支付、历史。');
  await pc.getByRole('spinbutton', { name: 'cap-price-cents' }).fill('150.00');
  await pc.getByRole('spinbutton', { name: 'duration-seconds' }).fill('90');
  await clickButton(pc, '创建拍品和竞拍');
  await pc.getByText(`当前选中「${title}」`).waitFor({ timeout: 15_000 });
  await shot(pc, '02-pc-created-selected');

  await clickButton(pc, '保存规则');
  await pc.waitForTimeout(700);
  await clickButton(pc, '开播中控');
  await clickByText(pc, title);
  await shot(pc, '03-pc-edit-object-not-live');

  await Promise.all([
    pc.waitForResponse((response) => response.url().includes('/api/auctions/') && response.url().endsWith('/schedule') && response.status() < 300),
    clickControlButton(pc, '排期')
  ]);
  await pc.waitForTimeout(700);
  await shot(pc, '04-pc-scheduled');

  await h5.goto(h5URL, { waitUntil: 'networkidle' });
  await ensureUser(h5);
  await h5.reload({ waitUntil: 'networkidle' });
  await h5.getByRole('button', { name: '商品列表' }).click();
  await h5.getByText(title).waitFor({ timeout: 15_000 });
  await shot(h5, '05-h5-queue-has-new-lot');
  await h5.keyboard.press('Escape').catch(() => {});

  await ensureHost(pc);
  await cancelActiveAuctionInRoom(pc, title);
  await pc.reload({ waitUntil: 'networkidle' });
  await clickButton(pc, '开播中控');
  await selectQueueCard(pc, title, '已排期');
  await Promise.all([
    pc.waitForResponse((response) => response.url().includes('/api/auctions/') && response.url().endsWith('/start') && response.status() < 300),
    clickControlButton(pc, '开拍')
  ]);
  await pc.getByText('买家端正在看').waitFor({ timeout: 10_000 });
  await pc.getByText('开拍中').first().waitFor({ timeout: 10_000 });
  await shot(pc, '06-pc-started-current');

  await h5.bringToFront();
  await ensureUser(h5);
  await h5.getByText(title).first().waitFor({ timeout: 12_000 });
  await h5.getByText(/出一手|当前最高价|下一口/).first().waitFor({ timeout: 12_000 });
  await shot(h5, '07-h5-active-new-lot');
  await clickButton(h5, '进入竞拍面板');
  await shot(h5, '08-h5-bid-panel');
  await h5.getByRole('button', { name: /出一手|滑动出一手|确认高额出价/ }).first().click();
  await h5.getByTestId('result-sheet').waitFor({ timeout: 20_000 });
  await shot(h5, '09-h5-sold-result');

  const payButton = h5.getByTestId('result-pay-cta');
  if (await payButton.isVisible().catch(() => false)) {
    await payButton.click();
    await h5.getByText('已支付').first().waitFor({ timeout: 15_000 });
  }
  await shot(h5, '10-h5-paid-orders');

  await h5.getByText('订单').first().waitFor({ timeout: 10_000 });
  await shot(h5, '11-h5-order-history');

  await pc.bringToFront();
  await ensureHost(pc);
  await clickButton(pc, '订单记录');
  await pc.getByText('已支付').waitFor({ timeout: 15_000 }).catch(() => {});
  await shot(pc, '12-pc-orders-paid');

  await browser.close();
  await fs.writeFile(path.join(outDir, 'network.json'), JSON.stringify(network, null, 2));
  console.log(JSON.stringify({ title, screenshots: 12, outDir, networkCount: network.length }, null, 2));
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
