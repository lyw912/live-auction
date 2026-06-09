import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, BadgeCheck, Bell, BellOff, CheckCircle2, ChevronUp, Clock3, CreditCard, Flame, History, Info, MessageCircle, MoreHorizontal, PackageCheck, RefreshCw, Send, ShieldCheck, ShoppingCart, Sparkles, Truck, Trophy, Users, Wifi, WifiOff, X } from 'lucide-react';
import CertificateIcon from '@icon-park/react/es/icons/Certificate';
import CommentIcon from '@icon-park/react/es/icons/CommentOne';
import JewelryIcon from '@icon-park/react/es/icons/Jewelry';
import LikeIcon from '@icon-park/react/es/icons/Like';
import MoreIcon from '@icon-park/react/es/icons/More';
import ShoppingBagIcon from '@icon-park/react/es/icons/ShoppingBagOne';
import SoundIcon from '@icon-park/react/es/icons/SoundOne';
import TruckIcon from '@icon-park/react/es/icons/Truck';
import type { AtmosphereCue, AtmosphereIntensity } from './atmosphere';
import type { AuctionItem, AuctionState, AuctionSummary, BottomSheetKey, ChatMessage, ConnectionPhase, CountdownPhase, CountdownPhaseState, HeatSnapshot, HistoryRow, LeaderboardPayload, LiveOpsCampaign, MaxBidIntent, MaxBidPhase, PaymentPhase, ProductQAAnswer, ResultSheetKind, Scenario, SoundCapability, SystemMessage } from './domain';
import { auctionStatusLabel, connectionSyncCopy, demoLiveVideoURL, demoProductImageURL, displayMediaURL, formatCents, formatClockTime, isDangerousActionDisabled, leaderboardActionCopy, rankBadgeLabel, riskActionCopy, scenarios } from './domain';
import { h5Copy } from './copy';
import { ResultSheet } from './result';

export type WaterfallChip = {
  id: string;
  seq: number;
  amount_cents: number;
  user_masked: string;
  is_current: boolean;
  created_at: number;
};

function displayChatUser(userID: string, currentUserID: string) {
  if (userID === currentUserID) return '我';
  if (/^[\u4e00-\u9fa5].*\*\*$/.test(userID)) return userID;
  return '匿名买家';
}

const iconParkProofFill = ['#D4AF37', '#2c2c2c', '#EFBF04', '#F7E6CA'];
const iconParkActionFill = ['#fff', '#fff', '#fff', '#fff'];

export function LiveStage({
  activeAuctionID,
  atmosphereCue,
  auctions,
  chatMessages,
  connectionPhase,
  countdownPhase,
  countdownCopy,
  currentPriceCents,
  currentUserID,
  followed,
  heat,
  item,
  likeCount,
  leaderboard,
  lotTitle,
  nextBidCents,
  roomID,
  scenario,
  soundEnabled,
  soundCapability,
  systemMessages,
  terminalPriceCents,
  waterfallChips,
  raceBoardExpanded,
  atmosphereIntensity,
  atmosphereGated,
  onOpenBid,
  onOpenLiveOps,
  onOpenMore,
  onOpenProducts,
  onToggleFollow,
  onLike,
  onToggleSound
}: {
  activeAuctionID: string;
  atmosphereCue: AtmosphereCue | null;
  auctions: AuctionSummary[];
  chatMessages: ChatMessage[];
  connectionPhase: ConnectionPhase;
  countdownPhase: CountdownPhaseState;
  countdownCopy: string;
  currentPriceCents: number;
  currentUserID: string;
  followed: boolean;
  heat: HeatSnapshot;
  item: AuctionItem;
  likeCount: number;
  leaderboard: LeaderboardPayload | null;
  lotTitle: string;
  nextBidCents: number;
  roomID: string;
  scenario: Scenario;
  soundEnabled: boolean;
  soundCapability: SoundCapability;
  systemMessages: SystemMessage[];
  terminalPriceCents: number;
  waterfallChips: WaterfallChip[];
  raceBoardExpanded: boolean;
  atmosphereIntensity: AtmosphereIntensity;
  atmosphereGated: boolean;
  onOpenBid: () => void;
  onOpenLiveOps: () => void;
  onOpenMore: () => void;
  onOpenProducts: () => void;
  onToggleFollow: () => void;
  onLike: () => void;
  onToggleSound: () => void;
}) {
  const mediaURL = displayMediaURL(item.video_poster_url ?? item.videoPosterURL ?? item.image_url ?? item.imageURL);
  const videoURL = demoLiveVideoURL;
  const activeAuction = auctions.find((auction) => auction.id === activeAuctionID);
  const queuedCount = auctions.filter((auction) => auction.id !== activeAuctionID).length;
  const proofChips: Array<{ icon: React.ReactNode; label: string }> = [];
  if (item.certificate) proofChips.push({ icon: <CertificateIcon size={13} theme="multi-color" fill={iconParkProofFill} />, label: item.certificate });
  if (item.condition) proofChips.push({ icon: <JewelryIcon size={13} theme="multi-color" fill={iconParkProofFill} />, label: item.condition });
  if (item.shipping) proofChips.push({ icon: <TruckIcon size={13} theme="multi-color" fill={['#00A870', '#2c2c2c', '#F7E6CA', '#D4AF37']} />, label: item.shipping });
  const barrageMessages = systemMessages;
  const visibleSystem = barrageMessages.slice(0, 2).map((message) => ({
    id: `sys-${message.id}`,
    user: message.source === 'SYSTEM_AI' || message.source === 'HOST_SCRIPT' ? '主播提示' : '系统提示',
    body: message.body
  }));
  const visibleChat = [
    ...visibleSystem,
    ...chatMessages.slice(-3).map((message) => ({
      id: `chat-${message.id}`,
      user: displayChatUser(message.user_id, currentUserID),
      body: message.body
    }))
  ].slice(-4);
  const connectionCopy = connectionPhase === 'connected'
    ? '直播已连接'
    : connectionPhase === 'recovering'
      ? '直播重连中'
      : connectionPhase === 'connecting'
        ? '直播连接中'
        : '直播已断开';
  const onlineBuyerCopy = heat.watcherCountAvailable && heat.watcherCount != null
    ? `在线买家 ${heat.watcherCount}`
    : '在线买家';
  const roomCopy = roomID === 'room_main'
    ? '竞拍专场'
    : roomID.replace(/^room[_-]?/i, '专场 ');
  const stageStatusCopy = scenario.stale
    ? h5Copy.loading
    : scenario.status === 'ACTIVE'
      ? heat.source === 'leaderboard' ? `${heat.acceptedBidderCount} 人已出价` : h5Copy.noBidShort
      : auctionStatusLabel(scenario.status);
  const floatingActionCopy = scenario.sold
    ? '查看结果'
    : scenario.status === 'ACTIVE' && !scenario.ctaDisabled
      ? `出一手 ${formatCents(nextBidCents)}`
      : '看拍品信息';

  return (
    <section
      className={`video-stage ${mediaURL ? 'has-media' : 'no-media'}`}
      aria-label="live-stage"
      data-testid="live-stage"
      data-atmosphere-kind={atmosphereCue?.kind ?? 'none'}
      data-countdown-phase={countdownPhase.phase}
      data-race-board={raceBoardExpanded ? 'expanded' : 'rest'}
      data-atmosphere-intensity={atmosphereIntensity}
      data-atmosphere-gated={atmosphereGated ? 'true' : 'false'}
      style={mediaURL ? { '--stage-media-url': `url("${mediaURL}")` } as React.CSSProperties : undefined}
    >
      <video className="live-video-bg" src={videoURL} poster={mediaURL || demoProductImageURL} autoPlay muted loop playsInline aria-hidden="true" />
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
      <BidWaterfall chips={atmosphereGated ? [] : waterfallChips} intensity={atmosphereIntensity} />
      <ClimaxLayer
        atmosphereCue={atmosphereCue}
        heat={heat}
        scenario={scenario}
        terminalPriceCents={terminalPriceCents || currentPriceCents}
        motionEnabled={!atmosphereGated}
      />
      <BarrageLayer messages={barrageMessages} />
      <div className="video-topbar">
        <div className="host-profile">
          <span className="host-avatar">{roomID.slice(0, 1).toUpperCase()}</span>
          <span><strong>{roomCopy}</strong><em>正在直播</em></span>
          <button type="button" className={followed ? 'is-followed' : ''} onClick={onToggleFollow}>{followed ? '已关注' : '关注'}</button>
        </div>
        <span className="viewer-count avatar-stack" title="当前直播间在线买家数"><Users size={13} /> {onlineBuyerCopy}</span>
        {followed ? <span className="viewer-count" title="已关注本直播间"><BadgeCheck size={13} /> 已关注</span> : null}
        <span className="viewer-count" title="直播间实时连接状态"><Wifi size={13} /> {connectionCopy}</span>
        <button
          className="sound-toggle"
          type="button"
          aria-label={soundEnabled ? '关闭提示音' : soundCapability === 'ready' ? '开启提示音' : '提示音不可用'}
          title={soundCapability === 'blocked' ? '浏览器阻止音频，请再次点击授权' : soundCapability === 'unavailable' ? '当前浏览器不支持提示音' : undefined}
          disabled={soundCapability === 'unavailable'}
          onClick={onToggleSound}
        >
          {soundEnabled ? <SoundIcon size={14} theme="filled" fill="#fff" /> : <BellOff size={14} />}
        </button>
      </div>
      <RaceBoard
        leaderboard={leaderboard}
        nextBidCents={nextBidCents}
        forceExpanded={raceBoardExpanded}
        intensity={atmosphereIntensity}
        atmosphereCue={atmosphereCue}
        onOpenBid={onOpenBid}
      />
      <PressureActionCard
        atmosphereCue={atmosphereCue}
        countdownPhase={countdownPhase}
        leaderboard={leaderboard}
        nextBidCents={nextBidCents}
        scenario={scenario}
        onOpenBid={onOpenBid}
      />
      <FinalSecondsLayer countdownPhase={countdownPhase} atmosphereCue={atmosphereCue} scenario={scenario} />
      <div className="stage-safe-zone">
        <div className="live-topic-row" aria-label="live-topic">
          <span>{auctionStatusLabel(scenario.status)}</span>
          <span>{stageStatusCopy}</span>
        </div>
        {proofChips.length > 0 && <div className="proof-chip-row" aria-label="product-proof">
          {proofChips.map((chip) => (
            <span className="proof-chip" key={chip.label}>{chip.icon}{chip.label}</span>
          ))}
        </div>}
        <div className="stage-chat-overlay" data-testid="stage-chat-overlay" aria-label="live-chat-overlay">
          {visibleChat.length === 0 ? (
            <span className="stage-chat-empty">等待实时弹幕</span>
          ) : visibleChat.map((message) => (
            <span className="stage-chat-line" key={message.id}>
              <strong>{message.user}</strong>
              {message.body}
            </span>
          ))}
        </div>
        <div className="focus-copy">
          <h1>{lotTitle}</h1>
          <p>确认价格和时间后再出价 · {scenario.countdown ?? countdownCopy}</p>
        </div>
      </div>
      <HeatMeter heat={heat} countdownPhase={countdownPhase.phase} />
      <button className="floating-product-card" type="button" onClick={onOpenBid} data-testid="floating-product-card" aria-label="进入竞拍面板">
        <span className={`floating-thumb ${mediaURL ? 'has-media' : ''}`} style={mediaURL ? { '--floating-media-url': `url("${mediaURL}")` } as React.CSSProperties : undefined}>
          {!mediaURL && <ShoppingBagIcon size={18} theme="multi-color" fill={['#FE2C55', '#2c2c2c', '#F7E6CA', '#D4AF37']} />}
        </span>
        <span className="floating-product-copy">
          <strong>{activeAuction?.item?.title ?? lotTitle}</strong>
          <span className="floating-auction-meta">
            <em data-testid="floating-auction-price">{scenario.status === 'ACTIVE' ? `当前最高价 ${formatCents(currentPriceCents)}` : `${auctionStatusLabel(scenario.status)} · ${scenario.price}`}</em>
            <small data-testid="floating-auction-countdown"><Clock3 size={12} />{scenario.countdown ?? countdownCopy}</small>
            <small data-testid="floating-auction-status">{auctionStatusLabel(scenario.status)} · {connectionCopy}</small>
          </span>
        </span>
        <span className="floating-bid-strip" data-testid="h5-sticky-bid-strip">
          <span>
            <small>{scenario.status === 'ACTIVE' ? '当前' : auctionStatusLabel(scenario.status)}</small>
            <strong>{scenario.status === 'ACTIVE' ? formatCents(currentPriceCents) : scenario.price}</strong>
          </span>
          <em aria-hidden="true">→</em>
          <span className="floating-product-action">{floatingActionCopy}</span>
        </span>
      </button>
      <div className="live-action-rail" aria-label="live-actions">
        <button type="button" onClick={onOpenProducts} aria-label="商品列表"><ShoppingBagIcon className="action-rail-icon" size={26} theme="outline" fill={iconParkActionFill} strokeWidth={4} /><span className="live-action-badge">{queuedCount + 1}</span></button>
        <button type="button" onClick={onOpenLiveOps} aria-label="直播互动"><CommentIcon className="action-rail-icon" size={26} theme="outline" fill={iconParkActionFill} strokeWidth={4} /></button>
        <button type="button" onClick={onLike} aria-label="点赞"><LikeIcon className="action-rail-icon" size={26} theme="outline" fill={iconParkActionFill} strokeWidth={4} /><span className="live-action-badge">{likeCount}</span></button>
        <button type="button" onClick={onOpenMore} aria-label="我的"><MoreIcon className="action-rail-icon" size={26} theme="outline" fill={iconParkActionFill} strokeWidth={4} /></button>
      </div>
    </section>
  );
}

