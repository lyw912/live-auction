import { expect, request, test } from '@playwright/test';
import fs from 'node:fs/promises';
import { dirname, join } from 'node:path';

const root = join(__dirname, '..', '..');
const evidencePath = join(root, 'docs', 'perf', 'raw', 'p10-no-mock-live-smoke.json');

async function writeEvidence(record: {
  auctionID: string;
  itemID: string;
  roomID: string;
  title: string;
  imageURL: string;
  videoAssetPath: string;
}) {
  await fs.mkdir(dirname(evidencePath), { recursive: true });
  await fs.writeFile(evidencePath, `${JSON.stringify({
    gate: 'P10 no-mock live backend smoke',
    generated_at: new Date().toISOString(),
    backend_url: process.env.LIVE_AUCTION_API_TARGET || 'http://127.0.0.1:18080',
    h5_url: process.env.LIVE_AUCTION_H5_URL || 'http://127.0.0.1:5276',
    pc_url: process.env.LIVE_AUCTION_PC_URL || 'http://127.0.0.1:5277',
    no_browser_route_mocks: true,
    local_identity_setup: 'host/user session login plus X-Mock-User-Id for the second seeded demo bidder',
    room_id: record.roomID,
    smoke_auction_id: record.auctionID,
    smoke_item_id: record.itemID,
    smoke_item_title: record.title,
    image_asset_path: record.imageURL,
    video_asset_path: record.videoAssetPath,
    flight_recorder_path: `/api/monitor/auctions/${record.auctionID}/flight-recorder?limit=80&timeline_limit=120`,
    proved: [
      'host-created item through real backend API',
      'host-created auction through real backend API',
      'schedule/start through real backend API',
      'H5 loaded room and auction through real backend API',
      'WebSocket connected without route interception',
      'bid rejection persisted and visible in host-only flight recorder',
      'competing seeded bidder created the cap SOLD bid',
      'cap bid created SOLD auction and pending order'
    ],
    result: 'PASS'
  }, null, 2)}\n`, 'utf8');
}

