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
  Popconfirm,
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
  MenuOutlined,
  PlusOutlined,
  QuestionCircleOutlined,
  StopOutlined,
} from '@ant-design/icons';
import {
  getSystemInterfaces,
  type RuntimeConfig,
  type TapxDevice,
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
import './DevicePage.css';

type DeviceRecord = TapxDevice & NodeOwned & {
  DNSList?: string[];
  DNSSearchList?: string[];
  DNSOptions?: string[];
  DNSOutputPath?: string;
  BridgeMTU?: number;
};

const defaultDevice: DeviceRecord = {
  ID: '',
  Enabled: true,
  Name: '',
  Type: 'tun',
  IfName: '',
  MTU: 1500,
  AddressConfigEnabled: false,
  LinkAutoOptimize: false,
  AddressAssignMode: 'manual',
  DNSList: [],
  DNSSearchList: [],
  DNSOptions: [],
  Routes: [],
  BridgeEnabled: false,
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
  const [interfaces, setInterfaces] = useState<string[]>([]);
  const [form] = Form.useForm<DeviceRecord>();
  const [messageApi, messageContextHolder] = message.useMessage();
  const { nodes, scope, setScope } = useNodeScope();
  const nodeTargetOptions = useNodeTargetOptions(nodes);

  const devices = useMemo(() => ((config.Devices || []) as DeviceRecord[]), [config.Devices]);
  const visibleDevices = useMemo(() => filterNodeOwned(devices, scope), [devices, scope]);
  const selectedDevices = useMemo(
    () => devices.filter((item) => selectedRowKeys.includes(nodeObjectKey(item))),
    [devices, selectedRowKeys],
  );
  useEffect(() => {
    const visibleKeys = new Set(visibleDevices.map(nodeObjectKey));
    setSelectedRowKeys((current) => current.filter((key) => visibleKeys.has(key)));
  }, [visibleDevices]);
  const deviceType = Form.useWatch('Type', form) ?? 'tun';
  const addressConfigEnabled = Form.useWatch('AddressConfigEnabled', form) ?? false;
  const addressAssignMode = Form.useWatch('AddressAssignMode', form) ?? 'manual';
  const bridgeEnabled = Form.useWatch('BridgeEnabled', form) ?? false;
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
    const next = { ...defaultDevice, ID: makeDeviceId(name), Name: name, IfName: name, ManagedNodeID: defaultTargetNodeID(scope) };
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
      DNSList: record.DNS?.Nameservers || [],
      DNSSearchList: record.DNS?.SearchDomains || record.DNSSearch || [],
      DNSOptions: record.DNS?.Options || [],
      DNSOutputPath: record.DNS?.OutputPath || '',
      Routes: (record.Routes || []).map((route) => ({ ...route })),
      BridgeEnabled: record.Bridge?.Enabled === true || record.BridgeEnabled === true,
      BridgeName: record.Bridge?.Name || record.BridgeName,
      BridgeMember: record.Bridge?.IfName || record.BridgeMember,
      BridgeMTU: record.Bridge?.MTU || record.MTU || 1500,
    });
    setOpen(true);
  }, [form]);

  async function submit() {
    await form.validateFields();
    const values = form.getFieldsValue(true) as DeviceRecord;
    const id = values.ID || editing?.ID || makeDeviceId(values.Name || values.IfName || '');
    const ifName = (values.IfName || values.Name || id).trim();
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
    const nameservers = values.DNSList || [];
    const searchDomains = values.DNSSearchList || [];
    const dnsOptions = values.DNSOptions || [];
    const dnsOutputPath = String(values.DNSOutputPath || '').trim();
    const bridgeEnabledForTap = values.Type === 'tap' && values.BridgeEnabled === true;
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
      AddressConfigEnabled: values.AddressConfigEnabled === true,
      AddressAssignMode: values.AddressAssignMode === 'auto' ? 'auto' : 'manual',
      IPv4CIDR: values.AddressConfigEnabled === true && values.AddressAssignMode !== 'auto' ? values.IPv4CIDR : undefined,
      IPv6CIDR: values.AddressConfigEnabled === true && values.AddressAssignMode !== 'auto' ? values.IPv6CIDR : undefined,
      Gateway: values.AddressConfigEnabled === true && values.AddressAssignMode !== 'auto' ? values.Gateway : undefined,
      DNS: nameservers.length > 0 || searchDomains.length > 0 || dnsOptions.length > 0 || dnsOutputPath
        ? {
          ...editing?.DNS,
          Enabled: true,
          Nameservers: nameservers,
          SearchDomains: searchDomains,
          Options: dnsOptions,
          OutputPath: dnsOutputPath,
        }
        : undefined,
      DNSSearch: searchDomains,
      Routes: routes,
      Bridge: bridgeEnabledForTap
        ? { Enabled: true, Name: values.BridgeName || 'br-tapx', IfName: values.BridgeMember || '', MTU: Number(values.BridgeMTU) || Number(values.MTU) || 1500 }
        : undefined,
      BridgeEnabled: bridgeEnabledForTap,
      BridgeName: bridgeEnabledForTap ? values.BridgeName : undefined,
      BridgeMember: bridgeEnabledForTap ? values.BridgeMember : undefined,
      Source: values.Source || editing?.Source || 'manual',
      UpdatedAt: Date.now(),
    };
    const index = devices.findIndex((item) => sameNodeObject(item, next));
    const nextDevices = index < 0
      ? [...devices, next]
      : devices.map((item) => (sameNodeObject(item, next) ? next : item));

    setSaving(true);
    try {
      const saved = await saveRuntimeConfig({ ...config, Devices: nextDevices });
      setConfig(saved);
      setOpen(false);
      try {
        await applyRuntimeConfig();
        messageApi.success(t('device.saved'));
      } catch (applyError) {
        messageApi.warning(t('device.applyFailed', { error: applyError instanceof Error ? applyError.message : String(applyError) }));
      }
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('device.saveFailed'));
    } finally {
      setSaving(false);
    }
  }

  async function deleteDevice(record: DeviceRecord) {
    const sameNodeReference = (value: object, id: string | undefined) => id === record.ID && nodeIDOf(value) === nodeIDOf(record);
    const referenced = (config.Listeners || []).some((item) => sameNodeReference(item, item.Binding?.DeviceID))
      || (config.Connectors || []).some((item) => sameNodeReference(item, item.Binding?.DeviceID))
      || (config.Routes || []).some((item) => sameNodeReference(item, item.DeviceID))
      || (config.Addresses || []).some((item) => sameNodeReference(item, item.DeviceID));
    if (referenced) {
      messageApi.warning(t('device.referenced'));
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
    const sameNodeReference = (value: object, id: string | undefined) => id === record.ID && nodeIDOf(value) === nodeIDOf(record);
    return (config.Listeners || []).some((item) => sameNodeReference(item, item.Binding?.DeviceID))
      || (config.Connectors || []).some((item) => sameNodeReference(item, item.Binding?.DeviceID))
      || (config.Routes || []).some((item) => sameNodeReference(item, item.DeviceID))
      || (config.Addresses || []).some((item) => sameNodeReference(item, item.DeviceID));
  }

  async function setSelectedDevicesEnabled(enabled: boolean) {
    if (selectedDevices.length === 0) return;
    const selected = new Set(selectedDevices.map(nodeObjectKey));
    setSaving(true);
    try {
      const saved = await saveRuntimeConfig({
        ...config,
        Devices: devices.map((item) => selected.has(nodeObjectKey(item)) ? { ...item, Enabled: enabled } : item),
      });
      setConfig(saved);
      await applyRuntimeConfig();
      setSelectedRowKeys([]);
      messageApi.success(enabled ? t('device.batchEnabled') : t('device.batchDisabled'));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('device.saveFailed'));
    } finally {
      setSaving(false);
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

  const batchItems: MenuProps['items'] = [
    { key: 'enable', icon: <CheckCircleOutlined />, label: t('device.enableSelected') },
    { key: 'disable', icon: <StopOutlined />, label: t('device.disableSelected') },
    { type: 'divider' },
    { key: 'delete', icon: <DeleteOutlined />, label: t('device.deleteSelected'), danger: true },
  ];

  const onBatchClick: MenuProps['onClick'] = ({ key }) => {
    if (key === 'enable') void setSelectedDevicesEnabled(true);
    if (key === 'disable') void setSelectedDevicesEnabled(false);
    if (key === 'delete') confirmDeleteSelectedDevices();
  };

  const columns = useMemo<TableColumnsType<DeviceRecord>>(() => [
    {
      title: t('node.sourceNode'),
      key: 'ManagedNodeID',
      width: 150,
      render: (_, record) => <NodeSourceTag value={record} />,
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
      title: t('device.status'),
      dataIndex: 'Enabled',
      key: 'Enabled',
      width: 100,
      render: (enabled: boolean) => <Tag color={enabled !== false ? 'success' : 'default'}>{enabled !== false ? t('common.enabled') : t('common.disabled')}</Tag>,
    },
    { title: 'MTU', dataIndex: 'MTU', key: 'MTU', width: 90 },
    {
      title: 'IPv4',
      dataIndex: 'IPv4CIDR',
      key: 'IPv4CIDR',
      render: (value: string, record) => addressText(record, value, t),
    },
    {
      title: 'IPv6',
      dataIndex: 'IPv6CIDR',
      key: 'IPv6CIDR',
      render: (value: string, record) => addressText(record, value, t),
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
        const labels = linkedEndpointLabels(record);
        if (labels.length === 0) return '-';
        return (
          <Space wrap size={[4, 4]}>
            {labels.map((label) => <Tag key={label}>{label}</Tag>)}
          </Space>
        );
      },
    },
    {
      title: t('device.bridge'),
      key: 'Bridge',
      render: (_, record) => record.Type === 'tap' && record.BridgeEnabled
        ? `${record.BridgeName || '-'} / ${record.BridgeMember || '-'}`
        : '-',
    },
    {
      title: t('device.actions'),
      key: 'actions',
      align: 'right',
      width: 120,
      render: (_, record) => (
        <Space size={4}>
          <Button type="text" icon={<EditOutlined />} aria-label={t('device.edit')} onClick={() => openEdit(record)} />
          <Popconfirm title={t('device.deleteConfirm', { name: record.IfName || record.ID })} okText={t('common.delete')} cancelText={t('device.cancel')} onConfirm={() => void deleteDevice(record)}>
            <Button type="text" danger icon={<DeleteOutlined />} aria-label={t('common.delete')} />
          </Popconfirm>
        </Space>
      ),
    },
  ], [config.Addresses, config.Connectors, config.Listeners, config.Routes, devices, openEdit, t]);

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
          <Form.Item name="AddressConfigEnabled" label={t('device.configureAddress')} tooltip={t('device.configureAddressHelp')} valuePropName="checked">
            <Switch />
          </Form.Item>
          {addressConfigEnabled && (
            <>
              <Form.Item name="AddressAssignMode" label={t('device.addressMode')} tooltip={t('device.addressModeHelp')}>
                <Radio.Group buttonStyle="solid">
                  <Radio.Button value="auto">{t('device.auto')}</Radio.Button>
                  <Radio.Button value="manual">{t('device.manual')}</Radio.Button>
                </Radio.Group>
              </Form.Item>
              {addressAssignMode === 'manual' && (
                <>
                  <Form.Item name="IPv4CIDR" label={t('device.ipv4Cidr')}>
                    <Input placeholder="10.10.0.1/24" />
                  </Form.Item>
                  <Form.Item name="IPv6CIDR" label={t('device.ipv6Cidr')}>
                    <Input placeholder="fd00::1/64" />
                  </Form.Item>
                  <Form.Item name="Gateway" label={t('device.gateway')}>
                    <Input placeholder="10.10.0.1" />
                  </Form.Item>
                </>
              )}
            </>
          )}
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
                  <div className="device-route-default">
                    <span>{t('device.allowDefaultRoute')}</span>
                    <Tooltip title={t('device.allowDefaultRouteHelp')}><QuestionCircleOutlined className="device-route-help" /></Tooltip>
                    <Form.Item name="AllowDefaultRoute" valuePropName="checked" noStyle>
                      <Switch size="small" aria-label={t('device.allowDefaultRoute')} />
                    </Form.Item>
                  </div>
                </div>
              </div>
            )}
          </Form.List>
        </>
      ),
    },
    {
      key: 'dns',
      label: 'DNS',
      children: (
        <>
          <Form.Item name="DNSList" label="DNS" tooltip={t('device.dnsListHelp')}>
            <Select mode="tags" tokenSeparators={[',', '\n']} placeholder="1.1.1.1" />
          </Form.Item>
          <Form.Item name="DNSSearchList" label={t('device.searchDomains')} tooltip={t('device.searchDomainsHelp')}>
            <Select mode="tags" tokenSeparators={[',', '\n']} placeholder="lan" />
          </Form.Item>
          <Form.Item name="DNSOptions" label={t('device.dnsOptions')} tooltip={t('device.dnsOptionsHelp')}>
            <Select mode="tags" tokenSeparators={[',', '\n']} placeholder="timeout:2" />
          </Form.Item>
          <Form.Item name="DNSOutputPath" label={t('device.dnsOutputPath')} tooltip={t('device.dnsOutputPathHelp')}>
            <Input placeholder="/run/tapx/resolv/tapx-tun0.conf" />
          </Form.Item>
        </>
      ),
    },
    ...(deviceType === 'tap' ? [{
      key: 'bridge',
      label: t('device.bridgeSettings'),
      children: (
        <>
          <Form.Item name="BridgeEnabled" label={t('device.enableBridge')} tooltip={t('device.bridgeHelp')} valuePropName="checked">
            <Switch />
          </Form.Item>
          {bridgeEnabled ? (
            <>
              <Form.Item name="BridgeName" label={t('device.bridgeName')} tooltip={t('device.bridgeNameHelp')}>
                <Input placeholder="br-tapx" />
              </Form.Item>
              <Form.Item name="BridgeMember" label={t('device.bridgeMember')} tooltip={t('device.bridgeMemberHelp')}>
                <Select
                  allowClear
                  showSearch
                  options={interfaceOptions}
                  placeholder={t('device.selectInterface')}
                  filterOption={(input, option) => String(option?.value || '').toLowerCase().includes(input.toLowerCase())}
                  notFoundContent={t('device.noBridgeInterface')}
                />
              </Form.Item>
              <Form.Item name="BridgeMTU" label={t('device.bridgeMtu')} tooltip={t('device.bridgeMtuHelp')}>
                <InputNumber min={576} max={9000} />
              </Form.Item>
              <Form.Item label=" ">
                <Button onClick={() => void refreshInterfaces(targetNodeID)}>{t('device.refreshInterfaces')}</Button>
              </Form.Item>
            </>
          ) : null}
        </>
      ),
    }] : []),
  ];

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
              {selectedDevices.length > 0 ? (
                <>
                  <Tag color="blue" closable onClose={() => setSelectedRowKeys([])}>
                    {t('device.selectedCount', { count: selectedDevices.length })}
                  </Tag>
                  <Dropdown trigger={['click']} menu={{ items: batchItems, onClick: onBatchClick }}>
                    <Button icon={<MenuOutlined />}>{t('device.batchActions')}</Button>
                  </Dropdown>
                </>
              ) : null}
            </Space>
          </Card>
        </Col>
        <Col span={24}>
          <Card hoverable>
            <Table
              rowKey={nodeObjectKey}
              rowSelection={{ selectedRowKeys, onChange: (keys) => setSelectedRowKeys(keys.map(String)) }}
              columns={columns}
              dataSource={visibleDevices}
              loading={loading || saving}
              pagination={false}
              scroll={{ x: 1330 }}
              size="middle"
              locale={{ emptyText: t('device.empty') }}
            />
          </Card>
        </Col>
      </Row>

      <Modal
        open={open}
        title={editing ? t('device.editTitle') : t('device.addTitle')}
        okText={t('common.save')}
        cancelText={t('device.cancel')}
        width={760}
        forceRender
        confirmLoading={saving}
        onOk={submit}
        onCancel={() => setOpen(false)}
      >
        <Form form={form} layout="horizontal" labelCol={{ span: 7 }} wrapperCol={{ span: 17 }}>
          <Tabs items={modalTabs} />
        </Form>
      </Modal>
    </div>
  );
}

