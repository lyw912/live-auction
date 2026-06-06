import type { AtmosphereCue } from './atmosphere';

export type AuctionState =
  | 'scheduled'
  | 'active_empty'
  | 'active_bids'
  | 'self_leading'
  | 'pending'
  | 'rejected'
  | 'extended'
  | 'recovering'
  | 'disconnected'
  | 'sold_winner'
  | 'sold_loser'
  | 'ended'
  | 'cancelled';

export type Scenario = {
  key: AuctionState;
  title: string;
  status: string;
  price: string;
  leader: string;
  feedback: string;
  cta: string;
  ctaDisabled: boolean;
  countdown?: string;
  stale?: boolean;
  staleCopy?: string;
  pending?: boolean;
  rejected?: boolean;
  winner?: boolean;
  sold?: boolean;
};

export type BidderRequirement = {
  verification_required?: boolean;
  deposit_required?: boolean;
  verified?: boolean;
  deposit_held?: boolean;
  reason?: string;
};

export type BidPhase = 'idle' | 'pending' | 'engine_pending' | 'engine_sold_pending' | 'accepted' | 'rejected' | 'uncertain' | 'confirm_required' | 'confirming';
export type PaymentPhase = 'idle' | 'pending' | 'paid' | 'failed' | 'expired';
export type RecoveryPhase = 'idle' | 'recovering';
export type ConnectionPhase = 'connecting' | 'connected' | 'recovering' | 'disconnected';
export type BottomSheetKey = 'products' | 'details' | 'maxBid' | 'leaderboard' | 'history' | 'orders' | 'qa' | 'more';
export type AuctionOverlayMode = 'feed' | 'bid';
export type ResultSheetKind = 'winner' | 'loser' | 'unsold';
export type SoundCapability = 'ready' | 'unavailable' | 'blocked';
export type MaxBidPhase = 'idle' | 'pending' | 'canceling' | 'error';
export type CountdownPhase = 'normal' | 'hot' | 'critical' | 'hammer' | 'syncing' | 'stale' | 'terminal';
export type CountdownPhaseState = { phase: CountdownPhase; remainingMS: number | null; beat: string };

export type MaxBidIntent = {
  id: string;
  auction_id: string;
  user_id: string;
  max_amount_cents: number;
  status: 'ACTIVE' | 'CANCELLED' | 'EXHAUSTED' | 'TERMINAL';
  source: 'MAX_BID' | 'PRE_BID';
  last_applied_seq?: number;
  version?: number;
};

export type BidResponse = {
  result?: string;
  auction_id?: string;
  seq?: number;
  engine_seq?: number;
  engine_epoch?: number;
  decision_status?: string;
  durability_status?: string;
  settlement_status?: string;
  decision_basis?: {
    previous_price_cents?: number;
    required_min_price_cents?: number;
    current_price_cents?: number;
    reason?: string;
    engine_seq?: number;
  };
  current_price_cents?: number;
  current_winner_id?: string;
  end_at?: string;
  server_time_ms?: number;
  reject_reason?: string | null;
  code?: string;
  confirm_token?: string;
  expires_in_ms?: number;
  amount_cents?: number;
  details?: {
    retry_after_ms?: number;
    retry_after_secs?: number;
  };
};

export type PendingBidRequest = {
  auctionID: string;
  clientBidID: string;
  amountCents: number;
  clientSeenSeq: number;
};

export function isBidConfirmationPending(payload?: BidResponse | null) {
  return payload?.result === 'BID_CONFIRMATION_PENDING'
    || payload?.decision_status === 'PENDING_DURABILITY'
    || payload?.durability_status === 'KAFKA_UNKNOWN'
    || (Boolean(payload?.result?.startsWith('ENGINE_')) && payload?.settlement_status === 'PENDING');
}

export function isEngineRejected(payload?: BidResponse | null) {
  return payload?.result === 'ENGINE_REJECTED';
}

