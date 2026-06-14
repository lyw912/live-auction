import { createClientBidID, isBidConfirmationPending, isEngineRejected, type BidResponse, type PendingBidRequest } from '../../domain';

export const BID_REQUEST_TIMEOUT_MS = 8000;

export type BidSubmitDecision =
  | { kind: 'accepted'; clearPending: boolean }
  | { kind: 'confirm_required'; token: string; amountCents?: number }
  | { kind: 'rejected'; code: string; keepPending: boolean; retryAfterMS?: number; nextValidBidCents?: number }
  | { kind: 'uncertain'; code: 'NETWORK_ERROR'; keepPending: true };

export function canRetryPendingBid(input: {
  pending: PendingBidRequest | null;
  auctionID: string;
  bidPhase: string;
  riskCode: string;
}) {
  return Boolean(
    input.pending &&
    input.pending.auctionID === input.auctionID &&
    (
      input.bidPhase === 'uncertain' ||
      input.bidPhase === 'processing_retry' ||
      input.bidPhase === 'engine_pending' ||
      input.riskCode === 'PROCESSING_RETRY_LATER' ||
      input.riskCode === 'BID_CONFIRMATION_PENDING'
    )
  );
}

export function prepareBidRequest(input: {
  pending: PendingBidRequest | null;
  auctionID: string;
  amountCents: number;
  clientSeenSeq: number;
}): PendingBidRequest {
  if (input.pending?.auctionID === input.auctionID) return input.pending;
  return {
    auctionID: input.auctionID,
    clientBidID: createClientBidID(),
    amountCents: input.amountCents,
    clientSeenSeq: input.clientSeenSeq
  };
}

export function bidRequestPayload(request: PendingBidRequest) {
  return {
    client_bid_id: request.clientBidID,
    amount_cents: request.amountCents,
    client_seen_seq: request.clientSeenSeq
  };
}

export function bidRequestHeaders(request: PendingBidRequest) {
  return {
    'Content-Type': 'application/json',
    'Idempotency-Key': request.clientBidID
  };
}

export function interpretBidResponse(input: {
  ok: boolean;
  payload: BidResponse;
  retryAfterMS?: number;
  activeIncrementCents: number;
}): BidSubmitDecision {
  const { ok, payload, activeIncrementCents } = input;
  if (payload.result === 'FAT_FINGER_CONFIRM_REQUIRED' && payload.confirm_token) {
    return { kind: 'confirm_required', token: payload.confirm_token, amountCents: payload.amount_cents };
  }
  if (!ok || (payload.reject_reason && !isEngineRejected(payload)) || payload.code) {
    const code = payload.reject_reason ?? payload.code ?? '';
    const nextValidBidCents = payload.next_valid_bid_cents ?? (
      payload.current_price_cents != null ? payload.current_price_cents + activeIncrementCents : undefined
    );
    return {
      kind: 'rejected',
      code,
      keepPending: code === 'PROCESSING_RETRY_LATER',
      retryAfterMS: input.retryAfterMS,
      nextValidBidCents
    };
  }
  return { kind: 'accepted', clearPending: !isBidConfirmationPending(payload) };
}

export function networkBidFailure(): BidSubmitDecision {
  return { kind: 'uncertain', code: 'NETWORK_ERROR', keepPending: true };
}
