import http from 'k6/http';
import { check } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import { BASE_URL, AUCTION_ID, placeBid } from './lib/live-auction.js';

const rate = Number(__ENV.RATE || 100);
const duration = __ENV.DURATION || '60s';
const preAllocatedVUs = Number(__ENV.PRE_ALLOCATED_VUS || 80);
const maxVUs = Number(__ENV.MAX_VUS || 256);

export const options = {
  scenarios: {
    bid_pressure: {
      executor: 'constant-arrival-rate',
      rate,
      timeUnit: '1s',
      duration,
      preAllocatedVUs,
      maxVUs,
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    auction_k6_pressure_http_errors_total: ['count==0'],
  },
};

const accepted = new Counter('auction_k6_pressure_bid_accepted_total');
const rejected = new Counter('auction_k6_pressure_bid_rejected_total');
const limited = new Counter('auction_k6_pressure_bid_limited_total');
const tooHot = new Counter('auction_k6_pressure_bid_too_hot_total');
const retryLater = new Counter('auction_k6_pressure_bid_retry_later_total');
const httpErrors = new Counter('auction_k6_pressure_http_errors_total');
const businessResponseRate = new Rate('auction_k6_pressure_business_response_rate');

export function setup() {
  const res = http.get(`${BASE_URL}/api/auctions/${AUCTION_ID}`, {
    headers: {
      'Content-Type': 'application/json',
      'X-Mock-Role': 'user',
      'X-Mock-User-Id': 'k6_user_1',
    },
    tags: { name: 'pressure_setup_snapshot' },
  });
  check(res, { 'pressure setup snapshot status 200': (r) => r.status === 200 });
  return {
    current: Number(res.json('current_price_cents') || 10000),
    increment: Number(res.json('increment_cents') || 5000),
    startedAt: Date.now(),
  };
}

export default function (data) {
  const userBucket = ((__VU - 1) % 512) + 1;
  const lane = __ITER % 7;
  const userID = `k6_bidder_${userBucket}_${lane}`;
  const elapsedMs = Date.now() - data.startedAt;
  const globalStep = Math.floor((elapsedMs * rate) / 1000) + (__ITER % 3) + 2;
  const amount = data.current + data.increment * globalStep;
  const res = placeBid(amount, userID, 'p3-pressure');
  const ok = check(res, {
    'bid pressure returned expected HTTP envelope': (r) => r.status === 200 || r.status === 429,
  });
  if (!ok) {
    httpErrors.add(1);
    businessResponseRate.add(false);
    return;
  }

  const result = String(res.json('result') || '');
  const code = String(res.json('code') || '');
  const reason = String(res.json('reject_reason') || '');
  if (result === 'ACCEPTED' || result === 'ACCEPTED_EXTENDED' || result === 'ACCEPTED_SOLD') {
    accepted.add(1);
    businessResponseRate.add(true);
  } else if (res.status === 429 || code === 'RATE_LIMITED' || result === 'RATE_LIMITED') {
    limited.add(1);
    businessResponseRate.add(false);
  } else if (code === 'BID_AUCTION_TOO_HOT' || result === 'BID_AUCTION_TOO_HOT') {
    tooHot.add(1);
    businessResponseRate.add(false);
  } else {
    rejected.add(1);
    businessResponseRate.add(true);
    if (reason === 'BID_RETRY_LATER' || result === 'BID_RETRY_LATER') {
      retryLater.add(1);
    }
  }
}
