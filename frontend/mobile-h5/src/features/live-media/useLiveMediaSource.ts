import { useQuery } from '@tanstack/react-query';
import { fetchLiveSession } from '../../entities/live-session/api';
import { posterOnlyPlayback } from '../../entities/live-session/model';
import { liveSessionQueryKey } from './select-source';

export { liveSessionQueryKey };

export function useLiveMediaSource(auctionID: string, posterCandidate?: string) {
  const fallbackPlayback = posterOnlyPlayback(auctionID, posterCandidate);
  const query = useQuery({
    queryKey: liveSessionQueryKey(auctionID),
    queryFn: () => fetchLiveSession(auctionID, posterCandidate),
    enabled: Boolean(auctionID),
    staleTime: 60_000,
    retry: 1,
    refetchOnWindowFocus: false,
    placeholderData: () => fallbackPlayback,
    throwOnError: false
  });
  return query.data ? query : { ...query, data: fallbackPlayback };
}
