import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Button, Drawer, Form, Input, InputNumber, Layout, Message, Modal, Space, Table, Tabs, Tag } from '@arco-design/web-react';
import '@arco-design/web-react/dist/css/arco.css';
import { Activity, AlertTriangle, Bell, BellOff, CheckCircle2, ClipboardList, Clock3, Database, ExternalLink, Gavel, ImageIcon, Play, RadioTower, RefreshCw, ShieldCheck, Square, Upload, Wifi } from 'lucide-react';
import './styles.css';

type Room = {
  id: string;
  host_id: string;
  status: string;
  role: string;
};

type Item = {
  id: string;
  title: string;
  image_url?: string;
  description?: string;
};

type RuleDraft = {
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

type Auction = {
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

type Order = {
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

type MonitorPayload = {
  items: Array<Record<string, unknown>>;
};

type RedisEngineMonitorPayload = MonitorPayload & {
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

type RedisEngineSummary = {
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

type FlightRecorderPayload = {
  summary?: FlightRecorderSummary;
  rules?: Array<Record<string, unknown>>;
  orders?: Array<Record<string, unknown>>;
  payment_events?: Array<Record<string, unknown>>;
  anomalies?: Array<Record<string, unknown>>;
  timeline?: FlightRecorderTimelineRow[];
};

type FlightRecorderSummary = {
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

type FlightRecorderTimelineRow = {
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

type HostPrompt = {
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

type HostPromptsPayload = {
  prompts?: HostPrompt[];
};

type HeatSummary = {
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

type MaxBidSummary = {
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

type SignalRequest = {
  signal_type: string;
  target_type: string;
  target_id: string;
  reason: string;
  payload_json?: Record<string, unknown>;
};

type RuleAPIError = {
  code?: string;
  message?: string;
  details?: {
    suggested_caps?: number[];
  };
};
type AuthUser = {
  ID: string;
  Role: string;
};

const defaultRoomID = 'room_main';

function formatCents(cents?: number) {
  return `¥${((cents ?? 0) / 100).toFixed(2)}`;
}

function maskUser(userID?: string) {
  if (!userID) return '-';
  return `${userID.slice(0, 2)}**`;
}

function formatRemaining(endAt?: string, now = Date.now()) {
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

function statusTagColor(status: string) {
  if (status === 'ACTIVE') return 'green';
  if (status === 'SCHEDULED') return 'arcoblue';
  if (status === 'DRAFT') return 'gray';
  if (status === 'SOLD') return 'orangered';
  if (status === 'CANCELLED' || status === 'ENDED') return 'red';
  return 'arcoblue';
}

function terminalStatus(status: string) {
  return ['SOLD', 'ENDED', 'CANCELLED'].includes(status);
}

function queuePriority(status: string) {
  if (status === 'ACTIVE') return 0;
  if (status === 'SCHEDULED') return 1;
  if (status === 'DRAFT') return 2;
  return 3;
}

function sortedAuctions(auctions: Auction[]) {
  return [...auctions].sort((left, right) => {
    const priority = queuePriority(left.status) - queuePriority(right.status);
    if (priority !== 0) return priority;
    return (left.start_at ?? left.end_at ?? left.id).localeCompare(right.start_at ?? right.end_at ?? right.id);
  });
}

function queueGroups(auctions: Auction[]) {
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

function activeAuction(auctions: Auction[]) {
  return auctions.find((auction) => auction.status === 'ACTIVE');
}

function narratingAuction(auctions: Auction[]) {
  return auctions.find((auction) => auction.is_narrating);
}

function monitorCount(payload?: MonitorPayload) {
  return payload?.items?.length ?? 0;
}

function monitorItems(payload?: MonitorPayload) {
  return payload?.items ?? [];
}

function riskQueue(monitor: Record<string, MonitorPayload>, selectedAuction: Auction) {
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

function connectionLabel(monitor: Record<string, MonitorPayload>, roomID: string) {
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

function redisEngineSummary(payload?: MonitorPayload): RedisEngineSummary {
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

function signalTargetID(row: Record<string, unknown>) {
  return String(row.target_id ?? '');
}

function signalType(row: Record<string, unknown>) {
  return String(row.signal_type ?? '');
}

function signalCreatedAt(row: Record<string, unknown>) {
  const value = Date.parse(String(row.created_at ?? ''));
  return Number.isFinite(value) ? value : 0;
}

function anomalyKey(row: Record<string, unknown>) {
  return `alert:${String(row.id ?? row.type ?? row.created_at ?? '')}`;
}

function anomalySeverity(row: Record<string, unknown>) {
  return String(row.severity ?? 'LOW').toUpperCase();
}

function severityTagColor(severity: string) {
  if (severity === 'CRITICAL') return 'red';
  if (severity === 'HIGH') return 'orangered';
  if (severity === 'MED' || severity === 'WARNING') return 'orange';
  return 'green';
}

function isMutedAlert(row: Record<string, unknown>, signals: Record<string, MonitorPayload>, now = Date.now()) {
  const key = anomalyKey(row);
  return monitorItems(signals.signals)
    .filter((signal) => signalType(signal) === 'mute_alert_10m' && signalTargetID(signal) === key)
    .some((signal) => now - signalCreatedAt(signal) < 10 * 60_000);
}

function isAckedAlert(row: Record<string, unknown>, signals: Record<string, MonitorPayload>) {
  const key = anomalyKey(row);
  return monitorItems(signals.signals).some((signal) => signalType(signal) === 'ack_alert' && signalTargetID(signal) === key);
}

function visibleAnomalies(monitor: Record<string, MonitorPayload>, now = Date.now()) {
  return monitorItems(monitor.anomalies)
    .filter((row) => !isMutedAlert(row, monitor, now))
    .sort((left, right) => {
      const severityOrder: Record<string, number> = { CRITICAL: 0, HIGH: 1, MED: 2, WARNING: 2, LOW: 3 };
      const bySeverity = (severityOrder[anomalySeverity(left)] ?? 4) - (severityOrder[anomalySeverity(right)] ?? 4);
      if (bySeverity !== 0) return bySeverity;
      return Date.parse(String(right.created_at ?? '')) - Date.parse(String(left.created_at ?? ''));
    });
}

function auctionScopedRows(payload: MonitorPayload | undefined, auction?: Auction) {
  if (!auction) return monitorItems(payload);
  return monitorItems(payload).filter((row) => {
    const auctionID = String(row.auction_id ?? row.aggregate_id ?? row.target_id ?? '');
    const roomID = String(row.room_id ?? '');
    return !auctionID || auctionID === auction.id || roomID === auction.room_id;
  });
}

function liveHealthSummary(auctions: Auction[], orders: Order[], monitor: Record<string, MonitorPayload>, heatSummary: HeatSummary | undefined, selectedAuction: Auction | undefined, now: number) {
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

function overallCopy(overall: string) {
  if (overall === 'critical') return { title: 'Critical', body: '停止强推成交，先处理告警和账本风险。', color: 'red' };
  if (overall === 'degraded') return { title: 'Degraded', body: '直播可继续，但需要关注买家体验或结算积压。', color: 'orange' };
  return { title: 'Healthy', body: '竞拍、买家体验和结算链路当前无阻断信号。', color: 'green' };
}

function signalCopy(signalTypeValue: string) {
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

function rowSourceURL(sourceKey: string, record: Record<string, unknown>) {
  const auctionID = String(record.auction_id ?? record.aggregate_id ?? record.target_id ?? '');
  if (auctionID && (sourceKey === 'auction_id' || sourceKey === 'trace_id' || sourceKey === 'outbox_id' || sourceKey === 'job_id' || sourceKey === 'request_id' || sourceKey === 'id')) {
    return `/api/monitor/auctions/${encodeURIComponent(auctionID)}/flight-recorder?limit=50&timeline_limit=100`;
  }
  return '';
}

function rowAuctionID(record: Record<string, unknown>) {
  return String(record.auction_id ?? record.aggregate_id ?? record.target_id ?? '');
}

function timelineTone(row: FlightRecorderTimelineRow) {
  const text = `${row.kind} ${row.event_type} ${row.status ?? ''}`.toLowerCase();
  if (text.includes('anomaly') || text.includes('dead') || text.includes('failed') || text.includes('rejected')) return 'red';
  if (text.includes('sold') || text.includes('paid') || text.includes('published') || text.includes('accepted')) return 'green';
  if (text.includes('snapshot') || text.includes('recover')) return 'arcoblue';
  return 'gray';
}

function timelineImpact(row: FlightRecorderTimelineRow) {
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

function timelineNextAction(row: FlightRecorderTimelineRow) {
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

function createRuleDraft(auction?: Auction): RuleDraft {
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

function nearestLegalCaps(rule: RuleDraft) {
  const minCap = rule.startPriceCents + rule.incrementCents;
  if (rule.incrementCents <= 0 || rule.capPriceCents < minCap) {
    return [minCap, minCap + rule.incrementCents].filter((value, index, values) => value > 0 && values.indexOf(value) === index);
  }
  const steps = Math.floor((rule.capPriceCents - rule.startPriceCents) / rule.incrementCents);
  const lower = rule.startPriceCents + steps * rule.incrementCents;
  const upper = lower + rule.incrementCents;
  return [lower, upper].filter((value, index, values) => value >= minCap && values.indexOf(value) === index);
}

function validateRule(rule: RuleDraft) {
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

function monitorQuery(roomID: string, filter: { type: string; auctionID: string; userID: string; traceID: string }) {
  const params = new URLSearchParams();
  params.set('room_id', roomID);
  if (filter.type.trim()) params.set('type', filter.type.trim());
  if (filter.auctionID.trim()) params.set('auction_id', filter.auctionID.trim());
  if (filter.userID.trim()) params.set('user_id', filter.userID.trim());
  if (filter.traceID.trim()) params.set('trace_id', filter.traceID.trim());
  return params.toString();
}

async function readJSON<T>(response: Response): Promise<T> {
  return await response.json() as T;
}

function promptSeverityClass(severity?: string) {
  const normalized = (severity || 'info').toLowerCase().replace(/[^a-z0-9_-]/g, '');
  return normalized || 'info';
}

function formatSeconds(seconds: number) {
  if (seconds >= 3600 && seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds >= 60 && seconds % 60 === 0) return `${seconds / 60}m`;
  return `${seconds}s`;
}

function depositPreview(rule: RuleDraft) {
  const percent = `${(rule.depositBPS / 100).toFixed(rule.depositBPS % 100 === 0 ? 0 : 2)}%`;
  return `${percent} · ${formatCents(rule.depositFloorCents)}-${formatCents(rule.depositCapCents)}`;
}

async function ensureDemoSession(account: 'host' | 'user') {
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

function App() {
  const [items, setItems] = useState<Item[]>([]);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [roomID, setRoomID] = useState(defaultRoomID);
  const [auctions, setAuctions] = useState<Auction[]>([]);
  const [orders, setOrders] = useState<Order[]>([]);
  const [selectedAuctionID, setSelectedAuctionID] = useState('');
  const [monitor, setMonitor] = useState<Record<string, MonitorPayload>>({});
  const [recentEvents, setRecentEvents] = useState<Array<Record<string, unknown>>>([]);
  const [flightRecorderAuctionID, setFlightRecorderAuctionID] = useState('');
  const [flightRecorder, setFlightRecorder] = useState<FlightRecorderPayload | undefined>();
  const [flightRecorderLoading, setFlightRecorderLoading] = useState(false);
  const [orderDetailID, setOrderDetailID] = useState('');
  const [hostPrompts, setHostPrompts] = useState<HostPrompt[]>([]);
  const [dismissedPromptIDs, setDismissedPromptIDs] = useState<string[]>([]);
  const [promptsLoading, setPromptsLoading] = useState(false);
  const [heatSummary, setHeatSummary] = useState<HeatSummary | undefined>();
  const [heatLoading, setHeatLoading] = useState(false);
  const [maxBidSummary, setMaxBidSummary] = useState<MaxBidSummary | undefined>();
  const [maxBidLoading, setMaxBidLoading] = useState(false);
  const [monitorFilter, setMonitorFilter] = useState({ type: '', auctionID: '', userID: '', traceID: '' });
  const [loading, setLoading] = useState(false);
  const [savingRule, setSavingRule] = useState(false);
  const [creating, setCreating] = useState(false);
  const [ruleSaveState, setRuleSaveState] = useState<'idle' | 'saved' | 'error'>('idle');
  const [workspaceTab, setWorkspaceTab] = useState('rules');
  const [backendRuleError, setBackendRuleError] = useState('');
  const [backendSuggestions, setBackendSuggestions] = useState<number[]>([]);
  const [itemDraft, setItemDraft] = useState({ title: '新拍品', description: '本场直播竞拍拍品', imageURL: '' });
  const [itemImageFile, setItemImageFile] = useState<File | null>(null);
  const [scheduleStartAt, setScheduleStartAt] = useState('');
  const [cancelReason, setCancelReason] = useState('主播异常取消');
  const [rule, setRule] = useState<RuleDraft>(createRuleDraft());
  const [sessionReady, setSessionReady] = useState(false);
  const [now, setNow] = useState(Date.now());
  const selectedAuction = useMemo(() => auctions.find((auction) => auction.id === selectedAuctionID) ?? sortedAuctions(auctions)[0], [auctions, selectedAuctionID]);
  const pinnedActiveAuction = useMemo(() => activeAuction(auctions), [auctions]);
  const currentNarratingAuction = useMemo(() => narratingAuction(auctions), [auctions]);
  const ruleValidation = validateRule(rule);
  const shownSuggestions = ruleValidation.valid ? backendSuggestions : ruleValidation.suggestions;

  const openFlightRecorder = async (auctionID: string) => {
    const nextAuctionID = auctionID.trim();
    if (!nextAuctionID) return;
    setFlightRecorderAuctionID(nextAuctionID);
    setFlightRecorderLoading(true);
    setFlightRecorder(undefined);
    try {
      const response = await fetch(`/api/monitor/auctions/${encodeURIComponent(nextAuctionID)}/flight-recorder?limit=80&timeline_limit=120`);
      const payload = await readJSON<FlightRecorderPayload>(response);
      if (!response.ok) throw new Error('flight recorder query failed');
      setFlightRecorder(payload);
    } catch {
      Message.error('Flight recorder 读取失败');
      setFlightRecorder(undefined);
    } finally {
      setFlightRecorderLoading(false);
    }
  };

  const createMonitorSignal = async (request: SignalRequest) => {
    try {
      const response = await fetch('/api/monitor/signals', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request)
      });
      const payload = await readJSON<{ id?: number; message?: string }>(response);
      if (!response.ok) {
        Message.error(payload.message ?? `${signalCopy(request.signal_type)} 失败`);
        return false;
      }
      Message.success(`${signalCopy(request.signal_type)} 已记录 #${payload.id ?? '-'}`);
      await loadAll();
      return true;
    } catch {
      Message.error(`${signalCopy(request.signal_type)} 失败`);
      return false;
    }
  };

  const loadAll = async () => {
    if (!sessionReady) return;
    setLoading(true);
    try {
      const roomPayload = await fetch('/api/rooms').then((r) => readJSON<{ items?: Room[] }>(r));
      const roomRows = roomPayload.items ?? [];
      const nextRoomID = roomRows.find((room) => room.id === roomID)?.id
        ?? roomRows.find((room) => room.id === defaultRoomID)?.id
        ?? roomRows[0]?.id
        ?? roomID;
      const [auctionRows, orderRows, auctionsDiag, redisEngine, anomalies, outbox, outboxWatermarks, snapshots, signals, scheduler, rejects, recovery] = await Promise.all([
        fetch(`/api/auctions?room_id=${nextRoomID}`).then((r) => readJSON<Auction[]>(r)),
        fetch('/api/orders').then((r) => readJSON<Order[]>(r)),
        fetch('/api/monitor/auctions').then((r) => readJSON<MonitorPayload>(r)),
        fetch('/api/monitor/redis-engine').then((r) => readJSON<RedisEngineMonitorPayload>(r)),
        fetch(`/api/monitor/anomalies?${monitorQuery(nextRoomID, monitorFilter)}`).then((r) => readJSON<MonitorPayload>(r)),
        fetch('/api/monitor/outbox').then((r) => readJSON<MonitorPayload>(r)),
        fetch('/api/monitor/outbox/watermarks').then((r) => readJSON<MonitorPayload>(r)),
        fetch('/api/monitor/snapshots').then((r) => readJSON<MonitorPayload>(r)),
        fetch('/api/monitor/signals').then((r) => readJSON<MonitorPayload>(r)),
        fetch('/api/monitor/scheduler').then((r) => readJSON<MonitorPayload>(r)),
        fetch('/api/monitor/rejects').then((r) => readJSON<MonitorPayload>(r)),
        fetch('/api/monitor/recovery').then((r) => readJSON<MonitorPayload>(r))
      ]);
      setRooms(roomRows);
      if (nextRoomID !== roomID) setRoomID(nextRoomID);
      setAuctions(auctionRows);
      setOrders(orderRows);
      setMonitor({ auctions: auctionsDiag, redisEngine, anomalies, outbox, outboxWatermarks, snapshots, signals, scheduler, rejects, recovery });
      const nextSelected = auctionRows.find((row) => row.id === selectedAuctionID)?.id ?? auctionRows.find((row) => row.status === 'ACTIVE')?.id ?? sortedAuctions(auctionRows)[0]?.id ?? '';
      setSelectedAuctionID(nextSelected);
      setItems(auctionRows.map((auction) => auction.item).filter(Boolean));
      const nextAuction = auctionRows.find((row) => row.id === nextSelected) ?? sortedAuctions(auctionRows)[0];
      if (nextAuction) setRule(createRuleDraft(nextAuction));
    } catch {
      Message.error('主控台数据读取失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    let cancelled = false;
    ensureDemoSession('host')
      .then(() => {
        if (!cancelled) setSessionReady(true);
      })
      .catch(() => Message.error('登录失败，请检查后端服务'));
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (sessionReady) void loadAll();
  }, [sessionReady, roomID, monitorFilter.type, monitorFilter.auctionID, monitorFilter.userID, monitorFilter.traceID]);

  useEffect(() => {
    if (selectedAuction) {
      setRule(createRuleDraft(selectedAuction));
      setRuleSaveState('idle');
      setBackendRuleError('');
      setBackendSuggestions([]);
    }
  }, [selectedAuction?.id]);

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    let cancelled = false;
    const loadFlightRecorder = async () => {
      if (!sessionReady || !selectedAuction?.id) {
        setRecentEvents([]);
        return;
      }
      try {
        const response = await fetch(`/api/monitor/auctions/${selectedAuction.id}/flight-recorder?limit=20&timeline_limit=20`);
        const payload = await readJSON<FlightRecorderPayload>(response);
        if (!cancelled) {
          setRecentEvents(response.ok ? (payload.timeline ?? []).slice(0, 6) : []);
        }
      } catch {
        if (!cancelled) setRecentEvents([]);
      }
    };
    void loadFlightRecorder();
    return () => {
      cancelled = true;
    };
  }, [selectedAuction?.id, sessionReady, loading]);

  useEffect(() => {
    let cancelled = false;
    const loadHostPrompts = async () => {
      if (!sessionReady || !selectedAuction?.id) {
        setHostPrompts([]);
        return;
      }
      setPromptsLoading(true);
      try {
        const response = await fetch(`/api/host/auctions/${selectedAuction.id}/prompts`);
        const payload = await readJSON<HostPromptsPayload>(response);
        if (!cancelled) {
          setHostPrompts(response.ok ? (payload.prompts ?? []) : []);
        }
      } catch {
        if (!cancelled) setHostPrompts([]);
      } finally {
        if (!cancelled) setPromptsLoading(false);
      }
    };
    void loadHostPrompts();
    return () => {
      cancelled = true;
    };
  }, [selectedAuction?.id, sessionReady, loading]);

  useEffect(() => {
    let cancelled = false;
    const loadHeatSummary = async () => {
      if (!sessionReady || !selectedAuction?.id) {
        setHeatSummary(undefined);
        return;
      }
      setHeatLoading(true);
      try {
        const response = await fetch(`/api/host/auctions/${selectedAuction.id}/heat-summary`);
        const payload = await readJSON<HeatSummary>(response);
        if (!cancelled) {
          setHeatSummary(response.ok ? payload : undefined);
        }
      } catch {
        if (!cancelled) setHeatSummary(undefined);
      } finally {
        if (!cancelled) setHeatLoading(false);
      }
    };
    void loadHeatSummary();
    return () => {
      cancelled = true;
    };
  }, [selectedAuction?.id, sessionReady, loading]);

  useEffect(() => {
    let cancelled = false;
    const loadMaxBidSummary = async () => {
      if (!sessionReady || !selectedAuction?.id) {
        setMaxBidSummary(undefined);
        return;
      }
      setMaxBidLoading(true);
      try {
        const response = await fetch(`/api/host/auctions/${selectedAuction.id}/max-bid-summary`);
        const payload = await readJSON<MaxBidSummary>(response);
        if (!cancelled) {
          setMaxBidSummary(response.ok ? payload : undefined);
        }
      } catch {
        if (!cancelled) setMaxBidSummary(undefined);
      } finally {
        if (!cancelled) setMaxBidLoading(false);
      }
    };
    void loadMaxBidSummary();
    return () => {
      cancelled = true;
    };
  }, [selectedAuction?.id, sessionReady, loading]);

  const updateRule = (patch: Partial<RuleDraft>) => {
    setRule((current) => ({ ...current, ...patch }));
    setRuleSaveState('idle');
    setBackendRuleError('');
    setBackendSuggestions([]);
  };

  const createItemAndAuction = async () => {
    setCreating(true);
    try {
      let imageURL = itemDraft.imageURL.trim();
      if (itemImageFile) {
        const safeName = itemImageFile.name.replace(/[^a-zA-Z0-9._-]/g, '-');
        const objectName = `items/${Date.now()}-${safeName}`;
        const upload = await fetch('/api/items/upload-url', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ object_name: objectName, content_type: itemImageFile.type || 'application/octet-stream' })
        });
        if (!upload.ok) throw new Error('create upload url failed');
        const payload = await upload.json() as { upload_url?: string; public_url?: string };
        if (!payload.upload_url) throw new Error('missing upload url');
        const put = await fetch(payload.upload_url, {
          method: 'PUT',
          headers: { 'Content-Type': itemImageFile.type || 'application/octet-stream' },
          body: itemImageFile
        });
        if (!put.ok) throw new Error('upload failed');
        imageURL = payload.public_url ?? imageURL;
      }
      const itemResponse = await fetch('/api/items', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: itemDraft.title, description: itemDraft.description, image_url: imageURL || null })
      });
      if (!itemResponse.ok) throw new Error('create item failed');
      const item = await itemResponse.json() as Item;
      const auctionResponse = await fetch('/api/auctions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          room_id: roomID,
          item_id: item.id,
          start_price_cents: rule.startPriceCents,
          increment_cents: rule.incrementCents,
          cap_price_cents: rule.capPriceCents,
          rule: rulePayload(rule)
        })
      });
      if (!auctionResponse.ok) throw new Error('create auction failed');
      const auction = await auctionResponse.json() as Auction;
      setSelectedAuctionID(auction.id);
      Message.success('拍品和竞拍已创建');
      setItemImageFile(null);
      await loadAll();
    } catch {
      Message.error('创建失败，请检查规则和服务端状态');
    } finally {
      setCreating(false);
    }
  };

  const saveRule = async () => {
    if (!ruleValidation.valid || !selectedAuction) return;
    setSavingRule(true);
    setRuleSaveState('idle');
    setBackendRuleError('');
    setBackendSuggestions([]);
    try {
      const response = await fetch(`/api/auctions/${selectedAuction.id}/rules`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          start_price_cents: rule.startPriceCents,
          increment_cents: rule.incrementCents,
          cap_price_cents: rule.capPriceCents,
          ...rulePayload(rule)
        })
      });
      if (!response.ok) {
        const payload = await response.json() as RuleAPIError;
        setRuleSaveState('error');
        setBackendRuleError(payload.code === 'INVALID_AUCTION_RULE_CAP_UNREACHABLE' ? '后端拒绝：封顶价不可达' : payload.message ?? '规则保存失败');
        setBackendSuggestions(payload.details?.suggested_caps ?? []);
        return;
      }
      const updated = await response.json() as Auction;
      setSelectedAuctionID(updated.id);
      setRuleSaveState('saved');
      await loadAll();
    } catch {
      setRuleSaveState('error');
      setBackendRuleError('规则保存失败');
    } finally {
      setSavingRule(false);
    }
  };

  const auctionAction = async (action: 'schedule' | 'unschedule' | 'start' | 'cancel' | 'narrate-start' | 'narrate-stop') => {
    if (!selectedAuction) return;
    try {
      const response = await fetch(`/api/auctions/${selectedAuction.id}/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: action === 'cancel'
          ? JSON.stringify({ reason: cancelReason.trim() || '主播异常取消' })
          : action === 'schedule'
            ? JSON.stringify({ start_at: scheduleStartAt ? new Date(scheduleStartAt).toISOString() : null })
            : undefined
      });
      if (!response.ok) {
        const err = await response.json() as RuleAPIError;
        Message.error(err.message ?? `${action} failed`);
        return;
      }
      Message.success('操作已提交');
      await loadAll();
    } catch {
      Message.error('操作失败');
    }
  };

  const driveDemoBid = async (mode: 'reject' | 'outbid' | 'extend' | 'sold') => {
    if (!selectedAuction || selectedAuction.status !== 'ACTIVE') {
      Message.warning('请选择 ACTIVE 竞拍后再驱动演示');
      return;
    }
    const price = selectedAuction.current_price_cents;
    const increment = selectedAuction.increment_cents;
    const cap = selectedAuction.cap_price_cents ?? price + increment * 5;
    const amount = mode === 'reject'
      ? price + increment + 1
      : mode === 'sold'
        ? cap
        : Math.min(cap, price + increment);
    const clientBidID = `host-demo-${mode}-${Date.now()}`;
    try {
      const response = await fetch(`/api/demo/auctions/${selectedAuction.id}/competing-bid`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          bidder_id: 'user_2',
          client_bid_id: clientBidID,
          amount_cents: amount,
          client_seen_seq: selectedAuction.seq
        })
      });
      const payload = await readJSON<{ result?: string; reject_reason?: string; message?: string }>(response);
      if (!response.ok) {
        Message.error(payload.message ?? '演示出价失败');
        return;
      }
      Message.success(`第二买家真实出价：${payload.result ?? payload.reject_reason ?? mode}`);
      await loadAll();
    } catch {
      Message.error('演示出价失败');
    }
  };

  return (
    <Layout className="console-shell">
      <Layout.Sider className="sider" width={224}>
        <ConsoleNav activeTab={workspaceTab} onSelect={setWorkspaceTab} />
      </Layout.Sider>
      <Layout.Content className="content">
        <HealthRibbon
          active={pinnedActiveAuction ?? selectedAuction}
          loading={loading}
          monitor={monitor}
          now={now}
          roomID={roomID}
          rooms={rooms}
          onRefresh={loadAll}
          onRoomChange={(nextRoomID) => {
            setSelectedAuctionID('');
            setRoomID(nextRoomID);
          }}
        />

        {workspaceTab === 'inventory' && (
          <section className="workspace-page inventory-page" data-testid="pc-inventory-page">
            <div className="section-title">
              <div>
                <h1>拍品管理</h1>
                <p>上架拍品并配置冻结前竞拍规则；开拍和取消进入竞拍页处理。</p>
              </div>
              <span>{selectedAuction ? `当前选中 ${selectedAuction.id}` : '未选中竞拍'}</span>
            </div>
            <InventoryLotsPanel
              auctions={auctions}
              selectedAuction={selectedAuction}
              onSelect={setSelectedAuctionID}
            />
            <div className="two-column inventory-workspace">
              <ItemCreatePanel
                creating={creating}
                itemDraft={itemDraft}
                ruleValid={ruleValidation.valid}
                onCreate={createItemAndAuction}
                onFileChange={setItemImageFile}
                onDraftChange={setItemDraft}
              />
              <RuleEditor
                backendRuleError={backendRuleError}
                rule={rule}
                ruleSaveState={ruleSaveState}
                ruleValidation={ruleValidation}
                savingRule={savingRule}
                selectedAuction={selectedAuction}
                shownSuggestions={shownSuggestions}
                onRuleChange={updateRule}
                onSave={saveRule}
              />
            </div>
          </section>
        )}

        {workspaceTab === 'rules' && (
          <section className="workspace-page auction-page" data-testid="pc-auction-page">
            <div className="section-title">
              <div>
                <h1>竞拍控场</h1>
                <p>选择队列中的拍品，执行排期、开拍、取消、讲解和实时氛围演示。</p>
              </div>
              <span>{pinnedActiveAuction ? `ACTIVE ${pinnedActiveAuction.id}` : '当前无 ACTIVE'}</span>
            </div>
            <div className="command-center" data-testid="pc-command-center">
              <AuctionQueue
                active={pinnedActiveAuction}
                auctions={auctions}
                narrating={currentNarratingAuction}
                selectedAuction={selectedAuction}
                onSelect={setSelectedAuctionID}
              />
              {selectedAuction ? (
                <AuctionControlSummary
                  monitor={monitor}
                  now={now}
                  recentEvents={recentEvents}
                  selectedAuction={selectedAuction}
                >
                  <AuctionCommandPanel
                    cancelReason={cancelReason}
                    activeAuction={pinnedActiveAuction}
                    narratingAuction={currentNarratingAuction}
                    scheduleStartAt={scheduleStartAt}
                    selectedAuction={selectedAuction}
                    onAction={auctionAction}
                    onCancelReasonChange={setCancelReason}
                    onScheduleStartAtChange={setScheduleStartAt}
                  />
                </AuctionControlSummary>
              ) : <div className="command-panel"><div className="empty-state">暂无可控制竞拍</div></div>}
              <LiveAssistRail
                dismissedPromptIDs={dismissedPromptIDs}
                heatLoading={heatLoading}
                heatSummary={heatSummary}
                maxBidLoading={maxBidLoading}
                maxBidSummary={maxBidSummary}
                monitor={monitor}
                onOpenFlightRecorder={openFlightRecorder}
                prompts={hostPrompts}
                promptsLoading={promptsLoading}
                recentEvents={recentEvents}
                selectedAuction={selectedAuction}
                onDismissPrompt={(promptID) => setDismissedPromptIDs((current) => Array.from(new Set([...current, promptID])))}
                onDriveDemoBid={driveDemoBid}
              />
            </div>
            <OrdersPanel orders={orders} onOpenFlightRecorder={openFlightRecorder} onOpenOrder={setOrderDetailID} />
          </section>
        )}

        {workspaceTab === 'health' && (
          <section className="workspace-page live-health-page" data-testid="pc-live-health-page">
            <LiveHealthPanel
              auctions={auctions}
              heatSummary={heatSummary}
              loading={loading}
              monitor={monitor}
              now={now}
              orders={orders}
              selectedAuction={pinnedActiveAuction ?? selectedAuction}
              onOpenFlightRecorder={openFlightRecorder}
              onRefresh={loadAll}
              onSignal={createMonitorSignal}
            />
          </section>
        )}

        {workspaceTab === 'diagnostics' && (
          <section className="workspace-page diagnostics-page" data-testid="pc-diagnostics-page">
            <DiagnosticsPanel
              monitor={monitor}
              monitorFilter={monitorFilter}
              onOpenFlightRecorder={openFlightRecorder}
              onFilterChange={setMonitorFilter}
            />
          </section>
        )}
        <FlightRecorderDrawer
          auctionID={flightRecorderAuctionID}
          loading={flightRecorderLoading}
          payload={flightRecorder}
          visible={Boolean(flightRecorderAuctionID)}
          onClose={() => {
            setFlightRecorderAuctionID('');
            setFlightRecorder(undefined);
          }}
        />
        <OrderDetailDrawer
          order={orders.find((order) => order.id === orderDetailID)}
          visible={Boolean(orderDetailID)}
          onClose={() => setOrderDetailID('')}
          onOpenFlightRecorder={openFlightRecorder}
        />
      </Layout.Content>
    </Layout>
  );
}

function ConsoleNav({ activeTab, onSelect }: { activeTab: string; onSelect: (tab: string) => void }) {
  const rows = [
    { key: 'inventory', label: '拍品', icon: <ClipboardList size={16} /> },
    { key: 'rules', label: '竞拍', icon: <RadioTower size={16} /> },
    { key: 'health', label: '直播健康', icon: <ShieldCheck size={16} /> },
    { key: 'diagnostics', label: '诊断', icon: <Activity size={16} /> }
  ];
  return (
    <>
      <div className="brand">Live Auction</div>
      <nav>
        {rows.map((row) => (
          <button
            type="button"
            className={activeTab === row.key ? 'active' : ''}
            key={row.key}
            onClick={() => onSelect(row.key)}
          >
            {row.icon} {row.label}
          </button>
        ))}
      </nav>
    </>
  );
}

function InventoryLotsPanel({
  auctions,
  selectedAuction,
  onSelect
}: {
  auctions: Auction[];
  selectedAuction?: Auction;
  onSelect: (auctionID: string) => void;
}) {
  const visible = sortedAuctions(auctions).slice(0, 8);
  return (
    <section className="inventory-lots" data-testid="inventory-lot-list" aria-label="拍品列表">
      <div className="panel-heading">
        <h2>拍品列表</h2>
        <span>选择 DRAFT 修改规则；开拍/取消在竞拍页执行</span>
      </div>
      {visible.length === 0 ? <div className="empty-state compact-empty">暂无拍品</div> : (
        <div className="inventory-lot-grid">
          {visible.map((auction) => {
            const selected = selectedAuction?.id === auction.id;
            const editable = auction.status === 'DRAFT';
            return (
              <button
                key={auction.id}
                type="button"
                className={`inventory-lot-card ${selected ? 'selected' : ''}`}
                data-status={auction.status.toLowerCase()}
                onClick={() => onSelect(auction.id)}
              >
                <span className={`queue-thumb ${auction.item?.image_url ? 'has-media' : ''}`} style={auction.item?.image_url ? { '--queue-thumb-url': `url("${auction.item.image_url}")` } as React.CSSProperties : undefined}>
                  {!auction.item?.image_url && <ImageIcon size={18} />}
                </span>
                <strong>{auction.item?.title ?? auction.id}</strong>
                <em>{auction.status} · {editable ? '规则可编辑' : '规则已冻结'}</em>
              </button>
            );
          })}
        </div>
      )}
    </section>
  );
}

function HealthRibbon({
  active,
  loading,
  monitor,
  now,
  roomID,
  rooms,
  onRefresh,
  onRoomChange
}: {
  active?: Auction;
  loading: boolean;
  monitor: Record<string, MonitorPayload>;
  now: number;
  roomID: string;
  rooms: Room[];
  onRefresh: () => void;
  onRoomChange: (roomID: string) => void;
}) {
  const outboxRows = monitor.outbox?.items ?? [];
  const retryAgeMS = Math.max(0, ...outboxRows.map((row) => Number(row.oldest_retry_age_ms ?? row.lag_ms ?? 0)));
  const recoveryLabel = active ? connectionLabel(monitor, active.room_id) : connectionLabel(monitor, roomID);
  return (
    <section className="health-ribbon" data-testid="health-ribbon">
      <div className="ribbon-room">
        <strong>{roomID}</strong>
        <span>Server clock {new Date(now).toLocaleTimeString()}</span>
      </div>
      <div className="ribbon-metrics" data-testid="health-ribbon-status" role="status" aria-live="polite">
        <span><Wifi size={15} /> {recoveryLabel}</span>
        <span><Database size={15} /> Outbox {monitorCount(monitor.outbox)} · oldest {retryAgeMS}ms</span>
        <span><Clock3 size={15} /> Scheduler {monitorCount(monitor.scheduler)}</span>
        <span><AlertTriangle size={15} /> Anomalies {monitorCount(monitor.anomalies)}</span>
      </div>
      <Space>
        <select
          aria-label="room-selector"
          className="native-input ribbon-select"
          value={roomID}
          onChange={(event) => onRoomChange(event.currentTarget.value)}
        >
          {rooms.length === 0 ? <option value={roomID}>{roomID}</option> : rooms.map((room) => (
            <option key={room.id} value={room.id}>{room.id}</option>
          ))}
        </select>
        <Button type="primary" icon={<RefreshCw size={16} />} loading={loading} onClick={onRefresh}>刷新</Button>
      </Space>
    </section>
  );
}

function ItemCreatePanel({
  creating,
  itemDraft,
  ruleValid,
  onCreate,
  onDraftChange,
  onFileChange
}: {
  creating: boolean;
  itemDraft: { title: string; description: string; imageURL: string };
  ruleValid: boolean;
  onCreate: () => void;
  onDraftChange: React.Dispatch<React.SetStateAction<{ title: string; description: string; imageURL: string }>>;
  onFileChange: (file: File | null) => void;
}) {
  return (
    <div className="rule-panel item-create-panel" data-testid="wizard-product-step">
      <h2>拍品上架</h2>
      <Form layout="vertical">
        <Form.Item label="标题">
          <Input aria-label="item-title" value={itemDraft.title} onChange={(value) => onDraftChange((current) => ({ ...current, title: value }))} />
        </Form.Item>
        <Form.Item label="图片 URL">
          <Input aria-label="item-image-url" value={itemDraft.imageURL} onChange={(value) => onDraftChange((current) => ({ ...current, imageURL: value }))} prefix={<Upload size={14} />} />
        </Form.Item>
        <Form.Item label="上传图片文件">
          <input
            aria-label="item-image-file"
            className="native-input"
            type="file"
            accept="image/*"
            onChange={(event) => onFileChange(event.currentTarget.files?.[0] ?? null)}
          />
        </Form.Item>
        <Form.Item label="描述">
          <Input.TextArea aria-label="item-description" value={itemDraft.description} onChange={(value) => onDraftChange((current) => ({ ...current, description: value }))} />
        </Form.Item>
        <Button type="primary" loading={creating} disabled={!itemDraft.title || !ruleValid} onClick={onCreate}>创建拍品和竞拍</Button>
      </Form>
    </div>
  );
}

function AuctionCommandPanel({
  activeAuction,
  cancelReason,
  narratingAuction,
  scheduleStartAt,
  selectedAuction,
  onAction,
  onCancelReasonChange,
  onScheduleStartAtChange
}: {
  activeAuction?: Auction;
  cancelReason: string;
  narratingAuction?: Auction;
  scheduleStartAt: string;
  selectedAuction?: Auction;
  onAction: (action: 'schedule' | 'unschedule' | 'start' | 'cancel' | 'narrate-start' | 'narrate-stop') => void;
  onCancelReasonChange: (reason: string) => void;
  onScheduleStartAtChange: (startAt: string) => void;
}) {
  const isTerminal = selectedAuction ? terminalStatus(selectedAuction.status) : true;
  const activeConflict = Boolean(selectedAuction && activeAuction && activeAuction.id !== selectedAuction.id);
  const narratingConflict = Boolean(selectedAuction && narratingAuction && narratingAuction.id !== selectedAuction.id);
  const canStart = selectedAuction?.status === 'SCHEDULED' && !activeConflict;
  const canNarrate = Boolean(selectedAuction && !selectedAuction.is_narrating && !isTerminal && !narratingConflict);
  return (
    <div className="rule-panel command-actions">
      <h2>竞拍控制</h2>
      {selectedAuction ? (
        <>
          <div className="control-grid">
            <Form.Item label="排期时间">
              <input
                aria-label="schedule-start-at"
                className="native-input"
                type="datetime-local"
                value={scheduleStartAt}
                onChange={(event) => onScheduleStartAtChange(event.currentTarget.value)}
              />
            </Form.Item>
            <Form.Item label="取消原因">
              <Input aria-label="cancel-reason" value={cancelReason} onChange={onCancelReasonChange} />
            </Form.Item>
          </div>
          <Space wrap>
            <Button disabled={selectedAuction.status !== 'DRAFT'} onClick={() => onAction('schedule')}>排期</Button>
            <Button disabled={selectedAuction.status !== 'SCHEDULED'} onClick={() => onAction('unschedule')}>撤回排期</Button>
            <Button disabled={!canStart} icon={<Play size={14} />} onClick={() => onAction('start')}>开拍</Button>
            <Button disabled={isTerminal} status="danger" icon={<Square size={14} />} onClick={() => {
              Modal.confirm({ title: '确认取消竞拍', content: selectedAuction.id, onOk: () => onAction('cancel') });
            }}>取消</Button>
            <Button disabled={!canNarrate} onClick={() => onAction('narrate-start')}>开始讲解</Button>
            <Button disabled={!selectedAuction.is_narrating} onClick={() => onAction('narrate-stop')}>停止讲解</Button>
          </Space>
          <div className="action-guardrail">
            {selectedAuction.status === 'DRAFT' && 'DRAFT 可编辑规则并排期；排期后锁定买家预期，避免开拍前临时改价。'}
            {selectedAuction.status === 'SCHEDULED' && !activeConflict && 'SCHEDULED 已冻结价格规则；如需修改，先撤回排期回到 DRAFT，再改规则并重新排期。'}
            {selectedAuction.status === 'SCHEDULED' && activeConflict && `房间已有 ACTIVE ${activeAuction?.id}；同一房间只能有一个 ACTIVE，需先结束或取消当前竞拍。`}
            {selectedAuction.status === 'ACTIVE' && 'ACTIVE 仅允许讲解切换和带原因取消，不能修改价格真源。'}
            {isTerminal && '终态竞拍不可再操作，订单和诊断保留可追溯记录。'}
            {selectedAuction.status !== 'SCHEDULED' && narratingConflict && `讲解中拍品为 ${narratingAuction?.id}；切换讲解前需先停止当前讲解。`}
          </div>
        </>
      ) : <div className="empty-state">暂无可控制竞拍</div>}
    </div>
  );
}

function AuctionQueue({
  active,
  auctions,
  narrating,
  selectedAuction,
  onSelect
}: {
  active?: Auction;
  auctions: Auction[];
  narrating?: Auction;
  selectedAuction?: Auction;
  onSelect: (auctionID: string) => void;
}) {
  const groups = queueGroups(auctions);
  return (
    <section className="queue-panel" data-testid="auction-queue">
      <div className="panel-heading">
        <h2>竞拍队列</h2>
        <span>{groups.active.length + groups.scheduled.length + groups.draft.length} live · {groups.finishedTotal} history</span>
      </div>
      {auctions.length === 0 ? <div className="empty-state compact-empty">暂无竞拍</div> : (
        <>
          <QueueGroup
            active={active}
            auctions={groups.active}
            label="ACTIVE pinned"
            narrating={narrating}
            selectedAuction={selectedAuction}
            onSelect={onSelect}
          />
          <QueueGroup
            active={active}
            auctions={groups.scheduled}
            label="SCHEDULED"
            narrating={narrating}
            selectedAuction={selectedAuction}
            onSelect={onSelect}
          />
          <QueueGroup
            active={active}
            auctions={groups.draft}
            label="DRAFT"
            narrating={narrating}
            selectedAuction={selectedAuction}
            onSelect={onSelect}
          />
          <QueueGroup
            active={active}
            auctions={groups.finished}
            label={groups.finishedTotal > groups.finished.length ? `FINISHED latest ${groups.finished.length}/${groups.finishedTotal}` : 'FINISHED'}
            narrating={narrating}
            selectedAuction={selectedAuction}
            onSelect={onSelect}
          />
        </>
      )}
    </section>
  );
}

function QueueGroup({
  active,
  auctions,
  label,
  narrating,
  selectedAuction,
  onSelect
}: {
  active?: Auction;
  auctions: Auction[];
  label: string;
  narrating?: Auction;
  selectedAuction?: Auction;
  onSelect: (auctionID: string) => void;
}) {
  if (auctions.length === 0) return null;
  return (
    <div className="queue-group" data-testid={`queue-group-${label.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`}>
      <div className="queue-group-heading">
        <span>{label}</span>
        <strong>{auctions.length}</strong>
      </div>
      {auctions.map((auction) => (
        <QueueCard
          active={active}
          auction={auction}
          key={auction.id}
          narrating={narrating}
          selected={auction.id === selectedAuction?.id}
          onSelect={onSelect}
        />
      ))}
    </div>
  );
}

function QueueCard({
  active,
  auction,
  narrating,
  selected,
  onSelect
}: {
  active?: Auction;
  auction: Auction;
  narrating?: Auction;
  selected: boolean;
  onSelect: (auctionID: string) => void;
}) {
  const activeConflict = Boolean(active && active.id !== auction.id);
  const narratingConflict = Boolean(narrating && narrating.id !== auction.id);
  const constraints: string[] = [];
  if (auction.status === 'SCHEDULED' && activeConflict) constraints.push(`ACTIVE locked by ${active?.id}`);
  if (!auction.is_narrating && narratingConflict) constraints.push(`Narrating locked by ${narrating?.id}`);
  if (auction.status === 'ACTIVE') constraints.push('Pinned current live auction');
  if (auction.status === 'DRAFT') constraints.push('Editable before schedule');
  return (
    <button
      type="button"
      className={`queue-card ${selected ? 'is-selected' : ''} ${auction.status === 'ACTIVE' ? 'is-active' : ''}`}
      onClick={() => onSelect(auction.id)}
    >
      <span className="thumb">
        {auction.item?.image_url ? <img src={auction.item.image_url} alt="" /> : <ImageIcon size={18} />}
      </span>
      <span className="queue-main">
        <span className="queue-title">{auction.item?.title ?? auction.item_id}</span>
        <span className="queue-meta">
          <Tag color={statusTagColor(auction.status)}>{auction.status}</Tag>
          {auction.is_narrating ? <Tag color="green">讲解中</Tag> : <Tag>未讲解</Tag>}
        </span>
        <span className="queue-rules">
          起 {formatCents(auction.start_price_cents)} · 加 {formatCents(auction.increment_cents)} · 封 {formatCents(auction.cap_price_cents)}
        </span>
        <span className="queue-rules">
          当前/成交 {formatCents(auction.current_price_cents)} · {auction.accepted_bid_count} bids · {auction.end_at ? formatRemaining(auction.end_at) : '-'}
        </span>
        <span className="queue-constraints">
          {constraints.map((constraint) => <em key={constraint}>{constraint}</em>)}
        </span>
      </span>
    </button>
  );
}

function AuctionControlSummary({
  monitor,
  now,
  recentEvents,
  selectedAuction,
  children
}: {
  monitor: Record<string, MonitorPayload>;
  now: number;
  recentEvents: Array<Record<string, unknown>>;
  selectedAuction: Auction;
  children?: React.ReactNode;
}) {
  return (
    <section className={`command-panel status-${selectedAuction.status.toLowerCase()}`} data-testid="auction-control-summary">
      <div className="command-hero">
        <div className="command-media">
          {selectedAuction.item?.image_url ? <img src={selectedAuction.item.image_url} alt="" /> : <Gavel size={42} />}
        </div>
        <div className="command-copy">
          <div className="command-kicker">
            <Tag color={statusTagColor(selectedAuction.status)}>{selectedAuction.status}</Tag>
            {selectedAuction.is_narrating ? <Tag color="green">讲解中</Tag> : <Tag>未讲解</Tag>}
            <span><Wifi size={15} /> {connectionLabel(monitor, selectedAuction.room_id)}</span>
          </div>
          <h2>{selectedAuction.item?.title ?? selectedAuction.item_id}</h2>
          <div className="command-price">
            <strong>{formatCents(selectedAuction.current_price_cents)}</strong>
            <span><Clock3 size={18} /> {formatRemaining(selectedAuction.end_at, now)}</span>
          </div>
          <div className="command-subline">
            <span>Leader {maskUser(selectedAuction.current_winner_id)}</span>
            <span>Seq {selectedAuction.seq}</span>
            <span>{selectedAuction.accepted_bid_count} accepted bids</span>
          </div>
        </div>
      </div>
      <div className="control-stats">
        <div>
          <span>当前价</span>
          <strong>{formatCents(selectedAuction.current_price_cents)}</strong>
        </div>
        <div>
          <span>领先者</span>
          <strong>{maskUser(selectedAuction.current_winner_id)}</strong>
        </div>
        <div>
          <span>服务端倒计时</span>
          <strong><Clock3 size={15} /> {formatRemaining(selectedAuction.end_at, now)}</strong>
        </div>
        <div>
          <span>出价 / 参与</span>
          <strong>{selectedAuction.accepted_bid_count} / approx</strong>
        </div>
        <div>
          <span>延时次数</span>
          <strong>{selectedAuction.extend_count} / {selectedAuction.rule.max_extend_count}</strong>
        </div>
        <div>
          <span>状态 / seq</span>
          <strong>{selectedAuction.status} · {selectedAuction.seq}</strong>
        </div>
      </div>
      {children}
    </section>
  );
}

function LiveAssistRail({
  dismissedPromptIDs,
  heatLoading,
  heatSummary,
  maxBidLoading,
  maxBidSummary,
  monitor,
  onOpenFlightRecorder,
  prompts,
  promptsLoading,
  recentEvents,
  selectedAuction,
  onDismissPrompt,
  onDriveDemoBid
}: {
  dismissedPromptIDs: string[];
  heatLoading: boolean;
  heatSummary?: HeatSummary;
  maxBidLoading: boolean;
  maxBidSummary?: MaxBidSummary;
  monitor: Record<string, MonitorPayload>;
  onOpenFlightRecorder: (auctionID: string) => void;
  prompts: HostPrompt[];
  promptsLoading: boolean;
  recentEvents: Array<Record<string, unknown>>;
  selectedAuction?: Auction;
  onDismissPrompt: (promptID: string) => void;
  onDriveDemoBid: (mode: 'reject' | 'outbid' | 'extend' | 'sold') => void;
}) {
  if (!selectedAuction) {
    return (
      <aside className="assist-rail">
        <div className="panel-heading">
          <h2>Live Assist</h2>
          <span>idle</span>
        </div>
        <div className="empty-state compact-empty">选择竞拍后显示事件和控场状态</div>
      </aside>
    );
  }
  const recovery = connectionLabel(monitor, selectedAuction.room_id);
  const hasAnomaly = monitorCount(monitor.anomalies) > 0;
  const visiblePrompts = prompts.filter((prompt) => prompt.id && !dismissedPromptIDs.includes(prompt.id)).slice(0, 3);
  const topPrompt = visiblePrompts[0];
  const risks = riskQueue(monitor, selectedAuction);
  return (
    <aside className="assist-rail" data-testid="live-assist-rail">
      <div className="panel-heading">
        <h2>Live Assist</h2>
        <span>{promptsLoading ? 'loading' : `${prompts.length} prompts`}</span>
      </div>
      <div className="prompter-stack" data-testid="prompter-cards">
        {visiblePrompts.length === 0 ? (
          <div className="assist-card pending">
            <span>{promptsLoading ? 'Prompter loading' : 'Prompter clear'}</span>
            <strong>{promptsLoading ? '正在读取 host-only prompts API' : '暂无主播提示'}</strong>
            <small>提示仅来自后端真实竞拍数据；不会自动修改竞拍或发送弹幕。</small>
          </div>
        ) : visiblePrompts.map((prompt) => (
          <div className={`assist-card prompt severity-${promptSeverityClass(prompt.severity)}`} key={prompt.id}>
            <span>{prompt.type} · {prompt.source}</span>
            <strong>{prompt.title}</strong>
            <small>{prompt.body}</small>
            <div className="prompt-meta">
              {prompt.reference_price_cents !== undefined ? <em>参考下一口 {formatCents(prompt.reference_price_cents)}</em> : null}
              {prompt.metric_label ? <em>{prompt.metric_label}: {prompt.metric_value ?? 0}</em> : null}
              {prompt.event_seq !== undefined ? <em>seq {prompt.event_seq}</em> : null}
            </div>
            <Button size="mini" onClick={() => onDismissPrompt(prompt.id)}>本场隐藏</Button>
          </div>
        ))}
      </div>
      <div className="talk-points" data-testid="talk-points">
        <span>Talk Points</span>
        <button type="button">证书/瑕疵</button>
        <button type="button">封顶/保证金</button>
        <button type="button">延时规则</button>
      </div>
      <div className="demo-driver" data-testid="demo-driver">
        <div className="heat-summary-head">
          <span>本地演示驱动</span>
          <strong>{selectedAuction.status === 'ACTIVE' ? 'real bid API' : 'ACTIVE only'}</strong>
        </div>
        <div className="demo-driver-grid">
          <Button size="mini" disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('reject')}>触发 reject</Button>
          <Button size="mini" disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('outbid')}>第二买家超越</Button>
          <Button size="mini" disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('extend')}>窗口出价/延时</Button>
          <Button size="mini" status="danger" disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('sold')}>封顶 SOLD</Button>
        </div>
        <small>按钮调用本地 host-only demo API，最终仍写入真实 bids、auction_events、outbox、orders；不是前端改状态。</small>
      </div>
      <div className="max-bid-summary" data-testid="max-bid-summary">
        <div className="heat-summary-head">
          <span>Max Bid readiness</span>
          <strong>{maxBidLoading ? 'loading' : maxBidSummary ? maxBidSummary.source : 'unavailable'}</strong>
        </div>
        {maxBidSummary ? (
          <>
            <div className="heat-grid">
              <div><span>Active intents</span><strong>{maxBidSummary.active_intent_count}</strong></div>
              <div><span>Pre-bids</span><strong>{maxBidSummary.pre_bid_count}</strong></div>
              <div><span>Max bids</span><strong>{maxBidSummary.max_bid_count}</strong></div>
              <div><span>Auto applied</span><strong>{maxBidSummary.applied_intent_count}</strong></div>
              <div><span>Exhausted</span><strong>{maxBidSummary.exhausted_count}</strong></div>
              <div><span>Cancelled</span><strong>{maxBidSummary.cancelled_count}</strong></div>
            </div>
            <small>{maxBidSummary.has_private_pressure ? '有私有托管出价压力；主播只看聚合计数，明细请查 flight recorder。' : '暂无活跃私有托管出价。'}</small>
            <Button size="mini" icon={<ExternalLink size={13} />} onClick={() => onOpenFlightRecorder(selectedAuction.id)}>审计自动出价</Button>
          </>
        ) : (
          <div className="heat-unavailable">{maxBidLoading ? '正在读取真实聚合' : 'Max Bid 聚合暂不可用'}</div>
        )}
      </div>
      <div className="heat-summary" data-testid="heat-summary">
        <div className="heat-summary-head">
          <span>Heat 30s</span>
          <strong>{heatLoading ? 'loading' : heatSummary ? heatSummary.source : 'unavailable'}</strong>
        </div>
        {heatSummary ? (
          <div className="heat-grid">
            <div><span>Active bidders</span><strong>{heatSummary.active_bidders_30s}</strong></div>
            <div><span>Accepted bids</span><strong>{heatSummary.accepted_bids_30s}</strong></div>
            <div><span>Rejected bids</span><strong>{heatSummary.rejected_bids_30s}</strong></div>
            <div><span>Chat</span><strong>{heatSummary.chat_messages_30s}</strong></div>
            <div><span>Recovery</span><strong>{heatSummary.recovery_events_30s}</strong></div>
            <div><span>Watchers</span><strong>{heatSummary.watcher_count_available ? heatSummary.watcher_count ?? 0 : 'unavailable'}</strong></div>
          </div>
        ) : (
          <div className="heat-unavailable">{heatLoading ? '正在读取真实聚合' : '热度聚合暂不可用'}</div>
        )}
      </div>
      <div className="assist-grid">
        <div>
          <span>Recovery</span>
          <strong>{recovery}</strong>
        </div>
        <div>
          <span>Outbox</span>
          <strong>{monitorCount(monitor.outbox)}</strong>
        </div>
        <div>
          <span>Rejects</span>
          <strong>{monitorCount(monitor.rejects)}</strong>
        </div>
        <div className={hasAnomaly ? 'risk' : ''}>
          <span>Risk</span>
          <strong>{hasAnomaly ? `${monitorCount(monitor.anomalies)} anomalies` : 'clear'}</strong>
        </div>
      </div>
      <div className="risk-queue" data-testid="risk-queue" role="status" aria-live="polite">
        <div className="heat-summary-head">
          <span>Risk queue</span>
          <strong>{risks.length ? `${risks.length} real signals` : 'clear'}</strong>
        </div>
        {risks.length === 0 ? (
          <div className="heat-unavailable">暂无拒绝、异常或恢复压力信号</div>
        ) : risks.map((risk) => (
          <div className={`risk-row risk-${risk.level}`} key={`${risk.source}-${risk.title}`}>
            <strong>{risk.title}</strong>
            <span>{risk.body}</span>
            <em>{risk.source}</em>
          </div>
        ))}
      </div>
      <div className="system-chat-disabled" data-testid="system-chat-disabled">
        <strong>系统弹幕模板</strong>
        <span>当前 chat API 只支持用户消息，本场不启用主播一键系统弹幕。</span>
        <Button disabled>发送模板</Button>
      </div>
      {topPrompt ? <div className="risk-hint" data-testid="risk-hint">优先处理：{topPrompt.title}</div> : null}
      <EventTimeline events={recentEvents} selectedAuction={selectedAuction} onOpenFlightRecorder={onOpenFlightRecorder} />
    </aside>
  );
}

function EventTimeline({
  events,
  onOpenFlightRecorder,
  selectedAuction
}: {
  events: Array<Record<string, unknown>>;
  onOpenFlightRecorder: (auctionID: string) => void;
  selectedAuction: Auction;
}) {
  return (
    <div className="recent-events" data-testid="recent-events">
      <div className="recent-title">
        <strong>最近事件</strong>
        <button type="button" className="link-button" onClick={() => onOpenFlightRecorder(selectedAuction.id)}>
          Flight recorder <ExternalLink size={13} />
        </button>
      </div>
      {events.length === 0 ? (
        <div className="empty-state compact-empty">暂无最近事件</div>
      ) : events.map((event, index) => (
        <div className="recent-event-row" key={`${String(event.kind ?? event.event_type ?? 'event')}-${index}`}>
          <Tag color={String(event.kind ?? event.event_type).includes('anomaly') ? 'red' : 'arcoblue'}>{String(event.kind ?? event.event_type ?? '-')}</Tag>
          <span>{String(event.event_type ?? event.status ?? event.result ?? '-')}</span>
          <code>{String(event.seq ?? event.trace_id ?? event.outbox_id ?? event.order_id ?? '-')}</code>
        </div>
      ))}
    </div>
  );
}

function RuleEditor({
  backendRuleError,
  rule,
  ruleSaveState,
  ruleValidation,
  savingRule,
  selectedAuction,
  shownSuggestions,
  onRuleChange,
  onSave
}: {
  backendRuleError: string;
  rule: RuleDraft;
  ruleSaveState: 'idle' | 'saved' | 'error';
  ruleValidation: ReturnType<typeof validateRule>;
  savingRule: boolean;
  selectedAuction?: Auction;
  shownSuggestions: number[];
  onRuleChange: (patch: Partial<RuleDraft>) => void;
  onSave: () => void;
}) {
  const freezeReason = selectedAuction && selectedAuction.status !== 'DRAFT'
    ? `规则已随 ${selectedAuction.status} 状态冻结，仅 DRAFT 竞拍允许修改。`
    : '';
  const saveDisabled = !ruleValidation.valid || !selectedAuction || selectedAuction.status !== 'DRAFT';
  const steps = [
    { key: 'product', label: 'Product', summary: selectedAuction?.item?.title ?? '新拍品草稿' },
    { key: 'price', label: 'Price', summary: `${formatCents(rule.startPriceCents)} / +${formatCents(rule.incrementCents)} / cap ${formatCents(rule.capPriceCents)}` },
    { key: 'time', label: 'Time', summary: `${formatSeconds(rule.durationSeconds)} · extend ${formatSeconds(rule.extendWindowSeconds)} +${formatSeconds(rule.extendBySeconds)}` },
    { key: 'trust', label: 'Trust', summary: depositPreview(rule) },
    { key: 'preview', label: 'Preview', summary: ruleValidation.valid ? 'ready' : ruleValidation.field }
  ];
  return (
    <div className="rule-panel rule-wizard" data-testid="seller-rule-wizard">
      <h2>规则 {selectedAuction ? selectedAuction.id : ''}</h2>
      <div className="wizard-steps" aria-label="seller-rule-wizard-steps">
        {steps.map((step) => (
          <div className={`wizard-step ${step.key === 'preview' && !ruleValidation.valid ? 'has-error' : ''}`} key={step.key}>
            <strong>{step.label}</strong>
            <span>{step.summary}</span>
          </div>
        ))}
      </div>
      <Form layout="vertical">
        <section className="wizard-section" data-testid="wizard-price-step">
          <div className="wizard-section-title">
            <span>Price</span>
            <strong>起拍、加价、封顶必须形成可达价格网格</strong>
          </div>
          <div className="rule-subgrid">
            <NumberField label="起拍价" name="start-price-cents" value={rule.startPriceCents} min={0} onChange={(value) => onRuleChange({ startPriceCents: value })} />
            <NumberField label="加价幅度" name="increment-cents" value={rule.incrementCents} min={1} onChange={(value) => onRuleChange({ incrementCents: value })} />
          </div>
          <Form.Item label="封顶价" validateStatus={ruleValidation.valid ? 'success' : 'error'} help={ruleValidation.message}>
            <InputNumber aria-label="cap-price-cents" value={rule.capPriceCents} min={0} suffix="分" onChange={(value) => onRuleChange({ capPriceCents: Number(value) || 0 })} />
          </Form.Item>
        </section>
        {backendRuleError && <div className="backend-rule-error" role="alert">{backendRuleError}</div>}
        {ruleSaveState === 'saved' && <div className="rule-save-ok" role="status">规则已保存</div>}
        {!ruleValidation.valid && ruleValidation.field !== 'cap' && <div className="backend-rule-error" role="alert">{ruleValidation.message}</div>}
        {shownSuggestions.length > 0 && (
          <div className="cap-suggestions" data-testid="cap-suggestions">
            {shownSuggestions.map((cap) => <button key={cap} type="button" onClick={() => onRuleChange({ capPriceCents: cap })}>{formatCents(cap)}</button>)}
          </div>
        )}
        <section className="wizard-section" data-testid="wizard-time-step">
          <div className="wizard-section-title">
            <span>Time / Extension</span>
            <strong>倒计时只展示状态，成交仍由后端状态机决定</strong>
          </div>
          <div className="rule-subgrid">
            <NumberField label="时长" name="duration-seconds" value={rule.durationSeconds} min={30} max={86400} onChange={(value) => onRuleChange({ durationSeconds: value })} />
            <NumberField label="延时窗口" name="extend-window-seconds" value={rule.extendWindowSeconds} min={0} onChange={(value) => onRuleChange({ extendWindowSeconds: value })} />
            <NumberField label="每次延时" name="extend-by-seconds" value={rule.extendBySeconds} min={0} onChange={(value) => onRuleChange({ extendBySeconds: value })} />
            <NumberField label="最多延时" name="max-extend-count" value={rule.maxExtendCount} min={0} onChange={(value) => onRuleChange({ maxExtendCount: value })} />
          </div>
        </section>
        <section className="wizard-section" data-testid="wizard-trust-step">
          <div className="wizard-section-title">
            <span>Trust / Deposit</span>
            <strong>高额确认和保证金提示必须在买家下单前可见</strong>
          </div>
          <div className="rule-subgrid">
            <NumberField label="高额确认" name="fat-finger-threshold-cents" value={rule.fatFingerThresholdCents} min={rule.incrementCents + 1} onChange={(value) => onRuleChange({ fatFingerThresholdCents: value })} />
            <NumberField label="保证金比例" name="deposit-bps" value={rule.depositBPS} min={0} max={10000} onChange={(value) => onRuleChange({ depositBPS: value })} suffix="bps" />
            <NumberField label="保证金下限" name="deposit-floor-cents" value={rule.depositFloorCents} min={0} onChange={(value) => onRuleChange({ depositFloorCents: value })} />
            <NumberField label="保证金上限" name="deposit-cap-cents" value={rule.depositCapCents} min={0} onChange={(value) => onRuleChange({ depositCapCents: value })} />
          </div>
          <div className="verified-bidder-placeholder" data-testid="verified-bidder-placeholder">
            <div>
              <strong>Verified bidder gate</strong>
              <span>后端尚未提供强制验证规则字段；当前只展示买家端兼容状态，不写入竞拍规则。</span>
            </div>
            <Button disabled>启用验证门槛</Button>
          </div>
        </section>
        <section className="wizard-section h5-rule-preview" data-testid="h5-rule-preview">
          <div className="wizard-section-title">
            <span>Preview</span>
            <strong>H5 出价区规则芯片</strong>
          </div>
          <div className="h5-preview-surface">
            <div className="h5-preview-price">
              <span>当前价</span>
              <strong>{formatCents(rule.startPriceCents)}</strong>
              <em>下一口 {formatCents(rule.startPriceCents + rule.incrementCents)}</em>
            </div>
            <div className="h5-preview-chips">
              <span>+{formatCents(rule.incrementCents)}</span>
              <span>封顶 {formatCents(rule.capPriceCents)}</span>
              <span>延时 {formatSeconds(rule.extendWindowSeconds)} +{formatSeconds(rule.extendBySeconds)}</span>
              <span>保证金 {depositPreview(rule)}</span>
              <span>高额确认 {formatCents(rule.fatFingerThresholdCents)}</span>
            </div>
          </div>
        </section>
        {freezeReason && <div className="rule-freeze-reason" data-testid="rule-freeze-reason">{freezeReason}</div>}
        <Button type="primary" disabled={saveDisabled} loading={savingRule} onClick={onSave}>保存规则</Button>
      </Form>
    </div>
  );
}

function OrdersPanel({ orders, onOpenFlightRecorder, onOpenOrder }: { orders: Order[]; onOpenFlightRecorder: (auctionID: string) => void; onOpenOrder: (orderID: string) => void }) {
  return (
    <div className="rule-panel">
      <div className="panel-heading">
        <h2>订单</h2>
        <span>成交后自动生成，详情保留 winner、保证金、支付时效和审计入口</span>
      </div>
      {orders.length === 0 ? <div className="empty-state">暂无订单</div> : orders.map((order) => (
        <div className="order-line" key={order.id}>
          <button type="button" className="order-id-link" onClick={() => onOpenOrder(order.id)}>{order.id}</button>
          <Tag color={order.status === 'PAID' ? 'green' : 'orange'}>{order.status}</Tag>
          <span>winner {maskUser(order.winner_id)}</span>
          <span>{order.deposit_status}</span>
          <span>{order.expire_at ? `expires ${new Date(order.expire_at).toLocaleTimeString()}` : 'no expiry'}</span>
          <strong>{formatCents(order.amount_cents)}</strong>
          <Button size="mini" icon={<ExternalLink size={13} />} onClick={() => onOpenFlightRecorder(order.auction_id)}>审计</Button>
        </div>
      ))}
    </div>
  );
}

function LiveHealthPanel({
  auctions,
  heatSummary,
  loading,
  monitor,
  now,
  orders,
  selectedAuction,
  onOpenFlightRecorder,
  onRefresh,
  onSignal
}: {
  auctions: Auction[];
  heatSummary?: HeatSummary;
  loading: boolean;
  monitor: Record<string, MonitorPayload>;
  now: number;
  orders: Order[];
  selectedAuction?: Auction;
  onOpenFlightRecorder: (auctionID: string) => void;
  onRefresh: () => Promise<void>;
  onSignal: (request: SignalRequest) => Promise<boolean>;
}) {
  const summary = liveHealthSummary(auctions, orders, monitor, heatSummary, selectedAuction, now);
  const active = summary.active;
  const overall = overallCopy(summary.overall);
  const [note, setNote] = useState('');
  const [noteTarget, setNoteTarget] = useState<'auction' | 'room'>('auction');
  const targetID = noteTarget === 'auction' ? active?.id : active?.room_id;

  const sendSignal = async (signalTypeValue: string, targetType: string, targetIDValue: string, reason: string, payload?: Record<string, unknown>) => {
    if (!targetIDValue.trim() || !reason.trim()) {
      Message.error('缺少 target 或原因');
      return;
    }
    await onSignal({
      signal_type: signalTypeValue,
      target_type: targetType,
      target_id: targetIDValue,
      reason,
      payload_json: payload
    });
  };

  const confirmEngineAction = (signalTypeValue: string, reason: string) => {
    if (!active) {
      Message.error('当前没有可操作的竞拍');
      return;
    }
    Modal.confirm({
      title: signalCopy(signalTypeValue),
      content: `目标竞拍 ${active.id}。该操作会写入 system_control_signals，由后台 worker 审计处理。`,
      okText: '确认执行',
      cancelText: '取消',
      onOk: () => sendSignal(signalTypeValue, 'auction', active.id, reason, {
        source: 'pc_live_health',
        room_id: active.room_id,
        status: active.status
      })
    });
  };

  const submitNote = async () => {
    if (!targetID || !note.trim()) {
      Message.error('请填写备注内容');
      return;
    }
    const ok = await onSignal({
      signal_type: 'merchant_incident_note',
      target_type: noteTarget,
      target_id: targetID,
      reason: note.trim(),
      payload_json: {
        source: 'pc_live_health',
        auction_id: active?.id,
        room_id: active?.room_id,
        health_status: summary.overall
      }
    });
    if (ok) setNote('');
  };

  const timelineRows = [
    ...monitorItems(monitor.anomalies).map((row) => ({ kind: 'alert', time: String(row.created_at ?? ''), title: String(row.type ?? 'Anomaly'), body: String(row.message ?? ''), severity: anomalySeverity(row), ref: anomalyKey(row), auctionID: String(row.auction_id ?? '') })),
    ...monitorItems(monitor.signals).map((row) => ({ kind: 'signal', time: String(row.created_at ?? ''), title: signalCopy(signalType(row)), body: String(row.reason ?? ''), severity: String(row.status ?? 'PENDING'), ref: String(row.id ?? ''), auctionID: signalTargetID(row) }))
  ].sort((left, right) => Date.parse(right.time) - Date.parse(left.time)).slice(0, 10);

  return (
    <>
      <div className="section-title live-health-title">
        <div>
          <h1>直播健康</h1>
          <p>把实时经营、买家影响、结算风险和告警处置放在同一个商家工作台。</p>
        </div>
        <Space>
          <Tag color={overall.color}>{overall.title}</Tag>
          <Button icon={<RefreshCw size={15} />} loading={loading} onClick={() => void onRefresh()}>刷新</Button>
        </Space>
      </div>

      <section className={`health-overview health-${summary.overall}`} data-testid="live-health-overview">
        <div className="health-status-block">
          <span>Room health</span>
          <strong>{overall.title}</strong>
          <p>{overall.body}</p>
        </div>
        <div className="health-auction-block">
          <span>Current auction</span>
          <strong>{active?.item.title ?? '暂无 ACTIVE 竞拍'}</strong>
          <p>{active ? `${active.status} · ${formatCents(active.current_price_cents)} · seq ${active.seq} · ${formatRemaining(active.end_at, now)}` : '选择或开拍后展示实时风险'}</p>
        </div>
        <div className="health-action-row">
          <Button disabled={!active} icon={<ExternalLink size={15} />} onClick={() => active && onOpenFlightRecorder(active.id)}>飞行记录</Button>
          <Button disabled={!active} onClick={() => confirmEngineAction('reconcile_redis_engine', '商家端直播健康发现风险，触发热引擎对账')}>触发对账</Button>
          <Button disabled={!active} onClick={() => confirmEngineAction('force_snapshot_rebuild', '商家端直播健康要求重建客户端快照')}>重建快照</Button>
          <Button status="danger" disabled={!active} onClick={() => confirmEngineAction('pause_redis_engine', '商家端直播健康人工暂停热引擎')}>暂停引擎</Button>
        </div>
      </section>

      <section className="health-grid" data-testid="live-health-grid">
        <div className="health-panel">
          <div className="health-panel-head">
            <span><Activity size={15} /> 实时经营</span>
            <strong>{heatSummary ? `${heatSummary.window_seconds}s window` : 'monitor fallback'}</strong>
          </div>
          <div className="funnel-bars">
            <FunnelBar label="观看" value={summary.funnel.watchers ?? 0} max={Math.max(summary.funnel.watchers ?? 0, summary.funnel.activeBidders, summary.funnel.accepted, 1)} muted={summary.funnel.watchers === undefined} />
            <FunnelBar label="活跃出价人" value={summary.funnel.activeBidders} max={Math.max(summary.funnel.watchers ?? 0, summary.funnel.activeBidders, summary.funnel.accepted, 1)} />
            <FunnelBar label="接受出价" value={summary.funnel.accepted} max={Math.max(summary.funnel.accepted + summary.funnel.rejected, 1)} />
            <FunnelBar label="拒绝出价" value={summary.funnel.rejected} max={Math.max(summary.funnel.accepted + summary.funnel.rejected, 1)} tone="warn" />
            <FunnelBar label="订单/支付" value={summary.funnel.orders} max={Math.max(summary.funnel.orders, 1)} caption={`${summary.funnel.paid} paid`} />
          </div>
        </div>

        <div className="health-panel">
          <div className="health-panel-head">
            <span><RadioTower size={15} /> 系统健康</span>
            <strong>Redis/Kafka/PG</strong>
          </div>
          <HealthMetric label="Redis pending" value={summary.engine.pending_redis_decisions} status={summary.engine.pending_redis_decisions > 0 ? 'bad' : 'ok'} />
          <HealthMetric label="Settlement pending" value={summary.engine.pending_settlements} status={summary.engine.pending_settlements > 0 ? 'warn' : 'ok'} />
          <HealthMetric label="Settlement failed" value={summary.engine.failed_settlements} status={summary.engine.failed_settlements > 0 ? 'bad' : 'ok'} />
          <HealthMetric label="Lag max" value={`${summary.engine.settlement_lag_max_ms}ms`} status={summary.engine.settlement_lag_max_ms > 5000 ? 'bad' : summary.engine.settlement_lag_max_ms > 1000 ? 'warn' : 'ok'} />
          <HealthMetric label="Paused engines" value={summary.engine.paused_auctions} status={summary.engine.paused_auctions > 0 ? 'bad' : 'ok'} />
        </div>

        <div className="health-panel">
          <div className="health-panel-head">
            <span><Wifi size={15} /> 买家影响</span>
            <strong>{summary.buyerRisk ? 'attention' : 'normal'}</strong>
          </div>
          <HealthMetric label="Recovery pressure" value={summary.recoveryPressure} status={summary.recoveryPressure > 0 ? 'warn' : 'ok'} />
          <HealthMetric label="Reject rows" value={summary.scopedRejects.length} status={summary.scopedRejects.length > 0 ? 'warn' : 'ok'} />
          <HealthMetric label="Recent anomalies" value={summary.anomalies.length} status={summary.criticalAnomalies.length > 0 ? 'bad' : summary.anomalies.length > 0 ? 'warn' : 'ok'} />
          <p className="health-copy">出现恢复压力或 ENGINE_PAUSED 时，主播不应继续强推用户加价，先确认 H5 CTA 是否进入恢复/暂停态。</p>
        </div>
      </section>

      <section className="incident-workspace" data-testid="incident-workspace">
        <div className="health-panel alert-panel">
          <div className="health-panel-head">
            <span><Bell size={15} /> 告警处置</span>
            <strong>{summary.anomalies.length} active</strong>
          </div>
          {summary.anomalies.length === 0 ? <div className="empty-state compact-empty">暂无未静默告警</div> : summary.anomalies.slice(0, 6).map((row) => {
            const key = anomalyKey(row);
            const auctionID = String(row.auction_id ?? '');
            return (
              <div className={`alert-row severity-${anomalySeverity(row).toLowerCase()}`} key={key}>
                <div>
                  <Tag color={severityTagColor(anomalySeverity(row))}>{anomalySeverity(row)}</Tag>
                  {isAckedAlert(row, monitor) ? <Tag color="green">ACK</Tag> : null}
                </div>
                <div className="alert-main">
                  <strong>{String(row.type ?? 'Anomaly')}</strong>
                  <p>{String(row.message ?? '-')}</p>
                  <span>{new Date(String(row.created_at ?? '')).toLocaleString()} · {auctionID || 'room/system'}</span>
                </div>
                <div className="alert-actions">
                  {auctionID ? <Button size="mini" onClick={() => onOpenFlightRecorder(auctionID)}>记录</Button> : null}
                  <Button size="mini" icon={<CheckCircle2 size={13} />} onClick={() => void sendSignal('ack_alert', 'alert', key, `确认告警 ${String(row.type ?? '')}`, { anomaly_id: row.id, severity: row.severity })}>确认</Button>
                  <Button size="mini" icon={<BellOff size={13} />} onClick={() => void sendSignal('mute_alert_10m', 'alert', key, `静默告警 ${String(row.type ?? '')} 10 分钟`, { anomaly_id: row.id, muted_until_ms: now + 10 * 60_000 })}>静默</Button>
                </div>
              </div>
            );
          })}
        </div>

        <div className="health-panel note-panel">
          <div className="health-panel-head">
            <span><ClipboardList size={15} /> 主动事件</span>
            <strong>audit trail</strong>
          </div>
          <div className="note-target">
            <select className="native-input" value={noteTarget} onChange={(event) => setNoteTarget(event.currentTarget.value as 'auction' | 'room')}>
              <option value="auction">当前竞拍</option>
              <option value="room">当前直播间</option>
            </select>
            <code>{targetID ?? '-'}</code>
          </div>
          <Input.TextArea
            autoSize={{ minRows: 4, maxRows: 6 }}
            placeholder="记录商家侧观察、客服反馈、人工处置原因。会写入 system_control_signals。"
            value={note}
            onChange={setNote}
          />
          <Button type="primary" disabled={!targetID || !note.trim()} onClick={() => void submitNote()}>记录事件</Button>
        </div>
      </section>

      <section className="health-panel health-timeline-panel" data-testid="health-timeline">
        <div className="health-panel-head">
          <span><Clock3 size={15} /> 告警与处置时间线</span>
          <strong>{timelineRows.length} rows</strong>
        </div>
        <div className="health-timeline">
          {timelineRows.length === 0 ? <div className="empty-state compact-empty">暂无告警或控制信号</div> : timelineRows.map((row) => (
            <div className={`health-timeline-row ${row.kind}`} key={`${row.kind}-${row.ref}`}>
              <time>{row.time ? new Date(row.time).toLocaleTimeString() : '-'}</time>
              <Tag color={row.kind === 'alert' ? severityTagColor(row.severity) : 'arcoblue'}>{row.kind}</Tag>
              <div>
                <strong>{row.title}</strong>
                <p>{row.body}</p>
              </div>
              {row.auctionID && row.auctionID.startsWith('auc') ? <Button size="mini" onClick={() => onOpenFlightRecorder(row.auctionID)}>drilldown</Button> : null}
            </div>
          ))}
        </div>
      </section>
    </>
  );
}

function FunnelBar({ label, value, max, caption, muted, tone = 'normal' }: { label: string; value: number; max: number; caption?: string; muted?: boolean; tone?: 'normal' | 'warn' }) {
  const width = muted ? 16 : Math.max(8, Math.min(100, (value / Math.max(max, 1)) * 100));
  return (
    <div className={`funnel-row ${tone} ${muted ? 'muted' : ''}`}>
      <span>{label}</span>
      <div><i style={{ width: `${width}%` }} /></div>
      <strong>{muted ? 'n/a' : value}</strong>
      {caption ? <em>{caption}</em> : null}
    </div>
  );
}

function HealthMetric({ label, value, status }: { label: string; value: React.ReactNode; status: 'ok' | 'warn' | 'bad' }) {
  return (
    <div className={`health-metric metric-${status}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function OrderDetailDrawer({
  order,
  visible,
  onClose,
  onOpenFlightRecorder
}: {
  order?: Order;
  visible: boolean;
  onClose: () => void;
  onOpenFlightRecorder: (auctionID: string) => void;
}) {
  return (
    <Drawer
      className="order-detail-drawer"
      width={560}
      title="订单详情"
      visible={visible}
      onCancel={onClose}
      footer={null}
      unmountOnExit
    >
      {!order ? (
        <div className="empty-state compact-empty">订单不存在或尚未刷新</div>
      ) : (
        <div className="order-detail" data-testid="order-detail-drawer">
          <div className="order-detail-head">
            <div>
              <span>order</span>
              <strong>{order.id}</strong>
            </div>
            <Tag color={order.status === 'PAID' ? 'green' : order.status === 'ORDER_EXPIRED' ? 'red' : 'orange'}>{order.status}</Tag>
          </div>

          <div className="order-detail-grid">
            <div><span>成交价</span><strong>{formatCents(order.amount_cents)}</strong></div>
            <div><span>中标人</span><strong>{maskUser(order.winner_id)}</strong></div>
            <div><span>保证金</span><strong>{formatCents(order.deposit_cents ?? 0)}</strong></div>
            <div><span>保证金状态</span><strong>{order.deposit_status}</strong></div>
            <div><span>支付截止</span><strong>{order.expire_at ? new Date(order.expire_at).toLocaleString() : '-'}</strong></div>
            <div><span>支付完成</span><strong>{order.paid_at ? new Date(order.paid_at).toLocaleString() : '-'}</strong></div>
          </div>

          <div className="order-detail-section">
            <span>Payment provider</span>
            <code>{order.provider_payment_id ?? '尚未发起 provider payment'}</code>
          </div>

          <div className="order-detail-section">
            <span>Linked auction</span>
            <div className="order-linked-row">
              <code>{order.auction_id}</code>
              <Button icon={<ExternalLink size={14} />} onClick={() => onOpenFlightRecorder(order.auction_id)}>打开飞行记录</Button>
            </div>
          </div>

          <div className="order-detail-section">
            <span>Next action</span>
            <p>
              {order.status === 'ORDER_PENDING' && '提醒中标人完成支付；若支付链路异常，查看 flight recorder 中的 payment_events 与 anomaly。'}
              {order.status === 'PAYMENT_INITIATED' && '支付已发起但未收敛，检查 provider_payment_id 与 webhook。'}
              {order.status === 'PAID' && '订单已支付，可进入履约交接。'}
              {order.status === 'ORDER_EXPIRED' && '订单已超时，核查保证金状态与是否有迟到 webhook。'}
              {!['ORDER_PENDING', 'PAYMENT_INITIATED', 'PAID', 'ORDER_EXPIRED'].includes(order.status) && '查看订单、支付事件和竞拍 timeline 后再对外承诺处理结果。'}
            </p>
          </div>
        </div>
      )}
    </Drawer>
  );
}

function DiagnosticsPanel({
  monitor,
  monitorFilter,
  onOpenFlightRecorder,
  onFilterChange
}: {
  monitor: Record<string, MonitorPayload>;
  monitorFilter: { type: string; auctionID: string; userID: string; traceID: string };
  onOpenFlightRecorder: (auctionID: string) => void;
  onFilterChange: React.Dispatch<React.SetStateAction<{ type: string; auctionID: string; userID: string; traceID: string }>>;
}) {
  const engineSummary = redisEngineSummary(monitor.redisEngine);
  const latestAppend = engineSummary.latest_append;
  const latestAppendLabel = latestAppend
    ? `${latestAppend.latest_append_status ?? '-'} · seq ${latestAppend.latest_append_engine_seq ?? '-'} · ${latestAppend.latest_append_topic ?? '-'}:${latestAppend.latest_append_partition ?? '-'}:${latestAppend.latest_append_offset ?? '-'}`
    : '暂无 append marker';
  return (
    <section className="band diagnostics" data-testid="diagnostics">
      <div className="section-title">
        <h2>诊断</h2>
        <span><Database size={16} /> API</span>
      </div>
      <div className="engine-diagnostics" data-testid="redis-engine-summary">
        <span><RadioTower size={14} /> redis_ledger</span>
        <span>pending Redis {engineSummary.pending_redis_decisions}</span>
        <span>append {engineSummary.append_success_count}/{engineSummary.append_failure_count}/{engineSummary.append_unknown_count}</span>
        <span>settlement {engineSummary.pending_settlements}/{engineSummary.failed_settlements}</span>
        <span>lag max {engineSummary.settlement_lag_max_ms}ms</span>
        <span>recovery RTO {engineSummary.last_recovery_rto_ms ?? '-'}ms {engineSummary.last_recovery_status ?? ''}</span>
        <span>paused {engineSummary.paused_auctions}</span>
        <span>{latestAppendLabel}</span>
      </div>
      <div className="monitor-filter" aria-label="monitor-filter">
        <select
          aria-label="monitor-anomaly-type"
          className="native-input"
          value={monitorFilter.type}
          onChange={(event) => onFilterChange((current) => ({ ...current, type: event.currentTarget.value }))}
        >
          <option value="">全部异常</option>
          <option value="AUTH_SESSION_EXPIRED">AUTH_SESSION_EXPIRED</option>
          <option value="ACL_FORBIDDEN">ACL_FORBIDDEN</option>
          <option value="RATE_LIMIT_REDIS_DOWN">RATE_LIMIT_REDIS_DOWN</option>
          <option value="BID_AUCTION_TOO_HOT">BID_AUCTION_TOO_HOT</option>
          <option value="RATE_LIMITED">RATE_LIMITED</option>
          <option value="PAYMENT_WEBHOOK_INVALID_SIGNATURE">PAYMENT_WEBHOOK_INVALID_SIGNATURE</option>
          <option value="PAYMENT_RECONCILE_MISMATCH">PAYMENT_RECONCILE_MISMATCH</option>
        </select>
        <input aria-label="monitor-auction-id" data-testid="monitor-auction-id" className="native-input" placeholder="auction_id" value={monitorFilter.auctionID} onChange={(event) => onFilterChange((current) => ({ ...current, auctionID: event.currentTarget.value }))} />
        <input aria-label="monitor-user-id" data-testid="monitor-user-id" className="native-input" placeholder="user_id" value={monitorFilter.userID} onChange={(event) => onFilterChange((current) => ({ ...current, userID: event.currentTarget.value }))} />
        <input aria-label="monitor-trace-id" data-testid="monitor-trace-id" className="native-input" placeholder="trace_id" value={monitorFilter.traceID} onChange={(event) => onFilterChange((current) => ({ ...current, traceID: event.currentTarget.value }))} />
      </div>
      <Tabs defaultActiveTab="auctions">
        <Tabs.TabPane key="auctions" title="Auctions"><MonitorTable payload={monitor.auctions} empty="暂无竞拍诊断数据" sourceKey="auction_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="redisEngine" title="Redis Engine"><MonitorTable payload={monitor.redisEngine} empty="暂无 redis engine 数据" sourceKey="auction_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="rejects" title="Rejects"><MonitorTable payload={monitor.rejects} empty="暂无拒绝出价" sourceKey="trace_id" icon={<AlertTriangle size={16} />} onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="recovery" title="Recovery"><MonitorTable payload={monitor.recovery} empty="暂无恢复数据" sourceKey="room_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="anomalies" title="Anomalies"><MonitorTable payload={monitor.anomalies} empty="暂无异常" sourceKey="id" icon={<AlertTriangle size={16} />} onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="outbox" title="Outbox"><MonitorTable payload={monitor.outbox} empty="暂无 outbox 数据" sourceKey="outbox_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="watermarks" title="Watermarks"><MonitorTable payload={monitor.outboxWatermarks} empty="暂无 outbox watermark" sourceKey="shard_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="snapshots" title="Snapshots"><MonitorTable payload={monitor.snapshots} empty="暂无 snapshot 记录" sourceKey="request_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="signals" title="Signals"><MonitorTable payload={monitor.signals} empty="暂无 control signal" sourceKey="id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="scheduler" title="Scheduler"><MonitorTable payload={monitor.scheduler} empty="暂无 scheduler 数据" sourceKey="job_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
      </Tabs>
    </section>
  );
}

function rulePayload(rule: RuleDraft) {
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

function NumberField({ label, name, value, min, max, suffix = '分', onChange }: { label: string; name: string; value: number; min: number; max?: number; suffix?: string; onChange: (value: number) => void }) {
  return (
    <Form.Item label={label}>
      <InputNumber aria-label={name} value={value} min={min} max={max} suffix={suffix} onChange={(next) => onChange(Number(next) || min)} />
    </Form.Item>
  );
}

function MonitorTable({ payload, empty, icon, sourceKey, onOpenFlightRecorder }: {
  payload?: MonitorPayload;
  empty: string;
  icon?: React.ReactNode;
  sourceKey: string;
  onOpenFlightRecorder: (auctionID: string) => void;
}) {
  const rows = payload?.items ?? [];
  if (rows.length === 0) return <div className="empty-state">{icon}{empty}</div>;
  const priorityKeys = [
    sourceKey,
    'engine_mode',
    'engine_seq',
    'db_engine_seq',
    'redis_pending_decisions',
    'pending_settlements',
    'failed_settlements',
    'settlement_lag_p99_ms',
    'settlement_lag_max_ms',
    'latest_append_status',
    'latest_append_engine_seq',
    'latest_append_topic',
    'latest_append_partition',
    'latest_append_offset',
    'append_success_count',
    'append_failure_count',
      'append_unknown_count',
      'append_stats_last_status',
      'last_recovery_rto_ms',
      'last_recovery_status',
      'last_recovery_at',
      'checkpoint_topic',
    'checkpoint_partition',
    'checkpoint_next_offset',
    'delivery_state',
    'delivery_message_id',
    'event_key',
    'seq',
    'attempts',
    'max_attempts',
    'redelivery_count',
    'ack_pending_count',
    'oldest_retry_age_ms',
    'slow_pending_bytes',
    'max_queue_bytes',
    'status',
    'error_class'
  ];
  const keys = Array.from(new Set([
    ...priorityKeys.filter((key) => key in rows[0]),
    ...Object.keys(rows[0])
  ])).slice(0, 14);
  return (
    <Table
      rowKey={(record) => String(record.id ?? record.auction_id ?? record.outbox_id ?? record.job_id ?? record.room_id)}
      data={rows}
      pagination={false}
      columns={[
        {
          title: 'source',
          dataIndex: sourceKey,
          render: (value, record) => {
            const auctionID = rowAuctionID(record);
            const sourceURL = rowSourceURL(sourceKey, record);
            return auctionID ? (
              <button type="button" className="source-link source-button" onClick={() => onOpenFlightRecorder(auctionID)}>
                <Tag color="arcoblue">{String(value ?? '-')}</Tag>
                <ExternalLink size={13} />
              </button>
            ) : sourceURL ? (
              <a className="source-link" href={sourceURL} target="_blank" rel="noreferrer">
                <Tag color="arcoblue">{String(value ?? '-')}</Tag>
                <ExternalLink size={13} />
              </a>
            ) : <Tag color="arcoblue">{String(value ?? '-')}</Tag>;
          }
        },
        ...keys.filter((key) => key !== sourceKey).map((key) => ({
          title: key,
          dataIndex: key,
          render: (value: unknown) => String(value ?? '-')
        }))
      ]}
    />
  );
}

function FlightRecorderDrawer({
  auctionID,
  loading,
  payload,
  visible,
  onClose
}: {
  auctionID: string;
  loading: boolean;
  payload?: FlightRecorderPayload;
  visible: boolean;
  onClose: () => void;
}) {
  const timeline = payload?.timeline ?? [];
  const summary = payload?.summary;
  return (
    <Drawer
      className="flight-recorder-drawer"
      width={760}
      title="Flight Recorder"
      visible={visible}
      onCancel={onClose}
      footer={null}
      unmountOnExit
    >
      <div className="flight-recorder" data-testid="flight-recorder-drawer">
        <div className="flight-recorder-head">
          <div>
            <span>auction</span>
            <strong>{summary?.auction_id ?? auctionID}</strong>
          </div>
          <div>
            <span>item</span>
            <strong>{summary?.item_title ?? '-'}</strong>
          </div>
          <div>
            <span>status / seq</span>
            <strong>{summary ? `${summary.status} / ${summary.seq}` : '-'}</strong>
          </div>
          <div>
            <span>price</span>
            <strong>{formatCents(summary?.current_price_cents)}</strong>
          </div>
        </div>

        <div className="flight-recorder-counts">
          <span>rules {payload?.rules?.length ?? 0}</span>
          <span>orders {payload?.orders?.length ?? 0}</span>
          <span>payments {payload?.payment_events?.length ?? 0}</span>
          <span>anomalies {payload?.anomalies?.length ?? 0}</span>
          <span>timeline {timeline.length}</span>
        </div>

        {loading ? (
          <div className="empty-state compact-empty">正在读取后端 flight recorder</div>
        ) : timeline.length === 0 ? (
          <div className="empty-state compact-empty">暂无 flight recorder timeline</div>
        ) : (
          <div className="flight-timeline">
            {timeline.map((row, index) => (
              <div className="flight-row" key={`${row.kind}-${row.ref_id}-${index}`}>
                <div className="flight-row-main">
                  <Tag color={timelineTone(row)}>{row.kind}</Tag>
                  <div>
                    <strong>{row.event_type}</strong>
                    <span>{new Date(row.time).toLocaleString()} · ref {row.ref_id}</span>
                  </div>
                  <code>{row.seq !== undefined ? `seq ${row.seq}` : row.status ?? '-'}</code>
                </div>
                <div className="flight-row-meta">
                  {row.user_id ? <span>user {maskUser(row.user_id)}</span> : null}
                  {row.amount_cents !== undefined ? <span>{formatCents(row.amount_cents)}</span> : null}
                  {typeof row.payload?.source === 'string' ? <span>source {row.payload.source}</span> : null}
                  {row.trace_id ? <span>trace {row.trace_id}</span> : null}
                  {row.status ? <span>{row.status}</span> : null}
                </div>
                <div className="flight-row-explain">
                  <div><span>Impact</span><p>{timelineImpact(row)}</p></div>
                  <div><span>Next action</span><p>{timelineNextAction(row)}</p></div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </Drawer>
  );
}

createRoot(document.getElementById('root')!).render(<App />);
