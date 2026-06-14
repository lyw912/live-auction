import { normalizeLiveSessionResponse } from './model';
import type { MediaPlayback } from '../../shared/media/contract';

export async function fetchLiveSession(auctionId: string, posterCandidate?: string, fetcher: typeof fetch = fetch): Promise<MediaPlayback> {
  const response = await fetcher(`/api/live/sessions/${encodeURIComponent(auctionId)}`, {
    method: 'GET',
    headers: { Accept: 'application/json' }
  });
  if (!response.ok) {
    throw new Error(`live session request failed: ${response.status}`);
  }
  return normalizeLiveSessionResponse(await response.json(), auctionId, posterCandidate);
}
