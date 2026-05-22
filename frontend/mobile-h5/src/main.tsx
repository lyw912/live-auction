import React, { useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { AlertTriangle, CheckCircle2, ChevronUp, CreditCard, Radio, Wifi, WifiOff } from 'lucide-react';
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

function App() {
  const [selected, setSelected] = useState<AuctionState>('active_bids');
  const scenario = useMemo(() => scenarios.find((item) => item.key === selected) ?? scenarios[0], [selected]);

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
          {scenario.stale ? <WifiOff size={16} /> : <Wifi size={16} />}
          <span>{scenario.stale ? '状态可能已过期' : '状态来自服务端事件'}</span>
        </div>
        <div className="bid-stepper">
          <button type="button" aria-label="decrease">-</button>
          <span>{scenario.sold ? 'ORDER' : '¥450.00'}</span>
          <button type="button" aria-label="increase"><ChevronUp size={18} /></button>
        </div>
        <button className="primary-cta" data-testid="bid-cta" disabled={scenario.ctaDisabled}>
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
    </main>
  );
}

createRoot(document.getElementById('root')!).render(<App />);
