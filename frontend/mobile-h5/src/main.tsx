import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { AlertTriangle, CheckCircle2, ChevronUp, CreditCard, History, Radio, RefreshCw, Wifi, WifiOff } from 'lucide-react';
import './styles.css';

type AuctionState =
  | 'scheduled'
  | 'active_empty'
  | 'active_bids'
  | 'self_leading'
  | 'pending'
  | 'rejected'
  | 'extended'
  | 'recovering'
  | 'disconnected'
  | 'sold_winner'
  | 'sold_loser'
  | 'ended'
  | 'cancelled';

type Scenario = {
  key: AuctionState;
  title: string;
  status: string;
  price: string;
  leader: string;
  feedback: string;
  cta: string;
  ctaDisabled: boolean;
  stale?: boolean;
  pending?: boolean;
  rejected?: boolean;
  winner?: boolean;
  sold?: boolean;
};

type BidPhase = 'idle' | 'pending' | 'accepted' | 'rejected' | 'confirm_required' | 'confirming';
type PaymentPhase = 'idle' | 'pending' | 'paid' | 'failed';
type RecoveryPhase = 'idle' | 'recovering';
type ConnectionPhase = 'connecting' | 'connected' | 'recovering' | 'disconnected';

type BidResponse = {
  result?: string;
  seq?: number;
  current_price_cents?: number;
  current_winner_id?: string;
  reject_reason?: string | null;
  code?: string;
  confirm_token?: string;
  expires_in_ms?: number;
  amount_cents?: number;
};

type AuctionRealtimeEvent = {
  auction_id: string;
  event_type: string;
  seq?: number;
  payload?: {
    current_price_cents?: number;
    leader_user_masked?: string;
  };
};

type SnapshotResponse = {
  auction_id: string;
  event_type?: string;
  seq: number;
  source?: string;
  stale?: boolean;
  payload?: {
    current_price_cents?: number;
    leader_user_masked?: string;
  };
};

type HistoryRow = Record<string, unknown>;
type WSTicketResponse = {
  ticket?: string;
  expires_in_ms?: number;
};

const roomID = 'room_main';
const auctionID = 'auc_live';
const orderID = 'ord_pending';
const currentUserID = 'user_1';

const scenarios: Scenario[] = [
  { key: 'scheduled', title: '即将开拍', status: 'SCHEDULED', price: '¥100.00', leader: '暂无领先', feedback: '19:58 开始', cta: '等待开拍', ctaDisabled: true },
  { key: 'active_empty', title: '首拍', status: 'ACTIVE', price: '¥100.00', leader: '暂无领先', feedback: '最低 ¥150.00', cta: '出价 ¥150.00', ctaDisabled: false },
  { key: 'active_bids', title: '竞价中', status: 'ACTIVE', price: '¥350.00', leader: '张** 领先', feedback: '下一口 ¥400.00', cta: '出价 ¥400.00', ctaDisabled: false },
  { key: 'self_leading', title: '领先中', status: 'ACTIVE', price: '¥400.00', leader: '你已领先', feedback: '等待其他用户出价', cta: '已领先', ctaDisabled: true },
  { key: 'pending', title: '提交中', status: 'ACTIVE', price: '¥400.00', leader: '李** 领先', feedback: '等待服务端确认', cta: '确认中', ctaDisabled: true, pending: true },
  { key: 'rejected', title: '被拒绝', status: 'ACTIVE', price: '¥400.00', leader: '李** 领先', feedback: '请按加价幅度出价', cta: '出价 ¥450.00', ctaDisabled: false, rejected: true },
  { key: 'extended', title: '已延时', status: 'ACTIVE', price: '¥450.00', leader: '王** 领先', feedback: '已延时 10 秒', cta: '出价 ¥500.00', ctaDisabled: false },
  { key: 'recovering', title: '恢复中', status: 'RECOVERING', price: '¥450.00', leader: '同步中', feedback: '正在同步权威状态', cta: '同步中', ctaDisabled: true, stale: true },
  { key: 'disconnected', title: '已断开', status: 'DISCONNECTED', price: '¥450.00', leader: '离线', feedback: '重连中', cta: '重连中', ctaDisabled: true, stale: true },
  { key: 'sold_winner', title: '成交', status: 'SOLD', price: '¥600.00', leader: '你已拍中', feedback: '订单待支付', cta: '去支付', ctaDisabled: false, winner: true, sold: true },
  { key: 'sold_loser', title: '已成交', status: 'SOLD', price: '¥600.00', leader: '赵** 拍中', feedback: '本场已结束', cta: '已结束', ctaDisabled: true, sold: true },
  { key: 'ended', title: '流拍', status: 'ENDED', price: '¥100.00', leader: '无成交', feedback: '无人出价', cta: '已结束', ctaDisabled: true },
  { key: 'cancelled', title: '已取消', status: 'CANCELLED', price: '¥350.00', leader: '取消前价格', feedback: '主播已取消', cta: '已取消', ctaDisabled: true }
];

