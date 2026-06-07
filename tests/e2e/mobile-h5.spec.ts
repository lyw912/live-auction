import { expect, test, type Page } from '@playwright/test';

const productImageDataURL = 'data:image/svg+xml;utf8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%20600%20800%22%3E%3Crect%20width%3D%22600%22%20height%3D%22800%22%20fill%3D%22%23222f2b%22%2F%3E%3Ccircle%20cx%3D%22300%22%20cy%3D%22300%22%20r%3D%22142%22%20fill%3D%22%23e5f3ef%22%2F%3E%3Cellipse%20cx%3D%22300%22%20cy%3D%22320%22%20rx%3D%22164%22%20ry%3D%2286%22%20fill%3D%22%2310b981%22%2F%3E%3Cpath%20d%3D%22M170%20352c52%2068%20208%2068%20260%200%22%20stroke%3D%22%23d6a84f%22%20stroke-width%3D%2218%22%20fill%3D%22none%22%20stroke-linecap%3D%22round%22%2F%3E%3Ctext%20x%3D%22300%22%20y%3D%22640%22%20text-anchor%3D%22middle%22%20font-size%3D%2248%22%20font-family%3D%22Arial%22%20fill%3D%22white%22%3ELOT%20A-102%3C%2Ftext%3E%3C%2Fsvg%3E';

async function openBidPanel(page: Page) {
  await page.getByTestId('floating-product-card').click();
  await expect(page.getByLabel('auction-state')).toBeVisible();
}

async function selectActiveBidsState(page: Page) {
  await page.getByRole('button', { name: '竞价中' }).click();
  const auctionState = page.getByLabel('auction-state');
  await expect(auctionState.locator('.eyebrow')).toHaveText('竞价中');
  await expect(auctionState.getByTestId('auction-price')).toHaveText('¥350.00');
  await expect(page.getByTestId('bid-cta')).toHaveText(/出一手 ¥400.00/);
}

async function expectDockConnection(page: Page, text: string) {
  await expect(page.getByLabel('auction-state').locator('.signal-row')).toContainText(text);
}

async function raisePreparedBid(page: Page, amountText: string) {
  const amount = page.getByLabel('auction-state').locator('.bid-stepper').locator('span');
  for (let attempt = 0; attempt < 3; attempt += 1) {
    if ((await amount.textContent().catch(() => ''))?.includes(amountText)) return;
    await page.getByRole('button', { name: 'increase' }).click();
    await page.waitForTimeout(50);
  }
  await expect(amount).toContainText(amountText);
}

async function openLiveOpsPanel(page: Page) {
  if (await page.getByTestId('live-ops-panel').isVisible().catch(() => false)) return;
  await page.getByRole('button', { name: '直播互动' }).click();
  await expect(page.getByTestId('live-ops-panel')).toBeVisible();
}

