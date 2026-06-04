/**
 * S2-read-interference — steady bid decisions while reader traffic polls the
 * auction state.
 *
 * This is not the S3 WebSocket fanout test. It isolates HTTP read pressure:
 * auction snapshot, leaderboard, and current user's bid history while the bid
 * path continues to run as an open-model arrival-rate workload.
 *
 * Env vars:
 *   BASE_URL            default http://127.0.0.1:18080
 *   AUCTION_ID          default auc_live
 *   BID_USER_PREFIX     default k6_bidder_
 *   READ_USER_PREFIX    default k6_user_
 *   STAGE_DUR           default 5m
 *   BID_STAGE{1,2,3}_RATE  default 20,60,100 bids/s
 *   READ_STAGE{1,2,3}_RATE default 200,600,1000 reads/s
 *   BID_PRE_ALLOC_VUS   default 80
 *   BID_MAX_VUS         default 300
 *   READ_PRE_ALLOC_VUS  default 160
 *   READ_MAX_VUS        default 600
 */

import http from 'k6/http';
import { check } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:18080';
const AUCTION_ID = __ENV.AUCTION_ID || 'auc_live';
const BID_USER_PREFIX = __ENV.BID_USER_PREFIX || 'k6_bidder_';
const READ_USER_PREFIX = __ENV.READ_USER_PREFIX || 'k6_user_';

const STAGE_DUR = __ENV.STAGE_DUR || '5m';
const BID_STAGE1_RATE = Number(__ENV.BID_STAGE1_RATE || 20);
const BID_STAGE2_RATE = Number(__ENV.BID_STAGE2_RATE || 60);
const BID_STAGE3_RATE = Number(__ENV.BID_STAGE3_RATE || 100);
const READ_STAGE1_RATE = Number(__ENV.READ_STAGE1_RATE || 200);
const READ_STAGE2_RATE = Number(__ENV.READ_STAGE2_RATE || 600);
const READ_STAGE3_RATE = Number(__ENV.READ_STAGE3_RATE || 1000);
const BID_PRE_ALLOC_VUS = Number(__ENV.BID_PRE_ALLOC_VUS || 80);
const BID_MAX_VUS = Number(__ENV.BID_MAX_VUS || 300);
const READ_PRE_ALLOC_VUS = Number(__ENV.READ_PRE_ALLOC_VUS || 160);
const READ_MAX_VUS = Number(__ENV.READ_MAX_VUS || 600);

const INCREMENT_CENTS = Number(__ENV.INCREMENT_CENTS || 5000);
const BASE_PRICE_CENTS = Number(__ENV.BASE_PRICE_CENTS || 10000);
const CLIMB_PERIOD_S = Number(__ENV.CLIMB_PERIOD_S || 30);
const RUN_ID = __ENV.RUN_ID || String(Date.now());

