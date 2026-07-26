import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Dropdown,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Radio,
  Row,
  Col,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  Tooltip,
  message,
  type MenuProps,
  type TableColumnsType,
} from 'antd';
import {
  ApiOutlined,
  ArrowDownOutlined,
  ArrowUpOutlined,
  CloudOutlined,
  CheckCircleOutlined,
  DashboardOutlined,
  DeleteOutlined,
  EditOutlined,
  ExportOutlined,
  ImportOutlined,
  MoreOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  RetweetOutlined,
  StopOutlined,
  VerticalAlignTopOutlined,
} from '@ant-design/icons';
import {
  resetConnectorTraffic,
  testConnector,
  type ConnectorTestKind,
  type ConnectorTestResult,
  type RuntimeConfig,
  type TapxDevice,
  type TapxEndpoint,
  type TapxVKey,
  type TapxXrayProfile,
} from '../shared/api';
import {
  applyManagedRuntimeConfig as applyRuntimeConfig,
  defaultTargetNodeID,
  filterNodeOwned,
  getManagedStats as getStats,
  getManagedRuntimeConfig as getRuntimeConfig,
  nodeIDOf,
  nodeObjectKey,
  saveManagedRuntimeConfig as saveRuntimeConfig,
  sameNodeObject,
  type NodeOwned,
} from '../features/nodes/managedConfig';
import { NodeScopeSelect, NodeSourceTag, useNodeScope, useNodeTargetOptions } from '../features/nodes/NodeScope';
import { errorMessage } from '../shared/localized-error';
import { copyText } from '../shared/clipboard';
import { unixSeconds as nowSecond } from '../shared/time';
import { parseObjectJSON } from '../shared/json';
import { removeUnusedXrayProfiles, upsertXrayProfile } from '../shared/xray-profiles';
import { booleanValue, numberValue, stringValue } from '../shared/values';
import { safeID as safeId, uniqueID as uniqueId } from '../shared/ids';
import { isManagedLinkAddressRemark } from '../shared/managed-objects';
import { clearBindingReferences, relationKey } from '../shared/config-relations';
import { TapxConnectorDtlsFields, TapxConnectorFastPathFields, TapxConnectorTlsFields } from '../features/endpoints/TapxEndpointFields';
import { EndpointBindingFields } from '../features/endpoints/EndpointBindingFields';
import { runtimeModeChangesTransportFamily, tapxProtocolOptions, type AddressAssignMode, type DeviceBindMode, type EndpointDeviceBinding, type EndpointRuntimeMode, type InterfaceType } from '../features/endpoints/endpoint-types';
import { resolveTcpLengthMode } from '../features/endpoints/tcpLengthMode';
import { DeviceTypeConflictError, deviceTypeConflictValues, hydrateSavedDeviceBinding, materializeEndpointAutoDevice, normalizeDeviceBinding } from '../features/endpoints/deviceBinding';
import { materializeConnectorVKey } from '../features/endpoints/vkeyBinding';
import { connectorIDConflicts, mergeConnectorJson } from '../features/endpoints/connectorJson';
import { parseRawConnectorLink } from '../features/endpoints/rawConnectorLink';
import { NordModal } from '../features/integrations/NordModal';
import { WarpModal } from '../features/integrations/WarpModal';
import type { WireguardIntegrationDraft } from '../features/integrations/types';
import { defaultOutboundSettings, defaultTapxConnectorFields } from '../features/xray/outbounds/defaults';
import { OutboundLinkParseError, parseOutboundLink } from '../features/xray/outbounds/linkParser';
import {
  outboundSettingsFromWire,
  outboundSettingsToWire,
  outboundStreamFromWire,
  outboundStreamToWire,
} from '../features/xray/outbounds/profileAdapter';
import { SniffingFields } from '../features/xray/shared/SniffingFields';
import { FinalMaskFields } from '../features/xray/transport/FinalMaskFields';
import { SockoptFields } from '../features/xray/transport/SockoptFields';
import {
  XrayMuxFields,
  XrayOutboundProtocolFields,
  XraySecurityFields,
  XrayTransportFields,
  canEnableStream,
  canEnableTlsFlow,
  newStreamSlice,
  outboundXrayProtocolOptions,
  targetStrategyOptions,
} from '../features/xray/XrayFormFields';
import { formatBytes } from '../shared/format';
import { labelDevice, nextId } from '../shared/tapx-model';
import { settingsToObject } from '../shared/settings';
import { useI18n } from '../i18n/I18nProvider';
import './ConnectorPage.css';

type RuntimeMode = EndpointRuntimeMode;
type TestKind = ConnectorTestKind;

type ConnectorBinding = EndpointDeviceBinding;

type ConnectorRecord = TapxEndpoint & NodeOwned & {
  RuntimeMode?: RuntimeMode;
  Protocol?: string;
  Network?: string;
  Security?: 'none' | 'tls' | 'reality' | 'dtls' | string;
  VKey?: string;
  SendThrough?: string;
  DomainStrategy?: string;
  settings?: Record<string, unknown>;
  streamSettings?: Record<string, unknown>;
  mux?: Record<string, unknown>;
  TLS?: Record<string, unknown>;
  Binding?: ConnectorBinding;
  JSONText?: string;
  ImportLink?: string;
  LastDelayMs?: number;
  LastTestKind?: TestKind;
  LastTestConfirmed?: boolean;
  LastTestMessage?: string;
  LastTestResult?: ConnectorTestResult;
  CreatedAt?: number;
  UpdatedAt?: number;
};

type ConnectorStats = {
  rxBytes: number;
  txBytes: number;
};

type ThroughputDirection = 'upload' | 'download';

type ThroughputSample = {
  at: number;
  direction: ThroughputDirection;
  bps: number;
};

type ExportModalState = {
  open: boolean;
  title: string;
  value: string;
};

const defaultConnector: ConnectorRecord = {
  ID: '',
  Enabled: true,
  Name: '',
  Remote: '',
  Port: 443,
  Transport: 'xray',
  RuntimeMode: 'embedded-xray',
  Protocol: 'vless',
  Network: 'tcp',
  Security: 'none',
  settings: defaultOutboundSettings('vless'),
  streamSettings: newStreamSlice('tcp'),
  Binding: {
    DeviceBindingEnabled: true,
    DeviceBindMode: 'autoCreate',
    AutoCreateDevice: true,
    InterfaceType: 'tun',
    DeviceName: 'tapx-tun0',
    AddressConfigEnabled: false,
    AddressAssignMode: 'manual',
    IPv4CIDR: '10.10.0.1/24',
    MTU: 1500,
  },
};

