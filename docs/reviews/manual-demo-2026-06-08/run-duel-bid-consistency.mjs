import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const apiBase = process.env.LIVE_AUCTION_API_URL || 'http://127.0.0.1:18080';
const h5Base = process.env.LIVE_AUCTION_H5_URL || 'http://127.0.0.1:5276';
const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/duel-bid-consistency';

let hostCookie = '';
let userCookie = '';

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
  if (!response.ok) {
    throw new Error(`${options.method || 'GET'} ${pathname} ${response.status}: ${text}`);
  }
  return { response, body };
}

async function login(account) {
  const { response, body } = await api('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ account })
  });
  const setCookie = response.headers.get('set-cookie');
  if (!setCookie) throw new Error(`login ${account} did not return a cookie`);
  return { cookie: setCookie.split(';')[0], user: body.user };
}

async function wait(ms) {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitForState(auctionID, priceCents, winnerID) {
  let last = null;
  for (let attempt = 0; attempt < 80; attempt += 1) {
    const [{ body: auction }, { body: leaderboard }] = await Promise.all([
      api(`/api/auctions/${auctionID}`, {}, userCookie),
      api(`/api/auctions/${auctionID}/leaderboard?limit=5`, {}, userCookie)
    ]);
    last = { auction, leaderboard };
    if (
      auction.current_price_cents === priceCents &&
      auction.current_winner_id === winnerID &&
      leaderboard.current_price_cents === priceCents &&
      leaderboard.current_winner_id === winnerID
    ) {
      return last;
    }
    await wait(75);
  }
  throw new Error(`state did not settle to ${winnerID} ${priceCents}: ${JSON.stringify(last)}`);
}

async function userBid(auctionID, amountCents, step) {
  const clientBidID = `duel-h5-${Date.now()}-${step}`;
  const { body } = await api(`/api/auctions/${auctionID}/bids`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': clientBidID
    },
    body: JSON.stringify({
      client_bid_id: clientBidID,
      amount_cents: amountCents,
      client_seen_seq: 0
    })
  }, userCookie);
  if (body.result !== 'ENGINE_ACCEPTED' && body.result !== 'ACCEPTED') {
    throw new Error(`H5 user bid ${amountCents} failed: ${JSON.stringify(body)}`);
  }
  return waitForState(auctionID, amountCents, 'user_1');
}

async function pcCompetingBid(auctionID, amountCents, step) {
  const clientBidID = `duel-pc-${Date.now()}-${step}`;
  const { body } = await api(`/api/demo/auctions/${auctionID}/competing-bid`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      bidder_id: 'user_2',
      client_bid_id: clientBidID,
      amount_cents: amountCents,
      client_seen_seq: 0
    })
  }, hostCookie);
  if (body.result !== 'ENGINE_ACCEPTED' && body.result !== 'ACCEPTED') {
    throw new Error(`PC competing bid ${amountCents} failed: ${JSON.stringify(body)}`);
  }
  if (body.durability_status && body.durability_status !== 'KAFKA_ACKED' && body.durability_status !== 'ENGINE_DURABLE') {
    throw new Error(`unexpected durability status: ${JSON.stringify(body)}`);
  }
  return waitForState(auctionID, amountCents, 'user_2');
}

async function createAuction() {
  await api('/api/test/rooms', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      room_id: `room_duel_${Date.now()}`,
      host_id: 'host_1',
      users: ['user_1', 'user_2']
    })
  }, hostCookie).catch(async (error) => {
    if (String(error).includes('test setup disabled')) {
      throw new Error('backend must run with AppEnv=test to create isolated duel rooms');
    }
    throw error;
  }).then(async ({ body }) => {
    createAuction.roomID = body.room_id;
  });

  const title = `Duel Consistency ${Date.now()}`;
  const { body: item } = await api('/api/items', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title, description: 'H5/PC repeated bid consistency verification', image_url: null })
  }, hostCookie);
  const { body: auction } = await api('/api/auctions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      room_id: createAuction.roomID,
      item_id: item.id,
      start_price_cents: 10000,
      increment_cents: 5000,
      cap_price_cents: null,
      rule: {
        duration_seconds: 300,
        extend_window_seconds: 10,
        extend_by_seconds: 10,
        max_extend_count: 3,
        deposit_bps: 1000,
        deposit_floor_cents: 5000,
        deposit_cap_cents: 50000
      }
    })
  }, hostCookie);
  await api(`/api/auctions/${auction.id}/schedule`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ start_at: null })
  }, hostCookie);
  await api(`/api/auctions/${auction.id}/start`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' }
  }, hostCookie);
  return { auctionID: auction.id, roomID: createAuction.roomID, title };
}

async function expectText(page, locator, pattern, label) {
  for (let attempt = 0; attempt < 80; attempt += 1) {
    const text = await locator.textContent().catch(() => '');
    if (pattern.test(text || '')) return text;
    await wait(100);
  }
  throw new Error(`${label} did not match ${pattern}`);
}

