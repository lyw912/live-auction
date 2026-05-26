import { expect, test } from '@playwright/test';
import { atmospherePriority, normalizeAtmosphere } from '../../frontend/mobile-h5/src/atmosphere';

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