export function ConnectorPage() {
  const { t } = useI18n();
  const [config, setConfig] = useState<RuntimeConfig>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<ConnectorRecord | null>(null);
  const [selectedRowKeys, setSelectedRowKeys] = useState<string[]>([]);
  const [diagnosticOpen, setDiagnosticOpen] = useState(false);
  const [diagnosticTarget, setDiagnosticTarget] = useState<ConnectorRecord | null>(null);
  const [diagnosticResults, setDiagnosticResults] = useState<Partial<Record<TestKind, ConnectorTestResult>>>({});
  const [diagnosticLoading, setDiagnosticLoading] = useState<TestKind | null>(null);
  const [throughputDuration, setThroughputDuration] = useState(10);
  const [throughputContinuous, setThroughputContinuous] = useState(false);
  const [throughputDirection, setThroughputDirection] = useState<ThroughputDirection>('download');
  const [throughputGaugeLimit, setThroughputGaugeLimit] = useState(0);
  const [throughputPhase, setThroughputPhase] = useState<ThroughputDirection>('download');
  const [throughputElapsedMs, setThroughputElapsedMs] = useState(0);
  const [throughputSamples, setThroughputSamples] = useState<ThroughputSample[]>([]);
  const throughputAbortRef = useRef<AbortController | null>(null);
  const [exportModal, setExportModal] = useState<ExportModalState>({ open: false, title: '', value: '' });
  const [importOpen, setImportOpen] = useState(false);
  const [importText, setImportText] = useState('');
  const [importTargetNodeID, setImportTargetNodeID] = useState('local');
  const [warpOpen, setWarpOpen] = useState(false);
  const [nordOpen, setNordOpen] = useState(false);
  const [integrationTargetNodeID, setIntegrationTargetNodeID] = useState('local');
  const [activeFormTab, setActiveFormTab] = useState('basic');
  const [jsonDirty, setJsonDirty] = useState(false);
  const [connectorStats, setConnectorStats] = useState<Record<string, ConnectorStats>>({});
  const [activeConnectorKeys, setActiveConnectorKeys] = useState<Set<string>>(() => new Set());
  const [messageApi, messageContextHolder] = message.useMessage();
  const [form] = Form.useForm<ConnectorRecord>();
  const { nodes, scope, setScope } = useNodeScope();
  const nodeTargetOptions = useNodeTargetOptions(nodes);
  const runtimeOptions = useMemo(() => [
    { value: 'embedded-xray', label: t('listener.embeddedXray') },
    { value: 'external-xray', label: t('listener.externalXray') },
    { value: 'tapx', label: 'TapX' },
  ], [t]);
  const externalXrayReady = useMemo(() => {
    const stored = settingsToObject<{ externalXrayEnabled?: boolean; externalXrayPath?: string }>(config.Settings);
    return stored.externalXrayEnabled === true && Boolean(stored.externalXrayPath);
  }, [config.Settings]);

  useEffect(() => () => throughputAbortRef.current?.abort(), []);

  const connectors = useMemo(() => ((config.Connectors || []) as ConnectorRecord[]), [config.Connectors]);
  const visibleConnectors = useMemo(() => filterNodeOwned(connectors, scope), [connectors, scope]);
  const selectedConnectors = useMemo(
    () => connectors.filter((item) => selectedRowKeys.includes(nodeObjectKey(item))),
    [connectors, selectedRowKeys],
  );
  useEffect(() => {
    const visibleKeys = new Set(visibleConnectors.map(nodeObjectKey));
    setSelectedRowKeys((current) => current.filter((key) => visibleKeys.has(key)));
  }, [visibleConnectors]);
  const devices = useMemo(() => ((config.Devices || []) as TapxDevice[]), [config.Devices]);
  const warpConnector = useMemo(() => connectors.find((item) => (
    item.Name === 'warp' && nodeIDOf(item) === integrationTargetNodeID
  )), [connectors, integrationTargetNodeID]);
  const nordConnector = useMemo(() => connectors.find((item) => (
    item.Name?.startsWith('nord-') && nodeIDOf(item) === integrationTargetNodeID
  )), [connectors, integrationTargetNodeID]);

  const runtimeMode = (Form.useWatch('RuntimeMode', form) || defaultConnector.RuntimeMode) as RuntimeMode;
  const protocol = String(Form.useWatch('Protocol', form) || defaultConnector.Protocol);
  const tapxSecurity = String(Form.useWatch('Security', form) || 'none');
  const bindMode = (Form.useWatch(['Binding', 'DeviceBindMode'], form) || 'autoCreate') as DeviceBindMode;
  const deviceBindingEnabled = Form.useWatch(['Binding', 'DeviceBindingEnabled'], form) !== false;
  const linkAutoOptimize = Form.useWatch(['Binding', 'LinkAutoOptimize'], form) === true;
  const interfaceType = (Form.useWatch(['Binding', 'InterfaceType'], form) || 'tun') as InterfaceType;
  const addressConfigEnabled = Form.useWatch(['Binding', 'AddressConfigEnabled'], form) === true;
  const addressAssignMode = (Form.useWatch(['Binding', 'AddressAssignMode'], form) || 'auto') as AddressAssignMode;
  const watchedStreamNetwork = Form.useWatch(['streamSettings', 'network'], { form, preserve: true });
  const streamNetwork = String(watchedStreamNetwork ?? (canEnableStream(protocol) && protocol !== 'wireguard' ? 'tcp' : ''));
  const streamSecurity = String(Form.useWatch(['streamSettings', 'security'], { form, preserve: true }) || 'none');
  const encryption = String(Form.useWatch(['settings', 'encryption'], form) || '');
  const reverseTag = String(Form.useWatch(['settings', 'reverseTag'], form) || '');
  const flow = String(Form.useWatch(['settings', 'flow'], form) || '');
  const targetNodeID = String(Form.useWatch('ManagedNodeID', form) || defaultTargetNodeID(scope));
  const outboundTags = useMemo(
    () => Array.from(new Set(filterNodeOwned(connectors, targetNodeID).map((item) => item.Name || item.ID).filter(Boolean))),
    [connectors, targetNodeID],
  );

  const streamEnabled = runtimeMode !== 'tapx' && canEnableStream(protocol);
  const tlsFlowAllowed = runtimeMode !== 'tapx' && canEnableTlsFlow(protocol, streamNetwork, streamSecurity, encryption);
  const isTapxRawTcp = runtimeMode === 'tapx' && protocol === 'raw-tcp';
  const isTapxRawUdp = runtimeMode === 'tapx' && protocol === 'raw-udp';

  const deviceOptions = useMemo(() => filterNodeOwned(devices as Array<TapxDevice & NodeOwned>, targetNodeID)
    .filter((device) => device.Enabled !== false && (!interfaceType || device.Type === interfaceType))
    .map((device) => ({ value: device.ID, label: labelDevice(device) })), [devices, interfaceType, targetNodeID]);

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    let cancelled = false;
    let timer: number | undefined;
    const poll = async () => {
      try {
        const report = await getStats();
        if (cancelled) return;
        const next: Record<string, ConnectorStats> = {};
        const active = new Set<string>();
        for (const bucket of report.byEndpoint || []) {
          if (bucket.kind !== 'connector' && !bucket.id.startsWith('connector:')) continue;
          const id = bucket.name || bucket.id.replace(/^connector:/, '');
          const key = `${nodeIDOf(bucket)}:${id}`;
          if ((bucket.activePipes ?? bucket.pipes ?? 0) > 0) active.add(key);
          next[key] = {
            rxBytes: Number(bucket.counters?.rxBytes || 0),
            txBytes: Number(bucket.counters?.txBytes || 0),
          };
        }
        setConnectorStats(next);
        setActiveConnectorKeys(active);
      } catch {
        // Preserve the last valid counters while the runtime is restarting.
      } finally {
        if (!cancelled) timer = window.setTimeout(poll, 2000);
      }
    };
    void poll();
    return () => {
      cancelled = true;
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }, []);

  async function refresh() {
    setLoading(true);
    try {
      const stored = hydrateConnectorConfig(await getRuntimeConfig());
      setConfig(stored);
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('connector.loadFailed'));
    } finally {
      setLoading(false);
    }
  }

  function openCreate() {
    const id = nextId('connector', new Set(connectors.map((item) => item.ID)));
    form.resetFields();
    form.setFieldsValue({
      ...defaultConnector,
      ID: id,
      ManagedNodeID: defaultTargetNodeID(scope),
      Name: '',
      Binding: {
        ...defaultConnector.Binding,
        DeviceName: `tapx-tun-${connectors.length + 1}`,
      },
      settings: defaultOutboundSettings('vless'),
      streamSettings: newStreamSlice('tcp'),
      JSONText: '',
      ImportLink: '',
    });
    setEditing(null);
    setActiveFormTab('basic');
    setJsonDirty(false);
    setOpen(true);
  }

  function openEdit(record: ConnectorRecord) {
    const runtime = record.RuntimeMode || 'embedded-xray';
    const recordProtocol = record.Protocol || (runtime === 'tapx' ? 'raw-udp' : 'vless');
    const streamSettings = runtime === 'tapx' ? undefined : (record.streamSettings || defaultStreamForProtocol(recordProtocol));
    form.resetFields();
    form.setFieldsValue({
      ...defaultConnector,
      ...record,
      RuntimeMode: runtime,
      Protocol: recordProtocol,
      settings: { ...defaultOutboundSettings(recordProtocol), ...(record.settings || {}) },
      streamSettings,
      Binding: { ...defaultConnector.Binding, ...record.Binding },
      JSONText: JSON.stringify(record, null, 2),
      ImportLink: '',
    });
    setEditing(record);
    setActiveFormTab('basic');
    setJsonDirty(false);
    setOpen(true);
  }

  async function submit() {
    await form.validateFields();
    const values = form.getFieldsValue(true) as ConnectorRecord;
    const id = values.ID || editing?.ID || nextId('connector', new Set(connectors.map((item) => item.ID)));
    let next = normalizeForSave({ ...defaultConnector, ...editing, ...values, ID: id, Name: values.Name || id });
    if (jsonDirty && values.JSONText?.trim()) {
      try {
        next = normalizeForSave(mergeConnectorJson(next, values.JSONText, t));
      } catch (error) {
        messageApi.error(error instanceof Error ? error.message : t('connector.invalidJsonFormat'));
        return;
      }
    }
    if (connectorIDConflicts(next.ID, editing?.ID, connectors.map((item) => item.ID))) {
      messageApi.error(t('connector.idExists', { id: next.ID }));
      return;
    }
    if (next.RuntimeMode === 'external-xray' && !externalXrayReady) {
      messageApi.error(t('listener.externalXrayRequired'));
      return;
    }

    const materializedVKey = materializeConnectorVKey(next, config.VKeys || [], {
      listeners: config.Listeners || [],
      connectors,
      clients: config.Clients || [],
      routes: config.Routes || [],
    });
    next = materializedVKey.connector;
    let materialized;
    try {
      materialized = materializeEndpointAutoDevice(next, devices, {
        role: 'connector', defaultMode: 'autoCreate', defaultAddressMode: 'auto',
      });
    } catch (error) {
      if (error instanceof DeviceTypeConflictError) {
        messageApi.error(t('device.typeConflict', deviceTypeConflictValues(error)));
        return;
      }
      throw error;
    }
    next = materialized.endpoint;
    let nextProfiles = config.XrayProfiles || [];
    if (next.RuntimeMode !== 'tapx') {
      const profile = buildConnectorXrayProfile(next);
      next = { ...next, XrayProfileID: profile.ID };
      nextProfiles = upsertXrayProfile(nextProfiles, profile);
    } else if (editing?.XrayProfileID) {
      next = { ...next, XrayProfileID: '' };
      nextProfiles = removeUnusedXrayProfiles(nextProfiles, [editing.XrayProfileID], {
        listeners: config.Listeners || [],
        connectors: connectors.filter((item) => !sameNodeObject(item, next)),
      });
    }
    const index = connectors.findIndex((item) => sameNodeObject(item, next));
    const nextConnectors = index < 0 ? [...connectors, next] : connectors.map((item) => (sameNodeObject(item, next) ? next : item));
    const committed = await commitConfig({
      ...config,
      Devices: materialized.devices,
      Connectors: nextConnectors,
      VKeys: materializedVKey.vkeys,
      XrayProfiles: nextProfiles,
    }, t('connector.saved'));
    if (committed) setOpen(false);
  }

  async function commitConfig(nextConfig: RuntimeConfig, successMessage?: string): Promise<boolean> {
    setSaving(true);
    try {
      const saved = await saveRuntimeConfig(nextConfig);
      try {
        await applyRuntimeConfig();
      } catch (applyError) {
        await saveRuntimeConfig(config);
        await applyRuntimeConfig().catch(() => undefined);
        throw applyError;
      }
      setConfig(hydrateConnectorConfig(saved));
      if (successMessage) messageApi.success(successMessage);
      return true;
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('connector.saveFailed'));
      return false;
    } finally {
      setSaving(false);
    }
  }

  async function upsertIntegrationConnector(
    draft: WireguardIntegrationDraft,
    current: ConnectorRecord | undefined,
    successMessage: string,
  ) {
    const { record, profile } = integrationConnectorRecord(draft, current, connectors, integrationTargetNodeID);
    const nextConnectors = current
      ? connectors.map((item) => sameNodeObject(item, current) ? record : item)
      : [...connectors, record];
    const profiles = config.XrayProfiles || [];
    const nextProfiles = profiles.some((item) => sameNodeObject(item, profile))
      ? profiles.map((item) => sameNodeObject(item, profile) ? profile : item)
      : [...profiles, profile];
    await commitConfig({ ...config, Connectors: nextConnectors, XrayProfiles: nextProfiles }, successMessage);
  }

  async function removeIntegrationConnector(current: ConnectorRecord | undefined, successMessage: string) {
    if (!current) return;
    await deleteConnectors([current], successMessage);
  }

  async function deleteConnectors(records: ConnectorRecord[], successMessage = t('connector.deleted')) {
    if (records.length === 0) return;
    const keys = new Set(records.map(nodeObjectKey));
    const nextConnectors = connectors.filter((item) => !keys.has(nodeObjectKey(item)));
    const removedProfileKeys = new Set(records
      .filter((item) => item.XrayProfileID)
      .map((item) => `${nodeIDOf(item)}:${item.XrayProfileID}`));
    const profileInUse = (profile: TapxXrayProfile & NodeOwned) => [...(config.Listeners || []), ...nextConnectors]
      .some((endpoint) => endpoint.XrayProfileID === profile.ID && nodeIDOf(endpoint) === nodeIDOf(profile));
    const nextProfiles = (config.XrayProfiles || []).filter((profile) => (
      !removedProfileKeys.has(`${nodeIDOf(profile)}:${profile.ID}`) || profileInUse(profile)
    ));
    const removedRoutes = (config.Routes || []).filter((route) => (
      Boolean(route.ConnectorID && keys.has(`${nodeIDOf(route)}:${route.ConnectorID}`))
    ));
    const removedRouteKeys = new Set(removedRoutes.map((route) => relationKey(route, route.ID)));
    const removedConnectorKeys = new Set(records.map((record) => relationKey(record, record.ID)));
    const removedAddressKeys = new Set(removedRoutes
      .filter((route) => route.AddressID)
      .map((route) => `${nodeIDOf(route)}:${route.AddressID}`));
    const nextRoutes = (config.Routes || []).filter((route) => !removedRoutes.includes(route));
    const nextAddresses = (config.Addresses || []).filter((address) => (
      !removedAddressKeys.has(`${nodeIDOf(address)}:${address.ID}`) || !isManagedLinkAddressRemark(address.Remark)
    ));
    const nextDevices = devices;
    const nextListeners = clearBindingReferences(
      clearBindingReferences(config.Listeners || [], 'ConnectorID', removedConnectorKeys),
      'RouteID',
      removedRouteKeys,
    );
    const nextClients = clearBindingReferences(
      clearBindingReferences(config.Clients || [], 'ConnectorID', removedConnectorKeys),
      'RouteID',
      removedRouteKeys,
    );
    const cleanedConnectors = clearBindingReferences(
      clearBindingReferences(nextConnectors, 'ConnectorID', removedConnectorKeys),
      'RouteID',
      removedRouteKeys,
    );
    if (await commitConfig({
      ...config,
      Devices: nextDevices,
      Listeners: nextListeners,
      Connectors: cleanedConnectors,
      Clients: nextClients,
      Routes: nextRoutes,
      Addresses: nextAddresses,
      XrayProfiles: nextProfiles,
    }, successMessage)) {
      setSelectedRowKeys([]);
    }
  }

  async function setSelectedConnectorsEnabled(enabled: boolean) {
    if (selectedConnectors.length === 0) return;
    const selected = new Set(selectedConnectors.map(nodeObjectKey));
    const selectedProfiles = new Set(selectedConnectors
      .filter((item) => item.XrayProfileID)
      .map((item) => `${nodeIDOf(item)}:${item.XrayProfileID}`));
    if (await commitConfig({
      ...config,
      Connectors: connectors.map((item) => selected.has(nodeObjectKey(item)) ? { ...item, Enabled: enabled } : item),
      XrayProfiles: (config.XrayProfiles || []).map((profile) => (
        selectedProfiles.has(`${nodeIDOf(profile)}:${profile.ID}`) ? { ...profile, Enabled: enabled } : profile
      )),
    }, enabled ? t('connector.batchEnabled') : t('connector.batchDisabled'))) {
      setSelectedRowKeys([]);
    }
  }

  async function toggleEnable(record: ConnectorRecord, enabled: boolean) {
    const profileKey = record.XrayProfileID ? `${nodeIDOf(record)}:${record.XrayProfileID}` : '';
    await commitConfig({
      ...config,
      Connectors: connectors.map((item) => sameNodeObject(item, record) ? { ...item, Enabled: enabled } : item),
      XrayProfiles: (config.XrayProfiles || []).map((profile) => (
        profileKey && `${nodeIDOf(profile)}:${profile.ID}` === profileKey ? { ...profile, Enabled: enabled } : profile
      )),
    }, enabled ? t('connector.enabled') : t('connector.disabled'));
  }

  function exportConnectors(records: ConnectorRecord[]) {
    const target = records.length > 0 ? records : connectors;
    setExportModal({
      open: true,
      title: records.length > 0 ? t('connector.exportCount', { count: records.length }) : t('connector.export'),
      value: JSON.stringify(target, null, 2),
    });
  }

  async function submitImport() {
    let imported: ConnectorRecord[];
    try {
      imported = normalizeImportedConnectors(importText, connectors, importTargetNodeID);
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('connector.invalidJson'));
      return;
    }
    if (imported.length === 0) {
      messageApi.warning(t('connector.noneToImport'));
      return;
    }
    const index = new Map(connectors.map((item) => [nodeObjectKey(item), item]));
    let nextProfiles = config.XrayProfiles || [];
    let nextDevices = devices;
    let nextVKeys = config.VKeys || [];
    for (const item of imported) {
      let normalized = { ...index.get(nodeObjectKey(item)), ...normalizeForSave(item), UpdatedAt: nowSecond() } as ConnectorRecord;
      const materializedVKey = materializeConnectorVKey(normalized, nextVKeys, {
        listeners: config.Listeners || [],
        connectors: [...index.values()],
        clients: config.Clients || [],
        routes: config.Routes || [],
      });
      normalized = materializedVKey.connector;
      nextVKeys = materializedVKey.vkeys;
      let materializedDevice;
      try {
        materializedDevice = materializeEndpointAutoDevice(normalized, nextDevices, {
          role: 'connector', defaultMode: 'autoCreate', defaultAddressMode: 'auto',
        });
      } catch (error) {
        if (error instanceof DeviceTypeConflictError) {
          messageApi.error(t('device.typeConflict', deviceTypeConflictValues(error)));
          return;
        }
        throw error;
      }
      normalized = materializedDevice.endpoint;
      nextDevices = materializedDevice.devices;
      if (normalized.RuntimeMode !== 'tapx') {
        const profile = buildConnectorXrayProfile(normalized);
        normalized = { ...normalized, XrayProfileID: profile.ID };
        nextProfiles = upsertXrayProfile(nextProfiles, profile);
      }
      index.set(nodeObjectKey(normalized), normalized);
    }
    await commitConfig({
      ...config,
      Devices: nextDevices,
      Connectors: [...index.values()],
      VKeys: nextVKeys,
      XrayProfiles: nextProfiles,
    }, t('connector.importedCount', { count: imported.length }));
    setImportOpen(false);
    setImportText('');
  }

  async function copyExportValue() {
    try {
      await copyText(exportModal.value);
      messageApi.success(t('connector.copied'));
    } catch {
      messageApi.error(t('connector.copyFailed'));
    }
  }

  function handleRuntimeModeChange(mode: RuntimeMode) {
    if (!runtimeModeChangesTransportFamily(runtimeMode, mode)) return;
    if (mode === 'tapx') {
      form.setFieldValue('Protocol', 'raw-udp');
      form.setFieldValue('Network', 'udp');
      form.setFieldValue('Security', 'none');
      form.setFieldValue('settings', {});
      form.setFieldValue('streamSettings', undefined);
      form.setFieldsValue(defaultTapxConnectorFields());
      return;
    }
    form.setFieldValue('Protocol', 'vless');
    form.setFieldValue('Network', 'tcp');
    form.setFieldValue('Security', 'none');
    form.setFieldValue('settings', defaultOutboundSettings('vless'));
    form.setFieldValue('streamSettings', newStreamSlice('tcp'));
  }

  function handleProtocolChange(nextProtocol: string) {
    if (runtimeMode === 'tapx') {
      form.setFieldValue('Network', nextProtocol === 'raw-udp' ? 'udp' : 'tcp');
      form.setFieldValue('Security', 'none');
      form.setFieldsValue(defaultTapxConnectorFields());
      return;
    }
    const stream = defaultStreamForProtocol(nextProtocol);
    form.setFieldValue('Network', String(stream?.network || ''));
    form.setFieldValue('Security', String(stream?.security || 'none'));
    form.setFieldValue('settings', defaultOutboundSettings(nextProtocol));
    form.setFieldValue('streamSettings', stream);
  }

  function importLink() {
    const link = String(form.getFieldValue('ImportLink') || '').trim();
    if (!link) return;
    try {
      const parsed = parseImportLink(link, connectors, t, targetNodeID);
      form.setFieldsValue({
        ...parsed,
        JSONText: JSON.stringify(parsed, null, 2),
      });
      setJsonDirty(false);
      messageApi.success(t('connector.linkImported'));
    } catch (err) {
      messageApi.error(err instanceof OutboundLinkParseError
        ? t('connector.invalidShadowsocksLink')
        : err instanceof Error ? err.message : t('connector.linkImportFailed'));
    }
  }

  async function runTest(kind: TestKind, records: ConnectorRecord[], durationSeconds?: number) {
    const target = records.length > 0 ? records : connectors;
    if (target.length === 0) {
      messageApi.warning(t('connector.noneToTest'));
      return;
    }
    setSaving(true);
    try {
      const settled = await Promise.allSettled(target.map((record) => testConnector(record.ID, kind, durationSeconds, nodeIDOf(record))));
      const successful = new Map<string, Awaited<ReturnType<typeof testConnector>>>();
      const failures: string[] = [];
      settled.forEach((result, index) => {
        if (result.status === 'fulfilled') {
          successful.set(nodeObjectKey(target[index]), result.value);
        } else {
          failures.push(`${target[index].Name || target[index].ID}: ${errorMessage(result.reason, t, 'connector.testFailed')}`);
        }
      });
      setConfig((current) => ({
        ...current,
        Connectors: ((current.Connectors || []) as ConnectorRecord[]).map((item) => {
          const result = successful.get(nodeObjectKey(item));
          return result ? {
            ...item,
            LastDelayMs: result.delayMs,
            LastTestKind: kind,
            LastTestConfirmed: result.confirmed,
            LastTestMessage: result.message,
            LastTestResult: result,
          }
            : item;
        }),
      }));
      if (failures.length > 0) messageApi.warning(failures.join('；'));
      if (successful.size === 1) {
        const result = [...successful.values()][0];
        if (result.confirmed) messageApi.success(diagnosticResultMessage(result));
        else messageApi.info(diagnosticResultMessage(result));
      } else if (successful.size > 1) {
        messageApi.success(t('connector.testCompletedCount', { count: successful.size }));
      }
    } finally {
      setSaving(false);
    }
  }

  function openDiagnostics(record: ConnectorRecord) {
    throughputAbortRef.current?.abort();
    setThroughputElapsedMs(0);
    setThroughputSamples([]);
    setDiagnosticTarget(record);
    setDiagnosticResults(record.LastTestResult ? { [record.LastTestResult.kind]: record.LastTestResult } : {});
    setDiagnosticOpen(true);
  }

  async function runDiagnostic(kind: TestKind) {
    if (!diagnosticTarget) return;
    setDiagnosticLoading(kind);
    try {
      const result = await testConnector(diagnosticTarget.ID, kind, kind === 'throughput' ? 2 : undefined, nodeIDOf(diagnosticTarget));
      setDiagnosticResults((current) => ({ ...current, [kind]: result }));
      setConfig((current) => ({
        ...current,
        Connectors: ((current.Connectors || []) as ConnectorRecord[]).map((item) => sameNodeObject(item, diagnosticTarget)
          ? {
            ...item,
            LastDelayMs: result.delayMs,
            LastTestKind: kind,
            LastTestConfirmed: result.confirmed,
            LastTestMessage: result.message,
            LastTestResult: result,
          }
          : item),
      }));
    } catch (err) {
      messageApi.error(errorMessage(err, t, 'connector.testFailed'));
    } finally {
      setDiagnosticLoading(null);
    }
  }

  function diagnosticResultMessage(result: ConnectorTestResult) {
    switch (result.messageCode) {
      case 'channel-confirmed': return t('connector.diagnosticResult.channel-confirmed');
      case 'udp-channel-confirmed': return t('connector.diagnosticResult.udp-channel-confirmed');
      case 'tcp-path-measured': return t('connector.diagnosticResult.tcp-path-measured');
      case 'tcp-mss-measured': return t('connector.diagnosticResult.tcp-mss-measured');
      case 'stream-frame-confirmed': return t('connector.diagnosticResult.stream-frame-confirmed');
      case 'udp-path-confirmed': return t('connector.diagnosticResult.udp-path-confirmed');
      case 'throughput-measured': return t('connector.diagnosticResult.throughput-measured');
      default:
        if (result.kind === 'throughput') return t('connector.diagnosticResult.throughput-measured');
        return result.message;
    }
  }

  async function runThroughputDiagnostic() {
    if (!diagnosticTarget) return;
    const controller = new AbortController();
    throughputAbortRef.current?.abort();
    throughputAbortRef.current = controller;
    setDiagnosticLoading('throughput');
    setThroughputElapsedMs(0);
    setThroughputSamples([]);
    let elapsedMs = 0;
    let uploadBytes = 0;
    let downloadBytes = 0;
    let latestUploadBps = 0;
    let latestDownloadBps = 0;
    try {
      const directions: ThroughputDirection[] = throughputContinuous ? [throughputDirection] : ['download', 'upload'];
      for (const direction of directions) {
        setThroughputPhase(direction);
        let phaseElapsedMs = 0;
        while (!controller.signal.aborted && (throughputContinuous || phaseElapsedMs < throughputDuration * 1000)) {
          const remainingSeconds = throughputContinuous ? 1 : Math.max(0.25, Math.min(1, (throughputDuration * 1000 - phaseElapsedMs) / 1000));
          const sample = await testConnector(
            diagnosticTarget.ID,
            'throughput',
            remainingSeconds,
            nodeIDOf(diagnosticTarget),
            controller.signal,
            direction,
          );
          const sampleDuration = Math.max(1, sample.durationMs || Math.round(remainingSeconds * 1000));
          phaseElapsedMs += sampleDuration;
          elapsedMs += sampleDuration;
          uploadBytes += sample.uploadBytes || 0;
          downloadBytes += sample.downloadBytes || 0;
          if (direction === 'upload') latestUploadBps = sample.uploadBps || 0;
          else latestDownloadBps = sample.downloadBps || 0;
          const result: ConnectorTestResult = {
            ...sample,
            uploadBps: latestUploadBps,
            downloadBps: latestDownloadBps,
            uploadBytes,
            downloadBytes,
            durationMs: elapsedMs,
          };
          setThroughputElapsedMs(elapsedMs);
          setThroughputSamples((current) => [...current.slice(-239), { at: Date.now(), direction, bps: direction === 'upload' ? latestUploadBps : latestDownloadBps }]);
          setDiagnosticResults((current) => ({ ...current, throughput: result }));
        }
        if (throughputContinuous) break;
      }
    } catch (err) {
      if (!controller.signal.aborted) messageApi.error(errorMessage(err, t, 'connector.testFailed'));
    } finally {
      if (throughputAbortRef.current === controller) throughputAbortRef.current = null;
      setDiagnosticLoading(null);
    }
  }

  function stopThroughputDiagnostic() {
    throughputAbortRef.current?.abort();
  }

  const moreItems: MenuProps['items'] = selectedConnectors.length > 0 ? [
    { key: 'test-selected', icon: <PlayCircleOutlined />, label: t('connector.testSelected') },
    { key: 'enable-selected', icon: <CheckCircleOutlined />, label: t('connector.enableSelected') },
    { key: 'disable-selected', icon: <StopOutlined />, label: t('connector.disableSelected') },
    { type: 'divider' },
    { key: 'export-selected', icon: <ExportOutlined />, label: t('connector.exportSelected') },
    { key: 'reset-selected', icon: <RetweetOutlined />, label: t('connector.resetSelectedTraffic') },
  ] : [
    { key: 'warp', icon: <CloudOutlined />, label: 'WARP' },
    { key: 'nord', icon: <ApiOutlined />, label: 'NordVPN' },
    { type: 'divider' },
    { key: 'import', icon: <ImportOutlined />, label: t('connector.import') },
    { key: 'export', icon: <ExportOutlined />, label: t('connector.export'), disabled: visibleConnectors.length === 0 },
  ];

  const onMoreClick: MenuProps['onClick'] = ({ key }) => {
    switch (key) {
      case 'import':
        setImportTargetNodeID(defaultTargetNodeID(scope));
        setImportOpen(true);
        break;
      case 'export':
        exportConnectors(visibleConnectors);
        break;
      case 'export-selected':
        exportConnectors(selectedConnectors);
        break;
      case 'test-selected':
        void runTest('channel', selectedConnectors);
        break;
      case 'enable-selected':
        setSelectedConnectorsEnabled(true);
        break;
      case 'disable-selected':
        setSelectedConnectorsEnabled(false);
        break;
      case 'reset-selected':
        void resetTraffic(selectedConnectors);
        break;
      case 'warp':
        setIntegrationTargetNodeID(defaultTargetNodeID(scope));
        setWarpOpen(true);
        break;
      case 'nord':
        setIntegrationTargetNodeID(defaultTargetNodeID(scope));
        setNordOpen(true);
        break;
    }
  };

  function moveConnector(from: number, to: number) {
    if (to < 0 || to >= visibleConnectors.length || from === to) return;
    const reordered = [...visibleConnectors];
    const [record] = reordered.splice(from, 1);
    reordered.splice(to, 0, record);
    let visibleIndex = 0;
    const next = connectors.map((item) => (
      visibleConnectors.some((visible) => sameNodeObject(visible, item)) ? reordered[visibleIndex++] : item
    ));
    void commitConfig({ ...config, Connectors: next }, t('connector.orderUpdated'));
  }

  async function resetTraffic(records: ConnectorRecord[]) {
    if (records.length === 0) return;
    setSaving(true);
    try {
      const resetByKey = new Map<string, TapxEndpoint>();
      for (const record of records) {
        const resetConfig = await resetConnectorTraffic(record.ID, nodeIDOf(record));
        const reset = (resetConfig.Connectors || []).find((item) => item.ID === record.ID);
        if (reset) resetByKey.set(nodeObjectKey(record), reset);
      }
      setConfig((current) => {
        const next = {
          ...current,
          Connectors: (current.Connectors || []).map((item) => {
          const reset = resetByKey.get(nodeObjectKey(item as ConnectorRecord));
          return reset ? {
            ...item,
            TrafficResetAt: reset.TrafficResetAt,
            TrafficResetGeneration: reset.TrafficResetGeneration,
            TrafficRXOffset: reset.TrafficRXOffset,
            TrafficTXOffset: reset.TrafficTXOffset,
          } : item;
          }),
        };
        return next;
      });
      setConnectorStats((current) => ({
        ...current,
        ...Object.fromEntries(records.map((record) => [nodeObjectKey(record), { rxBytes: 0, txBytes: 0 }])),
      }));
      messageApi.success(t('connector.trafficReset'));
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('connector.trafficResetFailed'));
    } finally {
      setSaving(false);
    }
  }

  function confirmDelete(record: ConnectorRecord) {
    Modal.confirm({
      title: t('connector.deleteConfirm', { name: record.Name || record.ID }),
      content: t('connector.deleteHelp'),
      okText: t('common.delete'),
      okType: 'danger',
      cancelText: t('connector.cancel'),
      onOk: () => deleteConnectors([record]),
    });
  }

  function confirmDeleteSelected() {
    Modal.confirm({
      title: t('connector.batchDeleteConfirm', { count: selectedConnectors.length }),
      content: t('connector.deleteHelp'),
      okText: t('common.delete'),
      okType: 'danger',
      cancelText: t('connector.cancel'),
      onOk: () => deleteConnectors(selectedConnectors),
    });
  }

  const columns = useMemo<TableColumnsType<ConnectorRecord>>(() => [
    { title: 'ID', key: 'Index', width: 64, align: 'center', fixed: 'left', render: (_, _record, index) => index + 1 },
    { title: t('node.sourceNode'), key: 'ManagedNodeID', width: 150, fixed: 'left', render: (_, record) => <NodeSourceTag value={record} /> },
    {
      title: t('connector.actions'),
      key: 'actions',
      width: 80,
      align: 'center',
      fixed: 'left',
      render: (_, record, index) => (
        <Space size={2}>
          <Button shape="circle" size="small" icon={<EditOutlined />} aria-label={t('connector.edit')} onClick={() => openEdit(record)} />
          <Dropdown
            trigger={['click']}
            menu={{
              items: [
                ...(index > 0 ? [{ key: 'top', icon: <VerticalAlignTopOutlined />, label: t('listener.moveTop'), onClick: () => moveConnector(index, 0) }] : []),
                { key: 'up', icon: <ArrowUpOutlined />, label: t('common.moveUp'), disabled: index === 0, onClick: () => moveConnector(index, index - 1) },
                { key: 'down', icon: <ArrowDownOutlined />, label: t('common.moveDown'), disabled: index === visibleConnectors.length - 1, onClick: () => moveConnector(index, index + 1) },
                { key: 'export', icon: <ExportOutlined />, label: t('connector.export'), onClick: () => exportConnectors([record]) },
                { key: 'reset', icon: <RetweetOutlined />, label: t('connector.resetTraffic'), onClick: () => void resetTraffic([record]) },
                { key: 'delete', icon: <DeleteOutlined />, label: t('common.delete'), danger: true, onClick: () => confirmDelete(record) },
              ],
            }}
          >
            <Button shape="circle" size="small" icon={<MoreOutlined />} aria-label={t('connector.more')} />
          </Dropdown>
        </Space>
      ),
    },
    {
      title: t('connector.tag'),
      key: 'Name',
      width: 190,
      render: (_, record) => <span>{record.Name || record.ID}</span>,
    },
    {
      title: t('common.enabled'),
      key: 'Enabled',
      align: 'center',
      width: 80,
      render: (_, record) => <Switch size="small" checked={record.Enabled !== false} onChange={(checked) => void toggleEnable(record, checked)} />,
    },
    {
      title: t('connector.runtimeMode'),
      key: 'RuntimeMode',
      width: 130,
      align: 'center',
      render: (_, record) => {
        const mode = record.RuntimeMode || 'embedded-xray';
        return <Tag color={mode === 'tapx' ? 'volcano' : 'blue'}>{runtimeOptions.find((item) => item.value === mode)?.label || mode}</Tag>;
      },
    },
    {
      title: t('connector.protocol'),
      key: 'Protocol',
      width: 190,
      render: (_, record) => {
        const security = String(record.Security || record.streamSettings?.security || 'none');
        return (
          <Space wrap size={[4, 4]}>
            <Tag color="green">{connectorProtocolLabel(record.Protocol || 'vless')}</Tag>
            {record.Network && !record.Protocol?.startsWith('raw-') ? <Tag>{record.Network}</Tag> : null}
            {security !== 'none' ? <Tag color="blue">{security.toUpperCase()}</Tag> : null}
            {record.VKey ? <Tag color="green">vKey</Tag> : null}
          </Space>
        );
      },
    },
    {
      title: t('connector.boundDevice'),
      key: 'Binding',
      width: 180,
      render: (_, record) => {
        const device = record.Binding?.DeviceID ? devices.find((item) => item.ID === record.Binding?.DeviceID && nodeIDOf(item) === nodeIDOf(record)) : undefined;
        if (device) return <Space size={4}><Tag color={device.Type === 'tap' ? 'geekblue' : 'green'}>{String(device.Type || 'tun').toUpperCase()}</Tag><span>{device.IfName || device.Name}</span></Space>;
        if (record.Binding?.AutoCreateDevice) return <Space size={4}><Tag color={record.Binding.InterfaceType === 'tap' ? 'geekblue' : 'green'}>{String(record.Binding.InterfaceType || 'tun').toUpperCase()}</Tag><span>{record.Binding.DeviceName || t('listener.autoCreate')}</span></Space>;
        return <span className="criterion-empty">-</span>;
      },
    },
    {
      title: t('xray.address'),
      key: 'Remote',
      width: 190,
      render: (_, record) => endpointAddress(record),
    },
    {
      title: t('connector.traffic'),
      key: 'Traffic',
      align: 'center',
      width: 140,
      render: (_, record) => (
        <Space size={12}>
          <span>↑ {formatBytes(connectorStats[nodeObjectKey(record)]?.txBytes || 0)}</span>
          <span>↓ {formatBytes(connectorStats[nodeObjectKey(record)]?.rxBytes || 0)}</span>
        </Space>
      ),
    },
    {
      title: t('connector.latency'),
      key: 'Delay',
      align: 'center',
      width: 100,
      render: (_, record) => record.LastDelayMs ? (
        <Tooltip title={record.LastTestMessage || ''}>
          <Tag color={record.LastTestConfirmed === false ? 'gold' : 'green'}>{record.LastDelayMs} ms</Tag>
        </Tooltip>
      ) : <span className="criterion-empty">—</span>,
    },
    {
      title: t('connector.diagnostics'),
      key: 'test',
      align: 'center',
      width: 118,
      render: (_, record) => (
        <Tooltip title={t('connector.openDiagnostics')}>
          <Button
            type="primary"
            shape="circle"
            icon={<DashboardOutlined />}
            aria-label={t('connector.diagnostics')}
            disabled={endpointAddress(record) === '-'}
            onClick={() => openDiagnostics(record)}
          />
        </Tooltip>
      ),
    },
    {
      title: t('common.status'),
      key: 'Status',
      align: 'center',
      width: 92,
      render: (_, record) => {
        if (record.Enabled === false) return <Tag>{t('common.stopped')}</Tag>;
        return activeConnectorKeys.has(nodeObjectKey(record))
          ? <Tag color="success">{t('common.running')}</Tag>
          : <Tag color="warning">{t('common.stopped')}</Tag>;
      },
    },
  ], [activeConnectorKeys, connectors, connectorStats, devices, runtimeOptions, t, visibleConnectors]);

  const protocolOptions = runtimeMode === 'tapx' ? tapxProtocolOptions : outboundXrayProtocolOptions;

  function diagnosticContent(kind: TestKind) {
    const result = diagnosticResults[kind];
    const isStreamTransport = Boolean(result && (result.network === 'tcp' || result.network === 'tls' || result.network.startsWith('xray')));
    const unavailable = (value?: number, notApplicable = false) => value || (notApplicable ? t('connector.notApplicable') : t('connector.notDetected'));
    const items = result ? [
      { key: 'status', label: t('connector.diagnosticStatus'), children: <Tag color={result.confirmed ? 'green' : 'gold'}>{result.confirmed ? t('connector.confirmed') : t('connector.unconfirmed')}</Tag> },
      { key: 'target', label: t('connector.diagnosticTarget'), children: result.target || '-' },
      { key: 'transport', label: t('connector.diagnosticTransport'), children: result.network || '-' },
      { key: 'device', label: t('connector.boundDevice'), children: result.deviceName || '-' },
      ...(kind === 'channel' ? [
        { key: 'delay', label: t('connector.controlLatency'), children: result.delayMs ? `${result.delayMs} ms` : '-' },
      ] : []),
      ...(kind === 'path-mtu' ? [
        { key: 'pathMtu', label: result.pathMtuSource === 'frame' ? t('connector.verifiedFrameMtu') : t('connector.confirmedPathMtu'), children: unavailable(result.confirmedPathMtu) },
        { key: 'effectiveMtu', label: t('connector.effectiveNetworkMtu'), children: unavailable(result.effectiveNetworkMtu) },
        { key: 'payload', label: t('connector.maxDatagramPayload'), children: unavailable(result.maxDatagramPayload, isStreamTransport) },
        { key: 'mss4', label: 'TCP MSS IPv4', children: unavailable(result.tcpMssIpv4, result.addressFamily === 'ipv6' || result.pathMtuSource === 'frame') },
        { key: 'mss6', label: 'TCP MSS IPv6', children: unavailable(result.tcpMssIpv6, result.addressFamily === 'ipv4' || result.pathMtuSource === 'frame') },
      ] : []),
      ...(kind === 'throughput' ? [
        { key: 'upload', label: t('connector.uploadThroughput'), children: formatBitRate(result.uploadBps || 0) },
        { key: 'download', label: t('connector.downloadThroughput'), children: formatBitRate(result.downloadBps || 0) },
        { key: 'uploadBytes', label: t('connector.uploadData'), children: formatBytes(result.uploadBytes || 0) },
        { key: 'downloadBytes', label: t('connector.downloadData'), children: formatBytes(result.downloadBytes || 0) },
        { key: 'duration', label: t('connector.testDuration'), children: result.durationMs ? `${(result.durationMs / 1000).toFixed(1)} s` : '-' },
      ] : []),
    ] : [];
    if (kind === 'throughput') {
      const running = diagnosticLoading === 'throughput';
      const activeDirection = running ? throughputPhase : 'download';
      const activeSample = running
        ? [...throughputSamples].reverse().find((sample) => sample.direction === activeDirection)
        : undefined;
      const activeBps = activeSample?.bps || 0;
      const peakBps = Math.max(activeBps, ...throughputSamples.map((sample) => sample.bps));
      const scale = throughputGaugeLimit || (peakBps > 0 ? throughputGaugeScale(peakBps) : 1_000_000_000);
      return (
        <Space orientation="vertical" size="large" style={{ width: '100%' }}>
          <div className="throughput-controls">
            <div className="throughput-duration-control">
              <span>{t('connector.testDuration')}</span>
              <InputNumber
                min={1}
                max={3600}
                precision={0}
                value={throughputDuration}
                disabled={throughputContinuous || running}
                addonAfter="s"
                onChange={(value) => setThroughputDuration(Number(value) || 10)}
              />
            </div>
            <div className="throughput-scale-control">
              <span>{t('connector.testScale')}</span>
              <Select
                value={throughputGaugeLimit}
                disabled={running}
                options={[
                  { label: t('connector.autoScale'), value: 0 },
                  { label: '10 Mbps', value: 10_000_000 },
                  { label: '100 Mbps', value: 100_000_000 },
                  { label: '1 Gbps', value: 1_000_000_000 },
                  { label: '2.5 Gbps', value: 2_500_000_000 },
                  { label: '10 Gbps', value: 10_000_000_000 },
                  { label: '100 Gbps', value: 100_000_000_000 },
                ]}
                onChange={setThroughputGaugeLimit}
              />
            </div>
            <Space>
              <Switch checked={throughputContinuous} disabled={running} onChange={setThroughputContinuous} />
              <span>{t('connector.continuousTest')}</span>
            </Space>
            {throughputContinuous && (
              <Radio.Group
                optionType="button"
                buttonStyle="solid"
                value={throughputDirection}
                disabled={running}
                options={[
                  { label: t('connector.download'), value: 'download' },
                  { label: t('connector.upload'), value: 'upload' },
                ]}
                onChange={(event) => setThroughputDirection(event.target.value as ThroughputDirection)}
              />
            )}
            {running
              ? <Button danger icon={<StopOutlined />} onClick={stopThroughputDiagnostic}>{t('connector.stopThroughputTest')}</Button>
              : <Button type="primary" icon={<PlayCircleOutlined />} onClick={() => void runThroughputDiagnostic()}>{t('connector.startThroughputTest')}</Button>}
          </div>
          <div className="throughput-speedometer">
            <ThroughputGauge
              label={activeDirection === 'upload' ? t('connector.uploadThroughput') : t('connector.downloadThroughput')}
              direction={activeDirection}
              value={activeBps}
              scale={scale}
            />
          </div>
          <div className="throughput-progress-text">
            {running
              ? t(activeDirection === 'upload' ? 'connector.testingUploadThroughput' : 'connector.testingDownloadThroughput')
              : result ? diagnosticResultMessage(result) : t('connector.noDiagnosticResult')}
            {(running || result) && <span>{t('connector.elapsedTime')}: {(throughputElapsedMs / 1000).toFixed(0)} s</span>}
          </div>
          {!running && result && throughputSamples.length > 0 && (
            <ThroughputHistoryChart
              samples={throughputSamples}
              result={result}
              uploadLabel={t('connector.upload')}
              downloadLabel={t('connector.download')}
              durationLabel={t('connector.testDuration')}
              uploadDataLabel={t('connector.uploadData')}
              downloadDataLabel={t('connector.downloadData')}
            />
          )}
        </Space>
      );
    }
    return (
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Alert type="info" showIcon title={kind === 'channel'
          ? t('connector.channelTestHelp')
          : t('connector.pathMtuTestHelp')} />
        <Button type="primary" loading={diagnosticLoading === kind} disabled={diagnosticLoading !== null && diagnosticLoading !== kind} onClick={() => void runDiagnostic(kind)}>
          {t('connector.startDiagnostic')}
        </Button>
        {result ? <>
          <Descriptions bordered size="small" column={2} items={items} />
          <Alert type={result.confirmed ? 'success' : 'warning'} showIcon title={diagnosticResultMessage(result)} />
        </> : <span className="criterion-empty">{t('connector.noDiagnosticResult')}</span>}
      </Space>
    );
  }

  return (
    <div className="connector-page">
      {messageContextHolder}
      <Card hoverable>
        <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
          <Row gutter={[12, 12]} align="middle" justify="space-between">
            <Col xs={24} sm={12}>
              <Space wrap>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('connector.add')}</Button>
                <NodeScopeSelect scope={scope} onChange={setScope} />
                {selectedConnectors.length > 0 ? (
                  <Tag color="blue" closable onClose={() => setSelectedRowKeys([])}>
                    {t('connector.selectedCount', { count: selectedConnectors.length })}
                  </Tag>
                ) : null}
                <Dropdown trigger={['click']} menu={{ items: moreItems, onClick: onMoreClick }}>
                  <Button icon={<MoreOutlined />}>{selectedConnectors.length > 0 ? t('connector.batchActions') : t('connector.more')}</Button>
                </Dropdown>
                {selectedConnectors.length > 0 ? (
                  <Button danger icon={<DeleteOutlined />} onClick={confirmDeleteSelected}>{t('connector.deleteSelected')}</Button>
                ) : null}
              </Space>
            </Col>
            <Col xs={24} sm={12} className="toolbar-right">
              <Space wrap>
                <Button type="primary" icon={<PlayCircleOutlined />} onClick={() => void runTest('channel', connectors)}>{t('connector.testAllChannels')}</Button>
                <Popconfirm
                  placement="topRight"
                  okText={t('common.reset')}
                  cancelText={t('connector.cancel')}
                  title={t('connector.resetAllConfirm')}
                  onConfirm={() => void resetTraffic(connectors)}
                >
                  <Button aria-label={t('connector.resetTraffic')} icon={<RetweetOutlined />} />
                </Popconfirm>
              </Space>
            </Col>
          </Row>
          <Table
            rowKey={nodeObjectKey}
            rowSelection={{ selectedRowKeys, onChange: (keys) => setSelectedRowKeys(keys.map(String)) }}
            columns={columns}
            dataSource={visibleConnectors}
            loading={loading || saving}
            pagination={false}
            scroll={{ x: 1450 }}
            size="small"
            locale={{ emptyText: t('connector.empty') }}
          />
        </Space>
      </Card>

      <Modal
        open={diagnosticOpen}
        title={`${t('connector.diagnostics')} - ${diagnosticTarget?.Name || diagnosticTarget?.ID || ''}`}
        footer={null}
        width={760}
        destroyOnHidden
        onCancel={() => {
          throughputAbortRef.current?.abort();
          setDiagnosticOpen(false);
        }}
      >
        <Tabs
          items={[
            { key: 'channel', label: t('connector.channelTest'), children: diagnosticContent('channel') },
            { key: 'path-mtu', label: t('connector.pathMtuTest'), children: diagnosticContent('path-mtu') },
            { key: 'throughput', label: t('connector.throughputTest'), children: diagnosticContent('throughput') },
          ]}
        />
      </Modal>

      <Modal
        open={open}
        title={editing ? t('connector.editTitle') : t('connector.addTitle')}
        okText={editing ? t('connector.saveChanges') : t('common.create')}
        cancelText={t('common.close')}
        width={780}
        forceRender
        confirmLoading={saving}
        mask={{ closable: false }}
        onOk={submit}
        onCancel={() => setOpen(false)}
        styles={{ body: { maxHeight: 'calc(100vh - 160px)', overflowY: 'auto', overflowX: 'hidden' } }}
      >
        <Form form={form} colon={false} labelCol={{ sm: { span: 8 } }} wrapperCol={{ sm: { span: 14 } }} labelWrap>
          <Tabs activeKey={activeFormTab} onChange={changeFormTab} items={[
            { key: 'basic', label: t('connector.basic'), forceRender: true, children: basicTab() },
            { key: 'binding', label: t('connector.binding'), forceRender: true, children: bindingTab() },
            { key: 'json', label: 'JSON', forceRender: true, children: jsonTab() },
          ]} />
        </Form>
      </Modal>

      <Modal
        open={exportModal.open}
        title={exportModal.title}
        width={720}
        okText={t('connector.copy')}
        cancelText={t('common.close')}
        onOk={copyExportValue}
        onCancel={() => setExportModal({ open: false, title: '', value: '' })}
      >
        <Input.TextArea value={exportModal.value} readOnly autoSize={{ minRows: 12, maxRows: 22 }} />
      </Modal>

      <Modal
        open={importOpen}
        title={t('connector.import')}
        okText={t('connector.importAction')}
        cancelText={t('connector.cancel')}
        confirmLoading={saving}
        onOk={submitImport}
        onCancel={() => setImportOpen(false)}
      >
        <Form.Item label={t('node.targetNode')} tooltip={t('node.targetNodeHelp')}>
          <Select value={importTargetNodeID} options={nodeTargetOptions} onChange={setImportTargetNodeID} />
        </Form.Item>
        <Form.Item label={t('connector.importContent')} tooltip={t('connector.importHelp')}>
          <Input.TextArea placeholder='[{"ID":"connector-1"}]' value={importText} onChange={(event) => setImportText(event.target.value)} autoSize={{ minRows: 10, maxRows: 18 }} />
        </Form.Item>
      </Modal>

      <WarpModal
        open={warpOpen}
        connectorExists={Boolean(warpConnector)}
        managedNodeID={integrationTargetNodeID}
        nodeTargetOptions={nodeTargetOptions}
        onManagedNodeIDChange={setIntegrationTargetNodeID}
        onClose={() => setWarpOpen(false)}
        onAddConnector={(draft) => upsertIntegrationConnector(draft, undefined, t('connector.warpAdded'))}
        onReplaceConnector={(draft) => upsertIntegrationConnector(draft, warpConnector, t('connector.warpUpdated'))}
        onRemoveConnector={() => removeIntegrationConnector(warpConnector, t('connector.warpDeleted'))}
      />
      <NordModal
        open={nordOpen}
        connectorExists={Boolean(nordConnector)}
        managedNodeID={integrationTargetNodeID}
        nodeTargetOptions={nodeTargetOptions}
        onManagedNodeIDChange={setIntegrationTargetNodeID}
        onClose={() => setNordOpen(false)}
        onAddConnector={(draft) => upsertIntegrationConnector(draft, undefined, t('connector.nordAdded'))}
        onReplaceConnector={(draft) => upsertIntegrationConnector(draft, nordConnector, t('connector.nordUpdated'))}
        onRemoveConnector={() => removeIntegrationConnector(nordConnector, t('connector.nordDeleted'))}
      />
    </div>
  );

  function basicTab() {
    return (
      <>
        <Form.Item name="ID" hidden><Input /></Form.Item>
        <Form.Item name="Enabled" hidden valuePropName="checked"><Switch /></Form.Item>
        <Form.Item name="ManagedNodeID" label={t('node.targetNode')} tooltip={t('node.targetNodeHelp')} rules={[{ required: true }]}>
          <Select options={nodeTargetOptions} disabled={Boolean(editing)} />
        </Form.Item>
        <Form.Item
          name="RuntimeMode"
          label={t('connector.runtimeMode')}
          tooltip={t('listener.runtimeModeHelp')}
        >
          <Select
            options={runtimeOptions.map((item) => item.value === 'external-xray'
              ? { ...item, disabled: !externalXrayReady, label: externalXrayReady ? item.label : t('listener.externalXrayNotConfigured') }
              : item)}
            onChange={handleRuntimeModeChange}
          />
        </Form.Item>
        <Form.Item name="Protocol" label={t('connector.protocol')} tooltip={runtimeMode === 'tapx' ? t('listener.rawTransportHelp') : t('listener.xrayProtocolHelp')}><Select options={protocolOptions} onChange={handleProtocolChange} /></Form.Item>
        <Form.Item name="Name" label={t('connector.tag')} tooltip={t('connector.tagHelp')} rules={[{ required: true, message: t('connector.tagRequired') }]}>
          <Input placeholder={t('connector.uniqueTag')} />
        </Form.Item>

        {runtimeMode === 'tapx' ? tapxBasicFields() : xrayBasicFields()}
      </>
    );
  }

  function tapxBasicFields() {
    return (
      <>
        <Form.Item name="Remote" label={t('xray.address')} tooltip={t('connector.remoteHelp')} rules={[{ required: true, message: t('xray.addressRequired') }]}>
          <Input placeholder="edge.example.com / 203.0.113.10" />
        </Form.Item>
        <Form.Item name="Port" label={t('xray.port')} tooltip={t('connector.portHelp')} rules={[{ required: true, message: t('xray.portRequired') }]}>
          <InputNumber min={1} max={65535} placeholder="443" style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="VKey" label="vKey" tooltip={t('user.vkeyHelp')}><Input placeholder={t('user.vkeyPlaceholder')} /></Form.Item>
        <Form.Item name="Security" label={t('xray.security')} tooltip={t('connector.securityHelp')}>
          <Radio.Group buttonStyle="solid">
            <Radio.Button value="none">{t('xray.none')}</Radio.Button>
            {isTapxRawTcp ? <Radio.Button value="tls">TLS</Radio.Button> : null}
            {isTapxRawUdp ? <Radio.Button value="dtls">DTLS</Radio.Button> : null}
          </Radio.Group>
        </Form.Item>
        {isTapxRawTcp && tapxSecurity === 'tls' ? <TapxConnectorTlsFields /> : null}
        {isTapxRawUdp && tapxSecurity === 'dtls' ? <TapxConnectorDtlsFields /> : null}
        <TapxConnectorFastPathFields tcp={isTapxRawTcp} />
      </>
    );
  }

  function xrayBasicFields() {
    return (
      <>
        <Form.Item name="SendThrough" label={t('connector.sendThrough')} tooltip={t('connector.sendThroughHelp')}><Input placeholder="192.0.2.10" /></Form.Item>
        <Form.Item
          name="DomainStrategy"
          label={t('connector.targetStrategy')}
          tooltip={t('connector.targetStrategyHelp')}
        >
          <Select allowClear placeholder="AsIs" options={targetStrategyOptions} />
        </Form.Item>
        <XrayOutboundProtocolFields form={form} protocol={protocol} />
        {protocol === 'vless' && reverseTag ? (
          <SniffingFields form={form} name={['settings', 'reverseSniffing']} label={t('connector.reverseSniffing')} />
        ) : null}
        {streamEnabled && streamNetwork ? (
          <>
            <XrayTransportFields
              form={form}
              direction="outbound"
              protocol={protocol}
              network={streamNetwork}
              outboundTags={outboundTags}
              includeExtras={false}
            />
            {tlsFlowAllowed ? (
              <Form.Item label="Flow" name={['settings', 'flow']}>
                <Select
                  allowClear
                  placeholder={t('xray.none')}
                  options={[{ value: '', label: t('xray.none') }, { value: 'xtls-rprx-vision', label: 'xtls-rprx-vision' }]}
                />
              </Form.Item>
            ) : null}
            {tlsFlowAllowed && flow === 'xtls-rprx-vision' ? (
              <>
                <Form.Item label="Vision testpre" name={['settings', 'testpre']}>
                  <InputNumber min={0} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item label="Vision testseed">
                  <Space.Compact block>
                    {[900, 500, 900, 256].map((value, index) => (
                      <Form.Item key={index} name={['settings', 'testseed', index]} noStyle>
                        <InputNumber min={1} placeholder={String(value)} style={{ width: '25%' }} />
                      </Form.Item>
                    ))}
                  </Space.Compact>
                </Form.Item>
              </>
            ) : null}
            <XraySecurityFields form={form} direction="outbound" protocol={protocol} network={streamNetwork} security={streamSecurity} />
          </>
        ) : null}
        <SockoptFields form={form} direction="outbound" network={streamNetwork} outboundTags={outboundTags} />
        <FinalMaskFields name={['streamSettings', 'finalmask']} network={streamNetwork} protocol={protocol} />
        <XrayMuxFields form={form} protocol={protocol} network={streamNetwork} />
      </>
    );
  }

  function bindingTab() {
    return (
      <EndpointBindingFields
        bindingEnabled={deviceBindingEnabled}
        bindMode={bindMode}
        linkAutoOptimize={linkAutoOptimize}
        addressConfigEnabled={addressConfigEnabled}
        addressAssignMode={addressAssignMode}
        interfaceType={interfaceType}
        role="connector"
        deviceOptions={deviceOptions}
        addressPlaceholders={{ ipv4: '10.10.0.2/24', ipv6: 'fd00:10:10::2/64', gateway: '10.10.0.1' }}
      />
    );
  }

  function jsonTab() {
    return (
      <Space orientation="vertical" size={10} style={{ width: '100%', marginTop: 10 }}>
        <Input.Search
          placeholder="vmess:// vless:// trojan:// ss:// hysteria2:// wireguard:// raw://"
          enterButton={t('connector.importAction')}
          onSearch={importLink}
          onChange={(event) => form.setFieldValue('ImportLink', event.target.value)}
        />
        <Form.Item name="JSONText" label="JSON">
          <Input.TextArea rows={14} spellCheck={false} onChange={() => setJsonDirty(true)} />
        </Form.Item>
      </Space>
    );
  }

  function changeFormTab(key: string) {
    if (key === 'json' && !jsonDirty) {
      const draft = form.getFieldsValue(true) as ConnectorRecord;
      const { JSONText: _jsonText, ImportLink: _importLink, ...serializable } = draft;
      form.setFieldValue('JSONText', JSON.stringify(serializable, null, 2));
    }
    setActiveFormTab(key);
  }
}