async function main() {
  await fs.mkdir(outDir, { recursive: true });
  hostCookie = (await login('host')).cookie;
  userCookie = (await login('user')).cookie;
  const { auctionID, roomID, title } = await createAuction();

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 390, height: 844 },
    isMobile: true
  });
  const page = await context.newPage();
  page.on('request', (request) => {
    if (request.url().startsWith(h5Base) || request.url().startsWith(apiBase)) return;
  });
  await page.goto(`${h5Base}/?room_id=${encodeURIComponent(roomID)}`, { waitUntil: 'networkidle' });
  await page.evaluate(async () => {
    await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ account: 'user' })
    });
  });
  await page.reload({ waitUntil: 'networkidle' });
  await expectText(page, page.locator('body'), new RegExp(title), 'H5 room title');

  const snapshots = [];
  const ensureBidDockOpen = async () => {
    if (await page.getByLabel('auction-state').isVisible().catch(() => false)) return;
    await page.getByTestId('floating-product-card').click();
  };
  const capture = async (name, expectedPrice, expectedRankText) => {
    await expectText(page, page.getByTestId('race-board'), new RegExp(expectedPrice.replace('.', '\\.')), `race board ${name}`);
    await ensureBidDockOpen();
    await expectText(page, page.getByLabel('auction-state'), new RegExp(expectedPrice.replace('.', '\\.')), `bid dock price ${name}`);
    if (expectedRankText) {
      await expectText(page, page.getByTestId('rank-strip'), expectedRankText, `rank strip ${name}`);
    }
    await page.screenshot({ path: path.join(outDir, `${name}.png`), fullPage: true });
    snapshots.push(`${name}.png`);
  };

  await userBid(auctionID, 15000, 1);
  await capture('01-h5-150-leading', '¥150.00', /第 1 名|正在领先|等待其他买家/);
  await pcCompetingBid(auctionID, 20000, 2);
  await capture('02-pc-200-outbid', '¥200.00', /第 2 名|差|下一口|立即反超|已有效出价/);
  await userBid(auctionID, 25000, 3);
  await capture('03-h5-250-leading', '¥250.00', /第 1 名|正在领先|等待其他买家/);
  await pcCompetingBid(auctionID, 30000, 4);
  await capture('04-pc-300-outbid', '¥300.00', /第 2 名|差|下一口|立即反超|已有效出价/);
  await userBid(auctionID, 35000, 5);
  await capture('05-h5-350-leading-final', '¥350.00', /第 1 名|正在领先|等待其他买家/);

  const { body: leaderboard } = await api(`/api/auctions/${auctionID}/leaderboard?limit=5`, {}, userCookie);
  const ctaText = await page.getByTestId('bid-cta').textContent();
  const ctaDisabled = await page.getByTestId('bid-cta').isDisabled();
  if (!ctaDisabled || !/等待其他买家/.test(ctaText || '')) {
    throw new Error(`leading CTA should be disabled wait state, got disabled=${ctaDisabled} text=${ctaText}`);
  }
  const entries = leaderboard.entries.map((entry) => ({
    rank: entry.rank,
    user_id: entry.user_id,
    user_masked: entry.user_masked,
    is_current: entry.is_current,
    amount_cents: entry.amount_cents,
    bid_count: entry.bid_count
  }));
  if (leaderboard.my_rank !== 1 || leaderboard.current_winner_id !== 'user_1' || entries[0]?.user_id !== 'user_1' || entries[0]?.amount_cents !== 35000) {
    throw new Error(`final leaderboard mismatch: ${JSON.stringify({ leaderboard, entries })}`);
  }
  if (entries[1]?.user_id !== 'user_2' || entries[1]?.amount_cents !== 30000) {
    throw new Error(`PC bidder should be rank 2 at 30000: ${JSON.stringify(entries)}`);
  }
  if (!entries.every((entry) => /^匿名用户 \d+$/.test(entry.user_masked))) {
    throw new Error(`anonymous labels should be stable numeric labels: ${JSON.stringify(entries)}`);
  }

  await fs.writeFile(path.join(outDir, 'result.json'), JSON.stringify({
    auction_id: auctionID,
    room_id: roomID,
    title,
    current_price_cents: leaderboard.current_price_cents,
    current_winner_id: leaderboard.current_winner_id,
    my_rank: leaderboard.my_rank,
    cta: { text: ctaText, disabled: ctaDisabled },
    entries,
    snapshots
  }, null, 2));

  await browser.close();
  console.log(JSON.stringify({
    outDir,
    auction_id: auctionID,
    current_price_cents: leaderboard.current_price_cents,
    current_winner_id: leaderboard.current_winner_id,
    my_rank: leaderboard.my_rank,
    cta: { text: ctaText, disabled: ctaDisabled },
    entries
  }, null, 2));
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
