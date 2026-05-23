import { expect, test } from '@playwright/test';

const auctionLive = {
  id: 'auc_live',
  room_id: 'room_main',
  item_id: 'item_live',
  status: 'ACTIVE',
  is_narrating: true,
  current_price_cents: 45000,
  current_winner_id: 'user_2',
  start_price_cents: 10000,
  increment_cents: 5000,
  cap_price_cents: 60000,
  seq: 42,
  accepted_bid_count: 18,
  extend_count: 0,
  end_at: '2026-05-22T14:00:00Z',
  item: { id: 'item_live', title: '青瓷手作茶盏', description: 'demo' },
  rule: {
    duration_seconds: 600,
    extend_window_seconds: 10,
    extend_by_seconds: 10,
    max_extend_count: 3,
    fat_finger_threshold_cents: 100000,
    deposit_bps: 1000,
    deposit_floor_cents: 5000,
    deposit_cap_cents: 50000,
    frozen_at: '2026-05-22T13:00:00Z'
  }
};

const auctionDraft = {
  ...auctionLive,
  id: 'auc_next',
  item_id: 'item_next',
  status: 'DRAFT',
  is_narrating: false,
  current_price_cents: 80000,
  current_winner_id: undefined,
  item: { id: 'item_next', title: '紫砂壶', description: 'next' },
  rule: {
    ...auctionLive.rule,
    frozen_at: undefined
  }
};

test.beforeEach(async ({ page }) => {
  await page.route('/api/auth/me', async (route) => {
    await route.fulfill({ json: { user: { ID: 'host_1', Role: 'host' } } });
  });
  await page.route('/api/auth/login', async (route) => {
    await route.fulfill({ json: { user: { ID: 'host_1', Role: 'host' }, expires_in_ms: 43200000 } });
  });
  await page.route('/api/rooms', async (route) => {
    await route.fulfill({ json: { items: [{ id: 'room_main', host_id: 'host_1', status: 'OPEN', role: 'host' }] } });
  });
  await page.route('/api/auctions?room_id=room_main', async (route) => route.fulfill({ json: [auctionLive, auctionDraft] }));
  await page.route('/api/orders', async (route) => route.fulfill({
    json: [{ id: 'ord_pending', auction_id: 'auc_live', winner_id: 'user_1', amount_cents: 60000, status: 'ORDER_PENDING', deposit_status: 'HELD' }]
  }));
  await page.route('/api/monitor/auctions', async (route) => route.fulfill({
    json: { items: [{ auction_id: 'auc_live', room_id: 'room_main', status: 'ACTIVE', current_price_cents: 45000, seq: 42 }] }
  }));
  await page.route(/\/api\/monitor\/anomalies(\?.*)?$/, async (route) => route.fulfill({
    json: { items: [{ id: 1, severity: 'HIGH', type: 'CLOCK_STEP_BACKWARD', message: 'scheduler detected clock step backward' }] }
  }));
  await page.route('/api/monitor/outbox', async (route) => route.fulfill({
    json: { items: [{ outbox_id: 7, aggregate_id: 'auc_live', status: 'PENDING', attempts: 0, lag_ms: 1200 }] }
  }));
  await page.route('/api/monitor/scheduler', async (route) => route.fulfill({
    json: { items: [{ job_id: 'job_1', job_type: 'END_AUCTION', target_id: 'auc_live', status: 'PENDING' }] }
  }));
  await page.route('/api/monitor/rejects', async (route) => route.fulfill({
    json: { items: [{ time: '2026-05-22T13:00:01Z', auction_id: 'auc_live', user_id: 'user_1', amount_cents: 1, current_price_cents: 45000, reject_reason: 'BID_TOO_LOW', trace_id: 'tr_reject' }] }
  }));
  await page.route('/api/monitor/recovery', async (route) => route.fulfill({
    json: { items: [{ room_id: 'room_main', reconnect_count_recent: 3, history_recovered: 2, snapshot_recovered: 1, snapshot_from_db: 1, snapshot_stale: 0, slow_consumer_disconnects: 0 }] }
  }));
});

test('PC console renders live API auctions, orders, and expanded diagnostic panels', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByText('青瓷手作茶盏')).toBeVisible();
  await expect(page.getByText('紫砂壶')).toBeVisible();
  await expect(page.getByText('ord_pending')).toBeVisible();
  await expect(page.getByTestId('diagnostics')).toBeVisible();
  await expect(page.getByLabel('monitor-anomaly-type')).toBeVisible();

  await page.getByRole('tab', { name: 'Rejects' }).click();
  await expect(page.getByText('BID_TOO_LOW')).toBeVisible();
  await expect(page.getByText('tr_reject')).toBeVisible();

  await page.getByRole('tab', { name: 'Recovery' }).click();
  await expect(page.getByLabel('Recovery').getByText('room_main')).toBeVisible();
  await expect(page.getByText('snapshot_from_db')).toBeVisible();
});

