import { useEffect, useMemo, useState } from 'react';
import dayjs, { type Dayjs } from 'dayjs';
import {
  Badge,
  Button,
  Card,
  Checkbox,
  Divider,
  Drawer,
  Dropdown,
  DatePicker,
  Form,
  Input,
  InputNumber,
  Modal,
  Popover,
  Pagination,
  Progress,
  Popconfirm,
  Radio,
  Select,
  Space,
  Switch,
  Spin,
  Table,
  Tabs,
  Tag,
  Tooltip,
  message,
  type MenuProps,
  type TableColumnsType,
} from 'antd';
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  ExportOutlined,
  FilterOutlined,
  ImportOutlined,
  InfoCircleOutlined,
  MoreOutlined,
  PlusOutlined,
  ReloadOutlined,
  RetweetOutlined,
  ClockCircleOutlined,
  DisconnectOutlined,
  RestOutlined,
  UsergroupAddOutlined,
  UsergroupDeleteOutlined,
  TeamOutlined,
} from '@ant-design/icons';
import {
  getClientShare,
  resetClientTraffic,
  type ClientQuotaState,
  type RuntimeConfig,
  type TapxAddressLimit,
  type TapxClient,
  type TapxDevice,
  type TapxEndpoint,
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
  type NodeOwned,
} from '../features/nodes/managedConfig';
import { NodeScopeSelect, NodeSourceTag, useNodeScope, useNodeTargetOptions } from '../features/nodes/NodeScope';
import { errorMessage } from '../shared/localized-error';
import { formatBytes } from '../shared/format';
import { copyText } from '../shared/clipboard';
import { labelDevice, labelEndpoint, nextId, splitList } from '../shared/tapx-model';
import { BulkAdjustModal, BulkCreateModal, ListenerBindingModal, type BulkCreateInput } from '../features/users/UserBulkModals';
import { applyBulkUserAdjustment, changeListenerIDs, type BulkUserAdjustment } from '../features/users/userBulk';
import { exportUserBundle, importUserBundle } from '../features/users/userTransfer';
import { useMediaQuery } from '../shared/useMediaQuery';
import { isManagedUserAddressRemark, managedUserAddressRemark } from '../shared/managed-objects';
import { randomLowerAndNumber, randomUUID } from '../shared/random';
import { unixSeconds as nowSecond } from '../shared/time';
import { userProtocols } from '../features/users/userProtocols';
import { materializeClientVKey } from '../features/endpoints/vkeyBinding';
import {
  isShadowsocks2022Password,
  randomShadowsocksPassword,
  shadowsocksRequirements,
  validateUserCredentialSet,
} from '../features/users/userCredentials';
import { useI18n } from '../i18n/I18nProvider';
import './UserPage.css';

type CredentialType =
  | 'vless'
  | 'vmess'
  | 'trojan'
  | 'shadowsocks'
  | 'hysteria'
  | 'wireguard'
  | 'raw-tcp'
  | 'raw-udp';

type TrafficShape = {
  up?: number;
  down?: number;
  Up?: number;
  Down?: number;
  lastOnline?: number;
  LastOnline?: number;
};

type UserRecord = TapxClient & NodeOwned & {
  CredentialType?: CredentialType | string;
  ListenerIDs?: string[];
  Traffic?: TrafficShape;
  traffic?: TrafficShape;
  TrafficUp?: number;
  TrafficDown?: number;
  CreatedAt?: number;
  UpdatedAt?: number;
  Online?: boolean;
  online?: boolean;
  IsOnline?: boolean;
};

type UserDraft = UserRecord & {
  ListenerIDs?: string[];
  AllowedDevicesText?: string[];
  AllowedIPsText?: string;
  AllowedIPv6Text?: string;
  AllowedMACsText?: string;
  DelayedStart?: boolean;
  ExpireDays?: number;
  ExpireAtValue?: Dayjs | null;
  UploadRateMbps?: number | null;
  DownloadRateMbps?: number | null;
};

type UserFilters = {
  statuses: string[];
  protocols: string[];
  listenerIds: string[];
  usageFromGB?: number;
  usageToGB?: number;
  hasVKey: '' | 'yes' | 'no';
  hasRemark: '' | 'yes' | 'no';
};

type ExportModalState = {
  open: boolean;
  title: string;
  value: string;
};

const defaultUser: UserDraft = {
  ID: '',
  Enabled: true,
  Name: '',
  Email: '',
  ListenerID: '',
  ListenerIDs: [],
  CredentialType: 'vless',
  CredentialValue: '',
  UUID: '',
  Password: '',
  Auth: '',
  VKey: '',
  TrafficCap: 0,
  TrafficReset: 'never',
  AllowedDevicesText: [],
  AllowedIPsText: '',
  AllowedIPv6Text: '',
  AllowedMACsText: '',
  DelayedStart: false,
  ExpireDays: 0,
  ExpireAtValue: null,
};

const emptyFilters: UserFilters = {
  statuses: [],
  protocols: [],
  listenerIds: [],
  usageFromGB: undefined,
  usageToGB: undefined,
  hasVKey: '',
  hasRemark: '',
};

const credentialOptions = [
  { value: 'vless', label: 'VLESS' },
  { value: 'vmess', label: 'VMess' },
  { value: 'trojan', label: 'Trojan' },
  { value: 'shadowsocks', label: 'Shadowsocks' },
  { value: 'hysteria', label: 'Hysteria' },
  { value: 'wireguard', label: 'WireGuard' },
  { value: 'raw-tcp', label: 'Raw TCP' },
  { value: 'raw-udp', label: 'Raw UDP' },
];

const userTabOrder: Record<string, number> = { basic: 0, limits: 1, credential: 2 };
const sortValues = ['created:asc', 'created:desc', 'updated:desc', 'online:desc', 'email:asc', 'email:desc', 'traffic:desc', 'remaining:desc', 'expiry:asc'];
const userFilterStorageKey = 'tapx-user-filter-state';

function readUserFilterState(): { filters: UserFilters; search: string; sort: string } {
  try {
    const parsed = JSON.parse(localStorage.getItem(userFilterStorageKey) || '{}') as Partial<{ filters: UserFilters; search: string; sort: string }>;
    return {
      filters: { ...emptyFilters, ...(parsed.filters || {}) },
      search: typeof parsed.search === 'string' ? parsed.search : '',
      sort: sortValues.includes(parsed.sort || '') ? parsed.sort! : sortValues[0],
    };
  } catch {
    return { filters: emptyFilters, search: '', sort: sortValues[0] };
  }
}

