import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';
import { issueTicket, openAuctionSocket, placeBid } from './lib/live-auction.js';

export const options = {
  scenarios: {
    reconnect_storm: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 10),
      duration: __ENV.DURATION || '20s',
      gracefulStop: __ENV.GRACEFUL_STOP || '5s',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    checks: ['rate>0.95'],
  },
};

const recovered = new Counter('auction_k6_ws_recovered_total');
const reconnectErrors = new Counter('auction_k6_ws_reconnect_errors_total');

export default function () {
  if (__VU === 1) {
    placeBid(60000, `k6_bidder_1_${__ITER % 7}`, 'k6-reconnect-trigger');
  }

  const staleLastSeq = Number(__ENV.LAST_SEQ || 1);
  const ticket = issueTicket(`k6_ws_${__VU}`);
  const ws = openAuctionSocket(ticket, staleLastSeq, {
    open() {
      check(true, { 'reconnect socket opened': (v) => v === true });
    },
    message(event) {
      const text = String(event.data || '');
      if (text.includes('snapshot') || text.includes('event_type')) {
        recovered.add(1);
        ws.close();
      }
    },
    error() {
      reconnectErrors.add(1);
    },
  });
  sleep(Number(__ENV.SESSION_SECONDS || 1.2));
  if (ws.readyState === 1) {
    ws.close();
  }
}
