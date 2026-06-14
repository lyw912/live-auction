import type { AuctionRealtimeEvent, LeaderboardPayload, OrderRow, SnapshotResponse } from '../../domain';

export const auctionQueryKeys = {
  snapshot: (auctionID: string) => ['auction', auctionID, 'snapshot'] as const,
  leaderboard: (auctionID: string) => ['auction', auctionID, 'leaderboard'] as const,
  orders: (auctionID: string) => ['auction', auctionID, 'orders'] as const
};

export type AuctionCachePatch = {
  snapshot?: SnapshotResponse;
  leaderboard?: LeaderboardPayload;
  orders?: OrderRow[];
};

export function patchAuctionCacheFromRealtime(event: AuctionRealtimeEvent): AuctionCachePatch {
  if (event.event_type === 'leaderboard') return { leaderboard: event.payload as LeaderboardPayload };
  if (event.event_type === 'snapshot') return { snapshot: event.payload as SnapshotResponse };
  return {};
}
