import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const apiBase = 'http://127.0.0.1:18080';
const h5Base = 'http://127.0.0.1:5276';
const pcBase = 'http://127.0.0.1:5277';
const auctionID = 'auc_live';
const roomID = 'room_main';
const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/main-room-order-detail';

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

async function getTruth(cookie) {
  const [auction, leaderboard, bids, orders] = await Promise.all([
    api(`/api/auctions/${auctionID}`, {}, cookie),
    api(`/api/auctions/${auctionID}/leaderboard?limit=10`, {}, cookie),
    api(`/api/users/me/bids?auction_id=${auctionID}&limit=20`, {}, cookie),
    api(`/api/users/me/orders?auction_id=${auctionID}&limit=20`, {}, cookie)
  ]);
  for (const result of [auction, leaderboard, bids, orders]) {
    if (!result.response.ok) throw new Error(`truth API failed: ${result.response.status} ${result.text}`);
  }
  return { auction: auction.body, leaderboard: leaderboard.body, bids: bids.body, orders: orders.body };
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
    ) {
      return last;
    }
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

async function clickH5Bid(page, priceText, { skipPriceWait = false } = {}) {
  const cta = page.getByTestId('bid-cta');
  const before = {
    text: await cta.textContent(),
    disabled: await cta.isDisabled()
  };
  await cta.click();
  if (!skipPriceWait) {
    await page.getByLabel('auction-state').getByRole('heading', { name: priceText }).waitFor({ timeout: 12000 });
  }
  return {
    before,
    after: {
      text: await cta.textContent().catch(() => ''),
      disabled: await cta.isDisabled().catch(() => null)
    }
  };
}

