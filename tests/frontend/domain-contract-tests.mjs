import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';
import { mkdir, readFile, rm } from 'node:fs/promises';
import { build } from 'vite';

const outDir = 'artifacts/tmp/frontend-domain-tests';

async function bundle(entry, name) {
  const fileName = `${name}.mjs`;
  await build({
    configFile: false,
    logLevel: 'silent',
    build: {
      emptyOutDir: false,
      lib: {
        entry,
        formats: ['es'],
        fileName: () => fileName
      },
      outDir,
      rollupOptions: {
        external: [
          'react',
          'react-dom',
          'lucide-react',
          '@arco-design/web-react',
          '@radix-ui/react-slot',
          'class-variance-authority',
          'clsx',
          'tailwind-merge'
        ]
      }
    }
  });
  return import(pathToFileURL(`${process.cwd()}/${outDir}/${fileName}`).href);
}

await rm(outDir, { recursive: true, force: true });
await mkdir(outDir, { recursive: true });

const h5 = await bundle('frontend/mobile-h5/src/domain.ts', 'h5-domain');
const pc = await bundle('frontend/pc-console/src/domain.ts', 'pc-domain');
const h5BidContract = await bundle('frontend/mobile-h5/src/features/contracts/bid-contract.ts', 'h5-bid-contract');
const h5PaymentContract = await bundle('frontend/mobile-h5/src/features/contracts/payment-contract.ts', 'h5-payment-contract');
const h5RecoveryContract = await bundle('frontend/mobile-h5/src/features/contracts/recovery-contract.ts', 'h5-recovery-contract');
const h5WSContract = await bundle('frontend/mobile-h5/src/features/contracts/ws-contract.ts', 'h5-ws-contract');
const h5BidMachine = await bundle('frontend/mobile-h5/src/features/place-bid/bid-machine.ts', 'h5-bid-machine');
const h5ConnectionMachine = await bundle('frontend/mobile-h5/src/features/connection/connection-machine.ts', 'h5-connection-machine');
const h5AuctionQuery = await bundle('frontend/mobile-h5/src/entities/auction/query.ts', 'h5-auction-query');
const h5LiveMedia = await bundle('frontend/mobile-h5/src/features/live-media/useLiveMediaSource.ts', 'h5-live-media');
const h5PayMockAction = await bundle('frontend/mobile-h5/src/features/pay-order/pay-mock-action.ts', 'h5-pay-mock-action');
const pcPaymentContract = await bundle('frontend/pc-console/src/features/contracts/payment-readonly-contract.ts', 'pc-payment-contract');