export type AuctionRealtimeEvent = {
  auction_id: string;
  event_type: string;
  seq?: number;
  end_at?: string;
  server_time_ms?: number;
  payload?: {
    status?: string;
    current_price_cents?: number;
    amount_cents?: number;
    old_end_at?: string;
    end_at?: string;
    extend_count?: number;
    max_extend_count?: number;
    server_time_ms?: number;
    order_id?: string;
    leader_user_masked?: string;
    current_winner_id?: string;
    user_id?: string;
    reason?: string;
    order_status?: string;
    deposit_status?: string;
  };
};

export type SnapshotResponse = {
  auction_id: string;
  id?: string;
  status?: string;
  current_price_cents?: number;
  current_winner_id?: string;
  increment_cents?: number;
  end_at?: string;
  server_time_ms?: number;
  event_type?: string;
  seq: number;
  source?: string;
  stale?: boolean;
  max_bid_intent?: MaxBidIntent;
  payload?: {
    status?: string;
    current_price_cents?: number;
    current_winner_id?: string;
    leader_user_masked?: string;
    old_end_at?: string;
    end_at?: string;
    extend_count?: number;
    max_extend_count?: number;
    server_time_ms?: number;
    reason?: string;
    item?: AuctionItem;
    bidder_requirement?: BidderRequirement;
  };
  item?: AuctionItem;
  bidder_requirement?: BidderRequirement;
};

export type AuctionItem = {
  title?: string;
  description?: string;
  image_url?: string;
  imageURL?: string;
  video_poster_url?: string;
  videoPosterURL?: string;
  certificate?: string;
  condition?: string;
  shipping?: string;
  dimensions?: string;
  material?: string;
  return_policy?: string;
  flaws?: string;
};

export type HistoryRow = Record<string, unknown>;
export type ChatMessage = {
  id: number;
  room_id: string;
  user_id: string;
  body: string;
  created_at: string;
};
export type SystemMessage = {
  id: number;
  room_id: string;
  auction_id?: string;
  source: string;
  source_seq?: number;
  style: string;
  body: string;
  created_at: string;
};
export type ProductQAAnswer = {
  auction_id: string;
  answer: string;
  facts_used: string[];
  safety_note: string;
};
export type OrderRow = {
  order_id?: string;
  auction_id?: string;
  amount_cents?: number;
  order_status?: string;
};
export type AuctionSummary = {
  id: string;
  room_id?: string;
  status?: string;
  item_id?: string;
  current_price_cents?: number;
  current_winner_id?: string;
  increment_cents?: number;
  cap_price_cents?: number;
  accepted_bid_count?: number;
  seq?: number;
  start_at?: string;
  end_at?: string;
  server_time_ms?: number;
  item?: AuctionItem;
  bidder_requirement?: BidderRequirement;
  rule?: {
    duration_seconds?: number;
    extend_window_seconds?: number;
    extend_by_seconds?: number;
    max_extend_count?: number;
    fat_finger_threshold_cents?: number;
    deposit_bps?: number;
    deposit_floor_cents?: number;
    deposit_cap_cents?: number;
  };
};
export type LeaderboardEntry = {
  rank: number;
  user_id: string;
  user_masked: string;
  amount_cents: number;
  bid_count: number;
  is_current?: boolean;
};
export type LeaderboardPayload = {
  auction_id: string;
  seq?: number;
  server_time_ms?: number;
  current_price_cents: number;
  current_winner_id?: string;
  my_rank?: number;
  my_best_amount_cents?: number;
  gap_to_leader_cents?: number;
  gap_to_next_rank_cents?: number;
  next_valid_bid_cents?: number;
  state?: 'NOT_BID' | 'OUTBID' | 'LEADING';
  leader_amount_cents: number;
  accepted_bidder_count: number;
  active_bidders_30s?: number;
  accepted_bids_30s?: number;
  price_velocity_cents_per_min?: number;
  entries?: LeaderboardEntry[];
};
export type HeatSnapshot = {
  activeBidders30s: number;
  acceptedBids30s: number;
  priceVelocityCentsPerMin: number;
  acceptedBidderCount: number;
  totalAcceptedBids?: number;
  source: 'leaderboard' | 'auction' | 'fallback';
};
export type ResultRecap = {
  title: string;
  status: string;
  price: string;
  winner: string;
  facts: string[];
  nextAction: string;
  shareCopy: string;
};
export type WSTicketResponse = {
  ticket?: string;
  expires_in_ms?: number;
};
export type AuthUser = {
  ID: string;
  Role: string;
};

