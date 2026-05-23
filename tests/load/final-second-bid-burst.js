import { check, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import { getSnapshot, placeBid } from './lib/live-auction.js';

export const options = {
  scenarios: {
    final_second_bid_burst: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 8),
      duration: __ENV.DURATION || '20s',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    checks: ['rate>0.95'],
    bid_http_errors: ['count==0'],
  },
};

const accepted = new Counter('auction_k6_bid_accepted_total');
const rejected = new Counter('auction_k6_bid_rejected_total');
const limited = new Counter('auction_k6_bid_limited_total');
const tooHot = new Counter('auction_k6_bid_too_hot_total');
const retryLater = new Counter('auction_k6_bid_retry_later_total');
const bidHTTPError = new Counter('bid_http_errors');
const acceptedRate = new Rate('auction_k6_bid_accepted_rate');

export default function () {
  const snapshot = getSnapshot();
  const current = Number(snapshot.current_price_cents || 0);
  const increment = Number(snapshot.increment_cents || 5000);
  const rawCap = Number(snapshot.cap_price_cents || 0);
  const cap = rawCap > 0 ? rawCap : current + increment * 1000;
  const allowSold = String(__ENV.ALLOW_SOLD || '').toLowerCase() === 'true';
  const desired = current + increment * ((__VU % 3) + 1);
  const amount = allowSold ? Math.min(cap, desired) : Math.min(cap - increment, desired);
  const userID = `k6_bidder_${__VU}_${__ITER % 7}`;
  const res = placeBid(amount, userID, 'k6-final-second');
  const ok = check(res, {
    'bid endpoint returned business response': (r) => r.status === 200 || r.status === 429,
  });
  if (!ok) {
    bidHTTPError.add(1);
    sleep(0.1);
    return;
  }
  const result = String(res.json('result') || '');
  const code = String(res.json('code') || '');
  const reason = String(res.json('reject_reason') || '');
  if (result === 'ACCEPTED' || result === 'ACCEPTED_EXTENDED' || result === 'ACCEPTED_SOLD') {
    accepted.add(1);
    acceptedRate.add(true);
  } else if (code === 'RATE_LIMITED' || result === 'RATE_LIMITED') {
    limited.add(1);
    acceptedRate.add(false);
  } else if (code === 'BID_AUCTION_TOO_HOT' || result === 'BID_AUCTION_TOO_HOT') {
    tooHot.add(1);
    acceptedRate.add(false);
  } else {
    rejected.add(1);
    acceptedRate.add(false);
    if (reason === 'BID_RETRY_LATER' || result === 'BID_RETRY_LATER') {
      retryLater.add(1);
    }
  }
  sleep(Number(__ENV.SLEEP_SECONDS || 0.05));
}
