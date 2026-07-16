import { useEffect, useMemo, useRef, useState } from 'react';
import dayjs, { type Dayjs } from 'dayjs';
import {
  Button,
  Card,
  DatePicker,
  Dropdown,
  Form,
  Input,
  InputNumber,
  Modal,
  Radio,
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
  ArrowDownOutlined,
  ArrowUpOutlined,
  CheckCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  ExportOutlined,
  ImportOutlined,
  LinkOutlined,
  BarsOutlined,
  MenuOutlined,
  MoreOutlined,
  PieChartOutlined,
  PlusOutlined,
  QuestionCircleOutlined,
  RetweetOutlined,
  StopOutlined,
  VerticalAlignTopOutlined,
} from '@ant-design/icons';
import {
  resetListenerTraffic,
  type RuntimeConfig,
  type TapxDevice,
  type TapxEndpoint,
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
import { copyText } from '../shared/clipboard';
import { TapxListenerDtlsFields, TapxListenerFastPathFields, TapxListenerTlsFields } from '../features/endpoints/TapxEndpointFields';
import { EndpointBindingFields } from '../features/endpoints/EndpointBindingFields';
import { tapxProtocolOptions, type DeviceBindMode, type EndpointDeviceBinding, type EndpointRuntimeMode } from '../features/endpoints/endpoint-types';
import { resolveTcpLengthMode } from '../features/endpoints/tcpLengthMode';
import { stripTapxSocketOverrides } from '../features/endpoints/tapxRawSettings';
import { buildRawConnectorLink } from '../features/endpoints/rawConnectorLink';
import { DeviceTypeConflictError, deviceTypeConflictValues, hydrateSavedDeviceBinding, materializeEndpointAutoDevice, normalizeDeviceBinding } from '../features/endpoints/deviceBinding';
import { defaultInboundSettings, defaultTapxListenerFields } from '../features/xray/inbounds/defaults';
import { AdvancedInboundAllEditor, AdvancedInboundSliceEditor } from '../features/xray/inbounds/AdvancedInboundEditors';
import { FallbacksFields } from '../features/xray/inbounds/FallbacksFields';
import {
  inboundSettingsFromWire,
  inboundSettingsToWire,
  inboundStreamFromWire,
  inboundStreamToWire,
  restoreInboundFallbacks,
  takeInboundFallbacks,
} from '../features/xray/inbounds/profileAdapter';
import {
  XrayInboundProtocolFields,
  XraySecurityFields,
  XrayTransportFields,
  canEnableStream,
  inboundXrayProtocolOptions,
  newInboundStreamSlice,
  newInboundHysteriaStreamSlice,
  shouldShowInboundProtocolTab,
} from '../features/xray/XrayFormFields';
import { labelDevice, nextId } from '../shared/tapx-model';
import { settingsToObject } from '../shared/settings';
import { isManagedLinkAddressRemark } from '../shared/managed-objects';
import { randomInteger } from '../shared/random';
import { parseJSON, parseObjectJSON } from '../shared/json';
import { removeUnusedXrayProfiles, upsertXrayProfile } from '../shared/xray-profiles';
import { booleanValue, numberValue, stringValue } from '../shared/values';
import { uniqueID as uniqueId } from '../shared/ids';
import { formatBytes } from '../shared/format';
import { useI18n } from '../i18n/I18nProvider';
import './ListenerPage.css';

type RuntimeMode = EndpointRuntimeMode;
type SecurityMode = 'none' | 'tls' | 'reality' | 'dtls';

type ExportModalState = {
  open: boolean;
  title: string;
  value: string;
};

type ListenerStats = {
  rxBytes: number;
  txBytes: number;
  rxBytesPerSecond: number;
  txBytesPerSecond: number;
};

type ListenerRecord = TapxEndpoint & NodeOwned & {
  RuntimeMode?: RuntimeMode;
  Protocol?: string;
  Network?: string;
  Security?: SecurityMode | string;
  settings?: Record<string, unknown>;
  streamSettings?: Record<string, unknown>;
  mux?: Record<string, unknown>;
  sniffing?: Record<string, unknown>;
  FastPath?: Record<string, unknown>;
  TLS?: Record<string, unknown>;
  ShareAddressStrategy?: 'listen' | 'custom';
  ShareAddress?: string;
  TotalTraffic?: number;
  TrafficReset?: string;
  ExpireAt?: Dayjs | string | null;
  Binding?: EndpointDeviceBinding;
};

function labelWithHint(label: string, hint: string) {
  return (
    <span>
      {label}
      <Tooltip title={hint}>
        <QuestionCircleOutlined style={{ marginInlineStart: 4, color: 'rgba(128,128,128,0.65)' }} />
      </Tooltip>
    </span>
  );
}

const defaultListener: ListenerRecord = {
  ...defaultTapxListenerFields(),
  ID: '',
  Enabled: true,
  Name: '',
  BindHost: '',
  BindPort: 10000,
  RuntimeMode: 'embedded-xray',
  Protocol: 'vless',
  Transport: 'xray',
  Network: 'tcp',
  Security: 'none',
  settings: defaultInboundSettings('vless'),
  streamSettings: newInboundStreamSlice('tcp'),
  ShareAddressStrategy: 'listen',
  TotalTraffic: 0,
  TrafficReset: 'never',
  ExpireAt: null,
  Binding: {
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

export function ListenerPage() {
  const { t } = useI18n();
  const [config, setConfig] = useState<RuntimeConfig>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<ListenerRecord | null>(null);
  const [search, setSearch] = useState('');
  const [selectedRowKeys, setSelectedRowKeys] = useState<string[]>([]);
  const [importOpen, setImportOpen] = useState(false);
  const [importText, setImportText] = useState('');
  const [importTargetNodeID, setImportTargetNodeID] = useState('local');
  const [exportModal, setExportModal] = useState<ExportModalState>({ open: false, title: '', value: '' });
  const [listenerStats, setListenerStats] = useState<Record<string, ListenerStats>>({});
  const previousStats = useRef<{ at: number; counters: Record<string, { rxBytes: number; txBytes: number }> }>({ at: 0, counters: {} });
  const [messageApi, messageContextHolder] = message.useMessage();
  const { nodes, scope, setScope } = useNodeScope();
  const nodeTargetOptions = useNodeTargetOptions(nodes);
  const [form] = Form.useForm<ListenerRecord>();
  const runtimeOptions = useMemo(() => [
    { value: 'embedded-xray', label: t('listener.embeddedXray') },
    { value: 'external-xray', label: t('listener.externalXray') },
    { value: 'tapx', label: 'TapX' },
  ], [t]);

  const listeners = useMemo(() => ((config.Listeners || []) as ListenerRecord[]), [config.Listeners]);
  const devices = useMemo(() => ((config.Devices || []) as TapxDevice[]), [config.Devices]);
  const clients = useMemo(() => config.Clients || [], [config.Clients]);
  const scopedListeners = useMemo(() => filterNodeOwned(listeners, scope), [listeners, scope]);
  const filteredListeners = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return scopedListeners.filter((item) => !needle || [
      item.ID,
      item.Name,
      item.Protocol,
      item.Transport,
      item.BindHost,
      String(item.BindPort || ''),
      item.Binding?.DeviceID,
      item.Binding?.DeviceName,
    ].some((value) => String(value || '').toLowerCase().includes(needle)));
  }, [scopedListeners, search]);
  const selectedListeners = useMemo(() => listeners.filter((item) => selectedRowKeys.includes(nodeObjectKey(item))), [listeners, selectedRowKeys]);
  useEffect(() => {
    const visibleKeys = new Set(scopedListeners.map(nodeObjectKey));
    setSelectedRowKeys((current) => current.filter((key) => visibleKeys.has(key)));
  }, [scopedListeners]);
  const panelCertificate = useMemo(() => panelCertificateFromSettings(config.Settings), [config.Settings]);
  const externalXrayReady = useMemo(() => {
    const stored = settingsToObject<{ externalXrayEnabled?: boolean; externalXrayPath?: string }>(config.Settings);
    return stored.externalXrayEnabled === true && Boolean(stored.externalXrayPath);
  }, [config.Settings]);
  const panelPageSize = useMemo(() => {
    const stored = settingsToObject<{ pageSize?: number }>(config.Settings);
    const value = Number(stored.pageSize ?? 10);
    return Number.isFinite(value) && value >= 0 ? Math.floor(value) : 10;
  }, [config.Settings]);
  const listenerTotals = useMemo(() => Object.values(listenerStats).reduce((total, item) => ({
    rxBytes: total.rxBytes + item.rxBytes,
    txBytes: total.txBytes + item.txBytes,
  }), { rxBytes: 0, txBytes: 0 }), [listenerStats]);

  const runtimeMode = (Form.useWatch('RuntimeMode', form) || defaultListener.RuntimeMode) as RuntimeMode;
  const protocol = String(Form.useWatch('Protocol', form) || defaultListener.Protocol);
  const tapxSecurity = String(Form.useWatch('Security', form) || 'none');
  const streamNetwork = String(Form.useWatch(['streamSettings', 'network'], form) || 'tcp');
  const streamSecurity = String(Form.useWatch(['streamSettings', 'security'], form) || 'none');
  const bindMode = (Form.useWatch(['Binding', 'DeviceBindMode'], form) || 'autoCreate') as DeviceBindMode;
  const linkAutoOptimize = Form.useWatch(['Binding', 'LinkAutoOptimize'], form) === true;
  const interfaceType = (Form.useWatch(['Binding', 'InterfaceType'], form) || 'tun') as 'tun' | 'tap';
  const addressConfigEnabled = Form.useWatch(['Binding', 'AddressConfigEnabled'], form) === true;
  const addressAssignMode = (Form.useWatch(['Binding', 'AddressAssignMode'], form) || 'manual') as 'auto' | 'manual';
  const shareAddressStrategy = (Form.useWatch('ShareAddressStrategy', form) || 'listen') as 'listen' | 'custom';
  const targetNodeID = String(Form.useWatch('ManagedNodeID', form) || defaultTargetNodeID(scope));
  const xrayConnectorTags = useMemo(
    () => filterNodeOwned((config.Connectors || []) as Array<TapxEndpoint & NodeOwned>, targetNodeID)
      .filter((connector) => ((connector as ListenerRecord).RuntimeMode || 'embedded-xray') !== 'tapx')
      .map((connector) => connector.ID.trim())
      .filter(Boolean),
    [config.Connectors, targetNodeID],
  );

  const streamEnabled = runtimeMode !== 'tapx' && canEnableStream(protocol);
  const showProtocolTab = shouldShowInboundProtocolTab(runtimeMode, protocol, streamNetwork, streamSecurity);
  const showSecurityTab = runtimeMode === 'tapx' || (streamEnabled && !['wireguard', 'tunnel'].includes(protocol));
  const isTapxRawTcp = runtimeMode === 'tapx' && protocol === 'raw-tcp';
  const isTapxRawUdp = runtimeMode === 'tapx' && protocol === 'raw-udp';

  const deviceOptions = useMemo(() => filterNodeOwned(devices as Array<TapxDevice & NodeOwned>, targetNodeID)
    .filter((device) => device.Enabled !== false && (!interfaceType || device.Type === interfaceType))
    .map((device) => ({ value: device.ID, label: labelDevice(device) })), [devices, interfaceType, targetNodeID]);

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    setSelectedRowKeys((current) => current.filter((key) => scopedListeners.some((item) => nodeObjectKey(item) === key)));
  }, [scopedListeners]);

  useEffect(() => {
    let cancelled = false;
    let timer: number | undefined;
    const poll = async () => {
      try {
        const report = await getStats();
        if (cancelled) return;
        const now = Date.now();
        const elapsed = previousStats.current.at > 0 ? Math.max(0.001, (now - previousStats.current.at) / 1000) : 0;
        const nextCounters: Record<string, { rxBytes: number; txBytes: number }> = {};
        const nextStats: Record<string, ListenerStats> = {};
        for (const bucket of report.byEndpoint || []) {
          if (bucket.kind !== 'listener' && !bucket.id.startsWith('listener:')) continue;
          const id = bucket.name || bucket.id.replace(/^listener:/, '');
          const key = `${nodeIDOf(bucket)}:${id}`;
          const rxBytes = Number(bucket.counters?.rxBytes || 0);
          const txBytes = Number(bucket.counters?.txBytes || 0);
          const previous = previousStats.current.counters[key];
          nextCounters[key] = { rxBytes, txBytes };
          nextStats[key] = {
            rxBytes,
            txBytes,
            rxBytesPerSecond: elapsed > 0 && previous ? counterRate(previous.rxBytes, rxBytes, elapsed) : 0,
            txBytesPerSecond: elapsed > 0 && previous ? counterRate(previous.txBytes, txBytes, elapsed) : 0,
          };
        }
        previousStats.current = { at: now, counters: nextCounters };
        setListenerStats(nextStats);
      } catch {
        // Keep the last valid counters while the runtime is restarting.
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
      setConfig(hydrateListenerConfig(await getRuntimeConfig()));
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('listener.loadFailed'));
    } finally {
      setLoading(false);
    }
  }

  function openCreate() {
    const ids = new Set(listeners.map((item) => item.ID));
    const id = nextId('listener', ids);
    form.resetFields();
    form.setFieldsValue({
      ...defaultListener,
      ID: id,
      ManagedNodeID: defaultTargetNodeID(scope),
      Name: '',
      BindPort: nextPort(listeners),
      settings: defaultInboundSettings('vless'),
      streamSettings: newInboundStreamSlice('tcp'),
      Binding: {
        ...defaultListener.Binding,
        DeviceName: `tapx-tun-${listeners.length + 1}`,
      },
    });
    setEditing(null);
    setOpen(true);
  }

  function openEdit(record: ListenerRecord) {
    const runtime = record.RuntimeMode || 'embedded-xray';
    const streamSettings = runtime === 'tapx'
      ? undefined
      : (record.streamSettings || newInboundStreamSlice(record.Network || 'tcp'));
    form.resetFields();
    form.setFieldsValue({
      ...defaultListener,
      ...record,
      RuntimeMode: runtime,
      Protocol: record.Protocol || (runtime === 'tapx' ? 'raw-udp' : 'vless'),
      settings: {
        ...defaultInboundSettings(record.Protocol || (runtime === 'tapx' ? 'raw-udp' : 'vless')),
        ...(record.settings || {}),
      },
      streamSettings,
      Binding: { ...defaultListener.Binding, ...record.Binding },
    });
    setEditing(record);
    setOpen(true);
  }

  async function submit() {
    await form.validateFields();
    const values = form.getFieldsValue(true) as ListenerRecord;
    const id = values.ID || editing?.ID || nextId('listener', new Set(listeners.map((item) => item.ID)));
    const normalized = normalizeForSave({ ...defaultListener, ...editing, ...values, ID: id, Name: values.Name || '' });
    let next: ListenerRecord = normalized;
    if (next.RuntimeMode === 'external-xray' && !externalXrayReady) {
      messageApi.error(t('listener.externalXrayRequired'));
      return;
    }
    let materialized;
    try {
      materialized = materializeEndpointAutoDevice(next, devices, {
        role: 'listener', defaultMode: 'existing', defaultAddressMode: 'manual',
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
      const profile = buildListenerXrayProfile(next);
      next = { ...next, XrayProfileID: profile.ID };
      nextProfiles = upsertXrayProfile(nextProfiles, profile);
    } else if (editing?.XrayProfileID) {
      next = { ...next, XrayProfileID: '' };
      nextProfiles = removeUnusedXrayProfiles(nextProfiles, [editing.XrayProfileID], {
        listeners: listeners.filter((item) => !sameNodeObject(item, next)),
        connectors: config.Connectors || [],
      });
    }
    const index = listeners.findIndex((item) => sameNodeObject(item, next));
    const nextListeners = index < 0 ? [...listeners, next] : listeners.map((item) => (sameNodeObject(item, next) ? next : item));
    setSaving(true);
    try {
      const saved = await saveRuntimeConfig({
        ...config,
        Devices: materialized.devices,
        Listeners: nextListeners,
        XrayProfiles: nextProfiles,
      });
      setConfig(hydrateListenerConfig(saved));
      setOpen(false);
      try {
        await applyRuntimeConfig();
        messageApi.success(t('listener.saved'));
      } catch (applyError) {
        messageApi.warning(t('listener.applyFailed', { error: applyError instanceof Error ? applyError.message : String(applyError) }));
      }
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('listener.saveFailed'));
    } finally {
      setSaving(false);
    }
  }

  async function toggleEnable(record: ListenerRecord, enabled: boolean) {
    const nextListeners = listeners.map((item) => (sameNodeObject(item, record) ? { ...item, Enabled: enabled } : item));
    const nextProfiles = (config.XrayProfiles || []).map((profile) => (
      profile.ID === record.XrayProfileID && nodeIDOf(profile) === nodeIDOf(record) ? { ...profile, Enabled: enabled } : profile
    ));
    try {
      const saved = await saveRuntimeConfig({ ...config, Listeners: nextListeners, XrayProfiles: nextProfiles });
      setConfig(hydrateListenerConfig(saved));
      try {
        await applyRuntimeConfig();
      } catch (applyError) {
        messageApi.warning(t('listener.applyFailed', { error: applyError instanceof Error ? applyError.message : String(applyError) }));
      }
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('listener.saveGenericFailed'));
    }
  }

  async function setSelectedListenersEnabled(enabled: boolean) {
    if (selectedListeners.length === 0) return;
    const selected = new Set(selectedListeners.map(nodeObjectKey));
    const selectedProfiles = new Set(selectedListeners
      .filter((item) => item.XrayProfileID)
      .map((item) => `${nodeIDOf(item)}:${item.XrayProfileID}`));
    const nextListeners = listeners.map((item) => (
      selected.has(nodeObjectKey(item)) ? { ...item, Enabled: enabled } : item
    ));
    const nextProfiles = (config.XrayProfiles || []).map((profile) => (
      selectedProfiles.has(`${nodeIDOf(profile)}:${profile.ID}`) ? { ...profile, Enabled: enabled } : profile
    ));
    setSaving(true);
    try {
      const saved = await saveRuntimeConfig({ ...config, Listeners: nextListeners, XrayProfiles: nextProfiles });
      setConfig(hydrateListenerConfig(saved));
      await applyRuntimeConfig();
      setSelectedRowKeys([]);
      messageApi.success(enabled ? t('listener.batchEnabled') : t('listener.batchDisabled'));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('listener.saveGenericFailed'));
    } finally {
      setSaving(false);
    }
  }

  async function resetTraffic(records: ListenerRecord[]) {
    if (records.length === 0) return;
    setSaving(true);
    try {
      const resetByKey = new Map<string, TapxEndpoint>();
      for (const record of records) {
        const resetConfig = await resetListenerTraffic(record.ID, nodeIDOf(record));
        const reset = (resetConfig.Listeners || []).find((item) => item.ID === record.ID);
        if (reset) resetByKey.set(nodeObjectKey(record), reset);
      }
      setConfig((current) => ({
        ...current,
        Listeners: (current.Listeners || []).map((item) => {
          const reset = resetByKey.get(nodeObjectKey(item as ListenerRecord));
          return reset ? {
            ...item,
            TrafficResetAt: reset.TrafficResetAt,
            TrafficResetGeneration: reset.TrafficResetGeneration,
            TrafficRXOffset: reset.TrafficRXOffset,
            TrafficTXOffset: reset.TrafficTXOffset,
          } : item;
        }),
      }));
      setListenerStats((current) => ({
        ...current,
        ...Object.fromEntries(records.map((record) => [nodeObjectKey(record), {
          rxBytes: 0,
          txBytes: 0,
          rxBytesPerSecond: 0,
          txBytesPerSecond: 0,
        }])),
      }));
      setSelectedRowKeys([]);
      messageApi.success(t('listener.trafficReset'));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('listener.trafficResetFailed'));
    } finally {
      setSaving(false);
    }
  }

  async function moveListener(from: number, to: number) {
    if (to < 0 || to >= filteredListeners.length || from === to) return;
    const reordered = [...filteredListeners];
    const [record] = reordered.splice(from, 1);
    reordered.splice(to, 0, record);
    let visibleIndex = 0;
    const next = listeners.map((item) => (
      filteredListeners.some((visible) => sameNodeObject(visible, item)) ? reordered[visibleIndex++] : item
    ));
    try {
      const saved = await saveRuntimeConfig({ ...config, Listeners: next });
      setConfig(hydrateListenerConfig(saved));
      try {
        await applyRuntimeConfig();
        messageApi.success(t('listener.orderUpdated'));
      } catch (applyError) {
        messageApi.warning(t('listener.applyFailed', { error: applyError instanceof Error ? applyError.message : String(applyError) }));
      }
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('listener.orderFailed'));
    }
  }

  function deleteListener(record: ListenerRecord) {
    const attached = clients.filter((client) => nodeIDOf(client) === nodeIDOf(record)
      && (client.ListenerID === record.ID || client.ListenerIDs?.includes(record.ID)));
    if (attached.length > 0) {
      messageApi.error(t('listener.detachUsersFirst', { count: attached.length }));
      return;
    }
    Modal.confirm({
      title: t('listener.deleteConfirm', { name: record.Name || record.ID }),
      content: t('listener.deleteHelp'),
      okText: t('common.delete'),
      okType: 'danger',
      cancelText: t('listener.cancel'),
      onOk: () => deleteListenerRecords([record]),
    });
  }

  async function deleteListenerRecords(records: ListenerRecord[]) {
    if (records.length === 0) return;
    const selected = new Set(records.map(nodeObjectKey));
    const isSelectedListenerReference = (value: object, listenerID?: string) => (
      Boolean(listenerID && selected.has(`${nodeIDOf(value)}:${listenerID}`))
    );
    const nextListeners = listeners.filter((item) => !selected.has(nodeObjectKey(item)));
    const removedRoutes = (config.Routes || []).filter((route) => isSelectedListenerReference(route, route.ListenerID));
    const removedAddressKeys = new Set(removedRoutes
      .filter((route) => route.AddressID)
      .map((route) => `${nodeIDOf(route)}:${route.AddressID}`));
    const nextRoutes = (config.Routes || []).filter((route) => !isSelectedListenerReference(route, route.ListenerID));
    const nextAddresses = (config.Addresses || []).filter((address) => (
      !removedAddressKeys.has(`${nodeIDOf(address)}:${address.ID}`) || !isManagedLinkAddressRemark(address.Remark)
    ));
    const candidateProfiles = new Set(records
      .filter((item) => item.XrayProfileID)
      .map((item) => `${nodeIDOf(item)}:${item.XrayProfileID}`));
    const usedProfiles = new Set([...nextListeners, ...(config.Connectors || [])]
      .filter((item) => item.XrayProfileID)
      .map((item) => `${nodeIDOf(item)}:${item.XrayProfileID}`));
    const nextProfiles = (config.XrayProfiles || []).filter((profile) => {
      const key = `${nodeIDOf(profile)}:${profile.ID}`;
      return !candidateProfiles.has(key) || usedProfiles.has(key);
    });
    const nextDevices = devices.map((device) => {
      const removedOnNode = records.filter((record) => nodeIDOf(record) === nodeIDOf(device));
      if (removedOnNode.length === 0) return device;
      const removedIDs = new Set(removedOnNode.map((record) => record.ID));
      const removedNames = new Set(removedOnNode.map((record) => record.Name).filter(Boolean));
      return {
        ...device,
        LinkedListenerIDs: (device.LinkedListenerIDs || []).filter((id) => !removedIDs.has(id)),
        LinkedListenerNames: (device.LinkedListenerNames || []).filter((name) => !removedNames.has(name)),
      };
    });
    setSaving(true);
    try {
      const saved = await saveRuntimeConfig({
        ...config,
        Devices: nextDevices,
        Listeners: nextListeners,
        Routes: nextRoutes,
        Addresses: nextAddresses,
        XrayProfiles: nextProfiles,
      });
      setConfig(hydrateListenerConfig(saved));
      setSelectedRowKeys([]);
      await applyRuntimeConfig();
      messageApi.success(t('listener.deletedCount', { count: records.length }));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('listener.saveGenericFailed'));
    } finally {
      setSaving(false);
    }
  }

  function confirmDeleteSelectedListeners() {
    const attached = clients.filter((client) => selectedListeners.some((listener) => (
      nodeIDOf(client) === nodeIDOf(listener)
      && (client.ListenerID === listener.ID || client.ListenerIDs?.includes(listener.ID))
    )));
    if (attached.length > 0) {
      messageApi.error(t('listener.detachUsersFirst', { count: attached.length }));
      return;
    }
    Modal.confirm({
      title: t('listener.batchDeleteConfirm', { count: selectedListeners.length }),
      content: t('listener.deleteHelp'),
      okText: t('common.delete'),
      okType: 'danger',
      cancelText: t('listener.cancel'),
      onOk: () => deleteListenerRecords(selectedListeners),
    });
  }

  function exportListeners(records: ListenerRecord[]) {
    setExportModal({
      open: true,
      title: t('listener.exportCount', { count: records.length }),
      value: JSON.stringify(records, null, 2),
    });
  }

  function exportShareLink(record: ListenerRecord) {
    try {
      if (record.RuntimeMode !== 'tapx') throw new Error(t('listener.tapxShareOnly'));
      const address = listenerShareAddress(record);
      const rawTCP = record.RawTCP || {};
      const rawUDP = record.RawUDP || {};
      const security = String(record.Security || (record.Protocol === 'raw-tcp'
        ? rawTCP.TLS?.Enabled ? 'tls' : 'none'
        : rawUDP.DTLS?.Enabled ? 'dtls' : 'none')) as 'none' | 'tls' | 'dtls';
      const serverName = record.Protocol === 'raw-tcp'
        ? String(rawTCP.TLS?.ServerName || '')
        : String(rawUDP.DTLS?.ServerName || '');
      const value = buildRawConnectorLink({
        protocol: record.Protocol === 'raw-tcp' ? 'raw-tcp' : 'raw-udp',
        name: record.Name || record.ID,
        address,
        port: Number(record.BindPort || 0),
        security,
        vkey: '',
        serverName,
        lengthMode: record.Protocol === 'raw-tcp'
          ? resolveTcpLengthMode({ stored: rawTCP.LengthMode })
          : undefined,
      }, t);
      setExportModal({ open: true, title: t('listener.shareLinkTitle'), value });
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('listener.shareLinkFailed'));
    }
  }

  async function submitImport() {
    let imported: ListenerRecord[];
    try {
      imported = normalizeImportedListeners(importText, listeners, importTargetNodeID, t);
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('listener.invalidJson'));
      return;
    }
    if (imported.length === 0) {
      messageApi.warning(t('listener.noneToImport'));
      return;
    }

    const byID = new Map(listeners.map((item) => [nodeObjectKey(item), item]));
    let nextDevices = devices;
    let nextProfiles = config.XrayProfiles || [];
    for (const importedRecord of imported) {
      const existing = byID.get(nodeObjectKey(importedRecord));
      let next = normalizeForSave({ ...existing, ...importedRecord });
      if (next.Binding?.DeviceID && !nextDevices.some((device) => (
        device.ID === next.Binding?.DeviceID && nodeIDOf(device) === nodeIDOf(next)
      ))) {
        next = {
          ...next,
          Binding: {
            ...next.Binding,
            DeviceBindMode: next.Binding.DeviceName ? 'autoCreate' : 'existing',
            AutoCreateDevice: Boolean(next.Binding.DeviceName),
            DeviceID: '',
          },
        };
      }
      let materialized;
      try {
        materialized = materializeEndpointAutoDevice(next, nextDevices, {
          role: 'listener', defaultMode: 'existing', defaultAddressMode: 'manual',
        });
      } catch (error) {
        if (error instanceof DeviceTypeConflictError) {
          messageApi.error(t('device.typeConflict', deviceTypeConflictValues(error)));
          return;
        }
        throw error;
      }
      next = materialized.endpoint;
      nextDevices = materialized.devices;
      if (next.RuntimeMode !== 'tapx') {
        next = { ...next, XrayProfileID: existing?.XrayProfileID || `xray-${next.ID}` };
        nextProfiles = upsertXrayProfile(nextProfiles, buildListenerXrayProfile(next));
      } else {
        next = { ...next, XrayProfileID: '' };
      }
      byID.set(nodeObjectKey(next), next);
    }

    setSaving(true);
    try {
      const saved = await saveRuntimeConfig({
        ...config,
        Devices: nextDevices,
        Listeners: [...byID.values()],
        XrayProfiles: nextProfiles,
      });
      setConfig(hydrateListenerConfig(saved));
      setImportOpen(false);
      setImportText('');
      try {
        await applyRuntimeConfig();
        messageApi.success(t('listener.importedCount', { count: imported.length }));
      } catch (applyError) {
        messageApi.warning(t('listener.applyFailed', { error: applyError instanceof Error ? applyError.message : String(applyError) }));
      }
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('listener.importFailed'));
    } finally {
      setSaving(false);
    }
  }

  async function copyExportValue() {
    try {
      await copyText(exportModal.value);
      messageApi.success(t('listener.copied'));
    } catch {
      messageApi.error(t('listener.copyFailed'));
    }
  }

  function handleRuntimeModeChange(mode: RuntimeMode) {
    if (mode === 'tapx') {
      form.setFieldValue('Protocol', 'raw-udp');
      form.setFieldValue('Network', 'udp');
      form.setFieldValue('Security', 'none');
      form.setFieldValue('settings', {});
      form.setFieldValue('streamSettings', undefined);
      return;
    }
    form.setFieldValue('Protocol', 'vless');
    form.setFieldValue('Network', 'tcp');
    form.setFieldValue('Security', 'none');
    form.setFieldValue('settings', defaultInboundSettings('vless'));
    form.setFieldValue('streamSettings', newInboundStreamSlice('tcp'));
  }

  function handleProtocolChange(nextProtocol: string) {
    if (runtimeMode === 'tapx') {
      form.setFieldValue('Network', nextProtocol === 'raw-udp' ? 'udp' : 'tcp');
      form.setFieldValue('Security', 'none');
      form.setFieldValue('settings', {});
      return;
    }
    const nextNetwork = nextProtocol === 'hysteria' ? 'hysteria' : 'tcp';
    form.setFieldValue('Network', nextNetwork);
    form.setFieldValue('Security', nextNetwork === 'hysteria' ? 'tls' : 'none');
    form.setFieldValue('settings', defaultInboundSettings(nextProtocol));
    form.setFieldValue('streamSettings', defaultStreamForProtocol(nextProtocol));
  }

  const moreItems: MenuProps['items'] = selectedListeners.length > 0 ? [
    { key: 'enable-selected', icon: <CheckCircleOutlined />, label: t('listener.enableSelected') },
    { key: 'disable-selected', icon: <StopOutlined />, label: t('listener.disableSelected') },
    { type: 'divider' },
    { key: 'export-selected', icon: <ExportOutlined />, label: t('listener.exportSelected') },
    { key: 'reset-selected', icon: <RetweetOutlined />, label: t('listener.resetSelectedTraffic') },
  ] : [
    { key: 'import', icon: <ImportOutlined />, label: t('listener.import') },
    { key: 'export', icon: <ExportOutlined />, label: t('listener.export'), disabled: scopedListeners.length === 0 },
  ];

  const onMoreClick: MenuProps['onClick'] = ({ key }) => {
    if (key === 'import') {
      setImportTargetNodeID(defaultTargetNodeID(scope));
      setImportOpen(true);
    }
    if (key === 'export') exportListeners(scopedListeners);
    if (key === 'export-selected') exportListeners(selectedListeners);
    if (key === 'reset-selected') void resetTraffic(selectedListeners);
    if (key === 'enable-selected') void setSelectedListenersEnabled(true);
    if (key === 'disable-selected') void setSelectedListenersEnabled(false);
  };

  const columns = useMemo<TableColumnsType<ListenerRecord>>(() => [
    { title: 'ID', dataIndex: 'ID', align: 'right', width: 70 },
    { title: t('node.sourceNode'), key: 'ManagedNodeID', width: 150, render: (_, record) => <NodeSourceTag value={record} /> },
    {
      title: t('listener.menu'),
      key: 'actions',
      align: 'center',
      width: 80,
      render: (_, record, index) => (
        <Space size={4}>
          <Button shape="circle" size="small" icon={<EditOutlined />} aria-label={t('listener.edit')} onClick={() => openEdit(record)} />
          <Dropdown
            trigger={['click']}
            menu={{ items: [
              { key: 'top', icon: <VerticalAlignTopOutlined />, label: t('listener.moveTop'), disabled: index === 0, onClick: () => moveListener(index, 0) },
              { key: 'up', icon: <ArrowUpOutlined />, label: t('common.moveUp'), disabled: index === 0, onClick: () => moveListener(index, index - 1) },
              { key: 'down', icon: <ArrowDownOutlined />, label: t('common.moveDown'), disabled: index === filteredListeners.length - 1, onClick: () => moveListener(index, index + 1) },
              { type: 'divider' },
              { key: 'share', icon: <LinkOutlined />, label: t('listener.shareLink'), disabled: record.RuntimeMode !== 'tapx', onClick: () => exportShareLink(record) },
              { key: 'reset', icon: <RetweetOutlined />, label: t('listener.resetTraffic'), onClick: () => void resetTraffic([record]) },
              { key: 'delete', icon: <DeleteOutlined />, label: t('common.delete'), danger: true, onClick: () => deleteListener(record) },
            ] }}
          >
            <Button shape="circle" size="small" icon={<MoreOutlined />} aria-label={t('listener.more')} />
          </Dropdown>
        </Space>
      ),
    },
    {
      title: t('common.enabled'),
      key: 'Enabled',
      align: 'center',
      width: 80,
      render: (_, record) => <Switch checked={record.Enabled !== false} onChange={(checked) => toggleEnable(record, checked)} />,
    },
    { title: t('xray.port'), dataIndex: 'BindPort', align: 'center', width: 90 },
    {
      title: t('listener.runtimeMode'),
      key: 'RuntimeMode',
      align: 'center',
      width: 130,
      render: (_, record) => {
        const mode = record.RuntimeMode || 'embedded-xray';
        return <Tag color={mode === 'tapx' ? 'volcano' : 'blue'}>{runtimeOptions.find((item) => item.value === mode)?.label || mode}</Tag>;
      },
    },
    {
      title: t('listener.boundDevice'),
      key: 'Binding',
      width: 170,
      render: (_, record) => {
        const device = record.Binding?.DeviceID ? devices.find((item) => item.ID === record.Binding?.DeviceID && nodeIDOf(item) === nodeIDOf(record)) : undefined;
        if (device) return <Tag color={device.Type === 'tap' ? 'geekblue' : 'green'}>{labelDevice(device)}</Tag>;
        if (record.Binding?.AutoCreateDevice) return <Tag color="cyan">{record.Binding.DeviceName || t('listener.autoCreate')}</Tag>;
        return <span className="criterion-empty">-</span>;
      },
    },
    {
      title: t('listener.protocol'),
      key: 'Protocol',
      width: 190,
      render: (_, record) => {
        const security = String(record.Security || record.streamSettings?.security || 'none');
        return (
          <Space size={[4, 4]} wrap>
            <Tag color="purple">{protocolLabel(record.Protocol || 'vless')}</Tag>
            {record.RuntimeMode !== 'tapx' && record.Network ? <Tag color="green">{networkLabel(record.Network)}</Tag> : null}
            {security !== 'none' ? <Tag color="blue">{security.toUpperCase()}</Tag> : null}
          </Space>
        );
      },
    },
    {
      title: t('listener.users'),
      key: 'Users',
      align: 'center',
      width: 110,
      render: (_, record) => {
        const attached = clients.filter((client) => nodeIDOf(client) === nodeIDOf(record)
          && (client.ListenerID === record.ID || client.ListenerIDs?.includes(record.ID)));
        const active = attached.filter((client) => client.Enabled !== false).length;
        return <Space size={4}><Tag>{attached.length}</Tag><Tag color="green">{active}</Tag></Space>;
      },
    },
    { title: t('listener.traffic'), key: 'Traffic', align: 'center', width: 150, render: (_, record) => {
      const stats = listenerStats[nodeObjectKey(record)];
      return <Tag color="purple">{formatBytes((stats?.rxBytes || 0) + (stats?.txBytes || 0))} / {formatTrafficCap(record.TrafficCap)}</Tag>;
    } },
    { title: t('listener.speed'), key: 'Speed', align: 'center', width: 180, render: (_, record) => {
      const stats = listenerStats[nodeObjectKey(record)];
      return stats
        ? <Tag>↑ {formatBytes(stats.txBytesPerSecond)}/s / ↓ {formatBytes(stats.rxBytesPerSecond)}/s</Tag>
        : <Tag>-</Tag>;
    } },
    { title: t('listener.expiry'), key: 'Expire', align: 'center', width: 150, render: (_, record) => <Tag color={isExpired(record.ExpiresAt) ? 'red' : 'purple'}>{formatExpiry(record.ExpiresAt)}</Tag> },
  ], [clients, config, devices, filteredListeners, listenerStats, listeners, runtimeOptions, t]);

  const protocolOptions = runtimeMode === 'tapx' ? tapxProtocolOptions : inboundXrayProtocolOptions;

  return (
    <div className="listener-page">
      {messageContextHolder}
      <Card hoverable className="listener-summary-card">
        <div className="listener-summary-grid">
          <div>
            <div className="listener-summary-label">{t('listener.totalUploadDownload')}</div>
            <div className="listener-summary-value"><ArrowUpOutlined /> {formatBytes(listenerTotals.txBytes)} / <ArrowDownOutlined /> {formatBytes(listenerTotals.rxBytes)}</div>
          </div>
          <div>
            <div className="listener-summary-label">{t('listener.totalUsage')}</div>
            <div className="listener-summary-value"><PieChartOutlined /> {formatBytes(listenerTotals.rxBytes + listenerTotals.txBytes)}</div>
          </div>
          <div>
            <div className="listener-summary-label">{t('listener.count')}</div>
            <div className="listener-summary-value"><BarsOutlined /> {scopedListeners.length}</div>
          </div>
        </div>
      </Card>

      <Card hoverable>
        <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
          <Space wrap>
            {selectedListeners.length === 0 ? (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('listener.add')}</Button>
            ) : (
              <Tag color="blue" closable onClose={() => setSelectedRowKeys([])}>{t('listener.selectedCount', { count: selectedListeners.length })}</Tag>
            )}
            <NodeScopeSelect scope={scope} onChange={setScope} />
            <Dropdown trigger={['click']} menu={{ items: moreItems, onClick: onMoreClick }}><Button icon={<MenuOutlined />}>{selectedListeners.length > 0 ? t('listener.batchActions') : t('listener.generalActions')}</Button></Dropdown>
            {selectedListeners.length > 0 ? <Button danger icon={<DeleteOutlined />} onClick={confirmDeleteSelectedListeners}>{t('listener.deleteSelected')}</Button> : null}
            <Input.Search className="listener-search" placeholder={t('listener.search')} allowClear value={search} onChange={(event) => setSearch(event.target.value)} />
          </Space>
          <Table
            rowKey={nodeObjectKey}
            rowSelection={{ selectedRowKeys, onChange: (keys) => setSelectedRowKeys(keys.map(String)) }}
            columns={columns}
            dataSource={filteredListeners}
            loading={loading || saving}
            pagination={panelPageSize > 0 ? { pageSize: panelPageSize, showSizeChanger: false } : false}
            scroll={{ x: 1530 }}
            size="small"
            locale={{ emptyText: t('listener.empty') }}
          />
        </Space>
      </Card>

      <Modal
        open={open}
        title={editing ? t('listener.editTitle') : t('listener.addTitle')}
        okText={editing ? t('listener.saveChanges') : t('common.create')}
        cancelText={t('common.close')}
        width={780}
        forceRender
        confirmLoading={saving}
        mask={{ closable: false }}
        onOk={submit}
        onCancel={() => setOpen(false)}
      >
        <Form form={form} colon={false} labelCol={{ sm: { span: 8 } }} wrapperCol={{ sm: { span: 14 } }} labelWrap>
          <Tabs items={[
            { key: 'basic', label: t('listener.basic'), forceRender: true, children: basicTab() },
            { key: 'binding', label: t('listener.binding'), forceRender: true, children: bindingTab() },
            ...(showProtocolTab ? [{ key: 'protocol', label: t('listener.protocol'), forceRender: true, children: protocolTab() }] : []),
            ...(runtimeMode === 'tapx' || streamEnabled ? [{ key: 'transport', label: t('xray.transport'), forceRender: true, children: transportTab() }] : []),
            ...(showSecurityTab ? [{ key: 'security', label: t('xray.security'), forceRender: true, children: securityTab() }] : []),
            { key: 'advanced', label: t('listener.advanced'), forceRender: true, children: advancedTab() },
          ]} />
        </Form>
      </Modal>

      <Modal
        open={exportModal.open}
        title={exportModal.title}
        width={720}
        okText={t('listener.copy')}
        cancelText={t('common.close')}
        onOk={copyExportValue}
        onCancel={() => setExportModal({ open: false, title: '', value: '' })}
      >
        <Input.TextArea value={exportModal.value} readOnly autoSize={{ minRows: 12, maxRows: 22 }} />
      </Modal>

      <Modal
        open={importOpen}
        title={t('listener.import')}
        okText={t('listener.importAction')}
        cancelText={t('listener.cancel')}
        confirmLoading={saving}
        onOk={submitImport}
        onCancel={() => setImportOpen(false)}
      >
        <Form.Item label={t('node.targetNode')} tooltip={t('node.targetNodeHelp')}>
          <Select value={importTargetNodeID} options={nodeTargetOptions} onChange={setImportTargetNodeID} />
        </Form.Item>
        <Form.Item label={t('listener.importContent')} tooltip={t('listener.importHelp')}>
          <Input.TextArea placeholder='[{"ID":"listener-1"}]' value={importText} onChange={(event) => setImportText(event.target.value)} autoSize={{ minRows: 10, maxRows: 18 }} />
        </Form.Item>
      </Modal>
    </div>
  );

  function basicTab() {
    return (
      <>
        <Form.Item name="ID" hidden><Input /></Form.Item>
        <Form.Item name="ManagedNodeID" label={t('node.targetNode')} tooltip={t('node.targetNodeHelp')} rules={[{ required: true }]}>
          <Select options={nodeTargetOptions} disabled={Boolean(editing)} />
        </Form.Item>
        <Form.Item name="Enabled" label={t('common.enabled')} valuePropName="checked"><Switch /></Form.Item>
        <Form.Item name="Name" label={t('listener.remark')}><Input placeholder={t('listener.remarkPlaceholder')} /></Form.Item>
        <Form.Item
          name="RuntimeMode"
          label={labelWithHint(t('listener.runtimeMode'), t('listener.runtimeModeHelp'))}
        >
          <Select
            options={runtimeOptions.map((item) => item.value === 'external-xray'
              ? { ...item, disabled: !externalXrayReady, label: externalXrayReady ? item.label : t('listener.externalXrayNotConfigured') }
              : item)}
            onChange={handleRuntimeModeChange}
          />
        </Form.Item>
        <Form.Item
          name="Protocol"
          label={labelWithHint(
            runtimeMode === 'tapx' ? t('listener.rawTransport') : t('listener.protocol'),
            runtimeMode === 'tapx'
              ? t('listener.rawTransportHelp')
              : t('listener.xrayProtocolHelp'),
          )}
        >
          <Select options={protocolOptions} onChange={handleProtocolChange} />
        </Form.Item>
        <Form.Item
          name="BindHost"
          label={labelWithHint(
            t('xray.address'),
            runtimeMode === 'tapx'
              ? t('listener.tapxAddressHelp')
              : t('listener.xrayAddressHelp'),
          )}
        >
          <Input placeholder={t('listener.listenAllPlaceholder')} />
        </Form.Item>
        <Form.Item
          name="ShareAddressStrategy"
          label={labelWithHint(t('listener.shareAddressStrategy'), t('listener.shareAddressStrategyHelp'))}
        >
          <Select options={[{ value: 'listen', label: t('listener.listenerAddress') }, { value: 'custom', label: t('listener.customAddress') }]} />
        </Form.Item>
        {shareAddressStrategy === 'custom' ? (
          <Form.Item
            name="ShareAddress"
            label={labelWithHint(t('listener.customShareAddress'), t('listener.customShareAddressHelp'))}
            rules={[{
              validator: (_, value) => (
                isValidShareAddress(String(value ?? ''))
                  ? Promise.resolve()
                  : Promise.reject(new Error(t('listener.customShareAddressInvalid')))
              ),
            }]}
          >
            <Input placeholder="edge.example.com" />
          </Form.Item>
        ) : null}
        <Form.Item name="BindPort" label={t('xray.port')} tooltip={t('listener.portHelp')} rules={[{ required: true, message: t('xray.portRequired') }]}><InputNumber min={1} max={65535} placeholder="443" /></Form.Item>
        <Form.Item name="TotalTraffic" label={t('listener.totalTraffic')} tooltip={t('user.trafficCapHelp')}><InputNumber min={0} placeholder="100" /></Form.Item>
        <Form.Item name="TrafficReset" label={t('listener.trafficReset')} tooltip={t('user.trafficResetHelp')}>
          <Select options={[{ value: 'never', label: t('user.resetNever') }, { value: 'hourly', label: t('user.resetHourly') }, { value: 'daily', label: t('user.resetDaily') }, { value: 'weekly', label: t('user.resetWeekly') }, { value: 'monthly', label: t('user.resetMonthly') }]} />
        </Form.Item>
        <Form.Item name="ExpireAt" label={t('listener.expiry')} tooltip={t('user.expiryHelp')}>
          <DatePicker showTime format="YYYY-MM-DD HH:mm:ss" placeholder="2026-12-31 23:59:59" style={{ width: '100%' }} />
        </Form.Item>
      </>
    );
  }

  function bindingTab() {
    return (
      <EndpointBindingFields
        bindMode={bindMode}
        linkAutoOptimize={linkAutoOptimize}
        addressConfigEnabled={addressConfigEnabled}
        addressAssignMode={addressAssignMode}
        deviceOptions={deviceOptions}
        addressPlaceholders={{ ipv4: '10.10.0.1/24', ipv6: 'fd00::1/64', gateway: '10.10.0.254' }}
      />
    );
  }

  function protocolTab() {
    const fallbackProtocol = protocol === 'vless' || protocol === 'trojan';
    const fallbackEnabled = fallbackProtocol
      && streamNetwork === 'tcp'
      && (streamSecurity === 'tls' || streamSecurity === 'reality');
    return (
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <XrayInboundProtocolFields
          form={form}
          protocol={protocol}
          network={streamNetwork}
          security={streamSecurity}
          outboundTags={xrayConnectorTags}
        />
        {fallbackProtocol && streamNetwork === 'tcp' ? (
          <FallbacksFields
            form={form}
            listeners={filterNodeOwned(listeners, targetNodeID)}
            listenerID={editing?.ID}
            enabled={fallbackEnabled}
          />
        ) : null}
      </Space>
    );
  }

  function transportTab() {
    if (runtimeMode === 'tapx') return <TapxListenerFastPathFields tcp={isTapxRawTcp} />;
    return <XrayTransportFields form={form} direction="inbound" protocol={protocol} network={streamNetwork} />;
  }

  function securityTab() {
    if (runtimeMode === 'tapx') {
      return (
        <>
          <Form.Item name="Security" label={t('xray.security')}>
            <Radio.Group buttonStyle="solid">
              <Radio.Button value="none">{t('xray.none')}</Radio.Button>
              {isTapxRawTcp ? <Radio.Button value="tls">TLS</Radio.Button> : null}
              {isTapxRawUdp ? <Radio.Button value="dtls">DTLS</Radio.Button> : null}
            </Radio.Group>
          </Form.Item>
          {isTapxRawTcp && tapxSecurity === 'tls' ? <TapxListenerTlsFields panelCertificate={panelCertificate} /> : null}
          {isTapxRawUdp && tapxSecurity === 'dtls' ? <TapxListenerDtlsFields panelCertificate={panelCertificate} /> : null}
        </>
      );
    }
    return <XraySecurityFields form={form} direction="inbound" protocol={protocol} network={streamNetwork} security={streamSecurity} panelCertificate={panelCertificate} />;
  }

  function advancedTab() {
    return (
      <>
        <div className="listener-advanced-panel">
          <div className="listener-advanced-title">{t('listener.advancedTitle')}</div>
          <div className="listener-advanced-subtitle">{t('listener.advancedHelp')}</div>
          <Tabs items={[
            { key: 'all', label: t('listener.all'), forceRender: true, children: <AdvancedInboundAllEditor form={form} streamEnabled={streamEnabled} /> },
            { key: 'binding', label: t('listener.binding'), forceRender: true, children: <AdvancedInboundSliceEditor form={form} path="Binding" wrapKey="tapxBinding" /> },
            { key: 'settings', label: t('listener.settings'), forceRender: true, children: <AdvancedInboundSliceEditor form={form} path="settings" wrapKey="settings" /> },
            ...(streamEnabled ? [{ key: 'stream', label: 'Stream', forceRender: true, children: <AdvancedInboundSliceEditor form={form} path="streamSettings" wrapKey="streamSettings" /> }] : []),
          ]} />
        </div>
      </>
    );
  }
}

