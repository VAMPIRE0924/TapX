import { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Button, Card, Form, Input, InputNumber, Modal, Select, Space, message } from 'antd';
import { ReloadOutlined, SaveOutlined } from '@ant-design/icons';
import {
  getRuntimeConfig,
  restartPanelService,
  saveRuntimeConfig,
  updateAdminCredentials,
  type RuntimeConfig,
} from '../shared/api';
import { objectToSettings, settingsToObject, stableSettingsSnapshot } from '../shared/settings';
import { languageOptions, type LanguageCode } from '../i18n/dictionaries';
import { useI18n } from '../i18n/I18nProvider';
import { SecuritySettings } from '../features/security/SecuritySettings';
import './SettingsPage.css';
import { hashFromPath } from '../app/hash-route';

interface PanelSettings extends Record<string, unknown> {
  listenIP?: string;
  listenDomain?: string;
  listenPort?: number;
  uriPath?: string;
  sessionMinutes?: number;
  panelOutbound?: string;
  pageSize?: number;
  language?: LanguageCode;
  certPublicPath?: string;
  certPrivatePath?: string;
  timezone?: string;
  oldUsername?: string;
  oldPassword?: string;
  newUsername?: string;
  newPassword?: string;
}

type SettingsSection = 'general' | 'certificate' | 'security' | 'timezone';

const defaultSettings: PanelSettings = {
  listenIP: '',
  listenDomain: '',
  listenPort: 2053,
  uriPath: '/',
  sessionMinutes: 60,
  panelOutbound: 'direct',
  pageSize: 10,
  language: 'zh-CN',
  timezone: 'Asia/Hong_Kong',
};