test.beforeEach(async ({ page }) => {
  const liveOpsProgress = new Set<string>();
  let liveOpsTeam: 'craft' | 'story' | undefined;
  let liveOpsDrawStatus: 'READY' | 'ENTERED' | 'OPENED' = 'READY';
  const liveOpsPayload = () => ({
    id: 'loc_test',
    room_id: 'room_main',
    status: 'ACTIVE',
    title: '开拍前准备',
    description: '完成信息查看、关注、问答和榜单确认，帮助自己理解本场规则。',
    progress: liveOpsProgress.size,
    lucky_draw: {
      status: liveOpsDrawStatus,
      title: '开拍福袋',
      description: '完成准备后参与，开奖展示演示奖励，不影响价格或中标。',
      opens_at: '2099-05-22T14:00:00Z',
      server_time: '2099-05-22T13:58:45Z',
      participants: liveOpsDrawStatus === 'READY' ? 0 : 1,
      my_entry_status: liveOpsDrawStatus === 'READY' ? undefined : liveOpsDrawStatus,
      my_reward_key: liveOpsDrawStatus === 'OPENED' ? 'badge' : undefined,
      my_reward_label: liveOpsDrawStatus === 'OPENED' ? '直播间高光入场牌' : undefined,
      eligible_task_count: 4,
      completed_task_count: liveOpsProgress.size,
      can_enter: liveOpsProgress.size >= 4
    },
    my_team: liveOpsTeam,
    team_scores: [
      { key: 'craft', label: '工艺派', count: liveOpsTeam === 'craft' ? 1 : 0 },
      { key: 'story', label: '故事派', count: liveOpsTeam === 'story' ? 1 : 0 }
    ],
    disclaimer: '福袋和阵营为比赛演示玩法；奖励不影响价格、排名、成交或保证金。',
    tasks: ['watch', 'follow', 'ask', 'leaderboard'].map((key) => ({
      key,
      label: ({ watch: '看拍品', follow: '关注', ask: '问拍品', leaderboard: '看榜单' } as Record<string, string>)[key],
      description: key,
      ...(liveOpsProgress.has(key) ? { completed_at: '2026-06-06T08:00:00Z' } : {})
    }))
  });
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
            description: '天然A货翡翠平安扣，附GID证书，顺丰包邮，支持7天无理由。',
            image_url: productImageDataURL,
            certificate: 'GID 20260607 · 可核验',
            condition: '品相完整',
            shipping: '顺丰包邮',
            dimensions: '直径 9.2cm',
            material: '天然A货翡翠',
            flaws: '以实物图为准',
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
            title: '和田玉福牌吊坠',
            image_url: productImageDataURL,
            certificate: '国检证书',
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
        max_bid_intent: {
          id: 'mbi_existing',
          auction_id: 'auc_live',
          user_id: 'user_1',
          max_amount_cents: 55000,
          status: 'ACTIVE',
          source: 'MAX_BID',
          last_applied_seq: 40,
          version: 1
        },
        payload: {
          status: 'ACTIVE',
          current_price_cents: 35000,
          leader_user_masked: '张**',
          current_winner_id: 'user_2',
          end_at: '2099-05-22T14:00:00Z',
          server_time_ms: Date.parse('2099-05-22T13:58:45Z'),
          item: {
            title: '天然翡翠A货平安扣吊坠',
            description: '天然A货翡翠平安扣，附GID证书，顺丰包邮，支持7天无理由。',
            image_url: productImageDataURL,
            certificate: 'GID 20260607 · 可核验',
            condition: '品相完整',
            shipping: '顺丰包邮',
            dimensions: '直径 9.2cm',
            material: '天然A货翡翠',
            flaws: '以实物图为准',
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
          { id: 1, room_id: 'room_main', user_id: 'user_2', body: '这件翡翠水头不错', created_at: '2026-05-22T13:00:00Z' }
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
  await page.route('/api/rooms/room_main/liveops', async (route) => {
    await route.fulfill({ json: liveOpsPayload() });
  });
  await page.route('/api/rooms/room_main/liveops/tasks/*', async (route, request) => {
    const task = new URL(request.url()).pathname.split('/').pop() ?? '';
    if (!['watch', 'follow', 'ask', 'leaderboard'].includes(task)) {
      await route.fulfill({ status: 400, json: { code: 'INVALID_ARGUMENT', message: 'unknown liveops task' } });
      return;
    }
    liveOpsProgress.add(task);
    await route.fulfill({ json: liveOpsPayload() });
  });
  await page.route('/api/rooms/room_main/liveops/team', async (route, request) => {
    const body = request.postDataJSON() as { team_key?: 'craft' | 'story' };
    if (body.team_key !== 'craft' && body.team_key !== 'story') {
      await route.fulfill({ status: 400, json: { code: 'INVALID_ARGUMENT', message: 'unknown liveops team' } });
      return;
    }
    liveOpsTeam = body.team_key;
    await route.fulfill({ json: liveOpsPayload() });
  });
  await page.route('/api/rooms/room_main/liveops/lucky-draw/enter', async (route) => {
    if (liveOpsProgress.size < 4) {
      await route.fulfill({ status: 400, json: { code: 'INVALID_ARGUMENT', message: 'complete tasks' } });
      return;
    }
    liveOpsDrawStatus = 'ENTERED';
    await route.fulfill({ json: liveOpsPayload() });
  });
  await page.route('/api/rooms/room_main/liveops/lucky-draw/open', async (route) => {
    if (liveOpsDrawStatus === 'READY') {
      await route.fulfill({ status: 400, json: { code: 'INVALID_ARGUMENT', message: 'enter first' } });
      return;
    }
    liveOpsDrawStatus = 'OPENED';
    await route.fulfill({ json: liveOpsPayload() });
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

test('H5 renders honest heat and all visible live actions are interactive', async ({ page }) => {
  const liveOpsTasks: string[] = [];
  const liveOpsTeams: string[] = [];
  await page.route('/api/rooms/room_main/liveops/tasks/*', async (route, request) => {
    const task = new URL(request.url()).pathname.split('/').pop() ?? '';
    liveOpsTasks.push(task);
    await route.fallback();
  });
  await page.route('/api/rooms/room_main/liveops/team', async (route, request) => {
    liveOpsTeams.push((request.postDataJSON() as { team_key?: string }).team_key ?? '');
    await route.fallback();
  });
  await page.goto('/');
  await expect(page.getByText('2333')).toHaveCount(0);
  await expect(page.locator('.viewer-count.avatar-stack')).toContainText('近30秒 2 人');
  await expect(page.getByTestId('heat-meter')).toContainText('近30秒 2 人 · 3 次出价');
  await expect(page.getByText('热点 明星大货撑')).toHaveCount(0);
  await expect(page.getByText('古玩榜第 8 名')).toHaveCount(0);
  await expect(page.getByText('保证金锁定')).toHaveCount(0);

  await page.getByTestId('live-stage').getByRole('button', { name: '关注' }).click();
  await expect(page.getByTestId('live-stage').getByRole('button', { name: '已关注' })).toBeVisible();
  await expect.poll(() => liveOpsTasks).toContain('follow');
  await page.getByRole('button', { name: '点赞' }).click();
  await expect(page.getByRole('button', { name: '点赞' })).toContainText('1');
  await openLiveOpsPanel(page);
  await expect(page.getByTestId('warmup-card')).toContainText('暖场任务');
  await page.getByTestId('warmup-card').getByRole('button', { name: '看拍品' }).click();
  await expect(page.getByTestId('bottom-sheet')).toContainText('本场');
  await page.getByTestId('bottom-sheet').getByRole('button', { name: '关闭' }).click();
  await openLiveOpsPanel(page);
  await expect(page.getByTestId('warmup-card')).toContainText('2/4 已完成');
  await expect(page.getByTestId('buyer-pk-card')).toContainText('买家阵营');
  await page.getByTestId('buyer-pk-card').getByRole('button', { name: /故事派/ }).click();
  await expect.poll(() => liveOpsTeams).toContain('story');
  await expect(page.getByTestId('buyer-pk-card').getByRole('button', { name: /故事派/ })).toHaveClass(/active/);
  await page.getByTestId('entry-effect-card').click();
  await expect(page.getByTestId('leaderboard-sheet')).toBeVisible();
  await page.getByTestId('bottom-sheet').getByRole('button', { name: '关闭' }).click();
  await openLiveOpsPanel(page);
  await page.getByTestId('warmup-card').getByRole('button', { name: '看榜单' }).click();
  await expect.poll(() => liveOpsTasks).toContain('leaderboard');
  await expect(page.getByTestId('leaderboard-sheet')).toBeVisible();
  await page.getByTestId('bottom-sheet').getByRole('button', { name: '关闭' }).click();
  await expect(page.getByTestId('live-stage').getByRole('button', { name: '已关注' })).toBeVisible();
  await page.getByRole('button', { name: '更多' }).click();
  await expect(page.getByTestId('more-sheet')).toBeVisible();
  await expect(page.getByTestId('buyer-trust-card')).toContainText('本场受反作弊保护');
  await expect(page.getByTestId('buyer-trust-card')).toContainText('价格、倒计时和有效出价以服务端为准');
  await expect(page.getByTestId('buyer-trust-card')).toContainText('页面不展示虚构观看人数');
  await expect(page.getByTestId('buyer-trust-card')).toContainText('不向买家泄露风控策略');
  await page.getByTestId('bottom-sheet').getByRole('button', { name: '关闭' }).click();
  await expect(page.getByTestId('more-sheet')).toHaveCount(0);
});

test('H5 product QA completes liveops ask task only after a real answer', async ({ page }) => {
  const liveOpsTasks: string[] = [];
  let luckyEntered = false;
  let luckyOpened = false;
  let qaRequest: Record<string, unknown> | undefined;
  await page.route('/api/rooms/room_main/liveops/tasks/*', async (route, request) => {
    const task = new URL(request.url()).pathname.split('/').pop() ?? '';
    liveOpsTasks.push(task);
    await route.fallback();
  });
  await page.route('/api/rooms/room_main/liveops/lucky-draw/enter', async (route) => {
    luckyEntered = true;
    await route.fallback();
  });
  await page.route('/api/rooms/room_main/liveops/lucky-draw/open', async (route) => {
    luckyOpened = true;
    await route.fallback();
  });
  await page.route('/api/rooms/room_main/product-qa', async (route, request) => {
    qaRequest = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({
      json: {
        auction_id: 'auc_live',
        thread_id: qaRequest.thread_id,
        question: qaRequest.question,
        answer: '起拍价是 ¥350.00，加价阶梯是 ¥50.00。',
        facts_used: ['current_price_cents', 'increment_cents'],
        safety_note: '仅基于本场已展示信息。',
        follow_up_prompts: ['有瑕疵说明吗？']
      }
    });
  });

  await page.goto('/');
  await openLiveOpsPanel(page);
  await page.getByTestId('warmup-card').getByRole('button', { name: '问拍品' }).click();
  await expect(page.getByTestId('product-qa-sheet')).toBeVisible();
  await expect(page.getByTestId('product-qa-sheet')).toContainText('可问拍品详情、竞拍规则和履约保障');
  await page.getByLabel('product-qa-input').fill('起拍价是多少？');
  await page.getByLabel('ask-product-qa').click();

  await expect(page.getByLabel('product-qa-thread')).toContainText('起拍价是 ¥350.00');
  await expect.poll(() => qaRequest?.question).toBe('起拍价是多少？');
  await expect.poll(() => liveOpsTasks).toContain('ask');
  await page.getByTestId('bottom-sheet').getByRole('button', { name: '关闭' }).click();
  await openLiveOpsPanel(page);
  await expect(page.getByTestId('warmup-card')).toContainText('1/4 已完成');
  await page.getByTestId('warmup-card').getByRole('button', { name: '看拍品' }).click();
  await page.getByTestId('bottom-sheet').getByRole('button', { name: '关闭' }).click();
  await openLiveOpsPanel(page);
  await page.getByTestId('warmup-card').getByRole('button', { name: '关注' }).click();
  await openLiveOpsPanel(page);
  await page.getByTestId('warmup-card').getByRole('button', { name: '看榜单' }).click();
  await page.getByTestId('bottom-sheet').getByRole('button', { name: '关闭' }).click();
  await openLiveOpsPanel(page);
  await expect(page.getByTestId('warmup-card')).toContainText('4/4 已完成');
  await page.getByTestId('lucky-draw-card').getByRole('button', { name: '参与福袋' }).click();
  await expect.poll(() => luckyEntered).toBe(true);
  await expect(page.getByTestId('lucky-draw-card')).toContainText('可开奖');
  await page.getByTestId('lucky-draw-card').getByRole('button', { name: '开奖' }).click();
  await expect.poll(() => luckyOpened).toBe(true);
  await expect(page.getByTestId('lucky-draw-card')).toContainText('直播间高光入场牌');
});

test('H5 disables bid CTA for unsafe states and keeps text inside controls', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  const unsafeStates = ['领先中', '提交中', '恢复中', '已断开', '流拍', '已取消', '已成交'];
  for (const state of unsafeStates) {
    await page.getByRole('button', { name: state }).click();
    await expect(page.getByTestId('bid-cta')).toBeDisabled();
  }

  await page.goto('/?stateMatrix=1');
  await selectActiveBidsState(page);
  await expect(page.getByTestId('bid-cta')).toBeEnabled();

  await page.getByRole('button', { name: '提交中' }).click();
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('提交中');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await expect(page.getByTestId('auction-countdown')).toBeVisible();
  await page.getByRole('button', { name: '出价榜' }).click();
  await expect(page.getByText('2 次')).toBeVisible();
  await expect(page.getByText('1 次')).toBeVisible();

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
  await page.getByRole('button', { name: /进入竞拍面板/ }).click();
  await expectDockConnection(page, '已连接');
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

  await expect(page.getByText('竞拍状态已更新')).toBeVisible();
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥400.00');
  await expect(page.getByLabel('auction-state').getByText('陈** 领先').first()).toBeVisible();
});

test('H5 applies WebSocket leaderboard delta without polling leaderboard', async ({ page }) => {
  let leaderboardReads = 0;
  await page.route('/api/auctions/auc_live/leaderboard?limit=5', async (route) => {
    leaderboardReads += 1;
    await route.fallback();
  });

  await page.goto('/?stateMatrix=1');
  await expectDockConnection(page, '已连接');
  await expect(page.getByTestId('leaderboard-panel')).toContainText('榜一');
  const readsAfterInitialLoad = leaderboardReads;

  await page.evaluate(() => {
    const [entry] = ((window as typeof window & { __auctionWS?: Array<{ url: string; socket: { dispatchServerMessage: (payload: unknown) => void } }> }).__auctionWS ?? [])
      .filter(({ url }) => url.includes('/ws?'));
    entry.socket.dispatchServerMessage({
      auction_id: 'auc_live',
      event_type: 'leaderboard_delta',
      seq: 44,
      current_price_cents: 52000,
      next_valid_bid_cents: 57000,
      current_winner_id: 'user_1',
      leader_amount_cents: 52000,
      accepted_bidder_count: 3,
      active_bidders_30s: 3,
      accepted_bids_30s: 6,
      price_velocity_cents_per_min: 12000,
      entries: [
        { rank: 1, user_id: 'user_1', user_masked: '我**', amount_cents: 52000, bid_count: 4 },
        { rank: 2, user_id: 'user_2', user_masked: '陈**', amount_cents: 50000, bid_count: 3 }
      ]
    });
  });

  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥520.00');
  await expect(page.getByTestId('leaderboard-panel-rank-strip')).toContainText('第 1 名');
  await expect(page.getByTestId('leaderboard-panel-rank-strip')).toContainText('正在领先');
  await expect(page.getByTestId('leaderboard-panel')).toContainText('4 次');
  expect(leaderboardReads).toBe(readsAfterInitialLoad);
});

test('H5 renders always-on race board and waterfall from authoritative leaderboard delta', async ({ page }) => {
  await page.goto('/');
  await expect.poll(async () => page.evaluate(() => {
    return ((window as typeof window & { __auctionWS?: Array<{ url: string }> }).__auctionWS ?? [])
      .some(({ url }) => url.includes('/ws?'));
  })).toBe(true);
  await expect(page.getByTestId('live-stage').getByTestId('race-board')).toBeVisible();

  await page.evaluate(() => {
    const [entry] = ((window as typeof window & { __auctionWS?: Array<{ url: string; socket: { dispatchServerMessage: (payload: unknown) => void } }> }).__auctionWS ?? [])
      .filter(({ url }) => url.includes('/ws?'));
    entry.socket.dispatchServerMessage({
      auction_id: 'auc_live',
      event_type: 'leaderboard_delta',
      seq: 44,
      current_price_cents: 52000,
      next_valid_bid_cents: 57000,
      current_winner_id: 'user_1',
      leader_amount_cents: 52000,
      accepted_bidder_count: 3,
      active_bidders_30s: 3,
      accepted_bids_30s: 6,
      price_velocity_cents_per_min: 12000,
      entries: [
        { rank: 1, user_id: 'user_1', user_masked: '我**', amount_cents: 52000, bid_count: 4 },
        { rank: 2, user_id: 'user_2', user_masked: '陈**', amount_cents: 50000, bid_count: 3 }
      ]
    });
  });

  const stage = page.getByTestId('live-stage');
  await expect(stage).toHaveAttribute('data-atmosphere-gated', 'false');
  await expect(stage).toHaveAttribute('data-atmosphere-intensity', '3');
  await expect(stage.getByTestId('race-board')).toContainText('榜一 我 ¥520.00');
  await expect(stage.getByTestId('race-board')).toContainText('竞速榜');
  await expect(stage.getByTestId('bid-waterfall')).toBeVisible();
  await expect(stage.getByTestId('atmosphere-cue')).toContainText('领先');

  await page.evaluate(() => {
    const [entry] = ((window as typeof window & { __auctionWS?: Array<{ url: string; socket: { dispatchServerMessage: (payload: unknown) => void } }> }).__auctionWS ?? [])
      .filter(({ url }) => url.includes('/ws?'));
    entry.socket.dispatchServerMessage({
      auction_id: 'auc_live',
      event_type: 'leaderboard_delta',
      seq: 45,
      current_price_cents: 57000,
      next_valid_bid_cents: 62000,
      current_winner_id: 'user_2',
      leader_amount_cents: 57000,
      accepted_bidder_count: 3,
      active_bidders_30s: 3,
      accepted_bids_30s: 7,
      price_velocity_cents_per_min: 15000,
      entries: [
        { rank: 1, user_id: 'user_2', user_masked: '陈**', amount_cents: 57000, bid_count: 4 },
        { rank: 2, user_id: 'user_1', user_masked: '我**', amount_cents: 52000, bid_count: 4 }
      ]
    });
  });

  await expect(stage.getByTestId('race-board')).toContainText('我 #2 差 ¥50.00');
  await expect(stage.getByTestId('atmosphere-cue')).toContainText('差 ¥50.00');
  await expect(stage.getByTestId('atmosphere-cue')).toContainText('立即反超');
});

test('H5 coalesces burst leaderboard deltas to the latest visible rank state', async ({ page }) => {
  let leaderboardReads = 0;
  await page.route('/api/auctions/auc_live/leaderboard?limit=5', async (route) => {
    leaderboardReads += 1;
    await route.fallback();
  });

  await page.goto('/?stateMatrix=1');
  await expectDockConnection(page, '已连接');
  await expect.poll(async () => page.evaluate(() => {
    return ((window as typeof window & { __auctionWS?: Array<{ url: string }> }).__auctionWS ?? [])
      .some(({ url }) => url.includes('/ws?'));
  })).toBe(true);
  const readsAfterInitialLoad = leaderboardReads;

  await page.evaluate(() => {
    const [entry] = ((window as typeof window & { __auctionWS?: Array<{ url: string; socket: { dispatchServerMessage: (payload: unknown) => void } }> }).__auctionWS ?? [])
      .filter(({ url }) => url.includes('/ws?'));
    for (const payload of [
      { seq: 44, current_price_cents: 52000, user: '张**', bids: 4 },
      { seq: 45, current_price_cents: 57000, user: '陈**', bids: 5 },
      { seq: 46, current_price_cents: 62000, user: '我**', bids: 6 }
    ]) {
      entry.socket.dispatchServerMessage({
        auction_id: 'auc_live',
        event_type: 'leaderboard_delta',
        seq: payload.seq,
        current_price_cents: payload.current_price_cents,
        next_valid_bid_cents: payload.current_price_cents + 5000,
        current_winner_id: 'user_1',
        leader_amount_cents: payload.current_price_cents,
        accepted_bidder_count: 3,
        entries: [
          { rank: 1, user_id: 'user_1', user_masked: payload.user, amount_cents: payload.current_price_cents, bid_count: payload.bids },
          { rank: 2, user_id: 'user_2', user_masked: '陈**', amount_cents: payload.current_price_cents - 5000, bid_count: 3 }
        ]
      });
    }
  });

  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥620.00');
  await expect(page.getByTestId('leaderboard-panel')).toContainText('6 次');
  await expect(page.getByTestId('leaderboard-panel')).not.toContainText('4 次');
  expect(leaderboardReads).toBe(readsAfterInitialLoad);
});

test('H5 keeps recovery snapshot quiet while WebSocket is healthy', async ({ page }) => {
  let snapshotReads = 0;
  await page.route('/api/auctions/auc_live', async (route) => {
    snapshotReads += 1;
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
          server_time_ms: Date.parse('2099-05-22T13:58:45Z')
        }
      }
    });
  });

  await page.goto('/');
  await page.getByRole('button', { name: /进入竞拍面板/ }).click();
  await expectDockConnection(page, '已连接');
  await page.waitForTimeout(3200);
  expect(snapshotReads).toBe(1);
});

test('H5 disables bid CTA while WebSocket is still connecting', async ({ page }) => {
  await page.route('/api/auth/ws-ticket', async (route) => {
    await route.fulfill({
      status: 429,
      headers: { 'Retry-After': '2' },
      body: 'ws admission retry later'
    });
  });

  await page.goto('/');
  await page.getByRole('button', { name: /进入竞拍面板/ }).click();
  await expectDockConnection(page, '连接中');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await page.waitForTimeout(500);
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
});

test('H5 recovering and disconnected states show stale marker', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '恢复中' }).click();
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('恢复中');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await page.getByRole('button', { name: '已断开' }).click();
  await expect(page.getByLabel('auction-state').locator('.dock-feedback')).toContainText('重连中');
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

  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('提交中');
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥350.00');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  expect(bidRequest.idempotencyKey).toBe(bidRequest.body.client_bid_id);
  expect(bidRequest.body.amount_cents).toBe(40000);
  expect(bidRequest.body.client_seen_seq).toBe(41);

  releaseBid();
  await expect(page.getByText('出价已确认')).toBeVisible();
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥400.00');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
});

