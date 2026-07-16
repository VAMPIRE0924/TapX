import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ClearOutlined,
  CloudDownloadOutlined,
  CloudUploadOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { Alert, Button, Empty, Modal, Select, Space, Spin, Table, Tabs, Tag, Upload, message } from 'antd';
import type { UploadProps } from 'antd';
import {
  applyRuntimeConfig,
  clearPanelLogs,
  downloadBackupDatabase,
  getPanelLogs,
  restoreBackupDatabase,
  type PanelLogEvent,
} from '../../shared/api';
import './DashboardDialogs.css';
import { useI18n } from '../../i18n/I18nProvider';

export type LogScope = 'all' | 'tapx' | 'xray';

export function LogDialog({ open, scope, onClose }: { open: boolean; scope: LogScope; onClose: () => void }) {
  const { t } = useI18n();
  const [events, setEvents] = useState<PanelLogEvent[]>([]);
  const [loading, setLoading] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();

  const filtered = useMemo(() => events.filter((event) => {
    if (scope === 'all') return true;
    const action = event.action.toLowerCase();
    return scope === 'xray' ? action.includes('xray') : !action.includes('xray');
  }), [events, scope]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setEvents(await getPanelLogs());
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('dashboard.logLoadFailed'));
    } finally {
      setLoading(false);
    }
  }, [messageApi, t]);

  useEffect(() => {
    if (open) void load();
  }, [load, open, scope]);

  async function clear() {
    try {
      await clearPanelLogs();
      setEvents([]);
      messageApi.success(t('dashboard.logsCleared'));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('dashboard.logClearFailed'));
    }
  }

  const title = scope === 'all' ? t('dashboard.panelLogs') : scope === 'xray' ? t('dashboard.xrayLogs') : t('dashboard.tapxLogs');
  return (
    <>
      {contextHolder}
      <Modal
        open={open}
        title={title}
        width={900}
        onCancel={onClose}
        footer={(
          <Space>
            <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void load()}>{t('common.refresh')}</Button>
            <Button danger icon={<ClearOutlined />} disabled={events.length === 0} onClick={() => void clear()}>{t('dashboard.clear')}</Button>
            <Button type="primary" onClick={onClose}>{t('common.close')}</Button>
          </Space>
        )}
      >
        <Table<PanelLogEvent>
          rowKey="seq"
          size="small"
          loading={loading}
          dataSource={[...filtered].reverse()}
          pagination={{ pageSize: 20, hideOnSinglePage: true }}
          scroll={{ x: 760, y: 460 }}
          locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('dashboard.noLogs')} /> }}
          columns={[
            { title: t('dashboard.time'), dataIndex: 'time', width: 190, render: (value: string) => formatLogTime(value) },
            { title: t('dashboard.level'), dataIndex: 'level', width: 80, render: (value: string) => <LogLevel level={value} /> },
            { title: t('dashboard.action'), dataIndex: 'action', width: 180 },
            { title: t('dashboard.content'), dataIndex: 'message' },
          ]}
        />
      </Modal>
    </>
  );
}

export function BackupDialog({ open, onClose, onRestored }: { open: boolean; onClose: () => void; onRestored: () => void }) {
  const { t } = useI18n();
  const [restoring, setRestoring] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [restoreFile, setRestoreFile] = useState<File | null>(null);
  const [messageApi, contextHolder] = message.useMessage();

  const uploadProps: UploadProps = {
    accept: '.db,application/vnd.sqlite3,application/x-sqlite3',
    maxCount: 1,
    fileList: restoreFile ? [{ uid: restoreFile.name, name: restoreFile.name, status: 'done' }] : [],
    beforeUpload: (file) => {
      setRestoreFile(file);
      return Upload.LIST_IGNORE;
    },
    onRemove: () => {
      setRestoreFile(null);
      return true;
    },
  };

  async function exportBackup() {
    setExporting(true);
    try {
      const backup = await downloadBackupDatabase();
      downloadBlob(backup.blob, backup.filename || `tapx-backup-${timestampForFile()}.db`);
      messageApi.success(t('dashboard.backupExported'));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('dashboard.backupExportFailed'));
    } finally {
      setExporting(false);
    }
  }

  function confirmRestore() {
    if (!restoreFile) return;
    Modal.confirm({
      title: t('dashboard.restoreConfirm'),
      content: t('dashboard.restoreConfirmHelp'),
      okText: t('dashboard.restore'),
      cancelText: t('dashboard.cancel'),
      okButtonProps: { danger: true },
      onOk: restore,
    });
  }

  async function restore() {
    if (!restoreFile) return;
    setRestoring(true);
    try {
      await restoreBackupDatabase(restoreFile);
      try {
        await applyRuntimeConfig();
        messageApi.success(t('dashboard.backupRestored'));
      } catch (applyError) {
        messageApi.warning(t('dashboard.restoreApplyFailed', { error: applyError instanceof Error ? applyError.message : String(applyError) }));
      }
      setRestoreFile(null);
      onRestored();
      onClose();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('dashboard.restoreFailed'));
    } finally {
      setRestoring(false);
    }
  }

  return (
    <>
      {contextHolder}
      <Modal open={open} title={t('common.backupRestore')} width={620} onCancel={onClose} footer={<Button onClick={onClose}>{t('common.close')}</Button>}>
        <div className="backup-dialog-grid">
          <section>
            <h3>{t('dashboard.exportBackup')}</h3>
            <p>{t('dashboard.exportBackupHelp')}</p>
            <Button type="primary" icon={<CloudDownloadOutlined />} loading={exporting} onClick={() => void exportBackup()}>
              {t('dashboard.exportDatabase')}
            </Button>
          </section>
          <section>
            <h3>{t('dashboard.restoreBackup')}</h3>
            <Alert type="warning" showIcon title={t('dashboard.restoreWarning')} />
            <Upload {...uploadProps}>
              <Button icon={<CloudUploadOutlined />}>{t('dashboard.selectDatabaseFile')}</Button>
            </Upload>
            <Button danger type="primary" disabled={!restoreFile} loading={restoring} onClick={confirmRestore}>{t('dashboard.restoreAndApply')}</Button>
          </section>
        </div>
      </Modal>
    </>
  );
}