function normalizeForSave(record: ConnectorRecord): ConnectorRecord {
  const runtime = record.RuntimeMode || 'embedded-xray';
  const stream = runtime === 'tapx' ? undefined : (record.streamSettings || (canEnableStream(record.Protocol || '') ? newStreamSlice(record.Network || 'tcp') : undefined));
  const settings = record.settings || {};
  const tls = record.TLS || {};
  const rawUDP = record.RawUDP || {};
  const rawTCP = record.RawTCP || {};
  const now = nowSecond();
  return {
    ...record,
    RuntimeMode: runtime,
    Transport: runtime === 'tapx' ? (record.Protocol === 'raw-udp' ? 'udp' : 'tcp') : 'xray',
    Network: runtime === 'tapx' ? (record.Protocol === 'raw-udp' ? 'udp' : 'tcp') : String(stream?.network || record.Network || 'tcp'),
    Security: runtime === 'tapx' ? record.Security || 'none' : String(stream?.security || 'none'),
    Remote: runtime === 'tapx' || record.Protocol === 'wireguard' ? record.Remote : String(settings.address || record.Remote || ''),
    Port: runtime === 'tapx' ? record.Port : Number(settings.port || record.Port || 0),
    settings,
    streamSettings: stream,
    RawUDP: runtime === 'tapx' && record.Protocol === 'raw-udp' ? {
      ...rawUDP,
      Workers: numberValue(rawUDP.Workers),
      QueueSize: numberValue(rawUDP.QueueSize),
      ZeroCopy: booleanValue(rawUDP.ZeroCopy),
      ConnectTimeout: numberValue(rawUDP.ConnectTimeout),
      IdleTimeout: numberValue(rawUDP.IdleTimeout),
      DTLS: {
        ...rawUDP.DTLS,
        Enabled: record.Security === 'dtls',
        ServerName: stringValue(tls.ServerName, rawUDP.DTLS?.ServerName),
        MinVersion: stringValue(tls.MinVersion, rawUDP.DTLS?.MinVersion),
        MaxVersion: stringValue(tls.MaxVersion, rawUDP.DTLS?.MaxVersion),
        AllowInsecure: booleanValue(tls.AllowInsecure, rawUDP.DTLS?.AllowInsecure),
        MTU: numberValue(tls.DtlsMtu, rawUDP.DTLS?.MTU),
        ReplayWindow: numberValue(tls.DtlsReplayWindow, rawUDP.DTLS?.ReplayWindow),
      },
    } : rawUDP,
    RawTCP: runtime === 'tapx' && record.Protocol === 'raw-tcp' ? {
      ...rawTCP,
      LengthMode: resolveTcpLengthMode({ mode: rawTCP.LengthMode }),
      NoDelay: booleanValue(rawTCP.NoDelay),
      KeepAliveSecond: numberValue(rawTCP.KeepAliveSecond),
      QueueSize: numberValue(rawTCP.QueueSize),
      ZeroCopy: booleanValue(rawTCP.ZeroCopy),
      IdleTimeout: numberValue(rawTCP.IdleTimeout),
      Workers: numberValue(rawTCP.Workers),
      ConnectTimeout: numberValue(rawTCP.ConnectTimeout),
      TLS: {
        ...rawTCP.TLS,
        Enabled: record.Security === 'tls',
        ServerName: stringValue(tls.ServerName, rawTCP.TLS?.ServerName),
        MinVersion: stringValue(tls.MinVersion, rawTCP.TLS?.MinVersion),
        MaxVersion: stringValue(tls.MaxVersion, rawTCP.TLS?.MaxVersion),
        AllowInsecure: booleanValue(tls.AllowInsecure, rawTCP.TLS?.AllowInsecure),
      },
    } : rawTCP,
    Binding: normalizeBinding(record.Binding),
    CreatedAt: record.CreatedAt || now,
    UpdatedAt: now,
  };
}

