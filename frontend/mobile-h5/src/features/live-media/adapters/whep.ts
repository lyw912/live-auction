import type { MediaAdapter } from './types';

type InboundStatsSnapshot = {
  bytesReceived: number;
  timestamp: number;
};

type RTCStatsMapEntry = RTCStats & Record<string, unknown>;

type VideoInboundStats = RTCStatsMapEntry & {
  kind?: string;
  frameWidth?: number;
  frameHeight?: number;
  framesPerSecond?: number;
  bytesReceived?: number;
  packetsLost?: number;
  jitter?: number;
  codecId?: string;
};

type VideoCodecStats = RTCStatsMapEntry & {
  mimeType?: string;
};

type IceCandidatePairRuntimeStats = RTCStatsMapEntry & {
  selected?: boolean;
  nominated?: boolean;
  state?: string;
  localCandidateId?: string;
  remoteCandidateId?: string;
};

type IceCandidateRuntimeStats = RTCStatsMapEntry & {
  protocol?: string;
  candidateType?: string;
};

function resolveEndpoint(rawURL: string) {
  try {
    const endpoint = new URL(rawURL, window.location.href);
    if ((endpoint.hostname === '127.0.0.1' || endpoint.hostname === 'localhost') &&
      window.location.hostname !== '127.0.0.1' &&
      window.location.hostname !== 'localhost') {
      endpoint.hostname = window.location.hostname;
    }
    return endpoint.toString();
  } catch {
    return rawURL;
  }
}

function waitForIceGatheringComplete(pc: RTCPeerConnection, timeoutMS = 5000) {
  if (pc.iceGatheringState === 'complete') return Promise.resolve();
  return new Promise<void>((resolve) => {
    let done = false;
    const finish = () => {
      if (done) return;
      done = true;
      window.clearTimeout(timer);
      pc.removeEventListener('icegatheringstatechange', onChange);
      resolve();
    };
    const onChange = () => {
      if (pc.iceGatheringState === 'complete') finish();
    };
    const timer = window.setTimeout(finish, timeoutMS);
    pc.addEventListener('icegatheringstatechange', onChange);
  });
}

function qualityLabel(width?: number, height?: number, bitrateKbps?: number) {
  const pixels = (width ?? 0) * (height ?? 0);
  if (pixels >= 1920 * 1080 && (bitrateKbps ?? 0) >= 5500) return '1080p 高清';
  if (pixels >= 1920 * 1080) return '1080p 码率不足';
  if (pixels >= 1280 * 720 && (bitrateKbps ?? 0) >= 2400) return '720p 清晰';
  if (pixels >= 1280 * 720) return '720p 码率不足';
  if (pixels >= 960 * 540) return '540p 可用';
  if (width || height) return '低清晰度';
  return '等待视频帧';
}

function formatCandidatePair(localType?: string, remoteType?: string) {
  if (!localType && !remoteType) return undefined;
  return [localType, remoteType].filter(Boolean).join(' -> ');
}

