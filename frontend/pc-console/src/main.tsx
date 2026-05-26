import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Button, Form, Input, InputNumber, Layout, Message, Modal, Space, Table, Tabs, Tag } from '@arco-design/web-react';
import '@arco-design/web-react/dist/css/arco.css';
import { Activity, AlertTriangle, ClipboardList, Clock3, Database, ExternalLink, Gavel, ImageIcon, Play, RadioTower, RefreshCw, Square, Upload, Wifi } from 'lucide-react';
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
  deposit_status: string;
  expire_at?: string;
  paid_at?: string;
};

type MonitorPayload = {
  items: Array<Record<string, unknown>>;
};

type FlightRecorderPayload = {
  timeline?: Array<Record<string, unknown>>;
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
  return {
    active: sorted.filter((auction) => auction.status === 'ACTIVE'),
    scheduled: sorted.filter((auction) => auction.status === 'SCHEDULED'),
    draft: sorted.filter((auction) => auction.status === 'DRAFT'),
    finished: sorted.filter((auction) => !['ACTIVE', 'SCHEDULED', 'DRAFT'].includes(auction.status))
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

function rowSourceURL(sourceKey: string, record: Record<string, unknown>) {
  const auctionID = String(record.auction_id ?? record.aggregate_id ?? record.target_id ?? '');
  if (auctionID && (sourceKey === 'auction_id' || sourceKey === 'trace_id' || sourceKey === 'outbox_id' || sourceKey === 'job_id' || sourceKey === 'request_id' || sourceKey === 'id')) {
    return `/api/monitor/auctions/${encodeURIComponent(auctionID)}/flight-recorder?limit=50&timeline_limit=100`;
  }
  return '';
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
  const [monitorFilter, setMonitorFilter] = useState({ type: '', auctionID: '', userID: '', traceID: '' });
  const [loading, setLoading] = useState(false);
  const [savingRule, setSavingRule] = useState(false);
  const [creating, setCreating] = useState(false);
  const [ruleSaveState, setRuleSaveState] = useState<'idle' | 'saved' | 'error'>('idle');
  const [backendRuleError, setBackendRuleError] = useState('');
  const [backendSuggestions, setBackendSuggestions] = useState<number[]>([]);
  const [itemDraft, setItemDraft] = useState({ title: '新拍品', description: '本场直播竞拍拍品', imageURL: '' });
  const [itemImageFile, setItemImageFile] = useState<File | null>(null);
  const [scheduleStartAt, setScheduleStartAt] = useState('');
  const [cancelReason, setCancelReason] = useState('主播异常取消');
  const [rule, setRule] = useState<RuleDraft>(createRuleDraft());
  const [sessionReady, setSessionReady] = useState(false);
  const [now, setNow] = useState(Date.now());
  const selectedAuction = useMemo(() => auctions.find((auction) => auction.id === selectedAuctionID) ?? auctions[0], [auctions, selectedAuctionID]);
  const pinnedActiveAuction = useMemo(() => activeAuction(auctions), [auctions]);
  const currentNarratingAuction = useMemo(() => narratingAuction(auctions), [auctions]);
  const ruleValidation = validateRule(rule);
  const shownSuggestions = ruleValidation.valid ? backendSuggestions : ruleValidation.suggestions;

  const loadAll = async () => {
    if (!sessionReady) return;
    setLoading(true);
    try {
      const roomPayload = await fetch('/api/rooms').then((r) => readJSON<{ items?: Room[] }>(r));
      const roomRows = roomPayload.items ?? [];
      const nextRoomID = roomRows.find((room) => room.id === roomID)?.id ?? roomRows[0]?.id ?? roomID;
      const [auctionRows, orderRows, auctionsDiag, anomalies, outbox, outboxWatermarks, snapshots, signals, scheduler, rejects, recovery] = await Promise.all([
        fetch(`/api/auctions?room_id=${nextRoomID}`).then((r) => readJSON<Auction[]>(r)),
        fetch('/api/orders').then((r) => readJSON<Order[]>(r)),
        fetch('/api/monitor/auctions').then((r) => readJSON<MonitorPayload>(r)),
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
      setMonitor({ auctions: auctionsDiag, anomalies, outbox, outboxWatermarks, snapshots, signals, scheduler, rejects, recovery });
      const nextSelected = auctionRows.find((row) => row.id === selectedAuctionID)?.id ?? auctionRows.find((row) => row.status === 'ACTIVE')?.id ?? auctionRows[0]?.id ?? '';
      setSelectedAuctionID(nextSelected);
      setItems(auctionRows.map((auction) => auction.item).filter(Boolean));
      const nextAuction = auctionRows.find((row) => row.id === nextSelected) ?? auctionRows[0];
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

  return (
    <Layout className="console-shell">
      <Layout.Sider className="sider" width={224}>
        <ConsoleNav />
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

        <section className="command-center" data-testid="pc-command-center">
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
          <LiveAssistRail monitor={monitor} recentEvents={recentEvents} selectedAuction={selectedAuction} />
        </section>

        <section className="band two-column secondary-workspace" data-testid="secondary-workspace">
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
        </section>

        <section className="band two-column secondary-workspace">
          <OrdersPanel orders={orders} />
          <div className="workspace-note">
            <h2>Command Center Notes</h2>
            <p>ACTIVE 操作在上方主控区完成；规则、订单、诊断保留在二级工作区，所有数据仍来自后端 API。</p>
            <p>下一步 P8-S2 会细化队列 pinning 和讲解/ACTIVE 约束展示。</p>
          </div>
        </section>

        <DiagnosticsPanel
          monitor={monitor}
          monitorFilter={monitorFilter}
          onFilterChange={setMonitorFilter}
        />
      </Layout.Content>
    </Layout>
  );
}

function ConsoleNav() {
  return (
    <>
      <div className="brand">Live Auction</div>
      <nav>
        <span><ClipboardList size={16} /> 拍品</span>
        <span><RadioTower size={16} /> 竞拍</span>
        <span><Activity size={16} /> 诊断</span>
      </nav>
    </>
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
      <div className="ribbon-metrics">
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
    <div className="rule-panel">
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
            <Button disabled={!canStart} icon={<Play size={14} />} onClick={() => onAction('start')}>开拍</Button>
            <Button disabled={isTerminal} status="danger" icon={<Square size={14} />} onClick={() => {
              Modal.confirm({ title: '确认取消竞拍', content: selectedAuction.id, onOk: () => onAction('cancel') });
            }}>取消</Button>
            <Button disabled={!canNarrate} onClick={() => onAction('narrate-start')}>开始讲解</Button>
            <Button disabled={!selectedAuction.is_narrating} onClick={() => onAction('narrate-stop')}>停止讲解</Button>
          </Space>
          <div className="action-guardrail">
            {selectedAuction.status === 'DRAFT' && 'DRAFT 可编辑规则并排期；开拍后价格规则冻结。'}
            {selectedAuction.status === 'SCHEDULED' && !activeConflict && 'SCHEDULED 已冻结价格规则，可开拍或取消。'}
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
        <span>{auctions.length} lots</span>
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
            label="FINISHED"
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
  monitor,
  recentEvents,
  selectedAuction
}: {
  monitor: Record<string, MonitorPayload>;
  recentEvents: Array<Record<string, unknown>>;
  selectedAuction?: Auction;
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
  const nextBidSummary = selectedAuction.status === 'ACTIVE'
    ? `当前价 ${formatCents(selectedAuction.current_price_cents)} · 加价幅度 ${formatCents(selectedAuction.increment_cents)}`
    : `${selectedAuction.status} · ${selectedAuction.accepted_bid_count} accepted bids`;
  return (
    <aside className="assist-rail" data-testid="live-assist-rail">
      <div className="panel-heading">
        <h2>Live Assist</h2>
        <span>event-backed</span>
      </div>
      <div className="assist-card pending">
        <span>Prompter pending</span>
        <strong>{nextBidSummary}</strong>
        <small>P8-S3 接入 host-only prompts API 前，本栏不生成主播话术，也不自动发送系统弹幕。</small>
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
      <EventTimeline events={recentEvents} selectedAuction={selectedAuction} />
    </aside>
  );
}

function EventTimeline({
  events,
  selectedAuction
}: {
  events: Array<Record<string, unknown>>;
  selectedAuction: Auction;
}) {
  return (
    <div className="recent-events" data-testid="recent-events">
      <div className="recent-title">
        <strong>最近事件</strong>
        <a href={`/api/monitor/auctions/${encodeURIComponent(selectedAuction.id)}/flight-recorder?limit=50&timeline_limit=100`} target="_blank" rel="noreferrer">
          Flight recorder <ExternalLink size={13} />
        </a>
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
  return (
    <div className="rule-panel">
      <h2>规则 {selectedAuction ? selectedAuction.id : ''}</h2>
      <Form layout="vertical">
        <div className="rule-subgrid">
          <NumberField label="起拍价" name="start-price-cents" value={rule.startPriceCents} min={0} onChange={(value) => onRuleChange({ startPriceCents: value })} />
          <NumberField label="加价幅度" name="increment-cents" value={rule.incrementCents} min={1} onChange={(value) => onRuleChange({ incrementCents: value })} />
        </div>
        <Form.Item label="封顶价" validateStatus={ruleValidation.valid ? 'success' : 'error'} help={ruleValidation.message}>
          <InputNumber aria-label="cap-price-cents" value={rule.capPriceCents} min={0} suffix="分" onChange={(value) => onRuleChange({ capPriceCents: Number(value) || 0 })} />
        </Form.Item>
        {backendRuleError && <div className="backend-rule-error" role="alert">{backendRuleError}</div>}
        {ruleSaveState === 'saved' && <div className="rule-save-ok" role="status">规则已保存</div>}
        {!ruleValidation.valid && ruleValidation.field !== 'cap' && <div className="backend-rule-error" role="alert">{ruleValidation.message}</div>}
        {shownSuggestions.length > 0 && (
          <div className="cap-suggestions" data-testid="cap-suggestions">
            {shownSuggestions.map((cap) => <button key={cap} type="button" onClick={() => onRuleChange({ capPriceCents: cap })}>{formatCents(cap)}</button>)}
          </div>
        )}
        <div className="rule-subgrid">
          <NumberField label="时长" name="duration-seconds" value={rule.durationSeconds} min={30} max={86400} onChange={(value) => onRuleChange({ durationSeconds: value })} />
          <NumberField label="延时窗口" name="extend-window-seconds" value={rule.extendWindowSeconds} min={0} onChange={(value) => onRuleChange({ extendWindowSeconds: value })} />
          <NumberField label="每次延时" name="extend-by-seconds" value={rule.extendBySeconds} min={0} onChange={(value) => onRuleChange({ extendBySeconds: value })} />
          <NumberField label="最多延时" name="max-extend-count" value={rule.maxExtendCount} min={0} onChange={(value) => onRuleChange({ maxExtendCount: value })} />
          <NumberField label="高额确认" name="fat-finger-threshold-cents" value={rule.fatFingerThresholdCents} min={rule.incrementCents + 1} onChange={(value) => onRuleChange({ fatFingerThresholdCents: value })} />
          <NumberField label="保证金比例" name="deposit-bps" value={rule.depositBPS} min={0} max={10000} onChange={(value) => onRuleChange({ depositBPS: value })} />
          <NumberField label="保证金下限" name="deposit-floor-cents" value={rule.depositFloorCents} min={0} onChange={(value) => onRuleChange({ depositFloorCents: value })} />
          <NumberField label="保证金上限" name="deposit-cap-cents" value={rule.depositCapCents} min={0} onChange={(value) => onRuleChange({ depositCapCents: value })} />
        </div>
        <Button type="primary" disabled={!ruleValidation.valid || !selectedAuction || selectedAuction.status !== 'DRAFT'} loading={savingRule} onClick={onSave}>保存规则</Button>
      </Form>
    </div>
  );
}

function OrdersPanel({ orders }: { orders: Order[] }) {
  return (
    <div className="rule-panel">
      <h2>订单</h2>
      {orders.length === 0 ? <div className="empty-state">暂无订单</div> : orders.map((order) => (
        <div className="order-line" key={order.id}>
          <span>{order.id}</span>
          <Tag color={order.status === 'PAID' ? 'green' : 'orange'}>{order.status}</Tag>
          <strong>{formatCents(order.amount_cents)}</strong>
        </div>
      ))}
    </div>
  );
}

function DiagnosticsPanel({
  monitor,
  monitorFilter,
  onFilterChange
}: {
  monitor: Record<string, MonitorPayload>;
  monitorFilter: { type: string; auctionID: string; userID: string; traceID: string };
  onFilterChange: React.Dispatch<React.SetStateAction<{ type: string; auctionID: string; userID: string; traceID: string }>>;
}) {
  return (
    <section className="band diagnostics" data-testid="diagnostics">
      <div className="section-title">
        <h2>诊断</h2>
        <span><Database size={16} /> API</span>
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
        <Tabs.TabPane key="auctions" title="Auctions"><MonitorTable payload={monitor.auctions} empty="暂无竞拍诊断数据" sourceKey="auction_id" /></Tabs.TabPane>
        <Tabs.TabPane key="rejects" title="Rejects"><MonitorTable payload={monitor.rejects} empty="暂无拒绝出价" sourceKey="trace_id" icon={<AlertTriangle size={16} />} /></Tabs.TabPane>
        <Tabs.TabPane key="recovery" title="Recovery"><MonitorTable payload={monitor.recovery} empty="暂无恢复数据" sourceKey="room_id" /></Tabs.TabPane>
        <Tabs.TabPane key="anomalies" title="Anomalies"><MonitorTable payload={monitor.anomalies} empty="暂无异常" sourceKey="id" icon={<AlertTriangle size={16} />} /></Tabs.TabPane>
        <Tabs.TabPane key="outbox" title="Outbox"><MonitorTable payload={monitor.outbox} empty="暂无 outbox 数据" sourceKey="outbox_id" /></Tabs.TabPane>
        <Tabs.TabPane key="watermarks" title="Watermarks"><MonitorTable payload={monitor.outboxWatermarks} empty="暂无 outbox watermark" sourceKey="shard_id" /></Tabs.TabPane>
        <Tabs.TabPane key="snapshots" title="Snapshots"><MonitorTable payload={monitor.snapshots} empty="暂无 snapshot 记录" sourceKey="request_id" /></Tabs.TabPane>
        <Tabs.TabPane key="signals" title="Signals"><MonitorTable payload={monitor.signals} empty="暂无 control signal" sourceKey="id" /></Tabs.TabPane>
        <Tabs.TabPane key="scheduler" title="Scheduler"><MonitorTable payload={monitor.scheduler} empty="暂无 scheduler 数据" sourceKey="job_id" /></Tabs.TabPane>
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

function NumberField({ label, name, value, min, max, onChange }: { label: string; name: string; value: number; min: number; max?: number; onChange: (value: number) => void }) {
  return (
    <Form.Item label={label}>
      <InputNumber aria-label={name} value={value} min={min} max={max} suffix="分" onChange={(next) => onChange(Number(next) || min)} />
    </Form.Item>
  );
}

function MonitorTable({ payload, empty, icon, sourceKey }: { payload?: MonitorPayload; empty: string; icon?: React.ReactNode; sourceKey: string }) {
  const rows = payload?.items ?? [];
  if (rows.length === 0) return <div className="empty-state">{icon}{empty}</div>;
  const priorityKeys = [
    sourceKey,
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
  ])).slice(0, 10);
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
            const sourceURL = rowSourceURL(sourceKey, record);
            return sourceURL ? (
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

createRoot(document.getElementById('root')!).render(<App />);
