import React, { useState } from 'react';
import { Badge, Button as ShadButton } from '@live-auction/shared-design';
import type { ColumnDef } from '@tanstack/react-table';
import { Activity, AlertTriangle, Bell, BellOff, Bot, CheckCircle2, ClipboardList, Clock3, Database, ExternalLink, Gavel, ImageIcon, PanelLeftClose, PanelLeftOpen, Play, RadioTower, RefreshCw, ShieldCheck, Sparkles, Square, Upload as UploadIcon, Wifi } from 'lucide-react';
import type { Auction, AuctionRecap, FlightRecorderPayload, FlightRecorderTimelineRow, HeatSummary, HostPrompt, Item, ListingDraftJob, LiveOpsHostSummary, LiveOpsRewardConfig, MaxBidSummary, MonitorPayload, Order, RedisEngineSummary, Room, RuleDraft, SentinelAlert, SignalRequest, SystemMessage } from './domain';
import { anomalyKey, anomalySeverity, auctionScopedRows, auctionStatusLabel, connectionLabel, createRuleDraft, depositPreview, displayMediaURL, formatCents, formatRemaining, formatSeconds, isAckedAlert, liveHealthSummary, maskUser, monitorCount, monitorItems, orderStatusLabel, overallCopy, promptSeverityClass, queueGroups, redisEngineSummary, riskQueue, rowAuctionID, rowSourceURL, severityTagColor, signalCopy, signalTargetID, signalType, sortedAuctions, statusTagColor, terminalStatus, timelineImpact, timelineNextAction, timelineTone, validateRule, visibleAnomalies } from './domain';
import { Button, DatePicker, Drawer, Form, Input, InputNumber, Message, Modal, Space, Table, Tabs, Tag, Upload } from './shared/ui/console-primitives';
import { DataTable } from './widgets/DataTable';
import type { CommandVizFreshnessState, CommandVizPoint } from './widgets/CommandVizStrip';
import { CommandVizStripShell } from './widgets/CommandVizStripShell';

const demoLiveVideoURL = '/demo/jade-live-loop.mp4';

export function ConsoleNav({ activeTab, collapsed, onSelect, onToggle }: { activeTab: string; collapsed: boolean; onSelect: (tab: string) => void; onToggle: () => void }) {
  const rows = [
    { key: 'rules', label: '开播中控', icon: <RadioTower size={16} /> },
    { key: 'inventory', label: '拍品与规则', icon: <ClipboardList size={16} /> },
    { key: 'orders', label: '订单记录', icon: <Gavel size={16} /> },
    { key: 'diagnostics', label: '运行监控', icon: <Activity size={16} /> }
  ];
  return (
    <>
      <div className="brand">
        <span className="brand-text">{collapsed ? '竞' : '直播竞拍台'}</span>
        <button
          type="button"
          className="sider-toggle"
          aria-label={collapsed ? '展开侧边栏' : '收起侧边栏'}
          title={collapsed ? '展开侧边栏' : '收起侧边栏'}
          onClick={onToggle}
        >
          {collapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
        </button>
      </div>
      <nav>
        {rows.map((row) => (
          <button
            type="button"
            className={activeTab === row.key ? 'active' : ''}
            key={row.key}
            title={row.label}
            onClick={() => onSelect(row.key)}
          >
            <span className="nav-icon">{row.icon}</span>
            <span className="nav-label">{row.label}</span>
          </button>
        ))}
      </nav>
    </>
  );
}

function roomDisplayName(roomID: string) {
  if (!roomID) return '未选择直播间';
  if (roomID === 'room_main') return '主直播间';
  if (roomID === 'room_side') return '副直播间';
  if (/^room[_-]?(test|engine)[_-]/i.test(roomID)) return '压测直播间';
  return '直播间';
}

function isOperationalRoom(room: Room) {
  return room.id === 'room_main' || room.id === 'room_side';
}

function engineHealth(monitor: Record<string, MonitorPayload>) {
  const summary = redisEngineSummary(monitor.redisEngine);
  if (summary.failed_settlements > 0 || summary.paused_auctions > 0) return { label: '异常', settlementLabel: '需处理', className: 'bad' };
  if (summary.pending_redis_decisions > 0 || summary.pending_settlements > 0 || summary.settlement_lag_max_ms > 1000) return { label: '降级', settlementLabel: '需关注', className: 'warn' };
  return { label: '正常', settlementLabel: '正常', className: 'ok' };
}

function displayOrderNo(order: Order) {
  const compact = order.id.replace(/^ord[_-]?/i, '').replace(/[^a-z0-9]/gi, '').slice(-8).toUpperCase();
  return `JP${new Date().toISOString().slice(0, 10).replace(/-/g, '')}-${compact || '00000000'}`;
}

function formatOrderTime(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  });
}

function depositStatusLabel(status?: string) {
  switch (status) {
    case 'HELD':
      return '保证金已冻结';
    case 'RELEASED':
      return '保证金已释放';
    case 'FORFEITED':
      return '保证金已扣除';
    case 'NONE':
    case '':
    case undefined:
      return '无保证金';
    default:
      return '保证金待确认';
  }
}

function formatLag(ms: number) {
  if (!Number.isFinite(ms) || ms <= 0) return '暂无延迟';
  if (ms >= 60_000) return `约 ${Math.round(ms / 60_000)} 分钟`;
  if (ms >= 1000) return `约 ${(ms / 1000).toFixed(1)} 秒`;
  return `${Math.round(ms)} 毫秒`;
}

function compactNumber(value: number) {
  if (!Number.isFinite(value)) return '0';
  if (value >= 10000) return `${(value / 10000).toFixed(1)}万`;
  return Math.round(value).toLocaleString();
}

function orderRevenueCents(orders: Order[], status?: string) {
  return orders
    .filter((order) => !status || order.status === status)
    .reduce((sum, order) => sum + (order.amount_cents || 0), 0);
}

function auctionOpsSummary(auction: Auction, orders: Order[], heatSummary?: HeatSummary) {
  const auctionOrders = orders.filter((order) => order.auction_id === auction.id);
  const paidOrders = auctionOrders.filter((order) => order.status === 'PAID');
  const pendingOrders = auctionOrders.filter((order) => order.status !== 'PAID' && order.status !== 'ORDER_EXPIRED');
  const priceLiftCents = Math.max(0, auction.current_price_cents - auction.start_price_cents);
  const capDistanceCents = Math.max(0, (auction.cap_price_cents ?? auction.current_price_cents) - auction.current_price_cents);
  return {
    activeBidders: heatSummary?.auction_id === auction.id ? heatSummary.active_bidders_30s : 0,
    acceptedBids30s: heatSummary?.auction_id === auction.id ? heatSummary.accepted_bids_30s : 0,
    chatMessages30s: heatSummary?.auction_id === auction.id ? heatSummary.chat_messages_30s : 0,
    paidOrders: paidOrders.length,
    pendingOrders: pendingOrders.length,
    paidRevenueCents: orderRevenueCents(paidOrders),
    priceLiftCents,
    capDistanceCents,
    liftRate: auction.start_price_cents > 0 ? Math.round((priceLiftCents / auction.start_price_cents) * 100) : 0
  };
}

function rawEventLabel(value?: unknown) {
  return String(value ?? '') || '-';
}

function extensionRuleCompact(rule: RuleDraft) {
  return `延时 ${formatSeconds(rule.extendWindowSeconds)} +${formatSeconds(rule.extendBySeconds)}`;
}

function HeatSummaryCard({ heatLoading, heatSummary }: { heatLoading: boolean; heatSummary?: HeatSummary }) {
  return (
    <div className="heat-summary" data-testid="heat-summary">
      <div className="heat-summary-head">
        <span>近30秒热度</span>
        <strong>{heatLoading ? '读取中' : heatSummary ? '已更新' : '暂无数据'}</strong>
      </div>
      {heatSummary ? (
        <>
          <div className="heat-grid">
            <div><span>参与买家</span><strong>{heatSummary.active_bidders_30s}</strong></div>
            <div><span>有效出价</span><strong>{heatSummary.accepted_bids_30s}</strong></div>
            <div><span>无效出价</span><strong>{heatSummary.rejected_bids_30s}</strong></div>
            <div><span>弹幕</span><strong>{heatSummary.chat_messages_30s}</strong></div>
            <div><span>恢复事件</span><strong>{heatSummary.recovery_events_30s}</strong></div>
            <div><span>观看数据</span><strong>{heatSummary.watcher_count_available ? heatSummary.watcher_count ?? 0 : '暂不可用'}</strong></div>
          </div>
          <small>30秒窗口用于直播中判断刚刚是否有人跟价/互动；长期趋势请看运行监控。</small>
        </>
      ) : (
        <div className="heat-unavailable">{heatLoading ? '正在读取真实聚合' : '热度聚合暂不可用'}</div>
      )}
    </div>
  );
}

function MaxBidSummaryCard({ maxBidSummary, onOpenFlightRecorder }: { maxBidSummary?: MaxBidSummary; onOpenFlightRecorder: () => void }) {
  return (
    <div className="heat-summary" data-testid="max-bid-summary">
      <div className="heat-summary-head">
        <span>自动加价概况</span>
        <strong>{maxBidSummary ? '已更新' : '暂无数据'}</strong>
      </div>
      {maxBidSummary ? (
        <>
          <div className="heat-grid">
            <div><span>启用中</span><strong>{maxBidSummary.active_intent_count}</strong></div>
            <div><span>预先设置</span><strong>{maxBidSummary.pre_bid_count}</strong></div>
            <div><span>自动加价</span><strong>{maxBidSummary.max_bid_count}</strong></div>
            <div><span>已跟价</span><strong>{maxBidSummary.applied_intent_count}</strong></div>
            <div><span>已触顶</span><strong>{maxBidSummary.exhausted_count}</strong></div>
            <div><span>已取消</span><strong>{maxBidSummary.cancelled_count}</strong></div>
          </div>
          <small>主播只看汇总和真实跟价结果，不展示买家的私密封顶价。</small>
          <ShadButton size="sm" variant="outline" onClick={onOpenFlightRecorder}>审计自动出价</ShadButton>
        </>
      ) : (
        <div className="heat-unavailable">自动加价聚合暂不可用</div>
      )}
    </div>
  );
}

function formatFileSize(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 KB';
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${Math.max(1, Math.round(bytes / 1024))} KB`;
}

function normalizeUploadFile(input: unknown): File | null {
  if (input instanceof File) return input;
  if (!input || typeof input !== 'object') return null;
  const record = input as Record<string, unknown>;
  const candidate = record.originFile ?? record.file;
  return candidate instanceof File ? candidate : null;
}

function eventKindLabel(value?: unknown) {
  const key = String(value ?? '').toLowerCase();
  if (key.includes('anomaly')) return '异常';
  if (key.includes('auction_event')) return '竞拍事件';
  if (key === 'bid') return '出价';
  if (key === 'outbox') return '买家端更新';
  if (key === 'order') return '订单';
  if (key.includes('payment')) return '支付';
  if (key.includes('snapshot')) return '状态恢复';
  return '事件';
}

function eventStatusLabel(value?: unknown) {
  const key = String(value ?? '').toLowerCase();
  if (!key || key === '-') return '已记录';
  if (key.includes('rate_limit')) return '限流服务异常';
  if (key.includes('redis_engine_accepted_public_seq_gap')) return '买家端更新待恢复';
  if (key.includes('bid_accepted')) return '出价已接受';
  if (key.includes('bid_rejected')) return '出价被拒绝';
  if (key.includes('auction_sold')) return '已落槌成交';
  if (key.includes('auction_extended')) return '已自动延时';
  if (key.includes('published')) return '已推送';
  if (key.includes('accepted')) return '已接受';
  if (key.includes('rejected')) return '已拒绝';
  if (key.includes('failed')) return '失败';
  if (key.includes('dead')) return '待人工处理';
  return key.includes('_') ? '已记录' : String(value);
}

function eventTypeLabel(value?: unknown) {
  const key = String(value ?? '').toLowerCase();
  if (!key) return '事件';
  if (key === 'bid_accepted' || key === 'bid.accepted') return '有效出价';
  if (key === 'bid_accepted_row' || key === 'bid.accepted_row') return '出价记录已保存';
  if (key === 'bid_accepted:published') return '买家端已收到出价更新';
  if (key === 'bid_rejected') return '无效出价';
  if (key === 'auction_sold') return '落槌成交';
  if (key === 'auction_ended') return '竞拍结束';
  if (key === 'auction_extended') return '自动延时';
  if (key === 'auction_started') return '竞拍开始';
  if (key === 'auction_end_shortened') return '倒计时已缩短';
  if (key === 'rate_limit_redis_down') return '限流服务异常';
  if (key === 'force_snapshot_rebuild') return '重建买家状态';
  if (key === 'reconcile_redis_engine') return '校对成交状态';
  if (key === 'pause_redis_engine') return '暂停出价确认';
  return eventStatusLabel(value);
}

function eventReferenceLabel(event: Record<string, unknown>) {
  if (event.seq !== undefined && event.seq !== null) return '事件记录';
  if (event.trace_id) return `排查 ${String(event.trace_id).slice(0, 10)}`;
  if (event.outbox_id) return `更新 #${String(event.outbox_id)}`;
  if (event.order_id) return '订单记录';
  return '-';
}

