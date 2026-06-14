import React from 'react';
import { demoProductImageURL } from '../../domain';
import type { LiveMediaSourceV0 } from '../../features/live-media/useLiveMediaSource';

export function LiveBackdrop({ source, poster }: { source: LiveMediaSourceV0; poster?: string }) {
  return (
    <video
      className="live-video-bg"
      src={source.url}
      poster={poster || source.posterURL || demoProductImageURL}
      autoPlay
      muted
      loop
      playsInline
      aria-hidden="true"
      data-media-kind={source.kind}
      data-media-live={source.isLive ? 'true' : 'false'}
    />
  );
}