export const demoUserID = 'user_1';
export const demoLiveVideoURL = '/demo/pottery-live-loop.webm';
export const demoProductImageURL = '/demo/ceramic-tea-cup.jpg';

export const scenarios: Scenario[] = [
  { key: 'scheduled', title: '即将开拍', status: 'SCHEDULED', price: '¥100.00', leader: '暂无领先', feedback: '19:58 开始', countdown: '距开拍 12:00', cta: '等待开拍', ctaDisabled: true },
  { key: 'active_empty', title: '首拍', status: 'ACTIVE', price: '¥100.00', leader: '暂无领先', feedback: '最低 ¥150.00', countdown: '剩余 10:00', cta: '出价 ¥150.00', ctaDisabled: false },
  { key: 'active_bids', title: '竞价中', status: 'ACTIVE', price: '¥350.00', leader: '张** 领先', feedback: '下一口 ¥400.00', countdown: '剩余 02:34', cta: '出价 ¥400.00', ctaDisabled: false },
  { key: 'self_leading', title: '领先中', status: 'ACTIVE', price: '¥400.00', leader: '你已领先', feedback: '等待其他用户出价', countdown: '剩余 01:18', cta: '已领先', ctaDisabled: true },
  { key: 'pending', title: '提交中', status: 'ACTIVE', price: '¥400.00', leader: '李** 领先', feedback: '等待服务端确认', countdown: '剩余 01:08', cta: '确认中', ctaDisabled: true, pending: true },
  { key: 'rejected', title: '被拒绝', status: 'ACTIVE', price: '¥400.00', leader: '李** 领先', feedback: '请按加价幅度出价', countdown: '剩余 00:58', cta: '出价 ¥450.00', ctaDisabled: false, rejected: true },
  { key: 'extended', title: '已延时', status: 'ACTIVE', price: '¥450.00', leader: '王** 领先', feedback: '已延时 10 秒', countdown: '延时后 00:20', cta: '出价 ¥500.00', ctaDisabled: false },
  { key: 'recovering', title: '恢复中', status: 'RECOVERING', price: '¥450.00', leader: '同步中', feedback: '正在同步权威状态', countdown: '剩余时间待同步', cta: '同步中', ctaDisabled: true, stale: true },
  { key: 'disconnected', title: '已断开', status: 'DISCONNECTED', price: '¥450.00', leader: '离线', feedback: '重连中', countdown: '剩余时间已过期', cta: '重连中', ctaDisabled: true, stale: true },
  { key: 'sold_winner', title: '成交', status: 'SOLD', price: '¥600.00', leader: '你已拍中', feedback: '订单待支付', countdown: '支付倒计时同步中', cta: '去支付', ctaDisabled: false, winner: true, sold: true },
  { key: 'sold_loser', title: '已成交', status: 'SOLD', price: '¥600.00', leader: '赵** 拍中', feedback: '本场已结束', countdown: '已落锤', cta: '已结束', ctaDisabled: true, sold: true },
  { key: 'ended', title: '流拍', status: 'ENDED', price: '¥100.00', leader: '无成交', feedback: '无人出价', countdown: '已结束', cta: '已结束', ctaDisabled: true },
  { key: 'cancelled', title: '已取消', status: 'CANCELLED', price: '¥350.00', leader: '取消前价格', feedback: '主播已取消', countdown: '已取消', cta: '已取消', ctaDisabled: true }
];

export function formatCents(cents: number) {
  return `¥${(cents / 100).toFixed(2)}`;
}

