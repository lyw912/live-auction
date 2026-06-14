import React, { useRef } from 'react';
import type { MediaPlayback } from '../../shared/media/contract';
import { usePlaybackEngine } from '../../features/live-media/usePlaybackEngine';

export function LiveBackdrop({ playback, poster, mediaError }: { playback?: MediaPlayback; poster?: string; mediaError?: boolean }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const { status, activeSource, stats } = usePlaybackEngine(videoRef, playback);
  const posterURL = poster || playback?.posterURL || undefined;
  const showStats = playback?.isLive && activeSource?.protocol === 'whep' && stats;
  return (
    <>
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
      {showStats ? (
        <div className="live-quality-pill" aria-label="直播清晰度诊断">
          <strong>{stats.qualityLabel}</strong>
          <span>
            {stats.width && stats.height ? `${stats.width}×${stats.height}` : '等待帧'}
            {stats.fps ? ` · ${stats.fps}fps` : ''}
            {stats.bitrateKbps ? ` · ${(stats.bitrateKbps / 1000).toFixed(1)}Mbps` : ''}
          </span>
          <em>{stats.transport ? stats.transport.toUpperCase() : 'ICE'}{stats.jitterMs != null ? ` · ${stats.jitterMs}ms` : ''}</em>
        </div>
      ) : null}
    </>
  );
}
