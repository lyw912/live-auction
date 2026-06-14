export const AUCTION_WS_PROTOCOL = 'auction.v1';

export function wsTicketRequest(roomID: string, auctionID: string, userID: string) {
  return {
    url: '/api/auth/ws-ticket',
    init: {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ room_id: roomID, auction_id: auctionID, user_id: userID })
    }
  };
}

export function auctionWSProtocols(ticket: string) {
  return [AUCTION_WS_PROTOCOL, `ticket.${ticket}`];
}

export function auctionWSURL(origin: string, roomID: string, auctionID: string, lastSeq: number) {
  const wsURL = new URL('/ws', origin);
  wsURL.protocol = wsURL.protocol === 'https:' ? 'wss:' : 'ws:';
  wsURL.searchParams.set('room_id', roomID);
  wsURL.searchParams.set('auction_id', auctionID);
  wsURL.searchParams.set('last_seq', String(lastSeq));
  return wsURL.toString();
}
