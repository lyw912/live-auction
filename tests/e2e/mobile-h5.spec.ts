import { expect, test } from '@playwright/test';

test('H5 disables bid CTA for unsafe states and keeps text inside controls', async ({ page }) => {
  await page.goto('/');
  const unsafeStates = ['领先中', '提交中', '恢复中', '已断开', '已成交', '流拍', '已取消'];
  for (const state of unsafeStates) {
    await page.getByRole('button', { name: state }).click();
    await expect(page.getByTestId('bid-cta')).toBeDisabled();
  }

  await page.getByRole('button', { name: '竞价中' }).click();
  await expect(page.getByTestId('bid-cta')).toBeEnabled();

  await page.getByRole('button', { name: '提交中' }).click();
  await expect(page.getByText('等待服务端确认')).toBeVisible();
  await expect(page.getByText('状态来自服务端事件')).toBeVisible();

  const buttons = await page.locator('button').all();
  for (const button of buttons) {
    const box = await button.boundingBox();
    if (!box) continue;
    expect(box.height).toBeGreaterThan(20);
    expect(box.width).toBeGreaterThan(24);
  }
});

test('H5 recovering and disconnected states show stale marker', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '恢复中' }).click();
  await expect(page.getByText('状态可能已过期')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await page.getByRole('button', { name: '已断开' }).click();
  await expect(page.getByLabel('auction-state').locator('.leader-row strong')).toHaveText('重连中');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
});

test('H5 bid stays pending until authoritative accepted response', async ({ page }) => {
  let releaseBid: (value?: unknown) => void = () => undefined;
  const bidArrived = new Promise<{ idempotencyKey: string | null; body: Record<string, unknown> }>((resolve) => {
    void page.route('/api/auctions/auc_live/bids', async (route, request) => {
      resolve({
        idempotencyKey: request.headers()['idempotency-key'] ?? null,
        body: request.postDataJSON() as Record<string, unknown>
      });
      await new Promise((release) => {
        releaseBid = release;
      });
      await route.fulfill({
        json: {
          result: 'ACCEPTED',
          bid_id: 'bid_1',
          auction_id: 'auc_live',
          seq: 42,
          current_price_cents: 40000,
          current_winner_id: 'user_1',
          reject_reason: null
        }
      });
    });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.getByTestId('bid-cta').click();
  const bidRequest = await bidArrived;

  await expect(page.getByText('等待服务端确认')).toBeVisible();
  await expect(page.getByText('¥350.00')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  expect(bidRequest.idempotencyKey).toBe(bidRequest.body.client_bid_id);
  expect(bidRequest.body.amount_cents).toBe(40000);
  expect(bidRequest.body.client_seen_seq).toBe(41);

  releaseBid();
  await expect(page.getByText('服务端确认 seq 42')).toBeVisible();
  await expect(page.getByText('¥400.00')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
});

test('H5 rejected bid shows business copy and re-enables CTA', async ({ page }) => {
  await page.route('/api/auctions/auc_live/bids', async (route) => {
    await route.fulfill({
      status: 400,
      json: {
        code: 'BID_INCREMENT_MISMATCH',
        message: 'bid amount does not match increment grid',
        trace_id: 'tr_test',
        details: {}
      }
    });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.getByTestId('bid-cta').click();

  await expect(page.getByText('请按加价幅度出价')).toBeVisible();
  await expect(page.getByText('¥350.00')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeEnabled();
});

test('H5 payment double click sends one mock payment and reaches paid UI', async ({ page }) => {
  let payCount = 0;
  let releasePayment: (value?: unknown) => void = () => undefined;
  const paymentArrived = new Promise<{ idempotencyKey: string | null; body: Record<string, unknown> }>((resolve) => {
    void page.route('/api/orders/ord_pending/pay-mock', async (route, request) => {
      payCount += 1;
      resolve({
        idempotencyKey: request.headers()['idempotency-key'] ?? null,
        body: request.postDataJSON() as Record<string, unknown>
      });
      await new Promise((release) => {
        releasePayment = release;
      });
      await route.fulfill({
        json: {
          order_id: 'ord_pending',
          order_status: 'PAID',
          paid_at: '2026-05-22T13:10:00Z',
          deposit_status: 'REFUNDED'
        }
      });
    });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '成交', exact: true }).click();
  const payButton = page.getByTestId('bid-cta');
  await payButton.dblclick();
  const paymentRequest = await paymentArrived;

  await expect(page.getByText('等待服务端确认支付')).toBeVisible();
  await expect(payButton).toBeDisabled();
  expect(paymentRequest.idempotencyKey).toBeTruthy();
  expect(paymentRequest.body.confirm).toBe(true);
  expect(payCount).toBe(1);

  releasePayment();
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('已支付');
  await expect(page.getByText('保证金已处理')).toBeVisible();
  await expect(payButton).toHaveText(/已支付/);
  await expect(payButton).toBeDisabled();
  expect(payCount).toBe(1);
});

test('H5 seq gap enters recovering and resumes from fresh snapshot', async ({ page }) => {
  let releaseSnapshot: (value?: unknown) => void = () => undefined;
  const snapshotArrived = new Promise((resolve) => {
    void page.route('/api/auctions/auc_live', async (route) => {
      resolve(undefined);
      await new Promise((release) => {
        releaseSnapshot = release;
      });
      await route.fulfill({
        json: {
          event_type: 'snapshot',
          auction_id: 'auc_live',
          seq: 44,
          source: 'db',
          stale: false,
          payload: {
            current_price_cents: 45000,
            leader_user_masked: '王**'
          }
        }
      });
    });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'bid_accepted',
        seq: 44,
        payload: {
          current_price_cents: 45000,
          leader_user_masked: '王**'
        }
      }
    }));
  });
  await snapshotArrived;

  await expect(page.getByText('状态可能已过期')).toBeVisible();
  await expect(page.getByText('正在同步权威状态')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();

  releaseSnapshot();
  await expect(page.getByText('snapshot db seq 44')).toBeVisible();
  await expect(page.getByText('¥450.00')).toBeVisible();
  await expect(page.getByText('王** 领先')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeEnabled();
});

test('H5 stale snapshot keeps recovering CTA disabled', async ({ page }) => {
  await page.route('/api/auctions/auc_live', async (route) => {
    await route.fulfill({
      json: {
        event_type: 'snapshot',
        auction_id: 'auc_live',
        seq: 42,
        source: 'redis',
        stale: true,
        payload: {
          current_price_cents: 40000,
          leader_user_masked: '李**'
        }
      }
    });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'outbox_gap_notice',
        seq: 44
      }
    }));
  });

  await expect(page.getByText('状态可能已过期')).toBeVisible();
  await expect(page.getByText('正在同步权威状态')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
});