function ClimaxLayer({
  atmosphereCue,
  heat,
  motionEnabled,
  scenario,
  terminalPriceCents
}: {
  atmosphereCue: AtmosphereCue | null;
  heat: HeatSnapshot;
  motionEnabled: boolean;
  scenario: Scenario;
  terminalPriceCents: number;
}) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const [confettiEngine, setConfettiEngine] = useState<'idle' | 'loading' | 'canvas-confetti' | 'failed'>('idle');
  const active = scenario.sold && atmosphereCue?.kind === 'sold';
  const isWinner = active && atmosphereCue?.user_scope === 'self';
  const bidderCopy = heat.acceptedBidderCount > 0 ? `${heat.acceptedBidderCount} 人有效出价` : '真实竞拍记录已锁定';
  const totalBidCopy = heat.totalAcceptedBids != null && heat.totalAcceptedBids > 0
    ? `${heat.totalAcceptedBids} 次真实出价`
    : heat.acceptedBids30s > 0
      ? `近30秒 ${heat.acceptedBids30s} 次有效出价`
      : '成交事实以服务端为准';

  useEffect(() => {
    if (!active || !motionEnabled) return;
    const canvas = canvasRef.current;
    if (!canvas) return;
    const reduceMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
    if (reduceMotion) return;
    let cancelled = false;
    setConfettiEngine('loading');
    void import('canvas-confetti')
      .then(({ default: confetti }) => {
        if (cancelled) return;
        const fire = confetti.create(canvas, {
          resize: true,
          useWorker: true,
          disableForReducedMotion: true
        });
        setConfettiEngine('canvas-confetti');
        void fire({
          particleCount: 72,
          spread: 64,
          startVelocity: 42,
          gravity: 1.05,
          ticks: 180,
          origin: { x: 0.5, y: 0.28 },
          colors: ['#ffd166', '#ff5a6a', '#2a9d8f', '#5b8cff', '#ff8a3d'],
          shapes: ['square'],
          scalar: 0.92
        });
        window.setTimeout(() => {
          if (cancelled) return;
          void fire({
            particleCount: 46,
            angle: 72,
            spread: 52,
            startVelocity: 34,
            gravity: 1.1,
            ticks: 160,
            origin: { x: 0.16, y: 0.4 },
            colors: ['#ffd166', '#ff8a3d', '#2a9d8f'],
            shapes: ['square'],
            scalar: 0.82
          });
          void fire({
            particleCount: 46,
            angle: 108,
            spread: 52,
            startVelocity: 34,
            gravity: 1.1,
            ticks: 160,
            origin: { x: 0.84, y: 0.4 },
            colors: ['#ffd166', '#ff5a6a', '#5b8cff'],
            shapes: ['square'],
            scalar: 0.82
          });
        }, 170);
      })
      .catch(() => {
        if (!cancelled) setConfettiEngine('failed');
      });
    return () => {
      cancelled = true;
    };
  }, [active, motionEnabled, atmosphereCue?.id]);

  if (!active) return null;
  return (
    <section
      className={`climax-layer ${isWinner ? 'is-winner' : 'is-loser'} ${motionEnabled ? 'motion-on' : 'motion-off'}`}
      data-testid="climax-layer"
      data-motion={motionEnabled ? 'on' : 'off'}
      aria-live="assertive"
      aria-label="落槌结果"
    >
      {motionEnabled ? (
        <canvas
          ref={canvasRef}
          className="climax-confetti-canvas"
          data-testid="climax-confetti-canvas"
          data-engine={confettiEngine}
          data-worker="true"
          aria-hidden="true"
        />
      ) : null}
      <div className="climax-spotlight" aria-hidden="true" />
      <div className="climax-card" data-testid="climax-stage-card">
        <span>{isWinner ? '成交凭证' : '本场落槌'}</span>
        <strong>{isWinner ? '中拍！' : '已成交'}</strong>
        <em>{formatCents(terminalPriceCents)}</em>
        <p>{bidderCopy} · {totalBidCopy}</p>
        <small>{isWinner ? '先确认成交事实，再进入支付' : '成交事实已锁定，查看记录继续下一件'}</small>
      </div>
    </section>
  );
}

function FinalSecondsLayer({
  atmosphereCue,
  countdownPhase,
  scenario
}: {
  atmosphereCue: AtmosphereCue | null;
  countdownPhase: CountdownPhaseState;
  scenario: Scenario;
}) {
  if (scenario.stale || scenario.sold) return null;
  const isExtended = atmosphereCue?.kind === 'extended';
  const isFinal = countdownPhase.phase === 'critical' || countdownPhase.phase === 'hammer';
  if (!isExtended && !isFinal) return null;
  const seconds = countdownPhase.remainingMS != null && countdownPhase.remainingMS > 0
    ? Math.max(1, Math.ceil(countdownPhase.remainingMS / 1000))
    : null;
  const extendedCopy = atmosphereCue?.detail.match(/延时\s*\+\d+s/)?.[0] ?? '延时';
  const title = isExtended
    ? extendedCopy
    : countdownPhase.phase === 'hammer'
      ? countdownPhase.beat || '落槌窗口'
      : '最后 5 秒';
  const detail = isExtended
    ? '最后窗口有真实出价，竞拍继续'
    : countdownPhase.phase === 'hammer'
      ? countdownPhase.beat === '最后一次' ? '落槌前最后确认' : '有效出价仍会延时'
      : 'going once · 盯紧下一口';
  return (
    <div
      className={`final-seconds-layer ${isExtended ? 'is-extended' : countdownPhase.phase}`}
      data-testid="final-seconds-layer"
      aria-live="assertive"
    >
      <span>{title}</span>
      <strong>{seconds != null && !isExtended ? `${seconds}s` : detail}</strong>
      {!isExtended && <em>{detail}</em>}
    </div>
  );
}

