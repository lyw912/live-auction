import { useEffect, useMemo, useState, type RefObject } from 'react';
import { detectMediaPlaybackCapabilities } from '../../shared/media/detect';
import type { MediaPlayback, MediaSource } from '../../shared/media/contract';
import type { MediaAdapter, MediaAdapterHandle, RealtimeMediaStats } from './adapters/types';
import { whepAdapter } from './adapters/whep';
export { choosePlayableSource, liveSessionQueryKey } from './select-source';

export type PlaybackStatus = 'idle' | 'connecting' | 'playing' | 'exhausted';

export interface PlaybackEngineState {
  status: PlaybackStatus;
  activeSource?: MediaSource;
  lastError?: string;
  stats?: RealtimeMediaStats;
}

const adapters: MediaAdapter[] = [whepAdapter];
const LIVE_RETRYABLE_ERROR = /(404|no stream|not found|whep handshake failed|connection failed|ice connection failed)/i;

function shouldRetryLiveSource(source: MediaSource | undefined, reason: string) {
  return source?.protocol === 'whep' && LIVE_RETRYABLE_ERROR.test(reason);
}

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
    let retryTimer = 0;
    const env = detectMediaPlaybackCapabilities(videoEl);
    const attachFrom = (startIndex: number) => {
      if (detached) return;
      window.clearTimeout(retryTimer);
      for (let index = startIndex; index < sortedSources.length; index += 1) {
        const source = sortedSources[index];
        const adapter = adapters.find((candidate) => candidate.protocols.includes(source.protocol));
        if (!adapter || !adapter.canPlay(source, { videoEl, ...env })) {
          continue;
        }
        setState({ status: 'connecting', activeSource: source });
        try {
          handle = adapter.attach(source, { videoEl, ...env }, {
            onPlaying: () => {
              if (!detached) setState((current) => ({ ...current, status: 'playing', activeSource: source }));
            },
            onFatal: (reason) => {
              if (detached) return;
              handle?.detach();
              handle = null;
              if (shouldRetryLiveSource(source, reason)) {
                setState({ status: 'connecting', activeSource: source, lastError: reason });
                retryTimer = window.setTimeout(() => attachFrom(index), 1500);
                return;
              }
              setState({ status: 'exhausted', lastError: reason });
            },
            onRecoverableError: (reason) => {
              if (!detached) setState((current) => ({ ...current, lastError: reason }));
            },
            onStats: (stats) => {
              if (!detached) setState((current) => ({ ...current, stats }));
            }
          });
        } catch (error) {
        handle?.detach();
        handle = null;
        const reason = error instanceof Error ? error.message : 'media adapter attach failed';
        if (shouldRetryLiveSource(source, reason)) {
          setState({ status: 'connecting', activeSource: source, lastError: reason });
          retryTimer = window.setTimeout(() => attachFrom(index), 1500);
          return;
        }
        setState({ status: 'exhausted', lastError: reason });
        }
        return;
      }
      setState({ status: 'exhausted' });
    };
    attachFrom(0);
    return () => {
      detached = true;
      window.clearTimeout(retryTimer);
      handle?.detach();
    };
  }, [playback?.auctionId, playback?.sessionEpoch, sortedSources, videoRef]);

  return state;
}