export function UserPage() {
  const { t } = useI18n();
  const [config, setConfig] = useState<RuntimeConfig>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [open, setOpen] = useState(false);
  const [info, setInfo] = useState<UserRecord | null>(null);
  const [shareLinks, setShareLinks] = useState<string[]>([]);
  const [shareLoading, setShareLoading] = useState(false);
  const [editing, setEditing] = useState<UserRecord | null>(null);
  const [filters, setFilters] = useState<UserFilters>(() => readUserFilterState().filters);
  const [filterOpen, setFilterOpen] = useState(false);
  const [search, setSearch] = useState(() => readUserFilterState().search);
  const [sort, setSort] = useState(() => readUserFilterState().sort);
  const [selectedRowKeys, setSelectedRowKeys] = useState<string[]>([]);
  const [exportModal, setExportModal] = useState<ExportModalState>({ open: false, title: '', value: '' });
  const [importOpen, setImportOpen] = useState(false);
  const [importText, setImportText] = useState('');
  const [importTargetNodeID, setImportTargetNodeID] = useState('local');
  const [bindingMode, setBindingMode] = useState<'attach' | 'detach' | null>(null);
  const [adjustOpen, setAdjustOpen] = useState(false);
  const [bulkCreateOpen, setBulkCreateOpen] = useState(false);
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);
  const [form] = Form.useForm<UserDraft>();
  const [messageApi, messageContextHolder] = message.useMessage();
  const { nodes, scope, setScope } = useNodeScope();
  const nodeTargetOptions = useNodeTargetOptions(nodes);
  const isMobile = useMediaQuery('(max-width: 760px)');
  const sortOptions = useMemo(() => [
    { value: 'created:asc', label: t('user.sort.oldest') },
    { value: 'created:desc', label: t('user.sort.newest') },
    { value: 'updated:desc', label: t('user.sort.updated') },
    { value: 'online:desc', label: t('user.sort.online') },
    { value: 'email:asc', label: t('user.sort.az') },
    { value: 'email:desc', label: t('user.sort.za') },
    { value: 'traffic:desc', label: t('user.sort.traffic') },
    { value: 'remaining:desc', label: t('user.sort.remaining') },
    { value: 'expiry:asc', label: t('user.sort.expiry') },
  ], [t]);

  const users = useMemo(() => ((config.Clients || []) as UserRecord[]), [config.Clients]);
  const listeners = useMemo(() => ((config.Listeners || []) as TapxEndpoint[]), [config.Listeners]);
  const devices = useMemo(() => ((config.Devices || []) as TapxDevice[]), [config.Devices]);
  const addresses = useMemo(() => ((config.Addresses || []) as TapxAddressLimit[]), [config.Addresses]);
  const delayedStart = Form.useWatch('DelayedStart', form) === true;
  const targetNodeID = String(Form.useWatch('ManagedNodeID', form) || defaultTargetNodeID(scope));
  const scopedUsers = useMemo(() => filterNodeOwned(users, scope), [scope, users]);
  const scopedListeners = useMemo(
    () => filterNodeOwned(listeners as Array<TapxEndpoint & NodeOwned>, scope),
    [listeners, scope],
  );
  const stats = useMemo(() => buildUserStats(scopedUsers), [scopedUsers]);
  const activeFilterCount = countFilters(filters);

  useEffect(() => {
    try {
      localStorage.setItem(userFilterStorageKey, JSON.stringify({ filters, search, sort }));
    } catch {
      // Filters still work for the current session when storage is unavailable.
    }
  }, [filters, search, sort]);

  useEffect(() => {
    if (!info?.ID) {
      setShareLinks([]);
      return;
    }
    let active = true;
    setShareLoading(true);
    getClientShare(info.ID, nodeIDOf(info))
      .then((share) => {
        if (active) setShareLinks((share.links?.length ? share.links : [share.link]).filter(Boolean));
      })
      .catch((error: unknown) => {
        if (!active) return;
        setShareLinks([]);
        messageApi.error(errorMessage(error, t, 'user.shareLoadFailed'));
      })
      .finally(() => {
        if (active) setShareLoading(false);
      });
    return () => {
      active = false;
    };
  }, [info?.ID, info?.ManagedNodeID, messageApi]);

  const listenerOptions = useMemo(
    () => filterNodeOwned(listeners as Array<TapxEndpoint & NodeOwned>, targetNodeID).map((item) => ({ value: item.ID, label: labelEndpoint(item) })),
    [listeners, targetNodeID],
  );
  const deviceOptions = useMemo(
    () => filterNodeOwned(devices as Array<TapxDevice & NodeOwned>, targetNodeID).filter((item) => item.Enabled !== false).map((item) => ({ value: item.ID, label: labelDevice(item) })),
    [devices, targetNodeID],
  );
  const protocolOptions = useMemo(() => {
    const values = new Set(users.flatMap((item) => userProtocols(item, listeners, config.XrayProfiles || [])));
    credentialOptions.forEach((item) => values.add(item.value));
    return [...values].sort().map((value) => ({ value, label: credentialLabel(value) }));
  }, [config.XrayProfiles, listeners, users]);

  const filteredUsers = useMemo(() => {
    const normalizedSearch = search.trim().toLowerCase();
    const next = scopedUsers.filter((record) => {
      const actualProtocols = userProtocols(record, listeners, config.XrayProfiles || []);
      if (normalizedSearch) {
        const haystack = [
          record.ID,
          record.Email,
          record.Name,
          record.Remark,
          record.VKey,
          record.UUID,
          record.Password,
          record.Auth,
          ...actualProtocols,
        ].filter(Boolean).join(' ').toLowerCase();
        if (!haystack.includes(normalizedSearch)) return false;
      }

      if (filters.statuses.length > 0 && !filters.statuses.some((status) => statusMatches(record, status))) return false;
      if (filters.protocols.length > 0 && !filters.protocols.some((protocol) => actualProtocols.includes(protocol))) return false;
      if (filters.listenerIds.length > 0) {
        const ids = listenerIdsFor(record);
        if (!filters.listenerIds.some((id) => ids.includes(id))) return false;
      }

      const usedGB = trafficUsedBytes(record) / 1024 / 1024 / 1024;
      if (typeof filters.usageFromGB === 'number' && usedGB < filters.usageFromGB) return false;
      if (typeof filters.usageToGB === 'number' && usedGB > filters.usageToGB) return false;
      if (filters.hasVKey === 'yes' && !record.VKey) return false;
      if (filters.hasVKey === 'no' && record.VKey) return false;
      if (filters.hasRemark === 'yes' && !record.Remark && !record.Name) return false;
      if (filters.hasRemark === 'no' && (record.Remark || record.Name)) return false;
      return true;
    });
    return sortUsers(next, sort);
  }, [config.XrayProfiles, filters, listeners, scopedUsers, search, sort]);

  useEffect(() => {
    setCurrentPage(1);
  }, [filters, search, sort]);

  const pagedUsers = useMemo(
    () => filteredUsers.slice((currentPage - 1) * pageSize, currentPage * pageSize),
    [currentPage, filteredUsers, pageSize],
  );

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    setSelectedRowKeys((prev) => prev.filter((id) => scopedUsers.some((item) => nodeObjectKey(item) === id)));
  }, [scopedUsers]);

  async function refresh() {
    setLoading(true);
    try {
      const [nextConfig, report] = await Promise.all([getRuntimeConfig(), getStats()]);
      setConfig(hydrateUserStats(hydrateUserConfig(nextConfig), report.clients || []));
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('user.loadFailed'));
    } finally {
      setLoading(false);
    }
  }

  function openCreate() {
    const ids = new Set(users.map((item) => item.ID));
    const id = nextId('user', ids);
    const uuid = randomUUID();
    form.resetFields();
    form.setFieldsValue({
      ...defaultUser,
      ID: id,
      ManagedNodeID: defaultTargetNodeID(scope),
      Name: '',
      Email: `${id}@tapx.local`,
      UUID: uuid,
      Password: randomLowerAndNumber(16),
      Auth: randomLowerAndNumber(16),
      VKey: randomLowerAndNumber(24),
      CredentialValue: uuid,
      ListenerIDs: [],
    });
    setEditing(null);
    setOpen(true);
  }

  function openEdit(record: UserRecord) {
    const address = record.AddressID
      ? addresses.find((item) => item.ID === record.AddressID && nodeIDOf(item) === nodeIDOf(record))
      : record.Binding?.AddressID
        ? addresses.find((item) => item.ID === record.Binding?.AddressID && nodeIDOf(item) === nodeIDOf(record))
        : undefined;
    form.resetFields();
    form.setFieldsValue({
      ...defaultUser,
      ...record,
      ListenerIDs: listenerIdsFor(record),
      UUID: record.UUID || (usesUuid(record.CredentialType) ? record.CredentialValue : '') || '',
      Password: record.Password || (record.CredentialType === 'trojan' || record.CredentialType === 'shadowsocks' ? record.CredentialValue : '') || '',
      Auth: record.Auth || (record.CredentialType === 'hysteria' ? record.CredentialValue : '') || '',
      VKey: record.VKey || '',
      TrafficCap: bytesToGB(record.TrafficCap),
      UploadRateMbps: bpsToMbps(record.UploadRateLimit),
      DownloadRateMbps: bpsToMbps(record.DownloadRateLimit),
      DelayedStart: Number(record.ExpiresAt || 0) < 0,
      ExpireDays: Number(record.ExpiresAt || 0) < 0 ? Math.round(Number(record.ExpiresAt) / -86400) : 0,
      ExpireAtValue: Number(record.ExpiresAt || 0) > 0 ? dayjs(Number(record.ExpiresAt) * 1000) : null,
      AllowedDevicesText: record.AllowedDeviceIDs || record.AllowedDevices || (record.Binding?.DeviceID ? [record.Binding.DeviceID] : []),
      AllowedIPsText: (record.AllowedIPs || address?.IPv4CIDRs || []).join('\n'),
      AllowedIPv6Text: (address?.IPv6CIDRs || []).join('\n'),
      AllowedMACsText: (record.AllowedMACs || address?.MACs || []).join('\n'),
    });
    setEditing(record);
    setOpen(true);
  }

  async function submit() {
    await form.validateFields();
    const values = form.getFieldsValue(true) as UserDraft;
    const id = values.ID || editing?.ID || nextId('user', new Set(users.map((item) => item.ID)));
    const credential = primaryCredential(values);
    const allowedIPs = splitList(values.AllowedIPsText || '');
    const allowedIPv6 = splitList(values.AllowedIPv6Text || '');
    const allowedMACs = splitList(values.AllowedMACsText || '');
    const allowedDevices = values.AllowedDevicesText || [];
    const listenerIds = values.ListenerIDs || [];
    const actualProtocols = userProtocols({ ...values, ID: id, ListenerIDs: listenerIds }, listeners, config.XrayProfiles || []);
    const shadowsocks = shadowsocksRequirements(listenerIds, listeners, config.XrayProfiles || [], targetNodeID);
    const credentialError = validateUserCredentialSet(values, actualProtocols, shadowsocks, t);
    if (credentialError) {
      messageApi.error(credentialError);
      return;
    }
    const expiresAt = values.DelayedStart
      ? -Math.max(0, Math.trunc(values.ExpireDays || 0)) * 86400
      : values.ExpireAtValue?.unix() || 0;
    const addressResult = mergeUserAddress(
      config.Addresses || [],
      id,
      editing?.AddressID || editing?.Binding?.AddressID,
      allowedIPs,
      allowedIPv6,
      allowedMACs,
      allowedDevices.length === 1 ? allowedDevices[0] : undefined,
      targetNodeID,
    );
    const now = Math.floor(Date.now() / 1000);
    const next: UserRecord = {
      ...editing,
      ...values,
      ID: id,
      Name: values.Name || '',
      Email: values.Email || `${id}@tapx.local`,
      ListenerID: listenerIds[0] || '',
      ListenerIDs: listenerIds,
      CredentialValue: credential,
      UUID: values.UUID || '',
      Password: values.Password || '',
      Auth: values.Auth || '',
      VKey: values.VKey || '',
      AllowedDeviceIDs: allowedDevices,
      ExpiresAt: expiresAt,
      TrafficCap: gbToBytes(values.TrafficCap),
      UploadRateLimit: mbpsToBps(values.UploadRateMbps),
      DownloadRateLimit: mbpsToBps(values.DownloadRateMbps),
      Binding: {
        ...editing?.Binding,
        ...values.Binding,
        VKeyID: values.Binding?.VKeyID,
        AddressID: addressResult.addressId || undefined,
        DeviceID: allowedDevices.length === 1 ? allowedDevices[0] : undefined,
      },
      AddressID: addressResult.addressId || '',
      AllowedDevices: allowedDevices,
      AllowedIPs: allowedIPs,
      AllowedMACs: allowedMACs,
      CreatedAt: editing?.CreatedAt || now,
      UpdatedAt: now,
    };
    delete next.Security;
    delete next.ReverseTag;
    delete next.Flow;
    delete next.WireguardPrivateKey;
    delete next.WireguardPublicKey;
    delete next.WireguardPreSharedKey;
    delete next.WireguardAllowedIPs;
    const materializedVKey = materializeClientVKey(next, config.VKeys || [], {
      listeners: config.Listeners || [],
      connectors: config.Connectors || [],
      clients: users,
      routes: config.Routes || [],
    });
    const savedUser = materializedVKey.client as UserRecord;
    const savedKey = nodeObjectKey(savedUser);
    const index = users.findIndex((item) => nodeObjectKey(item) === savedKey);
    const nextUsers = index < 0 ? [...users, savedUser] : users.map((item) => (nodeObjectKey(item) === savedKey ? savedUser : item));
    await commitConfig({
      ...config,
      Clients: nextUsers,
      Addresses: addressResult.addresses,
      VKeys: materializedVKey.vkeys,
    }, t('user.saved'));
    setOpen(false);
  }

  function updateUserListeners(listenerIDs: string[]) {
    form.setFieldValue('ListenerIDs', listenerIDs);
    const requirement = shadowsocksRequirements(listenerIDs, listeners, config.XrayProfiles || [], targetNodeID);
    if (requirement.keyBytes.length > 1) {
      messageApi.warning(t('user.ssPasswordConflict'));
      return;
    }
    const keyBytes = requirement.keyBytes[0];
    const current = String(form.getFieldValue('Password') || '');
    if (keyBytes && !isShadowsocks2022Password(current, keyBytes)) {
      form.setFieldValue('Password', randomShadowsocksPassword(keyBytes));
    }
  }

  function regenerateUserPassword() {
    const listenerIDs = form.getFieldValue('ListenerIDs') || [];
    const requirement = shadowsocksRequirements(listenerIDs, listeners, config.XrayProfiles || [], targetNodeID);
    if (requirement.keyBytes.length > 1) {
      messageApi.error(t('user.ssPasswordGenerateConflict'));
      return;
    }
    form.setFieldValue('Password', randomShadowsocksPassword(requirement.keyBytes[0] || 32));
  }

  async function commitConfig(nextConfig: RuntimeConfig, successMessage?: string) {
    setSaving(true);
    try {
      const saved = await saveRuntimeConfig(nextConfig);
      let applyError = '';
      try {
        await applyRuntimeConfig();
      } catch (error) {
        applyError = error instanceof Error ? error.message : t('user.runtimeReloadFailed');
      }
      const report = await getStats();
      setConfig(hydrateUserStats(hydrateUserConfig(saved), report.clients || []));
      if (applyError) messageApi.warning(t('user.savedReloadFailed', { message: successMessage || t('user.dataSaved'), error: applyError }));
      else if (successMessage) messageApi.success(successMessage);
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('user.saveFailed'));
    } finally {
      setSaving(false);
    }
  }

  function toggleEnable(record: UserRecord, enabled: boolean) {
    const key = nodeObjectKey(record);
    const nextUsers = users.map((item) => (nodeObjectKey(item) === key ? { ...item, Enabled: enabled, UpdatedAt: nowSecond() } : item));
    void commitConfig({ ...config, Clients: nextUsers }, enabled ? t('user.enabled') : t('user.disabled'));
  }

  async function resetTraffic(records: UserRecord[]) {
    if (records.length === 0) return;
    setSaving(true);
    try {
      const resetByKey = new Map<string, TapxClient>();
      for (const record of records) {
        const resetConfig = await resetClientTraffic(record.ID, nodeIDOf(record));
        const reset = (resetConfig.Clients || []).find((item) => item.ID === record.ID);
        if (reset) resetByKey.set(nodeObjectKey(record), reset);
      }
      const report = await getStats();
      const nextConfig: RuntimeConfig = {
        ...config,
        Clients: users.map((item) => {
          const reset = resetByKey.get(nodeObjectKey(item));
          return reset ? {
            ...item,
            TrafficResetAt: reset.TrafficResetAt,
            TrafficResetGeneration: reset.TrafficResetGeneration,
            TrafficRXOffset: reset.TrafficRXOffset,
            TrafficTXOffset: reset.TrafficTXOffset,
          } : item;
        }),
      };
      setConfig(hydrateUserStats(hydrateUserConfig(nextConfig), report.clients || []));
      setSelectedRowKeys([]);
      messageApi.success(t('user.trafficResetCount', { count: records.length }));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('user.trafficResetFailed'));
    } finally {
      setSaving(false);
    }
  }

  function deleteUsers(records: UserRecord[]) {
    if (records.length === 0) return;
    const target = new Set(records.map(nodeObjectKey));
    const isTargetReference = (value: object, id: string | undefined) => Boolean(id && target.has(`${nodeIDOf(value)}:${id}`));
    const nextUsers = users.filter((item) => !target.has(nodeObjectKey(item)));
    const nextAddresses = (config.Addresses || []).filter((item) => !isTargetReference(item, item.ClientID));
    const keptAddressIDs = new Set(nextAddresses.map((item) => `${nodeIDOf(item)}:${item.ID}`));
    const nextRoutes = (config.Routes || []).filter((item) => !isTargetReference(item, item.ClientID)).map((item) => (
      item.AddressID && !keptAddressIDs.has(`${nodeIDOf(item)}:${item.AddressID}`) ? { ...item, AddressID: '' } : item
    ));
    const nextListeners = (config.Listeners || []).map((item) => isTargetReference(item, item.Binding?.ClientID)
      ? { ...item, Binding: { ...item.Binding, ClientID: '' } }
      : item);
    const nextConnectors = (config.Connectors || []).map((item) => isTargetReference(item, item.Binding?.ClientID)
      ? { ...item, Binding: { ...item.Binding, ClientID: '' } }
      : item);
    const candidateVKeys = new Set(records
      .filter((item) => item.Binding?.VKeyID?.startsWith(`vkey-${item.ID}`))
      .map((item) => `${nodeIDOf(item)}:${item.Binding?.VKeyID}`)
      .filter((id): id is string => Boolean(id)));
    const usedVKeys = new Set<string>();
    for (const item of [
      ...nextUsers,
      ...nextListeners,
      ...nextConnectors,
      ...nextRoutes,
    ]) {
      const reference = item as { VKeyID?: string; Binding?: { VKeyID?: string } };
      const id = reference.VKeyID || reference.Binding?.VKeyID;
      if (id) usedVKeys.add(`${nodeIDOf(item)}:${id}`);
    }
    const nextVKeys = (config.VKeys || []).filter((item) => {
      const key = `${nodeIDOf(item)}:${item.ID}`;
      return !candidateVKeys.has(key) || usedVKeys.has(key);
    });
    void commitConfig({
      ...config,
      Clients: nextUsers,
      Addresses: nextAddresses,
      VKeys: nextVKeys,
      Routes: nextRoutes,
      Listeners: nextListeners,
      Connectors: nextConnectors,
    }, t('user.deleted'));
    setSelectedRowKeys([]);
  }

  function bulkSetEnable(enabled: boolean) {
    const target = new Set(selectedRowKeys);
    const nextUsers = users.map((item) => target.has(nodeObjectKey(item)) ? { ...item, Enabled: enabled, UpdatedAt: nowSecond() } : item);
    void commitConfig({ ...config, Clients: nextUsers }, enabled ? t('user.bulkEnabled') : t('user.bulkDisabled'));
    setSelectedRowKeys([]);
  }

  async function changeSelectedListeners(mode: 'attach' | 'detach', listenerIDs: string[]) {
    const selected = new Set(selectedRowKeys);
    const nextUsers = users.map((item) => {
      if (!selected.has(nodeObjectKey(item))) return item;
      const current = listenerIdsFor(item);
      const next = changeListenerIDs(current, listenerIDs, mode);
      return { ...item, ListenerID: next[0] || '', ListenerIDs: next, UpdatedAt: nowSecond() };
    });
    await commitConfig({ ...config, Clients: nextUsers }, mode === 'attach' ? t('user.listenersAttached') : t('user.listenersDetached'));
    setBindingMode(null);
    setSelectedRowKeys([]);
  }

  async function adjustSelected(input: BulkUserAdjustment) {
    const selected = new Set(selectedRowKeys);
    const now = nowSecond();
    const nextUsers = users.map((item) => {
      if (!selected.has(nodeObjectKey(item))) return item;
      return applyBulkUserAdjustment(item, input, now);
    });
    await commitConfig({ ...config, Clients: nextUsers }, t('user.adjustedCount', { count: selectedRowKeys.length }));
    setAdjustOpen(false);
    setSelectedRowKeys([]);
  }

  async function createUsersInBulk(input: BulkCreateInput) {
    const ownerID = defaultTargetNodeID(scope);
    const usersOnTarget = users.filter((item) => nodeIDOf(item) === ownerID);
    const usedIDs = new Set(usersOnTarget.map((item) => item.ID));
    const usedEmails = new Set(usersOnTarget.map((item) => String(item.Email || '').trim().toLowerCase()).filter(Boolean));
    let nextUsers = [...users];
    let nextAddresses = [...(config.Addresses || [])];
    let nextVKeys = [...(config.VKeys || [])];
    const now = nowSecond();
    let createdCount = 0;
    for (const email of input.emails) {
      const normalizedEmail = email.trim();
      if (!normalizedEmail || usedEmails.has(normalizedEmail.toLowerCase())) continue;
      usedEmails.add(normalizedEmail.toLowerCase());
      const id = nextId('user', usedIDs);
      usedIDs.add(id);
      const uuid = randomUUID();
      const addressResult = mergeUserAddress(
        nextAddresses,
        id,
        undefined,
        input.allowedIPs,
        [],
        [],
        input.allowedDevices.length === 1 ? input.allowedDevices[0] : undefined,
        ownerID,
      );
      nextAddresses = addressResult.addresses;
      const draft: UserRecord = {
        ManagedNodeID: ownerID,
        ID: id,
        Enabled: true,
        Name: input.remark,
        Email: normalizedEmail,
        ListenerID: input.listenerIds[0] || '',
        ListenerIDs: input.listenerIds,
        CredentialType: 'vless',
        CredentialValue: uuid,
        UUID: uuid,
        Password: randomLowerAndNumber(16),
        Auth: randomLowerAndNumber(16),
        VKey: input.vKey,
        AllowedDeviceIDs: input.allowedDevices,
        AllowedDevices: input.allowedDevices,
        AllowedIPs: input.allowedIPs,
        AddressID: addressResult.addressId,
        Binding: { AddressID: addressResult.addressId || undefined },
        ExpiresAt: input.expiresAt,
        TrafficCap: gbToBytes(input.trafficGB),
        UploadRateLimit: mbpsToBps(input.uploadRateMbps),
        DownloadRateLimit: mbpsToBps(input.downloadRateMbps),
        TrafficReset: input.trafficReset,
        Remark: input.remark,
        CreatedAt: now,
        UpdatedAt: now,
      };
      const materialized = materializeClientVKey(draft, nextVKeys, {
        listeners: config.Listeners || [],
        connectors: config.Connectors || [],
        clients: nextUsers,
        routes: config.Routes || [],
      });
      nextVKeys = materialized.vkeys;
      nextUsers.push(materialized.client as UserRecord);
      createdCount += 1;
    }
    if (createdCount === 0) {
      messageApi.warning(t('user.noneCreated'));
      return;
    }
    await commitConfig({ ...config, Clients: nextUsers, Addresses: nextAddresses, VKeys: nextVKeys }, t('user.createdCount', { count: createdCount }));
    setBulkCreateOpen(false);
  }

  function confirmDeleteByState(kind: 'depleted' | 'orphan') {
    const records = kind === 'depleted'
      ? scopedUsers.filter((item) => item.TrafficCap && remainingBytes(item) === 0)
      : scopedUsers.filter((item) => listenerIdsFor(item).length === 0);
    if (records.length === 0) {
      messageApi.info(kind === 'depleted' ? t('user.noDepleted') : t('user.noOrphans'));
      return;
    }
    Modal.confirm({
      title: kind === 'depleted' ? t('user.deleteDepletedCount', { count: records.length }) : t('user.deleteOrphansCount', { count: records.length }),
      content: t('user.deleteCleanupHelp'),
      okText: t('common.delete'),
      okType: 'danger',
      cancelText: t('user.cancel'),
      onOk: () => deleteUsers(records),
    });
  }

  function exportUsers(records: UserRecord[]) {
    const exporting = records.length > 0 ? records : scopedUsers;
    const value = JSON.stringify(exportUserBundle(exporting, config), null, 2);
    setExportModal({
      open: true,
      title: records.length > 0 ? t('user.exportCount', { count: records.length }) : t('user.export'),
      value,
    });
  }

  async function submitImport() {
    let imported: ReturnType<typeof importUserBundle>;
    try {
      imported = importUserBundle(importText, config, importTargetNodeID, t);
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('user.invalidJson'));
      return;
    }
    const created = imported.clients.length - users.length;
    if (created === 0) {
      messageApi.warning(t('user.noneToImport'));
      return;
    }
    await commitConfig({
      ...config,
      Clients: imported.clients,
      Addresses: imported.addresses,
      VKeys: imported.vkeys,
    }, imported.skipped ? t('user.importedWithSkipped', { created, skipped: imported.skipped }) : t('user.importedCount', { count: created }));
    setImportOpen(false);
    setImportText('');
  }

  async function copyExportValue() {
    try {
      await copyText(exportModal.value);
      messageApi.success(t('user.copied'));
    } catch {
      messageApi.error(t('user.copyFailed'));
    }
  }

  const selectedUsers = useMemo(
    () => users.filter((item) => selectedRowKeys.includes(nodeObjectKey(item))),
    [selectedRowKeys, users],
  );
  const selectedUserNodeID = useMemo(() => {
    const nodeIDs = new Set(selectedUsers.map(nodeIDOf));
    return nodeIDs.size === 1 ? [...nodeIDs][0] : '';
  }, [selectedUsers]);

  const moreItems: MenuProps['items'] = selectedRowKeys.length > 0
    ? [
      { key: 'attach-selected', label: t('user.attachListeners'), icon: <UsergroupAddOutlined />, disabled: !selectedUserNodeID },
      { key: 'detach-selected', label: t('user.detachListeners'), icon: <UsergroupDeleteOutlined />, danger: true, disabled: !selectedUserNodeID },
      { type: 'divider' },
      { key: 'enable-selected', label: t('user.enableSelected') },
      { key: 'disable-selected', label: t('user.disableSelected'), danger: true },
      { key: 'adjust-selected', label: t('user.bulkAdjust'), icon: <ClockCircleOutlined /> },
      { type: 'divider' },
      { key: 'export-selected', label: t('user.exportSelected'), icon: <ExportOutlined /> },
      { key: 'reset-selected', label: t('user.resetSelectedTraffic'), icon: <RetweetOutlined /> },
    ]
    : [
      { key: 'bulk-create', icon: <UsergroupAddOutlined />, label: t('user.bulk.createTitle') },
      { key: 'import', icon: <ImportOutlined />, label: t('user.import') },
      { key: 'export', icon: <ExportOutlined />, label: t('user.export'), disabled: scopedUsers.length === 0 },
      { key: 'reset-all', icon: <RetweetOutlined />, label: t('user.resetAllTraffic'), disabled: scopedUsers.length === 0 },
      { type: 'divider' },
      { key: 'delete-depleted', icon: <RestOutlined />, label: t('user.deleteDepleted'), danger: true, disabled: scopedUsers.length === 0 },
      { key: 'delete-orphans', icon: <DisconnectOutlined />, label: t('user.deleteOrphans'), danger: true, disabled: scopedUsers.length === 0 },
    ];

  const onMoreClick: MenuProps['onClick'] = ({ key }) => {
    switch (key) {
      case 'import':
        setImportTargetNodeID(defaultTargetNodeID(scope));
        setImportOpen(true);
        break;
      case 'bulk-create':
        setBulkCreateOpen(true);
        break;
      case 'export':
        exportUsers(scopedUsers);
        break;
      case 'reset-all':
        void resetTraffic(scopedUsers);
        break;
      case 'attach-selected':
        if (!selectedUserNodeID) {
          messageApi.warning(t('user.sameNodeRequired'));
          break;
        }
        setBindingMode('attach');
        break;
      case 'detach-selected':
        if (!selectedUserNodeID) {
          messageApi.warning(t('user.sameNodeRequired'));
          break;
        }
        setBindingMode('detach');
        break;
      case 'adjust-selected':
        setAdjustOpen(true);
        break;
      case 'enable-selected':
        bulkSetEnable(true);
        break;
      case 'disable-selected':
        bulkSetEnable(false);
        break;
      case 'export-selected':
        exportUsers(selectedUsers);
        break;
      case 'reset-selected':
        void resetTraffic(selectedUsers);
        break;
      case 'delete-depleted':
        confirmDeleteByState('depleted');
        break;
      case 'delete-orphans':
        confirmDeleteByState('orphan');
        break;
    }
  };

  function confirmDeleteSelected() {
    Modal.confirm({
      title: t('user.deleteCount', { count: selectedUsers.length }),
      content: t('user.deleteAddressHelp'),
      okText: t('common.delete'),
      okType: 'danger',
      cancelText: t('user.cancel'),
      onOk: () => deleteUsers(selectedUsers),
    });
  }

  function confirmDeleteUser(record: UserRecord) {
    Modal.confirm({
      title: t('user.deleteNamed', { name: record.Email || record.ID }),
      content: t('user.deleteOneHelp'),
      okText: t('common.delete'),
      okType: 'danger',
      cancelText: t('user.cancel'),
      onOk: () => deleteUsers([record]),
    });
  }

  const columns = useMemo<TableColumnsType<UserRecord>>(() => [
    { title: t('node.sourceNode'), key: 'ManagedNodeID', width: 150, render: (_, record) => <NodeSourceTag value={record} /> },
    {
      title: t('user.actions'),
      key: 'actions',
      width: 170,
      render: (_, record) => (
        <Space size={2}>
          <Tooltip title={t('user.info')}>
            <Button size="small" type="text" icon={<InfoCircleOutlined />} aria-label={t('user.info')} onClick={() => setInfo(record)} />
          </Tooltip>
          <Tooltip title={t('user.resetTraffic')}>
            <Popconfirm
              title={t('user.resetTraffic')}
              description={t('user.resetTrafficConfirm')}
              okText={t('common.reset')}
              cancelText={t('user.cancel')}
              onConfirm={() => resetTraffic([record])}
            >
              <Button size="small" type="text" icon={<RetweetOutlined />} aria-label={t('user.resetTraffic')} />
            </Popconfirm>
          </Tooltip>
          <Tooltip title={t('user.edit')}>
            <Button size="small" type="text" icon={<EditOutlined />} aria-label={t('user.edit')} onClick={() => openEdit(record)} />
          </Tooltip>
          <Tooltip title={t('common.delete')}>
            <Popconfirm
              title={t('user.deleteNamed', { name: record.Email || record.ID })}
              description={t('user.deleteAddressHelp')}
              okText={t('common.delete')}
              cancelText={t('user.cancel')}
              okButtonProps={{ danger: true }}
              onConfirm={() => deleteUsers([record])}
            >
              <Button size="small" type="text" danger icon={<DeleteOutlined />} aria-label={t('common.delete')} />
            </Popconfirm>
          </Tooltip>
        </Space>
      ),
    },
    {
      title: t('common.enabled'),
      key: 'Enabled',
      align: 'center',
      width: 76,
      render: (_, record) => <Switch size="small" checked={record.Enabled !== false} onChange={(checked) => toggleEnable(record, checked)} />,
    },
    {
      title: t('user.online'),
      key: 'Online',
      align: 'center',
      width: 92,
      render: (_, record) => {
        const lastOnline = t('user.lastOnline', { value: lastOnlineLabel(record) });
        if (isExhausted(record)) return <Tooltip title={lastOnline}><Tag color="red">{t('user.exhausted')}</Tag></Tooltip>;
        if (record.Enabled === false) return <Tag>{t('common.disabled')}</Tag>;
        if (isOnline(record)) return <Tag color="green" className="dot-tag"><span className="online-dot" />{t('user.online')}</Tag>;
        if (isExpiring(record)) return <Tag color="orange">{t('user.expiring')}</Tag>;
        return <Tooltip title={lastOnline}><Tag>{t('user.offline')}</Tag></Tooltip>;
      },
    },
    {
      title: t('user.user'),
      key: 'User',
      width: 220,
      render: (_, record) => (
        <div className="user-email-cell">
          <span>{record.Email || record.Name || record.ID}</span>
          {record.Name || record.Remark ? <span className="user-subline">{record.Name || record.Remark}</span> : null}
        </div>
      ),
    },
    {
      title: t('user.listeners'),
      key: 'ListenerIDs',
      width: 190,
      render: (_, record) => {
        const ids = listenerIdsFor(record);
        if (ids.length === 0) return <span className="criterion-empty">-</span>;
        const visible = ids.slice(0, 1);
        const overflow = ids.length - visible.length;
        return (
          <Space size={4} wrap>
            {visible.map((id) => {
              const listener = listeners.find((item) => item.ID === id && nodeIDOf(item) === nodeIDOf(record));
              return listener ? <Tag key={id} color={listenerTagColor(listener)}>{labelEndpoint(listener)}</Tag> : <Tag key={id}>{id}</Tag>;
            })}
            {overflow > 0 ? <Tag>+{overflow}</Tag> : null}
          </Space>
        );
      },
    },
    {
      title: t('user.traffic'),
      key: 'Traffic',
      align: 'center',
      width: 280,
      render: (_, record) => <UserTrafficCell user={record} />,
    },
    {
      title: t('user.remaining'),
      key: 'Remain',
      align: 'center',
      width: 120,
      render: (_, record) => {
        const remaining = remainingBytes(record);
        return remaining == null ? <Tag color="purple">{t('user.unlimited')}</Tag> : <Tag color={remaining > 0 ? 'green' : 'red'}>{formatBytes(remaining)}</Tag>;
      },
    },
    {
      title: t('user.expiry'),
      key: 'Duration',
      align: 'center',
      width: 140,
      render: (_, record) => <Tag color={expiryColor(record)}>{expiryLabel(record, t)}</Tag>,
    },
  ], [listeners, t, users]);

  return (
    <div className="user-page">
      {messageContextHolder}
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Card size="small" hoverable className="user-summary-card">
          <div className="user-stat-grid">
            <UserStat title={t('user.user')} value={stats.total} tone="blue" primary />
            <UserStat title={t('user.online')} value={stats.online.length} tone="blue" users={stats.online} />
            <UserStat title={t('user.exhausted')} value={stats.exhausted.length} tone="red" users={stats.exhausted} />
            <UserStat title={t('user.expiring')} value={stats.nearlyExhausted.length} tone="orange" users={stats.nearlyExhausted} />
            <UserStat title={t('user.closed')} value={stats.disabled.length} tone="gray" users={stats.disabled} />
            <UserStat title={t('user.enabledUsers')} value={stats.enabled} tone="green" />
          </div>
        </Card>
        <Card
          size="small"
          hoverable
          title={<Space wrap className="user-toolbar">
            {selectedRowKeys.length === 0 ? (
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                {t('user.add')}
              </Button>
            ) : (
              <Tag
                color="blue"
                closable
                onClose={() => setSelectedRowKeys([])}
                style={{ padding: '4px 8px', fontSize: 13 }}
              >
                {t('user.selectedCount', { count: selectedRowKeys.length })}
              </Tag>
            )}
            <Dropdown trigger={['click']} menu={{ items: moreItems, onClick: onMoreClick }}>
              <Button icon={<MoreOutlined />}>{t('user.more')}</Button>
            </Dropdown>
            <NodeScopeSelect scope={scope} onChange={setScope} />
            {selectedRowKeys.length > 0 ? (
              <Button danger icon={<DeleteOutlined />} onClick={confirmDeleteSelected}>{t('common.delete')}</Button>
            ) : null}
            <Button icon={<FilterOutlined />} onClick={() => setFilterOpen(true)}>
              {t('user.filter')}{activeFilterCount > 0 ? ` (${activeFilterCount})` : ''}
            </Button>
            <Select
              className="user-sort-select"
              value={sort}
              options={sortOptions}
              onChange={setSort}
            />
            <Input.Search className="user-search" placeholder={t('user.searchPlaceholder')} allowClear value={search} onChange={(event) => setSearch(event.target.value)} />
          </Space>}
        >
          {activeFilterCount > 0 ? (
            <Space wrap className="user-filter-tags">
              <Button type="link" danger size="small" onClick={() => setFilters(emptyFilters)}>{t('user.clearFilters')}</Button>
              {filters.statuses.map((value) => (
                <Tag key={`status-${value}`} closable color="blue" onClose={() => setFilters({ ...filters, statuses: filters.statuses.filter((item) => item !== value) })}>
                  {statusFilterLabel(value, t)}
                </Tag>
              ))}
              {filters.protocols.map((value) => (
                <Tag key={`protocol-${value}`} closable color="cyan" onClose={() => setFilters({ ...filters, protocols: filters.protocols.filter((item) => item !== value) })}>
                  {credentialLabel(value)}
                </Tag>
              ))}
              {filters.listenerIds.map((value) => (
                <Tag key={`listener-${value}`} closable color="geekblue" onClose={() => setFilters({ ...filters, listenerIds: filters.listenerIds.filter((item) => item !== value) })}>
                  {labelEndpoint(scopedListeners.find((item) => item.ID === value) || { ID: value })}
                </Tag>
              ))}
              {filters.usageFromGB != null || filters.usageToGB != null ? (
                <Tag closable color="orange" onClose={() => setFilters({ ...filters, usageFromGB: undefined, usageToGB: undefined })}>
                  {t('user.traffic')} {filters.usageFromGB ?? 0} - {filters.usageToGB ?? '∞'} GB
                </Tag>
              ) : null}
              {filters.hasVKey ? <Tag closable onClose={() => setFilters({ ...filters, hasVKey: '' })}>vKey: {filters.hasVKey === 'yes' ? t('user.set') : t('user.unset')}</Tag> : null}
              {filters.hasRemark ? <Tag closable onClose={() => setFilters({ ...filters, hasRemark: '' })}>{t('user.remark')}: {filters.hasRemark === 'yes' ? t('user.yes') : t('user.no')}</Tag> : null}
            </Space>
          ) : null}
          {!isMobile ? (
            <Table
              rowKey={nodeObjectKey}
              rowSelection={{
                selectedRowKeys,
                onChange: (keys) => setSelectedRowKeys(keys.map(String)),
              }}
              columns={columns}
              dataSource={filteredUsers}
              loading={loading || saving}
              pagination={{
                current: currentPage,
                pageSize,
                showSizeChanger: true,
                pageSizeOptions: ['10', '25', '50', '100', '200'],
                hideOnSinglePage: filteredUsers.length <= pageSize,
                showTotal: (total) => `${total}`,
                onChange: (page, size) => { setCurrentPage(page); setPageSize(size); },
              }}
              scroll={{ x: 1350 }}
              size="small"
              locale={{ emptyText: t('user.empty') }}
            />
          ) : (
            <Spin spinning={loading || saving}>
              <div className="user-cards">
                {filteredUsers.length > 0 ? (
                  <div className="user-card-bulk">
                    <Checkbox
                      checked={selectedRowKeys.length === filteredUsers.length}
                      indeterminate={selectedRowKeys.length > 0 && selectedRowKeys.length < filteredUsers.length}
                      onChange={(event) => setSelectedRowKeys(event.target.checked ? filteredUsers.map(nodeObjectKey) : [])}
                    >{t('user.selectAll')}</Checkbox>
                    {selectedRowKeys.length > 0 ? <span>{selectedRowKeys.length}</span> : null}
                  </div>
                ) : null}
                {filteredUsers.length === 0 ? <div className="user-card-empty"><TeamOutlined /><div>{t('user.empty')}</div></div> : null}
                {pagedUsers.map((record) => (
                  <div key={nodeObjectKey(record)} className={`user-mobile-card${selectedRowKeys.includes(nodeObjectKey(record)) ? ' is-selected' : ''}`}>
                    <div className="user-mobile-head">
                      <Checkbox
                        checked={selectedRowKeys.includes(nodeObjectKey(record))}
                        onChange={(event) => setSelectedRowKeys((current) => event.target.checked ? [...new Set([...current, nodeObjectKey(record)])] : current.filter((id) => id !== nodeObjectKey(record)))}
                      />
                      {isOnline(record) ? <span className="online-dot" /> : <Badge status={isExhausted(record) ? 'error' : record.Enabled === false ? 'default' : 'warning'} />}
                      <span className="user-mobile-name">{record.Email || record.Name || record.ID}</span>
                      <NodeSourceTag value={record} />
                      {isExhausted(record) ? <Tag color="red">{t('user.exhausted')}</Tag> : isExpiring(record) ? <Tag color="orange">{t('user.expiring')}</Tag> : null}
                      <div className="user-mobile-actions">
                        <Button type="text" size="small" icon={<InfoCircleOutlined />} aria-label={t('user.info')} onClick={() => setInfo(record)} />
                        <Switch size="small" checked={record.Enabled !== false} onChange={(checked) => toggleEnable(record, checked)} />
                        <Dropdown
                          trigger={['click']}
                          placement="bottomRight"
                          menu={{ items: [
                            { key: 'reset', label: <><RetweetOutlined /> {t('user.resetTraffic')}</>, onClick: () => void resetTraffic([record]) },
                            { key: 'edit', label: <><EditOutlined /> {t('user.edit')}</>, onClick: () => openEdit(record) },
                            { key: 'delete', danger: true, label: <><DeleteOutlined /> {t('common.delete')}</>, onClick: () => confirmDeleteUser(record) },
                          ] }}
                        >
                          <Button type="text" size="small" icon={<MoreOutlined />} aria-label={t('user.more')} />
                        </Dropdown>
                      </div>
                    </div>
                    <UserTrafficCell user={record} />
                  </div>
                ))}
                {filteredUsers.length > 0 ? (
                  <Pagination
                    current={currentPage}
                    pageSize={pageSize}
                    total={filteredUsers.length}
                    showSizeChanger={filteredUsers.length > 10}
                    pageSizeOptions={['10', '25', '50', '100', '200']}
                    hideOnSinglePage={filteredUsers.length <= pageSize}
                    size="small"
                    showTotal={(total) => `${total}`}
                    onChange={(page, size) => { setCurrentPage(page); setPageSize(size); }}
                  />
                ) : null}
              </div>
            </Spin>
          )}
        </Card>
      </Space>

      <Modal
        open={open}
        title={editing ? t('user.editTitle') : t('user.addTitle')}
        okText={editing ? t('user.saveChanges') : t('common.create')}
        cancelText={t('user.cancel')}
        width={760}
        forceRender
        mask={{ closable: false }}
        confirmLoading={saving}
        onOk={submit}
        onCancel={() => setOpen(false)}
        styles={{ body: { maxHeight: 'calc(100vh - 160px)', overflowY: 'auto', overflowX: 'hidden' } }}
      >
        <Form form={form} colon={false} labelCol={{ sm: { span: 8 } }} wrapperCol={{ sm: { span: 14 } }} labelWrap>
          <Tabs
            items={[
              {
                key: 'basic',
                label: t('user.basic'),
                children: (
                  <>
                    <Form.Item name="ID" hidden><Input /></Form.Item>
                    <Form.Item name="ManagedNodeID" label={t('node.targetNode')} tooltip={t('node.targetNodeHelp')} rules={[{ required: true }]}>
                      <Select options={nodeTargetOptions} disabled={Boolean(editing)} />
                    </Form.Item>
                    <Form.Item label={t('user.email')} htmlFor="Email" tooltip={t('user.emailHelp')} required>
                      <Space.Compact style={{ display: 'flex' }}>
                        <Form.Item name="Email" noStyle rules={[{ required: true, message: t('user.emailRequired') }]}>
                          <Input id="Email" placeholder="user@example.com" style={{ flex: 1 }} />
                        </Form.Item>
                        {!editing ? (
                          <Button aria-label={t('user.regenerateEmail')} icon={<ReloadOutlined />} onClick={() => form.setFieldValue('Email', `${randomLowerAndNumber(12)}@tapx.local`)} />
                        ) : null}
                      </Space.Compact>
                    </Form.Item>
                    <Form.Item name="Name" label={t('user.remark')}><Input placeholder={t('user.remarkExample')} /></Form.Item>
                    <Form.Item name="ListenerIDs" label={t('user.listeners')} tooltip={t('user.listenersHelp')}>
                      <Select
                        mode="multiple"
                        allowClear
                        showSearch
                        options={listenerOptions}
                        placeholder={t('user.allowedListenersPlaceholder')}
                        maxTagCount="responsive"
                        optionFilterProp="label"
                        listHeight={220}
                        onChange={updateUserListeners}
                      />
                    </Form.Item>
                    <Form.Item wrapperCol={{ sm: { offset: 8, span: 14 } }}>
                      <Space>
                        <Button onClick={() => form.setFieldValue('ListenerIDs', listenerOptions.map((item) => item.value))}>{t('user.selectAll')}</Button>
                        <Button onClick={() => form.setFieldValue('ListenerIDs', [])}>{t('user.clearAll')}</Button>
                      </Space>
                    </Form.Item>
                    <Form.Item name="Enabled" label={t('common.enabled')} valuePropName="checked"><Switch /></Form.Item>
                  </>
                ),
              },
              {
                key: 'limits',
                label: t('user.limits'),
                children: (
                  <>
                    <Form.Item name="TrafficCap" label={t('user.totalTraffic')} tooltip={t('user.trafficCapHelp')}>
                      <InputNumber min={0} placeholder="100" style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="UploadRateMbps" label={t('user.uploadRateLimit')} tooltip={t('user.rateLimitHelp')}>
                      <InputNumber min={0.001} step={1} precision={3} addonAfter="Mbps" placeholder="100" style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="DownloadRateMbps" label={t('user.downloadRateLimit')} tooltip={t('user.rateLimitHelp')}>
                      <InputNumber min={0.001} step={1} precision={3} addonAfter="Mbps" placeholder="100" style={{ width: '100%' }} />
                    </Form.Item>
                    <Form.Item name="TrafficReset" label={t('user.trafficReset')} tooltip={t('user.trafficResetHelp')}>
                      <Select options={[
                        { value: 'never', label: t('user.resetNever') },
                        { value: 'hourly', label: t('user.resetHourly') },
                        { value: 'daily', label: t('user.resetDaily') },
                        { value: 'weekly', label: t('user.resetWeekly') },
                        { value: 'monthly', label: t('user.resetMonthly') },
                      ]} />
                    </Form.Item>
                    <Form.Item name="DelayedStart" label={t('user.delayedStart')} tooltip={t('user.delayedStartHelp')} valuePropName="checked">
                      <Switch onChange={(checked) => {
                        if (checked) form.setFieldValue('ExpireAtValue', null);
                        else form.setFieldValue('ExpireDays', 0);
                      }} />
                    </Form.Item>
                    {delayedStart ? (
                      <Form.Item name="ExpireDays" label={t('user.validDays')} tooltip={t('user.validDaysHelp')}>
                        <InputNumber min={0} precision={0} placeholder="30" style={{ width: '100%' }} />
                      </Form.Item>
                    ) : (
                      <Form.Item name="ExpireAtValue" label={t('user.expiry')} tooltip={t('user.expiryHelp')}>
                        <DatePicker
                          showTime
                          format="YYYY-MM-DD HH:mm:ss"
                          placeholder="2026-12-31 23:59:59"
                          style={{ width: '100%' }}
                        />
                      </Form.Item>
                    )}
                    <Form.Item name="AllowedDevicesText" label={t('user.allowedTunTap')} tooltip={t('user.allowedTunTapHelp')}>
                      <Select mode="multiple" allowClear options={deviceOptions} placeholder={t('user.selectAllowedTunTap')} maxTagCount="responsive" />
                    </Form.Item>
                    <Form.Item name="AllowedIPsText" label={t('user.sourceIpv4Limit')} tooltip={t('user.allowedSourceIpHelp')}>
                      <Input.TextArea rows={3} placeholder={t('user.ipListPlaceholder')} />
                    </Form.Item>
                    <Form.Item name="AllowedIPv6Text" label={t('user.sourceIpv6Limit')} tooltip={t('user.allowedSourceIpHelp')}>
                      <Input.TextArea rows={3} placeholder={t('user.ipv6ListPlaceholder')} />
                    </Form.Item>
                    <Form.Item name="AllowedMACsText" label={t('user.sourceMacLimit')} tooltip={t('user.sourceMacHelp')}>
                      <Input.TextArea rows={3} placeholder="02:00:00:00:00:01" />
                    </Form.Item>
                  </>
                ),
              },
              {
                key: 'credential',
                label: t('user.credentials'),
                children: (
                  <>
                    <Form.Item label="UUID" htmlFor="UUID" tooltip={t('user.uuidHelp')}>
                      <Space.Compact style={{ display: 'flex' }}>
                        <Form.Item name="UUID" noStyle>
                          <Input id="UUID" placeholder="550e8400-e29b-41d4-a716-446655440000" style={{ flex: 1 }} />
                        </Form.Item>
                        <Button aria-label={t('user.regenerateUuid')} icon={<ReloadOutlined />} onClick={() => form.setFieldValue('UUID', randomUUID())} />
                      </Space.Compact>
                    </Form.Item>

                    <Form.Item label={t('user.password')} htmlFor="Password" tooltip={t('user.passwordHelp')}>
                      <Space.Compact style={{ display: 'flex' }}>
                        <Form.Item name="Password" noStyle>
                          <Input id="Password" placeholder="p@ssword-7f3a" style={{ flex: 1 }} />
                        </Form.Item>
                        <Button aria-label={t('user.regeneratePassword')} icon={<ReloadOutlined />} onClick={regenerateUserPassword} />
                      </Space.Compact>
                    </Form.Item>

                    <Form.Item label="vKey" htmlFor="VKey" tooltip={t('user.vkeyHelp')}>
                      <Space.Compact style={{ display: 'flex' }}>
                        <Form.Item name="VKey" noStyle>
                          <Input id="VKey" placeholder={t('user.vkeyPlaceholder')} style={{ flex: 1 }} />
                        </Form.Item>
                        <Button aria-label={t('user.regenerateVkey')} icon={<ReloadOutlined />} onClick={() => form.setFieldValue('VKey', randomLowerAndNumber(24))} />
                      </Space.Compact>
                    </Form.Item>

                    <Form.Item label={t('user.hysteriaAuth')} htmlFor="Auth" tooltip={t('user.hysteriaAuthHelp')}>
                      <Space.Compact style={{ display: 'flex' }}>
                        <Form.Item name="Auth" noStyle>
                          <Input id="Auth" placeholder="auth-7f3a91c2" style={{ flex: 1 }} />
                        </Form.Item>
                        <Button aria-label={t('user.regenerateAuth')} icon={<ReloadOutlined />} onClick={() => form.setFieldValue('Auth', randomLowerAndNumber(16))} />
                      </Space.Compact>
                    </Form.Item>
                  </>
                ),
              },
            ].sort((left, right) => userTabOrder[String(left.key)] - userTabOrder[String(right.key)])}
          />
        </Form>
      </Modal>

      <Modal
        open={!!info}
        title={info ? t('user.infoNamed', { name: info.Email || info.ID }) : t('user.info')}
        width={680}
        footer={<Button type="primary" onClick={() => setInfo(null)}>{t('common.close')}</Button>}
        onCancel={() => setInfo(null)}
      >
        {info ? (
          <>
            <table className="user-info-table">
              <tbody>
                <tr><td>{t('user.online')}</td><td>{isOnline(info) ? <Tag color="green">{t('user.online')}</Tag> : <Tag>{t('user.offline')}</Tag>} <span className="user-info-hint">{t('user.lastOnline', { value: lastOnlineLabel(info) })}</span></td></tr>
                <tr><td>{t('user.status')}</td><td>{info.Enabled === false ? <Tag>{t('common.disabled')}</Tag> : <Tag color="green">{t('common.enabled')}</Tag>}</td></tr>
                <tr><td>{t('user.email')}</td><td><Tag color="green">{info.Email || '-'}</Tag></td></tr>
                <tr><td>{t('user.protocol')}</td><td>{tagList(userProtocols(info, listeners, config.XrayProfiles || []).map(credentialLabel), 'cyan', t)}</td></tr>
                <tr><td>UUID</td><td>{credentialToken(info, 'uuid', t)}</td></tr>
                <tr><td>{t('user.passwordAuth')}</td><td>{credentialToken(info, 'password', t)}</td></tr>
                <tr><td>vKey</td><td>{copyableToken(info.VKey || '-', info.VKey || '', messageApi, t)}</td></tr>
                <tr><td>{t('user.hysteriaAuth')}</td><td>{credentialToken(info, 'hysteria', t)}</td></tr>
                <tr><td>{t('user.traffic')}</td><td><Tag>↑ {formatBytes(trafficShape(info).up)} / ↓ {formatBytes(trafficShape(info).down)}</Tag> <span className="user-info-hint">{formatBytes(trafficUsedBytes(info))} / {info.TrafficCap ? formatTrafficCap(info.TrafficCap) : '∞'}</span></td></tr>
                <tr><td>{t('user.bandwidthLimit')}</td><td><Tag>↑ {formatRateLimit(info.UploadRateLimit, t)}</Tag> <Tag>↓ {formatRateLimit(info.DownloadRateLimit, t)}</Tag></td></tr>
                <tr><td>{t('user.remaining')}</td><td>{remainingBytes(info) == null ? <Tag color="purple">{t('user.unlimited')}</Tag> : <Tag>{formatBytes(remainingBytes(info) || 0)}</Tag>}</td></tr>
                <tr><td>{t('user.expiry')}</td><td><Tag color={expiryColor(info)}>{expiryLabel(info, t)}</Tag></td></tr>
                <tr><td>{t('user.allowedTunTap')}</td><td>{tagList(info.AllowedDevices || [], 'blue', t)}</td></tr>
                <tr><td>{t('user.sourceIpv4Limit')}</td><td>{tagList(info.AllowedIPs || [], undefined, t)}</td></tr>
                <tr><td>{t('user.sourceMacLimit')}</td><td>{tagList(info.AllowedMACs || [], undefined, t)}</td></tr>
                <tr><td>{t('user.createdAt')}</td><td><Tag>{dateTimeLabel(info.CreatedAt)}</Tag></td></tr>
                <tr><td>{t('user.updatedAt')}</td><td><Tag>{dateTimeLabel(info.UpdatedAt)}</Tag></td></tr>
                {info.Name || info.Remark ? <tr><td>{t('user.remark')}</td><td><Tag className="info-large-tag">{info.Name || info.Remark}</Tag></td></tr> : null}
                <tr><td>{t('user.listeners')}</td><td>{listenerTags(info, listeners, t)}</td></tr>
              </tbody>
            </table>
            <Divider>{t('user.tapxImportLinks')}</Divider>
            <Space.Compact style={{ display: 'flex' }}>
              <Input.TextArea value={shareLoading ? t('user.generatingLinks') : shareLinks.join('\n')} readOnly autoSize={{ minRows: 2, maxRows: 5 }} />
              <Tooltip title={t('user.copyImportLinks')}>
                <Button
                  icon={<CopyOutlined />}
                  disabled={shareLoading || shareLinks.length === 0}
                  onClick={() => void copyShareLinks(shareLinks, messageApi, t)}
                />
              </Tooltip>
            </Space.Compact>
          </>
        ) : null}
      </Modal>

      <Drawer
        title={t('user.filterUsers')}
        open={filterOpen}
        size={420}
        destroyOnHidden
        onClose={() => setFilterOpen(false)}
        footer={
          <div className="user-filter-footer">
            <Button danger onClick={() => setFilters(emptyFilters)}>{t('user.clearFilters')}</Button>
            <Button type="primary" onClick={() => setFilterOpen(false)}>{t('user.done')}</Button>
          </div>
        }
      >
        <Form layout="vertical">
          <Form.Item label={t('user.status')}>
            <Checkbox.Group
              value={filters.statuses}
              onChange={(values) => setFilters({ ...filters, statuses: values.map(String) })}
            >
              <Space orientation="vertical">
                <Checkbox value="active">{t('user.enabledUsers')}</Checkbox>
                <Checkbox value="online">{t('user.online')}</Checkbox>
                <Checkbox value="expiring">{t('user.expiring')}</Checkbox>
                <Checkbox value="exhausted">{t('user.exhausted')}</Checkbox>
                <Checkbox value="disabled">{t('user.closed')}</Checkbox>
              </Space>
            </Checkbox.Group>
          </Form.Item>
          <Form.Item label={t('user.protocol')}>
            <Select
              mode="multiple"
              value={filters.protocols}
              options={protocolOptions}
              placeholder={t('user.protocol')}
              maxTagCount="responsive"
              allowClear
              onChange={(values) => setFilters({ ...filters, protocols: values })}
            />
          </Form.Item>
          <Form.Item label={t('user.listener')}>
            <Select
              mode="multiple"
              value={filters.listenerIds}
              options={listenerOptions}
              placeholder={t('user.listener')}
              maxTagCount="responsive"
              allowClear
              showSearch
              optionFilterProp="label"
              onChange={(values) => setFilters({ ...filters, listenerIds: values })}
            />
          </Form.Item>
          <Form.Item label={t('user.trafficGb')}>
            <Space.Compact style={{ display: 'flex' }}>
              <InputNumber
                value={filters.usageFromGB}
                min={0}
                placeholder={t('user.from')}
                style={{ flex: 1 }}
                onChange={(value) => setFilters({ ...filters, usageFromGB: typeof value === 'number' ? value : undefined })}
              />
              <InputNumber
                value={filters.usageToGB}
                min={0}
                placeholder={t('user.to')}
                style={{ flex: 1 }}
                onChange={(value) => setFilters({ ...filters, usageToGB: typeof value === 'number' ? value : undefined })}
              />
            </Space.Compact>
          </Form.Item>
          <Form.Item label="vKey">
            <Radio.Group
              value={filters.hasVKey}
              optionType="button"
              buttonStyle="solid"
              options={[
                { value: '', label: t('user.all') },
                { value: 'yes', label: t('user.set') },
                { value: 'no', label: t('user.unset') },
              ]}
              onChange={(event) => setFilters({ ...filters, hasVKey: event.target.value })}
            />
          </Form.Item>
          <Form.Item label={t('user.remark')}>
            <Radio.Group
              value={filters.hasRemark}
              optionType="button"
              buttonStyle="solid"
              options={[
                { value: '', label: t('user.all') },
                { value: 'yes', label: t('user.hasRemark') },
                { value: 'no', label: t('user.noRemark') },
              ]}
              onChange={(event) => setFilters({ ...filters, hasRemark: event.target.value })}
            />
          </Form.Item>
        </Form>
      </Drawer>

      <ListenerBindingModal
        mode={bindingMode || 'attach'}
        open={bindingMode !== null}
        count={selectedUsers.length}
        listeners={selectedUserNodeID
          ? filterNodeOwned(listeners as Array<TapxEndpoint & NodeOwned>, selectedUserNodeID)
          : []}
        saving={saving}
        onClose={() => setBindingMode(null)}
        onSubmit={(listenerIDs) => changeSelectedListeners(bindingMode || 'attach', listenerIDs)}
      />

      <BulkAdjustModal
        open={adjustOpen}
        count={selectedUsers.length}
        saving={saving}
        onClose={() => setAdjustOpen(false)}
        onSubmit={adjustSelected}
      />

      <BulkCreateModal
        open={bulkCreateOpen}
        listeners={filterNodeOwned(listeners as Array<TapxEndpoint & NodeOwned>, defaultTargetNodeID(scope))}
        devices={filterNodeOwned(devices as Array<TapxDevice & NodeOwned>, defaultTargetNodeID(scope))}
        saving={saving}
        onClose={() => setBulkCreateOpen(false)}
        onSubmit={createUsersInBulk}
      />

      <Modal
        open={exportModal.open}
        title={exportModal.title}
        width={720}
        okText={t('user.copy')}
        cancelText={t('common.close')}
        onOk={copyExportValue}
        onCancel={() => setExportModal({ open: false, title: '', value: '' })}
      >
        <Input.TextArea value={exportModal.value} readOnly autoSize={{ minRows: 12, maxRows: 22 }} />
      </Modal>

      <Modal
        open={importOpen}
        title={t('user.import')}
        okText={t('user.importAction')}
        cancelText={t('user.cancel')}
        confirmLoading={saving}
        onOk={submitImport}
        onCancel={() => setImportOpen(false)}
      >
        <Form.Item label={t('node.targetNode')} tooltip={t('node.targetNodeHelp')}>
          <Select value={importTargetNodeID} options={nodeTargetOptions} onChange={setImportTargetNodeID} />
        </Form.Item>
        <Form.Item label={t('user.importContent')} tooltip={t('user.importHelp')}>
          <Input.TextArea placeholder='[{"ID":"user-1"}]' value={importText} onChange={(event) => setImportText(event.target.value)} autoSize={{ minRows: 10, maxRows: 18 }} />
        </Form.Item>
      </Modal>
    </div>
  );
}