function RaceBoard({
  leaderboard,
  nextBidCents,
  forceExpanded,
  intensity,
  atmosphereCue,
  onOpenBid
}: {
  leaderboard: LeaderboardPayload | null;
  nextBidCents: number;
  forceExpanded: boolean;
  intensity: AtmosphereIntensity;
  atmosphereCue: AtmosphereCue | null;
  onOpenBid: () => void;
}) {
  const entries = leaderboard?.entries ?? [];
  const top = entries.slice(0, 3);
  const mine = entries.find((entry) => entry.is_current);
  const leader = top[0];
  const myRank = mine?.rank ?? leaderboard?.my_rank;
  const gap = leaderboard?.gap_to_leader_cents ?? (mine && leader ? Math.max(0, leader.amount_cents - mine.amount_cents) : undefined);
  const recovering = leaderboard?.state === 'RECOVERING';
  const hasBids = entries.length > 0 || (leaderboard?.accepted_bidder_count ?? 0) > 0;
  const expanded = Boolean(forceExpanded || leaderboard?.burst_mode || (leaderboard?.accepted_bids_30s ?? 0) >= 4 || leaderboard?.state === 'OUTBID');
  const lastCueKind = atmosphereCue?.user_scope === 'self' ? atmosphereCue.kind : 'none';
  const bidderLabel = (entry: NonNullable<LeaderboardPayload['entries']>[number]) => entry.is_current ? `我（${entry.user_masked}）` : entry.user_masked;
  const headline = recovering
    ? '竞拍状态校对中'
    : hasBids && leader
    ? `榜一 ${bidderLabel(leader)} ${formatCents(leader.amount_cents)}`
    : '等你第一手登顶';
  const mineCopy = myRank
    ? myRank === 1
      ? '我 #1 正在领先'
      : `我 #${myRank}${gap != null ? ` 差 ${formatCents(gap)}` : ''}`
    : recovering
      ? '等待服务端确认'
      : `下一口 ${formatCents(nextBidCents)}`;

  return (
    <section
      className={`race-board ${expanded ? 'is-expanded' : 'is-rest'}`}
      data-testid="race-board"
      data-intensity={intensity}
      data-race-state={leaderboard?.state ?? 'NOT_BID'}
      data-cue-kind={lastCueKind}
      aria-label="常驻竞速榜"
      aria-live="polite"
    >
      <button className="race-board-rest" type="button" onClick={onOpenBid}>
        <Trophy size={15} />
        <strong>{headline}</strong>
        <span>{mineCopy}</span>
        <em>{(leaderboard?.accepted_bids_30s ?? 0) > 0 ? `近30秒 ${leaderboard?.accepted_bids_30s} 次` : '真实有效出价'}</em>
      </button>
      <div className="race-board-expanded">
        <div className="race-board-title">
          <strong>竞速榜</strong>
          <span>{(leaderboard?.accepted_bids_30s ?? 0) > 0 ? `近30s ${leaderboard?.accepted_bids_30s} 次` : '最高有效价优先'}</span>
        </div>
        {recovering ? (
          <p>竞拍状态校对中，以服务端修复后的榜单为准</p>
        ) : hasBids ? (
          <LeaderboardRows entries={top.length > 0 ? top : entries} burstMode={Boolean(leaderboard?.burst_mode)} highlightKind={lastCueKind} />
        ) : (
          <p>等你第一手登顶</p>
        )}
        {mine && !top.some((entry) => entry.user_id === mine.user_id) ? (
          <div className="leaderboard-row is-current race-board-mine">
            <span>#{mine.rank}</span>
            <strong>我</strong>
            <em>{formatCents(mine.amount_cents)}</em>
            <small>{gap != null && gap > 0 ? `差 ${formatCents(gap)}` : '正在领先'}</small>
          </div>
        ) : null}
      </div>
    </section>
  );
}

function PressureActionCard({
  atmosphereCue,
  countdownPhase,
  leaderboard,
  nextBidCents,
  scenario,
  onOpenBid
}: {
  atmosphereCue: AtmosphereCue | null;
  countdownPhase: CountdownPhaseState;
  leaderboard: LeaderboardPayload | null;
  nextBidCents: number;
  scenario: Scenario;
  onOpenBid: () => void;
}) {
  if (scenario.stale || scenario.sold || scenario.status !== 'ACTIVE') return null;
  const state = leaderboard?.state ?? 'NOT_BID';
  const isSelfCue = atmosphereCue?.user_scope === 'self';
  const isOutbid = state === 'OUTBID' || (isSelfCue && atmosphereCue?.kind === 'outbid');
  const isLeading = state === 'LEADING' || (isSelfCue && atmosphereCue?.kind === 'leading');
  const isFinal = countdownPhase.phase === 'critical' || countdownPhase.phase === 'hammer';
  if (!isOutbid && !isLeading && !isFinal) return null;

  const leader = leaderboard?.entries?.[0];
  const gap = leaderboard?.gap_to_leader_cents;
  const mode = isOutbid ? 'outbid' : isFinal ? 'final' : 'leading';
  const title = mode === 'outbid' ? '被超越' : mode === 'final' ? '最后窗口' : '领先中';
  const detail = mode === 'outbid'
    ? `${leader?.user_masked ?? '对手'} 当前领先${gap != null && gap > 0 ? `，差 ${formatCents(gap)}` : ''}`
    : mode === 'final'
      ? '有效出价会刷新倒计时，别把最后一口让出去'
      : `当前最高 ${formatCents(leaderboard?.leader_amount_cents ?? leader?.amount_cents ?? nextBidCents)}`;
  const action = mode === 'outbid'
    ? `立即反超 ${formatCents(nextBidCents)}`
    : mode === 'final'
      ? `抢最后一口 ${formatCents(nextBidCents)}`
      : '查看出价榜';

  return (
    <div
      className={`pressure-action-card is-${mode}`}
      data-testid="pressure-action-card"
      data-pressure-state={mode}
      role={mode === 'outbid' ? 'alert' : 'status'}
      aria-live={mode === 'outbid' ? 'assertive' : 'polite'}
    >
      <div>
        <span>{title}</span>
        <strong>{detail}</strong>
      </div>
      <button type="button" onClick={onOpenBid}>{action}</button>
    </div>
  );
}

function BidWaterfall({ chips, intensity }: { chips: WaterfallChip[]; intensity: AtmosphereIntensity }) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const reduceMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
    if (reduceMotion) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    let frame = 0;
    const draw = () => {
      const rect = canvas.getBoundingClientRect();
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      const width = Math.max(1, Math.round(rect.width * dpr));
      const height = Math.max(1, Math.round(rect.height * dpr));
      if (canvas.width !== width || canvas.height !== height) {
        canvas.width = width;
        canvas.height = height;
      }
      ctx.clearRect(0, 0, width, height);
      const now = Date.now();
      const activeLimit = intensity >= 3 ? 24 : intensity === 2 ? 16 : 8;
      const active = chips.slice(-activeLimit);
      active.forEach((chip, index) => {
        const age = now - chip.created_at;
        const life = 2600;
        if (age < 0 || age > life) return;
        const progress = age / life;
        const lane = index % 3;
        const chipWidth = 104 * dpr;
        const chipHeight = 28 * dpr;
        const x = width - chipWidth - (8 + lane * 18) * dpr;
        const y = (34 + progress * (rect.height - 86) + lane * 12) * dpr;
        const alpha = progress < .15 ? progress / .15 : Math.max(0, 1 - (progress - .72) / .28);
        ctx.save();
        ctx.globalAlpha = alpha;
        ctx.translate(x, y);
        ctx.fillStyle = chip.is_current ? 'rgba(255, 210, 96, .94)' : 'rgba(18, 28, 35, .76)';
        ctx.strokeStyle = chip.is_current ? 'rgba(255, 255, 255, .72)' : 'rgba(255, 255, 255, .24)';
        ctx.lineWidth = 1 * dpr;
        const radius = 10 * dpr;
        ctx.beginPath();
        ctx.roundRect(0, 0, chipWidth, chipHeight, radius);
        ctx.fill();
        ctx.stroke();
        ctx.fillStyle = chip.is_current ? '#16120a' : '#fff';
        ctx.font = `${11 * dpr}px sans-serif`;
        ctx.fillText(chip.is_current ? '我' : chip.user_masked, 10 * dpr, 18 * dpr);
        ctx.font = `bold ${12 * dpr}px sans-serif`;
        ctx.fillText(formatCents(chip.amount_cents), 42 * dpr, 18 * dpr);
        ctx.restore();
      });
      frame = window.requestAnimationFrame(draw);
    };
    draw();
    return () => window.cancelAnimationFrame(frame);
  }, [chips]);

  return <canvas className="bid-waterfall" data-testid="bid-waterfall" ref={canvasRef} aria-hidden="true" />;
}

function systemMessageLabel(message: SystemMessage) {
  return message.source === 'SYSTEM_AI' || message.source === 'HOST_SCRIPT' ? '主播提示' : '系统提示';
}

function BarrageLayer({ messages }: { messages: SystemMessage[] }) {
  const items = messages.slice(0, 4);
  if (items.length === 0) return null;
  return (
    <div className="system-barrage-layer" aria-label="系统飘屏" aria-live="polite">
      {items.map((message, index) => (
        <span
          className={`system-barrage system-barrage-${message.style || 'steady'}`}
          key={message.id}
          style={{ '--barrage-lane': index, '--barrage-delay': `${index * 260}ms` } as React.CSSProperties}
        >
          <strong>{systemMessageLabel(message)}</strong>
          {message.body}
        </span>
      ))}
    </div>
  );
}

function buyerHistoryStatus(row: HistoryRow) {
  const status = String(row.result ?? row.status ?? '');
  if (status.includes('ACCEPT') || status.includes('LEADING')) return '已出价成功';
  if (status.includes('REJECT') || status.includes('LOW')) return '未达到有效出价';
  if (status.includes('PENDING')) return '确认中';
  if (status.includes('SOLD')) return '已成交';
  return '已记录';
}

function buyerOrderStatus(status: string) {
  switch (status) {
    case 'ORDER_PENDING':
    case 'PAYMENT_INITIATED':
      return '待支付';
    case 'PAID':
      return '已支付';
    case 'ORDER_EXPIRED':
    case 'EXPIRED':
      return '已超时';
    case 'FAILED':
      return '处理失败';
    default:
      return '等待确认';
  }
}

function historyTime(row: HistoryRow) {
  const raw = String(row.created_at ?? row.updated_at ?? row.paid_at ?? '');
  if (!raw) return '';
  const time = new Date(raw);
  if (Number.isNaN(time.getTime())) return '';
  return time.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
}

function buyerHistorySecondary(row: HistoryRow) {
  const time = historyTime(row);
  const status = buyerHistoryStatus(row);
  return time ? `${status} · ${time}` : status;
}

function buyerOrderSecondary(row: HistoryRow) {
  const time = historyTime(row);
  const status = buyerOrderStatus(String(row.order_status ?? row.status ?? ''));
  return time ? `${status} · ${time}` : status;
}

function displayOrderNo(orderID: string) {
  if (!orderID) return h5Copy.loading;
  const compact = orderID.replace(/^ord[_-]?/i, '').replace(/[^a-z0-9]/gi, '').slice(-8).toUpperCase();
  return `JP${new Date().toISOString().slice(0, 10).replace(/-/g, '')}-${compact || '00000000'}`;
}

function orderIDFromRow(row: HistoryRow) {
  return String(row.order_id ?? row.id ?? '');
}

function orderAmount(row?: HistoryRow) {
  return Number(row?.amount_cents ?? 0);
}

function orderStatus(row?: HistoryRow) {
  return buyerOrderStatus(String(row?.order_status ?? row?.status ?? ''));
}

