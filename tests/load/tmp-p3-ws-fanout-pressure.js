import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';
import { issueTicket, openAuctionSocket, placeBid } from './lib/live-auction.js';

const watchers = Number(__ENV.WATCHERS || 100);
const duration = __ENV.DURATION || '60s';
const sessionSeconds = Number(__ENV.SESSION_SECONDS || 30);
const triggerVUs = Number(__ENV.TRIGGER_VUS || 4);
const triggerSleep = Number(__ENV.TRIGGER_SLEEP_SECONDS || 0.1);

export const options = {
  scenarios: {
    watchers: {
      executor: 'constant-vus',
      vus: watchers,
      duration,
      gracefulStop: '10s',
      exec: 'watcher',
    },
    triggers: {
      executor: 'constant-vus',
      vus: triggerVUs,
      duration,
      gracefulStop: '10s',
      exec: 'triggerBid',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    auction_k6_ws_pressure_errors_total: ['count==0'],
  },
};

const wsOpened = new Counter('auction_k6_ws_pressure_opened_total');
const wsMessages = new Counter('auction_k6_ws_pressure_messages_total');
const wsErrors = new Counter('auction_k6_ws_pressure_errors_total');
const wsClosed = new Counter('auction_k6_ws_pressure_closed_total');
const bidResponses = new Counter('auction_k6_ws_pressure_bid_responses_total');
const bidLimited = new Counter('auction_k6_ws_pressure_bid_limited_total');
const sessionMs = new Trend('auction_k6_ws_pressure_session_ms');

export function watcher() {
  const started = Date.now();
  const userID = `k6_ws_${((__VU - 1) % 512) + 1}`;
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
  const amount = 60000 + ((__ITER % 5) * 5000);
  const res = placeBid(amount, userID, 'tmp-p3-ws-trigger');
  if (res.status === 200 || res.status === 429) {
    bidResponses.add(1);
    const code = String(res.json('code') || '');
    if (res.status === 429 || code === 'RATE_LIMITED' || code === 'BID_AUCTION_TOO_HOT') {
      bidLimited.add(1);
    }
  }
  sleep(triggerSleep);
}