function listenerShareAddress(record: ListenerRecord): string {
  if (record.ShareAddressStrategy === 'custom') return String(record.ShareAddress || '').trim();
  const listen = String(record.BindHost || '').trim();
  if (listen && !['0.0.0.0', '::', '[::]'].includes(listen)) return listen.replace(/^\[|\]$/g, '');
  return window.location.hostname;
}

function buildListenerXrayProfile(record: ListenerRecord): TapxXrayProfile & NodeOwned {
  const profileID = record.XrayProfileID || `xray-${record.ID}`;
  const protocol = record.Protocol || 'vless';
  const prepared = takeInboundFallbacks(inboundSettingsToWire(protocol, record.settings || {}));
  const streamSettings = inboundStreamToWire(record.streamSettings || {});
  const sockopt = streamSettings.sockopt;
  delete streamSettings.sockopt;
  const sniffing = record.sniffing && typeof record.sniffing === 'object' ? record.sniffing : undefined;

  return {
    ManagedNodeID: record.ManagedNodeID,
    ID: profileID,
    Enabled: record.Enabled !== false,
    Name: record.Name || record.ID,
    Runtime: record.RuntimeMode === 'external-xray' ? 'external' : 'embedded',
    InboundProtocol: protocol,
    InboundSettingsJSON: JSON.stringify(prepared.settings),
    Network: record.Network || String(streamSettings.network || ''),
    Security: record.Security || String(streamSettings.security || 'none'),
    StreamSettingsJSON: Object.keys(streamSettings).length > 0 ? JSON.stringify(streamSettings) : '',
    SniffingJSON: sniffing && Object.keys(sniffing).length > 0 ? JSON.stringify(sniffing) : '',
    SockoptJSON: sockopt && typeof sockopt === 'object' ? JSON.stringify(sockopt) : '',
    FallbacksJSON: prepared.fallbacks && prepared.fallbacks.length > 0 ? JSON.stringify(prepared.fallbacks) : '',
    Remark: `tapx:listener-profile:${record.Name || record.ID}`,
  };
}

