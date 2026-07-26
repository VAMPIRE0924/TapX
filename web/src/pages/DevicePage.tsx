import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Card,
  Col,
  Dropdown,
  Form,
  Input,
  InputNumber,
  Modal,
  Radio,
  Row,
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
  CheckCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  ExportOutlined,
  ImportOutlined,
  MenuOutlined,
  MoreOutlined,
  PlusOutlined,
  StopOutlined,
} from '@ant-design/icons';
import {
  getSystemInterfaces,
  type RuntimeConfig,
  type TapxDHCPMode,
  type TapxDevice,
  type TapxEndpoint,
  type TapxTapMode,
} from '../shared/api';
import {
  applyManagedRuntimeConfig as applyRuntimeConfig,
  defaultTargetNodeID,
  filterNodeOwned,
  getManagedRuntimeConfig as getRuntimeConfig,
  nodeIDOf,
  nodeObjectKey,
  sameNodeObject,
  saveManagedRuntimeConfig as saveRuntimeConfig,
  type NodeOwned,
} from '../features/nodes/managedConfig';
import { NodeScopeSelect, NodeSourceTag, useNodeScope, useNodeTargetOptions } from '../features/nodes/NodeScope';
import { useI18n } from '../i18n/I18nProvider';
import { UnitInputNumber } from '../components/UnitInputNumber';
import { AdvancedConfigEditor } from '../components/AdvancedConfigEditor';
import { copyText } from '../shared/clipboard';
import { objectMatchesSearch } from '../shared/object-search';
import './DevicePage.css';

type DeviceRecord = TapxDevice & NodeOwned & {
  BridgeName?: string;
  BridgeMember?: string;
  BridgeMTU?: number;
  DHCPDNSList?: string[];
  SharedDNSList?: string[];
  TUNDHCPDNSList?: string[];
  TUNDHCPServiceDNSList?: string[];
  RelayServerList?: string[];
  RelayDownstreamList?: string[];
  TUNServerDHCPEnabled?: boolean;
};

const defaultDevice: DeviceRecord = {
  ID: '',
  Enabled: false,
  Name: '',
  Type: 'tun',
  IfName: '',
  MTU: 1500,
  LinkAutoOptimize: false,
  Routes: [],
  BridgeName: 'br-tapx',
  BridgeMTU: 1500,
  TapMode: 'standalone',
  AccessRole: 'client',
  DHCP: {
    Mode: 'off',
    PrefixLength: 24,
    LeaseSeconds: 86400,
    Authoritative: true,
    ConflictDetection: true,
    StaticLeases: [],
  },
  SharedIP: {
    Role: 'service',
    AddressSource: 'auto',
    FirewallBackend: 'auto',
    HostPortPriority: true,
    TrackAddressChanges: true,
    ReservedTCPPorts: [],
    ReservedUDPPorts: [],
  },
  OneArmRollbackSeconds: 60,
  TUNDHCP: {
    Mode: 'off',
    Protocol: 'ipv4',
    RelayEnabled: false,
    RelayProtocol: 'ipv4',
    LeaseSeconds: 86400,
	Authoritative: true,
	ConflictDetection: true,
	MaxHops: 4,
  },
  TUNServerDHCPEnabled: false,
  Source: 'manual',
};