function UserStat({ title, value, tone, users = [], primary = false }: { title: string; value: number; tone: 'blue' | 'green' | 'orange' | 'red' | 'gray'; users?: UserRecord[]; primary?: boolean }) {
  const statistic = (
    <div className="user-stat-card">
      <span>{title}</span>
      <strong>
        {primary ? <TeamOutlined /> : <i className={`user-stat-dot user-stat-${tone}`} />}
        {value}
      </strong>
    </div>
  );
  if (users.length === 0) return statistic;
  return (
    <Popover title={title} content={<div className="user-stat-list">{users.map((item) => <div key={nodeObjectKey(item)}>{item.Email || item.Name || item.ID}</div>)}</div>}>
      {statistic}
    </Popover>
  );
}

function UserTrafficCell({ user }: { user: UserRecord }) {
  const { t } = useI18n();
  const traffic = trafficShape(user);
  const used = traffic.up + traffic.down;
  const cap = trafficCapBytes(user.TrafficCap);
  const percent = cap > 0 ? Math.min(100, Math.round((used / cap) * 100)) : 100;
  const popover = (
    <table className="user-traffic-popover"><tbody>
      <tr><td>↑</td><td>{formatBytes(traffic.up)}</td><td>↓</td><td>{formatBytes(traffic.down)}</td></tr>
      {cap > 0 ? <tr><td colSpan={2}>{t('user.remaining')}</td><td colSpan={2}>{formatBytes(Math.max(0, cap - used))}</td></tr> : null}
    </tbody></table>
  );
  return (
    <Popover content={popover} trigger={['hover', 'click']} placement="top">
      <div className="user-traffic-cell">
        <span>{formatBytes(used)}</span>
        <Progress percent={percent} showInfo={false} strokeColor={trafficColor(user) === 'red' ? 'var(--ant-color-error)' : trafficColor(user) === 'orange' ? 'var(--ant-color-warning)' : 'var(--ant-color-primary)'} status={isExhausted(user) ? 'exception' : 'normal'} size="small" />
        <span>{cap > 0 ? formatBytes(cap) : '∞'}</span>
      </div>
    </Popover>
  );
}