function PaymentConfirmDialog({
  amountCents,
  orderID,
  onCancel,
  onConfirm
}: {
  amountCents: number;
  orderID: string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className="payment-confirm-backdrop" data-testid="payment-confirm-dialog" role="dialog" aria-modal="true" aria-label="确认支付">
      <section className="payment-confirm-card">
        <div className="payment-confirm-head">
          <CreditCard size={20} />
          <div>
            <span>确认支付</span>
            <strong>{formatCents(amountCents)}</strong>
          </div>
        </div>
        <div className="payment-confirm-grid">
          <div><span>订单号</span><strong>{displayOrderNo(orderID)}</strong></div>
          <div><span>支付方式</span><strong>演示支付</strong></div>
          <div><span>支付结果</span><strong>确认后写入服务端订单状态</strong></div>
        </div>
        <p>这里不接真实资金通道，但会调用订单支付接口，更新订单为已支付并回到订单详情。</p>
        <div className="payment-confirm-actions">
          <button type="button" onClick={onCancel}>稍后支付</button>
          <button type="button" onClick={onConfirm}>确认支付 {formatCents(amountCents)}</button>
        </div>
      </section>
    </div>
  );
}

export function ChatComposer({
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
        <input aria-label="chat-input" value={chatDraft} onChange={(event) => onDraftChange(event.currentTarget.value)} placeholder="说点什么..." />
        <button type="button" aria-label="send-chat" disabled={chatSending || !chatDraft.trim()} onClick={onSend}>
          <Send size={16} />
        </button>
      </div>
    </section>
  );
}

export function HeatMeter({ countdownPhase, heat }: { countdownPhase: CountdownPhase; heat: HeatSnapshot }) {
  const hasHeat = heat.activeBidders30s > 0 || heat.acceptedBids30s > 0 || heat.priceVelocityCentsPerMin > 0;
  const totalBids = Math.max(0, heat.totalAcceptedBids ?? 0);
  const primaryCopy = hasHeat
    ? `近30秒 ${heat.activeBidders30s} 人 · ${heat.acceptedBids30s} 次出价`
    : totalBids > 0
      ? `累计 ${totalBids} 次出价`
      : h5Copy.noBids;
  const secondaryCopy = heat.priceVelocityCentsPerMin > 0
    ? `${formatCents(heat.priceVelocityCentsPerMin)}/分钟`
    : totalBids > 0
      ? '最新状态已确认'
      : '等待第一手';
  return (
    <div className="heat-meter" data-testid="heat-meter" data-countdown-phase={countdownPhase}>
      <span><Sparkles size={13} /> 竞价热度</span>
      <strong>{primaryCopy}</strong>
      <em>{secondaryCopy}</em>
    </div>
  );
}

export function LiveOpsPanel({
  activeTeam,
  followed,
  heat,
  leaderboard,
  liveOpsCampaign,
  liveOpsBusy,
  liveOpsError,
  likeCount,
  scenario,
  onOpenProducts,
  onOpenQA,
  onOpenLeaderboard,
  onEnterLuckyDraw,
  onOpenLuckyDraw,
  onSelectTeam,
  onToggleFollow
}: {
  activeTeam: 'craft' | 'story';
  followed: boolean;
  heat: HeatSnapshot;
  leaderboard: LeaderboardPayload | null;
  liveOpsCampaign: LiveOpsCampaign | null;
  liveOpsBusy: string;
  liveOpsError: string;
  likeCount: number;
  scenario: Scenario;
  onOpenProducts: () => void;
  onOpenQA: () => void;
  onOpenLeaderboard: () => void;
  onEnterLuckyDraw: () => void;
  onOpenLuckyDraw: () => void;
  onSelectTeam: (team: 'craft' | 'story') => void;
  onToggleFollow: () => void;
}) {
  const taskMap = new Map((liveOpsCampaign?.tasks ?? []).map((task) => [task.key, task]));
  const taskDone = (key: 'watch' | 'follow' | 'ask' | 'leaderboard') => Boolean(taskMap.get(key)?.completed_at);
  const finishedTasks = liveOpsCampaign?.progress ?? 0;
  const craftCount = liveOpsCampaign?.team_scores?.find((team) => team.key === 'craft')?.count ?? 0;
  const storyCount = liveOpsCampaign?.team_scores?.find((team) => team.key === 'story')?.count ?? 0;
  const teamTotal = craftCount + storyCount;
  const heatTotal = Math.max(1, heat.acceptedBids30s + heat.activeBidders30s + likeCount);
  const craftScore = teamTotal > 0
    ? Math.round((craftCount / teamTotal) * 100)
    : Math.min(100, Math.max(12, Math.round(((heat.acceptedBids30s * 2 + likeCount) / (heatTotal + 3)) * 100)));
  const storyScore = Math.max(0, 100 - craftScore);
  const topEntry = leaderboard?.entries?.[0];
  const leaderIsMe = topEntry?.is_current === true || leaderboard?.my_rank === 1;
  const entryCopy = leaderIsMe
    ? '榜一特效已点亮'
    : followed
      ? '欢迎回来，已关注'
      : '关注后点亮入场牌';
  const luckyDraw = liveOpsCampaign?.lucky_draw;
  const drawStatus = luckyDraw?.my_entry_status === 'OPENED'
    ? `已领取：${luckyDraw.my_reward_label ?? '直播间权益'}`
    : luckyDraw?.my_entry_status === 'ENTERED'
      ? `${luckyDraw.participants} 人已领取资格 · 可查看奖励`
      : luckyDraw?.can_enter
        ? `${luckyDraw.participants} 人已领取资格 · 现在可参与`
        : `完成 ${luckyDraw?.completed_task_count ?? finishedTasks}/${luckyDraw?.eligible_task_count ?? 4} 后解锁`;
  return (
    <section className="live-ops-panel" data-testid="live-ops-panel" aria-label="live-ops-panel">
      <div className="warmup-card" data-testid="warmup-card">
        <div>
          <span><Sparkles size={13} /> 暖场任务</span>
          <strong>{finishedTasks}/{liveOpsCampaign?.tasks.length ?? 4} 已完成</strong>
        </div>
        <div className="warmup-task-grid">
          <button type="button" className={taskDone('watch') ? 'done' : ''} disabled={liveOpsBusy === 'watch'} onClick={onOpenProducts}>看拍品</button>
          <button type="button" className={taskDone('follow') ? 'done' : ''} disabled={liveOpsBusy === 'follow'} onClick={onToggleFollow}>{followed ? '已关注' : '关注'}</button>
          <button type="button" className={taskDone('ask') ? 'done' : ''} disabled={liveOpsBusy === 'ask'} onClick={onOpenQA}>问拍品</button>
          <button type="button" className={taskDone('leaderboard') ? 'done' : ''} disabled={liveOpsBusy === 'leaderboard'} onClick={onOpenLeaderboard}>看榜单</button>
        </div>
        <small>{liveOpsError || liveOpsCampaign?.disclaimer || '只记录互动准备，不承诺真实奖品、优惠或订单权益。'}</small>
      </div>
      <div className="lucky-draw-card" data-testid="lucky-draw-card" data-draw-state={luckyDraw?.my_entry_status ?? 'READY'}>
        <div>
          <span><Sparkles size={13} /> {luckyDraw?.title ?? '直播间权益'}</span>
          <strong>{drawStatus}</strong>
        </div>
        <p>{luckyDraw?.description ?? '完成互动任务后领取直播间权益；当前权益只用于本场互动展示，不抵扣订单金额。'}</p>
        {luckyDraw?.my_entry_status === 'OPENED' ? (
          <div className="lucky-reward-reveal" aria-label="lucky-draw-reward">
            {luckyDraw.my_reward_label ?? '直播间奖励'}
          </div>
        ) : null}
        <div className="lucky-draw-actions">
          {luckyDraw?.my_entry_status === 'ENTERED' ? (
            <button type="button" onClick={onOpenLuckyDraw}>查看奖励</button>
          ) : luckyDraw?.my_entry_status === 'OPENED' ? (
            <button type="button" onClick={onOpenLeaderboard}>查看榜单</button>
          ) : (
            <button type="button" disabled={!luckyDraw?.can_enter} onClick={onEnterLuckyDraw}>领取资格</button>
          )}
        </div>
      </div>
      <div className="buyer-pk-card" data-testid="buyer-pk-card">
        <div className="pk-title">
          <span><Flame size={13} /> 讲解偏好</span>
          <strong>{auctionStatusLabel(scenario.status)}</strong>
        </div>
        <div className="pk-bars" style={{ '--craft-score': `${craftScore}%`, '--story-score': `${storyScore}%` } as React.CSSProperties}>
          <button type="button" className={activeTeam === 'craft' ? 'active' : ''} onClick={() => onSelectTeam('craft')}>
            <span>看工艺</span><strong>{craftScore}%</strong>
          </button>
          <button type="button" className={activeTeam === 'story' ? 'active' : ''} onClick={() => onSelectTeam('story')}>
            <span>听故事</span><strong>{storyScore}%</strong>
          </button>
        </div>
        <small>投票会汇总给商家，帮助主播下一段讲证书、瑕疵或工艺；不影响价格、排名或成交。</small>
      </div>
      <button type="button" className={`entry-effect-card ${leaderIsMe ? 'is-leader' : followed ? 'is-followed' : ''}`} data-testid="entry-effect-card" onClick={leaderIsMe || followed ? onOpenLeaderboard : onToggleFollow}>
        <span>{leaderIsMe ? <Trophy size={15} /> : <ShieldCheck size={15} />} {entryCopy}</span>
        <strong>{leaderIsMe ? `${topEntry?.user_masked ?? '我'} · ${formatCents(topEntry?.amount_cents ?? 0)}` : followed ? '入场牌已显示' : '点亮入场牌'}</strong>
      </button>
    </section>
  );
}