test('H5 local bid lock suppresses rapid duplicate clicks', async ({ page }) => {
  let bidRequests = 0;
  let releaseBid: (value?: unknown) => void = () => undefined;
  await page.route('/api/auctions/auc_live/bids', async (route) => {
    bidRequests += 1;
    await new Promise((release) => {
      releaseBid = release;
    });
    await route.fulfill({
      json: {
        result: 'ENGINE_ACCEPTED',
        bid_id: 'bid_rapid',
        auction_id: 'auc_live',
        seq: 41,
        engine_seq: 42,
        settlement_status: 'PENDING',
        current_price_cents: 40000,
        current_winner_id: 'user_1',
        end_at: '2099-05-22T14:00:00Z',
        server_time_ms: Date.parse('2099-05-22T13:59:00Z'),
        reject_reason: null
      }
    });
  });

  await page.goto('/?stateMatrix=1');
  await selectActiveBidsState(page);
  const bidCTA = page.getByTestId('bid-cta');
  await expect(bidCTA).toBeEnabled();
  const firstRequest = page.waitForRequest('/api/auctions/auc_live/bids');
  await bidCTA.click();
  await firstRequest;
  await bidCTA.click({ force: true });
  await bidCTA.click({ force: true });
  await expect(bidCTA).toBeDisabled();
  await page.waitForTimeout(100);
  expect(bidRequests).toBe(1);

  releaseBid();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  expect(bidRequests).toBe(1);
});

