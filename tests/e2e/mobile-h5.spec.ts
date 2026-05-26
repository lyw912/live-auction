import { expect, test } from '@playwright/test';

const productImageDataURL = 'data:image/svg+xml;utf8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%20600%20800%22%3E%3Crect%20width%3D%22600%22%20height%3D%22800%22%20fill%3D%22%23222f2b%22%2F%3E%3Ccircle%20cx%3D%22300%22%20cy%3D%22300%22%20r%3D%22142%22%20fill%3D%22%23e5f3ef%22%2F%3E%3Cellipse%20cx%3D%22300%22%20cy%3D%22320%22%20rx%3D%22164%22%20ry%3D%2286%22%20fill%3D%22%2310b981%22%2F%3E%3Cpath%20d%3D%22M170%20352c52%2068%20208%2068%20260%200%22%20stroke%3D%22%23d6a84f%22%20stroke-width%3D%2218%22%20fill%3D%22none%22%20stroke-linecap%3D%22round%22%2F%3E%3Ctext%20x%3D%22300%22%20y%3D%22640%22%20text-anchor%3D%22middle%22%20font-size%3D%2248%22%20font-family%3D%22Arial%22%20fill%3D%22white%22%3ELOT%20A-102%3C%2Ftext%3E%3C%2Fsvg%3E';

