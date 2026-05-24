import { check, sleep } from 'k6';
import http from 'k6/http';
import { Counter, Rate, Trend } from 'k6/metrics';
import { BASE_URL, ROOM_ID, AUCTION_ID, userHeaders, openAuctionSocket } from './lib/live-auction.js';

const connections = Number(__ENV.CONNECTIONS || 300);
const duration = __ENV.DURATION || '30s';
const sessionSeconds = Number(__ENV.SESSION_SECONDS || 8);
const connectStaggerMs = Number(__ENV.CONNECT_STAGGER_MS || 0);
const ticketRetries = Number(__ENV.TICKET_RETRIES || 3);
const retrySleepMs = Number(__ENV.RETRY_SLEEP_MS || 250);
const probeConnectRejects = __ENV.PROBE_CONNECT_REJECTS === '1';
const connectRejectProbes = Number(__ENV.CONNECT_REJECT_PROBES || 20);
const rejectProbeStartDelay = __ENV.REJECT_PROBE_START_DELAY || '3s';

export const options = {
  scenarios: {
    storm: {
      executor: 'constant-vus',
      vus: connections,
      duration,
      gracefulStop: __ENV.GRACEFUL_STOP || '10s',
      exec: 'connectOnce',
    },
    ...(probeConnectRejects ? {
      connect_reject_probe: {
        executor: 'constant-vus',
        vus: connectRejectProbes,
        duration: __ENV.REJECT_PROBE_DURATION || '5s',
        startTime: rejectProbeStartDelay,
        gracefulStop: __ENV.GRACEFUL_STOP || '10s',
        exec: 'probeConnectReject',
      },
    } : {}),
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)'],
  thresholds: {
    auction_k6_ws_storm_uncontrolled_failure_rate: ['rate<0.01'],
    auction_k6_ws_storm_opened_total: ['count>0'],
    ...(probeConnectRejects ? {
      auction_k6_ws_storm_connect_rejected_total: ['count>0'],
    } : {}),
  },
};

const opened = new Counter('auction_k6_ws_storm_opened_total');
const retryLater = new Counter('auction_k6_ws_storm_retry_later_total');
const connectRejected = new Counter('auction_k6_ws_storm_connect_rejected_total');
const ticketOK = new Counter('auction_k6_ws_storm_ticket_ok_total');
const wsErrors = new Counter('auction_k6_ws_storm_ws_errors_total');
const uncontrolledFailureRate = new Rate('auction_k6_ws_storm_uncontrolled_failure_rate');
const sessionMs = new Trend('auction_k6_ws_storm_session_ms');

function issueTicketWithRetry(userID) {
  for (let attempt = 0; attempt <= ticketRetries; attempt += 1) {
    const res = http.post(
      `${BASE_URL}/api/auth/ws-ticket`,
      JSON.stringify({ room_id: ROOM_ID, auction_id: AUCTION_ID }),
      {
        headers: userHeaders(userID),
        tags: { name: 'issue_ws_ticket_storm' },
      },
    );
    if (res.status === 200) {
      ticketOK.add(1);
      uncontrolledFailureRate.add(false);
      return res.json('ticket');
    }
    if (res.status === 429) {
      retryLater.add(1);
      uncontrolledFailureRate.add(false);
      sleep(retrySleepMs / 1000);
      continue;
    }
    uncontrolledFailureRate.add(true);
    return '';
  }
  return '';
}

export function connectOnce() {
  if (connectStaggerMs > 0) {
    sleep(((__VU - 1) % connections) * connectStaggerMs / 1000);
  }
  const started = Date.now();
  const userID = `k6_ws_${((__VU - 1) % 512) + 1}`;
  const ticket = issueTicketWithRetry(userID);
  const gotTicket = check(ticket, { 'storm ticket eventually issued or controlled retry': (t) => Boolean(t) });
  if (!gotTicket) {
    return;
  }
  const ws = openAuctionSocket(ticket, 0, {
    open() {
      opened.add(1);
      check(true, { 'storm watcher opened': (v) => v === true });
    },
    error() {
      wsErrors.add(1);
      uncontrolledFailureRate.add(!probeConnectRejects);
    },
    close() {
      sessionMs.add(Date.now() - started);
    },
  });
  sleep(sessionSeconds);
  if (ws.readyState === 1) {
    ws.close();
  }
}

export function probeConnectReject() {
  const res = http.get(`${BASE_URL}/ws?room_id=${ROOM_ID}&auction_id=${AUCTION_ID}&last_seq=0`, {
    headers: { 'X-Auction-WS-Ticket': 'probe' },
    tags: { name: 'probe_ws_connect_admission' },
  });
  const controlled = check(res, {
    'connect admission returned retry later': (r) => r.status === 429 && Boolean(r.headers['Retry-After']),
  });
  if (controlled) {
    connectRejected.add(1);
    uncontrolledFailureRate.add(false);
  } else {
    uncontrolledFailureRate.add(true);
  }
  sleep(1);
}