function buildConnectorXrayProfile(record: ConnectorRecord): TapxXrayProfile & NodeOwned {
  const profileID = record.XrayProfileID || `xray-${record.ID}`;
  const protocol = record.Protocol || 'freedom';
  const streamSettings = outboundStreamToWire(record.streamSettings || {});
  const sockopt = streamSettings.sockopt;
  delete streamSettings.sockopt;
  const mux = record.mux?.enabled === true ? record.mux : undefined;

  return {
    ManagedNodeID: record.ManagedNodeID,
    ID: profileID,
    Enabled: record.Enabled !== false,
    Name: record.Name || record.ID,
    Runtime: record.RuntimeMode === 'external-xray' ? 'external' : 'embedded',
    OutboundProtocol: protocol,
    OutboundSettingsJSON: JSON.stringify(outboundSettingsToWire(protocol, record.settings || {})),
    SendThrough: record.SendThrough || '',
    TargetStrategy: record.DomainStrategy || '',
    Network: record.Network || String(streamSettings.network || ''),
    Security: record.Security || String(streamSettings.security || 'none'),
    StreamSettingsJSON: Object.keys(streamSettings).length > 0 ? JSON.stringify(streamSettings) : '',
    MuxJSON: mux ? JSON.stringify(mux) : '',
    SockoptJSON: sockopt && typeof sockopt === 'object' ? JSON.stringify(sockopt) : '',
    Remark: `tapx:connector-profile:${record.Name || record.ID}`,
  };
}

