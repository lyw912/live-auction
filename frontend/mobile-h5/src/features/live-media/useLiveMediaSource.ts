import { demoLiveVideoURL, displayMediaURL } from '../../domain';

export interface LiveMediaSourceV0 {
  kind: 'video-file';
  url: string;
  posterURL?: string;
  isLive: boolean;
}

export function useLiveMediaSource(_auctionID: string, posterCandidate?: string): LiveMediaSourceV0 {
  return {
    kind: 'video-file',
    url: demoLiveVideoURL,
    posterURL: displayMediaURL(posterCandidate),
    isLive: false
  };
}