function listenerTagColor(listener: TapxEndpoint): string {
  if (listener.Transport === 'udp') return 'cyan';
  if (listener.Transport === 'tcp') return 'blue';
  return 'purple';
}

function statusFilterLabel(value: string, t: ReturnType<typeof useI18n>['t']): string {
  return ({ active: t('user.enabledUsers'), online: t('user.online'), expiring: t('user.expiring'), exhausted: t('user.exhausted'), disabled: t('user.closed') } as Record<string, string>)[value] || value;
}

function trafficShape(user: UserRecord): { up: number; down: number } {
  const traffic = user.Traffic || user.traffic || {};
  return {
    up: Number(traffic.up ?? traffic.Up ?? user.TrafficUp ?? 0),
    down: Number(traffic.down ?? traffic.Down ?? user.TrafficDown ?? 0),
  };
}

function lastOnlineLabel(user: UserRecord): string {
  const traffic = user.Traffic || user.traffic || {};
  return dateTimeLabel(Number(traffic.lastOnline ?? traffic.LastOnline ?? 0));
}

function dateTimeLabel(value?: number): string {
  if (!value || value <= 0) return '-';
  return new Date(value * 1000).toLocaleString();
}

function buildUserStats(users: UserRecord[]) {
  const now = Date.now() / 1000;
  return users.reduce((acc, user) => {
    acc.total += 1;
    if (user.Enabled === false) acc.disabled.push(user);
    else acc.enabled += 1;
    if (isOnline(user)) acc.online.push(user);
    if (isExhausted(user, now)) acc.exhausted.push(user);
    else if (isExpiring(user, now)) acc.nearlyExhausted.push(user);
    return acc;
  }, {
    total: 0,
    online: [] as UserRecord[],
    exhausted: [] as UserRecord[],
    nearlyExhausted: [] as UserRecord[],
    disabled: [] as UserRecord[],
    enabled: 0,
  });
}

