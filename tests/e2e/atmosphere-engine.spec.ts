import { expect, test } from '@playwright/test';
import { atmospherePriority, calculateAtmosphereIntensity, normalizeAtmosphere, shouldGateAtmosphere } from '../../frontend/mobile-h5/src/atmosphere';
import { reconnectDelayMS } from '../../frontend/mobile-h5/src/realtime';

test('H5 atmosphere priority keeps terminal and recovery effects above bid and social noise', () => {
  expect(atmospherePriority.sold).toBeGreaterThan(atmospherePriority.recovering);
  expect(atmospherePriority.recovering).toBeGreaterThan(atmospherePriority.extended);
  expect(atmospherePriority.extended).toBeGreaterThan(atmospherePriority.outbid);
  expect(atmospherePriority.outbid).toBeGreaterThan(atmospherePriority.leading);
  expect(atmospherePriority.leading).toBeGreaterThan(atmospherePriority.social);
});

test('H5 atmosphere normalizer requires event truth metadata and fills deterministic priority', () => {
  expect(normalizeAtmosphere({
    kind: 'leading',
    title: '领先！',
    detail: '¥400.00 服务端确认',
    auction_id: 'auc_live',
    cause_seq: 42,
    event_type: 'bid_accepted',
    user_scope: 'self'
  }, 41, () => 1001)).toEqual({
    id: 1001,
    kind: 'leading',
    title: '领先！',
    detail: '¥400.00 服务端确认',
    auction_id: 'auc_live',
    cause_seq: 42,
    event_type: 'bid_accepted',
    user_scope: 'self',
    priority: atmospherePriority.leading
  });

  expect(normalizeAtmosphere({
    kind: 'outbid',
    title: '被超越！',
    detail: '张** 已领先',
    auction_id: '',
    cause_seq: 42,
    event_type: 'bid_accepted',
    user_scope: 'self'
  }, 41)).toBeNull();
});

test('H5 atmosphere cue ids are monotonic by default', () => {
  const first = normalizeAtmosphere({
    kind: 'leading',
    title: '领先！',
    detail: '¥400.00 服务端确认',
    auction_id: 'auc_live',
    cause_seq: 43,
    event_type: 'bid_accepted',
    user_scope: 'self'
  }, 42);
  const second = normalizeAtmosphere({
    kind: 'outbid',
    title: '被超越！',
    detail: '张** 已领先',
    auction_id: 'auc_live',
    cause_seq: 44,
    event_type: 'bid_accepted',
    user_scope: 'self'
  }, 43);
  expect(second?.id).toBeGreaterThan(first?.id ?? 0);
});

test('H5 atmosphere intensity maps real heat and final seconds to 0..3', () => {
  expect(calculateAtmosphereIntensity({ acceptedBids30s: 0, priceVelocityCentsPerMin: 0, remainingMS: 60_000 })).toBe(0);
  expect(calculateAtmosphereIntensity({ acceptedBids30s: 2, priceVelocityCentsPerMin: 0, remainingMS: 60_000 })).toBe(1);
  expect(calculateAtmosphereIntensity({ acceptedBids30s: 5, priceVelocityCentsPerMin: 5000, remainingMS: 60_000 })).toBe(2);
  expect(calculateAtmosphereIntensity({ acceptedBids30s: 10, priceVelocityCentsPerMin: 0, remainingMS: 60_000 })).toBe(3);
  expect(calculateAtmosphereIntensity({ acceptedBids30s: 0, priceVelocityCentsPerMin: 12000, remainingMS: 60_000 })).toBe(3);
  expect(calculateAtmosphereIntensity({ acceptedBids30s: 0, priceVelocityCentsPerMin: 0, remainingMS: 9_500 })).toBe(1);
  expect(calculateAtmosphereIntensity({ acceptedBids30s: 0, priceVelocityCentsPerMin: 0, remainingMS: 4_500 })).toBe(2);
  expect(calculateAtmosphereIntensity({ acceptedBids30s: 0, priceVelocityCentsPerMin: 0, remainingMS: 2_500 })).toBe(3);
  expect(calculateAtmosphereIntensity({ acceptedBids30s: 4, priceVelocityCentsPerMin: 0, remainingMS: 60_000, extended: true })).toBe(3);
});

test('H5 atmosphere gate suppresses hype during stale recovery while preserving AI-off separation', () => {
  expect(shouldGateAtmosphere({})).toEqual({
    gated: false,
    reasons: [],
    allowMotion: true,
    allowAI: true
  });
  expect(shouldGateAtmosphere({ recovering: true, stale: true })).toEqual({
    gated: true,
    reasons: ['recovering', 'stale'],
    allowMotion: false,
    allowAI: false
  });
  expect(shouldGateAtmosphere({ disconnected: true })).toEqual({
    gated: true,
    reasons: ['disconnected'],
    allowMotion: false,
    allowAI: false
  });
  expect(shouldGateAtmosphere({ reducedMotion: true, aiOff: true })).toEqual({
    gated: true,
    reasons: ['reducedMotion', 'aiOff'],
    allowMotion: false,
    allowAI: false
  });
  expect(shouldGateAtmosphere({ aiOff: true })).toEqual({
    gated: false,
    reasons: ['aiOff'],
    allowMotion: true,
    allowAI: false
  });
});

test('H5 reconnect backoff honors Retry-After and stays bounded', () => {
  expect(reconnectDelayMS(1, 4000)).toBe(4000);
  expect(reconnectDelayMS(1, 100)).toBe(1000);
  expect(reconnectDelayMS(12, 60000)).toBe(30000);
  for (let attempt = 1; attempt <= 12; attempt += 1) {
    const delay = reconnectDelayMS(attempt);
    expect(delay).toBeGreaterThanOrEqual(1000);
    expect(delay).toBeLessThanOrEqual(30000);
  }
});
