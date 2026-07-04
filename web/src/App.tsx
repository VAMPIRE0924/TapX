import { useCallback, useEffect, useMemo, useState } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { Button, Card, ConfigProvider, Form, Input, Layout, Spin, message } from 'antd';

import type { AnyRecord, AuthSession, RuntimeConfig } from './api';
import { deleteObject, loadAuthSession, loadConfig, loadRuntimeState, login, logout, saveConfig, upsertObject, validateConfig } from './api';
import { kindByKey, kindDefs, type KindDef } from './schema';
import { useTheme } from './theme';
import { useI18n } from './i18n';
import { AppSidebar } from './layout/AppSidebar';
import { DashboardPage } from './pages/DashboardPage';
import { ObjectListPage } from './pages/ObjectListPage';
import { SystemPage } from './pages/SystemPage';
import { XrayPage } from './pages/XrayPage';

function LoginScreen({ onLogin }: { onLogin: (username: string, password: string) => Promise<void> }) {
  const { t } = useI18n();
  const [loading, setLoading] = useState(false);

  return (
    <div className="login-screen">
      <Card className="login-card">
        <div className="login-brand">
          <span className="brand-mark">TX</span>
          <div>
            <strong>TapX</strong>
            <span>Control Plane</span>
          </div>
        </div>
        <Form
          layout="vertical"
          onFinish={async (values) => {
            setLoading(true);
            try {
              await onLogin(values.username, values.password);
            } finally {
              setLoading(false);
            }
          }}
        >
          <Form.Item name="username" label={t('username')} rules={[{ required: true }]}>
            <Input autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" label={t('password')} rules={[{ required: true }]}>
            <Input.Password autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={loading} block>{t('login')}</Button>
        </Form>
      </Card>
    </div>
  );
}

export function App() {
  const { antdThemeConfig, isDark, isUltra } = useTheme();
  const [auth, setAuth] = useState<AuthSession | null>(null);
  const [config, setConfig] = useState<RuntimeConfig>({});
  const [runtime, setRuntime] = useState<AnyRecord>({});
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    const [nextConfig, nextRuntime] = await Promise.all([loadConfig(), loadRuntimeState()]);
    setConfig(nextConfig);
    setRuntime(nextRuntime);
  }, []);

  useEffect(() => {
    (async () => {
      try {
        const session = await loadAuthSession();
        setAuth(session);
        if (!session.authEnabled || session.authenticated) await refresh();
      } catch (error) {
        message.error((error as Error).message);
      } finally {
        setLoading(false);
      }
    })();
  }, [refresh]);

  const locked = Boolean(auth?.authEnabled && !auth.authenticated);

  async function handleLogin(username: string, password: string) {
    const session = await login(username, password);
    setAuth(session);
    await refresh();
  }

  async function handleLogout() {
    await logout();
    setAuth({ authEnabled: true, authenticated: false });
  }

  async function handleSaveObject(kind: KindDef, value: AnyRecord) {
    if (!value.ID) throw new Error('ID is required');
    const response = await upsertObject(kind.key, value.ID, value);
    setConfig(response.config);
    await refresh();
    message.success('saved');
  }

  async function handleDeleteObject(kind: KindDef, id: string) {
    const response = await deleteObject(kind.key, id);
    setConfig(response.config);
    await refresh();
    message.success('deleted');
  }

  async function handleReplaceConfig(next: RuntimeConfig) {
    await validateConfig(next, 'save');
    const response = await saveConfig(next);
    setConfig(response.config);
    await refresh();
    message.success('saved');
  }

  const objectRoutes = useMemo(() => kindDefs
    .filter((kind) => kind.key !== 'xrayProfiles' && kind.key !== 'settings')
    .map((kind) => (
      <Route
        key={kind.key}
        path={`/${kind.key}`}
        element={(
          <ObjectListPage
            kind={kind}
            config={config}
            onSaveObject={handleSaveObject}
            onDeleteObject={handleDeleteObject}
            onReplaceConfig={handleReplaceConfig}
          />
        )}
      />
    )), [config]);

  if (loading) {
    return (
      <ConfigProvider theme={antdThemeConfig}>
        <div className="loading-screen"><Spin size="large" /></div>
      </ConfigProvider>
    );
  }

  if (locked) {
    return (
      <ConfigProvider theme={antdThemeConfig}>
        <LoginScreen onLogin={handleLogin} />
      </ConfigProvider>
    );
  }

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={`tapx-app ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim()}>
        <AppSidebar onLogout={handleLogout} />
        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <Routes>
              <Route path="/" element={<DashboardPage config={config} runtime={runtime} onRefresh={refresh} />} />
              {objectRoutes}
              <Route
                path="/settings"
                element={(
                  <ObjectListPage
                    kind={kindByKey.settings}
                    config={config}
                    onSaveObject={handleSaveObject}
                    onDeleteObject={handleDeleteObject}
                    onReplaceConfig={handleReplaceConfig}
                  />
                )}
              />
              <Route
                path="/xray"
                element={(
                  <XrayPage
                    config={config}
                    onSaveObject={handleSaveObject}
                    onDeleteObject={handleDeleteObject}
                    onReplaceConfig={handleReplaceConfig}
                  />
                )}
              />
              <Route path="/system" element={<SystemPage config={config} onRefresh={refresh} />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
