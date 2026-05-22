import React, { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { Button, Form, InputNumber, Layout, Message, Space, Table, Tabs, Tag } from '@arco-design/web-react';
import '@arco-design/web-react/dist/css/arco.css';
import { Activity, AlertTriangle, ClipboardList, Database, RadioTower, RefreshCw } from 'lucide-react';
import './styles.css';

type MonitorPayload = {
  items: Array<Record<string, unknown>>;
};

const auctionRows = [
  { id: 'auc_live', item: '青瓷手作茶盏', status: 'ACTIVE', narrating: true, price: '¥450.00', winner: '王**', end: '22:00:10', bids: 18 },
  { id: 'auc_next', item: '紫砂壶', status: 'SCHEDULED', narrating: false, price: '¥800.00', winner: '-', end: '22:12:00', bids: 0 },
  { id: 'auc_done', item: '木作托盘', status: 'SOLD', narrating: false, price: '¥620.00', winner: '赵**', end: '21:48:33', bids: 11 }
];

function App() {
  const [monitor, setMonitor] = useState<Record<string, MonitorPayload>>({});
  const [loading, setLoading] = useState(false);

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
                <InputNumber value={10000} min={0} suffix="分" />
              </Form.Item>
              <Form.Item label="加价幅度">
                <InputNumber value={5000} min={1} suffix="分" />
              </Form.Item>
              <Form.Item label="封顶价">
                <InputNumber value={60000} min={15000} suffix="分" />
              </Form.Item>
              <Space>
                <Button type="primary">保存规则</Button>
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
