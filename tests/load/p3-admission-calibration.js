import { check } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import { BASE_URL, AUCTION_ID, placeBid } from './lib/live-auction.js';
import http from 'k6/http';

const rate = Number(__ENV.RATE || 120);
const duration = __ENV.DURATION || '45s';
const preAllocatedVUs = Number(__ENV.PRE_ALLOCATED_VUS || 120);
const maxVUs = Number(__ENV.MAX_VUS || 400);
const users = Number(__ENV.USERS || 512);

export const options = {
  scenarios: {
    admission_calibration: {
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
    checks: ['rate>0.95'],
    auction_k6_admission_controlled_rejections_total: ['count>0'],
    auction_k6_admission_http_errors_total: ['count==0'],
  },
};

const accepted = new Counter('auction_k6_admission_accepted_total');
const rejected = new Counter('auction_k6_admission_business_rejected_total');
const rateLimited = new Counter('auction_k6_admission_rate_limited_total');
const tooHot = new Counter('auction_k6_admission_too_hot_total');
const controlledRejections = new Counter('auction_k6_admission_controlled_rejections_total');
const retryAfter = new Counter('auction_k6_admission_retry_after_total');
const httpErrors = new Counter('auction_k6_admission_http_errors_total');
const businessEnvelopeRate = new Rate('auction_k6_admission_business_envelope_rate');

export function setup() {
  const res = http.get(`${BASE_URL}/api/auctions/${AUCTION_ID}`, {
    headers: {
      'Content-Type': 'application/json',
      'X-Mock-Role': 'user',
      'X-Mock-User-Id': 'k6_bidder_1_0',
    },
    tags: { name: 'admission_setup_snapshot' },
  });
  check(res, { 'admission setup snapshot status 200': (r) => r.status === 200 });
  return {
    current: Number(res.json('current_price_cents') || 10000),
    increment: Number(res.json('increment_cents') || 5000),
    startedAt: Date.now(),
  };
}

export default function (data) {
  const userBucket = ((__VU + __ITER) % users) + 1;
  const lane = (__VU * 17 + __ITER) % 7;
  const userID = `k6_bidder_${userBucket}_${lane}`;
  const elapsedMs = Date.now() - data.startedAt;
  const step = Math.floor((elapsedMs * rate) / 1000) + (__ITER % 5) + 2;
  const amount = data.current + data.increment * step;
  const res = placeBid(amount, userID, 'p3-admission-calibration');
  const ok = check(res, {
    'admission calibration returned controlled envelope': (r) => r.status === 200 || r.status === 429,
  });
  if (!ok) {
    httpErrors.add(1);
    businessEnvelopeRate.add(false);
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
    businessEnvelopeRate.add(true);
  } else if (code === 'BID_AUCTION_TOO_HOT' || result === 'BID_AUCTION_TOO_HOT') {
    tooHot.add(1);
    controlledRejections.add(1);
    businessEnvelopeRate.add(true);
  } else if (res.status === 429 || code === 'RATE_LIMITED' || result === 'RATE_LIMITED') {
    rateLimited.add(1);
    controlledRejections.add(1);
    businessEnvelopeRate.add(true);
  } else {
    rejected.add(1);
    if (reason === 'RATE_LIMITED' || result === 'RATE_LIMITED') {
      controlledRejections.add(1);
    }
    businessEnvelopeRate.add(true);
  }
}
