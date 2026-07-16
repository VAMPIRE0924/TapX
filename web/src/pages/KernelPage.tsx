import { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Button, Card, Col, Collapse, Divider, Form, Input, InputNumber, Radio, Row, Select, Switch, Tag, Tooltip, Upload, message } from 'antd';
import { ApiOutlined, CloudDownloadOutlined, CloudUploadOutlined, CodeOutlined, ReloadOutlined, SaveOutlined } from '@ant-design/icons';
import {
  downloadExternalXray,
  getDashboard,
  getDiagnostics,
  getExternalXrayStatus,
  getGeneratedRuntime,
  getRuntimeConfig,
  applyRuntimeConfig,
  restartRuntimeComponent,
  saveRuntimeConfig,
  uploadExternalXray,
  type RuntimeConfig,
  type DashboardReport,
  type DiagnosticReport,
  type XrayBinaryStatus,
  type UpdateComponent,
} from '../shared/api';
import { errorMessage } from '../shared/localized-error';
import { UnitInputNumber } from '../components/UnitInputNumber';
import { objectToSettings, settingsToObject, stableSettingsSnapshot } from '../shared/settings';
import { formatBytes } from '../shared/format';
import { useI18n } from '../i18n/I18nProvider';
import type { TranslationKey } from '../i18n/dictionaries';
import './KernelPage.css';
import { hashFromPath } from '../app/hash-route';
import { ComponentUpdateDialog } from '../features/updates/ComponentUpdateDialog';

interface KernelSettings extends Record<string, unknown> {
  embeddedXrayEnabled?: boolean;
  tapxEnabled?: boolean;
  runtimeLogLevel?: string;
  tapxStatsInterval?: number;
  externalXrayEnabled?: boolean;
  externalXrayPath?: string;
  externalXrayConfigFile?: string;
  externalXrayWorkDir?: string;
  externalXrayArgs?: string;
  externalXrayDownloadURL?: string;
  externalXrayPinnedVersion?: string;
  externalXrayTargetArch?: string;
  externalXrayChecksum?: string;
  externalXrayDownloadTimeout?: number;
  externalXrayRetryCount?: number;
  externalXrayOverwriteStrategy?: string;
  externalXrayDownloadSource?: 'official' | 'custom';
  externalXrayVersionMode?: 'latest' | 'fixed';
  advancedJSONView?: AdvancedJSONView;
  fullRuntimeJSON?: string;
  xrayGeneratedJSON?: string;
  tapxRuntimeJSON?: string;
  devicesJSON?: string;
  listenersJSON?: string;
  connectorsJSON?: string;
  linkBindingsJSON?: string;
  clientsJSON?: string;
}

type AdvancedJSONView = 'full' | 'xray' | 'tapx' | 'devices' | 'listeners' | 'connectors' | 'links' | 'clients';
type KernelSection = 'kernel' | 'external' | 'advanced';

const generatedJSONKeys: Array<keyof KernelSettings> = [
  'fullRuntimeJSON',
  'xrayGeneratedJSON',
  'tapxRuntimeJSON',
  'devicesJSON',
  'listenersJSON',
  'connectorsJSON',
  'linkBindingsJSON',
  'clientsJSON',
];

const defaults: KernelSettings = {
  embeddedXrayEnabled: true,
  tapxEnabled: true,
  runtimeLogLevel: 'info',
  tapxStatsInterval: 5,
  externalXrayEnabled: false,
  externalXrayPinnedVersion: '',
  externalXrayTargetArch: 'linux-amd64',
  externalXrayDownloadTimeout: 60,
  externalXrayRetryCount: 3,
  externalXrayOverwriteStrategy: 'backup',
  externalXrayDownloadSource: 'official',
  externalXrayVersionMode: 'latest',
  advancedJSONView: 'full',
};

