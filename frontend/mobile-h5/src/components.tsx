import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, BadgeCheck, Bell, BellOff, CheckCircle2, ChevronUp, Clock3, Copy, CreditCard, Download, Flame, Heart, History, Info, MessageCircle, MoreHorizontal, PackageCheck, RefreshCw, Send, ShieldCheck, ShoppingCart, Sparkles, Truck, Trophy, Users, Wifi, WifiOff, X } from 'lucide-react';
import type { AtmosphereCue } from './atmosphere';
import type { AuctionItem, AuctionState, AuctionSummary, BottomSheetKey, ChatMessage, ConnectionPhase, CountdownPhase, CountdownPhaseState, HeatSnapshot, HistoryRow, LeaderboardPayload, LiveOpsCampaign, MaxBidIntent, MaxBidPhase, PaymentPhase, ProductQAAnswer, ResultRecap, ResultSheetKind, Scenario, SoundCapability, SystemMessage } from './domain';
import { auctionStatusLabel, buildHighlightCard, buildResultRecap, connectionSyncCopy, demoLiveVideoURL, demoProductImageURL, formatCents, formatClockTime, isDangerousActionDisabled, leaderboardActionCopy, rankBadgeLabel, riskActionCopy, scenarios } from './domain';
import { h5Copy } from './copy';

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
  lotTitle,
  nextBidCents,
  roomID,
  scenario,
  soundEnabled,
  soundCapability,
  systemMessages,
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
  lotTitle: string;
  nextBidCents: number;
  roomID: string;
  scenario: Scenario;
  soundEnabled: boolean;
  soundCapability: SoundCapability;
  systemMessages: SystemMessage[];
  onOpenBid: () => void;
  onOpenLiveOps: () => void;
  onOpenMore: () => void;
  onOpenProducts: () => void;
  onToggleFollow: () => void;
  onLike: () => void;
  onToggleSound: () => void;
}) {
  const mediaURL = item.video_poster_url ?? item.videoPosterURL ?? item.image_url ?? item.imageURL ?? '';
  const videoURL = demoLiveVideoURL;
  const activeAuction = auctions.find((auction) => auction.id === activeAuctionID);
  const queuedCount = auctions.filter((auction) => auction.id !== activeAuctionID).length;
  const proofChips: Array<{ icon: React.ReactNode; label: string }> = [];
  if (item.certificate) proofChips.push({ icon: <BadgeCheck size={13} />, label: item.certificate });
  if (item.condition) proofChips.push({ icon: <PackageCheck size={13} />, label: item.condition });
  if (item.shipping) proofChips.push({ icon: <Truck size={13} />, label: item.shipping });
  const visibleSystem = systemMessages.slice(0, 2).map((message) => ({
    id: `sys-${message.id}`,
    user: '助手',
    body: message.body
  }));
  const visibleChat = [
    ...visibleSystem,
    ...chatMessages.slice(-3).map((message) => ({
      id: `chat-${message.id}`,
      user: message.user_id === currentUserID ? '我' : `${message.user_id.slice(0, 2)}**`,
      body: message.body
    }))
  ].slice(-4);
  const connectionCopy = connectionPhase === 'connected'
    ? '已连接'
    : connectionPhase === 'recovering'
      ? h5Copy.loading
      : connectionPhase === 'connecting'
        ? '连接中'
        : '已断开';
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
      <BarrageLayer messages={systemMessages} />
      <div className="video-topbar">
        <div className="host-profile">
          <span className="host-avatar">{roomID.slice(0, 1).toUpperCase()}</span>
          <span><strong>{roomCopy}</strong><em>正在直播</em></span>
          <button type="button" className={followed ? 'is-followed' : ''} onClick={onToggleFollow}>{followed ? '已关注' : '关注'}</button>
        </div>
        <span className="viewer-count avatar-stack" title="真实竞价热度，不展示虚构观看人数"><Users size={13} /> {heat.activeBidders30s > 0 ? `近30秒 ${heat.activeBidders30s} 人` : '等待买家进入'}</span>
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
      {countdownPhase.phase === 'hammer' && countdownPhase.beat && !scenario.stale && !scenario.sold ? (
        <div className="hammer-beat-layer" data-testid="hammer-beat-layer" aria-live="polite">
          <span>{countdownPhase.beat}</span>
          <strong>{countdownPhase.beat === '最后一次' ? '落锤前最后确认' : '有效出价仍会延时'}</strong>
          <em>以最终成交结果为准</em>
        </div>
      ) : null}
      <HeatMeter heat={heat} countdownPhase={countdownPhase.phase} />
      <button className="floating-product-card" type="button" onClick={onOpenBid} data-testid="floating-product-card" aria-label="进入竞拍面板">
        <span className={`floating-thumb ${mediaURL ? 'has-media' : ''}`} style={mediaURL ? { '--floating-media-url': `url("${mediaURL}")` } as React.CSSProperties : undefined}>
          {!mediaURL && <ShoppingCart size={18} />}
        </span>
        <span className="floating-product-copy">
          <strong>{activeAuction?.item?.title ?? lotTitle}</strong>
          <span className="floating-auction-meta">
            <em data-testid="floating-auction-price">{scenario.status === 'ACTIVE' ? `当前最高价 ${formatCents(currentPriceCents)}` : `${auctionStatusLabel(scenario.status)} · ${scenario.price}`}</em>
            <small data-testid="floating-auction-countdown"><Clock3 size={12} />{scenario.countdown ?? countdownCopy}</small>
            <small data-testid="floating-auction-status">{auctionStatusLabel(scenario.status)} · {connectionCopy}</small>
          </span>
        </span>
        <span className="floating-product-action">{floatingActionCopy}</span>
      </button>
      <div className="live-action-rail" aria-label="live-actions">
        <button type="button" onClick={onOpenProducts} aria-label="商品列表"><ShoppingCart size={20} /><span>{queuedCount + 1}</span></button>
        <button type="button" onClick={onOpenLiveOps} aria-label="直播互动"><Sparkles size={20} /></button>
        <button type="button" onClick={onLike} aria-label="点赞"><Heart size={20} /><span>{likeCount}</span></button>
        <button type="button" onClick={onOpenMore} aria-label="更多"><MoreHorizontal size={20} /></button>
      </div>
    </section>
  );
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
          <strong>助手</strong>
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

