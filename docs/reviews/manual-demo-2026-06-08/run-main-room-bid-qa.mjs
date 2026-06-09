import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const apiBase = 'http://127.0.0.1:18080';
const h5Base = 'http://127.0.0.1:5276';
const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/main-room-bid-qa';

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
  const setCookie = response.headers.get('set-cookie');
  if (!setCookie) throw new Error(`login ${account} missing cookie`);
  return { cookie: setCookie.split(';')[0], user: body.user };
}

async function waitForBackend(auctionID, priceCents, winnerID, userCookie) {
  let last = null;
  for (let i = 0; i < 80; i += 1) {
    const [auctionResult, leaderboardResult] = await Promise.all([
      api(`/api/auctions/${auctionID}`, {}, userCookie),
      api(`/api/auctions/${auctionID}/leaderboard?limit=5`, {}, userCookie)
    ]);
    last = { auction: auctionResult.body, leaderboard: leaderboardResult.body };
    if (
      auctionResult.response.ok &&
      leaderboardResult.response.ok &&
      auctionResult.body.current_price_cents === priceCents &&
      auctionResult.body.current_winner_id === winnerID &&
      leaderboardResult.body.current_price_cents === priceCents &&
      leaderboardResult.body.current_winner_id === winnerID
    ) {
      return last;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`backend did not reach ${priceCents}/${winnerID}: ${JSON.stringify(last)}`);
}

async function main() {
  await fs.mkdir(outDir, { recursive: true });
  const host = await login('host');
  const user = await login('user');

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true });
  const page = await context.newPage();
  const apiLog = [];
  page.on('response', async (response) => {
    const url = response.url();
    if (!url.includes('/api/auctions/auc_live') && !url.includes('/api/demo/auctions/auc_live')) return;
    apiLog.push(`${response.status()} ${response.request().method()} ${url}`);
  });

  await page.goto(`${h5Base}/rooms/room_main`, { waitUntil: 'networkidle' });
  await page.getByTestId('floating-product-card').click();
  await page.screenshot({ path: path.join(outDir, '01-initial-350.png'), fullPage: true });

  await page.getByTestId('bid-cta').click();
  await page.getByLabel('auction-state').getByRole('heading', { name: '¥400.00' }).waitFor({ timeout: 8000 });
  await page.getByText(/我的排名 #1|正在领先/).first().waitFor({ timeout: 8000 });
  await waitForBackend('auc_live', 40000, 'user_1', user.cookie);
  await page.screenshot({ path: path.join(outDir, '02-h5-400-leading.png'), fullPage: true });

  const competing = await api('/api/demo/auctions/auc_live/competing-bid', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      bidder_id: 'user_2',
      client_bid_id: `main-room-outbid-${Date.now()}`,
      amount_cents: 45000,
      client_seen_seq: 0
    })
  }, host.cookie);
  if (!competing.response.ok || competing.body.result !== 'ENGINE_ACCEPTED') {
    throw new Error(`competing bid failed: ${competing.response.status} ${competing.text}`);
  }
  const backendOutbid = await waitForBackend('auc_live', 45000, 'user_2', user.cookie);

  await page.getByLabel('auction-state').getByRole('heading', { name: '¥450.00' }).waitFor({ timeout: 10000 });
  await page.getByText(/我的排名 #2|差 ¥50.00|第 2 名|立即反超/).first().waitFor({ timeout: 10000 });
  await page.getByTestId('race-board').getByText(/榜一 .*¥450\.00/).waitFor({ timeout: 8000 });
  await page.getByTestId('race-board').getByText(/我 #2|差 ¥50\.00/).waitFor({ timeout: 8000 });
  await page.getByTestId('bid-cta').waitFor({ state: 'visible', timeout: 8000 });
  const ctaText = await page.getByTestId('bid-cta').textContent();
  const ctaDisabled = await page.getByTestId('bid-cta').isDisabled();
  const raceBoardText = await page.getByTestId('race-board').textContent();
  const rankStripText = await page.getByTestId('rank-strip').textContent();
  await page.screenshot({ path: path.join(outDir, '03-h5-450-outbid.png'), fullPage: true });

  await fs.writeFile(path.join(outDir, 'result.json'), JSON.stringify({
    ctaText,
    ctaDisabled,
    raceBoardText,
    rankStripText,
    backendOutbid,
    apiLog
  }, null, 2));
  if (ctaDisabled || !/500\.00/.test(ctaText || '')) {
    throw new Error(`H5 should allow rebid at 500 after outbid, got disabled=${ctaDisabled} text=${ctaText}`);
  }
  if (!/榜一/.test(raceBoardText || '') || !/450\.00/.test(raceBoardText || '') || !/(我 #2|差 ¥50\.00)/.test(raceBoardText || '')) {
    throw new Error(`race board should show rank1 at 450 and current user rank2 gap 50: ${raceBoardText}`);
  }
  if (!/(第 2 名|差 ¥50\.00|下一口 ¥500\.00)/.test(rankStripText || '')) {
    throw new Error(`rank strip should show outbid action context: ${rankStripText}`);
  }
  await browser.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
