import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { AlertTriangle, CheckCircle2, ChevronUp, CreditCard, History, MessageCircle, Radio, RefreshCw, Send, Wifi, WifiOff } from 'lucide-react';
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
type PaymentPhase = 'idle' | 'pending' | 'paid' | 'failed' | 'expired';
type RecoveryPhase = 'idle' | 'recovering';
type ConnectionPhase = 'connecting' | 'connected' | 'recovering' | 'disconnected';

type BidResponse = {
  result?: string;
  auction_id?: string;
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
    status?: string;
    current_price_cents?: number;
    amount_cents?: number;
    order_id?: string;
    leader_user_masked?: string;
    current_winner_id?: string;
    user_id?: string;
    reason?: string;
    order_status?: string;
    deposit_status?: string;
  };
};

type SnapshotResponse = {
  auction_id: string;
  id?: string;
  status?: string;
  current_price_cents?: number;
  current_winner_id?: string;
  increment_cents?: number;
  event_type?: string;
  seq: number;
  source?: string;
  stale?: boolean;
  payload?: {
    status?: string;
    current_price_cents?: number;
    current_winner_id?: string;
    leader_user_masked?: string;
    reason?: string;
  };
};

type HistoryRow = Record<string, unknown>;
type ChatMessage = {
  id: number;
  room_id: string;
  user_id: string;
  body: string;
  created_at: string;
};
type OrderRow = {
  order_id?: string;
  auction_id?: string;
  amount_cents?: number;
  order_status?: string;
};
type AuctionSummary = {
  id: string;
  room_id?: string;
  status?: string;
  current_price_cents?: number;
  current_winner_id?: string;
  increment_cents?: number;
  seq?: number;
  item?: {
    title?: string;
  };
};
type WSTicketResponse = {
  ticket?: string;
  expires_in_ms?: number;
};

const roomID = 'room_main';
const currentUserID = 'user_1';
const mockUserHeaders = {
  'X-Mock-Role': 'user',
  'X-Mock-User-Id': currentUserID
};

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