export function KernelPage({ currentPath }: { currentPath: string }) {
  const { t } = useI18n();
  const [config, setConfig] = useState<RuntimeConfig>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [generatedRuntime, setGeneratedRuntime] = useState<unknown>();
  const [generationError, setGenerationError] = useState('');
  const [xrayStatus, setXrayStatus] = useState<XrayBinaryStatus>();
  const [diagnostics, setDiagnostics] = useState<DiagnosticReport>();
  const [dashboard, setDashboard] = useState<DashboardReport>();
  const [xrayAction, setXrayAction] = useState<'status' | 'download' | 'upload' | ''>('');
  const [restartingKernel, setRestartingKernel] = useState<'embedded-xray' | 'tapx' | ''>('');
  const [updateTarget, setUpdateTarget] = useState<UpdateComponent>();
  const [form] = Form.useForm<KernelSettings>();
  const baselineRef = useRef('');
  const [messageApi, messageContextHolder] = message.useMessage();

  const activeSection = kernelSectionFromHash(hashFromPath(currentPath));
  const advancedJSONView = Form.useWatch('advancedJSONView', form) ?? 'full';
  const values = useMemo(() => buildKernelFormValues(config, generatedRuntime, generationError, t('kernel.runtimeNotGenerated')), [config, generatedRuntime, generationError, t]);

  useEffect(() => {
    void refresh();
  }, []);

  async function refresh() {
    setLoading(true);
    try {
      const [next, diagnosticReport, dashboardReport] = await Promise.all([
        getRuntimeConfig(),
        getDiagnostics().catch(() => undefined),
        getDashboard().catch(() => undefined),
      ]);
      let runtime: unknown;
      let runtimeError = '';
      try {
        runtime = await getGeneratedRuntime();
      } catch (err) {
        runtimeError = err instanceof Error ? err.message : t('kernel.generateFailed');
      }
      const nextValues = buildKernelFormValues(next, runtime, runtimeError, t('kernel.runtimeNotGenerated'));
      nextValues.externalXrayTargetArch = diagnosticPlatform(diagnosticReport);
      applyExternalXrayPathDefaults(nextValues, diagnosticReport);
      baselineRef.current = serializeKernelSettings(nextValues);
      setConfig(next);
      setDiagnostics(diagnosticReport);
      setDashboard(dashboardReport);
      setGeneratedRuntime(runtime);
      setGenerationError(runtimeError);
      form.setFieldsValue(nextValues);
      setDirty(false);
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('kernel.loadFailed'));
    } finally {
      setLoading(false);
    }
  }

  async function submit() {
    try {
      await form.validateFields();
    } catch {
      return;
    }
    const formValues = form.getFieldsValue(true);
    setSaving(true);
    try {
      const merged = { ...settingsToObject<KernelSettings>(config.Settings), ...stripGeneratedJSON(formValues) };
      const saved = await saveRuntimeConfig({ ...config, Settings: objectToSettings(merged) });
      let runtime: unknown;
      let runtimeError = '';
      try {
        runtime = await getGeneratedRuntime();
      } catch (err) {
        runtimeError = err instanceof Error ? err.message : t('kernel.generateFailed');
      }
      const savedValues = buildKernelFormValues(saved, runtime, runtimeError, t('kernel.runtimeNotGenerated'));
      savedValues.externalXrayTargetArch = diagnosticPlatform(diagnostics);
      applyExternalXrayPathDefaults(savedValues, diagnostics);
      baselineRef.current = serializeKernelSettings(savedValues);
      setConfig(saved);
      setGeneratedRuntime(runtime);
      setGenerationError(runtimeError);
      form.setFieldsValue(savedValues);
      setDirty(false);
      try {
        await applyRuntimeConfig();
        setDashboard(await getDashboard().catch(() => undefined));
        messageApi.success(t('kernel.savedAndApplied'));
      } catch (err) {
        messageApi.warning(t('kernel.applyFailed', { error: err instanceof Error ? err.message : String(err) }));
      }
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('kernel.saveFailed'));
    } finally {
      setSaving(false);
    }
  }

  async function refreshXrayStatus() {
    const path = String(form.getFieldValue('externalXrayPath') || '').trim();
    if (!path) {
      messageApi.warning(t('kernel.externalPathRequired'));
      return;
    }
    setXrayAction('status');
    try {
      setXrayStatus(await getExternalXrayStatus(path));
    } catch (err) {
      setXrayStatus(undefined);
      messageApi.error(errorMessage(err, t, 'kernel.statusFailed'));
    } finally {
      setXrayAction('');
    }
  }

  async function downloadXray() {
    const values = form.getFieldsValue([
      'externalXrayDownloadSource',
      'externalXrayVersionMode',
      'externalXrayPinnedVersion',
      'externalXrayTargetArch',
      'externalXrayDownloadURL',
      'externalXrayPath',
      'externalXrayChecksum',
      'externalXrayDownloadTimeout',
      'externalXrayRetryCount',
      'externalXrayOverwriteStrategy',
    ]);
    const source = values.externalXrayDownloadSource || 'official';
    if (source === 'official' && values.externalXrayVersionMode === 'fixed' && !String(values.externalXrayPinnedVersion || '').trim()) {
      messageApi.warning(t('kernel.versionRequired'));
      return;
    }
    const url = source === 'official'
      ? officialXrayDownloadURL(
          String(values.externalXrayTargetArch || '').trim(),
          values.externalXrayVersionMode === 'fixed' ? String(values.externalXrayPinnedVersion || '').trim() : '',
        )
      : String(values.externalXrayDownloadURL || '').trim();
    if (!url) {
      messageApi.warning(t('kernel.downloadUrlRequired'));
      return;
    }
    const path = String(values.externalXrayPath || '').trim();
    if (!path) {
      messageApi.warning(t('kernel.externalPathRequired'));
      return;
    }
    setXrayAction('download');
    try {
      const status = await downloadExternalXray({
        url,
        path,
        sha256: String(values.externalXrayChecksum || ''),
        timeoutSecond: Number(values.externalXrayDownloadTimeout || 0),
        retryCount: Number(values.externalXrayRetryCount || 0),
        overwriteStrategy: values.externalXrayOverwriteStrategy as 'backup' | 'overwrite' | 'skip',
      });
      setXrayStatus(status);
      messageApi.success(t('kernel.downloaded'));
    } catch (err) {
      messageApi.error(errorMessage(err, t, 'kernel.downloadFailed'));
    } finally {
      setXrayAction('');
    }
  }

  async function uploadXray(file: File) {
    const path = String(form.getFieldValue('externalXrayPath') || '').trim();
    if (!path) {
      messageApi.warning(t('kernel.externalPathRequired'));
      return;
    }
    setXrayAction('upload');
    try {
      const status = await uploadExternalXray(file, path);
      setXrayStatus(status);
      messageApi.success(t('kernel.uploaded'));
    } catch (err) {
      messageApi.error(errorMessage(err, t, 'kernel.uploadFailed'));
    } finally {
      setXrayAction('');
    }
  }

  async function restartBuiltInKernel(kernel: 'embedded-xray' | 'tapx') {
    if (dirty) {
      messageApi.warning(t('kernel.saveBeforeRestart'));
      return;
    }
    setRestartingKernel(kernel);
    try {
      await restartRuntimeComponent(kernel);
      setDashboard(await getDashboard().catch(() => undefined));
      messageApi.success(t('kernel.restartSucceeded', { kernel: kernel === 'tapx' ? 'TapX' : t('kernel.embeddedXrayTitle') }));
    } catch (err) {
      messageApi.error(t('kernel.restartFailed', { error: err instanceof Error ? err.message : String(err) }));
    } finally {
      setRestartingKernel('');
    }
  }

  return (
    <div className="kernel-page">
      {messageContextHolder}
      <Form
        form={form}
        colon={false}
        labelCol={{ sm: { span: 7 } }}
        wrapperCol={{ sm: { span: 14 } }}
        labelWrap
        initialValues={values}
        onValuesChange={() => {
          setDirty(serializeKernelSettings(form.getFieldsValue(true)) !== baselineRef.current);
        }}
      >
        <div className="kernel-actions-toolbar">
          <KernelActions saving={saving} dirty={dirty} onSave={submit} />
        </div>
        <div className="kernel-content">
          {loading ? <Card loading className="kernel-loading-card" /> : null}
          {!loading && activeSection === 'kernel' ? (
            <BuiltInKernelSettings
              dashboard={dashboard}
              dirty={dirty}
              restarting={restartingKernel}
              onRestart={(kernel) => void restartBuiltInKernel(kernel)}
              onUpdate={() => setUpdateTarget('tapx')}
            />
          ) : null}
          {!loading && activeSection === 'external' ? (
            <ExternalKernelSettings
              status={xrayStatus}
              diagnostics={diagnostics}
              action={xrayAction}
              onStatus={() => void refreshXrayStatus()}
              onDownload={() => void downloadXray()}
              onUpload={(file) => void uploadXray(file)}
            />
          ) : null}
          {!loading && activeSection === 'advanced' ? <Card><AdvancedKernelFields activeView={advancedJSONView} /></Card> : null}
        </div>
      </Form>
      {updateTarget ? (
        <ComponentUpdateDialog
          open
          component={updateTarget}
          onClose={() => setUpdateTarget(undefined)}
          onUpdated={() => void refresh()}
        />
      ) : null}
    </div>
  );
}

