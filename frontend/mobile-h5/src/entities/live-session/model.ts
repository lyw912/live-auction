import { demoLiveVideoURL, displayMediaURL } from '../../domain';
import { assertMediaPlaybackShape, type MediaPlayback, type MediaProtocol, type MediaSource } from '../../shared/media/contract';

const allowedProtocols = new Set<MediaProtocol>(['mp4', 'hls', 'll-hls', 'whep']);

export function normalizeMediaSource(raw: unknown, fallbackPriority = 90): MediaSource | null {
  if (!raw || typeof raw !== 'object') return null;
  const record = raw as Record<string, unknown>;
  const protocol = record.protocol;
  const url = record.url;
  if (typeof protocol !== 'string' || !allowedProtocols.has(protocol as MediaProtocol) || typeof url !== 'string' || url.trim() === '') {
    return null;
  }
  const priority = typeof record.priority === 'number' && Number.isFinite(record.priority) ? record.priority : fallbackPriority;
  return {
    protocol: protocol as MediaProtocol,
    url,
    mimeType: typeof record.mimeType === 'string' ? record.mimeType : undefined,
    priority
  };
}

export function normalizeLiveSessionResponse(raw: unknown, auctionId: string, posterCandidate?: string): MediaPlayback {
  const record = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {};
  const rawSources = Array.isArray(record.sources) ? record.sources : [];
  const sources = rawSources
    .map((source, index) => normalizeMediaSource(source, 90 + index))
    .filter((source): source is MediaSource => source !== null)
    .sort((a, b) => a.priority - b.priority);
  const capabilities = record.capabilities && typeof record.capabilities === 'object'
    ? record.capabilities as Record<string, unknown>
    : {};
  const posterURL = typeof record.posterURL === 'string' && record.posterURL.trim() !== ''
    ? displayMediaURL(record.posterURL)
    : displayMediaURL(posterCandidate);
  return assertMediaPlaybackShape({
    auctionId: typeof record.auctionId === 'string' && record.auctionId !== '' ? record.auctionId : auctionId,
    isLive: record.isLive === true,
    posterURL,
    sources: sources.length > 0
      ? sources
      : [{ protocol: 'mp4', url: demoLiveVideoURL, mimeType: 'video/mp4', priority: 90 }],
    latencyTargetMs: typeof record.latencyTargetMs === 'number' && Number.isFinite(record.latencyTargetMs) ? record.latencyTargetMs : undefined,
    capabilities: {
      nativeHlsOnSafari: capabilities.nativeHlsOnSafari === true,
      mseHls: capabilities.mseHls === true,
      webrtc: capabilities.webrtc === true
    },
    sessionEpoch: typeof record.sessionEpoch === 'string' ? record.sessionEpoch : undefined
  });
}

export function posterOnlyPlayback(auctionId: string, posterCandidate?: string): MediaPlayback {
  return assertMediaPlaybackShape({
    auctionId,
    isLive: false,
    posterURL: displayMediaURL(posterCandidate),
    sources: [{ protocol: 'mp4', url: demoLiveVideoURL, mimeType: 'video/mp4', priority: 90 }],
    capabilities: {
      nativeHlsOnSafari: false,
      mseHls: false,
      webrtc: false
    }
  });
}
