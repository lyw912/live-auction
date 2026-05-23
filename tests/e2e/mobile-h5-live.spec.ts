import { expect, test } from '@playwright/test';

test('H5 covers live backend REST, fat-finger confirm, cap SOLD order, payment, and WebSocket event paths', async ({ page }) => {
  await page.goto('/rooms/room_main');
  const login = await page.request.post('/api/auth/login', {
    data: { account: 'user' }
  });
  expect(login.ok()).toBeTruthy();
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
    seq: 41,
    current_price_cents: 35000
  }));

  await expect(page.getByText('WebSocket 已连接 · 状态来自服务端事件')).toBeVisible();
  await expect(page.getByText('¥350.00')).toBeVisible();
  await expect(page.getByTestId('chat-panel').getByText('这件拍品状态不错')).toBeVisible();
  await page.getByLabel('chat-input').fill('live smoke chat');
  await page.getByRole('button', { name: 'send-chat' }).click();
  await expect(page.getByTestId('chat-panel').getByText('live smoke chat')).toBeVisible();

  await page.getByRole('button', { name: 'increase' }).click();
  await page.getByRole('button', { name: 'increase' }).click();
  await page.getByRole('button', { name: 'increase' }).click();
  await page.getByRole('button', { name: 'increase' }).click();
  await expect(page.getByTestId('bid-cta')).toHaveText(/¥600.00/);
  await page.getByTestId('bid-cta').click();
  await expect(page.getByText('确认 ¥600.00 出价')).toBeVisible();
  await expect(page.getByText('¥350.00')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveText(/确认高额出价/);

  await page.getByTestId('bid-cta').click();
  await expect(page.getByText('等待服务端确认高额出价')).toBeVisible();
  await expect(page.getByText('¥350.00')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('成交');
  await expect(page.getByText('¥600.00')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveText(/去支付/);
  await expect(page.getByTestId('bid-cta')).toBeEnabled();

  await page.getByTestId('history-panel').getByRole('button', { name: /刷新/ }).click();
  await expect(page.getByTestId('history-panel').getByText('auc_live')).toBeVisible();
  await expect(page.getByText('¥600.00 · ACCEPTED_SOLD').first()).toBeVisible();
  await expect(page.getByText('¥600.00 · ORDER_PENDING')).toBeVisible();

  await page.getByTestId('bid-cta').click();
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('已支付');
  await expect(page.getByText('保证金已处理')).toBeVisible();

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
  await page.goto('/rooms/room_side');
  const login = await page.request.post('/api/auth/login', {
    data: { account: 'user' }
  });
  expect(login.ok()).toBeTruthy();
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
  await expect(page.getByText('Side Room Smoke Item')).toBeVisible();
  await expect(page.getByTestId('chat-panel').getByText('侧房间独立弹幕')).toBeVisible();
});