function defaultStreamForProtocol(protocol: string): Record<string, unknown> | undefined {
  if (protocol === 'wireguard') return { security: 'none' };
  if (protocol === 'hysteria') return newStreamSlice('hysteria');
  if (!canEnableStream(protocol)) return undefined;
  return newStreamSlice('tcp');
}

// The Go API deliberately stores the runtime model, not UI-only labels such as
// RuntimeMode and Protocol. Reconstruct those labels from Transport after each
// API read so a persisted raw endpoint never renders as an Xray endpoint.
function hydrateConnectorConfig(config: RuntimeConfig): RuntimeConfig {
  const profiles = new Map((config.XrayProfiles || []).map((profile) => [`${nodeIDOf(profile)}:${profile.ID}`, profile]));
  const vkeys = new Map((config.VKeys || []).map((vkey) => [`${nodeIDOf(vkey)}:${vkey.ID}`, vkey]));
  const devices = new Map((config.Devices || []).map((device) => [`${nodeIDOf(device)}:${device.ID}`, device]));
  return {
    ...config,
    Connectors: (config.Connectors || []).map((item) => hydrateConnector(
      item as ConnectorRecord,
      profiles.get(`${nodeIDOf(item)}:${item.XrayProfileID || ''}`),
      vkeys.get(`${nodeIDOf(item)}:${item.Binding?.VKeyID || ''}`),
      devices.get(`${nodeIDOf(item)}:${item.Binding?.DeviceID || ''}`),
    )),
  };
}

