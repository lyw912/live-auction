import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { issueTicket, openAuctionSocket, placeBid } from './lib/live-auction.js';

const watchers = Number(__ENV.WATCHERS || 100);
const duration = __ENV.DURATION || '60s';
const sessionSeconds = Number(__ENV.SESSION_SECONDS || 30);
const triggerRate = Number(__ENV.TRIGGER_RATE || 20);
const triggerVUs = Number(__ENV.TRIGGER_PRE_ALLOCATED_VUS || 20);
const triggerMaxVUs = Number(__ENV.TRIGGER_MAX_VUS || 100);
const connectStaggerMs = Number(__ENV.CONNECT_STAGGER_MS || 0);

export const options = {
  scenarios: {
    watchers: {
      executor: 'constant-vus',
      vus: watchers,
      duration,
      gracefulStop: __ENV.GRACEFUL_STOP || '10s',
      exec: 'watcher',
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
    auction_k6_ws_pressure_opened_total: ['count>0'],
    auction_k6_ws_pressure_business_response_rate: ['rate>0.95'],
    auction_k6_ws_pressure_errors_total: ['count==0'],
  },
};

const wsOpened = new Counter('auction_k6_ws_pressure_opened_total');
const wsMessages = new Counter('auction_k6_ws_pressure_messages_total');
const wsErrors = new Counter('auction_k6_ws_pressure_errors_total');
const wsClosed = new Counter('auction_k6_ws_pressure_closed_total');
const bidAccepted = new Counter('auction_k6_ws_pressure_bid_accepted_total');
const bidRejected = new Counter('auction_k6_ws_pressure_bid_rejected_total');
const bidLimited = new Counter('auction_k6_ws_pressure_bid_limited_total');
const businessResponseRate = new Rate('auction_k6_ws_pressure_business_response_rate');
const sessionMs = new Trend('auction_k6_ws_pressure_session_ms');

export function watcher() {
  const started = Date.now();
  const userID = `k6_ws_${((__VU - 1) % 512) + 1}`;
  if (connectStaggerMs > 0) {
    sleep(((__VU - 1) % watchers) * connectStaggerMs / 1000);
  }
  const ticket = issueTicket(userID);
  const ws = openAuctionSocket(ticket, 0, {
    open() {
      wsOpened.add(1);
      check(true, { 'pressure watcher opened': (v) => v === true });
    },
    message() {
      wsMessages.add(1);
    },
    error() {
      wsErrors.add(1);
    },
    close() {
      wsClosed.add(1);
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
  const res = placeBid(amount, userID, 'p3-ws-trigger');
  const ok = check(res, {
    'pressure bid returned business response': (r) => r.status === 200 || r.status === 429,
  });
  if (!ok) {
    businessResponseRate.add(false);
    return;
  }
  businessResponseRate.add(true);
  const result = String(res.json('result') || '');
  const code = String(res.json('code') || '');
  if (result === 'ACCEPTED' || result === 'ACCEPTED_EXTENDED' || result === 'ACCEPTED_SOLD') {
    bidAccepted.add(1);
  } else if (res.status === 429 || code === 'RATE_LIMITED' || code === 'BID_AUCTION_TOO_HOT') {
    bidLimited.add(1);
  } else {
    bidRejected.add(1);
  }
}
