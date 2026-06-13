import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Button, Layout, Message, Modal } from '@arco-design/web-react';
import '@arco-design/web-react/dist/css/arco.css';

import { AICopilotDrawer, AuctionCommandPanel, AuctionControlSummary, AuctionQueue, ConsoleNav, CurrentAuctionOrderCard, DiagnosticsPanel, EventTimeline, FlightRecorderDrawer, HealthRibbon, InventoryLotsPanel, ItemCreatePanel, LiveAssistRail, LiveHealthPanel, OrderDetailDrawer, OrdersPanel, RuleEditor } from './components';
import type { Auction, AuctionAISettings, AuctionRecap, AuthUser, FlightRecorderPayload, HeatSummary, HighlightAsset, HostPrompt, HostPromptsPayload, Item, ListingDraftJob, LiveOpsHostSummary, LiveOpsRewardConfig, MonitorPayload, Order, RedisEngineMonitorPayload, Room, RuleAPIError, RuleDraft, SentinelAlert, SignalRequest, SystemMessage } from './domain';
import { activeAuction, auctionStatusLabel, createRuleDraft, defaultRoomID, depositPreview, ensureDemoSession, liveHealthSummary, monitorQuery, narratingAuction, readJSON, rulePayload, signalCopy, sortedAuctions, validateRule } from './domain';
import './styles.css';

function auctionDisplayName(auction?: Auction) {
  if (!auction) return '未选择拍品';
  return auction.item?.title ?? auction.item_id ?? '未命名拍品';
}

function businessErrorMessage(code?: string, fallback = '操作失败') {
  switch (code) {
    case 'ENGINE_PAUSED':
      return '出价确认已暂停，请先在运行监控查看引擎暂停原因并恢复后再操作。';
    case 'RECONCILING':
      return '竞拍状态正在校对，暂不能演示出价。请等待恢复或在运行监控处理。';
    case 'FORBIDDEN_ROOM':
      return '当前账号无权操作这场拍品，请刷新商家身份后重试。';
    case 'UNAUTHORIZED':
      return '商家登录已失效，请刷新页面重新进入。';
    case 'INVALID_ARGUMENT':
      return '请求内容不完整，请刷新拍品数据后重试。';
    default:
      return fallback;
  }
}

type WorkbenchTask = {
  active: boolean;
  title: string;
  detail: string;
  tone: 'loading' | 'ok' | 'error';
};

type BidDecisionPreview = {
  result?: string;
  reject_reason?: string;
  code?: string;
  message?: string;
  current_price_cents?: number;
  current_winner_id?: string;
  seq?: number;
  engine_seq?: number;
  end_at?: string;
};

const idleWorkbenchTask: WorkbenchTask = {
  active: false,
  title: '工作台已就绪',
  detail: '可以刷新数据、开拍或重置演示环境。',
  tone: 'ok'
};

function withTimeout<T>(promise: Promise<T>, timeoutMs: number, message: string): Promise<T> {
  let timer = 0;
  const timeout = new Promise<never>((_, reject) => {
    timer = window.setTimeout(() => reject(new Error(message)), timeoutMs);
  });
  return Promise.race([promise, timeout]).finally(() => window.clearTimeout(timer));
}

async function fetchJSONWithTimeout<T>(url: string, options?: RequestInit, timeoutMs = 12000) {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(url, { ...options, signal: controller.signal });
    const payload = await readJSON<T>(response);
    return { response, payload };
  } finally {
    window.clearTimeout(timer);
  }
}

async function fetchJSONWithHostRetry<T>(url: string, options?: RequestInit, timeoutMs = 12000) {
  let result = await fetchJSONWithTimeout<T>(url, options, timeoutMs);
  if (result.response.status === 401 || result.response.status === 403) {
    await withTimeout(ensureDemoSession('host'), 10000, 'auth timeout');
    result = await fetchJSONWithTimeout<T>(url, options, timeoutMs);
  }
  return result;
}

function asArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

function shouldKeepHotPreview(local: Auction | undefined, incoming: Auction) {
  if (!local) return false;
  const localSeq = Number(local.seq ?? 0);
  const incomingSeq = Number(incoming.seq ?? 0);
  if (!Number.isFinite(localSeq) || !Number.isFinite(incomingSeq)) return false;
  if (localSeq <= incomingSeq) return false;
  return incoming.status !== 'SOLD' && incoming.status !== 'ENDED' && incoming.status !== 'CANCELLED';
}