assert.equal(h5.formatCents(12345), '¥123.45');
assert.equal(h5.deriveCountdown('2026-06-05T10:00:10.000Z', Date.parse('2026-06-05T10:00:00.000Z'), Date.parse('2026-06-05T10:00:05.000Z'), Date.parse('2026-06-05T10:00:00.000Z'), false, false, false), '剩余 00:05.0');
assert.equal(h5.deriveCountdown('', 0, Date.now(), 0, false, true, false), '剩余时间确认中');
{
  const endAt = '2099-05-22T14:00:00Z';
  const base = Date.parse('2099-05-22T13:59:50Z');
  assert.equal(h5.deriveCountdownPhase({ endAt, serverTimeMS: base - 4_000, nowMS: base - 4_000, serverTimeSyncedAt: base - 4_000, terminal: false, stale: false, active: true }).phase, 'hot');
  assert.equal(h5.deriveCountdownPhase({ endAt, serverTimeMS: base, nowMS: base, serverTimeSyncedAt: base, terminal: false, stale: false, active: true }).phase, 'critical');
  assert.equal(h5.deriveCountdownPhase({ endAt, serverTimeMS: base + 4_100, nowMS: base + 4_100, serverTimeSyncedAt: base + 4_100, terminal: false, stale: false, active: true }).phase, 'hammer');
  assert.equal(h5.deriveCountdownPhase({ endAt, serverTimeMS: base + 4_100, nowMS: base + 4_100, serverTimeSyncedAt: base + 4_100, terminal: false, stale: false, active: true }).beat, '第一次');
  assert.equal(h5.deriveCountdownPhase({ endAt, serverTimeMS: base + 6_100, nowMS: base + 6_100, serverTimeSyncedAt: base + 6_100, terminal: false, stale: false, active: true }).beat, '第二次');
  assert.equal(h5.deriveCountdownPhase({ endAt, serverTimeMS: base + 8_100, nowMS: base + 8_100, serverTimeSyncedAt: base + 8_100, terminal: false, stale: false, active: true }).beat, '最后一次');
  assert.equal(h5.isBidCloseGuardActive(endAt, base + 8_900, base + 8_900, base + 8_900), true);
  assert.equal(h5.isBidCloseGuardActive(endAt, base + 8_700, base + 8_700, base + 8_700), false);
  assert.equal(h5.deriveCountdownPhase({ endAt, serverTimeMS: base + 7_500, nowMS: base + 7_500, serverTimeSyncedAt: base + 7_500, terminal: false, stale: true, active: true }).phase, 'stale');
  assert.equal(h5.deriveCountdownPhase({ endAt, serverTimeMS: base + 10_100, nowMS: base + 10_100, serverTimeSyncedAt: base + 10_100, terminal: false, stale: false, active: true }).phase, 'syncing');
}
assert.equal(h5.rankBadgeLabel(1), '榜一');
assert.equal(h5.rankBadgeLabel(4), '第 4 名');
assert.equal(typeof h5.loadAuctionSoundPack, 'function');
assert.equal(typeof h5.playAuctionSound, 'function');
{
  const recap = h5.buildResultRecap({
    itemTitle: '青瓷茶盏',
    kind: 'winner',
    terminalPriceCents: 88000,
    heat: {
      activeBidders30s: 2,
      acceptedBids30s: 4,
      priceVelocityCentsPerMin: 12000,
      acceptedBidderCount: 3,
      totalAcceptedBids: 9,
      source: 'leaderboard'
    },
    extendCount: 2
  });
  assert.equal(recap.status, '已中拍');
  assert.equal(recap.price, '¥880.00');
  assert.match(recap.shareCopy, /3 人有效出价/);
  const card = h5.buildHighlightCard({ ...recap, title: '青瓷<茶盏>&特别版' });
  assert.equal(card.mimeType, 'image/svg+xml;charset=utf-8');
  assert.match(card.filename, /青瓷-茶盏-特别版-highlight\.svg/);
  assert.match(card.content, /3 人有效出价/);
  assert.match(card.content, /青瓷&lt;茶盏&gt;&amp;特别版/);
  assert.doesNotMatch(card.content, /青瓷<茶盏>&特别版/);
}
assert.equal(h5.isDangerousActionDisabled({ ctaDisabled: false, stale: false, sold: false }, 'connected'), false);
assert.equal(h5.isDangerousActionDisabled({ ctaDisabled: false, stale: true, sold: false }, 'connected'), true);
assert.equal(h5.isEngineRejected({ result: 'ENGINE_REJECTED' }), true);
assert.equal(h5.isBidConfirmationPending({ result: 'ENGINE_ACCEPTED', settlement_status: 'PENDING' }), false);
assert.equal(h5.isBidConfirmationPending({ result: 'BID_CONFIRMATION_PENDING' }), true);
assert.equal(h5BidContract.BID_REQUEST_TIMEOUT_MS, 8000);
{
  const pending = { auctionID: 'auc_live', clientBidID: 'client-1', amountCents: 45000, clientSeenSeq: 41 };
  assert.equal(h5BidContract.canRetryPendingBid({ pending, auctionID: 'auc_live', bidPhase: 'uncertain', riskCode: '' }), true);
  assert.equal(h5BidContract.canRetryPendingBid({ pending, auctionID: 'auc_live', bidPhase: 'idle', riskCode: 'PROCESSING_RETRY_LATER' }), true);
  assert.equal(h5BidContract.canRetryPendingBid({ pending, auctionID: 'other', bidPhase: 'uncertain', riskCode: '' }), false);
  assert.strictEqual(h5BidContract.prepareBidRequest({ pending, auctionID: 'auc_live', amountCents: 50000, clientSeenSeq: 42 }), pending);
  assert.deepEqual(h5BidContract.bidRequestPayload(pending), {
    client_bid_id: 'client-1',
    amount_cents: 45000,
    client_seen_seq: 41
  });
  assert.equal(h5BidContract.bidRequestHeaders(pending)['Idempotency-Key'], 'client-1');
  assert.deepEqual(h5BidContract.interpretBidResponse({
    ok: true,
    payload: { result: 'FAT_FINGER_CONFIRM_REQUIRED', confirm_token: 'confirm-1', amount_cents: 250000 },
    activeIncrementCents: 5000
  }), { kind: 'confirm_required', token: 'confirm-1', amountCents: 250000 });
  assert.deepEqual(h5BidContract.interpretBidResponse({
    ok: false,
    payload: { code: 'BID_TOO_LOW', current_price_cents: 45000 },
    activeIncrementCents: 5000
  }), { kind: 'rejected', code: 'BID_TOO_LOW', keepPending: false, retryAfterMS: undefined, nextValidBidCents: 50000 });
  assert.deepEqual(h5BidContract.interpretBidResponse({
    ok: true,
    payload: { code: 'PROCESSING_RETRY_LATER' },
    activeIncrementCents: 5000,
    retryAfterMS: 1200
  }), { kind: 'rejected', code: 'PROCESSING_RETRY_LATER', keepPending: true, retryAfterMS: 1200, nextValidBidCents: undefined });
  assert.deepEqual(h5BidContract.interpretBidResponse({
    ok: true,
    payload: { result: 'ENGINE_ACCEPTED', decision_status: 'ACCEPTED', durability_status: 'KAFKA_ACKED' },
    activeIncrementCents: 5000
  }), { kind: 'accepted', clearPending: true });
  assert.deepEqual(h5BidContract.networkBidFailure(), { kind: 'uncertain', code: 'NETWORK_ERROR', keepPending: true });
}
{
  const ticketRequest = h5WSContract.wsTicketRequest('room_main', 'auc_live', 'user_1');
  assert.equal(ticketRequest.url, '/api/auth/ws-ticket');
  assert.equal(ticketRequest.init.method, 'POST');
  assert.deepEqual(JSON.parse(ticketRequest.init.body), { room_id: 'room_main', auction_id: 'auc_live', user_id: 'user_1' });
  assert.deepEqual(h5WSContract.auctionWSProtocols('ticket-abc'), ['auction.v1', 'ticket.ticket-abc']);
  const wsURL = h5WSContract.auctionWSURL('https://example.test/live', 'room_main', 'auc_live', 42);
  assert.match(wsURL, /^wss:\/\/example\.test\/ws\/auctions\?/);
  assert.match(wsURL, /room_id=room_main/);
  assert.match(wsURL, /auction_id=auc_live/);
  assert.match(wsURL, /last_seq=42/);
}
assert.deepEqual(h5RecoveryContract.requiredRecoveryTriggers, ['seq_gap', 'outbox_gap', 'socket_disconnect', 'snapshot_stale', 'manual_refresh', 'uncertain_bid']);
assert.deepEqual(h5RecoveryContract.requiredSnapshotSources, ['history', 'db', 'redis_stale', 'snapshot_unavailable']);
assert.equal(h5RecoveryContract.shouldRecoverFromRealtimeEvent({ gap: true }), true);
assert.equal(h5RecoveryContract.shouldRecoverFromRealtimeEvent({ outbox_gap: true }), true);
assert.equal(h5RecoveryContract.shouldRecoverFromRealtimeEvent({ stale: true }), true);
assert.equal(h5RecoveryContract.shouldRecoverFromRealtimeEvent({}), false);
assert.equal(h5RecoveryContract.normalizeSnapshotSource('db'), 'db');
assert.equal(h5RecoveryContract.normalizeSnapshotSource('unknown'), 'snapshot_unavailable');
assert.deepEqual(h5AuctionQuery.auctionQueryKeys.snapshot('auc_live'), ['auction', 'auc_live', 'snapshot']);
assert.deepEqual(h5AuctionQuery.patchAuctionCacheFromRealtime({
  auction_id: 'auc_live',
  event_type: 'leaderboard',
  payload: { state: 'ACTIVE', entries: [] }
}), { leaderboard: { state: 'ACTIVE', entries: [] } });
assert.equal(h5BidMachine.bidStateToPhase('confirmRequired'), 'confirm_required');
assert.equal(h5BidMachine.bidStateToPhase('uncertain'), 'uncertain');
assert.equal(h5ConnectionMachine.connectionStateToPhase('resuming'), 'recovering');
assert.equal(h5ConnectionMachine.connectionStateToPhase('connected'), 'connected');
assert.deepEqual(h5LiveMedia.useLiveMediaSource('auc_live', '/api/media/poster.jpg'), {
  kind: 'video-file',
  url: '/demo/jade-live-loop.mp4',
  posterURL: '/api/media/poster.jpg',
  isLive: false
});
assert.equal(h5PaymentContract.payMockEndpoint('ord_1'), '/api/orders/ord_1/pay-mock');
{
  const request = h5PayMockAction.createPaymentRequest('pay-key-1');
  assert.equal(request.method, 'POST');
  assert.equal(request.headers['Content-Type'], 'application/json');
  assert.equal(request.headers['Idempotency-Key'], 'pay-key-1');
  assert.deepEqual(JSON.parse(request.body), { confirm: true });
  const fetchCalls = [];
  const result = await h5PayMockAction.submitMockPayment('ord_1', async (url, init) => {
    fetchCalls.push({ url, init });
    return {
      ok: true,
      json: async () => ({ order_status: 'PAID' })
    };
  });
  assert.equal(fetchCalls[0].url, '/api/orders/ord_1/pay-mock');
  assert.equal(fetchCalls[0].init.method, 'POST');
  assert.equal(result.phase, 'paid');
  assert.equal(result.orderStatus, 'PAID');
  assert.equal(h5PaymentContract.interpretPaymentResponse({ ok: true, orderStatus: 'PAID' }), 'paid');
  assert.equal(h5PaymentContract.interpretPaymentResponse({ ok: true, orderStatus: 'ORDER_PENDING' }), 'failed');
  assert.equal(h5PaymentContract.interpretPaymentResponse({ ok: false, orderStatus: 'PAID' }), 'failed');
  assert.equal(h5PaymentContract.isPayablePendingOrder({ order_status: 'ORDER_PENDING', auction_id: 'auc_live' }, 'auc_live'), true);
  assert.equal(h5PaymentContract.isPayablePendingOrder({ order_status: 'PAID', auction_id: 'auc_live' }, 'auc_live'), false);
}