test('H5 network retry reuses original bid idempotency key', async ({ page }) => {
  const requests: Array<{ idempotencyKey: string | null; body: Record<string, unknown> }> = [];
  await page.route('/api/auctions/auc_live/bids', async (route) => {
    const request = route.request();
    requests.push({
      idempotencyKey: request.headers()['idempotency-key'] ?? null,
      body: JSON.parse(request.postData() ?? '{}') as Record<string, unknown>
    });
    if (requests.length === 1) {
      await route.abort('failed');
      return;
    }
    await route.fulfill({
      json: {
        result: 'ENGINE_ACCEPTED',
        bid_id: 'bid_network_retry',
        auction_id: 'auc_live',
        seq: 41,
        engine_seq: 42,
        settlement_status: 'PENDING',
        current_price_cents: 40000,
        current_winner_id: 'user_1',
        end_at: '2099-05-22T14:00:00Z',
        server_time_ms: Date.parse('2099-05-22T13:59:00Z'),
        reject_reason: null
      }
    });
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  const bidCTA = page.getByTestId('bid-cta');
  await bidCTA.click();
  await expect(page.getByText('响应丢失，使用同一请求确认结果')).toBeVisible();
  await expect(bidCTA).toHaveText(/用原请求重试/);

  await bidCTA.click();
  await expect(page.getByText('出价已提交，正在确认')).toBeVisible();
  expect(requests).toHaveLength(2);
  expect(requests[1].idempotencyKey).toBe(requests[0].idempotencyKey);
  expect(requests[1].body.client_bid_id).toBe(requests[0].body.client_bid_id);
  expect(requests[1].body.amount_cents).toBe(requests[0].body.amount_cents);
  expect(requests[1].body.client_seen_seq).toBe(requests[0].body.client_seen_seq);
});

test('H5 engine sold pending waits for settlement before payment copy', async ({ page }) => {
  await page.route('/api/auctions/auc_live/bids', async (route) => {
    await route.fulfill({
      json: {
        result: 'ENGINE_SOLD',
        bid_id: 'bid_engine_sold',
        auction_id: 'auc_live',
        seq: 41,
        engine_seq: 42,
        settlement_status: 'PENDING',
        current_price_cents: 60000,
        current_winner_id: 'user_1',
        end_at: '2099-05-22T14:00:00Z',
        server_time_ms: Date.parse('2099-05-22T13:59:00Z'),
        reject_reason: null
      }
    });
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.getByTestId('bid-cta').click();

  await expect(page.getByLabel('auction-state').getByText('落槌结算中')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveText(/等待订单/);
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await expect(page.getByText('订单 ord_pending 已锁定')).not.toBeVisible();
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
  await expect(page.getByTestId('h5-risk-action')).toContainText('按服务端给出的最低有效价和加价幅度调整');
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥350.00');
  await expect(page.getByTestId('bid-cta')).toBeEnabled();
});

test('H5 rate-limit rejects enter retry-after cooldown before another bid', async ({ page }) => {
  let requests = 0;
  await page.route('/api/auctions/auc_live/bids', async (route) => {
    requests += 1;
    await route.fulfill({
      status: 429,
      headers: { 'Retry-After': '2' },
      json: {
        code: 'BID_AUCTION_TOO_HOT',
        message: 'auction too hot',
        details: {
          retry_after_ms: 2000,
          retry_after_secs: 2
        }
      }
    });
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.getByTestId('bid-cta').click();
  await expect(page.getByText('竞价激烈，请稍候')).toBeVisible();
  await expect(page.getByTestId('h5-risk-action')).toContainText('系统正在削峰，请等待提示恢复后再出价');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await expect(page.getByTestId('bid-cta')).toHaveText(/秒后重试/);

  await page.waitForTimeout(250);
  expect(requests).toBe(1);
  await expect(page.getByTestId('bid-cta')).toBeEnabled({ timeout: 3000 });
});

test('H5 processing retry-later keeps duplicate-click guidance without cooldown', async ({ page }) => {
  let requests = 0;
  await page.route('/api/auctions/auc_live/bids', async (route) => {
    requests += 1;
    await route.fulfill({
      status: 409,
      json: {
        code: 'PROCESSING_RETRY_LATER',
        message: 'same idempotency key is still processing',
        details: {}
      }
    });
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.getByTestId('bid-cta').click();

  await expect(page.getByText('正在确认上一笔出价')).toBeVisible();
  await expect(page.getByTestId('h5-risk-action')).toContainText('上一笔请求仍在处理，不要连续点击');
  await expect(page.getByTestId('bid-cta')).toBeEnabled();

  await page.getByTestId('bid-cta').click();
  expect(requests).toBe(2);
});

test('H5 postgres lane retry-later enters retry-after cooldown', async ({ page }) => {
  let requests = 0;
  await page.route('/api/auctions/auc_live/bids', async (route) => {
    requests += 1;
    await route.fulfill({
      status: 409,
      headers: { 'Retry-After': '1' },
      json: {
        code: 'BID_RETRY_LATER',
        message: 'auction bid lane is busy; retry later',
        details: {
          retry_after_ms: 1000,
          retry_after_secs: 1
        }
      }
    });
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.getByTestId('bid-cta').click();
  await page.getByTestId('bid-cta').click({ force: true });

  await expect(page.getByText('竞价激烈，请稍候')).toBeVisible();
  await expect(page.getByTestId('h5-risk-action')).toContainText('系统正在削峰');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await expect(page.getByTestId('bid-cta')).toHaveText(/秒后重试/);
  expect(requests).toBe(1);
  await expect(page.getByTestId('bid-cta')).toBeEnabled({ timeout: 2500 });
});

test('H5 verified bidder requirement disables bid CTA with clear copy', async ({ page }) => {
  let bidRequests = 0;
  await page.route('/api/rooms/room_main/auctions', async (route) => {
    await route.fulfill({
      json: [{
        id: 'auc_live',
        room_id: 'room_main',
        status: 'ACTIVE',
        current_price_cents: 35000,
        increment_cents: 5000,
        accepted_bid_count: 3,
        seq: 41,
        end_at: '2099-05-22T14:00:00Z',
        bidder_requirement: {
          verification_required: true,
          deposit_required: true,
          verified: false,
          deposit_held: false,
          reason: '高价值拍品需完成买家验证和保证金冻结'
        },
        item: { title: '高价值珠宝', image_url: productImageDataURL }
      }]
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
        bidder_requirement: {
          verification_required: true,
          deposit_required: true,
          verified: false,
          deposit_held: false,
          reason: '高价值拍品需完成买家验证和保证金冻结'
        },
        payload: {
          status: 'ACTIVE',
          current_price_cents: 35000,
          leader_user_masked: '张**',
          current_winner_id: 'user_2',
          end_at: '2099-05-22T14:00:00Z',
          server_time_ms: Date.parse('2099-05-22T13:58:45Z'),
          bidder_requirement: {
            verification_required: true,
            deposit_required: true,
            verified: false,
            deposit_held: false,
            reason: '高价值拍品需完成买家验证和保证金冻结'
          },
          item: { title: '高价值珠宝', image_url: productImageDataURL }
        }
      }
    });
  });
  await page.route('/api/auctions/auc_live/bids', async (route) => {
    bidRequests += 1;
    await route.fulfill({ status: 500, json: { code: 'SHOULD_NOT_BID' } });
  });
  await page.goto('/');
  await openBidPanel(page);
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await expect(page.getByTestId('bid-cta')).toHaveText(/需完成验证/);
  await expect(page.getByTestId('bid-hint')).toContainText('高价值拍品需完成买家验证和保证金冻结');
  await page.getByTestId('bid-cta').click({ force: true });
  expect(bidRequests).toBe(0);
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
  await expect(page.getByText('正在确认最新价格...')).toBeVisible();
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥350.00');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  expect(confirmRequest.idempotencyKey).toBe(firstBidKey);
  expect(confirmRequest.body.confirm_token).toBe('ft_test');
  expect(confirmRequest.body.idempotency_key).toBe(firstBidKey);

  releaseConfirm();
  await expect(page.getByText('出价已确认')).toBeVisible();
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

  await expect(page.getByText('等待支付确认')).toBeVisible();
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
  await page.context().grantPermissions(['clipboard-write']);
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
  await selectActiveBidsState(page);
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_sold',
        seq: 42,
        payload: {
          amount_cents: 60000,
          current_price_cents: 60000,
          current_winner_id: 'user_1',
          user_id: 'user_1',
          order_id: 'ord_pending'
        }
      }
    }));
  });
  const sheet = page.getByTestId('result-sheet');
  await expect(sheet).toBeVisible();
  await expect(sheet.getByRole('heading', { name: '恭喜中拍' })).toBeVisible();
  await expect(sheet.getByText('成交价 ¥600.00')).toBeVisible();
  await expect(sheet.getByText(/订单 JP\d{8}-PENDING 已锁定/)).toBeVisible();
  await expect(sheet.getByText('保证金会随订单状态处理')).toBeVisible();
  await expect(sheet.getByTestId('result-climax-card')).toContainText('落槌高光');
  await expect(sheet.getByTestId('result-climax-card')).toContainText('¥600.00');
  await expect(sheet.getByTestId('result-climax-card')).toContainText('2 人有效出价');
  await expect(sheet.getByTestId('result-climax-card')).toContainText('3 次真实出价');
  await expect(sheet.getByTestId('result-fact-chips')).toContainText('击败 1 位有效出价者');
  await expect(sheet.getByTestId('result-fact-chips')).toContainText('成交回链 seq 42');
  await expect(sheet.getByTestId('result-fact-chips')).toContainText(/订单回链 JP\d{8}-PENDING/);
  await expect(sheet.getByTestId('result-fact-chips')).toContainText('榜单锁定 Top 2');
  await expect(sheet.getByTestId('result-climax-card')).toContainText('先确认成交事实再进入支付');
  await expect(page.getByTestId('rank-strip')).toContainText('已中拍 · 订单待支付');
  await expect(page.getByTestId('rank-strip')).toContainText('确认成交事实再支付');
  await expect(page.getByTestId('bid-hint')).toContainText('成交价 ¥600.00 · 订单以服务端为准');
  await expect(page.getByTestId('rank-strip')).not.toContainText('下一口');
  await expect(page.getByTestId('bid-hint')).not.toContainText('最低有效出价');
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);
  await sheet.getByLabel('copy-result-recap').click();
  await expect(sheet.getByText('已复制')).toBeVisible();
  await expect(sheet.getByTestId('h5-result-recap-card')).toContainText('成交回链 seq 42');
  await expect(sheet.getByTestId('h5-result-recap-card')).toContainText(/订单回链 JP\d{8}-PENDING/);
  const downloadPromise = page.waitForEvent('download');
  await sheet.getByLabel('download-highlight-card').click();
  const highlight = await downloadPromise;
  expect(highlight.suggestedFilename()).toMatch(/highlight\.svg$/);
  await expect(sheet.getByText('已保存')).toBeVisible();
  const videoDownloadPromise = page.waitForEvent('download');
  await sheet.getByLabel('download-highlight-video').click();
  const highlightVideo = await videoDownloadPromise;
  expect(highlightVideo.suggestedFilename()).toMatch(/highlight\.webm$/);
  await expect(sheet.getByText('视频已保存')).toBeVisible();

  await sheet.getByTestId('result-pay-cta').dblclick();
  await expect(sheet.getByRole('heading', { name: '支付已完成' })).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  expect(payCount).toBe(1);
});

test('H5 loser and unsold result sheets explain next action without enabling bid', async ({ page }) => {
  await page.goto('/?stateMatrix=1');

  await selectActiveBidsState(page);
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_sold',
        seq: 42,
        payload: {
          amount_cents: 60000,
          current_price_cents: 60000,
          current_winner_id: 'user_2',
          user_id: 'user_2',
          leader_user_masked: '赵**',
          end_at: '2099-05-22T14:00:00Z',
          server_time_ms: Date.parse('2099-05-22T13:59:00Z')
        }
      }
    }));
  });
  const loserSheet = page.getByTestId('result-sheet').filter({ hasText: '本场已落槌' }).first();
  await expect(loserSheet.getByRole('heading', { name: '本场已落槌' })).toBeVisible();
  await expect(loserSheet.getByText('赵** 以 ¥600.00 中拍')).toBeVisible();
  await expect(loserSheet.getByText('下一件：和田玉福牌吊坠')).toBeVisible();
  await expect(loserSheet.getByTestId('next-auction-handoff').getByText('直播间下一件', { exact: true })).toBeVisible();
  await expect(loserSheet.getByTestId('next-auction-handoff').getByText('即将开拍')).toBeVisible();
  await expect(loserSheet.getByTestId('next-auction-handoff').getByText(/未承诺相似度、库存预留或中标优先权/)).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();

  await page.goto('/?stateMatrix=1');
  await selectActiveBidsState(page);
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_ended',
        seq: 42,
        payload: {
          end_at: '2099-05-22T14:00:00Z',
          server_time_ms: Date.parse('2099-05-22T13:59:00Z')
        }
      }
    }));
  });
  const unsoldSheet = page.getByTestId('result-sheet').filter({ hasText: '本场未成交' }).first();
  await expect(unsoldSheet.getByRole('heading', { name: '本场未成交' })).toBeVisible();
  await expect(unsoldSheet.getByText('不会生成订单')).toBeVisible();
  await expect(unsoldSheet.getByText('和田玉福牌吊坠 即将开始')).toBeVisible();
  await expect(unsoldSheet.getByTestId('next-auction-handoff').getByText('和田玉福牌吊坠')).toBeVisible();
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

  await expect(page.getByTestId('bid-hint')).toHaveText('正在确认最新价格...');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();

  releaseSnapshot();
  await expect(page.getByTestId('auction-price')).toHaveText('¥450.00');
  await expect(page.getByLabel('auction-state').getByText('王** 领先').first()).toBeVisible();
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

  await expect(page.getByTestId('bid-hint')).toHaveText('正在确认最新价格...');
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
  await openBidPanel(page);
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'outbox_gap_notice',
        seq: 45
      }
    }));
  });
  await expect(page.getByTestId('auction-price')).toHaveText('¥450.00');
  await expect(page.getByLabel('auction-state').getByText('王** 领先').first()).toBeVisible();
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
  await openBidPanel(page);
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '保护' }).click();
  const historySheet = page.getByTestId('bottom-sheet');
  await expect(historySheet).toBeVisible();
  await expect(historySheet.getByTestId('buyer-trust-card')).toContainText('本场受反作弊保护');
  await historySheet.getByRole('button', { name: '我的出价' }).click();
  await historySheet.getByRole('button', { name: /刷新/ }).click();
  await expect(historySheet.getByText('出价 ¥450.00')).toBeVisible();
  await expect(historySheet.getByText('已出价成功')).toBeVisible();
  await expect(historySheet.getByText('出价 ¥400.00')).toBeVisible();
  await expect(historySheet.getByText('已记录')).toBeVisible();

  await historySheet.getByRole('tab', { name: '更多' }).click();
  await historySheet.getByRole('button', { name: '我的订单' }).click();
  await expect(historySheet.getByText('订单 ¥600.00')).toBeVisible();
  await expect(historySheet.getByText('已支付')).toBeVisible();
});