function countFilters(filters: UserFilters): number {
  let count = 0;
  if (filters.statuses.length > 0) count += 1;
  if (filters.protocols.length > 0) count += 1;
  if (filters.listenerIds.length > 0) count += 1;
  if (typeof filters.usageFromGB === 'number' || typeof filters.usageToGB === 'number') count += 1;
  if (filters.hasVKey) count += 1;
  if (filters.hasRemark) count += 1;
  return count;
}

function listenerIdsFor(record: UserRecord): string[] {
  if (Array.isArray(record.ListenerIDs)) return record.ListenerIDs.filter(Boolean);
  return record.ListenerID ? [record.ListenerID] : [];
}

function isOnline(user: UserRecord): boolean {
  return Boolean(user.Online || user.online || user.IsOnline);
}

function statusMatches(user: UserRecord, status: string): boolean {
  switch (status) {
    case 'active':
      return user.Enabled !== false && !isExhausted(user);
    case 'online':
      return isOnline(user);
    case 'expiring':
      return isExpiring(user);
    case 'exhausted':
      return isExhausted(user);
    case 'disabled':
      return user.Enabled === false;
    default:
      return false;
  }
}

function isExhausted(user: UserRecord, now = Date.now() / 1000): boolean {
  if (user.ExpiresAt && user.ExpiresAt > 0 && user.ExpiresAt < now) return true;
  const cap = trafficCapBytes(user.TrafficCap);
  return cap > 0 && trafficUsedBytes(user) >= cap;
}

