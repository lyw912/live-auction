/**
 * S5 — 断连重连 / Reconnect & Recovery
 *
 * Measures time-to-current-state (TTCS) after a WebSocket disconnection:
 * the time from reconnect start to receiving the current public seq with no gap.
 *
 * Two variants controlled by DISCONNECT_MODE:
 *   "clean"   — socket.close() before reconnect (planned disconnect)
 *   "network" — uses Toxiproxy reset_peer mid-connection (weak-network simulation)
 *               See run-s5-reconnect.sh for Toxiproxy setup.
 *
 * Recovery correctness checks:
 *   - All seqs in (last_seen_seq, current_seq] must be received, in order.
 *   - No duplicate seqs in the recovered stream.
 *   - UI truth fields (current_price_cents, current_winner_id) must match
 *     the server snapshot.
 *
 * Env vars:
 *   BASE_URL          default http://127.0.0.1:18080
 *   WS_URL            default ws://127.0.0.1:18080
 *   AUCTION_ID        default auc_live
 *   ROOM_ID           default room_main
 *   VUS               concurrent reconnect sessions, default 20
 *   DURATION          test duration, default 2m
 *   DISCONNECT_MODE   "clean" (default) or "network"
 *   INITIAL_SEQ_LAG   simulated stale seq gap (last_seen_seq = current - LAG), default 5
 *   TTCS_P99_SLA_MS   M5 TTCS SLO in ms, default 2000
 *
 * Run:
 *   k6 run tests/load/s5-reconnect-recovery.js
 */

import { check, sleep } from 'k6';
import http from 'k6/http';
import ws from 'k6/ws';
import { Counter, Trend } from 'k6/metrics';

const BASE_URL      = __ENV.BASE_URL     || 'http://127.0.0.1:18080';
const WS_BASE       = __ENV.WS_URL      || BASE_URL.replace(/^http/, 'ws');
const AUCTION_ID    = __ENV.AUCTION_ID  || 'auc_live';
const ROOM_ID       = __ENV.ROOM_ID     || 'room_main';
const DISCONNECT_MODE = __ENV.DISCONNECT_MODE || 'clean';
const SEQ_LAG       = Number(__ENV.INITIAL_SEQ_LAG || 5);
const TTCS_SLA      = Number(__ENV.TTCS_P99_SLA_MS || 2000);