export const options = {
  scenarios: {
    steady_bids: {
      executor: 'ramping-arrival-rate',
      startRate: BID_STAGE1_RATE,
      timeUnit: '1s',
      preAllocatedVUs: BID_PRE_ALLOC_VUS,
      maxVUs: BID_MAX_VUS,
      stages: [
        { target: BID_STAGE1_RATE, duration: STAGE_DUR },
        { target: BID_STAGE2_RATE, duration: STAGE_DUR },
        { target: BID_STAGE3_RATE, duration: STAGE_DUR },
        { target: 0, duration: '30s' },
      ],
      exec: 'bidFn',
    },
    reader_traffic: {
      executor: 'ramping-arrival-rate',
      startRate: READ_STAGE1_RATE,
      timeUnit: '1s',
      preAllocatedVUs: READ_PRE_ALLOC_VUS,
      maxVUs: READ_MAX_VUS,
      stages: [
        { target: READ_STAGE1_RATE, duration: STAGE_DUR },
        { target: READ_STAGE2_RATE, duration: STAGE_DUR },
        { target: READ_STAGE3_RATE, duration: STAGE_DUR },
        { target: 0, duration: '30s' },
      ],
      exec: 'readFn',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    dropped_iterations: ['count<500'],
    http_req_failed: ['rate==0'],
    'http_req_duration{sampler:bid-decision}': ['p(99)<100'],
    'http_req_duration{sampler:read-auction-snapshot}': ['p(99)<200'],
    'http_req_duration{sampler:read-leaderboard}': ['p(99)<200'],
    'http_req_duration{sampler:read-bid-history}': ['p(99)<300'],
    s2ri_auth_acl_failures: ['count==0'],
    s2ri_admission_contamination: ['count==0'],
    s2ri_non_decision_failures: ['count==0'],
    s2ri_read_failures: ['count==0'],
    s2ri_decided_total: ['count>0'],
    s2ri_read_success_total: ['count>0'],
  },
};

const decidedTotal = new Counter('s2ri_decided_total');
const acceptedTotal = new Counter('s2ri_accepted_total');
const rejectedTotal = new Counter('s2ri_rejected_total');
const pendingTotal = new Counter('s2ri_pending_total');
const admissionContam = new Counter('s2ri_admission_contamination');
const authACLFailures = new Counter('s2ri_auth_acl_failures');
const nonDecisionFailures = new Counter('s2ri_non_decision_failures');
const readSuccessTotal = new Counter('s2ri_read_success_total');
const readFailures = new Counter('s2ri_read_failures');
const bidDecisionLatency = new Trend('s2ri_bid_decision_latency_ms', true);
const readLatency = new Trend('s2ri_read_latency_ms', true);

const t0 = Date.now();

function currentBidAmount() {
  const elapsedS = (Date.now() - t0) / 1000;
  const steps = Math.floor(elapsedS / CLIMB_PERIOD_S);
  const baseAmount = BASE_PRICE_CENTS + steps * INCREMENT_CENTS;
  const isNoiseBid = (__VU * 7 + __ITER * 3) % 10 < 2;
  return isNoiseBid ? Math.max(0, baseAmount - INCREMENT_CENTS) : baseAmount + INCREMENT_CENTS;
}

function userHeaders(userID) {
  return {
    'Content-Type': 'application/json',
    'X-Mock-Role': 'user',
    'X-Mock-User-Id': userID,
  };
}

function classifyAuthACL(res) {
  if (res.status === 401 || res.status === 403) {
    authACLFailures.add(1);
    return true;
  }
  return false;
}

export function bidFn() {
  const userID = `${BID_USER_PREFIX}${__VU}`;
  const clientBidID = `s2ri-${RUN_ID}-${__VU}-${__ITER}`;
  const amount = currentBidAmount();
  const startMs = Date.now();

  const res = http.post(
    `${BASE_URL}/api/auctions/${AUCTION_ID}/bids`,
    JSON.stringify({
      client_bid_id: clientBidID,
      idempotency_key: clientBidID,
      amount_cents: amount,
      client_seen_seq: 0,
    }),
    {
      headers: { ...userHeaders(userID), 'Idempotency-Key': clientBidID },
      tags: { sampler: 'bid-decision' },
    },
  );

  let body;
  try { body = res.json(); } catch (_) { body = {}; }

  const result = String(body.result || '');
  const code = String(body.code || '');
  const decisionStatus = String(body.decision_status || '');
  const durability = String(body.durability_status || '');

  if (res.status === 429 || result === 'RATE_LIMITED' || code === 'RATE_LIMITED') {
    admissionContam.add(1);
    check(res, { 's2ri no admission contamination': () => false });
    return;
  }
  if (classifyAuthACL(res)) {
    check(res, { 's2ri auth and ACL reached bid engine': () => false });
    return;
  }
  if (res.status === 202) {
    pendingTotal.add(1);
    return;
  }
  if (res.status === 200 && decisionStatus === 'DECIDED') {
    bidDecisionLatency.add(Date.now() - startMs);
    decidedTotal.add(1);
    if (result === 'ENGINE_ACCEPTED' || result === 'ENGINE_SOLD') {
      acceptedTotal.add(1);
    } else {
      rejectedTotal.add(1);
    }
    check(res, {
      's2ri final decision has engine_seq': () => Number(body.engine_seq || 0) > 0,
      's2ri durability durable': () => durability === 'ENGINE_DURABLE',
    });
    return;
  }

  nonDecisionFailures.add(1);
  check(res, { 's2ri only final decision or explicit pending': () => false });
}

export function readFn() {
  const userID = `${READ_USER_PREFIX}${__VU}`;
  const pick = (__ITER * 17 + __VU * 5) % 10;
  let url = `${BASE_URL}/api/auctions/${AUCTION_ID}`;
  let sampler = 'read-auction-snapshot';

  if (pick >= 6 && pick < 9) {
    url = `${BASE_URL}/api/auctions/${AUCTION_ID}/leaderboard?limit=5`;
    sampler = 'read-leaderboard';
  } else if (pick >= 9) {
    url = `${BASE_URL}/api/users/me/bids`;
    sampler = 'read-bid-history';
  }

  const startMs = Date.now();
  const res = http.get(url, {
    headers: userHeaders(userID),
    tags: { sampler },
  });

  if (classifyAuthACL(res)) {
    check(res, { 's2ri reader auth and ACL ok': () => false });
    return;
  }
  if (res.status !== 200) {
    readFailures.add(1);
    check(res, { 's2ri read status 200': () => false });
    return;
  }
  readLatency.add(Date.now() - startMs);
  readSuccessTotal.add(1);
}
