import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const apiBase = 'http://127.0.0.1:18080';
const h5Base = 'http://127.0.0.1:5276';
const pcBase = 'http://127.0.0.1:5277';
const auctionID = 'auc_live';
const roomID = 'room_main';
const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/main-room-pc-proxy-defense-toast';

async function api(pathname, options = {}, cookie = '') {
  const headers = { ...(options.headers || {}) };
  if (cookie) headers.Cookie = cookie;
  const response = await fetch(`${apiBase}${pathname}`, { ...options, headers });
  const text = await response.text();
  let body = null;
  try { body = text ? JSON.parse(text) : null; } catch { body = text; }
  return { response, body, text };
}

async function login(account) {
  const { response, body, text } = await api('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ account })
  });
  if (!response.ok) throw new Error(`login ${account} failed: ${text}`);
  return { cookie: response.headers.get('set-cookie').split(';')[0], user: body.user };
}

async function getTruth(cookie) {
  const [auction, leaderboard] = await Promise.all([
    api(`/api/auctions/${auctionID}`, {}, cookie),
    api(`/api/auctions/${auctionID}/leaderboard?limit=10`, {}, cookie)
  ]);
  for (const result of [auction, leaderboard]) {
    if (!result.response.ok) throw new Error(`truth API failed: ${result.response.status} ${result.text}`);
  }
  return { auction: auction.body, leaderboard: leaderboard.body };
}

async function waitForTruth(cookie, expected) {
  let last = null;
  for (let i = 0; i < 120; i += 1) {
    last = await getTruth(cookie);
    const top = last.leaderboard.entries?.[0];
    if (
      last.auction.current_price_cents === expected.price &&
      last.auction.current_winner_id === expected.winner &&
      top?.amount_cents === expected.price &&
      top?.user_id === expected.winner
    ) return last;
    await new Promise((resolve) => setTimeout(resolve, 120));
  }
  throw new Error(`truth did not converge to ${JSON.stringify(expected)}: ${JSON.stringify(last)}`);
}

async function resetFromPC(page) {
  await page.getByRole('button', { name: '重置演示环境' }).click();
  await page.getByRole('button', { name: '确认重置' }).click();
  await page.getByTestId('pc-workbench-status').getByText('演示环境已重置').waitFor({ timeout: 30000 });
  await page.getByTestId('auction-control-summary').getByText('¥350.00').first().waitFor({ timeout: 12000 });
}

async function openDemoDriver(page) {
  if (await page.getByRole('button', { name: '旧价请求被拒绝' }).isVisible().catch(() => false)) return;
  await page.getByTestId('demo-driver').getByRole('button', { name: '打开场景演示' }).click();
}

async function pcDemoClick(page, label, expectedPriceText) {
  await openDemoDriver(page);
  await page.getByRole('button', { name: label }).click();
  await page.getByTestId('auction-control-summary').getByText(expectedPriceText).first().waitFor({ timeout: 12000 });
}

async function clickH5Bid(page, priceText) {
  await page.getByTestId('bid-cta').click();
  await page.getByLabel('auction-state').getByRole('heading', { name: priceText }).waitFor({ timeout: 12000 });
}

async function setH5MaxBid(page, amountYuanText) {
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '自动加价' }).click();
  const sheet = page.getByTestId('bottom-sheet');
  await sheet.getByTestId('max-bid-sheet').waitFor({ timeout: 8000 });
  await sheet.getByLabel('max-bid-yuan').fill(amountYuanText);
  await sheet.getByRole('button', { name: /设置 ¥600\.00|更新为 ¥600\.00/ }).click();
  await sheet.getByText(/自动加价已生效|自动加价已防守/).waitFor({ timeout: 12000 });
  await page.getByRole('button', { name: '关闭面板' }).click();
}

async function main() {
  await fs.mkdir(outDir, { recursive: true });
  const buyer = await login('user');
  const browser = await chromium.launch({ headless: true });
  const h5 = await browser.newPage({ viewport: { width: 390, height: 844 }, isMobile: true });
  const pc = await browser.newPage({ viewport: { width: 1440, height: 920 } });

  await pc.goto(`${pcBase}/`, { waitUntil: 'networkidle' });
  await resetFromPC(pc);
  await h5.goto(`${h5Base}/rooms/${roomID}`, { waitUntil: 'networkidle' });
  await h5.getByTestId('floating-product-card').click();

  await clickH5Bid(h5, '¥400.00');
  await waitForTruth(buyer.cookie, { price: 40000, winner: 'user_1' });
  await setH5MaxBid(h5, '600');

  await openDemoDriver(pc);
  await pc.getByRole('button', { name: '对手压过买家' }).click();
  await pc.getByText(/买家自动代理已防守到 ¥500\.00：代理防守成功/).waitFor({ timeout: 12000 });
  const errorVisible = await pc.getByText(/BID_TOO_LOW|低于当前可出价|演示出价失败/).isVisible().catch(() => false);
  if (errorVisible) throw new Error('PC showed a low-bid error while proxy defense succeeded');
  const truth = await waitForTruth(buyer.cookie, { price: 50000, winner: 'user_1' });
  await pc.screenshot({ path: path.join(outDir, '01-pc-proxy-defense-toast.png'), fullPage: true });
  await fs.writeFile(path.join(outDir, 'result.json'), JSON.stringify({ truth, toast: 'proxy defense success without low-bid error' }, null, 2));
  await browser.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
