import { useEffect, useMemo, useState } from 'react';
import { KeyOutlined, LockOutlined, MoonFilled, MoonOutlined, SunOutlined, TranslationOutlined, UserOutlined } from '@ant-design/icons';
import { Button, ConfigProvider, Dropdown, Form, Input, Spin, message } from 'antd';
import { buildThemeConfig } from '../app/antd-theme';
import { nextTheme, readTheme, writeTheme, type ThemeMode } from '../app/theme';
import { languageOptions } from '../i18n/dictionaries';
import { useI18n } from '../i18n/I18nProvider';
import { antdLocale } from '../i18n/locales';
import { getAuthSession, loginPanel } from '../shared/api';
import { errorMessage } from '../shared/localized-error';
import './LoginPage.css';
import { panelPath } from '../app/runtime-path';

interface LoginValues {
  username: string;
  password: string;
  twoFactorCode?: string;
}

export function LoginPage() {
  const { t, language, setLanguage } = useI18n();
  const [theme, setTheme] = useState<ThemeMode>(() => readTheme());
  const [checking, setChecking] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [twoFactorEnabled, setTwoFactorEnabled] = useState(false);
  const [headlineIndex, setHeadlineIndex] = useState(0);
  const [messageApi, messageContextHolder] = message.useMessage();
  const headlines = useMemo(() => [t('login.hello'), t('login.welcome')], [t]);

  useEffect(() => {
    writeTheme(theme);
  }, [theme]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      setHeadlineIndex((current) => (current + 1) % headlines.length);
    }, 2000);
    return () => window.clearInterval(timer);
  }, [headlines.length]);

  useEffect(() => {
    if (import.meta.env.DEV && window.location.hash === '#preview') {
      setChecking(false);
      return undefined;
    }
    let active = true;
    getAuthSession()
      .then((session) => {
        if (!active) return;
        if (!session.authEnabled || session.authenticated) window.location.replace(panelPath('/'));
        setTwoFactorEnabled(session.twoFactorEnabled === true);
      })
      .catch((error) => {
        if (active) messageApi.error(errorMessage(error, t, 'login.failed'));
      })
      .finally(() => {
        if (active) setChecking(false);
      });
    return () => {
      active = false;
    };
  }, [messageApi, t]);

  async function submit(values: LoginValues) {
    setSubmitting(true);
    try {
      await loginPanel(values.username, values.password, values.twoFactorCode || '');
      window.location.replace(panelPath('/'));
    } catch (error) {
      messageApi.error(errorMessage(error, t, 'login.failed'));
    } finally {
      setSubmitting(false);
    }
  }

  const themeIcon = theme === 'light' ? <SunOutlined /> : theme === 'dark' ? <MoonOutlined /> : <MoonFilled />;
  return (
    <ConfigProvider theme={buildThemeConfig(theme)} locale={antdLocale(language)}>
      {messageContextHolder}
      <main className="login-page">
        <div className="login-toolbar">
          <Button
            shape="circle"
            aria-label={t('common.theme')}
            title={t('common.theme')}
            icon={themeIcon}
            onClick={() => setTheme((current) => nextTheme(current))}
          />
          <Dropdown
            trigger={['click']}
            menu={{
              selectedKeys: [language],
              items: languageOptions.map((item) => ({ key: item.value, label: item.label })),
              onClick: ({ key }) => setLanguage(key as typeof language),
            }}
          >
            <Button shape="circle" aria-label={t('login.language')} title={t('login.language')} icon={<TranslationOutlined />} />
          </Dropdown>
        </div>

        <section className="login-panel" aria-labelledby="login-title">
          <header className="login-header">
            <div className="login-brand">
              <h1 id="login-title">{t('app.brand')}</h1>
              <span aria-hidden="true" />
            </div>
            <p className="login-headline" aria-live="polite">
              <strong key={headlineIndex}>{headlines[headlineIndex]}</strong>
            </p>
          </header>
          {checking ? (
            <div className="login-loading"><Spin size="large" /></div>
          ) : (
            <Form<LoginValues> layout="vertical" onFinish={submit} requiredMark={false}>
              <Form.Item name="username" label={t('login.username')} rules={[{ required: true, message: t('login.usernameRequired') }]}>
                <Input size="large" prefix={<UserOutlined />} autoComplete="username" autoFocus />
              </Form.Item>
              <Form.Item name="password" label={t('login.password')} rules={[{ required: true, message: t('login.passwordRequired') }]}>
                <Input.Password size="large" prefix={<LockOutlined />} autoComplete="current-password" />
              </Form.Item>
              {twoFactorEnabled ? (
                <Form.Item name="twoFactorCode" label={t('login.twoFactor')} rules={[{ required: true, pattern: /^\d{6}$/, message: t('login.twoFactorRequired') }]}>
                  <Input size="large" prefix={<KeyOutlined />} autoComplete="one-time-code" inputMode="numeric" maxLength={6} />
                </Form.Item>
              ) : null}
              <Button type="primary" htmlType="submit" size="large" block loading={submitting}>{t('common.login')}</Button>
            </Form>
          )}
        </section>
      </main>
    </ConfigProvider>
  );
}