export interface DashboardSample {
  at: number;
  cpu?: number;
  memory?: number;
  swap?: number;
  rx?: number;
  tx?: number;
  rxPackets?: number;
  txPackets?: number;
  tcpConnections?: number;
  udpConnections?: number;
  diskRead?: number;
  diskWrite?: number;
  diskUsage?: number;
  online?: number;
  load1?: number;
  load5?: number;
  load15?: number;
  embeddedXray?: number;
  externalXray?: number;
  tapx?: number;
  drops?: number;
  tapxHeap?: number;
  tapxSys?: number;
  tapxObjects?: number;
  tapxGC?: number;
  tapxGCPause?: number;
  tapxObservatory?: number;
  embeddedHeap?: number;
  embeddedSys?: number;
  embeddedObjects?: number;
  embeddedGC?: number;
  embeddedGCPause?: number;
  embeddedObservatory?: number;
  externalHeap?: number;
  externalSys?: number;
  externalObjects?: number;
  externalGC?: number;
  externalGCPause?: number;
  externalObservatory?: number;
}

export type ChartKind = 'system' | 'embedded-xray' | 'external-xray' | 'tapx';

type MetricUnit = 'percent' | 'bytes' | 'bytesPerSecond' | 'packetsPerSecond' | 'nanoseconds' | 'count' | 'load';

interface MetricSeries {
  key: keyof DashboardSample;
  label: string;
  color: string;
}

interface MetricTab {
  key: string;
  label: string;
  title: string;
  unit: MetricUnit;
  series: MetricSeries[];
  maximum?: number;
}

export function ChartDialog({ open, kind, samples, onClose }: { open: boolean; kind: ChartKind; samples: DashboardSample[]; onClose: () => void }) {
  const { t } = useI18n();
  const tabs = useMemo(() => metricTabs(kind, t), [kind, t]);
  const [activeKey, setActiveKey] = useState(tabs[0]?.key || 'cpu');
  const [rangeMinutes, setRangeMinutes] = useState(2);
  const active = tabs.find((item) => item.key === activeKey) || tabs[0];
  const title = kind === 'system' ? t('dashboard.systemHistory') : kind === 'tapx' ? t('dashboard.tapxMetrics') : kind === 'embedded-xray' ? t('dashboard.embeddedXrayMetrics') : t('dashboard.externalXrayMetrics');

  useEffect(() => {
    if (!open) return;
    setActiveKey(tabs[0]?.key || 'cpu');
    setRangeMinutes(2);
  }, [kind, open, tabs]);

  const visibleSamples = useMemo(() => {
    if (samples.length === 0) return [];
    const latest = samples[samples.length - 1].at;
    const cutoff = latest - rangeMinutes * 60_000;
    return samples.filter((sample) => sample.at >= cutoff);
  }, [rangeMinutes, samples]);

  const hasMetricData = Boolean(active && visibleSamples.some((sample) => active.series.some((item) => isMetricValue(sample[item.key]))));

  return (
    <Modal
      open={open}
      title={(
        <div className="metric-dialog-title">
          <span>{title}</span>
          <Select
            size="small"
            value={rangeMinutes}
            className="metric-range-select"
            onChange={setRangeMinutes}
            options={[
              { value: 2, label: '2m' },
              { value: 60, label: '1h' },
              { value: 180, label: '3h' },
              { value: 360, label: '6h' },
              { value: 720, label: '12h' },
              { value: 1440, label: '24h' },
              { value: 2880, label: '2d' },
              { value: 10080, label: '7d' },
            ]}
          />
        </div>
      )}
      width={900}
      onCancel={onClose}
      footer={null}
    >
      <Tabs
        size="small"
        activeKey={active?.key}
        className="metric-tabs"
        onChange={setActiveKey}
        items={tabs.map((item) => ({ key: item.key, label: item.label }))}
      />
      <section className="metric-chart-panel">
        <h3>{active?.title}</h3>
        {visibleSamples.length < 2 ? (
          <div className="chart-loading"><Spin /><span>{t('dashboard.collectingHistory')}</span></div>
        ) : hasMetricData && active ? (
          <LineChart
            samples={visibleSamples}
            series={active.series}
            unit={active.unit}
            maximum={active.maximum}
            ariaLabel={`${title} - ${active.label}`}
          />
        ) : (
          <div className="chart-empty"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('dashboard.metricUnavailable')} /></div>
        )}
      </section>
    </Modal>
  );
}