function kernelSectionFromHash(hash: string): KernelSection {
  if (hash === '#external') return 'external';
  if (hash === '#advanced') return 'advanced';
  return 'kernel';
}

function KernelActions({ saving, dirty, onSave }: { saving: boolean; dirty: boolean; onSave: () => void }) {
  const { t } = useI18n();
  return (
    <div className="kernel-actions">
      <Button type="primary" icon={<SaveOutlined />} loading={saving} disabled={!dirty} onClick={onSave}>{t('common.save')}</Button>
    </div>
  );
}

function BuiltInKernelSettings({ dashboard, dirty, restarting, onRestart, onUpdate }: {
  dashboard?: DashboardReport;
  dirty: boolean;
  restarting: 'embedded-xray' | 'tapx' | '';
  onRestart: (kernel: 'embedded-xray' | 'tapx') => void;
  onUpdate: () => void;
}) {
  const { t } = useI18n();
  const form = Form.useFormInstance<KernelSettings>();
  const embeddedXrayEnabled = Form.useWatch('embeddedXrayEnabled', form) !== false;
  const tapxEnabled = Form.useWatch('tapxEnabled', form) !== false;
  const runtime = dashboard?.runtime;
  const embeddedXray = runtime?.xrayRuntimes?.find((item) => item.runtime === 'embedded');
  const tapxRunning = runtime?.running === true;
  return (
    <div className="kernel-section-stack">
      <Card className="kernel-runtime-card" title={t('kernel.kernelManagement')}>
        <div className="kernel-engine-list">
          <section className="kernel-engine-row">
            <div className="kernel-engine-identity">
              <span className="kernel-engine-icon"><CodeOutlined /></span>
              <strong>{t('kernel.embeddedXrayTitle')}</strong>
              <KernelStateTag enabled={embeddedXrayEnabled} running={embeddedXray?.running === true} />
            </div>
            <div className="kernel-engine-actions">
              <label className="kernel-engine-switch">
                <Tooltip title={t('kernel.embeddedXrayEnabledHelp')}><span>{t('common.enabled')}</span></Tooltip>
                <Form.Item name="embeddedXrayEnabled" valuePropName="checked" noStyle><Switch /></Form.Item>
              </label>
              <Button
                icon={<ReloadOutlined />}
                loading={restarting === 'embedded-xray'}
                disabled={Boolean(restarting) || dirty || !embeddedXrayEnabled}
                onClick={() => onRestart('embedded-xray')}
              >
                {t('common.restart')}
              </Button>
              <Button icon={<CloudDownloadOutlined />} onClick={onUpdate}>{t('common.update')}</Button>
            </div>
          </section>
          {embeddedXray?.lastError ? <div className="kernel-engine-error">{embeddedXray.lastError}</div> : null}

          <section className="kernel-engine-row">
            <div className="kernel-engine-identity">
              <span className="kernel-engine-icon"><ApiOutlined /></span>
              <strong>TapX</strong>
              <KernelStateTag enabled={tapxEnabled} running={tapxRunning} />
            </div>
            <div className="kernel-engine-actions">
              <label className="kernel-engine-switch">
                <Tooltip title={t('kernel.tapxEnabledHelp')}><span>{t('common.enabled')}</span></Tooltip>
                <Form.Item name="tapxEnabled" valuePropName="checked" noStyle><Switch /></Form.Item>
              </label>
              <Button
                icon={<ReloadOutlined />}
                loading={restarting === 'tapx'}
                disabled={Boolean(restarting) || dirty || !tapxEnabled}
                onClick={() => onRestart('tapx')}
              >
                {t('common.restart')}
              </Button>
              <Button icon={<CloudDownloadOutlined />} onClick={onUpdate}>{t('common.update')}</Button>
            </div>
          </section>
        </div>
      </Card>

      <Card className="kernel-options-card" title={t('kernel.runtimeOptions')}>
        <div className="kernel-options-grid">
          <section className="kernel-option-group">
            <div className="kernel-option-title"><CodeOutlined />{t('kernel.embeddedXrayTitle')}</div>
            <Form.Item name="runtimeLogLevel" label={t('kernel.logLevel')} tooltip={t('kernel.logLevelHelp')}>
              <Select options={[
                { value: 'debug', label: 'debug' },
                { value: 'info', label: 'info' },
                { value: 'warn', label: 'warning' },
                { value: 'error', label: 'error' },
                { value: '', label: 'none' },
              ]} />
            </Form.Item>
          </section>
          <section className="kernel-option-group">
            <div className="kernel-option-title"><ApiOutlined />TapX</div>
            <Form.Item name="tapxStatsInterval" label={t('kernel.statsInterval')} tooltip={t('kernel.statsIntervalHelp')}>
              <UnitInputNumber min={1} unit="s" />
            </Form.Item>
          </section>
        </div>
      </Card>
    </div>
  );
}