export function DevicePage() {
  const { language, t } = useI18n();
  const [config, setConfig] = useState<RuntimeConfig>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<DeviceRecord | null>(null);
  const [selectedRowKeys, setSelectedRowKeys] = useState<string[]>([]);
  const [search, setSearch] = useState('');
  const [importOpen, setImportOpen] = useState(false);
  const [importText, setImportText] = useState('');
  const [importTargetNodeID, setImportTargetNodeID] = useState('local');
  const [exportOpen, setExportOpen] = useState(false);
  const [exportValue, setExportValue] = useState('');
  const [interfaces, setInterfaces] = useState<string[]>([]);
  const [form] = Form.useForm<DeviceRecord>();
  const [messageApi, messageContextHolder] = message.useMessage();
  const { nodes, scope, setScope } = useNodeScope();
  const nodeTargetOptions = useNodeTargetOptions(nodes);

  const devices = useMemo(() => ((config.Devices || []) as DeviceRecord[]), [config.Devices]);
  const visibleDevices = useMemo(() => filterNodeOwned(devices, scope), [devices, scope]);
  const filteredDevices = useMemo(
    () => visibleDevices.filter((item) => objectMatchesSearch(item, search, [
      deviceNetworkText(item),
      linkedEndpointLabels(item, config.Listeners || [], config.Connectors || []).join(' '),
    ])),
    [config.Connectors, config.Listeners, search, visibleDevices],
  );
  const selectedDevices = useMemo(
    () => devices.filter((item) => selectedRowKeys.includes(nodeObjectKey(item))),
    [devices, selectedRowKeys],
  );
  useEffect(() => {
    const visibleKeys = new Set(filteredDevices.map(nodeObjectKey));
    setSelectedRowKeys((current) => current.filter((key) => visibleKeys.has(key)));
  }, [filteredDevices]);
  const deviceType = Form.useWatch('Type', form) ?? 'tun';
  const tapMode = (Form.useWatch('TapMode', form) ?? 'standalone') as TapxTapMode;
  const accessRole = Form.useWatch('AccessRole', form) ?? 'client';
  const dhcpMode = (Form.useWatch(['DHCP', 'Mode'], form) ?? 'off') as TapxDHCPMode;
  const sharedAddressSource = Form.useWatch(['SharedIP', 'AddressSource'], form) ?? 'auto';
  const tunDHCPMode = Form.useWatch(['TUNDHCP', 'Mode'], form) ?? 'off';
  const tunServerDHCPEnabled = Form.useWatch('TUNServerDHCPEnabled', form) ?? false;
  const tunDHCPProtocol = Form.useWatch(['TUNDHCP', 'Protocol'], form) ?? 'ipv4';
	const relayEnabled = Form.useWatch(['TUNDHCP', 'RelayEnabled'], form) ?? false;
  const linkAutoOptimize = Form.useWatch('LinkAutoOptimize', form) ?? false;
  const targetNodeID = String(Form.useWatch('ManagedNodeID', form) || defaultTargetNodeID(scope));

  const interfaceOptions = useMemo(() => {
    const names = new Set(interfaces);
    if (editing?.BridgeMember) names.add(editing.BridgeMember);
    return Array.from(names)
      .filter(Boolean)
      .sort((a, b) => a.localeCompare(b, language))
      .map((name) => ({ value: name, label: name }));
  }, [editing?.BridgeMember, interfaces, language]);

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (open) void refreshInterfaces(targetNodeID);
  }, [open, targetNodeID]);

  async function refresh() {
    setLoading(true);
    try {
      setConfig(await getRuntimeConfig());
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('device.loadFailed'));
    } finally {
      setLoading(false);
    }
  }

  async function refreshInterfaces(nodeID: string) {
    try {
      setInterfaces(normalizeInterfaceNames(await getSystemInterfaces(nodeID)));
    } catch (error) {
      setInterfaces([]);
      messageApi.error(error instanceof Error ? error.message : t('device.loadFailed'));
    }
  }

  function openCreate() {
    const name = nextDeviceName(devices, 'tapx-tun');
    // Keep the ID empty until submit so changing the visible interface name
    // cannot leave a stale ID that collides with an earlier renamed device.
    const next = { ...defaultDevice, ID: '', Name: name, IfName: name, ManagedNodeID: defaultTargetNodeID(scope) };
    setEditing(null);
    form.resetFields();
    form.setFieldsValue(next);
    setOpen(true);
  }

  const openEdit = useCallback((record: DeviceRecord) => {
    setEditing(record);
    form.resetFields();
    form.setFieldsValue({
      ...defaultDevice,
      ...record,
      Routes: (record.Routes || []).map((route) => ({ ...route })),
      BridgeName: record.Bridge?.Name,
      BridgeMember: record.Bridge?.IfName,
      BridgeMTU: record.Bridge?.MTU || record.MTU || 1500,
      TapMode: record.TapMode || inferTapMode(record),
      AccessRole: record.AccessRole || inferAccessRole(record),
      DHCP: {
        ...defaultDevice.DHCP,
        ...record.DHCP,
        IPv4CIDR: record.DHCP?.IPv4CIDR || (record.DHCP?.Gateway ? `${record.DHCP.Gateway}/${record.DHCP.PrefixLength || 24}` : undefined),
        StaticLeases: (record.DHCP?.StaticLeases || []).map((lease) => ({ ...lease })),
      },
      DHCPDNSList: record.DHCP?.DNS || [],
      SharedIP: { ...defaultDevice.SharedIP, ...record.SharedIP },
      SharedDNSList: record.SharedIP?.DNS || [],
      TUNDHCP: {
        ...defaultDevice.TUNDHCP,
        ...record.TUNDHCP,
        Mode: record.TUNDHCP?.Mode || 'off',
        RelayEnabled: record.TUNDHCP?.RelayEnabled === true,
        RelayProtocol: record.TUNDHCP?.RelayProtocol || record.TUNDHCP?.Protocol || 'ipv4',
      },
      TUNDHCPDNSList: record.TUNDHCP?.DNS || [],
      TUNDHCPServiceDNSList: record.TUNDHCP?.OfferedDNS || [],
      TUNServerDHCPEnabled: record.TUNDHCP?.Mode === 'server',
      RelayServerList: record.TUNDHCP?.RelayServers || [],
      RelayDownstreamList: record.TUNDHCP?.RelayDownstreamInterfaces || [],
    });
    setOpen(true);
  }, [form]);

  async function submit() {
    try {
      await form.validateFields();
    } catch {
      // Ant Design already renders the field-level validation message. Keep the
      // modal interactive when the invalid field lives on another tab.
      return;
    }
    const values = form.getFieldsValue(true) as DeviceRecord;
    const ifNameSeed = String(values.IfName || values.Name || '').trim();
    const preferredID = String(values.ID || '').trim() || makeDeviceId(ifNameSeed);
    const id = editing?.ID || uniqueDeviceID(devices, preferredID, values.ManagedNodeID);
    const ifName = (ifNameSeed || id).trim();
    const routes = (values.Routes || [])
      .filter((route) => String(route.Destination || '').trim())
      .map((route) => ({
        ...route,
        Enabled: route.Enabled !== false,
        Destination: String(route.Destination || '').trim(),
        Gateway: String(route.Gateway || '').trim(),
        Source: String(route.Source || '').trim(),
        IfName: String(route.IfName || '').trim() || ifName,
        Metric: Math.max(0, Number(route.Metric) || 0),
        Table: String(route.Table || '').trim(),
      }));
    const tapModeForTap: TapxTapMode = values.Type === 'tap' ? (values.TapMode || 'standalone') : 'standalone';
    const accessRoleForDevice = values.AccessRole || 'client';
    const sharedRoleForTap = accessRoleForDevice === 'server' ? 'service' : 'access';
    const tapDHCPInterfaceCIDR = String(values.DHCP?.IPv4CIDR || '').trim();
    const tapDHCPPrefix = Number(tapDHCPInterfaceCIDR.split('/')[1]) || Number(values.DHCP?.PrefixLength) || 24;
    const tapDHCPInterfaceAddress = tapDHCPInterfaceCIDR.split('/')[0];
    const normalizedTunMode = accessRoleForDevice === 'server'
      ? (values.TUNServerDHCPEnabled ? 'server' : 'manual')
      : values.TUNDHCP?.Mode || 'off';
    const bridgeEnabledForTap = values.Type === 'tap'
      && tapModeForTap !== 'standalone'
      && !(tapModeForTap === 'shared-ip' && sharedRoleForTap === 'service');
    const normalizedDHCPMode: TapxDHCPMode = tapModeForTap === 'shared-ip'
      ? (sharedRoleForTap === 'access' ? 'passthrough' : 'mirror')
      : (accessRoleForDevice === 'server' ? values.DHCP?.Mode || 'off' : 'off');
    const next: DeviceRecord = {
      ...defaultDevice,
      ...editing,
      ...values,
      ID: id,
      Name: ifName,
      IfName: ifName,
      Enabled: values.Enabled !== false,
      Type: values.Type === 'tap' ? 'tap' : 'tun',
      MTU: Number(values.MTU) || 1500,
      LinkAutoOptimize: values.LinkAutoOptimize === true,
      MSSClamp: values.LinkAutoOptimize === true ? 0 : Number(values.MSSClamp) || 0,
      Routes: routes,
      Bridge: bridgeEnabledForTap
        ? { Enabled: true, Name: values.BridgeName || 'br-tapx', IfName: values.BridgeMember || '', MTU: Number(values.BridgeMTU) || Number(values.MTU) || 1500 }
        : undefined,
      TapMode: tapModeForTap,
      AccessRole: accessRoleForDevice,
      DHCP: values.Type === 'tap'
        ? {
          ...values.DHCP,
          Mode: normalizedDHCPMode,
          DNS: values.DHCPDNSList || [],
          IPv4CIDR: normalizedDHCPMode === 'server' ? tapDHCPInterfaceCIDR : undefined,
          PrefixLength: Math.min(32, Math.max(1, tapDHCPPrefix)),
          Gateway: normalizedDHCPMode === 'server' ? values.DHCP?.Gateway || tapDHCPInterfaceAddress : undefined,
          LeaseSeconds: Math.max(60, Number(values.DHCP?.LeaseSeconds) || 86400),
          StaticLeases: (values.DHCP?.StaticLeases || []).filter((lease) => lease.MAC || lease.Address),
        }
        : undefined,
      SharedIP: values.Type === 'tap' && tapModeForTap === 'shared-ip'
        ? {
          ...values.SharedIP,
          Role: sharedRoleForTap,
          DNS: values.SharedDNSList || [],
          UplinkInterface: sharedRoleForTap === 'service' ? values.SharedIP?.UplinkInterface : undefined,
          FirewallBackend: sharedRoleForTap === 'service' ? values.SharedIP?.FirewallBackend || 'auto' : undefined,
        }
        : undefined,
      OneArmRollbackSeconds: values.Type === 'tap' && tapModeForTap === 'one-arm'
        ? Math.max(15, Number(values.OneArmRollbackSeconds) || 60)
        : undefined,
      TUNDHCP: values.Type === 'tun'
        ? {
          ...values.TUNDHCP,
          Mode: normalizedTunMode,
          RelayEnabled: values.TUNDHCP?.RelayEnabled === true,
          RelayProtocol: values.TUNDHCP?.RelayProtocol || 'ipv4',
          DNS: values.TUNDHCPDNSList || [],
          OfferedGateway: normalizedTunMode === 'server' ? values.TUNDHCP?.OfferedGateway : undefined,
          OfferedDNS: normalizedTunMode === 'server' ? values.TUNDHCPServiceDNSList || [] : [],
          LeaseSeconds: Math.max(60, Number(values.TUNDHCP?.LeaseSeconds) || 86400),
          Authoritative: normalizedTunMode === 'server' && values.TUNDHCP?.Protocol !== 'ipv6' && values.TUNDHCP?.Authoritative === true,
          ConflictDetection: normalizedTunMode === 'server' && values.TUNDHCP?.ConflictDetection === true,
		  RelayServers: values.TUNDHCP?.RelayEnabled ? values.RelayServerList || [] : [],
		  RelayDownstreamInterfaces: values.TUNDHCP?.RelayEnabled ? values.RelayDownstreamList || [] : [],
		  MaxHops: Math.min(16, Math.max(1, Number(values.TUNDHCP?.MaxHops) || 4)),
        }
        : undefined,
      Source: values.Source || editing?.Source || 'manual',
    };
    const index = devices.findIndex((item) => sameNodeObject(item, next));
    const nextDevices = index < 0
      ? [...devices, next]
      : devices.map((item) => (sameNodeObject(item, next) ? next : item));

    setSaving(true);
    try {
      const saved = await saveRuntimeConfig({ ...config, Devices: nextDevices });
      try {
        await applyRuntimeConfig();
      } catch (applyError) {
        await saveRuntimeConfig(config);
        await applyRuntimeConfig().catch(() => undefined);
        throw applyError;
      }
      setConfig(saved);
      setOpen(false);
      messageApi.success(t('device.saved'));
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('device.saveFailed'));
    } finally {
      setSaving(false);
    }
  }

  function changeAccessRole(role: 'client' | 'server') {
    form.setFieldValue('AccessRole', role);
    form.setFieldValue(['SharedIP', 'Role'], role === 'server' ? 'service' : 'access');
    const mode = form.getFieldValue(['TUNDHCP', 'Mode']);
    if ((role === 'client' && mode === 'server') || (role === 'server' && mode === 'client')) {
      form.setFieldValue(['TUNDHCP', 'Mode'], role === 'server' ? 'manual' : 'off');
    }
    form.setFieldValue('TUNServerDHCPEnabled', role === 'server' && mode === 'server');
    if (role === 'client') {
      form.setFieldValue(['TUNDHCP', 'RelayEnabled'], false);
      form.setFieldValue('RelayDownstreamList', []);
      form.setFieldValue('RelayServerList', []);
    }
  }

  async function deleteDevice(record: DeviceRecord) {
    const references = deviceReferences(record);
    if (references.length > 0) {
      messageApi.warning(t('device.referencedBy', { references: references.join(', ') }));
      return;
    }
    setSaving(true);
    try {
      const saved = await saveRuntimeConfig({ ...config, Devices: devices.filter((item) => !sameNodeObject(item, record)) });
      setConfig(saved);
      try {
        await applyRuntimeConfig();
        messageApi.success(t('device.deleted'));
      } catch (applyError) {
        messageApi.warning(t('device.applyFailed', { error: applyError instanceof Error ? applyError.message : String(applyError) }));
      }
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('device.deleteFailed'));
    } finally {
      setSaving(false);
    }
  }

  function isDeviceReferenced(record: DeviceRecord) {
    return deviceReferences(record).length > 0;
  }

  function deviceReferences(record: DeviceRecord): string[] {
    const sameNodeReference = (value: object, id: string | undefined) => id === record.ID && nodeIDOf(value) === nodeIDOf(record);
    return [
      ...(config.Listeners || [])
        .filter((item) => sameNodeReference(item, item.Binding?.DeviceID))
        .map((item) => `${t('device.referenceListener')} ${item.Name || item.ID}`),
      ...(config.Connectors || [])
        .filter((item) => sameNodeReference(item, item.Binding?.DeviceID))
        .map((item) => `${t('device.referenceConnector')} ${item.Name || item.ID}`),
      ...(config.Routes || [])
        .filter((item) => sameNodeReference(item, item.DeviceID))
        .map((item) => `${t('device.referenceLink')} ${item.ID}`),
      ...(config.Addresses || [])
        .filter((item) => sameNodeReference(item, item.DeviceID))
        .map((item) => `${t('device.referenceAddress')} ${item.Name || item.ID}`),
      ...(config.Clients || [])
        .filter((item) => sameNodeReference(item, item.Binding?.DeviceID)
          || (nodeIDOf(item) === nodeIDOf(record) && item.AllowedDeviceIDs?.includes(record.ID)))
        .map((item) => `${t('device.referenceClient')} ${item.Email || item.Name || item.ID}`),
    ];
  }

  async function setSelectedDevicesEnabled(enabled: boolean) {
    if (selectedDevices.length === 0) return;
    const selected = new Set(selectedDevices.map(nodeObjectKey));
    setSaving(true);
    try {
      const nextConfig = {
        ...config,
        Devices: devices.map((item) => selected.has(nodeObjectKey(item)) ? { ...item, Enabled: enabled } : item),
      };
      const saved = await saveRuntimeConfig(nextConfig);
      try {
        await applyRuntimeConfig();
      } catch (applyError) {
        await saveRuntimeConfig(config);
        await applyRuntimeConfig().catch(() => undefined);
        throw applyError;
      }
      setConfig(saved);
      setSelectedRowKeys([]);
      messageApi.success(enabled ? t('device.batchEnabled') : t('device.batchDisabled'));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('device.saveFailed'));
    } finally {
      setSaving(false);
    }
  }

  async function setDeviceEnabled(record: DeviceRecord, enabled: boolean) {
    const nextConfig = {
      ...config,
      Devices: devices.map((item) => sameNodeObject(item, record) ? { ...item, Enabled: enabled } : item),
    };
    setSaving(true);
    try {
      const saved = await saveRuntimeConfig(nextConfig);
      try {
        await applyRuntimeConfig();
        setConfig(saved);
        messageApi.success(enabled ? t('device.enabled') : t('device.disabled'));
      } catch (applyError) {
        await saveRuntimeConfig(config);
        await applyRuntimeConfig().catch(() => undefined);
        throw applyError;
      }
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('device.saveFailed'));
    } finally {
      setSaving(false);
    }
  }

  function exportDevices(records: DeviceRecord[]) {
    setExportValue(JSON.stringify({ version: 1, type: 'tapx-devices', devices: records }, null, 2));
    setExportOpen(true);
  }

  function openDeviceImport() {
    setImportTargetNodeID(defaultTargetNodeID(scope));
    setImportText('');
    setImportOpen(true);
  }

  async function importDevices() {
    let parsed: unknown;
    try {
      parsed = JSON.parse(importText);
    } catch {
      messageApi.error(t('device.invalidJson'));
      return;
    }
    const raw = Array.isArray(parsed)
      ? parsed
      : parsed && typeof parsed === 'object' && Array.isArray((parsed as { devices?: unknown[] }).devices)
        ? (parsed as { devices: unknown[] }).devices
        : [];
    if (raw.length === 0) {
      messageApi.warning(t('device.noneToImport'));
      return;
    }
    const imported = raw.map((item) => ({
      ...(item as DeviceRecord),
      ManagedNodeID: importTargetNodeID,
      Enabled: false,
    }));
    const seenIDs = new Set<string>();
    const seenInterfaces = new Set<string>();
    for (const item of imported) {
      const id = String(item.ID || '').trim();
      const ifName = String(item.IfName || item.Name || '').trim();
      if (!id || !ifName) {
        messageApi.error(t('device.importMissingIdentity'));
        return;
      }
      const idConflict = seenIDs.has(id) || devices.some((existing) => nodeIDOf(existing) === importTargetNodeID && existing.ID === id);
      const interfaceConflict = seenInterfaces.has(ifName) || devices.some((existing) => nodeIDOf(existing) === importTargetNodeID && (existing.IfName || existing.Name) === ifName);
      if (idConflict || interfaceConflict) {
        messageApi.error(t('device.importConflict', { value: idConflict ? id : ifName }));
        return;
      }
      seenIDs.add(id);
      seenInterfaces.add(ifName);
    }
    setSaving(true);
    try {
      const saved = await saveRuntimeConfig({ ...config, Devices: [...devices, ...imported] });
      setConfig(saved);
      setImportOpen(false);
      setImportText('');
      messageApi.success(t('device.importedCount', { count: imported.length }));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('device.saveFailed'));
    } finally {
      setSaving(false);
    }
  }

  async function copyDeviceExport() {
    try {
      await copyText(exportValue);
      messageApi.success(t('device.copied'));
    } catch {
      messageApi.error(t('device.copyFailed'));
    }
  }

  function confirmDeleteSelectedDevices() {
    const referenced = selectedDevices.filter(isDeviceReferenced);
    if (referenced.length > 0) {
      messageApi.warning(t('device.batchReferenced', { count: referenced.length }));
      return;
    }
    Modal.confirm({
      title: t('device.batchDeleteConfirm', { count: selectedDevices.length }),
      okText: t('common.delete'),
      okType: 'danger',
      cancelText: t('device.cancel'),
      onOk: async () => {
        const selected = new Set(selectedDevices.map(nodeObjectKey));
        setSaving(true);
        try {
          const saved = await saveRuntimeConfig({
            ...config,
            Devices: devices.filter((item) => !selected.has(nodeObjectKey(item))),
          });
          setConfig(saved);
          await applyRuntimeConfig();
          setSelectedRowKeys([]);
          messageApi.success(t('device.deletedCount', { count: selectedDevices.length }));
        } catch (error) {
          messageApi.error(error instanceof Error ? error.message : t('device.deleteFailed'));
        } finally {
          setSaving(false);
        }
      },
    });
  }

  const batchItems: MenuProps['items'] = selectedDevices.length > 0 ? [
    { key: 'enable', icon: <CheckCircleOutlined />, label: t('device.enableSelected') },
    { key: 'disable', icon: <StopOutlined />, label: t('device.disableSelected') },
    { type: 'divider' },
    { key: 'export-selected', icon: <ExportOutlined />, label: t('device.exportSelected') },
    { key: 'delete', icon: <DeleteOutlined />, label: t('device.deleteSelected'), danger: true },
  ] : [
    { key: 'import', icon: <ImportOutlined />, label: t('device.import') },
    { key: 'export', icon: <ExportOutlined />, label: t('device.export'), disabled: visibleDevices.length === 0 },
  ];

  const onBatchClick: MenuProps['onClick'] = ({ key }) => {
    if (key === 'enable') void setSelectedDevicesEnabled(true);
    if (key === 'disable') void setSelectedDevicesEnabled(false);
    if (key === 'import') openDeviceImport();
    if (key === 'export') exportDevices(visibleDevices);
    if (key === 'export-selected') exportDevices(selectedDevices);
    if (key === 'delete') confirmDeleteSelectedDevices();
  };

  const columns = useMemo<TableColumnsType<DeviceRecord>>(() => [
    { title: 'ID', key: 'Index', width: 64, align: 'center', fixed: 'left', render: (_, _record, index) => index + 1 },
    {
      title: t('node.sourceNode'),
      key: 'ManagedNodeID',
      width: 150,
      fixed: 'left',
      render: (_, record) => <NodeSourceTag value={record} />,
    },
    {
      title: t('device.actions'),
      key: 'actions',
      align: 'center',
      width: 100,
      fixed: 'left',
      render: (_, record) => (
        <Space size={4}>
          <Button type="text" icon={<EditOutlined />} aria-label={t('device.edit')} onClick={() => openEdit(record)} />
          <Dropdown trigger={['click']} menu={{ items: [
            { key: 'export', icon: <ExportOutlined />, label: t('device.exportOne'), onClick: () => exportDevices([record]) },
            { key: 'delete', icon: <DeleteOutlined />, label: t('common.delete'), danger: true, onClick: () => Modal.confirm({
              title: t('device.deleteConfirm', { name: record.IfName || record.ID }),
              okText: t('common.delete'),
              okType: 'danger',
              cancelText: t('device.cancel'),
              onOk: () => deleteDevice(record),
            }) },
          ] }}>
            <Button type="text" icon={<MoreOutlined />} aria-label={t('device.more')} />
          </Dropdown>
        </Space>
      ),
    },
    {
      title: t('device.interfaceName'),
      dataIndex: 'IfName',
      key: 'IfName',
      render: (value: string, record) => (
        <Space>
          <span>{value || record.Name || record.ID}</span>
          <Tag color={record.Type === 'tap' ? 'geekblue' : 'green'}>{(record.Type || 'tun').toUpperCase()}</Tag>
        </Space>
      ),
    },
    {
      title: t('common.enabled'),
      dataIndex: 'Enabled',
      key: 'Enabled',
      width: 100,
      align: 'center',
      render: (enabled: boolean, record) => <Switch size="small" checked={enabled !== false} loading={saving} onChange={(checked) => void setDeviceEnabled(record, checked)} />,
    },
    { title: 'MTU', dataIndex: 'MTU', key: 'MTU', width: 90 },
    {
      title: t('device.runMode'),
      key: 'AccessRole',
      width: 110,
      render: (_, record) => inferAccessRole(record) === 'server' ? t('device.serverMode') : t('device.clientMode'),
    },
    {
      title: t('device.accessMode'),
      key: 'TapMode',
      width: 150,
      render: (_, record) => record.Type === 'tap'
        ? <Tag color={tapModeColor(record.TapMode || inferTapMode(record))}>{tapModeLabel(record.TapMode || inferTapMode(record), t)}</Tag>
        : <Tag>{t('device.layer3Mode')}</Tag>,
    },
    {
      title: t('device.addressMode'),
      key: 'DHCP',
      width: 190,
      render: (_, record) => record.Type === 'tap'
        ? <span>{dhcpModeLabel(record.DHCP?.Mode || defaultDHCPMode(record), t)}</span>
        : (
          <Space size={4} wrap>
            <span>{tunDHCPModeLabel(record.TUNDHCP?.Mode || 'off', t)}</span>
            {record.TUNDHCP?.RelayEnabled === true
              ? <Tag color="blue">DHCP Relay</Tag>
              : null}
          </Space>
        ),
    },
    {
      title: 'IPv4',
      key: 'IPv4',
      render: (_: string, record) => tunAddressText(record, 'ipv4', t),
    },
    {
      title: 'IPv6',
      key: 'IPv6',
      render: (_: string, record) => tunAddressText(record, 'ipv6', t),
    },
    {
      title: t('device.source'),
      key: 'Source',
      render: (_, record) => sourceTag(record, t),
    },
    {
      title: t('device.boundEndpoints'),
      key: 'LinkedEndpoints',
      render: (_, record) => {
        const labels = linkedEndpointLabels(record, config.Listeners || [], config.Connectors || []);
        if (labels.length === 0) return '-';
        return (
          <Space wrap size={[4, 4]}>
            {labels.map((label) => <Tag key={label}>{label}</Tag>)}
          </Space>
        );
      },
    },
    {
      title: t('device.physicalInterface'),
      key: 'Bridge',
      render: (_, record) => deviceNetworkText(record),
    },
    {
      title: t('device.status'),
      key: 'RuntimeStatus',
      width: 110,
      fixed: 'right',
      render: (_, record) => <Tag color={record.Enabled !== false ? 'success' : 'default'}>{record.Enabled !== false ? t('common.running') : t('common.stopped')}</Tag>,
    },
  ], [config.Addresses, config.Connectors, config.Listeners, config.Routes, devices, openEdit, saving, t]);

  const modalTabs = [
    {
      key: 'basic',
      label: t('device.basic'),
      children: (
        <>
          <Form.Item name="ID" hidden><Input /></Form.Item>
          <Form.Item name="Name" hidden><Input /></Form.Item>
          <Form.Item name="ManagedNodeID" label={t('node.targetNode')} tooltip={t('node.targetNodeHelp')} rules={[{ required: true }]}>
            <Select options={nodeTargetOptions} disabled={Boolean(editing)} />
          </Form.Item>
          <Form.Item name="Enabled" label={t('common.enabled')} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="IfName" label={t('device.interfaceName')} tooltip={t('device.interfaceNameHelp')} rules={[{ required: true, message: t('device.interfaceNameRequired') }]}>
            <Input placeholder="tapx-tun0" />
          </Form.Item>
          <Form.Item name="Type" label={t('device.type')} tooltip={t('device.typeHelp')} rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'tun', label: 'TUN' },
                { value: 'tap', label: 'TAP' },
              ]}
            />
          </Form.Item>
          <Form.Item name="AccessRole" label={t('device.runMode')} tooltip={t('device.accessRoleHelp')}>
            <Radio.Group className="device-role-options" buttonStyle="solid" onChange={(event) => changeAccessRole(event.target.value)}>
              <Radio.Button value="client">{t('device.clientMode')}</Radio.Button>
              <Radio.Button value="server">{t('device.serverMode')}</Radio.Button>
            </Radio.Group>
          </Form.Item>
          <Form.Item
            name="LinkAutoOptimize"
            label={t('device.linkAutoOptimize')}
            valuePropName="checked"
            tooltip={t('device.linkAutoOptimizeHelp')}
          >
            <Switch />
          </Form.Item>
          <Form.Item
            name="MTU"
            label={linkAutoOptimize ? t('device.mtuCeiling') : 'MTU'}
            tooltip={linkAutoOptimize ? t('device.mtuCeilingHelp') : t('device.mtuHelp')}
          >
            <InputNumber min={576} max={9000} />
          </Form.Item>
          {!linkAutoOptimize ? (
            <Form.Item name="MSSClamp" label={t('device.mssClamp')} tooltip={t('device.mssClampHelp')}>
              <InputNumber min={0} max={9000} placeholder="0" />
            </Form.Item>
          ) : null}
        </>
      ),
    },
    {
      key: 'routes',
      label: t('device.routes'),
      children: (
        <>
          <Form.List name="Routes">
            {(fields, { add, remove }) => (
              <div className="device-route-list">
                {fields.map(({ key, name, ...restField }, index) => (
                  <div key={key} className="device-route-row">
                    <div className="device-route-header">
                      <span>{t('device.routes')} #{index + 1}</span>
                      <div className="device-route-header-actions">
                        <Form.Item {...restField} name={[name, 'Enabled']} valuePropName="checked" noStyle initialValue>
                          <Switch size="small" aria-label={t('common.enabled')} />
                        </Form.Item>
                        <Tooltip title={t('device.deleteRoute', { index: index + 1 })}>
                          <Button danger type="text" size="small" icon={<DeleteOutlined />} aria-label={t('device.deleteRoute', { index: index + 1 })} onClick={() => remove(name)} />
                        </Tooltip>
                      </div>
                    </div>
                    <Row gutter={[12, 0]} align="top">
                      <Col xs={24} md={12}>
                        <Form.Item {...restField} name={[name, 'Destination']} label={t('device.destination')} tooltip={t('device.destinationHelp')} labelCol={{ span: 24 }} wrapperCol={{ span: 24 }} rules={[{ required: true, message: t('device.destinationRequired') }]}>
                          <Input placeholder="10.20.0.0/16" />
                        </Form.Item>
                      </Col>
                      <Col xs={12} md={6}>
                        <Form.Item {...restField} name={[name, 'Gateway']} label={t('device.gateway')} tooltip={t('device.routeGatewayHelp')} labelCol={{ span: 24 }} wrapperCol={{ span: 24 }}>
                          <Input placeholder="10.10.0.1" />
                        </Form.Item>
                      </Col>
                      <Col xs={12} md={6}>
                        <Form.Item {...restField} name={[name, 'Source']} label={t('device.sourceAddress')} tooltip={t('device.sourceAddressHelp')} labelCol={{ span: 24 }} wrapperCol={{ span: 24 }}>
                          <Input placeholder="10.10.0.2" />
                        </Form.Item>
                      </Col>
                    </Row>
                    <Row gutter={[12, 0]} align="top">
                      <Col xs={24} md={11}>
                        <Form.Item {...restField} name={[name, 'IfName']} label={t('device.outputInterface')} tooltip={t('device.outputInterfaceHelp')} labelCol={{ span: 24 }} wrapperCol={{ span: 24 }}>
                          <Input placeholder="tapx-tun0" />
                        </Form.Item>
                      </Col>
                      <Col xs={10} md={4}>
                        <Form.Item {...restField} name={[name, 'Metric']} label="Metric" tooltip={t('device.metricHelp')} labelCol={{ span: 24 }} wrapperCol={{ span: 24 }}>
                          <InputNumber min={0} precision={0} placeholder="100" style={{ width: '100%' }} />
                        </Form.Item>
                      </Col>
                      <Col xs={14} md={9}>
                        <Form.Item {...restField} name={[name, 'Table']} label={t('device.routeTable')} tooltip={t('device.routeTableHelp')} labelCol={{ span: 24 }} wrapperCol={{ span: 24 }}>
                          <Input placeholder="main" />
                        </Form.Item>
                      </Col>
                    </Row>
                  </div>
                ))}
                <div className="device-route-actions">
                  <Button type="dashed" icon={<PlusOutlined />} onClick={() => add({ Enabled: true, Metric: 0 })}>{t('device.addRoute')}</Button>
                </div>
              </div>
            )}
          </Form.List>
        </>
      ),
    },
    ...(deviceType === 'tun' ? [{
      key: 'network',
      label: t('device.networkAccess'),
      children: (
        <>
          {accessRole === 'client' ? (
            <Form.Item name={['TUNDHCP', 'Mode']} label={t('device.addressMode')} tooltip={t('device.tunAddressModeHelp')}>
              <Radio.Group className="device-mode-options device-client-address-options" buttonStyle="solid">
                <Radio.Button value="off">{t('device.dhcpOff')}</Radio.Button>
                <Radio.Button value="client">{t('device.tunDHCPClient')}</Radio.Button>
                <Radio.Button value="manual">{t('device.manualAddress')}</Radio.Button>
              </Radio.Group>
            </Form.Item>
          ) : null}
          {accessRole === 'server' || tunDHCPMode !== 'off' ? (
            <Form.Item name={['TUNDHCP', 'Protocol']} label={t('device.addressProtocol')} tooltip={t('device.tunDHCPProtocolHelp')}>
              <Select options={[
                { value: 'ipv4', label: 'IPv4' },
                { value: 'ipv6', label: 'IPv6' },
                { value: 'dual', label: t('device.dualStack') },
              ]} />
            </Form.Item>
          ) : null}
          {accessRole === 'client' && tunDHCPMode === 'manual' ? (
            <>
              {tunDHCPProtocol !== 'ipv6' ? (
                <Form.Item name={['TUNDHCP', 'IPv4CIDR']} label={t('device.ipv4Cidr')} tooltip={t('device.ipv4CidrHelp')} rules={[{ required: true }]}>
                  <Input placeholder="10.20.0.2/24" />
                </Form.Item>
              ) : null}
              {tunDHCPProtocol !== 'ipv4' ? (
                <Form.Item name={['TUNDHCP', 'IPv6CIDR']} label={t('device.ipv6Cidr')} tooltip={t('device.ipv6CidrHelp')} rules={[{ required: true }]}>
                  <Input placeholder="fd20::2/64" />
                </Form.Item>
              ) : null}
              <Form.Item name={['TUNDHCP', 'Gateway']} label={t('device.gateway')} tooltip={t('device.gatewayHelp')}>
                <Input placeholder="10.20.0.1" />
              </Form.Item>
              <Form.Item name="TUNDHCPDNSList" label="DNS" tooltip={t('device.tunDHCPDNSHelp')}>
                <Select mode="tags" tokenSeparators={[',', ' ', '\n']} placeholder="1.1.1.1, 8.8.8.8" />
              </Form.Item>
            </>
          ) : null}
          {accessRole === 'server' ? (
            <>
              {tunDHCPProtocol !== 'ipv6' ? (
                <Form.Item name={['TUNDHCP', 'IPv4CIDR']} label={t('device.interfaceIPv4Cidr')} tooltip={t('device.serverAddressHelp')} rules={[{ required: true }]}>
                  <Input placeholder="10.20.0.1/24" />
                </Form.Item>
              ) : null}
              {tunDHCPProtocol !== 'ipv4' ? (
                <Form.Item name={['TUNDHCP', 'IPv6CIDR']} label={t('device.interfaceIPv6Cidr')} tooltip={t('device.serverAddressHelp')} rules={[{ required: true }]}>
                  <Input placeholder="fd20::1/64" />
                </Form.Item>
              ) : null}
              <Form.Item name={['TUNDHCP', 'Gateway']} label={t('device.gateway')} tooltip={t('device.serverGatewayHelp')}>
                <Input placeholder="10.20.0.1" />
              </Form.Item>
              <Form.Item name="TUNDHCPDNSList" label="DNS" tooltip={t('device.tunDHCPDNSHelp')}>
                <Select mode="tags" tokenSeparators={[',', ' ', '\n']} placeholder="1.1.1.1, 8.8.8.8" />
              </Form.Item>
            </>
          ) : null}
        </>
      ),
    }, {
      key: 'dhcp',
      label: t('device.dhcpSettings'),
      children: (
        <>
          {accessRole === 'server' ? (
            <>
              <Form.Item name="TUNServerDHCPEnabled" label={t('device.dhcpService')} tooltip={t('device.enableDHCPHelp')} valuePropName="checked">
                <Switch />
              </Form.Item>
              {tunServerDHCPEnabled && tunDHCPProtocol !== 'ipv6' ? (
                <>
                  <Form.Item name={['TUNDHCP', 'PoolStart']} label={t('device.poolStart')} rules={[{ required: true }]}>
                    <Input placeholder="10.20.0.2" />
                  </Form.Item>
                  <Form.Item name={['TUNDHCP', 'PoolEnd']} label={t('device.poolEnd')} rules={[{ required: true }]}>
                    <Input placeholder="10.20.0.254" />
                  </Form.Item>
                </>
              ) : null}
              {tunServerDHCPEnabled && tunDHCPProtocol !== 'ipv4' ? (
                <>
                  <Form.Item name={['TUNDHCP', 'IPv6PoolStart']} label={t('device.ipv6PoolStart')} rules={[{ required: true }]}>
                    <Input placeholder="fd20::2" />
                  </Form.Item>
                  <Form.Item name={['TUNDHCP', 'IPv6PoolEnd']} label={t('device.ipv6PoolEnd')} rules={[{ required: true }]}>
                    <Input placeholder="fd20::ffff" />
                  </Form.Item>
                </>
              ) : null}
              {tunServerDHCPEnabled ? (
                <>
                  <Form.Item name={['TUNDHCP', 'OfferedGateway']} label={t('device.gateway')} tooltip={t('device.dhcpOfferedGatewayHelp')}>
                    <Input placeholder="10.20.0.1" />
                  </Form.Item>
                  <Form.Item name="TUNDHCPServiceDNSList" label="DNS" tooltip={t('device.dhcpOfferedDNSHelp')}>
                    <Select mode="tags" tokenSeparators={[',', ' ', '\n']} placeholder="10.20.0.1, 1.1.1.1" />
                  </Form.Item>
                  <Form.Item name={['TUNDHCP', 'LeaseSeconds']} label={t('device.leaseTime')} tooltip={t('device.leaseTimeHelp')}>
                    <UnitInputNumber min={60} unit="s" />
                  </Form.Item>
                  {tunDHCPProtocol !== 'ipv6' ? (
                    <Form.Item name={['TUNDHCP', 'Authoritative']} label={t('device.authoritativeDHCP')} tooltip={t('device.authoritativeDHCPHelp')} valuePropName="checked">
                      <Switch />
                    </Form.Item>
                  ) : null}
                  <Form.Item name={['TUNDHCP', 'ConflictDetection']} label={t('device.conflictDetection')} tooltip={t('device.tunConflictDetectionHelp')} valuePropName="checked">
                    <Switch />
                  </Form.Item>
                </>
              ) : null}
            </>
          ) : null}
          {accessRole === 'server' ? <Form.Item name={['TUNDHCP', 'RelayEnabled']} label={t('device.enableDHCPRelay')} tooltip={t('device.enableDHCPRelayHelp')} valuePropName="checked">
            <Switch />
          </Form.Item> : null}
          {accessRole === 'server' && relayEnabled ? (
            <>
              <Form.Item name={['TUNDHCP', 'RelayProtocol']} label={t('device.relayProtocol')} tooltip={t('device.relayProtocolHelp')}>
                <Select options={[
                  { value: 'ipv4', label: 'DHCPv4' },
                  { value: 'ipv6', label: 'DHCPv6' },
                  { value: 'dual', label: t('device.dualStack') },
                ]} />
              </Form.Item>
              <Form.Item name="RelayDownstreamList" label={t('device.relayDownstream')} tooltip={t('device.relayDownstreamHelp')} rules={[{ required: true }]}>
                <Select
                  mode="multiple"
                  showSearch
                  options={interfaceOptions}
                  placeholder="br-lan"
                  filterOption={(input, option) => String(option?.value || '').toLowerCase().includes(input.toLowerCase())}
                />
              </Form.Item>
              <Form.Item name="RelayServerList" label={t('device.relayServers')} tooltip={t('device.relayServersHelp')} rules={[{ required: true }]}>
                <Select mode="tags" tokenSeparators={[',', ' ', '\n']} placeholder="10.20.0.1, 10.20.0.2" />
              </Form.Item>
			  <Form.Item name={['TUNDHCP', 'MaxHops']} label={t('device.maxRelayHops')} tooltip={t('device.maxRelayHopsHelp')}>
				<InputNumber min={1} max={16} />
			  </Form.Item>
              <Form.Item wrapperCol={{ offset: 7, span: 17 }}>
                <Button onClick={() => void refreshInterfaces(targetNodeID)}>{t('device.refreshInterfaces')}</Button>
              </Form.Item>
            </>
          ) : null}
        </>
      ),
    }] : []),
    ...(deviceType === 'tap' ? [
      {
        key: 'network',
        label: t('device.networkAccess'),
        children: (
          <>
            <Form.Item name="TapMode" label={t('device.accessMode')} tooltip={t('device.accessModeHelp')}>
              <Radio.Group className="device-mode-options" buttonStyle="solid">
                <Radio.Button value="standalone">{t('device.modeStandalone')}</Radio.Button>
                <Radio.Button value="transparent">{t('device.modeTransparent')}</Radio.Button>
                <Radio.Button value="one-arm">{t('device.modeOneArm')}</Radio.Button>
                <Radio.Button value="shared-ip">{t('device.modeSharedIP')}</Radio.Button>
              </Radio.Group>
            </Form.Item>

            {tapMode !== 'standalone' && !(tapMode === 'shared-ip' && accessRole === 'server') ? (
              <>
                <Form.Item name="BridgeName" label={t('device.bridgeName')} tooltip={t('device.bridgeNameHelp')} rules={[{ required: true }]}>
                  <Input placeholder="br-tapx" />
                </Form.Item>
                <Form.Item name="BridgeMember" label={t('device.bridgeMember')} tooltip={t('device.bridgeMemberHelp')} rules={[{ required: true }]}>
                  <Select
                    showSearch
                    options={interfaceOptions}
                    placeholder="lan5"
                    filterOption={(input, option) => String(option?.value || '').toLowerCase().includes(input.toLowerCase())}
                    notFoundContent={t('device.noBridgeInterface')}
                  />
                </Form.Item>
                <Form.Item name="BridgeMTU" label={t('device.bridgeMtu')} tooltip={t('device.bridgeMtuHelp')}>
                  <InputNumber min={576} max={9000} style={{ width: 140 }} />
                </Form.Item>
              </>
            ) : null}

            {tapMode === 'one-arm' ? (
              <Form.Item name="OneArmRollbackSeconds" label={t('device.rollbackTimeout')} tooltip={t('device.rollbackTimeoutHelp')}>
                <UnitInputNumber min={15} max={600} unit="s" style={{ width: 180 }} />
              </Form.Item>
            ) : null}

            {tapMode === 'shared-ip' && accessRole === 'server' ? (
              <>
                <Form.Item name={['SharedIP', 'UplinkInterface']} label={t('device.uplinkInterface')} tooltip={t('device.uplinkInterfaceHelp')} rules={[{ required: true }]}>
                  <Select
                    showSearch
                    options={interfaceOptions}
                    placeholder="eth0"
                    filterOption={(input, option) => String(option?.value || '').toLowerCase().includes(input.toLowerCase())}
                  />
                </Form.Item>
                <Form.Item name={['SharedIP', 'AddressSource']} label={t('device.sharedAddressSource')} tooltip={t('device.sharedAddressSourceHelp')}>
                  <Radio.Group className="device-inline-options" buttonStyle="solid">
                    <Radio.Button value="auto">{t('device.followUplink')}</Radio.Button>
                    <Radio.Button value="manual">{t('device.manual')}</Radio.Button>
                  </Radio.Group>
                </Form.Item>
                {sharedAddressSource === 'manual' ? (
                  <>
                    <Form.Item name={['SharedIP', 'IPv4CIDR']} label={t('device.sharedIPv4')} rules={[{ required: true }]}>
                      <Input placeholder="203.0.113.10/24" />
                    </Form.Item>
                    <Form.Item name={['SharedIP', 'Gateway']} label={t('device.gateway')}>
                      <Input placeholder="203.0.113.1" />
                    </Form.Item>
                    <Form.Item name="SharedDNSList" label="DNS">
                      <Select mode="tags" tokenSeparators={[',', '\n']} placeholder="1.1.1.1, 8.8.8.8" />
                    </Form.Item>
                  </>
                ) : null}
                <Form.Item name={['SharedIP', 'FirewallBackend']} label={t('device.firewallBackend')} tooltip={t('device.firewallBackendHelp')}>
                  <Select options={[
                    { value: 'auto', label: t('device.firewallAuto') },
                    { value: 'nftables', label: 'nftables' },
                    { value: 'iptables', label: t('device.iptablesCompatible') },
                  ]} />
                </Form.Item>
                <Form.Item name={['SharedIP', 'HostPortPriority']} label={t('device.hostPortPriority')} tooltip={t('device.hostPortPriorityHelp')} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item name={['SharedIP', 'TrackAddressChanges']} label={t('device.trackAddressChanges')} tooltip={t('device.trackAddressChangesHelp')} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item name={['SharedIP', 'ReservedTCPPorts']} label={t('device.reservedTCPPorts')} tooltip={t('device.reservedPortsHelp')}>
                  <Select mode="tags" tokenSeparators={[',', ' ']} placeholder="22, 2053, 44000-44100" />
                </Form.Item>
                <Form.Item name={['SharedIP', 'ReservedUDPPorts']} label={t('device.reservedUDPPorts')} tooltip={t('device.reservedPortsHelp')}>
                  <Select mode="tags" tokenSeparators={[',', ' ']} placeholder="53, 44000-44100" />
                </Form.Item>
              </>
            ) : null}

            {tapMode === 'shared-ip' && accessRole === 'client' ? (
              <Form.Item name={['SharedIP', 'ClientMAC']} label={t('device.clientMAC')} tooltip={t('device.clientMACHelp')}>
                <Input placeholder="00:11:22:33:44:55" />
              </Form.Item>
            ) : null}

            {tapMode !== 'standalone' ? (
              <Form.Item wrapperCol={{ offset: 7, span: 17 }}>
                <Button onClick={() => void refreshInterfaces(targetNodeID)}>{t('device.refreshInterfaces')}</Button>
              </Form.Item>
            ) : null}
          </>
        ),
      },
      {
        key: 'dhcp',
        label: t('device.dhcpSettings'),
        children: (
          <>

            {accessRole === 'server' && tapMode !== 'shared-ip' ? (
              <>
                <Form.Item name={['DHCP', 'Mode']} hidden><Input /></Form.Item>
                <Form.Item label={t('device.dhcpPassthrough')} tooltip={t('device.dhcpModeHelp')}>
                  <Switch
                    checked={dhcpMode === 'passthrough'}
                    onChange={(checked) => form.setFieldValue(['DHCP', 'Mode'], checked ? 'passthrough' : 'off')}
                  />
                </Form.Item>
                <Form.Item label={t('device.dhcpService')} tooltip={t('device.enableDHCPHelp')}>
                  <Switch
                    checked={dhcpMode === 'server'}
                    onChange={(checked) => form.setFieldValue(['DHCP', 'Mode'], checked ? 'server' : 'off')}
                  />
                </Form.Item>
              </>
            ) : null}

            {accessRole === 'server' && dhcpMode === 'server' && tapMode !== 'shared-ip' ? (
              <>
                <Form.Item name={['DHCP', 'IPv4CIDR']} label={t('device.interfaceIPv4Cidr')} tooltip={t('device.serverAddressHelp')} rules={[{ required: true }]}>
                  <Input placeholder="192.168.10.1/24" />
                </Form.Item>
                <Form.Item name={['DHCP', 'PoolStart']} label={t('device.poolStart')} rules={[{ required: true }]}>
                  <Input placeholder="192.168.10.100" />
                </Form.Item>
                <Form.Item name={['DHCP', 'PoolEnd']} label={t('device.poolEnd')} rules={[{ required: true }]}>
                  <Input placeholder="192.168.10.200" />
                </Form.Item>
                <Form.Item name={['DHCP', 'Gateway']} label={t('device.gateway')} tooltip={t('device.serverGatewayHelp')}>
                  <Input placeholder="192.168.10.1" />
                </Form.Item>
                <Form.Item name="DHCPDNSList" label="DNS" tooltip={t('device.dhcpDNSHelp')}>
                  <Select mode="tags" tokenSeparators={[',', '\n']} placeholder="192.168.10.1, 1.1.1.1" />
                </Form.Item>
                <Form.Item name={['DHCP', 'LeaseSeconds']} label={t('device.leaseTime')} tooltip={t('device.leaseTimeHelp')}>
                  <UnitInputNumber min={60} unit="s" style={{ width: 220 }} />
                </Form.Item>
                <Form.Item name={['DHCP', 'Authoritative']} label={t('device.authoritativeDHCP')} tooltip={t('device.authoritativeDHCPHelp')} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item name={['DHCP', 'ConflictDetection']} label={t('device.conflictDetection')} tooltip={t('device.conflictDetectionHelp')} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.List name={['DHCP', 'StaticLeases']}>
                  {(fields, { add, remove }) => (
                    <div className="device-lease-list">
                      <div className="device-list-heading">
                        <span>{t('device.staticLeases')}</span>
                        <Button type="dashed" size="small" icon={<PlusOutlined />} onClick={() => add()}>{t('device.addLease')}</Button>
                      </div>
                      {fields.map(({ key, name, ...restField }) => (
                        <Row key={key} gutter={[8, 8]} align="middle" className="device-lease-row">
                          <Col xs={24} md={7}><Form.Item {...restField} name={[name, 'Name']} noStyle><Input placeholder={t('device.leaseNameExample')} /></Form.Item></Col>
                          <Col xs={24} md={8}><Form.Item {...restField} name={[name, 'MAC']} noStyle><Input placeholder="00:11:22:33:44:55" /></Form.Item></Col>
                          <Col xs={20} md={7}><Form.Item {...restField} name={[name, 'Address']} noStyle><Input placeholder="192.168.10.10" /></Form.Item></Col>
                          <Col xs={4} md={2}><Button type="text" danger icon={<DeleteOutlined />} onClick={() => remove(name)} /></Col>
                        </Row>
                      ))}
                    </div>
                  )}
                </Form.List>
              </>
            ) : null}
          </>
        ),
      },
    ] : []),
    {
      key: 'advanced',
      label: t('common.advancedConfig'),
      forceRender: true,
      children: <AdvancedConfigEditor form={form} />,
    },
  ];

  const tabOrder = deviceType === 'tap'
    ? ['basic', 'network', ...(accessRole === 'server' && tapMode !== 'shared-ip' ? ['dhcp'] : []), 'advanced']
    : ['basic', 'network', 'dhcp', 'routes', 'advanced'];
  const visibleModalTabs = modalTabs
    .filter((item) => tabOrder.includes(item.key))
    .sort((left, right) => tabOrder.indexOf(left.key) - tabOrder.indexOf(right.key));

  return (
    <div className="devices-page">
      {messageContextHolder}
      <Row gutter={[16, 12]}>
        <Col span={24}>
          <Card hoverable className="summary-card">
            <Space wrap>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                {t('device.add')}
              </Button>
              <NodeScopeSelect scope={scope} onChange={setScope} />
              {selectedDevices.length > 0 ? <Tag color="blue" closable onClose={() => setSelectedRowKeys([])}>{t('device.selectedCount', { count: selectedDevices.length })}</Tag> : null}
              <Dropdown trigger={['click']} menu={{ items: batchItems, onClick: onBatchClick }}>
                <Button icon={<MenuOutlined />}>{selectedDevices.length > 0 ? t('device.batchActions') : t('device.more')}</Button>
              </Dropdown>
              <Input.Search
                className="device-search"
                placeholder={t('device.searchPlaceholder')}
                allowClear
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </Space>
          </Card>
        </Col>
        <Col span={24}>
          <Card hoverable>
            <Table
              rowKey={nodeObjectKey}
              rowSelection={{ selectedRowKeys, onChange: (keys) => setSelectedRowKeys(keys.map(String)) }}
              columns={columns}
              dataSource={filteredDevices}
              loading={loading || saving}
              pagination={false}
              scroll={{ x: 1620 }}
              size="middle"
              locale={{ emptyText: t('device.empty') }}
            />
          </Card>
        </Col>
      </Row>

      <Modal
        className="device-editor-modal"
        open={open}
        title={editing ? t('device.editTitle') : t('device.addTitle')}
        okText={t('common.save')}
        cancelText={t('device.cancel')}
        width={920}
        forceRender
        confirmLoading={saving}
        onOk={submit}
        onCancel={() => setOpen(false)}
      >
        <Form form={form} layout="horizontal" labelCol={{ span: 7 }} wrapperCol={{ span: 17 }}>
          <Tabs items={visibleModalTabs} />
        </Form>
      </Modal>

      <Modal open={importOpen} title={t('device.import')} okText={t('device.importAction')} cancelText={t('device.cancel')} confirmLoading={saving} onOk={() => void importDevices()} onCancel={() => setImportOpen(false)}>
        <Form layout="vertical">
          <Form.Item label={t('node.targetNode')} required><Select value={importTargetNodeID} options={nodeTargetOptions} onChange={setImportTargetNodeID} /></Form.Item>
          <Form.Item label={t('device.importJson')}><Input.TextArea value={importText} onChange={(event) => setImportText(event.target.value)} autoSize={{ minRows: 10, maxRows: 20 }} placeholder={'{\n  "type": "tapx-devices",\n  "devices": []\n}'} /></Form.Item>
        </Form>
      </Modal>

      <Modal open={exportOpen} title={t('device.export')} okText={t('device.copy')} cancelText={t('device.cancel')} onOk={() => void copyDeviceExport()} onCancel={() => setExportOpen(false)}>
        <Input.TextArea value={exportValue} readOnly autoSize={{ minRows: 10, maxRows: 20 }} />
      </Modal>
    </div>
  );
}