function hydrateConnector(record: ConnectorRecord, profile?: TapxXrayProfile, vkey?: TapxVKey, device?: TapxDevice): ConnectorRecord {
  const binding = hydrateSavedDeviceBinding(record.Binding, device);
  if (record.Transport === 'xray' && profile) {
    const protocol = profile.OutboundProtocol || 'freedom';
    const settings = outboundSettingsFromWire(protocol, parseObjectJSON(profile.OutboundSettingsJSON));
    const streamSettings = outboundStreamFromWire(parseObjectJSON(profile.StreamSettingsJSON));
    const sockopt = parseObjectJSON(profile.SockoptJSON);
    if (Object.keys(sockopt).length > 0) streamSettings.sockopt = sockopt;
    const endpoint = protocol === 'wireguard'
      ? splitEndpoint(String(((settings.peers as Array<{ endpoint?: string }> | undefined)?.[0]?.endpoint) || ''))
      : { host: String(settings.address || record.Remote || ''), port: Number(settings.port || record.Port || 0) };
    return {
      ...record,
      RuntimeMode: profile.Runtime === 'external' ? 'external-xray' : 'embedded-xray',
      Protocol: protocol,
      SendThrough: profile.SendThrough || '',
      DomainStrategy: profile.TargetStrategy || '',
      Network: profile.Network || String(streamSettings.network || ''),
      Security: profile.Security || String(streamSettings.security || 'none'),
      Remote: endpoint.host || record.Remote,
      Port: endpoint.port || record.Port,
      settings,
      streamSettings: Object.keys(streamSettings).length > 0 ? streamSettings : undefined,
      mux: parseObjectJSON(profile.MuxJSON),
      Binding: binding,
    };
  }
  if (record.Transport !== 'udp' && record.Transport !== 'tcp') return { ...record, Binding: binding };

  const rawUDP = record.RawUDP;
  const rawTCP = record.RawTCP;
  const isUDP = record.Transport === 'udp';
  const security = isUDP
    ? (rawUDP?.DTLS?.Enabled ? 'dtls' : 'none')
    : (rawTCP?.TLS?.Enabled ? 'tls' : 'none');
  const tls = isUDP ? rawUDP?.DTLS : rawTCP?.TLS;

  return {
    ...record,
    RuntimeMode: 'tapx',
    Protocol: isUDP ? 'raw-udp' : 'raw-tcp',
    Network: isUDP ? 'udp' : 'tcp',
    Security: security,
    VKey: vkey?.Value || '',
    TLS: tls ? {
      ServerName: tls.ServerName,
      MinVersion: tls.MinVersion,
      MaxVersion: tls.MaxVersion,
      AllowInsecure: tls.AllowInsecure,
      DtlsMtu: rawUDP?.DTLS?.MTU,
      DtlsReplayWindow: rawUDP?.DTLS?.ReplayWindow,
    } : undefined,
    Binding: binding,
  };
}

