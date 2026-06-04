/**
 * S5 — 断连重连 / Reconnect & Recovery
 *
 * Measures time-to-current-state (TTCS) after a client really disconnects:
 * initial WS connect -> close -> miss real public seqs -> reconnect with last_seq.
 *
 * This uses k6/ws because its connect() call blocks until the socket closes,
 * which makes the sequential recovery leg deterministic for this harness.
 *
 * Env vars:
 *   BASE_URL              default http://127.0.0.1:18080
 *   WS_URL                default ws://127.0.0.1:18080
 *   INITIAL_WS_URL        default clean BASE_URL WS in network mode, otherwise WS_URL
 *   AUCTION_ID            default auc_live
 *   ROOM_ID               default room_main
 *   SESSION_CSV           default ../../docs/perf/pts/inputs/s1-s5/s1-s5-1000-user-sessions.csv
 *   VUS                   concurrent reconnect sessions, default 20
 *   DURATION              reconnect test duration, default 2m
 *   MISSED_EVENTS         minimum seqs to miss before reconnect, default 3
 *   MAX_WAIT_MISSED_MS    max wait for missed events, default 4000
 *   TTCS_P99_SLA_MS       TTCS SLO in ms, default 2000
 *   BID_RATE_PER_S        accepted update source rate, default 10
 *   BID_SOURCE_VUS        update source VUs, default 5
 *   BID_BASE_AMOUNT       starting amount_cents for update source, default 5000000000
 *   RECONNECT_ATTEMPTS    attempts for the stale-last_seq recovery leg, default 1 clean / 3 network
 *   INITIAL_CONNECT_ATTEMPTS attempts for the initial online leg, default 1 clean / same as reconnect
 *   RECONNECT_BACKOFF_MS  bounded backoff between reconnect attempts, default 100
 */

import { check, sleep } from 'k6';
import http from 'k6/http';
import ws from 'k6/ws';
import exec from 'k6/execution';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:18080';
const WS_BASE = __ENV.WS_URL || BASE_URL.replace(/^http/, 'ws');
const AUCTION_ID = __ENV.AUCTION_ID || 'auc_live';
const ROOM_ID = __ENV.ROOM_ID || 'room_main';
const SESSION_CSV = __ENV.SESSION_CSV || '../../docs/perf/pts/inputs/s1-s5/s1-s5-1000-user-sessions.csv';
const RECONNECT_VUS = Number(__ENV.VUS || 20);
const DURATION = __ENV.DURATION || '2m';
const MISSED_EVENTS = Number(__ENV.MISSED_EVENTS || 3);
const MAX_WAIT_MISSED_MS = Number(__ENV.MAX_WAIT_MISSED_MS || 4000);
const TTCS_SLA = Number(__ENV.TTCS_P99_SLA_MS || 2000);
const BID_RATE = Number(__ENV.BID_RATE_PER_S || 10);
const BID_SOURCE_VUS = Number(__ENV.BID_SOURCE_VUS || 5);
const BID_BASE_AMOUNT = Number(__ENV.BID_BASE_AMOUNT || 5000000000);
const DISCONNECT_MODE = __ENV.DISCONNECT_MODE || 'clean';
const INITIAL_WS_BASE = __ENV.INITIAL_WS_URL || (DISCONNECT_MODE === 'network' ? BASE_URL.replace(/^http/, 'ws') : WS_BASE);
const RECONNECT_ATTEMPTS = Number(__ENV.RECONNECT_ATTEMPTS || (DISCONNECT_MODE === 'network' ? 8 : 1));
const INITIAL_CONNECT_ATTEMPTS = Number(__ENV.INITIAL_CONNECT_ATTEMPTS || (DISCONNECT_MODE === 'network' ? RECONNECT_ATTEMPTS : 1));
const RECONNECT_BACKOFF_MS = Number(__ENV.RECONNECT_BACKOFF_MS || 100);
const RUN_ID = __ENV.RUN_ID || String(Date.now());

