export interface HlsCapabilityProbe {
  isSupported(): boolean;
}

export interface MediaPlaybackCapabilities {
  canNativeHls: boolean;
  mseHlsSupported: boolean;
}

export function canPlayNativeHls(videoEl: HTMLVideoElement) {
  return videoEl.canPlayType('application/vnd.apple.mpegurl') !== '' || videoEl.canPlayType('application/x-mpegURL') !== '';
}

export function canUseMSEHls() {
  if (typeof window === 'undefined' || !('MediaSource' in window)) return false;
  const mediaSource = window.MediaSource;
  if (!mediaSource || typeof mediaSource.isTypeSupported !== 'function') return false;
  return (
    mediaSource.isTypeSupported('video/mp4; codecs="avc1.42E01E,mp4a.40.2"') ||
    mediaSource.isTypeSupported('video/mp4; codecs="avc1.64001f,mp4a.40.2"')
  );
}

export function detectMediaPlaybackCapabilities(videoEl: HTMLVideoElement, hlsProbe?: HlsCapabilityProbe): MediaPlaybackCapabilities {
  return {
    canNativeHls: canPlayNativeHls(videoEl),
    mseHlsSupported: hlsProbe?.isSupported() ?? canUseMSEHls()
  };
}