function KernelStateTag({ enabled, running }: { enabled: boolean; running: boolean }) {
  const { t } = useI18n();
  if (!enabled) return <Tag>{t('common.disabled')}</Tag>;
  if (running) return <Tag color="green">{t('common.running')}</Tag>;
  return <Tag color="orange">{t('kernel.standby')}</Tag>;
}

function ExternalKernelSettings({ status, diagnostics, action, onStatus, onDownload, onUpload }: {
  status?: XrayBinaryStatus;
  diagnostics?: DiagnosticReport;
  action: 'status' | 'download' | 'upload' | '';
  onStatus: () => void;
  onDownload: () => void;
  onUpload: (file: File) => void;
}) {
  const { t } = useI18n();
  const form = Form.useFormInstance<KernelSettings>();
  const source = Form.useWatch('externalXrayDownloadSource', form) || 'official';
  const versionMode = Form.useWatch('externalXrayVersionMode', form) || 'latest';
  const platform = diagnosticPlatform(diagnostics);
  const officialSupported = Boolean(officialXrayAsset(platform));
  return (
    <div className="kernel-section-stack">
      <Card className="kernel-settings-card" title={t('kernel.externalRuntimeTitle')}>
        <Row gutter={[20, 0]}>
          <Col xs={24} xl={12}>
            <Form.Item name="externalXrayEnabled" label={t('kernel.enableExternalXray')} tooltip={t('kernel.enableExternalHelp')} valuePropName="checked"><Switch /></Form.Item>
            <Form.Item name="externalXrayPath" label={t('kernel.externalPath')} tooltip={t('kernel.externalPathHelp')}><Input placeholder="/usr/local/bin/xray" /></Form.Item>
            <Form.Item name="externalXrayConfigFile" label={t('kernel.configFile')} tooltip={t('kernel.configFileHelp')}><Input placeholder="/usr/local/etc/tapx/xray/generated.json" /></Form.Item>
          </Col>
          <Col xs={24} xl={12}>
            <Form.Item name="externalXrayWorkDir" label={t('kernel.workDir')} tooltip={t('kernel.workDirHelp')}><Input placeholder="/usr/local/etc/tapx/xray" /></Form.Item>
            <Form.Item name="externalXrayArgs" label={t('kernel.args')} tooltip={t('kernel.argsHelp')}><Input.TextArea rows={3} placeholder={'run\n-config\n{config}'} /></Form.Item>
          </Col>
        </Row>
        <div className="kernel-status-row">
          <Button icon={<ReloadOutlined />} loading={action === 'status'} onClick={onStatus}>{t('kernel.check')}</Button>
          {status ? <><Tag color={status.exists && status.executable ? 'green' : status.exists ? 'orange' : 'default'}>{status.exists ? status.executable ? t('kernel.executable') : t('kernel.notExecutable') : t('kernel.notFound')}</Tag><span className="kernel-status-path">{status.path}</span>{status.exists ? <span className="kernel-muted">{formatBytes(status.size)} · {status.mode || '-'}</span> : null}</> : <span className="kernel-muted">{t('kernel.notChecked')}</span>}
        </div>
      </Card>
      <Card className="kernel-settings-card" title={t('kernel.installExternalXray')}>
        <Row gutter={[20, 0]}>
          <Col xs={24} xl={12}>
            <Form.Item name="externalXrayDownloadSource" label={t('kernel.downloadSource')} tooltip={t('kernel.officialUpstreamNotice')}>
              <Radio.Group buttonStyle="solid">
                <Radio.Button value="official" disabled={!officialSupported}>{t('kernel.officialXray')}</Radio.Button>
                <Radio.Button value="custom">{t('kernel.customUrl')}</Radio.Button>
              </Radio.Group>
            </Form.Item>
            <Form.Item label={t('kernel.targetArchitecture')} tooltip={t('kernel.targetArchitectureHelp')}><Input value={platform} readOnly suffix={<Tag color="blue">{t('kernel.autoDetected')}</Tag>} /></Form.Item>
            {source === 'official' ? <>
              <Form.Item name="externalXrayVersionMode" label={t('kernel.version')} tooltip={t('kernel.versionHelp')}><Radio.Group><Radio value="latest">{t('kernel.latestVersion')}</Radio><Radio value="fixed">{t('kernel.specifiedVersion')}</Radio></Radio.Group></Form.Item>
              {versionMode === 'fixed' ? <Form.Item name="externalXrayPinnedVersion" label={t('kernel.pinnedVersion')} rules={[{ required: true, message: t('kernel.versionRequired') }]}><Input placeholder="v26.3.27" /></Form.Item> : null}
            </> : <Form.Item name="externalXrayDownloadURL" label={t('kernel.downloadUrl')} tooltip={t('kernel.downloadUrlHelp')} rules={[{ required: true, message: t('kernel.downloadUrlRequired') }, { type: 'url', message: t('kernel.invalidDownloadUrl') }]}><Input placeholder="https://example.com/Xray-linux-64.zip" /></Form.Item>}
            <Collapse ghost className="kernel-download-advanced" items={[{ key: 'download-advanced', label: t('kernel.advancedDownloadOptions'), children: <>
              <Form.Item name="externalXrayChecksum" label="SHA256" tooltip={t('kernel.checksumHelp')}><Input placeholder="0123456789abcdef..." /></Form.Item>
              <Row gutter={12}><Col span={12}><Form.Item name="externalXrayDownloadTimeout" label={t('kernel.downloadTimeout')} tooltip={t('kernel.downloadTimeoutHelp')}><UnitInputNumber min={1} unit="s" placeholder="60" /></Form.Item></Col><Col span={12}><Form.Item name="externalXrayRetryCount" label={t('kernel.retryCount')} tooltip={t('kernel.retryCountHelp')}><InputNumber min={0} max={20} placeholder="3" /></Form.Item></Col></Row>
              <Form.Item name="externalXrayOverwriteStrategy" label={t('kernel.overwriteStrategy')} tooltip={t('kernel.overwriteStrategyHelp')}><Select options={[{ value: 'backup', label: t('kernel.overwriteBackup') }, { value: 'overwrite', label: t('kernel.overwriteDirect') }, { value: 'skip', label: t('kernel.overwriteSkip') }]} /></Form.Item>
            </> }]} />
          </Col>
        </Row>
        {!officialSupported ? <Alert type="warning" showIcon className="kernel-platform-warning" title={t('kernel.unsupportedOfficialPlatform')} /> : null}
        <div className="kernel-install-actions">
          <Button type="primary" icon={<CloudDownloadOutlined />} loading={action === 'download'} onClick={onDownload}>{t('kernel.downloadAndInstall')}</Button>
          <Upload accept=".zip,.exe,application/zip,application/octet-stream" showUploadList={false} beforeUpload={(file) => { onUpload(file); return Upload.LIST_IGNORE; }}><Button icon={<CloudUploadOutlined />} loading={action === 'upload'}>{t('kernel.uploadLocal')}</Button></Upload>
        </div>
      </Card>
    </div>
  );
}

