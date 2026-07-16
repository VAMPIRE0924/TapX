import { lazy, Suspense, useEffect, useState } from 'react';
import { ConfigProvider, Spin } from 'antd';
import { Shell } from '../components/Shell';
import { navKeyFromPath, type NavKey } from './navigation';
import { buildThemeConfig } from './antd-theme';
import { readTheme, writeTheme, type ThemeMode } from './theme';
import { getAuthSession, getRuntimeConfig } from '../shared/api';
import { settingsToObject } from '../shared/settings';
import { useI18n } from '../i18n/I18nProvider';
import type { LanguageCode } from '../i18n/dictionaries';
import { antdLocale } from '../i18n/locales';
import { appLocation, appPathname, panelPath } from './runtime-path';

const DashboardPage = lazy(() => import('../pages/DashboardPage').then((module) => ({ default: module.DashboardPage })));
const NodePage = lazy(() => import('../pages/NodePage').then((module) => ({ default: module.NodePage })));
const DevicePage = lazy(() => import('../pages/DevicePage').then((module) => ({ default: module.DevicePage })));
const ListenerPage = lazy(() => import('../pages/ListenerPage').then((module) => ({ default: module.ListenerPage })));
const UserPage = lazy(() => import('../pages/UserPage').then((module) => ({ default: module.UserPage })));
const ConnectorPage = lazy(() => import('../pages/ConnectorPage').then((module) => ({ default: module.ConnectorPage })));
const LinkBindingPage = lazy(() => import('../pages/LinkBindingPage').then((module) => ({ default: module.LinkBindingPage })));
const SettingsPage = lazy(() => import('../pages/SettingsPage').then((module) => ({ default: module.SettingsPage })));
const KernelPage = lazy(() => import('../pages/KernelPage').then((module) => ({ default: module.KernelPage })));

export function App() {
  const { language, setLanguage } = useI18n();
  const [theme, setTheme] = useState<ThemeMode>(() => readTheme());
  const [active, setActive] = useState<NavKey>(() => navKeyFromPath(appPathname()));
  const [currentPath, setCurrentPath] = useState(appLocation);
  const [sessionReady, setSessionReady] = useState(false);

  useEffect(() => {
    let active = true;
    getAuthSession()
      .then(async (session) => {
        if (!active) return;
        if (session.authEnabled && !session.authenticated) {
          window.location.replace(panelPath('/login.html'));
          return;
        }
        try {
          const config = await getRuntimeConfig();
          const stored = settingsToObject<{ language?: LanguageCode }>(config.Settings);
          if (active && stored.language) setLanguage(stored.language);
        } catch {
          // The panel can still open when preferences are temporarily unavailable.
        }
        if (!active) return;
        setSessionReady(true);
      })
      .catch(() => {
        if (active) setSessionReady(true);
      });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    writeTheme(theme);
  }, [theme]);

  useEffect(() => {
    const handlePop = () => {
      setActive(navKeyFromPath(appPathname()));
      setCurrentPath(appLocation());
    };
    window.addEventListener('popstate', handlePop);
    window.addEventListener('hashchange', handlePop);
    return () => {
      window.removeEventListener('popstate', handlePop);
      window.removeEventListener('hashchange', handlePop);
    };
  }, []);

  function navigate(path: string) {
    window.history.pushState({}, '', panelPath(path));
    setActive(navKeyFromPath(path));
    setCurrentPath(path);
  }

  if (!sessionReady) {
    return <div className="page-loading"><Spin size="large" /></div>;
  }

  return (
    <ConfigProvider theme={buildThemeConfig(theme)} locale={antdLocale(language)}>
      <Shell active={active} theme={theme} currentPath={currentPath} onThemeChange={setTheme} onNavigate={navigate}>
        <Suspense fallback={<div className="page-loading"><Spin size="large" /></div>}>
          {active === 'dashboard'
            ? <DashboardPage />
            : active === 'nodes'
              ? <NodePage />
              : active === 'devices'
                ? <DevicePage />
                : active === 'listeners'
                ? <ListenerPage />
                : active === 'users'
                  ? <UserPage />
                  : active === 'connectors'
                    ? <ConnectorPage />
                    : active === 'links'
                      ? <LinkBindingPage />
                      : active === 'settings'
                        ? <SettingsPage currentPath={currentPath} />
                        : active === 'kernels'
                          ? <KernelPage currentPath={currentPath} />
                          : <DashboardPage />}
        </Suspense>
      </Shell>
    </ConfigProvider>
  );
}
