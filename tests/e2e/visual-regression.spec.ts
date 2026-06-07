import { expect, test, type Page } from '@playwright/test';

const productImageDataURL = 'data:image/svg+xml;utf8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%20600%20800%22%3E%3Crect%20width%3D%22600%22%20height%3D%22800%22%20fill%3D%22%231f2f2a%22%2F%3E%3Ccircle%20cx%3D%22300%22%20cy%3D%22288%22%20r%3D%22136%22%20fill%3D%22%23e5f3ef%22%2F%3E%3Cellipse%20cx%3D%22300%22%20cy%3D%22318%22%20rx%3D%22166%22%20ry%3D%2288%22%20fill%3D%22%2310b981%22%2F%3E%3Cpath%20d%3D%22M172%20352c54%2064%20202%2064%20256%200%22%20stroke%3D%22%23d6a84f%22%20stroke-width%3D%2218%22%20fill%3D%22none%22%20stroke-linecap%3D%22round%22%2F%3E%3Ctext%20x%3D%22300%22%20y%3D%22640%22%20text-anchor%3D%22middle%22%20font-size%3D%2248%22%20font-family%3D%22Arial%22%20fill%3D%22white%22%3ELIVE%20LOT%3C%2Ftext%3E%3C%2Fsvg%3E';

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
  extend_count: 1,
  end_at: '2099-05-22T14:00:00Z',
  item: { id: 'item_live', title: '天然翡翠A货平安扣吊坠', description: '珠宝可视验收', image_url: productImageDataURL, certificate: 'GID 20260607 · 可核验', condition: '品相完整', shipping: '顺丰包邮' },
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
  current_price_cents: 10000,
  current_winner_id: undefined,
  accepted_bid_count: 0,
  extend_count: 0,
  item: { id: 'item_next', title: '和田玉福牌吊坠', description: 'next 珠宝可视验收' },
  rule: {
    ...auctionLive.rule,
    frozen_at: undefined
  }
};

async function mockH5(page: Page) {
  await freezeVisualClock(page);
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
          seq: 41,
          end_at: '2099-05-22T14:00:00Z',
          server_time_ms: Date.parse('2099-05-22T13:58:45Z'),
          item: { title: '天然翡翠A货平安扣吊坠', image_url: productImageDataURL, certificate: 'GID 20260607 · 可核验', condition: '品相完整', shipping: '顺丰包邮' }
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
          item: { title: '天然翡翠A货平安扣吊坠', image_url: productImageDataURL, certificate: 'GID 20260607 · 可核验', condition: '品相完整', shipping: '顺丰包邮' }
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
      json: { items: [{ order_id: 'ord_pending', auction_id: 'auc_live', amount_cents: 60000, order_status: 'ORDER_PENDING' }] }
    });
  });
  await page.route('/api/rooms/room_main/chat?limit=30', async (route) => {
    await route.fulfill({
      json: {
        items: [
          { id: 1, room_id: 'room_main', user_id: 'user_2', body: '证书和瑕疵都已展示', created_at: '2026-05-22T13:00:00Z' }
        ]
      }
    });
  });
  await page.route('/api/auth/ws-ticket', async (route) => {
    await route.fulfill({ json: { ticket: 'ticket_visual', expires_in_ms: 60000 } });
  });
  await page.addInitScript(() => {
    class VisualWebSocket extends EventTarget {
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
      readyState = VisualWebSocket.CONNECTING;
      onopen: ((event: Event) => void) | null = null;
      onmessage: ((event: MessageEvent) => void) | null = null;
      onerror: ((event: Event) => void) | null = null;
      onclose: ((event: CloseEvent) => void) | null = null;
      url: string;

      constructor(url: string | URL) {
        super();
        this.url = String(url);
        window.setTimeout(() => {
          this.readyState = VisualWebSocket.OPEN;
          const event = new Event('open');
          this.onopen?.(event);
          this.dispatchEvent(event);
        }, 0);
      }

      close() {
        this.readyState = VisualWebSocket.CLOSED;
      }

      send() {
        return undefined;
      }
    }
    window.WebSocket = VisualWebSocket as unknown as typeof WebSocket;
  });
}

