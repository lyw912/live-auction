import { chromium, devices, expect } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const OUT = path.resolve('docs/reviews/极致竞价氛围-3.0-2026-06-07/evidence');
const H5_URL = process.env.H5_URL || 'http://127.0.0.1:5276/';
const productImageDataURL = 'data:image/svg+xml;utf8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%20600%20800%22%3E%3Crect%20width%3D%22600%22%20height%3D%22800%22%20fill%3D%22%23222f2b%22%2F%3E%3Ccircle%20cx%3D%22300%22%20cy%3D%22300%22%20r%3D%22142%22%20fill%3D%22%23e5f3ef%22%2F%3E%3Cellipse%20cx%3D%22300%22%20cy%3D%22320%22%20rx%3D%22164%22%20ry%3D%2286%22%20fill%3D%22%2310b981%22%2F%3E%3Cpath%20d%3D%22M170%20352c52%2068%20208%2068%20260%200%22%20stroke%3D%22%23d6a84f%22%20stroke-width%3D%2218%22%20fill%3D%22none%22%20stroke-linecap%3D%22round%22%2F%3E%3Ctext%20x%3D%22300%22%20y%3D%22640%22%20text-anchor%3D%22middle%22%20font-size%3D%2248%22%20font-family%3D%22Arial%22%20fill%3D%22white%22%3ELOT%20A-102%3C%2Ftext%3E%3C%2Fsvg%3E';

async function installRoutes(page) {
  await page.route('**/api/auth/me', async (route) => {
    await route.fulfill({ json: { user: { ID: 'user_1', Role: 'user' } } });
  });
  await page.route('**/api/auth/login', async (route) => {
    await route.fulfill({ json: { user: { ID: 'user_1', Role: 'user' }, expires_in_ms: 43_200_000 } });
  });
  await page.route('**/api/auth/ws-ticket', async (route) => {
    await route.fulfill({ json: { ticket: 'ticket_visual', expires_in_ms: 60_000 } });
  });
  await page.route('**/api/rooms/room_main/auctions', async (route) => {
    await route.fulfill({ json: [{
      id: 'auc_live',
      room_id: 'room_main',
      status: 'ACTIVE',
      current_price_cents: 35000,
      increment_cents: 5000,
      accepted_bid_count: 3,
      seq: 41,
      end_at: '2099-05-22T14:00:00Z',
      item: {
        title: '天然翡翠A货平安扣吊坠',
        image_url: productImageDataURL,
        certificate: 'GID 20260607 · 可核验',
        condition: '品相完整',
        shipping: '顺丰包邮'
      }
    }] });
  });
  await page.route('**/api/auctions/auc_live/leaderboard?limit=5', async (route) => {
    await route.fulfill({ json: {
      auction_id: 'auc_live',
      seq: 42,
      current_price_cents: 35000,
      current_winner_id: 'user_2',
      my_rank: 2,
      my_best_amount_cents: 30000,
      gap_to_leader_cents: 5000,
      next_valid_bid_cents: 40000,
      state: 'OUTBID',
      leader_amount_cents: 35000,
      accepted_bidder_count: 2,
      active_bidders_30s: 2,
      accepted_bids_30s: 3,
      price_velocity_cents_per_min: 10000,
      entries: [
        { rank: 1, user_id: 'user_2', user_masked: '张**', amount_cents: 35000, bid_count: 2 },
        { rank: 2, user_id: 'user_1', user_masked: '我', amount_cents: 30000, bid_count: 1 }
      ]
    } });
  });
  await page.route('**/api/rooms/room_main/system-messages?limit=10', async (route) => {
    await route.fulfill({ json: { items: [
      { id: 91, room_id: 'room_main', auction_id: 'auc_live', source: 'SYSTEM_AI', source_seq: 45, style: 'heat', body: '榜一被偷了！陈** ¥570 反超，差一口就能追回。', created_at: '2026-06-07T14:00:00Z' },
      { id: 90, room_id: 'room_main', auction_id: 'auc_live', source: 'SYSTEM_TEMPLATE', source_seq: 44, style: 'steady', body: '最高有效价优先，同额先到先得。', created_at: '2026-06-07T13:59:59Z' }
    ] } });
  });
  await page.route('**/api/rooms/room_main/chat?limit=30', async (route) => {
    await route.fulfill({ json: { items: [] } });
  });
  await page.route('**/api/users/me/orders', async (route) => {
    await route.fulfill({ json: { items: [] } });
  });
  await page.addInitScript(() => {
    class VisualAuctionWebSocket extends EventTarget {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSED = 3;
      CONNECTING = 0;
      OPEN = 1;
      CLOSING = 2;
      CLOSED = 3;
      binaryType = 'blob';
      bufferedAmount = 0;
      extensions = '';
      protocol = 'auction.v1';
      readyState = VisualAuctionWebSocket.CONNECTING;
      onopen = null;
      onmessage = null;
      onerror = null;
      onclose = null;
      url;

      constructor(url, protocols) {
        super();
        this.url = String(url);
        const protocolList = Array.isArray(protocols) ? protocols : protocols ? [protocols] : [];
        window.__auctionWS = [...(window.__auctionWS ?? []), { url: this.url, protocols: protocolList, socket: this }];
        window.setTimeout(() => {
          this.readyState = VisualAuctionWebSocket.OPEN;
          const event = new Event('open');
          this.onopen?.(event);
          this.dispatchEvent(event);
        }, 0);
      }

      send() {}
      close() {
        this.readyState = VisualAuctionWebSocket.CLOSED;
        const event = new CloseEvent('close');
        this.onclose?.(event);
        this.dispatchEvent(event);
      }
      dispatchServerMessage(payload) {
        const event = new MessageEvent('message', { data: JSON.stringify(payload) });
        this.onmessage?.(event);
        this.dispatchEvent(event);
      }
    }
    window.WebSocket = VisualAuctionWebSocket;
  });
}