export function formatRemaining(ms: number) {
  if (ms <= 10_000) {
    const tenths = Math.max(0, Math.ceil(ms / 100) / 10);
    return `00:${tenths.toFixed(1).padStart(4, '0')}`;
  }
  const totalSeconds = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

export function deriveCountdown(endAt: string, serverTimeMS: number, nowMS: number, serverTimeSyncedAt: number, terminal: boolean, stale: boolean, extended: boolean) {
  if (terminal) return '已结束';
  if (!endAt || serverTimeMS <= 0) return stale ? '剩余时间待同步' : '等待服务端时间';
  const endAtMS = Date.parse(endAt);
  if (!Number.isFinite(endAtMS)) return '等待服务端时间';
  // Measure elapsed using local-monotonic time since the last server-time sync.
  // This avoids mixing client-epoch with server-epoch, making the countdown
  // correct regardless of client/server clock skew.
  const syncedAt = serverTimeSyncedAt > 0 ? serverTimeSyncedAt : serverTimeMS;
  const elapsed = Math.max(0, nowMS - syncedAt);
  const remaining = endAtMS - serverTimeMS - elapsed;
  if (remaining <= 0) return stale ? '本地到零，正在同步' : '到点同步中';
  return `${extended ? '延时后' : '剩余'} ${formatRemaining(remaining)}`;
}

export function remainingCountdownMS(endAt: string, serverTimeMS: number, nowMS: number, serverTimeSyncedAt: number) {
  if (!endAt || serverTimeMS <= 0) return null;
  const endAtMS = Date.parse(endAt);
  if (!Number.isFinite(endAtMS)) return null;
  const syncedAt = serverTimeSyncedAt > 0 ? serverTimeSyncedAt : serverTimeMS;
  return endAtMS - serverTimeMS - Math.max(0, nowMS - syncedAt);
}

export function deriveCountdownPhase(input: {
  endAt: string;
  serverTimeMS: number;
  nowMS: number;
  serverTimeSyncedAt: number;
  terminal: boolean;
  stale: boolean;
  active: boolean;
}): CountdownPhaseState {
  if (input.terminal) return { phase: 'terminal', remainingMS: null, beat: '' };
  if (input.stale || !input.active) return { phase: 'stale', remainingMS: null, beat: '' };
  const remainingMS = remainingCountdownMS(input.endAt, input.serverTimeMS, input.nowMS, input.serverTimeSyncedAt);
  if (remainingMS == null) return { phase: 'stale', remainingMS: null, beat: '' };
  if (remainingMS <= 0) return { phase: 'syncing', remainingMS, beat: '' };
  if (remainingMS <= 3_000) {
    const second = Math.max(1, Math.ceil(remainingMS / 1000));
    return { phase: 'hammer', remainingMS, beat: second === 3 ? '第一次' : second === 2 ? '第二次' : '最后一次' };
  }
  if (remainingMS <= 5_000) return { phase: 'critical', remainingMS, beat: '' };
  if (remainingMS <= 10_000) return { phase: 'hot', remainingMS, beat: '' };
  return { phase: 'normal', remainingMS, beat: '' };
}

export function formatClockTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--:--:--';
  return date.toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

export function extensionCopyFromEvent(detail: AuctionRealtimeEvent, oldEndAt: string, nextEndAt: string) {
  const oldCopy = formatClockTime(detail.payload?.old_end_at ?? oldEndAt);
  const nextCopy = formatClockTime(nextEndAt);
  const count = detail.payload?.extend_count;
  const max = detail.payload?.max_extend_count;
  const countCopy = count != null && max != null ? ` · 第 ${count}/${max} 次` : count != null ? ` · 第 ${count} 次` : '';
  return `延时 ${oldCopy} -> ${nextCopy}${countCopy}`;
}

export function isCountdownExpired(endAt: string, serverTimeMS: number, nowMS: number, serverTimeSyncedAt: number) {
  if (!endAt || serverTimeMS <= 0) return false;
  const endAtMS = Date.parse(endAt);
  if (!Number.isFinite(endAtMS)) return false;
  const syncedAt = serverTimeSyncedAt > 0 ? serverTimeSyncedAt : serverTimeMS;
  return endAtMS - serverTimeMS - Math.max(0, nowMS - syncedAt) <= 0;
}

export function createClientBidID() {
  if (globalThis.crypto?.randomUUID) {
    return globalThis.crypto.randomUUID();
  }
  return `bid_${Date.now()}_${Math.random().toString(16).slice(2)}`;
}

export async function readJSON<T>(response: Response): Promise<T | null> {
  try {
    return await response.json() as T;
  } catch {
    return null;
  }
}

export function responseServerTimeMS(response: Response) {
  const dateHeader = response.headers.get('date');
  if (!dateHeader) return 0;
  const parsed = Date.parse(dateHeader);
  return Number.isFinite(parsed) ? parsed : 0;
}

export function leaderboardCopy(payload: LeaderboardPayload | null) {
  if (!payload || !payload.entries?.length) return '等待首个有效出价';
  if (payload.my_rank === 1) return '你正在领先';
  if (payload.my_rank && payload.gap_to_leader_cents != null) {
    return `第 ${payload.my_rank} 名 · 差 ${formatCents(payload.gap_to_leader_cents)}`;
  }
  return `${payload.accepted_bidder_count} 人已有效出价`;
}

export function leaderboardActionCopy(payload: LeaderboardPayload | null, fallbackBidCents: number) {
  const nextBid = payload?.next_valid_bid_cents ?? fallbackBidCents;
  if (!payload || !payload.entries?.length) {
    return {
      headline: '等待首个有效出价',
      action: `下一口 ${formatCents(nextBid)}`,
      freshness: '榜单等待服务端事件'
    };
  }
  if (payload.my_rank === 1 || payload.state === 'LEADING') {
    return {
      headline: '第 1 名 · 正在领先',
      action: `守住领先 · 下一口 ${formatCents(nextBid)}`,
      freshness: `seq ${payload.seq ?? '-'} · ${payload.accepted_bidder_count} 人入局`
    };
  }
  if (payload.my_rank && payload.gap_to_next_rank_cents != null) {
    return {
      headline: `第 ${payload.my_rank} 名 · 差 ${formatCents(payload.gap_to_next_rank_cents)}`,
      action: `下一口 ${formatCents(nextBid)}`,
      freshness: `seq ${payload.seq ?? '-'} · 近30秒 ${payload.accepted_bids_30s ?? 0} 次`
    };
  }
  return {
    headline: `${payload.accepted_bidder_count} 人已有效出价`,
    action: `一步入局 ${formatCents(nextBid)}`,
    freshness: `seq ${payload.seq ?? '-'} · 榜单已同步`
  };
}

export function rankBadgeLabel(rank: number) {
  if (rank === 1) return '榜一';
  if (rank === 2) return '榜二';
  if (rank === 3) return '榜三';
  return `第 ${rank} 名`;
}

export function heatSnapshot(payload: LeaderboardPayload | null, activeAuction?: AuctionSummary): HeatSnapshot {
  if (payload) {
    return {
      activeBidders30s: Math.max(0, payload.active_bidders_30s ?? 0),
      acceptedBids30s: Math.max(0, payload.accepted_bids_30s ?? 0),
      priceVelocityCentsPerMin: Math.max(0, payload.price_velocity_cents_per_min ?? 0),
      acceptedBidderCount: Math.max(0, payload.accepted_bidder_count ?? 0),
      totalAcceptedBids: activeAuction?.accepted_bid_count,
      source: 'leaderboard'
    };
  }
  if (activeAuction) {
    return {
      activeBidders30s: 0,
      acceptedBids30s: 0,
      priceVelocityCentsPerMin: 0,
      acceptedBidderCount: 0,
      totalAcceptedBids: activeAuction.accepted_bid_count,
      source: 'auction'
    };
  }
  return {
    activeBidders30s: 0,
    acceptedBids30s: 0,
    priceVelocityCentsPerMin: 0,
    acceptedBidderCount: 0,
    source: 'fallback'
  };
}

export function createAudioContext() {
  const AudioContextCtor = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!AudioContextCtor) return null;
  return new AudioContextCtor();
}

