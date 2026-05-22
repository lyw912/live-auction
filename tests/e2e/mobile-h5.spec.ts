import { expect, test } from '@playwright/test';

test.beforeEach(async ({ page }) => {
  await page.route('/api/rooms/room_main/auctions', async (route) => {
    await route.fulfill({
      json: [
        {
          id: 'auc_live',
          room_id: 'room_main',
          status: 'ACTIVE',
          current_price_cents: 35000,
          increment_cents: 5000,
          seq: 41,
          item: {
            title: '青瓷手作茶盏'
          }
        }
      ]
    });
  });
  await page.route('/api/users/me/orders', async (route) => {
    await route.fulfill({
      json: {
        items: [
          { order_id: 'ord_pending', auction_id: 'auc_live', amount_cents: 60000, order_status: 'ORDER_PENDING' }
        ]
      }
    });
  });
  await page.route('/api/auth/ws-ticket', async (route, request) => {
    await route.fulfill({
      json: {
        ticket: request.postDataJSON().auction_id === 'auc_live' ? 'ticket_test' : '',
        expires_in_ms: 60000
      }
    });
  });
  await page.addInitScript(() => {
    class MockAuctionWebSocket extends EventTarget {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSING = 2;
      static CLOSED = 3;
      readonly CONNECTING = 0;
      readonly OPEN = 1;
      readonly CLOSING = 2;
      readonly CLOSED = 3;
      binaryType: BinaryType = 'blob';
      bufferedAmount = 0;
      extensions = '';
      protocol = 'auction.v1';
      readyState = MockAuctionWebSocket.CONNECTING;
      onopen: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onerror: ((event: Event) => void) | null = null;
      onclose: ((event: CloseEvent) => void) | null = null;
      url: string;
      shouldRecord: boolean;

      constructor(url: string | URL, protocols?: string | string[]) {
        super();
        this.url = String(url);
        this.shouldRecord = this.url.includes('/ws?');
        const protocolList = Array.isArray(protocols) ? protocols : protocols ? [protocols] : [];
        if (this.shouldRecord) {
          (window as typeof window & { __auctionWS?: unknown[] }).__auctionWS = [
            ...((window as typeof window & { __auctionWS?: unknown[] }).__auctionWS ?? []),
            { url: this.url, protocols: protocolList, socket: this }
          ];
        }
        window.setTimeout(() => {
          this.readyState = MockAuctionWebSocket.OPEN;
          const event = new Event('open');
          this.onopen?.(event);
          this.dispatchEvent(event);
        }, 0);
      }

      close() {
        this.readyState = MockAuctionWebSocket.CLOSED;
        const event = new CloseEvent('close');
        this.onclose?.(event);
        this.dispatchEvent(event);
      }

      send() {
        return undefined;
      }

      dispatchServerMessage(payload: unknown) {
        const event = new MessageEvent('message', { data: JSON.stringify(payload) });
        this.onmessage?.(event);
        this.dispatchEvent(event);
      }
    }
    window.WebSocket = MockAuctionWebSocket as unknown as typeof WebSocket;
  });
});

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

