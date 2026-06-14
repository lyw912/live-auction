import type { MediaProtocol, MediaSource } from '../../../shared/media/contract';

export interface PlaybackEnv {
  videoEl: HTMLVideoElement;
  canNativeHls: boolean;
  mseHlsSupported: boolean;
}

export interface AdapterCallbacks {
  onPlaying(): void;
  onFatal(reason: string): void;
  onRecoverableError?(reason: string): void;
  onStats?(stats: RealtimeMediaStats): void;
}

export interface RealtimeMediaStats {
  direction: 'inbound' | 'outbound';
  width?: number;
  height?: number;
  fps?: number;
  bitrateKbps?: number;
  packetsLost?: number;
  jitterMs?: number;
  codec?: string;
  transport?: string;
  candidatePair?: string;
  qualityLabel: string;
  updatedAt: number;
}

export interface MediaAdapterHandle {
  detach(): void;
}

export interface MediaAdapter {
  readonly protocols: MediaProtocol[];
  canPlay(source: MediaSource, env: PlaybackEnv): boolean;
  attach(source: MediaSource, env: PlaybackEnv, cb: AdapterCallbacks): MediaAdapterHandle;
}
