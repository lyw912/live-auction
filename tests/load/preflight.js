import { check } from 'k6';
import { getSnapshot, issueTicket, placeBid } from './lib/live-auction.js';

export const options = {
  scenarios: {
    preflight: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: 1,
    },
  },
  thresholds: {
    checks: ['rate==1'],
  },
};

export default function () {
  const snapshot = getSnapshot();
  check(snapshot, {
    'preflight snapshot has auction id': (s) => Boolean(s.id),
  });

  const ticket = issueTicket('k6_ws_1');
  check(ticket, {
    'preflight ticket issued': (t) => Boolean(t),
  });

  const amount = Number(snapshot.current_price_cents || 0) + Number(snapshot.increment_cents || 5000);
  const res = placeBid(amount, 'k6_bidder_1_0', 'k6-preflight');
  check(res, {
    'preflight bid got business response': (r) => r.status === 200 || r.status === 429,
  });
}