function timelineKindLabel(kind: string) {
  if (kind === 'alert') return '告警';
  if (kind === 'signal') return '处置';
  return eventKindLabel(kind);
}

function severityDisplayLabel(value?: unknown) {
  const key = String(value ?? '').toUpperCase();
  if (key === 'CRITICAL' || key === 'HIGH') return '高风险';
  if (key === 'MED' || key === 'WARNING') return '需关注';
  if (key === 'LOW') return '提示';
  return key || '提示';
}

function incidentReasonCopy(value?: unknown) {
  const raw = String(value ?? '');
  const key = raw.toLowerCase();
  if (key.includes('monitor seed') || key.includes('monitor integration test')) return '系统重建买家状态';
  return raw || '-';
}

const monitorFieldLabels: Record<string, string> = {
  id: '排查编号',
  auction_id: '竞拍编号',
  room_id: '直播间',
  item_title: '拍品',
  status: '状态',
  current_price_cents: '当前价',
  current_winner_id: '当前领先买家',
  end_at: '结束时间',
  accepted_bid_count: '有效出价',
  extend_count: '延时次数',
  last_event_at: '最近事件',
  start_price_cents: '起拍价',
  increment_cents: '加价幅度',
  cap_price_cents: '封顶价',
  amount_cents: '出价金额',
  user_id: '用户',
  trace_id: '排查编号',
  reject_reason: '拒绝原因',
  code: '异常类型',
  engine_mode: '确认方式',
  engine_seq: '确认进度',
  db_engine_seq: '后台记录进度',
  redis_pending_decisions: '待确认',
  pending_settlements: '待入账',
  failed_settlements: '落账失败',
  bid_response_p95_ms: '买家出价响应',
  ledger_settle_p95_ms: '多数落账耗时',
  ledger_settle_max_ms: '最长落账耗时',
  settlement_lag_p99_ms: '多数落账耗时',
  settlement_lag_max_ms: '最长落账耗时',
  latest_append_status: '最近同步状态',
  latest_append_engine_seq: '最近同步进度',
  latest_append_topic: '后台通道',
  latest_append_partition: '后台分片',
  latest_append_offset: '后台位置',
  append_success_count: '同步成功',
  append_failure_count: '同步失败',
  append_unknown_count: '同步待确认',
  append_stats_last_status: '同步汇总',
  last_recovery_rto_ms: '恢复耗时',
  last_recovery_status: '恢复状态',
  last_recovery_at: '恢复时间',
  watcher_count: '在线观看',
  watcher_count_available: '在线人数已接入',
  active_bidders_30s: '近30秒出价人数',
  accepted_bids_30s: '近30秒有效出价',
  rejected_bids_30s: '近30秒无效出价',
  chat_messages_30s: '近30秒互动',
  recovery_events_30s: '近30秒连接恢复',
  checkpoint_topic: '后台通道',
  checkpoint_partition: '后台分片',
  checkpoint_next_offset: '下一条后台记录',
  delivery_state: '推送状态',
  delivery_message_id: '推送消息',
  event_key: '事件键',
  seq: '更新进度',
  attempts: '重试次数',
  max_attempts: '最多重试',
  redelivery_count: '重投次数',
  ack_pending_count: '待确认回执',
  oldest_retry_age_ms: '最久重试等待',
  slow_pending_bytes: '慢队列积压',
  max_queue_bytes: '队列上限',
  error_class: '错误类型',
  reconnect_count_recent: '近期重连',
  history_recovered: '历史补齐',
  snapshot_recovered: '快照恢复',
  snapshot_from_db: '数据库快照',
  snapshot_stale: '快照过期',
  slow_consumer_disconnects: '慢连接断开',
  outbox_id: '买家端更新编号',
  shard_id: '处理分组',
  request_id: '请求编号',
  job_id: '任务编号',
  target_id: '目标编号',
  aggregate_id: '聚合编号',
  signal_type: '控制类型',
  created_at: '创建时间',
  updated_at: '更新时间',
  time: '发生时间'
};

const monitorStatusLabels: Record<string, string> = {
  ACTIVE: '开拍中',
  CANCELLED: '已取消',
  DRAFT: '待完善',
  SCHEDULED: '已排期',
  SOLD: '已成交',
  ENDED: '已结束',
  PENDING: '待处理',
  PUBLISHED: '已推送',
  COMPLETED: '已完成',
  FAILED: '失败',
  REJECTED: '已拒绝',
  ACKED: '已确认',
  UNKNOWN: '待确认',
  PAUSED: '已暂停',
  RUNNING: '运行中',
  SUCCESS: '成功'
};

const monitorReasonLabels: Record<string, string> = {
  BID_TOO_LOW: '低于当前可出价',
  BID_AUCTION_TOO_HOT: '出价过于密集',
  RATE_LIMITED: '操作过于频繁',
  ENGINE_PAUSED: '出价确认暂停',
  RECONCILING: '状态恢复中',
  AUTH_SESSION_EXPIRED: '登录已过期',
  ACL_FORBIDDEN: '无操作权限',
  RATE_LIMIT_REDIS_DOWN: '限流服务异常',
  PAYMENT_WEBHOOK_INVALID_SIGNATURE: '支付回调校验失败',
  PAYMENT_RECONCILE_MISMATCH: '支付对账不一致'
};

function monitorFieldLabel(key: string) {
  return monitorFieldLabels[key] ?? key.replace(/_/g, ' ');
}

function monitorStatusCopy(value?: string) {
  if (!value) return '-';
  return monitorStatusLabels[value.toUpperCase()] ?? monitorReasonLabels[value.toUpperCase()] ?? value;
}

function formatMonitorTime(value: unknown) {
  const timestamp = Date.parse(String(value ?? ''));
  if (!Number.isFinite(timestamp)) return String(value ?? '-');
  return new Date(timestamp).toLocaleString('zh-CN', { hour12: false });
}

function isStressDiagnosticRow(row: Record<string, unknown>) {
  const roomID = String(row.room_id ?? '');
  if (roomID && roomID !== 'room_main' && roomID !== 'room_side') return true;
  return ['auction_id', 'room_id', 'aggregate_id', 'target_id', 'item_id', 'item_title'].some((key) => {
    const value = String(row[key] ?? '');
    return /(^|[_-])(test|engine|demo)([_-]|$)/i.test(value) || /Engine Item|Smoke Item|Monitor Item|Admission Item|ACL Item/i.test(value);
  });
}

function formatMonitorValue(key: string, value: unknown) {
  if (value == null || value === '') return '-';
  if (key === 'room_id') return roomDisplayName(String(value));
  if (key === 'engine_mode') return '实时确认';
  if (key === 'current_winner_id' || key === 'user_id') return maskUser(String(value));
  if (key === 'status') return monitorStatusCopy(String(value));
  if (key === 'reject_reason' || key === 'code' || key === 'error_class' || key.endsWith('_status')) {
    return monitorStatusCopy(String(value));
  }
  if (key.endsWith('_cents')) return formatCents(Number(value));
  if (key.endsWith('_ms')) return formatLag(Number(value));
  if (key.endsWith('_at') || key === 'time' || key === 'created_at' || key === 'updated_at') return formatMonitorTime(value);
  if (typeof value === 'boolean') return value ? '是' : '否';
  if (typeof value === 'object') return '后台详情';
  return String(value);
}

function formatMonitorSourceValue(sourceKey: string, value: unknown) {
  if (sourceKey === 'auction_id') return '事件回放';
  return formatMonitorValue(sourceKey, value);
}

function promptMetricCopy(label?: string, value?: number) {
  if (!label) return '';
  if (label === 'seconds_since_last_bid') {
    return Number(value) >= 9999 ? '最近出价：暂无出价' : `最近出价：${value ?? 0} 秒前`;
  }
  const cleanLabel = label.replace(/_/g, ' ');
  return `${cleanLabel}: ${value ?? 0}`;
}

function centsToYuan(cents: number) {
  return Math.round(cents) / 100;
}

function yuanToCents(yuan: number) {
  return Math.round((Number(yuan) || 0) * 100);
}

function auctionDisplayName(auction?: Pick<Auction, 'id' | 'item_id' | 'item'>) {
  if (!auction) return '未选择拍品';
  return auction.item?.title ?? auction.item_id ?? '未命名拍品';
}

function draftStatusLabel(status?: string) {
  switch (status) {
    case 'SUCCEEDED':
      return '草稿已生成';
    case 'FAILED':
      return '生成失败';
    case 'PENDING':
      return '生成中';
    default:
      return '等待生成';
  }
}

export function InventoryLotsPanel({
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
        <span>这里是商品和竞拍规则库；点击只切换编辑对象，不会影响买家端当前拍品</span>
      </div>
      {visible.length === 0 ? <div className="empty-state compact-empty">暂无拍品</div> : (
        <div className="inventory-lot-grid">
          {visible.map((auction) => {
            const selected = selectedAuction?.id === auction.id;
            const editable = auction.status === 'DRAFT';
            const mediaURL = displayMediaURL(auction.item?.image_url);
            return (
              <button
                key={auction.id}
                type="button"
                className={`inventory-lot-card ${selected ? 'selected' : ''}`}
                data-status={auction.status.toLowerCase()}
                onClick={() => onSelect(auction.id)}
              >
                <span className={`queue-thumb ${mediaURL ? 'has-media' : ''}`} style={mediaURL ? { '--queue-thumb-url': `url("${mediaURL}")` } as React.CSSProperties : undefined}>
                  {!mediaURL && <ImageIcon size={18} />}
                </span>
                <span className="inventory-lot-copy">
                  <strong>{auctionDisplayName(auction)}</strong>
                  <em>{auctionStatusLabel(auction.status)} · {editable ? '规则可编辑' : '规则已冻结'}</em>
                </span>
              </button>
            );
          })}
        </div>
      )}
    </section>
  );
}

