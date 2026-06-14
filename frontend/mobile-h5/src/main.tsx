/// <reference types="vite/client" />
import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { CheckCircle2, ChevronUp, Radio, RefreshCw } from 'lucide-react';
import { AppProviders } from './app/providers';
import type { AtmosphereCue, AtmosphereInput } from './atmosphere';
import { AuctionStatePanel, BottomSheet, ChatComposer, ChatPanel, LeaderboardPanel, LiveStage, StateMatrixTabs, type WaterfallChip } from './components';
import { ResultSheet } from './result';
import type { AuctionItem, AuctionOverlayMode, AuctionRealtimeEvent, AuctionState, AuctionSummary, AuctionSoundPack, AuthUser, BidderRequirement, BidPhase, BidResponse, BottomSheetKey, ChatMessage, ConnectionPhase, HeatSnapshot, HistoryRow, LeaderboardPayload, LiveOpsCampaign, MaxBidIntent, MaxBidPhase, OrderRow, PaymentPhase, PendingBidRequest, ProductQAAnswer, ProductQATurn, RecoveryPhase, ResultSheetKind, Scenario, SnapshotResponse, SoundCapability, SystemMessage, WSTicketResponse } from './domain';
import { createAudioContext, createClientBidID, demoProductImageURL, demoUserID, deriveCountdown, deriveCountdownPhase, ensureDemoSession, extendSecondsFromEvent, extensionCopyFromEvent, formatCents, heatSnapshot, isBidCloseGuardActive, isBidConfirmationPending, isCountdownExpired, isDangerousActionDisabled, isEngineRejected, isTestMatrixEnabled, loadAuctionSoundPack, maxBidErrorCopy, maxBidStatusCopy, playAuctionSound, playCountdownTone, playCueTone, playLayeredCue, readJSON, rejectCopy, responseServerTimeMS, retryAfterMS, retryAfterMSFromHeaders, roomIDFromPath, scenarios, selectEntryAuction, speakSystemMessage, vibrateCountdownPhase, vibratePattern, visibleRoomAuctions } from './domain';
import { BID_REQUEST_TIMEOUT_MS, bidRequestHeaders, bidRequestPayload, canRetryPendingBid, interpretBidResponse, networkBidFailure, prepareBidRequest } from './features/contracts/bid-contract';
import { queryOrderPayment, submitOrderPayment } from './features/pay-order/pay-mock-action';
import { auctionWSProtocols, auctionWSURL, wsTicketRequest } from './features/contracts/ws-contract';
import { calculateAtmosphereIntensity, normalizeAtmosphere, shouldGateAtmosphere } from './atmosphere';
import { reconnectDelayMS } from './realtime';
import { h5Copy } from './copy';
import './styles.css';

function normalizeStageItem(item: AuctionItem, fallbackTitle?: string): AuctionItem {
  const imageURL = item.image_url ?? item.imageURL;
  const videoPosterURL = item.video_poster_url ?? item.videoPosterURL;
  return {
    title: item.title ?? fallbackTitle,
    description: item.description,
    image_url: imageURL,
    imageURL,
    video_poster_url: videoPosterURL,
    videoPosterURL,
    certificate: item.certificate ?? 'GID 20260607 · 可核验',
    condition: item.condition ?? (imageURL ? '实物图已上传' : '待补充实物图'),
    shipping: item.shipping ?? '顺丰包邮',
    dimensions: item.dimensions,
    material: item.material,
    return_policy: item.return_policy ?? h5Copy.returnPolicy,
    flaws: item.flaws
  };
}

function yuanInputToCents(value: string) {
  const normalized = value.replace(/[^\d.]/g, '');
  if (!normalized) return 0;
  const amount = Number(normalized);
  if (!Number.isFinite(amount)) return 0;
  return Math.round(amount * 100);
}

function normalizeManualBidCents(amountCents: number, currentPriceCents: number, minimumNextBidCents: number, incrementCents: number, capCents?: number | null) {
  const step = Math.max(1, incrementCents);
  const base = Math.max(0, currentPriceCents);
  const minimum = Math.max(minimumNextBidCents, base + step);
  const requested = Math.max(amountCents || 0, minimum);
  const steps = Math.max(1, Math.ceil((requested - base) / step));
  const normalized = base + steps * step;
  return capCents && capCents > 0 ? Math.min(normalized, capCents) : normalized;
}

function clampMaxBidAmount(amountCents: number, minimumCents: number, capCents?: number | null) {
  const next = Math.max(amountCents || 0, minimumCents);
  if (capCents && capCents > 0) return Math.min(next, capCents);
  return next;
}