function defaultStreamForProtocol(protocol: string): Record<string, unknown> | undefined {
  if (protocol === 'wireguard' || protocol === 'tunnel') return { security: 'none' };
  if (protocol === 'hysteria') return newInboundHysteriaStreamSlice();
  if (!canEnableStream(protocol)) return undefined;
  return newInboundStreamSlice('tcp');
}

function normalizeForSave(record: ListenerRecord): ListenerRecord {
  const runtime = record.RuntimeMode || 'embedded-xray';
  const stream = runtime === 'tapx' ? undefined : (record.streamSettings || newInboundStreamSlice(record.Network || 'tcp'));
  const security = runtime === 'tapx' ? record.Security || 'none' : String(stream?.security || 'none');
  const fastPath = record.FastPath || {};
  const tls = record.TLS || {};
  const rawUDP = stripTapxSocketOverrides(record.RawUDP || {});
  const rawTCP = stripTapxSocketOverrides(record.RawTCP || {});
  return {
    ...record,
    RuntimeMode: runtime,
    Transport: runtime === 'tapx' ? (record.Protocol === 'raw-udp' ? 'udp' : 'tcp') : 'xray',
    Network: runtime === 'tapx' ? (record.Protocol === 'raw-udp' ? 'udp' : 'tcp') : String(stream?.network || 'tcp'),
    Security: security,
    ShareAddressStrategy: record.ShareAddressStrategy || 'listen',
    ShareAddress: record.ShareAddressStrategy === 'custom' ? String(record.ShareAddress || '').trim() : '',
    TrafficCap: record.TotalTraffic != null
      ? Math.max(0, Math.round(Number(record.TotalTraffic) * 1024 * 1024 * 1024))
      : (record.TrafficCap || 0),
    TrafficReset: record.TrafficReset || 'never',
    ExpiresAt: record.ExpireAt != null ? parseExpiry(record.ExpireAt) : (record.ExpiresAt || 0),
    streamSettings: stream,
    RawUDP: runtime === 'tapx' && record.Protocol === 'raw-udp' ? {
      ...rawUDP,
      Workers: numberValue(rawUDP.Workers, numberValue(fastPath.WorkerThreads)),
      QueueSize: numberValue(rawUDP.QueueSize, numberValue(fastPath.QueueSize)),
      ZeroCopy: booleanValue(rawUDP.ZeroCopy, booleanValue(fastPath.ZeroCopy)),
      DTLS: {
        ...rawUDP.DTLS,
        Enabled: security === 'dtls',
        CertFile: stringValue(tls.CertFile, rawUDP.DTLS?.CertFile),
        KeyFile: stringValue(tls.KeyFile, rawUDP.DTLS?.KeyFile),
        ServerName: stringValue(tls.ServerName, rawUDP.DTLS?.ServerName),
        ALPN: [],
        MinVersion: stringValue(tls.MinVersion, rawUDP.DTLS?.MinVersion),
        MaxVersion: stringValue(tls.MaxVersion, rawUDP.DTLS?.MaxVersion),
        AllowInsecure: booleanValue(tls.AllowInsecure, rawUDP.DTLS?.AllowInsecure),
        MTU: numberValue(tls.DtlsMtu, rawUDP.DTLS?.MTU),
        ReplayWindow: numberValue(tls.DtlsReplayWindow, rawUDP.DTLS?.ReplayWindow),
      },
    } : rawUDP,
    RawTCP: runtime === 'tapx' && record.Protocol === 'raw-tcp' ? {
      ...rawTCP,
      LengthMode: resolveTcpLengthMode({
        mode: rawTCP.LengthMode,
        legacyPrefix: fastPath.TcpLengthPrefix,
        stored: fastPath.TcpLengthMode,
      }),
      Workers: numberValue(rawTCP.Workers, numberValue(fastPath.WorkerThreads)),
      QueueSize: numberValue(rawTCP.QueueSize, numberValue(fastPath.QueueSize)),
      ZeroCopy: booleanValue(rawTCP.ZeroCopy, booleanValue(fastPath.ZeroCopy)),
      TLS: {
        ...rawTCP.TLS,
        Enabled: security === 'tls',
        CertFile: stringValue(tls.CertFile, rawTCP.TLS?.CertFile),
        KeyFile: stringValue(tls.KeyFile, rawTCP.TLS?.KeyFile),
        ServerName: stringValue(tls.ServerName, rawTCP.TLS?.ServerName),
        ALPN: [],
        MinVersion: stringValue(tls.MinVersion, rawTCP.TLS?.MinVersion),
        MaxVersion: stringValue(tls.MaxVersion, rawTCP.TLS?.MaxVersion),
        AllowInsecure: booleanValue(tls.AllowInsecure, rawTCP.TLS?.AllowInsecure),
      },
    } : rawTCP,
    Binding: normalizeBinding(record.Binding),
  };
}