function isExpiring(user: UserRecord, now = Date.now() / 1000): boolean {
  if (user.ExpiresAt && user.ExpiresAt > 0 && user.ExpiresAt - now < 7 * 24 * 60 * 60) return true;
  const cap = trafficCapBytes(user.TrafficCap);
  return cap > 0 && trafficUsedBytes(user) / cap >= 0.85;
}

function trafficUsedBytes(user: UserRecord): number {
  const traffic = user.Traffic || user.traffic || {};
  const up = user.TrafficUp ?? traffic.up ?? traffic.Up ?? 0;
  const down = user.TrafficDown ?? traffic.down ?? traffic.Down ?? 0;
  return Math.max(0, Number(up) || 0) + Math.max(0, Number(down) || 0);
}

function trafficCapBytes(value?: number): number {
  if (!value || value <= 0) return 0;
  return value;
}

function remainingBytes(user: UserRecord): number | null {
  const cap = trafficCapBytes(user.TrafficCap);
  if (cap <= 0) return null;
  return Math.max(0, cap - trafficUsedBytes(user));
}

function trafficColor(user: UserRecord): string {
  const cap = trafficCapBytes(user.TrafficCap);
  if (cap <= 0) return 'purple';
  const ratio = trafficUsedBytes(user) / cap;
  if (ratio >= 1) return 'red';
  if (ratio >= 0.85) return 'orange';
  return 'green';
}