export const whepAdapter: MediaAdapter = {
  protocols: ['whep'],
  canPlay: () => typeof RTCPeerConnection !== 'undefined',
  attach(source, env, cb) {
    const { videoEl } = env;
    const pc = new RTCPeerConnection();
    let detached = false;
    let sessionURL = '';
    let statsTimer = 0;
    let lastInbound: InboundStatsSnapshot | null = null;
    const remoteStream = new MediaStream();
    const onPlaying = () => cb.onPlaying();
    const fail = (reason: string) => {
      if (!detached) cb.onFatal(reason);
    };
    const onConnectionStateChange = () => {
      if (pc.connectionState === 'failed' || pc.connectionState === 'closed') {
        fail(`whep connection ${pc.connectionState}`);
      }
    };
    const onIceConnectionStateChange = () => {
      if (pc.iceConnectionState === 'failed') {
        fail('whep ice connection failed');
      }
    };
    videoEl.addEventListener('playing', onPlaying);
    pc.addEventListener('connectionstatechange', onConnectionStateChange);
    pc.addEventListener('iceconnectionstatechange', onIceConnectionStateChange);
    pc.addEventListener('track', (event) => {
      if (detached) return;
      if (event.streams[0]) {
        videoEl.srcObject = event.streams[0];
      } else {
        remoteStream.addTrack(event.track);
        videoEl.srcObject = remoteStream;
      }
      const playAttempt = videoEl.play();
      if (playAttempt && typeof playAttempt.catch === 'function') {
        playAttempt.catch(() => {
          cb.onRecoverableError?.('whep autoplay waiting for user gesture');
        });
      }
    });
    pc.addTransceiver('video', { direction: 'recvonly' });
    pc.addTransceiver('audio', { direction: 'recvonly' });

    const pollStats = async () => {
      if (detached) return;
      try {
        const report = await pc.getStats();
        let inbound: VideoInboundStats | undefined;
        let codec: VideoCodecStats | undefined;
        let selectedPair: IceCandidatePairRuntimeStats | undefined;
        let localCandidate: IceCandidateRuntimeStats | undefined;
        let remoteCandidate: IceCandidateRuntimeStats | undefined;
        report.forEach((entry) => {
          const stats = entry as RTCStatsMapEntry;
          if (entry.type === 'inbound-rtp' && (stats.kind === 'video' || stats.mediaType === 'video')) {
            inbound = stats as VideoInboundStats;
          }
          if (entry.type === 'candidate-pair' && (stats.selected || stats.nominated || stats.state === 'succeeded')) {
            selectedPair = stats as IceCandidatePairRuntimeStats;
          }
        });
        if (inbound?.codecId) {
          codec = report.get(inbound.codecId) as VideoCodecStats | undefined;
        }
        if (selectedPair?.localCandidateId) {
          localCandidate = report.get(selectedPair.localCandidateId) as IceCandidateRuntimeStats | undefined;
        }
        if (selectedPair?.remoteCandidateId) {
          remoteCandidate = report.get(selectedPair.remoteCandidateId) as IceCandidateRuntimeStats | undefined;
        }
        if (inbound) {
          const now = inbound.timestamp || performance.now();
          const bytesReceived = inbound.bytesReceived ?? 0;
          const bitrateKbps = lastInbound && now > lastInbound.timestamp
            ? Math.max(0, Math.round(((bytesReceived - lastInbound.bytesReceived) * 8) / (now - lastInbound.timestamp)))
            : undefined;
          lastInbound = { bytesReceived, timestamp: now };
          cb.onStats?.({
            direction: 'inbound',
            width: inbound.frameWidth,
            height: inbound.frameHeight,
            fps: inbound.framesPerSecond ? Math.round(inbound.framesPerSecond) : undefined,
            bitrateKbps,
            packetsLost: inbound.packetsLost,
            jitterMs: inbound.jitter ? Math.round(inbound.jitter * 1000) : undefined,
            codec: codec?.mimeType,
            transport: localCandidate?.protocol,
            candidatePair: formatCandidatePair(localCandidate?.candidateType, remoteCandidate?.candidateType),
            qualityLabel: qualityLabel(inbound.frameWidth, inbound.frameHeight, bitrateKbps),
            updatedAt: Date.now()
          });
        }
      } catch {
        cb.onRecoverableError?.('whep stats unavailable');
      }
    };
    statsTimer = window.setInterval(() => void pollStats(), 1000);

    void (async () => {
      try {
        const offer = await pc.createOffer();
        await pc.setLocalDescription(offer);
        await waitForIceGatheringComplete(pc);
        if (detached || !pc.localDescription?.sdp) return;
        const response = await fetch(resolveEndpoint(source.url), {
          method: 'POST',
          headers: { 'Content-Type': 'application/sdp' },
          body: pc.localDescription.sdp
        });
        if (!response.ok) {
          fail(`whep handshake failed: ${response.status}`);
          return;
        }
        sessionURL = response.headers.get('Location') || '';
        const answer = await response.text();
        if (detached) return;
        await pc.setRemoteDescription({ type: 'answer', sdp: answer });
      } catch (error) {
        fail(error instanceof Error ? error.message : 'whep playback failed');
      }
    })();

    return {
      detach() {
        if (detached) return;
        detached = true;
        window.clearInterval(statsTimer);
        videoEl.removeEventListener('playing', onPlaying);
        pc.removeEventListener('connectionstatechange', onConnectionStateChange);
        pc.removeEventListener('iceconnectionstatechange', onIceConnectionStateChange);
        videoEl.srcObject = null;
        pc.close();
        if (sessionURL) {
          void fetch(new URL(sessionURL, resolveEndpoint(source.url)).toString(), { method: 'DELETE', keepalive: true }).catch(() => undefined);
        }
      }
    };
  }
};
