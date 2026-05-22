import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';
import { placeBid } from './lib/live-auction.js';

export const options = {
  scenarios: {
    outbox_burst: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 8),
      duration: __ENV.DURATION || '20s',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    checks: ['rate>0.95'],
  },
};

const businessResponses = new Counter('auction_k6_outbox_business_responses_total');

export default function () {
  const amount = __VU % 2 === 0 ? 1000 : 60000;
  const res = placeBid(amount, `k6_outbox_${__VU}_${__ITER % 5}`, 'k6-outbox-burst');
  const ok = check(res, {
    'outbox burst bid got response': (r) => r.status === 200,
  });
  if (ok) {
    businessResponses.add(1);
  }
  sleep(Number(__ENV.SLEEP_SECONDS || 0.02));
}
