import { chromium } from '@playwright/test';
import { spawn } from 'node:child_process';
import fs from 'node:fs/promises';
import http from 'node:http';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
const node = process.execPath;
const viteCli = join(root, 'node_modules', 'vite', 'bin', 'vite.js');
const rawDir = join(root, 'docs', 'perf', 'raw');
const summaryPath = join(rawDir, 'p10-ui-performance-gate.json');
const tracePath = join(rawDir, 'p10-ui-performance-gate.zip');
const h5URL = 'http://127.0.0.1:5173/?stateMatrix=1&perfSurface=1';

const thresholds = {
  maxLongTaskMs: 100,
  maxFrameGapMs: 250,
  maxClickToPaintMs: 100,
  maxLayoutShift: 0.02,
  maxBidDockYDeltaPx: 2,
  minClickToPaintSamples: 10,
  minLayoutSamples: 20
};

let server;

function requestURL(url, timeoutMs = 1000) {
  return new Promise((resolve, reject) => {
    const req = http.get(url, (res) => {
      res.resume();
      resolve(res.statusCode ?? 0);
    });
    req.on('error', reject);
    req.setTimeout(timeoutMs, () => req.destroy(new Error(`Timed out requesting ${url}`)));
  });
}

async function isReady(url) {
  try {
    const status = await requestURL(url);
    return status >= 200 && status < 500;
  } catch {
    return false;
  }
}

async function waitFor(url, deadlineMs = 20_000) {
  const deadline = Date.now() + deadlineMs;
  while (Date.now() < deadline) {
    if (await isReady(url)) return;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`Timed out waiting for ${url}`);
}

async function startH5Server() {
  if (await isReady('http://127.0.0.1:5173')) return;
  server = spawn(node, [viteCli, 'frontend/mobile-h5', '--host', '127.0.0.1', '--port', '5173'], {
    cwd: root,
    stdio: ['ignore', 'pipe', 'pipe'],
    windowsHide: true
  });
  server.stdout.on('data', (chunk) => process.stdout.write(`[vite:h5] ${chunk}`));
  server.stderr.on('data', (chunk) => process.stderr.write(`[vite:h5] ${chunk}`));
  await waitFor('http://127.0.0.1:5173');
}

function stopH5Server() {
  if (server && !server.killed) server.kill();
}

function finite(values) {
  return values.filter((value) => Number.isFinite(value));
}

function round(value) {
  return Math.round(value * 100) / 100;
}

function max(values) {
  const numbers = finite(values);
  return numbers.length === 0 ? 0 : round(Math.max(...numbers));
}

function failIfThresholdExceeded(summary) {
  const failures = [];
  if (summary.max_longtask_ms >= thresholds.maxLongTaskMs) failures.push(`max longtask ${summary.max_longtask_ms}ms >= ${thresholds.maxLongTaskMs}ms`);
  if (summary.max_frame_gap_ms >= thresholds.maxFrameGapMs) failures.push(`max frame gap ${summary.max_frame_gap_ms}ms >= ${thresholds.maxFrameGapMs}ms`);
  if (summary.max_click_to_paint_ms >= thresholds.maxClickToPaintMs) failures.push(`max click-to-paint ${summary.max_click_to_paint_ms}ms >= ${thresholds.maxClickToPaintMs}ms`);
  if (summary.max_layout_shift > thresholds.maxLayoutShift) failures.push(`max layout shift ${summary.max_layout_shift} > ${thresholds.maxLayoutShift}`);
  if (summary.max_bid_dock_y_delta_px > thresholds.maxBidDockYDeltaPx) failures.push(`bid dock y delta ${summary.max_bid_dock_y_delta_px}px > ${thresholds.maxBidDockYDeltaPx}px`);
  if (summary.click_to_paint_samples < thresholds.minClickToPaintSamples) failures.push(`only ${summary.click_to_paint_samples} click-to-paint samples, want at least ${thresholds.minClickToPaintSamples}`);
  if (summary.bid_dock_layout_samples < thresholds.minLayoutSamples) failures.push(`only ${summary.bid_dock_layout_samples} bid dock samples, want at least ${thresholds.minLayoutSamples}`);
  if (failures.length > 0) throw new Error(failures.join('; '));
}