test('P10 no-mock demo creates auction and proves H5 plus flight recorder path', async ({ page }) => {
  const hostAPI = await request.newContext({
    baseURL: process.env.LIVE_AUCTION_API_TARGET || 'http://127.0.0.1:18080'
  });
  const competingBidderAPI = await request.newContext({
    baseURL: process.env.LIVE_AUCTION_API_TARGET || 'http://127.0.0.1:18080',
    extraHTTPHeaders: {
      'X-Mock-Role': 'user',
      'X-Mock-User-Id': 'user_2'
    }
  });
  const bidderAPI = page.request;
  const suffix = Date.now();
  const roomID = `room_p10_${suffix}`;
  const title = `P10 No-Mock Tea Cup ${suffix}`;
  const imageURL = 'docs/demo/assets/ceramic-tea-cup.jpg';
  const videoAssetPath = 'docs/demo/assets/pottery-live-loop.webm';

  try {
    const hostLogin = await hostAPI.post('/api/auth/login', { data: { account: 'host' } });
    expect(hostLogin.ok()).toBeTruthy();

    const roomSetup = await hostAPI.post('/api/test/rooms', {
      data: {
        room_id: roomID,
        host_id: 'host_1',
        users: ['user_1', 'user_2']
      }
    });
    expect(roomSetup.ok()).toBeTruthy();

    const itemResponse = await hostAPI.post('/api/items', {
      data: {
        title,
        image_url: imageURL,
        description: 'P10 judge demo item created through the live backend API with downloaded local media asset.'
      }
    });
    expect(itemResponse.status()).toBe(201);
    const item = await itemResponse.json();
    expect(item.id).toMatch(/^item_/);

    const capPriceCents = 45000;
    const auctionResponse = await hostAPI.post('/api/auctions', {
      data: {
        room_id: roomID,
        item_id: item.id,
        start_price_cents: 20000,
        increment_cents: 5000,
        cap_price_cents: capPriceCents,
        rule: {
          duration_seconds: 600,
          extend_window_seconds: 10,
          extend_by_seconds: 10,
          max_extend_count: 3,
          fat_finger_threshold_cents: 100000,
          deposit_bps: 1000,
          deposit_floor_cents: 5000,
          deposit_cap_cents: 50000
        }
      }
    });
    expect(auctionResponse.status()).toBe(201);
    const createdAuction = await auctionResponse.json();
    expect(createdAuction.id).toMatch(/^auc_/);

    const schedule = await hostAPI.post(`/api/auctions/${createdAuction.id}/schedule`, { data: { start_at: null } });
    expect(schedule.ok()).toBeTruthy();
    const start = await hostAPI.post(`/api/auctions/${createdAuction.id}/start`, { data: {} });
    expect(start.ok()).toBeTruthy();
    const startedAuction = await start.json();
    expect(startedAuction).toMatchObject({
      id: createdAuction.id,
      room_id: roomID,
      status: 'ACTIVE'
    });

    const bidderLogin = await bidderAPI.post('/api/auth/login', { data: { account: 'user' } });
    expect(bidderLogin.ok()).toBeTruthy();

    const roomAuctions = await bidderAPI.get(`/api/rooms/${roomID}/auctions`);
    expect(roomAuctions.ok()).toBeTruthy();
    await expect.poll(async () => {
      const payload = await (await bidderAPI.get(`/api/rooms/${roomID}/auctions`)).json();
      return payload.find((row: { id: string }) => row.id === createdAuction.id)?.status;
    }).toBe('ACTIVE');

    await page.goto(`/rooms/${roomID}`);
    await expect(page.getByTestId('floating-product-card')).toBeVisible();
    await expect(page.getByRole('heading', { name: title })).toBeVisible();
    await expect(page.getByTestId('floating-auction-status')).toContainText('ACTIVE');
    await expect(page.getByTestId('floating-auction-price')).toHaveText('当前最高价 ¥200.00');
    await page.getByTestId('floating-product-card').click();
    await expect(page.getByLabel('auction-state').getByText('ACTIVE')).toBeVisible();
    await expect(page.getByText('WebSocket 已连接 · 状态来自服务端事件')).toBeVisible();
    await expect(page.getByTestId('auction-price')).toHaveText('¥200.00');

    const rejectBidID = `p10-reject-${suffix}`;
    const rejectBid = await bidderAPI.post(`/api/auctions/${createdAuction.id}/bids`, {
      headers: { 'Idempotency-Key': rejectBidID },
      data: {
        client_bid_id: rejectBidID,
        amount_cents: 25001,
        client_seen_seq: startedAuction.seq
      }
    });
    expect(rejectBid.ok()).toBeTruthy();
    const rejectPayload = await rejectBid.json();
    expect(rejectPayload).toMatchObject({
      result: 'REJECTED',
      reject_reason: 'BID_INCREMENT_MISMATCH'
    });

    const normalBidID = `p10-normal-${suffix}`;
    const normalBid = await bidderAPI.post(`/api/auctions/${createdAuction.id}/bids`, {
      headers: { 'Idempotency-Key': normalBidID },
      data: {
        client_bid_id: normalBidID,
        amount_cents: 25000,
        client_seen_seq: rejectPayload.seq
      }
    });
    expect(normalBid.ok()).toBeTruthy();
    const normalPayload = await normalBid.json();
    expect(normalPayload).toMatchObject({
      result: 'ACCEPTED',
      auction_id: createdAuction.id,
      current_price_cents: 25000
    });
    await expect.poll(async () => (await (await bidderAPI.get(`/api/auctions/${createdAuction.id}`)).json()).current_price_cents).toBe(25000);
    await expect(page.getByTestId('auction-price')).toHaveText('¥250.00');

    const soldBidID = `p10-sold-${suffix}`;
    const soldBid = await competingBidderAPI.post(`/api/auctions/${createdAuction.id}/bids`, {
      headers: { 'Idempotency-Key': soldBidID },
      data: {
        client_bid_id: soldBidID,
        amount_cents: capPriceCents,
        client_seen_seq: normalPayload.seq
      }
    });
    expect(soldBid.ok()).toBeTruthy();
    const soldPayload = await soldBid.json();
    expect(soldPayload).toMatchObject({
      result: 'ACCEPTED_SOLD',
      auction_id: createdAuction.id,
      current_price_cents: capPriceCents
    });

    await expect.poll(async () => (await (await bidderAPI.get(`/api/auctions/${createdAuction.id}`)).json()).status).toBe('SOLD');
    await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText(/成交/);
    await expect(page.getByTestId('bid-cta')).toHaveText(/已结束|同步订单中|去支付/);

    const flightRecorder = await hostAPI.get(`/api/monitor/auctions/${createdAuction.id}/flight-recorder?limit=80&timeline_limit=120`);
    expect(flightRecorder.ok()).toBeTruthy();
    const recorderPayload = await flightRecorder.json();
    expect(recorderPayload.summary).toMatchObject({
      auction_id: createdAuction.id,
      item_id: item.id,
      status: 'SOLD'
    });
    expect(recorderPayload.timeline).toEqual(expect.arrayContaining([
      expect.objectContaining({ event_type: 'auction_created' }),
      expect.objectContaining({ event_type: 'auction_started' }),
      expect.objectContaining({ event_type: 'bid_accepted' }),
      expect.objectContaining({ event_type: 'bid_rejected_row' }),
      expect.objectContaining({ event_type: 'auction_sold' })
    ]));
    expect(recorderPayload.orders).toEqual(expect.arrayContaining([
      expect.objectContaining({
        amount_cents: capPriceCents,
        status: 'ORDER_PENDING'
      })
    ]));

    await writeEvidence({
      auctionID: createdAuction.id,
      itemID: item.id,
      roomID,
      title,
      imageURL,
      videoAssetPath
    });
  } finally {
    await hostAPI.dispose();
    await competingBidderAPI.dispose();
  }
});
