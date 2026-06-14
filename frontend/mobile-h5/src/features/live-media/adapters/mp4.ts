import type { MediaAdapter, MediaAdapterHandle } from './types';

export const mp4Adapter: MediaAdapter = {
  protocols: ['mp4'],
  canPlay: () => true,
  attach(source, env, cb): MediaAdapterHandle {
    const { videoEl } = env;
    let detached = false;
    const onPlaying = () => cb.onPlaying();
    const onError = () => cb.onFatal('mp4 playback error');
    videoEl.addEventListener('playing', onPlaying);
    videoEl.addEventListener('error', onError);
    videoEl.src = source.url;
    videoEl.load();
    const playAttempt = videoEl.play();
    if (playAttempt && typeof playAttempt.catch === 'function') {
      playAttempt.catch(() => {
        // Autoplay rejection is not fatal for auction truth; keep the poster/video ready for the next user gesture.
      });
    }
    return {
      detach() {
        if (detached) return;
        detached = true;
        videoEl.removeEventListener('playing', onPlaying);
        videoEl.removeEventListener('error', onError);
        videoEl.removeAttribute('src');
        videoEl.load();
      }
    };
  }
};
