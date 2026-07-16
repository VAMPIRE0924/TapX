import { useCallback, useEffect, useState } from 'react';
import type { Dayjs } from 'dayjs';
import {
  CopyOutlined,
  DeleteOutlined,
  KeyOutlined,
  LinkOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Alert, Button, DatePicker, Divider, Form, Input, Modal, Popconfirm, Space, Switch, Table, Tabs, Tag, Typography, message } from 'antd';
import { QRCodeSVG } from 'qrcode.react';
import {
  createPanelAPIToken,
  deletePanelAPIToken,
  disablePanelTOTP,
  enablePanelTOTP,
  getPanelSecurity,
  preparePanelTOTP,
  type PanelAPIToken,
  type PanelSecurityStatus,
  type TOTPSetup,
} from '../../shared/api';
import { errorMessage } from '../../shared/localized-error';
import { copyText } from '../../shared/clipboard';
import { useI18n } from '../../i18n/I18nProvider';
import { panelPath } from '../../app/runtime-path';
import './SecuritySettings.css';

const emptySecurity: PanelSecurityStatus = { twoFactorEnabled: false, apiTokens: [] };

export function SecuritySettings({
  updatingCredentials,
  onSubmitCredentials,
}: {
  updatingCredentials: boolean;
  onSubmitCredentials: () => void;
}) {
  const { t } = useI18n();
  const [security, setSecurity] = useState<PanelSecurityStatus>(emptySecurity);
  const [loading, setLoading] = useState(true);
  const [setup, setSetup] = useState<TOTPSetup>();
  const [setupCode, setSetupCode] = useState('');
  const [disableOpen, setDisableOpen] = useState(false);
  const [disableCode, setDisableCode] = useState('');
  const [tokenOpen, setTokenOpen] = useState(false);
  const [tokenName, setTokenName] = useState('');
  const [tokenExpiry, setTokenExpiry] = useState<Dayjs | null>(null);
  const [revealedToken, setRevealedToken] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setSecurity(await getPanelSecurity());
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('security.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [messageApi, t]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function beginTOTPSetup() {
    setSubmitting(true);
    try {
      setSetup(await preparePanelTOTP());
      setSetupCode('');
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('security.totpGenerateFailed'));
    } finally {
      setSubmitting(false);
    }
  }

  async function enableTOTP() {
    if (!setup || !/^\d{6}$/.test(setupCode)) {
      messageApi.warning(t('security.totpCodeRequired'));
      return;
    }
    setSubmitting(true);
    try {
      await enablePanelTOTP(setup.secret, setupCode);
      setSetup(undefined);
      setSecurity((current) => ({ ...current, twoFactorEnabled: true }));
      messageApi.success(t('security.totpEnabledNotice'));
      window.setTimeout(() => window.location.replace(panelPath('/login.html')), 900);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('security.totpEnableFailed'));
    } finally {
      setSubmitting(false);
    }
  }

  async function disableTOTP() {
    if (!/^\d{6}$/.test(disableCode)) {
      messageApi.warning(t('security.currentCodeRequired'));
      return;
    }
    setSubmitting(true);
    try {
      await disablePanelTOTP(disableCode);
      setDisableOpen(false);
      setDisableCode('');
      setSecurity((current) => ({ ...current, twoFactorEnabled: false }));
      messageApi.success(t('security.totpDisabledNotice'));
      window.setTimeout(() => window.location.replace(panelPath('/login.html')), 900);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('security.totpDisableFailed'));
    } finally {
      setSubmitting(false);
    }
  }

  async function createToken() {
    const name = tokenName.trim();
    if (!name) {
      messageApi.warning(t('security.tokenNameRequired'));
      return;
    }
    setSubmitting(true);
    try {
      const created = await createPanelAPIToken(name, tokenExpiry?.toISOString());
      setSecurity((current) => ({ ...current, apiTokens: [...current.apiTokens, created.item] }));
      setTokenOpen(false);
      setTokenName('');
      setTokenExpiry(null);
      setRevealedToken(created.token);
    } catch (error) {
      messageApi.error(errorMessage(error, t, 'security.tokenCreateFailed'));
    } finally {
      setSubmitting(false);
    }
  }

  async function removeToken(id: string) {
    try {
      await deletePanelAPIToken(id);
      setSecurity((current) => ({ ...current, apiTokens: current.apiTokens.filter((item) => item.id !== id) }));
      messageApi.success(t('security.tokenRevoked'));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('security.tokenRevokeFailed'));
    }
  }

  async function copy(value: string) {
    try {
      await copyText(value);
      messageApi.success(t('security.copied'));
    } catch {
      messageApi.error(t('security.copyFailed'));
    }
  }

  return (
    <>
      {contextHolder}
      <Tabs
        className="settings-security-tabs"
        items={[
          {
            key: 'credentials',
            label: <span className="security-tab-label"><UserOutlined />{t('security.credentials')}</span>,
            children: (
              <>
                <Form.Item name="oldUsername" label={t('security.oldUsername')} tooltip={t('security.currentCredentialHelp')}><Input autoComplete="username" placeholder="admin" /></Form.Item>
                <Form.Item name="oldPassword" label={t('security.oldPassword')} tooltip={t('security.currentCredentialHelp')}><Input.Password autoComplete="current-password" placeholder="••••••••••••" /></Form.Item>
                <Form.Item name="newUsername" label={t('security.newUsername')} tooltip={t('security.newCredentialHelp')}><Input autoComplete="username" placeholder="tapx-admin" /></Form.Item>
                <Form.Item name="newPassword" label={t('security.newPassword')} tooltip={t('security.newCredentialHelp')}><Input.Password autoComplete="new-password" placeholder="TapX-Admin-2026!" /></Form.Item>
                <Form.Item wrapperCol={{ sm: { offset: 7, span: 14 } }}>
                  <Button type="primary" loading={updatingCredentials} onClick={onSubmitCredentials}>{t('security.confirm')}</Button>
                </Form.Item>
              </>
            ),
          },
          {
            key: 'two-factor',
            label: <span className="security-tab-label"><SafetyCertificateOutlined />{t('security.twoFactor')}</span>,
            children: (
              <div className="security-toggle-row">
                <div className="security-toggle-copy">
                  <strong>{t('security.enable2FA')}</strong>
                  <span>{t('security.twoFactorSwitchHelp')}</span>
                </div>
                <Switch
                  checked={security.twoFactorEnabled}
                  loading={loading || submitting}
                  aria-label={t('security.enable2FA')}
                  onChange={(checked) => {
                    if (checked) void beginTOTPSetup();
                    else setDisableOpen(true);
                  }}
                />
              </div>
            ),
          },
          {
            key: 'api-token',
            label: <span className="security-tab-label"><LinkOutlined />{t('security.apiToken')}</span>,
            children: (
              <div className="settings-token-panel">
                <Alert type="warning" showIcon title={t('security.apiTokenWarning')} />
                <Space>
                  <Button type="primary" icon={<PlusOutlined />} onClick={() => setTokenOpen(true)}>{t('security.newToken')}</Button>
                  <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void refresh()}>{t('common.refresh')}</Button>
                </Space>
                <TokenTable tokens={security.apiTokens} onDelete={(id) => void removeToken(id)} />
              </div>
            ),
          },
        ]}
      />

      <Modal
        open={Boolean(setup)}
        title={t('security.enableTitle')}
        okText={t('security.verifyEnable')}
        cancelText={t('settings.cancel')}
        width={520}
        centered
        className="totp-setup-modal"
        confirmLoading={submitting}
        okButtonProps={{ disabled: !/^\d{6}$/.test(setupCode) }}
        onOk={() => void enableTOTP()}
        onCancel={() => setSetup(undefined)}
      >
        <div className="totp-setup-content">
          <Typography.Text strong>{t('security.setupIntro')}</Typography.Text>
          <Divider />
          <section className="totp-setup-step">
            <Typography.Text strong>{t('security.setupStepScan')}</Typography.Text>
            <div
              className="totp-qr-copy-surface"
              role="button"
              tabIndex={0}
              aria-label={t('security.copySecret')}
              onClick={() => setup?.secret && void copy(setup.secret)}
              onKeyDown={(event) => {
                if ((event.key === 'Enter' || event.key === ' ') && setup?.secret) {
                  event.preventDefault();
                  void copy(setup.secret);
                }
              }}
            >
              <div className="totp-qr-shell">
                {setup?.uri ? <QRCodeSVG value={setup.uri} size={180} level="M" marginSize={0} /> : null}
              </div>
              <Typography.Text className="totp-secret">
                {setup?.secret || ''}
              </Typography.Text>
            </div>
          </section>
          <Divider />
          <section className="totp-setup-step totp-code-step">
            <Typography.Text strong>{t('security.setupStepCode')}</Typography.Text>
            <Input
              value={setupCode}
              onChange={(event) => setSetupCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
              aria-label={t('security.sixDigitCode')}
              inputMode="numeric"
              maxLength={6}
              autoComplete="one-time-code"
            />
          </section>
        </div>
      </Modal>

      <Modal
        open={disableOpen}
        title={t('security.disableTitle')}
        okText={t('security.disable')}
        okButtonProps={{ danger: true }}
        cancelText={t('settings.cancel')}
        confirmLoading={submitting}
        onOk={() => void disableTOTP()}
        onCancel={() => setDisableOpen(false)}
      >
        <Input prefix={<KeyOutlined />} value={disableCode} onChange={(event) => setDisableCode(event.target.value.replace(/\D/g, '').slice(0, 6))} placeholder={t('security.currentSixDigitCode')} inputMode="numeric" />
      </Modal>

      <Modal
        open={tokenOpen}
        title={t('security.newTokenTitle')}
        okText={t('common.create')}
        cancelText={t('settings.cancel')}
        confirmLoading={submitting}
        onOk={() => void createToken()}
        onCancel={() => setTokenOpen(false)}
      >
        <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
          <Input value={tokenName} onChange={(event) => setTokenName(event.target.value)} placeholder="automation-ci" maxLength={80} />
          <DatePicker showTime value={tokenExpiry} onChange={setTokenExpiry} placeholder="2027-12-31 23:59" style={{ width: '100%' }} />
        </Space>
      </Modal>

      <Modal open={Boolean(revealedToken)} title={t('security.tokenCreated')} footer={<Button type="primary" onClick={() => setRevealedToken('')}>{t('security.savedToken')}</Button>} closable={false} maskClosable={false}>
        <Alert type="warning" showIcon title={t('security.tokenShownOnce')} />
        <Input.Search className="security-token-reveal" value={revealedToken} readOnly enterButton={<CopyOutlined />} onSearch={(value) => void copy(value)} />
      </Modal>
    </>
  );
}

