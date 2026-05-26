import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { AlertTriangle, BadgeCheck, Bell, BellOff, CheckCircle2, ChevronUp, Clock3, CreditCard, History, MessageCircle, PackageCheck, Radio, RefreshCw, Send, ShieldCheck, Truck, Trophy, Wifi, WifiOff } from 'lucide-react';
import type { AtmosphereCue, AtmosphereInput } from './atmosphere';
import { normalizeAtmosphere } from './atmosphere';
import './styles.css';

type AuctionState =
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

type Scenario = {
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
  pending?: boolean;
  rejected?: boolean;
  winner?: boolean;
  sold?: boolean;
};

type BidderRequirement = {
  verification_required?: boolean;
  deposit_required?: boolean;
  verified?: boolean;
  deposit_held?: boolean;
  reason?: string;
};

type BidPhase = 'idle' | 'pending' | 'accepted' | 'rejected' | 'confirm_required' | 'confirming';
type PaymentPhase = 'idle' | 'pending' | 'paid' | 'failed' | 'expired';
type RecoveryPhase = 'idle' | 'recovering';
type ConnectionPhase = 'connecting' | 'connected' | 'recovering' | 'disconnected';
type BottomSheetKey = 'products' | 'details' | 'leaderboard' | 'history' | 'orders';
type ResultSheetKind = 'winner' | 'loser' | 'unsold';
type SoundCapability = 'ready' | 'unavailable' | 'blocked';

type BidResponse = {
  result?: string;
  auction_id?: string;
  seq?: number;
  current_price_cents?: number;
  current_winner_id?: string;
  end_at?: string;
  server_time_ms?: number;
  reject_reason?: string | null;
  code?: string;
  confirm_token?: string;
  expires_in_ms?: number;
  amount_cents?: number;
};

