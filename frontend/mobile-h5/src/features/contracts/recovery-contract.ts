export type RecoveryTrigger =
  | 'seq_gap'
  | 'outbox_gap'
  | 'socket_disconnect'
  | 'snapshot_stale'
  | 'manual_refresh'
  | 'uncertain_bid';

export type SnapshotRecoverySource = 'history' | 'db' | 'redis_stale' | 'snapshot_unavailable';

export const requiredRecoveryTriggers: RecoveryTrigger[] = [
  'seq_gap',
  'outbox_gap',
  'socket_disconnect',
  'snapshot_stale',
  'manual_refresh',
  'uncertain_bid'
];

export const requiredSnapshotSources: SnapshotRecoverySource[] = [
  'history',
  'db',
  'redis_stale',
  'snapshot_unavailable'
];

export function shouldRecoverFromRealtimeEvent(event: { gap?: boolean; outbox_gap?: boolean; stale?: boolean }) {
  return Boolean(event.gap || event.outbox_gap || event.stale);
}

export function normalizeSnapshotSource(value: unknown): SnapshotRecoverySource {
  if (value === 'history' || value === 'db' || value === 'redis_stale' || value === 'snapshot_unavailable') return value;
  return 'snapshot_unavailable';
}
