/// <reference types="vite/client" />
import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { CheckCircle2, ChevronUp, Radio, RefreshCw } from 'lucide-react';
import type { AtmosphereCue, AtmosphereInput } from './atmosphere';
import { AuctionStatePanel, BottomSheet, ChatComposer, ChatPanel, LeaderboardPanel, LiveStage, ResultSheet, StateMatrixTabs } from './components';
import type { AuctionItem, AuctionOverlayMode, AuctionRealtimeEvent, AuctionState, AuctionSummary, AuctionSoundPack, AuthUser, BidderRequirement, BidPhase, BidResponse, BottomSheetKey, ChatMessage, ConnectionPhase, HistoryRow, LeaderboardPayload, LiveOpsCampaign, MaxBidIntent, MaxBidPhase, OrderRow, PaymentPhase, PendingBidRequest, ProductQAAnswer, ProductQATurn, RecoveryPhase, ResultSheetKind, Scenario, SnapshotResponse, SoundCapability, SystemMessage, WSTicketResponse } from './domain';
import { createAudioContext, createClientBidID, demoProductImageURL, demoUserID, deriveCountdown, deriveCountdownPhase, ensureDemoSession, extensionCopyFromEvent, formatCents, heatSnapshot, isBidConfirmationPending, isCountdownExpired, isDangerousActionDisabled, isEngineRejected, isTestMatrixEnabled, loadAuctionSoundPack, maxBidErrorCopy, maxBidStatusCopy, playAuctionSound, playCountdownTone, playCueTone, playLayeredCue, readJSON, rejectCopy, responseServerTimeMS, retryAfterMS, retryAfterMSFromHeaders, roomIDFromPath, scenarios, selectEntryAuction, speakSystemMessage, vibrateCountdownPhase, vibratePattern, visibleRoomAuctions } from './domain';
import { normalizeAtmosphere } from './atmosphere';
import { reconnectDelayMS } from './realtime';
import { h5Copy } from './copy';
import './styles.css';

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
  const [riskCode, setRiskCode] = useState('');
  const [leaderMasked, setLeaderMasked] = useState('张**');
  const [confirmToken, setConfirmToken] = useState('');
  const [confirmIdempotencyKey, setConfirmIdempotencyKey] = useState('');
  const [confirmAmountCents, setConfirmAmountCents] = useState(0);
  const [maxBidIntent, setMaxBidIntent] = useState<MaxBidIntent | null>(null);
  const [maxBidAmountCents, setMaxBidAmountCents] = useState(0);
  const [maxBidPhase, setMaxBidPhase] = useState<MaxBidPhase>('idle');
  const [maxBidFeedback, setMaxBidFeedback] = useState('仅自己可见，服务端按加价阶梯代出价');
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState('');
  const [bidHistory, setBidHistory] = useState<HistoryRow[]>([]);
  const [orderHistory, setOrderHistory] = useState<HistoryRow[]>([]);
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
  const [bidCooldownUntilMS, setBidCooldownUntilMS] = useState(0);
  const [extensionNotice, setExtensionNotice] = useState('');
  const [currentUserID, setCurrentUserID] = useState(demoUserID);
  const [sessionReady, setSessionReady] = useState(false);
  const [lotTitle, setLotTitle] = useState('青瓷手作茶盏');
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
  const lastSeqRef = useRef(lastSeq);
  const currentPriceRef = useRef(currentPriceCents);
  const leaderMaskedRef = useRef(leaderMasked);
  const activeAuctionIDRef = useRef(activeAuctionID);
  const activeIncrementCentsRef = useRef(activeIncrementCents);
  const maxBidAmountCentsRef = useRef(maxBidAmountCents);
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
  const atmosphereSeenRef = useRef<Set<string>>(new Set());
  const recoveringRef = useRef(false);
  const activeCueRef = useRef<AtmosphereCue | null>(null);
  const countdownCueRef = useRef('');
  const spokenSystemMessageRef = useRef(0);

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
    setQADraft('');
    setQAAnswer(undefined);
    setQAHistory([]);
    setQAThreadID(activeAuctionID ? `qa_${activeAuctionID}_${Date.now().toString(36)}` : '');
  }, [activeAuctionID]);

  useEffect(() => {
    activeIncrementCentsRef.current = activeIncrementCents;
  }, [activeIncrementCents]);

  useEffect(() => {
    maxBidAmountCentsRef.current = maxBidAmountCents;
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
  const heat = useMemo(() => heatSnapshot(leaderboard, activeAuction), [activeAuction, leaderboard]);
  const bidCooldownRemainingMS = Math.max(0, bidCooldownUntilMS - nowMS);
  const countdownExpired = useMemo(() => (
    selected === 'active_bids' &&
    connectionPhase === 'connected' &&
    recoveryPhase === 'idle' &&
    isCountdownExpired(auctionEndAt, serverTimeMS, nowMS, serverTimeSyncedAtRef.current)
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
    setCurrentPriceCents(delta.current_price_cents);
    setMinimumNextBidCents(delta.next_valid_bid_cents ?? delta.current_price_cents + activeIncrementCentsRef.current);
    setNextBidCents((prepared) => Math.max(delta.next_valid_bid_cents ?? delta.current_price_cents + activeIncrementCentsRef.current, prepared));
    if (delta.server_time_ms) syncServerTimeMS(delta.server_time_ms);
    if (leader?.user_masked) setLeaderMasked(leader.user_masked);
    if (!delta.burst_mode && soundEnabledRef.current && audioContextRef.current && soundCapabilityRef.current === 'ready') {
      playLayeredCue(audioContextRef.current, 'rank_change', soundPackRef.current);
    }
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
    const timer = window.setTimeout(() => setAtmosphereCue(null), 1800);
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
          leader: '你已拍中',
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
        feedback: payableOrderID ? '订单待支付' : '订单生成中',
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
        title: '落锤结算中',
        status: 'ENGINE_SOLD_PENDING',
        price: formatCents(currentPriceCents),
        leader: leaderMasked ? `${leaderMasked} 拍中` : '正在确认成交',
        feedback: bidFeedback || '等待订单结算确认',
        countdown: '订单同步中',
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
        feedback: bidFeedback || '出价已确认',
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
        cta: countdownExpired ? h5Copy.loading : `出价 ${formatCents(nextBidCents)}`,
        ctaDisabled: countdownExpired,
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
      feedback: countdownExpired ? '到点同步服务端结果' : bidFeedback,
      countdown: countdownCopy,
      cta: countdownExpired ? h5Copy.loading : `出价 ${formatCents(nextBidCents)}`,
      ctaDisabled: countdownExpired
    };
  }, [activeAuctionID, bidCooldownRemainingMS, bidFeedback, bidderRequirement, bidPhase, confirmAmountCents, connectionPhase, countdownCopy, countdownExpired, currentPriceCents, lastSeq, leaderMasked, minimumNextBidCents, nextBidCents, payableOrderAmountCents, payableOrderID, paymentPhase, recoveryPhase, selected, terminalPriceCents, terminalWinnerID]);
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
    setCurrentPriceCents(acceptedPrice);
    setMinimumNextBidCents(acceptedPrice + activeIncrementCents);
    setNextBidCents(acceptedPrice + activeIncrementCents);
    if (!isEnginePending && !isEngineSoldPending) {
      setLastSeq(payload.seq ?? lastSeq);
    }
    if (payload.end_at) setAuctionEndAt(payload.end_at);
    if (payload.server_time_ms) syncServerTimeMS(payload.server_time_ms);
    if (isEnginePending || isEngineSoldPending) {
      setBidFeedback(isEngineSoldPending
        ? '已到成交确认，等待订单生成'
        : '出价已提交，正在确认');
      setBidPhase(isEngineSoldPending ? 'engine_sold_pending' : 'engine_pending');
      showAtmosphere({
        kind: 'leading',
        title: isEngineSoldPending ? '成交确认中' : '出价已接收',
        detail: isEngineSoldPending ? '等待订单生成' : '等待最终确认',
        auction_id: payload.auction_id ?? activeAuctionIDRef.current,
        cause_seq: payload.engine_seq ?? payload.seq ?? lastSeqRef.current,
        event_type: payload.result ?? 'ENGINE_ACCEPTED',
        user_scope: 'self'
      });
      void loadLeaderboard(payload.auction_id ?? activeAuctionIDRef.current);
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
    if (nextServerTimeMS) syncServerTimeMS(nextServerTimeMS);
    setCurrentPriceCents(price);
    setMinimumNextBidCents(price + increment);
    setNextBidCents(price + increment);
    setMaxBidAmountCents((amount) => Math.max(amount || 0, price + increment));
    if (snapshot.max_bid_intent) {
      setMaxBidIntent(snapshot.max_bid_intent);
      setMaxBidAmountCents(Math.max(snapshot.max_bid_intent.max_amount_cents, price + increment));
      setMaxBidFeedback(maxBidStatusCopy(snapshot.max_bid_intent));
    } else {
      setMaxBidIntent(null);
      setMaxBidFeedback('仅自己可见，服务端按加价阶梯代出价');
    }
    setLastSeq(snapshot.seq);
    setLeaderMasked(snapshot.payload?.leader_user_masked ?? leaderMasked);
      setBidFeedback(h5Copy.latestConfirmed);
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
    } else if (status === 'SCHEDULED' || status === 'DRAFT') {
      setSelected('scheduled');
    } else if (status === 'ACTIVE') {
      setSelected('active_bids');
    }
    void loadLeaderboard(snapshotAuctionID ?? activeAuctionIDRef.current);
  };

  const recoverFromSnapshot = async () => {
    const auctionID = activeAuctionIDRef.current;
    if (!auctionID) return;
    if (recoveryInFlightRef.current) return;
    recoveryInFlightRef.current = true;
    setSelected('active_bids');
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
      setBidFeedback('暂未取到最新状态，正在继续同步');
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
    if (detail.seq == null || detail.seq <= currentSeq) return;
    const price = detail.payload?.current_price_cents ?? detail.payload?.amount_cents ?? currentPriceRef.current;
    const increment = activeIncrementCentsRef.current;
    const nextEndAt = detail.payload?.end_at ?? detail.end_at;
    const nextServerTimeMS = detail.payload?.server_time_ms ?? detail.server_time_ms;
    const previousEndAt = auctionEndAtRef.current;
    const previousWinnerID = leaderboardRef.current?.current_winner_id;
    const winnerID = detail.payload?.current_winner_id ?? detail.payload?.user_id ?? '';
    setCurrentPriceCents(price);
    setMinimumNextBidCents(price + increment);
    setNextBidCents((prepared) => Math.max(price + increment, prepared));
    setMaxBidAmountCents((amount) => Math.max(amount || 0, price + increment));
    setLastSeq(detail.seq);
    if (nextEndAt) setAuctionEndAt(nextEndAt);
    if (nextServerTimeMS) syncServerTimeMS(nextServerTimeMS);
    setLeaderMasked(detail.payload?.leader_user_masked ?? leaderMaskedRef.current);
    const wasExtended = Boolean(nextEndAt && previousEndAt && Date.parse(nextEndAt) > Date.parse(previousEndAt));
    if (wasExtended && nextEndAt) {
      setExtensionNotice(extensionCopyFromEvent(detail, previousEndAt, nextEndAt));
      setBidFeedback('最后时刻有出价，竞拍已延时');
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
      setBidFeedback('竞拍状态已更新');
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
    } else if (winnerID === currentUserIDRef.current || detail.payload?.current_winner_id === currentUserIDRef.current) {
      setBidPhase('accepted');
      setBidFeedback('你已领先，出价已确认');
      showAtmosphere({
        kind: 'leading',
        title: '领先！',
        detail: `${formatCents(price)} 已同步`,
        auction_id: detail.auction_id,
        cause_seq: detail.seq,
        event_type: detail.event_type,
        user_scope: 'self'
      });
    } else if (winnerID && winnerID !== currentUserIDRef.current) {
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
        syncServerTimeMS(selectedAuction.server_time_ms ?? 0);
        setBidFeedback('已进入当前拍品');
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
          setBidFeedback('已进入当前拍品');
        }
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
    const timer = window.setInterval(loadSystemMessages, 5_000);
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
        const ticketResponse = await fetch('/api/auth/ws-ticket', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify({ room_id: roomID, auction_id: activeAuctionID })
        });
        const ticketPayload = await readJSON<WSTicketResponse>(ticketResponse);
        if (!ticketResponse.ok || !ticketPayload?.ticket) {
          scheduleReconnect(retryAfterMSFromHeaders(ticketResponse));
          return;
        }
        if (cancelled) return;
        const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
        const wsURL = `${scheme}://${window.location.host}/ws?room_id=${encodeURIComponent(roomID)}&auction_id=${encodeURIComponent(activeAuctionID)}&last_seq=${lastSeqRef.current}`;
        const socket = new WebSocket(wsURL, ['auction.v1', `ticket.${ticketPayload.ticket}`]);
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
    if (!sessionReady || !activeAuctionID || selected !== 'active_bids') return undefined;
    if (connectionPhase === 'connected' && recoveryPhase === 'idle' && !countdownExpired) return undefined;
    const timer = window.setInterval(() => {
      void recoverFromSnapshot();
    }, 2_500);
    return () => window.clearInterval(timer);
  }, [activeAuctionID, connectionPhase, countdownExpired, recoveryPhase, sessionReady, selected]);

  const submitBid = async () => {
    const auctionID = activeAuctionIDRef.current;
    const canRetryPendingBid = Boolean(pendingBidRef.current && pendingBidRef.current.auctionID === auctionID && (bidPhase === 'uncertain' || bidPhase === 'engine_pending' || riskCode === 'PROCESSING_RETRY_LATER' || riskCode === 'BID_CONFIRMATION_PENDING'));
    if (selected !== 'active_bids' || (!canRetryPendingBid && scenario.ctaDisabled) || !auctionID) return;
    if (bidInFlightRef.current || bidCooldownUntilMS > Date.now()) return;
    bidInFlightRef.current = true;
    const pending = pendingBidRef.current;
    const bidRequest = pending && pending.auctionID === auctionID
      ? pending
      : {
          auctionID,
          clientBidID: createClientBidID(),
          amountCents: nextBidCents,
          clientSeenSeq: lastSeq
        };
    pendingBidRef.current = bidRequest;
    setBidPhase('pending');
    try {
      const response = await fetch(`/api/auctions/${auctionID}/bids`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': bidRequest.clientBidID
        },
        body: JSON.stringify({
          client_bid_id: bidRequest.clientBidID,
          amount_cents: bidRequest.amountCents,
          client_seen_seq: bidRequest.clientSeenSeq
        })
      });
      const payload = await response.json() as BidResponse;
      if (payload.result === 'FAT_FINGER_CONFIRM_REQUIRED' && payload.confirm_token) {
        setConfirmToken(payload.confirm_token);
        setConfirmIdempotencyKey(bidRequest.clientBidID);
        setConfirmAmountCents(payload.amount_cents ?? bidRequest.amountCents);
        pendingBidRef.current = null;
        setBidFeedback('高额出价需要二次确认');
        setBidPhase('confirm_required');
        return;
      }
      if (!response.ok || (payload.reject_reason && !isEngineRejected(payload)) || payload.code) {
        const code = payload.reject_reason ?? payload.code ?? '';
        const retryMS = retryAfterMS(response, payload);
        if (retryMS > 0) {
          setBidCooldownUntilMS(Date.now() + retryMS);
        }
        setRiskCode(code);
        setBidFeedback(rejectCopy(code));
        if (code !== 'PROCESSING_RETRY_LATER') {
          pendingBidRef.current = null;
        }
        setBidPhase('rejected');
        return;
      }
      setRiskCode('');
      setBidCooldownUntilMS(0);
      if (!isBidConfirmationPending(payload)) {
        pendingBidRef.current = null;
      }
      applyAcceptedBid(payload);
    } catch {
      setRiskCode('NETWORK_ERROR');
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
      if (response.status === 404) {
        setMaxBidIntent(null);
        setMaxBidFeedback('仅自己可见，服务端按加价阶梯代出价');
        return;
      }
      const payload = await readJSON<MaxBidIntent>(response);
      if (!response.ok || !payload) return;
      setMaxBidIntent(payload);
      setMaxBidAmountCents(Math.max(payload.max_amount_cents, minimumNextBidCents));
      setMaxBidFeedback(maxBidStatusCopy(payload));
    } catch {
      setMaxBidFeedback('自动加价状态读取失败');
    }
  };

  const submitMaxBidIntent = async () => {
    const auctionID = activeAuctionIDRef.current;
    if (!auctionID || (maxBidPhase !== 'idle' && maxBidPhase !== 'error') || isDangerousActionDisabled(scenario, connectionPhase)) return;
    const amount = Math.max(maxBidAmountCentsRef.current || 0, minimumNextBidCents);
    setMaxBidPhase('pending');
    setMaxBidFeedback('正在确认自动加价');
    try {
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
      const payload = await readJSON<{ intent?: MaxBidIntent; code?: string; message?: string }>(response);
      if (!response.ok || !payload?.intent) {
        setMaxBidPhase('error');
        setMaxBidFeedback(maxBidErrorCopy(payload?.code ?? payload?.message));
        return;
      }
      setMaxBidIntent(payload.intent);
      setMaxBidAmountCents(Math.max(payload.intent.max_amount_cents, minimumNextBidCents));
      setMaxBidFeedback(maxBidStatusCopy(payload.intent));
      setMaxBidPhase('idle');
    } catch {
      setMaxBidPhase('error');
      setMaxBidFeedback('网络异常，自动加价未确认');
    }
  };

  const cancelMaxBidIntent = async () => {
    const auctionID = activeAuctionIDRef.current;
    if (!auctionID || !maxBidIntent || maxBidPhase !== 'idle' || isDangerousActionDisabled(scenario, connectionPhase)) return;
    setMaxBidPhase('canceling');
    setMaxBidFeedback('正在取消自动加价');
    try {
      const key = createClientBidID();
      const response = await fetch(`/api/auctions/${auctionID}/max-bid-intent`, {
        method: 'DELETE',
        headers: { 'Idempotency-Key': key }
      });
      const payload = await readJSON<{ intent?: MaxBidIntent; code?: string; message?: string }>(response);
      if (!response.ok || !payload?.intent) {
        setMaxBidPhase('error');
        setMaxBidFeedback(maxBidErrorCopy(payload?.code ?? payload?.message));
        return;
      }
      setMaxBidIntent(payload.intent);
      setMaxBidFeedback(maxBidStatusCopy(payload.intent));
      setMaxBidPhase('idle');
    } catch {
      setMaxBidPhase('error');
      setMaxBidFeedback('网络异常，自动加价未取消');
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
        setLeaderboard((previous) => {
          const current = leaderboardRef.current ?? previous;
          if (payload.auction_id === current?.auction_id && payload.seq != null && current.seq != null && payload.seq < current.seq) {
            return previous;
          }
          leaderboardRef.current = payload;
          return payload;
        });
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

  const decreaseMaxBidAmount = () => {
    setMaxBidPhase((phase) => phase === 'error' ? 'idle' : phase);
    setMaxBidAmountCents((amount) => Math.max(minimumNextBidCents, amount - activeIncrementCents));
  };

  const increaseMaxBidAmount = () => {
    setMaxBidPhase((phase) => phase === 'error' ? 'idle' : phase);
    setMaxBidAmountCents((amount) => Math.max(minimumNextBidCents, amount + activeIncrementCents));
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
      if (next) void completeLiveOpsTask('follow');
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
        setLiveOpsError('阵营选择暂不可用，请稍后再试');
      }
    } catch {
      setLiveOpsError('阵营选择暂不可用，请稍后再试');
    }
  };

  const enterLuckyDraw = async () => {
    setLiveOpsError('');
    try {
      const response = await fetch(`/api/rooms/${roomID}/liveops/lucky-draw/enter`, { method: 'POST' });
      const payload = await readJSON<LiveOpsCampaign>(response);
      if (response.ok && payload) {
        setLiveOpsCampaign(payload);
        if (soundEnabledRef.current && audioContextRef.current && soundCapabilityRef.current === 'ready') {
          playLayeredCue(audioContextRef.current, 'entry_badge', soundPackRef.current);
        }
        vibratePattern('leading');
      } else {
        setLiveOpsError('请先完成暖场任务再参与福袋');
      }
    } catch {
      setLiveOpsError('福袋暂不可用，请稍后再试');
    }
  };

  const openLuckyDraw = async () => {
    setLiveOpsError('');
    try {
      const response = await fetch(`/api/rooms/${roomID}/liveops/lucky-draw/open`, { method: 'POST' });
      const payload = await readJSON<LiveOpsCampaign>(response);
      if (response.ok && payload) {
        setLiveOpsCampaign(payload);
        if (soundEnabledRef.current && audioContextRef.current && soundCapabilityRef.current === 'ready') {
          playLayeredCue(audioContextRef.current, 'lucky_open', soundPackRef.current);
        }
        vibratePattern('sold');
      } else {
        setLiveOpsError('请先参与福袋再开奖');
      }
    } catch {
      setLiveOpsError('开奖暂不可用，请稍后再试');
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
        lotTitle={lotTitle}
        roomID={roomID}
        scenario={scenario}
        soundEnabled={soundEnabled}
        soundCapability={soundCapability}
        systemMessages={systemMessages}
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
          terminalPriceCents={terminalPriceCents || currentPriceCents}
          terminalWinnerID={terminalWinnerID}
          onClose={() => setOverlayMode('feed')}
          onDecreaseBid={decreaseBidAmount}
          onIncreaseBid={increaseBidAmount}
          onOpenOrders={() => setActiveSheet('orders')}
          onOpenSheet={setActiveSheet}
          onPay={payOrder}
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
          terminalPriceCents={terminalPriceCents || currentPriceCents}
          terminalWinnerID={terminalWinnerID}
          userBestCents={leaderboard?.my_best_amount_cents ?? 0}
          onOpenOrders={() => setActiveSheet(resultSheetKind === 'winner' ? 'orders' : 'history')}
          onPay={payOrder}
        />
      ) : null}
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
          maxBidAmountCents={maxBidAmountCents}
          maxBidFeedback={maxBidFeedback}
          maxBidIntent={maxBidIntent}
          maxBidPhase={maxBidPhase}
          minimumNextBidCents={minimumNextBidCents}
          nextBidCents={nextBidCents}
          orderHistory={orderHistory}
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
          onClose={() => setActiveSheet(null)}
          onCancelMaxBid={cancelMaxBidIntent}
          onDecreaseMaxBid={decreaseMaxBidAmount}
          onIncreaseMaxBid={increaseMaxBidAmount}
          onOpenSheet={setActiveSheet}
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
        onDraftChange={setChatDraft}
        onSend={sendChat}
        />
      )}
    </main>
  );
}

createRoot(document.getElementById('root')!).render(<App />);
