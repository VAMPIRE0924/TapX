import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
  Alert,
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
  AimOutlined,
  ArrowDownOutlined,
  ArrowUpOutlined,
  CheckCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  ExportOutlined,
  HolderOutlined,
  ImportOutlined,
  MoreOutlined,
  PlusOutlined,
  SearchOutlined,
  StopOutlined,
  QuestionCircleOutlined,
  UnorderedListOutlined,
} from '@ant-design/icons';
import {
  type RuntimeConfig,
  type TapxAddressLimit,
  type TapxClient,
  type TapxDevice,
  type TapxRoute,
} from '../shared/api';
import {
  applyManagedRuntimeConfig as applyRuntimeConfig,
  defaultTargetNodeID,
  filterConfigByNode,
  filterNodeOwned,
  getManagedRuntimeConfig as getRuntimeConfig,
  nodeIDOf,
  nodeObjectKey,
  saveManagedRuntimeConfig as saveRuntimeConfig,
  sameNodeObject,
  type NodeOwned,
} from '../features/nodes/managedConfig';
import { NodeScopeSelect, NodeSourceTag, useNodeScope, useNodeTargetOptions } from '../features/nodes/NodeScope';
import {
  buildIndex,
  emptyConfig,
  joinList,
  labelClient,
  labelDevice,
  labelEndpoint,
  labelVKey,
  nextId,
  optionList,
  routeAddress,
  splitList,
} from '../shared/tapx-model';
import {
  buildRouteTransferBundle,
  importRouteTransferBundle,
} from '../features/links/routeTransfer';
import {
  buildLinkTestRows,
  filterLinkTestRows,
  sourceGuardForDevice,
  type LinkQueryMode,
  type LinkTestRow,
} from '../features/links/linkDiagnostics';
import { useI18n } from '../i18n/I18nProvider';
import { isManagedLinkAddressRemark, managedLinkAddressRemark } from '../shared/managed-objects';
import './LinkBindingPage.css';

type RouteAction = 'bind-device' | 'allow' | 'drop';

type RouteRecord = TapxRoute & NodeOwned & {
  Priority?: number;
  Action?: RouteAction;
};

interface RouteDraft {
  Mode: 'create' | 'edit';
  OriginalID: string;
  ID: string;
  Enabled: boolean;
  Priority: number;
  Action: RouteAction;
  VKeyID: string;
  ListenerID: string;
  DeviceID: string;
  ConnectorID: string;
  ClientID: string;
  AddressID: string;
  AddressName: string;
  IPv4CIDRs: string;
  IPv6CIDRs: string;
  MACs: string;
  ManagedNodeID: string;
}

interface RuleRow {
  key: string;
  index: number;
  route: RouteRecord;
  enabled: boolean;
  priority: number;
  action: RouteAction;
  id: string;
  vkey: string;
  user: string;
  listener: string;
  connector: string;
  device: string;
  allowedIPs: string;
  allowedMACs: string;
}

let linkBindingDraftCache: RuntimeConfig | null = null;
let linkBindingDraftDirty = false;

function tabLabel(icon: ReactNode, text: string, hint?: string): ReactNode {
  const label = (
    <span className="link-tab-label">
      {icon}
      <span>{text}</span>
      {hint ? <QuestionCircleOutlined /> : null}
    </span>
  );
  return hint ? <Tooltip title={hint}>{label}</Tooltip> : label;
}

function emptyDash() {
  return <span className="criterion-empty">-</span>;
}

