import { expect, test } from '@playwright/test';

test('H5 covers live backend REST, fat-finger confirm, payment, and WebSocket event paths', async ({ page }) => {
  await page.goto('/');
  const roomAuctions = await page.request.get('/api/rooms/room_main/auctions', {
    headers: {
      'X-Mock-Role': 'user',
      'X-Mock-User-Id': 'user_1'
    }
  });
  expect(roomAuctions.ok()).toBeTruthy();
  const roomPayload = await roomAuctions.json();
  expect(roomPayload).toEqual(expect.arrayContaining([
    expect.objectContaining({
      id: 'auc_live',
      room_id: 'room_main',
      status: 'ACTIVE'
    })
  ]));

  const snapshot = await page.request.get('/api/auctions/auc_live', {
    headers: {
      'X-Mock-Role': 'user',
      'X-Mock-User-Id': 'user_1'
    }
  });
  expect(snapshot.ok()).toBeTruthy();
  expect(await snapshot.json()).toEqual(expect.objectContaining({
    id: 'auc_live',
    seq: 41,
    current_price_cents: 35000
  }));

  await expect(page.getByText('WebSocket 已连接 · 状态来自服务端事件')).toBeVisible();
  await expect(page.getByText('¥350.00')).toBeVisible();

  await page.getByRole('button', { name: 'increase' }).click();
  await page.getByRole('button', { name: 'increase' }).click();
  await expect(page.getByTestId('bid-cta')).toHaveText(/¥500.00/);
  await page.getByTestId('bid-cta').click();
  await expect(page.getByText('确认 ¥500.00 出价')).toBeVisible();
  await expect(page.getByText('¥350.00')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveText(/确认高额出价/);

  await page.getByTestId('bid-cta').click();
  await expect(page.getByText('等待服务端确认高额出价')).toBeVisible();
  await expect(page.getByText('¥350.00')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await expect(page.getByText(/服务端确认 seq/)).toBeVisible();
  await expect(page.getByText('¥500.00')).toBeVisible();

  const clientBidID = `live-smoke-${Date.now()}`;
  const response = await page.request.post('/api/auctions/auc_live/bids', {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': clientBidID,
      'X-Mock-Role': 'user',
      'X-Mock-User-Id': 'user_2'
    },
    data: {
      client_bid_id: clientBidID,
      amount_cents: 55000,
      client_seen_seq: 42
    }
  });
  expect(response.ok()).toBeTruthy();
  const payload = await response.json();
  expect(payload).toEqual(expect.objectContaining({
    result: 'ACCEPTED',
    auction_id: 'auc_live',
    current_price_cents: 55000
  }));

  await expect(page.getByText(/event seq/)).toBeVisible();
  await expect(page.getByText('¥550.00')).toBeVisible();

  await page.getByTestId('history-panel').getByRole('button', { name: /刷新/ }).click();
  await expect(page.getByTestId('history-panel').getByText('auc_live')).toBeVisible();
  await expect(page.getByText('¥500.00 · ACCEPTED').first()).toBeVisible();
  await expect(page.getByText('ord_pending')).toBeVisible();
  await expect(page.getByText('¥600.00 · ORDER_PENDING')).toBeVisible();

  await page.getByRole('button', { name: '成交', exact: true }).click();
  await page.getByTestId('bid-cta').click();
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('已支付');
  await expect(page.getByText('保证金已处理')).toBeVisible();

  const orders = await page.request.get('/api/users/me/orders', {
    headers: {
      'X-Mock-Role': 'user',
      'X-Mock-User-Id': 'user_1'
    }
  });
  expect(orders.ok()).toBeTruthy();
  expect(await orders.json()).toEqual(expect.objectContaining({
    items: expect.arrayContaining([
      expect.objectContaining({
        order_id: 'ord_pending',
        auction_id: 'auc_live',
        amount_cents: 60000,
        order_status: 'PAID'
      })
    ])
  }));
});