function AdvancedKernelFields({ activeView }: { activeView: AdvancedJSONView }) {
  const { t } = useI18n();
  const jsonField = jsonFieldByView[activeView];
  return (
    <>
      <Divider>{t('kernel.advancedTitle')}</Divider>
      <Form.Item name="advancedJSONView" label={t('kernel.configView')} tooltip={t('kernel.advancedMessage')}>
        <Radio.Group buttonStyle="solid">
          <Radio.Button value="full">{t('kernel.viewFull')}</Radio.Button>
          <Radio.Button value="xray">{t('kernel.viewXray')}</Radio.Button>
          <Radio.Button value="tapx">{t('kernel.viewTapx')}</Radio.Button>
          <Radio.Button value="devices">{t('kernel.viewDevices')}</Radio.Button>
          <Radio.Button value="listeners">{t('kernel.viewListeners')}</Radio.Button>
          <Radio.Button value="connectors">{t('kernel.viewConnectors')}</Radio.Button>
          <Radio.Button value="links">{t('kernel.viewLinks')}</Radio.Button>
          <Radio.Button value="clients">{t('kernel.viewClients')}</Radio.Button>
        </Radio.Group>
      </Form.Item>
      <Form.Item key={activeView} name={jsonField} label={t(advancedJSONLabelKeys[activeView])} tooltip={t('kernel.generatedConfigHelp')}>
        <Input.TextArea rows={18} spellCheck={false} readOnly />
      </Form.Item>
    </>
  );
}

