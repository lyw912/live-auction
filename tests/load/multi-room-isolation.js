import { check, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import { issueTicketFor, openAuctionSocketFor, placeBidFor } from './lib/live-auction.js';

const HOT_ROOM_ID = __ENV.HOT_ROOM_ID || 'room_main';
const HOT_AUCTION_ID = __ENV.HOT_AUCTION_ID || 'auc_live';
const COLD_ROOM_ID = __ENV.COLD_ROOM_ID || 'room_side';
const COLD_AUCTION_ID = __ENV.COLD_AUCTION_ID || 'auc_side';

export const options = {
  scenarios: {
    hot_room_bidders: {
      executor: 'constant-vus',
      vus: Number(__ENV.HOT_BID_VUS || 8),
      duration: __ENV.DURATION || '20s',
      exec: 'hotRoomBidder',
    },
    cold_room_watchers: {
      executor: 'constant-vus',
      vus: Number(__ENV.COLD_WS_VUS || 4),
      duration: __ENV.DURATION || '20s',
      exec: 'coldRoomWatcher',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    checks: ['rate>0.95'],
    auction_k6_multi_room_cross_leak_rate: ['rate==0'],
    auction_k6_multi_room_cold_ws_errors_total: ['count==0'],
  },
};

const hotBidResponses = new Counter('auction_k6_multi_room_hot_bid_responses_total');
const coldMessages = new Counter('auction_k6_multi_room_cold_messages_total');
const coldErrors = new Counter('auction_k6_multi_room_cold_ws_errors_total');
const crossLeakRate = new Rate('auction_k6_multi_room_cross_leak_rate');

export function hotRoomBidder() {
  const amount = 15000 + ((__ITER % 10) * 5000);
  const res = placeBidFor(HOT_AUCTION_ID, amount, `k6_bidder_${__VU}_${__ITER % 7}`, 'k6-multi-hot');
  const ok = check(res, {
    'hot room bid got business response': (r) => r.status === 200,
  });
  if (ok) hotBidResponses.add(1);
  sleep(Number(__ENV.HOT_SLEEP_SECONDS || 0.05));
}

export function coldRoomWatcher() {
  const ticket = issueTicketFor(COLD_ROOM_ID, COLD_AUCTION_ID, `k6_ws_${__VU}`);
  const ws = openAuctionSocketFor(COLD_ROOM_ID, COLD_AUCTION_ID, ticket, 0, {
    open() {
      check(true, { 'cold room socket opened': (v) => v === true });
    },
    message(event) {
      const text = String(event.data || '');
      coldMessages.add(1);
      const leaked = text.includes(HOT_AUCTION_ID) || text.includes(HOT_ROOM_ID);
      crossLeakRate.add(leaked);
    },
    error() {
      coldErrors.add(1);
    },
  });
  sleep(Number(__ENV.COLD_SESSION_SECONDS || 4));
  if (ws.readyState === 1) {
    ws.close();
  }
}
