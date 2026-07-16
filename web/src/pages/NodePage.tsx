import { useEffect, useMemo, useState } from 'react';
import {
  Badge,
  Button,
  Card,
  Col,
  Dropdown,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  message,
  type MenuProps,
  type TableColumnsType,
} from 'antd';
import {
  CheckCircleOutlined,
  CloudDownloadOutlined,
  CloseCircleOutlined,
  ClusterOutlined,
  DeleteOutlined,
  EditOutlined,
  EyeInvisibleOutlined,
  EyeOutlined,
  MenuOutlined,
  PlusOutlined,
  SafetyCertificateOutlined,
  StopOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import {
  getManagedNodeMTLS,
  loadManagedNodes,
  readManagedNodes,
  removeManagedNode,
  saveManagedNode,
  saveManagedNodeMTLS,
  testManagedNode,
  testManagedNodeDraft,
  updateManagedNode,
  type ManagedNode,
  type ManagedNodeMTLS,
  type TLSVerifyMode,
} from '../features/nodes/nodeRegistry';
import { useI18n } from '../i18n/I18nProvider';
import './NodePage.css';

interface NodeFormValue extends Omit<ManagedNode, 'ID' | 'Status'> {
  ID?: string;
}

type MTLSFormValue = ManagedNodeMTLS;

const emptyNode: NodeFormValue = {
  Enabled: true,
  Name: '',
  Remark: '',
  Protocol: 'https',
  Host: '',
  Port: 2053,
  BasePath: '/',
  AllowPrivateAddress: false,
  TLSVerify: 'system',
  CertificateSHA256: '',
  APIToken: '',
  CPU: undefined,
  Memory: undefined,
  PanelVersion: undefined,
  TapXVersion: undefined,
  EmbeddedXrayVersion: undefined,
  ExternalXrayVersion: undefined,
  Uptime: undefined,
  Latency: undefined,
  LastSeen: undefined,
  ObjectCounts: undefined,
};

export function NodePage() {
  const { language, t } = useI18n();
  const [nodes, setNodes] = useState<ManagedNode[]>(readManagedNodes);
  const [loading, setLoading] = useState(true);
  const [selectedIDs, setSelectedIDs] = useState<string[]>([]);
  const [editing, setEditing] = useState<ManagedNode | null>(null);
  const [open, setOpen] = useState(false);
  const [showAddress, setShowAddress] = useState(false);
  const [testingID, setTestingID] = useState('');
  const [batchAction, setBatchAction] = useState('');
  const [testingForm, setTestingForm] = useState(false);
  const [mtlsOpen, setMTLSOpen] = useState(false);
  const [tlsVerify, setTLSVerify] = useState<TLSVerifyMode>('system');
  const [form] = Form.useForm<NodeFormValue>();
  const [mtlsForm] = Form.useForm<MTLSFormValue>();
  const [messageApi, messageContextHolder] = message.useMessage();

  const online = nodes.filter((node) => node.Enabled && node.Status === 'online');
  const offline = nodes.filter((node) => node.Enabled && node.Status === 'offline');
  const averageLatency = online.length > 0
    ? Math.round(online.reduce((sum, node) => sum + (node.Latency || 0), 0) / online.length)
    : undefined;
  const selectedNodes = useMemo(
    () => nodes.filter((node) => selectedIDs.includes(node.ID)),
    [nodes, selectedIDs],
  );

  useEffect(() => {
    setSelectedIDs((current) => current.filter((id) => nodes.some((node) => node.ID === id)));
  }, [nodes]);

  useEffect(() => {
    void refreshNodes();
  }, []);

  async function refreshNodes() {
    try {
      setNodes(await loadManagedNodes());
    } catch (error) {
      messageApi.error(errorText(error));
    } finally {
      setLoading(false);
    }
  }

  function openCreate() {
    setEditing(null);
    setTLSVerify('system');
    form.resetFields();
    form.setFieldsValue(emptyNode);
    setOpen(true);
  }

  function openEdit(node: ManagedNode) {
    setEditing(node);
    setTLSVerify(node.TLSVerify);
    form.resetFields();
    form.setFieldsValue({ ...node });
    setOpen(true);
  }

  async function saveNode() {
    const value = await form.validateFields();
    const normalizedPath = normalizeBasePath(value.BasePath);
    const nextNode: ManagedNode = {
      ...editing,
      ...value,
      ID: editing?.ID || makeNodeID(value.Name),
      Name: value.Name.trim(),
      Remark: value.Remark?.trim(),
      Host: value.Host.trim(),
      Port: value.Port || (value.Protocol === 'https' ? 443 : 80),
      BasePath: normalizedPath,
      APIToken: value.APIToken?.trim() || '',
      CertificateSHA256: value.TLSVerify === 'pin' ? value.CertificateSHA256?.trim() : undefined,
      Status: editing?.Status || 'offline',
    };
    try {
      await saveManagedNode(nextNode);
      setNodes(readManagedNodes());
      setOpen(false);
      messageApi.success(editing ? t('node.updated') : t('node.created'));
    } catch (error) {
      messageApi.error(errorText(error));
    }
  }

  async function removeNode(node: ManagedNode) {
    try {
      await removeManagedNode(node.ID);
      setNodes(readManagedNodes());
      messageApi.success(t('node.deleted'));
    } catch (error) {
      messageApi.error(errorText(error));
    }
  }

  async function toggleNode(node: ManagedNode, enabled: boolean) {
    try {
      await saveManagedNode({ ...node, Enabled: enabled });
      setNodes(readManagedNodes());
    } catch (error) {
      messageApi.error(errorText(error));
    }
  }

  async function testNode(node: ManagedNode) {
    setTestingID(node.ID);
    try {
      await testManagedNode(node.ID);
      setNodes(readManagedNodes());
      messageApi.success(t('node.testSuccess'));
    } catch (error) {
      await refreshNodes();
      messageApi.error(errorText(error));
    } finally {
      setTestingID('');
    }
  }

  async function testSelectedNodes() {
    if (selectedIDs.length === 0) return;
    const eligible = selectedNodes.filter((node) => node.Enabled);
    if (eligible.length === 0) {
      messageApi.warning(t('node.noTestEligible'));
      return;
    }
    setTestingID('batch');
    try {
      const results = await Promise.allSettled(eligible.map((node) => testManagedNode(node.ID)));
      await refreshNodes();
      const completed = results.filter((result) => result.status === 'fulfilled').length;
      if (completed !== results.length) messageApi.warning(t('node.batchResult', { completed, total: results.length }));
      else messageApi.success(t('node.batchTestCompleted', { count: eligible.length }));
    } finally {
      setTestingID('');
    }
  }

  async function setSelectedNodesEnabled(enabled: boolean) {
    if (selectedNodes.length === 0) return;
    setBatchAction(enabled ? 'enable' : 'disable');
    try {
      const results = await Promise.allSettled(selectedNodes.map((node) => saveManagedNode({ ...node, Enabled: enabled })));
      await refreshNodes();
      const completedIDs = selectedNodes.flatMap((node, index) => results[index]?.status === 'fulfilled' ? [node.ID] : []);
      setSelectedIDs((current) => current.filter((id) => !completedIDs.includes(id)));
      if (completedIDs.length !== results.length) messageApi.warning(t('node.batchResult', { completed: completedIDs.length, total: results.length }));
      else messageApi.success(enabled ? t('node.batchEnabled') : t('node.batchDisabled'));
    } finally {
      setBatchAction('');
    }
  }

  async function updateSelectedNodes() {
    const eligible = nodes.filter((node) => selectedIDs.includes(node.ID) && node.Enabled && node.Status === 'online');
    if (eligible.length === 0) {
      messageApi.warning(t('node.noUpdateEligible'));
      return;
    }
    setBatchAction('update');
    try {
      const results = await Promise.allSettled(eligible.map((node) => updateManagedNode(node.ID)));
      const completedIDs = eligible.flatMap((node, index) => results[index]?.status === 'fulfilled' ? [node.ID] : []);
      if (completedIDs.length !== results.length) messageApi.warning(t('node.batchResult', { completed: completedIDs.length, total: results.length }));
      else messageApi.success(t('node.updateSubmitted', { count: eligible.length }));
      setSelectedIDs((current) => current.filter((id) => !completedIDs.includes(id)));
    } finally {
      setBatchAction('');
    }
  }

  function deleteSelectedNodes() {
    Modal.confirm({
      title: t('node.batchDeleteConfirm', { count: selectedIDs.length }),
      okText: t('common.delete'),
      okType: 'danger',
      cancelText: t('common.cancel'),
      onOk: async () => {
        setBatchAction('delete');
        try {
          const deleting = [...selectedIDs];
          const results = await Promise.allSettled(deleting.map(removeManagedNode));
          const completedIDs = deleting.filter((_id, index) => results[index]?.status === 'fulfilled');
          setNodes(readManagedNodes());
          setSelectedIDs((current) => current.filter((id) => !completedIDs.includes(id)));
          if (completedIDs.length !== results.length) messageApi.warning(t('node.batchResult', { completed: completedIDs.length, total: results.length }));
          else messageApi.success(t('node.deleted'));
        } finally {
          setBatchAction('');
        }
      },
    });
  }

  async function testDraft() {
    const value = await form.validateFields();
    setTestingForm(true);
    try {
      await testManagedNodeDraft({ ...emptyNode, ...editing, ...value, ID: editing?.ID || makeNodeID(value.Name), Status: 'offline' });
      messageApi.success(t('node.testSuccess'));
    } catch (error) {
      messageApi.error(errorText(error));
    } finally {
      setTestingForm(false);
    }
  }

  async function openMTLS() {
    try {
      mtlsForm.setFieldsValue(await getManagedNodeMTLS());
      setMTLSOpen(true);
    } catch (error) {
      messageApi.error(errorText(error));
    }
  }

  async function saveMTLS() {
    const value = await mtlsForm.validateFields();
    try {
      await saveManagedNodeMTLS(value);
      setMTLSOpen(false);
      messageApi.success(t('node.mtlsSaved'));
    } catch (error) {
      messageApi.error(errorText(error));
    }
  }

  const columns = useMemo<TableColumnsType<ManagedNode>>(() => [
    {
      title: t('node.actions'),
      key: 'actions',
      width: 128,
      fixed: 'left',
      align: 'center',
      render: (_, node) => (
        <Space size={2}>
          <Tooltip title={t('node.testConnection')}>
            <Button type="text" shape="circle" size="small" icon={<ThunderboltOutlined />} loading={testingID === node.ID} onClick={() => void testNode(node)} />
          </Tooltip>
          <Tooltip title={t('node.edit')}>
            <Button type="text" shape="circle" size="small" icon={<EditOutlined />} onClick={() => openEdit(node)} />
          </Tooltip>
          <Tooltip title={t('common.delete')}>
            <Popconfirm title={t('node.deleteConfirm', { name: node.Name })} okText={t('common.delete')} cancelText={t('common.cancel')} onConfirm={() => removeNode(node)}>
              <Button type="text" shape="circle" size="small" danger icon={<DeleteOutlined />} aria-label={t('common.delete')} />
            </Popconfirm>
          </Tooltip>
        </Space>
      ),
    },
    {
      title: t('common.enabled'),
      key: 'Enabled',
      width: 78,
      fixed: 'left',
      align: 'center',
      render: (_, node) => <Switch size="small" checked={node.Enabled} onChange={(checked) => toggleNode(node, checked)} />,
    },
    {
      title: t('node.name'),
      key: 'Name',
      width: 164,
      fixed: 'left',
      render: (_, node) => (
        <div className="node-name-cell">
          <strong>{node.Name}</strong>
          <span>{node.Remark || '-'}</span>
        </div>
      ),
    },
    {
      title: (
        <Space size={5}>
          <span>{t('node.address')}</span>
          <Button
            type="text"
            size="small"
            className="node-address-toggle"
            icon={showAddress ? <EyeOutlined /> : <EyeInvisibleOutlined />}
            aria-label={showAddress ? t('node.hideAddress') : t('node.showAddress')}
            onClick={() => setShowAddress((current) => !current)}
          />
        </Space>
      ),
      key: 'Address',
      width: 250,
      render: (_, node) => showAddress
        ? <span className="node-address">{nodeURL(node)}</span>
        : <span className="node-address is-hidden">{maskedAddress(node.Host)}</span>,
    },
    {
      title: t('node.status'),
      key: 'Status',
      width: 110,
      align: 'center',
      render: (_, node) => statusBadge(node, t),
    },
    { title: 'CPU', dataIndex: 'CPU', width: 80, align: 'center', render: (value?: number) => percentText(value) },
    { title: t('node.memory'), dataIndex: 'Memory', width: 82, align: 'center', render: (value?: number) => percentText(value) },
    { title: t('node.panelVersion'), dataIndex: 'PanelVersion', width: 112, align: 'center', render: versionText },
    { title: t('node.tapxVersion'), dataIndex: 'TapXVersion', width: 105, align: 'center', render: versionText },
    { title: t('node.embeddedXrayVersion'), dataIndex: 'EmbeddedXrayVersion', width: 126, align: 'center', render: versionText },
    { title: t('node.externalXrayVersion'), dataIndex: 'ExternalXrayVersion', width: 126, align: 'center', render: versionText },
    { title: t('node.uptime'), dataIndex: 'Uptime', width: 96, align: 'center', render: (value?: string) => value || '-' },
    {
      title: t('node.objects'),
      key: 'Objects',
      width: 92,
      align: 'center',
      render: (_, node) => node.ObjectCounts
        ? <Tag>{Object.values(node.ObjectCounts).reduce((sum, value) => sum + value, 0)}</Tag>
        : '-',
    },
    { title: t('node.latency'), dataIndex: 'Latency', width: 82, align: 'center', render: (value?: number) => value == null ? '-' : `${value} ms` },
    { title: t('node.lastHeartbeat'), dataIndex: 'LastSeen', width: 112, align: 'center', render: (value?: string) => relativeTime(value, language, t) },
  ], [language, nodes, showAddress, t, testingID]);

  const batchItems: MenuProps['items'] = [
    {
      key: 'test',
      icon: <ThunderboltOutlined />,
      label: t('node.testSelected'),
      disabled: !selectedNodes.some((node) => node.Enabled),
    },
    {
      key: 'update',
      icon: <CloudDownloadOutlined />,
      label: t('node.updateSelected'),
      disabled: !selectedNodes.some((node) => node.Enabled && node.Status === 'online'),
    },
    { type: 'divider' },
    { key: 'enable', icon: <CheckCircleOutlined />, label: t('node.enableSelected') },
    { key: 'disable', icon: <StopOutlined />, label: t('node.disableSelected') },
    { type: 'divider' },
    { key: 'delete', icon: <DeleteOutlined />, label: t('node.deleteSelected'), danger: true },
  ];

  const onBatchClick: MenuProps['onClick'] = ({ key }) => {
    if (key === 'test') void testSelectedNodes();
    if (key === 'update') void updateSelectedNodes();
    if (key === 'enable') void setSelectedNodesEnabled(true);
    if (key === 'disable') void setSelectedNodesEnabled(false);
    if (key === 'delete') deleteSelectedNodes();
  };

  return (
    <div className="node-page">
      {messageContextHolder}
      <Card hoverable className="node-summary-card">
        <div className="node-summary-grid">
          <SummaryMetric icon={<ClusterOutlined />} label={t('node.total')} value={nodes.length} />
          <SummaryMetric icon={<CheckCircleOutlined />} tone="success" label={t('node.online')} value={online.length} />
          <SummaryMetric icon={<CloseCircleOutlined />} tone="danger" label={t('node.offline')} value={offline.length} />
          <SummaryMetric icon={<ThunderboltOutlined />} label={t('node.averageLatency')} value={averageLatency == null ? '-' : `${averageLatency} ms`} />
        </div>
      </Card>

      <Card hoverable className="node-table-card">
        <div className="node-toolbar">
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>{t('node.add')}</Button>
          <Button icon={<SafetyCertificateOutlined />} onClick={openMTLS}>{t('node.mtls')}</Button>
          {selectedIDs.length > 0 ? <>
            <Tag color="blue" closable onClose={() => setSelectedIDs([])}>{t('node.selectedCount', { count: selectedIDs.length })}</Tag>
            <Dropdown
              trigger={['click']}
              disabled={Boolean(batchAction) || testingID === 'batch'}
              menu={{ items: batchItems, onClick: onBatchClick }}
            >
              <Button icon={<MenuOutlined />} loading={Boolean(batchAction) || testingID === 'batch'}>
                {t('node.batchActions')}
              </Button>
            </Dropdown>
          </> : null}
        </div>
        <Table
          rowKey="ID"
          rowSelection={{ selectedRowKeys: selectedIDs, onChange: (keys) => setSelectedIDs(keys.map(String)) }}
          columns={columns}
          dataSource={nodes}
          loading={loading}
          size="small"
          pagination={false}
          scroll={{ x: 1870 }}
          locale={{ emptyText: t('node.empty') }}
          expandable={{
            expandedRowRender: (node) => <NodeDetails node={node} />,
            rowExpandable: (node) => Boolean(node.ObjectCounts),
            columnWidth: 40,
          }}
        />
      </Card>

      <Modal
        open={open}
        title={editing ? t('node.editTitle') : t('node.addTitle')}
        width={720}
        okText={t('common.save')}
        cancelText={t('common.cancel')}
        forceRender
        onOk={() => void saveNode()}
        onCancel={() => setOpen(false)}
      >
        <Form form={form} layout="vertical" className="node-form" onValuesChange={(changed) => {
          if ('TLSVerify' in changed) setTLSVerify(changed.TLSVerify as TLSVerifyMode);
        }}>
          <Row gutter={16}>
            <Col xs={24} md={12}>
              <Form.Item name="Name" label={t('node.name')} rules={[{ required: true, message: t('node.nameRequired') }]}>
                <Input placeholder="hongkong-edge" />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="Remark" label={t('node.remark')}>
                <Input placeholder={t('node.remarkPlaceholder')} />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} md={6}>
              <Form.Item name="Protocol" label={t('node.protocol')} rules={[{ required: true }]}>
                <Select options={[{ value: 'https', label: 'HTTPS' }, { value: 'http', label: 'HTTP' }]} />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item name="Host" label={t('node.host')} tooltip={t('node.hostHelp')} rules={[{ required: true, message: t('node.hostRequired') }]}>
                <Input placeholder="panel.example.com / 203.0.113.10" />
              </Form.Item>
            </Col>
            <Col xs={24} md={6}>
              <Form.Item name="Port" label={t('node.port')} rules={[{ required: true }]}>
                <InputNumber min={1} max={65535} style={{ width: '100%' }} placeholder="2053" />
              </Form.Item>
            </Col>
          </Row>
          <Row gutter={16} align="middle">
            <Col xs={24} md={12}>
              <Form.Item name="BasePath" label={t('node.basePath')} tooltip={t('node.basePathHelp')} rules={[{ required: true }]}>
                <Input placeholder="/tapx/" />
              </Form.Item>
            </Col>
            <Col xs={12} md={12}>
              <Form.Item name="Enabled" label={t('common.enabled')} valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
            <Col xs={24}>
              <Form.Item name="AllowPrivateAddress" label={t('node.allowPrivate')} tooltip={t('node.allowPrivateHelp')} valuePropName="checked">
                <Switch />
              </Form.Item>
            </Col>
          </Row>
          <Form.Item name="TLSVerify" label={t('node.tlsVerify')} tooltip={t('node.tlsVerifyHelp')}>
            <Select options={[
              { value: 'system', label: t('node.tlsSystem') },
              { value: 'pin', label: t('node.tlsPin') },
              { value: 'skip', label: t('node.tlsSkip') },
            ]} />
          </Form.Item>
          {tlsVerify === 'pin' ? (
            <Form.Item name="CertificateSHA256" label={t('node.certFingerprint')} tooltip={t('node.certFingerprintHelp')} rules={[{ required: true, message: t('node.certFingerprintRequired') }]}>
              <Input placeholder="sha256:7f:3a:..." />
            </Form.Item>
          ) : null}
          <Form.Item name="APIToken" label={t('node.apiToken')} tooltip={t('node.apiTokenHelp')} rules={[{ required: !editing?.APITokenConfigured, message: t('node.apiTokenRequired') }]}>
            <Input.Password placeholder="tapx_********************************" autoComplete="new-password" />
          </Form.Item>
          <Button block icon={<ThunderboltOutlined />} loading={testingForm} onClick={() => void testDraft()}>{t('node.testConnection')}</Button>
        </Form>
      </Modal>

      <Modal
        open={mtlsOpen}
        title={t('node.mtlsTitle')}
        width={620}
        okText={t('common.save')}
        cancelText={t('common.cancel')}
        forceRender
        onOk={() => void saveMTLS()}
        onCancel={() => setMTLSOpen(false)}
      >
        <Form form={mtlsForm} layout="vertical">
          <Form.Item name="Enabled" label={t('node.enableMTLS')} tooltip={t('node.enableMTLSHelp')} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item noStyle shouldUpdate={(previous, current) => previous.Enabled !== current.Enabled}>
            {({ getFieldValue }) => getFieldValue('Enabled') ? (
              <>
                <Form.Item name="CertificateFile" label={t('node.clientCertificate')} rules={[{ required: true }]}>
                  <Input placeholder="/etc/tapx/mtls/client.crt" />
                </Form.Item>
                <Form.Item name="PrivateKeyFile" label={t('node.clientPrivateKey')} rules={[{ required: true }]}>
                  <Input.Password placeholder="/etc/tapx/mtls/client.key" visibilityToggle={false} />
                </Form.Item>
                <Form.Item name="CAFile" label={t('node.nodeCA')} tooltip={t('node.nodeCAHelp')} rules={[{ required: true }]}>
                  <Input placeholder="/etc/tapx/mtls/ca.crt" />
                </Form.Item>
              </>
            ) : null}
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}

function SummaryMetric({ icon, label, value, tone }: { icon: React.ReactNode; label: string; value: React.ReactNode; tone?: 'success' | 'danger' }) {
  return (
    <div className={`node-summary-item${tone ? ` is-${tone}` : ''}`}>
      <span>{label}</span>
      <strong>{icon}{value}</strong>
    </div>
  );
}

function NodeDetails({ node }: { node: ManagedNode }) {
  const { t } = useI18n();
  const counts = node.ObjectCounts;
  if (!counts) return null;
  return (
    <div className="node-details-grid">
      <DetailItem label={t('menu.devices')} value={counts.devices} />
      <DetailItem label={t('menu.listeners')} value={counts.listeners} />
      <DetailItem label={t('menu.users')} value={counts.users} />
      <DetailItem label={t('menu.connectors')} value={counts.connectors} />
      <DetailItem label={t('menu.links')} value={counts.links} />
    </div>
  );
}

function DetailItem({ label, value }: { label: string; value: number }) {
  return <div><span>{label}</span><strong>{value}</strong></div>;
}

function statusBadge(node: ManagedNode, t: ReturnType<typeof useI18n>['t']) {
  if (!node.Enabled) return <Badge status="default" text={t('common.disabled')} />;
  if (node.Status === 'checking') return <Badge status="processing" text={t('node.checking')} />;
  return node.Status === 'online'
    ? <Badge status="success" text={t('node.online')} />
    : <Badge status="error" text={t('node.offline')} />;
}

function makeNodeID(name: string): string {
  const value = name.trim().toLowerCase().replace(/[^a-z0-9_.-]+/g, '-').replace(/^-+|-+$/g, '');
  return `node-${value || Date.now()}`;
}

function normalizeBasePath(value?: string): string {
  const path = String(value || '/').trim();
  return `/${path.replace(/^\/+|\/+$/g, '')}${path === '/' ? '' : '/'}`;
}

function nodeURL(node: ManagedNode): string {
  const defaultPort = node.Protocol === 'https' ? 443 : 80;
  const port = node.Port === defaultPort ? '' : `:${node.Port}`;
  const host = node.Host.includes(':') && !node.Host.startsWith('[') ? `[${node.Host}]` : node.Host;
  return `${node.Protocol}://${host}${port}${normalizeBasePath(node.BasePath)}`;
}

function maskedAddress(host: string): string {
  if (!host) return '-';
  if (host.length <= 8) return '********';
  return `${host.slice(0, 3)}******${host.slice(-3)}`;
}

function percentText(value?: number) {
  return value == null ? '-' : `${value.toFixed(1)}%`;
}

function versionText(value?: string) {
  return value ? <span className="node-version">{value}</span> : '-';
}

function relativeTime(value: string | undefined, language: string, t: ReturnType<typeof useI18n>['t']): string {
  if (!value) return '-';
  const elapsed = Math.max(0, Date.now() - new Date(value).getTime());
  if (elapsed < 60_000) return t('node.justNow');
  const minutes = Math.floor(elapsed / 60_000);
  if (minutes < 60) return language === 'zh-CN' ? `${minutes} 分钟前` : `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  return language === 'zh-CN' ? `${hours} 小时前` : `${hours}h ago`;
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
