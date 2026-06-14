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
}

export interface MediaAdapterHandle {
  detach(): void;
}

export interface MediaAdapter {
  readonly protocols: MediaProtocol[];
  canPlay(source: MediaSource, env: PlaybackEnv): boolean;
  attach(source: MediaSource, env: PlaybackEnv, cb: AdapterCallbacks): MediaAdapterHandle;
}
