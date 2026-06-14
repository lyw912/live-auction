import { expect, test, type Page } from '@playwright/test';

const productImageDataURL = 'data:image/svg+xml;utf8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%20600%20800%22%3E%3Crect%20width%3D%22600%22%20height%3D%22800%22%20fill%3D%22%23222f2b%22%2F%3E%3Ccircle%20cx%3D%22300%22%20cy%3D%22300%22%20r%3D%22142%22%20fill%3D%22%23e5f3ef%22%2F%3E%3Cellipse%20cx%3D%22300%22%20cy%3D%22320%22%20rx%3D%22164%22%20ry%3D%2286%22%20fill%3D%22%2310b981%22%2F%3E%3Ctext%20x%3D%22300%22%20y%3D%22640%22%20text-anchor%3D%22middle%22%20font-size%3D%2248%22%20font-family%3D%22Arial%22%20fill%3D%22white%22%3ELOT%20A-102%3C%2Ftext%3E%3C%2Fsvg%3E';

async function installAuctionMocks(page: Page) {
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
            title: '天然翡翠A货平安扣吊坠',
            description: '天然A货翡翠平安扣，附GID证书，顺丰包邮。',
            image_url: productImageDataURL,
            certificate: 'GID 20260607 · 可核验',
            condition: '品相完整',
            shipping: '顺丰包邮'
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
            title: '天然翡翠A货平安扣吊坠',
            image_url: productImageDataURL,
            certificate: 'GID 20260607 · 可核验',
            condition: '品相完整',
            shipping: '顺丰包邮'
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
  await page.route('/api/auctions/auc_live/heat', async (route) => {
    await route.fulfill({ json: { auction_id: 'auc_live', accepted_bidder_count: 2, active_bidders_30s: 2, accepted_bids_30s: 3, price_velocity_cents_per_min: 10000, source: 'leaderboard' } });
  });
  await page.route('/api/users/me/orders**', async (route) => route.fulfill({ json: { items: [] } }));
  await page.route('/api/users/me/bids**', async (route) => route.fulfill({ json: { items: [] } }));
  await page.route('/api/rooms/room_main/chat?limit=30', async (route) => route.fulfill({ json: { items: [] } }));
  await page.route('/api/rooms/room_main/system-messages?limit=10', async (route) => route.fulfill({ json: { items: [] } }));
  await page.route('/api/rooms/room_main/liveops', async (route) => {
    await route.fulfill({
      json: {
        id: 'loc_test',
        room_id: 'room_main',
        status: 'ACTIVE',
        title: '开拍前准备',
        description: '完成准备任务。',
        progress: 0,
        lucky_draw: { status: 'READY', participants: 0, eligible_task_count: 4, completed_task_count: 0, can_enter: false },
        tasks: [],
        team_scores: [],
        disclaimer: '互动不影响价格、排名、成交或保证金。'
      }
    });
  });
  await page.route('/api/auth/ws-ticket', async (route) => {
    await route.fulfill({ json: { ticket: 'ticket_media_test', expires_in_ms: 60000 } });
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

      constructor(url: string | URL) {
        super();
        this.url = String(url);
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
    }
    window.WebSocket = MockAuctionWebSocket as unknown as typeof WebSocket;
  });
}

async function expectAuctionStillInteractive(page: Page) {
  await expect(page.getByTestId('live-stage')).toBeVisible();
  await expect(page.getByTestId('floating-auction-countdown')).toBeVisible();
  await expect(page.getByTestId('floating-product-card')).toBeVisible();
  await page.getByTestId('floating-product-card').click();
  await expect(page.getByLabel('auction-state')).toBeVisible();
  await expect(page.getByTestId('auction-countdown')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveText(/出一手 ¥400.00/);
  await expect(page.getByTestId('bid-cta')).toBeEnabled();
  await expect(page.getByLabel('auction-state').getByTestId('auction-price')).toHaveText('¥350.00');
}

test.beforeEach(async ({ page }) => {
  await installAuctionMocks(page);
});

test('media descriptor 404 leaves bidding and countdown interactive', async ({ page }) => {
  await page.route('/api/live/sessions/auc_live', async (route) => {
    await route.fulfill({ status: 404, json: { code: 'NOT_FOUND', message: 'missing live session' } });
  });
  await page.goto('/');
  await expect(page.locator('.live-video-bg')).toHaveAttribute('data-media-status', 'descriptor-error');
  await expect(page.locator('.live-video-bg')).toHaveAttribute('data-media-protocol', 'mp4');
  await expectAuctionStillInteractive(page);
});

test('pending media descriptor does not block auction render or bid CTA', async ({ page }) => {
  await page.route('/api/live/sessions/auc_live', async () => {
    await new Promise(() => undefined);
  });
  await page.goto('/');
  await expect(page.locator('.live-video-bg')).toHaveAttribute('data-media-protocol', 'mp4');
  await expectAuctionStillInteractive(page);
});

test('bad hls source falls back to mp4 without touching auction truth', async ({ page }) => {
  await page.addInitScript(() => {
    const originalCanPlayType = HTMLMediaElement.prototype.canPlayType;
    HTMLMediaElement.prototype.canPlayType = function canPlayType(type: string) {
      if (type.includes('mpegurl') || type.includes('m3u8')) return '';
      return originalCanPlayType.call(this, type);
    };
    Object.defineProperty(window, 'MediaSource', {
      configurable: true,
      value: {
        isTypeSupported: () => false
      }
    });
  });
  await page.route('/api/live/sessions/auc_live', async (route) => {
    await route.fulfill({
      json: {
        auctionId: 'auc_live',
        isLive: true,
        posterURL: productImageDataURL,
        sources: [
          { protocol: 'll-hls', url: '/demo/bad/index.m3u8', mimeType: 'application/vnd.apple.mpegurl', priority: 10 },
          { protocol: 'mp4', url: '/demo/jade-live-loop.mp4', mimeType: 'video/mp4', priority: 90 }
        ],
        latencyTargetMs: 3000,
        capabilities: { nativeHlsOnSafari: true, mseHls: true, webrtc: false }
      }
    });
  });
  await page.goto('/');
  await expect(page.locator('.live-video-bg')).toHaveAttribute('data-media-protocol', 'mp4');
  await expectAuctionStillInteractive(page);
});
