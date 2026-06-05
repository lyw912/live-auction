import { expect, test } from '@playwright/test';
import { atmospherePriority, normalizeAtmosphere } from '../../frontend/mobile-h5/src/atmosphere';
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