export function LinkBindingPage() {
  const { t } = useI18n();
  const [config, setConfig] = useState<RuntimeConfig>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(linkBindingDraftDirty);
  const [error, setError] = useState('');
  const [editing, setEditing] = useState<RouteDraft | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [importValue, setImportValue] = useState('');
  const [importTargetNodeID, setImportTargetNodeID] = useState('local');
  const [exportValue, setExportValue] = useState('');
  const [selectedRowKeys, setSelectedRowKeys] = useState<string[]>([]);
  const [queryMode, setQueryMode] = useState<LinkQueryMode>('device');
  const [queryInput, setQueryInput] = useState('');
  const [query, setQuery] = useState('');
  const [draggedIndex, setDraggedIndex] = useState<number | null>(null);
  const [dropTargetIndex, setDropTargetIndex] = useState<number | null>(null);
  const dragRef = useRef<{ from: number | null; to: number | null; startY: number; moved: boolean }>({
    from: null,
    to: null,
    startY: 0,
    moved: false,
  });
  const [messageApi, messageContextHolder] = message.useMessage();
  const [modal, modalContextHolder] = Modal.useModal();
  const { nodes, scope, setScope } = useNodeScope();
  const nodeTargetOptions = useNodeTargetOptions(nodes);

  const normalized = useMemo(() => emptyConfig(config), [config]);
  const routes = useMemo(() => normalized.Routes as RouteRecord[], [normalized.Routes]);
  const visibleRoutes = useMemo(() => filterNodeOwned(routes, scope), [routes, scope]);
  const selectedRoutes = useMemo(
    () => routes.filter((route) => selectedRowKeys.includes(nodeObjectKey(route))),
    [routes, selectedRowKeys],
  );
  useEffect(() => {
    const visibleKeys = new Set(visibleRoutes.map(nodeObjectKey));
    setSelectedRowKeys((current) => current.filter((key) => visibleKeys.has(key)));
  }, [visibleRoutes]);
  const rows = useMemo(() => visibleRoutes.flatMap((route) => (
    buildRuleRows([route], buildIndex(filterConfigByNode(config, nodeIDOf(route))))
  )), [config, visibleRoutes]);
  const linkRows = useMemo(() => {
    if (scope !== 'all') return buildLinkTestRows(filterConfigByNode(config, scope));
    const nodeIDs = new Set((config.Routes || []).map(nodeIDOf));
    return [...nodeIDs].flatMap((nodeID) => buildLinkTestRows(filterConfigByNode(config, nodeID)));
  }, [config, scope]);
  const filteredRows = useMemo(() => filterLinkTestRows(linkRows, queryMode, query), [linkRows, queryMode, query]);

  const targetNodeID = editing?.ManagedNodeID || defaultTargetNodeID(scope);
  const deviceOptions = useMemo(() => optionList(filterNodeOwned(normalized.Devices as Array<TapxDevice & NodeOwned>, targetNodeID), labelDevice, (item) => item.IfName || item.ID), [normalized.Devices, targetNodeID]);
  const listenerOptions = useMemo(() => optionList(filterNodeOwned(normalized.Listeners as Array<(typeof normalized.Listeners)[number] & NodeOwned>, targetNodeID), labelEndpoint, (item) => item.Transport || 'xray'), [normalized.Listeners, targetNodeID]);
  const connectorOptions = useMemo(() => optionList(filterNodeOwned(normalized.Connectors as Array<(typeof normalized.Connectors)[number] & NodeOwned>, targetNodeID), labelEndpoint, (item) => item.Transport || 'xray'), [normalized.Connectors, targetNodeID]);
  const clientOptions = useMemo(() => optionList(filterNodeOwned(normalized.Clients as Array<TapxClient & NodeOwned>, targetNodeID), labelClient, (item) => item.ID), [normalized.Clients, targetNodeID]);
  const vkeyOptions = useMemo(() => optionList(filterNodeOwned(normalized.VKeys as Array<(typeof normalized.VKeys)[number] & NodeOwned>, targetNodeID), labelVKey), [normalized.VKeys, targetNodeID]);
  const queryModes = useMemo<Array<{ key: LinkQueryMode; label: string }>>(() => [
    { key: 'device', label: t('link.device') },
    { key: 'connector', label: t('link.connector') },
    { key: 'listener', label: t('link.listener') },
    { key: 'user', label: t('link.user') },
    { key: 'vkey', label: 'vKey' },
    { key: 'ip', label: t('link.sourceIp') },
    { key: 'mac', label: t('link.sourceMac') },
  ], [t]);
  const actionLabels = useMemo<Record<RouteAction, string>>(() => ({
    'bind-device': t('link.bindDevice'),
    allow: t('link.allow'),
    drop: t('link.drop'),
  }), [t]);

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    const preventAccidentalReload = (event: BeforeUnloadEvent) => {
      if (!dirty) return;
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', preventAccidentalReload);
    return () => window.removeEventListener('beforeunload', preventAccidentalReload);
  }, [dirty]);

  async function refresh() {
    setLoading(true);
    setError('');
    try {
      const stored = await getRuntimeConfig();
      if (linkBindingDraftDirty && linkBindingDraftCache) {
        setConfig(linkBindingDraftCache);
        setDirty(true);
      } else {
        linkBindingDraftCache = null;
        linkBindingDraftDirty = false;
        setConfig(stored);
        setDirty(false);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : t('link.loadFailed'));
    } finally {
      setLoading(false);
    }
  }

  function stageConfig(next: RuntimeConfig, successText?: string) {
    linkBindingDraftCache = next;
    linkBindingDraftDirty = true;
    setConfig(next);
    setDirty(true);
    if (successText) messageApi.info(t('link.saveRequired', { action: successText }));
  }

  async function persistStagedConfig() {
    if (!dirty) return;
    setSaving(true);
    setError('');
    try {
      const saved = await saveRuntimeConfig(config);
      setConfig(saved);
      linkBindingDraftCache = null;
      linkBindingDraftDirty = false;
      setDirty(false);
      try {
        await applyRuntimeConfig();
        messageApi.success(t('link.savedAndReloaded'));
      } catch (err) {
        messageApi.warning(t('link.reloadFailed', { error: err instanceof Error ? err.message : String(err) }));
      }
    } catch (err) {
      const messageText = err instanceof Error ? err.message : t('link.saveFailed');
      setError(messageText);
      messageApi.error(messageText);
    } finally {
      setSaving(false);
    }
  }

  function openCreate() {
    const ids = new Set(routes.map((item) => item.ID));
    setEditing({
      Mode: 'create',
      OriginalID: '',
      ID: nextId('route', ids),
      Enabled: true,
      Priority: 100,
      Action: 'bind-device',
      VKeyID: '',
      ListenerID: '',
      DeviceID: '',
      ConnectorID: '',
      ClientID: '',
      AddressID: '',
      AddressName: '',
      IPv4CIDRs: '',
      IPv6CIDRs: '',
      MACs: '',
      ManagedNodeID: defaultTargetNodeID(scope),
    });
  }

  function openEdit(route: RouteRecord) {
    const address = routeAddress(route, buildIndex(filterConfigByNode(config, nodeIDOf(route))));
    setEditing({
      Mode: 'edit',
      OriginalID: route.ID,
      ID: route.ID,
      Enabled: route.Enabled !== false,
      Priority: route.Priority ?? 100,
      Action: route.Action || 'bind-device',
      VKeyID: route.VKeyID || '',
      ListenerID: route.ListenerID || '',
      DeviceID: route.DeviceID || '',
      ConnectorID: route.ConnectorID || '',
      ClientID: route.ClientID || '',
      AddressID: route.AddressID || '',
      AddressName: address?.Name || '',
      IPv4CIDRs: joinList(address?.IPv4CIDRs),
      IPv6CIDRs: joinList(address?.IPv6CIDRs),
      MACs: joinList(address?.MACs),
      ManagedNodeID: nodeIDOf(route),
    });
  }

  async function persistDraft() {
    if (!editing) return;
    const id = editing.ID.trim();
    if (!id) {
      messageApi.error(t('link.idRequired'));
      return;
    }
    const duplicate = routes.some((route) => route.ID === id
      && nodeIDOf(route) === editing.ManagedNodeID
      && (editing.Mode === 'create' || route.ID !== editing.OriginalID));
    if (duplicate) {
      messageApi.error(t('link.idExists'));
      return;
    }

    const hasSourceLimit = splitList(editing.IPv4CIDRs).length > 0
      || splitList(editing.IPv6CIDRs).length > 0
      || splitList(editing.MACs).length > 0;
    const hasMatch = Boolean(
      editing.VKeyID
      || editing.ListenerID
      || editing.DeviceID
      || editing.ConnectorID
      || editing.ClientID
      || editing.AddressID
      || hasSourceLimit,
    );
    if (!hasMatch) {
      messageApi.error(t('link.matchRequired'));
      return;
    }
    if (editing.Action === 'bind-device' && !editing.DeviceID) {
      messageApi.error(t('link.deviceRequired'));
      return;
    }

    const selectedDevice = editing.DeviceID
      ? buildIndex(filterConfigByNode(config, editing.ManagedNodeID)).devices.get(editing.DeviceID)
      : undefined;
    const namedDraft = { ...editing, AddressName: editing.AddressName || t('link.sourceLimitName', { id }) };
    const sanitized = selectedDevice?.Type === 'tun' ? { ...namedDraft, MACs: '' } : namedDraft;
    stageConfig(mergeDraftIntoConfig(config, sanitized), editing.Mode === 'edit' ? t('link.updated') : t('link.created'));
    setEditing(null);
  }

  function confirmDelete(route: RouteRecord) {
    modal.confirm({
      title: t('link.deleteConfirm', { id: route.ID }),
      okText: t('common.delete'),
      okType: 'danger',
      cancelText: t('link.cancel'),
      onOk: () => stageConfig(removeRouteFromConfig(config, route), t('link.deleted')),
    });
  }

  function moveRoute(from: number, to: number) {
    if (from === to || from < 0 || to < 0 || from >= visibleRoutes.length || to >= visibleRoutes.length) return;
    const reordered = [...visibleRoutes];
    const [moved] = reordered.splice(from, 1);
    reordered.splice(to, 0, moved);
    let visibleIndex = 0;
    const nextRoutes = routes.map((route) => (
      visibleRoutes.some((visible) => sameNodeObject(visible, route)) ? reordered[visibleIndex++] : route
    ));
    stageConfig({ ...config, Routes: nextRoutes }, t('link.reordered'));
  }

  function toggleRoute(index: number, enabled: boolean) {
    const target = visibleRoutes[index];
    if (!target) return;
    const nextRoutes = routes.map((route) => sameNodeObject(route, target) ? { ...route, Enabled: enabled } : route);
    stageConfig({ ...config, Routes: nextRoutes });
  }

  function onHandlePointerDown(idxValue: number, ev: React.PointerEvent) {
    if (ev.button != null && ev.button !== 0) return;
    ev.preventDefault();
    try {
      (ev.currentTarget as Element).setPointerCapture(ev.pointerId);
    } catch {
      // Pointer capture can fail on detached nodes; dragging still works through document listeners.
    }
    dragRef.current = { from: idxValue, to: idxValue, startY: ev.clientY, moved: false };
    setDraggedIndex(idxValue);
    setDropTargetIndex(idxValue);

    const onMove = (event: PointerEvent) => {
      const state = dragRef.current;
      if (state.from == null) return;
      if (!state.moved && Math.abs(event.clientY - state.startY) < 5) return;
      state.moved = true;
      const target = document.elementFromPoint(event.clientX, event.clientY)?.closest('[data-row-key]');
      if (!target) return;
      const nextIndex = Number(target.getAttribute('data-row-key'));
      if (Number.isFinite(nextIndex) && nextIndex !== state.to) {
        state.to = nextIndex;
        setDropTargetIndex(nextIndex);
      }
    };

    const onUp = () => {
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onUp);
      document.removeEventListener('pointercancel', onUp);
      const { from, to, moved } = dragRef.current;
      dragRef.current = { from: null, to: null, startY: 0, moved: false };
      setDraggedIndex(null);
      setDropTargetIndex(null);
      if (moved && from != null && to != null && from !== to) moveRoute(from, to);
    };

    document.addEventListener('pointermove', onMove);
    document.addEventListener('pointerup', onUp);
    document.addEventListener('pointercancel', onUp);
  }

  function exportRules(exporting: RouteRecord[] = routes) {
    setExportValue(JSON.stringify(buildRouteTransferBundle({ ...config, Routes: exporting }), null, 2));
    setExportOpen(true);
  }

  function setSelectedRoutesEnabled(enabled: boolean) {
    if (selectedRoutes.length === 0) return;
    const selected = new Set(selectedRoutes.map(nodeObjectKey));
    stageConfig({
      ...config,
      Routes: routes.map((route) => selected.has(nodeObjectKey(route)) ? { ...route, Enabled: enabled } : route),
    }, enabled ? t('link.batchEnabled') : t('link.batchDisabled'));
    setSelectedRowKeys([]);
  }

  function confirmDeleteSelectedRoutes() {
    modal.confirm({
      title: t('link.batchDeleteConfirm', { count: selectedRoutes.length }),
      okText: t('common.delete'),
      okType: 'danger',
      cancelText: t('link.cancel'),
      onOk: () => {
        const next = selectedRoutes.reduce((current, route) => removeRouteFromConfig(current, route), config);
        stageConfig(next, t('link.deletedCount', { count: selectedRoutes.length }));
        setSelectedRowKeys([]);
      },
    });
  }

  function importRules() {
    let parsed: unknown;
    try {
      parsed = JSON.parse(importValue);
    } catch {
      messageApi.error(t('link.invalidJson'));
      return;
    }
    let imported: ReturnType<typeof importRouteTransferBundle>;
    try {
      imported = importRouteTransferBundle(parsed, config, importTargetNodeID, t);
    } catch (importError) {
      messageApi.error(importError instanceof Error ? importError.message : t('link.rulesNotFound'));
      return;
    }
    stageConfig({
      ...config,
      Routes: [...routes, ...imported.routes],
      Addresses: imported.addresses,
    }, t('link.imported'));
    setImportOpen(false);
    setImportValue('');
  }

  const moreItems: MenuProps['items'] = selectedRoutes.length > 0 ? [
    { key: 'enable-selected', icon: <CheckCircleOutlined />, label: t('link.enableSelected'), onClick: () => setSelectedRoutesEnabled(true) },
    { key: 'disable-selected', icon: <StopOutlined />, label: t('link.disableSelected'), onClick: () => setSelectedRoutesEnabled(false) },
    { type: 'divider' },
    { key: 'export-selected', icon: <ExportOutlined />, label: t('link.exportSelected'), onClick: () => exportRules(selectedRoutes) },
    { key: 'delete-selected', icon: <DeleteOutlined />, label: t('link.deleteSelected'), danger: true, onClick: confirmDeleteSelectedRoutes },
  ] : [
    { key: 'import', icon: <ImportOutlined />, label: t('link.import'), onClick: () => {
      setImportTargetNodeID(defaultTargetNodeID(scope));
      setImportOpen(true);
    } },
    { key: 'export', icon: <ExportOutlined />, label: t('link.export'), disabled: visibleRoutes.length === 0, onClick: () => exportRules(visibleRoutes) },
  ];

  const tableColumns = useMemo<TableColumnsType<RuleRow>>(
    () => [
      {
        title: '#',
        align: 'center',
        width: 60,
        key: 'index',
        render: (_value, _record, index) => (
          <div className="action-cell" style={{ justifyContent: 'center' }}>
            <HolderOutlined
              className="drag-handle"
              title={t('link.dragSort')}
              aria-hidden="true"
              onPointerDown={(event) => onHandlePointerDown(index, event)}
            />
            <span className="row-index">{index + 1}</span>
          </div>
        ),
      },
      {
        title: t('node.sourceNode'),
        key: 'ManagedNodeID',
        width: 150,
        render: (_value, record) => <NodeSourceTag value={record.route} />,
      },
      {
        title: t('link.actions'),
        align: 'center',
        width: 80,
        key: 'actions',
        render: (_value, record, index) => (
          <div className="action-buttons" style={{ justifyContent: 'center', margin: 0 }}>
            <Button shape="circle" size="small" icon={<EditOutlined />} aria-label={t('link.edit')} onClick={() => openEdit(record.route)} />
            <Dropdown
              trigger={['click']}
              menu={{
                items: [
                  { key: 'up', label: <><ArrowUpOutlined /> {t('common.moveUp')}</>, disabled: index === 0, onClick: () => moveRoute(index, index - 1) },
                  { key: 'down', label: <><ArrowDownOutlined /> {t('common.moveDown')}</>, disabled: index === routes.length - 1, onClick: () => moveRoute(index, index + 1) },
                  { key: 'delete', danger: true, label: <><DeleteOutlined /> {t('common.delete')}</>, onClick: () => confirmDelete(record.route) },
                ],
              }}
            >
              <Button shape="circle" size="small" icon={<MoreOutlined />} aria-label={t('link.more')} />
            </Dropdown>
          </div>
        ),
      },
      {
        title: t('common.enabled'),
        align: 'center',
        width: 80,
        key: 'enabled',
        render: (_value, record, index) => (
          <Switch size="small" checked={record.enabled} loading={saving} onChange={(checked) => toggleRoute(index, checked)} />
        ),
      },
      {
        title: t('link.priority'),
        align: 'center',
        width: 90,
        dataIndex: 'priority',
      },
      {
        title: t('link.action'),
        width: 120,
        dataIndex: 'action',
        render: (value: RouteAction) => <Tag color={value === 'drop' ? 'red' : value === 'allow' ? 'green' : 'blue'}>{actionLabels[value]}</Tag>,
      },
      {
        title: 'vKey',
        width: 150,
        dataIndex: 'vkey',
        render: (value: string) => value ? <Tag>{value}</Tag> : emptyDash(),
      },
      {
        title: t('link.user'),
        width: 180,
        dataIndex: 'user',
        render: (value: string) => value ? <Tooltip title={value}><span className="criterion-chip-value">{value}</span></Tooltip> : emptyDash(),
      },
      {
        title: t('link.listener'),
        width: 180,
        dataIndex: 'listener',
        render: (value: string) => value ? <Tag color="blue">{value}</Tag> : emptyDash(),
      },
      {
        title: 'ID',
        width: 140,
        dataIndex: 'id',
        render: (value: string) => <Tooltip title={value}><span className="criterion-chip-value">{value}</span></Tooltip>,
      },
      {
        title: t('link.connector'),
        width: 180,
        dataIndex: 'connector',
        render: (value: string) => value ? <Tag color="purple">{value}</Tag> : emptyDash(),
      },
      {
        title: t('link.tunTapDevice'),
        width: 160,
        dataIndex: 'device',
        render: (value: string) => value ? <Tag color="cyan">{value}</Tag> : emptyDash(),
      },
      {
        title: t('link.allowedIps'),
        width: 220,
        dataIndex: 'allowedIPs',
        render: (value: string) => value ? <Tooltip title={value}><span className="criterion-chip-value">{value}</span></Tooltip> : emptyDash(),
      },
      {
        title: t('link.allowedMacs'),
        width: 220,
        dataIndex: 'allowedMACs',
        render: (value: string) => value ? <Tooltip title={value}><span className="criterion-chip-value">{value}</span></Tooltip> : emptyDash(),
      },
    ],
    [actionLabels, routes.length, saving, t],
  );

  const testerColumns = useMemo<TableColumnsType<LinkTestRow>>(
    () => [
      {
        title: t('link.type'),
        dataIndex: 'kind',
        width: 120,
        render: (kind: LinkTestRow['kind']) => {
          const color = kind === 'connector' ? 'purple' : kind === 'listener' ? 'blue' : kind === 'user' ? 'green' : 'cyan';
          const label = kind === 'connector' ? t('link.connectorBinding') : kind === 'listener' ? t('link.listener') : kind === 'user' ? t('link.listenerUser') : t('link.rule');
          return <Tag color={color}>{label}</Tag>;
        },
      },
      { title: t('link.connector'), dataIndex: 'connector', width: 160, render: renderTextCell },
      { title: t('link.listener'), dataIndex: 'listener', width: 160, render: renderTextCell },
      { title: t('link.user'), dataIndex: 'user', width: 160, render: renderTextCell },
      { title: 'vKey', dataIndex: 'vkey', width: 140, render: (value: string) => value ? <Tag>{value}</Tag> : emptyDash() },
      { title: t('link.tunTapDevice'), dataIndex: 'device', width: 150, render: (value: string) => value ? <Tag color="cyan">{value}</Tag> : emptyDash() },
      { title: t('link.allowedIps'), dataIndex: 'allowedIPs', width: 220, render: renderTextCell },
      { title: t('link.allowedMacs'), dataIndex: 'allowedMACs', width: 220, render: renderTextCell },
      {
        title: t('link.endpointAction'),
        dataIndex: 'endpoint',
        width: 180,
        render: (value: string, record) => record.action
          ? renderTextCell(record.action === 'disabled' ? t('common.disabled') : actionLabels[record.action])
          : renderTextCell(value),
      },
    ],
    [actionLabels, t],
  );

  return (
    <>
      {messageContextHolder}
      {modalContextHolder}
      <div className="link-binding-page">
        <Card hoverable className="link-save-card">
          <Space wrap>
            <Button type="primary" loading={saving} disabled={!dirty} onClick={() => void persistStagedConfig()}>
              {t('common.save')}
            </Button>
          </Space>
        </Card>
        {error ? <Alert type="error" showIcon title={error} /> : null}
        <Tabs
          defaultActiveKey="rules"
          items={[
            {
              key: 'rules',
              label: tabLabel(<UnorderedListOutlined />, t('link.title')),
              children: (
                <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
                  <Space wrap>
                    <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                      {t('link.add')}
                    </Button>
                    <NodeScopeSelect scope={scope} onChange={setScope} />
                    {selectedRoutes.length > 0 ? (
                      <Tag color="blue" closable onClose={() => setSelectedRowKeys([])}>
                        {t('link.selectedCount', { count: selectedRoutes.length })}
                      </Tag>
                    ) : null}
                    <Dropdown trigger={['click']} menu={{ items: moreItems }}>
                      <Button icon={<MoreOutlined />}>{selectedRoutes.length > 0 ? t('link.batchActions') : t('link.more')}</Button>
                    </Dropdown>
                  </Space>

                  <Table
                    columns={tableColumns}
                    dataSource={rows}
                    rowKey={(row) => nodeObjectKey(row.route)}
                    rowSelection={{ selectedRowKeys, onChange: (keys) => setSelectedRowKeys(keys.map(String)) }}
                    loading={loading || saving}
                    pagination={false}
                    scroll={{ x: 2050 }}
                    size="small"
                    className="routing-table"
                    locale={{ emptyText: t('link.empty') }}
                    onRow={(_record, index) => {
                      const classes: string[] = [];
                      const rowIndex = index ?? -1;
                      if (draggedIndex === rowIndex) classes.push('row-dragging');
                      if (dropTargetIndex === rowIndex && draggedIndex !== rowIndex && draggedIndex != null) {
                        classes.push(rowIndex > draggedIndex ? 'drop-after' : 'drop-before');
                      }
                      return { className: classes.join(' '), 'data-row-key': rowIndex } as React.HTMLAttributes<HTMLElement>;
                    }}
                  />
                </Space>
              ),
            },
            {
              key: 'tester',
                label: tabLabel(<AimOutlined />, t('link.test'), t('link.testHelp')),
                children: (
                  <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
                    <Row gutter={[12, 12]} align="middle">
                    <Col flex="none">
                      <Radio.Group
                        optionType="button"
                        buttonStyle="solid"
                        value={queryMode}
                        onChange={(event) => setQueryMode(event.target.value as LinkQueryMode)}
                        options={queryModes.map((item) => ({ value: item.key, label: item.label }))}
                      />
                    </Col>
                    <Col flex="auto">
                      <Input
                        value={queryInput}
                        onChange={(event) => {
                          setQueryInput(event.target.value);
                          if (!event.target.value) setQuery('');
                        }}
                        onPressEnter={() => setQuery(queryInput)}
                        placeholder={t('link.queryPlaceholder')}
                        allowClear
                      />
                    </Col>
                    <Col flex="none">
                      <Button type="primary" icon={<SearchOutlined />} onClick={() => setQuery(queryInput)}>
                        {t('link.query')}
                      </Button>
                    </Col>
                  </Row>
                  <Table
                    size="small"
                    columns={testerColumns}
                    dataSource={filteredRows}
                    rowKey={(row) => row.key}
                    pagination={false}
                    scroll={{ x: 1500 }}
                    locale={{ emptyText: t('link.noMatches') }}
                  />
                </Space>
              ),
            },
          ]}
        />
      </div>

      <RouteEditor
        draft={editing}
        saving={saving}
        devices={normalized.Devices}
        clients={normalized.Clients}
        deviceOptions={deviceOptions}
        listenerOptions={listenerOptions}
        connectorOptions={connectorOptions}
        clientOptions={clientOptions}
        vkeyOptions={vkeyOptions}
        nodeTargetOptions={nodeTargetOptions}
        onChange={setEditing}
        onClose={() => setEditing(null)}
        onSave={() => void persistDraft()}
      />

      <Modal
        open={importOpen}
        title={t('link.import')}
        okText={t('link.importAction')}
        cancelText={t('common.close')}
        onOk={importRules}
        onCancel={() => setImportOpen(false)}
        mask={{ closable: false }}
      >
        <Form.Item label={t('node.targetNode')} tooltip={t('node.targetNodeHelp')}>
          <Select value={importTargetNodeID} options={nodeTargetOptions} onChange={setImportTargetNodeID} />
        </Form.Item>
        <Form.Item label={t('link.importContent')} tooltip={t('link.importHelp')}>
          <Input.TextArea
            value={importValue}
            onChange={(event) => setImportValue(event.target.value)}
            rows={10}
            placeholder={t('link.importPlaceholder')}
          />
        </Form.Item>
      </Modal>

      <Modal
        open={exportOpen}
        title={t('link.export')}
        okText={t('common.close')}
        cancelButtonProps={{ style: { display: 'none' } }}
        onOk={() => setExportOpen(false)}
        onCancel={() => setExportOpen(false)}
      >
        <Input.TextArea value={exportValue} rows={12} readOnly />
      </Modal>
    </>
  );
}