const validRule = {
  startPriceCents: 10000,
  incrementCents: 5000,
  capPriceCents: 60000,
  durationSeconds: 600,
  extendWindowSeconds: 10,
  extendBySeconds: 10,
  maxExtendCount: 3,
  fatFingerThresholdCents: 100000,
  depositBPS: 1000,
  depositFloorCents: 5000,
  depositCapCents: 50000
};
assert.equal(pc.validateRule(validRule).valid, true);
assert.equal(pc.validateRule({ ...validRule, capPriceCents: 61000 }).valid, false);
assert.deepEqual(pc.rulePayload(validRule), {
  duration_seconds: 600,
  extend_window_seconds: 10,
  extend_by_seconds: 10,
  max_extend_count: 3,
  fat_finger_threshold_cents: 100000,
  deposit_bps: 1000,
  deposit_floor_cents: 5000,
  deposit_cap_cents: 50000
});
assert.equal(pc.monitorQuery('room_main', { type: 'HIGH', auctionID: 'auc_live', userID: '', traceID: 'tr_1' }), 'room_id=room_main&type=HIGH&auction_id=auc_live&trace_id=tr_1');
assert.equal(pcPaymentContract.pcConsolePaymentIsReadonly(), true);
assert.deepEqual(pcPaymentContract.pcOrderPaymentFields({
  id: 'ord_1',
  order_status: 'PAID',
  payment_status: 'PAYMENT_SUCCEEDED',
  provider_payment_id: 'pay_1',
  provider: 'mock'
}), {
  orderID: 'ord_1',
  orderStatus: 'PAID',
  paymentStatus: 'PAYMENT_SUCCEEDED',
  providerPaymentID: 'pay_1',
  provider: 'mock'
});
{
  const sharedIndex = await readFile('frontend/shared-design/src/index.ts', 'utf8');
  const tokens = await readFile('frontend/shared-design/tokens.css', 'utf8');
  const h5Styles = await readFile('frontend/mobile-h5/src/styles.css', 'utf8');
  const h5Components = await readFile('frontend/mobile-h5/src/components.tsx', 'utf8');
  const priceOdometer = await readFile('frontend/mobile-h5/src/features/atmosphere/PriceOdometer.tsx', 'utf8');
  const pcViz = await readFile('frontend/pc-console/src/widgets/CommandVizStrip.tsx', 'utf8');
  const pcVizShell = await readFile('frontend/pc-console/src/widgets/CommandVizStripShell.tsx', 'utf8');
  const pcComponents = await readFile('frontend/pc-console/src/components.tsx', 'utf8');
  const pcStyles = await readFile('frontend/pc-console/src/styles.css', 'utf8');
  assert.match(sharedIndex, /export \* from '\.\/components\/ui\/button'/);
  assert.match(sharedIndex, /export \* from '\.\/components\/ui\/badge'/);
  assert.match(sharedIndex, /export \* from '\.\/components\/ui\/table'/);
  assert.match(tokens, /--state-leading:/);
  assert.match(tokens, /--state-outbid:/);
  assert.match(tokens, /--state-won:/);
  assert.match(tokens, /--bid-cta:/);
  assert.match(tokens, /--chart-1:/);
  assert.match(tokens, /--sidebar-background:/);
  assert.match(tokens, /--font-auction-display:/);
  assert.match(priceOdometer, /requestAnimationFrame/);
  assert.doesNotMatch(priceOdometer, /AnimateNumber|Ticker|Cursor/);
  assert.match(h5Components, /<PriceOdometer/);
  assert.match(h5Styles, /prefers-reduced-motion: reduce/);
  assert.doesNotMatch(h5Styles, /blink/i);
  assert.match(h5Components, /order_status=PAID/);
  assert.match(h5Components, /pay-mock/);
  assert.match(pcViz, /from 'echarts\/core'/);
  assert.match(pcViz, /points\.slice\(-120\)/);
  assert.match(pcViz, /CommandVizFreshnessState = 'live' \| 'stale' \| 'paused'/);
  assert.match(pcVizShell, /React\.lazy/);
  assert.match(pcVizShell, /import\('\.\/CommandVizStrip'\)/);
  assert.match(pcComponents, /Data as of/);
  assert.match(pcComponents, /freshnessState/);
  assert.match(pcStyles, /data-state="stale"/);
  assert.match(pcStyles, /data-state="paused"/);
}

await rm(outDir, { recursive: true, force: true });
console.log('frontend domain contract tests passed');
