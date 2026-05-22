import http from 'k6/http';
import { check, fail } from 'k6';
import { WebSocket } from 'k6/websockets';

export const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:18080';
export const WS_URL = __ENV.WS_URL || BASE_URL.replace(/^http/, 'ws');
export const ROOM_ID = __ENV.ROOM_ID || 'room_main';
export const AUCTION_ID = __ENV.AUCTION_ID || 'auc_live';

export function userHeaders(userID = `k6_user_${__VU}`) {
  return {
    'Content-Type': 'application/json',
    'X-Mock-Role': 'user',
    'X-Mock-User-Id': userID,
  };
}

export function hostHeaders() {
  return {
    'Content-Type': 'application/json',
    'X-Mock-Role': 'host',
    'X-Mock-User-Id': 'host_1',
  };
}

function randomIntBetween(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

export function getSnapshot() {
  const res = http.get(`${BASE_URL}/api/auctions/${AUCTION_ID}`, {
    headers: userHeaders(),
    tags: { name: 'get_auction_snapshot' },
  });
  check(res, { 'snapshot status 200': (r) => r.status === 200 });
  if (res.status !== 200) {
    fail(`snapshot failed status=${res.status} body=${res.body}`);
  }
  return res.json();
}

export function placeBid(amountCents, userID, keyPrefix = 'k6-bid') {
  const clientBidID = `${keyPrefix}-${__VU}-${__ITER}-${Date.now()}-${randomIntBetween(1, 1000000)}`;
  const body = JSON.stringify({
    client_bid_id: clientBidID,
    amount_cents: amountCents,
    client_seen_seq: 0,
  });
  return http.post(`${BASE_URL}/api/auctions/${AUCTION_ID}/bids`, body, {
    headers: {
      ...userHeaders(userID),
      'Idempotency-Key': clientBidID,
    },
    tags: { name: 'place_bid' },
  });
}

export function issueTicket(userID = `k6_ws_${__VU}`) {
  const res = http.post(
    `${BASE_URL}/api/auth/ws-ticket`,
    JSON.stringify({ room_id: ROOM_ID, auction_id: AUCTION_ID }),
    {
      headers: userHeaders(userID),
      tags: { name: 'issue_ws_ticket' },
    },
  );
  check(res, { 'ticket status 200': (r) => r.status === 200 });
  if (res.status !== 200) {
    fail(`ticket failed status=${res.status} body=${res.body}`);
  }
  return res.json('ticket');
}

export function openAuctionSocket(ticket, lastSeq = 0, handlers = {}) {
  const url = `${WS_URL}/ws?room_id=${ROOM_ID}&auction_id=${AUCTION_ID}&last_seq=${lastSeq}`;
  const ws = new WebSocket(url, ['auction.v1', `ticket.${ticket}`]);
  ws.addEventListener('open', () => {
    if (handlers.open) handlers.open(ws);
  });
  ws.addEventListener('message', (event) => {
    if (handlers.message) handlers.message(event, ws);
  });
  ws.addEventListener('close', () => {
    if (handlers.close) handlers.close(ws);
  });
  ws.addEventListener('error', (event) => {
    if (handlers.error) handlers.error(event, ws);
  });
  return ws;
}