export function isReducedMotionPreferred() {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
}

export function vibratePattern(kind: AtmosphereCue['kind']) {
  if (!('vibrate' in navigator) || isReducedMotionPreferred() || document.visibilityState === 'hidden') return;
  const pattern: Record<AtmosphereCue['kind'], number | number[]> = {
    leading: 18,
    outbid: [18, 24, 18],
    extended: [24, 32, 24],
    sold: [28, 36, 28],
    recovering: 12,
    social: 8
  };
  navigator.vibrate?.(pattern[kind]);
}

export function playCueTone(ctx: AudioContext, kind: AtmosphereCue['kind']) {
  if (document.visibilityState === 'hidden') return;
  const oscillator = ctx.createOscillator();
  const gain = ctx.createGain();
  const frequency: Record<AtmosphereCue['kind'], number> = {
    leading: 880,
    outbid: 520,
    extended: 660,
    sold: 740,
    recovering: 440,
    social: 620
  };
  oscillator.type = 'sine';
  oscillator.frequency.value = frequency[kind];
  gain.gain.setValueAtTime(0.0001, ctx.currentTime);
  gain.gain.exponentialRampToValueAtTime(0.08, ctx.currentTime + 0.02);
  gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + 0.16);
  oscillator.connect(gain);
  gain.connect(ctx.destination);
  oscillator.start();
  oscillator.stop(ctx.currentTime + 0.18);
}