function normalizeBinding(binding: ConnectorRecord['Binding']): ConnectorBinding {
  return normalizeDeviceBinding(binding, { mode: 'autoCreate', addressMode: 'auto' });
}

function integrationConnectorRecord(
  draft: WireguardIntegrationDraft,
  current: ConnectorRecord | undefined,
  connectors: ConnectorRecord[],
  managedNodeID: string,
): { record: ConnectorRecord; profile: TapxXrayProfile & NodeOwned } {
  const endpoint = draft.settings.peers[0]?.endpoint || '';
  const remote = splitEndpoint(endpoint);
  const nodeConnectors = connectors.filter((item) => nodeIDOf(item) === managedNodeID);
  const id = current?.ID || nextId('connector', new Set(nodeConnectors.map((item) => item.ID)));
  const profileID = current?.XrayProfileID || `xray-${id}`;
  const record = normalizeForSave({
    ...current,
    ID: id,
    ManagedNodeID: managedNodeID,
    Enabled: true,
    Name: draft.tag,
    Remote: remote.host,
    Port: remote.port,
    Transport: 'xray',
    XrayProfileID: profileID,
    RuntimeMode: 'embedded-xray',
    Protocol: 'wireguard',
    Network: '',
    Security: 'none',
    SendThrough: '',
    DomainStrategy: '',
    settings: draft.settings as unknown as Record<string, unknown>,
    streamSettings: undefined,
    mux: undefined,
    Binding: undefined,
    UpdatedAt: nowSecond(),
    CreatedAt: current?.CreatedAt || nowSecond(),
  });
  return {
    record,
    profile: {
      ID: profileID,
      ManagedNodeID: managedNodeID,
      Enabled: true,
      Name: draft.tag,
      Runtime: 'embedded',
      OutboundProtocol: 'wireguard',
      OutboundSettingsJSON: JSON.stringify(draft.settings),
      Network: '',
      Security: 'none',
      StreamSettingsJSON: '',
      Remark: `tapx:integration-connector:${draft.tag}`,
    },
  };
}

function splitEndpoint(endpoint: string): { host: string; port: number } {
  if (endpoint.startsWith('[')) {
    const end = endpoint.indexOf(']');
    if (end > 0) {
      return {
        host: endpoint.slice(1, end),
        port: Number(endpoint.slice(end + 2)) || 0,
      };
    }
  }
  const separator = endpoint.lastIndexOf(':');
  if (separator < 0) return { host: endpoint, port: 0 };
  return {
    host: endpoint.slice(0, separator),
    port: Number(endpoint.slice(separator + 1)) || 0,
  };
}