test('H5 bottom sheets open close and keep the primary bid CTA singular', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 844 });
  await page.goto('/');
  await openBidPanel(page);

  const dock = page.getByLabel('auction-state');
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);

  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '拍品与规则' }).click();
  const sheet = page.getByTestId('bottom-sheet');
  await expect(sheet).toBeVisible();
  await expect(sheet.getByRole('heading', { name: '商品与规则' })).toBeVisible();
  await sheet.getByRole('tab', { name: '本场' }).click();
  await expect(sheet.getByText('天然翡翠A货平安扣吊坠')).toBeVisible();
  await expect(sheet.getByText('和田玉福牌吊坠')).toBeVisible();
  await expect(sheet.getByText('当前拍品')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);
  await expect(page.getByTestId('bid-cta')).toBeVisible();
  await expect(dock).toBeVisible();

  await sheet.getByRole('tab', { name: '详情' }).click();
  await expect(sheet.getByRole('heading', { name: '商品与规则' })).toBeVisible();
  await expect(sheet.getByText('当前出价节奏')).toBeVisible();
  await expect(sheet.getByText('误触保护')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);

  await sheet.getByRole('tab', { name: '自动加价' }).click();
  await expect(sheet.getByRole('heading', { name: '自动加价' })).toBeVisible();
  await expect(sheet.getByTestId('max-bid-sheet')).toContainText('仅自己可见');
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);

  await sheet.getByRole('tab', { name: '出价榜' }).click();
  await expect(sheet.getByRole('heading', { name: '出价榜' })).toBeVisible();
  await expect(sheet.getByText('张**')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);

  await sheet.getByLabel('关闭面板').click();
  await expect(page.getByTestId('bottom-sheet')).toHaveCount(0);
  await expect(page.getByTestId('bid-cta')).toBeVisible();
});

test('H5 automatic bidding sheet waits for committed API response and disables during recovery', async ({ page }) => {
  let putSeen = false;
  let cancelSeen = false;
  await page.route('/api/auctions/auc_live/max-bid-intent', async (route, request) => {
    if (request.method() === 'GET') {
      await route.fulfill({
        json: {
          id: 'mbi_existing',
          auction_id: 'auc_live',
          user_id: 'user_1',
          max_amount_cents: 55000,
          status: 'ACTIVE',
          source: 'MAX_BID',
          last_applied_seq: 40,
          version: 1
        }
      });
      return;
    }
    if (request.method() === 'PUT') {
      putSeen = true;
      await new Promise((resolve) => setTimeout(resolve, 100));
      await route.fulfill({
        json: {
          result: 'ACTIVE',
          intent: {
            id: 'mbi_existing',
            auction_id: 'auc_live',
            user_id: 'user_1',
            max_amount_cents: 60000,
            status: 'ACTIVE',
            source: 'MAX_BID',
            last_applied_seq: 43,
            version: 2
          }
        }
      });
      return;
    }
    if (request.method() === 'DELETE') {
      cancelSeen = true;
      await route.fulfill({
        json: {
          result: 'CANCELLED',
          intent: {
            id: 'mbi_existing',
            auction_id: 'auc_live',
            user_id: 'user_1',
            max_amount_cents: 60000,
            status: 'CANCELLED',
            source: 'MAX_BID',
            version: 3
          }
        }
      });
      return;
    }
    await route.fallback();
  });

  await page.goto('/');
  await openBidPanel(page);
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '自动加价' }).click();
  const sheet = page.getByTestId('max-bid-sheet');
  await expect(sheet).toContainText('¥550.00');
  await expect(sheet).toContainText('已为你跟过一次价');

  await sheet.getByLabel('increase-max-bid').click();
  await sheet.getByRole('button', { name: '更新自动加价' }).click();
  await expect(sheet).toContainText('正在确认自动加价');
  await expect(sheet.getByRole('button', { name: '提交中' })).toBeDisabled();
  await expect(sheet).not.toContainText('¥600.00 · 仅自己可见');
  await expect(sheet).toContainText('¥600.00 · 仅自己可见');
  expect(putSeen).toBe(true);

  await sheet.getByRole('button', { name: '取消' }).click();
  await expect(sheet).toContainText('自动加价已取消');
  expect(cancelSeen).toBe(true);
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);

  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'outbox_gap_notice',
        seq: 99
      }
    }));
  });
  await expect(sheet.getByRole('button', { name: /设置自动加价|更新自动加价/ })).toBeDisabled();
});

