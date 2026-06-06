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
  source: string;
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
  share_card?: Record<string, unknown>;
  generated_at: string;
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
  return `¥${((cents ?? 0) / 100).toFixed(2)}`;
}

export function maskUser(userID?: string) {
  if (!userID) return '-';
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
      title: String(highAnomaly.type ?? 'Anomaly'),
      body: String(highAnomaly.message ?? 'Open Anomalies and flight recorder before claiming room health.'),
      source: 'system_anomaly_events'
    });
  }
  const rejects = monitorItems(monitor.rejects).filter(auctionScoped);
  if (rejects.length > 0) {
    const hot = rejects.some((row) => ['BID_AUCTION_TOO_HOT', 'RATE_LIMITED', 'PROCESSING_RETRY_LATER'].includes(String(row.reject_reason ?? row.code ?? '')));
    risks.push({
      level: hot ? 'high' : 'med',
      title: hot ? 'Bid pressure throttle' : 'Rejected bid pressure',
      body: `${rejects.length} recent reject rows; inspect reject_reason and user copy before prompting more bidding.`,
      source: 'bids'
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
      title: 'Recovery pressure',
      body: `${recoveryTotal} reconnect/stale/slow-consumer signals; avoid urging bids until recovery is stable.`,
      source: 'user_activity_events'
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
  if (stale > 0) return `snapshot stale ${stale}`;
  if (reconnects > 0) return `近期重连 ${reconnects}`;
  return '恢复链路正常';
}

export function redisEngineSummary(payload?: MonitorPayload): RedisEngineSummary {
  const rows = payload?.items ?? [];
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
  const engine = redisEngineSummary(monitor.redisEngine);
  const anomalies = visibleAnomalies(monitor, now);
  const scopedRejects = auctionScopedRows(monitor.rejects, active);
  const scopedRecovery = auctionScopedRows(monitor.recovery, active);
  const activeOrders = active ? orders.filter((order) => order.auction_id === active.id) : orders;
  const criticalAnomalies = anomalies.filter((row) => ['CRITICAL', 'HIGH'].includes(anomalySeverity(row)));
  const recoveryPressure = scopedRecovery.reduce((sum, row) => sum + Number(row.reconnect_count_recent ?? 0) + Number(row.snapshot_stale ?? 0) + Number(row.slow_consumer_disconnects ?? 0), 0);
  const moneyRisk = engine.failed_settlements > 0 || engine.settlement_lag_max_ms > 5000 || activeOrders.some((order) => ['FAILED', 'EXPIRED'].includes(order.status));
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
  if (overall === 'critical') return { title: 'Critical', body: '停止强推成交，先处理告警和账本风险。', color: 'red' };
  if (overall === 'degraded') return { title: 'Degraded', body: '直播可继续，但需要关注买家体验或结算积压。', color: 'orange' };
  return { title: 'Healthy', body: '竞拍、买家体验和结算链路当前无阻断信号。', color: 'green' };
}

export function signalCopy(signalTypeValue: string) {
  switch (signalTypeValue) {
  case 'reconcile_redis_engine':
    return '触发 Redis/PG/Kafka 对账';
  case 'force_snapshot_rebuild':
    return '强制重建客户端快照';
  case 'pause_redis_engine':
    return '暂停 Redis 热引擎';
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
  if (row.kind === 'bid' && row.payload?.source === 'AUTO_MAX_BID') return 'Automatic Max Bid settlement wrote a real bid row; private ceiling remains hidden.';
  if (row.kind === 'auction_event') {
    if (row.event_type === 'auction_sold') return 'Terminal auction result persisted; winner/order should be traceable below.';
    if (row.event_type === 'bid_rejected') return 'Bid was rejected by server rules; price and winner remain authoritative.';
    if (row.event_type === 'bid_accepted') return 'Authoritative bid advanced auction seq and current price.';
    if (row.event_type === 'auction_extended') return 'Server extended end_at; clients must use this event, not local countdown.';
    return 'Domain event changed the auction projection for clients.';
  }
  if (row.kind === 'bid') return row.status === 'ACCEPTED' ? 'Bid row confirms executable truth was recorded.' : 'Rejected bid row preserves audit and idempotency evidence.';
  if (row.kind === 'outbox') return row.status === 'PUBLISHED' ? 'Realtime delivery was acknowledged by the relay path.' : 'Realtime delivery still needs relay/retry attention.';
  if (row.kind === 'order') return 'Order/payment handoff state is tied to the auction terminal result.';
  if (row.kind === 'payment_event') return 'Payment provider boundary event is recorded for reconciliation.';
  if (row.kind === 'snapshot_rebuild') return 'Recovery path rebuilt client state from an authoritative source.';
  if (row.kind === 'anomaly') return 'Operational anomaly requires host/ops review before claiming demo health.';
  return 'Timeline row is sourced from backend flight recorder data.';
}

export function timelineNextAction(row: FlightRecorderTimelineRow) {
  if (row.kind === 'bid' && row.payload?.source === 'AUTO_MAX_BID') return 'Confirm public event payload exposes bid_source only, then inspect Max Bid aggregate readiness.';
  if (row.kind === 'outbox' && row.status !== 'PUBLISHED') return 'Open Outbox diagnostics and inspect attempts, shard, and last_error.';
  if (row.kind === 'anomaly') return 'Filter Anomalies by this auction/trace and capture the incident reason.';
  if (row.kind === 'snapshot_rebuild' && row.status !== 'COMPLETED') return 'Check recovery diagnostics and whether clients are stuck recovering.';
  if (row.kind === 'payment_event') return 'Compare order status and provider ids before discussing payment outcome.';
  if (row.kind === 'order') return 'Verify winner, amount, deposit, and expiry before demoing fulfillment.';
  if (row.kind === 'bid' && row.status === 'REJECTED') return 'Use reject_reason to explain user-facing copy and CTA behavior.';
  if (row.trace_id) return `Trace ${row.trace_id} in logs if this row needs deeper investigation.`;
  return 'No action needed unless the row conflicts with expected state.';
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
  if (rule.extendWindowSeconds < 10 || rule.extendWindowSeconds > 30 || rule.extendBySeconds < 10 || rule.extendBySeconds > 30) return { valid: false, field: 'extension', message: '延时窗口和每次延时必须在 10 秒到 30 秒之间', suggestions: [] };
  if (rule.maxExtendCount < 1 || rule.maxExtendCount > 10) return { valid: false, field: 'extension', message: '最多延时次数必须在 1 到 10 次之间', suggestions: [] };
  if (rule.fatFingerThresholdCents <= rule.incrementCents) return { valid: false, field: 'fatFinger', message: '高额确认阈值必须大于加价幅度', suggestions: [] };
  if (rule.depositBPS < 0 || rule.depositBPS > 10000 || rule.depositFloorCents < 0 || rule.depositCapCents < 0) return { valid: false, field: 'deposit', message: '保证金字段不能为负，比例不能超过 10000 bps', suggestions: [] };
  if (rule.depositFloorCents > rule.depositCapCents) return { valid: false, field: 'deposit', message: '保证金下限不能高于上限', suggestions: [] };
  const minCap = rule.startPriceCents + rule.incrementCents;
  if (rule.capPriceCents < minCap) return { valid: false, field: 'cap', message: `封顶价至少为 ${formatCents(minCap)}`, suggestions: nearestLegalCaps(rule) };
  if ((rule.capPriceCents - rule.startPriceCents) % rule.incrementCents !== 0) return { valid: false, field: 'cap', message: '封顶价必须落在起拍价 + N * 加价幅度', suggestions: nearestLegalCaps(rule) };
  return { valid: true, field: 'cap', message: `封顶价可达，预计 ${Math.floor((rule.capPriceCents - rule.startPriceCents) / rule.incrementCents)} 口到顶`, suggestions: [] };
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
  const me = await fetch('/api/auth/me');
  if (me.ok) {
    const payload = await readJSON<{ user?: AuthUser }>(me);
    if (payload.user) return payload.user;
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
  return payload.user;
}