export function AuctionStatePanel({
  atmosphereCue,
  connectionPhase,
  countdownPhase,
  countdownCopy,
  currentPriceCents,
  extensionNotice,
  heat,
  item,
  leaderboard,
  minimumNextBidCents,
  nextAuction,
  nextBidCents,
  orderAmountCents,
  orderID,
  paymentPhase,
  riskCode,
  resultSheetKind,
  scenario,
  settlementSeq,
  terminalPriceCents,
  terminalWinnerID,
  terminalWinnerMasked,
  onClose,
  onDecreaseBid,
  onIncreaseBid,
  onOpenOrders,
  onOpenSheet,
  onPay,
  onPrimaryAction
}: {
  atmosphereCue: AtmosphereCue | null;
  connectionPhase: ConnectionPhase;
  countdownPhase: CountdownPhaseState;
  countdownCopy: string;
  currentPriceCents: number;
  extensionNotice: string;
  heat: HeatSnapshot;
  item: AuctionItem;
  leaderboard: LeaderboardPayload | null;
  minimumNextBidCents: number;
  nextAuction?: AuctionSummary;
  nextBidCents: number;
  orderAmountCents: number;
  orderID: string;
  paymentPhase: PaymentPhase;
  riskCode: string;
  resultSheetKind: ResultSheetKind | null;
  scenario: Scenario;
  settlementSeq?: number;
  terminalPriceCents: number;
  terminalWinnerID: string;
  terminalWinnerMasked: string;
  onClose: () => void;
  onDecreaseBid: () => void;
  onIncreaseBid: () => void;
  onOpenOrders: () => void;
  onOpenSheet: (sheet: BottomSheetKey) => void;
  onPay: () => void;
  onPrimaryAction: () => void;
}) {
  const unsafeAction = isDangerousActionDisabled(scenario, connectionPhase);
  const primaryDisabled = scenario.sold ? scenario.ctaDisabled : unsafeAction;
  const dockState = scenario.pending
    ? 'PENDING'
    : unsafeAction && !scenario.sold
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
  const resultRankAction = scenario.sold
    ? {
      headline: scenario.winner ? '已中拍 · 订单待支付' : '本场已落槌',
      action: scenario.winner ? '确认成交事实再支付' : '查看出价记录',
      freshness: '结果以服务端终态为准'
    }
    : rankAction;
  const mediaURL = displayMediaURL(item.video_poster_url ?? item.videoPosterURL ?? item.image_url ?? item.imageURL);
  const bidHint = (() => {
    if (scenario.sold) {
      return scenario.winner
        ? `成交价 ${scenario.price} · 订单以服务端为准`
        : '本场已落槌，出价入口已关闭';
    }
    if (scenario.title === '需完成验证') return scenario.feedback;
    if (scenario.ctaDisabled && !scenario.sold && scenario.leader.includes('你')) return h5Copy.selfLeading;
    if (unsafeAction && !scenario.sold) return h5Copy.confirmingPrice;
    if (nextBidCents > minimumNextBidCents) return `高于当前价 ${formatCents(nextBidCents - currentPriceCents)} · 高于最低下一口 ${formatCents(nextBidCents - minimumNextBidCents)}`;
    return `最低有效出价 ${formatCents(minimumNextBidCents)} · 按 ${formatCents(Math.max(0, nextBidCents - currentPriceCents))} 加价`;
  })();
  const slideConfirmEnabled = !primaryDisabled && !scenario.sold && !scenario.winner;
  const [slideProgress, setSlideProgress] = useState(0);
  const [slideOffsetPx, setSlideOffsetPx] = useState(0);
  const slideProgressRef = useRef(0);
  const slideStartXRef = useRef<number | null>(null);
  const slideTrackRef = useRef<HTMLButtonElement | null>(null);
  const suppressSlideClickRef = useRef(false);
  const resetSlide = () => {
    slideProgressRef.current = 0;
    setSlideProgress(0);
    setSlideOffsetPx(0);
  };
  const updateSlide = (clientX: number) => {
    const startX = slideStartXRef.current;
    const track = slideTrackRef.current;
    if (startX == null || !track) return;
    const maxOffset = Math.max(1, track.getBoundingClientRect().width - 52);
    const offset = Math.max(0, Math.min(maxOffset, clientX - startX));
    const progress = (offset / maxOffset) * 100;
    slideProgressRef.current = progress;
    setSlideProgress(progress);
    setSlideOffsetPx(offset);
  };
  const finishSlide = () => {
    const confirmed = slideProgressRef.current >= 78;
    resetSlide();
    slideStartXRef.current = null;
    if (confirmed) {
      suppressSlideClickRef.current = true;
      window.setTimeout(() => {
        suppressSlideClickRef.current = false;
      }, 0);
      onPrimaryAction();
    }
  };
  useEffect(() => {
    resetSlide();
  }, [scenario.cta, primaryDisabled]);

  return (
    <section
      className={`auction-panel bid-dock ${scenario.stale ? 'is-stale' : ''}`}
      aria-label="auction-state"
      data-dock-state={dockState}
      data-atmosphere-kind={atmosphereCue?.kind ?? 'none'}
      data-countdown-phase={countdownPhase.phase}
    >
      <div className="bid-sheet-handle" aria-hidden="true" />
      <div className="bid-sheet-title">
        <strong>{scenario.sold ? '竞拍结果' : '参与竞拍'}</strong>
        <button type="button" aria-label="关闭竞拍面板" onClick={onClose}><X size={18} /></button>
      </div>
      <div className="bid-item-summary">
        <span className={`bid-item-thumb ${mediaURL ? 'has-media' : ''}`} style={mediaURL ? { '--bid-item-media-url': `url("${mediaURL}")` } as React.CSSProperties : undefined}>
          {!mediaURL && <PackageCheck size={18} />}
        </span>
        <div>
          <strong>{item.title ?? scenario.title}</strong>
          <em>{scenario.leader}</em>
        </div>
      </div>
      <div className="dock-price-row">
        <div>
          <p className="eyebrow">{scenario.title}</p>
          <h2 data-testid="auction-price" aria-live="polite" aria-atomic="true">{scenario.price}</h2>
        </div>
        <div className="countdown-row" data-testid="auction-countdown" data-effect={atmosphereCue?.kind === 'extended' ? 'extension-stretch' : 'none'} data-countdown-phase={countdownPhase.phase}>
          <Clock3 size={16} />
          <span>{scenario.countdown ?? countdownCopy}</span>
          {countdownPhase.phase === 'hammer' && !scenario.stale && !scenario.sold && <strong>{countdownPhase.beat || '落槌窗口'}</strong>}
          {extensionNotice && !scenario.sold && <strong>{extensionNotice}</strong>}
        </div>
      </div>
      <div className="dock-rank-row">
        <span className="status-chip" data-state={scenario.status}>{auctionStatusLabel(scenario.status)}</span>
        <span>{scenario.leader}</span>
        <strong>{rankCopy} · {gapCopy}</strong>
      </div>
      <div className="rank-strip" data-testid="rank-strip">
        <span>{resultRankAction.headline}</span>
        <strong>{resultRankAction.action}</strong>
        <em>{resultRankAction.freshness}</em>
      </div>
      <div className="signal-row">
        {scenario.stale || connectionPhase === 'disconnected' ? <WifiOff size={16} /> : <Wifi size={16} />}
        <span>{connectionSyncCopy(connectionPhase, scenario.stale, scenario.staleCopy)}</span>
      </div>
      <div className="dock-feedback" aria-live={scenario.rejected || scenario.stale ? 'assertive' : 'polite'}>
        <span>{scenario.feedback} · <strong data-testid="bid-hint">{bidHint}</strong></span>
      </div>
      {scenario.rejected || riskCode ? (
        <div className="risk-action" data-testid="h5-risk-action" role="status">
          <AlertTriangle size={15} />
          <span>{riskActionCopy(riskCode)}</span>
        </div>
      ) : null}
      <ResultSheet
        activeSheet={null}
        heat={heat}
        item={item}
        kind={resultSheetKind}
        nextAuction={nextAuction}
        paymentPhase={paymentPhase}
        scenario={scenario}
        settlementSeq={settlementSeq}
        terminalPriceCents={terminalPriceCents || currentPriceCents}
        terminalWinnerID={terminalWinnerID}
        terminalWinnerMasked={terminalWinnerMasked}
        leaderboard={leaderboard}
        userBestCents={leaderboard?.my_best_amount_cents ?? 0}
        orderID={orderID}
        orderAmountCents={orderAmountCents}
        compact
        onOpenOrders={onOpenOrders}
        onPay={onPay}
      />
      <div className="bid-stepper">
        <button type="button" aria-label="decrease" onClick={onDecreaseBid}>-</button>
        <span>{scenario.sold ? '查看订单' : formatCents(nextBidCents)}</span>
        <button type="button" aria-label="increase" onClick={onIncreaseBid}><ChevronUp size={18} /></button>
      </div>
      <button
        ref={slideTrackRef}
        className={`primary-cta ${slideConfirmEnabled ? 'slide-confirm-cta' : ''}`}
        data-testid="bid-cta"
        data-slide-progress={Math.round(slideProgress)}
        disabled={primaryDisabled}
        onClick={() => {
          if (slideConfirmEnabled && suppressSlideClickRef.current) return;
          onPrimaryAction();
        }}
        onKeyDown={(event) => {
          if (!slideConfirmEnabled) return;
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            onPrimaryAction();
          }
        }}
        onPointerDown={(event) => {
          if (!slideConfirmEnabled) return;
          slideStartXRef.current = event.clientX;
          setSlideProgress(8);
          event.currentTarget.setPointerCapture(event.pointerId);
        }}
        onPointerMove={(event) => {
          if (!slideConfirmEnabled) return;
          updateSlide(event.clientX);
        }}
        onPointerUp={(event) => {
          if (!slideConfirmEnabled) return;
          event.currentTarget.releasePointerCapture(event.pointerId);
          finishSlide();
        }}
        onPointerCancel={() => {
          slideStartXRef.current = null;
          resetSlide();
        }}
      >
        {slideConfirmEnabled && (
          <>
            <span className="slide-confirm-fill" style={{ width: `${slideProgress}%` }} aria-hidden="true" />
            <span className="slide-confirm-thumb" style={{ transform: `translateX(${slideOffsetPx}px)` }} aria-hidden="true">→</span>
          </>
        )}
        {scenario.winner ? <CreditCard size={18} /> : scenario.rejected ? <AlertTriangle size={18} /> : <CheckCircle2 size={18} />}
        <span>{slideConfirmEnabled ? `滑动${scenario.cta}` : scenario.cta}</span>
      </button>
      <div className="dock-shortcuts" aria-label="bid-dock-shortcuts">
        <button type="button" onClick={() => onOpenSheet('details')}>拍品与规则</button>
        <button type="button" onClick={() => onOpenSheet('leaderboard')}>出价榜</button>
        <button type="button" onClick={() => onOpenSheet('maxBid')}>自动加价</button>
        <button type="button" onClick={() => onOpenSheet('more')}>保护</button>
      </div>
    </section>
  );
}