test('H5 automatic bidding sheet surfaces server abuse rejects without optimistic success', async ({ page }) => {
  const errorQueue = [
    { code: 'MAX_BID_TOO_LOW', message: 'too low' },
    { code: 'MAX_BID_INCREMENT_MISMATCH', message: 'off grid' },
    { code: 'MAX_BID_ABOVE_CAP', message: 'above cap' },
    { code: 'PROCESSING_RETRY_LATER', message: 'same idempotency key is still processing' }
  ];
  const seenKeys: string[] = [];
  await page.route('/api/auctions/auc_live/max-bid-intent', async (route, request) => {
    if (request.method() === 'GET') {
      await route.fulfill({ status: 404, json: { code: 'AUCTION_NOT_FOUND', message: 'none' } });
      return;
    }
    if (request.method() === 'PUT') {
      seenKeys.push(request.headers()['idempotency-key'] ?? '');
      await route.fulfill({ status: 400, json: errorQueue.shift() ?? { code: 'MAX_BID_TOO_LOW', message: 'too low' } });
      return;
    }
    await route.fallback();
  });

  await page.goto('/');
  await openBidPanel(page);
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '自动加价' }).click();
  const sheet = page.getByTestId('max-bid-sheet');
  const submit = sheet.getByRole('button', { name: /设置自动加价|更新自动加价/ });

  await submit.click();
  await expect(sheet).toContainText('最高价低于当前最低有效出价');
  await expect(sheet).not.toContainText('已代出价');

  await submit.click();
  await expect(sheet).toContainText('最高价需要按加价幅度设置');
  await expect(sheet).not.toContainText('已代出价');

  await submit.click();
  await expect(sheet).toContainText('最高价超过本场封顶价');
  await expect(sheet).not.toContainText('已代出价');

  await submit.click();
  await expect(sheet).toContainText('上一笔自动加价仍在确认');
  await expect(sheet).not.toContainText('已代出价');
  expect(seenKeys).toHaveLength(4);
  expect(new Set(seenKeys).size).toBe(4);
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
  await openBidPanel(page);
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '保护' }).click();
  const sheet = page.getByTestId('bottom-sheet');
  await expect(sheet.getByTestId('buyer-trust-card')).toContainText('价格、倒计时和有效出价以服务端为准');
  await sheet.getByRole('button', { name: '我的出价' }).click();
  await sheet.getByRole('button', { name: /刷新/ }).click();
  await expect(sheet.getByText('出价 ¥450.00')).toBeVisible();
  await expect(sheet.getByText('已出价成功')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toHaveCount(1);

  await sheet.getByRole('tab', { name: '更多' }).click();
  await sheet.getByRole('button', { name: '我的订单' }).click();
  await expect(sheet.getByText('订单 ¥600.00')).toBeVisible();
  await expect(sheet.getByText('待支付')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeVisible();
});

test('H5 product trust sheet explains proof money and timing in user language', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 844 });
  await page.goto('/');

  await page.getByTestId('floating-product-card').click();
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '拍品与规则' }).click();
  const sheet = page.getByTestId('bottom-sheet');
  await expect(sheet.getByRole('heading', { name: '商品与规则' })).toBeVisible();
  await expect(sheet.getByText('商品信任详情')).toBeVisible();
  await expect(sheet.getByText('天然A货翡翠平安扣，附GID证书，顺丰包邮，支持7天无理由。')).toBeVisible();
  await expect(sheet.getByLabel('product-trust-proof').getByText('GID 20260607 · 可核验')).toBeVisible();
  await expect(sheet.getByLabel('product-trust-proof').getByText('直径 9.2cm')).toBeVisible();
  await expect(sheet.getByText('当前出价节奏')).toBeVisible();
  await expect(sheet.getByText('价格到达 ¥1,500.00 后不再继续抬价。')).toBeVisible();
  await expect(sheet.getByText('本场要求保证金，最低 ¥500.00')).toBeVisible();
  await expect(sheet.getByText('最后 10 秒内有有效出价，会自动延长 10 秒，最多 3 次')).toBeVisible();
  await expect(sheet.getByText('单次高额跳价达到 ¥1,000.00 会触发确认，防止误触。')).toBeVisible();
  await expect(sheet.getByRole('paragraph').filter({ hasText: '签收前可验货，证书不符支持售后复核。' })).toBeVisible();
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
  await expect(page.getByTestId('stage-chat-overlay').getByText('这件翡翠水头不错')).toBeVisible();
  await page.getByLabel('chat-input').fill('我跟一口');
  await page.getByRole('button', { name: 'send-chat' }).click();
  await expect(page.getByTestId('stage-chat-overlay').getByText('我跟一口')).toBeVisible();
  expect(chatBody?.body).toBe('我跟一口');
  expect(String(chatBody?.client_msg_id ?? '')).toBeTruthy();
});

test('H5 server terminal events drive sold, ended, and cancelled states', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await openBidPanel(page);
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('竞价中');
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

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('竞价中');
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_ended',
        seq: 42,
        payload: {}
      }
    }));
  });
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('流拍');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('竞价中');
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_cancelled',
        seq: 42,
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

  await expect(page.getByLabel('auction-state').getByText('竞拍中')).toBeVisible();
  await expectDockConnection(page, '已连接');
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
    await page.getByTestId('floating-product-card').click();

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
  await expect(stage.getByText('GID 20260607 · 可核验')).toBeVisible();
  await expect(stage.getByText('品相完整')).toBeVisible();
  await expect(stage.getByText('顺丰包邮')).toBeVisible();
  await expect(page.getByTestId('stage-chat-overlay').getByText('这件翡翠水头不错')).toBeVisible();

  const stageBox = await stage.boundingBox();
  const chatBox = await page.getByTestId('stage-chat-overlay').boundingBox();
  const cardBox = await page.getByTestId('floating-product-card').boundingBox();
  expect(stageBox).toBeTruthy();
  expect(chatBox).toBeTruthy();
  expect(cardBox).toBeTruthy();
  expect(chatBox!.y + chatBox!.height).toBeLessThanOrEqual(stageBox!.y + stageBox!.height);
  expect(chatBox!.y + chatBox!.height).toBeLessThan(cardBox!.y);
  await expect(page.getByTestId('floating-auction-price')).toHaveText('当前最高价 ¥350.00');
  await expect(page.getByTestId('floating-auction-countdown')).toBeVisible();
  await expect(page.getByTestId('floating-auction-status')).toContainText('竞拍中');
});

test('H5 feed product card opens the full bidding panel without losing auction pressure', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');

  await expect(page.getByLabel('auction-state')).toHaveCount(0);
  await expect(page.getByTestId('floating-product-card')).toBeVisible();
  await expect(page.getByTestId('floating-auction-price')).toHaveText('当前最高价 ¥350.00');
  await expect(page.getByTestId('floating-auction-countdown')).toBeVisible();
  await expect(page.getByTestId('floating-auction-status')).toContainText('竞拍中');

  await page.getByTestId('floating-product-card').click();
  await expect(page.getByLabel('auction-state')).toBeVisible();
  await expect(page.getByTestId('auction-price')).toHaveText('¥350.00');
  await expect(page.getByTestId('auction-countdown')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeEnabled();
  await page.getByRole('button', { name: '关闭竞拍面板' }).click();
  await expect(page.getByLabel('auction-state')).toHaveCount(0);
  await expect(page.getByTestId('floating-product-card')).toBeVisible();
});

test('H5 renders realtime leaderboard and event atmosphere controls', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await expect(page.getByLabel('auction-state').locator('h2')).toHaveText('¥350.00');
  await expect(page.getByTestId('rank-strip')).toContainText('第 2 名');

  await expect(page.getByTestId('leaderboard-panel')).toContainText('行动榜单');
  await expect(page.getByTestId('leaderboard-panel')).toContainText('第 2 名');
  await expect(page.getByTestId('leaderboard-panel')).toContainText('差 ¥50.00');
  await expect(page.getByTestId('rank-strip')).toContainText('第 2 名 · 差 ¥50.00');
  await expect(page.getByTestId('rank-strip')).toContainText('下一口 ¥400.00');
  await expect(page.getByTestId('rank-strip')).toContainText('刚刚更新');
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

test('H5 leaderboard sheet shows action metrics without moving the bid CTA', async ({ page }) => {
  await page.goto('/');
  await openBidPanel(page);
  const ctaBefore = await page.getByTestId('bid-cta').boundingBox();
  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '出价榜' }).click();
  await expect(page.getByTestId('leaderboard-sheet')).toContainText('第 2 名 · 差 ¥50.00');
  await expect(page.getByTestId('leaderboard-sheet')).toContainText('下一口 ¥400.00');
  await expect(page.getByTestId('leaderboard-sheet')).toContainText('近30秒 3 次出价');
  const ctaAfter = await page.getByTestId('bid-cta').boundingBox();
  expect(ctaBefore).not.toBeNull();
  expect(ctaAfter).not.toBeNull();
  expect(Math.abs((ctaAfter?.y ?? 0) - (ctaBefore?.y ?? 0))).toBeLessThan(2);
});

test('H5 official bid hints stay beside amount and CTA', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '领先中' }).click();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await expect(page.getByTestId('bid-hint')).not.toContainText('Kafka');
  await expect(page.getByTestId('bid-hint')).not.toContainText('Redis');

  await page.getByRole('button', { name: '竞价中' }).click();
  await raisePreparedBid(page, '¥450.00');
  await expect(page.getByTestId('bid-hint')).toContainText('高于当前价 ¥100.00');
  await expect(page.getByTestId('bid-hint')).toContainText('高于最低下一口 ¥50.00');
});

test('H5 prepared bid hint updates when authoritative price changes', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await raisePreparedBid(page, '¥500.00');
  await expect(page.getByTestId('bid-hint')).toContainText('高于最低下一口 ¥100.00');

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
          leader_user_masked: '张**'
        }
      }
    }));
  });

  await expect(page.getByTestId('bid-hint')).toHaveText('高于当前价 ¥100.00 · 高于最低下一口 ¥50.00');
});