export function playCountdownTone(ctx: AudioContext, phase: CountdownPhase, beat = '') {
  if (document.visibilityState === 'hidden') return;
  if (!['critical', 'hammer'].includes(phase)) return;
  const oscillator = ctx.createOscillator();
  const gain = ctx.createGain();
  const base = phase === 'hammer' ? 720 : 560;
  const beatBoost = beat.includes('最后') ? 220 : beat ? 120 : 0;
  oscillator.type = phase === 'hammer' ? 'triangle' : 'sine';
  oscillator.frequency.value = base + beatBoost;
  gain.gain.setValueAtTime(0.0001, ctx.currentTime);
  gain.gain.exponentialRampToValueAtTime(phase === 'hammer' ? 0.075 : 0.045, ctx.currentTime + 0.012);
  gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + (phase === 'hammer' ? 0.22 : 0.12));
  oscillator.connect(gain);
  gain.connect(ctx.destination);
  oscillator.start();
  oscillator.stop(ctx.currentTime + (phase === 'hammer' ? 0.24 : 0.14));
}

export function playLayeredCue(ctx: AudioContext, kind: 'system_message' | 'rank_change' | 'result') {
  if (document.visibilityState === 'hidden') return;
  const now = ctx.currentTime;
  const master = ctx.createGain();
  master.gain.setValueAtTime(0.0001, now);
  master.gain.exponentialRampToValueAtTime(kind === 'result' ? 0.085 : 0.052, now + 0.02);
  master.gain.exponentialRampToValueAtTime(0.0001, now + 0.52);
  master.connect(ctx.destination);
  const notes: Record<typeof kind, number[]> = {
    system_message: [660, 880],
    rank_change: [520, 740, 980],
    result: [440, 660, 880]
  };
  notes[kind].forEach((frequency, index) => {
    const oscillator = ctx.createOscillator();
    const gain = ctx.createGain();
    oscillator.type = index === 0 ? 'triangle' : 'sine';
    oscillator.frequency.setValueAtTime(frequency, now + index * 0.07);
    gain.gain.setValueAtTime(0.0001, now + index * 0.07);
    gain.gain.exponentialRampToValueAtTime(0.045, now + index * 0.07 + 0.018);
    gain.gain.exponentialRampToValueAtTime(0.0001, now + index * 0.07 + 0.18);
    oscillator.connect(gain);
    gain.connect(master);
    oscillator.start(now + index * 0.07);
    oscillator.stop(now + index * 0.07 + 0.2);
  });
}

export function speakSystemMessage(message: string) {
  if (document.visibilityState === 'hidden') return false;
  if (!('speechSynthesis' in window) || !('SpeechSynthesisUtterance' in window)) return false;
  window.speechSynthesis.cancel();
  const utterance = new SpeechSynthesisUtterance(message.slice(0, 48));
  utterance.lang = 'zh-CN';
  utterance.rate = 1.05;
  utterance.pitch = 1.02;
  utterance.volume = 0.82;
  window.speechSynthesis.speak(utterance);
  return true;
}

export function vibrateCountdownPhase(phase: CountdownPhase, beat = '') {
  if (!('vibrate' in navigator) || isReducedMotionPreferred() || document.visibilityState === 'hidden') return;
  if (phase === 'critical') navigator.vibrate?.(14);
  if (phase === 'hammer') navigator.vibrate?.(beat.includes('最后') ? [24, 28, 24] : [18, 18, 18]);
}