test('H5 opens browser WebSocket with ticket subprotocol and consumes authoritative events', async ({ page }) => {
  const ticketRequest = new Promise<Record<string, unknown>>((resolve) => {
    void page.route('/api/auth/ws-ticket', async (route, request) => {
      resolve(request.postDataJSON() as Record<string, unknown>);
      await route.fulfill({
        json: {
          ticket: 'ticket_live',
          expires_in_ms: 60000
        }
      });
    });
  });

  await page.goto('/');
  await expect(page.getByText('WebSocket 已连接 · 状态来自服务端事件')).toBeVisible();
  await expect.poll(async () => page.evaluate(() => {
    const entries = (window as typeof window & { __auctionWS?: Array<{ url: string; protocols: string[] }> }).__auctionWS ?? [];
    return entries.filter(({ url }) => url.includes('/ws?')).map(({ url, protocols }) => ({ url, protocols }));
  })).toEqual([
    {
      url: 'ws://127.0.0.1:5173/ws?room_id=room_main&auction_id=auc_live&last_seq=41',
      protocols: ['auction.v1', 'ticket.ticket_live']
    }
  ]);
  await expect(ticketRequest).resolves.toEqual({ room_id: 'room_main', auction_id: 'auc_live' });

  await page.evaluate(() => {
    const [entry] = ((window as typeof window & { __auctionWS?: Array<{ url: string; socket: { dispatchServerMessage: (payload: unknown) => void } }> }).__auctionWS ?? [])
      .filter(({ url }) => url.includes('/ws?'));
    entry.socket.dispatchServerMessage({
      auction_id: 'auc_live',
      event_type: 'bid_accepted',
      seq: 42,
      payload: {
        current_price_cents: 40000,
        leader_user_masked: '陈**'
      }
    });
  });

  await expect(page.getByText('event seq 42')).toBeVisible();
  await expect(page.getByText('¥400.00')).toBeVisible();
  await expect(page.getByText('陈** 领先')).toBeVisible();
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

test('H5 fat-finger confirm waits for confirm API before accepted UI', async ({ page }) => {
  let firstBidKey = '';
  await page.route('/api/auctions/auc_live/bids', async (route, request) => {
    firstBidKey = request.headers()['idempotency-key'] ?? '';
    await route.fulfill({
      json: {
        result: 'FAT_FINGER_CONFIRM_REQUIRED',
        confirm_token: 'ft_test',
        expires_in_ms: 30000,
        current_price_cents: 35000,
        amount_cents: 90000
      }
    });
  });

  let releaseConfirm: (value?: unknown) => void = () => undefined;
  const confirmArrived = new Promise<{ idempotencyKey: string | null; body: Record<string, unknown> }>((resolve) => {
    void page.route('/api/auctions/auc_live/bids/confirm', async (route, request) => {
      resolve({
        idempotencyKey: request.headers()['idempotency-key'] ?? null,
        body: request.postDataJSON() as Record<string, unknown>
      });
      await new Promise((release) => {
        releaseConfirm = release;
      });
      await route.fulfill({
        json: {
          result: 'ACCEPTED',
          bid_id: 'bid_confirmed',
          auction_id: 'auc_live',
          seq: 42,
          current_price_cents: 90000,
          current_winner_id: 'user_1',
          reject_reason: null
        }
      });
    });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.getByTestId('bid-cta').click();
  await expect(page.getByText('确认 ¥900.00 出价')).toBeVisible();
  await expect(page.getByText('¥350.00')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveText(/确认高额出价/);

  await page.getByTestId('bid-cta').click();
  const confirmRequest = await confirmArrived;
  await expect(page.getByText('等待服务端确认高额出价')).toBeVisible();
  await expect(page.getByText('¥350.00')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  expect(confirmRequest.idempotencyKey).toBe(firstBidKey);
  expect(confirmRequest.body.confirm_token).toBe('ft_test');
  expect(confirmRequest.body.idempotency_key).toBe(firstBidKey);

  releaseConfirm();
  await expect(page.getByText('服务端确认 seq 42')).toBeVisible();
  await expect(page.getByText('¥900.00')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
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

test('H5 renders bid and order history from user APIs', async ({ page }) => {
  await page.route('/api/users/me/bids', async (route) => {
    await route.fulfill({
      json: {
        items: [
          { bid_id: 'bid_hist_1', auction_id: 'auc_live', amount_cents: 45000, result: 'ACCEPTED' },
          { bid_id: 'bid_hist_2', auction_id: 'auc_live', amount_cents: 40000, result: 'OUTBID' }
        ]
      }
    });
  });
  await page.route('/api/users/me/orders', async (route) => {
    await route.fulfill({
      json: {
        items: [
          { order_id: 'ord_hist_1', auction_id: 'auc_live', amount_cents: 60000, order_status: 'PAID' }
        ]
      }
    });
  });

  await page.goto('/');
  await page.getByTestId('history-panel').getByRole('button', { name: /刷新/ }).click();
  await expect(page.getByTestId('history-panel').getByText('auc_live')).toHaveCount(2);
  await expect(page.getByText('¥450.00 · ACCEPTED')).toBeVisible();
  await expect(page.getByText('¥400.00 · OUTBID')).toBeVisible();
  await expect(page.getByText('ord_hist_1')).toBeVisible();
  await expect(page.getByText('¥600.00 · PAID')).toBeVisible();
});