async function mockPC(page: Page) {
  await freezeVisualClock(page);
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
  await page.route('/api/monitor/outbox/watermarks', async (route) => route.fulfill({
    json: { items: [{ shard_id: 0, ack_pending_count: 1, oldest_ready_age_ms: 1200 }] }
  }));
  await page.route('/api/monitor/snapshots', async (route) => route.fulfill({
    json: { items: [{ request_id: 'snap_1', auction_id: 'auc_live', source: 'db', status: 'COMPLETED', stale: false }] }
  }));
  await page.route('/api/monitor/signals', async (route) => route.fulfill({
    json: { items: [{ id: 1, signal_type: 'force_snapshot_rebuild', target_type: 'auction', target_id: 'auc_live', status: 'SUCCEEDED' }] }
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
  await page.route('/api/monitor/auctions/auc_live/flight-recorder?limit=20&timeline_limit=20', async (route) => route.fulfill({
    json: {
      timeline: [
        { kind: 'auction_event', event_type: 'bid_accepted', seq: 42, trace_id: 'tr_bid_42' },
        { kind: 'outbox', status: 'PUBLISHED', outbox_id: 7, seq: 42 }
      ]
    }
  }));
  await page.route('/api/host/auctions/auc_live/prompts', async (route) => route.fulfill({
    json: {
      auction_id: 'auc_live',
      room_id: 'room_main',
      generated_at: '2026-05-22T13:59:50Z',
      prompts: [
        {
          id: 'auc_live:last_10_seconds',
          type: 'last_10_seconds',
          severity: 'HIGH',
          title: '最后窗口',
          body: '竞拍进入最后 10 秒，建议提醒下一口有效价和延时规则。',
          action: 'highlight_countdown',
          source: 'auction',
          auction_id: 'auc_live',
          room_id: 'room_main',
          event_seq: 42,
          generated_at: '2026-05-22T13:59:50Z',
          window_seconds: 10,
          metric_value: 8,
          metric_label: 'seconds_remaining',
          reference_price_cents: 50000
        }
      ]
    }
  }));
  await page.route('/api/host/auctions/auc_live/heat-summary', async (route) => route.fulfill({
    json: {
      auction_id: 'auc_live',
      room_id: 'room_main',
      status: 'ACTIVE',
      generated_at: '2026-05-22T13:59:50Z',
      window_seconds: 30,
      active_bidders_30s: 2,
      accepted_bids_30s: 3,
      rejected_bids_30s: 1,
      chat_messages_30s: 4,
      recovery_events_30s: 1,
      watcher_count_available: false,
      source: 'postgres:bids,chat_messages,user_activity_events'
    }
  }));
  await page.route('/api/monitor/auctions/auc_next/flight-recorder?limit=20&timeline_limit=20', async (route) => route.fulfill({
    json: { timeline: [] }
  }));
  await page.route('/api/host/auctions/auc_next/prompts', async (route) => route.fulfill({
    json: { auction_id: 'auc_next', room_id: 'room_main', prompts: [] }
  }));
  await page.route('/api/host/auctions/auc_next/heat-summary', async (route) => route.fulfill({
    json: {
      auction_id: 'auc_next',
      room_id: 'room_main',
      status: 'DRAFT',
      generated_at: '2026-05-22T13:59:50Z',
      window_seconds: 30,
      active_bidders_30s: 0,
      accepted_bids_30s: 0,
      rejected_bids_30s: 0,
      chat_messages_30s: 0,
      recovery_events_30s: 0,
      watcher_count_available: false,
      source: 'postgres:bids,chat_messages,user_activity_events'
    }
  }));
}

async function stabilize(page: Page) {
  await page.addStyleTag({
    content: `
      *, *::before, *::after {
        animation-duration: 0s !important;
        animation-delay: 0s !important;
        transition-duration: 0s !important;
        transition-delay: 0s !important;
      }
      video {
        visibility: hidden !important;
      }
      .live-video-bg {
        background: var(--stage-media-url) center / cover no-repeat !important;
      }
    `
  });
  await page.evaluate(() => {
    for (const video of Array.from(document.querySelectorAll('video'))) {
      video.pause();
      video.currentTime = 0;
    }
  });
}

async function freezeVisualClock(page: Page) {
  await page.addInitScript(() => {
    const fixedNow = Date.parse('2026-05-22T13:58:45Z');
    Date.now = () => fixedNow;
  });
}

test.describe('H5 visual states @visual-h5', () => {
  test.beforeEach(async ({ page }) => {
    await mockH5(page);
  });

  for (const state of [
    ['active', '竞价中'],
    ['self-leading', '领先中'],
    ['outbid-rejected', '被拒绝'],
    ['recovering', '恢复中'],
    ['sold-winner', '成交'],
    ['sold-loser', '已成交'],
    ['unsold-ended', '流拍']
  ] as const) {
    test(`captures H5 ${state[0]} state`, async ({ page }) => {
      await page.goto('/?stateMatrix=1');
      await stabilize(page);
      await page.getByRole('button', { name: '关闭竞拍面板' }).click();
      await page.getByRole('button', { name: state[1], exact: true }).click();
      if (state[0].startsWith('sold') || state[0] === 'unsold-ended') {
        await expect(page.getByTestId('result-sheet')).toBeVisible();
      } else {
        await page.getByTestId('floating-product-card').click();
        await expect(page.getByLabel('auction-state')).toBeVisible();
        await expect(page.getByTestId('bid-cta')).toBeVisible();
      }
      await expect(page).toHaveScreenshot(`h5-${state[0]}.png`, {
        animations: 'disabled',
        caret: 'hide',
        fullPage: false,
        mask: [
          page.getByTestId('floating-auction-price'),
          page.getByTestId('floating-auction-countdown'),
          page.getByTestId('auction-price'),
          page.getByTestId('auction-countdown'),
          page.getByTestId('bid-cta')
        ],
        timeout: 10_000
      });
    });
  }
});

test.describe('PC visual states @visual-pc', () => {
  test.beforeEach(async ({ page }) => {
    await mockPC(page);
  });

  test('captures PC command and diagnostics initial state', async ({ page }) => {
    await page.goto('/');
    await stabilize(page);
    await expect(page.getByTestId('pc-command-center')).toBeVisible();
    await expect(page.getByTestId('health-ribbon')).toBeVisible();
    await expect(page.getByTestId('auction-control-summary')).toBeVisible();
    await page.getByRole('button', { name: '诊断', exact: true }).click();
    await expect(page.getByTestId('diagnostics')).toBeVisible();
    await expect(page.locator('.console-shell')).toHaveScreenshot('pc-command-diagnostics.png', {
      animations: 'disabled',
      caret: 'hide',
      timeout: 10_000
    });
  });
});