export function SettingsPage({ currentPath }: { currentPath: string }) {
  const { t, setLanguage } = useI18n();
  const [config, setConfig] = useState<RuntimeConfig>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [updatingCredentials, setUpdatingCredentials] = useState(false);
  const [form] = Form.useForm<PanelSettings>();
  const watchedSettings = Form.useWatch([], form);
  const baselineRef = useRef('');
  const [messageApi, messageContextHolder] = message.useMessage();
  const [modal, modalContextHolder] = Modal.useModal();

  const settings = useMemo(() => ({ ...defaultSettings, ...settingsToObject<PanelSettings>(config.Settings) }), [config.Settings]);
  const activeSection = settingsSectionFromHash(hashFromPath(currentPath));

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!watchedSettings || !baselineRef.current) return;
    setDirty(serializePanelSettings({ ...settings, ...watchedSettings }) !== baselineRef.current);
  }, [settings, watchedSettings]);

  async function refresh() {
    setLoading(true);
    try {
      const next = await getRuntimeConfig();
      const nextValues = { ...defaultSettings, ...settingsToObject<PanelSettings>(next.Settings) };
      baselineRef.current = serializePanelSettings(nextValues);
      setConfig(next);
      form.setFieldsValue(nextValues);
      if (nextValues.language) setLanguage(nextValues.language);
      setDirty(false);
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('settings.loadFailed'));
    } finally {
      setLoading(false);
    }
  }

  async function submit() {
    try {
      await form.validateFields();
    } catch {
      return;
    }
    const values = stripSecurityActionValues(form.getFieldsValue(true));
    setSaving(true);
    try {
      const saved = await saveRuntimeConfig({ ...config, Settings: objectToSettings({ ...settings, ...values }) });
      const savedValues = { ...defaultSettings, ...settingsToObject<PanelSettings>(saved.Settings) };
      baselineRef.current = serializePanelSettings(savedValues);
      setConfig(saved);
      form.setFieldsValue(savedValues);
      if (savedValues.language) setLanguage(savedValues.language);
      setDirty(false);
      messageApi.success(t('settings.saved'));
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('settings.saveFailed'));
    } finally {
      setSaving(false);
    }
  }

  function restartPanel() {
    modal.confirm({
      title: t('settings.restart'),
      content: t('settings.restartConfirm'),
      okText: t('settings.restart'),
      okButtonProps: { danger: true },
      cancelText: t('settings.cancel'),
      onOk: async () => {
        try {
          await restartPanelService();
          messageApi.success(t('settings.restarting'));
          window.setTimeout(() => window.location.reload(), 1500);
        } catch (err) {
          messageApi.error(err instanceof Error ? err.message : t('settings.restartFailed'));
          throw err;
        }
      },
    });
  }

  async function submitCredentials() {
    const values = form.getFieldsValue(['oldUsername', 'oldPassword', 'newUsername', 'newPassword']);
    if (!values.newUsername?.trim() || !values.newPassword) {
      messageApi.error(t('settings.credentialsRequired'));
      return;
    }
    setUpdatingCredentials(true);
    try {
      await updateAdminCredentials({
        oldUsername: values.oldUsername || '',
        oldPassword: values.oldPassword || '',
        newUsername: values.newUsername.trim(),
        newPassword: values.newPassword,
      });
      form.setFieldsValue({ oldUsername: '', oldPassword: '', newUsername: '', newPassword: '' });
      messageApi.success(t('settings.credentialsUpdated'));
    } catch (err) {
      messageApi.error(err instanceof Error ? err.message : t('settings.credentialsFailed'));
    } finally {
      setUpdatingCredentials(false);
    }
  }

  return (
    <div className="settings-page">
      {messageContextHolder}
      {modalContextHolder}
      <Form
        form={form}
        colon={false}
        labelCol={{ sm: { span: 7 } }}
        wrapperCol={{ sm: { span: 14 } }}
        labelWrap
        initialValues={settings}
      >
        <SettingsWarning values={{ ...settings, ...(watchedSettings || {}) }} />
        <Card hoverable loading={loading} className="settings-actions-card">
          <div className="settings-actions">
            <Space wrap>
              <Button type="primary" icon={<SaveOutlined />} loading={saving} disabled={!dirty} onClick={submit}>{t('common.save')}</Button>
              <Button type="primary" danger icon={<ReloadOutlined />} disabled={dirty || loading} onClick={restartPanel}>{t('settings.restart')}</Button>
            </Space>
            <Alert type="warning" showIcon title={t('settings.saveRestartNotice')} />
          </div>
        </Card>
        <Card hoverable loading={loading} className="settings-content-card">
          {activeSection === 'general' ? <GeneralSettingsFields config={config} /> : null}
          {activeSection === 'certificate' ? <CertificateSettingsFields /> : null}
          {activeSection === 'timezone' ? <TimezoneSettingsFields /> : null}
          {activeSection === 'security' ? <SecuritySettings updatingCredentials={updatingCredentials} onSubmitCredentials={() => void submitCredentials()} /> : null}
        </Card>
      </Form>
    </div>
  );
}

function settingsSectionFromHash(hash: string): SettingsSection {
  if (hash === '#certificate') return 'certificate';
  if (hash === '#security') return 'security';
  if (hash === '#timezone') return 'timezone';
  return 'general';
}