// Bound every hot-path bid request. Without a timeout a stalled network (the
// classic "request left, response never came back" weak-network case) leaves the
// fetch promise pending forever, so bidInFlightRef stays true and the CTA is stuck
// on "确认中" with no escape. Aborting on timeout surfaces through the existing
// catch -> 'uncertain' branch; because the same client_bid_id is reused on retry,
// re-sending is idempotent and safe (the engine dedupes by request hash).
async function fetchWithTimeout(url: string, options: RequestInit = {}, timeoutMs = BID_REQUEST_TIMEOUT_MS): Promise<Response> {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { ...options, signal: controller.signal });
  } finally {
    window.clearTimeout(timer);
  }
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
  const [bidAmountText, setBidAmountText] = useState('400.00');
  const [lastSeq, setLastSeq] = useState(41);
  const [bidFeedback, setBidFeedback] = useState('下一口 ¥400.00');
  const [riskCode, setRiskCode] = useState('');
  const [leaderMasked, setLeaderMasked] = useState('张**');
  const [confirmToken, setConfirmToken] = useState('');
  const [confirmIdempotencyKey, setConfirmIdempotencyKey] = useState('');
  const [confirmAmountCents, setConfirmAmountCents] = useState(0);
  const [maxBidIntent, setMaxBidIntent] = useState<MaxBidIntent | null>(null);
  const [maxBidAmountCents, setMaxBidAmountCents] = useState(0);
  const [maxBidAmountText, setMaxBidAmountText] = useState('');
  const [maxBidPhase, setMaxBidPhase] = useState<MaxBidPhase>('idle');
  const [maxBidFeedback, setMaxBidFeedback] = useState('仅自己可见，服务端按加价阶梯代出价');
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState('');
  const [bidHistory, setBidHistory] = useState<HistoryRow[]>([]);
  const [orderHistory, setOrderHistory] = useState<HistoryRow[]>([]);
  const [selectedOrderID, setSelectedOrderID] = useState('');
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([]);
  const [chatDraft, setChatDraft] = useState('');
  const [chatSending, setChatSending] = useState(false);
  const [systemMessages, setSystemMessages] = useState<SystemMessage[]>([]);
  const [qaDraft, setQADraft] = useState('');
  const [qaAnswer, setQAAnswer] = useState<ProductQAAnswer | undefined>();
  const [qaThreadID, setQAThreadID] = useState('');
  const [qaHistory, setQAHistory] = useState<ProductQAAnswer[]>([]);
  const [qaLoading, setQALoading] = useState(false);
  const [leaderboard, setLeaderboard] = useState<LeaderboardPayload | null>(null);
  const [presenceHeat, setPresenceHeat] = useState<Pick<HeatSnapshot, 'watcherCount' | 'watcherCountAvailable'> | null>(null);
  const [waterfallChips, setWaterfallChips] = useState<WaterfallChip[]>([]);
  const [raceBoardExpandedUntil, setRaceBoardExpandedUntil] = useState(0);
  const [atmosphereCue, setAtmosphereCue] = useState<AtmosphereCue | null>(null);
  const [reducedMotion, setReducedMotion] = useState(false);
  const [soundEnabled, setSoundEnabled] = useState(false);
  const [soundCapability, setSoundCapability] = useState<SoundCapability>('ready');
  const [connectionPhase, setConnectionPhase] = useState<ConnectionPhase>('connecting');
  const [activeAuctionID, setActiveAuctionID] = useState('');
  const [activeIncrementCents, setActiveIncrementCents] = useState(5_000);
  const [payableOrderID, setPayableOrderID] = useState('');
  const [payableOrderAmountCents, setPayableOrderAmountCents] = useState(0);
  const [paymentConfirmOpen, setPaymentConfirmOpen] = useState(false);
  const [terminalPriceCents, setTerminalPriceCents] = useState(0);
  const [terminalWinnerID, setTerminalWinnerID] = useState('');
  const [terminalWinnerMasked, setTerminalWinnerMasked] = useState('');
  const [terminalSeq, setTerminalSeq] = useState(0);
  const [auctionEndAt, setAuctionEndAt] = useState('');
  const [serverTimeMS, setServerTimeMS] = useState(0);
  const [nowMS, setNowMS] = useState(Date.now());
  const [bidCooldownUntilMS, setBidCooldownUntilMS] = useState(0);
  const [extensionNotice, setExtensionNotice] = useState('');
  const [currentUserID, setCurrentUserID] = useState(demoUserID);
  const [sessionReady, setSessionReady] = useState(false);
  const [lotTitle, setLotTitle] = useState('天然翡翠A货平安扣吊坠');
  const [roomAuctions, setRoomAuctions] = useState<AuctionSummary[]>([]);
  const [activeSheet, setActiveSheet] = useState<BottomSheetKey | null>(null);
  const [overlayMode, setOverlayMode] = useState<AuctionOverlayMode>(() => showStateMatrix ? 'bid' : 'feed');
  const [followed, setFollowed] = useState(false);
  const [likeCount, setLikeCount] = useState(0);
  const [activeBuyerTeam, setActiveBuyerTeam] = useState<'craft' | 'story'>('craft');
  const [liveOpsCampaign, setLiveOpsCampaign] = useState<LiveOpsCampaign | null>(null);
  const [liveOpsBusy, setLiveOpsBusy] = useState('');
  const [liveOpsError, setLiveOpsError] = useState('');
  const [stageItem, setStageItem] = useState<AuctionItem>({
    title: '天然翡翠A货平安扣吊坠',
    image_url: demoProductImageURL,
    video_poster_url: demoProductImageURL,
    certificate: 'GID 20260607 · 可核验',
    condition: '实物图已上传',
    shipping: '顺丰包邮',
    return_policy: h5Copy.returnPolicy
  });
  const [bidderRequirement, setBidderRequirement] = useState<BidderRequirement | null>(null);
  const paymentInFlight = useRef(false);
  const bidInFlightRef = useRef(false);
  const pendingBidRef = useRef<PendingBidRequest | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const reconnectAttemptRef = useRef(0);
  const recoveryInFlightRef = useRef(false);
  const selectedRef = useRef(selected);
  const bidPhaseRef = useRef(bidPhase);
  const paymentPhaseRef = useRef(paymentPhase);
  const lastSeqRef = useRef(lastSeq);
  const currentPriceRef = useRef(currentPriceCents);
  const leaderMaskedRef = useRef(leaderMasked);
  const activeAuctionIDRef = useRef(activeAuctionID);
  const activeIncrementCentsRef = useRef(activeIncrementCents);
  const roomAuctionsRef = useRef<AuctionSummary[]>(roomAuctions);
  const maxBidAmountCentsRef = useRef(maxBidAmountCents);
  const maxBidPhaseRef = useRef(maxBidPhase);
  const maxBidIntentRef = useRef<MaxBidIntent | null>(maxBidIntent);
  const auctionEndAtRef = useRef(auctionEndAt);
  const serverTimeMSRef = useRef(serverTimeMS);
  // Tracks the local Date.now() at the moment we last received a server-time anchor.
  // Used to compute clock-skew-independent elapsed time in deriveCountdown.
  const serverTimeSyncedAtRef = useRef(0);
  const currentUserIDRef = useRef(currentUserID);
  const soundEnabledRef = useRef(soundEnabled);
  const audioContextRef = useRef<AudioContext | null>(null);
  const soundPackRef = useRef<AuctionSoundPack | null>(null);
  const heartbeatRef = useRef<{ source: AudioBufferSourceNode; gain: GainNode } | null>(null);
  const soundCapabilityRef = useRef<SoundCapability>('ready');
  const leaderboardRef = useRef<LeaderboardPayload | null>(leaderboard);
  const pendingLeaderboardDeltaRef = useRef<LeaderboardPayload | null>(null);
  const leaderboardFrameRef = useRef<number | null>(null);
  const leaderboardBurstUntilRef = useRef(0);
  const hotAcceptedPreviewRef = useRef<{ auctionID: string; amountCents: number; expiresAt: number } | null>(null);
  const maxBidCancelFenceRef = useRef<Record<string, number>>({});
  const atmosphereSeenRef = useRef<Set<string>>(new Set());
  const recoveringRef = useRef(false);
  const activeCueRef = useRef<AtmosphereCue | null>(null);
  const countdownCueRef = useRef('');
  const spokenSystemMessageRef = useRef(0);
  const orderPollStartedAtRef = useRef(0);

  const keepBidOverlayForCurrentMode = () => {
    setOverlayMode(showStateMatrix ? 'bid' : 'feed');
  };

  const commitMaxBidIntentState = (intent: MaxBidIntent | null) => {
    maxBidIntentRef.current = intent;
    setMaxBidIntent(intent);
  };

  const commitPaymentPhase = (phase: PaymentPhase) => {
    paymentPhaseRef.current = phase;
    setPaymentPhase(phase);
  };

  const isTerminalPaymentPhase = (phase: PaymentPhase) => phase === 'paid' || phase === 'expired';

  const clearAutoMaxBidPresentation = (message = '自动加价已取消') => {
    const cue = activeCueRef.current;
    if (cue?.kind === 'leading' && /自动|防守/.test(`${cue.title}${cue.detail}`)) {
      activeCueRef.current = null;
      setAtmosphereCue(null);
    }
    setBidFeedback((current) => (/自动加价|自动防守|防守成功/.test(current) ? message : current));
    setMaxBidFeedback(message);
  };

  const markMaxBidCancelledFence = (auctionID: string) => {
    maxBidCancelFenceRef.current[auctionID] = Math.max(maxBidCancelFenceRef.current[auctionID] ?? 0, lastSeqRef.current);
  };

  useEffect(() => {
    selectedRef.current = selected;
  }, [selected]);

  useEffect(() => {
    bidPhaseRef.current = bidPhase;
  }, [bidPhase]);

  useEffect(() => {
    paymentPhaseRef.current = paymentPhase;
  }, [paymentPhase]);

  useEffect(() => {
    lastSeqRef.current = lastSeq;
  }, [lastSeq]);

  useEffect(() => {
    currentPriceRef.current = currentPriceCents;
  }, [currentPriceCents]);

  useEffect(() => {
    setBidAmountText((nextBidCents / 100).toFixed(2));
  }, [nextBidCents]);

  useEffect(() => {
    leaderMaskedRef.current = leaderMasked;
  }, [leaderMasked]);

  useEffect(() => {
    activeAuctionIDRef.current = activeAuctionID;
  }, [activeAuctionID]);

  useEffect(() => {
    setQADraft('');
    setQAAnswer(undefined);
    setQAHistory([]);
    setQAThreadID(activeAuctionID ? `qa_${activeAuctionID}_${Date.now().toString(36)}` : '');
  }, [activeAuctionID]);

  useEffect(() => {
    activeIncrementCentsRef.current = activeIncrementCents;
  }, [activeIncrementCents]);

  useEffect(() => {
    roomAuctionsRef.current = roomAuctions;
  }, [roomAuctions]);

  useEffect(() => {
    maxBidAmountCentsRef.current = maxBidAmountCents;
  }, [maxBidAmountCents]);

  useEffect(() => {
    maxBidPhaseRef.current = maxBidPhase;
  }, [maxBidPhase]);

  useEffect(() => {
    maxBidIntentRef.current = maxBidIntent;
  }, [maxBidIntent]);

  useEffect(() => {
    setMaxBidAmountText(maxBidAmountCents > 0 ? (maxBidAmountCents / 100).toFixed(2) : '');
  }, [maxBidAmountCents]);

  useEffect(() => {
    auctionEndAtRef.current = auctionEndAt;
  }, [auctionEndAt]);

  useEffect(() => {
    serverTimeMSRef.current = serverTimeMS;
  }, [serverTimeMS]);

  const syncServerTimeMS = (value: number) => {
    if (value > 0) {
      serverTimeSyncedAtRef.current = Date.now();
    }
    setServerTimeMS(value);
  };

  useEffect(() => {
    currentUserIDRef.current = currentUserID;
  }, [currentUserID]);

  const ensureBuyerSession = async () => {
    const user = await ensureDemoSession('user');
    setCurrentUserID(user.ID);
    currentUserIDRef.current = user.ID;
    return user;
  };

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
    const timer = window.setInterval(() => setNowMS(Date.now()), 100);
    return () => window.clearInterval(timer);
  }, []);

  const countdownCopy = useMemo(() => {
    const stale = connectionPhase === 'disconnected' || recoveryPhase === 'recovering' || !activeAuctionID;
    const terminal = selected === 'sold_winner' || selected === 'sold_loser' || selected === 'ended' || selected === 'cancelled';
    return deriveCountdown(auctionEndAt, serverTimeMS, nowMS, serverTimeSyncedAtRef.current, terminal, stale, Boolean(extensionNotice));
  }, [activeAuctionID, auctionEndAt, connectionPhase, extensionNotice, nowMS, recoveryPhase, selected, serverTimeMS]);
  const countdownPhase = useMemo(() => {
    const stale = connectionPhase === 'disconnected' || recoveryPhase === 'recovering' || !activeAuctionID;
    const terminal = selected === 'sold_winner' || selected === 'sold_loser' || selected === 'ended' || selected === 'cancelled';
    return deriveCountdownPhase({
      endAt: auctionEndAt,
      serverTimeMS,
      nowMS,
      serverTimeSyncedAt: serverTimeSyncedAtRef.current,
      terminal,
      stale,
      active: selected === 'active_bids'
    });
  }, [activeAuctionID, auctionEndAt, connectionPhase, nowMS, recoveryPhase, selected, serverTimeMS]);
  const activeAuction = useMemo(() => roomAuctions.find((auction) => auction.id === activeAuctionID), [activeAuctionID, roomAuctions]);
  const visibleMaxBidIntent = useMemo(
    () => maxBidIntent && maxBidIntent.auction_id === activeAuctionID ? maxBidIntent : null,
    [activeAuctionID, maxBidIntent]
  );
  const visibleMaxBidFeedback = visibleMaxBidIntent || maxBidPhase !== 'idle'
    ? maxBidFeedback
    : '仅自己可见，服务端按加价阶梯代出价';
  const heat = useMemo(() => ({ ...heatSnapshot(leaderboard, activeAuction), ...(presenceHeat ?? {}) }), [activeAuction, leaderboard, presenceHeat]);
  const atmosphereIntensity = useMemo(() => calculateAtmosphereIntensity({
    acceptedBids30s: heat.acceptedBids30s,
    priceVelocityCentsPerMin: heat.priceVelocityCentsPerMin,
    remainingMS: countdownPhase.remainingMS,
    extended: selected === 'extended'
  }), [countdownPhase.remainingMS, heat.acceptedBids30s, heat.priceVelocityCentsPerMin, selected]);
  const bidCooldownRemainingMS = Math.max(0, bidCooldownUntilMS - nowMS);
  const raceBoardExpanded = raceBoardExpandedUntil > nowMS;
  const isCurrentUserLeading = Boolean(
    selected === 'active_bids' &&
    activeAuctionID &&
    leaderboard?.auction_id === activeAuctionID &&
    currentUserID &&
    (
      leaderboard?.state === 'LEADING' ||
      leaderboard?.my_rank === 1 ||
      leaderboard?.current_winner_id === currentUserID
    )
  );
  const countdownExpired = useMemo(() => (
    selected === 'active_bids' &&
    connectionPhase === 'connected' &&
    recoveryPhase === 'idle' &&
    isCountdownExpired(auctionEndAt, serverTimeMS, nowMS, serverTimeSyncedAtRef.current)
  ), [auctionEndAt, connectionPhase, nowMS, recoveryPhase, selected, serverTimeMS]);
  const bidCloseGuardActive = useMemo(() => (
    selected === 'active_bids' &&
    connectionPhase === 'connected' &&
    recoveryPhase === 'idle' &&
    isBidCloseGuardActive(auctionEndAt, serverTimeMS, nowMS, serverTimeSyncedAtRef.current)
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
      playCueTone(audioContextRef.current, cue.kind, soundPackRef.current);
      vibratePattern(cue.kind);
    }
  };

  const commitLeaderboardDelta = (delta: LeaderboardPayload) => {
    if (!delta || delta.auction_id !== activeAuctionIDRef.current) return;
    const currentSeq = leaderboardRef.current?.seq ?? lastSeqRef.current;
    if (delta.seq != null && currentSeq != null && delta.seq < currentSeq) return;
    const previous = leaderboardRef.current;
    const previousSeq = previous?.seq ?? lastSeqRef.current;
    const previousWinnerID = previous?.current_winner_id;
    const previousState = previous?.state;
    const entries = (delta.entries ?? []).map((entry) => ({
      ...entry,
      is_current: entry.user_id === currentUserIDRef.current
    }));
    const mine = entries.find((entry) => entry.user_id === currentUserIDRef.current);
    const leader = entries[0];
    const leaderAmount = delta.leader_amount_cents || leader?.amount_cents || delta.current_price_cents;
    const myBestAmount = mine?.amount_cents;
    const gapToLeader = myBestAmount != null ? Math.max(0, leaderAmount - myBestAmount) : undefined;
    const nextLeaderboard: LeaderboardPayload = {
      ...previous,
      ...delta,
      entries,
      leader_amount_cents: leaderAmount,
      my_rank: mine?.rank ?? (previous?.auction_id === delta.auction_id ? previous.my_rank : undefined),
      my_best_amount_cents: myBestAmount ?? (previous?.auction_id === delta.auction_id ? previous.my_best_amount_cents : undefined),
      gap_to_leader_cents: gapToLeader ?? (previous?.auction_id === delta.auction_id ? previous.gap_to_leader_cents : undefined),
      state: mine?.rank === 1 ? 'LEADING' : mine ? 'OUTBID' : 'NOT_BID'
    };
    leaderboardRef.current = nextLeaderboard;
    setLeaderboard(nextLeaderboard);
    syncBidPhaseFromLeaderboard(nextLeaderboard);
    setCurrentPriceCents(delta.current_price_cents);
    setMinimumNextBidCents(delta.next_valid_bid_cents ?? delta.current_price_cents + activeIncrementCentsRef.current);
    setNextBidCents((prepared) => Math.max(delta.next_valid_bid_cents ?? delta.current_price_cents + activeIncrementCentsRef.current, prepared));
    if (delta.server_time_ms) syncServerTimeMS(delta.server_time_ms);
    if (leader?.user_masked) setLeaderMasked(leader.user_masked);
    const isNewDelta = delta.seq != null && delta.seq > previousSeq;
    if (isNewDelta && leader) {
      setRaceBoardExpandedUntil(Date.now() + 3200);
      setWaterfallChips((chips) => [
        ...chips.slice(-23),
        {
          id: `${delta.auction_id}:${delta.seq}`,
          seq: delta.seq ?? Date.now(),
          amount_cents: leader.amount_cents,
          user_masked: leader.is_current ? '我' : leader.user_masked,
          is_current: Boolean(leader.is_current),
          created_at: Date.now()
        }
      ]);
    }
    if (isNewDelta && mine?.rank === 1 && previousState !== 'LEADING') {
      showAtmosphere({
        kind: 'leading',
        title: '领先！',
        detail: `${formatCents(leaderAmount)} 已确认`,
        auction_id: delta.auction_id,
        cause_seq: delta.seq,
        event_type: delta.event_type || 'leaderboard_delta',
        user_scope: 'self'
      });
    } else if (isNewDelta && mine && mine.rank > 1 && (previousWinnerID === currentUserIDRef.current || previousState === 'LEADING')) {
      showAtmosphere({
        kind: 'outbid',
        title: '被超越！',
        detail: `${gapToLeader != null ? `差 ${formatCents(gapToLeader)}` : '有人已经领先'} · 立即反超`,
        auction_id: delta.auction_id,
        cause_seq: delta.seq,
        event_type: delta.event_type || 'leaderboard_delta',
        user_scope: 'self'
      });
      if (selectedRef.current === 'active_bids') {
        setOverlayMode('bid');
      }
    }
    if (!delta.burst_mode && soundEnabledRef.current && audioContextRef.current && soundCapabilityRef.current === 'ready') {
      playLayeredCue(audioContextRef.current, 'rank_change', soundPackRef.current);
    }
  };

  const syncBidPhaseFromLeaderboard = (payload: LeaderboardPayload) => {
    if (!payload || payload.auction_id !== activeAuctionIDRef.current) return;
    if (payload.state === 'LEADING' || payload.my_rank === 1 || payload.current_winner_id === currentUserIDRef.current) {
      hotAcceptedPreviewRef.current = null;
      if (payload.current_winner_id === currentUserIDRef.current || payload.state === 'LEADING') {
        setBidPhase((phase) => phase === 'pending' || phase === 'confirming' || phase === 'confirm_required' ? phase : 'accepted');
        setRiskCode('');
        setBidFeedback('你已领先，出价已确认');
      }
      return;
    }
    if (payload.state === 'OUTBID' || (payload.my_rank != null && payload.my_rank > 1) || (payload.current_winner_id && payload.current_winner_id !== currentUserIDRef.current)) {
      const hotPreview = hotAcceptedPreviewRef.current;
      const leaderAmount = payload.leader_amount_cents ?? payload.current_price_cents;
      if (
        hotPreview
        && hotPreview.auctionID === payload.auction_id
        && Date.now() < hotPreview.expiresAt
        && leaderAmount < hotPreview.amountCents
      ) {
        return;
      }
      hotAcceptedPreviewRef.current = null;
      pendingBidRef.current = null;
      setBidPhase((phase) => phase === 'pending' || phase === 'confirming' || phase === 'confirm_required' ? phase : 'idle');
      setConfirmToken('');
      setConfirmIdempotencyKey('');
      setConfirmAmountCents(0);
      setRiskCode('');
      const nextBid = payload.next_valid_bid_cents ?? (payload.current_price_cents + activeIncrementCentsRef.current);
      setBidFeedback(`被超越，下一口 ${formatCents(nextBid)}`);
      return;
    }
    if (payload.state === 'RECOVERING') {
      setBidPhase((phase) => phase === 'pending' || phase === 'confirming' || phase === 'confirm_required' ? phase : 'idle');
      setRiskCode('RECOVERING');
      setBidFeedback('竞拍状态正在校对，以服务端最新结果为准');
      setConnectionPhase('recovering');
    }
  };

  const normalizeLeaderboardPayload = (payload: LeaderboardPayload, previous?: LeaderboardPayload | null): LeaderboardPayload => {
    const entries = (payload.entries ?? []).map((entry) => ({
      ...entry,
      is_current: entry.user_id === currentUserIDRef.current
    }));
    const mine = entries.find((entry) => entry.user_id === currentUserIDRef.current);
    const leader = entries[0];
    const leaderAmount = payload.leader_amount_cents || leader?.amount_cents || payload.current_price_cents;
    const myBestAmount = mine?.amount_cents;
    const gapToLeader = myBestAmount != null ? Math.max(0, leaderAmount - myBestAmount) : undefined;
    return {
      ...previous,
      ...payload,
      entries,
      leader_amount_cents: leaderAmount,
      my_rank: mine?.rank ?? (previous?.auction_id === payload.auction_id ? previous.my_rank : payload.my_rank),
      my_best_amount_cents: myBestAmount ?? (previous?.auction_id === payload.auction_id ? previous.my_best_amount_cents : payload.my_best_amount_cents),
      gap_to_leader_cents: gapToLeader ?? (previous?.auction_id === payload.auction_id ? previous.gap_to_leader_cents : payload.gap_to_leader_cents),
      state: payload.state === 'RECOVERING' ? 'RECOVERING' : mine?.rank === 1 ? 'LEADING' : mine ? 'OUTBID' : 'NOT_BID'
    };
  };

  const applyLeaderboardDelta = (delta: LeaderboardPayload) => {
    if (!delta || delta.auction_id !== activeAuctionIDRef.current) return;
    const currentSeq = pendingLeaderboardDeltaRef.current?.seq ?? leaderboardRef.current?.seq ?? lastSeqRef.current;
    if (delta.seq != null && currentSeq != null && delta.seq < currentSeq) return;
    const now = Date.now();
    const nextDelta = pendingLeaderboardDeltaRef.current && now < leaderboardBurstUntilRef.current
      ? ({ ...delta, burst_mode: true } as LeaderboardPayload)
      : delta;
    leaderboardBurstUntilRef.current = now + 180;
    pendingLeaderboardDeltaRef.current = nextDelta;
    if (leaderboardFrameRef.current != null) return;
    leaderboardFrameRef.current = window.requestAnimationFrame(() => {
      leaderboardFrameRef.current = null;
      const next = pendingLeaderboardDeltaRef.current;
      pendingLeaderboardDeltaRef.current = null;
      if (next) commitLeaderboardDelta(next);
    });
  };

  const stopHeartbeat = () => {
    try {
      heartbeatRef.current?.source.stop();
    } catch {
      // Source may already be stopped when React tears down effects in quick succession.
    }
  };

  const toggleSound = async () => {
    if (soundEnabledRef.current) {
      setSoundEnabled(false);
      stopHeartbeat();
      heartbeatRef.current = null;
      soundPackRef.current = null;
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
      soundPackRef.current = await loadAuctionSoundPack(ctx);
      setSoundCapability('ready');
      setSoundEnabled(true);
    } catch {
      setSoundCapability('blocked');
      setSoundEnabled(false);
      await ctx.close?.();
    }
  };

  useEffect(() => {
    const query = window.matchMedia?.('(prefers-reduced-motion: reduce)');
    if (!query) return;
    const sync = () => setReducedMotion(query.matches);
    sync();
    query.addEventListener?.('change', sync);
    return () => query.removeEventListener?.('change', sync);
  }, []);

  useEffect(() => {
    return () => {
      stopHeartbeat();
      heartbeatRef.current = null;
      soundPackRef.current = null;
      audioContextRef.current?.close?.().catch?.(() => undefined);
      audioContextRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!atmosphereCue) return;
    const timer = window.setTimeout(() => setAtmosphereCue(null), atmosphereCue.kind === 'sold' ? 2700 : 1800);
    return () => {
      window.clearTimeout(timer);
      if (activeCueRef.current?.id === atmosphereCue.id) activeCueRef.current = null;
    };
  }, [atmosphereCue]);

  useEffect(() => {
    const latest = systemMessages[0];
    if (!latest || latest.id <= spokenSystemMessageRef.current) return;
    spokenSystemMessageRef.current = latest.id;
    if (!soundEnabledRef.current || soundCapabilityRef.current !== 'ready') return;
    if (audioContextRef.current) playLayeredCue(audioContextRef.current, 'system_message', soundPackRef.current);
    speakSystemMessage(latest.body);
  }, [systemMessages]);

  useEffect(() => {
    const cueKey = `${activeAuctionID}:${countdownPhase.phase}:${countdownPhase.beat}`;
    if (countdownCueRef.current === cueKey) return;
    countdownCueRef.current = cueKey;
    if (countdownPhase.phase !== 'critical' && countdownPhase.phase !== 'hammer') return;
    if (connectionPhase !== 'connected' || recoveryPhase !== 'idle' || selected !== 'active_bids') return;
    if (soundEnabledRef.current && audioContextRef.current && soundCapabilityRef.current === 'ready') {
      playCountdownTone(audioContextRef.current, countdownPhase.phase, countdownPhase.beat, soundPackRef.current);
    }
    vibrateCountdownPhase(countdownPhase.phase, countdownPhase.beat);
  }, [activeAuctionID, connectionPhase, countdownPhase.beat, countdownPhase.phase, recoveryPhase, selected]);

  useEffect(() => {
    const shouldPlayBed = soundEnabled &&
      soundCapability === 'ready' &&
      audioContextRef.current &&
      (countdownPhase.phase === 'critical' || countdownPhase.phase === 'hammer') &&
      connectionPhase === 'connected' &&
      recoveryPhase === 'idle' &&
      selected === 'active_bids';
    if (!shouldPlayBed) {
      stopHeartbeat();
      heartbeatRef.current = null;
      return;
    }
    if (!heartbeatRef.current && audioContextRef.current) {
      heartbeatRef.current = playAuctionSound(audioContextRef.current, soundPackRef.current, 'heartbeat_bed', countdownPhase.phase === 'hammer' ? 0.32 : 0.22, true);
    } else if (heartbeatRef.current) {
      heartbeatRef.current.gain.gain.setTargetAtTime(countdownPhase.phase === 'hammer' ? 0.32 : 0.22, audioContextRef.current!.currentTime, 0.08);
    }
  }, [connectionPhase, countdownPhase.phase, recoveryPhase, selected, soundCapability, soundEnabled]);

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
          leader: '你已中拍',
          feedback: '等待支付确认',
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
          leader: '你已中拍',
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
        leader: '你已中拍',
        feedback: payableOrderID ? '订单待支付' : '订单生成中',
        countdown: payableOrderID ? '支付倒计时以订单为准' : '订单生成中',
        cta: payableOrderID ? '去支付' : '等待订单',
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
        leader: terminalWinnerMasked ? `${terminalWinnerMasked} 中拍` : '已中拍',
        feedback: '本场已结束',
        countdown: '已落槌',
        cta: '已结束',
        ctaDisabled: true,
        sold: true
      };
    }
    if (selected !== 'active_bids') {
      if (selected === 'scheduled') {
        return {
          key: 'scheduled',
          title: '即将开拍',
          status: 'SCHEDULED',
          price: formatCents(currentPriceCents),
          leader: '等待主播开拍',
          feedback: '拍品已进入队列',
          countdown: countdownCopy,
          cta: '等待开拍',
          ctaDisabled: true
        };
      }
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
        title: h5Copy.loading,
        status: 'RECOVERING',
        price: formatCents(currentPriceCents),
        leader: h5Copy.loading,
        feedback: '正在读取当前竞拍',
        countdown: '剩余时间确认中',
        cta: h5Copy.loading,
        ctaDisabled: true,
        stale: true,
        staleCopy: '正在读取服务端竞拍'
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
        leader: h5Copy.loading,
        feedback: h5Copy.refreshing,
        countdown: countdownCopy,
        cta: h5Copy.loading,
        ctaDisabled: true,
        stale: true
      };
    }
    if (leaderboard?.state === 'RECOVERING') {
      return {
        key: 'recovering',
        title: '状态校对中',
        status: 'RECOVERING',
        price: formatCents(currentPriceCents),
        leader: '等待服务端确认',
        feedback: '竞拍状态正在校对，以服务端最新结果为准',
        countdown: countdownCopy,
        cta: h5Copy.loading,
        ctaDisabled: true,
        stale: true,
        staleCopy: '竞拍状态校对中'
      };
    }
    if (bidCooldownRemainingMS > 0) {
      const remainingSeconds = Math.ceil(bidCooldownRemainingMS / 1000);
      return {
        key: 'rejected' as AuctionState,
        title: '冷却中',
        status: 'THROTTLED',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: `${bidFeedback || '竞价激烈，请稍候'} · ${remainingSeconds} 秒后重试`,
        countdown: countdownCopy,
        cta: `${remainingSeconds} 秒后重试`,
        ctaDisabled: true,
        rejected: true
      };
    }
    if (bidCloseGuardActive && bidPhase !== 'pending' && bidPhase !== 'engine_pending' && bidPhase !== 'engine_sold_pending') {
      return {
        key: 'recovering',
        title: '到点结算中',
        status: 'RECOVERING',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: '已进入服务端落槌保护，正在确认最终结果',
        countdown: countdownCopy,
        cta: h5Copy.loading,
        ctaDisabled: true
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
    if (bidPhase === 'engine_pending') {
      return {
        key: 'pending' as AuctionState,
        title: '结算中',
        status: 'ENGINE_PENDING',
        price: formatCents(currentPriceCents),
        leader: leaderMasked ? `${leaderMasked} 领先` : '出价已接收',
        feedback: bidFeedback || '出价已提交，正在确认',
        countdown: countdownCopy,
        cta: '结算中',
        ctaDisabled: true,
        pending: true
      };
    }
    if (bidPhase === 'engine_sold_pending') {
      return {
        key: 'pending' as AuctionState,
        title: '落槌结算中',
        status: 'ENGINE_SOLD_PENDING',
        price: formatCents(currentPriceCents),
        leader: leaderMasked ? `${leaderMasked} 中拍` : '正在确认成交',
        feedback: bidFeedback || '等待订单结算确认',
        countdown: '订单生成中',
        cta: '等待订单',
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
        feedback: '等待服务端确认大额出价',
        countdown: countdownCopy,
        cta: '确认中',
        ctaDisabled: true,
        pending: true
      };
    }
    if (bidPhase === 'confirm_required') {
      return {
        key: 'active_bids' as AuctionState,
        title: '大额出价确认',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: `确认 ${formatCents(confirmAmountCents)} 出价`,
        countdown: countdownCopy,
        cta: '确认高额出价',
        ctaDisabled: false
      };
    }
    if (isCurrentUserLeading) {
      return {
        key: 'self_leading' as AuctionState,
        title: '领先中',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: '你已领先',
        feedback: bidFeedback || '等待其他买家出价',
        countdown: countdownCopy,
        cta: '等待其他买家',
        ctaDisabled: true
      };
    }
    if (bidPhase === 'accepted') {
      return {
        key: 'self_leading' as AuctionState,
        title: '领先中',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: '你已领先',
        feedback: bidFeedback || '等待其他买家出价',
        countdown: countdownCopy,
        cta: '等待其他买家',
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
        feedback: countdownExpired || bidCloseGuardActive ? '到点确认服务端结果' : bidFeedback,
        countdown: countdownCopy,
        cta: countdownExpired || bidCloseGuardActive ? h5Copy.loading : `出一手 ${formatCents(nextBidCents)}`,
        ctaDisabled: countdownExpired || bidCloseGuardActive,
        rejected: true
      };
    }
    if (bidPhase === 'processing_retry') {
      return {
        key: 'rejected' as AuctionState,
        title: '提交中',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: bidFeedback,
        countdown: countdownCopy,
        cta: '用原请求重试',
        ctaDisabled: false,
        rejected: true
      };
    }
    if (bidPhase === 'uncertain') {
      return {
        key: 'rejected' as AuctionState,
        title: '结果不确定',
        status: 'UNCERTAIN',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: bidFeedback,
        countdown: countdownCopy,
        cta: '用原请求重试',
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
      feedback: countdownExpired || bidCloseGuardActive ? '到点确认服务端结果' : bidFeedback,
      countdown: countdownCopy,
      cta: countdownExpired || bidCloseGuardActive ? h5Copy.loading : `出一手 ${formatCents(nextBidCents)}`,
      ctaDisabled: countdownExpired || bidCloseGuardActive
    };
  }, [activeAuctionID, bidCloseGuardActive, bidCooldownRemainingMS, bidFeedback, bidderRequirement, bidPhase, confirmAmountCents, connectionPhase, countdownCopy, countdownExpired, currentPriceCents, isCurrentUserLeading, lastSeq, leaderMasked, leaderboard?.state, minimumNextBidCents, nextBidCents, payableOrderAmountCents, payableOrderID, paymentPhase, recoveryPhase, selected, terminalPriceCents, terminalWinnerID]);
  const atmosphereGate = useMemo(() => shouldGateAtmosphere({
    recovering: recoveryPhase !== 'idle' || connectionPhase === 'recovering',
    stale: scenario.stale || countdownPhase.phase === 'stale' || selected === 'recovering',
    disconnected: connectionPhase === 'disconnected' || selected === 'disconnected',
    reducedMotion,
    lowPower: false,
    aiOff: false
  }), [connectionPhase, countdownPhase.phase, recoveryPhase, reducedMotion, scenario.stale, selected]);
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
    const isPendingDurability = payload.result === 'BID_CONFIRMATION_PENDING'
      || payload.result === 'PROCESSING_RETRY_LATER'
      || payload.decision_status === 'PENDING_DURABILITY'
      || payload.durability_status === 'KAFKA_UNKNOWN';
    if (isPendingDurability) {
      setRiskCode('BID_CONFIRMATION_PENDING');
      setBidFeedback('出价已提交，正在确认');
      setBidPhase('engine_pending');
      showAtmosphere({
        kind: 'leading',
        title: '出价确认中',
        detail: '等待最终确认',
        auction_id: payload.auction_id ?? activeAuctionIDRef.current,
        cause_seq: payload.engine_seq ?? payload.seq ?? lastSeqRef.current,
        event_type: payload.result ?? 'BID_CONFIRMATION_PENDING',
        user_scope: 'self'
      });
      return;
    }
    if (payload.result === 'RECONCILING' || payload.decision_status === 'RECONCILING' || payload.durability_status === 'KAFKA_FAILED') {
      setRiskCode('RECONCILING');
      setBidFeedback('竞拍状态正在恢复，暂不能确认本次出价结果');
      setBidPhase('uncertain');
      pendingBidRef.current = pendingBidRef.current ?? null;
      return;
    }
    if (isEngineRejected(payload)) {
      const code = payload.reject_reason ?? 'ENGINE_REJECTED';
      const refreshedNextBid = payload.next_valid_bid_cents ?? (
        payload.current_price_cents != null ? payload.current_price_cents + activeIncrementCents : undefined
      );
      if (refreshedNextBid != null) {
        setMinimumNextBidCents(refreshedNextBid);
        setNextBidCents((prepared) => Math.max(refreshedNextBid, prepared));
      }
      setRiskCode(code);
      setBidFeedback(`${rejectCopy(code)}，请按当前价格重新确认`);
      setBidPhase('rejected');
      pendingBidRef.current = null;
      void loadLeaderboard(payload.auction_id ?? activeAuctionIDRef.current);
      return;
    }
    const acceptedPrice = payload.current_price_cents ?? currentPriceCents;
    const acceptedWinnerID = payload.current_winner_id ?? '';
    const isEnginePending = payload.result === 'ENGINE_ACCEPTED' && payload.settlement_status !== 'SETTLED';
    const isEngineSoldPending = payload.result === 'ENGINE_SOLD' && payload.settlement_status !== 'SETTLED';
    const acceptedSequence = Math.max(payload.seq ?? 0, payload.engine_seq ?? 0);
    if (acceptedSequence > 0) {
      lastSeqRef.current = Math.max(lastSeqRef.current, acceptedSequence);
      setLastSeq((current) => Math.max(current, acceptedSequence));
    }
    setCurrentPriceCents(acceptedPrice);
    setMinimumNextBidCents(acceptedPrice + activeIncrementCents);
    setNextBidCents(acceptedPrice + activeIncrementCents);
    if (!isEnginePending && !isEngineSoldPending && payload.seq != null) {
      lastSeqRef.current = Math.max(lastSeqRef.current, payload.seq);
      setLastSeq((current) => Math.max(current, payload.seq ?? current));
    }
    if (payload.end_at) setAuctionEndAt(payload.end_at);
    if (payload.server_time_ms) syncServerTimeMS(payload.server_time_ms);
    const acceptedAuctionID = payload.auction_id ?? activeAuctionIDRef.current;
    if (acceptedWinnerID) {
      const previousLeaderboard = leaderboardRef.current?.auction_id === acceptedAuctionID ? leaderboardRef.current : null;
      const hotLeaderboard: LeaderboardPayload = {
        ...(previousLeaderboard ?? undefined),
        auction_id: acceptedAuctionID,
        event_type: payload.result ?? 'bid_accepted',
        seq: acceptedSequence > 0 ? acceptedSequence : (payload.seq ?? lastSeqRef.current),
        server_time_ms: payload.server_time_ms,
        current_price_cents: acceptedPrice,
        current_winner_id: acceptedWinnerID,
        next_valid_bid_cents: acceptedPrice + activeIncrementCents,
        leader_amount_cents: acceptedPrice,
        accepted_bidder_count: Math.max(previousLeaderboard?.accepted_bidder_count ?? 0, 1),
        entries: [{
          rank: 1,
          user_id: acceptedWinnerID,
          user_masked: acceptedWinnerID === currentUserIDRef.current ? '我' : (payload.leader_user_masked ?? leaderMaskedRef.current),
          amount_cents: acceptedPrice,
          bid_count: 1
        }]
      };
      commitLeaderboardDelta(hotLeaderboard);
    }
    if (isEnginePending || isEngineSoldPending) {
      if (acceptedWinnerID === currentUserID && !isEngineSoldPending) {
        setConfirmToken('');
        setConfirmIdempotencyKey('');
        setConfirmAmountCents(0);
        setRiskCode('');
        setBidFeedback('你已领先，后台同步中');
        bidPhaseRef.current = 'accepted';
        setBidPhase('accepted');
        pendingBidRef.current = null;
        hotAcceptedPreviewRef.current = {
          auctionID: payload.auction_id ?? activeAuctionIDRef.current,
          amountCents: acceptedPrice,
          expiresAt: Date.now() + 3000
        };
      } else {
        setBidFeedback(isEngineSoldPending
          ? '已到成交确认，等待订单生成'
          : '出价已提交，正在确认');
        const nextPhase = isEngineSoldPending ? 'engine_sold_pending' : 'engine_pending';
        bidPhaseRef.current = nextPhase;
        setBidPhase(nextPhase);
      }
      showAtmosphere({
        kind: 'leading',
        title: isEngineSoldPending ? '成交确认中' : acceptedWinnerID === currentUserID ? '领先！' : '出价已接收',
        detail: isEngineSoldPending ? '等待订单生成' : acceptedWinnerID === currentUserID ? `${formatCents(acceptedPrice)} 已由热引擎确认` : '等待最终确认',
        auction_id: payload.auction_id ?? activeAuctionIDRef.current,
        cause_seq: payload.engine_seq ?? payload.seq ?? lastSeqRef.current,
        event_type: payload.result ?? 'ENGINE_ACCEPTED',
        user_scope: 'self'
      });
      window.setTimeout(() => void loadLeaderboard(payload.auction_id ?? activeAuctionIDRef.current), 250);
      return;
    }
    if (payload.result === 'ACCEPTED_EXTENDED') {
      setExtensionNotice('服务端已延时');
      setBidFeedback('最后时刻有出价，竞拍已延时');
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
    setRiskCode('');
    if (payload.result === 'ACCEPTED_SOLD') {
      if (payload.seq != null) {
        lastSeqRef.current = Math.max(lastSeqRef.current, payload.seq);
        setLastSeq((current) => Math.max(current, payload.seq ?? current));
      }
      setTerminalPriceCents(acceptedPrice);
      setTerminalWinnerID(payload.current_winner_id ?? '');
      setTerminalWinnerMasked(payload.leader_user_masked ?? leaderMaskedRef.current);
      setSelected(payload.current_winner_id === currentUserIDRef.current ? 'sold_winner' : 'sold_loser');
      showAtmosphere({
        kind: 'sold',
        title: payload.current_winner_id === currentUserIDRef.current ? '成交！' : '已成交',
        detail: payload.current_winner_id === currentUserIDRef.current ? '你已中拍，订单待支付' : '本场已落槌',
        auction_id: payload.auction_id ?? activeAuctionIDRef.current,
        cause_seq: payload.seq ?? lastSeqRef.current,
        event_type: payload.result,
        user_scope: payload.current_winner_id === currentUserIDRef.current ? 'self' : 'other'
      });
      setBidPhase('idle');
      window.setTimeout(() => void loadLeaderboard(payload.auction_id ?? activeAuctionIDRef.current), 250);
      void loadPayableOrderForAuction(payload.auction_id ?? activeAuctionIDRef.current);
      return;
    }
    if (acceptedWinnerID === currentUserIDRef.current) {
      setBidFeedback('你已领先，出价已确认');
      showAtmosphere({
        kind: 'leading',
        title: '领先！',
        detail: `${formatCents(acceptedPrice)} 已结算`,
        auction_id: payload.auction_id ?? activeAuctionIDRef.current,
        cause_seq: payload.seq ?? lastSeqRef.current,
        event_type: payload.result ?? 'bid_accepted',
        user_scope: 'self'
      });
    }
    const nextBidPhase = acceptedWinnerID === currentUserID ? 'accepted' : 'idle';
    bidPhaseRef.current = nextBidPhase;
    setBidPhase(nextBidPhase);
    window.setTimeout(() => void loadLeaderboard(payload.auction_id ?? activeAuctionIDRef.current), 250);
  };

  const loadPayableOrderForAuction = async (auctionID: string) => {
    if (!auctionID) return null;
    try {
      const response = await fetch(`/api/users/me/orders?auction_id=${encodeURIComponent(auctionID)}&limit=5`);
      const payload = await readJSON<{ items?: OrderRow[] }>(response);
      if (!response.ok) return null;
      const rows = payload?.items ?? [];
      const order = rows.find((row) => String(row.auction_id ?? '') === auctionID && ['ORDER_PENDING', 'PAYMENT_INITIATED', 'PAID'].includes(String(row.order_status ?? '')));
      setPayableOrderID(order?.order_id ? String(order.order_id) : '');
      setPayableOrderAmountCents(Number(order?.amount_cents ?? 0));
      if (order?.order_status === 'PAID') {
        commitPaymentPhase('paid');
      } else if (order?.order_status === 'PAYMENT_INITIATED' && !isTerminalPaymentPhase(paymentPhaseRef.current)) {
        commitPaymentPhase('pending');
        void reconcilePayment(String(order.order_id ?? ''));
      } else if (order?.order_status === 'ORDER_PENDING' && !isTerminalPaymentPhase(paymentPhaseRef.current)) {
        commitPaymentPhase('idle');
      }
      return order ?? null;
    } catch {
      if (!isTerminalPaymentPhase(paymentPhaseRef.current)) {
        setPayableOrderID('');
        setPayableOrderAmountCents(0);
      }
      return null;
    }
  };

  const resetAuctionSessionState = () => {
    pendingBidRef.current = null;
    bidInFlightRef.current = false;
    paymentInFlight.current = false;
    setBidPhase('idle');
    commitPaymentPhase('idle');
    setRiskCode('');
    setConfirmToken('');
    setConfirmIdempotencyKey('');
    setConfirmAmountCents(0);
    setPayableOrderID('');
    setPayableOrderAmountCents(0);
    setTerminalPriceCents(0);
    setTerminalWinnerID('');
    setTerminalWinnerMasked('');
    setTerminalSeq(0);
    setExtensionNotice('');
    setAtmosphereCue(null);
    setWaterfallChips([]);
    setLeaderboard(null);
    hotAcceptedPreviewRef.current = null;
    commitMaxBidIntentState(null);
    setMaxBidPhase('idle');
    setMaxBidFeedback('仅自己可见，服务端按加价阶梯代出价');
    leaderboardRef.current = null;
    pendingLeaderboardDeltaRef.current = null;
    selectedRef.current = 'active_bids';
    setSelected('active_bids');
    setOverlayMode(showStateMatrix ? 'bid' : 'feed');
    setActiveSheet(null);
  };

  const enterAuctionFromSummary = async (selectedAuction: AuctionSummary, options: { switched?: boolean } = {}) => {
    if (!selectedAuction) return;
    if (options.switched || activeAuctionIDRef.current !== selectedAuction.id) {
      resetAuctionSessionState();
    }
    activeAuctionIDRef.current = selectedAuction.id;
    setActiveAuctionID(selectedAuction.id);
    setLotTitle(selectedAuction.item?.title ?? selectedAuction.id);
    if (selectedAuction.item) {
      setStageItem(normalizeStageItem(selectedAuction.item, selectedAuction.id));
    } else {
      setStageItem((current) => ({
        ...current,
        title: current.title ?? selectedAuction.id
      }));
    }
    setBidderRequirement(selectedAuction.bidder_requirement ?? null);
    const price = selectedAuction.current_price_cents ?? currentPriceRef.current;
    const increment = selectedAuction.increment_cents ?? activeIncrementCentsRef.current;
    const cap = selectedAuction.cap_price_cents ?? 0;
    const nextMaxBidAmount = clampMaxBidAmount(price + increment, price + increment, cap);
    setActiveIncrementCents(increment);
    setCurrentPriceCents(price);
    setMinimumNextBidCents(price + increment);
    setNextBidCents(price + increment);
    setMaxBidAmountCents(nextMaxBidAmount);
    setLastSeq(selectedAuction.seq ?? lastSeqRef.current);
    setAuctionEndAt(selectedAuction.end_at ?? '');
    syncServerTimeMS(selectedAuction.server_time_ms ?? 0);
    setConnectionPhase('connected');
    setRecoveryPhase('idle');
    setBidFeedback(options.switched ? '主播已切换到新拍品' : '已进入当前拍品');
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
      setBidFeedback(options.switched ? '主播已切换到新拍品' : '已进入当前拍品');
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
      setStageItem(normalizeStageItem(snapshotItem, snapshotAuctionID || activeAuctionIDRef.current));
      if (snapshotItem.title) setLotTitle(snapshotItem.title);
    }
    setBidderRequirement(snapshot.payload?.bidder_requirement ?? snapshot.bidder_requirement ?? null);
    if (snapshot.increment_cents != null) setActiveIncrementCents(increment);
    if (nextEndAt) setAuctionEndAt(nextEndAt);
    if (nextServerTimeMS) syncServerTimeMS(nextServerTimeMS);
    const snapshotCap = roomAuctionsRef.current.find((row) => row.id === (snapshotAuctionID || activeAuctionIDRef.current))?.cap_price_cents ?? 0;
    setCurrentPriceCents(price);
    setMinimumNextBidCents(price + increment);
    setNextBidCents(price + increment);
    setMaxBidAmountCents((amount) => clampMaxBidAmount(amount, price + increment, snapshotCap));
    if (snapshot.max_bid_intent && snapshot.max_bid_intent.auction_id === (snapshotAuctionID || activeAuctionIDRef.current)) {
      commitMaxBidIntentState(snapshot.max_bid_intent);
      setMaxBidAmountCents(clampMaxBidAmount(snapshot.max_bid_intent.max_amount_cents, price + increment, snapshotCap));
      setMaxBidFeedback(maxBidStatusCopy(snapshot.max_bid_intent));
    } else {
      commitMaxBidIntentState(null);
      setMaxBidFeedback('仅自己可见，服务端按加价阶梯代出价');
    }
    setLastSeq(snapshot.seq);
    setLeaderMasked(snapshot.payload?.leader_user_masked ?? leaderMasked);
      setBidFeedback(h5Copy.latestConfirmed);
    setBidPhase('idle');
    const status = snapshot.payload?.status ?? snapshot.status;
    const winnerID = snapshot.payload?.current_winner_id ?? snapshot.current_winner_id;
    const syncSelected = !showStateMatrix || selectedRef.current === 'active_bids';
    if (status === 'SOLD' && syncSelected) {
      setTerminalPriceCents(price);
      setTerminalWinnerID(winnerID ?? '');
      setTerminalWinnerMasked(snapshot.payload?.leader_user_masked ?? leaderMaskedRef.current);
      setSelected(winnerID === currentUserIDRef.current ? 'sold_winner' : 'sold_loser');
      keepBidOverlayForCurrentMode();
      if (winnerID === currentUserIDRef.current) {
        void loadPayableOrderForAuction(snapshotAuctionID ?? activeAuctionIDRef.current);
      }
    } else if (status === 'ENDED' && syncSelected) {
      setSelected('ended');
      keepBidOverlayForCurrentMode();
    } else if (status === 'CANCELLED' && syncSelected) {
      setSelected('cancelled');
      keepBidOverlayForCurrentMode();
      setBidFeedback(snapshot.payload?.reason ?? '主播已取消');
    } else if ((status === 'SCHEDULED' || status === 'DRAFT') && syncSelected) {
      setSelected('scheduled');
    } else if (status === 'ACTIVE' && syncSelected) {
      setSelected('active_bids');
    }
    void loadLeaderboard(snapshotAuctionID ?? activeAuctionIDRef.current);
  };

  const recoverFromSnapshot = async () => {
    const auctionID = activeAuctionIDRef.current;
    if (!auctionID) return;
    if (recoveryInFlightRef.current) return;
    recoveryInFlightRef.current = true;
    if (!showStateMatrix || selectedRef.current === 'active_bids') setSelected('active_bids');
    setRecoveryPhase('recovering');
    setConnectionPhase((phase) => phase === 'disconnected' ? phase : 'recovering');
    try {
      const response = await fetch(`/api/auctions/${auctionID}`);
      const snapshot = await readJSON<SnapshotResponse>(response);
      if (!response.ok || !snapshot || snapshot.stale) {
        setBidFeedback('状态较旧，正在继续刷新');
        return;
      }
      if (!snapshot.server_time_ms && !snapshot.payload?.server_time_ms) {
        snapshot.server_time_ms = responseServerTimeMS(response);
      }
      applySnapshot(snapshot);
      setRecoveryPhase('idle');
      setConnectionPhase('connected');
    } catch {
      setBidFeedback('暂未取到最新状态，正在继续确认');
    } finally {
      recoveryInFlightRef.current = false;
    }
  };

  useEffect(() => {
    if (!countdownExpired) return;
    setBidFeedback(h5Copy.settling);
    void recoverFromSnapshot();
  }, [countdownExpired]);

  const handleRealtimeEvent = (detail: AuctionRealtimeEvent) => {
    if (!detail || detail.auction_id !== activeAuctionIDRef.current) return;
    if (detail.event_type === 'leaderboard_delta') {
      applyLeaderboardDelta(detail as unknown as LeaderboardPayload);
      setConnectionPhase('connected');
      return;
    }
    const currentSeq = lastSeqRef.current;
    if (detail.event_type === 'outbox_gap_notice' || (detail.seq != null && detail.seq > currentSeq + 1)) {
      void recoverFromSnapshot();
      return;
    }
    if (detail.event_type === 'redis_engine_paused' || detail.event_type === 'redis_engine_reconciling') {
      setBidFeedback(detail.event_type === 'redis_engine_paused' ? '竞拍确认暂停，等待恢复' : '竞拍状态校对中');
      setConnectionPhase('recovering');
      return;
    }
    if (detail.event_type === 'order_paid') {
      if (detail.seq != null) setLastSeq((seq) => Math.max(seq, detail.seq ?? seq));
      if (detail.payload?.user_id === currentUserIDRef.current) {
        commitPaymentPhase('paid');
        setSelected('sold_winner');
        if (detail.payload?.order_id) setPayableOrderID(detail.payload.order_id);
        if (detail.payload?.amount_cents != null) setPayableOrderAmountCents(Number(detail.payload.amount_cents));
      }
      setBidFeedback('订单状态已更新');
      return;
    }
    if (detail.event_type === 'order_expired') {
      if (detail.seq != null) setLastSeq((seq) => Math.max(seq, detail.seq ?? seq));
      if (detail.payload?.user_id === currentUserIDRef.current) {
        commitPaymentPhase('expired');
        setSelected('sold_winner');
        if (detail.payload?.order_id) setPayableOrderID(detail.payload.order_id);
      }
      setBidFeedback('订单已超时');
      return;
    }
    if (detail.seq == null || detail.seq <= currentSeq) return;
    const price = detail.payload?.current_price_cents ?? detail.payload?.amount_cents ?? currentPriceRef.current;
    const increment = activeIncrementCentsRef.current;
    const nextEndAt = detail.payload?.end_at ?? detail.end_at;
    const nextServerTimeMS = detail.payload?.server_time_ms ?? detail.server_time_ms;
    const previousEndAt = auctionEndAtRef.current;
    const previousWinnerID = leaderboardRef.current?.current_winner_id;
    const winnerID = detail.payload?.current_winner_id ?? detail.payload?.user_id ?? '';
    const isAutoMaxBid = detail.payload?.bid_source === 'AUTO_MAX_BID';
    const hasActiveMaxBidIntent = maxBidIntentRef.current?.auction_id === detail.auction_id && maxBidIntentRef.current?.status === 'ACTIVE';
    const cancelFenceSeq = maxBidCancelFenceRef.current[detail.auction_id] ?? 0;
    const isAfterCancelFence = detail.seq != null && cancelFenceSeq > 0 && detail.seq <= cancelFenceSeq;
    const isConfirmedAutoMaxBid = isAutoMaxBid && hasActiveMaxBidIntent && !isAfterCancelFence;
    setCurrentPriceCents(price);
    setMinimumNextBidCents(price + increment);
    setNextBidCents((prepared) => Math.max(price + increment, prepared));
    const eventCap = roomAuctionsRef.current.find((row) => row.id === detail.auction_id)?.cap_price_cents ?? 0;
    setMaxBidAmountCents((amount) => clampMaxBidAmount(amount, price + increment, eventCap));
    setLastSeq(detail.seq);
    if (nextEndAt) setAuctionEndAt(nextEndAt);
    if (nextServerTimeMS) syncServerTimeMS(nextServerTimeMS);
    setLeaderMasked(detail.payload?.leader_user_masked ?? leaderMaskedRef.current);
    const wasExtended = Boolean(nextEndAt && previousEndAt && Date.parse(nextEndAt) > Date.parse(previousEndAt));
    if (wasExtended && nextEndAt) {
      const extendSeconds = extendSecondsFromEvent(detail, previousEndAt, nextEndAt);
      setExtensionNotice(extensionCopyFromEvent(detail, previousEndAt, nextEndAt));
      setBidFeedback('最后时刻有出价，竞拍已延时');
      showAtmosphere({
        kind: 'extended',
        title: '已延时',
        detail: `${extendSeconds ? `延时 +${extendSeconds}s · ` : ''}最后窗口出价，竞拍继续`,
        auction_id: detail.auction_id,
        cause_seq: detail.seq,
        event_type: detail.event_type,
        user_scope: 'global'
      });
    } else {
      setBidFeedback('竞拍状态已更新');
    }
    if (detail.event_type === 'auction_sold') {
      setTerminalPriceCents(price);
      setTerminalWinnerID(winnerID);
      setTerminalWinnerMasked(detail.payload?.leader_user_masked ?? leaderMaskedRef.current);
      setTerminalSeq(detail.seq ?? lastSeqRef.current);
      if (detail.payload?.order_id && winnerID === currentUserIDRef.current) {
        setPayableOrderID(detail.payload.order_id);
        setPayableOrderAmountCents(price);
      }
      setSelected(winnerID === currentUserIDRef.current ? 'sold_winner' : 'sold_loser');
      keepBidOverlayForCurrentMode();
      showAtmosphere({
        kind: 'sold',
        title: winnerID === currentUserIDRef.current ? '成交！' : '已成交',
        detail: winnerID === currentUserIDRef.current
          ? (isConfirmedAutoMaxBid ? `自动加价防守到 ${formatCents(price)} 并成交` : '你已中拍，订单待支付')
          : '本场已落槌',
        auction_id: detail.auction_id,
        cause_seq: detail.seq,
        event_type: detail.event_type,
        user_scope: winnerID === currentUserIDRef.current ? 'self' : 'other'
      });
      setBidPhase('idle');
      if (winnerID === currentUserIDRef.current) {
        void loadPayableOrderForAuction(detail.auction_id);
      }
    } else if (detail.event_type === 'auction_ended') {
      setSelected('ended');
      keepBidOverlayForCurrentMode();
      setBidPhase('idle');
    } else if (detail.event_type === 'auction_cancelled') {
      setSelected('cancelled');
      keepBidOverlayForCurrentMode();
      setBidFeedback(detail.payload?.reason ?? '主播已取消');
      setBidPhase('idle');
    } else if (winnerID === currentUserIDRef.current || detail.payload?.current_winner_id === currentUserIDRef.current) {
      setBidPhase('accepted');
      setBidFeedback(isConfirmedAutoMaxBid ? `自动加价已防守到 ${formatCents(price)}` : '你已领先，出价已确认');
      showAtmosphere({
        kind: 'leading',
        title: isConfirmedAutoMaxBid ? '自动防守成功' : '领先！',
        detail: isConfirmedAutoMaxBid ? `系统已按阶梯代你出到 ${formatCents(price)}` : `${formatCents(price)} 已确认`,
        auction_id: detail.auction_id,
        cause_seq: detail.seq,
        event_type: detail.event_type,
        user_scope: 'self'
      });
    } else if (winnerID && winnerID !== currentUserIDRef.current) {
      pendingBidRef.current = null;
      if (previousWinnerID === currentUserIDRef.current || bidPhase === 'accepted') {
        showAtmosphere({
          kind: 'outbid',
          title: '被超越！',
          detail: `${detail.payload?.leader_user_masked ?? '其他用户'} 已领先`,
          auction_id: detail.auction_id,
          cause_seq: detail.seq,
          event_type: detail.event_type,
          user_scope: 'self'
        });
        setOverlayMode('bid');
      }
      setBidPhase('idle');
      setConfirmToken('');
      setConfirmIdempotencyKey('');
      setConfirmAmountCents(0);
    }
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
        const selectedAuction = selectEntryAuction(auctions);
        if (!response.ok || !selectedAuction || cancelled) return;
        setRoomAuctions(visibleRoomAuctions(auctions));
        await enterAuctionFromSummary(selectedAuction);
      } catch {
        setBidFeedback('拍品列表暂不可用');
      }
    };
    void loadActiveAuction();
    return () => {
      cancelled = true;
    };
  }, [sessionReady]);

  useEffect(() => {
    if (!sessionReady) return undefined;
    let cancelled = false;
    const refreshRoomAuctions = async () => {
      try {
        const response = await fetch(`/api/rooms/${roomID}/auctions`);
        const payload = await readJSON<AuctionSummary[] | { items?: AuctionSummary[] }>(response);
        if (!response.ok || cancelled) return;
        const auctions = Array.isArray(payload) ? payload : payload?.items ?? [];
        const visible = visibleRoomAuctions(auctions);
        setRoomAuctions(visible);
        const liveAuction = visible.find((auction) => auction.status === 'ACTIVE');
        if (liveAuction && liveAuction.id !== activeAuctionIDRef.current) {
          await enterAuctionFromSummary(liveAuction, { switched: true });
        }
      } catch {
        if (!cancelled) setBidFeedback('拍品列表暂不可用');
      }
    };
    const timer = window.setInterval(refreshRoomAuctions, 3_000);
    void refreshRoomAuctions();
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [sessionReady, roomID]);

  useEffect(() => {
    let cancelled = false;
    let timer: number | null = null;
    const shouldPoll =
      sessionReady &&
      activeAuctionID &&
      !payableOrderID &&
      (selected === 'sold_winner' || bidPhase === 'engine_sold_pending');
    if (!shouldPoll) {
      orderPollStartedAtRef.current = 0;
      return () => {
        cancelled = true;
      };
    }
    orderPollStartedAtRef.current = orderPollStartedAtRef.current || Date.now();
    const poll = async () => {
      const order = await loadPayableOrderForAuction(activeAuctionID);
      if (cancelled || order) return;
      if (Date.now() - orderPollStartedAtRef.current > 12_000) {
        setBidFeedback('订单仍在生成，请打开订单记录刷新');
        return;
      }
      timer = window.setTimeout(poll, 800);
    };
    void poll();
    return () => {
      cancelled = true;
      if (timer != null) window.clearTimeout(timer);
    };
  }, [activeAuctionID, bidPhase, payableOrderID, selected, sessionReady]);

  const loadLiveOpsCampaign = async () => {
    try {
      const response = await fetch(`/api/rooms/${roomID}/liveops`);
      const payload = await readJSON<LiveOpsCampaign>(response);
      if (response.ok && payload) {
        setLiveOpsCampaign(payload);
        if (payload.my_team === 'craft' || payload.my_team === 'story') setActiveBuyerTeam(payload.my_team);
        setLiveOpsError('');
      } else {
        setLiveOpsError('互动准备暂不可用，请稍后再试');
      }
    } catch {
      setLiveOpsError('互动准备暂不可用，请稍后再试');
    }
  };

  useEffect(() => {
    if (!sessionReady) return;
    void loadLiveOpsCampaign();
  }, [sessionReady, roomID]);

  const completeLiveOpsTask = async (taskKey: 'watch' | 'follow' | 'ask' | 'leaderboard') => {
    if (liveOpsBusy) return false;
    setLiveOpsBusy(taskKey);
    setLiveOpsError('');
    try {
      await ensureBuyerSession();
      const response = await fetch(`/api/rooms/${roomID}/liveops/tasks/${taskKey}`, { method: 'POST' });
      const payload = await readJSON<LiveOpsCampaign>(response);
      if (response.ok && payload) {
        setLiveOpsCampaign(payload);
        if (payload.my_team === 'craft' || payload.my_team === 'story') setActiveBuyerTeam(payload.my_team);
        return true;
      }
      setLiveOpsError('互动准备暂不可用，请稍后再试');
      return false;
    } catch {
      setLiveOpsError('互动准备暂不可用，请稍后再试');
      return false;
    } finally {
      setLiveOpsBusy('');
    }
  };

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
    let cancelled = false;
    const loadSystemMessages = async () => {
      if (!sessionReady) return;
      try {
        const response = await fetch(`/api/rooms/${roomID}/system-messages?limit=10`);
        const payload = await readJSON<{ items?: SystemMessage[] }>(response);
        if (!response.ok || cancelled) return;
        setSystemMessages(payload?.items ?? []);
      } catch {
        if (!cancelled) setSystemMessages([]);
      }
    };
    void loadSystemMessages();
    const timer = window.setInterval(loadSystemMessages, 1_000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [sessionReady, roomID]);

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
    const scheduleReconnect = (retryAfter = 0) => {
      clearReconnect();
      reconnectAttemptRef.current += 1;
      const delay = reconnectDelayMS(reconnectAttemptRef.current, retryAfter);
      reconnectTimerRef.current = window.setTimeout(() => {
        if (!cancelled) void connectWebSocket();
      }, delay);
    };
    const connectWebSocket = async () => {
      if (!sessionReady || !activeAuctionID) return;
      setConnectionPhase((phase) => phase === 'recovering' ? phase : 'connecting');
      try {
        const ticketRequest = wsTicketRequest(roomID, activeAuctionID, currentUserIDRef.current);
        const ticketResponse = await fetch(ticketRequest.url, ticketRequest.init);
        const ticketPayload = await readJSON<WSTicketResponse>(ticketResponse);
        if (!ticketResponse.ok || !ticketPayload?.ticket) {
          scheduleReconnect(retryAfterMSFromHeaders(ticketResponse));
          return;
        }
        if (cancelled) return;
        const wsURL = auctionWSURL(window.location.origin, roomID, activeAuctionID, lastSeqRef.current);
        const socket = new WebSocket(wsURL, auctionWSProtocols(ticketPayload.ticket));
        wsRef.current = socket;
        socket.onopen = () => {
          if (!cancelled) {
            reconnectAttemptRef.current = 0;
            setConnectionPhase('connected');
          }
        };
        socket.onmessage = (message) => {
          try {
            handleRealtimeEvent(JSON.parse(String(message.data)) as AuctionRealtimeEvent);
          } catch {
            setBidFeedback('实时消息解析失败，正在刷新');
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

  useEffect(() => {
    if (!sessionReady || !activeAuctionID || activeSheet !== 'maxBid') return;
    void loadMaxBidIntent(activeAuctionID);
  }, [activeAuctionID, activeSheet, sessionReady]);

  useEffect(() => {
    if (!sessionReady || !activeAuctionID || selected !== 'active_bids') return undefined;
    if (connectionPhase === 'connected' && recoveryPhase === 'idle' && !countdownExpired && !bidCloseGuardActive) return undefined;
    const timer = window.setInterval(() => {
      void recoverFromSnapshot();
    }, 2_500);
    return () => window.clearInterval(timer);
  }, [activeAuctionID, bidCloseGuardActive, connectionPhase, countdownExpired, recoveryPhase, sessionReady, selected]);

  const submitBid = async () => {
    const auctionID = activeAuctionIDRef.current;
    const canRetryPending = canRetryPendingBid({ pending: pendingBidRef.current, auctionID, bidPhase, riskCode });
    if (selected !== 'active_bids' || (!canRetryPending && scenario.ctaDisabled) || !auctionID) return;
    if (!canRetryPending && isBidCloseGuardActive(auctionEndAtRef.current, serverTimeMSRef.current, Date.now(), serverTimeSyncedAtRef.current)) {
      setBidFeedback('已进入服务端落槌保护，正在确认最终结果');
      setBidPhase('rejected');
      void recoverFromSnapshot();
      return;
    }
    if (bidInFlightRef.current || bidCooldownUntilMS > Date.now()) return;
    bidInFlightRef.current = true;
    const cap = roomAuctionsRef.current.find((row) => row.id === auctionID)?.cap_price_cents ?? 0;
    const submittedAmountCents = canRetryPending
      ? nextBidCents
      : normalizeManualBidCents(yuanInputToCents(bidAmountText), currentPriceCents, minimumNextBidCents, activeIncrementCents, cap);
    if (!canRetryPending) {
      setNextBidCents(submittedAmountCents);
      setBidAmountText((submittedAmountCents / 100).toFixed(2));
    }
    const bidRequest = prepareBidRequest({ pending: pendingBidRef.current, auctionID, amountCents: submittedAmountCents, clientSeenSeq: lastSeq });
    pendingBidRef.current = bidRequest;
    setBidPhase('pending');
    try {
      if (!canRetryPending) {
        await ensureBuyerSession();
      }
      const response = await fetchWithTimeout(`/api/auctions/${auctionID}/bids`, {
        method: 'POST',
        headers: bidRequestHeaders(bidRequest),
        body: JSON.stringify(bidRequestPayload(bidRequest))
      });
      const payload = await response.json() as BidResponse;
      const decision = interpretBidResponse({
        ok: response.ok,
        payload,
        retryAfterMS: retryAfterMS(response, payload),
        activeIncrementCents: activeIncrementCentsRef.current
      });
      if (decision.kind === 'confirm_required') {
        setConfirmToken(decision.token);
        setConfirmIdempotencyKey(bidRequest.clientBidID);
        setConfirmAmountCents(decision.amountCents ?? bidRequest.amountCents);
        pendingBidRef.current = null;
        setBidFeedback('高额出价需要二次确认');
        setBidPhase('confirm_required');
        return;
      }
      if (decision.kind === 'rejected') {
        if (decision.nextValidBidCents != null) {
          setMinimumNextBidCents(decision.nextValidBidCents);
          setNextBidCents((prepared) => Math.max(decision.nextValidBidCents ?? prepared, prepared));
        }
        if (decision.retryAfterMS && decision.retryAfterMS > 0) {
          setBidCooldownUntilMS(Date.now() + decision.retryAfterMS);
        }
        setRiskCode(decision.code);
        setBidFeedback(rejectCopy(decision.code));
        if (!decision.keepPending) {
          pendingBidRef.current = null;
        }
        setBidPhase(decision.keepPending ? 'processing_retry' : 'rejected');
        return;
      }
      setRiskCode('');
      setBidCooldownUntilMS(0);
      if (decision.kind === 'accepted' && decision.clearPending) {
        pendingBidRef.current = null;
      }
      applyAcceptedBid(payload);
    } catch {
      const failure = networkBidFailure();
      if (failure.kind === 'uncertain') setRiskCode(failure.code);
      setBidFeedback('响应丢失，使用同一请求确认结果');
      setBidPhase('uncertain');
    } finally {
      bidInFlightRef.current = false;
    }
  };

  const confirmBid = async () => {
    const auctionID = activeAuctionIDRef.current;
    if (!confirmToken || !confirmIdempotencyKey || scenario.ctaDisabled || !auctionID) return;
    if (bidInFlightRef.current || bidCooldownUntilMS > Date.now()) return;
    bidInFlightRef.current = true;
    setBidPhase('confirming');
    try {
      await ensureBuyerSession();
      const response = await fetchWithTimeout(`/api/auctions/${auctionID}/bids/confirm`, {
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
      if (!response.ok || (payload.reject_reason && !isEngineRejected(payload)) || payload.code) {
        const code = payload.reject_reason ?? payload.code ?? '';
        const retryMS = retryAfterMS(response, payload);
        if (retryMS > 0) {
          setBidCooldownUntilMS(Date.now() + retryMS);
        }
        setRiskCode(code);
        setBidFeedback(rejectCopy(code));
        setBidPhase('rejected');
        return;
      }
      setRiskCode('');
      setBidCooldownUntilMS(0);
      applyAcceptedBid(payload);
    } catch {
      setRiskCode('NETWORK_ERROR');
      setBidFeedback('网络异常，请重试');
      setBidPhase('rejected');
    } finally {
      bidInFlightRef.current = false;
    }
  };

  const loadMaxBidIntent = async (auctionID = activeAuctionIDRef.current) => {
    if (!auctionID) return;
    try {
      const response = await fetch(`/api/auctions/${auctionID}/max-bid-intent`);
      if (auctionID !== activeAuctionIDRef.current) return;
      if (response.status === 404) {
        commitMaxBidIntentState(null);
        if (maxBidPhaseRef.current === 'idle') {
          setMaxBidFeedback('仅自己可见，服务端按加价阶梯代出价');
        }
        return;
      }
      const payload = await readJSON<MaxBidIntent>(response);
      if (!response.ok || !payload) return;
      if (payload.auction_id !== activeAuctionIDRef.current) return;
      commitMaxBidIntentState(payload);
      const cap = roomAuctionsRef.current.find((row) => row.id === auctionID)?.cap_price_cents ?? 0;
      setMaxBidAmountCents(clampMaxBidAmount(payload.max_amount_cents, minimumNextBidCents, cap));
      setMaxBidAmountText((current) => current || (payload.max_amount_cents / 100).toFixed(2));
      setMaxBidFeedback(maxBidStatusCopy(payload));
    } catch {
      setMaxBidFeedback('自动加价状态读取失败');
    }
  };

  const submitMaxBidIntent = async () => {
    const auctionID = activeAuctionIDRef.current;
    if (
      !auctionID ||
      (maxBidPhase !== 'idle' && maxBidPhase !== 'error') ||
      scenario.stale ||
      scenario.sold ||
      connectionPhase === 'connecting' ||
      connectionPhase === 'recovering' ||
      connectionPhase === 'disconnected'
    ) return;
    const parsedAmount = yuanInputToCents(maxBidAmountText) || maxBidAmountCentsRef.current;
    const cap = roomAuctions.find((row) => row.id === auctionID)?.cap_price_cents ?? 0;
    const requiredMaxBidCents = cap > 0 ? Math.min(minimumNextBidCents, cap) : minimumNextBidCents;
    if (parsedAmount < requiredMaxBidCents) {
      setMaxBidPhase('error');
      setMaxBidFeedback(`自动加价上限不能低于当前可设置价 ${formatCents(requiredMaxBidCents)}`);
      return;
    }
    if (cap > 0 && parsedAmount > cap) {
      setMaxBidPhase('error');
      setMaxBidFeedback(`自动加价上限不能高于封顶价 ${formatCents(cap)}`);
      return;
    }
    const amount = parsedAmount;
    maxBidPhaseRef.current = 'pending';
    setMaxBidPhase('pending');
    setMaxBidFeedback('正在确认自动加价');
    try {
      await ensureBuyerSession();
      const key = createClientBidID();
      const response = await fetch(`/api/auctions/${auctionID}/max-bid-intent`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': key
        },
        body: JSON.stringify({
          max_amount_cents: amount,
          client_seen_seq: lastSeq,
          source: 'MAX_BID'
        })
      });
      const payload = await readJSON<{ intent?: MaxBidIntent; trigger_bid?: BidResponse; code?: string; message?: string }>(response);
      if (!response.ok || !payload?.intent) {
        maxBidPhaseRef.current = 'error';
        setMaxBidPhase('error');
        setMaxBidFeedback(maxBidErrorCopy(payload?.code ?? payload?.message));
        return;
      }
      commitMaxBidIntentState(payload.intent);
      setMaxBidAmountCents(Math.max(payload.intent.max_amount_cents, minimumNextBidCents));
      setMaxBidAmountText((payload.intent.max_amount_cents / 100).toFixed(2));
      setMaxBidFeedback(maxBidStatusCopy(payload.intent));
      if (payload.trigger_bid) {
        applyAcceptedBid(payload.trigger_bid);
        const triggerPrice = payload.trigger_bid.current_price_cents ?? currentPriceRef.current;
        if (payload.trigger_bid.current_winner_id === currentUserIDRef.current) {
          const sold = payload.trigger_bid.result === 'ENGINE_SOLD' || payload.trigger_bid.result === 'ACCEPTED_SOLD';
          setBidFeedback(sold ? `自动加价防守到 ${formatCents(triggerPrice)} 并成交` : `自动加价已防守到 ${formatCents(triggerPrice)}`);
          setMaxBidFeedback(`自动加价已生效，已代你防守到 ${formatCents(triggerPrice)}`);
          showAtmosphere({
            kind: sold ? 'sold' : 'leading',
            title: sold ? '自动成交' : '自动防守成功',
            detail: sold ? `系统已按阶梯代你出到 ${formatCents(triggerPrice)} 并成交` : `系统已按阶梯代你出到 ${formatCents(triggerPrice)}`,
            auction_id: payload.trigger_bid.auction_id ?? auctionID,
            cause_seq: payload.trigger_bid.engine_seq ?? payload.trigger_bid.seq ?? lastSeqRef.current,
            event_type: payload.trigger_bid.result ?? 'AUTO_MAX_BID',
            user_scope: 'self'
          });
        }
      }
      maxBidPhaseRef.current = 'idle';
      setMaxBidPhase('idle');
    } catch {
      maxBidPhaseRef.current = 'error';
      setMaxBidPhase('error');
      setMaxBidFeedback('网络异常，自动加价未确认');
    }
  };

  const cancelMaxBidIntent = async () => {
    const intent = visibleMaxBidIntent;
    const auctionID = activeAuctionIDRef.current;
    if (
      !auctionID ||
      !intent ||
      intent.auction_id !== auctionID ||
      intent.status !== 'ACTIVE' ||
      maxBidPhase !== 'idle' ||
      connectionPhase === 'connecting' ||
      connectionPhase === 'recovering' ||
      connectionPhase === 'disconnected'
    ) return;
    maxBidPhaseRef.current = 'canceling';
    setMaxBidPhase('canceling');
    setMaxBidFeedback('正在取消自动加价');
    try {
      await ensureBuyerSession();
      const key = createClientBidID();
      const response = await fetch(`/api/auctions/${auctionID}/max-bid-intent`, {
        method: 'DELETE',
        headers: { 'Idempotency-Key': key }
      });
      const payload = await readJSON<{ intent?: MaxBidIntent; code?: string; message?: string }>(response);
      if (!response.ok || !payload?.intent) {
        if (response.status === 404 || payload?.code === 'AUCTION_NOT_FOUND') {
          commitMaxBidIntentState(null);
          maxBidPhaseRef.current = 'idle';
          setMaxBidPhase('idle');
          setMaxBidFeedback('没有找到可取消的自动加价，已刷新本地状态');
          void loadMaxBidIntent(activeAuctionIDRef.current);
          return;
        }
        maxBidPhaseRef.current = 'error';
        setMaxBidPhase('error');
        setMaxBidFeedback(maxBidErrorCopy(payload?.code ?? payload?.message));
        return;
      }
      const cancelledIntent = payload.intent;
      if (cancelledIntent.status === 'CANCELLED') {
        commitMaxBidIntentState(cancelledIntent.auction_id === auctionID ? cancelledIntent : null);
        markMaxBidCancelledFence(auctionID);
        clearAutoMaxBidPresentation(maxBidStatusCopy(cancelledIntent));
        maxBidPhaseRef.current = 'idle';
        setMaxBidPhase('idle');
        return;
      }
      const verify = await fetch(`/api/auctions/${auctionID}/max-bid-intent`);
      if (verify.status === 404) {
        commitMaxBidIntentState(null);
        markMaxBidCancelledFence(auctionID);
        clearAutoMaxBidPresentation('自动加价已取消');
        maxBidPhaseRef.current = 'idle';
        setMaxBidPhase('idle');
        return;
      }
      const verifiedIntent = await readJSON<MaxBidIntent>(verify);
      if (!verify.ok || !verifiedIntent || verifiedIntent.status === 'ACTIVE') {
        maxBidPhaseRef.current = 'error';
        setMaxBidPhase('error');
        setMaxBidFeedback('取消状态未确认，请重试');
        void loadMaxBidIntent(auctionID);
        return;
      }
      commitMaxBidIntentState(verifiedIntent.auction_id === auctionID ? verifiedIntent : cancelledIntent);
      markMaxBidCancelledFence(auctionID);
      clearAutoMaxBidPresentation(maxBidStatusCopy(verifiedIntent.auction_id === auctionID ? verifiedIntent : cancelledIntent));
      maxBidPhaseRef.current = 'idle';
      setMaxBidPhase('idle');
    } catch {
      maxBidPhaseRef.current = 'error';
      setMaxBidPhase('error');
      setMaxBidFeedback('网络异常，自动加价未取消');
    }
  };

  const payOrder = async () => {
    if (selected !== 'sold_winner' || scenario.ctaDisabled || paymentInFlight.current || !payableOrderID) return;
    paymentInFlight.current = true;
    commitPaymentPhase('pending');
    try {
      await ensureBuyerSession();
      const result = await submitOrderPayment(payableOrderID);
      commitPaymentPhase(result.phase);
      if (result.phase === 'paid') {
        setActiveSheet('orders');
        void loadHistory();
      }
    } catch {
      commitPaymentPhase('failed');
    } finally {
      paymentInFlight.current = false;
    }
  };

  const reconcilePayment = async (orderID = payableOrderID) => {
    if (!orderID || paymentInFlight.current || paymentPhase === 'paid') return;
    try {
      await ensureBuyerSession();
      const result = await queryOrderPayment(orderID);
      if (result.phase === 'paid') {
        commitPaymentPhase('paid');
        setSelected('sold_winner');
        setActiveSheet('orders');
        void loadHistory();
      } else if (result.phase === 'pending') {
        commitPaymentPhase('pending');
      }
    } catch {
      void loadHistory();
    }
  };

  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === 'visible') void reconcilePayment();
    };
    window.addEventListener('focus', onVisible);
    document.addEventListener('visibilitychange', onVisible);
    if (paymentPhase === 'pending') void reconcilePayment();
    return () => {
      window.removeEventListener('focus', onVisible);
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [payableOrderID, paymentPhase]);

  const loadLeaderboard = async (auctionID = activeAuctionIDRef.current) => {
    if (!auctionID) return;
    try {
      const response = await fetch(`/api/auctions/${auctionID}/leaderboard?limit=5`);
      const payload = await readJSON<LeaderboardPayload>(response);
      if (response.ok && payload) {
        if (payload.seq != null && payload.seq < lastSeqRef.current) return;
        const localAccepted = bidPhaseRef.current === 'accepted' || bidPhaseRef.current === 'engine_sold_pending';
        if (
          localAccepted &&
          payload.seq != null &&
          payload.seq <= lastSeqRef.current &&
          payload.current_winner_id &&
          payload.current_winner_id !== currentUserIDRef.current
        ) {
          return;
        }
        setConnectionPhase('connected');
        void loadPresenceHeat(auctionID);
        setLeaderboard((previous) => {
          const current = leaderboardRef.current ?? previous;
          if (payload.auction_id === current?.auction_id && payload.seq != null && current.seq != null && payload.seq < current.seq) {
            return previous;
          }
          const normalized = normalizeLeaderboardPayload(payload, current);
          leaderboardRef.current = normalized;
          syncBidPhaseFromLeaderboard(normalized);
          return normalized;
        });
      }
    } catch {
      setLeaderboard(null);
      void loadPresenceHeat(auctionID);
    }
  };

  const loadPresenceHeat = async (auctionID = activeAuctionIDRef.current) => {
    if (!auctionID) {
      setPresenceHeat(null);
      return;
    }
    try {
      const response = await fetch(`/api/auctions/${auctionID}/heat`);
      const payload = await readJSON<{ watcher_count_available?: boolean; watcher_count?: number } | null>(response);
      setPresenceHeat(response.ok ? {
        watcherCountAvailable: Boolean(payload?.watcher_count_available),
        watcherCount: typeof payload?.watcher_count === 'number' ? payload.watcher_count : undefined
      } : null);
    } catch {
      setPresenceHeat(null);
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

  const openBidOverlay = () => {
    setOverlayMode('bid');
    setActiveSheet(null);
  };

  const decreaseBidAmount = () => {
    setNextBidCents((amount) => Math.max(minimumNextBidCents, amount - activeIncrementCents));
  };

  const increaseBidAmount = () => {
    setNextBidCents((amount) => amount + activeIncrementCents);
  };

  const commitBidAmountText = (value = bidAmountText) => {
    const cap = roomAuctionsRef.current.find((row) => row.id === activeAuctionIDRef.current)?.cap_price_cents ?? 0;
    const normalized = normalizeManualBidCents(yuanInputToCents(value), currentPriceCents, minimumNextBidCents, activeIncrementCents, cap);
    setNextBidCents(normalized);
    setBidAmountText((normalized / 100).toFixed(2));
  };

  const changeBidAmountText = (value: string) => {
    const cleaned = value.replace(/[^\d.]/g, '');
    const parts = cleaned.split('.');
    const normalized = parts.length > 1 ? `${parts[0]}.${parts.slice(1).join('').slice(0, 2)}` : parts[0];
    setBidAmountText(normalized);
    const parsed = yuanInputToCents(normalized);
    if (parsed > 0) {
      const cap = roomAuctionsRef.current.find((row) => row.id === activeAuctionIDRef.current)?.cap_price_cents ?? 0;
      setNextBidCents(normalizeManualBidCents(parsed, currentPriceCents, minimumNextBidCents, activeIncrementCents, cap));
    }
  };

  const decreaseMaxBidAmount = () => {
    setMaxBidPhase((phase) => phase === 'error' ? 'idle' : phase);
    const cap = roomAuctionsRef.current.find((row) => row.id === activeAuctionIDRef.current)?.cap_price_cents ?? 0;
    const required = cap > 0 ? Math.min(minimumNextBidCents, cap) : minimumNextBidCents;
    setMaxBidAmountCents((amount) => Math.max(required, amount - activeIncrementCents));
  };

  const increaseMaxBidAmount = () => {
    setMaxBidPhase((phase) => phase === 'error' ? 'idle' : phase);
    const cap = roomAuctionsRef.current.find((row) => row.id === activeAuctionIDRef.current)?.cap_price_cents ?? 0;
    setMaxBidAmountCents((amount) => clampMaxBidAmount(amount + activeIncrementCents, minimumNextBidCents, cap));
  };

  const changeMaxBidAmountText = (value: string) => {
    setMaxBidPhase((phase) => phase === 'error' ? 'idle' : phase);
    const cleaned = value.replace(/[^\d.]/g, '');
    const parts = cleaned.split('.');
    const normalized = parts.length > 1 ? `${parts[0]}.${parts.slice(1).join('').slice(0, 2)}` : parts[0];
    setMaxBidAmountText(normalized);
    const cents = yuanInputToCents(normalized);
    if (cents > 0) setMaxBidAmountCents(cents);
  };

  const loadHistory = async () => {
    setHistoryLoading(true);
    setHistoryError('');
    try {
      await ensureBuyerSession();
      const currentAuctionID = activeAuctionIDRef.current;
      const scopedQuery = '?limit=20';
      const [bids, orders] = await Promise.all([
        fetch(`/api/users/me/bids${scopedQuery}`).then((response) => response.json()),
        fetch(`/api/users/me/orders${scopedQuery}`).then((response) => response.json())
      ]);
      setBidHistory(Array.isArray(bids.items) ? bids.items : []);
      const orderRows = Array.isArray(orders.items) ? orders.items : [];
      setOrderHistory(orderRows);
      const currentOrder = orderRows.find((row: HistoryRow) => (
        ['ORDER_PENDING', 'PAYMENT_INITIATED', 'PAID'].includes(String(row.order_status ?? row.status ?? '')) &&
        String(row.auction_id ?? '') === currentAuctionID
      ));
      if (currentOrder?.order_id) {
        setPayableOrderID(String(currentOrder.order_id));
        setPayableOrderAmountCents(Number(currentOrder.amount_cents ?? 0));
        if (currentOrder.order_status === 'PAID') commitPaymentPhase('paid');
        else if (currentOrder.order_status === 'PAYMENT_INITIATED' && !isTerminalPaymentPhase(paymentPhaseRef.current)) commitPaymentPhase('pending');
      }
    } catch {
      setHistoryError('历史读取失败');
    } finally {
      setHistoryLoading(false);
    }
  };

  useEffect(() => {
    if (activeSheet !== 'orders' && activeSheet !== 'history') return;
    void loadHistory();
  }, [activeSheet, activeAuctionID]);

  const sendChat = async () => {
    const body = chatDraft.trim();
    if (!body || chatSending) return;
    setChatSending(true);
    try {
      await ensureBuyerSession();
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

  const askProductQA = async (prompt?: string) => {
    const question = (prompt ?? qaDraft).trim();
    if (!question || qaLoading || !activeAuctionID) return;
    setQALoading(true);
    const threadID = qaThreadID || `qa_${activeAuctionID}_${Date.now().toString(36)}`;
    if (!qaThreadID) setQAThreadID(threadID);
    const history: ProductQATurn[] = qaHistory.slice(-4).map((turn) => ({
      question: turn.question || '',
      answer: turn.answer,
      facts_used: turn.facts_used
    })).filter((turn) => turn.question && turn.answer);
    try {
      await ensureBuyerSession();
      const response = await fetch(`/api/rooms/${roomID}/product-qa`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ auction_id: activeAuctionID, thread_id: threadID, question, history })
      });
      const payload = await readJSON<ProductQAAnswer | { answer?: ProductQAAnswer }>(response);
      const answer = payload && 'answer' in payload && typeof payload.answer === 'object' ? payload.answer : payload as ProductQAAnswer | undefined;
      if (response.ok && answer) {
        const nextAnswer = { ...answer, thread_id: answer.thread_id || threadID, question: answer.question || question };
        setQAAnswer(nextAnswer);
        setQAHistory((current) => [...current, nextAnswer].slice(-8));
        setQADraft('');
        void completeLiveOpsTask('ask');
      } else {
        const failedAnswer = { auction_id: activeAuctionID, thread_id: threadID, question, answer: '未提供', facts_used: [], safety_note: '问答暂不可用，请稍后重试。' };
        setQAAnswer(failedAnswer);
        setQAHistory((current) => [...current, failedAnswer].slice(-8));
      }
    } catch {
      const failedAnswer = { auction_id: activeAuctionID, thread_id: threadID, question, answer: '未提供', facts_used: [], safety_note: '问答暂不可用，请稍后重试。' };
      setQAAnswer(failedAnswer);
      setQAHistory((current) => [...current, failedAnswer].slice(-8));
    } finally {
      setQALoading(false);
    }
  };

  const askProductQAPrompt = (prompt: string) => {
    setQADraft(prompt);
    void askProductQA(prompt);
  };

  const toggleFollow = () => {
    setFollowed((value) => {
      const next = !value;
      if (next) {
        setBidFeedback('已关注，入场牌已点亮');
        void completeLiveOpsTask('follow');
      } else {
        setBidFeedback('已取消关注');
      }
      return next;
    });
  };

  const openWarmupSheet = (sheet: BottomSheetKey, task?: 'watch' | 'leaderboard') => {
    setActiveSheet(sheet);
    if (task) void completeLiveOpsTask(task);
  };

  const selectBuyerTeam = async (team: 'craft' | 'story') => {
    setActiveBuyerTeam(team);
    setLiveOpsError('');
    try {
      await ensureBuyerSession();
      const response = await fetch(`/api/rooms/${roomID}/liveops/team`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ team_key: team })
      });
      const payload = await readJSON<LiveOpsCampaign>(response);
      if (response.ok && payload) {
        setLiveOpsCampaign(payload);
        if (payload.my_team === 'craft' || payload.my_team === 'story') setActiveBuyerTeam(payload.my_team);
        if (soundEnabledRef.current && audioContextRef.current && soundCapabilityRef.current === 'ready') {
          playLayeredCue(audioContextRef.current, 'pk_surge', soundPackRef.current);
        }
      } else {
        setLiveOpsError('讲解偏好暂不可用，请稍后再试');
      }
    } catch {
      setLiveOpsError('讲解偏好暂不可用，请稍后再试');
    }
  };

  const enterLuckyDraw = async () => {
    setLiveOpsError('');
    try {
      await ensureBuyerSession();
      const response = await fetch(`/api/rooms/${roomID}/liveops/lucky-draw/enter`, { method: 'POST' });
      const payload = await readJSON<LiveOpsCampaign>(response);
      if (response.ok && payload) {
        setLiveOpsCampaign(payload);
        if (soundEnabledRef.current && audioContextRef.current && soundCapabilityRef.current === 'ready') {
          playLayeredCue(audioContextRef.current, 'entry_badge', soundPackRef.current);
        }
        vibratePattern('leading');
      } else {
        setLiveOpsError('请先完成互动任务再领取资格');
      }
    } catch {
      setLiveOpsError('互动奖励暂不可用，请稍后再试');
    }
  };

  const openLuckyDraw = async () => {
    setLiveOpsError('');
    try {
      await ensureBuyerSession();
      const response = await fetch(`/api/rooms/${roomID}/liveops/lucky-draw/open`, { method: 'POST' });
      const payload = await readJSON<LiveOpsCampaign>(response);
      if (response.ok && payload) {
        setLiveOpsCampaign(payload);
        if (soundEnabledRef.current && audioContextRef.current && soundCapabilityRef.current === 'ready') {
          playLayeredCue(audioContextRef.current, 'lucky_open', soundPackRef.current);
        }
        vibratePattern('sold');
      } else {
        setLiveOpsError('请先领取资格再查看奖励');
      }
    } catch {
      setLiveOpsError('互动奖励暂不可用，请稍后再试');
    }
  };

  return (
    <main className="app-shell" data-perf-surface={new URLSearchParams(window.location.search).get('perfSurface') === '1' ? '1' : undefined}>
      <LiveStage
        atmosphereCue={atmosphereCue}
        chatMessages={chatMessages}
        connectionPhase={connectionPhase}
        countdownPhase={countdownPhase}
        countdownCopy={countdownCopy}
        currentUserID={currentUserID}
        followed={followed}
        heat={heat}
        item={stageItem}
        likeCount={likeCount}
        leaderboard={leaderboard}
        lotTitle={lotTitle}
        roomID={roomID}
        scenario={scenario}
        soundEnabled={soundEnabled}
        soundCapability={soundCapability}
        systemMessages={systemMessages}
        terminalPriceCents={terminalPriceCents || currentPriceCents}
        waterfallChips={waterfallChips}
        raceBoardExpanded={raceBoardExpanded}
        atmosphereIntensity={atmosphereIntensity}
        atmosphereGated={atmosphereGate.gated}
        auctions={roomAuctions}
        activeAuctionID={activeAuctionID}
        currentPriceCents={currentPriceCents}
        nextBidCents={nextBidCents}
        onLike={() => setLikeCount((count) => count + 1)}
        onOpenLiveOps={() => setActiveSheet('liveops')}
        onOpenMore={() => setActiveSheet('more')}
        onOpenProducts={() => openWarmupSheet('products', 'watch')}
        onOpenBid={openBidOverlay}
        onToggleFollow={toggleFollow}
        onToggleSound={() => void toggleSound()}
      />
      {overlayMode === 'bid' && (
        <AuctionStatePanel
          atmosphereCue={atmosphereCue}
          connectionPhase={connectionPhase}
          countdownPhase={countdownPhase}
          countdownCopy={countdownCopy}
          currentPriceCents={currentPriceCents}
          extensionNotice={extensionNotice}
          heat={heat}
          item={stageItem}
          leaderboard={leaderboard}
          minimumNextBidCents={minimumNextBidCents}
          nextBidCents={nextBidCents}
          riskCode={riskCode}
          scenario={scenario}
          nextAuction={nextAuction}
          orderAmountCents={payableOrderAmountCents}
          orderID={payableOrderID}
          paymentPhase={paymentPhase}
          resultSheetKind={resultSheetKind}
          settlementSeq={terminalSeq || lastSeq}
          terminalPriceCents={terminalPriceCents || currentPriceCents}
          terminalWinnerID={terminalWinnerID}
          terminalWinnerMasked={terminalWinnerMasked}
          bidAmountText={bidAmountText}
          onClose={() => setOverlayMode('feed')}
          onBidAmountChange={changeBidAmountText}
          onBidAmountCommit={() => commitBidAmountText()}
          onDecreaseBid={decreaseBidAmount}
          onIncreaseBid={increaseBidAmount}
          onOpenOrders={() => setActiveSheet('orders')}
          onOpenSheet={(sheet) => {
            if (sheet === 'products') openWarmupSheet('products', 'watch');
            else if (sheet === 'leaderboard') openWarmupSheet('leaderboard', 'leaderboard');
            else setActiveSheet(sheet);
          }}
          onPay={() => void payOrder()}
          onPrimaryAction={handlePrimaryAction}
        />
      )}
      {overlayMode === 'feed' && resultSheetKind ? (
        <ResultSheet
          activeSheet={activeSheet}
          heat={heat}
          item={stageItem}
          kind={resultSheetKind}
          nextAuction={nextAuction}
          orderAmountCents={payableOrderAmountCents}
          orderID={payableOrderID}
          paymentPhase={paymentPhase}
          scenario={scenario}
          settlementSeq={terminalSeq || lastSeq}
          terminalPriceCents={terminalPriceCents || currentPriceCents}
          terminalWinnerID={terminalWinnerID}
          terminalWinnerMasked={terminalWinnerMasked}
          leaderboard={leaderboard}
          userBestCents={leaderboard?.my_best_amount_cents ?? 0}
          onOpenOrders={() => {
            if (resultSheetKind === 'winner') setActiveSheet('orders');
            else if (resultSheetKind === 'loser') setActiveSheet('history');
            else openWarmupSheet('products', 'watch');
          }}
          onPay={() => void payOrder()}
        />
      ) : null}
      {showStateMatrix && (
        <StateMatrixTabs
          selected={selected}
          onSelect={(state) => {
            setSelected(state);
            setOverlayMode('bid');
          }}
        />
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
          maxBidAmountCents={maxBidAmountCents}
          maxBidAmountText={maxBidAmountText}
          maxBidFeedback={visibleMaxBidFeedback}
          maxBidIntent={visibleMaxBidIntent}
          maxBidPhase={maxBidPhase}
          minimumNextBidCents={minimumNextBidCents}
          nextBidCents={nextBidCents}
          orderHistory={orderHistory}
          paymentConfirmOpen={paymentConfirmOpen}
          payableOrderAmountCents={payableOrderAmountCents}
          payableOrderID={payableOrderID}
          qaAnswer={qaAnswer}
          qaHistory={qaHistory}
          qaDraft={qaDraft}
          qaLoading={qaLoading}
          scenario={scenario}
          activeTeam={activeBuyerTeam}
          connectionPhase={connectionPhase}
          followed={followed}
          heat={heat}
          likeCount={likeCount}
          liveOpsBusy={liveOpsBusy}
          liveOpsCampaign={liveOpsCampaign}
          liveOpsError={liveOpsError}
          soundEnabled={soundEnabled}
          onClose={() => {
            if (activeSheet === 'maxBid' && (maxBidPhase === 'pending' || maxBidPhase === 'canceling')) return;
            setActiveSheet(null);
          }}
          onCancelMaxBid={cancelMaxBidIntent}
          onChangeMaxBidAmountText={changeMaxBidAmountText}
          onClosePaymentConfirm={() => setPaymentConfirmOpen(false)}
          onDecreaseMaxBid={decreaseMaxBidAmount}
          onOpenOrderDetail={setSelectedOrderID}
          onIncreaseMaxBid={increaseMaxBidAmount}
          onOpenLeaderboard={() => openWarmupSheet('leaderboard', 'leaderboard')}
          onOpenProducts={() => openWarmupSheet('products', 'watch')}
          onOpenSheet={(sheet) => {
            if (sheet === 'products') openWarmupSheet('products', 'watch');
            else if (sheet === 'leaderboard') openWarmupSheet('leaderboard', 'leaderboard');
            else setActiveSheet(sheet);
          }}
          onRefreshHistory={loadHistory}
          onRefreshLeaderboard={() => void loadLeaderboard()}
          onRefreshMaxBid={() => void loadMaxBidIntent()}
          onAskQA={() => void askProductQA()}
          onAskQAPrompt={askProductQAPrompt}
          onEnterLuckyDraw={() => void enterLuckyDraw()}
          onOpenLuckyDraw={() => void openLuckyDraw()}
          onQADraftChange={setQADraft}
          onSelectTeam={(team) => void selectBuyerTeam(team)}
          onSubmitMaxBid={submitMaxBidIntent}
          onConfirmPay={() => {
            setPaymentConfirmOpen(false);
            void payOrder();
          }}
          selectedOrderID={selectedOrderID}
          onToggleFollow={toggleFollow}
          onToggleSound={() => void toggleSound()}
        />
      )}
      {showStateMatrix && (
        <ChatPanel
          chatDraft={chatDraft}
          chatMessages={chatMessages}
          chatSending={chatSending}
          currentUserID={currentUserID}
          systemMessages={systemMessages}
          onDraftChange={setChatDraft}
          onSend={sendChat}
        />
      )}
      {!showStateMatrix && (
        <ChatComposer
        chatDraft={chatDraft}
        chatSending={chatSending}
        onOpenDetails={() => setActiveSheet('details')}
        onOpenProducts={() => openWarmupSheet('products', 'watch')}
        onDraftChange={setChatDraft}
        onSend={sendChat}
        />
      )}
    </main>
  );
}

createRoot(document.getElementById('root')!).render(
  <AppProviders>
    <App />
  </AppProviders>
);
