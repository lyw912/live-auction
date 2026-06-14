export type MediaProtocol = 'mp4' | 'hls' | 'll-hls' | 'whep';

export interface MediaSource {
  protocol: MediaProtocol;
  url: string;
  mimeType?: string;
  priority: number;
}

export interface MediaPlayback {
  auctionId: string;
  isLive: boolean;
  posterURL?: string;
  sources: MediaSource[];
  latencyTargetMs?: number;
  capabilities: {
    nativeHlsOnSafari: boolean;
    mseHls: boolean;
    webrtc: boolean;
  };
  sessionEpoch?: string;
}

export const mediaPlaybackAllowedKeys = [
  'auctionId',
  'isLive',
  'posterURL',
  'sources',
  'latencyTargetMs',
  'capabilities',
  'sessionEpoch'
] as const;

export const mediaPlaybackForbiddenAuctionTruthKeys = [
  'price',
  'priceCents',
  'currentPriceCents',
  'current_price_cents',
  'winner',
  'winnerId',
  'currentWinnerId',
  'current_winner_id',
  'status',
  'seq',
  'engineSeq',
  'engine_seq',
  'endAt',
  'end_at',
  'settlement',
  'settlementStatus',
  'settlement_status',
  'rule',
  'ruleVersion',
  'rule_version'
] as const;

export function assertMediaPlaybackShape(value: MediaPlayback) {
  const keys = Object.keys(value);
  for (const key of keys) {
    if (!(mediaPlaybackAllowedKeys as readonly string[]).includes(key)) {
      throw new Error(`MediaPlayback contains unsupported key: ${key}`);
    }
    if ((mediaPlaybackForbiddenAuctionTruthKeys as readonly string[]).includes(key)) {
      throw new Error(`MediaPlayback contains auction truth key: ${key}`);
    }
  }
  return value;
}