test.beforeEach(async ({ page }) => {
  await page.route('/api/auth/me', async (route) => {
    await route.fulfill({ json: { user: { ID: 'user_1', Role: 'user' } } });
  });
  await page.route('/api/auth/login', async (route) => {
    await route.fulfill({ json: { user: { ID: 'user_1', Role: 'user' }, expires_in_ms: 43200000 } });
  });
  await page.route('/api/rooms/room_main/auctions', async (route) => {
    await route.fulfill({
      json: [
        {
          id: 'auc_live',
          room_id: 'room_main',
          status: 'ACTIVE',
          current_price_cents: 35000,
          increment_cents: 5000,
          cap_price_cents: 150000,
          accepted_bid_count: 3,
          seq: 41,
          end_at: '2099-05-22T14:00:00Z',
          rule: {
            extend_window_seconds: 10,
            extend_by_seconds: 10,
            max_extend_count: 3,
            fat_finger_threshold_cents: 100000,
            deposit_floor_cents: 50000
          },
          item: {
            title: '青瓷手作茶盏',
            description: '孤品手作，窑变釉面带自然开片，适合收藏与日用。',
            image_url: productImageDataURL,
            certificate: '中检证书',
            condition: '无冲线',
            shipping: '顺丰保价',
            dimensions: '直径 9.2cm',
            material: '景德镇高温瓷',
            flaws: '口沿一处自然釉缩',
            return_policy: '签收前可验货，证书不符支持售后复核。'
          }
        },
        {
          id: 'auc_next',
          room_id: 'room_main',
          status: 'SCHEDULED',
          current_price_cents: 80000,
          increment_cents: 10000,
          cap_price_cents: 300000,
          accepted_bid_count: 0,
          end_at: '2099-05-22T14:10:00Z',
          item: {
            title: '紫砂壶',
            image_url: productImageDataURL,
            certificate: '作者证书',
            condition: '九五品',
            shipping: '包邮保价'
          }
        }
      ]
    });
  });
  await page.route('/api/auctions/auc_live', async (route) => {
    await route.fulfill({
      json: {
        event_type: 'snapshot',
        auction_id: 'auc_live',
        id: 'auc_live',
        status: 'ACTIVE',
        seq: 41,
        source: 'db',
        stale: false,
        current_price_cents: 35000,
        increment_cents: 5000,
        current_winner_id: 'user_2',
        end_at: '2099-05-22T14:00:00Z',
        server_time_ms: Date.parse('2099-05-22T13:58:45Z'),
        payload: {
          status: 'ACTIVE',
          current_price_cents: 35000,
          leader_user_masked: '张**',
          current_winner_id: 'user_2',
          end_at: '2099-05-22T14:00:00Z',
          server_time_ms: Date.parse('2099-05-22T13:58:45Z'),
          item: {
            title: '青瓷手作茶盏',
            description: '孤品手作，窑变釉面带自然开片，适合收藏与日用。',
            image_url: productImageDataURL,
            certificate: '中检证书',
            condition: '无冲线',
            shipping: '顺丰保价',
            dimensions: '直径 9.2cm',
            material: '景德镇高温瓷',
            flaws: '口沿一处自然釉缩',
            return_policy: '签收前可验货，证书不符支持售后复核。'
          }
        }
      }
    });
  });
  await page.route('/api/auctions/auc_live/leaderboard?limit=5', async (route) => {
    await route.fulfill({
      json: {
        auction_id: 'auc_live',
        seq: 42,
        server_time_ms: Date.parse('2099-05-22T13:58:45Z'),
        current_price_cents: 35000,
        current_winner_id: 'user_2',
        my_rank: 2,
        my_best_amount_cents: 30000,
        gap_to_leader_cents: 5000,
        gap_to_next_rank_cents: 5000,
        next_valid_bid_cents: 40000,
        state: 'OUTBID',
        leader_amount_cents: 35000,
        accepted_bidder_count: 2,
        active_bidders_30s: 2,
        accepted_bids_30s: 3,
        price_velocity_cents_per_min: 10000,
        entries: [
          { rank: 1, user_id: 'user_2', user_masked: '张**', amount_cents: 35000, bid_count: 2 },
          { rank: 2, user_id: 'user_1', user_masked: '我', amount_cents: 30000, bid_count: 1, is_current: true }
        ]
      }
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
  await page.route('/api/rooms/room_main/chat?limit=30', async (route) => {
    await route.fulfill({
      json: {
        items: [
          { id: 1, room_id: 'room_main', user_id: 'user_2', body: '这个茶盏釉色不错', created_at: '2026-05-22T13:00:00Z' }
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

test('H5 does not expose test state matrix in normal demo entry', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByLabel('state-matrix')).toHaveCount(0);
});

test('H5 disables bid CTA for unsafe states and keeps text inside controls', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
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
  await expect(page.getByTestId('auction-countdown')).toBeVisible();

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
  const expectedWSURL = await page.evaluate(() => {
    const wsScheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${wsScheme}//${window.location.host}/ws?room_id=room_main&auction_id=auc_live&last_seq=41`;
  });
  await expect.poll(async () => page.evaluate(() => {
    const entries = (window as typeof window & { __auctionWS?: Array<{ url: string; protocols: string[] }> }).__auctionWS ?? [];
    return entries.filter(({ url }) => url.includes('/ws?')).map(({ url, protocols }) => ({ url, protocols }));
  })).toEqual([
    {
      url: expectedWSURL,
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
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥400.00');
  await expect(page.getByText('陈** 领先')).toBeVisible();
});

test('H5 recovering and disconnected states show stale marker', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '恢复中' }).click();
  await expect(page.getByText('状态可能已过期')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await page.getByRole('button', { name: '已断开' }).click();
  await expect(page.getByLabel('auction-state').locator('.dock-feedback')).toHaveText('重连中');
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
          end_at: '2099-05-22T14:00:00Z',
          server_time_ms: Date.parse('2099-05-22T13:59:00Z'),
          reject_reason: null
        }
      });
    });
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.getByTestId('bid-cta').click();
  const bidRequest = await bidArrived;

  await expect(page.getByText('等待服务端确认')).toBeVisible();
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥350.00');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  expect(bidRequest.idempotencyKey).toBe(bidRequest.body.client_bid_id);
  expect(bidRequest.body.amount_cents).toBe(40000);
  expect(bidRequest.body.client_seen_seq).toBe(41);

  releaseBid();
  await expect(page.getByText('服务端确认 seq 42')).toBeVisible();
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥400.00');
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

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.getByTestId('bid-cta').click();

  await expect(page.getByText('请按加价幅度出价')).toBeVisible();
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥350.00');
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
          end_at: '2099-05-22T14:00:00Z',
          server_time_ms: Date.parse('2099-05-22T13:59:00Z'),
          reject_reason: null
        }
      });
    });
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.getByTestId('bid-cta').click();
  await expect(page.getByText('确认 ¥900.00 出价')).toBeVisible();
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥350.00');
  await expect(page.getByTestId('bid-cta')).toHaveText(/确认高额出价/);

  await page.getByTestId('bid-cta').click();
  const confirmRequest = await confirmArrived;
  await expect(page.getByText('等待服务端确认高额出价')).toBeVisible();
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥350.00');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  expect(confirmRequest.idempotencyKey).toBe(firstBidKey);
  expect(confirmRequest.body.confirm_token).toBe('ft_test');
  expect(confirmRequest.body.idempotency_key).toBe(firstBidKey);

  releaseConfirm();
  await expect(page.getByText('服务端确认 seq 42')).toBeVisible();
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥900.00');
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

  await page.goto('/?stateMatrix=1');
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

test('H5 winner result sheet locks order and shares the single payment path', async ({ page }) => {
  let payCount = 0;
  await page.route('/api/orders/ord_pending/pay-mock', async (route) => {
    payCount += 1;
    await route.fulfill({
      json: {
        order_id: 'ord_pending',
        order_status: 'PAID',
        paid_at: '2026-05-22T13:10:00Z',
        deposit_status: 'REFUNDED'
      }
    });
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '成交', exact: true }).click();
  const sheet = page.getByTestId('result-sheet');
  await expect(sheet).toBeVisible();
  await expect(sheet.getByRole('heading', { name: '恭喜拍中' })).toBeVisible();
  await expect(sheet.getByText('成交价 ¥600.00')).toBeVisible();
  await expect(sheet.getByText('订单 ord_pending 已锁定')).toBeVisible();
  await expect(sheet.getByText('保证金会随订单状态处理')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);

  await sheet.getByTestId('result-pay-cta').dblclick();
  await expect(sheet.getByRole('heading', { name: '支付已完成' })).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  expect(payCount).toBe(1);
});

test('H5 loser and unsold result sheets explain next action without enabling bid', async ({ page }) => {
  await page.goto('/?stateMatrix=1');

  await page.getByRole('button', { name: '竞价中' }).click();
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_sold',
        seq: 42,
        payload: {
          amount_cents: 60000,
          user_id: 'user_2',
          leader_user_masked: '赵**'
        }
      }
    }));
  });
  const loserSheet = page.getByTestId('result-sheet');
  await expect(loserSheet.getByRole('heading', { name: '本场已落锤' })).toBeVisible();
  await expect(loserSheet.getByText('us** 以 ¥600.00 拍中')).toBeVisible();
  await expect(loserSheet.getByText('下一件：紫砂壶')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();

  await page.getByRole('button', { name: '流拍', exact: true }).click();
  const unsoldSheet = page.getByTestId('result-sheet');
  await expect(unsoldSheet.getByRole('heading', { name: '本场未成交' })).toBeVisible();
  await expect(unsoldSheet.getByText('不会生成订单')).toBeVisible();
  await expect(unsoldSheet.getByText('紫砂壶 即将开始')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
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
          end_at: '2099-05-22T14:00:10Z',
          server_time_ms: Date.parse('2099-05-22T13:59:50Z'),
          source: 'db',
          stale: false,
          payload: {
            current_price_cents: 45000,
            leader_user_masked: '王**',
            end_at: '2099-05-22T14:00:10Z',
            server_time_ms: Date.parse('2099-05-22T13:59:50Z')
          }
        }
      });
    });
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'bid_accepted',
        seq: 44,
        end_at: '2099-05-22T14:00:10Z',
        server_time_ms: Date.parse('2099-05-22T13:59:50Z'),
        payload: {
          current_price_cents: 45000,
          leader_user_masked: '王**',
          end_at: '2099-05-22T14:00:10Z',
          server_time_ms: Date.parse('2099-05-22T13:59:50Z')
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
  await expect(page.getByTestId('auction-countdown')).toHaveText(/延时后|剩余/);
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

  await page.goto('/?stateMatrix=1');
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

test('H5 atmosphere engine dedupes strong effects after recovery snapshot', async ({ page }) => {
  await page.route('/api/auctions/auc_live', async (route) => {
    await route.fulfill({
      json: {
        event_type: 'snapshot',
        auction_id: 'auc_live',
        seq: 45,
        source: 'db',
        stale: false,
        current_price_cents: 45000,
        increment_cents: 5000,
        current_winner_id: 'user_2',
        end_at: '2099-05-22T14:00:10Z',
        server_time_ms: Date.parse('2099-05-22T13:59:50Z'),
        payload: {
          status: 'ACTIVE',
          current_price_cents: 45000,
          leader_user_masked: '王**',
          current_winner_id: 'user_2',
          end_at: '2099-05-22T14:00:10Z',
          server_time_ms: Date.parse('2099-05-22T13:59:50Z')
        }
      }
    });
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'outbox_gap_notice',
        seq: 45
      }
    }));
  });
  await expect(page.getByText('snapshot db seq 45')).toBeVisible();
  await expect(page.getByTestId('atmosphere-cue')).toHaveCount(0);

  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'bid_accepted',
        seq: 44,
        payload: {
          current_price_cents: 44000,
          current_winner_id: 'user_1',
          user_id: 'user_1',
          leader_user_masked: '你'
        }
      }
    }));
  });
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥450.00');
  await expect(page.getByTestId('atmosphere-cue')).toHaveCount(0);
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
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '历史' }).click();
  const historySheet = page.getByTestId('bottom-sheet');
  await expect(historySheet).toBeVisible();
  await historySheet.getByRole('button', { name: /刷新/ }).click();
  await expect(historySheet.getByText('auc_live')).toHaveCount(2);
  await expect(historySheet.getByText('¥450.00 · ACCEPTED')).toBeVisible();
  await expect(historySheet.getByText('¥400.00 · OUTBID')).toBeVisible();

  await historySheet.getByRole('tab', { name: '订单' }).click();
  await expect(historySheet.getByText('ord_hist_1')).toBeVisible();
  await expect(historySheet.getByText('¥600.00 · PAID')).toBeVisible();
});

