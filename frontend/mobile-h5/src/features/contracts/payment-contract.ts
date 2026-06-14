export type PaymentPhaseNext = 'paid' | 'failed';

export function payMockEndpoint(orderID: string) {
  return `/api/orders/${orderID}/pay-mock`;
}

export function interpretPaymentResponse(input: { ok: boolean; orderStatus?: string }): PaymentPhaseNext {
  if (!input.ok) return 'failed';
  return input.orderStatus === 'PAID' ? 'paid' : 'failed';
}

export function isPayablePendingOrder(row: { order_status?: string; status?: string; auction_id?: string }, auctionID: string) {
  return String(row.order_status ?? row.status ?? '') === 'ORDER_PENDING' && String(row.auction_id ?? '') === auctionID;
}