function LineChart({ samples, series, unit, maximum, ariaLabel }: {
  samples: DashboardSample[];
  series: MetricSeries[];
  unit: MetricUnit;
  maximum?: number;
  ariaLabel: string;
}) {
  const width = 760;
  const height = 300;
  const left = 64;
  const right = 18;
  const top = 18;
  const bottom = 34;
  const values = samples.flatMap((sample) => series.flatMap((item) => isMetricValue(sample[item.key]) ? [Number(sample[item.key])] : []));
  const chartMaximum = Math.max(1, maximum || 0, ...values);
  const yTicks = Array.from({ length: 5 }, (_, index) => chartMaximum * (4 - index) / 4);
  const xTicks = Array.from(new Set([0, Math.floor((samples.length - 1) / 3), Math.floor((samples.length - 1) * 2 / 3), samples.length - 1]));
  const points = (key: keyof DashboardSample) => samples.flatMap((sample, index) => {
    if (!isMetricValue(sample[key])) return [];
    const x = left + (index / Math.max(1, samples.length - 1)) * (width - left - right);
    const y = height - bottom - (Number(sample[key]) / chartMaximum) * (height - top - bottom);
    return [`${x.toFixed(1)},${y.toFixed(1)}`];
  }).join(' ');
  const formatter = metricFormatter(unit);

  return (
    <div className="history-chart">
      <div className="chart-toolbar">
        <div className="chart-legend">
          {series.map((item) => <span key={item.label}><i style={{ background: item.color }} />{item.label}</span>)}
        </div>
        <div className="chart-extrema">
          <span className="is-max">▲ {formatter(Math.max(...values))}</span>
          <span className="is-min">▼ {formatter(Math.min(...values))}</span>
        </div>
      </div>
      <svg role="img" aria-label={ariaLabel} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
        {yTicks.map((value, index) => {
          const y = top + index * (height - top - bottom) / 4;
          return (
            <g key={`y-${value}`}>
              <line x1={left} y1={y} x2={width - right} y2={y} className="chart-grid-line" />
              <text x={left - 8} y={y + 4} textAnchor="end" className="chart-axis-label">{formatter(value)}</text>
            </g>
          );
        })}
        {xTicks.map((index) => {
          const x = left + (index / Math.max(1, samples.length - 1)) * (width - left - right);
          return <text key={`x-${index}`} x={x} y={height - 8} textAnchor={index === 0 ? 'start' : index === samples.length - 1 ? 'end' : 'middle'} className="chart-axis-label">{formatChartTime(samples[index].at)}</text>;
        })}
        {series.map((item) => <polyline key={item.label} points={points(item.key)} fill="none" stroke={item.color} strokeWidth="2.5" vectorEffect="non-scaling-stroke" />)}
      </svg>
    </div>
  );
}

