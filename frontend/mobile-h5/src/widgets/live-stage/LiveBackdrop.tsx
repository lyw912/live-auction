import React, { useRef } from 'react';
import { demoProductImageURL } from '../../domain';
import type { MediaPlayback } from '../../shared/media/contract';
import { usePlaybackEngine } from '../../features/live-media/usePlaybackEngine';

export function LiveBackdrop({ playback, poster, mediaError }: { playback?: MediaPlayback; poster?: string; mediaError?: boolean }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const { status, activeSource } = usePlaybackEngine(videoRef, playback);
  const posterURL = poster || playback?.posterURL || demoProductImageURL;
  return (
    <video
      ref={videoRef}
      className="live-video-bg"
      poster={posterURL}
      autoPlay
      muted
      loop={!playback?.isLive}
      playsInline
      aria-hidden="true"
      data-media-status={mediaError ? 'descriptor-error' : status}
      data-media-protocol={activeSource?.protocol ?? 'poster'}
      data-media-live={playback?.isLive ? 'true' : 'false'}
    />
  );
}