export function BottomSheet({
  activeAuctionID,
  activeSheet,
  auctions,
  bidHistory,
  connectionPhase,
  historyError,
  historyLoading,
  item,
  leaderboard,
  maxBidAmountCents,
  maxBidAmountText,
  maxBidFeedback,
  maxBidIntent,
  maxBidPhase,
  minimumNextBidCents,
  nextBidCents,
  orderHistory,
  paymentConfirmOpen,
  payableOrderAmountCents,
  payableOrderID,
  qaAnswer,
  qaHistory,
  qaDraft,
  qaLoading,
  scenario,
  activeTeam,
  followed,
  heat,
  likeCount,
  liveOpsBusy,
  liveOpsCampaign,
  liveOpsError,
  soundEnabled,
  onCancelMaxBid,
  onChangeMaxBidAmountText,
  onClose,
  onClosePaymentConfirm,
  onDecreaseMaxBid,
  onIncreaseMaxBid,
  onOpenOrderDetail,
  onOpenLeaderboard,
  onOpenProducts,
  onOpenSheet,
  onRefreshHistory,
  onRefreshLeaderboard,
  onRefreshMaxBid,
  onAskQA,
  onAskQAPrompt,
  onEnterLuckyDraw,
  onOpenLuckyDraw,
  onQADraftChange,
  onSelectTeam,
  onSubmitMaxBid,
  onConfirmPay,
  selectedOrderID,
  onToggleFollow,
  onToggleSound
}: {
  activeAuctionID: string;
  activeSheet: BottomSheetKey | null;
  auctions: AuctionSummary[];
  bidHistory: HistoryRow[];
  connectionPhase: ConnectionPhase;
  historyError: string;
  historyLoading: boolean;
  item: AuctionItem;
  leaderboard: LeaderboardPayload | null;
  maxBidAmountCents: number;
  maxBidAmountText: string;
  maxBidFeedback: string;
  maxBidIntent: MaxBidIntent | null;
  maxBidPhase: MaxBidPhase;
  minimumNextBidCents: number;
  nextBidCents: number;
  orderHistory: HistoryRow[];
  paymentConfirmOpen: boolean;
  payableOrderAmountCents: number;
  payableOrderID: string;
  qaAnswer?: ProductQAAnswer;
  qaHistory: ProductQAAnswer[];
  qaDraft: string;
  qaLoading: boolean;
  scenario: Scenario;
  activeTeam: 'craft' | 'story';
  followed: boolean;
  heat: HeatSnapshot;
  likeCount: number;
  liveOpsBusy: string;
  liveOpsCampaign: LiveOpsCampaign | null;
  liveOpsError: string;
  soundEnabled: boolean;
  onCancelMaxBid: () => void;
  onChangeMaxBidAmountText: (value: string) => void;
  onClose: () => void;
  onClosePaymentConfirm: () => void;
  onDecreaseMaxBid: () => void;
  onIncreaseMaxBid: () => void;
  onOpenOrderDetail: (orderID: string) => void;
  onOpenLeaderboard?: () => void;
  onOpenProducts?: () => void;
  onOpenSheet: (sheet: BottomSheetKey) => void;
  onRefreshHistory: () => void;
  onRefreshLeaderboard: () => void;
  onRefreshMaxBid: () => void;
  onAskQA: () => void;
  onAskQAPrompt: (prompt: string) => void;
  onEnterLuckyDraw: () => void;
  onOpenLuckyDraw: () => void;
  onQADraftChange: (draft: string) => void;
  onSelectTeam: (team: 'craft' | 'story') => void;
  onSubmitMaxBid: () => void;
  onConfirmPay: () => void;
  selectedOrderID: string;
  onToggleFollow: () => void;
  onToggleSound: () => void;
}) {
  const titleMap: Record<BottomSheetKey, string> = {
    products: '商品袋',
    details: '规则凭证',
    maxBid: '自动加价',
    leaderboard: '出价榜',
    history: '我的记录',
    orders: '我的记录',
    qa: '拍品问答',
    liveops: '直播互动',
    more: '我的'
  };
  const sheetGroups: Record<BottomSheetKey, Array<[BottomSheetKey, string]>> = {
    products: [['products', '本场拍品'], ['details', '规则凭证'], ['leaderboard', '出价榜']],
    details: [['products', '本场拍品'], ['details', '规则凭证'], ['leaderboard', '出价榜']],
    leaderboard: [['products', '本场拍品'], ['details', '规则凭证'], ['leaderboard', '出价榜']],
    liveops: [['liveops', '互动任务'], ['qa', '拍品问答']],
    qa: [['liveops', '互动任务'], ['qa', '拍品问答']],
    more: [['more', '设置与保障'], ['orders', '我的记录']],
    orders: [['more', '设置与保障'], ['orders', '我的记录']],
    history: [['more', '设置与保障'], ['orders', '我的记录']],
    maxBid: [['maxBid', '自动加价']]
  };
  useEffect(() => {
    if (!activeSheet) return undefined;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [activeSheet, onClose]);

  if (!activeSheet) {
    return paymentConfirmOpen ? (
      <PaymentConfirmDialog
        amountCents={payableOrderAmountCents}
        orderID={payableOrderID}
        onCancel={onClosePaymentConfirm}
        onConfirm={onConfirmPay}
      />
    ) : null;
  }
  const activeTabs = sheetGroups[activeSheet] ?? [];
  const selectedTab = activeSheet === 'history' ? 'orders' : activeSheet;
  return (
    <div className="sheet-backdrop" data-testid="bottom-sheet-backdrop" onClick={onClose}>
      <section className="bottom-sheet" data-testid="bottom-sheet" role="dialog" aria-modal="true" aria-label={titleMap[activeSheet]} onClick={(event) => event.stopPropagation()}>
        <div className="sheet-handle" aria-hidden="true" />
        <div className="sheet-header">
          <h2>{titleMap[activeSheet]}</h2>
          <button type="button" aria-label="关闭面板" onClick={onClose}>关闭</button>
        </div>
        {activeTabs.length > 1 ? (
          <div className="sheet-tabs" role="tablist" aria-label={`${titleMap[activeSheet]}分区`}>
            {activeTabs.map(([key, label]) => (
              <button type="button" role="tab" aria-selected={selectedTab === key} key={key} onClick={() => onOpenSheet(key)}>{label}</button>
            ))}
          </div>
        ) : null}
        <div className="sheet-content">
          {activeSheet === 'products' && <ProductListSheet auctions={auctions} activeAuctionID={activeAuctionID} scenario={scenario} />}
          {activeSheet === 'details' && <ProductRuleSheet item={item} auction={auctions.find((row) => row.id === activeAuctionID)} scenario={scenario} />}
          {activeSheet === 'maxBid' && (
            <MaxBidSheet
              amountCents={maxBidAmountCents}
              amountText={maxBidAmountText}
              connectionPhase={connectionPhase}
              feedback={maxBidFeedback}
              intent={maxBidIntent}
              maxAmountCents={auctions.find((row) => row.id === activeAuctionID)?.cap_price_cents ?? 0}
              minimumNextBidCents={minimumNextBidCents}
              phase={maxBidPhase}
              scenario={scenario}
              onAmountTextChange={onChangeMaxBidAmountText}
              onCancel={onCancelMaxBid}
              onDecrease={onDecreaseMaxBid}
              onIncrease={onIncreaseMaxBid}
              onRefresh={onRefreshMaxBid}
              onSubmit={onSubmitMaxBid}
            />
          )}
          {activeSheet === 'leaderboard' && <LeaderboardSheet activeAuctionID={activeAuctionID} leaderboard={leaderboard} nextBidCents={nextBidCents} onRefresh={onRefreshLeaderboard} />}
          {(activeSheet === 'history' || activeSheet === 'orders') && (
            <HistoryPanel
              bidHistory={bidHistory}
              historyError={historyError}
              historyLoading={historyLoading}
              item={item}
              orderHistory={orderHistory}
              selectedOrderID={selectedOrderID}
              activeAuction={auctions.find((row) => row.id === activeAuctionID)}
              onOpenOrderDetail={onOpenOrderDetail}
              onRefresh={onRefreshHistory}
            />
          )}
          {activeSheet === 'qa' && (
            <ProductQASheet
              answer={qaAnswer}
              history={qaHistory}
              draft={qaDraft}
              loading={qaLoading}
              onAsk={onAskQA}
              onAskPrompt={onAskQAPrompt}
              onDraftChange={onQADraftChange}
            />
          )}
          {activeSheet === 'liveops' && (
            <LiveOpsPanel
              activeTeam={activeTeam}
              followed={followed}
              heat={heat}
              leaderboard={leaderboard}
              liveOpsCampaign={liveOpsCampaign}
              liveOpsBusy={liveOpsBusy}
              liveOpsError={liveOpsError}
              likeCount={likeCount}
              scenario={scenario}
              onOpenProducts={onOpenProducts ?? (() => onOpenSheet('products'))}
              onOpenQA={() => onOpenSheet('qa')}
              onOpenLeaderboard={onOpenLeaderboard ?? (() => onOpenSheet('leaderboard'))}
              onEnterLuckyDraw={onEnterLuckyDraw}
              onOpenLuckyDraw={onOpenLuckyDraw}
              onSelectTeam={onSelectTeam}
              onToggleFollow={onToggleFollow}
            />
          )}
          {activeSheet === 'more' && (
            <MoreSheet
              followed={followed}
              onOpenRecords={() => onOpenSheet('orders')}
              onToggleFollow={onToggleFollow}
              soundEnabled={soundEnabled}
              onToggleSound={onToggleSound}
            />
          )}
        </div>
      </section>
      {paymentConfirmOpen ? (
        <PaymentConfirmDialog
          amountCents={payableOrderAmountCents}
          orderID={payableOrderID}
          onCancel={onClosePaymentConfirm}
          onConfirm={onConfirmPay}
        />
      ) : null}
    </div>
  );
}

export function ProductListSheet({
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
              <span>{status} · {formatCents(auction.current_price_cents ?? 0)} · {auction.accepted_bid_count ?? 0} 次出价</span>
            </div>
            <em>{auction.id === activeAuctionID ? '当前拍品' : status}</em>
          </article>
        );
      })}
    </div>
  );
}

