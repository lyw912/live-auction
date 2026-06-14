import { useEffect, useMemo, useState, type RefObject } from 'react';
import { detectMediaPlaybackCapabilities } from '../../shared/media/detect';
import type { MediaPlayback, MediaSource } from '../../shared/media/contract';
import type { MediaAdapter, MediaAdapterHandle } from './adapters/types';
import { hlsAdapter } from './adapters/hls';
import { mp4Adapter } from './adapters/mp4';
import { whepAdapter } from './adapters/whep';
export { choosePlayableSource, liveSessionQueryKey } from './select-source';

export type PlaybackStatus = 'idle' | 'connecting' | 'playing' | 'degraded' | 'exhausted';

export interface PlaybackEngineState {
  status: PlaybackStatus;
  activeSource?: MediaSource;
  lastError?: string;
}

const adapters: MediaAdapter[] = [hlsAdapter, mp4Adapter, whepAdapter];

export function usePlaybackEngine(videoRef: RefObject<HTMLVideoElement>, playback?: MediaPlayback): PlaybackEngineState {
  const sortedSources = useMemo(() => [...(playback?.sources ?? [])].sort((a, b) => a.priority - b.priority), [playback?.sources]);
  const [state, setState] = useState<PlaybackEngineState>({ status: 'idle' });

  useEffect(() => {
    const videoEl = videoRef.current;
    if (!videoEl || !playback) {
      setState({ status: playback ? 'exhausted' : 'idle' });
      return;
    }
    let detached = false;
    let handle: MediaAdapterHandle | null = null;
    const env = detectMediaPlaybackCapabilities(videoEl);
    const attachFrom = (startIndex: number, degraded: boolean) => {
      if (detached) return;
      for (let index = startIndex; index < sortedSources.length; index += 1) {
        const source = sortedSources[index];
        const adapter = adapters.find((candidate) => candidate.protocols.includes(source.protocol));
        if (!adapter || !adapter.canPlay(source, { videoEl, ...env })) {
          continue;
        }
        setState({ status: degraded ? 'degraded' : 'connecting', activeSource: source });
        try {
          handle = adapter.attach(source, { videoEl, ...env }, {
            onPlaying: () => {
              if (!detached) setState({ status: 'playing', activeSource: source });
            },
            onFatal: (reason) => {
              if (detached) return;
              handle?.detach();
              handle = null;
              attachFrom(index + 1, true);
              if (index + 1 >= sortedSources.length) {
                setState({ status: 'exhausted', lastError: reason });
              }
            },
            onRecoverableError: (reason) => {
              if (!detached) setState((current) => ({ ...current, lastError: reason }));
            }
          });
        } catch (error) {
          handle?.detach();
          handle = null;
          const reason = error instanceof Error ? error.message : 'media adapter attach failed';
          if (index + 1 >= sortedSources.length) {
            setState({ status: 'exhausted', lastError: reason });
          } else {
            attachFrom(index + 1, true);
          }
        }
        return;
      }
      setState({ status: 'exhausted' });
    };
    attachFrom(0, false);
    return () => {
      detached = true;
      handle?.detach();
    };
  }, [playback?.auctionId, playback?.sessionEpoch, sortedSources, videoRef]);

  return state;
}
