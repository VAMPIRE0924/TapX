import { useEffect, useMemo, useState, type CSSProperties, type ReactNode } from 'react';
import {
  BranchesOutlined,
  CloudDownloadOutlined,
  CloudOutlined,
  CloudUploadOutlined,
  GithubOutlined,
  LineChartOutlined,
  OrderedListOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { Modal, Tooltip, message } from 'antd';
import {
  getDashboard,
  getDiagnostics,
  restartRuntimeComponent,
  stopRuntimeComponent,
  type RuntimeComponent,
  type DashboardReport,
  type DiagnosticReport,
  type UpdateComponent,
} from '../shared/api';
import { formatBytes } from '../shared/format';
import { useI18n } from '../i18n/I18nProvider';
import {
  BackupDialog,
  ChartDialog,
  LogDialog,
  type ChartKind,
  type DashboardSample,
  type LogScope,
} from '../features/dashboard/DashboardDialogs';
import './DashboardPage.css';
import { ComponentUpdateDialog } from '../features/updates/ComponentUpdateDialog';

const emptyReport: DashboardReport = {
  runtime: { running: false, generation: 0, udpPipes: [], tcpPipes: [], xrayPipes: [] },
  rates: { rxBytesPerSecond: 0, txBytesPerSecond: 0 },
  stats: { totals: { rxBytes: 0, txBytes: 0, dropsGuard: 0, dropsIO: 0 } },
  objectCounts: {},
  process: { heapAlloc: 0, goroutines: 0, uptimeSecond: 0 },
  recentLogs: [],
  system: {},
};

export function DashboardPage() {
  const { t } = useI18n();
  const [data, setData] = useState<DashboardReport>(emptyReport);
  const [diagnostics, setDiagnostics] = useState<DiagnosticReport>();
  const [error, setError] = useState<string>('');
  const [logs, setLogs] = useState<{ open: boolean; scope: LogScope }>({ open: false, scope: 'all' });
  const [backupOpen, setBackupOpen] = useState(false);
  const [chart, setChart] = useState<{ open: boolean; kind: ChartKind }>({ open: false, kind: 'system' });
  const [samples, setSamples] = useState<DashboardSample[]>([]);
  const [updateTarget, setUpdateTarget] = useState<UpdateComponent>();
  const [messageApi, messageContextHolder] = message.useMessage();

  useEffect(() => {
    let active = true;

    async function load() {
      try {
        const report = await getDashboard();
        if (active) {
          setData(report);
          setSamples((current) => appendSample(
            current.length > 0 ? current : (report.history || []),
            report,
          ));
          setError('');
        }
      } catch (err) {
        if (active) setError(err instanceof Error ? err.message : 'load failed');
      }
    }

    void load();
    const timer = window.setInterval(load, 2500);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    void getDiagnostics().then(setDiagnostics).catch(() => undefined);
  }, []);

  const totals = data.stats?.totals || emptyReport.stats!.totals!;
  const runtime = data.runtime || emptyReport.runtime!;
  const counts = data.objectCounts || {};
  const system = data.system || {};
  const rates = data.rates || {};
  const tapxPipes = (runtime.udpPipes?.length || 0) + (runtime.tcpPipes?.length || 0);
  const xrayPipes = runtime.xrayPipes?.length || 0;
  const activeEndpoints = (data.stats?.byEndpoint || []).filter((item) => (item.pipes || 0) > 0);
  const activeListeners = activeEndpoints.filter((item) => item.kind === 'listener').length;
  const activeConnectors = activeEndpoints.filter((item) => item.kind === 'connector').length;
  const activeDevices = (data.stats?.byDevice || []).filter((item) => item.id !== '(unbound)' && (item.pipes || 0) > 0).length;
  const onlineUsers = (data.stats?.clients || []).filter((item) => (item.activePipes || 0) > 0).length;
  const activeBindings = (data.stats?.byRoute || []).filter((item) => item.id !== '(unbound)' && (item.pipes || 0) > 0).length;
  const xrayRuntimes = runtime.xrayRuntimes || [];
  const embeddedXray = xrayRuntimes.find((item) => item.runtime === 'embedded');
  const externalXray = xrayRuntimes.find((item) => item.runtime === 'external');

  function confirmRuntimeAction(component: RuntimeComponent, label: string, action: 'stop' | 'restart') {
    Modal.confirm({
      title: action === 'stop'
        ? t('dashboard.componentStopConfirm', { component: label })
        : t('dashboard.componentRestartConfirm', { component: label }),
      content: action === 'stop'
        ? t('dashboard.componentStopHelp', { component: label })
        : t('dashboard.componentRestartHelp', { component: label }),
      okText: action === 'stop' ? t('common.stop') : t('dashboard.reload'),
      cancelText: t('dashboard.cancel'),
      okButtonProps: action === 'stop' ? { danger: true } : undefined,
      onOk: async () => {
        try {
          if (action === 'stop') await stopRuntimeComponent(component);
          else await restartRuntimeComponent(component);
          setData(await getDashboard());
          messageApi.success(action === 'stop'
            ? t('dashboard.componentStopped', { component: label })
            : t('dashboard.componentRestarted', { component: label }));
        } catch (actionError) {
          messageApi.error(actionError instanceof Error ? actionError.message : t('dashboard.runtimeActionFailed'));
        }
      },
    });
  }

  const gauges = useMemo(() => [
    {
      label: `${t('dashboard.cpu')}: ${system.cpuCores ?? 0} ${t('dashboard.cores')}`,
      value: percent(system.cpuPercent),
      sub: '',
    },
    {
      label: `${t('dashboard.memory')}: ${formatBytes(system.memoryUsed || 0)} / ${formatBytes(system.memoryTotal || 0)}`,
      value: percent(ratio(system.memoryUsed, system.memoryTotal)),
      sub: '',
    },
    {
      label: `${t('dashboard.swap')}: ${formatBytes(system.swapUsed || 0)} / ${formatBytes(system.swapTotal || 0)}`,
      value: percent(ratio(system.swapUsed, system.swapTotal)),
      sub: '',
    },
    {
      label: `${t('dashboard.storage')}: ${formatBytes(system.storageUsed || 0)} / ${formatBytes(system.storageTotal || 0)}`,
      value: percent(ratio(system.storageUsed, system.storageTotal)),
      sub: '',
    },
  ], [
    system.cpuCores,
    system.cpuPercent,
    system.memoryTotal,
    system.memoryUsed,
    system.storageTotal,
    system.storageUsed,
    system.swapTotal,
    system.swapUsed,
    t,
  ]);

  return (
    <div className="dashboard-page">
      {messageContextHolder}
      <section className="performance-card">
        {gauges.map((item) => (
          <Gauge key={item.label} label={item.label} percent={item.value} sub={item.sub} />
        ))}
      </section>

      {error ? <div className="inline-alert">{t('dashboard.backendUnavailable')}：{error}</div> : null}

      <section className="status-grid">
        <DashboardCard title={t('app.brand')}>
          <ActionGrid>
            <ActionItem icon={<GithubOutlined />} text="@TapX" onClick={() => window.open('https://github.com/VAMPIRE0924/TapX', '_blank', 'noopener,noreferrer')} />
            <ActionItem icon={<CloudOutlined />} text={displayVersion(diagnostics?.components?.panel || diagnostics?.version)} onClick={() => setUpdateTarget('panel')} />
          </ActionGrid>
        </DashboardCard>

        <DashboardCard title={t('dashboard.tapx')} status={tapxPipes > 0 ? t('common.running') : t('common.stopped')} statusTone={tapxPipes > 0 ? 'green' : 'orange'}>
          <ActionGrid>
            <ActionItem icon={<OrderedListOutlined />} text={t('common.logs')} onClick={() => setLogs({ open: true, scope: 'tapx' })} />
            <ActionItem icon={<PoweroffOutlined />} text={t('common.stop')} onClick={() => confirmRuntimeAction('tapx', 'TapX', 'stop')} />
            <ActionItem icon={<ReloadOutlined />} text={t('common.restart')} onClick={() => confirmRuntimeAction('tapx', 'TapX', 'restart')} />
            <ActionItem icon={<ToolOutlined />} text={t('common.config')} onClick={() => setUpdateTarget('tapx')} />
          </ActionGrid>
        </DashboardCard>

        <DashboardCard title={t('dashboard.embeddedXray')} status={runtimeStatus(embeddedXray, t)} statusTone={embeddedXray?.running ? 'green' : 'orange'}>
          <ActionGrid>
            <ActionItem icon={<OrderedListOutlined />} text={t('common.logs')} onClick={() => setLogs({ open: true, scope: 'xray' })} />
            <ActionItem icon={<PoweroffOutlined />} text={t('common.stop')} onClick={() => confirmRuntimeAction('embedded-xray', t('dashboard.embeddedXray'), 'stop')} />
            <ActionItem icon={<ReloadOutlined />} text={t('common.restart')} onClick={() => confirmRuntimeAction('embedded-xray', t('dashboard.embeddedXray'), 'restart')} />
            <ActionItem icon={<ToolOutlined />} text={t('common.config')} onClick={() => setUpdateTarget('tapx')} />
          </ActionGrid>
        </DashboardCard>

        <DashboardCard title={t('dashboard.externalXray')} status={runtimeStatus(externalXray, t)} statusTone={externalXray?.running ? 'green' : 'orange'}>
          <ActionGrid>
            <ActionItem icon={<OrderedListOutlined />} text={t('common.logs')} onClick={() => setLogs({ open: true, scope: 'xray' })} />
            <ActionItem icon={<PoweroffOutlined />} text={t('common.stop')} onClick={() => confirmRuntimeAction('external-xray', t('dashboard.externalXray'), 'stop')} />
            <ActionItem icon={<ReloadOutlined />} text={t('common.restart')} onClick={() => confirmRuntimeAction('external-xray', t('dashboard.externalXray'), 'restart')} />
            <ActionItem icon={<ToolOutlined />} text={t('common.config')} onClick={() => setUpdateTarget('external-xray')} />
          </ActionGrid>
        </DashboardCard>

        <DashboardCard title={t('dashboard.management')}>
          <ActionGrid>
            <ActionItem icon={<OrderedListOutlined />} text={t('common.logs')} onClick={() => setLogs({ open: true, scope: 'all' })} />
            <ActionItem icon={<CloudUploadOutlined />} text={t('common.backupRestore')} onClick={() => setBackupOpen(true)} />
          </ActionGrid>
        </DashboardCard>

        <DashboardCard title={t('dashboard.charts')}>
          <ActionGrid>
            <ActionItem icon={<LineChartOutlined />} text={t('dashboard.systemHistory')} onClick={() => setChart({ open: true, kind: 'system' })} />
            <ActionItem icon={<LineChartOutlined />} text={t('dashboard.tapx')} onClick={() => setChart({ open: true, kind: 'tapx' })} />
            <ActionItem icon={<LineChartOutlined />} text={t('dashboard.embeddedXray')} onClick={() => setChart({ open: true, kind: 'embedded-xray' })} />
            <ActionItem icon={<LineChartOutlined />} text={t('dashboard.externalXray')} onClick={() => setChart({ open: true, kind: 'external-xray' })} />
          </ActionGrid>
        </DashboardCard>

        <DashboardCard title={t('dashboard.realtimeTransport')}>
          <StatGrid columns={2}>
            <Stat label={t('dashboard.txRate')} value={`${formatBytes(rates.txBytesPerSecond || 0)}/s`} icon={<CloudUploadOutlined />} />
            <Stat label={t('dashboard.rxRate')} value={`${formatBytes(rates.rxBytesPerSecond || 0)}/s`} icon={<CloudDownloadOutlined />} />
            <Stat label={t('dashboard.txPacketRate')} value={`${formatCount(rates.txPacketsPerSecond)} pps`} />
            <Stat label={t('dashboard.rxPacketRate')} value={`${formatCount(rates.rxPacketsPerSecond)} pps`} />
          </StatGrid>
        </DashboardCard>

        <DashboardCard title={t('dashboard.dataPlane')}>
          <StatGrid columns={2}>
            <Stat label={t('dashboard.tapxPipes')} value={String(tapxPipes)} icon={<BranchesOutlined />} />
            <Stat label={t('dashboard.xrayPipes')} value={String(xrayPipes)} icon={<BranchesOutlined />} />
            <Stat label={t('dashboard.tcpPipes')} value={String(runtime.tcpPipes?.length || 0)} />
            <Stat label={t('dashboard.udpPipes')} value={String(runtime.udpPipes?.length || 0)} />
          </StatGrid>
        </DashboardCard>

        <DashboardCard title={t('dashboard.endpointStatus')}>
          <StatGrid columns={2}>
            <Stat label={t('dashboard.activeListeners')} value={String(activeListeners)} />
            <Stat label={t('dashboard.activeConnectors')} value={String(activeConnectors)} />
            <Stat label={t('dashboard.onlineUsers')} value={String(onlineUsers)} />
            <Stat label={t('dashboard.activeDevices')} value={String(activeDevices)} />
          </StatGrid>
        </DashboardCard>

        <DashboardCard title={t('dashboard.policyProtection')}>
          <StatGrid columns={2}>
            <Stat label={t('dashboard.activeBindings')} value={String(activeBindings)} />
            <Stat label={t('dashboard.addressLimits')} value={String(counts.addressLimits || 0)} />
            <Stat label={t('dashboard.guardDrops')} value={String(totals.dropsGuard || 0)} />
            <Stat label={t('dashboard.ioDrops')} value={String(totals.dropsIO || 0)} />
          </StatGrid>
        </DashboardCard>
      </section>

      <LogDialog open={logs.open} scope={logs.scope} onClose={() => setLogs((current) => ({ ...current, open: false }))} />
      <BackupDialog open={backupOpen} onClose={() => setBackupOpen(false)} onRestored={() => void getDashboard().then(setData)} />
      <ChartDialog open={chart.open} kind={chart.kind} samples={samples} onClose={() => setChart((current) => ({ ...current, open: false }))} />
      {updateTarget ? (
        <ComponentUpdateDialog
          open
          component={updateTarget}
          onClose={() => setUpdateTarget(undefined)}
          onUpdated={() => {
            void getDashboard().then(setData);
            void getDiagnostics().then(setDiagnostics);
          }}
        />
      ) : null}
    </div>
  );
}

function Gauge({ label, percent, sub }: { label: string; percent: number; sub?: string }) {
  const angle = Math.max(0, Math.min(100, percent)) * 3.6;
  return (
    <div className="gauge-block">
      <div className="gauge-ring" style={{ '--gauge-angle': `${angle}deg` } as CSSProperties}>
        <div className="gauge-inner">{Math.round(percent)}%</div>
      </div>
      <div className="gauge-label">
        <strong>{label}</strong>
        {sub ? <span>{sub}</span> : null}
      </div>
    </div>
  );
}

function formatCount(value: number | undefined): string {
  return Math.max(0, Math.round(value || 0)).toLocaleString();
}

function displayVersion(version?: string): string {
  const value = version?.trim() || 'dev';
  return value === 'dev' || value === 'unknown' ? value : `v${value.replace(/^v/, '')}`;
}

function DashboardCard({
  title,
  status,
  statusTone,
  children,
}: {
  title: string;
  status?: string;
  statusTone?: 'green' | 'orange';
  children: ReactNode;
}) {
  return (
    <article className="dashboard-card">
      <header>
        <strong>{title}</strong>
        {status ? (
          <span className={`card-status ${statusTone === 'green' ? 'is-green' : 'is-orange'}`}>
            <i />
            {status}
          </span>
        ) : null}
      </header>
      <div className="dashboard-card-body">{children}</div>
    </article>
  );
}

function ActionGrid({ children }: { children: ReactNode }) {
  return <div className="action-grid">{children}</div>;
}

function ActionItem({ icon, text, onClick, disabled, hint }: { icon: ReactNode; text: string; onClick?: () => void; disabled?: boolean; hint?: string }) {
  const button = (
    <button type="button" className="action-item" onClick={onClick} disabled={disabled}>
      {icon}
      <span>{text}</span>
    </button>
  );
  if (hint) return <Tooltip title={hint}>{button}</Tooltip>;
  return (
    button
  );
}

function StatGrid({ children, columns }: { children: ReactNode; columns: 2 | 3 }) {
  return <div className={`stat-grid stat-grid-${columns}`}>{children}</div>;
}

function Stat({ label, value, icon }: { label: string; value: string; icon?: ReactNode }) {
  return (
    <div className="stat-item">
      <span>{label}</span>
      <strong>{icon}{value}</strong>
    </div>
  );
}

function ratio(used?: number, total?: number) {
  if (!used || !total || total <= 0) return 0;
  return (used / total) * 100;
}

function percent(value?: number) {
  if (!value || !Number.isFinite(value)) return 0;
  return value <= 1 ? value * 100 : value;
}

function appendSample(current: DashboardSample[], report: DashboardReport): DashboardSample[] {
  const runtime = report.runtime || {};
  const totals = report.stats?.totals || {};
  const system = report.system || {};
  const runtimes = runtime.xrayRuntimes || [];
  const embedded = runtimes.find((item) => item.runtime === 'embedded');
  const external = runtimes.find((item) => item.runtime === 'external');
  const process = report.process || {};
  const rawPipes = (runtime.udpPipes?.length || 0) + (runtime.tcpPipes?.length || 0);
  const generatedAt = report.generatedAt ? Date.parse(report.generatedAt) : Number.NaN;
  const next: DashboardSample = {
    at: Number.isFinite(generatedAt) ? generatedAt : Date.now(),
    cpu: percent(system.cpuPercent),
    memory: percent(ratio(system.memoryUsed, system.memoryTotal)),
    swap: percent(ratio(system.swapUsed, system.swapTotal)),
    diskUsage: percent(ratio(system.storageUsed, system.storageTotal)),
	 diskRead: system.diskReadBytesPerSecond,
	 diskWrite: system.diskWriteBytesPerSecond,
    rxPackets: report.rates?.rxPacketsPerSecond,
    txPackets: report.rates?.txPacketsPerSecond,
    tcpConnections: system.tcpConnections,
    udpConnections: system.udpConnections,
    online: report.stats?.clients?.filter((client) => (client.activePipes || 0) > 0).length,
	 load1: system.load1,
	 load5: system.load5,
	 load15: system.load15,
    embeddedXray: embedded?.endpointCount || 0,
    externalXray: external?.endpointCount || 0,
    tapx: rawPipes,
    rx: report.rates?.rxBytesPerSecond || 0,
    tx: report.rates?.txBytesPerSecond || 0,
    drops: (totals.dropsGuard || 0) + (totals.dropsIO || 0),
    tapxHeap: process.heapAlloc,
    tapxSys: process.heapSys,
    tapxObjects: process.heapObjects,
    tapxGC: process.numGC,
    tapxGCPause: process.lastGCPauseNs,
    tapxObservatory: rawPipes,
    embeddedHeap: embedded?.running ? process.heapAlloc : undefined,
    embeddedSys: embedded?.running ? process.heapSys : undefined,
    embeddedObjects: embedded?.running ? process.heapObjects : undefined,
    embeddedGC: embedded?.running ? process.numGC : undefined,
    embeddedGCPause: embedded?.running ? process.lastGCPauseNs : undefined,
    embeddedObservatory: embedded?.endpointCount,
    externalObservatory: external?.endpointCount,
  };
  return [...current, next].slice(-120);
}

function runtimeStatus(runtime: { running?: boolean; endpointCount?: number } | undefined, t: ReturnType<typeof useI18n>['t']) {
  if (!runtime) return t('dashboard.notConfigured');
  return runtime.running ? t('dashboard.runningPipesStatus', { count: runtime.endpointCount || 0 }) : t('common.stopped');
}
