import type { WSTicketResponse } from '../../domain';
import { readJSON, retryAfterMSFromHeaders } from '../../domain';
import { reconnectDelayMS } from '../../realtime';
import { auctionWSProtocols, auctionWSURL, wsTicketRequest } from '../contracts/ws-contract';

export type ReconnectionManagerOptions = {
  roomID: string;
  auctionID: string;
  userID: string;
  getLastSeq: () => number;
  onOpen: () => void;
  onMessage: (data: unknown) => void;
  onDisconnect: () => void;
  onRetryScheduled?: (delayMS: number) => void;
};

export class ReconnectionManager {
  private socket: WebSocket | null = null;
  private reconnectTimer = 0;
  private reconnectAttempt = 0;
  private stopped = false;

  constructor(private readonly options: ReconnectionManagerOptions) {}

  async connect() {
    if (this.stopped) return;
    try {
      const ticketRequest = wsTicketRequest(this.options.roomID, this.options.auctionID, this.options.userID);
      const ticketResponse = await fetch(ticketRequest.url, ticketRequest.init);
      const ticketPayload = await readJSON<WSTicketResponse>(ticketResponse);
      if (!ticketResponse.ok || !ticketPayload?.ticket) {
        this.scheduleReconnect(retryAfterMSFromHeaders(ticketResponse));
        return;
      }
      if (this.stopped) return;
      const wsURL = auctionWSURL(window.location.origin, this.options.roomID, this.options.auctionID, this.options.getLastSeq());
      const socket = new WebSocket(wsURL, auctionWSProtocols(ticketPayload.ticket));
      this.socket = socket;
      socket.onopen = () => {
        this.reconnectAttempt = 0;
        this.options.onOpen();
      };
      socket.onmessage = (message) => {
        this.options.onMessage(JSON.parse(String(message.data)));
      };
      socket.onerror = () => {
        this.options.onDisconnect();
      };
      socket.onclose = () => {
        this.options.onDisconnect();
        this.scheduleReconnect();
      };
    } catch {
      this.options.onDisconnect();
      this.scheduleReconnect();
    }
  }

  stop() {
    this.stopped = true;
    if (this.reconnectTimer) window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = 0;
    this.socket?.close();
    this.socket = null;
  }

  private scheduleReconnect(retryAfter = 0) {
    if (this.stopped) return;
    if (this.reconnectTimer) window.clearTimeout(this.reconnectTimer);
    this.reconnectAttempt += 1;
    const delay = reconnectDelayMS(this.reconnectAttempt, retryAfter);
    this.options.onRetryScheduled?.(delay);
    this.reconnectTimer = window.setTimeout(() => {
      void this.connect();
    }, delay);
  }
}
