export type Room = {
  id: string;
  host_id: string;
  status: string;
  role: string;
};

export type Item = {
  id: string;
  title: string;
  image_url?: string;
  description?: string;
};

export function displayMediaURL(url?: string) {
  const value = (url ?? '').trim();
  if (!value) return '';
  try {
    const parsed = new URL(value, window.location.origin);
    const match = parsed.pathname.match(/\/live-auction-items\/(items\/.+)$/);
    if ((parsed.hostname === 'localhost' || parsed.hostname === '127.0.0.1') && parsed.port === '9000' && match) {
      return `/api/media/${match[1]}`;
    }
  } catch {
    return value;
  }
  return value;
}

export type RuleDraft = {
  startPriceCents: number;
  incrementCents: number;
  capPriceCents: number;
  durationSeconds: number;
  extendWindowSeconds: number;
  extendBySeconds: number;
  maxExtendCount: number;
  fatFingerThresholdCents: number;
  depositBPS: number;
  depositFloorCents: number;
  depositCapCents: number;
};

export function auctionStatusLabel(status?: string) {
  switch (status) {
    case 'DRAFT':
      return '待完善';
    case 'SCHEDULED':
      return '已排期';
    case 'ACTIVE':
      return '开拍中';
    case 'SOLD':
      return '已成交';
    case 'ENDED':
      return '已结束';
    case 'CANCELLED':
      return '已取消';
    default:
      return status || '未知';
  }
}

export function orderStatusLabel(status?: string) {
  switch (status) {
    case 'ORDER_PENDING':
      return '待支付';
    case 'PAYMENT_INITIATED':
      return '支付中';
    case 'PAID':
      return '已支付';
    case 'ORDER_EXPIRED':
      return '已超时';
    case 'FAILED':
      return '支付失败';
    default:
      return status || '未知';
  }
}

export type Auction = {
  id: string;
  room_id: string;
  item_id: string;
  status: string;
  is_narrating: boolean;
  current_price_cents: number;
  current_winner_id?: string;
  start_price_cents: number;
  increment_cents: number;
  cap_price_cents?: number;
  start_at?: string;
  end_at?: string;
  seq: number;
  accepted_bid_count: number;
  extend_count: number;
  item: Item;
  rule: {
    duration_seconds: number;
    extend_window_seconds: number;
    extend_by_seconds: number;
    max_extend_count: number;
    fat_finger_threshold_cents?: number;
    deposit_bps: number;
    deposit_floor_cents: number;
    deposit_cap_cents: number;
    frozen_at?: string;
  };
};

export type Order = {
  id: string;
  auction_id: string;
  winner_id: string;
  amount_cents: number;
  status: string;
  deposit_cents?: number;
  deposit_status: string;
  expire_at?: string;
  paid_at?: string;
  provider_payment_id?: string;
};

export type MonitorPayload = {
  items: Array<Record<string, unknown>>;
};

export type RedisEngineMonitorPayload = MonitorPayload & {
  summary?: {
    active_rows: number;
    paused_auctions: number;
    pending_redis_decisions: number;
    pending_settlements: number;
    failed_settlements: number;
    append_success_count: number;
    append_failure_count: number;
    append_unknown_count: number;
    settlement_lag_max_ms: number;
    last_recovery_rto_ms?: number;
    last_recovery_status?: string;
    latest_append?: Record<string, unknown>;
  };
};

export type RedisEngineSummary = {
  active_rows: number;
  paused_auctions: number;
  pending_redis_decisions: number;
  pending_settlements: number;
  failed_settlements: number;
  append_success_count: number;
  append_failure_count: number;
  append_unknown_count: number;
  settlement_lag_max_ms: number;
  last_recovery_rto_ms?: number;
  last_recovery_status?: string;
  latest_append?: Record<string, unknown>;
};

export type FlightRecorderPayload = {
  summary?: FlightRecorderSummary;
  rules?: Array<Record<string, unknown>>;
  orders?: Array<Record<string, unknown>>;
  payment_events?: Array<Record<string, unknown>>;
  anomalies?: Array<Record<string, unknown>>;
  timeline?: FlightRecorderTimelineRow[];
};

export type FlightRecorderSummary = {
  auction_id: string;
  room_id: string;
  item_id: string;
  item_title: string;
  status: string;
  current_price_cents: number;
  current_winner_id?: string;
  seq: number;
  accepted_bid_count: number;
  extend_count: number;
};