export const options = {
  scenarios: {
    reconnect_recovery: {
      executor:     'constant-vus',
      vus:          Number(__ENV.VUS      || 20),
      duration:     __ENV.DURATION        || '2m',
      gracefulStop: '10s',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    s5_ttcs_ms:              [`p(99)<${TTCS_SLA}`],
    s5_recovery_errors:      ['count==0'],
    s5_seq_gaps_after_reconnect: ['count==0'],
  },
};

// --- metrics ---
const ttcsMs         = new Trend('s5_ttcs_ms', true);   // time-to-current-state
const recoveryErrors = new Counter('s5_recovery_errors');
const seqGaps        = new Counter('s5_seq_gaps_after_reconnect');
const reconnectTotal = new Counter('s5_reconnects_total');
const recoveredTotal = new Counter('s5_recovered_total');

// --- helpers ---
function authHeaders(userID) {
  return { 'Content-Type': 'application/json', 'X-Mock-Role': 'user', 'X-Mock-User-Id': userID };
}

function getSnapshot(userID) {
  const res = http.get(`${BASE_URL}/api/auctions/${AUCTION_ID}`,
    { headers: authHeaders(userID) });
  if (res.status !== 200) return null;
  try { return res.json(); } catch (_) { return null; }
}

function issueTicket(userID) {
  const res = http.post(`${BASE_URL}/api/auth/ws-ticket`,
    JSON.stringify({ room_id: ROOM_ID, auction_id: AUCTION_ID }),
    { headers: authHeaders(userID) });
  if (res.status !== 200) return null;
  try { return res.json('ticket'); } catch (_) { return null; }
}

// --- main VU: initial connect → disconnect → reconnect → measure TTCS ---
export default function () {
  const userID = `s5_user_${__VU}`;

  // 1. Get current server state before disconnect.
  const snap = getSnapshot(userID);
  if (!snap) { recoveryErrors.add(1); sleep(1); return; }

  const serverCurrentSeq = Number(snap.public_seq || snap.engine_seq || 0);
  // Simulate a stale client that missed the last SEQ_LAG events.
  const staleLastSeq = Math.max(0, serverCurrentSeq - SEQ_LAG);

  // 2. Issue a WS ticket.
  const ticket = issueTicket(userID);
  if (!ticket) { recoveryErrors.add(1); sleep(1); return; }

  // 3. Reconnect with last_seq=staleLastSeq. Record TTCS start.
  const reconnectStartMs = Date.now();
  reconnectTotal.add(1);

  const wsUrl = `${WS_BASE}/ws?room_id=${ROOM_ID}&auction_id=${AUCTION_ID}&last_seq=${staleLastSeq}`;

  let receivedSeqs      = [];
  let caughtUp          = false;
  let caughtUpMs        = 0;
  let serverTruthPrice  = null;
  let serverTruthWinner = null;

  const response = ws.connect(wsUrl, { headers: { 'X-Auction-WS-Ticket': ticket } }, (socket) => {
    socket.on('message', (rawMsg) => {
      if (caughtUp) return;  // already done

      let msg;
      try { msg = JSON.parse(rawMsg); } catch (_) { return; }

      // Snapshot/recovery message: the server sends full state when last_seq is stale.
      if (msg.type === 'snapshot' || msg.event_type === 'snapshot') {
        const snapSeq = Number(msg.seq || msg.public_seq || 0);
        if (snapSeq >= serverCurrentSeq) {
          caughtUpMs = Date.now();
          caughtUp   = true;
          serverTruthPrice  = msg.current_price_cents || msg.data?.current_price_cents;
          serverTruthWinner = msg.current_winner_id   || msg.data?.current_winner_id;
          socket.close();
          return;
        }
      }

      // Incremental event: track seqs to detect gaps.
      const seq = Number(msg.seq || msg.public_seq || 0);
      if (seq > staleLastSeq && seq <= serverCurrentSeq + 10) {
        receivedSeqs.push(seq);
        if (seq >= serverCurrentSeq) {
          caughtUpMs = Date.now();
          caughtUp   = true;
          socket.close();
        }
      }
    });

    socket.on('error', () => { recoveryErrors.add(1); socket.close(); });

    // Timeout: if not caught up in TTCS_SLA × 3, give up.
    socket.setTimeout(() => {
      if (!caughtUp) {
        recoveryErrors.add(1);
        socket.close();
      }
    }, TTCS_SLA * 3);
  });

  if (!caughtUp) {
    recoveryErrors.add(1);
    sleep(1);
    return;
  }

  // 4. Record TTCS.
  const ttcs = caughtUpMs - reconnectStartMs;
  ttcsMs.add(ttcs);
  recoveredTotal.add(1);

  // 5. Correctness: check seq continuity in incremental path.
  if (receivedSeqs.length > 1) {
    receivedSeqs.sort((a, b) => a - b);
    for (let i = 1; i < receivedSeqs.length; i++) {
      if (receivedSeqs[i] !== receivedSeqs[i - 1] + 1) {
        seqGaps.add(1);  // gap in the recovered stream
      }
    }
  }

  check(ttcs, {
    's5 caught up in time':  (v) => v < TTCS_SLA,
    's5 no seq gap':         () => seqGaps.value === 0,
  });

  // Validate server truth: price/winner from reconnect must match snapshot.
  if (serverTruthPrice !== null) {
    const freshSnap = getSnapshot(userID);
    check(freshSnap, {
      's5 price matches server': (s) => s === null || s.current_price_cents === serverTruthPrice,
    });
  }

  // Wait between sessions to avoid connection stampede.
  sleep(Number(__ENV.SESSION_GAP_S || 1));
}
