/**
 * S3 — 万人围观 / Room Fanout Soak (local k6, 0 VUM)
 *
 * Holds N WebSocket connections in one room while a low-rate bid source
 * generates accepted updates (fanout events). Measures:
 *   M2: publish→receive p99 using server-embedded published_at_ms
 *   M4: connection stability (goroutines/fd/RAM watched via Grafana)
 *
 * Design:
 *   - Two k6 scenarios run together: `viewers` (WS hold) + `bidders` (bid source).
 *   - Each viewer tracks the highest seq received and the worst fanout latency.
 *   - Clock skew assumption: run against same-box server (loopback) or same-VPC
 *     PTS IPs with NTP discipline. published_at_ms is the server clock; k6
 *     Date.now() is the load-generator clock. Same-region residual skew ≪ 1s.
 *   - Heartbeat: uses k6/websockets so server ping/pong is handled by the
 *     standard WebSocket event loop during long holds.
 *   - Connection hold: WS connections stay open for HOLD_SECONDS; the viewer
 *     closes cleanly after that to avoid leaving fd/goroutine leaks.
 *
 * Env vars:
 *   BASE_URL          default http://127.0.0.1:18080
 *   WS_URL            default ws://127.0.0.1:18080
 *   AUCTION_ID        default auc_live
 *   ROOM_ID           default room_main
 *   VIEWER_VUS        WS connections to hold, default 1000 (use 10000 for headline)
 *   BIDDER_VUS        simultaneous bid sources, default 3
 *   BID_RATE_PER_S    accepted bid arrivals/s from bidders, default 5
 *   BID_BASE_AMOUNT   starting amount_cents for accepted-update source, default 1000000000
 *   HOLD_SECONDS      how long each viewer holds its connection, default 300
 *   FANOUT_P99_SLA_MS M2 SLO in ms, default 1000
 *   RUN_ID            idempotency key namespace, default Date.now() at init
 *   VIEWER_CSV        viewer session CSV, default docs/perf/pts/pts-l2-viewer-10000-sessions.csv
 *   BIDDER_CSV        bidder session CSV, default docs/perf/pts/pts-l2-bidder-1000-sessions.csv
 *
 * Local soak (10k connections — ensure ulimit -n and fs.nr_open are raised):
 *   k6 run --env VIEWER_VUS=10000 --env HOLD_SECONDS=600 tests/load/s3-fanout-soak.js
 *
 * Quick smoke (verify the script works):
 *   k6 run --env VIEWER_VUS=10 --env HOLD_SECONDS=30 tests/load/s3-fanout-soak.js
 */

import { check, sleep } from 'k6';
import http from 'k6/http';
import { WebSocket } from 'k6/websockets';
import exec from 'k6/execution';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE_URL     = __ENV.BASE_URL    || 'http://127.0.0.1:18080';
const WS_BASE      = __ENV.WS_URL     || BASE_URL.replace(/^http/, 'ws');
const AUCTION_ID   = __ENV.AUCTION_ID || 'auc_live';
const ROOM_ID      = __ENV.ROOM_ID    || 'room_main';
const VIEWER_VUS   = Number(__ENV.VIEWER_VUS    || 1000);
const BIDDER_VUS   = Number(__ENV.BIDDER_VUS    || 3);
const BID_RATE     = Number(__ENV.BID_RATE_PER_S|| 5);
const BID_BASE_AMOUNT = Number(__ENV.BID_BASE_AMOUNT || 1000000000);
const HOLD_SECONDS = Number(__ENV.HOLD_SECONDS  || 300);
const FANOUT_SLA   = Number(__ENV.FANOUT_P99_SLA_MS || 1000);
const RUN_ID       = __ENV.RUN_ID || String(Date.now());
const VIEWER_CSV   = __ENV.VIEWER_CSV || '../../docs/perf/pts/pts-l2-viewer-10000-sessions.csv';
const BIDDER_CSV   = __ENV.BIDDER_CSV || '../../docs/perf/pts/pts-l2-bidder-1000-sessions.csv';

function parseSessionCSV(path) {
  const text = open(path);
  return text.trim().split('\n').slice(1).filter(Boolean).map((line) => {
    const [userID, token] = line.split(',');
    return { userID, token };
  });
}

const viewerSessions = parseSessionCSV(VIEWER_CSV);
const bidderSessions = parseSessionCSV(BIDDER_CSV);

export const options = {
  scenarios: {
    // WS viewers: one iteration per VU, each holding one connection.
    viewers: {
      executor:        'shared-iterations',
      vus:             VIEWER_VUS,
      iterations:      VIEWER_VUS,
      maxDuration:     `${HOLD_SECONDS + 30}s`,
      gracefulStop:    '10s',
      exec:            'viewerFn',
    },
    // Bid source: generates accepted updates at BID_RATE/s to drive fanout
    // Open model so bid arrivals are independent of WS latency.
    bidders: {
      executor:    'constant-arrival-rate',
      rate:        BID_RATE,
      timeUnit:    '1s',
      duration:    `${HOLD_SECONDS}s`,
      preAllocatedVUs: BIDDER_VUS,
      maxVUs:          BIDDER_VUS * 3,
      exec:        'bidderFn',
      startTime:   '5s',  // let viewers connect first
    },
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    http_req_failed: ['rate<0.01'],
    // M2: fanout p99 ≤ FANOUT_SLA_MS
    s3_fanout_latency_ms: [`p(99)<${FANOUT_SLA}`],
    s3_fanout_samples: ['count>0'],
    // Connection stability: nearly all viewers must successfully receive at least one message
    s3_viewer_received_session: ['rate>0.90'],
    s3_viewer_errors: ['count==0'],
    s3_bid_accepted_updates: ['count>0'],
  },
};

