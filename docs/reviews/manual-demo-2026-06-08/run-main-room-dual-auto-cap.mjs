import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const apiBase = 'http://127.0.0.1:18080';
const h5Base = 'http://127.0.0.1:5276';
const pcBase = 'http://127.0.0.1:5277';
const auctionID = 'auc_live';
const roomID = 'room_main';
const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/main-room-dual-auto-cap';

async function api(pathname, options = {}, cookie = '') {
  const headers = { ...(options.headers || {}) };
  if (cookie) headers.Cookie = cookie;
  const response = await fetch(`${apiBase}${pathname}`, { ...options, headers });
  const text = await response.text();
  let body = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }
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
  const [auction, leaderboard, bids, orders] = await Promise.all([
    api(`/api/auctions/${auctionID}`, {}, cookie),
    api(`/api/auctions/${auctionID}/leaderboard?limit=10`, {}, cookie),
    api(`/api/users/me/bids?auction_id=${auctionID}&limit=20`, {}, cookie),
    api(`/api/users/me/orders?auction_id=${auctionID}&limit=20`, {}, cookie)
  ]);
  for (const result of [auction, leaderboard, bids, orders]) {
    if (!result.response.ok) throw new Error(result.text);
  }
  return { auction: auction.body, leaderboard: leaderboard.body, bids: bids.body, orders: orders.body };
}

async function waitForTruth(cookie, expected) {
  let last = null;
  for (let i = 0; i < 100; i += 1) {
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
    ) {
      return last;
    }
    await new Promise((resolve) => setTimeout(resolve, 120));
  }
  throw new Error(`truth did not converge: ${JSON.stringify(last)}`);
}

async function setH5MaxBid(page, targetText) {
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '自动加价' }).click();
  const sheet = page.getByTestId('bottom-sheet');
  await sheet.getByTestId('max-bid-sheet').waitFor({ timeout: 8000 });
  for (let i = 0; i < 10; i += 1) {
    const text = await sheet.getByLabel('max-bid-amount').textContent();
    if (text?.includes(targetText)) break;
    await sheet.getByLabel('increase-max-bid').click();
  }
  const finalText = await sheet.getByLabel('max-bid-amount').textContent();
  if (!finalText?.includes(targetText)) {
    throw new Error(`H5 max bid did not reach ${targetText}: ${finalText}`);
  }
  await sheet.getByRole('button', { name: /设置自动加价|更新自动加价/ }).click();
  await sheet.getByText('自动加价已生效').waitFor({ timeout: 10000 });
  await page.getByTestId('bottom-sheet-backdrop').click({ position: { x: 8, y: 8 } });
  return { finalText };
}

async function pcSetRivalMax(page, amountYuanText) {
  await page.getByTestId('demo-driver').getByRole('button', { name: '打开场景演示' }).click();
  const input = page.getByLabel('rival-max-bid-yuan');
  await input.fill(amountYuanText);
  await page.getByRole('button', { name: '设置对手自动加价' }).click();
  await page.keyboard.press('Escape');
}

async function pcDemoBid(page, label, expectedText) {
  await page.getByTestId('demo-driver').getByRole('button', { name: '打开场景演示' }).click();
  await page.getByRole('button', { name: label }).click();
  await page.keyboard.press('Escape');
  await page.getByTestId('auction-control-summary').getByText(expectedText).first().waitFor({ timeout: 12000 });
}

async function main() {
  await fs.mkdir(outDir, { recursive: true });
  const buyer = await login('user');
  const browser = await chromium.launch({ headless: true });
  const h5 = await browser.newPage({ viewport: { width: 390, height: 844 }, isMobile: true });
  const pc = await browser.newPage({ viewport: { width: 1440, height: 920 } });
  const evidence = [];

  await h5.goto(`${h5Base}/rooms/${roomID}`, { waitUntil: 'networkidle' });
  await pc.goto(`${pcBase}/`, { waitUntil: 'networkidle' });
  await h5.getByTestId('floating-product-card').click();
  await h5.screenshot({ path: path.join(outDir, '01-initial-h5.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '01-initial-pc.png'), fullPage: true });

  await h5.getByTestId('bid-cta').click();
  evidence.push({ step: 'h5_manual_400', truth: await waitForTruth(buyer.cookie, { price: 40000, winner: 'user_1' }) });

  await pcDemoBid(pc, '对手压过买家', '¥450.00');
  evidence.push({ step: 'pc_rival_manual_450', truth: await waitForTruth(buyer.cookie, { price: 45000, winner: 'user_2' }) });

  const h5Max = await setH5MaxBid(h5, '¥600.00');
  evidence.push({ step: 'h5_auto_max_600', action: h5Max, truth: await getTruth(buyer.cookie) });
  await h5.screenshot({ path: path.join(outDir, '02-h5-max-600.png'), fullPage: true });

  await pcSetRivalMax(pc, '550');
  evidence.push({ step: 'pc_rival_auto_max_550', truth: await getTruth(buyer.cookie) });
  await pc.screenshot({ path: path.join(outDir, '03-pc-rival-max-550.png'), fullPage: true });

  await pcDemoBid(pc, '第三方强挑战', '¥600.00');
  const finalTruth = await waitForTruth(buyer.cookie, { price: 60000, winner: 'user_1', status: 'SOLD' });
  evidence.push({ step: 'third_party_challenge_triggers_h5_auto_cap_600', truth: finalTruth });
  await h5.screenshot({ path: path.join(outDir, '04-h5-auto-cap-sold.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '04-pc-auto-cap-sold.png'), fullPage: true });

  const accepted = finalTruth.leaderboard.entries?.[0];
  if (accepted?.user_id !== 'user_1' || accepted.amount_cents !== 60000 || finalTruth.auction.status !== 'SOLD') {
    throw new Error(`dual auto cap did not finish with H5 cap winner: ${JSON.stringify(finalTruth)}`);
  }
  await fs.writeFile(path.join(outDir, 'result.json'), JSON.stringify({ evidence }, null, 2));
  await browser.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