test('H5 bottom sheets open close and keep the primary bid CTA singular', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 844 });
  await page.goto('/');

  const dock = page.getByLabel('auction-state');
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);

  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '商品' }).click();
  const sheet = page.getByTestId('bottom-sheet');
  await expect(sheet).toBeVisible();
  await expect(sheet.getByRole('heading', { name: '本场商品' })).toBeVisible();
  await expect(sheet.getByText('青瓷手作茶盏')).toBeVisible();
  await expect(sheet.getByText('紫砂壶')).toBeVisible();
  await expect(sheet.getByText('当前拍品')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);
  await expect(page.getByTestId('bid-cta')).toBeVisible();
  await expect(dock).toBeVisible();

  const sheetBox = await sheet.boundingBox();
  const dockBox = await dock.boundingBox();
  expect(sheetBox).toBeTruthy();
  expect(dockBox).toBeTruthy();
  expect(sheetBox!.y + sheetBox!.height).toBeLessThanOrEqual(dockBox!.y + 2);

  await sheet.getByRole('tab', { name: '规则' }).click();
  await expect(sheet.getByRole('heading', { name: '商品与规则' })).toBeVisible();
  await expect(sheet.getByText('当前出价节奏')).toBeVisible();
  await expect(sheet.getByText('误触保护')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);

  await sheet.getByRole('tab', { name: '榜单' }).click();
  await expect(sheet.getByRole('heading', { name: '实时榜单' })).toBeVisible();
  await expect(sheet.getByText('张**')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);

  await sheet.getByLabel('关闭面板').click();
  await expect(page.getByTestId('bottom-sheet')).toHaveCount(0);
  await expect(page.getByTestId('bid-cta')).toBeVisible();
});