function displayOrderNo(orderID: string) {
  if (!orderID) return h5Copy.loading;
  const compact = orderID.replace(/^ord[_-]?/i, '').replace(/[^a-z0-9]/gi, '').slice(-8).toUpperCase();
  return `JP${new Date().toISOString().slice(0, 10).replace(/-/g, '')}-${compact || '00000000'}`;
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
    ? `已开出：${luckyDraw.my_reward_label ?? '直播间奖励'}`
    : luckyDraw?.my_entry_status === 'ENTERED'
      ? `${luckyDraw.participants} 人已参与 · 可开奖`
      : luckyDraw?.can_enter
        ? `${luckyDraw.participants} 人已参与 · 现在可参加`
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
        <small>{liveOpsError || liveOpsCampaign?.disclaimer || '只记录互动准备，不抽奖、不承诺中奖或优惠。'}</small>
      </div>
      <div className="lucky-draw-card" data-testid="lucky-draw-card" data-draw-state={luckyDraw?.my_entry_status ?? 'READY'}>
        <div>
          <span><Sparkles size={13} /> {luckyDraw?.title ?? '开拍福袋'}</span>
          <strong>{drawStatus}</strong>
        </div>
        <p>{luckyDraw?.description ?? '完成准备后参与，奖励用于直播间互动展示。'}</p>
        {luckyDraw?.my_entry_status === 'OPENED' ? (
          <div className="lucky-reward-reveal" aria-label="lucky-draw-reward">
            {luckyDraw.my_reward_label ?? '直播间奖励'}
          </div>
        ) : null}
        <div className="lucky-draw-actions">
          {luckyDraw?.my_entry_status === 'ENTERED' ? (
            <button type="button" onClick={onOpenLuckyDraw}>开奖</button>
          ) : luckyDraw?.my_entry_status === 'OPENED' ? (
            <button type="button" onClick={onOpenLeaderboard}>查看榜单</button>
          ) : (
            <button type="button" disabled={!luckyDraw?.can_enter} onClick={onEnterLuckyDraw}>参与福袋</button>
          )}
        </div>
      </div>
      <div className="buyer-pk-card" data-testid="buyer-pk-card">
        <div className="pk-title">
          <span><Flame size={13} /> 买家阵营</span>
          <strong>{auctionStatusLabel(scenario.status)}</strong>
        </div>
        <div className="pk-bars" style={{ '--craft-score': `${craftScore}%`, '--story-score': `${storyScore}%` } as React.CSSProperties}>
          <button type="button" className={activeTeam === 'craft' ? 'active' : ''} onClick={() => onSelectTeam('craft')}>
            <span>工艺派</span><strong>{craftScore}%</strong>
          </button>
          <button type="button" className={activeTeam === 'story' ? 'active' : ''} onClick={() => onSelectTeam('story')}>
            <span>故事派</span><strong>{storyScore}%</strong>
          </button>
        </div>
        <small>进度来自真实出价热度和阵营选择，不影响价格和成交。</small>
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
  terminalPriceCents,
  terminalWinnerID,
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
  terminalPriceCents: number;
  terminalWinnerID: string;
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
  const mediaURL = item.video_poster_url ?? item.videoPosterURL ?? item.image_url ?? item.imageURL ?? '';
  const bidHint = (() => {
    if (scenario.title === '需完成验证') return scenario.feedback;
    if (scenario.ctaDisabled && !scenario.sold && scenario.leader.includes('你')) return h5Copy.selfLeading;
    if (unsafeAction && !scenario.sold) return h5Copy.confirmingPrice;
    if (nextBidCents > minimumNextBidCents) return `高于当前价 ${formatCents(nextBidCents - currentPriceCents)} · 高于最低下一口 ${formatCents(nextBidCents - minimumNextBidCents)}`;
    return `最低有效出价 ${formatCents(minimumNextBidCents)} · 按 ${formatCents(Math.max(0, nextBidCents - currentPriceCents))} 加价`;
  })();

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
          {countdownPhase.phase === 'hammer' && !scenario.stale && !scenario.sold && <strong>{countdownPhase.beat || '落锤窗口'}</strong>}
          {extensionNotice && !scenario.sold && <strong>{extensionNotice}</strong>}
        </div>
      </div>
      <div className="dock-rank-row">
        <span className="status-chip" data-state={scenario.status}>{auctionStatusLabel(scenario.status)}</span>
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
        heat={undefined}
        item={item}
        kind={resultSheetKind}
        nextAuction={nextAuction}
        paymentPhase={paymentPhase}
        scenario={scenario}
        terminalPriceCents={terminalPriceCents || currentPriceCents}
        terminalWinnerID={terminalWinnerID}
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
      <button className="primary-cta" data-testid="bid-cta" disabled={primaryDisabled} onClick={onPrimaryAction}>
        {scenario.winner ? <CreditCard size={18} /> : scenario.rejected ? <AlertTriangle size={18} /> : <CheckCircle2 size={18} />}
        {scenario.cta}
      </button>
      <div className="dock-shortcuts" aria-label="bid-dock-shortcuts">
        <button type="button" onClick={() => onOpenSheet('details')}>拍品与规则</button>
        <button type="button" onClick={() => onOpenSheet('leaderboard')}>出价榜</button>
        <button type="button" onClick={() => onOpenSheet('maxBid')}>自动加价</button>
        <button type="button" onClick={() => onOpenSheet('more')}>更多</button>
      </div>
    </section>
  );
}