function metricTabs(kind: ChartKind, t: ReturnType<typeof useI18n>['t']): MetricTab[] {
  if (kind === 'system') {
    return [
      metricTab('cpu', 'CPU', t('dashboard.cpuUsage'), 'percent', [['cpu', 'CPU', '#1677ff']], 100),
      metricTab('memory', t('dashboard.memory'), t('dashboard.memoryUsage'), 'percent', [['memory', t('dashboard.memory'), '#7c4dff'], ['swap', t('dashboard.swap'), '#fa8c16']], 100),
      metricTab('bandwidth', t('dashboard.bandwidth'), t('dashboard.networkThroughput'), 'bytesPerSecond', [['tx', t('dashboard.upload'), '#1677ff'], ['rx', t('dashboard.download'), '#13c2c2']]),
      metricTab('packets', t('dashboard.packets'), t('dashboard.packetRate'), 'packetsPerSecond', [['txPackets', t('dashboard.upload'), '#2f54eb'], ['rxPackets', t('dashboard.download'), '#36cfc9']]),
      metricTab('connections', t('dashboard.connections'), t('dashboard.connectionHistory'), 'count', [['tcpConnections', 'TCP', '#597ef7'], ['udpConnections', 'UDP', '#73d13d']]),
      metricTab('disk-io', t('dashboard.diskIO'), t('dashboard.diskIOHistory'), 'bytesPerSecond', [['diskRead', t('dashboard.read'), '#eb2f96'], ['diskWrite', t('dashboard.write'), '#722ed1']]),
      metricTab('disk-usage', t('dashboard.diskUsage'), t('dashboard.diskUsageHistory'), 'percent', [['diskUsage', t('dashboard.diskUsage'), '#13c2c2']], 100),
      metricTab('online', t('dashboard.online'), t('dashboard.onlineHistory'), 'count', [['online', t('dashboard.online'), '#52c41a']]),
      metricTab('load', t('dashboard.load'), t('dashboard.loadHistory'), 'load', [['load1', '1m', '#fa8c16'], ['load5', '5m', '#f5222d'], ['load15', '15m', '#a0d911']]),
    ];
  }

  const prefix = kind === 'tapx' ? 'tapx' : kind === 'embedded-xray' ? 'embedded' : 'external';
  const observatoryLabel = kind === 'tapx' ? t('dashboard.runningPipes') : t('dashboard.endpointCount');
  return [
    metricTab('heap', t('dashboard.heap'), t('dashboard.heapAllocated'), 'bytes', [[`${prefix}Heap` as keyof DashboardSample, t('dashboard.heap'), '#7c4dff']]),
    metricTab('system', t('dashboard.runtimeSystem'), t('dashboard.systemMemory'), 'bytes', [[`${prefix}Sys` as keyof DashboardSample, t('dashboard.runtimeSystem'), '#1890ff']]),
    metricTab('objects', t('dashboard.objects'), t('dashboard.heapObjects'), 'count', [[`${prefix}Objects` as keyof DashboardSample, t('dashboard.objects'), '#13c2c2']]),
    metricTab('gc-count', t('dashboard.gcCount'), t('dashboard.gcCountHistory'), 'count', [[`${prefix}GC` as keyof DashboardSample, t('dashboard.gcCount'), '#fa8c16']]),
    metricTab('gc-pause', t('dashboard.gcPause'), t('dashboard.gcPauseHistory'), 'nanoseconds', [[`${prefix}GCPause` as keyof DashboardSample, t('dashboard.gcPause'), '#f5222d']]),
    metricTab('observatory', t('dashboard.observatory'), t('dashboard.observatoryHistory'), 'count', [[`${prefix}Observatory` as keyof DashboardSample, observatoryLabel, '#52c41a']]),
  ];
}

function metricTab(key: string, label: string, title: string, unit: MetricUnit, series: Array<[keyof DashboardSample, string, string]>, maximum?: number): MetricTab {
  return { key, label, title, unit, maximum, series: series.map(([seriesKey, seriesLabel, color]) => ({ key: seriesKey, label: seriesLabel, color })) };
}

function isMetricValue(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}

function metricFormatter(unit: MetricUnit): (value: number) => string {
  if (unit === 'percent') return (value) => `${value.toFixed(1)}%`;
  if (unit === 'bytes') return (value) => formatMetricBytes(value);
  if (unit === 'bytesPerSecond') return (value) => `${formatMetricBytes(value)}/s`;
  if (unit === 'packetsPerSecond') return (value) => `${Math.round(value).toLocaleString()}/s`;
  if (unit === 'nanoseconds') return (value) => value >= 1_000_000 ? `${(value / 1_000_000).toFixed(2)} ms` : value >= 1_000 ? `${(value / 1_000).toFixed(1)} μs` : `${Math.round(value)} ns`;
  if (unit === 'load') return (value) => value.toFixed(2);
  return (value) => Math.round(value).toLocaleString();
}

function formatMetricBytes(value: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let amount = Math.max(0, Number(value) || 0);
  let index = 0;
  while (amount >= 1024 && index < units.length - 1) {
    amount /= 1024;
    index += 1;
  }
  return `${amount >= 10 || index === 0 ? amount.toFixed(0) : amount.toFixed(2)} ${units[index]}`;
}

function LogLevel({ level }: { level: string }) {
  const normalized = level.toLowerCase();
  const color = normalized === 'error' ? 'red' : normalized === 'warn' || normalized === 'warning' ? 'orange' : normalized === 'debug' ? 'default' : 'blue';
  return <Tag color={color}>{level || 'info'}</Tag>;
}

function formatLogTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function formatChartTime(value: number) {
  return new Date(value).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function timestampForFile() {
  return new Date().toISOString().replace(/[:.]/g, '-');
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}