test('H5 bottom sheet history and orders use existing user APIs', async ({ page }) => {
  await page.route('/api/users/me/bids', async (route) => {
    await route.fulfill({
      json: {
        items: [
          { bid_id: 'bid_sheet_1', auction_id: 'auc_live', amount_cents: 45000, result: 'ACCEPTED' }
        ]
      }
    });
  });
  await page.route('/api/users/me/orders', async (route) => {
    await route.fulfill({
      json: {
        items: [
          { order_id: 'ord_sheet_1', auction_id: 'auc_live', amount_cents: 60000, order_status: 'ORDER_PENDING' }
        ]
      }
    });
  });

  await page.goto('/');
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '历史' }).click();
  const sheet = page.getByTestId('bottom-sheet');
  await sheet.getByRole('button', { name: /刷新/ }).click();
  await expect(sheet.getByText('¥450.00 · ACCEPTED')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);

  await sheet.getByRole('tab', { name: '订单' }).click();
  await expect(sheet.getByText('ord_sheet_1')).toBeVisible();
  await expect(sheet.getByText('¥600.00 · ORDER_PENDING')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeVisible();
});

test('H5 product trust sheet explains proof money and timing in user language', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 844 });
  await page.goto('/');

  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '规则' }).click();
  const sheet = page.getByTestId('bottom-sheet');
  await expect(sheet.getByRole('heading', { name: '商品与规则' })).toBeVisible();
  await expect(sheet.getByText('商品信任详情')).toBeVisible();
  await expect(sheet.getByText('孤品手作，窑变釉面带自然开片，适合收藏与日用。')).toBeVisible();
  await expect(sheet.getByLabel('product-trust-proof').getByText('中检证书')).toBeVisible();
  await expect(sheet.getByLabel('product-trust-proof').getByText('直径 9.2cm')).toBeVisible();
  await expect(sheet.getByText('当前出价节奏')).toBeVisible();
  await expect(sheet.getByText('价格到达 ¥1500.00 后不再继续抬价。')).toBeVisible();
  await expect(sheet.getByText('本场要求保证金，最低 ¥500.00')).toBeVisible();
  await expect(sheet.getByText('最后 10 秒内有有效出价，会自动延长 10 秒，最多 3 次')).toBeVisible();
  await expect(sheet.getByText('单次高额跳价达到 ¥1000.00 会触发确认，防止误触。')).toBeVisible();
  await expect(sheet.getByText('签收前可验货，证书不符支持售后复核。')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);
  await expect(page.getByTestId('bid-cta')).toBeVisible();
});

