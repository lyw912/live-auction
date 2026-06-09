import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const apiBase = 'http://127.0.0.1:18080';
const h5Base = 'http://127.0.0.1:5276';
const pcBase = 'http://127.0.0.1:5277';
const auctionID = 'auc_live';
const roomID = 'room_main';
const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/main-room-auto-pay-order-detail';

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

async function pcSetRivalMax(page, amountYuanText) {
  await openDemoDriver(page);
  await page.getByLabel('rival-max-bid-yuan').fill(amountYuanText);
  await page.getByRole('button', { name: '设置对手自动加价' }).click();
}

async function clickH5Bid(page, priceText, { skipPriceWait = false } = {}) {
  await page.getByTestId('bid-cta').click();
  if (!skipPriceWait) {
    await page.getByLabel('auction-state').getByRole('heading', { name: priceText }).waitFor({ timeout: 12000 });
  }
}

async function setH5MaxBidByInput(page, amountYuanText) {
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '自动加价' }).click();
  const sheet = page.getByTestId('bottom-sheet');
  await sheet.getByTestId('max-bid-sheet').waitFor({ timeout: 8000 });
  await sheet.getByLabel('max-bid-yuan').fill(amountYuanText);
  await page.screenshot({ path: path.join(outDir, '04-h5-auto-bid-input-600.png'), fullPage: true });
  await sheet.getByRole('button', { name: /设置 ¥600\.00|更新为 ¥600\.00/ }).click();
  await page.waitForTimeout(300);
  await page.getByTestId('bottom-sheet-backdrop').click({ position: { x: 8, y: 8 } });
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

async function openPCOrderDetail(page) {
  await closePCDrawer(page);
  await page.getByRole('button', { name: '订单记录' }).click();
  await page.getByTestId('current-order-card').getByText('¥600.00').waitFor({ timeout: 12000 });
  await page.getByRole('button', { name: '查看详情' }).click();
  await page.getByTestId('order-detail-drawer').waitFor({ timeout: 12000 });
  const detail = await page.getByTestId('order-detail-drawer').textContent();
  if (!detail?.includes('成交商品') || !detail.includes('成交价') || !detail.includes('¥600.00')) {
    throw new Error(`PC order detail missing product/order fields: ${detail}`);
  }
  return detail;
}

async function confirmH5PaymentAndOrderDetail(page) {
  await page.getByRole('button', { name: '立即支付' }).click();
  const dialog = page.getByTestId('payment-confirm-dialog');
  await dialog.waitFor({ timeout: 12000 });
  const paymentText = await dialog.textContent();
  if (!paymentText?.includes('¥600.00') || !paymentText.includes('确认支付')) {
    throw new Error(`payment confirm dialog incomplete: ${paymentText}`);
  }
  await page.screenshot({ path: path.join(outDir, '08-h5-payment-confirm.png'), fullPage: true });
  await dialog.getByRole('button', { name: /确认支付 ¥600\.00/ }).click();
  await page.getByTestId('history-panel').waitFor({ timeout: 12000 });
  await page.getByTestId('history-panel').getByText('以 ¥600.00 拍下').waitFor({ timeout: 12000 });
  await page.getByTestId('history-panel').getByText('以 ¥600.00 拍下').click();
  await page.getByTestId('buyer-order-detail').waitFor({ timeout: 12000 });
  const detail = await page.getByTestId('buyer-order-detail').textContent();
  if (!detail?.includes('订单详情') || !detail.includes('拍下金额') || !detail.includes('商品详情') || !detail.includes('¥600.00')) {
    throw new Error(`H5 buyer order detail incomplete: ${detail}`);
  }
  return detail;
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
  await h5.screenshot({ path: path.join(outDir, '02-h5-leading-400.png'), fullPage: true });

  await pcDemoClick(pc, '对手压过买家', '¥450.00');
  evidence.push({ step: 'pc_rival_450', truth: await waitForTruth(buyer.cookie, { price: 45000, winner: 'user_2' }) });
  await h5.screenshot({ path: path.join(outDir, '03-h5-outbid-450.png'), fullPage: true });

  await setH5MaxBidByInput(h5, '600');
  evidence.push({ step: 'h5_set_auto_600_immediate_500', truth: await waitForTruth(buyer.cookie, { price: 50000, winner: 'user_1' }) });
  await h5.screenshot({ path: path.join(outDir, '05-h5-auto-immediate-leading-500.png'), fullPage: true });

  await pcDemoClick(pc, '第三方强挑战', '¥600.00');
  evidence.push({ step: 'third_party_cap_challenge_h5_proxy_sold', truth: await waitForTruth(buyer.cookie, { price: 60000, winner: 'user_1', status: 'SOLD' }) });
  await h5.screenshot({ path: path.join(outDir, '07-h5-sold-before-pay.png'), fullPage: true });

  const h5OrderDetail = await confirmH5PaymentAndOrderDetail(h5);
  evidence.push({ step: 'h5_confirm_pay_and_order_detail', h5OrderDetail, truth: await getTruth(buyer.cookie) });
  await h5.screenshot({ path: path.join(outDir, '09-h5-paid-order-detail.png'), fullPage: true });

  const pcOrderDetail = await openPCOrderDetail(pc);
  evidence.push({ step: 'pc_order_detail_with_product', pcOrderDetail });
  await pc.screenshot({ path: path.join(outDir, '10-pc-order-detail-with-product.png'), fullPage: true });

  const finalTruth = await getTruth(buyer.cookie);
  const orders = finalTruth.orders.items ?? [];
  if (finalTruth.auction.status !== 'SOLD' || finalTruth.auction.current_price_cents !== 60000 || finalTruth.auction.current_winner_id !== 'user_1') {
    throw new Error(`final auction truth wrong: ${JSON.stringify(finalTruth.auction)}`);
  }
  if (finalTruth.leaderboard.entries?.[0]?.user_id !== 'user_1' || finalTruth.leaderboard.entries?.[0]?.amount_cents !== 60000) {
    throw new Error(`final leaderboard wrong: ${JSON.stringify(finalTruth.leaderboard)}`);
  }
  if (orders.length !== 1 || orders[0].amount_cents !== 60000 || String(orders[0].order_status ?? orders[0].status) !== 'PAID') {
    throw new Error(`final order wrong: ${JSON.stringify(orders)}`);
  }
  await fs.writeFile(path.join(outDir, 'result.json'), JSON.stringify({ evidence, finalTruth }, null, 2));
  await browser.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