function mergeAuctionRowsPreservingHotPreview(current: Auction[], incoming: Auction[]) {
  const currentByID = new Map(current.map((row) => [row.id, row]));
  return incoming.map((row) => {
    const local = currentByID.get(row.id);
    if (!shouldKeepHotPreview(local, row) || !local) return row;
    return {
      ...row,
      current_price_cents: local.current_price_cents,
      current_winner_id: local.current_winner_id,
      end_at: local.end_at,
      seq: local.seq,
      accepted_bid_count: Math.max(row.accepted_bid_count ?? 0, local.accepted_bid_count ?? 0),
      extend_count: Math.max(row.extend_count ?? 0, local.extend_count ?? 0),
      item: row.item ?? local.item
    };
  });
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
  const [commentaryLoadingType, setCommentaryLoadingType] = useState('');
  const [heatSummary, setHeatSummary] = useState<HeatSummary | undefined>();
  const [heatLoading, setHeatLoading] = useState(false);
  const [liveOpsSummary, setLiveOpsSummary] = useState<LiveOpsHostSummary | undefined>();
  const [liveOpsDraft, setLiveOpsDraft] = useState<LiveOpsRewardConfig | undefined>();
  const [liveOpsSaving, setLiveOpsSaving] = useState(false);
  const [listingCopilotOpen, setListingCopilotOpen] = useState(false);
  const [listingNotes, setListingNotes] = useState('');
  const [listingCategory, setListingCategory] = useState('collectibles');
  const [listingDraftJob, setListingDraftJob] = useState<ListingDraftJob | undefined>();
  const [selectedListingDraftTitle, setSelectedListingDraftTitle] = useState('');
  const [listingDraftLoading, setListingDraftLoading] = useState(false);
  const [copilotImageFile, setCopilotImageFile] = useState<File | null>(null);
  const [copilotImagePreviewURL, setCopilotImagePreviewURL] = useState('');
  const [copilotImageURL, setCopilotImageURL] = useState('');
  const [systemMessages, setSystemMessages] = useState<SystemMessage[]>([]);
  const [sentinelAlerts, setSentinelAlerts] = useState<SentinelAlert[]>([]);
  const [latestRecap, setLatestRecap] = useState<AuctionRecap | undefined>();
  const [auctionAISettings, setAuctionAISettings] = useState<AuctionAISettings | undefined>();
  const [monitorFilter, setMonitorFilter] = useState({ type: '', auctionID: '', userID: '', traceID: '' });
  const [loading, setLoading] = useState(false);
  const [resettingSeed, setResettingSeed] = useState(false);
  const [workbenchTask, setWorkbenchTask] = useState<WorkbenchTask>({
    active: true,
    title: '正在进入商家工作台',
    detail: '正在确认商家身份。',
    tone: 'loading'
  });
  const [resetTask, setResetTask] = useState<WorkbenchTask>(idleWorkbenchTask);
  const [savingRule, setSavingRule] = useState(false);
  const [creating, setCreating] = useState(false);
  const [auctionActionPending, setAuctionActionPending] = useState('');
  const [ruleSaveState, setRuleSaveState] = useState<'idle' | 'saved' | 'error'>('idle');
  const [workspaceTab, setWorkspaceTab] = useState('rules');
  const [siderCollapsed, setSiderCollapsed] = useState(false);
  const [backendRuleError, setBackendRuleError] = useState('');
  const [backendSuggestions, setBackendSuggestions] = useState<number[]>([]);
  const [itemDraft, setItemDraft] = useState({ title: '新拍品', description: '本场直播竞拍拍品', imageURL: '' });
  const [itemImageFile, setItemImageFile] = useState<File | null>(null);
  const [scheduleStartAt, setScheduleStartAt] = useState('');
  const [cancelReason, setCancelReason] = useState('');
  const [rule, setRule] = useState<RuleDraft>(createRuleDraft());
  const [sessionReady, setSessionReady] = useState(false);
  const [now, setNow] = useState(Date.now());
  const skipNextAutoLoad = useRef(false);
  const selectedAuctionRef = useRef<Auction | undefined>();
  const selectedAuction = useMemo(() => auctions.find((auction) => auction.id === selectedAuctionID) ?? sortedAuctions(auctions)[0], [auctions, selectedAuctionID]);
  const pinnedActiveAuction = useMemo(() => activeAuction(auctions), [auctions]);
  const currentNarratingAuction = useMemo(() => narratingAuction(auctions), [auctions]);
  const ruleValidation = validateRule(rule);
  const shownSuggestions = ruleValidation.valid ? backendSuggestions : ruleValidation.suggestions;
  const visibleTask = resettingSeed || resetTask.tone === 'error' || (resetTask.title === '演示环境已重置' && !loading)
    ? resetTask
    : workbenchTask;

  const applyAuctionDecisionPreview = (auctionID: string, payload: BidDecisionPreview) => {
    if (!auctionID || payload.current_price_cents == null) return;
    setAuctions((current) => current.map((row) => {
      if (row.id !== auctionID) return row;
      const responseSeq = Number(payload.engine_seq ?? payload.seq ?? row.seq ?? 0);
      const currentSeq = Number(row.seq ?? 0);
      return {
        ...row,
        current_price_cents: payload.current_price_cents ?? row.current_price_cents,
        current_winner_id: payload.current_winner_id ?? row.current_winner_id,
        end_at: payload.end_at ?? row.end_at,
        seq: Number.isFinite(responseSeq) && responseSeq > currentSeq ? responseSeq : row.seq
      };
    }));
    setSelectedAuctionID(auctionID);
  };

  const openFlightRecorder = async (auctionID: string) => {
    const nextAuctionID = auctionID.trim();
    if (!nextAuctionID) return;
    setFlightRecorderAuctionID(nextAuctionID);
    setFlightRecorderLoading(true);
    setFlightRecorder(undefined);
    try {
      const { response, payload } = await fetchJSONWithHostRetry<FlightRecorderPayload>(
        `/api/monitor/auctions/${encodeURIComponent(nextAuctionID)}/flight-recorder?limit=80&timeline_limit=120`,
        undefined,
        8000
      );
      if (!response.ok) throw new Error('flight recorder query failed');
      setFlightRecorder(payload);
    } catch {
      Message.error('事件回放读取失败');
      setFlightRecorder(undefined);
    } finally {
      setFlightRecorderLoading(false);
    }
  };

  const createMonitorSignal = async (request: SignalRequest) => {
    try {
      const { response, payload } = await fetchJSONWithHostRetry<{ id?: number; message?: string }>('/api/monitor/signals', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request)
      });
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

  const refreshDiagnostics = async (nextRoomID: string) => {
    const [
      auctionsDiag,
      redisEngine,
      anomalies,
      outbox,
      outboxWatermarks,
      snapshots,
      signals,
      scheduler,
      rejects,
      recovery
    ] = await Promise.all([
      fetchJSONWithHostRetry<MonitorPayload>('/api/monitor/auctions', undefined, 6000).then(({ payload }) => payload).catch(() => monitor.auctions),
      fetchJSONWithHostRetry<RedisEngineMonitorPayload>('/api/monitor/redis-engine', undefined, 6000).then(({ payload }) => payload).catch(() => monitor.redisEngine),
      fetchJSONWithHostRetry<MonitorPayload>(`/api/monitor/anomalies?${monitorQuery(nextRoomID, monitorFilter)}`, undefined, 6000).then(({ payload }) => payload).catch(() => monitor.anomalies),
      fetchJSONWithHostRetry<MonitorPayload>('/api/monitor/outbox', undefined, 6000).then(({ payload }) => payload).catch(() => monitor.outbox),
      fetchJSONWithHostRetry<MonitorPayload>('/api/monitor/outbox/watermarks', undefined, 6000).then(({ payload }) => payload).catch(() => monitor.outboxWatermarks),
      fetchJSONWithHostRetry<MonitorPayload>('/api/monitor/snapshots', undefined, 6000).then(({ payload }) => payload).catch(() => monitor.snapshots),
      fetchJSONWithHostRetry<MonitorPayload>('/api/monitor/signals', undefined, 6000).then(({ payload }) => payload).catch(() => monitor.signals),
      fetchJSONWithHostRetry<MonitorPayload>('/api/monitor/scheduler', undefined, 6000).then(({ payload }) => payload).catch(() => monitor.scheduler),
      fetchJSONWithHostRetry<MonitorPayload>('/api/monitor/rejects', undefined, 6000).then(({ payload }) => payload).catch(() => monitor.rejects),
      fetchJSONWithHostRetry<MonitorPayload>('/api/monitor/recovery', undefined, 6000).then(({ payload }) => payload).catch(() => monitor.recovery)
    ]);
    setMonitor({ auctions: auctionsDiag, redisEngine, anomalies, outbox, outboxWatermarks, snapshots, signals, scheduler, rejects, recovery });
    setWorkbenchTask((current) => current.tone === 'error'
      ? current
      : { active: false, title: '工作台已刷新', detail: '拍品、订单和直播数据已同步。', tone: 'ok' });
  };

  const refreshAuctionRows = async (targetRoomID = roomID, preferredAuctionID = selectedAuctionID) => {
    if (!sessionReady || !targetRoomID) return;
    try {
      const { response, payload } = await fetchJSONWithHostRetry<Auction[]>(
        `/api/auctions?room_id=${targetRoomID}`,
        undefined,
        6000
      );
      if (!response.ok) return;
      const auctionRows = asArray(payload);
      setAuctions((current) => mergeAuctionRowsPreservingHotPreview(current, auctionRows));
      setItems(auctionRows.map((auction) => auction.item).filter(Boolean));
      const visibleAuctionID = preferredAuctionID && auctionRows.some((auction) => auction.id === preferredAuctionID)
        ? preferredAuctionID
        : auctionRows.find((auction) => auction.status === 'ACTIVE')?.id ?? auctionRows[0]?.id ?? '';
      if (visibleAuctionID) {
        setSelectedAuctionID(visibleAuctionID);
        const ordersResult = await fetchJSONWithHostRetry<Order[]>(
          `/api/orders?auction_id=${encodeURIComponent(visibleAuctionID)}&limit=20`,
          undefined,
          6000
        );
        if (ordersResult.response.ok) setOrders(asArray(ordersResult.payload));
      }
    } catch {
      // Full refresh surfaces errors; the live price poll should stay quiet.
    }
  };

  const loadAll = async (
    preferredRoomID = roomID,
    force = false,
    preserveResetResult = false,
    preferredAuctionID = selectedAuctionID,
    options: { includeDiagnostics?: boolean; suppressFailure?: boolean } = {}
  ) => {
    if (!sessionReady && !force) return;
    const includeDiagnostics = options.includeDiagnostics ?? true;
    const suppressFailure = options.suppressFailure ?? false;
    if (!preserveResetResult) setResetTask(idleWorkbenchTask);
    setLoading(true);
    setWorkbenchTask({ active: true, title: '正在刷新商家工作台', detail: '正在读取直播间和商家身份状态。', tone: 'loading' });
    try {
      await withTimeout(ensureDemoSession('host'), 10000, 'auth timeout');
      const { response: roomResponse, payload: roomPayload } = await fetchJSONWithHostRetry<{ items?: Room[] }>('/api/rooms', undefined, 10000);
      if (!roomResponse.ok) throw new Error('rooms failed');
      const roomRows = asArray(roomPayload.items);
      const nextRoomID = roomRows.find((room) => room.id === preferredRoomID)?.id
        ?? roomRows.find((room) => room.id === defaultRoomID)?.id
        ?? roomRows[0]?.id
        ?? preferredRoomID;
      setWorkbenchTask({ active: true, title: '正在刷新商家工作台', detail: '正在读取拍品、竞拍和订单。', tone: 'loading' });
      const [auctionPayload, liveOps] = await Promise.all([
        fetchJSONWithHostRetry<Auction[]>(`/api/auctions?room_id=${nextRoomID}`).then(({ response, payload }) => {
          if (!response.ok) throw new Error('auctions failed');
          return payload;
        }),
        fetchJSONWithHostRetry<LiveOpsHostSummary>(`/api/host/rooms/${nextRoomID}/liveops`).then(({ response, payload }) => {
          if (!response.ok) throw new Error('liveops failed');
          return payload;
        }).catch(() => undefined)
      ]);
      const auctionRows = asArray(auctionPayload);
      const nextSelected = auctionRows.find((row) => row.id === preferredAuctionID)?.id
        ?? auctionRows.find((row) => row.id === selectedAuctionID)?.id
        ?? auctionRows.find((row) => row.status === 'ACTIVE')?.id
        ?? sortedAuctions(auctionRows)[0]?.id
        ?? '';
      const orderRows = nextSelected
        ? await fetchJSONWithHostRetry<Order[]>(`/api/orders?auction_id=${encodeURIComponent(nextSelected)}&limit=20`).then(({ response, payload }) => {
          if (!response.ok) throw new Error('orders failed');
          return asArray(payload);
        })
        : [];
      setWorkbenchTask({ active: true, title: '正在刷新商家工作台', detail: '正在更新页面。', tone: 'loading' });
      setRooms(roomRows);
      if (nextRoomID !== roomID) setRoomID(nextRoomID);
      setAuctions((current) => mergeAuctionRowsPreservingHotPreview(current, auctionRows));
      setOrders(orderRows);
      if (liveOps) {
        setLiveOpsSummary(liveOps);
        setLiveOpsDraft(liveOps.reward_config);
      }
      setSelectedAuctionID(nextSelected);
      setItems(auctionRows.map((auction) => auction.item).filter(Boolean));
      const nextAuction = auctionRows.find((row) => row.id === nextSelected) ?? sortedAuctions(auctionRows)[0];
      if (nextAuction) setRule(createRuleDraft(nextAuction));
      setWorkbenchTask({
        active: false,
        title: includeDiagnostics ? '工作台可操作' : '工作台已刷新',
        detail: includeDiagnostics ? '拍品和订单已加载，直播数据正在后台补齐。' : '当前拍品已更新。',
        tone: 'ok'
      });
      if (includeDiagnostics) void refreshDiagnostics(nextRoomID);
    } catch (error) {
      if (suppressFailure) {
        setWorkbenchTask({
          active: false,
          title: '操作已完成，刷新稍后重试',
          detail: '拍品操作已提交成功，当前只是后台数据同步未及时返回，可继续操作或点重试刷新。',
          tone: 'ok'
        });
      } else {
        setWorkbenchTask({
          active: false,
          title: '商家工作台刷新失败',
          detail: error instanceof Error && (error.name === 'AbortError' || error.message.includes('timeout'))
            ? '核心数据暂时没有返回，请确认后端服务是否仍在运行，然后重试。'
            : '核心数据没有成功返回，请检查后端服务或点击刷新重试。',
          tone: 'error'
        });
        Message.error('商家工作台刷新失败');
      }
    } finally {
      setLoading(false);
    }
  };

  const saveLiveOpsReward = async () => {
    if (!liveOpsDraft) return;
    setLiveOpsSaving(true);
    try {
      await ensureHostSession();
      const { response, payload } = await fetchJSONWithHostRetry<LiveOpsHostSummary & { message?: string }>(`/api/host/rooms/${roomID}/liveops`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(liveOpsDraft)
      });
      if (!response.ok) {
        Message.error(payload.message ?? '互动权益保存失败');
        return;
      }
      setLiveOpsSummary(payload);
      setLiveOpsDraft(payload.reward_config);
      Message.success('互动权益已保存');
    } catch {
      Message.error('互动权益保存失败');
    } finally {
      setLiveOpsSaving(false);
    }
  };

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setWorkbenchTask({ active: true, title: '正在进入商家工作台', detail: '正在确认商家身份和操作权限。', tone: 'loading' });
    withTimeout(ensureDemoSession('host'), 10000, 'auth timeout')
      .then(async () => {
        if (cancelled) return;
        skipNextAutoLoad.current = true;
        setSessionReady(true);
        await loadAll(roomID, true);
      })
      .catch(() => {
        if (!cancelled) {
          setWorkbenchTask({
            active: false,
            title: '商家身份确认失败',
            detail: '10 秒内没有完成登录或权限校验，请确认后端服务可用后重试。',
            tone: 'error'
          });
          Message.error('登录失败，请检查后端服务');
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!sessionReady) return;
    if (skipNextAutoLoad.current) {
      skipNextAutoLoad.current = false;
      return;
    }
    void loadAll();
  }, [sessionReady, roomID, monitorFilter.type, monitorFilter.auctionID, monitorFilter.userID, monitorFilter.traceID]);

  useEffect(() => {
    if (!sessionReady || !roomID) return undefined;
    const timer = window.setInterval(() => {
      const activeID = selectedAuctionRef.current?.id || selectedAuctionID;
      void refreshAuctionRows(roomID, activeID);
    }, 1500);
    return () => window.clearInterval(timer);
  }, [roomID, selectedAuctionID, sessionReady]);

  useEffect(() => {
    selectedAuctionRef.current = selectedAuction;
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
        const { response, payload } = await fetchJSONWithHostRetry<FlightRecorderPayload>(
          `/api/monitor/auctions/${selectedAuction.id}/flight-recorder?limit=20&timeline_limit=20`,
          undefined,
          6000
        );
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
        const { response, payload } = await fetchJSONWithHostRetry<HostPromptsPayload>(
          `/api/host/auctions/${selectedAuction.id}/prompts`,
          undefined,
          6000
        );
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
        const { response, payload } = await fetchJSONWithHostRetry<HeatSummary>(
          `/api/host/auctions/${selectedAuction.id}/heat-summary`,
          undefined,
          6000
        );
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
    const loadSystemMessages = async () => {
      if (!sessionReady || !roomID) {
        setSystemMessages([]);
        return;
      }
      try {
        const { response, payload } = await fetchJSONWithHostRetry<{ items?: SystemMessage[] }>(
          `/api/rooms/${encodeURIComponent(roomID)}/system-messages?limit=10`,
          undefined,
          6000
        );
        if (!cancelled) setSystemMessages(response.ok ? (payload.items ?? []) : []);
      } catch {
        if (!cancelled) setSystemMessages([]);
      }
    };
    void loadSystemMessages();
    return () => {
      cancelled = true;
    };
  }, [sessionReady, roomID, selectedAuction?.seq, loading]);

  useEffect(() => {
    let cancelled = false;
    const loadAISettings = async () => {
      if (!sessionReady || !selectedAuction) {
        setAuctionAISettings(undefined);
        return;
      }
      try {
        const { response, payload } = await fetchJSONWithHostRetry<AuctionAISettings>(
          `/api/host/auctions/${selectedAuction.id}/ai-settings`,
          undefined,
          6000
        );
        if (!cancelled && response.ok) setAuctionAISettings(payload);
      } catch {
        if (!cancelled) setAuctionAISettings(undefined);
      }
    };
    void loadAISettings();
    return () => {
      cancelled = true;
    };
  }, [sessionReady, selectedAuction?.id, loading]);

  const updateRule = (patch: Partial<RuleDraft>) => {
    setRule((current) => ({ ...current, ...patch }));
    setRuleSaveState('idle');
    setBackendRuleError('');
    setBackendSuggestions([]);
  };

  const ensureHostSession = async () => {
    await withTimeout(ensureDemoSession('host'), 10000, 'auth timeout');
  };

  const uploadItemImage = async (file: File) => {
    await ensureHostSession();
    const data = new FormData();
    data.append('file', file);
    const { response: upload, payload } = await fetchJSONWithTimeout<{ public_url?: string; message?: string; code?: string }>('/api/items/upload', {
      method: 'POST',
      body: data
    }, 12000);
    if (!upload.ok) throw new Error(payload.message || 'image upload failed');
    if (!payload.public_url) throw new Error('missing uploaded image url');
    return payload.public_url ?? '';
  };

  const createItemAndAuction = async () => {
    setCreating(true);
    try {
      await ensureHostSession();
      let imageURL = itemDraft.imageURL.trim();
      if (itemImageFile) {
        setWorkbenchTask({ active: true, title: '正在创建拍品', detail: '正在上传拍品图片。', tone: 'loading' });
        Message.info('正在上传拍品图片');
        imageURL = await uploadItemImage(itemImageFile) || imageURL;
      }
      setWorkbenchTask({ active: true, title: '正在创建拍品', detail: '正在写入拍品资料。', tone: 'loading' });
      Message.info('正在创建拍品');
      const { response: itemResponse, payload: itemPayload } = await fetchJSONWithTimeout<Item & { message?: string }>('/api/items', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: itemDraft.title, description: itemDraft.description, image_url: imageURL || null })
      }, 12000);
      if (!itemResponse.ok) throw new Error(itemPayload.message || 'create item failed');
      const item = itemPayload as Item;
      setWorkbenchTask({ active: true, title: '正在创建拍品', detail: '正在写入竞拍规则。', tone: 'loading' });
      Message.info('正在创建竞拍规则');
      const { response: auctionResponse, payload: auctionPayload } = await fetchJSONWithTimeout<Auction & { message?: string }>('/api/auctions', {
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
      }, 12000);
      if (!auctionResponse.ok) throw new Error(auctionPayload.message || 'create auction failed');
      const auction = auctionPayload as Auction;
      const createdAuction = { ...auction, item };
      setAuctions((current) => [createdAuction, ...current.filter((row) => row.id !== createdAuction.id)]);
      setItems((current) => [item, ...current.filter((row) => row.id !== item.id)]);
      setSelectedAuctionID(auction.id);
      setRule(createRuleDraft(createdAuction));
      setListingDraftJob(undefined);
      setWorkbenchTask({ active: false, title: '拍品已创建', detail: '拍品和竞拍规则已写入后端，后台正在补充刷新列表。', tone: 'ok' });
      Message.success('拍品和竞拍已创建');
      setItemImageFile(null);
      void loadAll(roomID, false, false, auction.id, { includeDiagnostics: false, suppressFailure: true });
    } catch (error) {
      const message = error instanceof Error ? error.message : '';
      if (message.includes('image') || message.includes('upload')) {
        Message.error('图片上传失败，请确认图片格式和服务端存储状态');
      } else if (message.includes('item')) {
        Message.error('拍品创建失败，请检查标题、描述和图片地址');
      } else if (message.includes('auction')) {
        Message.error('竞拍创建失败，请检查价格、封顶和时间规则');
      } else {
        Message.error('创建失败，请检查规则和服务端状态');
      }
    } finally {
      setCreating(false);
    }
  };

  const saveRule = async () => {
    if (!ruleValidation.valid || !selectedAuction) return;
    if (selectedAuction.status !== 'DRAFT') {
      const statusCopy = auctionStatusLabel(selectedAuction.status);
      setRuleSaveState('error');
      setBackendRuleError(`当前拍品为“${statusCopy}”，规则已冻结。请先取消排期回到草稿，或新建草稿拍品后再修改规则。`);
      setWorkbenchTask({
        active: false,
        title: '规则已冻结',
        detail: `当前拍品为“${statusCopy}”，后端只允许草稿拍品修改竞拍规则。`,
        tone: 'error'
      });
      return;
    }
    setSavingRule(true);
    setRuleSaveState('idle');
    setBackendRuleError('');
    setBackendSuggestions([]);
    try {
      await ensureHostSession();
      setWorkbenchTask({ active: true, title: '正在保存规则', detail: '正在提交当前拍品的价格和倒计时规则。', tone: 'loading' });
      const { response, payload } = await fetchJSONWithTimeout<Auction | RuleAPIError>(`/api/auctions/${selectedAuction.id}/rules`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          start_price_cents: rule.startPriceCents,
          increment_cents: rule.incrementCents,
          cap_price_cents: rule.capPriceCents,
          ...rulePayload(rule)
        })
      }, 12000);
      if (!response.ok) {
        const errorPayload = payload as RuleAPIError;
        const frozen = errorPayload.code === 'RULE_FROZEN_AFTER_SCHEDULED';
        const message = frozen
          ? '当前拍品规则已冻结。只有草稿拍品可以保存规则，请先取消排期或新建草稿。'
          : errorPayload.code === 'INVALID_AUCTION_RULE_CAP_UNREACHABLE'
            ? '后端拒绝：封顶价不可达'
            : errorPayload.message ?? '规则保存失败';
        setRuleSaveState('error');
        setBackendRuleError(message);
        setBackendSuggestions(errorPayload.details?.suggested_caps ?? []);
        setWorkbenchTask({ active: false, title: frozen ? '规则已冻结' : '规则未保存', detail: message, tone: 'error' });
        return;
      }
      const updated = payload as Auction;
      setAuctions((current) => current.map((row) => row.id === updated.id ? { ...row, ...updated, item: updated.item ?? row.item } : row));
      setSelectedAuctionID(updated.id);
      setRule(createRuleDraft({ ...selectedAuction, ...updated, item: updated.item ?? selectedAuction.item }));
      setRuleSaveState('saved');
      setWorkbenchTask({ active: false, title: '规则已保存', detail: '后端已接受新规则，当前拍品已更新。', tone: 'ok' });
      Message.success('规则已保存');
      void loadAll(roomID, false, false, updated.id, { includeDiagnostics: false, suppressFailure: true });
    } catch (error) {
      setRuleSaveState('error');
      const timeout = error instanceof Error && (error.name === 'AbortError' || error.message.includes('timeout'));
      setBackendRuleError(timeout ? '保存请求超时，请重试' : '规则保存失败');
      setWorkbenchTask({
        active: false,
        title: '规则保存失败',
        detail: timeout ? '后端 12 秒内没有确认保存结果，请稍后重试。' : '规则没有写入成功，请检查后端服务状态。',
        tone: 'error'
      });
    } finally {
      setSavingRule(false);
    }
  };

  const auctionAction = async (action: 'schedule' | 'unschedule' | 'start' | 'cancel' | 'narrate-start' | 'narrate-stop') => {
    if (!selectedAuction || auctionActionPending) return;
    const actionAuctionID = selectedAuction.id;
    setAuctionActionPending(action);
    try {
      await ensureHostSession();
      const { response, payload } = await fetchJSONWithTimeout<Auction | RuleAPIError>(`/api/auctions/${actionAuctionID}/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: action === 'cancel'
          ? JSON.stringify({ reason: cancelReason.trim() || '商家取消本场竞拍' })
          : action === 'schedule'
            ? JSON.stringify({ start_at: scheduleStartAt ? new Date(scheduleStartAt).toISOString() : null })
            : undefined
      }, 12000);
      if (!response.ok) {
        const err = payload as RuleAPIError;
        Message.error(businessErrorMessage(err.code, err.message ?? `${action} failed`));
        return;
      }
      const updated = payload as Auction;
      setAuctions((current) => current.map((row) => row.id === updated.id ? { ...row, ...updated, item: updated.item ?? row.item } : row));
      setSelectedAuctionID(updated.id);
      setRule(createRuleDraft({ ...selectedAuction, ...updated, item: updated.item ?? selectedAuction.item }));
      if (action === 'schedule') {
        Message.success('已排期，可直接开拍');
      } else if (action === 'start') {
        Message.success('已开拍');
      } else {
        Message.success('操作已提交');
      }
      void loadAll(roomID, false, false, actionAuctionID, { includeDiagnostics: false, suppressFailure: true });
    } catch (error) {
      const timeout = error instanceof Error && (error.name === 'AbortError' || error.message.includes('timeout'));
      Message.error(timeout ? '操作请求超时，请刷新后重试' : '操作失败');
    } finally {
      setAuctionActionPending('');
    }
  };

  const resetDemoSeed = () => {
    Modal.confirm({
      title: '重置演示环境',
      content: '将清空 room_main/room_side 的演示拍品、出价、订单、事件和热状态，并恢复默认示例数据。',
      okText: '确认重置',
      cancelText: '取消',
      okButtonProps: { status: 'danger' },
      onOk: async () => {
        setResettingSeed(true);
        setResetTask({ active: true, title: '正在重置演示环境', detail: '正在切换到商家身份，确保有权限清理数据。', tone: 'loading' });
        try {
          await withTimeout(ensureDemoSession('host'), 10000, 'auth timeout');
          setResetTask({ active: true, title: '正在重置演示环境', detail: '正在清理旧拍品、出价、订单和事件。', tone: 'loading' });
          const { response, payload } = await fetchJSONWithTimeout<{ message?: string; active_room?: string; active_id?: string }>('/api/demo/reset-seed', { method: 'POST' }, 20000);
          if (!response.ok) {
            setResetTask({ active: false, title: '演示环境重置失败', detail: payload.message ?? '后端拒绝了重置请求，请确认当前是商家身份。', tone: 'error' });
            Message.error(payload.message ?? '演示环境重置失败');
            return;
          }
          setResetTask({ active: true, title: '正在重置演示环境', detail: '默认拍品已写入，正在刷新商家工作台。', tone: 'loading' });
          const nextRoomID = payload.active_room || defaultRoomID;
          setRoomID(nextRoomID);
          setSelectedAuctionID(payload.active_id || 'auc_live');
          setFlightRecorderAuctionID('');
          setFlightRecorder(undefined);
          setRecentEvents([]);
          setLatestRecap(undefined);
          await loadAll(nextRoomID, true, true, payload.active_id || 'auc_live');
          setResetTask({ active: false, title: '演示环境已重置', detail: '默认直播间、拍品、倒计时、订单和演示数据已恢复。', tone: 'ok' });
          Message.success('演示环境已重置');
        } catch (error) {
          setResetTask({
            active: false,
            title: '演示环境重置失败',
            detail: error instanceof Error && error.name === 'AbortError'
              ? '重置请求超过 20 秒未返回，请查看后端日志或稍后重试。'
              : '重置没有完成，请确认后端、数据库和商家登录状态后重试。',
            tone: 'error'
          });
          Message.error('演示环境重置失败');
        } finally {
          setResettingSeed(false);
        }
      }
    });
  };

  const driveDemoBid = async (
    mode: 'reject' | 'stale_low' | 'outbid' | 'challenge' | 'extend' | 'sold' | 'rival_max_bid',
    options: { amountCents?: number } = {}
  ) => {
    if (!selectedAuction || selectedAuction.status !== 'ACTIVE') {
      Message.warning('请选择开拍中的拍品后再演示');
      return;
    }
    const runOne = async (stepMode: Exclude<typeof mode, 'duel'>, index = 0) => {
      const { response, payload } = await fetchJSONWithHostRetry<BidDecisionPreview>(`/api/demo/auctions/${selectedAuction.id}/competing-bid`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          mode: stepMode,
          amount_cents: options.amountCents,
          client_bid_id: `host-demo-${stepMode}-${Date.now()}-${index}`
        })
      }, 12000);
      const proxyDefenseAccepted = payload.reject_reason === 'BID_TOO_LOW' &&
        stepMode !== 'reject' &&
        stepMode !== 'stale_low' &&
        payload.current_winner_id === 'user_1' &&
        Number(payload.current_price_cents ?? 0) > Number(selectedAuction.current_price_cents ?? 0);
      if (!response.ok || (payload.reject_reason && stepMode !== 'reject' && stepMode !== 'stale_low' && !proxyDefenseAccepted)) {
        throw new Error(businessErrorMessage(payload.code ?? payload.reject_reason, payload.message ?? payload.reject_reason ?? '演示出价失败'));
      }
      return payload;
    };
    try {
      await ensureHostSession();
      if (mode === 'rival_max_bid') {
        const payload = await runOne(mode);
        applyAuctionDecisionPreview(selectedAuction.id, payload);
        Message.success(`已为演示对手设置自动加价${options.amountCents ? `：¥${(options.amountCents / 100).toFixed(2)}` : ''}`);
      } else {
        const payload = await runOne(mode);
        applyAuctionDecisionPreview(selectedAuction.id, payload);
        const proxyDefenseAccepted = payload.reject_reason === 'BID_TOO_LOW' &&
          payload.current_winner_id === 'user_1' &&
          Number(payload.current_price_cents ?? 0) > Number(selectedAuction.current_price_cents ?? 0);
        const copy = proxyDefenseAccepted
          ? `买家自动代理已防守到 ¥${((payload.current_price_cents ?? 0) / 100).toFixed(2)}`
          : mode === 'outbid'
            ? '对手已真实压过买家'
            : mode === 'challenge'
              ? '第三方挑战已写入'
            : mode === 'extend'
              ? '末段延时出价已写入'
              : mode === 'sold'
                ? '封顶成交已写入'
                : mode === 'stale_low'
                  ? '旧价请求已被热引擎拒绝'
                  : '无效出价已写入';
        Message.success(proxyDefenseAccepted ? `${copy}：代理防守成功` : `${copy}：${payload.result ?? mode}`);
      }
      void refreshAuctionRows(roomID, selectedAuction.id);
    } catch (error) {
      const timeout = error instanceof Error && (error.name === 'AbortError' || error.message.includes('timeout'));
      Message.error(timeout ? '演示出价请求超时，请刷新后重试' : error instanceof Error ? error.message : '演示出价失败');
    }
  };

  const shortenDemoCountdown = async () => {
    if (!selectedAuction || selectedAuction.status !== 'ACTIVE') {
      Message.warning('请选择开拍中的拍品后再缩短倒计时');
      return;
    }
    try {
      await ensureHostSession();
      const { response, payload } = await fetchJSONWithHostRetry<RuleAPIError & Auction>(`/api/demo/auctions/${selectedAuction.id}/shorten-end`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ remaining_seconds: 15 })
      }, 12000);
      if (!response.ok) {
        Message.error(payload.message ?? '倒计时缩短失败');
        return;
      }
      Message.success('倒计时已缩短到约 15 秒');
      void loadAll(roomID, false, false, selectedAuction.id, { includeDiagnostics: false, suppressFailure: true });
    } catch {
      Message.error('倒计时缩短失败');
    }
  };

  const generateListingDraft = async () => {
    setListingDraftLoading(true);
    setListingDraftJob(undefined);
    setSelectedListingDraftTitle('');
    try {
      await ensureHostSession();
      let imageURL = copilotImageURL.trim() || itemDraft.imageURL.trim();
      let imageDataURL = '';
      if (copilotImageFile) {
        if (copilotImageFile.size <= 2_000_000) {
          imageDataURL = await fileToDataURL(copilotImageFile);
        } else {
          Message.warning('图片超过 2MB，已保存到表单，智能草稿将按文字备注生成');
        }
        imageURL = await uploadItemImage(copilotImageFile) || imageURL;
        setCopilotImageURL(imageURL);
        setCopilotImageFile(null);
        setItemDraft((current) => ({ ...current, imageURL: imageURL || current.imageURL }));
      }
      const providerImageURLs = imageURL.startsWith('https://') ? [imageURL] : [];
      const providerImageDataURLs = imageDataURL ? [imageDataURL] : [];
      const fallbackNotes = [
        itemDraft.title.trim(),
        itemDraft.description.trim()
      ].filter(Boolean).join('。');
      const sellerNotes = listingNotes.trim() || fallbackNotes;
      const { response, payload } = await fetchJSONWithHostRetry<ListingDraftJob & { message?: string }>('/api/host/ai/listing-drafts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          room_id: roomID,
          image_urls: providerImageURLs,
          image_data_urls: providerImageDataURLs,
          seller_notes: sellerNotes,
          target_category: listingCategory.trim()
        })
      }, 30000);
      if (!response.ok) {
        Message.error(payload.message ?? '智能草稿生成失败');
        return;
      }
      setListingDraftJob(payload);
      setSelectedListingDraftTitle(payload.output_json?.title_candidates?.[0] ?? '');
      if (imageURL && providerImageURLs.length === 0 && providerImageDataURLs.length === 0) {
        Message.warning('图片已保存到拍品表单；智能草稿未读取大图，请在备注里补充关键信息');
      }
      Message.success('智能草稿已生成，需商家确认后采用');
    } catch {
      Message.error('智能草稿生成失败');
    } finally {
      setListingDraftLoading(false);
    }
  };

  const applyListingDraft = async () => {
    const output = listingDraftJob?.output_json;
    if (!listingDraftJob || !output) return;
    const selectedTitle = selectedListingDraftTitle || output.title_candidates?.[0];
    setItemDraft((current) => ({
      ...current,
      title: selectedTitle || current.title,
      description: output.description || current.description
    }));
    const suggestion = output.rule_suggestion;
    if (suggestion) {
      updateRule({
        startPriceCents: suggestion.start_price_cents ?? rule.startPriceCents,
        incrementCents: suggestion.increment_cents ?? rule.incrementCents,
        capPriceCents: suggestion.cap_price_cents ?? rule.capPriceCents,
        durationSeconds: suggestion.duration_seconds ?? rule.durationSeconds,
        extendWindowSeconds: suggestion.extend_window_seconds ?? rule.extendWindowSeconds,
        extendBySeconds: suggestion.extend_by_seconds ?? rule.extendBySeconds,
        maxExtendCount: suggestion.max_extend_count ?? rule.maxExtendCount,
        fatFingerThresholdCents: suggestion.fat_finger_threshold_cents ?? rule.fatFingerThresholdCents
      });
    }
    try {
      await ensureHostSession();
      await fetch(`/api/host/ai/listing-drafts/${listingDraftJob.id}/apply`, { method: 'POST' });
    } catch {
      // Local form application remains explicit; the marker is audit-only.
    }
    Message.success('草稿已填入待发布表单，请人工检查后再创建或保存');
  };

  useEffect(() => {
    if (!copilotImageFile) {
      setCopilotImagePreviewURL('');
      return undefined;
    }
    const objectURL = URL.createObjectURL(copilotImageFile);
    setCopilotImagePreviewURL(objectURL);
    return () => URL.revokeObjectURL(objectURL);
  }, [copilotImageFile]);

  const itemImagePreviewURL = useMemo(() => {
    if (!itemImageFile) return '';
    return URL.createObjectURL(itemImageFile);
  }, [itemImageFile]);

  useEffect(() => {
    if (!itemImagePreviewURL) return undefined;
    return () => URL.revokeObjectURL(itemImagePreviewURL);
  }, [itemImagePreviewURL]);

  const createAICommentary = async (eventType: string) => {
    if (!selectedAuction) return;
    const quickTemplate = eventType === 'product_evidence' || eventType === 'rule_guardrail' || eventType === 'extended';
    const manualSourceSeq = Date.now();
    setCommentaryLoadingType(eventType);
    try {
      await ensureHostSession();
      const response = await fetch(`/api/host/auctions/${selectedAuction.id}/commentary`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          room_id: selectedAuction.room_id,
          auction_id: selectedAuction.id,
          source_seq: manualSourceSeq,
          event_type: eventType,
          item_title: selectedAuction.item?.title ?? selectedAuction.id,
          current_price_cents: selectedAuction.current_price_cents,
          current_winner_masked: selectedAuction.current_winner_id ?? '',
          active_bidders_30s: heatSummary?.active_bidders_30s ?? 0,
          accepted_bids_30s: heatSummary?.accepted_bids_30s ?? 0
        })
      });
      const payload = await readJSON<{ message?: SystemMessage | string; job?: { status?: string; provider?: string; model?: string; error_message?: string; safety_json?: Record<string, unknown> }; code?: string }>(response);
      if (!response.ok) {
        Message.error(businessErrorMessage(payload.code, typeof payload.message === 'string' ? payload.message : quickTemplate ? '快捷提示发送失败' : 'AI 解说生成失败'));
        return;
      }
      if (payload.message && typeof payload.message !== 'string') setSystemMessages((current) => [payload.message as SystemMessage, ...current.filter((row) => row.id !== (payload.message as SystemMessage).id)].slice(0, 10));
      if (payload.job?.safety_json?.quick_template === true || payload.job?.provider === 'host_quick_template') {
        Message.success('快捷提示已发送到买家端');
        return;
      }
      if (payload.job?.status === 'FAILED' || payload.job?.provider === 'deterministic' || payload.job?.model === 'fallback-template' || payload.job?.safety_json?.fallback === true) {
        Message.warning(payload.job?.error_message ? `AI 调用失败，已使用兜底话术：${payload.job.error_message}` : 'AI 调用失败，已使用兜底话术');
        return;
      }
      Message.success('AI 解说已发送到买家端');
    } catch {
      Message.error(quickTemplate ? '快捷提示发送失败' : 'AI 解说生成失败');
    } finally {
      setCommentaryLoadingType('');
    }
  };

  const evaluateSentinel = async () => {
    if (!selectedAuction) return;
    try {
      await ensureHostSession();
      const response = await fetch(`/api/host/auctions/${selectedAuction.id}/sentinel-evaluate`, { method: 'POST' });
      const payload = await readJSON<{ items?: SentinelAlert[] }>(response);
      if (!response.ok) {
        Message.error('风控扫描失败');
        return;
      }
      setSentinelAlerts(payload.items ?? []);
      Message.success((payload.items ?? []).length ? '已生成风控提醒' : '未发现需提醒的异常模式');
    } catch {
      Message.error('风控扫描失败');
    }
  };

  const buildRecap = async () => {
    if (!selectedAuction) return;
    try {
      await ensureHostSession();
      const response = await fetch(`/api/host/auctions/${selectedAuction.id}/recap`, { method: 'POST' });
      const payload = await readJSON<{ recap?: AuctionRecap; highlight_asset?: HighlightAsset }>(response);
      if (!response.ok || !payload.recap) {
        Message.error('复盘生成失败');
        return;
      }
      setLatestRecap({ ...payload.recap, highlight_asset: payload.highlight_asset });
      Message.success(payload.highlight_asset ? '复盘与成交凭证已生成' : '竞拍复盘已生成');
    } catch {
      Message.error('复盘生成失败');
    }
  };

  const toggleAutoCommentaryEnabled = async () => {
    if (!selectedAuction) return;
    const current = auctionAISettings?.auto_commentary_enabled ?? true;
    const next = !current;
    try {
      await ensureHostSession();
      const response = await fetch(`/api/host/auctions/${selectedAuction.id}/ai-settings`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ auto_commentary_enabled: next })
      });
      const payload = await readJSON<AuctionAISettings>(response);
      if (!response.ok) {
        Message.error('自动解说开关保存失败');
        return;
      }
      setAuctionAISettings(payload);
      Message.success(next ? '已开启本场自动解说' : '已关闭本场自动解说');
    } catch {
      Message.error('自动解说开关保存失败');
    }
  };

  return (
    <Layout className={`console-shell${siderCollapsed ? ' nav-collapsed' : ''}`}>
      <Layout.Sider className="sider" width={siderCollapsed ? 72 : 224}>
        <ConsoleNav
          activeTab={workspaceTab}
          collapsed={siderCollapsed}
          onSelect={setWorkspaceTab}
          onToggle={() => setSiderCollapsed((current) => !current)}
        />
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
        <div className={`workbench-status status-${visibleTask.tone}`} role="status" aria-live="polite" data-testid="pc-workbench-status">
          <div>
            <strong>{visibleTask.title}</strong>
            <span>{visibleTask.detail}</span>
          </div>
          {workbenchTask.tone === 'error' || resetTask.tone === 'error' ? (
            <Button size="small" onClick={() => void loadAll()}>重试刷新</Button>
          ) : null}
        </div>

        {workspaceTab === 'inventory' && (
          <section className="workspace-page inventory-page" data-testid="pc-inventory-page">
            <div className="section-title">
              <div>
                <h1>拍品与规则</h1>
                <p>四组发布表单管理拍品信息、竞价规则、场次时间和履约保障；开拍后规则冻结。</p>
              </div>
              <span>{selectedAuction ? `当前选中「${auctionDisplayName(selectedAuction)}」` : '未选中竞拍'}</span>
            </div>
            <InventoryLotsPanel
              auctions={auctions}
              selectedAuction={selectedAuction}
              onSelect={setSelectedAuctionID}
            />
            <div className="two-column inventory-workspace">
              <ItemCreatePanel
                creating={creating}
                imageFile={itemImageFile}
                imagePreviewURL={itemImagePreviewURL}
                itemDraft={itemDraft}
                listingDraft={listingDraftJob}
                listingDraftLoading={listingDraftLoading}
                ruleValid={ruleValidation.valid}
                onApplyListingDraft={applyListingDraft}
                onCreate={createItemAndAuction}
                onFileChange={setItemImageFile}
                onDraftChange={setItemDraft}
                onOpenListingCopilot={() => setListingCopilotOpen(true)}
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
                <h1>开播中控</h1>
                <p>主屏只放开播操作；订单、数据和诊断进入独立入口。</p>
              </div>
              <div className="section-actions">
                <span>{pinnedActiveAuction ? `${auctionStatusLabel(pinnedActiveAuction.status)}「${auctionDisplayName(pinnedActiveAuction)}」` : '当前无开拍中拍品'}</span>
                <Button status="danger" loading={resettingSeed} disabled={!sessionReady || loading} onClick={resetDemoSeed}>重置演示环境</Button>
              </div>
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
                  heatSummary={heatSummary}
                  liveAuction={pinnedActiveAuction}
                  monitor={monitor}
                  now={now}
                  orders={orders}
                  recentEvents={recentEvents}
                  selectedAuction={selectedAuction}
                >
                  <AuctionCommandPanel
                    cancelReason={cancelReason}
                    actionPending={auctionActionPending}
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
                latestRecap={latestRecap}
                liveOpsDraft={liveOpsDraft}
                liveOpsSaving={liveOpsSaving}
                liveOpsSummary={liveOpsSummary}
                monitor={monitor}
                onBuildRecap={buildRecap}
                onCreateCommentary={createAICommentary}
                onEvaluateSentinel={evaluateSentinel}
                onLiveOpsDraftChange={(patch) => setLiveOpsDraft((current) => current ? { ...current, ...patch } : current)}
                onSaveLiveOpsReward={saveLiveOpsReward}
                autoCommentaryEnabled={auctionAISettings?.auto_commentary_enabled ?? true}
                commentaryLoadingType={commentaryLoadingType}
                onToggleAutoCommentaryEnabled={toggleAutoCommentaryEnabled}
                onOpenFlightRecorder={openFlightRecorder}
                prompts={hostPrompts}
                promptsLoading={promptsLoading}
                recentEvents={recentEvents}
                selectedAuction={selectedAuction}
                sentinelAlerts={sentinelAlerts}
                systemMessages={(auctionAISettings?.auto_commentary_enabled ?? true) ? systemMessages : systemMessages.filter((row) => row.safety_json?.auto_generated !== true)}
                onDismissPrompt={(promptID) => setDismissedPromptIDs((current) => Array.from(new Set([...current, promptID])))}
                onShortenCountdown={shortenDemoCountdown}
                onDriveDemoBid={driveDemoBid}
              />
            </div>
          </section>
        )}

        {workspaceTab === 'orders' && (
          <section className="workspace-page orders-page" data-testid="pc-orders-page">
            <div className="section-title">
              <div>
                <h1>订单记录</h1>
                <p>成交后自动生成，金额右对齐、买家脱敏，技术编号只在详情和事件回放中追溯。</p>
              </div>
              <span>{selectedAuction ? `${auctionDisplayName(selectedAuction)} · ${orders.length || 0} 条` : '暂无订单'}</span>
            </div>
            <CurrentAuctionOrderCard
              auction={selectedAuction}
              orders={orders}
              onOpenFlightRecorder={openFlightRecorder}
              onOpenOrder={setOrderDetailID}
            />
            <OrdersPanel orders={orders} onOpenFlightRecorder={openFlightRecorder} onOpenOrder={setOrderDetailID} />
          </section>
        )}

        {workspaceTab === 'diagnostics' && (
          <section className="workspace-page diagnostics-page" data-testid="pc-diagnostics-page">
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
          auction={selectedAuction}
          order={orders.find((order) => order.id === orderDetailID)}
          visible={Boolean(orderDetailID)}
          onClose={() => setOrderDetailID('')}
          onOpenFlightRecorder={openFlightRecorder}
        />
        <AICopilotDrawer
          category={listingCategory}
          imageFile={copilotImageFile}
          imagePreviewURL={copilotImagePreviewURL}
          imageURL={copilotImageURL || itemDraft.imageURL}
          draft={listingDraftJob}
          loading={listingDraftLoading}
          notes={listingNotes}
          selectedTitle={selectedListingDraftTitle}
          visible={listingCopilotOpen}
          onApply={applyListingDraft}
          onCategoryChange={setListingCategory}
          onClose={() => setListingCopilotOpen(false)}
          onGenerate={generateListingDraft}
          onImageFileChange={(file) => {
            setCopilotImageFile(file);
            if (file) setListingDraftJob(undefined);
          }}
          onImageURLChange={setCopilotImageURL}
          onNotesChange={setListingNotes}
          onSelectedTitleChange={setSelectedListingDraftTitle}
        />
      </Layout.Content>
    </Layout>
  );
}

function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ''));
    reader.onerror = () => reject(reader.error ?? new Error('read image file'));
    reader.readAsDataURL(file);
  });
}

createRoot(document.getElementById('root')!).render(<App />);