async function closePCDrawer(page) {
  const closeButton = page.locator('.arco-drawer-close').last();
  if (await closeButton.isVisible().catch(() => false)) {
    await closeButton.click();
    await closeButton.waitFor({ state: 'hidden', timeout: 5000 }).catch(() => {});
    return;
  }
  const mask = page.locator('.arco-drawer-mask').last();
  if (await mask.isVisible().catch(() => false)) {
    await mask.click({ position: { x: 4, y: 4 }, force: true }).catch(() => {});
    await mask.waitFor({ state: 'hidden', timeout: 5000 }).catch(() => {});
  }
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

async function pcSetRivalMax(page, amountYuanText) {
  await openDemoDriver(page);
  await page.getByLabel('rival-max-bid-yuan').fill(amountYuanText);
  await page.getByRole('button', { name: '设置对手自动加价' }).click();
}

async function assertNoBuyerButtons(page) {
  await openDemoDriver(page);
  const text = await page.getByTestId('live-assist-rail').textContent();
  if (text?.includes('买家反超') || text?.includes('连续竞价')) {
    throw new Error(`PC demo helper still exposes buyer action buttons: ${text}`);
  }
}

async function assertH5(page, expected) {
  await page.getByLabel('auction-state').getByRole('heading', { name: expected.priceText }).waitFor({ timeout: 12000 });
  const body = await page.locator('body').textContent();
  if (!body?.includes(expected.priceText)) throw new Error(`H5 missing ${expected.priceText}: ${body}`);
  if (expected.pattern && !expected.pattern.test(body)) {
    throw new Error(`H5 does not match ${expected.pattern}: ${body}`);
  }
  return { body };
}

async function assertH5Body(page, expected) {
  await page.getByText(expected.priceText).first().waitFor({ timeout: 12000 });
  const body = await page.locator('body').textContent();
  if (!body?.includes(expected.priceText)) throw new Error(`H5 body missing ${expected.priceText}: ${body}`);
  if (expected.pattern && !expected.pattern.test(body)) {
    throw new Error(`H5 body does not match ${expected.pattern}: ${body}`);
  }
  return { body };
}

async function assertPCPrice(page, priceText) {
  await page.getByTestId('auction-control-summary').getByText(priceText).first().waitFor({ timeout: 12000 });
  return {
    summary: await page.getByTestId('auction-control-summary').textContent(),
    queue: await page.getByTestId('auction-queue').textContent().catch(() => '')
  };
}

async function openH5Records(page) {
  const resultSheet = page.getByTestId('result-sheet');
  if (await resultSheet.isVisible().catch(() => false)) {
    await resultSheet.getByRole('button', { name: /确认成交事实再支付|查看订单|查看出价记录/ }).click();
  } else {
    await page.getByLabel('我的').click();
    await page.getByRole('button', { name: '我的出价与订单' }).click();
  }
  await page.getByTestId('history-panel').waitFor({ timeout: 10000 });
}

async function assertH5Records(page) {
  const panel = page.getByTestId('history-panel');
  await panel.getByText('订单 ¥600.00').waitFor({ timeout: 12000 });
  const text = await panel.textContent();
  const bidRows = await panel.locator('.history-row').count();
  if (!text?.includes('我的记录') || !text.includes('订单 ¥600.00')) {
    throw new Error(`H5 current auction records not visible: ${text}`);
  }
  if (bidRows > 20) {
    throw new Error(`H5 records show too many rows for scoped current auction: ${bidRows}`);
  }
  return { text, rowCount: bidRows };
}

async function openPCOrdersAndDetail(page) {
  await closePCDrawer(page);
  await page.getByRole('button', { name: '订单记录' }).click();
  await page.getByTestId('pc-orders-page').waitFor({ timeout: 12000 });
  await page.getByTestId('current-order-card').getByText('¥600.00').waitFor({ timeout: 12000 });
  const card = await page.getByTestId('current-order-card').textContent();
  await page.getByRole('button', { name: '查看详情' }).click();
  await page.getByTestId('order-detail-drawer').waitFor({ timeout: 12000 });
  const detail = await page.getByTestId('order-detail-drawer').textContent();
  if (!card?.includes('当前拍品成交详情') || !card.includes('¥600.00') || !detail?.includes('成交价') || !detail.includes('¥600.00')) {
    throw new Error(`PC order detail incomplete: card=${card} detail=${detail}`);
  }
  return { card, detail };
}

async function main() {
  await fs.mkdir(outDir, { recursive: true });
  const buyer = await login('user');
  const browser = await chromium.launch({ headless: true });
  const h5Context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true });
  const pcContext = await browser.newContext({ viewport: { width: 1440, height: 920 } });
  const h5 = await h5Context.newPage();
  const pc = await pcContext.newPage();
  const network = [];
  const evidence = [];

  for (const page of [h5, pc]) {
    page.on('response', (response) => {
      const url = response.url();
      if (url.includes('/api/orders') || url.includes('/api/users/me/orders') || url.includes('/api/users/me/bids')) {
        network.push(`${response.status()} ${response.request().method()} ${url}`);
      }
    });
  }

  await pc.goto(`${pcBase}/`, { waitUntil: 'networkidle' });
  await resetFromPC(pc);
  await h5.goto(`${h5Base}/rooms/${roomID}`, { waitUntil: 'networkidle' });
  await h5.getByTestId('floating-product-card').click();
  await assertNoBuyerButtons(pc);
  await assertH5(h5, { priceText: '¥350.00', pattern: /下一口|400\.00/ });
  evidence.push({ step: 'reset_and_initial', truth: await getTruth(buyer.cookie), pc: await assertPCPrice(pc, '¥350.00') });
  await h5.screenshot({ path: path.join(outDir, '01-h5-initial.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '01-pc-initial.png'), fullPage: true });

  evidence.push({ step: 'h5_manual_400', action: await clickH5Bid(h5, '¥400.00'), truth: await waitForTruth(buyer.cookie, { price: 40000, winner: 'user_1' }) });
  await h5.screenshot({ path: path.join(outDir, '02-h5-leading-400.png'), fullPage: true });

  await pcDemoClick(pc, '对手压过买家', '¥450.00');
  evidence.push({ step: 'pc_demo_rival_450', truth: await waitForTruth(buyer.cookie, { price: 45000, winner: 'user_2' }), h5: await assertH5(h5, { priceText: '¥450.00', pattern: /500\.00|被超越|第 2 名|#2/ }) });
  await h5.screenshot({ path: path.join(outDir, '03-h5-outbid-450.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '03-pc-outbid-450.png'), fullPage: true });

  await pcSetRivalMax(pc, '550');
  evidence.push({ step: 'pc_set_rival_proxy_max_550', truth: await getTruth(buyer.cookie), pc: await assertPCPrice(pc, '¥450.00') });
  await pc.screenshot({ path: path.join(outDir, '04-pc-rival-proxy-550.png'), fullPage: true });

  evidence.push({ step: 'h5_manual_500_triggers_rival_proxy_550', action: await clickH5Bid(h5, '¥550.00'), truth: await waitForTruth(buyer.cookie, { price: 55000, winner: 'user_2' }), h5: await assertH5(h5, { priceText: '¥550.00', pattern: /600\.00|封顶|第 2 名|#2|被超越/ }) });
  await h5.screenshot({ path: path.join(outDir, '05-h5-rival-proxy-defense-550.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '05-pc-rival-proxy-defense-550.png'), fullPage: true });

  evidence.push({ step: 'h5_manual_cap_600', action: await clickH5Bid(h5, '¥600.00', { skipPriceWait: true }), truth: await waitForTruth(buyer.cookie, { price: 60000, winner: 'user_1', status: 'SOLD' }), h5: await assertH5Body(h5, { priceText: '¥600.00', pattern: /成交|中拍|订单|支付/ }), pc: await assertPCPrice(pc, '¥600.00') });
  await h5.screenshot({ path: path.join(outDir, '06-h5-sold-600.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '06-pc-sold-600.png'), fullPage: true });

  const pcOrder = await openPCOrdersAndDetail(pc);
  evidence.push({ step: 'pc_current_order_detail', order: pcOrder });
  await pc.screenshot({ path: path.join(outDir, '07-pc-order-detail.png'), fullPage: true });

  await openH5Records(h5);
  const h5Records = await assertH5Records(h5);
  evidence.push({ step: 'h5_current_auction_records', records: h5Records });
  await h5.screenshot({ path: path.join(outDir, '08-h5-current-records.png'), fullPage: true });

  const finalTruth = await getTruth(buyer.cookie);
  const orders = finalTruth.orders.items ?? [];
  if (orders.length !== 1 || orders[0].auction_id !== auctionID || orders[0].amount_cents !== 60000) {
    throw new Error(`current auction order scope is wrong: ${JSON.stringify(orders)}`);
  }
  if (finalTruth.auction.current_price_cents !== finalTruth.leaderboard.current_price_cents) {
    throw new Error('auction and leaderboard price diverged');
  }
  if (finalTruth.auction.current_winner_id !== finalTruth.leaderboard.current_winner_id) {
    throw new Error('auction and leaderboard winner diverged');
  }
  if ((finalTruth.leaderboard.entries?.[0]?.amount_cents ?? 0) !== finalTruth.auction.current_price_cents) {
    throw new Error('leaderboard top amount does not match auction price');
  }
  const unscopedFrontendOrderCalls = network.filter((entry) => {
    if (!entry.includes('127.0.0.1:5276') && !entry.includes('127.0.0.1:5277')) return false;
    if (!entry.includes('/api/orders') && !entry.includes('/api/users/me/orders') && !entry.includes('/api/users/me/bids')) return false;
    return !entry.includes('auction_id=auc_live') && !entry.includes('/api/orders/');
  });
  if (unscopedFrontendOrderCalls.length) {
    throw new Error(`frontend made unscoped current-history/order calls: ${JSON.stringify(unscopedFrontendOrderCalls, null, 2)}`);
  }

  await fs.writeFile(path.join(outDir, 'network.json'), JSON.stringify(network, null, 2));
  await fs.writeFile(path.join(outDir, 'result.json'), JSON.stringify({ evidence, finalTruth }, null, 2));
  await browser.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
