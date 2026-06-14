import { createClientBidID } from '../../domain';
import {
  interpretPaymentResponse,
  payEndpoint,
  payMockEndpoint,
  payQueryEndpoint,
  type PaymentClientAction,
  type PaymentPhaseNext
} from '../contracts/payment-contract';

export type PayMockActionResult = {
  phase: PaymentPhaseNext;
  orderStatus?: string;
  clientAction?: PaymentClientAction;
  provider?: string;
  errorCode?: string;
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

export async function queryOrderPayment(orderID: string, fetcher: typeof fetch = fetch): Promise<PayMockActionResult> {
  const response = await fetcher(payQueryEndpoint(orderID), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' }
  });
  const payload = await response.json().catch(() => ({})) as {
    order_status?: string;
    trade_status?: string;
    deposit_status?: string;
    provider_payment_id?: string;
    code?: string;
  };
  return {
    phase: response.ok && payload.order_status === 'PAID' ? 'paid' : 'pending',
    orderStatus: payload.order_status,
    errorCode: payload.code
  };
}

export async function submitMockPayment(orderID: string, fetcher: typeof fetch = fetch): Promise<PayMockActionResult> {
  const response = await fetcher(payMockEndpoint(orderID), createPaymentRequest());
  const payload = await response.json() as { order_status?: string };
  const phase = interpretPaymentResponse({ ok: response.ok, orderStatus: payload.order_status });
  return { phase, orderStatus: payload.order_status };
}

function isMissingAlipayConfig(payload: unknown) {
  const body = payload as { code?: string; message?: string } | null;
  return body?.code === 'INVALID_ARGUMENT' && /alipay sandbox/i.test(body.message ?? '');
}

function submitPaymentClientAction(action: PaymentClientAction) {
  if (action.type !== 'redirect_form' || !action.html) return false;
  document.open();
  document.write(action.html);
  document.close();
  return true;
}

export async function submitOrderPayment(orderID: string, fetcher: typeof fetch = fetch): Promise<PayMockActionResult> {
  const response = await fetcher(payEndpoint(orderID), createPaymentRequest());
  const payload = await response.json().catch(() => ({})) as {
    order_status?: string;
    provider?: string;
    client_action?: PaymentClientAction;
    code?: string;
    message?: string;
  };
  if (response.ok && payload.client_action) {
    const submitted = submitPaymentClientAction(payload.client_action);
    return {
      phase: submitted ? 'pending' : 'failed',
      orderStatus: payload.order_status,
      clientAction: payload.client_action,
      provider: payload.provider
    };
  }
  if (!response.ok && isMissingAlipayConfig(payload)) {
    return submitMockPayment(orderID, fetcher);
  }
  return {
    phase: interpretPaymentResponse({ ok: response.ok, orderStatus: payload.order_status }),
    orderStatus: payload.order_status,
    errorCode: payload.code
  };
}