export function normalizeInterfaceNames(input: unknown): string[] {
  const names = new Set<string>();
  const addName = (value: unknown) => {
    if (typeof value !== 'string') return;
    const trimmed = value.trim();
    if (trimmed) names.add(trimmed);
  };
  const values = Array.isArray(input)
    ? input
    : input && typeof input === 'object' && Array.isArray((input as { obj?: unknown }).obj)
      ? (input as { obj: unknown[] }).obj
      : [];
  if (Array.isArray(values)) {
    for (const item of values) {
      if (typeof item === 'string') addName(item);
      else if (item && typeof item === 'object') addName((item as { name?: unknown; Name?: unknown; IfName?: unknown }).name ?? (item as { Name?: unknown }).Name ?? (item as { IfName?: unknown }).IfName);
    }
  }
  return Array.from(names).sort((a, b) => a.localeCompare(b));
}

function makeDeviceId(name = '') {
  const suffix = name.trim().replace(/[^A-Za-z0-9_.-]/g, '-').slice(0, 32);
  return `dev-${suffix || Date.now()}`;
}

export function uniqueDeviceID(devices: DeviceRecord[], preferredID: string, managedNodeID?: string): string {
  const targetNodeID = managedNodeID || 'local';
  const used = new Set(
    devices
      .filter((item) => nodeIDOf(item) === targetNodeID)
      .map((item) => String(item.ID || '').trim())
      .filter(Boolean),
  );
  if (!used.has(preferredID)) return preferredID;
  let suffix = 2;
  while (used.has(`${preferredID}-${suffix}`)) suffix += 1;
  return `${preferredID}-${suffix}`;
}