function normalizeInterfaceNames(input: unknown): string[] {
  const names = new Set<string>();
  const addName = (value: unknown) => {
    if (typeof value !== 'string') return;
    const trimmed = value.trim();
    if (trimmed) names.add(trimmed);
  };
  if (Array.isArray(input)) {
    for (const item of input) {
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

function addressText(record: DeviceRecord, value: string | undefined, t: ReturnType<typeof useI18n>['t']) {
  if (!record.AddressConfigEnabled) return '-';
  if (record.AddressAssignMode === 'auto') return t('device.auto');
  return value || '-';
}

function sourceTag(record: DeviceRecord, t: ReturnType<typeof useI18n>['t']) {
  const source = record.Source || 'manual';
  if (source === 'listener-auto') return <Tag color="blue">{t('device.sourceListener')}</Tag>;
  if (source === 'connector-auto') return <Tag color="purple">{t('device.sourceConnector')}</Tag>;
  return <Tag>{t('device.sourceManual')}</Tag>;
}

function linkedEndpointLabels(record: DeviceRecord): string[] {
  const labels = [
    ...(record.LinkedListenerNames ?? []),
    ...(record.LinkedListenerIDs ?? []).map((id) => `#${id}`),
    ...(record.LinkedConnectorNames ?? []),
    ...(record.LinkedConnectorIDs ?? []).map((id) => `#${id}`),
  ]
    .filter((value): value is string => typeof value === 'string' && value.trim().length > 0)
    .map((value) => value.trim());
  return Array.from(new Set(labels));
}
