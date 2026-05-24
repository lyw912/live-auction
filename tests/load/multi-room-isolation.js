import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { issueTicketFor, openAuctionSocketFor, placeBidFor } from './lib/live-auction.js';

const HOT_ROOM_ID = __ENV.HOT_ROOM_ID || 'room_main';
const HOT_AUCTION_ID = __ENV.HOT_AUCTION_ID || 'auc_live';
const COLD_ROOM_ID = __ENV.COLD_ROOM_ID || 'room_side';
const COLD_AUCTION_ID = __ENV.COLD_AUCTION_ID || 'auc_side';
const coldTriggerRate = Number(__ENV.COLD_TRIGGER_RATE || 1);

export const options = {
  scenarios: {
    hot_room_bidders: {
      executor: 'constant-vus',
      vus: Number(__ENV.HOT_BID_VUS || 8),
      duration: __ENV.DURATION || '20s',
      gracefulStop: __ENV.GRACEFUL_STOP || '5s',
      exec: 'hotRoomBidder',
    },
    cold_room_watchers: {
      executor: 'constant-vus',
      vus: Number(__ENV.COLD_WS_VUS || 4),
      duration: __ENV.DURATION || '20s',
      gracefulStop: __ENV.GRACEFUL_STOP || '5s',
      exec: 'coldRoomWatcher',
    },
    cold_room_heartbeat: {
      executor: 'constant-arrival-rate',
      rate: coldTriggerRate,
      timeUnit: '1s',
      duration: __ENV.DURATION || '20s',
      startTime: __ENV.COLD_TRIGGER_START_DELAY || '2s',
      preAllocatedVUs: Number(__ENV.COLD_TRIGGER_PRE_ALLOCATED_VUS || 2),
      maxVUs: Number(__ENV.COLD_TRIGGER_MAX_VUS || 10),
      gracefulStop: __ENV.GRACEFUL_STOP || '5s',
      exec: 'coldRoomHeartbeat',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    checks: ['rate>0.95'],
    auction_k6_multi_room_cross_leak_rate: ['rate==0'],
    auction_k6_multi_room_cold_ws_errors_total: ['count==0'],
    auction_k6_multi_room_cold_opened_total: ['count>0'],
    auction_k6_multi_room_hot_business_response_rate: ['rate>0.95'],
  },
};

const hotBidResponses = new Counter('auction_k6_multi_room_hot_bid_responses_total');
const hotBidLimited = new Counter('auction_k6_multi_room_hot_bid_limited_total');
const hotBidHTTPError = new Counter('auction_k6_multi_room_hot_bid_http_errors_total');
const hotBusinessResponseRate = new Rate('auction_k6_multi_room_hot_business_response_rate');
const hotBidLatency = new Trend('auction_k6_multi_room_hot_bid_latency_ms');
const coldBidResponses = new Counter('auction_k6_multi_room_cold_bid_responses_total');
const coldBidHTTPError = new Counter('auction_k6_multi_room_cold_bid_http_errors_total');
const coldBidLatency = new Trend('auction_k6_multi_room_cold_bid_latency_ms');
const coldOpened = new Counter('auction_k6_multi_room_cold_opened_total');
const coldClosed = new Counter('auction_k6_multi_room_cold_closed_total');
const coldMessages = new Counter('auction_k6_multi_room_cold_messages_total');
const coldErrors = new Counter('auction_k6_multi_room_cold_ws_errors_total');
const coldSessionMs = new Trend('auction_k6_multi_room_cold_session_ms');
const coldFirstMessageMs = new Trend('auction_k6_multi_room_cold_first_message_ms');
const crossLeakRate = new Rate('auction_k6_multi_room_cross_leak_rate');

export function hotRoomBidder() {
  const started = Date.now();
  const amount = 15000 + ((__ITER % 10) * 5000);
  const res = placeBidFor(HOT_AUCTION_ID, amount, `k6_bidder_${__VU}_${__ITER % 7}`, 'k6-multi-hot');
  hotBidLatency.add(Date.now() - started);
  const ok = check(res, {
    'hot room bid got business response': (r) => r.status === 200 || r.status === 429,
  });
  hotBusinessResponseRate.add(ok);
  if (!ok) {
    hotBidHTTPError.add(1);
  } else {
    hotBidResponses.add(1);
    const code = String(res.json('code') || '');
    if (code === 'RATE_LIMITED' || code === 'BID_AUCTION_TOO_HOT') {
      hotBidLimited.add(1);
    }
  }
  sleep(Number(__ENV.HOT_SLEEP_SECONDS || 0.05));
}

export function coldRoomWatcher() {
  const started = Date.now();
  let firstMessage = false;
  const ticket = issueTicketFor(COLD_ROOM_ID, COLD_AUCTION_ID, `k6_ws_${__VU}`);
  const ws = openAuctionSocketFor(COLD_ROOM_ID, COLD_AUCTION_ID, ticket, 0, {
    open() {
      coldOpened.add(1);
      check(true, { 'cold room socket opened': (v) => v === true });
    },
    message(event) {
      const text = String(event.data || '');
      coldMessages.add(1);
      if (!firstMessage) {
        firstMessage = true;
        coldFirstMessageMs.add(Date.now() - started);
      }
      const leaked = text.includes(HOT_AUCTION_ID) || text.includes(HOT_ROOM_ID);
      crossLeakRate.add(leaked);
    },
    close() {
      coldClosed.add(1);
      coldSessionMs.add(Date.now() - started);
    },
    error() {
      coldErrors.add(1);
    },
  });
  sleep(Number(__ENV.COLD_SESSION_SECONDS || 1.2));
  if (ws.readyState === 1) {
    ws.close();
  }
}

export function coldRoomHeartbeat() {
  const started = Date.now();
  const amount = 15000 + ((__ITER % 10) * 5000);
  const res = placeBidFor(COLD_AUCTION_ID, amount, `k6_bidder_${(__VU % 4) + 1}_${__ITER % 7}`, 'k6-multi-cold');
  coldBidLatency.add(Date.now() - started);
  const ok = check(res, {
    'cold room bid got business response': (r) => r.status === 200 || r.status === 429,
  });
  if (ok) {
    coldBidResponses.add(1);
  } else {
    coldBidHTTPError.add(1);
  }
}