test('H5 chat reads seed messages and sends room chat API', async ({ page }) => {
  let chatBody: Record<string, unknown> | undefined;
  await page.route('/api/rooms/room_main/chat', async (route, request) => {
    chatBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({
      status: 201,
      json: {
        id: 2,
        room_id: 'room_main',
        user_id: 'user_1',
        body: chatBody.body,
        created_at: '2026-05-22T13:00:05Z'
      }
    });
  });

  await page.goto('/');
  await expect(page.getByTestId('stage-chat-overlay').getByText('这个茶盏釉色不错')).toBeVisible();
  await page.getByLabel('chat-input').fill('我跟一口');
  await page.getByRole('button', { name: 'send-chat' }).click();
  await expect(page.getByTestId('stage-chat-overlay').getByText('我跟一口')).toBeVisible();
  expect(chatBody?.body).toBe('我跟一口');
  expect(String(chatBody?.client_msg_id ?? '')).toBeTruthy();
});

test('H5 server terminal events drive sold, ended, and cancelled states', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_sold',
        seq: 42,
        payload: {
          amount_cents: 60000,
          user_id: 'user_2',
          leader_user_masked: '赵**'
        }
      }
    }));
  });
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('已成交');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();

  await page.getByRole('button', { name: '竞价中' }).click();
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_cancelled',
        seq: 43,
        payload: { current_price_cents: 60000, reason: '主播已取消' }
      }
    }));
  });
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('已取消');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
});

test('H5 live panel keeps server countdown visible with status connection and CTA', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();

  await expect(page.getByLabel('auction-state').getByText('ACTIVE')).toBeVisible();
  await expect(page.getByText('WebSocket 已连接 · 状态来自服务端事件')).toBeVisible();
  await expect(page.getByTestId('auction-countdown')).toHaveText(/剩余|延时后/);
  await expect(page.getByTestId('bid-cta')).toBeEnabled();
});