type AuctionRealtimeEvent = {
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

type SnapshotResponse = {
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

type AuctionItem = {
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

type HistoryRow = Record<string, unknown>;
type ChatMessage = {
  id: number;
  room_id: string;
  user_id: string;
  body: string;
  created_at: string;
};
type OrderRow = {
  order_id?: string;
  auction_id?: string;
  amount_cents?: number;
  order_status?: string;
};
type AuctionSummary = {
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
type LeaderboardEntry = {
  rank: number;
  user_id: string;
  user_masked: string;
  amount_cents: number;
  bid_count: number;
  is_current?: boolean;
};
type LeaderboardPayload = {
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
type WSTicketResponse = {
  ticket?: string;
  expires_in_ms?: number;
};
type AuthUser = {
  ID: string;
  Role: string;
};

const demoUserID = 'user_1';

const scenarios: Scenario[] = [
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

function formatCents(cents: number) {
  return `¥${(cents / 100).toFixed(2)}`;
}

function formatRemaining(ms: number) {
  if (ms <= 10_000) {
    const tenths = Math.max(0, Math.ceil(ms / 100) / 10);
    return `00:${tenths.toFixed(1).padStart(4, '0')}`;
  }
  const totalSeconds = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

function deriveCountdown(endAt: string, serverTimeMS: number, nowMS: number, terminal: boolean, stale: boolean, extended: boolean) {
  if (terminal) return '已结束';
  if (!endAt || serverTimeMS <= 0) return stale ? '剩余时间待同步' : '等待服务端时间';
  const endAtMS = Date.parse(endAt);
  if (!Number.isFinite(endAtMS)) return '等待服务端时间';
  const elapsed = Math.max(0, nowMS - serverTimeMS);
  const remaining = endAtMS - serverTimeMS - elapsed;
  if (remaining <= 0) return stale ? '本地到零，正在同步' : '到点同步中';
  return `${extended ? '延时后' : '剩余'} ${formatRemaining(remaining)}`;
}

function formatClockTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '--:--:--';
  return date.toLocaleTimeString('zh-CN', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function extensionCopyFromEvent(detail: AuctionRealtimeEvent, oldEndAt: string, nextEndAt: string) {
  const oldCopy = formatClockTime(detail.payload?.old_end_at ?? oldEndAt);
  const nextCopy = formatClockTime(nextEndAt);
  const count = detail.payload?.extend_count;
  const max = detail.payload?.max_extend_count;
  const countCopy = count != null && max != null ? ` · 第 ${count}/${max} 次` : count != null ? ` · 第 ${count} 次` : '';
  return `延时 ${oldCopy} -> ${nextCopy}${countCopy}`;
}

function isCountdownExpired(endAt: string, serverTimeMS: number, nowMS: number) {
  if (!endAt || serverTimeMS <= 0) return false;
  const endAtMS = Date.parse(endAt);
  if (!Number.isFinite(endAtMS)) return false;
  return endAtMS - serverTimeMS - Math.max(0, nowMS - serverTimeMS) <= 0;
}

function createClientBidID() {
  if (globalThis.crypto?.randomUUID) {
    return globalThis.crypto.randomUUID();
  }
  return `bid_${Date.now()}_${Math.random().toString(16).slice(2)}`;
}

async function readJSON<T>(response: Response): Promise<T | null> {
  try {
    return await response.json() as T;
  } catch {
    return null;
  }
}

function responseServerTimeMS(response: Response) {
  const dateHeader = response.headers.get('date');
  if (!dateHeader) return 0;
  const parsed = Date.parse(dateHeader);
  return Number.isFinite(parsed) ? parsed : 0;
}

function leaderboardCopy(payload: LeaderboardPayload | null) {
  if (!payload || !payload.entries?.length) return '等待首个有效出价';
  if (payload.my_rank === 1) return '你正在领先';
  if (payload.my_rank && payload.gap_to_leader_cents != null) {
    return `第 ${payload.my_rank} 名 · 差 ${formatCents(payload.gap_to_leader_cents)}`;
  }
  return `${payload.accepted_bidder_count} 人已有效出价`;
}

function leaderboardActionCopy(payload: LeaderboardPayload | null, fallbackBidCents: number) {
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

function createAudioContext() {
  const AudioContextCtor = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
  if (!AudioContextCtor) return null;
  return new AudioContextCtor();
}

function isReducedMotionPreferred() {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
}

function vibratePattern(kind: AtmosphereCue['kind']) {
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

function playCueTone(ctx: AudioContext, kind: AtmosphereCue['kind']) {
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

async function ensureDemoSession(account: 'host' | 'user') {
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

function rejectCopy(code?: string | null) {
  switch (code) {
    case 'BID_TOO_LOW':
      return '出价低于最低可出价';
    case 'BID_INCREMENT_MISMATCH':
      return '请按加价幅度出价';
    case 'REJECTED_SELF_LEADING':
      return '你已领先，无需重复出价';
    case 'BID_AUCTION_TOO_HOT':
      return '竞价激烈，请稍候';
    case 'PROCESSING_RETRY_LATER':
      return '正在确认上一笔出价';
    case 'IDEMPOTENCY_TIMEOUT':
      return '操作未确认，请重新出价';
    case 'AUCTION_ENDED':
      return '竞拍已结束，正在同步结果';
    case 'FORBIDDEN_ROOM':
      return '无法进入该直播间';
    default:
      return '出价未通过，请重试';
  }
}

function isTestMatrixEnabled() {
  return new URLSearchParams(window.location.search).get('stateMatrix') === '1';
}

function roomIDFromPath() {
  const match = window.location.pathname.match(/^\/rooms\/([^/?#]+)/);
  return match ? decodeURIComponent(match[1]) : 'room_main';
}

function App() {
  const showStateMatrix = useMemo(isTestMatrixEnabled, []);
  const roomID = useMemo(roomIDFromPath, []);
  const [selected, setSelected] = useState<AuctionState>('active_bids');
  const [bidPhase, setBidPhase] = useState<BidPhase>('idle');
  const [paymentPhase, setPaymentPhase] = useState<PaymentPhase>('idle');
  const [recoveryPhase, setRecoveryPhase] = useState<RecoveryPhase>('idle');
  const [currentPriceCents, setCurrentPriceCents] = useState(35_000);
  const [minimumNextBidCents, setMinimumNextBidCents] = useState(40_000);
  const [nextBidCents, setNextBidCents] = useState(40_000);
  const [lastSeq, setLastSeq] = useState(41);
  const [bidFeedback, setBidFeedback] = useState('下一口 ¥400.00');
  const [leaderMasked, setLeaderMasked] = useState('张**');
  const [confirmToken, setConfirmToken] = useState('');
  const [confirmIdempotencyKey, setConfirmIdempotencyKey] = useState('');
  const [confirmAmountCents, setConfirmAmountCents] = useState(0);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState('');
  const [bidHistory, setBidHistory] = useState<HistoryRow[]>([]);
  const [orderHistory, setOrderHistory] = useState<HistoryRow[]>([]);
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([]);
  const [chatDraft, setChatDraft] = useState('');
  const [chatSending, setChatSending] = useState(false);
  const [leaderboard, setLeaderboard] = useState<LeaderboardPayload | null>(null);
  const [atmosphereCue, setAtmosphereCue] = useState<AtmosphereCue | null>(null);
  const [soundEnabled, setSoundEnabled] = useState(false);
  const [soundCapability, setSoundCapability] = useState<SoundCapability>('ready');
  const [connectionPhase, setConnectionPhase] = useState<ConnectionPhase>('connecting');
  const [activeAuctionID, setActiveAuctionID] = useState('');
  const [activeIncrementCents, setActiveIncrementCents] = useState(5_000);
  const [payableOrderID, setPayableOrderID] = useState('');
  const [payableOrderAmountCents, setPayableOrderAmountCents] = useState(0);
  const [terminalPriceCents, setTerminalPriceCents] = useState(0);
  const [terminalWinnerID, setTerminalWinnerID] = useState('');
  const [auctionEndAt, setAuctionEndAt] = useState('');
  const [serverTimeMS, setServerTimeMS] = useState(0);
  const [nowMS, setNowMS] = useState(Date.now());
  const [extensionNotice, setExtensionNotice] = useState('');
  const [currentUserID, setCurrentUserID] = useState(demoUserID);
  const [sessionReady, setSessionReady] = useState(false);
  const [lotTitle, setLotTitle] = useState('青瓷手作茶盏');
  const [roomAuctions, setRoomAuctions] = useState<AuctionSummary[]>([]);
  const [activeSheet, setActiveSheet] = useState<BottomSheetKey | null>(null);
  const [stageItem, setStageItem] = useState<AuctionItem>({
    title: '青瓷手作茶盏',
    image_url: '',
    certificate: '证书待同步',
    condition: '品相待同步',
    shipping: '运费以订单为准'
  });
  const [bidderRequirement, setBidderRequirement] = useState<BidderRequirement | null>(null);
  const paymentInFlight = useRef(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const lastSeqRef = useRef(lastSeq);
  const currentPriceRef = useRef(currentPriceCents);
  const leaderMaskedRef = useRef(leaderMasked);
  const activeAuctionIDRef = useRef(activeAuctionID);
  const activeIncrementCentsRef = useRef(activeIncrementCents);
  const auctionEndAtRef = useRef(auctionEndAt);
  const serverTimeMSRef = useRef(serverTimeMS);
  const currentUserIDRef = useRef(currentUserID);
  const soundEnabledRef = useRef(soundEnabled);
  const audioContextRef = useRef<AudioContext | null>(null);
  const soundCapabilityRef = useRef<SoundCapability>('ready');
  const leaderboardRef = useRef<LeaderboardPayload | null>(leaderboard);
  const atmosphereSeenRef = useRef<Set<string>>(new Set());
  const recoveringRef = useRef(false);
  const activeCueRef = useRef<AtmosphereCue | null>(null);

  useEffect(() => {
    lastSeqRef.current = lastSeq;
  }, [lastSeq]);

  useEffect(() => {
    currentPriceRef.current = currentPriceCents;
  }, [currentPriceCents]);

  useEffect(() => {
    leaderMaskedRef.current = leaderMasked;
  }, [leaderMasked]);

  useEffect(() => {
    activeAuctionIDRef.current = activeAuctionID;
  }, [activeAuctionID]);

  useEffect(() => {
    activeIncrementCentsRef.current = activeIncrementCents;
  }, [activeIncrementCents]);

  useEffect(() => {
    auctionEndAtRef.current = auctionEndAt;
  }, [auctionEndAt]);

  useEffect(() => {
    serverTimeMSRef.current = serverTimeMS;
  }, [serverTimeMS]);

  useEffect(() => {
    currentUserIDRef.current = currentUserID;
  }, [currentUserID]);

  useEffect(() => {
    soundEnabledRef.current = soundEnabled;
  }, [soundEnabled]);

  useEffect(() => {
    soundCapabilityRef.current = soundCapability;
  }, [soundCapability]);

  useEffect(() => {
    leaderboardRef.current = leaderboard;
  }, [leaderboard]);

  useEffect(() => {
    recoveringRef.current = recoveryPhase === 'recovering' || connectionPhase === 'recovering' || connectionPhase === 'disconnected';
  }, [connectionPhase, recoveryPhase]);

  useEffect(() => {
    const timer = window.setInterval(() => setNowMS(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const countdownCopy = useMemo(() => {
    const stale = connectionPhase === 'disconnected' || recoveryPhase === 'recovering' || !activeAuctionID;
    const terminal = selected === 'sold_winner' || selected === 'sold_loser' || selected === 'ended' || selected === 'cancelled';
    return deriveCountdown(auctionEndAt, serverTimeMS, nowMS, terminal, stale, Boolean(extensionNotice));
  }, [activeAuctionID, auctionEndAt, connectionPhase, extensionNotice, nowMS, recoveryPhase, selected, serverTimeMS]);
  const countdownExpired = useMemo(() => (
    selected === 'active_bids' &&
    connectionPhase === 'connected' &&
    recoveryPhase === 'idle' &&
    isCountdownExpired(auctionEndAt, serverTimeMS, nowMS)
  ), [auctionEndAt, connectionPhase, nowMS, recoveryPhase, selected, serverTimeMS]);

  const showAtmosphere = (input: AtmosphereInput) => {
    if (recoveringRef.current) return;
    const cue = normalizeAtmosphere(input, lastSeqRef.current);
    if (!cue) return;
    if (cue.cause_seq <= lastSeqRef.current && input.cause_seq == null) return;
    const key = `${cue.auction_id}:${cue.event_type}:${cue.cause_seq}:${cue.user_scope}:${cue.kind}`;
    if (atmosphereSeenRef.current.has(key)) return;
    atmosphereSeenRef.current.add(key);
    const activeCue = activeCueRef.current;
    if (activeCue && activeCue.auction_id === cue.auction_id && activeCue.cause_seq === cue.cause_seq && activeCue.priority > cue.priority) {
      return;
    }
    activeCueRef.current = cue;
    setAtmosphereCue(cue);
    if (soundEnabledRef.current && audioContextRef.current && soundCapabilityRef.current === 'ready') {
      playCueTone(audioContextRef.current, cue.kind);
      vibratePattern(cue.kind);
    }
  };

  const toggleSound = async () => {
    if (soundEnabledRef.current) {
      setSoundEnabled(false);
      audioContextRef.current?.close?.().catch?.(() => undefined);
      audioContextRef.current = null;
      return;
    }
    const ctx = createAudioContext();
    if (!ctx) {
      setSoundCapability('unavailable');
      setSoundEnabled(false);
      return;
    }
    try {
      if (ctx.state === 'suspended') await ctx.resume();
      audioContextRef.current = ctx;
      setSoundCapability('ready');
      setSoundEnabled(true);
    } catch {
      setSoundCapability('blocked');
      setSoundEnabled(false);
      await ctx.close?.();
    }
  };

  useEffect(() => {
    return () => {
      audioContextRef.current?.close?.().catch?.(() => undefined);
      audioContextRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!atmosphereCue) return;
    const timer = window.setTimeout(() => setAtmosphereCue(null), 1800);
    return () => {
      window.clearTimeout(timer);
      if (activeCueRef.current?.id === atmosphereCue.id) activeCueRef.current = null;
    };
  }, [atmosphereCue]);

  useEffect(() => {
    let cancelled = false;
    ensureDemoSession('user')
      .then((user) => {
        if (!cancelled) {
          setCurrentUserID(user.ID);
          setSessionReady(true);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setConnectionPhase('disconnected');
          setBidFeedback('登录失败，请刷新重试');
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const scenario = useMemo<Scenario>(() => {
    if (selected === 'sold_winner') {
      const soldPrice = formatCents(payableOrderAmountCents || terminalPriceCents || currentPriceCents);
      if (paymentPhase === 'pending') {
        return {
          key: 'sold_winner',
          title: '支付中',
          status: 'SOLD',
          price: soldPrice,
          leader: '你已拍中',
          feedback: '等待服务端确认支付',
          countdown: '支付确认中',
          cta: '支付中',
          ctaDisabled: true,
          winner: true,
          sold: true
        };
      }
      if (paymentPhase === 'paid') {
        return {
          key: 'sold_winner',
          title: '已支付',
          status: 'PAID',
          price: soldPrice,
          leader: '订单已支付',
          feedback: '保证金已处理',
          countdown: '已支付',
          cta: '已支付',
          ctaDisabled: true,
          winner: true,
          sold: true
        };
      }
      if (paymentPhase === 'failed') {
        return {
          key: 'sold_winner',
          title: '支付失败',
          status: 'SOLD',
          price: soldPrice,
          leader: '你已拍中',
          feedback: '支付未确认，请重试',
          countdown: '订单仍待支付',
          cta: '重新支付',
          ctaDisabled: false,
          winner: true,
          sold: true
        };
      }
      if (paymentPhase === 'expired') {
        return {
          key: 'sold_winner',
          title: '订单已超时',
          status: 'ORDER_EXPIRED',
          price: soldPrice,
          leader: '支付窗口关闭',
          feedback: '订单已超时',
          countdown: '订单已超时',
          cta: '已超时',
          ctaDisabled: true,
          winner: true,
          sold: true
        };
      }
      return {
        key: 'sold_winner',
        title: '成交',
        status: 'SOLD',
        price: soldPrice,
        leader: '你已拍中',
        feedback: payableOrderID ? '订单待支付' : '正在同步订单',
        countdown: payableOrderID ? '支付倒计时以订单为准' : '订单同步中',
        cta: payableOrderID ? '去支付' : '同步订单中',
        ctaDisabled: !payableOrderID,
        winner: true,
        sold: true
      };
    }
    if (selected === 'sold_loser') {
      return {
        key: 'sold_loser',
        title: '已成交',
        status: 'SOLD',
        price: formatCents(terminalPriceCents || currentPriceCents),
        leader: terminalWinnerID ? `${terminalWinnerID.slice(0, 2)}** 拍中` : '已拍出',
        feedback: '本场已结束',
        countdown: '已落锤',
        cta: '已结束',
        ctaDisabled: true,
        sold: true
      };
    }
    if (selected !== 'active_bids') {
      return scenarios.find((item) => item.key === selected) ?? scenarios[0];
    }
    const verificationBlocked = Boolean(
      bidderRequirement &&
      ((bidderRequirement.verification_required && !bidderRequirement.verified) ||
        (bidderRequirement.deposit_required && !bidderRequirement.deposit_held))
    );
    if (verificationBlocked) {
      const requiredLabels = [
        bidderRequirement?.verification_required && !bidderRequirement?.verified ? '实名/买家验证' : '',
        bidderRequirement?.deposit_required && !bidderRequirement?.deposit_held ? '保证金冻结' : ''
      ].filter(Boolean).join(' + ');
      return {
        key: 'active_bids' as AuctionState,
        title: '需完成验证',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: bidderRequirement?.reason || `出价前需要完成 ${requiredLabels}`,
        countdown: countdownCopy,
        cta: '需完成验证',
        ctaDisabled: true
      };
    }
    if (!activeAuctionID) {
      return {
        key: 'recovering',
        title: '同步中',
        status: 'RECOVERING',
        price: formatCents(currentPriceCents),
        leader: '同步中',
        feedback: '正在读取当前拍卖',
        countdown: '剩余时间待同步',
        cta: '同步中',
        ctaDisabled: true,
        stale: true
      };
    }
    if (connectionPhase === 'disconnected') {
      return {
        key: 'disconnected',
        title: '已断开',
        status: 'DISCONNECTED',
        price: formatCents(currentPriceCents),
        leader: '重连中',
        feedback: '正在重新连接',
        countdown: countdownCopy,
        cta: '重连中',
        ctaDisabled: true,
        stale: true
      };
    }
    if (recoveryPhase === 'recovering') {
      return {
        key: 'recovering',
        title: '恢复中',
        status: 'RECOVERING',
        price: formatCents(currentPriceCents),
        leader: '同步中',
        feedback: '正在同步权威状态',
        countdown: countdownCopy,
        cta: '同步中',
        ctaDisabled: true,
        stale: true
      };
    }
    if (bidPhase === 'pending') {
      return {
        key: 'pending' as AuctionState,
        title: '提交中',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: '等待服务端确认',
        countdown: countdownCopy,
        cta: '确认中',
        ctaDisabled: true,
        pending: true
      };
    }
    if (bidPhase === 'confirming') {
      return {
        key: 'pending' as AuctionState,
        title: '确认中',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: '等待服务端确认高额出价',
        countdown: countdownCopy,
        cta: '确认中',
        ctaDisabled: true,
        pending: true
      };
    }
    if (bidPhase === 'confirm_required') {
      return {
        key: 'active_bids' as AuctionState,
        title: '高额确认',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: `确认 ${formatCents(confirmAmountCents)} 出价`,
        countdown: countdownCopy,
        cta: '确认高额出价',
        ctaDisabled: false
      };
    }
    if (bidPhase === 'accepted') {
      return {
        key: 'self_leading' as AuctionState,
        title: '领先中',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: '你已领先',
        feedback: `服务端确认 seq ${lastSeq}`,
        countdown: countdownCopy,
        cta: '已领先',
        ctaDisabled: true
      };
    }
    if (bidPhase === 'rejected') {
      return {
        key: 'rejected' as AuctionState,
        title: '被拒绝',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: countdownExpired ? '到点同步服务端结果' : bidFeedback,
        countdown: countdownCopy,
        cta: countdownExpired ? '同步中' : `出价 ${formatCents(nextBidCents)}`,
        ctaDisabled: countdownExpired,
        rejected: true
      };
    }
    return {
      key: 'active_bids' as AuctionState,
      title: '竞价中',
      status: 'ACTIVE',
      price: formatCents(currentPriceCents),
      leader: `${leaderMasked} 领先`,
      feedback: countdownExpired ? '到点同步服务端结果' : bidFeedback,
      countdown: countdownCopy,
      cta: countdownExpired ? '同步中' : `出价 ${formatCents(nextBidCents)}`,
      ctaDisabled: countdownExpired
    };
  }, [activeAuctionID, bidFeedback, bidderRequirement, bidPhase, confirmAmountCents, connectionPhase, countdownCopy, countdownExpired, currentPriceCents, lastSeq, leaderMasked, minimumNextBidCents, nextBidCents, payableOrderAmountCents, payableOrderID, paymentPhase, recoveryPhase, selected, terminalPriceCents, terminalWinnerID]);
  const resultSheetKind: ResultSheetKind | null = selected === 'sold_winner'
    ? 'winner'
    : selected === 'sold_loser'
      ? 'loser'
      : selected === 'ended'
        ? 'unsold'
        : null;
  const nextAuction = useMemo(() => roomAuctions.find((row) => (
    row.id !== activeAuctionID && (row.status === 'SCHEDULED' || row.status === 'DRAFT')
  )), [activeAuctionID, roomAuctions]);

  const applyAcceptedBid = (payload: BidResponse) => {
    const acceptedPrice = payload.current_price_cents ?? currentPriceCents;
    const acceptedWinnerID = payload.current_winner_id ?? '';
    setCurrentPriceCents(acceptedPrice);
    setMinimumNextBidCents(acceptedPrice + activeIncrementCents);
    setNextBidCents(acceptedPrice + activeIncrementCents);
    setLastSeq(payload.seq ?? lastSeq);
    if (payload.end_at) setAuctionEndAt(payload.end_at);
    if (payload.server_time_ms) setServerTimeMS(payload.server_time_ms);
    if (payload.result === 'ACCEPTED_EXTENDED') {
      setExtensionNotice('服务端已延时');
      setBidFeedback(`服务端已延时 seq ${payload.seq ?? lastSeq}`);
      showAtmosphere({
        kind: 'extended',
        title: '已延时',
        detail: '最后窗口出价，服务端延长竞拍',
        auction_id: payload.auction_id ?? activeAuctionIDRef.current,
        cause_seq: payload.seq ?? lastSeqRef.current,
        event_type: payload.result,
        user_scope: 'global'
      });
    }
    setConfirmToken('');
    setConfirmIdempotencyKey('');
    setConfirmAmountCents(0);
    if (payload.result === 'ACCEPTED_SOLD') {
      setTerminalPriceCents(acceptedPrice);
      setTerminalWinnerID(payload.current_winner_id ?? '');
      setSelected(payload.current_winner_id === currentUserID ? 'sold_winner' : 'sold_loser');
      showAtmosphere({
        kind: 'sold',
        title: payload.current_winner_id === currentUserID ? '成交！' : '已成交',
        detail: payload.current_winner_id === currentUserID ? '你已拍中，订单待支付' : '本场已落锤',
        auction_id: payload.auction_id ?? activeAuctionIDRef.current,
        cause_seq: payload.seq ?? lastSeqRef.current,
        event_type: payload.result,
        user_scope: payload.current_winner_id === currentUserID ? 'self' : 'other'
      });
      setBidPhase('idle');
      void loadLeaderboard(payload.auction_id ?? activeAuctionIDRef.current);
      void loadPayableOrderForAuction(payload.auction_id ?? activeAuctionIDRef.current);
      return;
    }
    if (acceptedWinnerID === currentUserID) {
      showAtmosphere({
        kind: 'leading',
        title: '领先！',
        detail: `${formatCents(acceptedPrice)} 服务端确认`,
        auction_id: payload.auction_id ?? activeAuctionIDRef.current,
        cause_seq: payload.seq ?? lastSeqRef.current,
        event_type: payload.result ?? 'bid_accepted',
        user_scope: 'self'
      });
    }
    setBidPhase(acceptedWinnerID === currentUserID ? 'accepted' : 'idle');
    void loadLeaderboard(payload.auction_id ?? activeAuctionIDRef.current);
  };

  const loadPayableOrderForAuction = async (auctionID: string) => {
    if (!auctionID) return null;
    try {
      const response = await fetch('/api/users/me/orders');
      const payload = await readJSON<{ items?: OrderRow[] }>(response);
      if (!response.ok) return null;
      const pendingOrder = (payload?.items ?? []).find((row) => (
        String(row.order_status ?? '') === 'ORDER_PENDING' && String(row.auction_id ?? '') === auctionID
      ));
      setPayableOrderID(pendingOrder?.order_id ? String(pendingOrder.order_id) : '');
      setPayableOrderAmountCents(Number(pendingOrder?.amount_cents ?? 0));
      return pendingOrder ?? null;
    } catch {
      setPayableOrderID('');
      setPayableOrderAmountCents(0);
      return null;
    }
  };

  const applySnapshot = (snapshot: SnapshotResponse) => {
    const price = snapshot.payload?.current_price_cents ?? snapshot.current_price_cents ?? currentPriceCents;
    const increment = snapshot.increment_cents ?? activeIncrementCents;
    const snapshotAuctionID = snapshot.auction_id ?? snapshot.id;
    const nextEndAt = snapshot.payload?.end_at ?? snapshot.end_at;
    const nextServerTimeMS = snapshot.payload?.server_time_ms ?? snapshot.server_time_ms;
    if (snapshotAuctionID) setActiveAuctionID(snapshotAuctionID);
    const snapshotItem = snapshot.payload?.item ?? snapshot.item;
    if (snapshotItem) {
      setStageItem((current) => ({ ...current, ...snapshotItem }));
      if (snapshotItem.title) setLotTitle(snapshotItem.title);
    }
    setBidderRequirement(snapshot.payload?.bidder_requirement ?? snapshot.bidder_requirement ?? null);
    if (snapshot.increment_cents != null) setActiveIncrementCents(increment);
    if (nextEndAt) setAuctionEndAt(nextEndAt);
    if (nextServerTimeMS) setServerTimeMS(nextServerTimeMS);
    setCurrentPriceCents(price);
    setMinimumNextBidCents(price + increment);
    setNextBidCents(price + increment);
    setLastSeq(snapshot.seq);
    setLeaderMasked(snapshot.payload?.leader_user_masked ?? leaderMasked);
    setBidFeedback(`snapshot ${snapshot.source ?? 'db'} seq ${snapshot.seq}`);
    setBidPhase('idle');
    const status = snapshot.payload?.status ?? snapshot.status;
    const winnerID = snapshot.payload?.current_winner_id ?? snapshot.current_winner_id;
    if (status === 'SOLD') {
      setTerminalPriceCents(price);
      setTerminalWinnerID(winnerID ?? '');
      setSelected(winnerID === currentUserID ? 'sold_winner' : 'sold_loser');
      if (winnerID === currentUserID) {
        void loadPayableOrderForAuction(snapshotAuctionID ?? activeAuctionIDRef.current);
      }
    } else if (status === 'ENDED') {
      setSelected('ended');
    } else if (status === 'CANCELLED') {
      setSelected('cancelled');
      setBidFeedback(snapshot.payload?.reason ?? '主播已取消');
    }
    void loadLeaderboard(snapshotAuctionID ?? activeAuctionIDRef.current);
  };

  const recoverFromSnapshot = async () => {
    const auctionID = activeAuctionIDRef.current;
    if (!auctionID) return;
    setSelected('active_bids');
    setRecoveryPhase('recovering');
    setConnectionPhase((phase) => phase === 'disconnected' ? phase : 'recovering');
    try {
      const response = await fetch(`/api/auctions/${auctionID}`);
      const snapshot = await readJSON<SnapshotResponse>(response);
      if (!response.ok || !snapshot || snapshot.stale) {
        setBidFeedback('snapshot stale，继续同步');
        return;
      }
      if (!snapshot.server_time_ms && !snapshot.payload?.server_time_ms) {
        snapshot.server_time_ms = responseServerTimeMS(response);
      }
      applySnapshot(snapshot);
      setRecoveryPhase('idle');
      setConnectionPhase('connected');
    } catch {
      setBidFeedback('snapshot unavailable，继续同步');
    }
  };

  useEffect(() => {
    if (!countdownExpired) return;
    setBidFeedback('本地倒计时到零，正在同步服务端结果');
    void recoverFromSnapshot();
  }, [countdownExpired]);

  const handleRealtimeEvent = (detail: AuctionRealtimeEvent) => {
    if (!detail || detail.auction_id !== activeAuctionIDRef.current) return;
    const currentSeq = lastSeqRef.current;
    if (detail.event_type === 'outbox_gap_notice' || (detail.seq != null && detail.seq > currentSeq + 1)) {
      void recoverFromSnapshot();
      return;
    }
    if (detail.seq == null || detail.seq <= currentSeq) return;
    const price = detail.payload?.current_price_cents ?? detail.payload?.amount_cents ?? currentPriceRef.current;
    const increment = activeIncrementCentsRef.current;
    const nextEndAt = detail.payload?.end_at ?? detail.end_at;
    const nextServerTimeMS = detail.payload?.server_time_ms ?? detail.server_time_ms;
    const previousEndAt = auctionEndAtRef.current;
    const previousLeading = leaderboardRef.current?.my_rank === 1;
    const winnerID = detail.payload?.current_winner_id ?? detail.payload?.user_id ?? '';
    setCurrentPriceCents(price);
    setMinimumNextBidCents(price + increment);
    setNextBidCents((prepared) => Math.max(price + increment, prepared));
    setLastSeq(detail.seq);
    if (nextEndAt) setAuctionEndAt(nextEndAt);
    if (nextServerTimeMS) setServerTimeMS(nextServerTimeMS);
    setLeaderMasked(detail.payload?.leader_user_masked ?? leaderMaskedRef.current);
    const wasExtended = Boolean(nextEndAt && previousEndAt && Date.parse(nextEndAt) > Date.parse(previousEndAt));
    if (wasExtended && nextEndAt) {
      setExtensionNotice(extensionCopyFromEvent(detail, previousEndAt, nextEndAt));
      setBidFeedback(`服务端已延时 seq ${detail.seq}`);
      showAtmosphere({
        kind: 'extended',
        title: '已延时',
        detail: '最后窗口出价，竞拍继续',
        auction_id: detail.auction_id,
        cause_seq: detail.seq,
        event_type: detail.event_type,
        user_scope: 'global'
      });
    } else {
      setBidFeedback(`event seq ${detail.seq}`);
    }
    if (detail.event_type === 'auction_sold') {
      setTerminalPriceCents(price);
      setTerminalWinnerID(winnerID);
      if (detail.payload?.order_id && winnerID === currentUserID) {
        setPayableOrderID(detail.payload.order_id);
        setPayableOrderAmountCents(price);
      }
      setSelected(winnerID === currentUserID ? 'sold_winner' : 'sold_loser');
      showAtmosphere({
        kind: 'sold',
        title: winnerID === currentUserID ? '成交！' : '已成交',
        detail: winnerID === currentUserID ? '你已拍中，订单待支付' : '本场已落锤',
        auction_id: detail.auction_id,
        cause_seq: detail.seq,
        event_type: detail.event_type,
        user_scope: winnerID === currentUserID ? 'self' : 'other'
      });
      setBidPhase('idle');
      if (winnerID === currentUserID) {
        void loadPayableOrderForAuction(detail.auction_id);
      }
    } else if (detail.event_type === 'auction_ended') {
      setSelected('ended');
      setBidPhase('idle');
    } else if (detail.event_type === 'auction_cancelled') {
      setSelected('cancelled');
      setBidFeedback(detail.payload?.reason ?? '主播已取消');
      setBidPhase('idle');
    } else if (detail.event_type === 'order_paid') {
      if (detail.payload?.user_id === currentUserID) {
        setPaymentPhase('paid');
        setSelected('sold_winner');
        if (detail.payload?.order_id) setPayableOrderID(detail.payload.order_id);
      }
      setBidFeedback('订单状态已同步');
    } else if (detail.event_type === 'order_expired') {
      if (detail.payload?.user_id === currentUserID) {
        setPaymentPhase('expired');
        setSelected('sold_winner');
        if (detail.payload?.order_id) setPayableOrderID(detail.payload.order_id);
      }
      setBidFeedback('订单已超时');
    } else if (detail.payload?.user_id && detail.payload.user_id !== currentUserID) {
      if (previousLeading) {
        showAtmosphere({
          kind: 'outbid',
          title: '被超越！',
          detail: `${detail.payload.leader_user_masked ?? '其他用户'} 已领先`,
          auction_id: detail.auction_id,
          cause_seq: detail.seq,
          event_type: detail.event_type,
          user_scope: 'self'
        });
      }
      setBidPhase('idle');
      setConfirmToken('');
      setConfirmIdempotencyKey('');
      setConfirmAmountCents(0);
    } else if (winnerID === currentUserIDRef.current || detail.payload?.current_winner_id === currentUserIDRef.current) {
      showAtmosphere({
        kind: 'leading',
        title: '领先！',
        detail: `${formatCents(price)} 已同步`,
        auction_id: detail.auction_id,
        cause_seq: detail.seq,
        event_type: detail.event_type,
        user_scope: 'self'
      });
    }
    void loadLeaderboard(detail.auction_id);
    setConnectionPhase('connected');
  };

  useEffect(() => {
    let cancelled = false;
    const loadActiveAuction = async () => {
      if (!sessionReady) return;
      try {
        const response = await fetch(`/api/rooms/${roomID}/auctions`);
        const payload = await readJSON<AuctionSummary[] | { items?: AuctionSummary[] }>(response);
        const auctions = Array.isArray(payload) ? payload : payload?.items ?? [];
        const selectedAuction = auctions.find((item) => item.status === 'ACTIVE') ?? auctions[0];
        if (!response.ok || !selectedAuction || cancelled) return;
        setRoomAuctions(auctions);
        setActiveAuctionID(selectedAuction.id);
        setLotTitle(selectedAuction.item?.title ?? selectedAuction.id);
        setStageItem((current) => ({
          ...current,
          ...(selectedAuction.item ?? {}),
          title: selectedAuction.item?.title ?? current.title ?? selectedAuction.id
        }));
        setBidderRequirement(selectedAuction.bidder_requirement ?? null);
        const price = selectedAuction.current_price_cents ?? currentPriceRef.current;
        const increment = selectedAuction.increment_cents ?? activeIncrementCents;
        setActiveIncrementCents(increment);
        setCurrentPriceCents(price);
        setMinimumNextBidCents(price + increment);
        setNextBidCents(price + increment);
        setLastSeq(selectedAuction.seq ?? lastSeqRef.current);
        setAuctionEndAt(selectedAuction.end_at ?? '');
        setServerTimeMS(selectedAuction.server_time_ms ?? 0);
        setBidFeedback(`auction ${selectedAuction.id}`);
        void loadLeaderboard(selectedAuction.id);
        try {
          const snapshotResponse = await fetch(`/api/auctions/${selectedAuction.id}`);
          const snapshot = await readJSON<SnapshotResponse>(snapshotResponse);
          if (snapshotResponse.ok && snapshot && !snapshot.stale) {
            if (!snapshot.server_time_ms && !snapshot.payload?.server_time_ms) {
              snapshot.server_time_ms = responseServerTimeMS(snapshotResponse);
            }
            applySnapshot(snapshot);
          }
        } catch {
          setBidFeedback(`auction ${selectedAuction.id}`);
        }
      } catch {
        setBidFeedback('auction list unavailable');
      }
    };
    void loadActiveAuction();
    return () => {
      cancelled = true;
    };
  }, [sessionReady]);

  useEffect(() => {
    let cancelled = false;
    const loadPayableOrder = async () => {
      if (!sessionReady || !activeAuctionID) return;
      const order = await loadPayableOrderForAuction(activeAuctionID);
      if (cancelled || order) return;
    };
    void loadPayableOrder();
    return () => {
      cancelled = true;
    };
  }, [activeAuctionID, sessionReady]);

  useEffect(() => {
    let cancelled = false;
    const loadChat = async () => {
      if (!sessionReady) return;
      try {
        const response = await fetch(`/api/rooms/${roomID}/chat?limit=30`);
        const payload = await readJSON<{ items?: ChatMessage[] }>(response);
        if (!response.ok || cancelled) return;
        setChatMessages((payload?.items ?? []).slice().reverse());
      } catch {
        setChatMessages([]);
      }
    };
    void loadChat();
    const timer = window.setInterval(loadChat, 5_000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [sessionReady]);

  useEffect(() => {
    const onAuctionEvent = (event: Event) => {
      const detail = (event as CustomEvent<AuctionRealtimeEvent>).detail;
      handleRealtimeEvent(detail);
    };
    window.addEventListener('auction:event', onAuctionEvent);
    return () => window.removeEventListener('auction:event', onAuctionEvent);
  }, []);

  useEffect(() => {
    let cancelled = false;
    const clearReconnect = () => {
      if (reconnectTimerRef.current != null) {
        window.clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
    };
    const scheduleReconnect = () => {
      clearReconnect();
      reconnectTimerRef.current = window.setTimeout(() => {
        if (!cancelled) void connectWebSocket();
      }, 2_000);
    };
    const connectWebSocket = async () => {
      if (!sessionReady || !activeAuctionID) return;
      setConnectionPhase((phase) => phase === 'recovering' ? phase : 'connecting');
      try {
        const ticketResponse = await fetch('/api/auth/ws-ticket', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify({ room_id: roomID, auction_id: activeAuctionID })
        });
        const ticketPayload = await ticketResponse.json() as WSTicketResponse;
        if (!ticketResponse.ok || !ticketPayload.ticket) {
          throw new Error('ws ticket unavailable');
        }
        if (cancelled) return;
        const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
        const wsURL = `${scheme}://${window.location.host}/ws?room_id=${encodeURIComponent(roomID)}&auction_id=${encodeURIComponent(activeAuctionID)}&last_seq=${lastSeqRef.current}`;
        const socket = new WebSocket(wsURL, ['auction.v1', `ticket.${ticketPayload.ticket}`]);
        wsRef.current = socket;
        socket.onopen = () => {
          if (!cancelled) setConnectionPhase('connected');
        };
        socket.onmessage = (message) => {
          try {
            handleRealtimeEvent(JSON.parse(String(message.data)) as AuctionRealtimeEvent);
          } catch {
            setBidFeedback('实时消息解析失败，正在同步');
            void recoverFromSnapshot();
          }
        };
        socket.onerror = () => {
          if (!cancelled) {
            setConnectionPhase('disconnected');
          }
        };
        socket.onclose = () => {
          if (!cancelled) {
            setConnectionPhase('disconnected');
            scheduleReconnect();
          }
        };
      } catch {
        if (!cancelled) {
          setConnectionPhase('disconnected');
          scheduleReconnect();
        }
      }
    };

    if (sessionReady && activeAuctionID) {
      void connectWebSocket();
      void loadLeaderboard(activeAuctionID);
    }
    return () => {
      cancelled = true;
      clearReconnect();
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, [activeAuctionID, sessionReady]);

  const submitBid = async () => {
    const auctionID = activeAuctionIDRef.current;
    if (selected !== 'active_bids' || scenario.ctaDisabled || !auctionID) return;
    const clientBidID = createClientBidID();
    setBidPhase('pending');
    try {
      const response = await fetch(`/api/auctions/${auctionID}/bids`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': clientBidID
        },
        body: JSON.stringify({
          client_bid_id: clientBidID,
          amount_cents: nextBidCents,
          client_seen_seq: lastSeq
        })
      });
      const payload = await response.json() as BidResponse;
      if (payload.result === 'FAT_FINGER_CONFIRM_REQUIRED' && payload.confirm_token) {
        setConfirmToken(payload.confirm_token);
        setConfirmIdempotencyKey(clientBidID);
        setConfirmAmountCents(payload.amount_cents ?? nextBidCents);
        setBidFeedback('高额出价需要二次确认');
        setBidPhase('confirm_required');
        return;
      }
      if (!response.ok || payload.reject_reason || payload.code) {
        setBidFeedback(rejectCopy(payload.reject_reason ?? payload.code));
        setBidPhase('rejected');
        return;
      }
      applyAcceptedBid(payload);
    } catch {
      setBidFeedback('网络异常，请重试');
      setBidPhase('rejected');
    }
  };

  const confirmBid = async () => {
    const auctionID = activeAuctionIDRef.current;
    if (!confirmToken || !confirmIdempotencyKey || scenario.ctaDisabled || !auctionID) return;
    setBidPhase('confirming');
    try {
      const response = await fetch(`/api/auctions/${auctionID}/bids/confirm`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': confirmIdempotencyKey
        },
        body: JSON.stringify({
          confirm_token: confirmToken,
          idempotency_key: confirmIdempotencyKey
        })
      });
      const payload = await response.json() as BidResponse;
      if (!response.ok || payload.reject_reason || payload.code) {
        setBidFeedback(rejectCopy(payload.reject_reason ?? payload.code));
        setBidPhase('rejected');
        return;
      }
      applyAcceptedBid(payload);
    } catch {
      setBidFeedback('网络异常，请重试');
      setBidPhase('rejected');
    }
  };

  const payOrder = async () => {
    if (selected !== 'sold_winner' || scenario.ctaDisabled || paymentInFlight.current || !payableOrderID) return;
    const idempotencyKey = createClientBidID();
    paymentInFlight.current = true;
    setPaymentPhase('pending');
    try {
      const response = await fetch(`/api/orders/${payableOrderID}/pay-mock`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': idempotencyKey
        },
        body: JSON.stringify({ confirm: true })
      });
      if (!response.ok) {
        setPaymentPhase('failed');
        return;
      }
      const payload = await response.json() as { order_status?: string };
      setPaymentPhase(payload.order_status === 'PAID' ? 'paid' : 'failed');
    } catch {
      setPaymentPhase('failed');
    } finally {
      paymentInFlight.current = false;
    }
  };

  const loadLeaderboard = async (auctionID = activeAuctionIDRef.current) => {
    if (!auctionID) return;
    try {
      const response = await fetch(`/api/auctions/${auctionID}/leaderboard?limit=5`);
      const payload = await readJSON<LeaderboardPayload>(response);
      if (response.ok && payload) {
        setLeaderboard(payload);
      }
    } catch {
      setLeaderboard(null);
    }
  };

  const handlePrimaryAction = () => {
    if (selected === 'sold_winner') {
      void payOrder();
      return;
    }
    if (bidPhase === 'confirm_required') {
      void confirmBid();
      return;
    }
    void submitBid();
  };

  const decreaseBidAmount = () => {
    setNextBidCents((amount) => Math.max(minimumNextBidCents, amount - activeIncrementCents));
  };

  const increaseBidAmount = () => {
    setNextBidCents((amount) => amount + activeIncrementCents);
  };

  const loadHistory = async () => {
    setHistoryLoading(true);
    setHistoryError('');
    try {
      const [bids, orders] = await Promise.all([
        fetch('/api/users/me/bids').then((response) => response.json()),
        fetch('/api/users/me/orders').then((response) => response.json())
      ]);
      setBidHistory(Array.isArray(bids.items) ? bids.items : []);
      const orderRows = Array.isArray(orders.items) ? orders.items : [];
      setOrderHistory(orderRows);
      const currentAuctionID = activeAuctionIDRef.current;
      const pendingOrder = orderRows.find((row: HistoryRow) => (
        String(row.order_status ?? '') === 'ORDER_PENDING' && String(row.auction_id ?? '') === currentAuctionID
      ));
      if (pendingOrder?.order_id) {
        setPayableOrderID(String(pendingOrder.order_id));
        setPayableOrderAmountCents(Number(pendingOrder.amount_cents ?? 0));
      }
    } catch {
      setHistoryError('历史读取失败');
    } finally {
      setHistoryLoading(false);
    }
  };

  const sendChat = async () => {
    const body = chatDraft.trim();
    if (!body || chatSending) return;
    setChatSending(true);
    try {
      const response = await fetch(`/api/rooms/${roomID}/chat`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          client_msg_id: createClientBidID(),
          body
        })
      });
      const payload = await readJSON<ChatMessage>(response);
      if (response.ok && payload) {
        setChatMessages((current) => [...current.slice(-29), payload]);
        setChatDraft('');
      }
    } finally {
      setChatSending(false);
    }
  };

  return (
    <main className="app-shell" data-perf-surface={new URLSearchParams(window.location.search).get('perfSurface') === '1' ? '1' : undefined}>
      <LiveStage
        atmosphereCue={atmosphereCue}
        chatMessages={chatMessages}
        connectionPhase={connectionPhase}
        countdownCopy={countdownCopy}
        currentUserID={currentUserID}
        item={stageItem}
        lotTitle={lotTitle}
        roomID={roomID}
        scenario={scenario}
        soundEnabled={soundEnabled}
        soundCapability={soundCapability}
        onToggleSound={() => void toggleSound()}
      />
      <AuctionStatePanel
        atmosphereCue={atmosphereCue}
        connectionPhase={connectionPhase}
        countdownCopy={countdownCopy}
        currentPriceCents={currentPriceCents}
        extensionNotice={extensionNotice}
        leaderboard={leaderboard}
        minimumNextBidCents={minimumNextBidCents}
        nextBidCents={nextBidCents}
        scenario={scenario}
        onDecreaseBid={decreaseBidAmount}
        onIncreaseBid={increaseBidAmount}
        onOpenSheet={setActiveSheet}
        onPrimaryAction={handlePrimaryAction}
      />
      {showStateMatrix && (
        <StateMatrixTabs selected={selected} onSelect={setSelected} />
      )}
      {showStateMatrix ? (
        <LeaderboardPanel
          activeAuctionID={activeAuctionID}
          leaderboard={leaderboard}
          nextBidCents={nextBidCents}
          onRefresh={() => void loadLeaderboard()}
        />
      ) : (
        <BottomSheet
          activeAuctionID={activeAuctionID}
          activeSheet={activeSheet}
          auctions={roomAuctions}
          bidHistory={bidHistory}
          historyError={historyError}
          historyLoading={historyLoading}
          item={stageItem}
          leaderboard={leaderboard}
          nextBidCents={nextBidCents}
          orderHistory={orderHistory}
          scenario={scenario}
          onClose={() => setActiveSheet(null)}
          onOpenSheet={setActiveSheet}
          onRefreshHistory={loadHistory}
          onRefreshLeaderboard={() => void loadLeaderboard()}
        />
      )}
      <ResultSheet
        activeSheet={activeSheet}
        kind={resultSheetKind}
        nextAuction={nextAuction}
        paymentPhase={paymentPhase}
        scenario={scenario}
        terminalPriceCents={terminalPriceCents || currentPriceCents}
        terminalWinnerID={terminalWinnerID}
        userBestCents={leaderboard?.my_best_amount_cents ?? 0}
        orderID={payableOrderID}
        orderAmountCents={payableOrderAmountCents}
        onOpenOrders={() => setActiveSheet('orders')}
        onPay={payOrder}
      />
      {showStateMatrix && (
        <ChatPanel
          chatDraft={chatDraft}
          chatMessages={chatMessages}
          chatSending={chatSending}
          currentUserID={currentUserID}
          onDraftChange={setChatDraft}
          onSend={sendChat}
        />
      )}
      {!showStateMatrix && (
        <ChatComposer
        chatDraft={chatDraft}
        chatSending={chatSending}
        onDraftChange={setChatDraft}
        onSend={sendChat}
        />
      )}
    </main>
  );
}

function LiveStage({
  atmosphereCue,
  chatMessages,
  connectionPhase,
  countdownCopy,
  currentUserID,
  item,
  lotTitle,
  roomID,
  scenario,
  soundEnabled,
  soundCapability,
  onToggleSound
}: {
  atmosphereCue: AtmosphereCue | null;
  chatMessages: ChatMessage[];
  connectionPhase: ConnectionPhase;
  countdownCopy: string;
  currentUserID: string;
  item: AuctionItem;
  lotTitle: string;
  roomID: string;
  scenario: Scenario;
  soundEnabled: boolean;
  soundCapability: SoundCapability;
  onToggleSound: () => void;
}) {
  const mediaURL = item.video_poster_url ?? item.videoPosterURL ?? item.image_url ?? item.imageURL ?? '';
  const proofChips = [
    { icon: <BadgeCheck size={13} />, label: item.certificate ?? '证书可查' },
    { icon: <PackageCheck size={13} />, label: item.condition ?? '品相已验' },
    { icon: <Truck size={13} />, label: item.shipping ?? '包邮保价' },
    { icon: <ShieldCheck size={13} />, label: '保证金锁定' }
  ];
  const visibleChat = chatMessages.slice(-3);
  const connectionCopy = connectionPhase === 'connected'
    ? '已连接'
    : connectionPhase === 'recovering'
      ? '同步中'
      : connectionPhase === 'connecting'
        ? '连接中'
        : '已断开';

  return (
    <section
      className={`video-stage ${mediaURL ? 'has-media' : 'no-media'}`}
      aria-label="live-stage"
      data-testid="live-stage"
      data-atmosphere-kind={atmosphereCue?.kind ?? 'none'}
      style={mediaURL ? { '--stage-media-url': `url("${mediaURL}")` } as React.CSSProperties : undefined}
    >
      {atmosphereCue && (
        <div className="atmosphere-effect-layer" aria-hidden="true">
          <span className="effect-leading-ring" />
          <span className="effect-outbid-edge" />
          <span className="effect-hammer-mark" />
        </div>
      )}
      {atmosphereCue && (
        <div
          className={`atmosphere-cue ${atmosphereCue.kind}`}
          role="status"
          aria-live="polite"
          key={atmosphereCue.id}
          data-testid="atmosphere-cue"
          data-auction-id={atmosphereCue.auction_id}
          data-cause-seq={atmosphereCue.cause_seq}
          data-event-type={atmosphereCue.event_type}
          data-user-scope={atmosphereCue.user_scope}
        >
          <strong>{atmosphereCue.title}</strong>
          <span>{atmosphereCue.detail}</span>
        </div>
      )}
      <div className="video-topbar">
        <span className="live-pill"><Radio size={14} /> LIVE</span>
        <span className="viewer-count">{roomID}</span>
        <span className="viewer-count"><Wifi size={13} /> {connectionCopy}</span>
        <button
          className="sound-toggle"
          type="button"
          aria-label={soundEnabled ? '关闭提示音' : soundCapability === 'ready' ? '开启提示音' : '提示音不可用'}
          title={soundCapability === 'blocked' ? '浏览器阻止音频，请再次点击授权' : soundCapability === 'unavailable' ? '当前浏览器不支持提示音' : undefined}
          disabled={soundCapability === 'unavailable'}
          onClick={onToggleSound}
        >
          {soundEnabled ? <Bell size={14} /> : <BellOff size={14} />}
        </button>
      </div>
      <div className="stage-safe-zone">
        <div className="proof-chip-row" aria-label="product-proof">
          {proofChips.map((chip) => (
            <span className="proof-chip" key={chip.label}>{chip.icon}{chip.label}</span>
          ))}
        </div>
        <div className="stage-chat-overlay" data-testid="stage-chat-overlay" aria-label="live-chat-overlay">
          {visibleChat.length === 0 ? (
            <span className="stage-chat-empty">等待实时弹幕</span>
          ) : visibleChat.map((message) => (
            <span className="stage-chat-line" key={message.id}>
              <strong>{message.user_id === currentUserID ? '我' : `${message.user_id.slice(0, 2)}**`}</strong>
              {message.body}
            </span>
          ))}
        </div>
        <div className="focus-copy">
          <h1>{lotTitle}</h1>
          <p>Lot A-102 · {scenario.countdown ?? countdownCopy}</p>
        </div>
      </div>
    </section>
  );
}

function ChatComposer({
  chatDraft,
  chatSending,
  onDraftChange,
  onSend
}: {
  chatDraft: string;
  chatSending: boolean;
  onDraftChange: (value: string) => void;
  onSend: () => void;
}) {
  return (
    <section className="chat-composer" data-testid="chat-panel">
      <div className="chat-input-row">
        <input aria-label="chat-input" value={chatDraft} onChange={(event) => onDraftChange(event.currentTarget.value)} placeholder="和主播互动" />
        <button type="button" aria-label="send-chat" disabled={chatSending || !chatDraft.trim()} onClick={onSend}>
          <Send size={16} />
        </button>
      </div>
    </section>
  );
}

function AuctionStatePanel({
  atmosphereCue,
  connectionPhase,
  countdownCopy,
  currentPriceCents,
  extensionNotice,
  leaderboard,
  minimumNextBidCents,
  nextBidCents,
  scenario,
  onDecreaseBid,
  onIncreaseBid,
  onOpenSheet,
  onPrimaryAction
}: {
  atmosphereCue: AtmosphereCue | null;
  connectionPhase: ConnectionPhase;
  countdownCopy: string;
  currentPriceCents: number;
  extensionNotice: string;
  leaderboard: LeaderboardPayload | null;
  minimumNextBidCents: number;
  nextBidCents: number;
  scenario: Scenario;
  onDecreaseBid: () => void;
  onIncreaseBid: () => void;
  onOpenSheet: (sheet: BottomSheetKey) => void;
  onPrimaryAction: () => void;
}) {
  const dockState = scenario.pending
    ? 'PENDING'
    : scenario.stale || connectionPhase === 'disconnected' || connectionPhase === 'recovering'
      ? 'RECOVERING'
      : scenario.winner
        ? 'SOLD_WINNER'
        : scenario.sold
          ? 'SOLD_LOSER'
          : scenario.rejected
            ? 'OUTBID'
            : scenario.ctaDisabled
              ? 'SELF_LEADING'
              : 'ACTIVE';
  const rankCopy = leaderboard?.my_rank != null
    ? `我的排名 #${leaderboard.my_rank}`
    : '出价后显示排名';
  const gapCopy = leaderboard?.gap_to_leader_cents != null && leaderboard.gap_to_leader_cents > 0
    ? `差 ${formatCents(leaderboard.gap_to_leader_cents)}`
    : leaderboard?.my_rank === 1
      ? '当前领先'
      : scenario.feedback;
  const rankAction = leaderboardActionCopy(leaderboard, nextBidCents);
  const bidHint = (() => {
    if (scenario.stale || connectionPhase === 'recovering' || connectionPhase === 'disconnected') return '权威价格同步中，暂不提交出价';
    if (scenario.title === '需完成验证') return scenario.feedback;
    if (scenario.ctaDisabled && !scenario.sold && scenario.leader.includes('你')) return '当前您已是最高价，等待其他用户出价';
    if (nextBidCents > minimumNextBidCents) return `高于当前价 ${formatCents(nextBidCents - currentPriceCents)} · 高于最低下一口 ${formatCents(nextBidCents - minimumNextBidCents)}`;
    return `最低有效出价 ${formatCents(minimumNextBidCents)} · 按 ${formatCents(Math.max(0, nextBidCents - currentPriceCents))} 加价`;
  })();

  return (
    <section
      className={`auction-panel bid-dock ${scenario.stale ? 'is-stale' : ''}`}
      aria-label="auction-state"
      data-dock-state={dockState}
      data-atmosphere-kind={atmosphereCue?.kind ?? 'none'}
    >
      <div className="dock-price-row">
        <div>
          <p className="eyebrow">{scenario.title}</p>
          <h2 data-testid="auction-price">{scenario.price}</h2>
        </div>
        <div className="countdown-row" data-testid="auction-countdown" data-effect={atmosphereCue?.kind === 'extended' ? 'extension-stretch' : 'none'}>
          <Clock3 size={16} />
          <span>{scenario.countdown ?? countdownCopy}</span>
          {extensionNotice && !scenario.sold && <strong>{extensionNotice}</strong>}
        </div>
      </div>
      <div className="dock-rank-row">
        <span className="status-chip" data-state={scenario.status}>{scenario.status}</span>
        <span>{scenario.leader}</span>
        <strong>{rankCopy} · {gapCopy}</strong>
      </div>
      <div className="rank-strip" data-testid="rank-strip">
        <span>{rankAction.headline}</span>
        <strong>{rankAction.action}</strong>
        <em>{rankAction.freshness}</em>
      </div>
      <div className="signal-row">
        {scenario.stale || connectionPhase === 'disconnected' ? <WifiOff size={16} /> : <Wifi size={16} />}
        <span>{scenario.stale ? '状态可能已过期' : connectionPhase === 'connected' ? 'WebSocket 已连接 · 状态来自服务端事件' : 'WebSocket 连接中 · 状态来自服务端事件'}</span>
      </div>
      <div className="dock-feedback" aria-live={scenario.rejected || scenario.stale ? 'assertive' : 'polite'}>
        <span>{scenario.feedback} · <strong data-testid="bid-hint">{bidHint}</strong></span>
      </div>
      <div className="bid-stepper">
        <button type="button" aria-label="decrease" onClick={onDecreaseBid}>-</button>
        <span>{scenario.sold ? 'ORDER' : formatCents(nextBidCents)}</span>
        <button type="button" aria-label="increase" onClick={onIncreaseBid}><ChevronUp size={18} /></button>
      </div>
      <button className="primary-cta" data-testid="bid-cta" disabled={scenario.ctaDisabled} onClick={onPrimaryAction}>
        {scenario.winner ? <CreditCard size={18} /> : scenario.rejected ? <AlertTriangle size={18} /> : <CheckCircle2 size={18} />}
        {scenario.cta}
      </button>
      <div className="dock-shortcuts" aria-label="bid-dock-shortcuts">
        <button type="button" onClick={() => onOpenSheet('products')}>商品</button>
        <button type="button" onClick={() => onOpenSheet('details')}>规则</button>
        <button type="button" onClick={() => onOpenSheet('leaderboard')}>榜单</button>
        <button type="button" onClick={() => onOpenSheet('history')}>历史</button>
        <button type="button" onClick={() => onOpenSheet('orders')}>订单</button>
      </div>
    </section>
  );
}

function ResultSheet({
  activeSheet,
  kind,
  nextAuction,
  orderAmountCents,
  orderID,
  paymentPhase,
  scenario,
  terminalPriceCents,
  terminalWinnerID,
  userBestCents,
  onOpenOrders,
  onPay
}: {
  activeSheet: BottomSheetKey | null;
  kind: ResultSheetKind | null;
  nextAuction?: AuctionSummary;
  orderAmountCents: number;
  orderID: string;
  paymentPhase: PaymentPhase;
  scenario: Scenario;
  terminalPriceCents: number;
  terminalWinnerID: string;
  userBestCents: number;
  onOpenOrders: () => void;
  onPay: () => void;
}) {
  if (!kind || activeSheet) return null;
  const soldPrice = formatCents(orderAmountCents || terminalPriceCents);
  const nextTitle = nextAuction?.item?.title ?? '下一件拍品';
  const nextPrice = nextAuction ? formatCents(nextAuction.current_price_cents ?? 0) : '';
  const nextStatus = nextAuction?.status ?? '';
  const gapCents = Math.max(0, terminalPriceCents - userBestCents);
  const isPaymentDisabled = scenario.ctaDisabled || paymentPhase === 'pending' || paymentPhase === 'paid' || paymentPhase === 'expired' || !orderID;
  const title = kind === 'winner'
    ? paymentPhase === 'paid'
      ? '支付已完成'
      : paymentPhase === 'expired'
        ? '支付窗口已关闭'
        : '恭喜拍中'
    : kind === 'loser'
      ? '本场已落锤'
      : '本场未成交';

  return (
    <section className={`result-sheet ${kind}`} data-testid="result-sheet" aria-label={title}>
      <div className="result-sheet-icon" aria-hidden="true">
        {kind === 'winner' ? <Trophy size={22} /> : kind === 'loser' ? <Clock3 size={22} /> : <AlertTriangle size={22} />}
      </div>
      <div className="result-sheet-copy">
        <p className="eyebrow">{kind === 'winner' ? '成交结果' : kind === 'loser' ? '输家承接' : '未成交说明'}</p>
        <h2>{title}</h2>
        {kind === 'winner' && (
          <>
            <p>成交价 {soldPrice}。订单 {orderID || '同步中'} 已锁定，支付状态：{paymentPhase === 'paid' ? '已支付' : paymentPhase === 'pending' ? '确认中' : paymentPhase === 'expired' ? '已超时' : '待支付'}。</p>
            <p>保证金会随订单状态处理；支付成功后订单完成，未支付超时会关闭支付窗口。</p>
          </>
        )}
        {kind === 'loser' && (
          <>
            <p>{terminalWinnerID ? `${terminalWinnerID.slice(0, 2)}**` : '领先者'} 以 {formatCents(terminalPriceCents)} 拍中。{gapCents > 0 ? `你距离成交差 ${formatCents(gapCents)}。` : '你未在最后价格领先。'}</p>
            <p>可继续关注 {nextTitle}，本场历史会保留在出价记录中。下一件来自当前直播间拍品列表，不是库存预留或个性化推荐。</p>
          </>
        )}
        {kind === 'unsold' && (
          <>
            <p>本场没有形成有效成交，出价入口已关闭，不会生成订单。</p>
            <p>{nextAuction ? `${nextTitle} 即将开始，可回到商品列表继续观看。` : '暂无下一件排期，稍后回到直播间。'}</p>
          </>
        )}
      </div>
      {kind !== 'winner' && nextAuction ? (
        <div className="next-auction-card" data-testid="next-auction-handoff">
          <span>Room list handoff</span>
          <strong>{nextTitle}</strong>
          <p>{nextStatus} · 当前/起拍 {nextPrice}</p>
          <small>仅展示同直播间下一件可见拍品；未承诺相似度、库存预留或中标优先权。</small>
        </div>
      ) : null}
      <div className="result-actions">
        {kind === 'winner' ? (
          <>
            <button type="button" data-testid="result-pay-cta" disabled={isPaymentDisabled} onClick={onPay}>
              {paymentPhase === 'paid' ? '已支付' : paymentPhase === 'pending' ? '支付确认中' : paymentPhase === 'expired' ? '已超时' : '立即支付'}
            </button>
            <button type="button" onClick={onOpenOrders}>查看订单</button>
          </>
        ) : (
          <>
            <button type="button" onClick={onOpenOrders}>{kind === 'loser' ? '查看出价记录' : '查看商品列表'}</button>
            <span>{nextAuction ? `下一件：${nextTitle}` : '等待主播切换下一件'}</span>
          </>
        )}
      </div>
    </section>
  );
}

function BottomSheet({
  activeAuctionID,
  activeSheet,
  auctions,
  bidHistory,
  historyError,
  historyLoading,
  item,
  leaderboard,
  nextBidCents,
  orderHistory,
  scenario,
  onClose,
  onOpenSheet,
  onRefreshHistory,
  onRefreshLeaderboard
}: {
  activeAuctionID: string;
  activeSheet: BottomSheetKey | null;
  auctions: AuctionSummary[];
  bidHistory: HistoryRow[];
  historyError: string;
  historyLoading: boolean;
  item: AuctionItem;
  leaderboard: LeaderboardPayload | null;
  nextBidCents: number;
  orderHistory: HistoryRow[];
  scenario: Scenario;
  onClose: () => void;
  onOpenSheet: (sheet: BottomSheetKey) => void;
  onRefreshHistory: () => void;
  onRefreshLeaderboard: () => void;
}) {
  if (!activeSheet) return null;
  const titleMap: Record<BottomSheetKey, string> = {
    products: '本场商品',
    details: '商品与规则',
    leaderboard: '实时榜单',
    history: '我的出价',
    orders: '我的订单'
  };
  return (
    <div className="sheet-backdrop" data-testid="bottom-sheet-backdrop" onClick={onClose}>
      <section className="bottom-sheet" data-testid="bottom-sheet" aria-label={titleMap[activeSheet]} onClick={(event) => event.stopPropagation()}>
        <div className="sheet-handle" aria-hidden="true" />
        <div className="sheet-header">
          <h2>{titleMap[activeSheet]}</h2>
          <button type="button" aria-label="关闭面板" onClick={onClose}>关闭</button>
        </div>
        <div className="sheet-tabs" role="tablist" aria-label="sheet-tabs">
          {([
            ['products', '商品'],
            ['details', '规则'],
            ['leaderboard', '榜单'],
            ['history', '历史'],
            ['orders', '订单']
          ] as Array<[BottomSheetKey, string]>).map(([key, label]) => (
            <button type="button" role="tab" aria-selected={activeSheet === key} key={key} onClick={() => onOpenSheet(key)}>{label}</button>
          ))}
        </div>
        <div className="sheet-content">
          {activeSheet === 'products' && <ProductListSheet auctions={auctions} activeAuctionID={activeAuctionID} scenario={scenario} />}
          {activeSheet === 'details' && <ProductRuleSheet item={item} auction={auctions.find((row) => row.id === activeAuctionID)} scenario={scenario} />}
          {activeSheet === 'leaderboard' && <LeaderboardSheet activeAuctionID={activeAuctionID} leaderboard={leaderboard} nextBidCents={nextBidCents} onRefresh={onRefreshLeaderboard} />}
          {activeSheet === 'history' && (
            <HistorySheet
              title="出价历史"
              empty="暂无出价"
              rows={bidHistory}
              historyError={historyError}
              historyLoading={historyLoading}
              onRefresh={onRefreshHistory}
              getPrimary={(row) => String(row.auction_id ?? row.bid_id ?? '-')}
              getSecondary={(row) => `${formatCents(Number(row.amount_cents ?? 0))} · ${String(row.result ?? row.status ?? '-')}`}
            />
          )}
          {activeSheet === 'orders' && (
            <HistorySheet
              title="订单"
              empty="暂无订单"
              rows={orderHistory}
              historyError={historyError}
              historyLoading={historyLoading}
              onRefresh={onRefreshHistory}
              getPrimary={(row) => String(row.order_id ?? row.auction_id ?? '-')}
              getSecondary={(row) => `${formatCents(Number(row.amount_cents ?? 0))} · ${String(row.order_status ?? '-')}`}
            />
          )}
        </div>
      </section>
    </div>
  );
}

function ProductListSheet({
  activeAuctionID,
  auctions,
  scenario
}: {
  activeAuctionID: string;
  auctions: AuctionSummary[];
  scenario: Scenario;
}) {
  const rows = auctions.length > 0 ? auctions : [{ id: activeAuctionID || 'current', status: scenario.status, item: { title: scenario.title } }];
  return (
    <div className="product-card-list">
      {rows.map((auction) => {
        const status = auction.id === activeAuctionID ? '竞拍中' : auction.status === 'SCHEDULED' || auction.status === 'DRAFT' ? '即将开拍' : auction.status === 'SOLD' ? '已成交' : auction.status === 'ENDED' ? '已结束' : auction.status === 'CANCELLED' ? '已取消' : String(auction.status ?? '排队中');
        return (
          <article className={`product-card ${auction.id === activeAuctionID ? 'is-active' : ''}`} key={auction.id}>
            <div>
              <strong>{auction.item?.title ?? auction.item_id ?? auction.id}</strong>
              <span>{status} · {formatCents(auction.current_price_cents ?? 0)} · {auction.accepted_bid_count ?? 0} 口</span>
            </div>
            <em>{auction.id === activeAuctionID ? '当前拍品' : status}</em>
          </article>
        );
      })}
    </div>
  );
}

function ProductRuleSheet({ auction, item, scenario }: { auction?: AuctionSummary; item: AuctionItem; scenario: Scenario }) {
  const mediaURL = item.video_poster_url ?? item.videoPosterURL ?? item.image_url ?? item.imageURL ?? '';
  const depositFloor = auction?.rule?.deposit_floor_cents ?? 0;
  const depositCap = auction?.rule?.deposit_cap_cents ?? 0;
  const depositBps = auction?.rule?.deposit_bps ?? 0;
  const depositCopy = depositFloor > 0 || depositBps > 0
    ? `本场要求保证金，最低 ${formatCents(depositFloor)}${depositCap > 0 ? `，最高 ${formatCents(depositCap)}` : ''}。未拍中或订单完成后按支付链路处理。`
    : '本场未展示固定保证金门槛；以服务端出价校验和订单状态为准。';
  const extensionCopy = `最后 ${auction?.rule?.extend_window_seconds ?? 10} 秒内有有效出价，会自动延长 ${auction?.rule?.extend_by_seconds ?? 10} 秒${auction?.rule?.max_extend_count ? `，最多 ${auction.rule.max_extend_count} 次` : ''}，避免最后一秒抢拍。`;
  const capCopy = auction?.cap_price_cents
    ? `价格到达 ${formatCents(auction.cap_price_cents)} 后不再继续抬价。`
    : '本场未设置展示封顶价，仍由服务端规则校验每次出价。';
  const confirmationCopy = auction?.rule?.fat_finger_threshold_cents
    ? `单次高额跳价达到 ${formatCents(auction.rule.fat_finger_threshold_cents)} 会触发确认，防止误触。`
    : '高额确认由服务端按风险规则判断。';
  const proofItems = [
    ['证书', item.certificate ?? '证书待同步'],
    ['品相', item.condition ?? '品相待同步'],
    ['尺寸', item.dimensions ?? '尺寸待同步'],
    ['材质', item.material ?? '材质待同步'],
    ['瑕疵', item.flaws ?? '未登记明显瑕疵'],
    ['运费', item.shipping ?? '运费以订单为准']
  ];

  return (
    <div className="product-rule-sheet">
      <div className="trust-hero">
        <div className={`trust-media ${mediaURL ? 'has-media' : ''}`} style={mediaURL ? { '--trust-media-url': `url("${mediaURL}")` } as React.CSSProperties : undefined}>
          {!mediaURL && <span>商品图待同步</span>}
        </div>
        <div>
          <p className="eyebrow">商品信任详情</p>
          <h3>{item.title ?? scenario.title}</h3>
          <p>{item.description ?? '主播讲解与证据材料会随当前拍品同步，出价前请确认品相、保证金和延时规则。'}</p>
        </div>
      </div>
      <div className="trust-proof-grid" aria-label="product-trust-proof">
        {proofItems.map(([label, value]) => (
          <span key={label}>
            {label}
            <strong>{value}</strong>
          </span>
        ))}
      </div>
      <div className="trust-rule-list">
        <article>
          <strong>当前出价节奏</strong>
          <p>当前价 {scenario.price}，每次至少加 {formatCents(auction?.increment_cents ?? 0)}。{capCopy}</p>
        </article>
        <article>
          <strong>保证金与支付</strong>
          <p>{depositCopy}</p>
        </article>
        <article>
          <strong>延时保护</strong>
          <p>{extensionCopy}</p>
        </article>
        <article>
          <strong>误触保护</strong>
          <p>{confirmationCopy}</p>
        </article>
        <article>
          <strong>售后口径</strong>
          <p>{item.return_policy ?? '成交后以订单支付状态和商家售后口径为准，保证金处理会在订单状态中体现。'}</p>
        </article>
      </div>
    </div>
  );
}

function LeaderboardSheet({ activeAuctionID, leaderboard, nextBidCents, onRefresh }: { activeAuctionID: string; leaderboard: LeaderboardPayload | null; nextBidCents: number; onRefresh: () => void }) {
  const entries = leaderboard?.entries ?? [];
  const actionCopy = leaderboardActionCopy(leaderboard, nextBidCents);
  return (
    <div className="sheet-leaderboard" data-testid="leaderboard-sheet">
      <div className="sheet-action-row">
        <strong>{actionCopy.headline}</strong>
        <button type="button" onClick={onRefresh} disabled={!activeAuctionID}>刷新</button>
      </div>
      <div className="leaderboard-action-card">
        <span>{actionCopy.action}</span>
        <em>{actionCopy.freshness}</em>
      </div>
      {entries.length === 0 ? <p>暂无有效出价</p> : entries.map((entry) => (
        <div className={`leaderboard-row ${entry.is_current ? 'is-current' : ''}`} key={`${entry.rank}-${entry.user_id}`}>
          <span>#{entry.rank}</span>
          <strong>{entry.is_current ? '我' : entry.user_masked}</strong>
          <em>{formatCents(entry.amount_cents)}</em>
        </div>
      ))}
    </div>
  );
}

function HistorySheet({
  empty,
  getPrimary,
  getSecondary,
  historyError,
  historyLoading,
  onRefresh,
  rows,
  title
}: {
  empty: string;
  getPrimary: (row: HistoryRow) => string;
  getSecondary: (row: HistoryRow) => string;
  historyError: string;
  historyLoading: boolean;
  onRefresh: () => void;
  rows: HistoryRow[];
  title: string;
}) {
  return (
    <div className="sheet-history">
      <div className="sheet-action-row">
        <strong>{title}</strong>
        <button type="button" onClick={onRefresh} disabled={historyLoading}>{historyLoading ? '刷新中' : '刷新'}</button>
      </div>
      {historyError && <div className="history-error" role="alert">{historyError}</div>}
      <HistoryList title={title} empty={empty} rows={rows} getPrimary={getPrimary} getSecondary={getSecondary} />
    </div>
  );
}

function LeaderboardPanel({
  activeAuctionID,
  leaderboard,
  nextBidCents,
  onRefresh
}: {
  activeAuctionID: string;
  leaderboard: LeaderboardPayload | null;
  nextBidCents: number;
  onRefresh: () => void;
}) {
  const actionCopy = leaderboardActionCopy(leaderboard, nextBidCents);
  return (
    <section className="leaderboard-panel leaderboard-panel-compact" data-testid="leaderboard-panel">
      <div className="leaderboard-title">
        <h2><Trophy size={16} /> 行动榜单</h2>
        <button type="button" onClick={onRefresh} disabled={!activeAuctionID}>
          <RefreshCw size={14} />
          刷新
        </button>
      </div>
      <div className="my-rank-card rank-strip leaderboard-panel-rank-strip" data-testid="leaderboard-panel-rank-strip">
        <strong>{actionCopy.headline}</strong>
        <span>{actionCopy.action}</span>
        <em>{actionCopy.freshness}</em>
      </div>
      <div className="leaderboard-list">
        {(leaderboard?.entries ?? []).length === 0 ? (
          <p>暂无有效出价</p>
        ) : (
          leaderboard?.entries?.map((entry) => (
            <div className={`leaderboard-row ${entry.is_current ? 'is-current' : ''}`} key={`${entry.rank}-${entry.user_id}`}>
              <span>#{entry.rank}</span>
              <strong>{entry.is_current ? '我' : entry.user_masked}</strong>
              <em>{formatCents(entry.amount_cents)}</em>
            </div>
          ))
        )}
      </div>
    </section>
  );
}

function StateMatrixTabs({
  selected,
  onSelect
}: {
  selected: AuctionState;
  onSelect: (state: AuctionState) => void;
}) {
  return (
    <nav className="state-tabs" aria-label="state-matrix">
      {scenarios.map((item) => (
        <button
          key={item.key}
          className={item.key === selected ? 'active' : ''}
          type="button"
          onClick={() => onSelect(item.key)}
        >
          {item.title}
        </button>
      ))}
    </nav>
  );
}

function HistoryPanel({
  bidHistory,
  historyError,
  historyLoading,
  orderHistory,
  onRefresh
}: {
  bidHistory: HistoryRow[];
  historyError: string;
  historyLoading: boolean;
  orderHistory: HistoryRow[];
  onRefresh: () => void;
}) {
  return (
    <section className="history-panel" data-testid="history-panel">
      <div className="history-title">
        <h2><History size={16} /> 我的历史</h2>
        <button type="button" onClick={onRefresh} disabled={historyLoading}>
          <RefreshCw size={14} />
          {historyLoading ? '刷新中' : '刷新'}
        </button>
      </div>
      {historyError && <div className="history-error" role="alert">{historyError}</div>}
      <div className="history-grid">
        <HistoryList
          title="出价"
          empty="暂无出价"
          rows={bidHistory}
          getPrimary={(row) => String(row.auction_id ?? row.bid_id ?? '-')}
          getSecondary={(row) => `${formatCents(Number(row.amount_cents ?? 0))} · ${String(row.result ?? row.status ?? '-')}`}
        />
        <HistoryList
          title="订单"
          empty="暂无订单"
          rows={orderHistory}
          getPrimary={(row) => String(row.order_id ?? row.auction_id ?? '-')}
          getSecondary={(row) => `${formatCents(Number(row.amount_cents ?? 0))} · ${String(row.order_status ?? '-')}`}
        />
      </div>
    </section>
  );
}

function ChatPanel({
  chatDraft,
  chatMessages,
  chatSending,
  currentUserID,
  onDraftChange,
  onSend
}: {
  chatDraft: string;
  chatMessages: ChatMessage[];
  chatSending: boolean;
  currentUserID: string;
  onDraftChange: (draft: string) => void;
  onSend: () => void;
}) {
  return (
    <section className="chat-panel" data-testid="chat-panel">
      <div className="history-title">
        <h2><MessageCircle size={16} /> 弹幕</h2>
      </div>
      <div className="chat-list">
        {chatMessages.length === 0 ? <p>暂无弹幕</p> : chatMessages.map((message) => (
          <div className="chat-row" key={message.id}>
            <strong>{message.user_id === currentUserID ? '我' : `${message.user_id.slice(0, 2)}**`}</strong>
            <span>{message.body}</span>
          </div>
        ))}
      </div>
      <div className="chat-input-row">
        <input
          aria-label="chat-input"
          maxLength={80}
          value={chatDraft}
          onChange={(event) => onDraftChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') void onSend();
          }}
        />
        <button type="button" aria-label="send-chat" disabled={!chatDraft.trim() || chatSending} onClick={onSend}>
          <Send size={15} />
        </button>
      </div>
    </section>
  );
}

function HistoryList({
  title,
  empty,
  rows,
  getPrimary,
  getSecondary
}: {
  title: string;
  empty: string;
  rows: HistoryRow[];
  getPrimary: (row: HistoryRow) => string;
  getSecondary: (row: HistoryRow) => string;
}) {
  return (
    <div className="history-list">
      <h3>{title}</h3>
      {rows.length === 0 ? (
        <p>{empty}</p>
      ) : rows.map((row, index) => (
        <div className="history-row" key={`${title}-${index}`}>
          <strong>{getPrimary(row)}</strong>
          <span>{getSecondary(row)}</span>
        </div>
      ))}
    </div>
  );
}

createRoot(document.getElementById('root')!).render(<App />);