function TokenTable({ tokens, onDelete }: { tokens: PanelAPIToken[]; onDelete: (id: string) => void }) {
  const { t } = useI18n();
  return (
    <Table<PanelAPIToken>
      rowKey="id"
      size="small"
      dataSource={tokens}
      pagination={false}
      locale={{ emptyText: t('security.noTokens') }}
      columns={[
        { title: t('security.name'), dataIndex: 'name' },
        { title: t('security.prefix'), dataIndex: 'prefix', render: (value: string) => <Typography.Text code>{value}...</Typography.Text> },
        { title: t('security.createdAt'), dataIndex: 'createdAt', render: formatTime },
        { title: t('security.expiresAt'), dataIndex: 'expiresAt', render: (value?: string) => value ? formatTime(value) : <Tag>{t('security.neverExpires')}</Tag> },
        {
          title: t('security.action'),
          width: 80,
          render: (_, item) => (
            <Popconfirm title={t('security.revokeConfirm', { name: item.name })} okText={t('security.revoke')} cancelText={t('settings.cancel')} onConfirm={() => onDelete(item.id)}>
              <Button type="text" danger icon={<DeleteOutlined />} aria-label={t('security.revoke')} />
            </Popconfirm>
          ),
        },
      ]}
    />
  );
}

function formatTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
