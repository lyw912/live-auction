import React, { useState } from 'react';
import { Button, DatePicker, Drawer, Form, Input, InputNumber, Layout, Message, Modal, Space, Table, Tabs, Tag, Upload } from '@arco-design/web-react';
import { Activity, AlertTriangle, Bell, BellOff, Bot, CheckCircle2, ClipboardList, Clock3, Database, ExternalLink, Gavel, ImageIcon, Play, RadioTower, RefreshCw, ShieldCheck, Sparkles, Square, Upload as UploadIcon, Wifi } from 'lucide-react';
import type { Auction, AuctionRecap, FlightRecorderPayload, FlightRecorderTimelineRow, HeatSummary, HostPrompt, Item, ListingDraftJob, MaxBidSummary, MonitorPayload, Order, RedisEngineSummary, Room, RuleDraft, SentinelAlert, SignalRequest, SystemMessage } from './domain';
import { anomalyKey, anomalySeverity, auctionScopedRows, auctionStatusLabel, connectionLabel, createRuleDraft, depositPreview, formatCents, formatRemaining, formatSeconds, isAckedAlert, liveHealthSummary, maskUser, monitorCount, monitorItems, orderStatusLabel, overallCopy, promptSeverityClass, queueGroups, redisEngineSummary, riskQueue, rowAuctionID, rowSourceURL, severityTagColor, signalCopy, signalTargetID, signalType, sortedAuctions, statusTagColor, terminalStatus, timelineImpact, timelineNextAction, timelineTone, validateRule, visibleAnomalies } from './domain';