function formatTrafficCap(value: number): string {
  return formatBytes(trafficCapBytes(value));
}

function bytesToGB(value?: number): number {
  if (!value || value <= 0) return 0;
  return Math.round((value / (1024 * 1024 * 1024)) * 100) / 100;
}

function gbToBytes(value?: number): number {
  if (!value || value <= 0) return 0;
  return Math.round(value * 1024 * 1024 * 1024);
}

function bpsToMbps(value?: number): number | undefined {
  if (!value || value <= 0) return undefined;
  return Math.round((value / 1_000_000) * 1000) / 1000;
}

function mbpsToBps(value?: number | null): number {
  if (!value || value <= 0) return 0;
  return Math.round(value * 1_000_000);
}

function formatRateLimit(value: number | undefined, t: ReturnType<typeof useI18n>['t']): string {
  if (!value || value <= 0) return t('user.unlimited');
  const mbps = value / 1_000_000;
  return `${mbps >= 100 ? mbps.toFixed(0) : mbps.toFixed(3).replace(/\.?0+$/, '')} Mbps`;
}

function expiryLabel(user: UserRecord, t: ReturnType<typeof useI18n>['t']): string {
  if (!user.ExpiresAt) return t('user.unlimited');
  if (user.ExpiresAt < 0) return t('user.delayedDays', { days: Math.round(user.ExpiresAt / -86400) });
  return new Date(user.ExpiresAt * 1000).toLocaleString();
}