function hydrateListenerConfig(config: RuntimeConfig): RuntimeConfig {
  const profiles = new Map((config.XrayProfiles || []).map((profile) => [`${nodeIDOf(profile)}:${profile.ID}`, profile]));
  const devices = new Map((config.Devices || []).map((device) => [`${nodeIDOf(device)}:${device.ID}`, device]));
  return {
    ...config,
    Listeners: (config.Listeners || []).map((item) => hydrateListener(
      item as ListenerRecord,
      profiles.get(`${nodeIDOf(item)}:${item.XrayProfileID || ''}`),
      devices.get(`${nodeIDOf(item)}:${item.Binding?.DeviceID || ''}`),
    )),
  };
}

function hydrateListener(record: ListenerRecord, profile?: TapxXrayProfile, device?: TapxDevice): ListenerRecord {
  const binding = hydrateSavedDeviceBinding(record.Binding, device);
  const displayLimits = {
    TotalTraffic: record.TrafficCap ? Number((record.TrafficCap / (1024 * 1024 * 1024)).toFixed(2)) : 0,
    TrafficReset: record.TrafficReset || 'never',
    ExpireAt: record.ExpiresAt ? dayjs(record.ExpiresAt * 1000) : null,
    ShareAddressStrategy: record.ShareAddressStrategy === 'custom' ? 'custom' as const : 'listen' as const,
    ShareAddress: record.ShareAddress || '',
  };
  if (record.Transport === 'xray' && profile) {
    const protocol = profile.InboundProtocol || 'vless';
    let settings = inboundSettingsFromWire(protocol, parseObjectJSON(profile.InboundSettingsJSON));
    settings = restoreInboundFallbacks(settings, parseJSON(profile.FallbacksJSON));
    const streamSettings = inboundStreamFromWire(parseObjectJSON(profile.StreamSettingsJSON));
    const sockopt = parseObjectJSON(profile.SockoptJSON);
    if (Object.keys(sockopt).length > 0) streamSettings.sockopt = sockopt;
    return {
      ...record,
      ...displayLimits,
      RuntimeMode: profile.Runtime === 'external' ? 'external-xray' : 'embedded-xray',
      Protocol: protocol,
      Network: profile.Network || String(streamSettings.network || ''),
      Security: profile.Security || String(streamSettings.security || 'none'),
      settings,
      streamSettings: Object.keys(streamSettings).length > 0 ? streamSettings : defaultStreamForProtocol(protocol),
      sniffing: parseObjectJSON(profile.SniffingJSON),
      Binding: binding,
    };
  }
  if (record.Transport !== 'udp' && record.Transport !== 'tcp') return { ...record, ...displayLimits, Binding: binding };
  const isUDP = record.Transport === 'udp';
  const rawUDP = record.RawUDP;
  const rawTCP = record.RawTCP;
  const tls = isUDP ? rawUDP?.DTLS : rawTCP?.TLS;
  return {
    ...record,
    ...displayLimits,
    RuntimeMode: 'tapx',
    Protocol: isUDP ? 'raw-udp' : 'raw-tcp',
    Network: isUDP ? 'udp' : 'tcp',
    Security: isUDP ? (tls?.Enabled ? 'dtls' : 'none') : (tls?.Enabled ? 'tls' : 'none'),
    TLS: tls ? {
      CertFile: tls.CertFile,
      KeyFile: tls.KeyFile,
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

function isValidShareAddress(value: string): boolean {
  const input = value.trim();
  if (!input) return true;
  if (input.includes('://') || input.startsWith('//') || /[/?#@]/.test(input)) return false;
  if (input.startsWith('[')) {
    if (!input.endsWith(']')) return false;
    try {
      new URL(`http://${input}`);
      return true;
    } catch {
      return false;
    }
  }
  if (input.includes(':')) {
    try {
      new URL(`http://[${input}]`);
      return true;
    } catch {
      return false;
    }
  }
  return /^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$/.test(input);
}

function normalizeBinding(binding: ListenerRecord['Binding']): ListenerRecord['Binding'] {
  return normalizeDeviceBinding(binding, { mode: 'existing', addressMode: 'manual' });
}

function protocolLabel(protocol: string) {
  if (protocol === 'raw-tcp') return 'Raw TCP';
  if (protocol === 'raw-udp') return 'Raw UDP';
  return protocol;
}

function networkLabel(network: string): string {
  const labels: Record<string, string> = {
    tcp: 'RAW',
    kcp: 'mKCP',
    ws: 'WebSocket',
    grpc: 'gRPC',
    httpupgrade: 'HTTPUpgrade',
    xhttp: 'XHTTP',
    hysteria: 'UDP',
  };
  return labels[network] || network;
}

function nextPort(listeners: ListenerRecord[]): number {
  const used = new Set(listeners.map((item) => Number(item.BindPort)).filter(Boolean));
  for (let attempt = 0; attempt < 128; attempt += 1) {
    const port = randomInteger(10000, 60000);
    if (!used.has(port)) return port;
  }
  for (let port = 10000; port <= 60000; port += 1) {
    if (!used.has(port)) return port;
  }
  return 10000;
}

function parseExpiry(value: Dayjs | string | null | undefined): number {
  if (value && typeof value === 'object' && 'isValid' in value && typeof value.isValid === 'function') {
    return value.isValid() ? Math.floor(value.valueOf() / 1000) : 0;
  }
  const input = String(value || '').trim();
  if (!input) return 0;
  if (/^\d+$/.test(input)) return Math.max(0, Number(input));
  const millis = Date.parse(input.includes('T') ? input : input.replace(' ', 'T'));
  return Number.isFinite(millis) ? Math.floor(millis / 1000) : 0;
}

function formatExpiryText(value?: number): string {
  if (!value) return '';
  const date = new Date(value * 1000);
  const pad = (part: number) => String(part).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

function formatExpiry(value?: number): string {
  return value ? formatExpiryText(value) : '∞';
}

function isExpired(value?: number): boolean {
  return Boolean(value && value <= Math.floor(Date.now() / 1000));
}

function formatTrafficCap(value?: number): string {
  if (!value) return '∞';
  const gb = value / (1024 * 1024 * 1024);
  return `${Number(gb.toFixed(2))} GB`;
}

function counterRate(previous: number, current: number, elapsed: number): number {
  if (current <= previous || elapsed <= 0) return 0;
  return (current - previous) / elapsed;
}

function normalizeImportedListeners(input: string, existing: ListenerRecord[], managedNodeID: string, t: ReturnType<typeof useI18n>['t']): ListenerRecord[] {
  const parsed = JSON.parse(input) as unknown;
  const source = Array.isArray(parsed)
    ? parsed
    : parsed && typeof parsed === 'object' && Array.isArray((parsed as { Listeners?: unknown[] }).Listeners)
      ? (parsed as { Listeners: unknown[] }).Listeners
      : null;
  if (!source) throw new Error(t('listener.importArrayRequired'));

  const used = new Set(existing.filter((item) => nodeIDOf(item) === managedNodeID).map((item) => item.ID));
  return source.map((value, index) => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      throw new Error(t('listener.invalidImportItem', { index: index + 1 }));
    }
    const raw = value as ListenerRecord;
    const requestedID = String(raw.ID || '').trim();
    const id = requestedID || uniqueId('listener-import', used);
    used.add(id);
    return normalizeForSave({
      ...defaultListener,
      ...raw,
      ID: id,
      ManagedNodeID: managedNodeID,
      Name: String(raw.Name || id),
      Binding: { ...defaultListener.Binding, ...(raw.Binding || {}) },
    });
  });
}

function panelCertificateFromSettings(settings: unknown[] | undefined): { certPublicPath?: string; certPrivatePath?: string } {
  const row = (settings || []).find((item) => item
    && typeof item === 'object'
    && (item as { Enabled?: boolean }).Enabled !== false
    && ('PanelCertFile' in item || 'PanelKeyFile' in item || 'PanelHTTPS' in item)) as {
    PanelCertFile?: string;
    PanelKeyFile?: string;
    Key?: string;
    Value?: unknown;
  } | undefined;
  if (row?.PanelCertFile || row?.PanelKeyFile) {
    return { certPublicPath: row.PanelCertFile, certPrivatePath: row.PanelKeyFile };
  }

  const legacy: Record<string, unknown> = {};
  for (const item of settings || []) {
    if (!item || typeof item !== 'object') continue;
    const entry = item as { Key?: string; Value?: unknown };
    if (entry.Key) legacy[entry.Key] = entry.Value;
  }
  return {
    certPublicPath: typeof legacy.certPublicPath === 'string' ? legacy.certPublicPath : undefined,
    certPrivatePath: typeof legacy.certPrivatePath === 'string' ? legacy.certPrivatePath : undefined,
  };
}