export function ConsoleNav({ activeTab, onSelect }: { activeTab: string; onSelect: (tab: string) => void }) {
  const rows = [
    { key: 'inventory', label: '拍品', icon: <ClipboardList size={16} /> },
    { key: 'rules', label: '竞拍', icon: <RadioTower size={16} /> },
    { key: 'health', label: '风险处理', icon: <ShieldCheck size={16} /> },
    { key: 'diagnostics', label: '诊断', icon: <Activity size={16} /> }
  ];
  return (
    <>
      <div className="brand">直播竞拍台</div>
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
  if (summary.failed_settlements > 0 || summary.paused_auctions > 0) return { label: '异常', className: 'bad' };
  if (summary.pending_redis_decisions > 0 || summary.pending_settlements > 0 || summary.settlement_lag_max_ms > 1000) return { label: '降级', className: 'warn' };
  return { label: '正常', className: 'ok' };
}

function displayOrderNo(order: Order) {
  const compact = order.id.replace(/^ord[_-]?/i, '').replace(/[^a-z0-9]/gi, '').slice(-8).toUpperCase();
  return `JP${new Date().toISOString().slice(0, 10).replace(/-/g, '')}-${compact || '00000000'}`;
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
  engine_mode: '引擎模式',
  engine_seq: '引擎序号',
  db_engine_seq: '落库序号',
  redis_pending_decisions: '待确认',
  pending_settlements: '待入账',
  failed_settlements: '结算失败',
  settlement_lag_p99_ms: 'P99 结算延迟',
  settlement_lag_max_ms: '最大结算延迟',
  latest_append_status: '最近写入状态',
  latest_append_engine_seq: '最近写入序号',
  latest_append_topic: '最近写入 Topic',
  latest_append_partition: '最近写入分区',
  latest_append_offset: '最近写入位点',
  append_success_count: '写入成功',
  append_failure_count: '写入失败',
  append_unknown_count: '写入待确认',
  append_stats_last_status: '写入汇总',
  last_recovery_rto_ms: '恢复耗时',
  last_recovery_status: '恢复状态',
  last_recovery_at: '恢复时间',
  checkpoint_topic: '检查点 Topic',
  checkpoint_partition: '检查点分区',
  checkpoint_next_offset: '下一位点',
  delivery_state: '推送状态',
  delivery_message_id: '推送消息',
  event_key: '事件键',
  seq: '更新序号',
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
  outbox_id: '推送编号',
  shard_id: '分片',
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
  ENGINE_PAUSED: '引擎暂停',
  RECONCILING: '对账恢复中',
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
  if (key === 'current_winner_id' || key === 'user_id') return maskUser(String(value));
  if (key === 'status') return monitorStatusCopy(String(value));
  if (key === 'reject_reason' || key === 'code' || key === 'error_class' || key.endsWith('_status')) {
    return monitorStatusCopy(String(value));
  }
  if (key.endsWith('_cents')) return formatCents(Number(value));
  if (key.endsWith('_ms')) return formatLag(Number(value));
  if (key.endsWith('_at') || key === 'time' || key === 'created_at' || key === 'updated_at') return formatMonitorTime(value);
  if (typeof value === 'boolean') return value ? '是' : '否';
  if (typeof value === 'object') return JSON.stringify(value);
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
        <span>先完善拍品，再排期或开拍；开拍后规则会锁定</span>
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
                <strong>{auctionDisplayName(auction)}</strong>
                <em>{auctionStatusLabel(auction.status)} · {editable ? '规则可编辑' : '规则已冻结'}</em>
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
  return (
    <section className="health-ribbon" data-testid="health-ribbon">
      <div className="ribbon-room">
        <strong>{roomDisplayName(roomID)}</strong>
        <span>当前时间 {new Date(now).toLocaleTimeString()}</span>
      </div>
      <div className="ribbon-metrics" data-testid="health-ribbon-status" role="status" aria-live="polite">
        <span><Wifi size={15} /> {recoveryLabel}</span>
        <span className={`engine-health ${health.className}`}><Database size={15} /> 引擎 ● {health.label}</span>
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
          <Button size="mini" onClick={onApplyListingDraft} disabled={listingDraft.status !== 'SUCCEEDED'}>应用到表单</Button>
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
              onFileChange(option.file as File);
              option.onSuccess?.({});
              return { abort() {} };
            }}
          >
            <Button icon={<UploadIcon size={14} />}>选择图片</Button>
          </Upload>
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
              <DatePicker
                aria-label="schedule-start-at"
                showTime
                format="YYYY-MM-DD HH:mm"
                value={scheduleStartAt}
                onChange={(value) => onScheduleStartAtChange(value)}
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
              Modal.confirm({ title: '确认取消竞拍', content: auctionDisplayName(selectedAuction), onOk: () => onAction('cancel') });
            }}>取消</Button>
            <Button disabled={!canNarrate} onClick={() => onAction('narrate-start')}>开始讲解</Button>
            <Button disabled={!selectedAuction.is_narrating} onClick={() => onAction('narrate-stop')}>停止讲解</Button>
          </Space>
          <div className="action-guardrail">
            {selectedAuction.status === 'DRAFT' && '待完善状态可编辑规则并排期；排期后会锁定买家预期，避免开拍前临时改价。'}
            {selectedAuction.status === 'SCHEDULED' && !activeConflict && '已排期后价格规则会冻结；如需修改，先撤回排期，再改规则并重新排期。'}
            {selectedAuction.status === 'SCHEDULED' && activeConflict && `房间已有开拍中的拍品「${auctionDisplayName(activeAuction)}」；同一房间只能有一个开拍中拍品，需先结束或取消当前竞拍。`}
            {selectedAuction.status === 'ACTIVE' && '开拍中只允许切换讲解或带原因取消，不能修改价格规则。'}
            {isTerminal && '终态竞拍不可再操作，订单和诊断保留可追溯记录。'}
            {selectedAuction.status !== 'SCHEDULED' && narratingConflict && `讲解中拍品为「${auctionDisplayName(narratingAuction)}」；切换讲解前需先停止当前讲解。`}
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
          <Tag color={statusTagColor(auction.status)}>{auctionStatusLabel(auction.status)}</Tag>
          {auction.is_narrating ? <Tag color="green">讲解中</Tag> : <Tag>未讲解</Tag>}
        </span>
        <span className="queue-rules">
          起 {formatCents(auction.start_price_cents)} · 加 {formatCents(auction.increment_cents)} · 封 {formatCents(auction.cap_price_cents)}
        </span>
        <span className="queue-rules">
          当前/成交 {formatCents(auction.current_price_cents)} · {auction.accepted_bid_count} 口 · {auction.end_at ? formatRemaining(auction.end_at) : '-'}
        </span>
        <span className="queue-constraints">
          {constraints.map((constraint) => <em key={constraint}>{constraint}</em>)}
        </span>
      </span>
    </button>
  );
}