export function ResultSheet({
  activeSheet,
  compact = false,
  heat,
  item,
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
  compact?: boolean;
  heat?: HeatSnapshot;
  item: AuctionItem;
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
  const [shareFeedback, setShareFeedback] = useState('');
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
  const recapHeat: HeatSnapshot = heat ?? {
    activeBidders30s: 0,
    acceptedBids30s: 0,
    priceVelocityCentsPerMin: 0,
    acceptedBidderCount: 0,
    totalAcceptedBids: 0,
    source: 'fallback'
  };
  const recap: ResultRecap = buildResultRecap({
    itemTitle: item.title ?? scenario.title,
    kind,
    terminalPriceCents: orderAmountCents || terminalPriceCents,
    terminalWinnerID,
    heat: recapHeat,
    nextTitle: nextAuction?.item?.title
  });
  const copyRecap = async () => {
    if (!recap) return;
    try {
      await copyText(recap.shareCopy);
      setShareFeedback('已复制');
    } catch {
      setShareFeedback('复制失败');
    }
  };
  const downloadHighlight = () => {
    if (!recap) return;
    const card = buildHighlightCard(recap);
    const blob = new Blob([card.content], { type: card.mimeType });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = card.filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 250);
    setShareFeedback('已保存');
  };
  const downloadHighlightVideo = async () => {
    if (!recap) return;
    try {
      const clip = await buildHighlightVideo(recap);
      const url = URL.createObjectURL(clip.blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = clip.filename;
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      setShareFeedback('视频已保存');
    } catch {
      setShareFeedback('当前浏览器不支持视频生成');
    }
  };

  return (
    <section className={`result-sheet ${kind} ${compact ? 'is-compact' : ''}`} data-testid="result-sheet" aria-label={title}>
      {!compact ? <div className="result-cinematic-bg" aria-hidden="true" /> : null}
      {!compact ? <div className="result-confetti" aria-hidden="true"><span /><span /><span /><span /><span /></div> : null}
      <div className="result-sheet-icon" aria-hidden="true">
        {kind === 'winner' ? <Trophy size={22} /> : kind === 'loser' ? <Clock3 size={22} /> : <AlertTriangle size={22} />}
      </div>
      <div className="result-sheet-copy">
        <p className="result-eyebrow">{kind === 'winner' ? '成交结果' : kind === 'loser' ? '输家承接' : '未成交说明'}</p>
        <h2>{title}</h2>
        {kind === 'winner' && (
          <>
            <p>成交价 {soldPrice}。订单 {displayOrderNo(orderID)} 已锁定，支付状态：{paymentPhase === 'paid' ? '已支付' : paymentPhase === 'pending' ? '确认中' : paymentPhase === 'expired' ? '已超时' : '待支付'}。</p>
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
      {recap ? (
        <div className="result-recap-card" data-testid="h5-result-recap-card">
          <span>{recap.status}</span>
          <strong>{recap.title}</strong>
          <div>
            <em>{recap.price}</em>
            <em>{recap.winner}</em>
          </div>
          <p>{recap.facts.length ? recap.facts.join(' · ') : '只展示真实竞拍记录'}</p>
          <small>{recap.nextAction}</small>
          <div className="result-recap-actions">
            <button type="button" aria-label="copy-result-recap" onClick={() => void copyRecap()}>
              <Copy size={14} /> 复制
            </button>
            <button type="button" aria-label="download-highlight-card" onClick={downloadHighlight}>
              <Download size={14} /> 高光卡
            </button>
            <button type="button" aria-label="download-highlight-video" onClick={() => void downloadHighlightVideo()}>
              <Download size={14} /> 短视频
            </button>
            {shareFeedback ? <b>{shareFeedback}</b> : null}
          </div>
        </div>
      ) : null}
      {kind !== 'winner' && nextAuction ? (
        <div className="next-auction-card" data-testid="next-auction-handoff">
          <span>直播间下一件</span>
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
  maxBidFeedback,
  maxBidIntent,
  maxBidPhase,
  minimumNextBidCents,
  nextBidCents,
  orderHistory,
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
  onClose,
  onDecreaseMaxBid,
  onIncreaseMaxBid,
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
  maxBidFeedback: string;
  maxBidIntent: MaxBidIntent | null;
  maxBidPhase: MaxBidPhase;
  minimumNextBidCents: number;
  nextBidCents: number;
  orderHistory: HistoryRow[];
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
  onClose: () => void;
  onDecreaseMaxBid: () => void;
  onIncreaseMaxBid: () => void;
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
  onToggleFollow: () => void;
  onToggleSound: () => void;
}) {
  const titleMap: Record<BottomSheetKey, string> = {
    products: '本场商品',
    details: '商品与规则',
    maxBid: '自动加价',
    leaderboard: '出价榜',
    history: '我的出价',
    orders: '我的订单',
    qa: '拍品问答',
    liveops: '互动福利',
    more: '直播设置'
  };
  useEffect(() => {
    if (!activeSheet) return undefined;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [activeSheet, onClose]);

  if (!activeSheet) return null;
  return (
    <div className="sheet-backdrop" data-testid="bottom-sheet-backdrop" onClick={onClose}>
      <section className="bottom-sheet" data-testid="bottom-sheet" role="dialog" aria-modal="true" aria-label={titleMap[activeSheet]} onClick={(event) => event.stopPropagation()}>
        <div className="sheet-handle" aria-hidden="true" />
        <div className="sheet-header">
          <h2>{titleMap[activeSheet]}</h2>
          <button type="button" aria-label="关闭面板" onClick={onClose}>关闭</button>
        </div>
        <div className="sheet-tabs" role="tablist" aria-label="sheet-tabs">
          {([
            ['products', '本场'],
            ['details', '详情'],
            ['leaderboard', '出价榜'],
            ['maxBid', '自动加价'],
            ['more', '更多']
          ] as Array<[BottomSheetKey, string]>).map(([key, label]) => (
            <button type="button" role="tab" aria-selected={activeSheet === key} key={key} onClick={() => onOpenSheet(key)}>{label}</button>
          ))}
        </div>
        <div className="sheet-content">
          {activeSheet === 'products' && <ProductListSheet auctions={auctions} activeAuctionID={activeAuctionID} scenario={scenario} />}
          {activeSheet === 'details' && <ProductRuleSheet item={item} auction={auctions.find((row) => row.id === activeAuctionID)} scenario={scenario} />}
          {activeSheet === 'maxBid' && (
            <MaxBidSheet
              amountCents={maxBidAmountCents}
              connectionPhase={connectionPhase}
              feedback={maxBidFeedback}
              intent={maxBidIntent}
              minimumNextBidCents={minimumNextBidCents}
              phase={maxBidPhase}
              scenario={scenario}
              onCancel={onCancelMaxBid}
              onDecrease={onDecreaseMaxBid}
              onIncrease={onIncreaseMaxBid}
              onRefresh={onRefreshMaxBid}
              onSubmit={onSubmitMaxBid}
            />
          )}
          {activeSheet === 'leaderboard' && <LeaderboardSheet activeAuctionID={activeAuctionID} leaderboard={leaderboard} nextBidCents={nextBidCents} onRefresh={onRefreshLeaderboard} />}
          {activeSheet === 'history' && (
            <HistorySheet
              title="出价历史"
              empty="暂无出价"
              rows={bidHistory}
              historyError={historyError}
              historyLoading={historyLoading}
              onRefresh={onRefreshHistory}
              getPrimary={(row) => `出价 ${formatCents(Number(row.amount_cents ?? 0))}`}
              getSecondary={(row) => buyerHistoryStatus(row)}
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
              getPrimary={(row) => `订单 ${formatCents(Number(row.amount_cents ?? 0))}`}
              getSecondary={(row) => buyerOrderStatus(String(row.order_status ?? row.status ?? ''))}
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
              onClose={onClose}
              onOpenHistory={() => onOpenSheet('history')}
              onOpenLiveOps={() => onOpenSheet('liveops')}
              onOpenOrders={() => onOpenSheet('orders')}
              onOpenQA={() => onOpenSheet('qa')}
              onToggleFollow={onToggleFollow}
              soundEnabled={soundEnabled}
              onToggleSound={onToggleSound}
            />
          )}
        </div>
      </section>
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

export function MaxBidSheet({
  amountCents,
  connectionPhase,
  feedback,
  intent,
  minimumNextBidCents,
  phase,
  scenario,
  onCancel,
  onDecrease,
  onIncrease,
  onRefresh,
  onSubmit
}: {
  amountCents: number;
  connectionPhase: ConnectionPhase;
  feedback: string;
  intent: MaxBidIntent | null;
  minimumNextBidCents: number;
  phase: MaxBidPhase;
  scenario: Scenario;
  onCancel: () => void;
  onDecrease: () => void;
  onIncrease: () => void;
  onRefresh: () => void;
  onSubmit: () => void;
}) {
  const disabled = isDangerousActionDisabled(scenario, connectionPhase) || phase === 'pending' || phase === 'canceling';
  const active = intent?.status === 'ACTIVE';
  return (
    <div className="max-bid-sheet" data-testid="max-bid-sheet">
      <div className="max-bid-status">
        <span>自动加价上限</span>
        <strong>{active ? `${formatCents(intent.max_amount_cents)} · 仅自己可见` : '未启用'}</strong>
        <em>{feedback}</em>
      </div>
      <div className="max-bid-stepper" aria-label="max-bid-amount">
        <button type="button" aria-label="decrease-max-bid" disabled={disabled || amountCents <= minimumNextBidCents} onClick={onDecrease}>-</button>
        <span>{formatCents(Math.max(amountCents, minimumNextBidCents))}</span>
        <button type="button" aria-label="increase-max-bid" disabled={disabled} onClick={onIncrease}><ChevronUp size={18} /></button>
      </div>
      <div className="max-bid-rules">
        <span>仅当前账号可见，不进入公开榜单或房间消息。</span>
        <span>系统只按当前加价阶梯帮你跟价，不会公开你的最高价。</span>
        <span>网络恢复、提交中或本场结束时会暂停设置和取消。</span>
      </div>
      <div className="max-bid-actions">
        <button type="button" onClick={onSubmit} disabled={disabled || Math.max(amountCents, minimumNextBidCents) < minimumNextBidCents}>
          {phase === 'pending' ? '提交中' : active ? '更新自动加价' : '设置自动加价'}
        </button>
        <button type="button" onClick={onCancel} disabled={disabled || !active}>
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

function LeaderboardRows({ entries, burstMode = false }: { entries: NonNullable<LeaderboardPayload['entries']>; burstMode?: boolean }) {
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
            data-flip-key={key}
            ref={(node) => {
              if (node) rowRefs.current.set(key, node);
              else rowRefs.current.delete(key);
            }}
          >
            <span>{rankBadgeLabel(entry.rank)}</span>
            <strong>{entry.is_current ? '我' : entry.user_masked}</strong>
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
  onClose,
  onOpenHistory,
  onOpenLiveOps,
  onOpenOrders,
  onOpenQA,
  onToggleFollow,
  soundEnabled,
  onToggleSound
}: {
  followed: boolean;
  onClose: () => void;
  onOpenHistory: () => void;
  onOpenLiveOps: () => void;
  onOpenOrders: () => void;
  onOpenQA: () => void;
  onToggleFollow: () => void;
  soundEnabled: boolean;
  onToggleSound: () => void;
}) {
  return (
    <div className="more-sheet" data-testid="more-sheet">
      <div className="sheet-action-row">
        <strong><Info size={16} /> 直播设置</strong>
        <button type="button" onClick={onClose}>关闭</button>
      </div>
      <button type="button" onClick={onToggleFollow}>
        <ShieldCheck size={16} />
        {followed ? '取消关注直播间' : '关注直播间'}
      </button>
      <button type="button" onClick={onToggleSound}>
        {soundEnabled ? <BellOff size={16} /> : <Bell size={16} />}
        {soundEnabled ? '关闭提示音' : '开启提示音'}
      </button>
      <button type="button" onClick={onOpenQA}>
        <MessageCircle size={16} />
        拍品问答
      </button>
      <button type="button" onClick={onOpenLiveOps}>
        <Sparkles size={16} />
        互动福利
      </button>
      <button type="button" onClick={onOpenHistory}>
        <History size={16} />
        我的出价
      </button>
      <button type="button" onClick={onOpenOrders}>
        <CreditCard size={16} />
        我的订单
      </button>
      <p>页面只展示真实竞价数据；异常竞拍由商家端处理。</p>
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
          getSecondary={(row) => buyerHistoryStatus(row)}
        />
        <HistoryList
          title="订单"
          empty="暂无订单"
          rows={orderHistory}
          getPrimary={(row) => `订单 ${formatCents(Number(row.amount_cents ?? 0))}`}
          getSecondary={(row) => buyerOrderStatus(String(row.order_status ?? row.status ?? ''))}
        />
      </div>
    </section>
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
            <strong>助手</strong>
            <span>{message.body}</span>
          </div>
        ))}
        {chatMessages.length === 0 && systemMessages.length === 0 ? <p>暂无弹幕</p> : chatMessages.map((message) => (
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
        <span>只回答拍品和规则已提供的信息</span>
        <button type="button" disabled={!draft.trim() || loading} onClick={onAsk}>{loading ? '查询中' : '提问'}</button>
      </div>
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
                {turn.facts_used.length ? <em>来自已展示信息</em> : <em>未找到已提供信息</em>}
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

async function copyText(value: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch {
      // Fall through to the legacy selection path for browsers without clipboard permission.
    }
  }
  const input = document.createElement('textarea');
  input.value = value;
  input.setAttribute('readonly', 'true');
  input.style.position = 'fixed';
  input.style.left = '-9999px';
  document.body.appendChild(input);
  input.select();
  const ok = document.execCommand('copy');
  input.remove();
  if (!ok) throw new Error('copy failed');
}

async function buildHighlightVideo(recap: ResultRecap): Promise<{ blob: Blob; filename: string }> {
  const canvas = document.createElement('canvas');
  canvas.width = 720;
  canvas.height = 1280;
  const ctx = canvas.getContext('2d');
  const capture = canvas.captureStream?.bind(canvas);
  if (!ctx || !capture || typeof MediaRecorder === 'undefined') {
    throw new Error('highlight video unsupported');
  }
  const mimeType = MediaRecorder.isTypeSupported('video/webm;codecs=vp9')
    ? 'video/webm;codecs=vp9'
    : MediaRecorder.isTypeSupported('video/webm;codecs=vp8')
      ? 'video/webm;codecs=vp8'
      : 'video/webm';
  const stream = capture(30);
  const chunks: BlobPart[] = [];
  const recorder = new MediaRecorder(stream, { mimeType });
  const done = new Promise<Blob>((resolve, reject) => {
    recorder.ondataavailable = (event) => {
      if (event.data.size > 0) chunks.push(event.data);
    };
    recorder.onerror = () => reject(new Error('record highlight video'));
    recorder.onstop = () => resolve(new Blob(chunks, { type: mimeType }));
  });
  recorder.start();
  const startedAt = performance.now();
  const durationMS = 4200;
  await new Promise<void>((resolve) => {
    const drawFrame = (now: number) => {
      const progress = Math.min(1, (now - startedAt) / durationMS);
      drawHighlightFrame(ctx, recap, progress);
      if (progress < 1) {
        requestAnimationFrame(drawFrame);
      } else {
        resolve();
      }
    };
    requestAnimationFrame(drawFrame);
  });
  recorder.stop();
  stream.getTracks().forEach((track) => track.stop());
  const filenameTitle = recap.title
    .replace(/[^\p{L}\p{N}]+/gu, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 28) || 'auction-highlight';
  return { blob: await done, filename: `${filenameTitle}-highlight.webm` };
}

function drawHighlightFrame(ctx: CanvasRenderingContext2D, recap: ResultRecap, progress: number) {
  const width = ctx.canvas.width;
  const height = ctx.canvas.height;
  const gradient = ctx.createLinearGradient(0, 0, width, height);
  gradient.addColorStop(0, '#101827');
  gradient.addColorStop(0.44, '#14433b');
  gradient.addColorStop(1, '#d18a2f');
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, width, height);
  ctx.save();
  ctx.globalAlpha = 0.18;
  ctx.fillStyle = '#ffffff';
  for (let index = 0; index < 18; index += 1) {
    const x = (index * 97 + progress * 180) % (width + 120) - 60;
    const y = (index * 139 + progress * 260) % (height + 120) - 60;
    ctx.beginPath();
    ctx.arc(x, y, index % 3 === 0 ? 18 : 10, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.restore();
  roundedRect(ctx, 48, 70, width - 96, height - 140, 30, 'rgba(255,255,255,0.13)', 'rgba(255,255,255,0.28)');
  ctx.fillStyle = '#ffe6a7';
  ctx.font = '700 34px Arial, sans-serif';
  ctx.fillText(recap.status, 82, 150);
  ctx.fillStyle = '#ffffff';
  ctx.font = '800 50px Arial, sans-serif';
  wrapCanvasText(ctx, recap.title, 82, 250, width - 164, 58, 2);
  const scale = 1 + Math.sin(progress * Math.PI) * 0.035;
  ctx.save();
  ctx.translate(82, 440);
  ctx.scale(scale, scale);
  ctx.fillStyle = '#ffffff';
  ctx.font = '900 78px Arial, sans-serif';
  ctx.fillText(recap.price, 0, 0);
  ctx.restore();
  ctx.fillStyle = '#fff0c7';
  ctx.font = '700 30px Arial, sans-serif';
  ctx.fillText(`成交/领先：${recap.winner}`, 82, 520);
  roundedRect(ctx, 82, 625, width - 164, 210, 22, 'rgba(255,255,255,0.15)');
  ctx.fillStyle = '#ffffff';
  ctx.font = '800 32px Arial, sans-serif';
  ctx.fillText('高光事实', 112, 690);
  ctx.fillStyle = '#fff5db';
  ctx.font = '400 28px Arial, sans-serif';
  wrapCanvasText(ctx, recap.facts.join(' · ') || '真实竞拍记录', 112, 755, width - 224, 38, 2);
  ctx.fillStyle = '#ffffff';
  ctx.font = '800 36px Arial, sans-serif';
  wrapCanvasText(ctx, recap.nextAction, 82, 975, width - 164, 44, 2);
  ctx.fillStyle = 'rgba(255,255,255,0.75)';
  ctx.font = '400 24px Arial, sans-serif';
  ctx.fillText('仅展示系统真实竞拍记录，用户身份已脱敏。', 82, 1158);
}

function roundedRect(ctx: CanvasRenderingContext2D, x: number, y: number, width: number, height: number, radius: number, fill: string, stroke?: string) {
  ctx.beginPath();
  ctx.moveTo(x + radius, y);
  ctx.arcTo(x + width, y, x + width, y + height, radius);
  ctx.arcTo(x + width, y + height, x, y + height, radius);
  ctx.arcTo(x, y + height, x, y, radius);
  ctx.arcTo(x, y, x + width, y, radius);
  ctx.closePath();
  ctx.fillStyle = fill;
  ctx.fill();
  if (stroke) {
    ctx.strokeStyle = stroke;
    ctx.lineWidth = 2;
    ctx.stroke();
  }
}

function wrapCanvasText(ctx: CanvasRenderingContext2D, text: string, x: number, y: number, maxWidth: number, lineHeight: number, maxLines: number) {
  const chars = Array.from(text);
  let line = '';
  let lineCount = 0;
  for (const char of chars) {
    const next = line + char;
    if (ctx.measureText(next).width > maxWidth && line) {
      lineCount += 1;
      ctx.fillText(lineCount === maxLines ? `${line.slice(0, Math.max(0, line.length - 1))}...` : line, x, y);
      if (lineCount >= maxLines) return;
      y += lineHeight;
      line = char;
    } else {
      line = next;
    }
  }
  if (line && lineCount < maxLines) ctx.fillText(line, x, y);
}

export function HistoryList({
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