export function buildResultRecap(input: {
  itemTitle: string;
  kind: 'winner' | 'loser' | 'unsold';
  terminalPriceCents: number;
  terminalWinnerID?: string;
  heat: HeatSnapshot;
  extendCount?: number;
  nextTitle?: string;
}): ResultRecap {
  const price = formatCents(input.terminalPriceCents);
  const status = input.kind === 'winner' ? '已拍中' : input.kind === 'loser' ? '已落锤' : '未成交';
  const winner = input.kind === 'unsold'
    ? '无成交买家'
    : input.kind === 'winner'
      ? '我'
      : input.terminalWinnerID ? `${input.terminalWinnerID.slice(0, 2)}**` : '领先者';
  const facts = [
    input.heat.acceptedBidderCount > 0 ? `${input.heat.acceptedBidderCount} 人有效出价` : '',
    input.heat.totalAcceptedBids != null ? `${input.heat.totalAcceptedBids} 口有效出价` : '',
    input.extendCount && input.extendCount > 0 ? `末段延时 ${input.extendCount} 次` : '',
    input.heat.priceVelocityCentsPerMin > 0 ? `${formatCents(input.heat.priceVelocityCentsPerMin)}/分` : ''
  ].filter(Boolean);
  const nextAction = input.kind === 'winner'
    ? '完成订单支付'
    : input.nextTitle
      ? `继续看 ${input.nextTitle}`
      : '回到商品列表';
  const shareCopy = `${input.itemTitle} · ${status} · ${price} · ${facts.join(' · ') || '真实竞拍记录'}`;
  return { title: input.itemTitle, status, price, winner, facts, nextAction, shareCopy };
}

export async function ensureDemoSession(account: 'host' | 'user') {
  const me = await fetch('/api/auth/me');
  if (me.ok) {
    const payload = await readJSON<{ user?: AuthUser }>(me);
    if (payload?.user) return payload.user;
  }
  const login = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ account })
  });
  if (!login.ok) {
    throw new Error(`login failed: ${login.status}`);
  }
  const payload = await readJSON<{ user?: AuthUser }>(login);
  if (!payload?.user) {
    throw new Error('login response missing user');
  }
  return payload.user;
}

export function rejectCopy(code?: string | null) {
  switch (code) {
    case 'BID_CONFIRMATION_PENDING':
      return '已进入确认队列';
    case 'BID_TOO_LOW':
      return '出价低于最低可出价';
    case 'BID_INCREMENT_MISMATCH':
      return '请按加价幅度出价';
    case 'REJECTED_SELF_LEADING':
      return '你已领先，无需重复出价';
    case 'BID_AUCTION_TOO_HOT':
    case 'BID_RETRY_LATER':
    case 'RATE_LIMITED':
      return '竞价激烈，请稍候';
    case 'PROCESSING_RETRY_LATER':
      return '正在确认上一笔出价';
    case 'IDEMPOTENCY_TIMEOUT':
      return '操作未确认，请重新出价';
    case 'AUCTION_ENDED':
      return '竞拍已结束，正在同步结果';
    case 'FORBIDDEN_ROOM':
      return '无法进入该直播间';
    case 'NETWORK_ERROR':
      return '网络异常，结果不确定';
    default:
      return '出价未通过，请重试';
  }
}

export function retryAfterMS(response: Response, payload?: BidResponse | null) {
  const detailMS = Number(payload?.details?.retry_after_ms ?? 0);
  if (Number.isFinite(detailMS) && detailMS > 0) return detailMS;
  const detailSeconds = Number(payload?.details?.retry_after_secs ?? 0);
  if (Number.isFinite(detailSeconds) && detailSeconds > 0) return detailSeconds * 1000;
  const headerSeconds = Number(response.headers.get('Retry-After') ?? 0);
  if (Number.isFinite(headerSeconds) && headerSeconds > 0) return headerSeconds * 1000;
  return 0;
}

