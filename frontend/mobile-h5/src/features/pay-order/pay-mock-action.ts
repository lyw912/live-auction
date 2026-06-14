import { createClientBidID } from '../../domain';
import { interpretPaymentResponse, payMockEndpoint, type PaymentPhaseNext } from '../contracts/payment-contract';

export type PayMockActionResult = {
  phase: PaymentPhaseNext;
  orderStatus?: string;
};

export function createPaymentRequest(idempotencyKey = createClientBidID()): RequestInit {
  return {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey
    },
    body: JSON.stringify({ confirm: true })
  };
}

export async function submitMockPayment(orderID: string, fetcher: typeof fetch = fetch): Promise<PayMockActionResult> {
  const response = await fetcher(payMockEndpoint(orderID), createPaymentRequest());
  const payload = await response.json() as { order_status?: string };
  const phase = interpretPaymentResponse({ ok: response.ok, orderStatus: payload.order_status });
  return { phase, orderStatus: payload.order_status };
}