function formatCents(cents: number) {
  return `¥${(cents / 100).toFixed(2)}`;
}

function createClientBidID() {
  if (globalThis.crypto?.randomUUID) {
    return globalThis.crypto.randomUUID();
  }
  return `bid_${Date.now()}_${Math.random().toString(16).slice(2)}`;
}

function rejectCopy(code?: string | null) {
  switch (code) {
    case 'BID_TOO_LOW':
      return '出价低于最低可出价';
    case 'BID_INCREMENT_MISMATCH':
      return '请按加价幅度出价';
    case 'REJECTED_SELF_LEADING':
      return '你已领先，无需重复出价';
    case 'BID_AUCTION_TOO_HOT':
      return '竞价激烈，请稍候';
    case 'PROCESSING_RETRY_LATER':
      return '正在确认上一笔出价';
    case 'IDEMPOTENCY_TIMEOUT':
      return '操作未确认，请重新出价';
    case 'AUCTION_ENDED':
      return '竞拍已结束，正在同步结果';
    case 'FORBIDDEN_ROOM':
      return '无法进入该直播间';
    default:
      return '出价未通过，请重试';
  }
}

function App() {
  const [selected, setSelected] = useState<AuctionState>('active_bids');
  const [bidPhase, setBidPhase] = useState<BidPhase>('idle');
  const [paymentPhase, setPaymentPhase] = useState<PaymentPhase>('idle');
  const [recoveryPhase, setRecoveryPhase] = useState<RecoveryPhase>('idle');
  const [currentPriceCents, setCurrentPriceCents] = useState(35_000);
  const [nextBidCents, setNextBidCents] = useState(40_000);
  const [lastSeq, setLastSeq] = useState(41);
  const [bidFeedback, setBidFeedback] = useState('下一口 ¥400.00');
  const [leaderMasked, setLeaderMasked] = useState('张**');
  const [confirmToken, setConfirmToken] = useState('');
  const [confirmIdempotencyKey, setConfirmIdempotencyKey] = useState('');
  const [confirmAmountCents, setConfirmAmountCents] = useState(0);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState('');
  const [bidHistory, setBidHistory] = useState<HistoryRow[]>([]);
  const [orderHistory, setOrderHistory] = useState<HistoryRow[]>([]);
  const [connectionPhase, setConnectionPhase] = useState<ConnectionPhase>('connecting');
  const paymentInFlight = useRef(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const lastSeqRef = useRef(lastSeq);
  const currentPriceRef = useRef(currentPriceCents);
  const leaderMaskedRef = useRef(leaderMasked);

  useEffect(() => {
    lastSeqRef.current = lastSeq;
  }, [lastSeq]);

  useEffect(() => {
    currentPriceRef.current = currentPriceCents;
  }, [currentPriceCents]);

  useEffect(() => {
    leaderMaskedRef.current = leaderMasked;
  }, [leaderMasked]);

  const scenario = useMemo<Scenario>(() => {
    if (selected === 'sold_winner') {
      if (paymentPhase === 'pending') {
        return {
          key: 'sold_winner',
          title: '支付中',
          status: 'SOLD',
          price: '¥600.00',
          leader: '你已拍中',
          feedback: '等待服务端确认支付',
          cta: '支付中',
          ctaDisabled: true,
          winner: true,
          sold: true
        };
      }
      if (paymentPhase === 'paid') {
        return {
          key: 'sold_winner',
          title: '已支付',
          status: 'PAID',
          price: '¥600.00',
          leader: '订单已支付',
          feedback: '保证金已处理',
          cta: '已支付',
          ctaDisabled: true,
          winner: true,
          sold: true
        };
      }
      if (paymentPhase === 'failed') {
        return {
          key: 'sold_winner',
          title: '支付失败',
          status: 'SOLD',
          price: '¥600.00',
          leader: '你已拍中',
          feedback: '支付未确认，请重试',
          cta: '重新支付',
          ctaDisabled: false,
          winner: true,
          sold: true
        };
      }
    }
    if (selected !== 'active_bids') {
      return scenarios.find((item) => item.key === selected) ?? scenarios[0];
    }
    if (connectionPhase === 'disconnected') {
      return {
        key: 'disconnected',
        title: '已断开',
        status: 'DISCONNECTED',
        price: formatCents(currentPriceCents),
        leader: '重连中',
        feedback: '正在重新连接',
        cta: '重连中',
        ctaDisabled: true,
        stale: true
      };
    }
    if (recoveryPhase === 'recovering') {
      return {
        key: 'recovering',
        title: '恢复中',
        status: 'RECOVERING',
        price: formatCents(currentPriceCents),
        leader: '同步中',
        feedback: '正在同步权威状态',
        cta: '同步中',
        ctaDisabled: true,
        stale: true
      };
    }
    if (bidPhase === 'pending') {
      return {
        key: 'pending' as AuctionState,
        title: '提交中',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: '等待服务端确认',
        cta: '确认中',
        ctaDisabled: true,
        pending: true
      };
    }
    if (bidPhase === 'confirming') {
      return {
        key: 'pending' as AuctionState,
        title: '确认中',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: '等待服务端确认高额出价',
        cta: '确认中',
        ctaDisabled: true,
        pending: true
      };
    }
    if (bidPhase === 'confirm_required') {
      return {
        key: 'active_bids' as AuctionState,
        title: '高额确认',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: `确认 ${formatCents(confirmAmountCents)} 出价`,
        cta: '确认高额出价',
        ctaDisabled: false
      };
    }
    if (bidPhase === 'accepted') {
      return {
        key: 'self_leading' as AuctionState,
        title: '领先中',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: '你已领先',
        feedback: `服务端确认 seq ${lastSeq}`,
        cta: '已领先',
        ctaDisabled: true
      };
    }
    if (bidPhase === 'rejected') {
      return {
        key: 'rejected' as AuctionState,
        title: '被拒绝',
        status: 'ACTIVE',
        price: formatCents(currentPriceCents),
        leader: `${leaderMasked} 领先`,
        feedback: bidFeedback,
        cta: `出价 ${formatCents(nextBidCents)}`,
        ctaDisabled: false,
        rejected: true
      };
    }
    return {
      key: 'active_bids' as AuctionState,
      title: '竞价中',
      status: 'ACTIVE',
      price: formatCents(currentPriceCents),
      leader: `${leaderMasked} 领先`,
      feedback: bidFeedback,
      cta: `出价 ${formatCents(nextBidCents)}`,
      ctaDisabled: false
    };
  }, [bidFeedback, bidPhase, confirmAmountCents, connectionPhase, currentPriceCents, lastSeq, leaderMasked, nextBidCents, paymentPhase, recoveryPhase, selected]);

  const applyAcceptedBid = (payload: BidResponse) => {
    const acceptedPrice = payload.current_price_cents ?? currentPriceCents;
    setCurrentPriceCents(acceptedPrice);
    setNextBidCents(acceptedPrice + 5_000);
    setLastSeq(payload.seq ?? lastSeq);
    setConfirmToken('');
    setConfirmIdempotencyKey('');
    setConfirmAmountCents(0);
    setBidPhase(payload.current_winner_id === currentUserID ? 'accepted' : 'idle');
  };

  const applySnapshot = (snapshot: SnapshotResponse) => {
    const price = snapshot.payload?.current_price_cents ?? currentPriceCents;
    setCurrentPriceCents(price);
    setNextBidCents(price + 5_000);
    setLastSeq(snapshot.seq);
    setLeaderMasked(snapshot.payload?.leader_user_masked ?? leaderMasked);
    setBidFeedback(`snapshot ${snapshot.source ?? 'db'} seq ${snapshot.seq}`);
    setBidPhase('idle');
  };

  const recoverFromSnapshot = async () => {
    setSelected('active_bids');
    setRecoveryPhase('recovering');
    setConnectionPhase((phase) => phase === 'disconnected' ? phase : 'recovering');
    try {
      const response = await fetch(`/api/auctions/${auctionID}`);
      const snapshot = await response.json() as SnapshotResponse;
      if (!response.ok || snapshot.stale) {
        setBidFeedback('snapshot stale，继续同步');
        return;
      }
      applySnapshot(snapshot);
      setRecoveryPhase('idle');
      setConnectionPhase('connected');
    } catch {
      setBidFeedback('snapshot unavailable，继续同步');
    }
  };

  const handleRealtimeEvent = (detail: AuctionRealtimeEvent) => {
    if (!detail || detail.auction_id !== auctionID) return;
    const currentSeq = lastSeqRef.current;
    if (detail.event_type === 'outbox_gap_notice' || (detail.seq != null && detail.seq > currentSeq + 1)) {
      void recoverFromSnapshot();
      return;
    }
    if (detail.seq == null || detail.seq <= currentSeq) return;
    const price = detail.payload?.current_price_cents ?? currentPriceRef.current;
    setCurrentPriceCents(price);
    setNextBidCents(price + 5_000);
    setLastSeq(detail.seq);
    setLeaderMasked(detail.payload?.leader_user_masked ?? leaderMaskedRef.current);
    setBidFeedback(`event seq ${detail.seq}`);
    setConnectionPhase('connected');
  };

  useEffect(() => {
    const onAuctionEvent = (event: Event) => {
      const detail = (event as CustomEvent<AuctionRealtimeEvent>).detail;
      handleRealtimeEvent(detail);
    };
    window.addEventListener('auction:event', onAuctionEvent);
    return () => window.removeEventListener('auction:event', onAuctionEvent);
  }, []);

  useEffect(() => {
    let cancelled = false;
    const clearReconnect = () => {
      if (reconnectTimerRef.current != null) {
        window.clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
    };
    const scheduleReconnect = () => {
      clearReconnect();
      reconnectTimerRef.current = window.setTimeout(() => {
        if (!cancelled) void connectWebSocket();
      }, 2_000);
    };
    const connectWebSocket = async () => {
      setConnectionPhase((phase) => phase === 'recovering' ? phase : 'connecting');
      try {
        const ticketResponse = await fetch('/api/auth/ws-ticket', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'X-Mock-Role': 'user',
            'X-Mock-User-Id': currentUserID
          },
          body: JSON.stringify({ room_id: roomID, auction_id: auctionID })
        });
        const ticketPayload = await ticketResponse.json() as WSTicketResponse;
        if (!ticketResponse.ok || !ticketPayload.ticket) {
          throw new Error('ws ticket unavailable');
        }
        if (cancelled) return;
        const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
        const wsURL = `${scheme}://${window.location.host}/ws?room_id=${encodeURIComponent(roomID)}&auction_id=${encodeURIComponent(auctionID)}&last_seq=${lastSeqRef.current}`;
        const socket = new WebSocket(wsURL, ['auction.v1', `ticket.${ticketPayload.ticket}`]);
        wsRef.current = socket;
        socket.onopen = () => {
          if (!cancelled) setConnectionPhase('connected');
        };
        socket.onmessage = (message) => {
          try {
            handleRealtimeEvent(JSON.parse(String(message.data)) as AuctionRealtimeEvent);
          } catch {
            setBidFeedback('实时消息解析失败，正在同步');
            void recoverFromSnapshot();
          }
        };
        socket.onerror = () => {
          if (!cancelled) {
            setConnectionPhase('disconnected');
          }
        };
        socket.onclose = () => {
          if (!cancelled) {
            setConnectionPhase('disconnected');
            scheduleReconnect();
          }
        };
      } catch {
        if (!cancelled) {
          setConnectionPhase('disconnected');
          scheduleReconnect();
        }
      }
    };

    void connectWebSocket();
    return () => {
      cancelled = true;
      clearReconnect();
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, []);

  const submitBid = async () => {
    if (selected !== 'active_bids' || scenario.ctaDisabled) return;
    const clientBidID = createClientBidID();
    setBidPhase('pending');
    try {
      const response = await fetch(`/api/auctions/${auctionID}/bids`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': clientBidID
        },
        body: JSON.stringify({
          client_bid_id: clientBidID,
          amount_cents: nextBidCents,
          client_seen_seq: lastSeq
        })
      });
      const payload = await response.json() as BidResponse;
      if (payload.result === 'FAT_FINGER_CONFIRM_REQUIRED' && payload.confirm_token) {
        setConfirmToken(payload.confirm_token);
        setConfirmIdempotencyKey(clientBidID);
        setConfirmAmountCents(payload.amount_cents ?? nextBidCents);
        setBidFeedback('高额出价需要二次确认');
        setBidPhase('confirm_required');
        return;
      }
      if (!response.ok || payload.reject_reason || payload.code) {
        setBidFeedback(rejectCopy(payload.reject_reason ?? payload.code));
        setBidPhase('rejected');
        return;
      }
      applyAcceptedBid(payload);
    } catch {
      setBidFeedback('网络异常，请重试');
      setBidPhase('rejected');
    }
  };

  const confirmBid = async () => {
    if (!confirmToken || !confirmIdempotencyKey || scenario.ctaDisabled) return;
    setBidPhase('confirming');
    try {
      const response = await fetch(`/api/auctions/${auctionID}/bids/confirm`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': confirmIdempotencyKey
        },
        body: JSON.stringify({
          confirm_token: confirmToken,
          idempotency_key: confirmIdempotencyKey
        })
      });
      const payload = await response.json() as BidResponse;
      if (!response.ok || payload.reject_reason || payload.code) {
        setBidFeedback(rejectCopy(payload.reject_reason ?? payload.code));
        setBidPhase('rejected');
        return;
      }
      applyAcceptedBid(payload);
    } catch {
      setBidFeedback('网络异常，请重试');
      setBidPhase('rejected');
    }
  };

  const payOrder = async () => {
    if (selected !== 'sold_winner' || scenario.ctaDisabled || paymentInFlight.current) return;
    const idempotencyKey = createClientBidID();
    paymentInFlight.current = true;
    setPaymentPhase('pending');
    try {
      const response = await fetch(`/api/orders/${orderID}/pay-mock`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': idempotencyKey
        },
        body: JSON.stringify({ confirm: true })
      });
      if (!response.ok) {
        setPaymentPhase('failed');
        return;
      }
      const payload = await response.json() as { order_status?: string };
      setPaymentPhase(payload.order_status === 'PAID' ? 'paid' : 'failed');
    } catch {
      setPaymentPhase('failed');
    } finally {
      paymentInFlight.current = false;
    }
  };

  const handlePrimaryAction = () => {
    if (selected === 'sold_winner') {
      void payOrder();
      return;
    }
    if (bidPhase === 'confirm_required') {
      void confirmBid();
      return;
    }
    void submitBid();
  };

  const loadHistory = async () => {
    setHistoryLoading(true);
    setHistoryError('');
    try {
      const [bids, orders] = await Promise.all([
        fetch('/api/users/me/bids').then((response) => response.json()),
        fetch('/api/users/me/orders').then((response) => response.json())
      ]);
      setBidHistory(Array.isArray(bids.items) ? bids.items : []);
      setOrderHistory(Array.isArray(orders.items) ? orders.items : []);
    } catch {
      setHistoryError('历史读取失败');
    } finally {
      setHistoryLoading(false);
    }
  };

  return (
    <main className="app-shell">
      <section className="video-stage" aria-label="live-stage">
        <div className="video-topbar">
          <span className="live-pill"><Radio size={14} /> LIVE</span>
          <span className="viewer-count">12,486 watching</span>
        </div>
        <div className="focus-copy">
          <h1>青瓷手作茶盏</h1>
          <p>Lot A-102 · 22:00 结束</p>
        </div>
      </section>

      <section className={`auction-panel ${scenario.stale ? 'is-stale' : ''}`} aria-label="auction-state">
        <div className="state-row">
          <div>
            <p className="eyebrow">{scenario.title}</p>
            <h2>{scenario.price}</h2>
          </div>
          <span className="status-chip" data-state={scenario.status}>{scenario.status}</span>
        </div>
        <div className="leader-row">
          <span>{scenario.leader}</span>
          <strong>{scenario.feedback}</strong>
        </div>
        <div className="signal-row">
          {scenario.stale || connectionPhase === 'disconnected' ? <WifiOff size={16} /> : <Wifi size={16} />}
          <span>{scenario.stale ? '状态可能已过期' : connectionPhase === 'connected' ? 'WebSocket 已连接 · 状态来自服务端事件' : 'WebSocket 连接中 · 状态来自服务端事件'}</span>
        </div>
        <div className="bid-stepper">
          <button type="button" aria-label="decrease">-</button>
          <span>{scenario.sold ? 'ORDER' : formatCents(nextBidCents)}</span>
          <button type="button" aria-label="increase"><ChevronUp size={18} /></button>
        </div>
        <button className="primary-cta" data-testid="bid-cta" disabled={scenario.ctaDisabled} onClick={handlePrimaryAction}>
          {scenario.winner ? <CreditCard size={18} /> : scenario.rejected ? <AlertTriangle size={18} /> : <CheckCircle2 size={18} />}
          {scenario.cta}
        </button>
      </section>

      <nav className="state-tabs" aria-label="state-matrix">
        {scenarios.map((item) => (
          <button
            key={item.key}
            className={item.key === selected ? 'active' : ''}
            type="button"
            onClick={() => setSelected(item.key)}
          >
            {item.title}
          </button>
        ))}
      </nav>

      <section className="history-panel" data-testid="history-panel">
        <div className="history-title">
          <h2><History size={16} /> 我的历史</h2>
          <button type="button" onClick={loadHistory} disabled={historyLoading}>
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
            getSecondary={(row) => `${formatCents(Number(row.amount_cents ?? 0))} · ${String(row.result ?? row.status ?? '-')}`}
          />
          <HistoryList
            title="订单"
            empty="暂无订单"
            rows={orderHistory}
            getPrimary={(row) => String(row.order_id ?? row.auction_id ?? '-')}
            getSecondary={(row) => `${formatCents(Number(row.amount_cents ?? 0))} · ${String(row.order_status ?? '-')}`}
          />
        </div>
      </section>
    </main>
  );
}

function HistoryList({
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

createRoot(document.getElementById('root')!).render(<App />);
