import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { issueTicket, openAuctionSocket, placeBid } from './lib/live-auction.js';

const healthyWatchers = Number(__ENV.HEALTHY_WATCHERS || 100);
const slowConsumers = Number(__ENV.SLOW_CONSUMERS || 100);
const duration = __ENV.DURATION || '60s';
const sessionSeconds = Number(__ENV.SESSION_SECONDS || 35);
const blockMs = Number(__ENV.BLOCK_MS || 150);
const connectStaggerMs = Number(__ENV.CONNECT_STAGGER_MS || 10);
const triggerRate = Number(__ENV.TRIGGER_RATE || 20);
const triggerVUs = Number(__ENV.TRIGGER_PRE_ALLOCATED_VUS || 40);
const triggerMaxVUs = Number(__ENV.TRIGGER_MAX_VUS || 120);

export const options = {
  scenarios: {
    healthy: {
      executor: 'constant-vus',
      vus: healthyWatchers,
      duration,
      gracefulStop: __ENV.GRACEFUL_STOP || '10s',
      exec: 'healthyWatcher',
    },
    slow: {
      executor: 'constant-vus',
      vus: slowConsumers,
      duration,
      gracefulStop: __ENV.GRACEFUL_STOP || '10s',
      exec: 'slowConsumer',
    },
    triggers: {
      executor: 'constant-arrival-rate',
      rate: triggerRate,
      timeUnit: '1s',
      duration,
      startTime: __ENV.TRIGGER_START_DELAY || '5s',
      preAllocatedVUs: triggerVUs,
      maxVUs: triggerMaxVUs,
      gracefulStop: __ENV.GRACEFUL_STOP || '10s',
      exec: 'triggerBid',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    auction_k6_hvs_healthy_opened_total: ['count>0'],
    auction_k6_hvs_healthy_error_total: ['count==0'],
    auction_k6_hvs_business_response_rate: ['rate>0.95'],
  },
};

const healthyOpened = new Counter('auction_k6_hvs_healthy_opened_total');
const healthyMessages = new Counter('auction_k6_hvs_healthy_messages_total');
const healthyErrors = new Counter('auction_k6_hvs_healthy_error_total');
const healthySessionMs = new Trend('auction_k6_hvs_healthy_session_ms');
const slowOpened = new Counter('auction_k6_hvs_slow_opened_total');
const slowMessages = new Counter('auction_k6_hvs_slow_messages_total');
const slowErrors = new Counter('auction_k6_hvs_slow_error_total');
const bidResponses = new Counter('auction_k6_hvs_bid_responses_total');
const businessResponseRate = new Rate('auction_k6_hvs_business_response_rate');

function stagger(index, total) {
  if (connectStaggerMs > 0) {
    sleep((index % total) * connectStaggerMs / 1000);
  }
}

export function healthyWatcher() {
  const started = Date.now();
  stagger(__VU - 1, Math.max(healthyWatchers, 1));
  const userID = `k6_ws_${((__VU - 1) % 512) + 1}`;
  const ticket = issueTicket(userID);
  const ws = openAuctionSocket(ticket, 0, {
    open() {
      healthyOpened.add(1);
      check(true, { 'healthy watcher opened': (v) => v === true });
    },
    message() {
      healthyMessages.add(1);
    },
    error() {
      healthyErrors.add(1);
    },
    close() {
      healthySessionMs.add(Date.now() - started);
    },
  });
  sleep(sessionSeconds);
  if (ws.readyState === 1) {
    ws.close();
  }
}

export function slowConsumer() {
  stagger(__VU - 1, Math.max(slowConsumers, 1));
  const userID = `k6_ws_${((__VU + healthyWatchers - 1) % 512) + 1}`;
  const ticket = issueTicket(userID);
  const ws = openAuctionSocket(ticket, 0, {
    open() {
      slowOpened.add(1);
      check(true, { 'slow watcher opened': (v) => v === true });
    },
    message() {
      slowMessages.add(1);
      const until = Date.now() + blockMs;
      while (Date.now() < until) {
        // Intentional busy wait to emulate a browser main thread that cannot drain events quickly.
      }
    },
    error() {
      slowErrors.add(1);
    },
  });
  sleep(sessionSeconds);
  if (ws.readyState === 1) {
    ws.close();
  }
}

export function triggerBid() {
  const userID = `k6_bidder_${((__VU - 1) % 512) + 1}_${__ITER % 7}`;
  const amount = 60000 + ((__ITER % 10) * 5000);
  const res = placeBid(amount, userID, 'p3-hvs-trigger');
  const ok = check(res, {
    'hvs bid returned business response': (r) => r.status === 200 || r.status === 429,
  });
  businessResponseRate.add(ok);
  if (ok) {
    bidResponses.add(1);
  }
}