function nextDeviceName(devices: DeviceRecord[], prefix: string): string {
  const names = new Set(devices.map((item) => item.Name || item.IfName).filter(Boolean));
  let index = 0;
  let name = `${prefix}${index}`;
  while (names.has(name)) {
    index += 1;
    name = `${prefix}${index}`;
  }
  return name;
}

function tunAddressText(record: DeviceRecord, family: 'ipv4' | 'ipv6', t: ReturnType<typeof useI18n>['t']) {
  if (record.Type !== 'tun') return '-';
  const mode = record.TUNDHCP?.Mode || 'off';
  if (mode === 'client') return t('device.auto');
  if (mode !== 'server' && mode !== 'manual') return '-';
  return family === 'ipv4' ? record.TUNDHCP?.IPv4CIDR || '-' : record.TUNDHCP?.IPv6CIDR || '-';
}

function tunDHCPModeLabel(mode: string, t: ReturnType<typeof useI18n>['t']) {
  if (mode === 'client') return t('device.tunDHCPClient');
  if (mode === 'server') return t('device.dhcpServer');
  if (mode === 'manual') return t('device.manualAddress');
  return t('device.dhcpOff');
}

function sourceTag(record: DeviceRecord, t: ReturnType<typeof useI18n>['t']) {
  const source = record.Source || 'manual';
  if (source === 'listener-auto') return <Tag color="blue">{t('device.sourceListener')}</Tag>;
  if (source === 'connector-auto') return <Tag color="purple">{t('device.sourceConnector')}</Tag>;
  return <Tag>{t('device.sourceManual')}</Tag>;
}

