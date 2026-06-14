import type { MediaAdapter, MediaAdapterHandle } from './types';

type HlsConstructor = typeof import('hls.js').default;
type HlsInstance = InstanceType<HlsConstructor>;

let hlsConstructorPromise: Promise<HlsConstructor> | null = null;

async function loadHlsConstructor() {
  hlsConstructorPromise ??= import('hls.js').then((mod) => mod.default);
  return hlsConstructorPromise;
}

function attachNativeHls(sourceURL: string, videoEl: HTMLVideoElement, cb: Parameters<MediaAdapter['attach']>[2]): MediaAdapterHandle {
  let detached = false;
  const onPlaying = () => cb.onPlaying();
  const onError = () => cb.onFatal('native hls playback error');
  videoEl.addEventListener('playing', onPlaying);
  videoEl.addEventListener('error', onError);
  videoEl.src = sourceURL;
  videoEl.load();
  const playAttempt = videoEl.play();
  if (playAttempt && typeof playAttempt.catch === 'function') {
    playAttempt.catch(() => {
      // Autoplay rejection is not a media-source failure.
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

export const hlsAdapter: MediaAdapter = {
  protocols: ['hls', 'll-hls'],
  canPlay: (_source, env) => env.canNativeHls || env.mseHlsSupported,
  attach(source, env, cb): MediaAdapterHandle {
    if (env.canNativeHls) {
      return attachNativeHls(source.url, env.videoEl, cb);
    }
    let detached = false;
    let hls: HlsInstance | null = null;
    let fatalCount = 0;
    const onPlaying = () => cb.onPlaying();
    env.videoEl.addEventListener('playing', onPlaying);
    loadHlsConstructor()
      .then((Hls) => {
        if (detached) return;
        hls = new Hls({
          lowLatencyMode: source.protocol === 'll-hls',
          backBufferLength: 30
        });
        hls.loadSource(source.url);
        hls.attachMedia(env.videoEl);
        hls.on(Hls.Events.ERROR, (_event, data) => {
          if (!data.fatal) {
            cb.onRecoverableError?.(String(data.details || data.type || 'hls recoverable error'));
            return;
          }
          fatalCount += 1;
          if (fatalCount === 1 && data.type === Hls.ErrorTypes.NETWORK_ERROR) {
            hls?.startLoad();
            cb.onRecoverableError?.('hls network recovery attempted');
            return;
          }
          if (fatalCount === 1 && data.type === Hls.ErrorTypes.MEDIA_ERROR) {
            hls?.recoverMediaError();
            cb.onRecoverableError?.('hls media recovery attempted');
            return;
          }
          cb.onFatal(`hls fatal: ${String(data.type || data.details || 'unknown')}`);
        });
        const playAttempt = env.videoEl.play();
        if (playAttempt && typeof playAttempt.catch === 'function') {
          playAttempt.catch(() => {
            // Autoplay rejection is not a stream fatal.
          });
        }
      })
      .catch((error) => {
        cb.onFatal(error instanceof Error ? error.message : 'hls.js load failed');
      });
    return {
      detach() {
        if (detached) return;
        detached = true;
        env.videoEl.removeEventListener('playing', onPlaying);
        hls?.destroy();
        hls = null;
        env.videoEl.removeAttribute('src');
        env.videoEl.load();
      }
    };
  }
};