function RouteEditor({
  draft,
  saving,
  devices,
  clients,
  deviceOptions,
  listenerOptions,
  connectorOptions,
  clientOptions,
  vkeyOptions,
  nodeTargetOptions,
  onChange,
  onClose,
  onSave,
}: {
  draft: RouteDraft | null;
  saving: boolean;
  devices: TapxDevice[];
  clients: TapxClient[];
  deviceOptions: Array<{ id: string; label: string; detail?: string }>;
  listenerOptions: Array<{ id: string; label: string; detail?: string }>;
  connectorOptions: Array<{ id: string; label: string; detail?: string }>;
  clientOptions: Array<{ id: string; label: string; detail?: string }>;
  vkeyOptions: Array<{ id: string; label: string; detail?: string }>;
  nodeTargetOptions: Array<{ value: string; label: string; disabled?: boolean }>;
  onChange: (draft: RouteDraft | null) => void;
  onClose: () => void;
  onSave: () => void;
}) {
  const { t } = useI18n();
  const selectedDevice = draft?.DeviceID
    ? devices.find((device) => device.ID === draft.DeviceID && nodeIDOf(device) === draft.ManagedNodeID)
    : undefined;
  const isTun = selectedDevice?.Type === 'tun';
  const title = draft?.Mode === 'edit' ? t('link.editTitle') : t('link.addTitle');
  const okText = draft?.Mode === 'edit' ? t('link.saveChanges') : t('common.create');
  const actionLabels: Record<RouteAction, string> = {
    'bind-device': t('link.bindDevice'),
    allow: t('link.allow'),
    drop: t('link.drop'),
  };

  function update<K extends keyof RouteDraft>(key: K, value: RouteDraft[K]) {
    if (!draft) return;
    onChange({ ...draft, [key]: value });
  }

  function updateAllowedIPs(value: string) {
    if (!draft) return;
    const values = splitList(value);
    onChange({
      ...draft,
      IPv4CIDRs: values.filter((item) => !item.includes(':')).join('\n'),
      IPv6CIDRs: values.filter((item) => item.includes(':')).join('\n'),
    });
  }

  function updateClient(clientId: string) {
    if (!draft) return;
    const client = clients.find((item) => item.ID === clientId && nodeIDOf(item) === draft.ManagedNodeID);
    onChange({
      ...draft,
      ClientID: clientId,
      ListenerID: draft.ListenerID || client?.ListenerID || '',
    });
  }

  return (
    <Modal
      open={!!draft}
      title={draft?.Mode === 'edit' ? title : t('link.addDialogTitle')}
      okText={okText}
      cancelText={t('common.close')}
      width={640}
      confirmLoading={saving}
      onOk={onSave}
      onCancel={onClose}
      mask={{ closable: false }}
    >
      {draft ? (
        <Form colon={false} labelCol={{ md: { span: 8 } }} wrapperCol={{ md: { span: 14 } }}>
          <Form.Item label={t('node.targetNode')} tooltip={t('node.targetNodeHelp')}>
            <Select
              value={draft.ManagedNodeID}
              options={nodeTargetOptions}
              disabled={draft.Mode === 'edit'}
              onChange={(value) => onChange({
                ...draft,
                ManagedNodeID: value,
                VKeyID: '',
                ListenerID: '',
                DeviceID: '',
                ConnectorID: '',
                ClientID: '',
                AddressID: '',
              })}
            />
          </Form.Item>
          <Form.Item label={t('common.enabled')}>
            <Switch checked={draft.Enabled} onChange={(checked) => update('Enabled', checked)} />
          </Form.Item>

          <Form.Item label={t('link.priority')} tooltip={t('link.priorityHelp')}>
            <InputNumber
              value={draft.Priority}
              min={0}
              style={{ width: 160 }}
              onChange={(value) => update('Priority', Number(value) || 0)}
            />
          </Form.Item>

          <Form.Item label={t('link.action')} tooltip={t('link.actionHelp')}>
            <Select
              value={draft.Action}
              onChange={(value) => update('Action', value)}
              options={Object.entries(actionLabels).map(([value, label]) => ({ value, label }))}
            />
          </Form.Item>

          <Form.Item label="vKey" tooltip={t('link.vkeyHelp')}>
            <Select
              value={draft.VKeyID || undefined}
              onChange={(value) => update('VKeyID', value || '')}
              options={deviceOptionToSelect(vkeyOptions, t('link.anyVkey'))}
              placeholder={t('link.anyVkey')}
              allowClear
              showSearch
            />
          </Form.Item>

          <Form.Item label={t('link.user')} tooltip={t('link.userHelp')}>
            <Select
              value={draft.ClientID || undefined}
              onChange={(value) => updateClient(value || '')}
              options={deviceOptionToSelect(clientOptions, t('link.anyUser'))}
              placeholder={t('link.anyUser')}
              allowClear
              showSearch
            />
          </Form.Item>

          <Form.Item label={t('link.listener')} tooltip={t('link.listenerHelp')}>
            <Select
              value={draft.ListenerID || undefined}
              onChange={(value) => update('ListenerID', value || '')}
              options={deviceOptionToSelect(listenerOptions, t('link.anyListener'))}
              placeholder={t('link.anyListener')}
              allowClear
              showSearch
            />
          </Form.Item>

          <Form.Item label="ID" tooltip={t('link.idHelp')}>
            <Input value={draft.ID} placeholder="route-1" onChange={(event) => update('ID', event.target.value)} allowClear />
          </Form.Item>

          <Form.Item label={t('link.connector')} tooltip={t('link.connectorHelp')}>
            <Select
              value={draft.ConnectorID || undefined}
              onChange={(value) => update('ConnectorID', value || '')}
              options={deviceOptionToSelect(connectorOptions, t('link.defaultConnector'))}
              placeholder={t('link.defaultConnector')}
              allowClear
              showSearch
            />
          </Form.Item>

          <Form.Item label={t('link.tunTapDevice')} tooltip={t('link.deviceHelp')}>
            <Select
              value={draft.DeviceID || undefined}
              onChange={(value) => update('DeviceID', value || '')}
              options={deviceOptionToSelect(deviceOptions, t('link.noForcedDevice'))}
              placeholder={t('link.noForcedDevice')}
              allowClear
              showSearch
            />
          </Form.Item>

          <Form.Item label={t('link.allowedIps')} tooltip={`${t('link.allowedIpsHelp')} ${t('link.sourceLimitHelp')}`}>
            <Input.TextArea
              value={allowedIPsText(draft)}
              onChange={(event) => updateAllowedIPs(event.target.value)}
              rows={2}
              placeholder="10.10.0.2/32, 10.10.0.0/24, fd00::2/128"
              allowClear
            />
          </Form.Item>

          {isTun ? (
            <Alert type="warning" showIcon className="hint-alert" title={t('link.tunMacIgnored')} />
          ) : (
            <Form.Item label={t('link.allowedMacs')} tooltip={t('link.allowedMacsHelp')}>
              <Input.TextArea
                value={draft.MACs}
                onChange={(event) => update('MACs', event.target.value)}
                rows={2}
                placeholder="02:00:00:00:00:01"
                allowClear
              />
            </Form.Item>
          )}
        </Form>
      ) : null}
    </Modal>
  );
}