export function ProductRuleSheet({ auction, item, scenario }: { auction?: AuctionSummary; item: AuctionItem; scenario: Scenario }) {
  const mediaURL = displayMediaURL(item.video_poster_url ?? item.videoPosterURL ?? item.image_url ?? item.imageURL);
  const depositFloor = auction?.rule?.deposit_floor_cents ?? 0;
  const depositCap = auction?.rule?.deposit_cap_cents ?? 0;
  const depositBps = auction?.rule?.deposit_bps ?? 0;
  const depositPercent = depositBps > 0 ? `${(depositBps / 100).toFixed(depositBps % 100 === 0 ? 0 : 2)}%` : '';
  const depositCopy = depositFloor > 0 || depositBps > 0
    ? `本场保证金按成交价${depositPercent ? ` ${depositPercent}` : ''} 预估，最低 ${formatCents(depositFloor)}${depositCap > 0 ? `，最高 ${formatCents(depositCap)}` : ''}。未中拍或订单完成后按支付链路处理。`
    : '本场未展示固定保证金门槛；以服务端出价校验和订单状态为准。';
  const extensionCopy = `最后 ${auction?.rule?.extend_window_seconds ?? 10} 秒内有有效出价，会自动延长 ${auction?.rule?.extend_by_seconds ?? 10} 秒${auction?.rule?.max_extend_count ? `，最多 ${auction.rule.max_extend_count} 次` : ''}，避免最后一秒抢拍。`;
  const capCopy = auction?.cap_price_cents
    ? `价格到达 ${formatCents(auction.cap_price_cents)} 后不再继续抬价。`
    : '本场未设置展示封顶价，仍由服务端规则校验每次出价。';
  const confirmationCopy = auction?.rule?.fat_finger_threshold_cents
    ? `单次出价达到 ${formatCents(auction.rule.fat_finger_threshold_cents)} 会先弹出确认，防止误触。`
    : '异常大额出价会先弹出确认，防止误触。';
  const certificateCopy = item.certificate ?? `GID 20260607 · 可核验`;
  const proofItems = [
    ['证书', certificateCopy],
    ['品相', item.condition ?? h5Copy.merchantTodo],
    ['尺寸', item.dimensions ?? h5Copy.merchantTodo],
    ['材质', item.material ?? h5Copy.merchantTodo],
    ['瑕疵', item.flaws ?? '未登记明显瑕疵'],
    ['运费', item.shipping ?? '运费以订单为准'],
    ['售后', item.return_policy ?? h5Copy.returnPolicy]
  ];

  return (
    <div className="product-rule-sheet">
      <div className="trust-hero">
        <div className={`trust-media ${mediaURL ? 'has-media' : ''}`} style={mediaURL ? { '--trust-media-url': `url("${mediaURL}")` } as React.CSSProperties : undefined}>
          {!mediaURL && <span>{h5Copy.imageTodo}</span>}
        </div>
        <div>
          <p className="eyebrow">商品信任详情</p>
          <h3>{item.title ?? scenario.title}</h3>
          <p>{item.description ?? '主播讲解与证据材料会随当前拍品更新，出价前请确认品相、保证金和延时规则。'}</p>
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
          <strong>大额出价确认</strong>
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

export function MaxBidSheet({
  amountCents,
  amountText,
  connectionPhase,
  feedback,
  intent,
  maxAmountCents,
  minimumNextBidCents,
  phase,
  scenario,
  onAmountTextChange,
  onCancel,
  onDecrease,
  onIncrease,
  onRefresh,
  onSubmit
}: {
  amountCents: number;
  amountText: string;
  connectionPhase: ConnectionPhase;
  feedback: string;
  intent: MaxBidIntent | null;
  maxAmountCents: number;
  minimumNextBidCents: number;
  phase: MaxBidPhase;
  scenario: Scenario;
  onAmountTextChange: (value: string) => void;
  onCancel: () => void;
  onDecrease: () => void;
  onIncrease: () => void;
  onRefresh: () => void;
  onSubmit: () => void;
}) {
  const settingDisabled = scenario.stale || scenario.sold || connectionPhase === 'connecting' || connectionPhase === 'recovering' || connectionPhase === 'disconnected' || phase === 'pending' || phase === 'canceling';
  const cancelDisabled = connectionPhase === 'connecting' || connectionPhase === 'recovering' || connectionPhase === 'disconnected' || phase === 'pending' || phase === 'canceling';
  const active = intent?.status === 'ACTIVE';
  const requiredAmount = maxAmountCents > 0 ? Math.min(minimumNextBidCents, maxAmountCents) : minimumNextBidCents;
  const submittedAmount = maxAmountCents > 0
    ? Math.min(Math.max(amountCents, requiredAmount), maxAmountCents)
    : Math.max(amountCents, requiredAmount);
  const invalid = amountCents < requiredAmount || (maxAmountCents > 0 && amountCents > maxAmountCents);
  return (
    <div className="max-bid-sheet" data-testid="max-bid-sheet">
      <div className="max-bid-status">
        <span>自动加价上限</span>
        <strong>{active ? `${formatCents(intent.max_amount_cents)} · 仅自己可见` : '未启用'}</strong>
        <em>{feedback}</em>
      </div>
      <label className="max-bid-input" htmlFor="max-bid-yuan">
        <span>最高愿付价</span>
        <div>
          <em>¥</em>
          <input
            id="max-bid-yuan"
            aria-label="max-bid-yuan"
            inputMode="decimal"
            disabled={settingDisabled}
            value={amountText}
            placeholder={(requiredAmount / 100).toFixed(2)}
            onChange={(event) => onAmountTextChange(event.target.value)}
          />
        </div>
      </label>
      <div className="max-bid-range">
        <span>不能低于当前可设置价 {formatCents(requiredAmount)}</span>
        {maxAmountCents > 0 ? <span>不能高于封顶价 {formatCents(maxAmountCents)}</span> : null}
      </div>
      <div className="max-bid-stepper" aria-label="max-bid-amount">
        <button type="button" aria-label="decrease-max-bid" disabled={settingDisabled || amountCents <= requiredAmount} onClick={onDecrease}>-</button>
        <span>快捷调整到 {formatCents(submittedAmount)}</span>
        <button type="button" aria-label="increase-max-bid" disabled={settingDisabled || (maxAmountCents > 0 && amountCents >= maxAmountCents)} onClick={onIncrease}><ChevronUp size={18} /></button>
      </div>
      <div className="max-bid-rules">
        <span>仅当前账号可见，不进入公开榜单或房间消息。</span>
        <span>系统只按当前加价阶梯帮你跟价，不会公开你的最高价。</span>
        <span>网络恢复、提交中或本场结束时会暂停设置和取消。</span>
      </div>
      <div className="max-bid-actions">
        <button type="button" onClick={onSubmit} disabled={settingDisabled || invalid}>
          {phase === 'pending' ? '提交中' : active ? `更新为 ${formatCents(submittedAmount)}` : `设置 ${formatCents(submittedAmount)}`}
        </button>
        <button type="button" onClick={onCancel} disabled={cancelDisabled || !active}>
          {phase === 'canceling' ? '取消中' : '取消'}
        </button>
        <button type="button" onClick={onRefresh} disabled={phase === 'pending' || phase === 'canceling'}>刷新</button>
      </div>
    </div>
  );
}

export function LeaderboardSheet({ activeAuctionID, leaderboard, nextBidCents, onRefresh }: { activeAuctionID: string; leaderboard: LeaderboardPayload | null; nextBidCents: number; onRefresh: () => void }) {
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
      {entries.length === 0 ? <p>暂无有效出价</p> : <LeaderboardRows entries={entries} />}
    </div>
  );
}

export function HistorySheet({
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

export function LeaderboardPanel({
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
          <LeaderboardRows entries={leaderboard?.entries ?? []} burstMode={Boolean(leaderboard?.burst_mode)} />
        )}
      </div>
    </section>
  );
}

function LeaderboardRows({ entries, burstMode = false, highlightKind = 'none' }: { entries: NonNullable<LeaderboardPayload['entries']>; burstMode?: boolean; highlightKind?: AtmosphereCue['kind'] | 'none' }) {
  const rowRefs = useRef(new Map<string, HTMLDivElement>());
  const previousRectsRef = useRef(new Map<string, DOMRect>());
  const lastAnimatedAtRef = useRef(0);
  const listKey = useMemo(() => entries.map((entry) => `${entry.user_id}:${entry.rank}:${entry.amount_cents}:${entry.bid_count}`).join('|'), [entries]);

  useLayoutEffect(() => {
    const previousRects = previousRectsRef.current;
    const nextRects = new Map<string, DOMRect>();
    const now = performance.now();
    const coalescing = burstMode || now - lastAnimatedAtRef.current < 180;
    lastAnimatedAtRef.current = now;
    rowRefs.current.forEach((node, key) => {
      const next = node.getBoundingClientRect();
      nextRects.set(key, next);
      const previous = previousRects.get(key);
      if (!previous) {
        if (coalescing) return;
        node.animate([
          { opacity: 0, transform: 'translate3d(10px, 0, 0)' },
          { opacity: 1, transform: 'translate3d(0, 0, 0)' }
        ], { duration: 180, easing: 'cubic-bezier(.2,.8,.2,1)' });
        return;
      }
      const deltaY = previous.top - next.top;
      if (Math.abs(deltaY) > 1) {
        node.animate([
          { transform: `translate3d(0, ${deltaY}px, 0)` },
          { transform: 'translate3d(0, 0, 0)' }
        ], { duration: coalescing ? 120 : 260, easing: 'cubic-bezier(.2,.8,.2,1)' });
      }
    });
    previousRectsRef.current = nextRects;
  }, [listKey, burstMode]);

  return (
    <>
      {entries.map((entry) => {
        const key = entry.user_id || `${entry.rank}-${entry.amount_cents}`;
        return (
          <div
            className={`leaderboard-row ${entry.is_current ? 'is-current' : ''}`}
            key={key}
            data-rank={entry.rank}
            data-current={entry.is_current ? 'true' : 'false'}
            data-highlight={entry.is_current ? highlightKind : entry.rank === 1 ? 'leader' : 'none'}
            data-flip-key={key}
            ref={(node) => {
              if (node) rowRefs.current.set(key, node);
              else rowRefs.current.delete(key);
            }}
          >
            <span>{rankBadgeLabel(entry.rank)}</span>
            <strong>{entry.is_current ? `我（${entry.user_masked}）` : entry.user_masked}</strong>
            <em>{formatCents(entry.amount_cents)}</em>
            <small>{entry.bid_count} 次</small>
          </div>
        );
      })}
    </>
  );
}

export function MoreSheet({
  followed,
  onOpenRecords,
  onToggleFollow,
  soundEnabled,
  onToggleSound
}: {
  followed: boolean;
  onOpenRecords: () => void;
  onToggleFollow: () => void;
  soundEnabled: boolean;
  onToggleSound: () => void;
}) {
  return (
    <div className="more-sheet" data-testid="more-sheet">
      <div className="sheet-action-row">
        <strong><ShieldCheck size={16} /> 直播保护</strong>
      </div>
      <div className="buyer-trust-card" data-testid="buyer-trust-card" role="status" aria-live="polite">
        <span><ShieldCheck size={15} /> 本场受反作弊保护</span>
        <strong>价格、倒计时和有效出价以服务端为准</strong>
        <p>页面不展示虚构观看人数；热度只来自真实出价、榜单和互动任务。异常竞拍由商家端哨兵面板处理，不向买家泄露风控策略。</p>
      </div>
      <button type="button" onClick={onToggleFollow}>
        <ShieldCheck size={16} />
        {followed ? '取消关注直播间' : '关注直播间'}
      </button>
      <button type="button" onClick={onToggleSound}>
        {soundEnabled ? <BellOff size={16} /> : <Bell size={16} />}
        {soundEnabled ? '关闭提示音' : '开启提示音'}
      </button>
      <button type="button" onClick={onOpenRecords}>
        <History size={16} />
        我的出价与订单
      </button>
      <p>保护说明只披露买家可验证事实；不会承诺库存预留、相似拍品优先权或虚构人气。</p>
    </div>
  );
}

export function StateMatrixTabs({
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

export function HistoryPanel({
  activeAuction,
  bidHistory,
  historyError,
  historyLoading,
  item,
  orderHistory,
  selectedOrderID,
  onOpenOrderDetail,
  onRefresh
}: {
  activeAuction?: AuctionSummary;
  bidHistory: HistoryRow[];
  historyError: string;
  historyLoading: boolean;
  item: AuctionItem;
  orderHistory: HistoryRow[];
  selectedOrderID: string;
  onOpenOrderDetail: (orderID: string) => void;
  onRefresh: () => void;
}) {
  const selectedOrder = orderHistory.find((row) => orderIDFromRow(row) === selectedOrderID);
  return (
    <section className="history-panel" data-testid="history-panel">
      <div className="history-title">
        <h2><History size={16} /> 我的记录</h2>
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
          getPrimary={(row) => `出价 ${formatCents(Number(row.amount_cents ?? 0))}`}
          getSecondary={buyerHistorySecondary}
        />
        <HistoryList
          title="订单"
          empty="暂无订单"
          rows={orderHistory}
          getPrimary={(row) => `以 ${formatCents(orderAmount(row))} 拍下`}
          getSecondary={buyerOrderSecondary}
          onRowClick={(row) => onOpenOrderDetail(orderIDFromRow(row))}
        />
      </div>
      {selectedOrder ? (
        <BuyerOrderDetail
          auction={activeAuction}
          item={item}
          order={selectedOrder}
          onClose={() => onOpenOrderDetail('')}
        />
      ) : null}
    </section>
  );
}

function BuyerOrderDetail({
  auction,
  item,
  order,
  onClose
}: {
  auction?: AuctionSummary;
  item: AuctionItem;
  order: HistoryRow;
  onClose: () => void;
}) {
  const amount = orderAmount(order);
  const mediaURL = displayMediaURL(item.image_url ?? item.imageURL ?? item.video_poster_url ?? item.videoPosterURL);
  const depositBPS = auction?.rule?.deposit_bps ?? 0;
  const depositFloor = auction?.rule?.deposit_floor_cents ?? 0;
  const depositCap = auction?.rule?.deposit_cap_cents ?? 0;
  const estimateDeposit = Math.max(depositFloor, Math.round(amount * depositBPS / 10_000));
  const deposit = depositCap > 0 ? Math.min(estimateDeposit, depositCap) : estimateDeposit;
  return (
    <div className="buyer-order-detail" data-testid="buyer-order-detail">
      <div className="buyer-order-detail-head">
        <div>
          <span>订单详情</span>
          <strong>{displayOrderNo(orderIDFromRow(order))}</strong>
        </div>
        <button type="button" onClick={onClose}>收起</button>
      </div>
      <div className="buyer-order-product">
        <span className={`buyer-order-thumb ${mediaURL ? 'has-media' : ''}`} style={mediaURL ? { '--buyer-order-media-url': `url("${mediaURL}")` } as React.CSSProperties : undefined}>
          {!mediaURL && <ShoppingBagIcon size={20} theme="multi-color" fill={iconParkActionFill} />}
        </span>
        <div>
          <strong>{item.title ?? '本场拍品'}</strong>
          <em>{item.certificate ?? '证书待核验'} · {item.condition ?? '实物状态以拍品页为准'}</em>
        </div>
      </div>
      <div className="buyer-order-grid">
        <div><span>拍下金额</span><strong>{formatCents(amount)}</strong></div>
        <div><span>订单状态</span><strong>{orderStatus(order)}</strong></div>
        <div><span>保证金</span><strong>{deposit > 0 ? formatCents(deposit) : '按服务端订单处理'}</strong></div>
        <div><span>支付截止</span><strong>{historyTime({ created_at: order.expire_at }) || '以订单为准'}</strong></div>
        <div><span>加价阶梯</span><strong>{formatCents(auction?.increment_cents ?? 0)}</strong></div>
        <div><span>封顶价</span><strong>{formatCents(auction?.cap_price_cents ?? amount)}</strong></div>
      </div>
      <div className="buyer-order-section">
        <span>商品详情</span>
        <p>{item.description ?? item.return_policy ?? '成交商品以本场拍品信息、证书和实物图为准。'}</p>
        <p>{item.shipping ?? '物流以商家履约配置为准'} · {item.return_policy ?? h5Copy.returnPolicy}</p>
      </div>
    </div>
  );
}

export function ChatPanel({
  chatDraft,
  chatMessages,
  chatSending,
  currentUserID,
  systemMessages,
  onDraftChange,
  onSend
}: {
  chatDraft: string;
  chatMessages: ChatMessage[];
  chatSending: boolean;
  currentUserID: string;
  systemMessages: SystemMessage[];
  onDraftChange: (draft: string) => void;
  onSend: () => void;
}) {
  return (
    <section className="chat-panel" data-testid="chat-panel">
      <div className="history-title">
        <h2><MessageCircle size={16} /> 弹幕</h2>
      </div>
      <div className="chat-list">
        {systemMessages.slice(0, 3).map((message) => (
          <div className="chat-row system" key={`sys-${message.id}`}>
            <strong>{systemMessageLabel(message)}</strong>
            <span>{message.body}</span>
          </div>
        ))}
        {chatMessages.length === 0 && systemMessages.length === 0 ? <p>暂无弹幕</p> : chatMessages.map((message) => (
          <div className="chat-row" key={message.id}>
            <strong>{displayChatUser(message.user_id, currentUserID)}</strong>
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

export function ProductQASheet({
  answer,
  history,
  draft,
  loading,
  onAsk,
  onAskPrompt,
  onDraftChange
}: {
  answer?: ProductQAAnswer;
  history: ProductQAAnswer[];
  draft: string;
  loading: boolean;
  onAsk: () => void;
  onAskPrompt: (prompt: string) => void;
  onDraftChange: (draft: string) => void;
}) {
  const turns = history.length ? history : answer ? [answer] : [];
  const prompts = (answer?.follow_up_prompts ?? []).filter(Boolean).slice(0, 3);
  return (
    <section className="product-qa-sheet" data-testid="product-qa-sheet">
      <div className="sheet-action-row">
        <span>可问拍品详情、竞拍规则和履约保障</span>
        <button type="button" disabled={!draft.trim() || loading} onClick={onAsk}>{loading ? '查询中' : '提问'}</button>
      </div>
      <div className="qa-safety-note">回答只基于商家已上架信息；未披露的真伪、来源、升值或隐藏出价不会回答。</div>
      <div className="chat-input-row">
        <input
          aria-label="product-qa-input"
          maxLength={80}
          value={draft}
          onChange={(event) => onDraftChange(event.currentTarget.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') onAsk();
          }}
          placeholder="例如：起拍价是多少？有瑕疵说明吗？"
        />
        <button type="button" aria-label="ask-product-qa" disabled={!draft.trim() || loading} onClick={onAsk}>
          <Send size={15} />
        </button>
      </div>
      {turns.length ? (
        <div className="qa-thread" aria-label="product-qa-thread">
          {turns.map((turn, index) => (
            <div className="qa-turn" key={`${turn.question ?? 'q'}-${index}`}>
              {turn.question ? <p className="qa-question">{turn.question}</p> : null}
              <div className="qa-answer">
                <strong>{turn.answer}</strong>
                <span>{turn.safety_note}</span>
                {turn.facts_used.length ? <em>来自商家已上架信息</em> : <em>未找到商家已提供信息</em>}
              </div>
            </div>
          ))}
        </div>
      ) : <div className="heat-unavailable">未提供的信息会明确回答“未提供”。</div>}
      {prompts.length ? (
        <div className="qa-prompts" aria-label="product-qa-follow-ups">
          {prompts.map((prompt) => (
            <button type="button" key={prompt} disabled={loading} onClick={() => onAskPrompt(prompt)}>
              {prompt}
            </button>
          ))}
        </div>
      ) : null}
    </section>
  );
}

export function HistoryList({
  title,
  empty,
  rows,
  getPrimary,
  getSecondary,
  onRowClick
}: {
  title: string;
  empty: string;
  rows: HistoryRow[];
  getPrimary: (row: HistoryRow) => string;
  getSecondary: (row: HistoryRow) => string;
  onRowClick?: (row: HistoryRow) => void;
}) {
  const visibleRows = rows.slice(0, 12);
  const hiddenCount = Math.max(0, rows.length - visibleRows.length);
  return (
    <div className="history-list">
      <h3>{title}{rows.length > 0 ? <small>{rows.length} 条</small> : null}</h3>
      {rows.length === 0 ? (
        <p>{empty}</p>
      ) : (
        <>
          {visibleRows.map((row, index) => (
            <button
              type="button"
              className="history-row"
              key={`${title}-${index}`}
              onClick={() => onRowClick?.(row)}
              disabled={!onRowClick}
            >
              <strong>{getPrimary(row)}</strong>
              <span>{getSecondary(row)}</span>
            </button>
          ))}
          {hiddenCount > 0 ? <p className="history-more">已收起更早 {hiddenCount} 条记录</p> : null}
        </>
      )}
    </div>
  );
}
