/**
 * S2 — 正常竞价 / Steady Auction Soak
 *
 * Open-model (ramping-arrival-rate) sustained bid load.
 * Purpose: prove stable decision p99 ≤ 100ms at sustained rate, and detect
 * resource leaks via long-running hold (watch Grafana: heap floor, goroutines, fd).
 *
 * Bid model:
 *   - Minority active bidders; amounts escalate with elapsed time (price ladder climbs).
 *   - Noise: 20% of bids use stale client_seen_seq=0 (normal reject path).
 *   - Closed-loop (constant-vus) is intentionally avoided: open arrival rate
 *     exposes real latency; closed loops hide overload via coordinated omission.
 *
 * Env vars:
 *   BASE_URL        default http://127.0.0.1:18080
 *   AUCTION_ID      default auc_live
 *   USER_PREFIX     default k6_bidder_ (must match prepare-l2p4 steady seed)
 *   STAGE1_RATE     offered bids/s stage 1, default 20
 *   STAGE2_RATE     offered bids/s stage 2, default 60
 *   STAGE3_RATE     offered bids/s stage 3, default 100
 *   STAGE_DUR       duration of each stage (e.g. 10m for soak), default 10m
 *   INCREMENT_CENTS bid increment in cents, default 5000
 *   BASE_PRICE_CENTS starting price in cents, default 10000
 *   CLIMB_PERIOD_S  seconds per increment step (price climbs slowly), default 30
 *   RUN_ID          unique suffix for idempotency/client bid ids, default Date.now()
 *   AMOUNT_MODE     time_ladder or fast_ladder, default time_ladder
 *   NOISE_PCT       stale/noise bid percentage, default 20
 *   AMOUNT_JITTER_STEPS fast_ladder amount spread per time bucket, default 1
 *   USER_COUNT      bidder identities to rotate through, default MAX_VUS
 *   DROPPED_ITERATIONS_MAX threshold upper bound, default 200
 *   STAIR_HOLD      set 1 to add ramp+hold pairs per target, default 0
 *   RAMP_DUR        ramp duration used when STAIR_HOLD=1, default 1s
 *
 * Run (short smoke):
 *   k6 run --env STAGE1_RATE=5 --env STAGE2_RATE=15 --env STAGE3_RATE=30 \
 *          --env STAGE_DUR=2m tests/load/s2-steady-soak.js
 *
 * Run (full soak — leave Grafana open):
 *   k6 run tests/load/s2-steady-soak.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE_URL       = __ENV.BASE_URL       || 'http://127.0.0.1:18080';
const AUCTION_ID     = __ENV.AUCTION_ID     || 'auc_live';
const USER_PREFIX    = __ENV.USER_PREFIX    || 'k6_bidder_';
const STAGE1_RATE    = Number(__ENV.STAGE1_RATE    || 20);
const STAGE2_RATE    = Number(__ENV.STAGE2_RATE    || 60);
const STAGE3_RATE    = Number(__ENV.STAGE3_RATE    || 100);
const STAGE_DUR      = __ENV.STAGE_DUR      || '10m';
const INCREMENT_CENTS= Number(__ENV.INCREMENT_CENTS|| 5000);
const BASE_PRICE_CENTS=Number(__ENV.BASE_PRICE_CENTS||10000);
const CLIMB_PERIOD_S = Number(__ENV.CLIMB_PERIOD_S || 30);
const RUN_ID         = __ENV.RUN_ID || String(Date.now());
const AMOUNT_MODE    = __ENV.AMOUNT_MODE || 'time_ladder';
const NOISE_PCT      = Number(__ENV.NOISE_PCT || 20);
const AMOUNT_JITTER_STEPS = Math.max(1, Number(__ENV.AMOUNT_JITTER_STEPS || 1));
const STAIR_HOLD     = __ENV.STAIR_HOLD === '1';
const RAMP_DUR       = __ENV.RAMP_DUR || '1s';

// Pre-allocate VUs: Little's Law = rate × expected_duration + 30% headroom.
// At 100 rps and ~80ms average duration: 100 × 0.08 × 1.3 ≈ 11; use 50 for safety.
const PRE_ALLOC_VUS  = Number(__ENV.PRE_ALLOC_VUS || 50);
const MAX_VUS        = Number(__ENV.MAX_VUS        || 200);
const USER_COUNT     = Math.max(1, Number(__ENV.USER_COUNT || MAX_VUS));
const DROPPED_ITERATIONS_MAX = Number(__ENV.DROPPED_ITERATIONS_MAX || 200);

function rateTargets() {
  const targets = [STAGE1_RATE, STAGE2_RATE, STAGE3_RATE];
  if (__ENV.STAGE4_RATE) targets.push(Number(__ENV.STAGE4_RATE));
  if (__ENV.STAGE5_RATE) targets.push(Number(__ENV.STAGE5_RATE));
  return targets;
}

function buildStages() {
  if (!STAIR_HOLD) {
    return [
      ...rateTargets().map((target) => ({ target, duration: STAGE_DUR })),
      { target: 0, duration: '30s' },
    ];
  }

  const stages = [];
  for (const target of rateTargets()) {
    stages.push({ target, duration: RAMP_DUR });
    stages.push({ target, duration: STAGE_DUR });
  }
  stages.push({ target: 0, duration: '30s' });
  return stages;
}

export const options = {
  scenarios: {
    steady_bids: {
      executor:        'ramping-arrival-rate',  // OPEN model: no coordinated omission
      startRate:       STAGE1_RATE,
      timeUnit:        '1s',
      preAllocatedVUs: PRE_ALLOC_VUS,
      maxVUs:          MAX_VUS,
      stages: buildStages(),
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    // M1 SLO for steady scenario: ≤100ms
    'http_req_duration{sampler:bid-decision}': ['p(99)<100'],
    // Overload signal: dropped iterations must be low
    dropped_iterations: [`count<${DROPPED_ITERATIONS_MAX}`],
    // No admission contamination
    s2_admission_contamination: ['count==0'],
    // Auth/ACL failures mean the harness did not reach the bid engine.
    s2_auth_acl_failures: ['count==0'],
    // Any HTTP/business response outside final decision or explicit pending is
    // a user-visible interruption and invalidates the clean S2 pass.
    http_req_failed: ['rate==0'],
    s2_non_decision_failures: ['count==0'],
  },
};

// --- metrics ---
const decidedTotal     = new Counter('s2_decided_total');
const acceptedTotal    = new Counter('s2_accepted_total');
const rejectedTotal    = new Counter('s2_rejected_total');
const pendingTotal     = new Counter('s2_pending_total');     // 202 PROCESSING_RETRY_LATER
const admissionContam  = new Counter('s2_admission_contamination');
const authACLFailures  = new Counter('s2_auth_acl_failures');
const nonDecisionFailures = new Counter('s2_non_decision_failures');
const decisionLatency  = new Trend('s2_decision_latency_ms', true);  // M1 custom trend

// --- helpers ---
const t0 = Date.now();

function currentUserOrdinal() {
  return ((__VU + __ITER - 1) % USER_COUNT) + 1;
}

function currentBidAmount(userOrdinal) {
  const elapsedMs = Date.now() - t0;
  const periodMs = Math.max(1, Math.floor(CLIMB_PERIOD_S * 1000));
  let steps;

  if (AMOUNT_MODE === 'fast_ladder') {
    // Capacity-stair mode: make bid amounts advance much faster than the old
    // 30s ladder so high-RPS stages do not degrade into mostly same-price
    // Redis rejects. This is an adversarial accepted-update profile, not a
    // realistic price curve.
    const bucket = Math.floor(elapsedMs / periodMs);
    steps = bucket * AMOUNT_JITTER_STEPS + ((userOrdinal + __ITER) % AMOUNT_JITTER_STEPS);
  } else {
    // Long-soak mode: slow business-like price movement with some stale bids.
    steps = Math.floor((elapsedMs / 1000) / CLIMB_PERIOD_S);
  }

  const baseAmount = BASE_PRICE_CENTS + steps * INCREMENT_CENTS;
  const isNoiseBid = ((userOrdinal * 7 + __ITER * 3) % 100) < NOISE_PCT;
  return isNoiseBid ? Math.max(0, baseAmount - INCREMENT_CENTS) : baseAmount + INCREMENT_CENTS;
}

function authHeaders(userID) {
  return {
    'Content-Type':   'application/json',
    'X-Mock-Role':    'user',
    'X-Mock-User-Id': userID,
  };
}

export default function () {
  const userOrdinal = currentUserOrdinal();
  const userID      = `${USER_PREFIX}${userOrdinal}`;
  const clientBidID = `s2-${RUN_ID}-${userOrdinal}-${__VU}-${__ITER}`;
  const amount      = currentBidAmount(userOrdinal);

  const startMs = Date.now();
  const res = http.post(
    `${BASE_URL}/api/auctions/${AUCTION_ID}/bids`,
    JSON.stringify({
      client_bid_id:    clientBidID,
      idempotency_key:  clientBidID,
      amount_cents:     amount,
      client_seen_seq:  0,
    }),
    {
      headers: { ...authHeaders(userID), 'Idempotency-Key': clientBidID },
      tags:    { sampler: 'bid-decision' },  // PTS sampler tag → "出价决策 bid-decision"
    },
  );

  // M1: only count final decisions, never 202 pending
  let body;
  try { body = res.json(); } catch (_) { body = {}; }

  const result         = String(body.result          || '');
  const code           = String(body.code            || '');
  const decisionStatus = String(body.decision_status || '');
  const durability     = String(body.durability_status|| '');

  // Admission contamination — must never appear
  if (res.status === 429 || result === 'RATE_LIMITED' || code === 'RATE_LIMITED') {
    admissionContam.add(1);
    check(res, { 's2 no admission contamination': () => false });
    return;
  }

  if (res.status === 401 || res.status === 403) {
    authACLFailures.add(1);
    check(res, { 's2 auth and ACL reached bid engine': () => false });
    return;
  }

  // 202 pending — count but do not record as final decision
  if (res.status === 202) {
    pendingTotal.add(1);
    return;
  }

  // Final decision (200 + DECIDED)
  if (res.status === 200 && decisionStatus === 'DECIDED') {
    const latMs = Date.now() - startMs;
    decisionLatency.add(latMs);
    decidedTotal.add(1);
    if (result === 'ENGINE_ACCEPTED' || result === 'ENGINE_SOLD') {
      acceptedTotal.add(1);
    } else {
      rejectedTotal.add(1);
    }
    check(res, {
      's2 final decision has engine_seq': () => Number(body.engine_seq || 0) > 0,
      's2 durability durable':            () => durability === 'ENGINE_DURABLE',
    });
    return;
  }

  nonDecisionFailures.add(1);
  check(res, { 's2 only final decision or explicit pending': () => false });
}