function allowedIPsText(draft: RouteDraft): string {
  return [draft.IPv4CIDRs, draft.IPv6CIDRs].filter(Boolean).join('\n');
}

function deviceOptionToSelect(options: Array<{ id: string; label: string; detail?: string }>, emptyLabel: string) {
  return [
    { value: '', label: emptyLabel },
    ...options.map((item) => ({
      value: item.id,
      label: item.detail ? `${item.label}  ${item.detail}` : item.label,
    })),
  ];
}

function renderTextCell(value?: string) {
  if (!value) return emptyDash();
  return (
    <Tooltip title={value}>
      <span className="criterion-chip-value">{value}</span>
    </Tooltip>
  );
}

function buildRuleRows(routes: RouteRecord[], idx: ReturnType<typeof buildIndex>): RuleRow[] {
  return routes.map((route, index) => {
    const address = routeAddress(route, idx);
    // The rule table represents fields owned by the rule. Effective inherited
    // relationships belong in Link Test, otherwise an unset DeviceID looks saved.
    const device = route.DeviceID ? idx.devices.get(route.DeviceID) : undefined;
    const guard = sourceGuardForDevice(address, device);
    return {
      key: route.ID || String(index),
      index,
      route,
      enabled: route.Enabled !== false,
      priority: route.Priority ?? 100,
      action: route.Action || 'bind-device',
      id: route.ID,
      vkey: route.VKeyID ? labelVKey(idx.vkeys.get(route.VKeyID)) : '',
      user: route.ClientID ? labelClient(idx.clients.get(route.ClientID)) : '',
      listener: route.ListenerID ? labelEndpoint(idx.listeners.get(route.ListenerID)) : '',
      connector: route.ConnectorID ? labelEndpoint(idx.connectors.get(route.ConnectorID)) : '',
      device: device ? labelDevice(device) : '',
      allowedIPs: guard.ips,
      allowedMACs: guard.macs,
    };
  });
}

