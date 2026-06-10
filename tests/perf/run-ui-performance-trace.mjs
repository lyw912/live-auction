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
const summaryPath = join(rawDir, 'p1-06-ui-performance-trace.json');
const tracePath = join(rawDir, 'p1-06-ui-performance-trace.zip');
const h5URL = 'http://127.0.0.1:5173/?stateMatrix=1';

const thresholds = {
  maxLongTaskMs: 100,
  maxFrameGapMs: 250,
  maxClickToPaintMs: 100,
  minSamples: 8
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
    if (await isReady(url)) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`Timed out waiting for ${url}`);
}

async function startH5Server() {
  if (await isReady('http://127.0.0.1:5173')) {
    return;
  }
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
  if (server && !server.killed) {
    server.kill();
  }
}

function failIfThresholdExceeded(summary) {
  const failures = [];
  if (summary.max_longtask_ms >= thresholds.maxLongTaskMs) {
    failures.push(`max longtask ${summary.max_longtask_ms}ms >= ${thresholds.maxLongTaskMs}ms`);
  }
  if (summary.max_frame_gap_ms >= thresholds.maxFrameGapMs) {
    failures.push(`max frame gap ${summary.max_frame_gap_ms}ms >= ${thresholds.maxFrameGapMs}ms`);
  }
  if (summary.max_click_to_paint_ms >= thresholds.maxClickToPaintMs) {
    failures.push(`max click-to-paint ${summary.max_click_to_paint_ms}ms >= ${thresholds.maxClickToPaintMs}ms`);
  }
  if (summary.click_to_paint_samples < thresholds.minSamples) {
    failures.push(`only ${summary.click_to_paint_samples} click-to-paint samples, want at least ${thresholds.minSamples}`);
  }
  if (failures.length > 0) {
    throw new Error(failures.join('; '));
  }
}

async function main() {
  await fs.mkdir(rawDir, { recursive: true });
  await startH5Server();

  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: 393, height: 851 },
    deviceScaleFactor: 2,
    isMobile: true,
    hasTouch: true,
    userAgent: 'P1-06-ui-performance-trace'
  });
  await context.tracing.start({ screenshots: true, snapshots: true, sources: false });
  const page = await context.newPage();

  await page.addInitScript(() => {
    const target = window;
    target.__p1PerfTrace = {
      longTasks: [],
      frameGaps: [],
      clickToPaint: []
    };

    if ('PerformanceObserver' in window) {
      try {
        const observer = new PerformanceObserver((list) => {
          target.__p1PerfTrace.longTasks.push(
            ...list.getEntries().map((entry) => ({
              duration: entry.duration,
              startTime: entry.startTime
            }))
          );
        });
        observer.observe({ type: 'longtask', buffered: true });
      } catch {
        target.__p1PerfTrace.longTasks = [];
      }
    }

    let previousFrame = performance.now();
    const sampleFrame = (now) => {
      target.__p1PerfTrace.frameGaps.push(now - previousFrame);
      previousFrame = now;
      requestAnimationFrame(sampleFrame);
    };
    requestAnimationFrame(sampleFrame);
  });

  await page.route('**/api/rooms/room_main/auctions', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ items: [] })
    });
  });
  await page.route('**/api/rooms/room_main/chat?limit=30', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ messages: [] })
    });
  });
  await page.route('**/api/rooms/room_main/chat', async (route) => {
    if (route.request().method() === 'POST') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'chat_trace',
          room_id: 'room_main',
          user_id: 'user_1',
          body: 'p1 ui trace',
          created_at: new Date().toISOString()
        })
      });
      return;
    }
    await route.continue();
  });
  await page.route('**/api/users/me/**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ items: [] })
    });
  });

  await page.goto(h5URL, { waitUntil: 'networkidle' });
  await page.getByTestId('bid-cta').waitFor({ state: 'visible' });

  const clickSelector = async (selector) => {
    const latency = await page.evaluate(async (targetSelector) => {
      const element = document.querySelector(targetSelector);
      if (!(element instanceof HTMLElement)) {
        throw new Error(`missing selector ${targetSelector}`);
      }
      const start = performance.now();
      element.click();
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      const duration = performance.now() - start;
      window.__p1PerfTrace.clickToPaint.push(duration);
      return duration;
    }, selector);
    return latency;
  };

  const labels = ['竞价中', '恢复中', '被拒绝', '已延时', '竞价中', '领先中', '成交', '已取消', '竞价中'];
  for (const label of labels) {
    await page.getByRole('button', { name: label, exact: true }).click();
    await page.waitForTimeout(40);
    await clickSelector('button[aria-label="increase"]');
    await clickSelector('button[aria-label="decrease"]');
  }

  await page.getByLabel('chat-input').fill('p1 ui trace');
  await clickSelector('button[aria-label="send-chat"]');
  await page.waitForTimeout(300);

  const metrics = await page.evaluate(() => {
    const trace = window.__p1PerfTrace;
    return {
      longTasks: trace.longTasks,
      frameGaps: trace.frameGaps,
      clickToPaint: trace.clickToPaint
    };
  });

  await context.tracing.stop({ path: tracePath });
  await browser.close();

  const finite = (values) => values.filter((value) => Number.isFinite(value));
  const round = (value) => Math.round(value * 100) / 100;
  const max = (values) => {
    const numbers = finite(values);
    return numbers.length === 0 ? 0 : round(Math.max(...numbers));
  };
  const longTaskDurations = metrics.longTasks.map((entry) => entry.duration);
  const summary = {
    gate: 'P1-06 UI performance trace',
    generated_at: new Date().toISOString(),
    environment: {
      os: process.platform,
      node: process.version,
      browser: 'Playwright Chromium',
      viewport: '393x851',
      target: h5URL
    },
    thresholds,
    trace_path: 'artifacts/perf/raw/p1-06-ui-performance-trace.zip',
    summary_path: 'artifacts/perf/raw/p1-06-ui-performance-trace.json',
    longtask_count: metrics.longTasks.length,
    max_longtask_ms: max(longTaskDurations),
    max_frame_gap_ms: max(metrics.frameGaps),
    click_to_paint_samples: metrics.clickToPaint.length,
    max_click_to_paint_ms: max(metrics.clickToPaint),
    raw: {
      longtasks_ms: finite(longTaskDurations).map(round),
      largest_frame_gaps_ms: finite(metrics.frameGaps).sort((a, b) => b - a).slice(0, 10).map(round),
      click_to_paint_ms: finite(metrics.clickToPaint).map(round)
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