async function readJSON<T>(response: Response): Promise<T | null> {
  try {
    return await response.json() as T;
  } catch {
    return null;
  }
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

function isTestMatrixEnabled() {
  return new URLSearchParams(window.location.search).get('stateMatrix') === '1';
}

function App() {
  const showStateMatrix = useMemo(isTestMatrixEnabled, []);
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
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([]);
  const [chatDraft, setChatDraft] = useState('');
  const [chatSending, setChatSending] = useState(false);
  const [connectionPhase, setConnectionPhase] = useState<ConnectionPhase>('connecting');
  const [activeAuctionID, setActiveAuctionID] = useState('');
  const [activeIncrementCents, setActiveIncrementCents] = useState(5_000);
  const [payableOrderID, setPayableOrderID] = useState('');
  const [payableOrderAmountCents, setPayableOrderAmountCents] = useState(0);
  const [terminalPriceCents, setTerminalPriceCents] = useState(0);
  const [terminalWinnerID, setTerminalWinnerID] = useState('');
  const [lotTitle, setLotTitle] = useState('青瓷手作茶盏');
  const paymentInFlight = useRef(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const lastSeqRef = useRef(lastSeq);
  const currentPriceRef = useRef(currentPriceCents);
  const leaderMaskedRef = useRef(leaderMasked);
  const activeAuctionIDRef = useRef(activeAuctionID);
  const activeIncrementCentsRef = useRef(activeIncrementCents);

  useEffect(() => {
    lastSeqRef.current = lastSeq;
  }, [lastSeq]);

  useEffect(() => {
    currentPriceRef.current = currentPriceCents;
  }, [currentPriceCents]);

  useEffect(() => {
    leaderMaskedRef.current = leaderMasked;
  }, [leaderMasked]);

  useEffect(() => {
    activeAuctionIDRef.current = activeAuctionID;
  }, [activeAuctionID]);

  useEffect(() => {
    activeIncrementCentsRef.current = activeIncrementCents;
  }, [activeIncrementCents]);

  const scenario = useMemo<Scenario>(() => {
    if (selected === 'sold_winner') {
      const soldPrice = formatCents(payableOrderAmountCents || terminalPriceCents || currentPriceCents);
      if (paymentPhase === 'pending') {
        return {
          key: 'sold_winner',
          title: '支付中',
          status: 'SOLD',
          price: soldPrice,
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
          price: soldPrice,
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
          price: soldPrice,
          leader: '你已拍中',
          feedback: '支付未确认，请重试',
          cta: '重新支付',
          ctaDisabled: false,
          winner: true,
          sold: true
        };
      }
      if (paymentPhase === 'expired') {
        return {
          key: 'sold_winner',
          title: '订单已超时',
          status: 'ORDER_EXPIRED',
          price: soldPrice,
          leader: '支付窗口关闭',
          feedback: '订单已超时',
          cta: '已超时',
          ctaDisabled: true,
          winner: true,
          sold: true
        };
      }
      return {
        key: 'sold_winner',
        title: '成交',
        status: 'SOLD',
        price: soldPrice,
        leader: '你已拍中',
        feedback: payableOrderID ? '订单待支付' : '正在同步订单',
        cta: payableOrderID ? '去支付' : '同步订单中',
        ctaDisabled: !payableOrderID,
        winner: true,
        sold: true
      };
    }
    if (selected === 'sold_loser') {
      return {
        key: 'sold_loser',
        title: '已成交',
        status: 'SOLD',
        price: formatCents(terminalPriceCents || currentPriceCents),
        leader: terminalWinnerID ? `${terminalWinnerID.slice(0, 2)}** 拍中` : '已拍出',
        feedback: '本场已结束',
        cta: '已结束',
        ctaDisabled: true,
        sold: true
      };
    }
    if (selected !== 'active_bids') {
      return scenarios.find((item) => item.key === selected) ?? scenarios[0];
    }
    if (!activeAuctionID) {
      return {
        key: 'recovering',
        title: '同步中',
        status: 'RECOVERING',
        price: formatCents(currentPriceCents),
        leader: '同步中',
        feedback: '正在读取当前拍卖',
        cta: '同步中',
        ctaDisabled: true,
        stale: true
      };
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
  }, [activeAuctionID, bidFeedback, bidPhase, confirmAmountCents, connectionPhase, currentPriceCents, lastSeq, leaderMasked, nextBidCents, payableOrderAmountCents, payableOrderID, paymentPhase, recoveryPhase, selected, terminalPriceCents, terminalWinnerID]);

  const applyAcceptedBid = (payload: BidResponse) => {
    const acceptedPrice = payload.current_price_cents ?? currentPriceCents;
    setCurrentPriceCents(acceptedPrice);
    setNextBidCents(acceptedPrice + activeIncrementCents);
    setLastSeq(payload.seq ?? lastSeq);
    setConfirmToken('');
    setConfirmIdempotencyKey('');
    setConfirmAmountCents(0);
    if (payload.result === 'ACCEPTED_SOLD') {
      setTerminalPriceCents(acceptedPrice);
      setTerminalWinnerID(payload.current_winner_id ?? '');
      setSelected(payload.current_winner_id === currentUserID ? 'sold_winner' : 'sold_loser');
      setBidPhase('idle');
      void loadPayableOrderForAuction(payload.auction_id ?? activeAuctionIDRef.current);
      return;
    }
    setBidPhase(payload.current_winner_id === currentUserID ? 'accepted' : 'idle');
  };

  const loadPayableOrderForAuction = async (auctionID: string) => {
    if (!auctionID) return null;
    try {
      const response = await fetch('/api/users/me/orders', { headers: mockUserHeaders });
      const payload = await readJSON<{ items?: OrderRow[] }>(response);
      if (!response.ok) return null;
      const pendingOrder = (payload?.items ?? []).find((row) => (
        String(row.order_status ?? '') === 'ORDER_PENDING' && String(row.auction_id ?? '') === auctionID
      ));
      setPayableOrderID(pendingOrder?.order_id ? String(pendingOrder.order_id) : '');
      setPayableOrderAmountCents(Number(pendingOrder?.amount_cents ?? 0));
      return pendingOrder ?? null;
    } catch {
      setPayableOrderID('');
      setPayableOrderAmountCents(0);
      return null;
    }
  };

  const applySnapshot = (snapshot: SnapshotResponse) => {
    const price = snapshot.payload?.current_price_cents ?? snapshot.current_price_cents ?? currentPriceCents;
    const increment = snapshot.increment_cents ?? activeIncrementCents;
    const snapshotAuctionID = snapshot.auction_id ?? snapshot.id;
    if (snapshotAuctionID) setActiveAuctionID(snapshotAuctionID);
    if (snapshot.increment_cents != null) setActiveIncrementCents(increment);
    setCurrentPriceCents(price);
    setNextBidCents(price + increment);
    setLastSeq(snapshot.seq);
    setLeaderMasked(snapshot.payload?.leader_user_masked ?? leaderMasked);
    setBidFeedback(`snapshot ${snapshot.source ?? 'db'} seq ${snapshot.seq}`);
    setBidPhase('idle');
    const status = snapshot.payload?.status ?? snapshot.status;
    const winnerID = snapshot.payload?.current_winner_id ?? snapshot.current_winner_id;
    if (status === 'SOLD') {
      setTerminalPriceCents(price);
      setTerminalWinnerID(winnerID ?? '');
      setSelected(winnerID === currentUserID ? 'sold_winner' : 'sold_loser');
      if (winnerID === currentUserID) {
        void loadPayableOrderForAuction(snapshotAuctionID ?? activeAuctionIDRef.current);
      }
    } else if (status === 'ENDED') {
      setSelected('ended');
    } else if (status === 'CANCELLED') {
      setSelected('cancelled');
      setBidFeedback(snapshot.payload?.reason ?? '主播已取消');
    }
  };

  const recoverFromSnapshot = async () => {
    const auctionID = activeAuctionIDRef.current;
    if (!auctionID) return;
    setSelected('active_bids');
    setRecoveryPhase('recovering');
    setConnectionPhase((phase) => phase === 'disconnected' ? phase : 'recovering');
    try {
      const response = await fetch(`/api/auctions/${auctionID}`);
      const snapshot = await readJSON<SnapshotResponse>(response);
      if (!response.ok || !snapshot || snapshot.stale) {
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
    if (!detail || detail.auction_id !== activeAuctionIDRef.current) return;
    const currentSeq = lastSeqRef.current;
    if (detail.event_type === 'outbox_gap_notice' || (detail.seq != null && detail.seq > currentSeq + 1)) {
      void recoverFromSnapshot();
      return;
    }
    if (detail.seq == null || detail.seq <= currentSeq) return;
    const price = detail.payload?.current_price_cents ?? detail.payload?.amount_cents ?? currentPriceRef.current;
    const increment = activeIncrementCentsRef.current;
    setCurrentPriceCents(price);
    setNextBidCents(price + increment);
    setLastSeq(detail.seq);
    setLeaderMasked(detail.payload?.leader_user_masked ?? leaderMaskedRef.current);
    setBidFeedback(`event seq ${detail.seq}`);
    if (detail.event_type === 'auction_sold') {
      const winnerID = detail.payload?.current_winner_id ?? detail.payload?.user_id ?? '';
      setTerminalPriceCents(price);
      setTerminalWinnerID(winnerID);
      if (detail.payload?.order_id && winnerID === currentUserID) {
        setPayableOrderID(detail.payload.order_id);
        setPayableOrderAmountCents(price);
      }
      setSelected(winnerID === currentUserID ? 'sold_winner' : 'sold_loser');
      setBidPhase('idle');
      if (winnerID === currentUserID) {
        void loadPayableOrderForAuction(detail.auction_id);
      }
    } else if (detail.event_type === 'auction_ended') {
      setSelected('ended');
      setBidPhase('idle');
    } else if (detail.event_type === 'auction_cancelled') {
      setSelected('cancelled');
      setBidFeedback(detail.payload?.reason ?? '主播已取消');
      setBidPhase('idle');
    } else if (detail.event_type === 'order_paid') {
      if (detail.payload?.user_id === currentUserID) {
        setPaymentPhase('paid');
        setSelected('sold_winner');
        if (detail.payload?.order_id) setPayableOrderID(detail.payload.order_id);
      }
      setBidFeedback('订单状态已同步');
    } else if (detail.event_type === 'order_expired') {
      if (detail.payload?.user_id === currentUserID) {
        setPaymentPhase('expired');
        setSelected('sold_winner');
        if (detail.payload?.order_id) setPayableOrderID(detail.payload.order_id);
      }
      setBidFeedback('订单已超时');
    } else if (detail.payload?.user_id && detail.payload.user_id !== currentUserID) {
      setBidPhase('idle');
      setConfirmToken('');
      setConfirmIdempotencyKey('');
      setConfirmAmountCents(0);
    }
    setConnectionPhase('connected');
  };

  useEffect(() => {
    let cancelled = false;
    const loadActiveAuction = async () => {
      try {
        const response = await fetch(`/api/rooms/${roomID}/auctions`, { headers: mockUserHeaders });
        const payload = await readJSON<AuctionSummary[] | { items?: AuctionSummary[] }>(response);
        const auctions = Array.isArray(payload) ? payload : payload?.items ?? [];
        const selectedAuction = auctions.find((item) => item.status === 'ACTIVE') ?? auctions[0];
        if (!response.ok || !selectedAuction || cancelled) return;
        setActiveAuctionID(selectedAuction.id);
        setLotTitle(selectedAuction.item?.title ?? selectedAuction.id);
        const price = selectedAuction.current_price_cents ?? currentPriceRef.current;
        const increment = selectedAuction.increment_cents ?? activeIncrementCents;
        setActiveIncrementCents(increment);
        setCurrentPriceCents(price);
        setNextBidCents(price + increment);
        setLastSeq(selectedAuction.seq ?? lastSeqRef.current);
        setBidFeedback(`auction ${selectedAuction.id}`);
      } catch {
        setBidFeedback('auction list unavailable');
      }
    };
    void loadActiveAuction();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    const loadPayableOrder = async () => {
      if (!activeAuctionID) return;
      const order = await loadPayableOrderForAuction(activeAuctionID);
      if (cancelled || order) return;
    };
    void loadPayableOrder();
    return () => {
      cancelled = true;
    };
  }, [activeAuctionID]);

  useEffect(() => {
    let cancelled = false;
    const loadChat = async () => {
      try {
        const response = await fetch(`/api/rooms/${roomID}/chat?limit=30`, { headers: mockUserHeaders });
        const payload = await readJSON<{ items?: ChatMessage[] }>(response);
        if (!response.ok || cancelled) return;
        setChatMessages((payload?.items ?? []).slice().reverse());
      } catch {
        setChatMessages([]);
      }
    };
    void loadChat();
    const timer = window.setInterval(loadChat, 5_000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

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
      if (!activeAuctionID) return;
      setConnectionPhase((phase) => phase === 'recovering' ? phase : 'connecting');
      try {
        const ticketResponse = await fetch('/api/auth/ws-ticket', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'X-Mock-Role': 'user',
            'X-Mock-User-Id': currentUserID
          },
          body: JSON.stringify({ room_id: roomID, auction_id: activeAuctionID })
        });
        const ticketPayload = await ticketResponse.json() as WSTicketResponse;
        if (!ticketResponse.ok || !ticketPayload.ticket) {
          throw new Error('ws ticket unavailable');
        }
        if (cancelled) return;
        const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
        const wsURL = `${scheme}://${window.location.host}/ws?room_id=${encodeURIComponent(roomID)}&auction_id=${encodeURIComponent(activeAuctionID)}&last_seq=${lastSeqRef.current}`;
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

    if (activeAuctionID) {
      void connectWebSocket();
    }
    return () => {
      cancelled = true;
      clearReconnect();
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, [activeAuctionID]);

  const submitBid = async () => {
    const auctionID = activeAuctionIDRef.current;
    if (selected !== 'active_bids' || scenario.ctaDisabled || !auctionID) return;
    const clientBidID = createClientBidID();
    setBidPhase('pending');
    try {
      const response = await fetch(`/api/auctions/${auctionID}/bids`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': clientBidID,
          ...mockUserHeaders
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
    const auctionID = activeAuctionIDRef.current;
    if (!confirmToken || !confirmIdempotencyKey || scenario.ctaDisabled || !auctionID) return;
    setBidPhase('confirming');
    try {
      const response = await fetch(`/api/auctions/${auctionID}/bids/confirm`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': confirmIdempotencyKey,
          ...mockUserHeaders
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
    if (selected !== 'sold_winner' || scenario.ctaDisabled || paymentInFlight.current || !payableOrderID) return;
    const idempotencyKey = createClientBidID();
    paymentInFlight.current = true;
    setPaymentPhase('pending');
    try {
      const response = await fetch(`/api/orders/${payableOrderID}/pay-mock`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': idempotencyKey,
          ...mockUserHeaders
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

  const decreaseBidAmount = () => {
    setNextBidCents((amount) => Math.max(currentPriceCents + activeIncrementCents, amount - activeIncrementCents));
  };

  const increaseBidAmount = () => {
    setNextBidCents((amount) => amount + activeIncrementCents);
  };

  const loadHistory = async () => {
    setHistoryLoading(true);
    setHistoryError('');
    try {
      const [bids, orders] = await Promise.all([
        fetch('/api/users/me/bids', { headers: mockUserHeaders }).then((response) => response.json()),
        fetch('/api/users/me/orders', { headers: mockUserHeaders }).then((response) => response.json())
      ]);
      setBidHistory(Array.isArray(bids.items) ? bids.items : []);
      const orderRows = Array.isArray(orders.items) ? orders.items : [];
      setOrderHistory(orderRows);
      const currentAuctionID = activeAuctionIDRef.current;
      const pendingOrder = orderRows.find((row: HistoryRow) => (
        String(row.order_status ?? '') === 'ORDER_PENDING' && String(row.auction_id ?? '') === currentAuctionID
      ));
      if (pendingOrder?.order_id) {
        setPayableOrderID(String(pendingOrder.order_id));
        setPayableOrderAmountCents(Number(pendingOrder.amount_cents ?? 0));
      }
    } catch {
      setHistoryError('历史读取失败');
    } finally {
      setHistoryLoading(false);
    }
  };

  const sendChat = async () => {
    const body = chatDraft.trim();
    if (!body || chatSending) return;
    setChatSending(true);
    try {
      const response = await fetch(`/api/rooms/${roomID}/chat`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...mockUserHeaders
        },
        body: JSON.stringify({
          client_msg_id: createClientBidID(),
          body
        })
      });
      const payload = await readJSON<ChatMessage>(response);
      if (response.ok && payload) {
        setChatMessages((current) => [...current.slice(-29), payload]);
        setChatDraft('');
      }
    } finally {
      setChatSending(false);
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
          <h1>{lotTitle}</h1>
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
          <button type="button" aria-label="decrease" onClick={decreaseBidAmount}>-</button>
          <span>{scenario.sold ? 'ORDER' : formatCents(nextBidCents)}</span>
          <button type="button" aria-label="increase" onClick={increaseBidAmount}><ChevronUp size={18} /></button>
        </div>
        <button className="primary-cta" data-testid="bid-cta" disabled={scenario.ctaDisabled} onClick={handlePrimaryAction}>
          {scenario.winner ? <CreditCard size={18} /> : scenario.rejected ? <AlertTriangle size={18} /> : <CheckCircle2 size={18} />}
          {scenario.cta}
        </button>
      </section>

      {showStateMatrix && (
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
      )}

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

      <section className="chat-panel" data-testid="chat-panel">
        <div className="history-title">
          <h2><MessageCircle size={16} /> 弹幕</h2>
        </div>
        <div className="chat-list">
          {chatMessages.length === 0 ? <p>暂无弹幕</p> : chatMessages.map((message) => (
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
            onChange={(event) => setChatDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') void sendChat();
            }}
          />
          <button type="button" aria-label="send-chat" disabled={!chatDraft.trim() || chatSending} onClick={sendChat}>
            <Send size={15} />
          </button>
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
