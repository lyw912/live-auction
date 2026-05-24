import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { issueTicket, openAuctionSocket, placeBid } from './lib/live-auction.js';

const consumers = Number(__ENV.CONSUMERS || 80);
const duration = __ENV.DURATION || '60s';
const sessionSeconds = Number(__ENV.SESSION_SECONDS || 30);
const blockMs = Number(__ENV.BLOCK_MS || 150);
const triggerRate = Number(__ENV.TRIGGER_RATE || 30);
const triggerVUs = Number(__ENV.TRIGGER_PRE_ALLOCATED_VUS || 20);
const triggerMaxVUs = Number(__ENV.TRIGGER_MAX_VUS || 100);
const connectStaggerMs = Number(__ENV.CONNECT_STAGGER_MS || 0);

export const options = {
  scenarios: {
    consumers: {
      executor: 'constant-vus',
      vus: consumers,
      duration,
      gracefulStop: __ENV.GRACEFUL_STOP || '10s',
      exec: 'consumer',
    },
    triggers: {
      executor: 'constant-arrival-rate',
      rate: triggerRate,
      timeUnit: '1s',
      duration,
      startTime: __ENV.TRIGGER_START_DELAY || '0s',
      preAllocatedVUs: triggerVUs,
      maxVUs: triggerMaxVUs,
      gracefulStop: __ENV.GRACEFUL_STOP || '10s',
      exec: 'triggerBid',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    auction_k6_slow_consumer_opened_total: ['count>0'],
    auction_k6_slow_consumer_business_response_rate: ['rate>0.95'],
  },
};

const opened = new Counter('auction_k6_slow_consumer_opened_total');
const messages = new Counter('auction_k6_slow_consumer_messages_total');
const errors = new Counter('auction_k6_slow_consumer_errors_total');
const closed = new Counter('auction_k6_slow_consumer_closed_total');
const bidResponses = new Counter('auction_k6_slow_consumer_bid_responses_total');
const bidLimited = new Counter('auction_k6_slow_consumer_bid_limited_total');
const businessResponseRate = new Rate('auction_k6_slow_consumer_business_response_rate');
const sessionMs = new Trend('auction_k6_slow_consumer_session_ms');

export function consumer() {
  const started = Date.now();
  const userID = `k6_ws_${((__VU - 1) % 512) + 1}`;
  if (connectStaggerMs > 0) {
    sleep(((__VU - 1) % consumers) * connectStaggerMs / 1000);
  }
  const ticket = issueTicket(userID);
  const ws = openAuctionSocket(ticket, 0, {
    open() {
      opened.add(1);
      check(true, { 'slow pressure socket opened': (v) => v === true });
    },
    message() {
      messages.add(1);
      const until = Date.now() + blockMs;
      while (Date.now() < until) {
        // Block this VU to simulate a slow JavaScript client.
      }
    },
    error() {
      errors.add(1);
    },
    close() {
      closed.add(1);
      sessionMs.add(Date.now() - started);
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
  const res = placeBid(amount, userID, 'p3-slow-trigger');
  const ok = check(res, {
    'slow pressure bid returned business response': (r) => r.status === 200 || r.status === 429,
  });
  if (!ok) {
    businessResponseRate.add(false);
    return;
  }
  businessResponseRate.add(true);
  bidResponses.add(1);
  const code = String(res.json('code') || '');
  if (res.status === 429 || code === 'RATE_LIMITED' || code === 'BID_AUCTION_TOO_HOT') {
    bidLimited.add(1);
  }
}
