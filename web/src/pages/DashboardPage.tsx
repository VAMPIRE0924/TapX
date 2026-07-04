import { useEffect, useMemo, useState } from 'react';
import { Button, Card, Col, Descriptions, Row, Space, Statistic, Table, Tag, Typography, message } from 'antd';
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  CheckCircleOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
} from '@ant-design/icons';

import type { AnyRecord, RuntimeConfig } from '@/api';
import { applyRuntime, loadDashboard, stopRuntime } from '@/api';
import { kindDefs, getItems } from '@/schema';
import { useI18n } from '@/i18n';

interface DashboardPageProps {
  config: RuntimeConfig;
  runtime: AnyRecord;
  onRefresh: () => Promise<void>;
}

function fmtBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = value;
  let idx = 0;
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024;
    idx += 1;
  }
  return `${size.toFixed(idx === 0 ? 0 : 2)} ${units[idx]}`;
}

export function DashboardPage({ config, runtime, onRefresh }: DashboardPageProps) {
  const { t } = useI18n();
  const [dashboard, setDashboard] = useState<AnyRecord>({});
  const [busy, setBusy] = useState(false);

  async function refreshDashboard() {
    try {
      setDashboard(await loadDashboard());
    } catch (error) {
      message.error((error as Error).message);
    }
  }

  useEffect(() => {
    refreshDashboard();
  }, []);

  const counts = useMemo(() => kindDefs.map((kind) => ({
    key: kind.key,
    title: kind.title,
    count: getItems(config, kind).length,
  })), [config]);

  async function runtimeAction(action: 'apply' | 'stop') {
    setBusy(true);
    try {
      if (action === 'apply') await applyRuntime();
      if (action === 'stop') await stopRuntime();
      await Promise.all([onRefresh(), refreshDashboard()]);
      message.success(action === 'apply' ? 'runtime applied' : 'runtime stopped');
    } catch (error) {
      message.error((error as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const totals = dashboard?.stats?.totals || {};
  const rates = dashboard?.rates || {};
  const recentLogs = Array.isArray(dashboard?.recentLogs) ? dashboard.recentLogs : [];
  const running = Boolean(runtime?.running);

  return (
    <Space direction="vertical" size="middle" className="full-width">
      <Card
        className="page-card"
        title={t('dashboard')}
        extra={(
          <Space wrap>
            <Button icon={<ReloadOutlined />} onClick={() => Promise.all([onRefresh(), refreshDashboard()])}>{t('refresh')}</Button>
            <Button type="primary" loading={busy} icon={<PlayCircleOutlined />} onClick={() => runtimeAction('apply')}>{t('apply')}</Button>
            <Button danger loading={busy} icon={<PauseCircleOutlined />} onClick={() => runtimeAction('stop')}>{t('stop')}</Button>
          </Space>
        )}
      >
        <Row gutter={[16, 16]}>
          <Col xs={24} md={8}>
            <Statistic
              title={t('runtime')}
              value={running ? 'running' : 'stopped'}
              prefix={running ? <CheckCircleOutlined /> : <PauseCircleOutlined />}
              valueStyle={{ color: running ? '#389e0d' : undefined }}
            />
          </Col>
          <Col xs={12} md={4}>
            <Statistic title="RX" value={fmtBytes(Number(totals.rxBytes || 0))} prefix={<ArrowDownOutlined />} />
          </Col>
          <Col xs={12} md={4}>
            <Statistic title="TX" value={fmtBytes(Number(totals.txBytes || 0))} prefix={<ArrowUpOutlined />} />
          </Col>
          <Col xs={12} md={4}>
            <Statistic title="RX Rate" value={fmtBytes(Number(rates.rxBytesPerSecond || 0)) + '/s'} />
          </Col>
          <Col xs={12} md={4}>
            <Statistic title="TX Rate" value={fmtBytes(Number(rates.txBytesPerSecond || 0)) + '/s'} />
          </Col>
        </Row>
      </Card>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <Card className="page-card" title={t('objects')}>
            <Row gutter={[12, 12]}>
              {counts.map((item) => (
                <Col xs={12} md={8} key={item.key}>
                  <Statistic title={item.title} value={item.count} />
                </Col>
              ))}
            </Row>
          </Card>
        </Col>
        <Col xs={24} lg={12}>
          <Card className="page-card" title={t('status')}>
            <Descriptions column={1} size="small">
              <Descriptions.Item label="Generation">{runtime?.generation || 0}</Descriptions.Item>
              <Descriptions.Item label="Reload Mode">{runtime?.lastReloadMode || ''}</Descriptions.Item>
              <Descriptions.Item label="UDP Pipes">{Array.isArray(runtime?.udpPipes) ? runtime.udpPipes.length : 0}</Descriptions.Item>
              <Descriptions.Item label="TCP Pipes">{Array.isArray(runtime?.tcpPipes) ? runtime.tcpPipes.length : 0}</Descriptions.Item>
              <Descriptions.Item label="Xray Pipes">{Array.isArray(runtime?.xrayPipes) ? runtime.xrayPipes.length : 0}</Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
      </Row>

      <Card className="page-card" title="Recent Logs">
        <Table
          rowKey={(_, idx) => String(idx)}
          dataSource={recentLogs}
          pagination={false}
          size="small"
          columns={[
            { title: 'Time', dataIndex: 'time', width: 220 },
            { title: 'Level', dataIndex: 'level', width: 100, render: (value) => <Tag>{value}</Tag> },
            { title: 'Action', dataIndex: 'action', width: 180 },
            { title: 'Message', dataIndex: 'message', render: (value) => <Typography.Text>{value}</Typography.Text> },
          ]}
        />
      </Card>
    </Space>
  );
}