function SettingsWarning({ values }: { values: PanelSettings }) {
  const { t } = useI18n();
  const warnings: string[] = [];
  if (window.location.protocol !== 'https:') warnings.push(t('settings.warningHttp'));
  if ((values.listenPort ?? defaultSettings.listenPort) === 2053) warnings.push(t('settings.warningPort'));
  if ((values.uriPath || '/') === '/') warnings.push(t('settings.warningPath'));
  if (warnings.length === 0) return null;
  return (
    <Alert
      type="error"
      showIcon
      closable
      className="settings-warning"
      title={t('settings.warningTitle')}
      description={(
        <div>
          <b>{t('settings.warningIntro')}</b>
          <ul>{warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>
        </div>
      )}
    />
  );
}

function GeneralSettingsFields({ config }: { config: RuntimeConfig }) {
  const { t } = useI18n();
  const profiles = new Map((config.XrayProfiles || []).map((profile) => [profile.ID, profile]));
  const panelOutboundOptions = [
    { value: 'direct', label: t('settings.direct') },
    ...(config.Connectors || [])
      .filter((connector) => {
        if (connector.Enabled === false || connector.Transport !== 'xray') return false;
        const profile = profiles.get(connector.XrayProfileID || '');
        return Boolean(profile && profile.Enabled !== false && profile.Runtime !== 'external');
      })
      .map((connector) => ({
        value: connector.ID,
        label: `${connector.Name || connector.ID} · ${t('settings.embeddedXraySuffix')}`,
      })),
  ];
  return (
    <>
      <Form.Item name="listenIP" label={t('settings.listenIP')} tooltip={t('settings.listenIPHelp')}>
        <Input allowClear placeholder="0.0.0.0" />
      </Form.Item>
      <Form.Item name="listenDomain" label={t('settings.listenDomain')} tooltip={t('settings.listenDomainHelp')}>
        <Input allowClear placeholder="panel.example.com" />
      </Form.Item>
      <Form.Item name="listenPort" label={t('settings.listenPort')} tooltip={t('settings.restartEffective')}>
        <InputNumber min={1} max={65535} placeholder="2053" />
      </Form.Item>
      <Form.Item
        name="uriPath"
        label={t('settings.uriPath')}
        tooltip={t('settings.uriPathHelp')}
        rules={[{ pattern: /^\/(?:.*\/)?$/, message: t('settings.uriPathHelp') }]}
      >
        <Input placeholder="/x-ui/" />
      </Form.Item>
      <Form.Item name="sessionMinutes" label={t('settings.sessionMinutes')} tooltip={t('settings.sessionMinutesHelp')}>
        <InputNumber min={1} placeholder="60" />
      </Form.Item>
      <Form.Item name="panelOutbound" label={t('settings.panelOutbound')} tooltip={t('settings.panelOutboundHelp')}>
        <Select
          allowClear
          showSearch
          options={panelOutboundOptions}
        />
      </Form.Item>
      <Form.Item name="pageSize" label={t('settings.pageSize')} tooltip={t('settings.pageSizeHelp')}>
        <InputNumber min={0} placeholder="50" />
      </Form.Item>
      <Form.Item name="language" label={t('settings.language')}>
        <Select options={languageOptions} />
      </Form.Item>
    </>
  );
}

function CertificateSettingsFields() {
  const { t } = useI18n();
  return (
    <>
      <Form.Item name="certPublicPath" label={t('settings.certPublicPath')} tooltip={`${t('settings.certificateMessage')} ${t('settings.certPublicHelp')}`}>
        <Input placeholder="/etc/ssl/tapx/fullchain.pem" />
      </Form.Item>
      <Form.Item name="certPrivatePath" label={t('settings.certPrivatePath')} tooltip={t('settings.certPrivateHelp')}>
        <Input placeholder="/etc/ssl/tapx/privkey.pem" />
      </Form.Item>
    </>
  );
}

function TimezoneSettingsFields() {
  const { t } = useI18n();
  return (
    <Form.Item name="timezone" label={t('settings.timezone')} tooltip={t('settings.timezoneHelp')}>
      <Select
        showSearch
        options={[
          { value: 'Asia/Hong_Kong', label: 'Asia/Hong_Kong' },
          { value: 'Asia/Shanghai', label: 'Asia/Shanghai' },
          { value: 'UTC', label: 'UTC' },
          { value: 'America/Los_Angeles', label: 'America/Los_Angeles' },
        ]}
      />
    </Form.Item>
  );
}

const securityActionFields = new Set(['oldUsername', 'oldPassword', 'newUsername', 'newPassword']);

function stripSecurityActionValues(values: PanelSettings): PanelSettings {
  const next = { ...values };
  for (const key of securityActionFields) delete next[key];
  return next;
}

function serializePanelSettings(values: PanelSettings): string {
  return stableSettingsSnapshot(stripSecurityActionValues(values));
}
