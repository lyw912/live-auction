import { expect, test } from '@playwright/test';

test('H5 covers live backend REST, fat-finger confirm, cap SOLD order, payment, and WebSocket event paths', async ({ page }) => {
  const login = await page.request.post('/api/auth/login', {
    data: { account: 'user' }
  });
  expect(login.ok()).toBeTruthy();
  await page.goto('/rooms/room_main');
  const roomAuctions = await page.request.get('/api/rooms/room_main/auctions');
  expect(roomAuctions.ok()).toBeTruthy();
  const roomPayload = await roomAuctions.json();
  expect(roomPayload).toEqual(expect.arrayContaining([
    expect.objectContaining({
      id: 'auc_live',
      room_id: 'room_main',
      status: 'ACTIVE'
    })
  ]));

  const snapshot = await page.request.get('/api/auctions/auc_live');
  expect(snapshot.ok()).toBeTruthy();
  expect(await snapshot.json()).toEqual(expect.objectContaining({
    id: 'auc_live',
    seq: 0,
    current_price_cents: 35000
  }));

  await expect(page.getByText('¥350.00')).toBeVisible();
  await expect(page.getByTestId('floating-product-card')).toBeVisible();
  await expect(page.getByTestId('floating-auction-price')).toHaveText('当前最高价 ¥350.00');
  await expect(page.getByTestId('floating-auction-countdown')).toBeVisible();
  await expect(page.getByTestId('stage-chat-overlay').getByText('这件拍品状态不错')).toBeVisible();
  await expect(page.getByTestId('stage-chat-overlay').locator('strong').filter({ hasText: '匿名买家' }).first()).toBeVisible();
  await page.getByLabel('chat-input').fill('live smoke chat');
  await page.getByRole('button', { name: 'send-chat' }).click();
  await expect(page.getByTestId('stage-chat-overlay').getByText('live smoke chat')).toBeVisible();

  await page.getByTestId('floating-product-card').click();
  await expect(page.getByLabel('auction-state').getByText('竞价中')).toBeVisible();
  await expect(page.getByLabel('auction-state').getByText('已连接')).toBeVisible();
  await expect(page.getByTestId('auction-countdown')).toBeVisible();
  await page.getByRole('button', { name: 'increase' }).click();
  await page.getByRole('button', { name: 'increase' }).click();
  await page.getByRole('button', { name: 'increase' }).click();
  await page.getByRole('button', { name: 'increase' }).click();
  await expect(page.getByTestId('bid-cta')).toHaveText(/¥600.00/);
  await page.getByTestId('bid-cta').click();
  await expect(page.getByTestId('bid-cta')).toHaveText(/确认高额出价/);
  await expect(page.getByLabel('auction-state').getByRole('heading', { name: '¥350.00' })).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveText(/确认高额出价/);

  await page.getByTestId('bid-cta').click();
  await expect(page.getByLabel('auction-state')).toContainText('等待服务端确认高额出价');
  await expect(page.getByLabel('auction-state').getByRole('heading', { name: '¥350.00' })).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('成交');
  await expect(page.getByLabel('auction-state').getByRole('heading', { name: '¥600.00' })).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveText(/去支付/);
  await expect(page.getByTestId('bid-cta')).toBeEnabled();

  const bids = await page.request.get('/api/users/me/bids');
  expect(bids.ok()).toBeTruthy();
  const bidPayload = await bids.json();
  const acceptedSoldBid = bidPayload.items.find((row: { auction_id: string; amount_cents: number }) => (
    row.auction_id === 'auc_live' && row.amount_cents === 60000
  ));
  expect(acceptedSoldBid).toMatchObject({
    auction_id: 'auc_live',
    amount_cents: 60000,
    result: 'ACCEPTED_SOLD'
  });

  const pendingOrders = await page.request.get('/api/users/me/orders');
  expect(pendingOrders.ok()).toBeTruthy();
  const pendingOrderPayload = await pendingOrders.json();
  const pendingOrder = pendingOrderPayload.items.find((row: { auction_id: string; amount_cents: number }) => (
    row.auction_id === 'auc_live' && row.amount_cents === 60000
  ));
  expect(pendingOrder).toMatchObject({
    auction_id: 'auc_live',
    amount_cents: 60000,
    order_status: 'ORDER_PENDING'
  });

  await page.getByTestId('bid-cta').click();
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('已支付');
  await expect(page.getByLabel('auction-state').locator('.dock-feedback')).toContainText('保证金已处理');

  const orders = await page.request.get('/api/users/me/orders');
  expect(orders.ok()).toBeTruthy();
  const orderPayload = await orders.json();
  expect(orderPayload).toEqual(expect.objectContaining({
    items: expect.arrayContaining([
      expect.objectContaining({
        auction_id: 'auc_live',
        amount_cents: 60000,
        order_status: 'PAID'
      })
    ])
  }));
});

test('H5 route isolates two room contexts', async ({ page }) => {
  const login = await page.request.post('/api/auth/login', {
    data: { account: 'user' }
  });
  expect(login.ok()).toBeTruthy();
  await page.goto('/rooms/room_side');
  const sideAuctions = await page.request.get('/api/rooms/room_side/auctions');
  expect(sideAuctions.ok()).toBeTruthy();
  const sidePayload = await sideAuctions.json();
  expect(sidePayload).toEqual(expect.arrayContaining([
    expect.objectContaining({
      id: 'auc_side',
      room_id: 'room_side'
    })
  ]));
  await expect(page.getByText('auc_live')).not.toBeVisible();
  await expect(page.getByLabel('live-stage').getByText('专场 side')).toBeVisible();
  await expect(page.getByLabel('live-stage').getByRole('heading', { name: '和田玉福牌吊坠' })).toBeVisible();
  await expect(page.getByTestId('stage-chat-overlay').getByText('侧房间独立弹幕')).toBeVisible();
});