function buildKernelFormValues(config: RuntimeConfig, generatedRuntime?: unknown, generationError = '', runtimeNotGenerated = ''): KernelSettings {
  const stored = settingsToObject<KernelSettings>(config.Settings);
  return {
    ...defaults,
    ...stored,
    ...buildGeneratedJSON(config, generatedRuntime, generationError, runtimeNotGenerated),
  };
}

export function diagnosticPlatform(report?: DiagnosticReport): string {
  const goos = report?.process?.goos?.trim() || 'linux';
  const goarch = report?.process?.goarch?.trim() || 'amd64';
  return `${goos}-${goarch}`;
}

function applyExternalXrayPathDefaults(values: KernelSettings, report?: DiagnosticReport) {
  const isWindows = report?.process?.goos === 'windows';
  if (!values.externalXrayPath) values.externalXrayPath = isWindows ? 'C:\\ProgramData\\TapX\\xray.exe' : '/usr/local/bin/xray';
  if (!values.externalXrayConfigFile) values.externalXrayConfigFile = isWindows ? 'C:\\ProgramData\\TapX\\xray.json' : '/usr/local/etc/tapx/xray/generated.json';
  if (!values.externalXrayWorkDir) values.externalXrayWorkDir = isWindows ? 'C:\\ProgramData\\TapX' : '/usr/local/etc/tapx/xray';
}

