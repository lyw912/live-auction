import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const apiBase = 'http://127.0.0.1:18080';
const h5Base = 'http://127.0.0.1:5276';
const pcBase = 'http://127.0.0.1:5277';
const auctionID = 'auc_live';
const roomID = 'room_main';
const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/main-room-auto-cancel-defense';

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
  const [auction, leaderboard, intent] = await Promise.all([
    api(`/api/auctions/${auctionID}`, {}, cookie),
    api(`/api/auctions/${auctionID}/leaderboard?limit=10`, {}, cookie),
    api(`/api/auctions/${auctionID}/max-bid-intent`, {}, cookie)
  ]);
  for (const result of [auction, leaderboard]) {
    if (!result.response.ok) throw new Error(`truth API failed: ${result.response.status} ${result.text}`);
  }
  return { auction: auction.body, leaderboard: leaderboard.body, intent: intent.response.ok ? intent.body : null };
}

async function waitForTruth(cookie, expected) {
  let last = null;
  for (let i = 0; i < 120; i += 1) {
    last = await getTruth(cookie);
    const top = last.leaderboard.entries?.[0];
    if (
      last.auction.current_price_cents === expected.price &&
      last.auction.current_winner_id === expected.winner &&
      last.leaderboard.current_price_cents === expected.price &&
      last.leaderboard.current_winner_id === expected.winner &&
      top?.amount_cents === expected.price &&
      top?.user_id === expected.winner &&
      (!expected.status || last.auction.status === expected.status)
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

async function openH5MaxBidSheet(page) {
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '自动加价' }).click();
  const sheet = page.getByTestId('bottom-sheet');
  await sheet.getByTestId('max-bid-sheet').waitFor({ timeout: 8000 });
  return sheet;
}

async function setH5MaxBid(page, amountYuanText, screenshotName) {
  const sheet = await openH5MaxBidSheet(page);
  await sheet.getByLabel('max-bid-yuan').fill(amountYuanText);
  await page.screenshot({ path: path.join(outDir, screenshotName), fullPage: true });
  await sheet.getByRole('button', { name: /设置 ¥600\.00|更新为 ¥600\.00/ }).click();
  await page.waitForTimeout(300);
  await page.getByTestId('bottom-sheet-backdrop').click({ position: { x: 8, y: 8 } });
}

async function cancelH5MaxBid(page) {
  const sheet = await openH5MaxBidSheet(page);
  await sheet.getByRole('button', { name: '取消' }).click();
  await sheet.getByText('自动加价已取消').waitFor({ timeout: 12000 });
  await page.screenshot({ path: path.join(outDir, '04-h5-auto-cancelled.png'), fullPage: true });
  await page.getByTestId('bottom-sheet-backdrop').click({ position: { x: 8, y: 8 } });
}

async function main() {
  await fs.mkdir(outDir, { recursive: true });
  const buyer = await login('user');
  const browser = await chromium.launch({ headless: true });
  const h5 = await browser.newPage({ viewport: { width: 390, height: 844 }, isMobile: true });
  const pc = await browser.newPage({ viewport: { width: 1440, height: 920 } });
  const evidence = [];

  await pc.goto(`${pcBase}/`, { waitUntil: 'networkidle' });
  await resetFromPC(pc);
  await h5.goto(`${h5Base}/rooms/${roomID}`, { waitUntil: 'networkidle' });
  await h5.getByTestId('floating-product-card').click();
  await h5.screenshot({ path: path.join(outDir, '01-h5-initial.png'), fullPage: true });

  await clickH5Bid(h5, '¥400.00');
  evidence.push({ step: 'h5_manual_400', truth: await waitForTruth(buyer.cookie, { price: 40000, winner: 'user_1' }) });

  await pcDemoClick(pc, '对手压过买家', '¥450.00');
  evidence.push({ step: 'pc_rival_450', truth: await waitForTruth(buyer.cookie, { price: 45000, winner: 'user_2' }) });
  await h5.screenshot({ path: path.join(outDir, '02-h5-outbid-450.png'), fullPage: true });

  await setH5MaxBid(h5, '600', '03-h5-auto-set-600.png');
  evidence.push({ step: 'h5_auto_immediate_500', truth: await waitForTruth(buyer.cookie, { price: 50000, winner: 'user_1' }) });
  await h5.screenshot({ path: path.join(outDir, '03b-h5-auto-immediate-500.png'), fullPage: true });

  await cancelH5MaxBid(h5);
  const cancelledTruth = await getTruth(buyer.cookie);
  if (cancelledTruth.intent?.status !== 'CANCELLED') {
    throw new Error(`max bid intent was not cancelled: ${JSON.stringify(cancelledTruth.intent)}`);
  }
  evidence.push({ step: 'h5_cancel_auto', truth: cancelledTruth });

  await setH5MaxBid(h5, '600', '05-h5-auto-reset-600.png');
  evidence.push({ step: 'h5_auto_reset', truth: await getTruth(buyer.cookie) });

  await pcDemoClick(pc, '第三方强挑战', '¥600.00');
  const soldTruth = await waitForTruth(buyer.cookie, { price: 60000, winner: 'user_1', status: 'SOLD' });
  await h5.waitForFunction(() => document.body.textContent?.includes('自动加价防守到 ¥600.00'), null, { timeout: 12000 });
  await h5.screenshot({ path: path.join(outDir, '06-h5-auto-defense-sold.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '06-pc-auto-defense-sold.png'), fullPage: true });
  evidence.push({ step: 'h5_auto_defense_sold', truth: soldTruth });

  await fs.writeFile(path.join(outDir, 'result.json'), JSON.stringify({ evidence, finalTruth: await getTruth(buyer.cookie) }, null, 2));
  await browser.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
