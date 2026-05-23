import { check, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import { getSnapshot, placeBid } from './lib/live-auction.js';

export const options = {
  scenarios: {
    repeated_user_ip_abuse: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 6),
      duration: __ENV.DURATION || '15s',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    checks: ['rate>0.95'],
    auction_k6_bid_limited_total: ['count>0'],
  },
};

const accepted = new Counter('auction_k6_bid_accepted_total');
const rejected = new Counter('auction_k6_bid_rejected_total');
const limited = new Counter('auction_k6_bid_limited_total');
const tooHot = new Counter('auction_k6_bid_too_hot_total');
const retryAfter = new Counter('auction_k6_bid_retry_after_total');
const httpErrors = new Counter('auction_k6_bid_http_error_total');
const limitedRate = new Rate('auction_k6_bid_limited_rate');

export default function () {
  const snapshot = getSnapshot();
  const increment = Number(snapshot.increment_cents || 5000);
  const amount = Number(snapshot.current_price_cents || 0) + increment;
  const abuseUser = __ENV.ABUSE_USER_ID || 'k6_bidder_1_0';
  const res = placeBid(amount, abuseUser, 'k6-abuse');
  const ok = check(res, {
    'bid abuse endpoint returned business response': (r) => r.status === 200 || r.status === 429,
  });
  if (!ok) {
    httpErrors.add(1);
    sleep(0.05);
    return;
  }
  if (res.headers['Retry-After']) {
    retryAfter.add(1);
  }
  const result = String(res.json('result') || '');
  const code = String(res.json('code') || '');
  const reason = String(res.json('reject_reason') || '');
  if (result === 'ACCEPTED' || result === 'ACCEPTED_EXTENDED' || result === 'ACCEPTED_SOLD') {
    accepted.add(1);
    limitedRate.add(false);
  } else if (code === 'RATE_LIMITED') {
    limited.add(1);
    limitedRate.add(true);
  } else if (code === 'BID_AUCTION_TOO_HOT') {
    tooHot.add(1);
    limitedRate.add(true);
  } else {
    rejected.add(1);
    limitedRate.add(reason === 'RATE_LIMITED' || result === 'RATE_LIMITED');
  }
  sleep(Number(__ENV.SLEEP_SECONDS || 0.01));
}