export function retryAfterMSFromHeaders(response: Response) {
  const header = response.headers.get('Retry-After');
  if (!header) return 0;
  const seconds = Number(header);
  if (Number.isFinite(seconds) && seconds > 0) return seconds * 1000;
  const dateMS = Date.parse(header);
  return Number.isFinite(dateMS) ? Math.max(0, dateMS - Date.now()) : 0;
}

export function riskActionCopy(code?: string | null) {
  switch (code) {
    case 'BID_CONFIRMATION_PENDING':
      return '等待服务端确认，断线后用同一请求自动恢复';
    case 'BID_AUCTION_TOO_HOT':
    case 'BID_RETRY_LATER':
    case 'RATE_LIMITED':
      return '系统正在削峰，请等待提示恢复后再出价';
    case 'PROCESSING_RETRY_LATER':
      return '上一笔请求仍在处理，不要连续点击';
    case 'IDEMPOTENCY_TIMEOUT':
      return '上一笔结果不确定，请用新的出价请求重试';
    case 'BID_INCREMENT_MISMATCH':
    case 'BID_TOO_LOW':
      return '按服务端给出的最低有效价和加价幅度调整';
    case 'AUCTION_ENDED':
      return '等待服务端结果同步，当前不要继续提交';
    case 'NETWORK_ERROR':
      return '响应丢失不代表请求失败，请用同一请求重试确认';
    default:
      return '本次未成交，按当前权威价格重新确认';
  }
}

export function maxBidStatusCopy(intent: MaxBidIntent) {
  if (intent.status === 'ACTIVE') {
    const applied = intent.last_applied_seq ? ` · 已代出价 seq ${intent.last_applied_seq}` : '';
    return `Max Bid 已生效${applied}`;
  }
  if (intent.status === 'EXHAUSTED') return 'Max Bid 已被超越';
  if (intent.status === 'CANCELLED') return 'Max Bid 已取消';
  return '本场已结束，Max Bid 不再执行';
}

export function maxBidErrorCopy(code?: string | null) {
  switch (code) {
    case 'MAX_BID_TOO_LOW':
      return '最高价低于当前最低有效出价';
    case 'MAX_BID_INCREMENT_MISMATCH':
      return '最高价需要按加价幅度设置';
    case 'MAX_BID_ABOVE_CAP':
      return '最高价超过本场封顶价';
    case 'PROCESSING_RETRY_LATER':
      return '上一笔 Max Bid 仍在确认';
    case 'AUCTION_NOT_ACTIVE':
      return '当前拍品暂不能设置 Max Bid';
    default:
      return 'Max Bid 未确认，请重试';
  }
}

export function isDangerousActionDisabled(scenario: Scenario, connectionPhase: ConnectionPhase) {
  return scenario.ctaDisabled || scenario.stale || scenario.sold || connectionPhase === 'connecting' || connectionPhase === 'recovering' || connectionPhase === 'disconnected';
}

export function isTestMatrixEnabled() {
  // import.meta.env.DEV is false in production builds, so tree-shaking removes
  // the demo component entirely. URL param is still required as a deliberate activation
  // signal in development mode.
  return import.meta.env.DEV &&
    new URLSearchParams(window.location.search).get('stateMatrix') === '1';
}

export function roomIDFromPath() {
  const match = window.location.pathname.match(/^\/rooms\/([^/?#]+)/);
  return match ? decodeURIComponent(match[1]) : 'room_main';
}

export function auctionPriority(auction: AuctionSummary) {
  if (auction.status === 'ACTIVE') return 0;
  if (auction.status === 'SCHEDULED') return 1;
  if (auction.status === 'DRAFT') return 2;
  return 3;
}

export function visibleRoomAuctions(auctions: AuctionSummary[]) {
  return [...auctions].sort((left, right) => {
    const priority = auctionPriority(left) - auctionPriority(right);
    if (priority !== 0) return priority;
    return String(left.start_at ?? left.end_at ?? left.id).localeCompare(String(right.start_at ?? right.end_at ?? right.id));
  });
}

export function selectEntryAuction(auctions: AuctionSummary[]) {
  return visibleRoomAuctions(auctions).find((auction) => ['ACTIVE', 'SCHEDULED', 'DRAFT'].includes(String(auction.status)))
    ?? visibleRoomAuctions(auctions)[0];
}
