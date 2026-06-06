import React, { useEffect, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Layout, Message } from '@arco-design/web-react';
import '@arco-design/web-react/dist/css/arco.css';

import { AICopilotDrawer, AuctionCommandPanel, AuctionControlSummary, AuctionQueue, ConsoleNav, DiagnosticsPanel, EventTimeline, FlightRecorderDrawer, HealthRibbon, InventoryLotsPanel, ItemCreatePanel, LiveAssistRail, LiveHealthPanel, OrderDetailDrawer, OrdersPanel, RuleEditor } from './components';
import type { Auction, AuctionAISettings, AuctionRecap, AuthUser, FlightRecorderPayload, HeatSummary, HighlightAsset, HostPrompt, HostPromptsPayload, Item, ListingDraftJob, MaxBidSummary, MonitorPayload, Order, RedisEngineMonitorPayload, Room, RuleAPIError, RuleDraft, SentinelAlert, SignalRequest, SystemMessage } from './domain';
import { activeAuction, auctionStatusLabel, createRuleDraft, defaultRoomID, depositPreview, ensureDemoSession, liveHealthSummary, monitorQuery, narratingAuction, readJSON, rulePayload, signalCopy, sortedAuctions, validateRule } from './domain';
import './styles.css';

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
  const [listingCopilotOpen, setListingCopilotOpen] = useState(false);
  const [listingNotes, setListingNotes] = useState('');
  const [listingCategory, setListingCategory] = useState('collectibles');
  const [listingDraftJob, setListingDraftJob] = useState<ListingDraftJob | undefined>();
  const [listingDraftLoading, setListingDraftLoading] = useState(false);
  const [copilotImageFile, setCopilotImageFile] = useState<File | null>(null);
  const [copilotImageURL, setCopilotImageURL] = useState('');
  const [systemMessages, setSystemMessages] = useState<SystemMessage[]>([]);
  const [sentinelAlerts, setSentinelAlerts] = useState<SentinelAlert[]>([]);
  const [latestRecap, setLatestRecap] = useState<AuctionRecap | undefined>();
  const [auctionAISettings, setAuctionAISettings] = useState<AuctionAISettings | undefined>();
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
      Message.error('事件回放读取失败');
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

  useEffect(() => {
    let cancelled = false;
    const loadSystemMessages = async () => {
      if (!sessionReady || !roomID) {
        setSystemMessages([]);
        return;
      }
      try {
        const response = await fetch(`/api/rooms/${encodeURIComponent(roomID)}/system-messages?limit=10`);
        const payload = await readJSON<{ items?: SystemMessage[] }>(response);
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
        const response = await fetch(`/api/host/auctions/${selectedAuction.id}/ai-settings`);
        const payload = await readJSON<AuctionAISettings>(response);
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

  const uploadItemImage = async (file: File) => {
    const safeName = file.name.replace(/[^a-zA-Z0-9._-]/g, '-');
    const objectName = `items/${Date.now()}-${safeName}`;
    const upload = await fetch('/api/items/upload-url', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ object_name: objectName, content_type: file.type || 'application/octet-stream' })
    });
    if (!upload.ok) throw new Error('create upload url failed');
    const payload = await upload.json() as { upload_url?: string; public_url?: string };
    if (!payload.upload_url) throw new Error('missing upload url');
    const put = await fetch(payload.upload_url, {
      method: 'PUT',
      headers: { 'Content-Type': file.type || 'application/octet-stream' },
      body: file
    });
    if (!put.ok) throw new Error('upload failed');
    return payload.public_url ?? '';
  };

  const createItemAndAuction = async () => {
    setCreating(true);
    try {
      let imageURL = itemDraft.imageURL.trim();
      if (itemImageFile) {
        imageURL = await uploadItemImage(itemImageFile) || imageURL;
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
      Message.warning('请选择开拍中的拍品后再演示');
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

  const generateListingDraft = async () => {
    setListingDraftLoading(true);
    try {
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
      const response = await fetch('/api/host/ai/listing-drafts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          room_id: roomID,
          image_urls: providerImageURLs,
          image_data_urls: providerImageDataURLs,
          seller_notes: listingNotes.trim(),
          target_category: listingCategory.trim()
        })
      });
      const payload = await readJSON<ListingDraftJob & { message?: string }>(response);
      if (!response.ok) {
        Message.error(payload.message ?? '智能草稿生成失败');
        return;
      }
      setListingDraftJob(payload);
      if (imageURL && providerImageURLs.length === 0 && providerImageDataURLs.length === 0) {
        Message.warning('图片已保存到拍品表单；智能草稿未读取大图，请在备注里补充关键信息');
      }
      Message.success('智能草稿已生成，需人工确认后应用');
    } catch {
      Message.error('智能草稿生成失败');
    } finally {
      setListingDraftLoading(false);
    }
  };

  const applyListingDraft = async () => {
    const output = listingDraftJob?.output_json;
    if (!listingDraftJob || !output) return;
    const firstTitle = output.title_candidates?.[0];
    setItemDraft((current) => ({
      ...current,
      title: firstTitle || current.title,
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
      await fetch(`/api/host/ai/listing-drafts/${listingDraftJob.id}/apply`, { method: 'POST' });
    } catch {
      // Local form application remains explicit; the marker is audit-only.
    }
    Message.success('草稿已应用到表单，请人工检查后创建或保存');
  };

  const createAICommentary = async (eventType: string) => {
    if (!selectedAuction) return;
    try {
      const response = await fetch(`/api/host/auctions/${selectedAuction.id}/commentary`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          room_id: selectedAuction.room_id,
          auction_id: selectedAuction.id,
          source_seq: Math.max(1, selectedAuction.seq),
          event_type: eventType,
          item_title: selectedAuction.item?.title ?? selectedAuction.id,
          current_price_cents: selectedAuction.current_price_cents,
          current_winner_masked: selectedAuction.current_winner_id ?? '',
          active_bidders_30s: heatSummary?.active_bidders_30s ?? 0,
          accepted_bids_30s: heatSummary?.accepted_bids_30s ?? 0
        })
      });
      const payload = await readJSON<{ message?: SystemMessage; code?: string }>(response);
      if (!response.ok) {
        Message.error(payload.code ?? '智能解说生成失败');
        return;
      }
      if (payload.message) setSystemMessages((current) => [payload.message!, ...current.filter((row) => row.id !== payload.message!.id)].slice(0, 10));
      Message.success('系统解说已生成');
    } catch {
      Message.error('智能解说生成失败');
    }
  };

  const evaluateSentinel = async () => {
    if (!selectedAuction) return;
    try {
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
      const response = await fetch(`/api/host/auctions/${selectedAuction.id}/recap`, { method: 'POST' });
      const payload = await readJSON<{ recap?: AuctionRecap; highlight_asset?: HighlightAsset }>(response);
      if (!response.ok || !payload.recap) {
        Message.error('复盘生成失败');
        return;
      }
      setLatestRecap({ ...payload.recap, highlight_asset: payload.highlight_asset });
      Message.success(payload.highlight_asset ? '复盘与服务端高光已生成' : '竞拍复盘已生成');
    } catch {
      Message.error('复盘生成失败');
    }
  };

  const toggleAutoCommentaryEnabled = async () => {
    if (!selectedAuction) return;
    const current = auctionAISettings?.auto_commentary_enabled ?? true;
    const next = !current;
    try {
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
                <h1>竞拍控场</h1>
                <p>选择队列中的拍品，执行排期、开拍、取消、讲解和实时氛围演示。</p>
              </div>
              <span>{pinnedActiveAuction ? `${auctionStatusLabel(pinnedActiveAuction.status)} ${pinnedActiveAuction.id}` : '当前无开拍中拍品'}</span>
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
                latestRecap={latestRecap}
                maxBidLoading={maxBidLoading}
                maxBidSummary={maxBidSummary}
                monitor={monitor}
                onBuildRecap={buildRecap}
                onCreateCommentary={createAICommentary}
                onEvaluateSentinel={evaluateSentinel}
                autoCommentaryEnabled={auctionAISettings?.auto_commentary_enabled ?? true}
                onToggleAutoCommentaryEnabled={toggleAutoCommentaryEnabled}
                onOpenFlightRecorder={openFlightRecorder}
                prompts={hostPrompts}
                promptsLoading={promptsLoading}
                recentEvents={recentEvents}
                selectedAuction={selectedAuction}
                sentinelAlerts={sentinelAlerts}
                systemMessages={(auctionAISettings?.auto_commentary_enabled ?? true) ? systemMessages : systemMessages.filter((row) => row.safety_json?.auto_generated !== true)}
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
        <AICopilotDrawer
          category={listingCategory}
          imageFile={copilotImageFile}
          imageURL={copilotImageURL || itemDraft.imageURL}
          draft={listingDraftJob}
          loading={listingDraftLoading}
          notes={listingNotes}
          visible={listingCopilotOpen}
          onApply={applyListingDraft}
          onCategoryChange={setListingCategory}
          onClose={() => setListingCopilotOpen(false)}
          onGenerate={generateListingDraft}
          onImageFileChange={setCopilotImageFile}
          onImageURLChange={setCopilotImageURL}
          onNotesChange={setListingNotes}
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
