import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';
import { mkdir, rm } from 'node:fs/promises';
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
        external: ['react', 'react-dom', 'lucide-react', '@arco-design/web-react']
      }
    }
  });
  return import(pathToFileURL(`${process.cwd()}/${outDir}/${fileName}`).href);
}

await rm(outDir, { recursive: true, force: true });
await mkdir(outDir, { recursive: true });

const h5 = await bundle('frontend/mobile-h5/src/domain.ts', 'h5-domain');
const pc = await bundle('frontend/pc-console/src/domain.ts', 'pc-domain');

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
  assert.match(card.filename, /青瓷-茶盏-特别版-credential\.svg/);
  assert.match(card.content, /3 人有效出价/);
  assert.match(card.content, /青瓷&lt;茶盏&gt;&amp;特别版/);
  assert.doesNotMatch(card.content, /青瓷<茶盏>&特别版/);
}
assert.equal(h5.isDangerousActionDisabled({ ctaDisabled: false, stale: false, sold: false }, 'connected'), false);
assert.equal(h5.isDangerousActionDisabled({ ctaDisabled: false, stale: true, sold: false }, 'connected'), true);
assert.equal(h5.isEngineRejected({ result: 'ENGINE_REJECTED' }), true);
assert.equal(h5.isBidConfirmationPending({ result: 'ENGINE_ACCEPTED', settlement_status: 'PENDING' }), false);
assert.equal(h5.isBidConfirmationPending({ result: 'BID_CONFIRMATION_PENDING' }), true);

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

await rm(outDir, { recursive: true, force: true });
console.log('frontend domain contract tests passed');
