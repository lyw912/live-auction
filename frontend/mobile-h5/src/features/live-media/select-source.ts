import { detectMediaPlaybackCapabilities } from '../../shared/media/detect';
import type { MediaSource } from '../../shared/media/contract';
import type { MediaAdapter } from './adapters/types';
import { hlsAdapter } from './adapters/hls';
import { mp4Adapter } from './adapters/mp4';
import { whepAdapter } from './adapters/whep';

const adapters: MediaAdapter[] = [hlsAdapter, mp4Adapter, whepAdapter];

export function choosePlayableSource(sources: MediaSource[], env: ReturnType<typeof detectMediaPlaybackCapabilities>, startIndex = 0) {
  for (let index = startIndex; index < sources.length; index += 1) {
    const source = sources[index];
    const adapter = adapters.find((candidate) => candidate.protocols.includes(source.protocol));
    if (!adapter) continue;
    if (adapter.canPlay(source, { videoEl: {} as HTMLVideoElement, ...env })) {
      return { source, adapter, index };
    }
  }
  return null;
}

export function liveSessionQueryKey(auctionID: string) {
  return ['live-session', auctionID] as const;
}