function linkedEndpointLabels(record: DeviceRecord, listeners: TapxEndpoint[], connectors: TapxEndpoint[]): string[] {
  const labels = [...listeners, ...connectors]
    .filter((endpoint) => endpoint.Binding?.DeviceID === record.ID && nodeIDOf(endpoint) === nodeIDOf(record))
    .map((endpoint) => endpoint.Name || `#${endpoint.ID}`);
  return Array.from(new Set(labels));
}

function inferTapMode(record: DeviceRecord): TapxTapMode {
  if (record.Type !== 'tap') return 'standalone';
  return record.Bridge?.Enabled ? 'transparent' : 'standalone';
}

function inferAccessRole(record: DeviceRecord): 'client' | 'server' {
  return record.AccessRole || 'client';
}

function defaultDHCPMode(record: DeviceRecord): TapxDHCPMode {
  const mode = record.TapMode || inferTapMode(record);
  if (mode === 'shared-ip') return record.SharedIP?.Role === 'access' ? 'passthrough' : 'mirror';
  return mode === 'transparent' || mode === 'one-arm' ? 'passthrough' : 'off';
}

function tapModeLabel(mode: TapxTapMode, t: ReturnType<typeof useI18n>['t']) {
  const labels: Record<TapxTapMode, string> = {
    standalone: t('device.modeStandalone'),
    transparent: t('device.modeTransparent'),
    'one-arm': t('device.modeOneArm'),
    'shared-ip': t('device.modeSharedIP'),
  };
  return labels[mode];
}

function tapModeColor(mode: TapxTapMode) {
  if (mode === 'shared-ip') return 'gold';
  if (mode === 'one-arm') return 'purple';
  if (mode === 'transparent') return 'blue';
  return 'default';
}

function dhcpModeLabel(mode: TapxDHCPMode, t: ReturnType<typeof useI18n>['t']) {
  const labels: Record<TapxDHCPMode, string> = {
    off: t('device.dhcpOff'),
    passthrough: t('device.dhcpPassthrough'),
    server: t('device.dhcpServer'),
    mirror: t('device.dhcpMirror'),
  };
  return labels[mode];
}

function deviceNetworkText(record: DeviceRecord) {
  if (record.Type !== 'tap') return '-';
  const mode = record.TapMode || inferTapMode(record);
  if (mode === 'shared-ip' && record.SharedIP?.Role === 'service') return record.SharedIP.UplinkInterface || '-';
  if (mode === 'standalone') return '-';
  return record.Bridge?.IfName ? `${record.Bridge.Name || 'br-tapx'} / ${record.Bridge.IfName}` : record.Bridge?.Name || '-';
}