// --- metrics ---
const fanoutLatency   = new Trend('s3_fanout_latency_ms', true);  // M2
const fanoutSamples   = new Counter('s3_fanout_samples');
const viewerMsgRate   = new Counter('s3_viewer_messages_received');
const viewerSessionOK = new Rate('s3_viewer_received_session');
const viewerConnected = new Counter('s3_viewer_connected');
const viewerErrors    = new Counter('s3_viewer_errors');
const bidDecisions    = new Counter('s3_bid_decisions');
const bidAccepted     = new Counter('s3_bid_accepted_updates');

// --- helper: issue WS ticket ---
function issueTicket(userID) {
  const session = viewerSessions[(__VU - 1) % viewerSessions.length];
  const res = http.post(
    `${BASE_URL}/api/auth/ws-ticket`,
    JSON.stringify({ room_id: ROOM_ID, auction_id: AUCTION_ID }),
    { headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${session.token}` } },
  );
  if (res.status !== 200) return null;
  try { return res.json('ticket'); } catch (_) { return null; }
}

// --- viewer VU: connect, hold, measure fanout latency ---
export function viewerFn() {
  const userID = viewerSessions[(__VU - 1) % viewerSessions.length].userID;
  const ticket = issueTicket(userID);
  if (!ticket) {
    viewerErrors.add(1);
    sleep(5);
    return;
  }

  const wsUrl = `${WS_BASE}/ws?room_id=${ROOM_ID}&auction_id=${AUCTION_ID}&last_seq=0`;
  let receivedCount = 0;
  let connectedAtMs = 0;

  const socket = new WebSocket(wsUrl, undefined, {
    headers: { 'X-Auction-WS-Ticket': ticket },
  });

  let intendedClose = false;
  socket.addEventListener('open', () => {
    connectedAtMs = Date.now();
    viewerConnected.add(1);
    setTimeout(() => {
      intendedClose = true;
      socket.close();
    }, HOLD_SECONDS * 1000);
  });

  socket.addEventListener('message', (event) => {
      // M2 measurement: server embeds published_at_ms in every broadcast envelope.
      // fanout_latency = client_receive_ms − published_at_ms
      // Clock assumption: same-region NTP-disciplined (residual skew ≪ 1s target).
      const recvMs = Date.now();
      let msg;
      try { msg = JSON.parse(event.data); } catch (_) { return; }

      // Only price/event updates carry published_at_ms (not heartbeats/pings).
      const publishedAtMs = msg.published_at_ms || msg.data?.published_at_ms;
      // Recovery/history messages can also carry published_at_ms. They answer
      // reconnect correctness, not M2 live fanout, so only measure messages
      // published after this viewer's connection opened.
      if (publishedAtMs && publishedAtMs >= connectedAtMs) {
        const latMs = recvMs - publishedAtMs;
        if (latMs >= 0 && latMs < 60000) {  // sanity: ignore clock skew anomalies > 60s
          fanoutLatency.add(latMs);
          fanoutSamples.add(1);
        }
      }

      viewerMsgRate.add(1);
      receivedCount++;
  });

  socket.addEventListener('error', () => {
    if (!intendedClose) {
      viewerErrors.add(1);
    }
  });

  socket.addEventListener('close', (event) => {
    const normalClose = intendedClose || event.code === 1000 || event.code === undefined;
    if (!normalClose) {
      viewerErrors.add(1);
    }
    check(null, {
      's3 viewer received at least one message': () => receivedCount > 0,
      's3 viewer closed normally': () => normalClose,
    });
    viewerSessionOK.add(receivedCount > 0);
  });

}

// --- bidder VU: place bids to generate accepted updates (fanout source) ---
export function bidderFn() {
  const globalIter  = exec.scenario.iterationInTest;
  const session     = bidderSessions[globalIter % bidderSessions.length];
  const userID      = session.userID;
  const clientBidID = `s3-${RUN_ID}-${globalIter}`;
  // Use an escalating amount so bids are periodically accepted and fan out.
  // Bidders start well above any initial price so most are accepted (they are the fanout source).
  const amount = BID_BASE_AMOUNT + globalIter * 5000;

  const res = http.post(
    `${BASE_URL}/api/auctions/${AUCTION_ID}/bids`,
    JSON.stringify({ client_bid_id: clientBidID, idempotency_key: clientBidID,
                     amount_cents: amount, client_seen_seq: 0 }),
    { headers: { 'Content-Type': 'application/json',
                 'Authorization': `Bearer ${session.token}`,
                 'Idempotency-Key': clientBidID } },
  );

  try {
    const body = res.json();
    const ds   = String(body.decision_status || '');
    if (res.status === 200 && ds === 'DECIDED') bidDecisions.add(1);
    if (res.status === 200 && String(body.result || '') === 'ENGINE_ACCEPTED') bidAccepted.add(1);
  } catch (_) {}
}
