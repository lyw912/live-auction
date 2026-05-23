import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';
import { issueTicket, openAuctionSocket, placeBid } from './lib/live-auction.js';

export const options = {
  scenarios: {
    watcher_fanout: {
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

const wsMessages = new Counter('auction_k6_ws_messages_total');
const wsErrors = new Counter('auction_k6_ws_errors_total');
const wsSessionMs = new Trend('auction_k6_ws_session_ms');

export default function () {
  const started = Date.now();
  const ticket = issueTicket(`k6_ws_${__VU}`);
  const ws = openAuctionSocket(ticket, 0, {
    open(socket) {
      check(true, { 'watcher socket opened': (v) => v === true });
      if (__VU === 1) {
        placeBid(60000, `k6_bidder_1_${__ITER % 7}`, 'k6-fanout-trigger');
      }
      setTimeout(() => socket.close(), Number(__ENV.SESSION_MS || 1000));
    },
    message() {
      wsMessages.add(1);
    },
    error() {
      wsErrors.add(1);
    },
    close() {
      wsSessionMs.add(Date.now() - started);
    },
  });
  sleep(Number(__ENV.SESSION_SECONDS || 1.2));
  if (ws.readyState === 1) {
    ws.close();
  }
}