function parseImportLink(link: string, connectors: ConnectorRecord[], t: ReturnType<typeof useI18n>['t'], managedNodeID: string): ConnectorRecord {
  const url = new URL(link);
  const protocol = url.protocol.replace(':', '').toLowerCase();
  const id = nextId('connector', new Set(connectors.filter((item) => nodeIDOf(item) === managedNodeID).map((item) => item.ID)));
  const base: ConnectorRecord = {
    ...defaultConnector,
    ID: id,
    ManagedNodeID: managedNodeID,
    Name: decodeURIComponent(url.hash.replace(/^#/, '')) || id,
    Remote: url.hostname,
    Port: Number(url.port || 0) || defaultConnector.Port,
  };

  if (protocol === 'raw') {
    const raw = parseRawConnectorLink(link, t);
    if (!raw) throw new Error(t('connector.invalidRawLink'));
    return {
      ...base,
      Name: raw.name || base.Name,
      RuntimeMode: 'tapx',
      Protocol: raw.protocol,
      Remote: raw.address,
      Port: raw.port,
      Network: raw.protocol === 'raw-tcp' ? 'tcp' : 'udp',
      Transport: raw.protocol === 'raw-tcp' ? 'tcp' : 'udp',
      Security: raw.security,
      VKey: raw.vkey,
      RawTCP: raw.protocol === 'raw-tcp' ? {
        ...defaultTapxConnectorFields().RawTCP,
        LengthMode: raw.lengthMode || 'uint16',
      } : undefined,
      RawUDP: raw.protocol === 'raw-udp' ? {
        ...defaultTapxConnectorFields().RawUDP,
      } : undefined,
      TLS: {
        ServerName: raw.serverName,
      },
      streamSettings: undefined,
    };
  }

  const parsed = parseOutboundLink(link);
  if (!parsed) throw new Error(t('connector.unsupportedLink'));
  const stream = parsed.streamSettings;
  return {
    ...base,
    Name: parsed.name || base.Name,
    RuntimeMode: 'embedded-xray',
    Protocol: parsed.protocol,
    Transport: 'xray',
    Remote: parsed.address,
    Port: parsed.port,
    Network: String(stream?.network || ''),
    Security: String(stream?.security || 'none'),
    settings: parsed.settings,
    streamSettings: stream,
  };
}

function normalizeImportedConnectors(value: string, existing: ConnectorRecord[], managedNodeID: string): ConnectorRecord[] {
  const parsed = JSON.parse(value) as unknown;
  const input = Array.isArray(parsed)
    ? parsed
    : parsed && typeof parsed === 'object' && 'Connectors' in parsed && Array.isArray((parsed as RuntimeConfig).Connectors)
      ? (parsed as RuntimeConfig).Connectors || []
      : [];
  const used = new Set(existing.filter((item) => nodeIDOf(item) === managedNodeID).map((item) => item.ID));
  return input.map((item, index) => {
    const row = item as Partial<ConnectorRecord>;
    const baseId = row.ID || row.Name || `connector-import-${index + 1}`;
    const id = uniqueId(safeId(String(baseId)), used);
    used.add(id);
    return normalizeForSave({
      ...defaultConnector,
      ...row,
      ID: id,
      ManagedNodeID: managedNodeID,
      Name: row.Name || id,
    });
  });
}

function endpointAddress(record: ConnectorRecord): string {
  const host = record.Remote || String(record.settings?.address || '');
  const port = record.Port || Number(record.settings?.port || 0);
  if (!host) return '-';
  return port ? `${host}:${port}` : host;
}

function ThroughputGauge({ label, direction, value, scale }: { label: string; direction: ThroughputDirection; value: number; scale: number }) {
  const progress = speedGaugeProgress(value, scale);
  const needleAngle = -130 + progress * 260;
  const needle = gaugeNeedlePoints(130, 124, 68, needleAngle);
  const marks = speedGaugeMarks(scale);
  const valueMbps = value / 1_000_000;
  const speedGradientID = `throughput-speed-gradient-${direction}`;
  return (
    <div className="throughput-gauge">
      <svg className="throughput-dial" viewBox="0 0 260 222" role="meter" aria-label={label} aria-valuemin={0} aria-valuemax={scale} aria-valuenow={value}>
        <defs>
          <linearGradient id={speedGradientID} x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stopColor={`var(--speed-${direction}-start)`} />
            <stop offset="48%" stopColor={`var(--speed-${direction}-middle)`} />
            <stop offset="100%" stopColor={`var(--speed-${direction}-end)`} />
          </linearGradient>
          <linearGradient id="throughput-needle-gradient" x1="0" y1="1" x2="1" y2="0">
            <stop offset="0%" stopColor="var(--speed-needle-start)" />
            <stop offset="100%" stopColor="var(--speed-needle-end)" />
          </linearGradient>
        </defs>
        <path d={gaugeArcPath(130, 124, 96, -130, 130)} className="throughput-dial-track" />
        <path
          d={gaugeArcPath(130, 124, 96, -130, needleAngle)}
          className="throughput-dial-spectrum"
          stroke={`url(#${speedGradientID})`}
        />
        {marks.map((mark, index) => {
          const angle = -130 + index / (marks.length - 1) * 260;
          const tickOuter = polarPoint(130, 124, 84, angle);
          const tickInner = polarPoint(130, 124, 79, angle);
          const textPoint = polarPoint(130, 124, 69, angle);
          const active = index / (marks.length - 1) <= progress;
          return <g key={`${mark}-${index}`}>
            <line x1={tickInner.x} y1={tickInner.y} x2={tickOuter.x} y2={tickOuter.y} className={`throughput-tick major${active ? ' active' : ''}`} />
            <text x={textPoint.x} y={textPoint.y + 3} textAnchor="middle" className={`throughput-mark-label${active ? ' active' : ''}`}>{formatGaugeMark(mark)}</text>
          </g>;
        })}
        <polygon points={needle} className="throughput-needle" />
        <text x="130" y="178" textAnchor="middle" className="throughput-dial-number">{formatGaugeSpeed(valueMbps)}</text>
        <text x="130" y="200" textAnchor="middle" className={`throughput-dial-label ${direction}`}>
          {direction === 'upload' ? '↑' : '↓'} Mbps
        </text>
      </svg>
    </div>
  );
}

function polarPoint(cx: number, cy: number, radius: number, angle: number) {
  const radians = (angle - 90) * Math.PI / 180;
  return { x: cx + radius * Math.cos(radians), y: cy + radius * Math.sin(radians) };
}

function gaugeArcPath(cx: number, cy: number, radius: number, startAngle: number, endAngle: number) {
  const start = polarPoint(cx, cy, radius, startAngle);
  const end = polarPoint(cx, cy, radius, endAngle);
  const largeArc = endAngle - startAngle <= 180 ? 0 : 1;
  return `M ${start.x} ${start.y} A ${radius} ${radius} 0 ${largeArc} 1 ${end.x} ${end.y}`;
}

function gaugeNeedlePoints(cx: number, cy: number, length: number, angle: number) {
  const radians = (angle - 90) * Math.PI / 180;
  const dx = Math.cos(radians);
  const dy = Math.sin(radians);
  const px = -dy;
  const py = dx;
  const baseX = cx;
  const baseY = cy;
  const tipX = cx + dx * length;
  const tipY = cy + dy * length;
  return [
    `${baseX + px * 5},${baseY + py * 5}`,
    `${tipX + px * 2},${tipY + py * 2}`,
    `${tipX - px * 2},${tipY - py * 2}`,
    `${baseX - px * 5},${baseY - py * 5}`,
  ].join(' ');
}

function speedGaugeMarks(scale: number) {
  const scaleMbps = scale / 1_000_000;
  return [0, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 0.75, 1].map((ratio) => scaleMbps * ratio);
}

function speedGaugeProgress(value: number, scale: number) {
  const valueMbps = Math.max(0, value / 1_000_000);
  const marks = speedGaugeMarks(scale);
  if (valueMbps >= marks.at(-1)!) return 1;
  const upperIndex = marks.findIndex((mark) => valueMbps <= mark);
  if (upperIndex <= 0) return 0;
  const lower = marks[upperIndex - 1];
  const upper = marks[upperIndex];
  return ((upperIndex - 1) + (valueMbps - lower) / Math.max(0.0001, upper - lower)) / (marks.length - 1);
}

function formatGaugeMark(valueMbps: number) {
  if (valueMbps > 1000) return `${Number((valueMbps / 1000).toFixed(1))}G`;
  if (valueMbps >= 10) return valueMbps.toFixed(0);
  if (valueMbps >= 1) return Number(valueMbps.toFixed(1));
  return Number(valueMbps.toFixed(2));
}

function formatGaugeSpeed(valueMbps: number) {
  if (!Number.isFinite(valueMbps) || valueMbps <= 0) return '0.00';
  if (valueMbps >= 1000) return valueMbps.toFixed(1);
  return valueMbps.toFixed(2);
}

function ThroughputHistoryChart({ samples, result, uploadLabel, downloadLabel, durationLabel, uploadDataLabel, downloadDataLabel }: {
  samples: ThroughputSample[];
  result: ConnectorTestResult;
  uploadLabel: string;
  downloadLabel: string;
  durationLabel: string;
  uploadDataLabel: string;
  downloadDataLabel: string;
}) {
  const renderSeries = (direction: ThroughputDirection, label: string, value: number) => {
    const series = samples.filter((sample) => sample.direction === direction);
    const width = 560;
    const height = 84;
    const inset = 4;
    const maximum = Math.max(1, ...series.map((sample) => sample.bps));
    const points = series.map((sample, index) => {
      const x = inset + index / Math.max(1, series.length - 1) * (width - inset * 2);
      const y = height - inset - sample.bps / maximum * (height - inset * 2);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    }).join(' ');
    const area = points ? `${inset},${height - inset} ${points} ${width - inset},${height - inset}` : '';
    return (
      <div className={`throughput-result-row ${direction}`}>
        <div className="throughput-result-value">
          <span>{label}/Mbps</span>
          <strong>{formatGaugeSpeed(value / 1_000_000)}</strong>
        </div>
        <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${label} throughput history`}>
          <defs>
            <linearGradient id={`throughput-area-${direction}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={`var(--speed-${direction}-end)`} stopOpacity="0.28" />
              <stop offset="100%" stopColor={`var(--speed-${direction}-end)`} stopOpacity="0" />
            </linearGradient>
          </defs>
          {[0.25, 0.5, 0.75].map((ratio) => <line key={ratio} x1="0" x2={width} y1={height * ratio} y2={height * ratio} className="throughput-grid" />)}
          {area && <polygon points={area} fill={`url(#throughput-area-${direction})`} />}
          {points && <polyline points={points} className={`throughput-line ${direction}`} />}
        </svg>
      </div>
    );
  };
  return (
    <div className="throughput-result">
      {renderSeries('download', downloadLabel, result.downloadBps || 0)}
      {renderSeries('upload', uploadLabel, result.uploadBps || 0)}
      <div className="throughput-result-meta">
        <span>{downloadDataLabel}<strong>{formatBytes(result.downloadBytes || 0)}</strong></span>
        <span>{uploadDataLabel}<strong>{formatBytes(result.uploadBytes || 0)}</strong></span>
        <span>{durationLabel}<strong>{result.durationMs ? `${(result.durationMs / 1000).toFixed(1)} s` : '-'}</strong></span>
      </div>
    </div>
  );
}

function throughputGaugeScale(value: number): number {
  const scales = [10_000_000, 100_000_000, 1_000_000_000, 2_500_000_000, 10_000_000_000, 100_000_000_000];
  return scales.find((scale) => value <= scale) || Math.max(value, scales[scales.length - 1]);
}

function formatBitRate(bitsPerSecond: number): string {
  if (!Number.isFinite(bitsPerSecond) || bitsPerSecond <= 0) return '0 bit/s';
  const units = ['bit/s', 'Kbit/s', 'Mbit/s', 'Gbit/s', 'Tbit/s'];
  let value = bitsPerSecond;
  let index = 0;
  while (value >= 1000 && index < units.length - 1) {
    value /= 1000;
    index += 1;
  }
  return `${value >= 100 ? value.toFixed(0) : value.toFixed(2)} ${units[index]}`;
}

function connectorProtocolLabel(protocol: string) {
  if (protocol === 'raw-tcp') return 'Raw TCP';
  if (protocol === 'raw-udp') return 'Raw UDP';
  return protocol;
}