export type FlightRecorderTimelineRow = {
  time: string;
  kind: string;
  auction_id: string;
  seq?: number;
  event_type: string;
  ref_id: string;
  user_id?: string;
  amount_cents?: number;
  status?: string;
  trace_id?: string;
  payload?: Record<string, unknown>;
};

export type HostPrompt = {
  id: string;
  type: string;
  severity: string;
  title: string;
  body: string;
  action: string;
  source: string;
  auction_id: string;
  room_id: string;
  event_seq?: number;
  generated_at: string;
  window_seconds: number;
  metric_value?: number;
  metric_label?: string;
  reference_price_cents?: number;
  expires_at?: string;
};

export type HostPromptsPayload = {
  prompts?: HostPrompt[];
};

export type HeatSummary = {
  auction_id: string;
  room_id: string;
  status: string;
  generated_at: string;
  window_seconds: number;
  active_bidders_30s: number;
  accepted_bids_30s: number;
  rejected_bids_30s: number;
  chat_messages_30s: number;
  recovery_events_30s: number;
  watcher_count_available: boolean;
  watcher_count?: number;
  bid_response_p95_ms: number;
  ledger_settle_p95_ms: number;
  ledger_settle_max_ms: number;
  source: string;
};

export type LiveOpsRewardConfig = {
  enabled: boolean;
  title: string;
  description: string;
  reward_name: string;
  reward_quota: number;
  required_task_count: number;
};

export type LiveOpsHostSummary = {
  campaign: {
    id: string;
    room_id: string;
    status: string;
    title: string;
    description: string;
    progress: number;
    disclaimer: string;
    updated_at: string;
  };
  reward_config: LiveOpsRewardConfig;
  participant_count: number;
  qualified_count: number;
  opened_count: number;
  preference_summary: Array<{ key: string; label: string; count: number }>;
  recent_rewards: Array<{
    user_id: string;
    user_masked: string;
    status: string;
    reward_label?: string;
    entered_at: string;
    opened_at?: string;
  }>;
};

export type MaxBidSummary = {
  auction_id: string;
  room_id: string;
  status: string;
  generated_at: string;
  active_intent_count: number;
  pre_bid_count: number;
  max_bid_count: number;
  applied_intent_count: number;
  exhausted_count: number;
  cancelled_count: number;
  has_private_pressure: boolean;
  source: string;
};

export type SignalRequest = {
  signal_type: string;
  target_type: string;
  target_id: string;
  reason: string;
  payload_json?: Record<string, unknown>;
};

export type ListingDraftJob = {
  id: string;
  room_id: string;
  kind: string;
  status: string;
  provider: string;
  model: string;
  output_json: {
    title_candidates?: string[];
    description?: string;
    category?: string;
    selling_points?: string[];
    condition_questions?: string[];
    compliance_flags?: string[];
    requires_evidence?: string[];
    unsupported_claims?: string[];
    confidence?: number;
    rationale?: string;
    human_review_required?: boolean;
    rule_suggestion?: {
      start_price_cents?: number;
      increment_cents?: number;
      cap_price_cents?: number;
      duration_seconds?: number;
      extend_window_seconds?: number;
      extend_by_seconds?: number;
      max_extend_count?: number;
      fat_finger_threshold_cents?: number;
    };
  };
  safety_json?: Record<string, unknown>;
  error_message?: string;
  created_at: string;
  applied_at?: string;
};

export type AuctionAISettings = {
  auction_id: string;
  auto_commentary_enabled: boolean;
  updated_by?: string;
  updated_at: string;
};

export type SystemMessage = {
  id: number;
  room_id: string;
  auction_id?: string;
  source: string;
  source_seq?: number;
  style: string;
  body: string;
  facts_json?: Record<string, unknown>;
  safety_json?: Record<string, unknown>;
  created_at: string;
};

export type SentinelAlert = {
  id: number;
  room_id: string;
  auction_id: string;
  severity: string;
  risk_type: string;
  score: number;
  explanation: string;
  recommended_action: string;
  features_json?: Record<string, unknown>;
  status: string;
  created_at: string;
};