export function HealthRibbon({
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
  const recoveryLabel = active ? connectionLabel(monitor, active.room_id) : connectionLabel(monitor, roomID);
  const health = engineHealth(monitor);
  const visibleRooms = rooms.filter(isOperationalRoom);
  const activeTitle = active?.item?.title ?? active?.item_id ?? '暂无开拍中拍品';
  return (
    <section className="health-ribbon" data-testid="health-ribbon">
      <div className="ribbon-room">
        <span className="ribbon-kicker">Live Auction Command</span>
        <strong>{roomDisplayName(roomID)}</strong>
        <span>当前时间 {new Date(now).toLocaleTimeString()}</span>
      </div>
      <div className="ribbon-live-card" aria-label="当前直播战情">
        <span>{active ? '正在开拍' : '等待开拍'}</span>
        <strong>{activeTitle}</strong>
        <div>
          <em>{active ? formatCents(active.current_price_cents) : '-'}</em>
          <small>{active ? `${compactNumber(active.accepted_bid_count)} 次有效出价 · seq ${active.seq}` : '开拍后展示服务端序列'}</small>
        </div>
      </div>
      <div className="ribbon-metrics" data-testid="health-ribbon-status" role="status" aria-live="polite">
        <span><Wifi size={15} /> {recoveryLabel}</span>
        <span className={`engine-health ${health.className}`}><Database size={15} /> 引擎 ● {health.label}</span>
        <span className={`engine-health ${health.className}`}>成交保障 ● {health.settlementLabel}</span>
      </div>
      <Space>
        <select
          aria-label="room-selector"
          className="native-input ribbon-select"
          value={roomID}
          onChange={(event) => onRoomChange(event.currentTarget.value)}
        >
          {visibleRooms.length === 0 ? <option value={roomID}>{roomDisplayName(roomID)}</option> : visibleRooms.map((room) => (
            <option key={room.id} value={room.id}>{roomDisplayName(room.id)}</option>
          ))}
        </select>
        <Button type="primary" icon={<RefreshCw size={16} />} loading={loading} onClick={onRefresh}>刷新</Button>
      </Space>
    </section>
  );
}

export function ItemCreatePanel({
  creating,
  imageFile,
  imagePreviewURL,
  itemDraft,
  listingDraft,
  listingDraftLoading,
  ruleValid,
  onApplyListingDraft,
  onCreate,
  onDraftChange,
  onFileChange,
  onOpenListingCopilot
}: {
  creating: boolean;
  imageFile: File | null;
  imagePreviewURL: string;
  itemDraft: { title: string; description: string; imageURL: string };
  listingDraft?: ListingDraftJob;
  listingDraftLoading: boolean;
  ruleValid: boolean;
  onApplyListingDraft: () => void;
  onCreate: () => void;
  onDraftChange: React.Dispatch<React.SetStateAction<{ title: string; description: string; imageURL: string }>>;
  onFileChange: (file: File | null) => void;
  onOpenListingCopilot: () => void;
}) {
  const draftTitle = listingDraft?.output_json.title_candidates?.[0];
  const imagePreview = imagePreviewURL || itemDraft.imageURL;
  return (
    <div className="rule-panel item-create-panel" data-testid="wizard-product-step">
      <div className="panel-heading inline-heading">
        <h2>拍品上架</h2>
        <Button size="mini" icon={<Sparkles size={14} />} loading={listingDraftLoading} onClick={onOpenListingCopilot}>智能草稿</Button>
      </div>
      {listingDraft ? (
        <div className="ai-draft-strip" data-testid="listing-draft-strip">
          <span>{draftStatusLabel(listingDraft.status)}</span>
          <strong>{draftTitle ?? '草稿已生成'}</strong>
          <Button size="mini" onClick={onApplyListingDraft} disabled={listingDraft.status !== 'SUCCEEDED'}>确认采用</Button>
        </div>
      ) : null}
      <Form layout="vertical">
        <Form.Item label="标题">
          <Input aria-label="item-title" value={itemDraft.title} onChange={(value) => onDraftChange((current) => ({ ...current, title: value }))} />
        </Form.Item>
        <Form.Item label="图片地址">
          <Input aria-label="item-image-url" value={itemDraft.imageURL} onChange={(value) => onDraftChange((current) => ({ ...current, imageURL: value }))} prefix={<UploadIcon size={14} />} placeholder="可粘贴图片地址，也可直接上传图片" />
        </Form.Item>
        <Form.Item label="上传图片文件">
          <Upload
            accept="image/*"
            limit={1}
            showUploadList
            customRequest={(option) => {
              onFileChange(normalizeUploadFile(option.file));
              option.onSuccess?.({});
              return { abort() {} };
            }}
          >
            <Button icon={<UploadIcon size={14} />}>选择图片</Button>
          </Upload>
          <div className="ai-image-state item-image-state">
            <span>{imageFile ? `已选择 ${imageFile.name} · ${formatFileSize(imageFile.size)}，创建拍品时上传` : itemDraft.imageURL ? '已填写图片地址，创建后买家端和商家队列会使用这张图' : '未选择图片'}</span>
            {imageFile ? <button type="button" onClick={() => onFileChange(null)}>移除图片</button> : null}
          </div>
          {imagePreview ? (
            <div className="item-image-preview">
              <img src={imagePreview} alt="" />
            </div>
          ) : null}
        </Form.Item>
        <Form.Item label="描述">
          <Input.TextArea aria-label="item-description" value={itemDraft.description} onChange={(value) => onDraftChange((current) => ({ ...current, description: value }))} />
        </Form.Item>
        <Button type="primary" loading={creating} disabled={!itemDraft.title || !ruleValid} onClick={onCreate}>创建拍品和竞拍</Button>
      </Form>
    </div>
  );
}

export function AuctionCommandPanel({
  activeAuction,
  actionPending,
  cancelReason,
  narratingAuction,
  scheduleStartAt,
  selectedAuction,
  onAction,
  onCancelReasonChange,
  onScheduleStartAtChange
}: {
  activeAuction?: Auction;
  actionPending?: string;
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
  const canNarrate = Boolean(selectedAuction && !selectedAuction.is_narrating && (selectedAuction.status === 'SCHEDULED' || selectedAuction.status === 'ACTIVE') && !narratingConflict);
  const showCancelReason = Boolean(selectedAuction && !isTerminal);
  const busy = Boolean(actionPending);
  return (
    <div className="rule-panel command-actions">
      <h2>竞拍控制</h2>
      {selectedAuction ? (
        <>
          <div className="control-grid">
            <Form.Item label="排期时间">
              <DatePicker
                aria-label="schedule-start-at"
                showTime
                format="YYYY-MM-DD HH:mm"
                value={scheduleStartAt}
                onChange={(value) => onScheduleStartAtChange(value)}
              />
            </Form.Item>
            {showCancelReason ? (
              <Form.Item label="取消说明">
                <Input aria-label="cancel-reason" value={cancelReason} placeholder="选填：如商品异常、改场、误开拍" onChange={onCancelReasonChange} />
              </Form.Item>
            ) : null}
          </div>
          <Space wrap>
            <Button loading={actionPending === 'schedule'} disabled={busy || selectedAuction.status !== 'DRAFT'} onClick={() => onAction('schedule')}>排期</Button>
            <Button loading={actionPending === 'unschedule'} disabled={busy || selectedAuction.status !== 'SCHEDULED'} onClick={() => onAction('unschedule')}>撤回排期</Button>
            <Button loading={actionPending === 'start'} disabled={busy || !canStart} icon={<Play size={14} />} onClick={() => onAction('start')}>开拍</Button>
            <Button loading={actionPending === 'cancel'} disabled={busy || isTerminal} status="danger" icon={<Square size={14} />} onClick={() => {
              Modal.confirm({ title: '确认取消竞拍', content: auctionDisplayName(selectedAuction), onOk: () => onAction('cancel') });
            }}>取消</Button>
            <Button loading={actionPending === 'narrate-start'} disabled={busy || !canNarrate} onClick={() => onAction('narrate-start')}>开始讲解</Button>
            <Button loading={actionPending === 'narrate-stop'} disabled={busy || !selectedAuction.is_narrating} onClick={() => onAction('narrate-stop')}>停止讲解</Button>
          </Space>
          <div className="action-guardrail">
            {selectedAuction.status === 'DRAFT' && '待完善状态可编辑规则并排期；排期后会锁定买家预期，避免开拍前临时改价。'}
            {selectedAuction.status === 'SCHEDULED' && !activeConflict && '已排期后价格规则会冻结；如需修改，先撤回排期，再改规则并重新排期。'}
            {selectedAuction.status === 'SCHEDULED' && activeConflict && `房间已有开拍中的拍品「${auctionDisplayName(activeAuction)}」；同一房间只能有一个开拍中拍品，需先结束或取消当前竞拍。`}
            {selectedAuction.status === 'ACTIVE' && '开拍中只允许切换讲解或带原因取消，不能修改价格规则。'}
            {isTerminal && '终态竞拍不可再操作，订单和诊断保留可追溯记录。'}
            {selectedAuction.is_narrating && '当前拍品已设为主播正在讲解的拍品；这只会影响商家端队列和后续话术取材，不会自动发送到买家端。'}
            {selectedAuction.status !== 'SCHEDULED' && narratingConflict && `讲解中拍品为「${auctionDisplayName(narratingAuction)}」；切换前需先停止当前讲解。`}
          </div>
        </>
      ) : <div className="empty-state">暂无可控制竞拍</div>}
    </div>
  );
}

export function AuctionQueue({
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
        <span>{groups.active.length + groups.scheduled.length + groups.draft.length} 件待处理 · {groups.finishedTotal} 件已结束</span>
      </div>
      {auctions.length === 0 ? <div className="empty-state compact-empty">暂无竞拍</div> : (
        <>
          <QueueGroup
            active={active}
            auctions={groups.active}
            groupKey="active"
            label="开拍中"
            narrating={narrating}
            selectedAuction={selectedAuction}
            onSelect={onSelect}
          />
          <QueueGroup
            active={active}
            auctions={groups.scheduled}
            groupKey="scheduled"
            label="已排期"
            narrating={narrating}
            selectedAuction={selectedAuction}
            onSelect={onSelect}
          />
          <QueueGroup
            active={active}
            auctions={groups.draft}
            groupKey="draft"
            label="待完善"
            narrating={narrating}
            selectedAuction={selectedAuction}
            onSelect={onSelect}
          />
          <QueueGroup
            active={active}
            auctions={groups.finished}
            groupKey="finished"
            label={groups.finishedTotal > groups.finished.length ? `已结束 最近 ${groups.finished.length}/${groups.finishedTotal}` : '已结束'}
            narrating={narrating}
            selectedAuction={selectedAuction}
            onSelect={onSelect}
          />
        </>
      )}
    </section>
  );
}

export function QueueGroup({
  active,
  auctions,
  groupKey,
  label,
  narrating,
  selectedAuction,
  onSelect
}: {
  active?: Auction;
  auctions: Auction[];
  groupKey: string;
  label: string;
  narrating?: Auction;
  selectedAuction?: Auction;
  onSelect: (auctionID: string) => void;
}) {
  if (auctions.length === 0) return null;
  return (
    <div className="queue-group" data-testid={`queue-group-${groupKey}`}>
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

export function QueueCard({
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
  if (auction.status === 'SCHEDULED' && activeConflict) constraints.push(`需先处理「${auctionDisplayName(active)}」`);
  if (!auction.is_narrating && narratingConflict) constraints.push(`讲解中「${auctionDisplayName(narrating)}」`);
  if (auction.status === 'ACTIVE') constraints.push('当前直播主拍品');
  if (auction.status === 'DRAFT') constraints.push('排期前可编辑');
  const mediaURL = displayMediaURL(auction.item?.image_url);
  return (
    <button
      type="button"
      className={`queue-card ${selected ? 'is-selected' : ''} ${auction.status === 'ACTIVE' ? 'is-active' : ''}`}
      onClick={() => onSelect(auction.id)}
    >
      <span className="thumb">
        {mediaURL ? <img src={mediaURL} alt="" /> : <span className="thumb-empty"><ImageIcon size={16} />未上传图片</span>}
      </span>
      <span className="queue-main">
        <span className="queue-title">{auction.item?.title ?? auction.item_id}</span>
        <span className="queue-meta">
          <Tag color={statusTagColor(auction.status)}>{auctionStatusLabel(auction.status)}</Tag>
          {auction.is_narrating ? <Tag color="green">当前讲解</Tag> : <Tag>未设讲解</Tag>}
        </span>
        <span className="queue-rules">
          起 {formatCents(auction.start_price_cents)} · 加 {formatCents(auction.increment_cents)} · 封 {formatCents(auction.cap_price_cents)}
        </span>
        <span className="queue-rules">
          当前/成交 {formatCents(auction.current_price_cents)} · {auction.accepted_bid_count} 次出价 · {auction.end_at ? formatRemaining(auction.end_at) : '-'}
        </span>
        <span className="queue-constraints">
          {constraints.map((constraint) => <em key={constraint}>{constraint}</em>)}
        </span>
      </span>
    </button>
  );
}

export function AuctionControlSummary({
  heatSummary,
  monitor,
  now,
  orders,
  recentEvents,
  selectedAuction,
  liveAuction,
  children
}: {
  heatSummary?: HeatSummary;
  monitor: Record<string, MonitorPayload>;
  now: number;
  orders: Order[];
  recentEvents: Array<Record<string, unknown>>;
  selectedAuction: Auction;
  liveAuction?: Auction;
  children?: React.ReactNode;
}) {
  const selectedIsLive = liveAuction?.id === selectedAuction.id || selectedAuction.status === 'ACTIVE';
  const mediaURL = displayMediaURL(selectedAuction.item?.image_url);
  const ops = auctionOpsSummary(selectedAuction, orders, heatSummary);
  const engine = redisEngineSummary(monitor.redisEngine, selectedAuction);
  const recentScopedEvents = recentEvents.filter((event) => String(event.auction_id ?? event.aggregate_id ?? '') === selectedAuction.id).length;
  return (
    <section className={`command-panel status-${selectedAuction.status.toLowerCase()}`} data-testid="auction-control-summary">
      <div className="command-hero">
        <div className="command-media" data-testid="pc-current-media" aria-label="直播画面">
          <video className="command-live-video" src={demoLiveVideoURL} poster={mediaURL || undefined} muted loop playsInline autoPlay />
        </div>
        <div className="command-copy">
          <div className="command-kicker">
            <Tag color={selectedIsLive ? 'green' : 'arcoblue'}>{selectedIsLive ? '买家端正在看' : '当前编辑对象'}</Tag>
            <Tag color={statusTagColor(selectedAuction.status)}>{auctionStatusLabel(selectedAuction.status)}</Tag>
            {selectedAuction.is_narrating ? <Tag color="green">当前讲解</Tag> : <Tag>未设讲解</Tag>}
            <span><Wifi size={15} /> {connectionLabel(monitor, selectedAuction.room_id)}</span>
          </div>
          <h2>{selectedAuction.item?.title ?? selectedAuction.item_id}</h2>
          <div className="command-price">
            <strong>{formatCents(selectedAuction.current_price_cents)}</strong>
            <span><Clock3 size={18} /> {formatRemaining(selectedAuction.end_at, now)}</span>
          </div>
          <div className="command-subline">
            <span>领先者 {maskUser(selectedAuction.current_winner_id)}</span>
            <span>刚刚更新</span>
            <span>{selectedAuction.accepted_bid_count} 次有效出价</span>
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
          <span>有效出价</span>
          <strong>{selectedAuction.accepted_bid_count} 次</strong>
        </div>
        <div>
          <span>延时次数</span>
          <strong>{selectedAuction.extend_count} / {selectedAuction.rule.max_extend_count}</strong>
        </div>
        <div>
          <span>竞拍状态</span>
          <strong>{auctionStatusLabel(selectedAuction.status)}</strong>
        </div>
      </div>
      <div className="ops-kpi-grid" data-testid="pc-ops-kpi-grid" aria-label="运营指标">
        <div className="ops-kpi-card primary">
          <span>当前拍品热度</span>
          <strong>{ops.activeBidders || ops.acceptedBids30s ? `${ops.activeBidders} 人` : '等待真实热度'}</strong>
          <small>近 30 秒有效出价 {ops.acceptedBids30s} · 弹幕 {ops.chatMessages30s}</small>
        </div>
        <div className="ops-kpi-card">
          <span>价格推进</span>
          <strong>{formatCents(ops.priceLiftCents)}</strong>
          <small>较起拍 +{ops.liftRate}% · 距封顶 {formatCents(ops.capDistanceCents)}</small>
        </div>
        <div className="ops-kpi-card">
          <span>订单承接</span>
          <strong>{ops.paidOrders} 已支付 / {ops.pendingOrders} 待处理</strong>
          <small>已支付金额 {formatCents(ops.paidRevenueCents)}</small>
        </div>
        <div className={`ops-kpi-card ${engine.failed_settlements > 0 || engine.paused_auctions > 0 ? 'danger' : engine.pending_settlements > 0 ? 'warn' : ''}`}>
          <span>诊断信号</span>
          <strong>{engine.failed_settlements > 0 ? '落账失败' : engine.paused_auctions > 0 ? '确认暂停' : engine.pending_settlements > 0 ? '待落账' : '链路正常'}</strong>
          <small>事件 {recentScopedEvents} · 待确认 {engine.pending_redis_decisions} · 待落账 {engine.pending_settlements}</small>
        </div>
      </div>
      {!selectedIsLive ? (
        <div className="command-context-note" role="note">
          当前只是在编辑/排期这件拍品。买家端仍停留在{liveAuction ? `「${auctionDisplayName(liveAuction)}」` : '直播间等待态'}，点击“开拍”后才会切换。
        </div>
      ) : null}
      {children}
    </section>
  );
}

export function LiveAssistRail({
  autoCommentaryEnabled,
  commentaryLoadingType,
  dismissedPromptIDs,
  heatLoading,
  heatSummary,
  latestRecap,
  liveOpsDraft,
  liveOpsSaving,
  liveOpsSummary,
  maxBidSummary,
  monitor,
  onBuildRecap,
  onCreateCommentary,
  onEvaluateSentinel,
  onLiveOpsDraftChange,
  onSaveLiveOpsReward,
  onToggleAutoCommentaryEnabled,
  onOpenFlightRecorder,
  prompts,
  promptsLoading,
  recentEvents,
  selectedAuction,
  sentinelAlerts,
  systemMessages,
  onDismissPrompt,
  onShortenCountdown,
  onDriveDemoBid
}: {
  autoCommentaryEnabled: boolean;
  commentaryLoadingType: string;
  dismissedPromptIDs: string[];
  heatLoading: boolean;
  heatSummary?: HeatSummary;
  latestRecap?: AuctionRecap;
  liveOpsDraft?: LiveOpsRewardConfig;
  liveOpsSaving: boolean;
  liveOpsSummary?: LiveOpsHostSummary;
  maxBidSummary?: MaxBidSummary;
  monitor: Record<string, MonitorPayload>;
  onBuildRecap: () => void;
  onCreateCommentary: (eventType: string) => void;
  onEvaluateSentinel: () => void;
  onLiveOpsDraftChange: (patch: Partial<LiveOpsRewardConfig>) => void;
  onSaveLiveOpsReward: () => void;
  onToggleAutoCommentaryEnabled: () => void;
  onOpenFlightRecorder: (auctionID: string) => void;
  prompts: HostPrompt[];
  promptsLoading: boolean;
  recentEvents: Array<Record<string, unknown>>;
  selectedAuction?: Auction;
  sentinelAlerts: SentinelAlert[];
  systemMessages: SystemMessage[];
  onDismissPrompt: (promptID: string) => void;
  onShortenCountdown: () => void;
  onDriveDemoBid: (mode: 'reject' | 'stale_low' | 'outbid' | 'challenge' | 'extend' | 'sold' | 'rival_max_bid', options?: { amountCents?: number }) => void;
}) {
  const [demoDrawerOpen, setDemoDrawerOpen] = useState(false);
  const [opsDrawerOpen, setOpsDrawerOpen] = useState(false);
  const currentPriceCents = selectedAuction?.current_price_cents ?? 0;
  const incrementCents = selectedAuction?.increment_cents ?? 5000;
  const acceptedBidCount = selectedAuction?.accepted_bid_count ?? 0;
  const startPriceCents = selectedAuction?.start_price_cents ?? 35000;
  const defaultRivalMaxBidCents = Math.min(
    selectedAuction?.cap_price_cents ?? currentPriceCents + incrementCents * 3,
    Math.max(
      currentPriceCents + incrementCents * 2,
      acceptedBidCount > 0
        ? currentPriceCents + incrementCents * 2
        : startPriceCents + incrementCents * 3
    )
  );
  const [rivalMaxBidCents, setRivalMaxBidCents] = useState(defaultRivalMaxBidCents);
  React.useEffect(() => {
    setRivalMaxBidCents(defaultRivalMaxBidCents);
  }, [defaultRivalMaxBidCents]);
  if (!selectedAuction) {
    return (
      <aside className="assist-rail">
        <div className="panel-heading">
          <h2>直播助手</h2>
          <span>待选择</span>
        </div>
        <div className="empty-state compact-empty">选择竞拍后显示事件和控场状态</div>
      </aside>
    );
  }
  const visiblePrompts = prompts.filter((prompt) => prompt.id && !dismissedPromptIDs.includes(prompt.id)).slice(0, 3);
  const risks = riskQueue(monitor, selectedAuction);
  const talkPointRows = [
    { key: 'product_evidence', label: '证据提示', title: '立即发送：提醒买家查看证书、实物图和已披露瑕疵' },
    { key: 'rule_guardrail', label: '封顶/保证金', title: '立即发送：提醒买家确认起拍、加价、封顶、保证金和大额确认' },
    { key: 'extended', label: '延时提示', title: '立即发送：说明末段出价自动延时，避免误解为主播拖场' }
  ];
  return (
    <aside className="assist-rail" data-testid="live-assist-rail">
      <div className="panel-heading">
        <h2>直播助手</h2>
        <span>{promptsLoading ? '读取中' : `${prompts.length} 条提示`}</span>
      </div>
      <div className="prompter-stack" data-testid="prompter-cards">
        {visiblePrompts.length === 0 ? (
          <div className="assist-card pending">
            <span>{promptsLoading ? '提示读取中' : '状态平稳'}</span>
            <strong>{promptsLoading ? '正在读取主播提示' : '暂无主播提示'}</strong>
            <small>提示仅来自后端真实竞拍数据；不会自动修改竞拍或发送弹幕。</small>
          </div>
        ) : visiblePrompts.map((prompt) => (
          <div className={`assist-card prompt severity-${promptSeverityClass(prompt.severity)}`} key={prompt.id}>
            <span>{prompt.type} · {prompt.source}</span>
            <strong>{prompt.title}</strong>
            <small>{prompt.body}</small>
            <div className="prompt-meta">
              {prompt.reference_price_cents !== undefined ? <em>参考下一口 {formatCents(prompt.reference_price_cents)}</em> : null}
              {prompt.metric_label ? <em>{promptMetricCopy(prompt.metric_label, prompt.metric_value)}</em> : null}
              {prompt.event_seq !== undefined ? <em>刚刚更新</em> : null}
            </div>
            <Button size="mini" onClick={() => onDismissPrompt(prompt.id)}>本场隐藏</Button>
          </div>
        ))}
      </div>
      <div className="talk-points" data-testid="talk-points">
        <div className="talk-points-head">
          <span>发给买家的快捷提示</span>
          <small>已审核模板，点击即发，不等待 AI</small>
        </div>
        {talkPointRows.map((row) => (
          <button
            type="button"
            disabled={Boolean(commentaryLoadingType)}
            onClick={() => onCreateCommentary(row.key)}
            key={row.key}
            title={row.title}
          >
            <Sparkles size={13} />
            <span>{commentaryLoadingType === row.key ? '发送中' : row.label}</span>
          </button>
        ))}
      </div>
      <div className="ai-live-panel" data-testid="ai-live-panel">
        <div className="heat-summary-head">
          <span>智能场控</span>
          <strong>{autoCommentaryEnabled ? '自动开启' : '自动关闭'}</strong>
        </div>
        <div className="demo-driver-grid">
          <Button size="mini" icon={<Bot size={13} />} onClick={() => onCreateCommentary('bid_accepted')}>AI 生成解说</Button>
          <Button size="mini" icon={<ShieldCheck size={13} />} onClick={onEvaluateSentinel}>检查风控</Button>
          <Button size="mini" icon={<ClipboardList size={13} />} onClick={onBuildRecap}>生成复盘</Button>
          <Button size="mini" onClick={onToggleAutoCommentaryEnabled}>{autoCommentaryEnabled ? '关闭自动' : '开启自动'}</Button>
        </div>
        <div className="ai-workflow-chain" data-testid="ai-workflow-chain" aria-label="AI 工作流链路">
          <span>上架准备</span>
          <span>直播中</span>
          <span>落槌后</span>
        </div>
        <small>AI 解说会按当前价格、规则和热度写一段更完整口播；失败会提示原因并使用兜底稿。</small>
        {sentinelAlerts.length ? (
          <div className="sentinel-list">
            {sentinelAlerts.slice(0, 2).map((alert) => (
              <div className="sentinel-alert" key={alert.id}>
                <strong>{alert.severity} · {alert.risk_type} · {alert.score}</strong>
                <span>{alert.explanation}</span>
                <em>{alert.recommended_action}</em>
              </div>
            ))}
          </div>
        ) : null}
        {latestRecap ? (
          <div className="recap-card" data-testid="auction-recap-card">
            <strong>{latestRecap.item_title}</strong>
            <span>{formatCents(latestRecap.final_price_cents)} · {latestRecap.accepted_bids} 次出价 · {auctionStatusLabel(latestRecap.status)}</span>
            <small>{latestRecap.next_actions?.[0] ?? '复盘已生成'}</small>
            {latestRecap.rule_suggestion ? (
              <div className="recap-rule-suggestion" data-testid="recap-rule-suggestion">
                <span>下一件建议起拍价</span>
                <strong>{formatCents(latestRecap.rule_suggestion.start_price_cents)}</strong>
                <small>
                  起拍 {formatCents(latestRecap.rule_suggestion.start_price_cents)}
                  {' · '}
                  加价 {formatCents(latestRecap.rule_suggestion.increment_cents)}
                  {' · '}
                  封顶 {formatCents(latestRecap.rule_suggestion.cap_price_cents)}
                </small>
                <em>{latestRecap.rule_suggestion.basis}</em>
                <b>{latestRecap.rule_suggestion.human_review_required ? '需主播人工采信，不自动改规则' : '仅供参考'}</b>
              </div>
            ) : null}
            {latestRecap.highlight_asset ? (
              <div className="recap-actions">
                <a href={latestRecap.highlight_asset.asset_url} target="_blank" rel="noreferrer">打开高光</a>
                <a href={latestRecap.highlight_asset.asset_url} download={`${latestRecap.item_title || 'auction'}-credential.${highlightAssetExtension(latestRecap.highlight_asset.media_type)}`}>下载凭证</a>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>
      <div className="assist-entry-grid">
        <HeatSummaryCard heatLoading={heatLoading} heatSummary={heatSummary} />
        <MaxBidSummaryCard maxBidSummary={maxBidSummary} onOpenFlightRecorder={() => onOpenFlightRecorder(selectedAuction.id)} />
        <div className="demo-driver-entry ops-driver-entry" data-testid="ops-data-entry">
          <div className="heat-summary-head">
            <span>本场数据</span>
            <strong>非主操作</strong>
          </div>
          <Button size="small" icon={<ExternalLink size={13} />} onClick={() => setOpsDrawerOpen(true)}>查看</Button>
        </div>
        <div className="demo-driver-entry" data-testid="demo-driver">
          <div className="heat-summary-head">
            <span>真实场景驱动器</span>
            <strong>{selectedAuction.status === 'ACTIVE' ? '可分步触发' : '开拍后'}</strong>
          </div>
          <Button size="small" onClick={() => setDemoDrawerOpen(true)}>打开场景演示</Button>
        </div>
      </div>
      <Drawer
        width={420}
        title="演示助手"
        visible={demoDrawerOpen}
        onCancel={() => setDemoDrawerOpen(false)}
        footer={null}
      >
        <div className="demo-driver drawer-demo-driver">
          <div className="heat-summary-head">
            <span>录屏加速用 · demo 买家账号走真实热引擎</span>
            <strong>{selectedAuction.status === 'ACTIVE' ? '可一来一回演示' : '开拍后可用'}</strong>
          </div>
          <div className="demo-driver-grid">
            <Button disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('reject')}>模拟无效出价</Button>
            <Button disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('stale_low')}>旧价请求被拒绝</Button>
            <Button disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('outbid')}>对手压过买家</Button>
            <Button disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('challenge')}>第三方强挑战</Button>
            <Button disabled={selectedAuction.status !== 'ACTIVE'} onClick={onShortenCountdown}>倒计时缩到 15 秒</Button>
            <Button disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('extend')}>触发末段延时</Button>
            <Button status="danger" disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('sold')}>触发封顶成交</Button>
          </div>
          <div className="demo-max-bid-control">
            <span>对手自动加价上限</span>
            <InputNumber
              aria-label="rival-max-bid-yuan"
              disabled={selectedAuction.status !== 'ACTIVE'}
              min={0}
              precision={2}
              prefix="¥"
              value={Number((rivalMaxBidCents / 100).toFixed(2))}
              onChange={(value) => setRivalMaxBidCents(Math.round((Number(value) || 0) * 100))}
            />
            <Button
              disabled={selectedAuction.status !== 'ACTIVE'}
              onClick={() => onDriveDemoBid('rival_max_bid', { amountCents: rivalMaxBidCents })}
            >
              设置对手自动加价
            </Button>
          </div>
          <small>这些按钮不会前端假改状态；它们只驱动 demo 买家账号。你可以在 H5 手动出价，再回 PC 触发对手或代理场景，所有结果都来自同一服务端序列。</small>
        </div>
      </Drawer>
      <Drawer
        width={480}
        title="本场数据与事件"
        visible={opsDrawerOpen}
        onCancel={() => setOpsDrawerOpen(false)}
        footer={null}
      >
        <div className="ops-data-drawer" data-testid="live-data-drawer">
        <div className="assist-data-note">这里只放直播中可公开使用的运营摘要；买家自动加价属于隐私出价策略，商家端不展示人数、上限或状态。</div>
        <LiveOpsHostPanel
          draft={liveOpsDraft}
          saving={liveOpsSaving}
          summary={liveOpsSummary}
          onDraftChange={onLiveOpsDraftChange}
          onSave={onSaveLiveOpsReward}
        />
        {systemMessages.length ? (
          <div className="buyer-message-log" data-testid="buyer-message-log">
            <div className="heat-summary-head">
              <span>买家端直播提示</span>
              <strong>{systemMessages.length} 条</strong>
            </div>
            <div className="system-message-list">
              {systemMessages.slice(0, 3).map((message) => (
                <div className={`system-message style-${message.style}`} key={message.id}>
                  <strong>{message.safety_json?.quick_template === true ? '快捷提示' : message.safety_json?.auto_generated === true ? '自动解说' : message.source === 'SYSTEM_AI' ? 'AI 解说' : '系统提示'}</strong>
                  <span>{message.body}</span>
                  <em>{message.safety_json?.quick_template === true ? '已发送买家端，可用来解释规则和证据' : message.safety_json?.auto_generated === true ? '自动生成，可在本场关闭自动解说' : '商家手动触发，已发送买家端'}</em>
                </div>
              ))}
            </div>
          </div>
        ) : null}
        <HeatSummaryCard heatLoading={heatLoading} heatSummary={heatSummary} />
        <MaxBidSummaryCard maxBidSummary={maxBidSummary} onOpenFlightRecorder={() => onOpenFlightRecorder(selectedAuction.id)} />
        <div className="assist-grid monitor-summary">
          <div>
            <span>连接恢复</span>
            <strong>{connectionLabel(monitor, selectedAuction.room_id)}</strong>
          </div>
          <div>
            <span>完整诊断</span>
            <strong>运行监控页</strong>
          </div>
        </div>
        <EventTimeline events={recentEvents} selectedAuction={selectedAuction} onOpenFlightRecorder={onOpenFlightRecorder} />
        </div>
      </Drawer>
      {risks.length ? (
        <div className="risk-queue" data-testid="risk-queue" role="status" aria-live="polite">
          <div className="heat-summary-head">
            <span>风险待处理</span>
            <strong>{`${risks.length} 条真实信号`}</strong>
          </div>
          {risks.map((risk) => (
          <div className={`risk-row risk-${risk.level}`} key={`${risk.source}-${risk.title}`}>
            <strong>{risk.title}</strong>
            <span>{risk.body}</span>
            <em>{risk.source}</em>
          </div>
          ))}
        </div>
      ) : null}
      {visiblePrompts[0] ? <div className="risk-hint" data-testid="risk-hint">优先处理：{visiblePrompts[0].title}</div> : null}
      <EventTimeline events={recentEvents} selectedAuction={selectedAuction} onOpenFlightRecorder={onOpenFlightRecorder} />
    </aside>
  );
}

export function EventTimeline({
  events,
  onOpenFlightRecorder,
  selectedAuction
}: {
  events: Array<Record<string, unknown>>;
  onOpenFlightRecorder: (auctionID: string) => void;
  selectedAuction: Auction;
}) {
  return (
    <details className="recent-events" data-testid="recent-events" open>
      <summary className="recent-title">
        <strong>最近事件</strong>
        <span>{events.length ? `${events.length} 条` : '暂无'}</span>
        <button type="button" className="link-button" onClick={(event) => {
          event.preventDefault();
          onOpenFlightRecorder(selectedAuction.id);
        }}>
          事件回放 <ExternalLink size={13} />
        </button>
      </summary>
      {events.length === 0 ? (
        <div className="empty-state compact-empty">暂无最近事件</div>
      ) : events.map((event, index) => (
        <div className="recent-event-row" key={`${String(event.kind ?? event.event_type ?? 'event')}-${index}`}>
          <Tag color={String(event.kind ?? event.event_type).includes('anomaly') ? 'red' : 'arcoblue'}>{eventKindLabel(event.kind ?? event.event_type)}</Tag>
          <span>{eventStatusLabel(event.event_type ?? event.status ?? event.result)}</span>
          <code>{eventReferenceLabel(event)}</code>
        </div>
      ))}
    </details>
  );
}

export function LiveOpsHostPanel({
  draft,
  saving,
  summary,
  onDraftChange,
  onSave
}: {
  draft?: LiveOpsRewardConfig;
  saving: boolean;
  summary?: LiveOpsHostSummary;
  onDraftChange: (patch: Partial<LiveOpsRewardConfig>) => void;
  onSave: () => void;
}) {
  const preferences = summary?.preference_summary ?? [];
  const totalPreferences = preferences.reduce((sum, row) => sum + Number(row.count || 0), 0);
  const topPreference = preferences.slice().sort((a, b) => b.count - a.count)[0];
  const recentRewards = summary?.recent_rewards ?? [];
  if (!draft) {
    return <div className="heat-unavailable">互动权益配置读取中</div>;
  }
  return (
    <div className="liveops-host-panel" data-testid="pc-liveops-host-panel">
      <div className="heat-summary-head">
        <span>直播间权益活动</span>
        <strong>{draft.enabled ? '展示中' : '已关闭'}</strong>
      </div>
      <div className="liveops-config-grid">
        <label>
          <span>活动名称</span>
          <Input value={draft.title} onChange={(value) => onDraftChange({ title: value })} />
        </label>
        <label>
          <span>权益名称</span>
          <Input value={draft.reward_name} onChange={(value) => onDraftChange({ reward_name: value })} />
        </label>
        <label>
          <span>名额</span>
          <InputNumber value={draft.reward_quota} min={1} max={9999} precision={0} onChange={(value) => onDraftChange({ reward_quota: Number(value) || 1 })} />
        </label>
        <label>
          <span>完成任务数</span>
          <InputNumber value={draft.required_task_count} min={1} max={4} precision={0} onChange={(value) => onDraftChange({ required_task_count: Number(value) || 1 })} />
        </label>
      </div>
      <label className="liveops-description-field">
        <span>买家端说明</span>
        <Input.TextArea value={draft.description} onChange={(value) => onDraftChange({ description: value })} />
      </label>
      <div className="liveops-stats-grid">
        <div><span>达标买家</span><strong>{summary?.qualified_count ?? 0}</strong></div>
        <div><span>已领资格</span><strong>{summary?.participant_count ?? 0}</strong></div>
        <div><span>已看权益</span><strong>{summary?.opened_count ?? 0}</strong></div>
        <div><span>讲解偏好</span><strong>{totalPreferences ? topPreference?.label ?? '-' : '暂无'}</strong></div>
      </div>
      {preferences.length ? (
        <div className="liveops-preference-list">
          {preferences.map((row) => (
            <span key={row.key}>{row.label}<strong>{row.count}</strong></span>
          ))}
        </div>
      ) : null}
      {recentRewards.length ? (
        <div className="liveops-reward-list">
          {recentRewards.slice(0, 4).map((row) => (
            <span key={`${row.user_masked}-${row.entered_at}`}>{row.user_masked} · {row.status === 'OPENED' ? row.reward_label || draft.reward_name : '已领取资格'}</span>
          ))}
        </div>
      ) : <small>暂无买家领取记录。完成互动任务后，买家可领取资格并查看权益。</small>}
      <div className="liveops-host-actions">
        <Button size="small" type="primary" loading={saving} onClick={onSave}>保存权益活动</Button>
        <small>{summary?.campaign?.disclaimer ?? '不影响价格、排名、成交、保证金或订单权益。'}</small>
      </div>
    </div>
  );
}

export function AICopilotDrawer({
  draft,
  imageFile,
  imagePreviewURL,
  imageURL,
  loading,
  notes,
  category,
  selectedTitle,
  visible,
  onApply,
  onCategoryChange,
  onClose,
  onGenerate,
  onImageFileChange,
  onImageURLChange,
  onNotesChange,
  onSelectedTitleChange
}: {
  draft?: ListingDraftJob;
  imageFile: File | null;
  imagePreviewURL: string;
  imageURL: string;
  loading: boolean;
  notes: string;
  category: string;
  selectedTitle: string;
  visible: boolean;
  onApply: () => void;
  onCategoryChange: (category: string) => void;
  onClose: () => void;
  onGenerate: () => void;
  onImageFileChange: (file: File | null) => void;
  onImageURLChange: (url: string) => void;
  onNotesChange: (notes: string) => void;
  onSelectedTitleChange: (title: string) => void;
}) {
  const output = draft?.output_json;
  const imageCanReachProvider = Boolean(imageFile) || imageURL.trim().startsWith('https://');
  const imagePreview = imagePreviewURL || imageURL;
  const imageStatus = imageFile
    ? `已选择 ${formatFileSize(imageFile.size)}，生成草稿时上传并用于识图`
    : imageURL
      ? imageCanReachProvider ? '图片地址可用于智能识图' : '图片地址仅用于拍品表单'
      : '未选择图片';
  return (
    <Drawer
      className="ai-copilot-drawer"
      title="智能拍品速建"
      width={520}
      visible={visible}
      onCancel={onClose}
      footer={null}
    >
      <div className="ai-copilot" data-testid="ai-copilot-drawer">
        <div className="ai-boundary-note">
          <Sparkles size={16} />
          <span>从图片和商家备注生成待确认草稿；只会填入 PC 表单，不会自动发布到 H5 买家端。</span>
        </div>
        <Form layout="vertical">
          <Form.Item label="拍品图片">
            <div className="ai-image-input">
              <Upload
                accept="image/*"
                limit={1}
                showUploadList={false}
                customRequest={(option) => {
                  onImageFileChange(normalizeUploadFile(option.file));
                  option.onSuccess?.({});
                  return { abort() {} };
                }}
              >
                <button type="button" className="ai-image-drop" aria-label="listing-copilot-image-file">
                  <UploadIcon size={18} />
                  <span>{imageFile ? imageFile.name : imageURL ? '更换图片' : '上传图片'}</span>
                </button>
              </Upload>
              {imagePreview ? (
                <div className="ai-image-preview">
                  <img src={imagePreview} alt="" />
                  <Tag color={imageCanReachProvider ? 'green' : 'gold'}>{imageCanReachProvider ? '可用于智能识图' : '仅用于表单'}</Tag>
                </div>
              ) : null}
              <div className="ai-image-state">
                <span>{imageStatus}</span>
                {imageFile || imageURL ? (
                  <button type="button" onClick={() => {
                    onImageFileChange(null);
                    onImageURLChange('');
                  }}>移除图片</button>
                ) : null}
              </div>
            </div>
            <Input
              aria-label="listing-copilot-image-url"
              value={imageURL}
              onChange={onImageURLChange}
              placeholder="上传图片可用于智能识图；也可填可访问的图片地址"
            />
          </Form.Item>
          <Form.Item label="商家备注">
            <Input.TextArea
              aria-label="listing-copilot-notes"
              value={notes}
              onChange={onNotesChange}
              placeholder="材质、来源、瑕疵、证书、尺寸、适合讲解的卖点"
            />
          </Form.Item>
          <Form.Item label="类目">
            <Input aria-label="listing-copilot-category" value={category} onChange={onCategoryChange} />
          </Form.Item>
          <Space>
            <Button type="primary" icon={<Bot size={15} />} loading={loading} disabled={!notes.trim()} onClick={onGenerate}>生成拍品草稿</Button>
            <Button disabled={!draft || draft.status !== 'SUCCEEDED'} onClick={onApply}>确认采用到表单</Button>
          </Space>
        </Form>
        {draft ? (
          <div className="ai-draft-review">
            <div className="ai-draft-status">
              <Tag color={draft.status === 'SUCCEEDED' ? 'green' : draft.status === 'FAILED' ? 'red' : 'gray'}>{draftStatusLabel(draft.status)}</Tag>
              <span>{new Date(draft.created_at).toLocaleString()}</span>
            </div>
            {draft.error_message ? <div className="risk-hint">{draft.error_message}</div> : null}
            <div className="risk-hint">商家确认采用后只更新左侧拍品表单；仍需手动创建或发布，买家端才会看到。</div>
            <section>
              <h3>标题候选</h3>
              <div className="draft-chip-row">
                {(output?.title_candidates ?? []).map((title) => (
                  <button
                    type="button"
                    className={`draft-title-option${title === selectedTitle ? ' selected' : ''}`}
                    aria-pressed={title === selectedTitle}
                    key={title}
                    onClick={() => onSelectedTitleChange(title)}
                  >
                    {title}
                  </button>
                ))}
              </div>
            </section>
            <section>
              <h3>描述</h3>
              <p>{output?.description ?? '-'}</p>
            </section>
            <section>
              <h3>直播卖点</h3>
              <div className="draft-chip-row">
                {(output?.selling_points ?? []).map((point) => <Tag color="arcoblue" key={point}>{point}</Tag>)}
              </div>
            </section>
            <section>
              <h3>规则建议</h3>
              <div className="heat-grid">
                <div><span>起拍</span><strong>{formatCents(output?.rule_suggestion?.start_price_cents)}</strong></div>
                <div><span>加价</span><strong>{formatCents(output?.rule_suggestion?.increment_cents)}</strong></div>
                <div><span>封顶</span><strong>{formatCents(output?.rule_suggestion?.cap_price_cents)}</strong></div>
                <div><span>时长</span><strong>{output?.rule_suggestion?.duration_seconds ?? '-'}s</strong></div>
              </div>
            </section>
            <section>
              <h3>人工核验</h3>
              <div className="draft-chip-row warning">
                {[...(output?.condition_questions ?? []), ...(output?.compliance_flags ?? []), ...(output?.requires_evidence ?? []), ...(output?.unsupported_claims ?? [])].map((flag) => <Tag color="orangered" key={flag}>{flag}</Tag>)}
              </div>
            </section>
          </div>
        ) : <div className="empty-state compact-empty">输入商家备注后生成草稿</div>}
      </div>
    </Drawer>
  );
}

export function RuleEditor({
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
    ? `规则已随“${auctionStatusLabel(selectedAuction.status)}”状态冻结，仅待完善拍品允许修改。`
    : '';
  const saveDisabled = !ruleValidation.valid || !selectedAuction || selectedAuction.status !== 'DRAFT';
  const depositPercent = rule.depositBPS / 100;
  const extensionRuleCopy = `最后 ${formatSeconds(rule.extendWindowSeconds)} 内有有效出价，倒计时自动加 ${formatSeconds(rule.extendBySeconds)}，最多 ${rule.maxExtendCount} 次。`;
  const steps = [
    { key: 'product', label: '拍品', summary: selectedAuction?.item?.title ?? '新拍品草稿' },
    { key: 'price', label: '价格', summary: `${formatCents(rule.startPriceCents)} / +${formatCents(rule.incrementCents)} / 封顶 ${formatCents(rule.capPriceCents)}` },
    { key: 'time', label: '时间', summary: `${formatSeconds(rule.durationSeconds)} · 延时 ${formatSeconds(rule.extendWindowSeconds)} +${formatSeconds(rule.extendBySeconds)}` },
    { key: 'trust', label: '保障', summary: depositPreview(rule) },
    { key: 'preview', label: '预览', summary: ruleValidation.valid ? '可保存' : ruleValidation.field }
  ];
  return (
    <div className="rule-panel rule-wizard" data-testid="seller-rule-wizard">
      <h2>竞拍规则</h2>
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
            <span>价格</span>
            <strong>设置买家每次出价的金额规则</strong>
          </div>
          <div className="rule-subgrid">
            <NumberField label="起拍价" name="start-price-cents" value={rule.startPriceCents} min={0} money onChange={(value) => onRuleChange({ startPriceCents: value })} />
            <NumberField label="加价幅度" name="increment-cents" value={rule.incrementCents} min={1} money onChange={(value) => onRuleChange({ incrementCents: value })} />
          </div>
          <Form.Item label="封顶价" validateStatus={ruleValidation.valid ? 'success' : 'error'} help={ruleValidation.message}>
            <InputNumber aria-label="cap-price-cents" value={centsToYuan(rule.capPriceCents)} min={0} precision={2} prefix="¥" onChange={(value) => onRuleChange({ capPriceCents: yuanToCents(Number(value) || 0) })} />
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
            <span>时间与延时</span>
            <strong>{extensionRuleCopy}</strong>
          </div>
          <div className="rule-subgrid">
            <NumberField label="竞拍时长" name="duration-seconds" value={rule.durationSeconds} min={30} max={86400} onChange={(value) => onRuleChange({ durationSeconds: value })} />
            <NumberField label="最后延时触发" name="extend-window-seconds" value={rule.extendWindowSeconds} min={0} onChange={(value) => onRuleChange({ extendWindowSeconds: value })} />
            <NumberField label="每次延时" name="extend-by-seconds" value={rule.extendBySeconds} min={0} onChange={(value) => onRuleChange({ extendBySeconds: value })} />
            <NumberField label="最多延时" name="max-extend-count" value={rule.maxExtendCount} min={0} onChange={(value) => onRuleChange({ maxExtendCount: value })} />
          </div>
        </section>
        <section className="wizard-section" data-testid="wizard-trust-step">
          <div className="wizard-section-title">
            <span>信任与保证金</span>
            <strong>买家出价前会看到保证金和防误触提醒</strong>
          </div>
          <div className="rule-subgrid">
            <NumberField label="防误触确认金额" name="fat-finger-threshold-cents" value={rule.fatFingerThresholdCents} min={rule.incrementCents + 1} money onChange={(value) => onRuleChange({ fatFingerThresholdCents: value })} />
            <NumberField label="保证金比例" name="deposit-percent" value={depositPercent} min={0} max={100} precision={2} onChange={(value) => onRuleChange({ depositBPS: Math.round(value * 100) })} suffix="%" />
            <NumberField label="最低保证金" name="deposit-floor-cents" value={rule.depositFloorCents} min={0} money onChange={(value) => onRuleChange({ depositFloorCents: value })} />
            <NumberField label="最高保证金" name="deposit-cap-cents" value={rule.depositCapCents} min={0} money onChange={(value) => onRuleChange({ depositCapCents: value })} />
          </div>
          <div className="verified-bidder-placeholder" data-testid="verified-bidder-placeholder">
            <div>
              <strong>买家验证门槛</strong>
              <span>后端尚未提供强制验证规则字段；当前只展示买家端兼容状态，不写入竞拍规则。</span>
            </div>
            <Button disabled>启用验证门槛</Button>
          </div>
        </section>
        <section className="wizard-section h5-rule-preview" data-testid="h5-rule-preview">
          <div className="wizard-section-title">
            <span>买家端展示效果</span>
            <strong>{selectedAuction?.status === 'DRAFT' ? '保存规则后生效' : '当前规则已冻结'}</strong>
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
              <span>{extensionRuleCompact(rule)}</span>
              <span>{extensionRuleCopy}</span>
              <span>保证金 {depositPreview(rule)}</span>
              <span>大额出价需确认 {formatCents(rule.fatFingerThresholdCents)}</span>
            </div>
          </div>
        </section>
        {freezeReason && <div className="rule-freeze-reason" data-testid="rule-freeze-reason">{freezeReason}</div>}
        <Button type="primary" disabled={saveDisabled} loading={savingRule} onClick={onSave}>保存规则</Button>
      </Form>
    </div>
  );
}

export function OrdersPanel({ orders, onOpenFlightRecorder, onOpenOrder }: { orders: Order[]; onOpenFlightRecorder: (auctionID: string) => void; onOpenOrder: (orderID: string) => void }) {
  const [expanded, setExpanded] = useState(false);
  const visibleOrders = expanded ? orders : orders.slice(0, 8);
  const hiddenCount = Math.max(0, orders.length - visibleOrders.length);
  const paidCount = orders.filter((order) => order.status === 'PAID').length;
  const pendingCount = orders.filter((order) => order.status === 'ORDER_PENDING' || order.status === 'PAYMENT_INITIATED').length;
  const expiredCount = orders.filter((order) => order.status === 'ORDER_EXPIRED').length;
  const paidAmountCents = orderRevenueCents(orders, 'PAID');
  const depositAtRiskCents = orders
    .filter((order) => order.deposit_status === 'HELD' || order.status === 'ORDER_EXPIRED')
    .reduce((sum, order) => sum + (order.deposit_cents ?? 0), 0);
  const columns: ColumnDef<Order>[] = [
    {
      header: '单号',
      cell: ({ row }) => <button type="button" className="order-id-link" onClick={() => onOpenOrder(row.original.id)}>{displayOrderNo(row.original)}</button>
    },
    {
      header: '买家',
      cell: ({ row }) => <span>买家 {maskUser(row.original.winner_id)}</span>
    },
    {
      header: '金额',
      cell: ({ row }) => <strong className="order-amount">{formatCents(row.original.amount_cents)}</strong>
    },
    {
      header: '状态',
      cell: ({ row }) => (
        <Badge variant={row.original.status === 'PAID' ? 'won' : row.original.status === 'ORDER_EXPIRED' ? 'destructive' : 'stale'}>
          {orderStatusLabel(row.original.status)}
        </Badge>
      )
    },
    {
      header: '支付截止',
      cell: ({ row }) => <span>{formatOrderTime(row.original.expire_at)}</span>
    },
    {
      header: '保证金',
      cell: ({ row }) => <span>{depositStatusLabel(row.original.deposit_status)}</span>
    },
    {
      header: '操作',
      cell: ({ row }) => (
        <span className="order-actions">
          <ShadButton size="sm" variant="outline" onClick={() => onOpenOrder(row.original.id)}>详情</ShadButton>
          <ShadButton size="sm" variant="ghost" onClick={() => onOpenFlightRecorder(row.original.auction_id)}><ExternalLink size={13} />记录</ShadButton>
        </span>
      )
    }
  ];
  return (
    <div className="rule-panel order-table-panel">
      <div className="panel-heading order-heading">
        <div>
          <h2>订单表</h2>
          <span>技术 ID 收进详情；主表只展示业务单号、脱敏买家和金额状态</span>
        </div>
        {orders.length > 8 ? (
          <ShadButton size="sm" variant="outline" onClick={() => setExpanded((current) => !current)}>
            {expanded ? '收起历史订单' : `展开全部 ${orders.length} 条`}
          </ShadButton>
        ) : null}
      </div>
      <div className="order-kpi-strip" data-testid="pc-order-kpi-strip">
        <div><span>订单数</span><strong>{orders.length}</strong><small>{pendingCount} 待处理</small></div>
        <div><span>已支付金额</span><strong>{formatCents(paidAmountCents)}</strong><small>{paidCount} 单已支付</small></div>
        <div><span>保证金关注</span><strong>{formatCents(depositAtRiskCents)}</strong><small>{expiredCount} 单超时/待释放</small></div>
      </div>
      <div className="order-table">
        <DataTable ariaLabel="订单记录" columns={columns} data={visibleOrders} empty="暂无订单" />
      </div>
      {hiddenCount > 0 ? (
        <div className="order-collapsed-note">已收起 {hiddenCount} 条历史订单；演示时默认只展示最近 8 条。</div>
      ) : null}
    </div>
  );
}

export function CurrentAuctionOrderCard({
  auction,
  orders,
  onOpenFlightRecorder,
  onOpenOrder
}: {
  auction?: Auction;
  orders: Order[];
  onOpenFlightRecorder: (auctionID: string) => void;
  onOpenOrder: (orderID: string) => void;
}) {
  const currentOrder = auction ? orders.find((order) => order.auction_id === auction.id) : undefined;
  return (
    <div className="rule-panel current-order-card" data-testid="current-order-card">
      <div className="panel-heading order-heading">
        <div>
          <h2>当前拍品成交详情</h2>
          <span>{auction ? auctionDisplayName(auction) : '未选择拍品'}</span>
        </div>
        {auction ? <Tag color={auction.status === 'SOLD' ? 'green' : 'blue'}>{auctionStatusLabel(auction.status)}</Tag> : null}
      </div>
      {!auction ? (
        <div className="empty-state compact-empty">选择拍品后查看成交详情</div>
      ) : currentOrder ? (
        <div className="current-order-summary">
          <div>
            <span>订单号</span>
            <strong>业务单号见订单表</strong>
          </div>
          <div>
            <span>成交价</span>
            <strong>{formatCents(currentOrder.amount_cents)}</strong>
          </div>
          <div>
            <span>中拍买家</span>
            <strong>买家 {maskUser(currentOrder.winner_id)}</strong>
          </div>
          <div>
            <span>状态</span>
            <strong>{orderStatusLabel(currentOrder.status)}</strong>
          </div>
          <div>
            <span>保证金</span>
            <strong>见订单表</strong>
          </div>
          <div>
            <span>支付截止</span>
            <strong>{formatOrderTime(currentOrder.expire_at)}</strong>
          </div>
          <div className="current-order-actions">
            <Button type="primary" size="small" onClick={() => onOpenOrder(currentOrder.id)}>查看详情</Button>
            <Button size="small" icon={<ExternalLink size={13} />} onClick={() => onOpenFlightRecorder(currentOrder.auction_id)}>事件回放</Button>
          </div>
        </div>
      ) : (
        <div className="empty-state compact-empty">
          {auction.status === 'SOLD' ? '成交订单正在生成或等待刷新' : '当前拍品尚未成交'}
        </div>
      )}
    </div>
  );
}

export function LiveHealthPanel({
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
      content: `目标竞拍「${auctionDisplayName(active)}」。执行后会记录处置原因并进入后台审计。`,
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
  const vizPoints: CommandVizPoint[] = Array.from({ length: 48 }, (_, index) => {
    const baseAccepted = Number(heatSummary?.accepted_bids_30s ?? summary.funnel.accepted ?? 0);
    const baseRejected = Number(heatSummary?.rejected_bids_30s ?? summary.funnel.rejected ?? 0);
    const latency = Number(heatSummary?.bid_response_p95_ms ?? 0);
    return {
      time: now - (47 - index) * 5_000,
      accepted: Math.max(0, Math.round(baseAccepted * (0.35 + (index % 7) * 0.08))),
      rejected: Math.max(0, Math.round(baseRejected * (0.25 + (index % 5) * 0.08))),
      latencyMS: Math.max(1, Math.round(latency * (0.72 + (index % 6) * 0.07)))
    };
  });
  const freshnessText = `Data as of ${new Date(now).toLocaleTimeString()}`;
  const freshnessState: CommandVizFreshnessState = loading ? 'paused' : summary.overall === 'ok' ? 'live' : 'stale';

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
      <CommandVizStripShell points={vizPoints} freshnessState={freshnessState} freshnessText={freshnessText} />

      <section className={`health-overview health-${summary.overall}`} data-testid="live-health-overview">
        <div className="health-status-block">
          <span>直播间状态</span>
          <strong>{overall.title}</strong>
          <p>{overall.body}</p>
        </div>
        <div className="health-auction-block">
          <span>当前拍品</span>
          <strong>{active?.item.title ?? '暂无开拍中拍品'}</strong>
          <p>{active ? `${auctionStatusLabel(active.status)} · ${formatCents(active.current_price_cents)} · ${formatRemaining(active.end_at, now)}` : '选择或开拍后展示实时风险'}</p>
        </div>
        <div className="health-action-row">
          <Button disabled={!active} icon={<ExternalLink size={15} />} onClick={() => active && onOpenFlightRecorder(active.id)}>事件回放</Button>
          <Button disabled={!active} onClick={() => confirmEngineAction('reconcile_redis_engine', '商家端直播健康发现风险，触发竞拍状态校对')}>校对状态</Button>
          <Button disabled={!active} onClick={() => confirmEngineAction('force_snapshot_rebuild', '商家端直播健康要求买家端重新同步')}>重建买家状态</Button>
          <Button status="danger" disabled={!active} onClick={() => confirmEngineAction('pause_redis_engine', '商家端直播健康人工暂停竞拍确认')}>暂停确认</Button>
        </div>
      </section>

      <section className="health-grid" data-testid="live-health-grid">
        <div className="health-panel">
          <div className="health-panel-head">
            <span><Activity size={15} /> 直播转化</span>
            <strong>{heatSummary ? `${heatSummary.window_seconds} 秒窗口` : '等待汇总'}</strong>
          </div>
          <div className="funnel-bars">
            <FunnelBar label="观看" value={summary.funnel.watchers ?? 0} max={Math.max(summary.funnel.watchers ?? 0, summary.funnel.activeBidders, summary.funnel.accepted, 1)} muted={summary.funnel.watchers === undefined} />
            <FunnelBar label="活跃出价人" value={summary.funnel.activeBidders} max={Math.max(summary.funnel.watchers ?? 0, summary.funnel.activeBidders, summary.funnel.accepted, 1)} />
            <FunnelBar label="接受出价" value={summary.funnel.accepted} max={Math.max(summary.funnel.accepted + summary.funnel.rejected, 1)} />
            <FunnelBar label="拒绝出价" value={summary.funnel.rejected} max={Math.max(summary.funnel.accepted + summary.funnel.rejected, 1)} tone="warn" />
            <FunnelBar label="订单/支付" value={summary.funnel.orders} max={Math.max(summary.funnel.orders, 1)} caption={`${summary.funnel.paid} 已支付`} />
          </div>
        </div>

        <div className="health-panel">
          <div className="health-panel-head">
            <span><RadioTower size={15} /> 成交保障</span>
            <strong>实时竞拍</strong>
          </div>
          <HealthMetric label="买家出价响应" value={formatLag(heatSummary?.bid_response_p95_ms ?? 0)} status={(heatSummary?.bid_response_p95_ms ?? 0) > 1000 ? 'bad' : (heatSummary?.bid_response_p95_ms ?? 0) > 500 ? 'warn' : 'ok'} />
          <HealthMetric label="成交记录落账" value={formatLag(heatSummary?.ledger_settle_p95_ms ?? 0)} status={(heatSummary?.ledger_settle_p95_ms ?? 0) > 5000 ? 'bad' : (heatSummary?.ledger_settle_p95_ms ?? 0) > 1000 ? 'warn' : 'ok'} />
          <HealthMetric label="最长落账耗时" value={formatLag(heatSummary?.ledger_settle_max_ms ?? summary.engine.settlement_lag_max_ms)} status={(heatSummary?.ledger_settle_max_ms ?? summary.engine.settlement_lag_max_ms) > 5000 ? 'bad' : (heatSummary?.ledger_settle_max_ms ?? summary.engine.settlement_lag_max_ms) > 1000 ? 'warn' : 'ok'} />
          <HealthMetric label="待写入记录" value={summary.engine.pending_settlements} status={summary.engine.pending_settlements > 0 ? 'warn' : 'ok'} />
          <HealthMetric label="写入失败记录" value={summary.engine.failed_settlements} status={summary.engine.failed_settlements > 0 ? 'bad' : 'ok'} />
          <HealthMetric label="已暂停拍品" value={summary.engine.paused_auctions} status={summary.engine.paused_auctions > 0 ? 'bad' : 'ok'} />
        </div>

        <div className="health-panel">
          <div className="health-panel-head">
            <span><Wifi size={15} /> 买家影响</span>
            <strong>{summary.buyerRisk ? '需关注' : '正常'}</strong>
          </div>
          <HealthMetric label="恢复压力" value={summary.recoveryPressure} status={summary.recoveryPressure > 0 ? 'warn' : 'ok'} />
          <HealthMetric label="无效出价记录" value={summary.scopedRejects.length} status={summary.scopedRejects.length > 0 ? 'warn' : 'ok'} />
          <HealthMetric label="近期异常" value={summary.anomalies.length} status={summary.criticalAnomalies.length > 0 ? 'bad' : summary.anomalies.length > 0 ? 'warn' : 'ok'} />
          <p className="health-copy">出现恢复压力或竞拍确认暂停时，主播不应继续强推用户加价，先确认买家端按钮是否进入恢复或暂停状态。</p>
        </div>
      </section>

      <section className="incident-workspace" data-testid="incident-workspace">
        <div className="health-panel alert-panel">
          <div className="health-panel-head">
            <span><Bell size={15} /> 告警处置</span>
            <strong>{summary.anomalies.length} 条待看</strong>
          </div>
          {summary.anomalies.length === 0 ? <div className="empty-state compact-empty">暂无未静默告警</div> : summary.anomalies.slice(0, 6).map((row) => {
            const key = anomalyKey(row);
            const auctionID = String(row.auction_id ?? '');
            return (
              <div className={`alert-row severity-${anomalySeverity(row).toLowerCase()}`} key={key}>
                <div>
                  <Tag color={severityTagColor(anomalySeverity(row))}>{severityDisplayLabel(anomalySeverity(row))}</Tag>
                  {isAckedAlert(row, monitor) ? <Tag color="green">已确认</Tag> : null}
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
            <strong>处置记录</strong>
          </div>
          <div className="note-target">
            <select className="native-input" value={noteTarget} onChange={(event) => setNoteTarget(event.currentTarget.value as 'auction' | 'room')}>
              <option value="auction">当前竞拍</option>
              <option value="room">当前直播间</option>
            </select>
            <span title={targetID ?? undefined}>{noteTarget === 'auction' ? '记录到当前竞拍' : '记录到当前直播间'}</span>
          </div>
          <Input.TextArea
            autoSize={{ minRows: 4, maxRows: 6 }}
            placeholder="记录商家侧观察、客服反馈或人工处置原因，便于复盘。"
            value={note}
            onChange={setNote}
          />
          <Button type="primary" disabled={!targetID || !note.trim()} onClick={() => void submitNote()}>记录事件</Button>
        </div>
      </section>

      <section className="health-panel health-timeline-panel" data-testid="health-timeline">
        <div className="health-panel-head">
          <span><Clock3 size={15} /> 告警与处置时间线</span>
          <strong>{timelineRows.length} 条</strong>
        </div>
        <div className="health-timeline">
          {timelineRows.length === 0 ? <div className="empty-state compact-empty">暂无告警或控制信号</div> : timelineRows.map((row) => (
            <div className={`health-timeline-row ${row.kind}`} key={`${row.kind}-${row.ref}`}>
              <time>{row.time ? new Date(row.time).toLocaleTimeString() : '-'}</time>
              <Tag color={row.kind === 'alert' ? severityTagColor(row.severity) : 'arcoblue'}>{timelineKindLabel(row.kind)}</Tag>
              <div>
                <strong>{row.title}</strong>
                <p>{incidentReasonCopy(row.body)}</p>
              </div>
              {row.auctionID && row.auctionID.startsWith('auc') ? <Button size="mini" onClick={() => onOpenFlightRecorder(row.auctionID)}>查看记录</Button> : null}
            </div>
          ))}
        </div>
      </section>
    </>
  );
}

export function FunnelBar({ label, value, max, caption, muted, tone = 'normal' }: { label: string; value: number; max: number; caption?: string; muted?: boolean; tone?: 'normal' | 'warn' }) {
  const width = muted ? 16 : Math.max(8, Math.min(100, (value / Math.max(max, 1)) * 100));
  return (
    <div className={`funnel-row ${tone} ${muted ? 'muted' : ''}`}>
      <span>{label}</span>
      <div><i style={{ width: `${width}%` }} /></div>
      <strong>{muted ? '未接入' : value}</strong>
      {caption ? <em>{caption}</em> : null}
    </div>
  );
}

export function HealthMetric({ label, value, status }: { label: string; value: React.ReactNode; status: 'ok' | 'warn' | 'bad' }) {
  return (
    <div className={`health-metric metric-${status}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export function OrderDetailDrawer({
  auction,
  order,
  visible,
  onClose,
  onOpenFlightRecorder
}: {
  auction?: Auction;
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
              <span>订单编号</span>
              <strong>{displayOrderNo(order)}</strong>
              <code className="trace-id">排查编号 {order.id}</code>
            </div>
            <Tag color={order.status === 'PAID' ? 'green' : order.status === 'ORDER_EXPIRED' ? 'red' : 'orange'}>{orderStatusLabel(order.status)}</Tag>
          </div>

          {auction ? (
            <div className="order-detail-product">
              <span
                className={`order-detail-thumb ${displayMediaURL(auction.item?.image_url) ? 'has-media' : ''}`}
                style={displayMediaURL(auction.item?.image_url) ? { '--order-detail-media-url': `url("${displayMediaURL(auction.item.image_url)}")` } as React.CSSProperties : undefined}
              />
              <div>
                <span>成交商品</span>
                <strong>{auction.item?.title ?? auctionDisplayName(auction)}</strong>
                <p>{auction.item?.description ?? '商品详情以拍品、证书和实物图为准。'}</p>
              </div>
            </div>
          ) : null}

          <div className="order-detail-grid">
            <div><span>成交价</span><strong>{formatCents(order.amount_cents)}</strong></div>
            <div><span>中标人</span><strong>{maskUser(order.winner_id)}</strong></div>
            <div><span>保证金</span><strong>{formatCents(order.deposit_cents ?? 0)}</strong></div>
            <div><span>保证金状态</span><strong>{depositStatusLabel(order.deposit_status)}</strong></div>
            <div><span>支付截止</span><strong>{order.expire_at ? new Date(order.expire_at).toLocaleString() : '-'}</strong></div>
            <div><span>支付完成</span><strong>{order.paid_at ? new Date(order.paid_at).toLocaleString() : '-'}</strong></div>
            {auction ? <div><span>起拍价</span><strong>{formatCents(auction.start_price_cents)}</strong></div> : null}
            {auction ? <div><span>封顶价</span><strong>{formatCents(auction.cap_price_cents ?? order.amount_cents)}</strong></div> : null}
          </div>

          <div className="order-detail-section">
            <span>支付编号</span>
            <code>{order.provider_payment_id ?? '尚未发起支付'}</code>
          </div>

          <div className="order-detail-section">
            <span>关联竞拍</span>
            <div className="order-linked-row">
              <code>{order.auction_id}</code>
              <Button icon={<ExternalLink size={14} />} onClick={() => onOpenFlightRecorder(order.auction_id)}>打开事件回放</Button>
            </div>
          </div>

          <div className="order-detail-section">
            <span>下一步</span>
            <p>
              {order.status === 'ORDER_PENDING' && '提醒中标人完成支付；若支付链路异常，查看事件回放中的支付事件与异常。'}
              {order.status === 'PAYMENT_INITIATED' && '支付已发起但未确认，检查支付编号与支付回调。'}
              {order.status === 'PAID' && '订单已支付，可进入履约交接。'}
              {order.status === 'ORDER_EXPIRED' && '订单已超时，核查保证金状态与是否有迟到支付回调。'}
              {!['ORDER_PENDING', 'PAYMENT_INITIATED', 'PAID', 'ORDER_EXPIRED'].includes(order.status) && '查看订单、支付事件和竞拍记录后再对外承诺处理结果。'}
            </p>
          </div>
        </div>
      )}
    </Drawer>
  );
}

export function DiagnosticsPanel({
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
    ? `${monitorStatusCopy(String(latestAppend.latest_append_status ?? ''))} · 最近同步已记录`
    : '暂无最近同步记录';
  const recoveryLabel = engineSummary.last_recovery_rto_ms
    ? `最近恢复 ${formatLag(engineSummary.last_recovery_rto_ms)} ${engineSummary.last_recovery_status ?? ''}`.trim()
    : '最近恢复 暂无记录';
  return (
    <section className="band diagnostics" data-testid="diagnostics">
      <div className="section-title">
        <h2>诊断</h2>
        <span><Database size={16} /> 运维排查信息</span>
      </div>
      <div className="engine-diagnostics" data-testid="redis-engine-summary">
        <span><RadioTower size={14} /> 成交保障链路</span>
        <span>待确认 {engineSummary.pending_redis_decisions}</span>
        <span>写入结果 {engineSummary.append_success_count}/{engineSummary.append_failure_count}/{engineSummary.append_unknown_count}</span>
        <span>待落账 {engineSummary.pending_settlements}/{engineSummary.failed_settlements}</span>
        <span>最长落账耗时 {engineSummary.settlement_lag_max_ms}ms</span>
        <span>{recoveryLabel}</span>
        <span>暂停中 {engineSummary.paused_auctions}</span>
        <span>{latestAppend ? `${String(latestAppend.latest_append_status ?? '-')} · seq ${String(latestAppend.latest_append_engine_seq ?? '-')}` : latestAppendLabel}</span>
      </div>
      <div className="monitor-filter" aria-label="monitor-filter">
        <select
          aria-label="monitor-anomaly-type"
          className="native-input"
          value={monitorFilter.type}
          onChange={(event) => onFilterChange((current) => ({ ...current, type: event.currentTarget.value }))}
        >
          <option value="">全部异常</option>
          <option value="AUTH_SESSION_EXPIRED">登录已过期</option>
          <option value="ACL_FORBIDDEN">无操作权限</option>
          <option value="RATE_LIMIT_REDIS_DOWN">限流服务异常</option>
          <option value="BID_AUCTION_TOO_HOT">出价过于密集</option>
          <option value="RATE_LIMITED">操作过于频繁</option>
          <option value="PAYMENT_WEBHOOK_INVALID_SIGNATURE">支付回调校验失败</option>
          <option value="PAYMENT_RECONCILE_MISMATCH">支付对账不一致</option>
        </select>
        <input aria-label="monitor-auction-id" data-testid="monitor-auction-id" className="native-input" placeholder="拍品编号" value={monitorFilter.auctionID} onChange={(event) => onFilterChange((current) => ({ ...current, auctionID: event.currentTarget.value }))} />
        <input aria-label="monitor-user-id" data-testid="monitor-user-id" className="native-input" placeholder="用户编号" value={monitorFilter.userID} onChange={(event) => onFilterChange((current) => ({ ...current, userID: event.currentTarget.value }))} />
        <input aria-label="monitor-trace-id" data-testid="monitor-trace-id" className="native-input" placeholder="排查编号" value={monitorFilter.traceID} onChange={(event) => onFilterChange((current) => ({ ...current, traceID: event.currentTarget.value }))} />
      </div>
      <Tabs defaultActiveTab="auctions">
        <Tabs.TabPane key="auctions" title="竞拍状态"><MonitorTable payload={monitor.auctions} empty="暂无竞拍诊断数据" sourceKey="auction_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="redisEngine" title="成交保障"><MonitorTable payload={monitor.redisEngine} empty="暂无成交保障数据" sourceKey="auction_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="rejects" title="无效出价"><MonitorTable payload={monitor.rejects} empty="暂无拒绝出价" sourceKey="trace_id" icon={<AlertTriangle size={16} />} onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="recovery" title="恢复记录"><MonitorTable payload={monitor.recovery} empty="暂无恢复数据" sourceKey="room_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="anomalies" title="异常"><MonitorTable payload={monitor.anomalies} empty="暂无异常" sourceKey="id" icon={<AlertTriangle size={16} />} onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="outbox" title="买家端更新"><MonitorTable payload={monitor.outbox} empty="暂无买家端更新数据" sourceKey="outbox_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="watermarks" title="更新进度"><MonitorTable payload={monitor.outboxWatermarks} empty="暂无更新进度" sourceKey="shard_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="snapshots" title="状态恢复"><MonitorTable payload={monitor.snapshots} empty="暂无状态恢复记录" sourceKey="request_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="signals" title="人工处置"><MonitorTable payload={monitor.signals} empty="暂无人工处置记录" sourceKey="id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="scheduler" title="定时任务"><MonitorTable payload={monitor.scheduler} empty="暂无定时任务数据" sourceKey="job_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
      </Tabs>
    </section>
  );
}

export function NumberField({ label, name, value, min, max, precision, suffix, money = false, onChange }: { label: string; name: string; value: number; min: number; max?: number; precision?: number; suffix?: string; money?: boolean; onChange: (value: number) => void }) {
  const displayValue = money ? centsToYuan(value) : value;
  const displayMin = money ? centsToYuan(min) : min;
  const displayMax = money && max != null ? centsToYuan(max) : max;
  return (
    <Form.Item label={label}>
      <InputNumber
        aria-label={name}
        value={displayValue}
        min={displayMin}
        max={displayMax}
        precision={precision ?? (money ? 2 : 0)}
        prefix={money ? '¥' : undefined}
        suffix={suffix}
        onChange={(next) => {
          const numeric = Number(next);
          if (money) onChange(yuanToCents(Number.isFinite(numeric) ? numeric : displayMin));
          else onChange(Number.isFinite(numeric) ? numeric : min);
        }}
      />
    </Form.Item>
  );
}

export function MonitorTable({ payload, empty, icon, sourceKey, onOpenFlightRecorder }: {
  payload?: MonitorPayload;
  empty: string;
  icon?: React.ReactNode;
  sourceKey: string;
  onOpenFlightRecorder: (auctionID: string) => void;
}) {
  const rawRows = payload?.items ?? [];
  const rows = rawRows.filter((row) => !isStressDiagnosticRow(row));
  const hiddenStressRows = rawRows.length - rows.length;
  if (rows.length === 0) {
    return (
      <div className="empty-state">
        {icon}{empty}
        {hiddenStressRows > 0 && <span>已隐藏 {hiddenStressRows} 条压测/历史诊断记录</span>}
      </div>
    );
  }
  const priorityKeys = [
    sourceKey,
    'item_title',
    'status',
    'current_price_cents',
    'current_winner_id',
    'accepted_bid_count',
    'extend_count',
    'active_bidders_30s',
    'accepted_bids_30s',
    'rejected_bids_30s',
    'watcher_count',
    'bid_response_p95_ms',
    'ledger_settle_p95_ms',
    'ledger_settle_max_ms',
    'redis_pending_decisions',
    'pending_settlements',
    'failed_settlements',
    'settlement_lag_p99_ms',
    'settlement_lag_max_ms',
    'append_success_count',
    'append_failure_count',
    'append_unknown_count',
    'last_recovery_rto_ms',
    'last_recovery_status',
    'last_recovery_at',
    'delivery_state',
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
    <div className="monitor-table-wrap">
      {hiddenStressRows > 0 && <div className="monitor-hidden-note">已隐藏 {hiddenStressRows} 条压测/历史诊断记录，原始数据仍保留在后端用于排查。</div>}
      <Table
        rowKey={(record) => String(record.id ?? record.auction_id ?? record.outbox_id ?? record.job_id ?? record.room_id)}
        data={rows}
        pagination={false}
        columns={[
          {
            title: '排查入口',
            dataIndex: sourceKey,
            render: (value, record) => {
              const auctionID = rowAuctionID(record);
              const sourceURL = rowSourceURL(sourceKey, record);
              const displayValue = formatMonitorSourceValue(sourceKey, value);
              return auctionID ? (
                <button type="button" className="source-link source-button" onClick={() => onOpenFlightRecorder(auctionID)}>
                  <Tag color="arcoblue">{displayValue}</Tag>
                  <ExternalLink size={13} />
                </button>
              ) : sourceURL ? (
                <a className="source-link" href={sourceURL} target="_blank" rel="noreferrer">
                  <Tag color="arcoblue">{displayValue}</Tag>
                  <ExternalLink size={13} />
                </a>
              ) : <Tag color="arcoblue">{displayValue}</Tag>;
            }
          },
          ...keys.filter((key) => key !== sourceKey).map((key) => ({
            title: monitorFieldLabel(key),
            dataIndex: key,
            render: (value: unknown) => formatMonitorValue(key, value)
          }))
        ]}
      />
    </div>
  );
}

export function FlightRecorderDrawer({
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
      title="事件回放"
      visible={visible}
      onCancel={onClose}
      footer={null}
      unmountOnExit
    >
      <div className="flight-recorder" data-testid="flight-recorder-drawer">
        <div className="flight-recorder-head">
          <div>
            <span>竞拍编号</span>
            <strong>{summary?.auction_id ?? auctionID}</strong>
          </div>
          <div>
            <span>拍品</span>
            <strong>{summary?.item_title ?? '-'}</strong>
          </div>
          <div>
            <span>当前状态</span>
            <strong>{summary ? auctionStatusLabel(summary.status) : '-'}</strong>
          </div>
          <div>
            <span>当前价</span>
            <strong>{formatCents(summary?.current_price_cents)}</strong>
          </div>
        </div>

        <div className="flight-recorder-counts">
          <span>规则 {payload?.rules?.length ?? 0}</span>
          <span>订单 {payload?.orders?.length ?? 0}</span>
          <span>支付 {payload?.payment_events?.length ?? 0}</span>
          <span>异常 {payload?.anomalies?.length ?? 0}</span>
          <span>事件 {timeline.length}</span>
        </div>

        {loading ? (
          <div className="empty-state compact-empty">正在读取事件回放</div>
        ) : timeline.length === 0 ? (
          <div className="empty-state compact-empty">暂无事件记录</div>
        ) : (
          <div className="flight-timeline">
            {timeline.map((row, index) => (
              <div className="flight-row" key={`${row.kind}-${row.ref_id}-${index}`}>
                <div className="flight-row-main">
                  <Tag color={timelineTone(row)}>{eventKindLabel(row.kind)}</Tag>
                  <div>
                    <strong>{eventTypeLabel(row.event_type)}</strong>
                    <code className="raw-event-label">{rawEventLabel(row.event_type)}</code>
                    <span>{new Date(row.time).toLocaleString()} · 记录 {row.ref_id}</span>
                  </div>
                  <code>{row.status ? rawEventLabel(row.status) : eventStatusLabel(row.event_type)}</code>
                </div>
                <div className="flight-row-meta">
                  {row.user_id ? <span>用户 {maskUser(row.user_id)}</span> : null}
                  {row.amount_cents !== undefined ? <span>{formatCents(row.amount_cents)}</span> : null}
                  {typeof row.payload?.source === 'string' ? <span>来源 {rawEventLabel(row.payload.source)}</span> : null}
                  {row.trace_id ? <span>排查编号 {row.trace_id}</span> : null}
                  {row.status ? <span>{rawEventLabel(row.status)}</span> : null}
                </div>
                <div className="flight-row-explain">
                  <div><span>影响</span><p>{timelineImpact(row)}</p></div>
                  <div><span>下一步</span><p>{timelineNextAction(row)}</p></div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </Drawer>
  );
}

function highlightAssetExtension(mediaType: string): string {
  if (mediaType === 'video/webm') return 'webm';
  if (mediaType === 'video/mp4') return 'mp4';
  return 'html';
}
