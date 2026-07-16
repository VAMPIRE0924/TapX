import { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Divider, Form, Input, Modal, Popconfirm, Select, Space, Tabs, Tag, message } from 'antd';
import { LoginOutlined, SaveOutlined } from '@ant-design/icons';
import { useI18n } from '../../i18n/I18nProvider';
import {
  integrations,
  type NordAccount,
  type NordCity,
  type NordCountry,
  type NordServer,
} from '../../shared/integrations';
import type { WireguardIntegrationDraft } from './types';
import './IntegrationModal.css';

interface NordModalProps {
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

export function NordModal({
  open,
  connectorExists,
  managedNodeID,
  nodeTargetOptions,
  onManagedNodeIDChange,
  onClose,
  onAddConnector,
  onReplaceConnector,
  onRemoveConnector,
}: NordModalProps) {
  const { t } = useI18n();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [loading, setLoading] = useState(false);
  const [account, setAccount] = useState<NordAccount | null>(null);
  const [token, setToken] = useState('');
  const [privateKey, setPrivateKey] = useState('');
  const [countries, setCountries] = useState<NordCountry[]>([]);
  const [cities, setCities] = useState<NordCity[]>([]);
  const [servers, setServers] = useState<NordServer[]>([]);
  const [countryId, setCountryId] = useState<number>();
  const [cityId, setCityId] = useState<number | null>(null);
  const [serverId, setServerId] = useState<number>();

  const filteredServers = useMemo(
    () => cityId == null ? servers : servers.filter((server) => server.cityId === cityId),
    [cityId, servers],
  );
  const selectedServer = useMemo(() => servers.find((server) => server.id === serverId), [serverId, servers]);
  const draft = useMemo(() => buildNordDraft(account, selectedServer), [account, selectedServer]);

  const loadCountries = useCallback(async () => {
    try {
      setCountries(await integrations.nord.countries(managedNodeID));
    } catch (error) {
      messageApi.error(errorMessage(error));
    }
  }, [managedNodeID, messageApi]);

  const loadAccount = useCallback(async () => {
    setLoading(true);
    try {
      const next = await integrations.nord.data(managedNodeID);
      setAccount(next);
      if (next) await loadCountries();
    } catch (error) {
      messageApi.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, [loadCountries, messageApi]);

  useEffect(() => {
    if (!open) return;
    resetState();
    void loadAccount();
  }, [open, loadAccount]);

  useEffect(() => {
    setServerId(filteredServers[0]?.id);
  }, [filteredServers]);

  async function login() {
    setLoading(true);
    try {
      const next = await integrations.nord.login(token.trim(), managedNodeID);
      setAccount(next);
      await loadCountries();
    } catch (error) {
      messageApi.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function savePrivateKey() {
    setLoading(true);
    try {
      const next = await integrations.nord.privateKey(privateKey.trim(), managedNodeID);
      setAccount(next);
      await loadCountries();
    } catch (error) {
      messageApi.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function logout() {
    setLoading(true);
    try {
      await integrations.nord.remove(managedNodeID);
      await onRemoveConnector();
      resetState();
      onClose();
    } catch (error) {
      messageApi.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function selectCountry(nextCountryId: number) {
    setCountryId(nextCountryId);
    setCityId(null);
    setServerId(undefined);
    setCities([]);
    setServers([]);
    setLoading(true);
    try {
      const result = await integrations.nord.servers(nextCountryId, managedNodeID);
      const locationToCity = new Map<number, NordCity>();
      const uniqueCities = new Map<number, NordCity>();
      for (const location of result.locations || []) {
        const city = location.country?.city;
        if (!city) continue;
        locationToCity.set(location.id, city);
        uniqueCities.set(city.id, city);
      }
      const nextServers = (result.servers || []).map((server) => {
        const city = locationToCity.get(server.location_ids?.[0] ?? -1);
        return { ...server, cityId: city?.id || null, cityName: city?.name || '-' };
      }).sort((left, right) => left.load - right.load);
      setCities([...uniqueCities.values()].sort((left, right) => left.name.localeCompare(right.name)));
      setServers(nextServers);
      if (nextServers.length === 0) messageApi.warning(t('integration.nord.noServers'));
    } catch (error) {
      messageApi.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }

  async function saveConnector() {
    if (!draft) {
      messageApi.error(t('integration.nord.noPublicKey'));
      return;
    }
    setLoading(true);
    try {
      if (connectorExists) {
        await onReplaceConnector(draft);
        messageApi.success(t('integration.nord.connectorUpdated'));
      } else {
        await onAddConnector(draft);
        messageApi.success(t('integration.nord.connectorAdded'));
      }
      onClose();
    } catch {
      // The parent reports persistence errors and keeps the modal open.
    } finally {
      setLoading(false);
    }
  }

  function resetState() {
    setAccount(null);
    setToken('');
    setPrivateKey('');
    setCountries([]);
    setCities([]);
    setServers([]);
    setCountryId(undefined);
    setCityId(null);
    setServerId(undefined);
  }

  return (
    <>
      {messageContextHolder}
      <Modal open={open} title={t('integration.nord.title')} footer={null} onCancel={onClose} destroyOnHidden>
        <Form colon={false} labelCol={{ sm: { span: 7 } }} wrapperCol={{ sm: { span: 16 } }}>
          <Form.Item label={t('node.targetNode')} tooltip={t('node.targetNodeHelp')}>
            <Select value={managedNodeID} options={nodeTargetOptions} disabled={loading} onChange={onManagedNodeIDChange} />
          </Form.Item>
        </Form>
        {!account ? (
          <Tabs
            defaultActiveKey="token"
            items={[
              {
                key: 'token',
                label: t('integration.nord.accessToken'),
                children: (
                  <Form colon={false} labelCol={{ sm: { span: 7 } }} wrapperCol={{ sm: { span: 16 } }}>
                    <Form.Item label={t('integration.nord.accessToken')}>
                      <Space orientation="vertical" style={{ width: '100%' }}>
                        <Input.Password value={token} placeholder={t('integration.nord.accessToken')} onChange={(event) => setToken(event.target.value)} />
                        <Button type="primary" loading={loading} disabled={!token.trim()} icon={<LoginOutlined />} onClick={login}>
                          {t('common.login')}
                        </Button>
                      </Space>
                    </Form.Item>
                  </Form>
                ),
              },
              {
                key: 'key',
                label: t('integration.nord.privateKey'),
                children: (
                  <Form colon={false} labelCol={{ sm: { span: 7 } }} wrapperCol={{ sm: { span: 16 } }}>
                    <Form.Item label={t('integration.nord.privateKey')}>
                      <Space orientation="vertical" style={{ width: '100%' }}>
                        <Input.Password value={privateKey} placeholder={t('integration.nord.privateKey')} onChange={(event) => setPrivateKey(event.target.value)} />
                        <Button type="primary" loading={loading} disabled={!privateKey.trim()} icon={<SaveOutlined />} onClick={savePrivateKey}>
                          {t('common.save')}
                        </Button>
                      </Space>
                    </Form.Item>
                  </Form>
                ),
              },
            ]}
          />
        ) : (
          <>
            <table className="integration-data-table">
              <tbody>
                {account.token ? <tr><td>{t('integration.nord.accessToken')}</td><td>{account.token}</td></tr> : null}
                <tr><td>{t('integration.nord.privateKey')}</td><td>{account.private_key}</td></tr>
              </tbody>
            </table>
            <Popconfirm title={t('common.logout')} okText={t('common.logout')} cancelText={t('common.close')} onConfirm={logout}>
              <Button danger type="primary" loading={loading}>{t('common.logout')}</Button>
            </Popconfirm>

            <Divider className="integration-divider">{t('integration.warp.settings')}</Divider>
            <Form colon={false} labelCol={{ sm: { span: 7 } }} wrapperCol={{ sm: { span: 16 } }}>
              <Form.Item label={t('integration.nord.country')}>
                <Select
                  showSearch
                  optionFilterProp="label"
                  value={countryId}
                  options={countries.map((country) => ({ value: country.id, label: `${country.name} (${country.code})` }))}
                  onChange={selectCountry}
                />
              </Form.Item>
              {cities.length > 0 ? (
                <Form.Item label={t('integration.nord.city')}>
                  <Select
                    showSearch
                    optionFilterProp="label"
                    value={cityId}
                    options={[
                      { value: null, label: t('integration.nord.allCities') },
                      ...cities.map((city) => ({ value: city.id, label: city.name })),
                    ]}
                    onChange={setCityId}
                  />
                </Form.Item>
              ) : null}
              {filteredServers.length > 0 ? (
                <Form.Item label={t('integration.nord.server')}>
                  <Select
                    showSearch
                    optionFilterProp="label"
                    value={serverId}
                    options={filteredServers.map((server) => ({
                      value: server.id,
                      label: `${server.cityName} ${server.name} ${server.hostname}`,
                      children: (
                        <span className="integration-server-option">
                          <span>{server.cityName} - {server.name}</span>
                          <Tag color={server.load < 30 ? 'green' : server.load < 70 ? 'orange' : 'red'}>{server.load}%</Tag>
                        </span>
                      ),
                    }))}
                    onChange={setServerId}
                  />
                </Form.Item>
              ) : null}
            </Form>

            <Divider className="integration-divider">{t('integration.outboundStatus')}</Divider>
            <Space>
              <Tag color={connectorExists ? 'green' : 'orange'}>{t(connectorExists ? 'common.enabled' : 'common.disabled')}</Tag>
              <Button type="primary" danger={connectorExists} loading={loading} disabled={!draft} onClick={saveConnector}>
                {connectorExists ? t('common.reset') : t('integration.addConnector')}
              </Button>
            </Space>
          </>
        )}
      </Modal>
    </>
  );
}

function buildNordDraft(account: NordAccount | null, server?: NordServer): WireguardIntegrationDraft | null {
  if (!account || !server) return null;
  const technology = server.technologies?.find((candidate) => candidate.id === 35);
  const publicKey = technology?.metadata?.find((item) => item.name === 'public_key')?.value;
  if (!publicKey) return null;
  return {
    tag: `nord-${server.hostname}`,
    settings: {
      secretKey: account.private_key,
      address: ['10.5.0.2/32'],
      peers: [{ publicKey, endpoint: `${server.station}:51820` }],
      noKernelTun: true,
    },
  };
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