function expiryColor(user: UserRecord): string {
  if (!user.ExpiresAt) return 'purple';
  if (user.ExpiresAt < 0) return 'blue';
  if (user.ExpiresAt <= Date.now() / 1000) return 'red';
  if (user.ExpiresAt - Date.now() / 1000 < 3 * 24 * 60 * 60) return 'orange';
  return 'green';
}

function sortUsers(users: UserRecord[], sort: string): UserRecord[] {
  const next = [...users];
  next.sort((left, right) => {
    switch (sort) {
      case 'updated:desc':
        return (right.UpdatedAt || 0) - (left.UpdatedAt || 0);
      case 'created:desc':
        return (right.CreatedAt || 0) - (left.CreatedAt || 0);
      case 'online:desc': {
        const leftTraffic = left.Traffic || left.traffic || {};
        const rightTraffic = right.Traffic || right.traffic || {};
        return Number(rightTraffic.lastOnline ?? rightTraffic.LastOnline ?? 0) - Number(leftTraffic.lastOnline ?? leftTraffic.LastOnline ?? 0);
      }
      case 'email:asc':
        return (left.Email || left.ID).localeCompare(right.Email || right.ID);
      case 'email:desc':
        return (right.Email || right.ID).localeCompare(left.Email || left.ID);
      case 'traffic:desc':
        return trafficUsedBytes(right) - trafficUsedBytes(left);
      case 'remaining:desc': {
        const l = remainingBytes(left);
        const r = remainingBytes(right);
        return (r == null ? Number.MAX_SAFE_INTEGER : r) - (l == null ? Number.MAX_SAFE_INTEGER : l);
      }
      case 'expiry:asc':
        return (left.ExpiresAt || Number.MAX_SAFE_INTEGER) - (right.ExpiresAt || Number.MAX_SAFE_INTEGER);
      case 'created:asc':
      default:
        return (left.CreatedAt || 0) - (right.CreatedAt || 0);
    }
  });
  return next;
}

function usesUuid(type?: string): boolean {
  return type === 'vless' || type === 'vmess';
}

function credentialLabel(type?: string): string {
  return credentialOptions.find((item) => item.value === type)?.label || type || 'VLESS';
}

function primaryCredential(user: UserDraft): string {
  if (usesUuid(user.CredentialType)) return user.UUID || user.CredentialValue || '';
  if (user.CredentialType === 'trojan' || user.CredentialType === 'shadowsocks') return user.Password || user.CredentialValue || '';
  if (user.CredentialType === 'hysteria') return user.Auth || user.CredentialValue || '';
  if (user.CredentialType === 'raw-tcp' || user.CredentialType === 'raw-udp') return user.VKey || '';
  return user.CredentialValue || '';
}

function hydrateUserConfig(config: RuntimeConfig): RuntimeConfig {
  const vkeys = new Map((config.VKeys || []).map((item) => [`${nodeIDOf(item)}:${item.ID}`, item]));
  return {
    ...config,
    Clients: (config.Clients || []).map((item) => {
      const record = item as UserRecord;
      const vkey = vkeys.get(`${nodeIDOf(record)}:${record.Binding?.VKeyID || ''}`);
      return {
        ...record,
        ListenerIDs: record.ListenerIDs?.length ? record.ListenerIDs : (record.ListenerID ? [record.ListenerID] : []),
        VKey: vkey?.Value || '',
        AllowedDevices: record.AllowedDeviceIDs || [],
        UUID: record.UUID || (usesUuid(record.CredentialType) ? record.CredentialValue || '' : ''),
        Password: record.Password || (record.CredentialType === 'trojan' || record.CredentialType === 'shadowsocks' ? record.CredentialValue || '' : ''),
        Auth: record.Auth || (record.CredentialType === 'hysteria' ? record.CredentialValue || '' : ''),
      };
    }),
  };
}

function hydrateUserStats(config: RuntimeConfig, quotaStates: Array<ClientQuotaState & NodeOwned>): RuntimeConfig {
  const states = new Map(quotaStates.map((item) => [`${nodeIDOf(item)}:${item.id}`, item]));
  return {
    ...config,
    Clients: (config.Clients || []).map((item) => {
      const state = states.get(`${nodeIDOf(item)}:${item.ID}`);
      if (!state) return item;
      return {
        ...item,
        Online: (state.activePipes || 0) > 0,
        Traffic: {
          up: state.counters?.rxBytes || 0,
          down: state.counters?.txBytes || 0,
          lastOnline: 0,
        },
      } as UserRecord;
    }),
  };
}

function mergeUserAddress(
  addresses: TapxAddressLimit[],
  userId: string,
  existingAddressId: string | undefined,
  ipv4: string[],
  ipv6: string[],
  macs: string[],
  deviceId?: string,
  ownerID = 'local',
): { addressId: string; addresses: TapxAddressLimit[] } {
  const hasLimit = ipv4.length > 0 || ipv6.length > 0 || macs.length > 0 || !!deviceId;
  const generatedId = `addr-user-${userId}`;
  const addressId = existingAddressId || generatedId;
  const isGenerated = (item: TapxAddressLimit) => item.ID === addressId
    && nodeIDOf(item) === ownerID
    && isManagedUserAddressRemark(item.Remark);
  if (!hasLimit) {
    return {
      addressId: '',
      addresses: addresses.filter((item) => !isGenerated(item)),
    };
  }

  const address: TapxAddressLimit & NodeOwned = {
    ManagedNodeID: ownerID,
    ID: addressId,
    Enabled: true,
    Name: `${userId}-source-limit`,
    ClientID: userId,
    DeviceID: deviceId || '',
    IPv4CIDRs: ipv4,
    IPv6CIDRs: ipv6,
    MACs: macs,
    Remark: managedUserAddressRemark,
  };
  const index = addresses.findIndex((item) => item.ID === addressId && nodeIDOf(item) === ownerID);
  if (index < 0) return { addressId, addresses: [...addresses, address] };
  return {
    addressId,
    addresses: addresses.map((item) => (item.ID === addressId && nodeIDOf(item) === ownerID ? { ...item, ...address } : item)),
  };
}

async function copyShareLinks(links: string[], messageApi: ReturnType<typeof message.useMessage>[0], t: ReturnType<typeof useI18n>['t']) {
  try {
    await copyText(links.join('\n'));
    messageApi.success(t('user.importLinksCopied'));
  } catch {
    messageApi.error(t('user.copyFailed'));
  }
}

function credentialToken(user: UserRecord, kind: 'uuid' | 'password' | 'hysteria', t: ReturnType<typeof useI18n>['t']) {
  const value = kind === 'uuid'
    ? user.UUID || (usesUuid(user.CredentialType) ? user.CredentialValue : '')
    : kind === 'password'
      ? user.Password || (user.CredentialType === 'trojan' || user.CredentialType === 'shadowsocks' ? user.CredentialValue : '')
      : user.Auth || (user.CredentialType === 'hysteria' ? user.CredentialValue : '');
  return value ? <Tag className="info-large-tag">{value}</Tag> : <Tag>{t('user.none')}</Tag>;
}

function copyableToken(label: string, value: string, messageApi: ReturnType<typeof message.useMessage>[0], t: ReturnType<typeof useI18n>['t']) {
  return (
    <Space size={4}>
      <Tag className="info-large-tag">{label}</Tag>
      {value ? (
        <Button
          size="small"
          type="text"
          icon={<CopyOutlined />}
          aria-label={t('user.copy')}
          onClick={() => {
            void copyText(value).then(() => messageApi.success(t('user.copied'))).catch(() => messageApi.error(t('user.copyFailed')));
          }}
        />
      ) : null}
    </Space>
  );
}

function tagList(values: string[], color: string | undefined, t: ReturnType<typeof useI18n>['t']) {
  if (values.length === 0) return <Tag>{t('user.none')}</Tag>;
  return (
    <Space size={4} wrap>
      {values.map((value) => <Tag key={value} color={color}>{value}</Tag>)}
    </Space>
  );
}

function listenerTags(user: UserRecord, listeners: TapxEndpoint[], t: ReturnType<typeof useI18n>['t']) {
  const ids = listenerIdsFor(user);
  if (ids.length === 0) return <Tag>{t('user.none')}</Tag>;
  return (
    <Space size={4} wrap>
      {ids.map((id) => {
        const listener = listeners.find((item) => item.ID === id && nodeIDOf(item) === nodeIDOf(user));
        return <Tag key={id} color="blue">{listener ? labelEndpoint(listener) : id}</Tag>;
      })}
    </Space>
  );
}