for (const viewport of [
  { width: 390, height: 844 },
  { width: 360, height: 844 }
]) {
  test(`H5 sticky bid dock keeps price countdown rank and CTA visible at ${viewport.width}px`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto('/');

    const stage = page.getByTestId('live-stage');
    const dock = page.getByLabel('auction-state');
    const price = dock.locator('h2');
    const countdown = page.getByTestId('auction-countdown');
    const cta = page.getByTestId('bid-cta');
    await expect(stage).toBeVisible();
    await expect(dock).toBeVisible();
    await expect(price).toBeVisible();
    await expect(countdown).toBeVisible();
    await expect(dock.getByText(/我的排名|出价后显示排名/)).toBeVisible();
    await expect(cta).toBeVisible();

    const viewportHeight = viewport.height;
    for (const locator of [stage, price, countdown, cta]) {
      const box = await locator.boundingBox();
      expect(box).toBeTruthy();
      expect(box!.y).toBeGreaterThanOrEqual(0);
      expect(box!.y + box!.height).toBeLessThanOrEqual(viewportHeight);
    }
    const ctaBox = await cta.boundingBox();
    const dockBox = await dock.boundingBox();
    expect(ctaBox).toBeTruthy();
    expect(dockBox).toBeTruthy();
    expect(ctaBox!.x).toBeGreaterThanOrEqual(dockBox!.x);
    expect(ctaBox!.x + ctaBox!.width).toBeLessThanOrEqual(dockBox!.x + dockBox!.width);
  });
}

test('H5 live stage uses product media and keeps chat inside safe zone at 360px', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 844 });
  await page.goto('/');

  const stage = page.getByTestId('live-stage');
  await expect(stage).toBeVisible();
  await expect(stage).toHaveClass(/has-media/);
  await expect(stage.getByText('中检证书')).toBeVisible();
  await expect(stage.getByText('无冲线')).toBeVisible();
  await expect(stage.getByText('顺丰保价')).toBeVisible();
  await expect(page.getByTestId('stage-chat-overlay').getByText('这个茶盏釉色不错')).toBeVisible();

  const stageBox = await stage.boundingBox();
  const chatBox = await page.getByTestId('stage-chat-overlay').boundingBox();
  const ctaBox = await page.getByTestId('bid-cta').boundingBox();
  expect(stageBox).toBeTruthy();
  expect(chatBox).toBeTruthy();
  expect(ctaBox).toBeTruthy();
  expect(chatBox!.y + chatBox!.height).toBeLessThanOrEqual(stageBox!.y + stageBox!.height);
  expect(chatBox!.y + chatBox!.height).toBeLessThan(ctaBox!.y);
  await expect(page.getByTestId('bid-cta')).toBeVisible();
});

test('H5 renders realtime leaderboard and event atmosphere controls', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥350.00');

  await expect(page.getByTestId('leaderboard-panel')).toContainText('实时排行榜');
  await expect(page.getByTestId('leaderboard-panel')).toContainText('第 2 名');
  await expect(page.getByTestId('leaderboard-panel')).toContainText('差 ¥50.00');
  await expect(page.getByTestId('leaderboard-panel')).toContainText('¥350.00');
  const leaderboardPayload = await page.evaluate(async () => {
    const response = await fetch('/api/auctions/auc_live/leaderboard?limit=5');
    return response.json();
  });
  expect(leaderboardPayload).toMatchObject({
    seq: 42,
    gap_to_next_rank_cents: 5000,
    next_valid_bid_cents: 40000,
    state: 'OUTBID',
    active_bidders_30s: 2,
    accepted_bids_30s: 3,
    price_velocity_cents_per_min: 10000
  });
  await expect(page.getByRole('button', { name: '开启提示音' })).toBeVisible();

  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'bid_accepted',
        seq: 42,
        payload: {
          current_price_cents: 40000,
          current_winner_id: 'user_1',
          user_id: 'user_1',
          leader_user_masked: '你'
        }
      }
    }));
  });

  const cue = page.getByTestId('atmosphere-cue');
  await expect(cue).toContainText('领先！');
  await expect(cue).toHaveAttribute('data-auction-id', 'auc_live');
  await expect(cue).toHaveAttribute('data-cause-seq', '42');
  await expect(cue).toHaveAttribute('data-event-type', 'bid_accepted');
  await expect(cue).toHaveAttribute('data-user-scope', 'self');
});