export function AuctionControlSummary({
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
            <Tag color={statusTagColor(selectedAuction.status)}>{auctionStatusLabel(selectedAuction.status)}</Tag>
            {selectedAuction.is_narrating ? <Tag color="green">讲解中</Tag> : <Tag>未讲解</Tag>}
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
            <span>{selectedAuction.accepted_bid_count} 口有效出价</span>
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
          <strong>{selectedAuction.accepted_bid_count} 口</strong>
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
      {children}
    </section>
  );
}

export function LiveAssistRail({
  autoCommentaryEnabled,
  dismissedPromptIDs,
  heatLoading,
  heatSummary,
  latestRecap,
  maxBidLoading,
  maxBidSummary,
  monitor,
  onBuildRecap,
  onCreateCommentary,
  onEvaluateSentinel,
  onToggleAutoCommentaryEnabled,
  onOpenFlightRecorder,
  prompts,
  promptsLoading,
  recentEvents,
  selectedAuction,
  sentinelAlerts,
  systemMessages,
  onDismissPrompt,
  onDriveDemoBid
}: {
  autoCommentaryEnabled: boolean;
  dismissedPromptIDs: string[];
  heatLoading: boolean;
  heatSummary?: HeatSummary;
  latestRecap?: AuctionRecap;
  maxBidLoading: boolean;
  maxBidSummary?: MaxBidSummary;
  monitor: Record<string, MonitorPayload>;
  onBuildRecap: () => void;
  onCreateCommentary: (eventType: string) => void;
  onEvaluateSentinel: () => void;
  onToggleAutoCommentaryEnabled: () => void;
  onOpenFlightRecorder: (auctionID: string) => void;
  prompts: HostPrompt[];
  promptsLoading: boolean;
  recentEvents: Array<Record<string, unknown>>;
  selectedAuction?: Auction;
  sentinelAlerts: SentinelAlert[];
  systemMessages: SystemMessage[];
  onDismissPrompt: (promptID: string) => void;
  onDriveDemoBid: (mode: 'reject' | 'outbid' | 'extend' | 'sold') => void;
}) {
  const [demoDrawerOpen, setDemoDrawerOpen] = useState(false);
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
  const recovery = connectionLabel(monitor, selectedAuction.room_id);
  const hasAnomaly = monitorCount(monitor.anomalies) > 0;
  const visiblePrompts = prompts.filter((prompt) => prompt.id && !dismissedPromptIDs.includes(prompt.id)).slice(0, 3);
  const topPrompt = visiblePrompts[0];
  const risks = riskQueue(monitor, selectedAuction);
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
        <span>主播话术</span>
        <button type="button" onClick={() => onCreateCommentary('product_evidence')}>证书/瑕疵</button>
        <button type="button" onClick={() => onCreateCommentary('rule_guardrail')}>封顶/保证金</button>
        <button type="button" onClick={() => onCreateCommentary('extended')}>延时规则</button>
      </div>
      <div className="ai-live-panel" data-testid="ai-live-panel">
        <div className="heat-summary-head">
          <span>智能场控</span>
          <strong>{autoCommentaryEnabled ? '自动开启' : '自动关闭'}</strong>
        </div>
        <div className="demo-driver-grid">
          <Button size="mini" icon={<Bot size={13} />} onClick={() => onCreateCommentary('bid_accepted')}>生成解说</Button>
          <Button size="mini" icon={<ShieldCheck size={13} />} onClick={onEvaluateSentinel}>检查风控</Button>
          <Button size="mini" icon={<ClipboardList size={13} />} onClick={onBuildRecap}>生成复盘</Button>
          <Button size="mini" onClick={onToggleAutoCommentaryEnabled}>{autoCommentaryEnabled ? '关闭自动' : '开启自动'}</Button>
        </div>
        <small>智能内容只基于已发生的竞拍事实生成，不决定价格、赢家或订单。</small>
        {systemMessages.length ? (
          <div className="system-message-list">
            {systemMessages.slice(0, 3).map((message) => (
              <div className={`system-message style-${message.style}`} key={message.id}>
                <strong>{message.source_seq ? '直播提示' : '系统提示'}</strong>
                <span>{message.body}</span>
              </div>
            ))}
          </div>
        ) : null}
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
            {latestRecap.highlight_asset ? (
              <div className="recap-actions">
                <a href={latestRecap.highlight_asset.asset_url} target="_blank" rel="noreferrer">打开高光</a>
                <a href={latestRecap.highlight_asset.asset_url} download={`${latestRecap.item_title || 'auction'}-highlight.${highlightAssetExtension(latestRecap.highlight_asset.media_type)}`}>下载高光</a>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>
      <div className="demo-driver-entry" data-testid="demo-driver">
        <div className="heat-summary-head">
          <span>演示助手</span>
          <strong>{selectedAuction.status === 'ACTIVE' ? '可触发' : '开拍后可用'}</strong>
        </div>
        <Button size="small" onClick={() => setDemoDrawerOpen(true)}>打开演示助手</Button>
        <small>录屏加速用，操作会走真实后端接口。</small>
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
            <span>录屏加速用 · 走真实后端接口</span>
            <strong>{selectedAuction.status === 'ACTIVE' ? '可触发' : '开拍后可用'}</strong>
          </div>
          <div className="demo-driver-grid">
            <Button disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('reject')}>模拟无效出价</Button>
            <Button disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('outbid')}>模拟另一位买家出一手</Button>
            <Button disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('extend')}>触发末段延时</Button>
            <Button status="danger" disabled={selectedAuction.status !== 'ACTIVE'} onClick={() => onDriveDemoBid('sold')}>触发封顶成交</Button>
          </div>
          <small>这些按钮不会前端假改状态；每次都会写入真实竞拍记录、事件和订单。</small>
        </div>
      </Drawer>
      <div className="max-bid-summary" data-testid="max-bid-summary">
        <div className="heat-summary-head">
          <span>自动加价概况</span>
          <strong>{maxBidLoading ? '读取中' : maxBidSummary ? '已更新' : '暂无数据'}</strong>
        </div>
        {maxBidSummary ? (
          <>
            <div className="heat-grid">
              <div><span>启用中</span><strong>{maxBidSummary.active_intent_count}</strong></div>
              <div><span>预先设置</span><strong>{maxBidSummary.pre_bid_count}</strong></div>
              <div><span>自动加价</span><strong>{maxBidSummary.max_bid_count}</strong></div>
              <div><span>已跟价</span><strong>{maxBidSummary.applied_intent_count}</strong></div>
              <div><span>已被超越</span><strong>{maxBidSummary.exhausted_count}</strong></div>
              <div><span>已取消</span><strong>{maxBidSummary.cancelled_count}</strong></div>
            </div>
            <small>{maxBidSummary.has_private_pressure ? '有买家设置了私密自动加价；主播只看汇总，不看个人上限。' : '暂无活跃自动加价。'}</small>
            <Button size="mini" icon={<ExternalLink size={13} />} onClick={() => onOpenFlightRecorder(selectedAuction.id)}>审计自动出价</Button>
          </>
        ) : (
          <div className="heat-unavailable">{maxBidLoading ? '正在读取汇总' : '自动加价汇总暂不可用'}</div>
        )}
      </div>
      <div className="heat-summary" data-testid="heat-summary">
        <div className="heat-summary-head">
          <span>近30秒热度</span>
          <strong>{heatLoading ? '读取中' : heatSummary ? '已更新' : '暂无数据'}</strong>
        </div>
        {heatSummary ? (
          <div className="heat-grid">
            <div><span>参与买家</span><strong>{heatSummary.active_bidders_30s}</strong></div>
            <div><span>有效出价</span><strong>{heatSummary.accepted_bids_30s}</strong></div>
            <div><span>无效出价</span><strong>{heatSummary.rejected_bids_30s}</strong></div>
            <div><span>弹幕</span><strong>{heatSummary.chat_messages_30s}</strong></div>
            <div><span>恢复事件</span><strong>{heatSummary.recovery_events_30s}</strong></div>
            <div><span>观看数据</span><strong>{heatSummary.watcher_count_available ? heatSummary.watcher_count ?? 0 : '暂不可用'}</strong></div>
          </div>
        ) : (
          <div className="heat-unavailable">{heatLoading ? '正在读取真实聚合' : '热度聚合暂不可用'}</div>
        )}
      </div>
      <div className="assist-grid">
        <div>
          <span>恢复状态</span>
          <strong>{recovery}</strong>
        </div>
        <div>
          <span>待推送</span>
          <strong>{monitorCount(monitor.outbox)}</strong>
        </div>
        <div>
          <span>无效出价</span>
          <strong>{monitorCount(monitor.rejects)}</strong>
        </div>
        <div className={hasAnomaly ? 'risk' : ''}>
          <span>风险</span>
          <strong>{hasAnomaly ? `${monitorCount(monitor.anomalies)} 条异常` : '正常'}</strong>
        </div>
      </div>
      <div className="risk-queue" data-testid="risk-queue" role="status" aria-live="polite">
        <div className="heat-summary-head">
          <span>风险待处理</span>
          <strong>{risks.length ? `${risks.length} 条真实信号` : '正常'}</strong>
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
      {topPrompt ? <div className="risk-hint" data-testid="risk-hint">优先处理：{topPrompt.title}</div> : null}
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
    <div className="recent-events" data-testid="recent-events">
      <div className="recent-title">
        <strong>最近事件</strong>
        <button type="button" className="link-button" onClick={() => onOpenFlightRecorder(selectedAuction.id)}>
          事件回放 <ExternalLink size={13} />
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

