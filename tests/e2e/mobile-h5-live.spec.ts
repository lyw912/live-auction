import { expect, test } from '@playwright/test';

test('H5 consumes live backend WebSocket ticket and outbox bid event', async ({ page }) => {
  const clientBidID = `live-smoke-${Date.now()}`;

  await page.goto('/');
  await expect(page.getByText('WebSocket 已连接 · 状态来自服务端事件')).toBeVisible();
  await expect(page.getByText('¥350.00')).toBeVisible();

  const response = await page.request.post('/api/auctions/auc_live/bids', {
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': clientBidID,
      'X-Mock-Role': 'user',
      'X-Mock-User-Id': 'user_3'
    },
    data: {
      client_bid_id: clientBidID,
      amount_cents: 40000,
      client_seen_seq: 41
    }
  });
  expect(response.ok()).toBeTruthy();
  const payload = await response.json();
  expect(payload).toEqual(expect.objectContaining({
    result: 'ACCEPTED',
    auction_id: 'auc_live',
    seq: 42,
    current_price_cents: 40000
  }));

  await expect(page.getByText('event seq 42')).toBeVisible();
  await expect(page.getByText('¥400.00')).toBeVisible();
});