function mergeDraftIntoConfig(config: RuntimeConfig, draft: RouteDraft): RuntimeConfig {
  const ids = new Set((config.Addresses || [])
    .filter((item) => nodeIDOf(item) === draft.ManagedNodeID)
    .map((item) => item.ID));
  const ipv4 = splitList(draft.IPv4CIDRs);
  const ipv6 = splitList(draft.IPv6CIDRs);
  const macs = splitList(draft.MACs);
  const hasAddressLimit = ipv4.length > 0 || ipv6.length > 0 || macs.length > 0;
  const addressId = hasAddressLimit ? (draft.AddressID || uniqueAddressID(`addr-${draft.ID}`, ids)) : '';

  const route: RouteRecord = {
    ID: draft.ID.trim(),
    Enabled: draft.Enabled,
    Priority: draft.Priority,
    Action: draft.Action,
    VKeyID: draft.VKeyID || '',
    ListenerID: draft.ListenerID || '',
    DeviceID: draft.DeviceID || '',
    ConnectorID: draft.ConnectorID || '',
    ClientID: draft.ClientID || '',
    AddressID: addressId,
    ManagedNodeID: draft.ManagedNodeID,
  };

  const routes = [...((config.Routes || []) as RouteRecord[])];
  const routeIndex = routes.findIndex((item) => item.ID === (draft.OriginalID || route.ID)
    && nodeIDOf(item) === draft.ManagedNodeID);
  if (routeIndex >= 0) routes[routeIndex] = route;
  else routes.push(route);

  let addresses = [...(config.Addresses || [])];
  if (addressId && (ipv4.length > 0 || ipv6.length > 0 || macs.length > 0)) {
    const address: TapxAddressLimit & NodeOwned = {
      ID: addressId,
      Enabled: true,
      Name: draft.AddressName || `${draft.ID}-source-limit`,
      DeviceID: draft.DeviceID,
      ClientID: draft.ClientID,
      IPv4CIDRs: ipv4,
      IPv6CIDRs: ipv6,
      MACs: macs,
      Remark: managedLinkAddressRemark,
      ManagedNodeID: draft.ManagedNodeID,
    };
    const addressIndex = addresses.findIndex((item) => item.ID === addressId && nodeIDOf(item) === draft.ManagedNodeID);
    if (addressIndex >= 0) addresses[addressIndex] = { ...addresses[addressIndex], ...address };
    else addresses.push(address);
  } else if (draft.AddressID) {
    addresses = addresses.filter((item) => item.ID !== draft.AddressID
      || nodeIDOf(item) !== draft.ManagedNodeID
      || !isManagedLinkAddress(item));
  }

  return { ...config, Routes: routes, Addresses: addresses };
}

function removeRouteFromConfig(config: RuntimeConfig, route: RouteRecord): RuntimeConfig {
  const routes = ((config.Routes || []) as RouteRecord[]).filter((item) => !sameNodeObject(item, route));
  const addresses = (config.Addresses || []).filter((item) => (
    item.ID !== route.AddressID || nodeIDOf(item) !== nodeIDOf(route) || !isManagedLinkAddress(item)
  ));
  return { ...config, Routes: routes, Addresses: addresses };
}

function uniqueAddressID(base: string, existing: Set<string>): string {
  if (!existing.has(base)) return base;
  return nextId(base, existing);
}

function isManagedLinkAddress(address: TapxAddressLimit): boolean {
  return isManagedLinkAddressRemark(address.Remark);
}