export function AICopilotDrawer({
  draft,
  imageFile,
  imageURL,
  loading,
  notes,
  category,
  visible,
  onApply,
  onCategoryChange,
  onClose,
  onGenerate,
  onImageFileChange,
  onImageURLChange,
  onNotesChange
}: {
  draft?: ListingDraftJob;
  imageFile: File | null;
  imageURL: string;
  loading: boolean;
  notes: string;
  category: string;
  visible: boolean;
  onApply: () => void;
  onCategoryChange: (category: string) => void;
  onClose: () => void;
  onGenerate: () => void;
  onImageFileChange: (file: File | null) => void;
  onImageURLChange: (url: string) => void;
  onNotesChange: (notes: string) => void;
}) {
  const output = draft?.output_json;
  const imageCanReachProvider = Boolean(imageFile) || imageURL.trim().startsWith('https://');
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
          <span>生成内容只进入草稿；发布和规则保存仍走现有后端校验。</span>
        </div>
        <Form layout="vertical">
          <Form.Item label="拍品图片">
            <div className="ai-image-input">
              <Upload
                accept="image/*"
                limit={1}
                showUploadList={false}
                customRequest={(option) => {
                  onImageFileChange(option.file as File);
                  option.onSuccess?.({});
                  return { abort() {} };
                }}
              >
                <button type="button" className="ai-image-drop" aria-label="listing-copilot-image-file">
                  <UploadIcon size={18} />
                  <span>{imageFile ? imageFile.name : imageURL ? '更换图片' : '上传图片'}</span>
                </button>
              </Upload>
              {imageURL ? (
                <div className="ai-image-preview">
                  <img src={imageURL} alt="" />
                  <Tag color={imageCanReachProvider ? 'green' : 'gold'}>{imageCanReachProvider ? '可用于智能识图' : '仅用于表单'}</Tag>
                </div>
              ) : null}
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
            <Button disabled={!draft || draft.status !== 'SUCCEEDED'} onClick={onApply}>应用到表单</Button>
          </Space>
        </Form>
        {draft ? (
          <div className="ai-draft-review">
            <div className="ai-draft-status">
              <Tag color={draft.status === 'SUCCEEDED' ? 'green' : draft.status === 'FAILED' ? 'red' : 'gray'}>{draftStatusLabel(draft.status)}</Tag>
              <span>{new Date(draft.created_at).toLocaleString()}</span>
            </div>
            {draft.error_message ? <div className="risk-hint">{draft.error_message}</div> : null}
            <section>
              <h3>标题候选</h3>
              <div className="draft-chip-row">
                {(output?.title_candidates ?? []).map((title) => <Tag key={title}>{title}</Tag>)}
              </div>
            </section>
            <section>
              <h3>描述</h3>
              <p>{output?.description ?? '-'}</p>
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
                {[...(output?.compliance_flags ?? []), ...(output?.requires_evidence ?? []), ...(output?.unsupported_claims ?? [])].map((flag) => <Tag color="orangered" key={flag}>{flag}</Tag>)}
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
            <strong>起拍、加价、封顶必须形成可达价格网格</strong>
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
            <span>信任与保证金</span>
            <strong>高额确认和保证金提示必须在买家下单前可见</strong>
          </div>
          <div className="rule-subgrid">
            <NumberField label="高额确认" name="fat-finger-threshold-cents" value={rule.fatFingerThresholdCents} min={rule.incrementCents + 1} money onChange={(value) => onRuleChange({ fatFingerThresholdCents: value })} />
            <NumberField label="保证金比例" name="deposit-bps" value={rule.depositBPS} min={0} max={10000} onChange={(value) => onRuleChange({ depositBPS: value })} suffix="基点" />
            <NumberField label="保证金下限" name="deposit-floor-cents" value={rule.depositFloorCents} min={0} money onChange={(value) => onRuleChange({ depositFloorCents: value })} />
            <NumberField label="保证金上限" name="deposit-cap-cents" value={rule.depositCapCents} min={0} money onChange={(value) => onRuleChange({ depositCapCents: value })} />
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
            <span>买家端预览</span>
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

export function OrdersPanel({ orders, onOpenFlightRecorder, onOpenOrder }: { orders: Order[]; onOpenFlightRecorder: (auctionID: string) => void; onOpenOrder: (orderID: string) => void }) {
  const [expanded, setExpanded] = useState(false);
  const visibleOrders = expanded ? orders : orders.slice(0, 8);
  const hiddenCount = Math.max(0, orders.length - visibleOrders.length);
  return (
    <div className="rule-panel">
      <div className="panel-heading order-heading">
        <div>
          <h2>订单</h2>
          <span>成交后自动生成，详情保留中标人、保证金、支付时效和审计入口</span>
        </div>
        {orders.length > 8 ? (
          <Button size="mini" onClick={() => setExpanded((current) => !current)}>
            {expanded ? '收起历史订单' : `展开全部 ${orders.length} 条`}
          </Button>
        ) : null}
      </div>
      {orders.length === 0 ? <div className="empty-state">暂无订单</div> : visibleOrders.map((order) => (
        <div className="order-line" key={order.id}>
          <button type="button" className="order-id-link" onClick={() => onOpenOrder(order.id)}>单号 {displayOrderNo(order)}</button>
          <Tag color={order.status === 'PAID' ? 'green' : 'orange'}>{orderStatusLabel(order.status)}</Tag>
          <span>中标人 {maskUser(order.winner_id)}</span>
          <span>{depositStatusLabel(order.deposit_status)}</span>
          <span>{order.expire_at ? `支付截止 ${new Date(order.expire_at).toLocaleTimeString()}` : '无支付截止'}</span>
          <strong>{formatCents(order.amount_cents)}</strong>
          <Button size="mini" icon={<ExternalLink size={13} />} onClick={() => onOpenFlightRecorder(order.auction_id)}>审计</Button>
        </div>
      ))}
      {hiddenCount > 0 ? (
        <div className="order-collapsed-note">已收起 {hiddenCount} 条历史订单；演示时默认只展示最近 8 条。</div>
      ) : null}
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
          <Button disabled={!active} icon={<ExternalLink size={15} />} onClick={() => active && onOpenFlightRecorder(active.id)}>飞行记录</Button>
          <Button disabled={!active} onClick={() => confirmEngineAction('reconcile_redis_engine', '商家端直播健康发现风险，触发竞拍状态校对')}>校对状态</Button>
          <Button disabled={!active} onClick={() => confirmEngineAction('force_snapshot_rebuild', '商家端直播健康要求买家端重新同步')}>重建买家状态</Button>
          <Button status="danger" disabled={!active} onClick={() => confirmEngineAction('pause_redis_engine', '商家端直播健康人工暂停竞拍确认')}>暂停确认</Button>
        </div>
      </section>

      <section className="health-grid" data-testid="live-health-grid">
        <div className="health-panel">
          <div className="health-panel-head">
            <span><Activity size={15} /> 实时经营</span>
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
            <span><RadioTower size={15} /> 系统健康</span>
            <strong>Redis/Kafka/PG</strong>
          </div>
          <HealthMetric label="实时决策积压" value={summary.engine.pending_redis_decisions} status={summary.engine.pending_redis_decisions > 0 ? 'bad' : 'ok'} />
          <HealthMetric label="待结算" value={summary.engine.pending_settlements} status={summary.engine.pending_settlements > 0 ? 'warn' : 'ok'} />
          <HealthMetric label="结算失败" value={summary.engine.failed_settlements} status={summary.engine.failed_settlements > 0 ? 'bad' : 'ok'} />
          <HealthMetric label="最大延迟" value={formatLag(summary.engine.settlement_lag_max_ms)} status={summary.engine.settlement_lag_max_ms > 5000 ? 'bad' : summary.engine.settlement_lag_max_ms > 1000 ? 'warn' : 'ok'} />
          <HealthMetric label="暂停的竞拍" value={summary.engine.paused_auctions} status={summary.engine.paused_auctions > 0 ? 'bad' : 'ok'} />
        </div>

        <div className="health-panel">
          <div className="health-panel-head">
            <span><Wifi size={15} /> 买家影响</span>
            <strong>{summary.buyerRisk ? 'attention' : 'normal'}</strong>
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

export function FunnelBar({ label, value, max, caption, muted, tone = 'normal' }: { label: string; value: number; max: number; caption?: string; muted?: boolean; tone?: 'normal' | 'warn' }) {
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

export function HealthMetric({ label, value, status }: { label: string; value: React.ReactNode; status: 'ok' | 'warn' | 'bad' }) {
  return (
    <div className={`health-metric metric-${status}`}>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export function OrderDetailDrawer({
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
              <span>订单编号</span>
              <strong>{displayOrderNo(order)}</strong>
              <code className="trace-id">排查编号 {order.id}</code>
            </div>
            <Tag color={order.status === 'PAID' ? 'green' : order.status === 'ORDER_EXPIRED' ? 'red' : 'orange'}>{orderStatusLabel(order.status)}</Tag>
          </div>

          <div className="order-detail-grid">
            <div><span>成交价</span><strong>{formatCents(order.amount_cents)}</strong></div>
            <div><span>中标人</span><strong>{maskUser(order.winner_id)}</strong></div>
            <div><span>保证金</span><strong>{formatCents(order.deposit_cents ?? 0)}</strong></div>
            <div><span>保证金状态</span><strong>{depositStatusLabel(order.deposit_status)}</strong></div>
            <div><span>支付截止</span><strong>{order.expire_at ? new Date(order.expire_at).toLocaleString() : '-'}</strong></div>
            <div><span>支付完成</span><strong>{order.paid_at ? new Date(order.paid_at).toLocaleString() : '-'}</strong></div>
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
    ? `${latestAppend.latest_append_status ?? '-'} · seq ${latestAppend.latest_append_engine_seq ?? '-'} · ${latestAppend.latest_append_topic ?? '-'}:${latestAppend.latest_append_partition ?? '-'}:${latestAppend.latest_append_offset ?? '-'}`
    : '暂无 append marker';
  return (
    <section className="band diagnostics" data-testid="diagnostics">
      <div className="section-title">
        <h2>诊断</h2>
        <span><Database size={16} /> 运维排查信息</span>
      </div>
      <div className="engine-diagnostics" data-testid="redis-engine-summary">
        <span><RadioTower size={14} /> 出价确认链路</span>
        <span>待确认 {engineSummary.pending_redis_decisions}</span>
        <span>写入结果 {engineSummary.append_success_count}/{engineSummary.append_failure_count}/{engineSummary.append_unknown_count}</span>
        <span>待入账 {engineSummary.pending_settlements}/{engineSummary.failed_settlements}</span>
        <span>最长延迟 {engineSummary.settlement_lag_max_ms}ms</span>
        <span>最近恢复 {engineSummary.last_recovery_rto_ms ?? '-'}ms {engineSummary.last_recovery_status ?? ''}</span>
        <span>暂停中 {engineSummary.paused_auctions}</span>
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
        <Tabs.TabPane key="redisEngine" title="出价确认"><MonitorTable payload={monitor.redisEngine} empty="暂无出价确认数据" sourceKey="auction_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="rejects" title="无效出价"><MonitorTable payload={monitor.rejects} empty="暂无拒绝出价" sourceKey="trace_id" icon={<AlertTriangle size={16} />} onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="recovery" title="恢复记录"><MonitorTable payload={monitor.recovery} empty="暂无恢复数据" sourceKey="room_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="anomalies" title="异常"><MonitorTable payload={monitor.anomalies} empty="暂无异常" sourceKey="id" icon={<AlertTriangle size={16} />} onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="outbox" title="推送队列"><MonitorTable payload={monitor.outbox} empty="暂无推送队列数据" sourceKey="outbox_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="watermarks" title="推送水位"><MonitorTable payload={monitor.outboxWatermarks} empty="暂无推送水位" sourceKey="shard_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="snapshots" title="状态快照"><MonitorTable payload={monitor.snapshots} empty="暂无状态快照记录" sourceKey="request_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="signals" title="控制信号"><MonitorTable payload={monitor.signals} empty="暂无控制信号" sourceKey="id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
        <Tabs.TabPane key="scheduler" title="定时任务"><MonitorTable payload={monitor.scheduler} empty="暂无定时任务数据" sourceKey="job_id" onOpenFlightRecorder={onOpenFlightRecorder} /></Tabs.TabPane>
      </Tabs>
    </section>
  );
}

export function NumberField({ label, name, value, min, max, suffix, money = false, onChange }: { label: string; name: string; value: number; min: number; max?: number; suffix?: string; money?: boolean; onChange: (value: number) => void }) {
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
        precision={money ? 2 : 0}
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
            <strong>{summary ? `${summary.status} / 第 ${summary.seq} 次更新` : '-'}</strong>
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
                  <Tag color={timelineTone(row)}>{row.kind}</Tag>
                  <div>
                    <strong>{row.event_type}</strong>
                    <span>{new Date(row.time).toLocaleString()} · 记录 {row.ref_id}</span>
                  </div>
                  <code>{row.seq !== undefined ? `第 ${row.seq} 次更新` : row.status ?? '-'}</code>
                </div>
                <div className="flight-row-meta">
                  {row.user_id ? <span>用户 {maskUser(row.user_id)}</span> : null}
                  {row.amount_cents !== undefined ? <span>{formatCents(row.amount_cents)}</span> : null}
                  {typeof row.payload?.source === 'string' ? <span>来源 {row.payload.source}</span> : null}
                  {row.trace_id ? <span>追踪号 {row.trace_id}</span> : null}
                  {row.status ? <span>{row.status}</span> : null}
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
