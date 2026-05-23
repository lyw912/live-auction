import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';
import { issueTicket, openAuctionSocket, placeBid } from './lib/live-auction.js';

export const options = {
  scenarios: {
    slow_consumer: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 6),
      duration: __ENV.DURATION || '20s',
      gracefulStop: __ENV.GRACEFUL_STOP || '5s',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    checks: ['rate>0.95'],
  },
};

const closed = new Counter('auction_k6_ws_closed_total');

export default function () {
  const ticket = issueTicket(`k6_ws_${__VU}`);
  const ws = openAuctionSocket(ticket, 0, {
    open(socket) {
      check(true, { 'slow-consumer socket opened': (v) => v === true });
      if (__VU === 1) {
        for (let i = 0; i < 3; i += 1) {
          placeBid(60000, `k6_bidder_${i + 1}_${__ITER % 7}`, 'k6-slow-trigger');
        }
      }
      setTimeout(() => socket.close(), Number(__ENV.SESSION_MS || 1000));
    },
    message() {
      if (__ENV.CONSUME_MESSAGES !== '1') {
        const until = Date.now() + Number(__ENV.BLOCK_MS || 250);
        while (Date.now() < until) {
          // Intentionally block this VU to simulate a slow reader.
        }
      }
    },
    close() {
      closed.add(1);
    },
  });
  sleep(Number(__ENV.SESSION_SECONDS || 1.2));
  if (ws.readyState === 1) {
    ws.close();
  }
}