async function capture(width, file) {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    ...devices['iPhone 13'],
    viewport: { width, height: 844 },
    locale: 'zh-CN'
  });
  const page = await context.newPage();
  await installRoutes(page);
  await page.goto(H5_URL);
  await expect(page.getByTestId('race-board')).toBeVisible();
  await expect.poll(async () => page.evaluate(() => Boolean(window.__auctionWS?.some(({ url }) => url.includes('/ws?'))))).toBe(true);
  await page.evaluate(() => {
    const [entry] = window.__auctionWS.filter(({ url }) => url.includes('/ws?'));
    entry.socket.dispatchServerMessage({
      auction_id: 'auc_live',
      event_type: 'leaderboard_delta',
      seq: 44,
      current_price_cents: 52000,
      next_valid_bid_cents: 57000,
      current_winner_id: 'user_1',
      leader_amount_cents: 52000,
      accepted_bidder_count: 3,
      active_bidders_30s: 3,
      accepted_bids_30s: 6,
      price_velocity_cents_per_min: 12000,
      entries: [
        { rank: 1, user_id: 'user_1', user_masked: '我**', amount_cents: 52000, bid_count: 4 },
        { rank: 2, user_id: 'user_2', user_masked: '陈**', amount_cents: 50000, bid_count: 3 }
      ]
    });
    entry.socket.dispatchServerMessage({
      auction_id: 'auc_live',
      event_type: 'leaderboard_delta',
      seq: 45,
      current_price_cents: 57000,
      next_valid_bid_cents: 62000,
      current_winner_id: 'user_2',
      leader_amount_cents: 57000,
      accepted_bidder_count: 3,
      active_bidders_30s: 3,
      accepted_bids_30s: 7,
      price_velocity_cents_per_min: 15000,
      entries: [
        { rank: 1, user_id: 'user_2', user_masked: '陈**', amount_cents: 57000, bid_count: 4 },
        { rank: 2, user_id: 'user_1', user_masked: '我**', amount_cents: 52000, bid_count: 4 }
      ]
    });
  });
  await expect(page.getByTestId('race-board')).toContainText('我 #2 差 ¥50.00');
  await expect(page.getByTestId('cohost-ribbon')).toContainText('气氛官');
  await page.waitForTimeout(240);
  await page.screenshot({ path: path.join(OUT, file), fullPage: false });
  await browser.close();
}

await fs.mkdir(OUT, { recursive: true });
await capture(390, '08-race-board-waterfall-ai-390.png');
await capture(360, '09-race-board-waterfall-ai-360.png');
console.log(JSON.stringify({ ok: true, out: OUT }, null, 2));