export function officialXrayAsset(platform: string): string {
  const assets: Record<string, string> = {
    'linux-amd64': 'Xray-linux-64.zip',
    'linux-386': 'Xray-linux-32.zip',
    'linux-arm64': 'Xray-linux-arm64-v8a.zip',
    'linux-arm': 'Xray-linux-arm32-v7a.zip',
    'linux-loong64': 'Xray-linux-loong64.zip',
    'linux-mips': 'Xray-linux-mips32.zip',
    'linux-mipsle': 'Xray-linux-mips32le.zip',
    'linux-mips64': 'Xray-linux-mips64.zip',
    'linux-mips64le': 'Xray-linux-mips64le.zip',
    'linux-ppc64': 'Xray-linux-ppc64.zip',
    'linux-ppc64le': 'Xray-linux-ppc64le.zip',
    'linux-riscv64': 'Xray-linux-riscv64.zip',
    'linux-s390x': 'Xray-linux-s390x.zip',
    'windows-amd64': 'Xray-windows-64.zip',
    'windows-386': 'Xray-windows-32.zip',
    'windows-arm64': 'Xray-windows-arm64-v8a.zip',
    'darwin-amd64': 'Xray-macos-64.zip',
    'darwin-arm64': 'Xray-macos-arm64-v8a.zip',
  };
  return assets[platform] || '';
}

