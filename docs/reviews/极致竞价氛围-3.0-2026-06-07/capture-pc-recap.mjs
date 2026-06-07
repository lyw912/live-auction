import { chromium, expect } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const OUT = path.resolve('docs/reviews/极致竞价氛围-3.0-2026-06-07/evidence');
const PC_URL = process.env.PC_URL || 'http://127.0.0.1:5277/';

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
  item: {
    id: 'item_live',
    title: '天然翡翠A货平安扣吊坠',
    image_url: '/demo/jade-pendant.jpg',
    description: '附GID证书，可核验，支持 7 天无理由。'
  },
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

async function routeJSON(page, url, json) {
  await page.route(url, async (route) => route.fulfill({ json }));
}

async function installRoutes(page) {
  await routeJSON(page, '**/api/auth/me', { user: { ID: 'host_1', Role: 'host' } });
  await routeJSON(page, '**/api/auth/login', { user: { ID: 'host_1', Role: 'host' }, expires_in_ms: 43200000 });
  await routeJSON(page, '**/api/rooms', { items: [{ id: 'room_main', host_id: 'host_1', status: 'OPEN', role: 'host' }] });
  await routeJSON(page, /\/api\/auctions\?room_id=room_main$/, [auctionLive]);
  await routeJSON(page, '**/api/orders', []);
  await routeJSON(page, '**/api/monitor/auctions', { items: [{ auction_id: 'auc_live', room_id: 'room_main', status: 'ACTIVE', current_price_cents: 45000, seq: 42 }] });
  await routeJSON(page, '**/api/monitor/redis-engine', { items: [] });
  await routeJSON(page, /\/api\/monitor\/anomalies(\?.*)?$/, { items: [] });
  await routeJSON(page, '**/api/monitor/outbox', { items: [] });
  await routeJSON(page, '**/api/monitor/outbox/watermarks', { items: [] });
  await routeJSON(page, '**/api/monitor/snapshots', { items: [] });
  await routeJSON(page, '**/api/monitor/signals', { items: [] });
  await routeJSON(page, '**/api/monitor/scheduler', { items: [] });
  await routeJSON(page, '**/api/monitor/rejects', { items: [] });
  await routeJSON(page, '**/api/monitor/recovery', { items: [] });
  await routeJSON(page, '**/api/monitor/auctions/auc_live/flight-recorder?limit=20&timeline_limit=20', { timeline: [] });
  await routeJSON(page, '**/api/host/auctions/auc_live/prompts', {
    auction_id: 'auc_live',
    room_id: 'room_main',
    prompts: [{
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
    }]
  });
  await routeJSON(page, '**/api/host/auctions/auc_live/heat-summary', {
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
  });
  await routeJSON(page, '**/api/host/auctions/auc_live/max-bid-summary', {
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
  });
  await routeJSON(page, '**/api/host/auctions/auc_live/recap', {
    recap: {
      auction_id: 'auc_live',
      room_id: 'room_main',
      item_title: '天然翡翠A货平安扣吊坠',
      status: 'SOLD',
      final_price_cents: 60000,
      winner_masked: '张**',
      accepted_bids: 18,
      accepted_bidders: 4,
      extend_count: 1,
      highlights: ['成交价 ¥600.00', '真实参与出价 4 人', '末段延时 1 次'],
      next_actions: ['提醒赢家完成支付', '准备下一件承接热度'],
      rule_suggestion: {
        start_price_cents: 25000,
        increment_cents: 5000,
        cap_price_cents: 60000,
        basis: '基于本场起拍价、成交价、加价幅度、有效出价人数生成；仅供下一件人工采信',
        source: 'auction_recap:server_facts',
        human_review_required: true
      },
      generated_at: '2026-05-22T14:01:00Z'
    },
    highlight_asset: {
      id: 'asset_1',
      auction_id: 'auc_live',
      room_id: 'room_main',
      job_id: 'aijob_1',
      status: 'READY',
      media_type: 'text/html',
      title: '天然翡翠A货平安扣吊坠 高光复盘',
      asset_url: '/api/host/highlight-assets/asset_1',
      render_profile: 'html',
      duration_ms: 0,
      created_at: '2026-05-22T14:01:00Z',
      updated_at: '2026-05-22T14:01:00Z'
    }
  });
}

await fs.mkdir(OUT, { recursive: true });
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1440, height: 980 }, locale: 'zh-CN' });
const diagnostics = [];
page.on('console', (message) => diagnostics.push(`console:${message.type()}: ${message.text()}`));
page.on('requestfailed', (request) => diagnostics.push(`requestfailed: ${request.method()} ${request.url()} ${request.failure()?.errorText ?? ''}`));
await installRoutes(page);
await page.goto(PC_URL);
try {
  await expect(page.getByTestId('live-assist-rail')).toBeVisible();
} catch (error) {
  await fs.writeFile('/tmp/pc-recap-capture.html', await page.content(), 'utf8');
  await fs.writeFile('/tmp/pc-recap-capture.log', diagnostics.join('\n'), 'utf8');
  await page.screenshot({ path: '/tmp/pc-recap-capture.png', fullPage: true });
  throw error;
}
await page.getByTestId('live-assist-rail').getByRole('button', { name: '生成复盘' }).click();
const recap = page.getByTestId('auction-recap-card');
await expect(recap.getByTestId('recap-rule-suggestion')).toContainText('下一件建议起拍价');
await expect(recap.getByTestId('recap-rule-suggestion')).toContainText('需主播人工采信，不自动改规则');
await recap.screenshot({ path: path.join(OUT, '14-pc-recap-start-price-suggestion.png') });
await browser.close();