export type AuctionRecap = {
  auction_id: string;
  room_id: string;
  item_title: string;
  status: string;
  final_price_cents: number;
  winner_masked?: string;
  accepted_bids: number;
  accepted_bidders: number;
  extend_count: number;
  highlights: string[];
  next_actions: string[];
  rule_suggestion?: {
    start_price_cents: number;
    increment_cents: number;
    cap_price_cents: number;
    basis: string;
    source: string;
    human_review_required: boolean;
  };
  share_card?: Record<string, unknown>;
  generated_at: string;
  highlight_asset?: HighlightAsset;
};

export type HighlightAsset = {
  id: string;
  auction_id: string;
  room_id: string;
  job_id: string;
  status: string;
  media_type: string;
  title: string;
  asset_url: string;
  render_profile: string;
  duration_ms: number;
  facts_json?: Record<string, unknown>;
  risk_json?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type RuleAPIError = {
  code?: string;
  message?: string;
  details?: {
    suggested_caps?: number[];
  };
};
export type AuthUser = {
  ID: string;
  Role: string;
};

export const defaultRoomID = 'room_main';

export function formatCents(cents?: number) {
  return `¥${((cents ?? 0) / 100).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

export function maskUser(userID?: string) {
  if (!userID) return '-';
  if (/^[\u4e00-\u9fa5].*\*\*$/.test(userID)) return userID;
  if (/^user[_-]?\w+/i.test(userID)) return '匿名买家';
  return `${userID.slice(0, 2)}**`;
}

export function formatRemaining(endAt?: string, now = Date.now()) {
  if (!endAt) return '-';
  const endAtMS = Date.parse(endAt);
  if (!Number.isFinite(endAtMS)) return '-';
  const totalSeconds = Math.max(0, Math.ceil((endAtMS - now) / 1000));
  if (totalSeconds >= 86400) {
    const days = Math.floor(totalSeconds / 86400);
    if (days > 99) return '>99d';
    const hours = Math.floor((totalSeconds % 86400) / 3600);
    return `${days}d ${hours}h`;
  }
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

export function statusTagColor(status: string) {
  if (status === 'ACTIVE') return 'green';
  if (status === 'SCHEDULED') return 'arcoblue';
  if (status === 'DRAFT') return 'gray';
  if (status === 'SOLD') return 'orangered';
  if (status === 'CANCELLED' || status === 'ENDED') return 'red';
  return 'arcoblue';
}

export function terminalStatus(status: string) {
  return ['SOLD', 'ENDED', 'CANCELLED'].includes(status);
}

export function queuePriority(status: string) {
  if (status === 'ACTIVE') return 0;
  if (status === 'SCHEDULED') return 1;
  if (status === 'DRAFT') return 2;
  return 3;
}

export function sortedAuctions(auctions: Auction[]) {
  return [...auctions].sort((left, right) => {
    const priority = queuePriority(left.status) - queuePriority(right.status);
    if (priority !== 0) return priority;
    return (left.start_at ?? left.end_at ?? left.id).localeCompare(right.start_at ?? right.end_at ?? right.id);
  });
}

export function queueGroups(auctions: Auction[]) {
  const sorted = sortedAuctions(auctions);
  const finished = sorted.filter((auction) => !['ACTIVE', 'SCHEDULED', 'DRAFT'].includes(auction.status));
  const visibleFinished = finished.slice(0, 4);
  const referenceFinished = finished.find((auction) => auction.id === 'auc_live');
  const finishedRows = referenceFinished && !visibleFinished.some((auction) => auction.id === referenceFinished.id)
    ? [...visibleFinished.slice(0, 3), referenceFinished]
    : visibleFinished;
  return {
    active: sorted.filter((auction) => auction.status === 'ACTIVE'),
    scheduled: sorted.filter((auction) => auction.status === 'SCHEDULED'),
    draft: sorted.filter((auction) => auction.status === 'DRAFT'),
    finished: finishedRows,
    finishedTotal: finished.length
  };
}

export function activeAuction(auctions: Auction[]) {
  return auctions.find((auction) => auction.status === 'ACTIVE');
}

export function narratingAuction(auctions: Auction[]) {
  return auctions.find((auction) => auction.is_narrating);
}

export function monitorCount(payload?: MonitorPayload) {
  return payload?.items?.length ?? 0;
}

export function monitorItems(payload?: MonitorPayload) {
  return payload?.items ?? [];
}

export function riskQueue(monitor: Record<string, MonitorPayload>, selectedAuction: Auction) {
  const risks: Array<{ level: 'high' | 'med' | 'low'; title: string; body: string; source: string }> = [];
  const auctionScoped = (row: Record<string, unknown>) => {
    const auctionID = String(row.auction_id ?? row.aggregate_id ?? row.target_id ?? '');
    const roomID = String(row.room_id ?? '');
    return !auctionID || auctionID === selectedAuction.id || roomID === selectedAuction.room_id;
  };
  const anomalies = monitorItems(monitor.anomalies).filter(auctionScoped);
  const highAnomaly = anomalies.find((row) => ['CRITICAL', 'HIGH'].includes(String(row.severity ?? '').toUpperCase())) ?? anomalies[0];
  if (highAnomaly) {
    risks.push({
      level: String(highAnomaly.severity ?? '').toUpperCase() === 'CRITICAL' ? 'high' : 'med',
      title: '系统异常待确认',
      body: String(highAnomaly.message ?? '先查看运行监控和事件回放，再继续强推成交。'),
      source: '运行监控'
    });
  }
  const rejects = monitorItems(monitor.rejects).filter(auctionScoped);
  if (rejects.length > 0) {
    const hot = rejects.some((row) => ['BID_AUCTION_TOO_HOT', 'RATE_LIMITED', 'PROCESSING_RETRY_LATER'].includes(String(row.reject_reason ?? row.code ?? '')));
    risks.push({
      level: hot ? 'high' : 'med',
      title: hot ? '出价过于密集' : '有无效出价',
      body: `${rejects.length} 条无效出价记录；先确认买家提示和规则说明，再继续引导出价。`,
      source: '出价记录'
    });
  }
  const recoveryRows = monitorItems(monitor.recovery).filter(auctionScoped);
  const recoveryTotal = recoveryRows.reduce((sum, row) => (
    sum
    + Number(row.reconnect_count_recent ?? 0)
    + Number(row.snapshot_stale ?? 0)
    + Number(row.slow_consumer_disconnects ?? 0)
  ), 0);
  if (recoveryTotal > 0) {
    risks.push({
      level: recoveryRows.some((row) => Number(row.snapshot_stale ?? 0) > 0 || Number(row.slow_consumer_disconnects ?? 0) > 0) ? 'high' : 'low',
      title: '恢复记录',
      body: `${recoveryTotal} 条买家端重连或卡顿信号；确认直播间状态稳定后再催促出价。`,
      source: '连接记录'
    });
  }
  return risks.slice(0, 3);
}

export function connectionLabel(monitor: Record<string, MonitorPayload>, roomID: string) {
  const row = monitor.recovery?.items?.find((item) => String(item.room_id ?? '') === roomID) ?? monitor.recovery?.items?.[0];
  if (!row) return '恢复数据未上报';
  const reconnects = Number(row.reconnect_count_recent ?? 0);
  const stale = Number(row.snapshot_stale ?? 0);
  const slow = Number(row.slow_consumer_disconnects ?? 0);
  if (slow > 0) return `慢客户端断开 ${slow}`;
  if (stale > 0) return `状态快照待刷新 ${stale}`;
  if (reconnects > 0) return `近期重连 ${reconnects}`;
  return '恢复链路正常';
}

export function redisEngineSummary(payload?: MonitorPayload, auction?: Auction): RedisEngineSummary {
  const rows = (payload?.items ?? []).filter((row) => {
    if (!auction) return true;
    return String(row.auction_id ?? '') === auction.id;
  });
  const initial: RedisEngineSummary = {
    active_rows: 0,
    paused_auctions: 0,
    pending_redis_decisions: 0,
    pending_settlements: 0,
    failed_settlements: 0,
    append_success_count: 0,
    append_failure_count: 0,
    append_unknown_count: 0,
    settlement_lag_max_ms: 0
  };
  return rows.reduce<RedisEngineSummary>((acc, row) => {
    acc.active_rows += 1;
    if (row.engine_paused) acc.paused_auctions += 1;
    acc.pending_redis_decisions += Number(row.redis_pending_decisions ?? 0);
    acc.pending_settlements += Number(row.pending_settlements ?? 0);
    acc.failed_settlements += Number(row.failed_settlements ?? 0);
    acc.append_success_count += Number(row.append_success_count ?? 0);
    acc.append_failure_count += Number(row.append_failure_count ?? 0);
    acc.append_unknown_count += Number(row.append_unknown_count ?? 0);
    acc.settlement_lag_max_ms = Math.max(acc.settlement_lag_max_ms, Number(row.settlement_lag_max_ms ?? 0));
    const rto = Number(row.last_recovery_rto_ms ?? 0);
    if (rto > 0 && rto >= Number(acc.last_recovery_rto_ms ?? 0)) {
      acc.last_recovery_rto_ms = rto;
      acc.last_recovery_status = String(row.last_recovery_status ?? '');
    }
    const appendSeq = Number(row.latest_append_engine_seq ?? 0);
    const previousSeq = Number(acc.latest_append?.latest_append_engine_seq ?? 0);
    if (appendSeq >= previousSeq && row.latest_append_status) {
      acc.latest_append = row;
    }
    return acc;
  }, initial);
}

export function signalTargetID(row: Record<string, unknown>) {
  return String(row.target_id ?? '');
}

export function signalType(row: Record<string, unknown>) {
  return String(row.signal_type ?? '');
}

export function signalCreatedAt(row: Record<string, unknown>) {
  const value = Date.parse(String(row.created_at ?? ''));
  return Number.isFinite(value) ? value : 0;
}

export function anomalyKey(row: Record<string, unknown>) {
  return `alert:${String(row.id ?? row.type ?? row.created_at ?? '')}`;
}

export function anomalySeverity(row: Record<string, unknown>) {
  return String(row.severity ?? 'LOW').toUpperCase();
}

export function severityTagColor(severity: string) {
  if (severity === 'CRITICAL') return 'red';
  if (severity === 'HIGH') return 'orangered';
  if (severity === 'MED' || severity === 'WARNING') return 'orange';
  return 'green';
}

export function isMutedAlert(row: Record<string, unknown>, signals: Record<string, MonitorPayload>, now = Date.now()) {
  const key = anomalyKey(row);
  return monitorItems(signals.signals)
    .filter((signal) => signalType(signal) === 'mute_alert_10m' && signalTargetID(signal) === key)
    .some((signal) => now - signalCreatedAt(signal) < 10 * 60_000);
}

export function isAckedAlert(row: Record<string, unknown>, signals: Record<string, MonitorPayload>) {
  const key = anomalyKey(row);
  return monitorItems(signals.signals).some((signal) => signalType(signal) === 'ack_alert' && signalTargetID(signal) === key);
}

export function visibleAnomalies(monitor: Record<string, MonitorPayload>, now = Date.now()) {
  return monitorItems(monitor.anomalies)
    .filter((row) => !isMutedAlert(row, monitor, now))
    .sort((left, right) => {
      const severityOrder: Record<string, number> = { CRITICAL: 0, HIGH: 1, MED: 2, WARNING: 2, LOW: 3 };
      const bySeverity = (severityOrder[anomalySeverity(left)] ?? 4) - (severityOrder[anomalySeverity(right)] ?? 4);
      if (bySeverity !== 0) return bySeverity;
      return Date.parse(String(right.created_at ?? '')) - Date.parse(String(left.created_at ?? ''));
    });
}

export function auctionScopedRows(payload: MonitorPayload | undefined, auction?: Auction) {
  if (!auction) return monitorItems(payload);
  return monitorItems(payload).filter((row) => {
    const auctionID = String(row.auction_id ?? row.aggregate_id ?? row.target_id ?? '');
    const roomID = String(row.room_id ?? '');
    return !auctionID || auctionID === auction.id || roomID === auction.room_id;
  });
}

export function liveHealthSummary(auctions: Auction[], orders: Order[], monitor: Record<string, MonitorPayload>, heatSummary: HeatSummary | undefined, selectedAuction: Auction | undefined, now: number) {
  const active = activeAuction(auctions) ?? selectedAuction;
  const engine = redisEngineSummary(monitor.redisEngine, active);
  const anomalies = visibleAnomalies(monitor, now);
  const scopedRejects = auctionScopedRows(monitor.rejects, active);
  const scopedRecovery = auctionScopedRows(monitor.recovery, active);
  const activeOrders = active ? orders.filter((order) => order.auction_id === active.id) : orders;
  const criticalAnomalies = anomalies.filter((row) => ['CRITICAL', 'HIGH'].includes(anomalySeverity(row)));
  const recoveryPressure = scopedRecovery.reduce((sum, row) => sum + Number(row.reconnect_count_recent ?? 0) + Number(row.snapshot_stale ?? 0) + Number(row.slow_consumer_disconnects ?? 0), 0);
  const ledgerMaxMS = heatSummary?.ledger_settle_max_ms ?? engine.settlement_lag_max_ms;
  const moneyRisk = engine.failed_settlements > 0 || ledgerMaxMS > 5000 || activeOrders.some((order) => ['FAILED', 'EXPIRED'].includes(order.status));
  const buyerRisk = recoveryPressure > 0 || scopedRejects.some((row) => ['BID_AUCTION_TOO_HOT', 'RATE_LIMITED', 'ENGINE_PAUSED', 'RECONCILING'].includes(String(row.reject_reason ?? row.code ?? '')));
  const systemRisk = engine.paused_auctions > 0 || engine.pending_redis_decisions > 0 || engine.append_failure_count > 0 || engine.append_unknown_count > 0;
  const overall = criticalAnomalies.length > 0 || moneyRisk || systemRisk
    ? 'critical'
    : buyerRisk || anomalies.length > 0 || engine.pending_settlements > 0
      ? 'degraded'
      : 'healthy';
  const accepted = heatSummary?.accepted_bids_30s ?? active?.accepted_bid_count ?? 0;
  const rejected = heatSummary?.rejected_bids_30s ?? scopedRejects.length;
  const activeBidders = heatSummary?.active_bidders_30s ?? 0;
  const watchers = heatSummary?.watcher_count_available ? (heatSummary.watcher_count ?? 0) : undefined;
  return {
    active,
    engine,
    anomalies,
    criticalAnomalies,
    recoveryPressure,
    scopedRejects,
    scopedRecovery,
    activeOrders,
    moneyRisk,
    buyerRisk,
    systemRisk,
    overall,
    funnel: {
      watchers,
      activeBidders,
      accepted,
      rejected,
      orders: activeOrders.length,
      paid: activeOrders.filter((order) => order.status === 'PAID').length
    }
  };
}

export function overallCopy(overall: string) {
  if (overall === 'critical') return { title: '需先处理', body: '暂停催拍，先确认成交记录、买家状态和告警原因。', color: 'red' };
  if (overall === 'degraded') return { title: '需关注', body: '直播可继续，但需要留意买家体验或后台记录积压。', color: 'orange' };
  return { title: '正常', body: '竞拍、买家体验和成交记录当前无阻断信号。', color: 'green' };
}

export function signalCopy(signalTypeValue: string) {
  switch (signalTypeValue) {
  case 'reconcile_redis_engine':
    return '校对成交状态';
  case 'force_snapshot_rebuild':
    return '重建买家状态';
  case 'pause_redis_engine':
    return '暂停出价确认';
  case 'ack_alert':
    return '确认告警';
  case 'mute_alert_10m':
    return '静默告警 10 分钟';
  default:
    return signalTypeValue;
  }
}

export function rowSourceURL(sourceKey: string, record: Record<string, unknown>) {
  const auctionID = String(record.auction_id ?? record.aggregate_id ?? record.target_id ?? '');
  if (auctionID && (sourceKey === 'auction_id' || sourceKey === 'trace_id' || sourceKey === 'outbox_id' || sourceKey === 'job_id' || sourceKey === 'request_id' || sourceKey === 'id')) {
    return `/api/monitor/auctions/${encodeURIComponent(auctionID)}/flight-recorder?limit=50&timeline_limit=100`;
  }
  return '';
}

export function rowAuctionID(record: Record<string, unknown>) {
  return String(record.auction_id ?? record.aggregate_id ?? record.target_id ?? '');
}

export function timelineTone(row: FlightRecorderTimelineRow) {
  const text = `${row.kind} ${row.event_type} ${row.status ?? ''}`.toLowerCase();
  if (text.includes('anomaly') || text.includes('dead') || text.includes('failed') || text.includes('rejected')) return 'red';
  if (text.includes('sold') || text.includes('paid') || text.includes('published') || text.includes('accepted')) return 'green';
  if (text.includes('snapshot') || text.includes('recover')) return 'arcoblue';
  return 'gray';
}

export function timelineImpact(row: FlightRecorderTimelineRow) {
  if (row.kind === 'bid' && row.payload?.source === 'AUTO_MAX_BID') return '自动加价已真实写入一条出价记录，买家的封顶价没有对外暴露。';
  if (row.kind === 'auction_event') {
    if (row.event_type === 'auction_sold') return '成交结果已经落库，下面应能追到中标订单。';
    if (row.event_type === 'bid_rejected') return '这次出价被规则拒绝，当前价和领先人不变。';
    if (row.event_type === 'bid_accepted') return '有效出价已经推进竞拍状态和当前价。';
    if (row.event_type === 'auction_extended') return '最后窗口触发延时，买家端应以这次更新后的结束时间为准。';
    return '这条竞拍事件改变了买家端看到的状态。';
  }
  if (row.kind === 'bid') return row.status === 'ACCEPTED' ? '出价记录确认这次加价已经被系统接受。' : '被拒绝的出价也会保留，便于解释为什么没有改变价格。';
  if (row.kind === 'outbox') return row.status === 'PUBLISHED' ? '直播间状态已推送给买家端。' : '买家端状态推送仍在等待重试。';
  if (row.kind === 'order') return '订单和支付状态来自成交结果。';
  if (row.kind === 'payment_event') return '支付回调已记录，可用于对账。';
  if (row.kind === 'snapshot_rebuild') return '买家端状态已按成交记录重新同步。';
  if (row.kind === 'anomaly') return '这条异常需要主播或运维确认后再继续演示。';
  return '这条记录来自后端事件回放。';
}

export function timelineNextAction(row: FlightRecorderTimelineRow) {
  if (row.kind === 'bid' && row.payload?.source === 'AUTO_MAX_BID') return '确认买家端只展示自动加价结果，不展示封顶金额。';
  if (row.kind === 'outbox' && row.status !== 'PUBLISHED') return '查看买家端是否已收到更新，必要时重建买家状态。';
  if (row.kind === 'anomaly') return '按这场竞拍筛选异常，记录原因后再继续。';
  if (row.kind === 'snapshot_rebuild' && row.status !== 'COMPLETED') return '检查买家端恢复情况，确认是否仍停留在重连中。';
  if (row.kind === 'payment_event') return '先核对订单状态和支付编号，再说明支付结果。';
  if (row.kind === 'order') return '核对中标人、金额、保证金和支付截止时间。';
  if (row.kind === 'bid' && row.status === 'REJECTED') return '用拒绝原因解释买家端提示和按钮状态。';
  if (row.trace_id) return `需要深查时，用排查编号 ${row.trace_id} 查后台日志。`;
  return '状态符合预期时无需处理。';
}

export function createRuleDraft(auction?: Auction): RuleDraft {
  return {
    startPriceCents: auction?.start_price_cents ?? 10_000,
    incrementCents: auction?.increment_cents ?? 5_000,
    capPriceCents: auction?.cap_price_cents ?? 60_000,
    durationSeconds: auction?.rule.duration_seconds ?? 600,
    extendWindowSeconds: auction?.rule.extend_window_seconds ?? 10,
    extendBySeconds: auction?.rule.extend_by_seconds ?? 10,
    maxExtendCount: auction?.rule.max_extend_count ?? 3,
    fatFingerThresholdCents: auction?.rule.fat_finger_threshold_cents ?? 100_000,
    depositBPS: auction?.rule.deposit_bps ?? 1000,
    depositFloorCents: auction?.rule.deposit_floor_cents ?? 5_000,
    depositCapCents: auction?.rule.deposit_cap_cents ?? 50_000
  };
}

export function nearestLegalCaps(rule: RuleDraft) {
  const minCap = rule.startPriceCents + rule.incrementCents;
  if (rule.incrementCents <= 0 || rule.capPriceCents < minCap) {
    return [minCap, minCap + rule.incrementCents].filter((value, index, values) => value > 0 && values.indexOf(value) === index);
  }
  const steps = Math.floor((rule.capPriceCents - rule.startPriceCents) / rule.incrementCents);
  const lower = rule.startPriceCents + steps * rule.incrementCents;
  const upper = lower + rule.incrementCents;
  return [lower, upper].filter((value, index, values) => value >= minCap && values.indexOf(value) === index);
}

export function validateRule(rule: RuleDraft) {
  if (rule.startPriceCents < 0) return { valid: false, field: 'start', message: '起拍价不能为负', suggestions: [] };
  if (rule.incrementCents <= 0) return { valid: false, field: 'increment', message: '加价幅度必须大于 0', suggestions: [] };
  if (rule.durationSeconds < 30 || rule.durationSeconds > 86400) return { valid: false, field: 'duration', message: '竞拍时长必须在 30 秒到 86400 秒之间', suggestions: [] };
  if (rule.extendWindowSeconds < 10 || rule.extendWindowSeconds > 30 || rule.extendBySeconds < 10 || rule.extendBySeconds > 30) return { valid: false, field: 'extension', message: '最后延时触发时间和每次加时必须在 10 秒到 30 秒之间', suggestions: [] };
  if (rule.maxExtendCount < 1 || rule.maxExtendCount > 10) return { valid: false, field: 'extension', message: '最多延时次数必须在 1 到 10 次之间', suggestions: [] };
  if (rule.fatFingerThresholdCents <= rule.incrementCents) return { valid: false, field: 'fatFinger', message: '防误触确认金额必须大于加价幅度', suggestions: [] };
  if (rule.depositBPS < 0 || rule.depositBPS > 10000 || rule.depositFloorCents < 0 || rule.depositCapCents < 0) return { valid: false, field: 'deposit', message: '保证金字段不能为负，比例不能超过 100%', suggestions: [] };
  if (rule.depositFloorCents > rule.depositCapCents) return { valid: false, field: 'deposit', message: '保证金下限不能高于上限', suggestions: [] };
  const minCap = rule.startPriceCents + rule.incrementCents;
  if (rule.capPriceCents < minCap) return { valid: false, field: 'cap', message: `封顶价至少为 ${formatCents(minCap)}`, suggestions: nearestLegalCaps(rule) };
  if ((rule.capPriceCents - rule.startPriceCents) % rule.incrementCents !== 0) return { valid: false, field: 'cap', message: '封顶价需要刚好按当前加价幅度到达', suggestions: nearestLegalCaps(rule) };
  return { valid: true, field: 'cap', message: `买家按当前加价幅度出价，预计 ${Math.floor((rule.capPriceCents - rule.startPriceCents) / rule.incrementCents)} 次到达封顶价`, suggestions: [] };
}

export function monitorQuery(roomID: string, filter: { type: string; auctionID: string; userID: string; traceID: string }) {
  const params = new URLSearchParams();
  params.set('room_id', roomID);
  if (filter.type.trim()) params.set('type', filter.type.trim());
  if (filter.auctionID.trim()) params.set('auction_id', filter.auctionID.trim());
  if (filter.userID.trim()) params.set('user_id', filter.userID.trim());
  if (filter.traceID.trim()) params.set('trace_id', filter.traceID.trim());
  return params.toString();
}

export async function readJSON<T>(response: Response): Promise<T> {
  return await response.json() as T;
}

export function promptSeverityClass(severity?: string) {
  const normalized = (severity || 'info').toLowerCase().replace(/[^a-z0-9_-]/g, '');
  return normalized || 'info';
}

export function formatSeconds(seconds: number) {
  if (seconds >= 3600 && seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds >= 60 && seconds % 60 === 0) return `${seconds / 60}m`;
  return `${seconds}s`;
}

export function depositPreview(rule: RuleDraft) {
  const percent = `${(rule.depositBPS / 100).toFixed(rule.depositBPS % 100 === 0 ? 0 : 2)}%`;
  return `${percent} · ${formatCents(rule.depositFloorCents)}-${formatCents(rule.depositCapCents)}`;
}

export function rulePayload(rule: RuleDraft) {
  return {
    duration_seconds: rule.durationSeconds,
    extend_window_seconds: rule.extendWindowSeconds,
    extend_by_seconds: rule.extendBySeconds,
    max_extend_count: rule.maxExtendCount,
    fat_finger_threshold_cents: rule.fatFingerThresholdCents,
    deposit_bps: rule.depositBPS,
    deposit_floor_cents: rule.depositFloorCents,
    deposit_cap_cents: rule.depositCapCents
  };
}

export async function ensureDemoSession(account: 'host' | 'user') {
  const expectedRole = account === 'host' ? 'host' : 'user';
  const me = await fetch('/api/auth/me');
  if (me.ok) {
    const payload = await readJSON<{ user?: AuthUser }>(me);
    if (payload.user?.Role === expectedRole) return payload.user;
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
  if (!payload.user) {
    throw new Error('login response missing user');
  }
  if (payload.user.Role !== expectedRole) {
    throw new Error(`login role mismatch: ${payload.user.Role}`);
  }
  return payload.user;
}
