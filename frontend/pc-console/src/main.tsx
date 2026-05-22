import React, { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Button, Form, InputNumber, Layout, Message, Space, Table, Tabs, Tag } from '@arco-design/web-react';
import '@arco-design/web-react/dist/css/arco.css';
import { Activity, AlertTriangle, ClipboardList, Database, RadioTower, RefreshCw } from 'lucide-react';
import './styles.css';

type MonitorPayload = {
  items: Array<Record<string, unknown>>;
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

type RuleAPIError = {
  code?: string;
  message?: string;
  details?: {
    suggested_caps?: number[];
  };
};

const auctionRows = [
  { id: 'auc_live', item: '青瓷手作茶盏', status: 'ACTIVE', narrating: true, price: '¥450.00', winner: '王**', end: '22:00:10', bids: 18 },
  { id: 'auc_next', item: '紫砂壶', status: 'SCHEDULED', narrating: false, price: '¥800.00', winner: '-', end: '22:12:00', bids: 0 },
  { id: 'auc_done', item: '木作托盘', status: 'SOLD', narrating: false, price: '¥620.00', winner: '赵**', end: '21:48:33', bids: 11 }
];

function formatCents(cents: number) {
  return `¥${(cents / 100).toFixed(2)}`;
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
  if (rule.incrementCents <= 0) {
    return { valid: false, message: '加价幅度必须大于 0', suggestions: [] };
  }
  const minCap = rule.startPriceCents + rule.incrementCents;
  if (rule.capPriceCents < minCap) {
    return {
      valid: false,
      message: `封顶价至少为 ${formatCents(minCap)}`,
      suggestions: nearestLegalCaps(rule)
    };
  }
  if ((rule.capPriceCents - rule.startPriceCents) % rule.incrementCents !== 0) {
    return {
      valid: false,
      message: '封顶价必须落在起拍价 + N * 加价幅度',
      suggestions: nearestLegalCaps(rule)
    };
  }
  return { valid: true, message: `封顶价可达，预计 ${Math.floor((rule.capPriceCents - rule.startPriceCents) / rule.incrementCents)} 口到顶`, suggestions: [] };
}

function App() {
  const [monitor, setMonitor] = useState<Record<string, MonitorPayload>>({});
  const [loading, setLoading] = useState(false);
  const [savingRule, setSavingRule] = useState(false);
  const [ruleSaveState, setRuleSaveState] = useState<'idle' | 'saved' | 'error'>('idle');
  const [backendRuleError, setBackendRuleError] = useState('');
  const [backendSuggestions, setBackendSuggestions] = useState<number[]>([]);
  const [rule, setRule] = useState<RuleDraft>({
    startPriceCents: 10_000,
    incrementCents: 5_000,
    capPriceCents: 60_000,
    durationSeconds: 600,
    extendWindowSeconds: 10,
    extendBySeconds: 10,
    maxExtendCount: 3,
    fatFingerThresholdCents: 100_000,
    depositBPS: 1000,
    depositFloorCents: 5_000,
    depositCapCents: 50_000
  });
  const ruleValidation = validateRule(rule);
  const shownSuggestions = ruleValidation.valid ? backendSuggestions : ruleValidation.suggestions;

  const loadMonitor = async () => {
    setLoading(true);
    try {
      const [auctions, anomalies, outbox, scheduler] = await Promise.all([
        fetch('/api/monitor/auctions').then((r) => r.json()),
        fetch('/api/monitor/anomalies').then((r) => r.json()),
        fetch('/api/monitor/outbox').then((r) => r.json()),
        fetch('/api/monitor/scheduler').then((r) => r.json())
      ]);
      setMonitor({ auctions, anomalies, outbox, scheduler });
    } catch {
      Message.error('诊断数据读取失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadMonitor();
  }, []);

  const updateRule = (patch: Partial<RuleDraft>) => {
    setRule((current) => ({ ...current, ...patch }));
    setRuleSaveState('idle');
    setBackendRuleError('');
    setBackendSuggestions([]);
  };

  const saveRule = async () => {
    if (!ruleValidation.valid) return;
    setSavingRule(true);
    setRuleSaveState('idle');
    setBackendRuleError('');
    setBackendSuggestions([]);
    try {
      const response = await fetch('/api/auctions/auc_next/rules', {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          start_price_cents: rule.startPriceCents,
          increment_cents: rule.incrementCents,
          cap_price_cents: rule.capPriceCents,
          duration_seconds: rule.durationSeconds,
          extend_window_seconds: rule.extendWindowSeconds,
          extend_by_seconds: rule.extendBySeconds,
          max_extend_count: rule.maxExtendCount,
          fat_finger_threshold_cents: rule.fatFingerThresholdCents,
          deposit_bps: rule.depositBPS,
          deposit_floor_cents: rule.depositFloorCents,
          deposit_cap_cents: rule.depositCapCents
        })
      });
      if (!response.ok) {
        const payload = await response.json() as RuleAPIError;
        setRuleSaveState('error');
        setBackendRuleError(payload.code === 'INVALID_AUCTION_RULE_CAP_UNREACHABLE' ? '后端拒绝：封顶价不可达' : payload.message ?? '规则保存失败');
        setBackendSuggestions(payload.details?.suggested_caps ?? []);
        return;
      }
      setRuleSaveState('saved');
    } catch {
      setRuleSaveState('error');
      setBackendRuleError('规则保存失败');
    } finally {
      setSavingRule(false);
    }
  };

  return (
    <Layout className="console-shell">
      <Layout.Sider className="sider" width={224}>
        <div className="brand">Live Auction</div>
        <nav>
          <span><ClipboardList size={16} /> 拍品</span>
          <span><RadioTower size={16} /> 竞拍</span>
          <span><Activity size={16} /> 诊断</span>
        </nav>
      </Layout.Sider>
      <Layout.Content className="content">
        <section className="toolbar">
          <div>
            <h1>主控台</h1>
            <p>room_1 · host_1</p>
          </div>
          <Button type="primary" icon={<RefreshCw size={16} />} loading={loading} onClick={loadMonitor}>刷新</Button>
        </section>

        <section className="band">
          <Table
            rowKey="id"
            data={auctionRows}
            pagination={false}
            columns={[
              { title: '商品', dataIndex: 'item' },
              { title: '状态', dataIndex: 'status', render: (value) => <Tag color={value === 'ACTIVE' ? 'green' : value === 'SOLD' ? 'orangered' : 'arcoblue'}>{value}</Tag> },
              { title: '讲解', dataIndex: 'narrating', render: (value) => value ? <Tag color="green">ON</Tag> : <Tag>OFF</Tag> },
              { title: '当前价', dataIndex: 'price' },
              { title: '领先', dataIndex: 'winner' },
              { title: '结束', dataIndex: 'end' },
              { title: '出价数', dataIndex: 'bids' }
            ]}
          />
        </section>

        <section className="band two-column">
          <div className="rule-panel">
            <h2>规则</h2>
            <Form layout="vertical">
              <Form.Item label="起拍价">
                <InputNumber
                  aria-label="start-price-cents"
                  value={rule.startPriceCents}
                  min={0}
                  suffix="分"
                  onChange={(value) => updateRule({ startPriceCents: Number(value) || 0 })}
                />
              </Form.Item>
              <Form.Item label="加价幅度">
                <InputNumber
                  aria-label="increment-cents"
                  value={rule.incrementCents}
                  min={1}
                  suffix="分"
                  onChange={(value) => updateRule({ incrementCents: Number(value) || 0 })}
                />
              </Form.Item>
              <Form.Item
                label="封顶价"
                validateStatus={ruleValidation.valid ? 'success' : 'error'}
                help={ruleValidation.message}
              >
                <InputNumber
                  aria-label="cap-price-cents"
                  value={rule.capPriceCents}
                  min={0}
                  suffix="分"
                  onChange={(value) => updateRule({ capPriceCents: Number(value) || 0 })}
                />
              </Form.Item>
              {backendRuleError && <div className="backend-rule-error" role="alert">{backendRuleError}</div>}
              {ruleSaveState === 'saved' && <div className="rule-save-ok" role="status">规则已保存</div>}
              {shownSuggestions.length > 0 && (
                <div className="cap-suggestions" data-testid="cap-suggestions">
                  {shownSuggestions.map((cap) => (
                    <button key={cap} type="button" onClick={() => updateRule({ capPriceCents: cap })}>
                      {formatCents(cap)}
                    </button>
                  ))}
                </div>
              )}
              <div className="rule-subgrid">
                <Form.Item label="时长">
                  <InputNumber
                    aria-label="duration-seconds"
                    value={rule.durationSeconds}
                    min={30}
                    max={86400}
                    suffix="秒"
                    onChange={(value) => updateRule({ durationSeconds: Number(value) || 30 })}
                  />
                </Form.Item>
                <Form.Item label="延时窗口">
                  <InputNumber
                    aria-label="extend-window-seconds"
                    value={rule.extendWindowSeconds}
                    min={0}
                    suffix="秒"
                    onChange={(value) => updateRule({ extendWindowSeconds: Number(value) || 0 })}
                  />
                </Form.Item>
                <Form.Item label="每次延时">
                  <InputNumber
                    aria-label="extend-by-seconds"
                    value={rule.extendBySeconds}
                    min={0}
                    suffix="秒"
                    onChange={(value) => updateRule({ extendBySeconds: Number(value) || 0 })}
                  />
                </Form.Item>
                <Form.Item label="最多延时">
                  <InputNumber
                    aria-label="max-extend-count"
                    value={rule.maxExtendCount}
                    min={0}
                    suffix="次"
                    onChange={(value) => updateRule({ maxExtendCount: Number(value) || 0 })}
                  />
                </Form.Item>
                <Form.Item label="高额确认">
                  <InputNumber
                    aria-label="fat-finger-threshold-cents"
                    value={rule.fatFingerThresholdCents}
                    min={rule.incrementCents + 1}
                    suffix="分"
                    onChange={(value) => updateRule({ fatFingerThresholdCents: Number(value) || 0 })}
                  />
                </Form.Item>
                <Form.Item label="保证金比例">
                  <InputNumber
                    aria-label="deposit-bps"
                    value={rule.depositBPS}
                    min={0}
                    max={10000}
                    suffix="bps"
                    onChange={(value) => updateRule({ depositBPS: Number(value) || 0 })}
                  />
                </Form.Item>
                <Form.Item label="保证金下限">
                  <InputNumber
                    aria-label="deposit-floor-cents"
                    value={rule.depositFloorCents}
                    min={0}
                    suffix="分"
                    onChange={(value) => updateRule({ depositFloorCents: Number(value) || 0 })}
                  />
                </Form.Item>
                <Form.Item label="保证金上限">
                  <InputNumber
                    aria-label="deposit-cap-cents"
                    value={rule.depositCapCents}
                    min={0}
                    suffix="分"
                    onChange={(value) => updateRule({ depositCapCents: Number(value) || 0 })}
                  />
                </Form.Item>
              </div>
              <Space>
                <Button type="primary" disabled={!ruleValidation.valid} loading={savingRule} onClick={saveRule}>保存规则</Button>
                <Button>排期开拍</Button>
              </Space>
            </Form>
          </div>
          <div className="rule-panel">
            <h2>订单</h2>
            <div className="order-line"><span>ord_pending</span><Tag color="orange">ORDER_PENDING</Tag><strong>¥600.00</strong></div>
            <div className="order-line"><span>ord_paid</span><Tag color="green">PAID</Tag><strong>¥450.00</strong></div>
          </div>
        </section>

        <section className="band diagnostics" data-testid="diagnostics">
          <div className="section-title">
            <h2>诊断</h2>
            <span><Database size={16} /> API</span>
          </div>
          <Tabs defaultActiveTab="auctions">
            <Tabs.TabPane key="auctions" title="Auctions">
              <MonitorTable payload={monitor.auctions} empty="暂无竞拍诊断数据" />
            </Tabs.TabPane>
            <Tabs.TabPane key="anomalies" title="Anomalies">
              <MonitorTable payload={monitor.anomalies} empty="暂无异常" icon={<AlertTriangle size={16} />} />
            </Tabs.TabPane>
            <Tabs.TabPane key="outbox" title="Outbox">
              <MonitorTable payload={monitor.outbox} empty="暂无 outbox 数据" />
            </Tabs.TabPane>
            <Tabs.TabPane key="scheduler" title="Scheduler">
              <MonitorTable payload={monitor.scheduler} empty="暂无 scheduler 数据" />
            </Tabs.TabPane>
          </Tabs>
        </section>
      </Layout.Content>
    </Layout>
  );
}

function MonitorTable({ payload, empty, icon }: { payload?: MonitorPayload; empty: string; icon?: React.ReactNode }) {
  const rows = payload?.items ?? [];
  if (rows.length === 0) {
    return <div className="empty-state">{icon}{empty}</div>;
  }
  const keys = Object.keys(rows[0]).slice(0, 6);
  return (
    <Table
      rowKey={(record) => String(record.id ?? record.auction_id ?? record.outbox_id ?? record.job_id)}
      data={rows}
      pagination={false}
      columns={keys.map((key) => ({
        title: key,
        dataIndex: key,
        render: (value) => String(value ?? '-')
      }))}
    />
  );
}

createRoot(document.getElementById('root')!).render(<App />);