function parseSessionCSV(path) {
  const text = open(path);
  return text.trim().split('\n').slice(1).filter(Boolean).map((line) => {
    const [userID, token, role] = line.split(',');
    return { userID, token, role };
  });
}

const sessions = parseSessionCSV(SESSION_CSV);

function validateSessions() {
  const required = Math.max(RECONNECT_VUS, BID_SOURCE_VUS + 700);
  if (sessions.length < required) {
    throw new Error(`SESSION_CSV ${SESSION_CSV} has ${sessions.length} usable sessions, need at least ${required}`);
  }
  const invalid = sessions.find((session) => !session.userID || !session.token || !session.role);
  if (invalid) {
    throw new Error(`SESSION_CSV ${SESSION_CSV} contains an invalid session row`);
  }
}

validateSessions();

export const options = {
  scenarios: {
    bid_source: {
      executor: 'constant-arrival-rate',
      rate: BID_RATE,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: BID_SOURCE_VUS,
      maxVUs: Math.max(BID_SOURCE_VUS * 3, 10),
      exec: 'bidSourceFn',
    },
    reconnect_recovery: {
      executor: 'constant-vus',
      vus: RECONNECT_VUS,
      duration: DURATION,
      gracefulStop: '10s',
      startTime: '3s',
      exec: 'reconnectFn',
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    s5_ttcs_ms: [`p(99)<${TTCS_SLA}`],
    s5_recovery_errors: ['count==0'],
    s5_seq_gaps_after_reconnect: ['count==0'],
    s5_duplicate_seq_after_reconnect: ['count==0'],
    s5_truth_mismatch: ['count==0'],
    s5_missed_window_ready: ['rate>0.95'],
    s5_recovered_total: ['count>0'],
  },
};

const ttcsMs = new Trend('s5_ttcs_ms', true);
const missedWaitMs = new Trend('s5_missed_wait_ms', true);
const recoveryErrors = new Counter('s5_recovery_errors');
const initialAttemptErrors = new Counter('s5_initial_attempt_errors_total');
const initialRetries = new Counter('s5_initial_retries_total');
const reconnectAttemptErrors = new Counter('s5_reconnect_attempt_errors_total');
const reconnectRetries = new Counter('s5_reconnect_retries_total');
const skippedNoMissedWindow = new Counter('s5_skipped_no_missed_window_total');
const seqGaps = new Counter('s5_seq_gaps_after_reconnect');
const duplicateSeqs = new Counter('s5_duplicate_seq_after_reconnect');
const truthMismatch = new Counter('s5_truth_mismatch');
const reconnectTotal = new Counter('s5_reconnects_total');
const recoveredTotal = new Counter('s5_recovered_total');
const initialConnected = new Counter('s5_initial_connected_total');
const missedReady = new Rate('s5_missed_window_ready');
const bidAccepted = new Counter('s5_bid_source_accepted_total');
const bidFailures = new Counter('s5_bid_source_failures_total');

function sessionFor(index) {
  return sessions[index % sessions.length];
}

function authHeaders(session) {
  return {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${session.token}`,
  };
}

function parseJSONResponse(res) {
  try { return res.json(); } catch (_) { return null; }
}

function getSnapshot(session) {
  const res = http.get(`${BASE_URL}/api/auctions/${AUCTION_ID}`, {
    headers: authHeaders(session),
    tags: { name: 's5_get_auction_snapshot' },
  });
  if (res.status !== 200) return null;
  return parseJSONResponse(res);
}

function issueTicket(session) {
  const res = http.post(
    `${BASE_URL}/api/auth/ws-ticket`,
    JSON.stringify({ room_id: ROOM_ID, auction_id: AUCTION_ID }),
    {
      headers: authHeaders(session),
      tags: { name: 's5_issue_ws_ticket' },
    },
  );
  if (res.status !== 200) return null;
  try { return res.json('ticket'); } catch (_) { return null; }
}

function messageSeq(msg) {
  return Number(msg.seq || msg.public_seq || msg.data?.seq || msg.payload?.seq || 0);
}

function messagePrice(msg) {
  return msg.current_price_cents ?? msg.data?.current_price_cents ?? msg.payload?.current_price_cents ?? null;
}

function messageWinner(msg) {
  return msg.current_winner_id ?? msg.data?.current_winner_id ?? msg.payload?.current_winner_id ?? null;
}

function wsURL(lastSeq, phase) {
  const base = phase === 'initial-close' ? INITIAL_WS_BASE : WS_BASE;
  return `${base}/ws?room_id=${encodeURIComponent(ROOM_ID)}&auction_id=${encodeURIComponent(AUCTION_ID)}&last_seq=${lastSeq}`;
}

function connectUntilSeq(session, lastSeq, targetSeq, timeoutMs, phase) {
  const ticket = issueTicket(session);
  if (!ticket) {
    return { ok: false, maxSeq: 0, source: '', price: null, winner: null, seqs: [] };
  }

  let maxSeq = 0;
  let source = '';
  let price = null;
  let winner = null;
  const seqs = [];
  let ok = false;

  ws.connect(wsURL(lastSeq, phase), { headers: { 'X-Auction-WS-Ticket': ticket } }, (socket) => {
    socket.on('message', (rawMsg) => {
      let msg;
      try { msg = JSON.parse(rawMsg); } catch (_) { return; }

      const seq = messageSeq(msg);
      if (seq > 0) {
        maxSeq = Math.max(maxSeq, seq);
        if (seq > lastSeq) seqs.push(seq);
      }
      if (msg.event_type === 'snapshot' || msg.type === 'snapshot') {
        source = msg.source || msg.data?.source || msg.payload?.source || 'snapshot';
        price = messagePrice(msg);
        winner = messageWinner(msg);
      }
      if (targetSeq <= 0 || maxSeq >= targetSeq || source) {
        ok = true;
        socket.close();
      }
    });

    socket.on('error', () => {
      socket.close();
    });

    socket.setTimeout(() => {
      socket.close();
    }, timeoutMs);
  });

  return { ok, maxSeq, source, price, winner, seqs };
}

function connectWithRetry(session, lastSeq, targetSeq, timeoutMs, attempts, phase) {
  let best = { ok: false, maxSeq: 0, source: '', price: null, winner: null, seqs: [] };
  const started = Date.now();
  const maxAttempts = Math.max(1, attempts);
  for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
    const result = connectUntilSeq(session, lastSeq, targetSeq, timeoutMs, phase);
    if (result.maxSeq > best.maxSeq || result.source) {
      best = result;
    }
    if (result.ok && result.maxSeq >= targetSeq) {
      return { ...result, attempts: attempt, ttcs: Date.now() - started };
    }
    if (phase === 'initial-close') {
      initialAttemptErrors.add(1);
    } else {
      reconnectAttemptErrors.add(1);
    }
    if (attempt < maxAttempts) {
      if (phase === 'initial-close') {
        initialRetries.add(1);
      } else {
        reconnectRetries.add(1);
      }
      sleep(RECONNECT_BACKOFF_MS / 1000);
    }
  }
  return { ...best, attempts: maxAttempts, ttcs: Date.now() - started };
}

function recoverWithRetry(session, lastSeq, targetSeq) {
  return connectWithRetry(session, lastSeq, targetSeq, TTCS_SLA * 3, RECONNECT_ATTEMPTS, 'reconnect');
}

function waitForMissedWindow(session, baseSeq) {
  const targetSeq = baseSeq + MISSED_EVENTS;
  const started = Date.now();
  let snap = null;
  while (Date.now() - started < MAX_WAIT_MISSED_MS) {
    snap = getSnapshot(session);
    const seq = Number(snap?.seq || snap?.public_seq || snap?.engine_seq || 0);
    if (seq >= targetSeq) {
      missedWaitMs.add(Date.now() - started);
      missedReady.add(true);
      return { ok: true, targetSeq: seq, snap };
    }
    sleep(0.1);
  }
  missedWaitMs.add(Date.now() - started);
  missedReady.add(false);
  return { ok: false, targetSeq, snap };
}

function countSequenceProblems(seqs) {
  if (seqs.length <= 1) return { gaps: 0, duplicates: 0 };
  const ordered = [...seqs].sort((a, b) => a - b);
  let gaps = 0;
  let duplicates = 0;
  for (let i = 1; i < ordered.length; i += 1) {
    if (ordered[i] === ordered[i - 1]) duplicates += 1;
    if (ordered[i] > ordered[i - 1] + 1) gaps += 1;
  }
  return { gaps, duplicates };
}

export function reconnectFn() {
  const session = sessionFor(__VU - 1);
  const initialSnap = getSnapshot(session);
  if (!initialSnap) {
    recoveryErrors.add(1);
    sleep(1);
    return;
  }

  const initialSeq = Number(initialSnap.seq || initialSnap.public_seq || initialSnap.engine_seq || 0);
  const initial = connectWithRetry(session, initialSeq, initialSeq, 1000, INITIAL_CONNECT_ATTEMPTS, 'initial-close');
  if (!initial.ok) {
    recoveryErrors.add(1);
    sleep(1);
    return;
  }
  initialConnected.add(1);
  const lastSeq = Math.max(initial.maxSeq, initialSeq);

  const missed = waitForMissedWindow(session, lastSeq);
  if (!missed.ok) {
    skippedNoMissedWindow.add(1);
    sleep(0.5);
    return;
  }

  reconnectTotal.add(1);
  const recovered = recoverWithRetry(session, lastSeq, missed.targetSeq);
  if (!recovered.ok || recovered.maxSeq < missed.targetSeq) {
    recoveryErrors.add(1);
    sleep(0.5);
    return;
  }

  const ttcs = recovered.ttcs;
  ttcsMs.add(ttcs);
  recoveredTotal.add(1);

  const problems = countSequenceProblems(recovered.seqs);
  if (problems.gaps > 0) seqGaps.add(problems.gaps);
  if (problems.duplicates > 0) duplicateSeqs.add(problems.duplicates);

  if (recovered.source) {
    const fresh = getSnapshot(session);
    const freshPrice = Number(fresh?.current_price_cents ?? 0);
    const recoveredPrice = Number(recovered.price ?? 0);
    if (recoveredPrice > 0 && freshPrice > 0 && recoveredPrice !== freshPrice) {
      truthMismatch.add(1);
    }
  }

  check(null, {
    's5 initial socket connected before disconnect': () => initial.ok,
    's5 missed real events before reconnect': () => missed.ok,
    's5 caught up in time': () => ttcs < TTCS_SLA,
    's5 no seq gap in recovered stream': () => problems.gaps === 0,
    's5 no duplicate seq in recovered stream': () => problems.duplicates === 0,
  });

  sleep(Number(__ENV.SESSION_GAP_S || 0.2));
}

export function bidSourceFn() {
  const globalIter = exec.scenario.iterationInTest;
  const session = sessionFor(globalIter + 700);
  const snap = getSnapshot(session);
  const current = Number(snap?.current_price_cents || BID_BASE_AMOUNT);
  const increment = Number(snap?.increment_cents || 5000);
  const amount = Math.max(BID_BASE_AMOUNT + globalIter * increment, current + increment);
  const clientBidID = `s5-${RUN_ID}-${globalIter}`;

  const res = http.post(
    `${BASE_URL}/api/auctions/${AUCTION_ID}/bids`,
    JSON.stringify({
      client_bid_id: clientBidID,
      idempotency_key: clientBidID,
      amount_cents: amount,
      client_seen_seq: 0,
    }),
    {
      headers: { ...authHeaders(session), 'Idempotency-Key': clientBidID },
      tags: { name: 's5_bid_source' },
    },
  );

  let body = {};
  try { body = res.json(); } catch (_) { body = {}; }
  if (res.status === 200 && String(body.result || '') === 'ENGINE_ACCEPTED') {
    bidAccepted.add(1);
    return;
  }
  if (res.status !== 200 && res.status !== 202) {
    bidFailures.add(1);
  }
}