async function installRouteMocks(page) {
  const productImageDataURL = 'data:image/svg+xml;utf8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%20600%20800%22%3E%3Crect%20width%3D%22600%22%20height%3D%22800%22%20fill%3D%22%23222f2b%22%2F%3E%3Ccircle%20cx%3D%22300%22%20cy%3D%22300%22%20r%3D%22142%22%20fill%3D%22%23e5f3ef%22%2F%3E%3Cellipse%20cx%3D%22300%22%20cy%3D%22320%22%20rx%3D%22164%22%20ry%3D%2286%22%20fill%3D%22%2310b981%22%2F%3E%3Ctext%20x%3D%22300%22%20y%3D%22640%22%20text-anchor%3D%22middle%22%20font-size%3D%2248%22%20font-family%3D%22Arial%22%20fill%3D%22white%22%3ELOT%20A-102%3C%2Ftext%3E%3C%2Fsvg%3E';
  const auction = {
    id: 'auc_live',
    room_id: 'room_main',
    status: 'ACTIVE',
    current_price_cents: 35000,
    increment_cents: 5000,
    cap_price_cents: 150000,
    accepted_bid_count: 3,
    seq: 41,
    end_at: '2099-05-22T14:00:00Z',
    item: {
      title: '青瓷手作茶盏',
      description: 'P10 UI performance gate lot',
      image_url: productImageDataURL,
      certificate: '中检证书',
      condition: '无冲线',
      shipping: '顺丰保价'
    }
  };
  const leaderboard = {
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
  };

  await page.route('**/api/auth/me', async (route) => route.fulfill({ json: { user: { ID: 'user_1', Role: 'user' } } }));
  await page.route('**/api/auth/login', async (route) => route.fulfill({ json: { user: { ID: 'user_1', Role: 'user' }, expires_in_ms: 43200000 } }));
  await page.route('**/api/auth/ws-ticket', async (route) => route.fulfill({ json: { ticket: 'ticket_p10_perf', expires_in_ms: 60000 } }));
  await page.route('**/api/rooms/room_main/auctions', async (route) => route.fulfill({ json: [auction] }));
  await page.route('**/api/auctions/auc_live', async (route) => route.fulfill({
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
        item: auction.item
      }
    }
  }));
  await page.route('**/api/auctions/auc_live/leaderboard?limit=5', async (route) => route.fulfill({ json: leaderboard }));
  await page.route('**/api/users/me/orders', async (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/rooms/room_main/chat?limit=30', async (route) => route.fulfill({ json: { items: [] } }));
}

async function main() {
  await fs.mkdir(rawDir, { recursive: true });
  await startH5Server();

  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: 360, height: 844 },
    deviceScaleFactor: 2,
    isMobile: true,
    hasTouch: true,
    reducedMotion: 'no-preference',
    userAgent: 'P10-ui-performance-gate'
  });
  await context.tracing.start({ screenshots: true, snapshots: true, sources: false });
  const page = await context.newPage();
  await installRouteMocks(page);

  await page.addInitScript(() => {
    class MockAuctionWebSocket extends EventTarget {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSING = 2;
      static CLOSED = 3;
      CONNECTING = 0;
      OPEN = 1;
      CLOSING = 2;
      CLOSED = 3;
      binaryType = 'blob';
      bufferedAmount = 0;
      extensions = '';
      protocol = 'auction.v1';
      readyState = MockAuctionWebSocket.CONNECTING;
      onopen = null;
      onmessage = null;
      onerror = null;
      onclose = null;
      url;

      constructor(url) {
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
    window.WebSocket = MockAuctionWebSocket;

    const target = window;
    target.__p10PerfGate = {
      longTasks: [],
      frameGaps: [],
      clickToPaint: [],
      layoutShifts: [],
      bidDockBoxes: []
    };

    if ('PerformanceObserver' in window) {
      try {
        const longTaskObserver = new PerformanceObserver((list) => {
          target.__p10PerfGate.longTasks.push(...list.getEntries().map((entry) => ({
            duration: entry.duration,
            startTime: entry.startTime
          })));
        });
        longTaskObserver.observe({ type: 'longtask', buffered: true });
      } catch {
        target.__p10PerfGate.longTasks = [];
      }

      try {
        const layoutObserver = new PerformanceObserver((list) => {
          target.__p10PerfGate.layoutShifts.push(...list.getEntries().filter((entry) => !entry.hadRecentInput).map((entry) => ({
            value: entry.value,
            startTime: entry.startTime
          })));
        });
        layoutObserver.observe({ type: 'layout-shift', buffered: true });
      } catch {
        target.__p10PerfGate.layoutShifts = [];
      }
    }

    let previousFrame = performance.now();
    const sampleFrame = (now) => {
      target.__p10PerfGate.frameGaps.push(now - previousFrame);
      previousFrame = now;
      requestAnimationFrame(sampleFrame);
    };
    requestAnimationFrame(sampleFrame);
  });

  await page.goto(h5URL, { waitUntil: 'networkidle' });
  await page.getByTestId('bid-cta').waitFor({ state: 'visible' });
  await page.getByRole('button', { name: '竞价中' }).click();
  await page.waitForTimeout(150);
  await page.evaluate(() => {
    window.__p10PerfGate.longTasks = [];
    window.__p10PerfGate.frameGaps = [];
    window.__p10PerfGate.clickToPaint = [];
    window.__p10PerfGate.layoutShifts = [];
    window.__p10PerfGate.bidDockBoxes = [];
  });

  const sampleBidDock = async () => page.evaluate(() => {
    const cta = document.querySelector('[data-testid="bid-cta"]');
    if (!(cta instanceof HTMLElement)) throw new Error('missing bid cta');
    const box = cta.getBoundingClientRect();
    window.__p10PerfGate.bidDockBoxes.push({
      x: box.x,
      y: box.y,
      width: box.width,
      height: box.height
    });
  });

  const clickSelector = async (selector) => page.evaluate(async (targetSelector) => {
    const element = document.querySelector(targetSelector);
    if (!(element instanceof HTMLElement)) throw new Error(`missing selector ${targetSelector}`);
    const start = performance.now();
    element.click();
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const duration = performance.now() - start;
    window.__p10PerfGate.clickToPaint.push(duration);
    return duration;
  }, selector);

  await sampleBidDock();
  for (let i = 0; i < 24; i += 1) {
    const price = 40000 + i * 5000;
    const isSelf = i % 3 === 0;
    const extended = i % 5 === 0;
    const endAt = extended ? new Date(Date.parse('2099-05-22T14:00:00Z') + (i + 1) * 10_000).toISOString() : '2099-05-22T14:00:00Z';
    await page.evaluate((eventDetail) => {
      window.dispatchEvent(new CustomEvent('auction:event', { detail: eventDetail }));
    }, {
      auction_id: 'auc_live',
      event_type: 'bid_accepted',
      seq: 42 + i,
      payload: {
        current_price_cents: price,
        amount_cents: price,
        current_winner_id: isSelf ? 'user_1' : 'user_2',
        user_id: isSelf ? 'user_1' : 'user_2',
        leader_user_masked: isSelf ? '你' : '张**',
        end_at: endAt,
        server_time_ms: Date.parse('2099-05-22T13:58:45Z') + i * 1000
      }
    });
    await sampleBidDock();
    await clickSelector('button[aria-label="increase"]');
    await clickSelector('button[aria-label="decrease"]');
    await page.waitForTimeout(25);
  }

  await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '榜单' }).click();
  await sampleBidDock();
  await page.keyboard.press('Escape');
  await page.waitForTimeout(250);

  const metrics = await page.evaluate(() => window.__p10PerfGate);
  await context.tracing.stop({ path: tracePath });
  await browser.close();

  const longTaskDurations = metrics.longTasks.map((entry) => entry.duration);
  const layoutShiftValues = metrics.layoutShifts.map((entry) => entry.value);
  const baselineY = metrics.bidDockBoxes[0]?.y ?? 0;
  const yDeltas = metrics.bidDockBoxes.map((box) => Math.abs(box.y - baselineY));
  const summary = {
    gate: 'P10 UI performance gate',
    generated_at: new Date().toISOString(),
    environment: {
      os: process.platform,
      node: process.version,
      browser: 'Playwright Chromium',
      viewport: '360x844',
      target: h5URL
    },
    thresholds,
    trace_path: 'artifacts/perf/raw/p10-ui-performance-gate.zip',
    summary_path: 'artifacts/perf/raw/p10-ui-performance-gate.json',
    route_mock_classification: 'UI_CONTRACT_COVERAGE_ONLY',
    workload: {
      rapid_bid_events: 24,
      click_interactions: metrics.clickToPaint.length,
      bid_dock_layout_samples: metrics.bidDockBoxes.length
    },
    longtask_count: metrics.longTasks.length,
    max_longtask_ms: max(longTaskDurations),
    max_frame_gap_ms: max(metrics.frameGaps),
    click_to_paint_samples: metrics.clickToPaint.length,
    max_click_to_paint_ms: max(metrics.clickToPaint),
    layout_shift_count: metrics.layoutShifts.length,
    max_layout_shift: max(layoutShiftValues),
    bid_dock_layout_samples: metrics.bidDockBoxes.length,
    max_bid_dock_y_delta_px: max(yDeltas),
    raw: {
      longtasks_ms: finite(longTaskDurations).map(round),
      largest_frame_gaps_ms: finite(metrics.frameGaps).sort((a, b) => b - a).slice(0, 10).map(round),
      click_to_paint_ms: finite(metrics.clickToPaint).map(round),
      layout_shift_values: finite(layoutShiftValues).map(round),
      bid_dock_y_deltas_px: finite(yDeltas).map(round)
    },
    verdict: 'PASS'
  };

  try {
    failIfThresholdExceeded(summary);
  } catch (error) {
    summary.verdict = 'FAIL';
    summary.failure = error.message;
    await fs.writeFile(summaryPath, `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
    throw error;
  }

  await fs.writeFile(summaryPath, `${JSON.stringify(summary, null, 2)}\n`, 'utf8');
  console.log(JSON.stringify(summary, null, 2));
}

try {
  await main();
} finally {
  stopH5Server();
}
