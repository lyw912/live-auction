export function pcConsolePaymentIsReadonly() {
  return true;
}

export function pcOrderPaymentFields(order: {
  id?: string;
  order_status?: string;
  payment_status?: string;
  provider_payment_id?: string;
  provider?: string;
}) {
  return {
    orderID: order.id ?? '',
    orderStatus: order.order_status ?? '',
    paymentStatus: order.payment_status ?? '',
    providerPaymentID: order.provider_payment_id ?? '',
    provider: order.provider ?? ''
  };
}