export function officialXrayDownloadURL(platform: string, version = ''): string {
  const asset = officialXrayAsset(platform);
  if (!asset) return '';
  const normalizedVersion = version.trim();
  if (!normalizedVersion) return `https://github.com/XTLS/Xray-core/releases/latest/download/${asset}`;
  const tag = normalizedVersion.startsWith('v') ? normalizedVersion : `v${normalizedVersion}`;
  return `https://github.com/XTLS/Xray-core/releases/download/${encodeURIComponent(tag)}/${asset}`;
}

function buildGeneratedJSON(config: RuntimeConfig, generatedRuntime?: unknown, generationError = '', runtimeNotGenerated = ''): Pick<KernelSettings, typeof generatedJSONKeys[number]> {
  const source = {
    Devices: config.Devices || [],
    Listeners: config.Listeners || [],
    Connectors: config.Connectors || [],
    Clients: config.Clients || [],
    Routes: config.Routes || [],
    VKeys: config.VKeys || [],
    Addresses: config.Addresses || [],
    XrayProfiles: config.XrayProfiles || [],
    Settings: config.Settings || [],
  };
  const runtime = generatedRuntime && typeof generatedRuntime === 'object'
    ? generatedRuntime as Record<string, unknown>
    : undefined;
  const runtimeDocument = runtime || { Error: generationError || runtimeNotGenerated, SourceConfig: source };
  return {
    fullRuntimeJSON: stringify(runtimeDocument),
    xrayGeneratedJSON: stringify(runtime ? { XrayProfiles: runtime.XrayProfiles || [], XrayPipes: runtime.XrayPipes || [] } : runtimeDocument),
    tapxRuntimeJSON: stringify(runtime ? {
      Devices: runtime.Devices || [],
      Listeners: runtime.Listeners || [],
      Connectors: runtime.Connectors || [],
      Routes: runtime.Routes || [],
      UDPPipes: runtime.UDPPipes || [],
      TCPPipes: runtime.TCPPipes || [],
    } : runtimeDocument),
    devicesJSON: stringify(config.Devices || []),
    listenersJSON: stringify(config.Listeners || []),
    connectorsJSON: stringify(config.Connectors || []),
    linkBindingsJSON: stringify(config.Routes || []),
    clientsJSON: stringify(config.Clients || []),
  };
}

function serializeKernelSettings(values: KernelSettings): string {
  return stableSettingsSnapshot(stripGeneratedJSON(values));
}

function stripGeneratedJSON(values: KernelSettings): KernelSettings {
  const next = { ...values };
  for (const key of generatedJSONKeys) {
    delete next[key];
  }
  return next;
}

function stringify(value: unknown): string {
  return JSON.stringify(value, null, 2);
}

const jsonFieldByView: Record<AdvancedJSONView, keyof KernelSettings> = {
  full: 'fullRuntimeJSON',
  xray: 'xrayGeneratedJSON',
  tapx: 'tapxRuntimeJSON',
  devices: 'devicesJSON',
  listeners: 'listenersJSON',
  connectors: 'connectorsJSON',
  links: 'linkBindingsJSON',
  clients: 'clientsJSON',
};

const advancedJSONLabelKeys: Record<AdvancedJSONView, TranslationKey> = {
  full: 'kernel.viewFull',
  xray: 'kernel.viewXray',
  tapx: 'kernel.viewTapx',
  devices: 'kernel.viewDevices',
  listeners: 'kernel.viewListeners',
  connectors: 'kernel.viewConnectors',
  links: 'kernel.viewLinks',
  clients: 'kernel.viewClients',
};