test('PC creates item and auction through backend upload and create APIs', async ({ page }) => {
  let uploadURLBody: Record<string, unknown> | undefined;
  let uploadedBytes = 0;
  let itemBody: Record<string, unknown> | undefined;
  let auctionBody: Record<string, unknown> | undefined;
  await page.route('/api/items/upload-url', async (route, request) => {
    uploadURLBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({
      json: {
        upload_url: 'http://upload.local/item-created.jpg',
        public_url: 'http://cdn.local/item-created.jpg'
      }
    });
  });
  await page.route('http://upload.local/item-created.jpg', async (route, request) => {
    uploadedBytes = (request.postDataBuffer() ?? Buffer.from([])).byteLength;
    await route.fulfill({ status: 200, body: '' });
  });
  await page.route('/api/items', async (route, request) => {
    itemBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({ status: 201, json: { id: 'item_created', title: itemBody.title, description: itemBody.description, image_url: itemBody.image_url, status: 'READY' } });
  });
  await page.route('/api/auctions', async (route, request) => {
    auctionBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({ status: 201, json: { ...auctionDraft, id: 'auc_created', item: { id: 'item_created', title: itemBody?.title } } });
  });

  await page.goto('/');
  await page.getByLabel('item-title').fill('白瓷杯');
  await page.getByLabel('item-image-file').setInputFiles({
    name: 'cup.jpg',
    mimeType: 'image/jpeg',
    buffer: Buffer.from('fake image bytes')
  });
  await page.getByRole('button', { name: '创建拍品和竞拍' }).click();
  await expect.poll(() => uploadURLBody?.object_name).toContain('cup.jpg');
  await expect.poll(() => uploadedBytes).toBeGreaterThan(0);
  await expect.poll(() => itemBody?.title).toBe('白瓷杯');
  expect(itemBody?.image_url).toBe('http://cdn.local/item-created.jpg');
  await expect.poll(() => auctionBody?.room_id).toBe('room_main');
  expect(auctionBody?.room_id).toBe('room_main');
  expect(auctionBody?.item_id).toBe('item_created');
  expect((auctionBody?.rule as Record<string, unknown>).duration_seconds).toBe(600);
});

test('PC rule save targets selected draft auction and includes all money/rule fields', async ({ page }) => {
  let saveBody: Record<string, unknown> | undefined;
  await page.route('/api/auctions/auc_next/rules', async (route, request) => {
    saveBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({ json: { ...auctionDraft, start_price_cents: saveBody.start_price_cents, increment_cents: saveBody.increment_cents, cap_price_cents: saveBody.cap_price_cents } });
  });

  await page.goto('/');
  await page.getByText('紫砂壶').click();
  await page.getByLabel('start-price-cents').fill('20000');
  await page.getByLabel('increment-cents').fill('10000');
  await page.getByLabel('cap-price-cents').fill('70000');
  await page.getByRole('button', { name: '保存规则' }).click();

  await expect(page.getByText('规则已保存')).toBeVisible();
  expect(saveBody?.start_price_cents).toBe(20000);
  expect(saveBody?.increment_cents).toBe(10000);
  expect(saveBody?.cap_price_cents).toBe(70000);
  expect(saveBody?.duration_seconds).toBe(600);
  expect(saveBody?.deposit_cap_cents).toBe(50000);
});

test('PC lifecycle controls call selected auction APIs', async ({ page }) => {
  let scheduleBody: Record<string, unknown> | undefined;
  let cancelBody: Record<string, unknown> | undefined;
  await page.route('/api/auctions/auc_next/schedule', async (route, request) => {
    scheduleBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({ json: { ...auctionDraft, status: 'SCHEDULED' } });
  });
  await page.route('/api/auctions/auc_next/cancel', async (route, request) => {
    cancelBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({ json: { ...auctionDraft, status: 'CANCELLED' } });
  });

  await page.goto('/');
  await page.getByText('紫砂壶').click();
  await page.getByLabel('schedule-start-at').fill('2026-05-22T14:30');
  await page.getByRole('button', { name: '排期' }).click();
  await expect.poll(() => scheduleBody?.start_at).toBe('2026-05-22T06:30:00.000Z');
  await page.getByLabel('cancel-reason').fill('主播临时下架');
  await page.getByRole('button', { name: '取消' }).click();
  await page.getByRole('dialog', { name: '确认取消竞拍' }).getByRole('button', { name: '确定' }).click();
  await expect.poll(() => cancelBody?.reason).toBe('主播临时下架');
});