test('H5 sound and haptic policy requires opt-in before cue playback', async ({ page }) => {
  await page.addInitScript(() => {
    const target = window as typeof window & {
      __audioContextCreated?: number;
      __oscillatorStarted?: number;
      __vibrations?: Array<number | number[]>;
    };
    target.__audioContextCreated = 0;
    target.__oscillatorStarted = 0;
    target.__vibrations = [];
    class MockGain {
      gain = {
        setValueAtTime: () => undefined,
        setTargetAtTime: () => undefined,
        exponentialRampToValueAtTime: () => undefined
      };
      connect() { return undefined; }
    }
    class MockBufferSource {
      buffer: unknown = null;
      loop = false;
      connect() { return undefined; }
      start() { target.__oscillatorStarted = (target.__oscillatorStarted ?? 0) + 1; }
      stop() { return undefined; }
    }
    class MockOscillator {
      type = 'sine';
      frequency = { value: 0, setValueAtTime: () => undefined };
      connect() { return undefined; }
      start() { target.__oscillatorStarted = (target.__oscillatorStarted ?? 0) + 1; }
      stop() { return undefined; }
    }
    class MockAudioContext {
      currentTime = 0;
      destination = {};
      state = 'running';
      constructor() { target.__audioContextCreated = (target.__audioContextCreated ?? 0) + 1; }
      createBufferSource() { return new MockBufferSource(); }
      createOscillator() { return new MockOscillator(); }
      createGain() { return new MockGain(); }
      decodeAudioData() { return Promise.resolve({}); }
      resume() { this.state = 'running'; return Promise.resolve(); }
      close() { return Promise.resolve(); }
    }
    Object.defineProperty(window, 'AudioContext', { value: MockAudioContext, configurable: true });
    const originalFetch = window.fetch.bind(window);
    window.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes('/audio/auction/')) {
        return Promise.resolve(new Response(new Uint8Array([1, 2, 3]), { status: 200 }));
      }
      return originalFetch(input, init);
    }) as typeof window.fetch;
    Object.defineProperty(navigator, 'vibrate', {
      value: (pattern: number | number[]) => {
        target.__vibrations?.push(pattern);
        return true;
      },
      configurable: true
    });
  });

  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();
  await openBidPanel(page);
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
  await expect(page.getByTestId('atmosphere-cue')).toBeVisible();
  await expect.poll(() => page.evaluate(() => (window as typeof window & { __audioContextCreated?: number }).__audioContextCreated ?? 0)).toBe(0);
  await expect.poll(() => page.evaluate(() => (window as typeof window & { __oscillatorStarted?: number }).__oscillatorStarted ?? 0)).toBe(0);

  await page.getByLabel('开启提示音').click();
  await expect(page.getByLabel('关闭提示音')).toBeVisible();
  await expect.poll(() => page.evaluate(() => (window as typeof window & { __audioContextCreated?: number }).__audioContextCreated ?? 0)).toBe(1);

  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'bid_accepted',
        seq: 43,
        payload: {
          current_price_cents: 45000,
          current_winner_id: 'user_1',
          user_id: 'user_1',
          leader_user_masked: '你'
        }
      }
    }));
  });
  await expect.poll(() => page.evaluate(() => (window as typeof window & { __oscillatorStarted?: number }).__oscillatorStarted ?? 0)).toBe(1);
  await expect.poll(() => page.evaluate(() => (window as typeof window & { __vibrations?: unknown[] }).__vibrations?.length ?? 0)).toBe(1);
});

test('H5 sold climax plays hammer asset after sound opt-in', async ({ page }) => {
  await page.addInitScript(() => {
    const target = window as typeof window & {
      __playedBuffers?: string[];
      __decodedSoundURLs?: string[];
    };
    target.__playedBuffers = [];
    target.__decodedSoundURLs = [];
    const soundCodeByFile: Record<string, number> = {
      'heartbeat-bed.wav': 1,
      'whoosh-rank.wav': 2,
      'coin-leading.wav': 3,
      'hammer-hit.wav': 4,
      'system-chime.wav': 5,
      'lucky-open.wav': 6,
      'entry-badge.wav': 7,
      'pk-surge.wav': 8
    };
    const soundKeyByCode: Record<number, string> = {
      1: 'heartbeat_bed',
      2: 'rank_whoosh',
      3: 'coin_leading',
      4: 'hammer_hit',
      5: 'system_chime',
      6: 'lucky_open',
      7: 'entry_badge',
      8: 'pk_surge'
    };
    class MockGain {
      gain = {
        setValueAtTime: () => undefined,
        setTargetAtTime: () => undefined,
        exponentialRampToValueAtTime: () => undefined
      };
      connect() { return undefined; }
    }
    class MockBufferSource {
      buffer: { key?: string } | null = null;
      loop = false;
      connect() { return undefined; }
      start() { target.__playedBuffers?.push(this.buffer?.key ?? 'oscillator-fallback'); }
      stop() { return undefined; }
    }
    class MockOscillator {
      type = 'sine';
      frequency = { value: 0, setValueAtTime: () => undefined };
      connect() { return undefined; }
      start() { target.__playedBuffers?.push('oscillator-fallback'); }
      stop() { return undefined; }
    }
    class MockAudioContext {
      currentTime = 0;
      destination = {};
      state = 'running';
      createBufferSource() { return new MockBufferSource(); }
      createOscillator() { return new MockOscillator(); }
      createGain() { return new MockGain(); }
      decodeAudioData(buffer: ArrayBuffer) {
        const code = new Uint8Array(buffer)[0] ?? 0;
        return Promise.resolve({ key: soundKeyByCode[code] ?? 'unknown' });
      }
      resume() { this.state = 'running'; return Promise.resolve(); }
      close() { return Promise.resolve(); }
    }
    Object.defineProperty(window, 'AudioContext', { value: MockAudioContext, configurable: true });
    const originalFetch = window.fetch.bind(window);
    window.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input instanceof URL ? input.pathname : input.url;
      if (url.includes('/audio/auction/')) {
        target.__decodedSoundURLs?.push(url);
        const file = url.split('/').pop() ?? '';
        return Promise.resolve(new Response(new Uint8Array([soundCodeByFile[file] ?? 0]), { status: 200 }));
      }
      return originalFetch(input, init);
    }) as typeof window.fetch;
  });

  await page.goto('/?stateMatrix=1');
  await page.getByLabel('开启提示音').click();
  await expect(page.getByLabel('关闭提示音')).toBeVisible();
  await expect.poll(() => page.evaluate(() => (
    (window as typeof window & { __decodedSoundURLs?: string[] }).__decodedSoundURLs ?? []
  ).some((url) => url.includes('hammer-hit.wav')))).toBe(true);

  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_sold',
        seq: 42,
        payload: {
          current_price_cents: 60000,
          current_winner_id: 'user_1',
          leader_user_masked: '我',
          order_id: 'ord_pending',
          end_at: '2099-05-22T14:00:00Z',
          server_time_ms: Date.parse('2099-05-22T13:59:52Z')
        }
      }
    }));
  });

  await expect(page.getByTestId('climax-layer')).toBeVisible();
  await expect.poll(() => page.evaluate(() => (
    (window as typeof window & { __playedBuffers?: string[] }).__playedBuffers ?? []
  ).includes('hammer_hit'))).toBe(true);
  await expect(page.evaluate(() => (
    (window as typeof window & { __playedBuffers?: string[] }).__playedBuffers ?? []
  ).includes('oscillator-fallback'))).resolves.toBe(false);
});

test('H5 sound policy degrades for unsupported audio and reduced motion', async ({ page }) => {
  await page.addInitScript(() => {
    const target = window as typeof window & { __vibrations?: Array<number | number[]> };
    target.__vibrations = [];
    Object.defineProperty(window, 'AudioContext', { value: undefined, configurable: true });
    Object.defineProperty(window, 'webkitAudioContext', { value: undefined, configurable: true });
    Object.defineProperty(navigator, 'vibrate', {
      value: (pattern: number | number[]) => {
        target.__vibrations?.push(pattern);
        return true;
      },
      configurable: true
    });
  });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/?stateMatrix=1');
  await page.getByLabel('开启提示音').click();
  await expect(page.getByLabel('提示音不可用')).toBeDisabled();
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
  await expect.poll(() => page.evaluate(() => (window as typeof window & { __vibrations?: unknown[] }).__vibrations?.length ?? 0)).toBe(0);
});

test('H5 event-driven visual effects stay nonblocking and respect reduced motion', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await selectActiveBidsState(page);
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
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'bid_accepted',
        seq: 43,
        payload: {
          current_price_cents: 45000,
          current_winner_id: 'user_1',
          user_id: 'user_1',
          leader_user_masked: '你'
        }
      }
    }));
  });
  await expect(page.getByTestId('auction-price')).toHaveCSS('animation-name', 'none');
  await expect(page.getByTestId('atmosphere-cue')).toHaveCSS('animation-name', 'none');
  await expect(page.locator('.effect-leading-ring')).toHaveCSS('animation-name', 'none');
});

test('H5 accessibility gate exposes live cues labels and practical touch targets', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 844 });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/?stateMatrix=1');
  await page.getByRole('button', { name: '竞价中' }).click();

  const state = page.getByLabel('auction-state');
  await expect(state.getByTestId('auction-price')).toHaveAttribute('aria-live', 'polite');
  await expect(state.locator('.dock-feedback')).toHaveAttribute('aria-live', 'polite');
  await expect(state.locator('.status-chip')).toHaveText('竞拍中');
  await expectDockConnection(page, '已连接');

  await expect(page.getByTestId('rank-strip')).toContainText('差 ¥50.00');

  for (const locator of [
    page.getByTestId('bid-cta'),
    page.getByRole('button', { name: 'increase' }),
    page.getByRole('button', { name: 'decrease' }),
    page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '拍品与规则' })
  ]) {
    const box = await locator.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.width).toBeGreaterThanOrEqual(40);
    expect(box!.height).toBeGreaterThanOrEqual(40);
  }
});

