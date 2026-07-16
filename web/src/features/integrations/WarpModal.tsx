import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Collapse,
  Divider,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Tag,
  message,
} from 'antd';
import { ApiOutlined, DeleteOutlined, PlusOutlined, SyncOutlined } from '@ant-design/icons';
import { useI18n } from '../../i18n/I18nProvider';
import { formatBytes } from '../../shared/format';
import { integrations, type WarpAccount, type WarpConfig } from '../../shared/integrations';
import { generateWireguardKeypair } from '../../shared/wireguard';
import type { WireguardIntegrationDraft } from './types';
import './IntegrationModal.css';

interface WarpModalProps {
  open: boolean;
  connectorExists: boolean;
  managedNodeID: string;
  nodeTargetOptions: Array<{ value: string; label: string; disabled?: boolean }>;
  onManagedNodeIDChange: (value: string) => void;
  onClose: () => void;
  onAddConnector: (draft: WireguardIntegrationDraft) => Promise<void> | void;
  onReplaceConnector: (draft: WireguardIntegrationDraft) => Promise<void> | void;
  onRemoveConnector: () => Promise<void> | void;
}

export function WarpModal({
  open,
  connectorExists,
  managedNodeID,
  nodeTargetOptions,
  onManagedNodeIDChange,
  onClose,
  onAddConnector,
  onReplaceConnector,
  onRemoveConnector,
}: WarpModalProps) {
  const { t } = useI18n();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [loading, setLoading] = useState(false);
  const [account, setAccount] = useState<WarpAccount | null>(null);
  const [config, setConfig] = useState<WarpConfig | null>(null);
  const [license, setLicense] = useState('');
  const [licenseError, setLicenseError] = useState('');
  const [intervalDays, setIntervalDays] = useState(0);

  const draft = useMemo(() => buildWarpDraft(account, config), [account, config]);

  const loadAccount = useCallback(async () => {
    setLoading(true);
    try {
      const next = await integrations.warp.data(managedNodeID);
      setAccount(next);
      setIntervalDays(next?.update_interval_days || 0);
    } catch (error) {
      messageApi.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, [managedNodeID, messageApi]);

  useEffect(() => {
    if (!open) return;
    setAccount(null);
    setConfig(null);
    setLicense('');
    setLicenseError('');
    void loadAccount();
  }, [open, loadAccount]);

  async function register() {
    setLoading(true);
    try {
      const pair = generateWireguardKeypair();
      const result = await integrations.warp.register(pair.privateKey, pair.publicKey, managedNodeID);
      setAccount(result.data);
      setConfig(result.config);
      setIntervalDays(result.data.update_interval_days || 0);
    } catch (error) {
      messageApi.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function refreshConfig() {
    setLoading(true);
    try {
      setConfig(await integrations.warp.config(managedNodeID));
    } catch (error) {
      messageApi.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function rotateAddress() {
    setLoading(true);
    try {
      const pair = generateWireguardKeypair();
      const result = await integrations.warp.rotate(pair.privateKey, pair.publicKey, managedNodeID);
      setAccount(result.data);
      setConfig(result.config);
      const nextDraft = buildWarpDraft(result.data, result.config);
      if (nextDraft && connectorExists) await onReplaceConnector(nextDraft);
      messageApi.success(t('integration.warp.changeSuccess'));
    } catch (error) {
      messageApi.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function saveLicense() {
    setLoading(true);
    setLicenseError('');
    try {
      const next = await integrations.warp.license(license.trim(), managedNodeID);
      setAccount(next);
      setLicense('');
    } catch (error) {
      setLicenseError(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function saveInterval() {
    setLoading(true);
    try {
      const next = await integrations.warp.interval(intervalDays, managedNodeID);
      setAccount(next);
      messageApi.success(t('integration.saved'));
    } catch (error) {
      messageApi.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function removeAccount() {
    setLoading(true);
    try {
      await integrations.warp.remove(managedNodeID);
      await onRemoveConnector();
      setAccount(null);
      setConfig(null);
      onClose();
    } catch (error) {
      messageApi.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function saveConnector() {
    if (!draft) {
      messageApi.warning(t('integration.warp.fetchFirst'));
      return;
    }
    setLoading(true);
    try {
      if (connectorExists) await onReplaceConnector(draft);
      else await onAddConnector(draft);
      onClose();
    } catch {
      // The parent reports persistence errors and keeps the modal open.
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      {messageContextHolder}
      <Modal open={open} title={t('integration.warp.title')} footer={null} onCancel={onClose} destroyOnHidden>
        <Form colon={false} labelCol={{ sm: { span: 7 } }} wrapperCol={{ sm: { span: 16 } }}>
          <Form.Item label={t('node.targetNode')} tooltip={t('node.targetNodeHelp')}>
            <Select value={managedNodeID} options={nodeTargetOptions} disabled={loading} onChange={onManagedNodeIDChange} />
          </Form.Item>
        </Form>
        {!account ? (
          <Button type="primary" loading={loading} icon={<ApiOutlined />} onClick={register}>
            {t('integration.warp.createAccount')}
          </Button>
        ) : (
          <>
            <table className="integration-data-table">
              <tbody>
                <DataRow label={t('integration.warp.accessToken')} value={account.access_token} />
                <DataRow label={t('integration.warp.deviceId')} value={account.device_id} />
                <DataRow label={t('integration.warp.licenseKey')} value={account.license_key} />
                <DataRow label={t('integration.warp.privateKey')} value={account.private_key} />
              </tbody>
            </table>

            <Popconfirm
              title={t('integration.warp.deleteAccount')}
              okText={t('common.delete')}
              cancelText={t('common.close')}
              onConfirm={removeAccount}
            >
              <Button danger type="primary" loading={loading} icon={<DeleteOutlined />}>
                {t('integration.warp.deleteAccount')}
              </Button>
            </Popconfirm>

            <Divider className="integration-divider">{t('integration.warp.settings')}</Divider>
            <Collapse
              items={[
                {
                  key: 'license',
                  label: t('integration.warp.licenseSetting'),
                  children: (
                    <Form colon={false} labelCol={{ sm: { span: 7 } }} wrapperCol={{ sm: { span: 16 } }}>
                      <Form.Item label={t('integration.warp.key')}>
                        <Input value={license} placeholder={t('integration.warp.keyPlaceholder')} onChange={(event) => {
                          setLicense(event.target.value);
                          setLicenseError('');
                        }} />
                      </Form.Item>
                      <div className="integration-inline-actions">
                        <Button type="primary" disabled={license.trim().length < 26} loading={loading} onClick={saveLicense}>
                          {t('common.update')}
                        </Button>
                        {licenseError ? <Alert type="error" showIcon title={licenseError} /> : null}
                      </div>
                    </Form>
                  ),
                },
                {
                  key: 'interval',
                  label: t('integration.warp.autoUpdate'),
                  children: (
                    <Form colon={false} labelCol={{ sm: { span: 9 } }} wrapperCol={{ sm: { span: 14 } }}>
                      <Form.Item label={t('integration.warp.intervalDays')} tooltip={t('integration.warp.intervalHelp')}>
                        <Space orientation="vertical" style={{ width: '100%' }}>
                          <InputNumber min={0} max={3650} value={intervalDays} onChange={(value) => setIntervalDays(value || 0)} style={{ width: '100%' }} />
                          <Button type="primary" loading={loading} onClick={saveInterval}>{t('common.save')}</Button>
                        </Space>
                      </Form.Item>
                    </Form>
                  ),
                },
              ]}
            />

            <Divider className="integration-divider">{t('integration.warp.accountInfo')}</Divider>
            <Space wrap>
              <Button type="primary" loading={loading} icon={<SyncOutlined />} onClick={refreshConfig}>{t('common.refresh')}</Button>
              <Button type="primary" loading={loading} icon={<SyncOutlined />} onClick={rotateAddress}>{t('integration.warp.changeIp')}</Button>
            </Space>

            {config ? (
              <>
                <table className="integration-data-table">
                  <tbody>
                    <DataRow label={t('integration.warp.deviceName')} value={config.name} />
                    <DataRow label={t('integration.warp.deviceModel')} value={config.model} />
                    <DataRow label={t('integration.warp.deviceEnabled')} value={String(config.enabled)} />
                    {config.account ? <DataRow label={t('integration.warp.accountType')} value={config.account.account_type} /> : null}
                    {config.account ? <DataRow label={t('integration.warp.role')} value={config.account.role} /> : null}
                    {config.account ? <DataRow label={t('integration.warp.plusData')} value={formatBytes(config.account.premium_data || 0)} /> : null}
                    {config.account ? <DataRow label={t('integration.warp.quota')} value={formatBytes(config.account.quota || 0)} /> : null}
                    {config.account?.usage != null ? <DataRow label={t('integration.warp.usage')} value={formatBytes(config.account.usage)} /> : null}
                  </tbody>
                </table>
                <Divider className="integration-divider">{t('integration.outboundStatus')}</Divider>
                <Space>
                  <Tag color={connectorExists ? 'green' : 'orange'}>{t(connectorExists ? 'common.enabled' : 'common.disabled')}</Tag>
                  <Button type="primary" danger={connectorExists} loading={loading} icon={connectorExists ? undefined : <PlusOutlined />} onClick={saveConnector}>
                    {connectorExists ? t('common.reset') : t('integration.addConnector')}
                  </Button>
                </Space>
              </>
            ) : null}
          </>
        )}
      </Modal>
    </>
  );
}

function DataRow({ label, value }: { label: string; value: unknown }) {
  return <tr><td>{label}</td><td>{value == null || value === '' ? '-' : String(value)}</td></tr>;
}

function buildWarpDraft(account: WarpAccount | null, config: WarpConfig | null): WireguardIntegrationDraft | null {
  const wireguard = config?.config;
  const peer = wireguard?.peers?.[0];
  if (!account || !peer?.public_key || !peer.endpoint?.host) return null;
  const addresses: string[] = [];
  const v4 = wireguard?.interface?.addresses?.v4;
  const v6 = wireguard?.interface?.addresses?.v6;
  if (v4) addresses.push(`${v4}/32`);
  if (v6) addresses.push(`${v6}/128`);
  return {
    tag: 'warp',
    settings: {
      mtu: 1420,
      secretKey: account.private_key,
      address: addresses,
      reserved: decodeReserved(wireguard?.client_id || account.client_id),
      domainStrategy: 'ForceIPv4v6',
      peers: [{ publicKey: peer.public_key, endpoint: peer.endpoint.host }],
      noKernelTun: true,
    },
  };
}

function decodeReserved(clientId?: string): number[] {
  if (!clientId) return [];
  try {
    return Array.from(atob(clientId), (char) => char.charCodeAt(0));
  } catch {
    return [];
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