test('H5 event-driven visual effects stay nonblocking and respect reduced motion', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'bid_accepted',
        seq: 42,
        payload: {
          current_price_cents: 40000,
          current_winner_id: 'user_1',
          user_id: 'user_1',
          leader_user_masked: '你'
        }
      }
    }));
  });

  await expect(page.getByTestId('live-stage')).toHaveAttribute('data-atmosphere-kind', 'leading');
  await expect(page.getByLabel('auction-state')).toHaveAttribute('data-atmosphere-kind', 'leading');
  await expect(page.getByTestId('auction-price')).toHaveCSS('animation-name', 'price-tick');

  const ctaBox = await page.getByTestId('bid-cta').boundingBox();
  const cueBox = await page.getByTestId('atmosphere-cue').boundingBox();
  expect(ctaBox).not.toBeNull();
  expect(cueBox).not.toBeNull();
  expect((cueBox?.y ?? 0) + (cueBox?.height ?? 0)).toBeLessThan(ctaBox?.y ?? 0);

  await page.emulateMedia({ reducedMotion: 'reduce' });
  await expect(page.getByTestId('auction-price')).toHaveCSS('animation-name', 'none');
});

test('H5 extension and sold visual effects use bounded nonblocking motion layers', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'bid_accepted',
        seq: 42,
        payload: {
          current_price_cents: 40000,
          current_winner_id: 'user_2',
          user_id: 'user_2',
          leader_user_masked: '张**',
          end_at: '2099-05-22T14:00:20Z',
          server_time_ms: Date.parse('2099-05-22T13:59:50Z')
        }
      }
    }));
  });

  await expect(page.getByTestId('live-stage')).toHaveAttribute('data-atmosphere-kind', 'extended');
  await expect(page.getByTestId('auction-countdown')).toHaveAttribute('data-effect', 'extension-stretch');
  await expect(page.getByTestId('auction-countdown')).toHaveCSS('animation-name', 'countdown-stretch');

  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_sold',
        seq: 43,
        payload: {
          current_price_cents: 40000,
          current_winner_id: 'user_2',
          leader_user_masked: '张**'
        }
      }
    }));
  });

  await expect(page.getByTestId('live-stage')).toHaveAttribute('data-atmosphere-kind', 'sold');
  await expect(page.getByTestId('atmosphere-cue')).toHaveAttribute('data-event-type', 'auction_sold');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
});

test('H5 order realtime events update winner payment state', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '成交', exact: true }).click();

  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'order_paid',
        seq: 42,
        payload: {
          user_id: 'user_1',
          order_id: 'ord_pending',
          order_status: 'PAID',
          deposit_status: 'REFUNDED'
        }
      }
    }));
  });
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('已支付');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();

  await page.getByRole('button', { name: '成交', exact: true }).click();
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'order_expired',
        seq: 43,
        payload: {
          user_id: 'user_1',
          order_id: 'ord_pending',
          order_status: 'ORDER_EXPIRED',
          deposit_status: 'FORFEITED'
        }
      }
    }));
  });
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('订单已超时');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
});

test('H5 interaction surface has no unacceptable animation longtask', async ({ page }) => {
  await page.addInitScript(() => {
    const target = window as typeof window & { __longTasks?: number[] };
    target.__longTasks = [];
    if ('PerformanceObserver' in window) {
      try {
        const observer = new PerformanceObserver((list) => {
          target.__longTasks = [
            ...(target.__longTasks ?? []),
            ...list.getEntries().map((entry) => entry.duration)
          ];
        });
        observer.observe({ type: 'longtask', buffered: true });
      } catch {
        target.__longTasks = [];
      }
    }
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.getByRole('button', { name: 'increase' }).click();
  await page.getByRole('button', { name: 'decrease' }).click();
  await page.getByRole('button', { name: '恢复中' }).click();
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.waitForTimeout(250);

  const maxLongTask = await page.evaluate(() => Math.max(0, ...((window as typeof window & { __longTasks?: number[] }).__longTasks ?? [])));
  expect(maxLongTask).toBeLessThan(100);
});
