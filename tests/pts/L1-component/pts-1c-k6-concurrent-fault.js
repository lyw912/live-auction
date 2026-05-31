/**
 * Layer C — concurrent fault injection load script.
 *
 * Purpose: generate sustained bid pressure while an external orchestrator
 * injects Redis / Kafka faults. This script does NOT know when the fault
 * fires; it runs continuously and tracks response distribution so the
 * runner can verify: (a) fault was observed (paused_total > 0), and
 * (b) no admission contamination occurred at any point.
 *
 * Key differences from PTS-1B (intentional, documented):
 *   - Requires ALLOW_MOCK_AUTH=true on the server (X-Mock-* headers).
 *     JWT session pool is not needed because Layer C proves correctness
 *     under faults, not the auth pipeline under load.
 *   - ENGINE_PAUSED and RECONCILING responses are EXPECTED during the
 *     fault window and are counted as correct behaviour, not errors.
 *   - VU count defaults to 200, not 1000. Fault correctness is structural
 *     (single-writer Lua atomicity), not statistical, so 200 concurrent
 *     requests is sufficient to expose the race conditions that matter.
 *
 * Env vars (all optional):
 *   BASE_URL       default http://127.0.0.1:18080
 *   AUCTION_ID     default auc_live
 *   VUS            default 200
 *   DURATION       default 45s
 *   SLEEP_MS       inter-iteration pause in ms, default 50
 */

import { check, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import http from 'k6/http';

const BASE_URL   = __ENV.BASE_URL   || 'http://127.0.0.1:18080';
const AUCTION_ID = __ENV.AUCTION_ID || 'auc_live';
const SLEEP_S    = Number(__ENV.SLEEP_MS || 50) / 1000;

export const options = {
  scenarios: {
    concurrent_fault_bids: {
      executor:     'constant-vus',
      vus:          Number(__ENV.VUS      || 200),
      duration:     __ENV.DURATION        || '45s',
      gracefulStop: __ENV.GRACEFUL_STOP   || '5s',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
  // Thresholds intentionally loose: latency degrades during fault window by design.
  // The correctness invariants are proven by the post-run verifier, not by k6 thresholds.
  thresholds: {
    // Hard stop only for admission contamination — those must never appear.
    bid_admission_contamination: ['count==0'],
  },
};

// --- metrics ---
const decidedTotal      = new Counter('bid_decided_total');       // DECIDED (normal path)
const pausedTotal       = new Counter('bid_paused_total');        // ENGINE_PAUSED (expected during fault)
const reconcilingTotal  = new Counter('bid_reconciling_total');   // RECONCILING (expected post-fault)
const rejectedTotal     = new Counter('bid_rejected_total');      // normal BID_TOO_LOW etc.
const httpErrorTotal    = new Counter('bid_http_error_total');    // non-200/503/409 — unexpected
const admissionContam   = new Counter('bid_admission_contamination'); // RATE_LIMITED — should be 0

// accepted rate tracks what fraction of DECIDED responses are accepts
const acceptedRate = new Rate('bid_accepted_rate');

// --- helpers ---
function mockHeaders(userID) {
  return {
    'Content-Type':   'application/json',
    'X-Mock-Role':    'user',
    'X-Mock-User-Id': userID,
  };
}

function getSnapshot() {
  const res = http.get(`${BASE_URL}/api/auctions/${AUCTION_ID}`, {
    headers: mockHeaders(`l1c_snap_${__VU}`),
    tags:    { name: 'snapshot' },
  });
  if (res.status !== 200) return null;
  try { return res.json(); } catch (_) { return null; }
}

function placeBid(amountCents, userID, clientBidID) {
  return http.post(
    `${BASE_URL}/api/auctions/${AUCTION_ID}/bids`,
    JSON.stringify({ client_bid_id: clientBidID, amount_cents: amountCents, client_seen_seq: 0 }),
    {
      headers: { ...mockHeaders(userID), 'Idempotency-Key': clientBidID },
      tags:    { name: 'place_bid' },
    },
  );
}

// --- main VU loop ---
export default function () {
  const userID      = `l1c_bidder_${__VU}`;
  // Each iteration gets a unique bid ID — idempotency key is per-attempt.
  const clientBidID = `l1c-${__VU}-${__ITER}-${Date.now()}`;

  // Snapshot to derive a valid bid amount.
  const snap      = getSnapshot();
  const current   = snap ? Number(snap.current_price_cents || 0) : 0;
  const increment = snap ? Number(snap.increment_cents || 5000)  : 5000;
  // Each VU bids a slightly different multiple to create realistic contention.
  const amount = current + increment * ((__VU % 4) + 1);

  const res = placeBid(amount, userID, clientBidID);

  // --- classify response ---
  const status = res.status;
  let body;
  try { body = res.json(); } catch (_) { body = {}; }

  const result          = String(body.result          || '');
  const code            = String(body.code            || '');
  const decisionStatus  = String(body.decision_status || '');

  // Admission contamination — must never happen with ADMISSION_ENABLED=false.
  if (status === 429 || result === 'RATE_LIMITED' || code === 'RATE_LIMITED' ||
      result === 'BID_AUCTION_TOO_HOT' || code === 'BID_AUCTION_TOO_HOT') {
    admissionContam.add(1);
    check(res, { 'no admission contamination': () => false });
    sleep(SLEEP_S);
    return;
  }

  // ENGINE_PAUSED / RECONCILING — expected during fault window.
  if (result === 'ENGINE_PAUSED' || code === 'ENGINE_PAUSED' ||
      body.engine_paused === true) {
    pausedTotal.add(1);
    // Do NOT mark as check failure — this is correct fail-closed behaviour.
    sleep(SLEEP_S);
    return;
  }
  if (result === 'RECONCILING' || code === 'RECONCILING' ||
      decisionStatus === 'RECONCILING') {
    reconcilingTotal.add(1);
    sleep(SLEEP_S);
    return;
  }

  // DECIDED path — normal operation before/after fault window.
  if (status === 200 && decisionStatus === 'DECIDED') {
    decidedTotal.add(1);
    const isAccepted = result === 'ENGINE_ACCEPTED' || result === 'ENGINE_SOLD';
    acceptedRate.add(isAccepted ? 1 : 0);
    rejectedTotal.add(isAccepted ? 0 : 1);
    check(res, {
      'decided has engine_seq':        () => Number(body.engine_seq || 0) > 0,
      'decided has durability_status': () => typeof body.durability_status === 'string' &&
                                             body.durability_status.length > 0,
    });
    sleep(SLEEP_S);
    return;
  }

  // Unexpected response (non-200, non-paused, non-decided).
  httpErrorTotal.add(1);
  check(res, { 'unexpected http response': () => false });
  sleep(SLEEP_S);
}
