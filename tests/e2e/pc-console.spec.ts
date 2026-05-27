import { expect, test } from '@playwright/test';

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
  extend_count: 0,
  end_at: '2026-05-22T14:00:00Z',
  item: { id: 'item_live', title: '青瓷手作茶盏', description: 'demo' },
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
  current_price_cents: 80000,
  current_winner_id: undefined,
  item: { id: 'item_next', title: '紫砂壶', description: 'next' },
  rule: {
    ...auctionLive.rule,
    frozen_at: undefined
  }
};

const auctionScheduled = {
  ...auctionLive,
  id: 'auc_scheduled',
  item_id: 'item_scheduled',
  status: 'SCHEDULED',
  is_narrating: false,
  current_price_cents: 20000,
  current_winner_id: undefined,
  accepted_bid_count: 0,
  start_at: '2026-05-22T14:30:00Z',
  item: { id: 'item_scheduled', title: '银壶', description: 'scheduled' }
};

test.beforeEach(async ({ page }) => {
  await page.route('/api/auth/me', async (route) => {
    await route.fulfill({ json: { user: { ID: 'host_1', Role: 'host' } } });
  });
  await page.route('/api/auth/login', async (route) => {
    await route.fulfill({ json: { user: { ID: 'host_1', Role: 'host' }, expires_in_ms: 43200000 } });
  });
  await page.route('/api/rooms', async (route) => {
    await route.fulfill({ json: { items: [{ id: 'room_main', host_id: 'host_1', status: 'OPEN', role: 'host' }] } });
  });
  await page.route('/api/auctions?room_id=room_main', async (route) => route.fulfill({ json: [auctionDraft, auctionScheduled, auctionLive] }));
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
    json: {
      items: [
        { time: '2026-05-22T13:00:01Z', auction_id: 'auc_live', user_id: 'user_1', amount_cents: 1, current_price_cents: 45000, reject_reason: 'BID_TOO_LOW', trace_id: 'tr_reject' },
        { time: '2026-05-22T13:00:02Z', auction_id: 'auc_live', user_id: 'user_4', amount_cents: 50000, current_price_cents: 45000, reject_reason: 'BID_AUCTION_TOO_HOT', trace_id: 'tr_hot' }
      ]
    }
  }));
  await page.route('/api/monitor/recovery', async (route) => route.fulfill({
    json: { items: [{ room_id: 'room_main', reconnect_count_recent: 3, history_recovered: 2, snapshot_recovered: 1, snapshot_from_db: 1, snapshot_stale: 0, slow_consumer_disconnects: 0 }] }
  }));
  await page.route('/api/monitor/auctions/auc_live/flight-recorder?limit=20&timeline_limit=20', async (route) => route.fulfill({
    json: {
      timeline: [
        { kind: 'auction_event', event_type: 'bid_accepted', seq: 42, trace_id: 'tr_bid_42', payload: { bid_source: 'AUTO_MAX_BID' } },
        { kind: 'outbox', status: 'PUBLISHED', outbox_id: 7, seq: 42 }
      ]
    }
  }));
  await page.route('/api/monitor/auctions/auc_live/flight-recorder?limit=80&timeline_limit=120', async (route) => route.fulfill({
    json: {
      summary: {
        auction_id: 'auc_live',
        room_id: 'room_main',
        item_id: 'item_live',
        item_title: '青瓷手作茶盏',
        status: 'ACTIVE',
        current_price_cents: 45000,
        current_winner_id: 'user_2',
        seq: 42,
        accepted_bid_count: 18,
        extend_count: 0
      },
      rules: [{ version: 1 }],
      orders: [{ id: 'ord_pending', status: 'ORDER_PENDING' }],
      payment_events: [{ id: 3, event_type: 'PAYMENT_AUTHORIZED' }],
      anomalies: [{ id: 1, type: 'CLOCK_STEP_BACKWARD' }],
      timeline: [
        { time: '2026-05-22T13:00:00Z', kind: 'auction_event', auction_id: 'auc_live', seq: 41, event_type: 'bid_accepted', ref_id: 'ev_41', user_id: 'user_2', amount_cents: 45000, trace_id: 'tr_bid_41', payload: { amount_cents: 45000 } },
        { time: '2026-05-22T13:00:01Z', kind: 'bid', auction_id: 'auc_live', seq: 41, event_type: 'bid_rejected_row', ref_id: 'bid_reject', user_id: 'user_1', amount_cents: 1, status: 'REJECTED', trace_id: 'tr_reject', payload: { reject_reason: 'BID_TOO_LOW', source: 'MANUAL' } },
        { time: '2026-05-22T13:00:01Z', kind: 'bid', auction_id: 'auc_live', seq: 42, event_type: 'bid_accepted_row', ref_id: 'bid_auto', user_id: 'user_3', amount_cents: 50000, status: 'ACCEPTED', trace_id: 'tr_auto', payload: { source: 'AUTO_MAX_BID', client_bid_id: 'auto-mbi-1' } },
        { time: '2026-05-22T13:00:02Z', kind: 'outbox', auction_id: 'auc_live', seq: 42, event_type: 'bid_accepted:PUBLISHED', ref_id: '7', status: 'PUBLISHED', payload: { delivery_state: 'ACKED' } },
        { time: '2026-05-22T13:00:03Z', kind: 'snapshot_rebuild', auction_id: 'auc_live', event_type: 'COMPLETED', ref_id: 'snap_1', status: 'COMPLETED', payload: { source: 'db', stale: false } },
        { time: '2026-05-22T13:00:04Z', kind: 'order', auction_id: 'auc_live', event_type: 'ORDER_PENDING', ref_id: 'ord_pending', user_id: 'user_1', amount_cents: 60000, status: 'ORDER_PENDING', payload: { deposit_status: 'HELD' } },
        { time: '2026-05-22T13:00:05Z', kind: 'payment_event', auction_id: 'auc_live', event_type: 'PAYMENT_AUTHORIZED', ref_id: '3', user_id: 'user_1', amount_cents: 60000, status: 'ORDER_PENDING', trace_id: 'tr_payment', payload: { order_id: 'ord_pending' } },
        { time: '2026-05-22T13:00:06Z', kind: 'anomaly', auction_id: 'auc_live', event_type: 'CLOCK_STEP_BACKWARD', ref_id: '1', status: 'HIGH', trace_id: 'tr_clock', payload: { message: 'clock moved backward' } }
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
  await page.route('/api/host/auctions/auc_live/max-bid-summary', async (route) => route.fulfill({
    json: {
      auction_id: 'auc_live',
      room_id: 'room_main',
      status: 'ACTIVE',
      generated_at: '2026-05-22T13:59:50Z',
      active_intent_count: 3,
      pre_bid_count: 1,
      max_bid_count: 2,
      applied_intent_count: 1,
      exhausted_count: 1,
      cancelled_count: 0,
      has_private_pressure: true,
      source: 'postgres:max_bid_intents'
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
  await page.route('/api/host/auctions/auc_next/max-bid-summary', async (route) => route.fulfill({
    json: {
      auction_id: 'auc_next',
      room_id: 'room_main',
      status: 'DRAFT',
      generated_at: '2026-05-22T13:59:50Z',
      active_intent_count: 0,
      pre_bid_count: 0,
      max_bid_count: 0,
      applied_intent_count: 0,
      exhausted_count: 0,
      cancelled_count: 0,
      has_private_pressure: false,
      source: 'postgres:max_bid_intents'
    }
  }));
  await page.route('/api/monitor/auctions/auc_scheduled/flight-recorder?limit=20&timeline_limit=20', async (route) => route.fulfill({
    json: { timeline: [] }
  }));
  await page.route('/api/host/auctions/auc_scheduled/prompts', async (route) => route.fulfill({
    json: { auction_id: 'auc_scheduled', room_id: 'room_main', prompts: [] }
  }));
  await page.route('/api/host/auctions/auc_scheduled/heat-summary', async (route) => route.fulfill({
    json: {
      auction_id: 'auc_scheduled',
      room_id: 'room_main',
      status: 'SCHEDULED',
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
  await page.route('/api/host/auctions/auc_scheduled/max-bid-summary', async (route) => route.fulfill({
    json: {
      auction_id: 'auc_scheduled',
      room_id: 'room_main',
      status: 'SCHEDULED',
      generated_at: '2026-05-22T13:59:50Z',
      active_intent_count: 0,
      pre_bid_count: 0,
      max_bid_count: 0,
      applied_intent_count: 0,
      exhausted_count: 0,
      cancelled_count: 0,
      has_private_pressure: false,
      source: 'postgres:max_bid_intents'
    }
  }));
});

test('PC console renders live API auctions, orders, and expanded diagnostic panels', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('auction-queue').getByText('青瓷手作茶盏')).toBeVisible();
  await expect(page.getByTestId('auction-queue').getByText('紫砂壶')).toBeVisible();
  await expect(page.getByTestId('health-ribbon')).toBeVisible();
  await expect(page.getByTestId('pc-auction-page')).toBeVisible();
  await expect(page.getByTestId('pc-command-center')).toBeVisible();
  await expect(page.getByTestId('auction-queue')).toBeVisible();
  await expect(page.getByTestId('queue-group-active-pinned').getByText('青瓷手作茶盏')).toBeVisible();
  await expect(page.getByTestId('queue-group-scheduled').getByText('银壶')).toBeVisible();
  await expect(page.getByTestId('queue-group-draft').getByText('紫砂壶')).toBeVisible();
  await expect(page.getByTestId('live-assist-rail')).toBeVisible();
  await expect(page.getByText('ord_pending')).toBeVisible();
  await page.getByRole('button', { name: '拍品', exact: true }).click();
  await expect(page.getByTestId('pc-inventory-page')).toBeVisible();
  await expect(page.getByTestId('pc-command-center')).not.toBeVisible();
  await page.getByRole('button', { name: '诊断', exact: true }).click();
  await expect(page.getByTestId('pc-diagnostics-page')).toBeVisible();
  await expect(page.getByTestId('auction-queue')).not.toBeVisible();
  await expect(page.getByTestId('diagnostics')).toBeVisible();
  await expect(page.getByLabel('monitor-anomaly-type')).toBeVisible();
  await page.getByRole('button', { name: '竞拍', exact: true }).click();
  await expect(page.getByTestId('auction-control-summary')).toBeVisible();
  await expect(page.getByTestId('auction-control-summary').getByText('当前价')).toBeVisible();
  await expect(page.getByTestId('auction-control-summary').getByText(/服务端倒计时/)).toBeVisible();
  await expect(page.getByTestId('auction-control-summary').getByText('延时次数')).toBeVisible();
  await expect(page.getByTestId('auction-control-summary').getByText(/近期重连 3/)).toBeVisible();
  await expect(page.getByTestId('auction-control-summary').getByRole('button', { name: '取消' })).toBeVisible();
  await expect(page.getByTestId('auction-control-summary').getByRole('button', { name: '停止讲解' })).toBeVisible();
  await expect(page.getByTestId('prompter-cards').getByText('最后窗口')).toBeVisible();
  await expect(page.getByTestId('prompter-cards').getByText(/参考下一口 ¥500.00/)).toBeVisible();
  await expect(page.getByTestId('talk-points').getByText('封顶/保证金')).toBeVisible();
  await expect(page.getByTestId('heat-summary').getByText('Active bidders')).toBeVisible();
  await expect(page.getByTestId('heat-summary').getByText('Accepted bids')).toBeVisible();
  await expect(page.getByTestId('heat-summary').getByText('Rejected bids')).toBeVisible();
  await expect(page.getByTestId('heat-summary').getByText('Watchers')).toBeVisible();
  await expect(page.getByTestId('heat-summary').getByText('unavailable')).toBeVisible();
  await expect(page.getByTestId('max-bid-summary').getByText('Max Bid readiness')).toBeVisible();
  await expect(page.getByTestId('max-bid-summary').getByText('Active intents')).toBeVisible();
  await expect(page.getByTestId('max-bid-summary').getByText('Auto applied')).toBeVisible();
  await expect(page.getByTestId('max-bid-summary')).not.toContainText('max_amount_cents');
  await expect(page.getByTestId('max-bid-summary')).not.toContainText('95000');
  await expect(page.getByTestId('risk-queue').getByText('Bid pressure throttle')).toBeVisible();
  await expect(page.getByTestId('risk-queue').getByText(/2 recent reject rows/)).toBeVisible();
  await expect(page.getByTestId('risk-queue').getByText('CLOCK_STEP_BACKWARD')).toBeVisible();
  await expect(page.getByTestId('risk-queue').getByText('user_activity_events')).toBeVisible();
  await expect(page.getByTestId('system-chat-disabled').getByRole('button', { name: '发送模板' })).toBeDisabled();
  await expect(page.getByTestId('recent-events').getByText('bid_accepted')).toBeVisible();
  await expect(page.getByRole('button', { name: /Flight recorder/ })).toBeVisible();

  await page.getByRole('button', { name: '诊断', exact: true }).click();
  await page.getByRole('tab', { name: 'Rejects' }).click();
  await expect(page.getByText('BID_TOO_LOW')).toBeVisible();
  await expect(page.getByText('tr_reject')).toBeVisible();
  await expect(page.getByLabel('Rejects').getByRole('button', { name: /tr_reject/ })).toBeVisible();

  await page.getByRole('tab', { name: 'Recovery' }).click();
  await expect(page.getByLabel('Recovery').getByText('room_main')).toBeVisible();
  await expect(page.getByText('snapshot_from_db')).toBeVisible();
});

test('PC accessibility gate exposes live diagnostic state and named controls', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('health-ribbon-status')).toHaveAttribute('role', 'status');
  await expect(page.getByTestId('health-ribbon-status')).toHaveAttribute('aria-live', 'polite');
  await expect(page.getByTestId('health-ribbon-status')).toContainText('Outbox');
  await expect(page.getByLabel('room-selector')).toBeVisible();
  await expect(page.getByRole('button', { name: '刷新' })).toBeVisible();

  const riskQueue = page.getByTestId('risk-queue');
  await expect(riskQueue).toHaveAttribute('role', 'status');
  await expect(riskQueue).toHaveAttribute('aria-live', 'polite');
  await expect(riskQueue.getByText('Bid pressure throttle')).toBeVisible();
  await expect(riskQueue.locator('em').getByText('bids', { exact: true })).toBeVisible();

  await page.getByRole('button', { name: '诊断', exact: true }).click();
  await page.getByRole('tab', { name: 'Rejects' }).click();
  await expect(page.getByLabel('Rejects').getByRole('button', { name: /tr_reject/ })).toBeVisible();
  await expect(page.getByLabel('monitor-auction-id')).toBeVisible();
  await expect(page.getByLabel('monitor-user-id')).toBeVisible();
  await expect(page.getByLabel('monitor-trace-id')).toBeVisible();
});

test('PC host live assist renders API prompts and dismisses locally without mutating auction state', async ({ page }) => {
  const mutationRequests: string[] = [];
  page.on('request', (request) => {
    if (request.method() !== 'GET' && request.url().includes('/api/host/auctions/auc_live/prompts')) {
      mutationRequests.push(`${request.method()} ${request.url()}`);
    }
  });
  await page.goto('/');
  await expect(page.getByTestId('prompter-cards').getByText('最后窗口')).toBeVisible();
  await page.getByTestId('prompter-cards').getByRole('button', { name: '本场隐藏' }).click();
  await expect(page.getByTestId('prompter-cards').getByText('最后窗口')).not.toBeVisible();
  await expect(page.getByTestId('prompter-cards').getByText('暂无主播提示')).toBeVisible();
  expect(mutationRequests).toEqual([]);
});

test('PC host live assist renders real heat summary and labels watcher count unavailable', async ({ page }) => {
  await page.goto('/');
  const heat = page.getByTestId('heat-summary');
  await expect(heat.getByText('Heat 30s')).toBeVisible();
  await expect(heat.getByText('postgres:bids,chat_messages,user_activity_events')).toBeVisible();
  await expect(heat.getByText('Active bidders')).toBeVisible();
  await expect(heat.getByText('Accepted bids')).toBeVisible();
  await expect(heat.getByText('Rejected bids')).toBeVisible();
  await expect(heat.getByText('Chat', { exact: true })).toBeVisible();
  await expect(heat.getByText('Recovery')).toBeVisible();
  await expect(heat.getByText('Watchers')).toBeVisible();
  await expect(heat.getByText('unavailable')).toBeVisible();
  await expect(heat.locator('.heat-grid div').filter({ hasText: 'Active bidders' }).getByText('2')).toBeVisible();
  await expect(heat.locator('.heat-grid div').filter({ hasText: 'Accepted bids' }).getByText('3')).toBeVisible();
  await expect(heat.locator('.heat-grid div').filter({ hasText: 'Rejected bids' }).getByText('1')).toBeVisible();
  await expect(heat.locator('.heat-grid div').filter({ hasText: 'Chat' }).getByText('4')).toBeVisible();
});

test('PC host live assist renders private Max Bid readiness without exposing ceilings', async ({ page }) => {
  await page.goto('/');
  const summary = page.getByTestId('max-bid-summary');
  await expect(summary.getByText('Max Bid readiness')).toBeVisible();
  await expect(summary.getByText('postgres:max_bid_intents')).toBeVisible();
  await expect(summary.locator('.heat-grid div').filter({ hasText: 'Active intents' }).getByText('3')).toBeVisible();
  await expect(summary.locator('.heat-grid div').filter({ hasText: 'Pre-bids' }).getByText('1')).toBeVisible();
  await expect(summary.locator('.heat-grid div').filter({ hasText: 'Max bids' }).getByText('2')).toBeVisible();
  await expect(summary.locator('.heat-grid div').filter({ hasText: 'Auto applied' }).getByText('1')).toBeVisible();
  await expect(summary.getByText(/主播只看聚合计数/)).toBeVisible();
  await expect(summary).not.toContainText('max_amount_cents');
  await expect(summary).not.toContainText('90000');
  await summary.getByRole('button', { name: /审计自动出价/ }).click();
  const drawer = page.getByTestId('flight-recorder-drawer');
  await expect(drawer.getByText('AUTO_MAX_BID')).toBeVisible();
  await expect(drawer.getByText(/Automatic Max Bid settlement wrote a real bid row/)).toBeVisible();
  await expect(drawer).not.toContainText('max_amount_cents');
});

test('PC diagnostics row opens flight recorder drawer with real timeline impact and next action', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '诊断', exact: true }).click();
  await page.getByRole('tab', { name: 'Rejects' }).click();
  await page.getByLabel('Rejects').getByRole('button', { name: /tr_reject/ }).click();
  const drawer = page.getByTestId('flight-recorder-drawer');
  await expect(drawer.getByText('auc_live')).toBeVisible();
  await expect(drawer.getByText('青瓷手作茶盏')).toBeVisible();
  await expect(drawer.getByText('bid_accepted', { exact: true })).toBeVisible();
  await expect(drawer.getByText('bid_rejected_row')).toBeVisible();
  await expect(drawer.getByText('bid_accepted:PUBLISHED')).toBeVisible();
  await expect(drawer.getByText('PAYMENT_AUTHORIZED')).toBeVisible();
  await expect(drawer.getByText('CLOCK_STEP_BACKWARD')).toBeVisible();
  await expect(drawer.getByText(/Authoritative bid advanced auction seq/)).toBeVisible();
  await expect(drawer.getByText(/Use reject_reason to explain user-facing copy/)).toBeVisible();
  await expect(drawer.getByText(/Operational anomaly requires host\/ops review/)).toBeVisible();
});

test('PC auction queue pins active auction and explains active and narrating constraints', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('queue-group-active-pinned').getByText('Pinned current live auction')).toBeVisible();
  await page.getByTestId('auction-queue').getByText('银壶').click();
  await expect(page.getByTestId('auction-control-summary').getByRole('button', { name: '开拍' })).toBeDisabled();
  await expect(page.getByText(/房间已有 ACTIVE auc_live/)).toBeVisible();
  await expect(page.getByTestId('queue-group-scheduled').getByText(/ACTIVE locked by auc_live/)).toBeVisible();
  await page.getByTestId('auction-queue').getByText('紫砂壶').click();
  await expect(page.getByTestId('auction-control-summary').getByRole('button', { name: '开始讲解' })).toBeDisabled();
  await expect(page.getByText(/讲解中拍品为 auc_live/)).toBeVisible();
  await expect(page.getByTestId('queue-group-draft').getByText(/Narrating locked by auc_live/)).toBeVisible();
});

test('PC creates item and auction through backend upload and create APIs', async ({ page }) => {
  let uploadURLBody: Record<string, unknown> | undefined;
  let uploadedBytes = 0;
  let itemBody: Record<string, unknown> | undefined;
  let auctionBody: Record<string, unknown> | undefined;
  await page.route('/api/items/upload-url', async (route, request) => {
    uploadURLBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({
      json: {
        upload_url: 'http://upload.local/item-created.jpg',
        public_url: 'http://cdn.local/item-created.jpg'
      }
    });
  });
  await page.route('http://upload.local/item-created.jpg', async (route, request) => {
    uploadedBytes = (request.postDataBuffer() ?? Buffer.from([])).byteLength;
    await route.fulfill({ status: 200, body: '' });
  });
  await page.route('/api/items', async (route, request) => {
    itemBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({ status: 201, json: { id: 'item_created', title: itemBody.title, description: itemBody.description, image_url: itemBody.image_url, status: 'READY' } });
  });
  await page.route('/api/auctions', async (route, request) => {
    auctionBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({ status: 201, json: { ...auctionDraft, id: 'auc_created', item: { id: 'item_created', title: itemBody?.title } } });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '拍品', exact: true }).click();
  await expect(page.getByTestId('wizard-product-step')).toBeVisible();
  await page.getByLabel('item-title').fill('白瓷杯');
  await page.getByLabel('item-image-file').setInputFiles({
    name: 'cup.jpg',
    mimeType: 'image/jpeg',
    buffer: Buffer.from('fake image bytes')
  });
  await page.getByRole('button', { name: '创建拍品和竞拍' }).click();
  await expect.poll(() => uploadURLBody?.object_name).toContain('cup.jpg');
  await expect.poll(() => uploadedBytes).toBeGreaterThan(0);
  await expect.poll(() => itemBody?.title).toBe('白瓷杯');
  expect(itemBody?.image_url).toBe('http://cdn.local/item-created.jpg');
  await expect.poll(() => auctionBody?.room_id).toBe('room_main');
  expect(auctionBody?.room_id).toBe('room_main');
  expect(auctionBody?.item_id).toBe('item_created');
  expect((auctionBody?.rule as Record<string, unknown>).duration_seconds).toBe(600);
});

test('PC rule save targets selected draft auction and includes all money/rule fields', async ({ page }) => {
  let saveBody: Record<string, unknown> | undefined;
  await page.route('/api/auctions/auc_next/rules', async (route, request) => {
    saveBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({ json: { ...auctionDraft, start_price_cents: saveBody.start_price_cents, increment_cents: saveBody.increment_cents, cap_price_cents: saveBody.cap_price_cents } });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '拍品', exact: true }).click();
  await page.getByText('紫砂壶').click();
  await expect(page.getByTestId('seller-rule-wizard')).toBeVisible();
  await expect(page.getByLabel('seller-rule-wizard-steps').getByText('Product')).toBeVisible();
  await expect(page.getByLabel('seller-rule-wizard-steps').getByText('Price')).toBeVisible();
  await expect(page.getByLabel('seller-rule-wizard-steps').getByText('Time')).toBeVisible();
  await expect(page.getByLabel('seller-rule-wizard-steps').getByText('Trust')).toBeVisible();
  await expect(page.getByLabel('seller-rule-wizard-steps').getByText('Preview')).toBeVisible();
  await expect(page.getByTestId('verified-bidder-placeholder').getByText('Verified bidder gate')).toBeVisible();
  await expect(page.getByTestId('verified-bidder-placeholder').getByRole('button', { name: '启用验证门槛' })).toBeDisabled();
  await page.getByLabel('start-price-cents').fill('20000');
  await page.getByLabel('increment-cents').fill('10000');
  await page.getByLabel('cap-price-cents').fill('70000');
  await expect(page.getByTestId('h5-rule-preview').getByText('下一口 ¥300.00')).toBeVisible();
  await expect(page.getByTestId('h5-rule-preview').getByText('封顶 ¥700.00')).toBeVisible();
  await expect(page.getByTestId('h5-rule-preview').getByText(/保证金 10%/)).toBeVisible();
  await page.getByRole('button', { name: '保存规则' }).click();

  await expect(page.getByText('规则已保存')).toBeVisible();
  expect(saveBody?.start_price_cents).toBe(20000);
  expect(saveBody?.increment_cents).toBe(10000);
  expect(saveBody?.cap_price_cents).toBe(70000);
  expect(saveBody?.duration_seconds).toBe(600);
  expect(saveBody?.deposit_cap_cents).toBe(50000);
});

test('PC rule wizard explains frozen rules for non-draft auctions', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '拍品', exact: true }).click();
  await expect(page.getByTestId('rule-freeze-reason')).toContainText('ACTIVE');
  await expect(page.getByTestId('rule-freeze-reason')).toContainText('仅 DRAFT');
  await expect(page.getByRole('button', { name: '保存规则' })).toBeDisabled();
  await expect(page.getByTestId('h5-rule-preview').getByText(/延时 10s \+10s/)).toBeVisible();
});

test('PC lifecycle controls call selected auction APIs', async ({ page }) => {
  let scheduleBody: Record<string, unknown> | undefined;
  let cancelBody: Record<string, unknown> | undefined;
  await page.route('/api/auctions/auc_next/schedule', async (route, request) => {
    scheduleBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({ json: { ...auctionDraft, status: 'SCHEDULED' } });
  });
  await page.route('/api/auctions/auc_next/cancel', async (route, request) => {
    cancelBody = request.postDataJSON() as Record<string, unknown>;
    await route.fulfill({ json: { ...auctionDraft, status: 'CANCELLED' } });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '竞拍', exact: true }).click();
  await page.getByText('紫砂壶').click();
  await page.getByLabel('schedule-start-at').fill('2026-05-22T14:30');
  await page.getByRole('button', { name: '排期' }).click();
  await expect.poll(() => scheduleBody?.start_at).toBe('2026-05-22T06:30:00.000Z');
  await page.getByLabel('cancel-reason').fill('主播临时下架');
  await page.getByRole('button', { name: '取消' }).click();
  await page.getByRole('dialog', { name: '确认取消竞拍' }).getByRole('button', { name: '确定' }).click();
  await expect.poll(() => cancelBody?.reason).toBe('主播临时下架');
});
