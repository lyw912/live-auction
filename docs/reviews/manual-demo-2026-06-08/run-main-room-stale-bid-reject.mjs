import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const apiBase = 'http://127.0.0.1:18080';
const h5Base = 'http://127.0.0.1:5276';
const pcBase = 'http://127.0.0.1:5277';
const auctionID = 'auc_live';
const roomID = 'room_main';
const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/main-room-stale-bid-reject';

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
  const [auction, leaderboard] = await Promise.all([
    api(`/api/auctions/${auctionID}`, {}, cookie),
    api(`/api/auctions/${auctionID}/leaderboard?limit=10`, {}, cookie)
  ]);
  if (!auction.response.ok) throw new Error(auction.text);
  if (!leaderboard.response.ok) throw new Error(leaderboard.text);
  return { auction: auction.body, leaderboard: leaderboard.body };
}

async function waitForTruth(cookie, expected) {
  let last = null;
  for (let i = 0; i < 80; i += 1) {
    last = await getTruth(cookie);
    const top = last.leaderboard.entries?.[0];
    if (
      last.auction.current_price_cents === expected.price &&
      last.auction.current_winner_id === expected.winner &&
      last.leaderboard.current_price_cents === expected.price &&
      last.leaderboard.current_winner_id === expected.winner &&
      top?.amount_cents === expected.price &&
      top?.user_id === expected.winner
    ) {
      return last;
    }
    await new Promise((resolve) => setTimeout(resolve, 120));
  }
  throw new Error(`truth did not converge: ${JSON.stringify(last)}`);
}

async function resetFromPC(page) {
  await page.getByRole('button', { name: '重置演示环境' }).click();
  await page.getByRole('button', { name: '确认重置' }).click();
  await page.getByTestId('pc-workbench-status').getByText('演示环境已重置').waitFor({ timeout: 30000 });
  await page.getByTestId('auction-control-summary').getByText('¥350.00').first().waitFor({ timeout: 12000 });
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
  await h5.getByTestId('bid-cta').click();
  const after400 = await waitForTruth(buyer.cookie, { price: 40000, winner: 'user_1' });
  evidence.push({ step: 'h5_accepted_400', truth: after400 });

  await pc.getByTestId('demo-driver').getByRole('button', { name: '打开场景演示' }).click();
  await pc.getByRole('button', { name: '对手压过买家' }).click();
  const after450 = await waitForTruth(buyer.cookie, { price: 45000, winner: 'user_2' });
  evidence.push({ step: 'pc_accepted_450', truth: after450 });
  await h5.screenshot({ path: path.join(outDir, '01-before-stale-reject-h5.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '01-before-stale-reject-pc.png'), fullPage: true });

  const staleBidID = `stale-demo-${Date.now()}`;
  const stale = await api(`/api/auctions/${auctionID}/bids`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': staleBidID
    },
    body: JSON.stringify({
      client_bid_id: staleBidID,
      amount_cents: 45000,
      client_seen_seq: after400.auction.seq
    })
  }, buyer.cookie);
  const afterReject = await waitForTruth(buyer.cookie, { price: 45000, winner: 'user_2' });
  evidence.push({
    step: 'stale_low_bid_rejected',
    request: { amount_cents: 45000, client_seen_seq: after400.auction.seq },
    response: { status: stale.response.status, ok: stale.response.ok, body: stale.body },
    truth: afterReject
  });
  await h5.screenshot({ path: path.join(outDir, '02-after-stale-reject-h5.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '02-after-stale-reject-pc.png'), fullPage: true });

  if (stale.body?.result !== 'ENGINE_REJECTED' || stale.body?.reject_reason !== 'BID_TOO_LOW') {
    throw new Error(`stale bid was not rejected as BID_TOO_LOW: ${JSON.stringify(stale.body)}`);
  }
  if (afterReject.auction.current_price_cents !== 45000 || afterReject.leaderboard.entries?.[0]?.amount_cents !== 45000) {
    throw new Error('rejected stale bid changed auction or leaderboard truth');
  }
  await fs.writeFile(path.join(outDir, 'result.json'), JSON.stringify({ evidence }, null, 2));
  await browser.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