test('H5 bottom sheet is dialog-labelled and keyboard dismissible without moving bid CTA', async ({ page }) => {
  await page.setViewportSize({ width: 360, height: 844 });
  await page.goto('/');
  await openBidPanel(page);
  const ctaBefore = await page.getByTestId('bid-cta').boundingBox();

  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '拍品与规则' }).click();
  const sheet = page.getByRole('dialog', { name: '商品与规则' });
  await expect(sheet).toBeVisible();
  await expect(sheet.getByRole('button', { name: '关闭面板' })).toBeVisible();
  await expect(sheet.getByRole('tab', { name: '详情' })).toHaveAttribute('aria-selected', 'true');
  await expect(page.getByTestId('bid-cta')).toBeVisible();

  const ctaDuring = await page.getByTestId('bid-cta').boundingBox();
  expect(ctaBefore).not.toBeNull();
  expect(ctaDuring).not.toBeNull();
  expect(Math.abs((ctaDuring?.y ?? 0) - (ctaBefore?.y ?? 0))).toBeLessThan(2);

  await page.keyboard.press('Escape');
  await expect(page.getByTestId('bottom-sheet')).toHaveCount(0);
  await expect(page.getByTestId('bid-cta')).toBeVisible();
});

test('H5 extension and sold visual effects use bounded nonblocking motion layers', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await selectActiveBidsState(page);
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
  await expect(page.getByTestId('final-seconds-layer')).toContainText('延时 +20s');
  await expect(page.getByTestId('final-seconds-layer')).toContainText('最后窗口有真实出价');
  await expect(page.getByTestId('auction-countdown')).toHaveAttribute('data-effect', 'extension-stretch');
  await expect(page.getByTestId('auction-countdown')).toHaveCSS('animation-name', 'countdown-stretch');

  await expect(page.getByTestId('live-stage')).toHaveAttribute('data-atmosphere-kind', 'none', { timeout: 3000 });

  await page.goto('/?stateMatrix=1');
  await selectActiveBidsState(page);
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_sold',
        seq: 42,
        payload: {
          current_price_cents: 60000,
          current_winner_id: 'user_2',
          leader_user_masked: '张**',
          end_at: '2099-05-22T14:00:00Z',
          server_time_ms: Date.parse('2099-05-22T13:59:52Z')
        }
      }
    }));
  });

  await expect(page.getByTestId('live-stage')).toHaveAttribute('data-atmosphere-kind', 'sold');
  await expect(page.getByTestId('atmosphere-cue')).toHaveAttribute('data-event-type', 'auction_sold');
  await expect(page.getByTestId('climax-layer')).toBeVisible();
  await expect(page.getByTestId('climax-layer')).toHaveAttribute('data-motion', 'on');
  await expect(page.getByTestId('climax-confetti-canvas')).toBeVisible();
  await expect(page.getByTestId('climax-confetti-canvas')).toHaveAttribute('data-engine', 'canvas-confetti');
  await expect(page.getByTestId('climax-confetti-canvas')).toHaveAttribute('data-worker', 'true');
  await expect(page.getByTestId('climax-stage-card')).toContainText('本场落槌');
  await expect(page.getByTestId('climax-stage-card')).toContainText('¥600.00');
  await expect(page.getByTestId('climax-stage-card')).toContainText('2 人有效出价');
  await expect(page.getByTestId('climax-stage-card')).toContainText('3 次真实出价');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
});

test('H5 sold climax keeps facts but disables motion under reduced-motion', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/?stateMatrix=1');
  await selectActiveBidsState(page);
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_sold',
        seq: 42,
        payload: {
          current_price_cents: 60000,
          current_winner_id: 'user_1',
          leader_user_masked: '我',
          order_id: 'ord_pending',
          end_at: '2099-05-22T14:00:00Z',
          server_time_ms: Date.parse('2099-05-22T13:59:52Z')
        }
      }
    }));
  });

  await expect(page.getByTestId('live-stage')).toHaveAttribute('data-atmosphere-gated', 'true');
  await expect(page.getByTestId('climax-layer')).toBeVisible();
  await expect(page.getByTestId('climax-layer')).toHaveAttribute('data-motion', 'off');
  await expect(page.getByTestId('climax-confetti-canvas')).toHaveCount(0);
  await expect(page.getByTestId('climax-stage-card')).toContainText('落槌高光');
  await expect(page.getByTestId('climax-stage-card')).toContainText('中拍！');
  await expect(page.getByTestId('climax-stage-card')).toContainText('¥600.00');
  await expect(page.getByTestId('climax-stage-card')).toContainText('2 人有效出价');
  await expect(page.getByTestId('result-climax-card')).toContainText('先确认成交事实再进入支付');
});

test('H5 countdown shows stable tenths and authoritative extension explanation', async ({ page }) => {
  const oldEndAt = '2099-05-22T14:00:04.900+08:00';
  const serverTimeMS = Date.parse('2099-05-22T14:00:00+08:00');
  await page.route('/api/auctions/auc_live', async (route) => {
    await route.fulfill({
      json: {
        event_type: 'snapshot',
        auction_id: 'auc_live',
        seq: 41,
        source: 'db',
        stale: false,
        current_price_cents: 35000,
        increment_cents: 5000,
        current_winner_id: 'user_2',
        end_at: oldEndAt,
        server_time_ms: serverTimeMS,
        payload: {
          status: 'ACTIVE',
          current_price_cents: 35000,
          leader_user_masked: '张**',
          current_winner_id: 'user_2',
          end_at: oldEndAt,
          server_time_ms: serverTimeMS
        }
      }
    });
  });

  await page.goto('/');
  await expect(page.getByTestId('final-seconds-layer')).toContainText('最后 5 秒');
  await openBidPanel(page);
  await expect(page.getByTestId('auction-countdown')).toContainText(/剩余 00:0[1-5]\.\d/);
  const before = await page.getByTestId('auction-countdown').boundingBox();

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
          old_end_at: '2099-05-22T14:00:04.900+08:00',
          end_at: '2099-05-22T14:00:14.900+08:00',
          extend_count: 2,
          max_extend_count: 3,
          server_time_ms: Date.parse('2099-05-22T14:00:00+08:00')
        }
      }
    }));
  });

  await expect(page.getByTestId('auction-countdown')).toContainText('延时后 00:15');
  await expect(page.getByTestId('auction-countdown')).toContainText('延时 14:00:04 -> 14:00:14 · 第 2/3 次');
  const after = await page.getByTestId('auction-countdown').boundingBox();
  expect(before).not.toBeNull();
  expect(after).not.toBeNull();
  expect(Math.abs((after?.width ?? 0) - (before?.width ?? 0))).toBeLessThan(80);
});

test('H5 local zero enters syncing without local hammer', async ({ page }) => {
  let servedSnapshots = 0;
  await page.route('/api/auctions/auc_live', async (route) => {
    servedSnapshots += 1;
    const serverTimeMS = Date.now();
    const endAt = new Date(serverTimeMS + (servedSnapshots === 1 ? 100 : -100)).toISOString();
    await route.fulfill({
      json: {
        event_type: 'snapshot',
        auction_id: 'auc_live',
        seq: 41,
        source: 'db',
        stale: false,
        status: 'ACTIVE',
        current_price_cents: 35000,
        increment_cents: 5000,
        current_winner_id: 'user_2',
        end_at: endAt,
        server_time_ms: serverTimeMS,
        payload: {
          status: 'ACTIVE',
          current_price_cents: 35000,
          leader_user_masked: '张**',
          current_winner_id: 'user_2',
          end_at: endAt,
          server_time_ms: serverTimeMS
        }
      }
    });
  });

  await page.goto('/');
  await openBidPanel(page);
  await expect(page.getByTestId('auction-countdown')).toContainText('到点结算中...');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).not.toHaveText(/成交|已成交|流拍/);
});

test('H5 order realtime events update winner payment state', async ({ page }) => {
  await page.goto('/?stateMatrix=1');
  await selectActiveBidsState(page);
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_sold',
        seq: 42,
        payload: {
          current_price_cents: 60000,
          current_winner_id: 'user_1',
          user_id: 'user_1',
          order_id: 'ord_pending'
        }
      }
    }));
  });
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('成交');

  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'order_paid',
        seq: 43,
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

  await page.goto('/?stateMatrix=1');
  await selectActiveBidsState(page);
  await page.evaluate(() => {
    window.dispatchEvent(new CustomEvent('auction:event', {
      detail: {
        auction_id: 'auc_live',
        event_type: 'auction_sold',
        seq: 42,
        payload: {
          current_price_cents: 60000,
          current_winner_id: 'user_1',
          user_id: 'user_1',
          order_id: 'ord_pending'
        }
      }
    }));
  });
  await expect(page.getByLabel('auction-state').locator('.eyebrow')).toHaveText('成交');
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
  await page.goto('/?stateMatrix=1');
  await page.evaluate(() => document.querySelector('.app-shell')?.setAttribute('data-perf-surface', '1'));
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.evaluate(() => {
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
        observer.observe({ type: 'longtask' });
      } catch {
        target.__longTasks = [];
      }
    }
  });
  await page.getByRole('button', { name: 'increase' }).click();
  await page.getByRole('button', { name: 'decrease' }).click();
  await page.waitForTimeout(250);

  const maxLongTask = await page.evaluate(() => Math.max(0, ...((window as typeof window & { __longTasks?: number[] }).__longTasks ?? [])));
  expect(maxLongTask).toBeLessThan(100);
});
