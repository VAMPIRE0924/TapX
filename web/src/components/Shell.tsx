import { useEffect, useMemo, useState, type ComponentType, type ReactNode } from 'react';
import { Drawer, Layout, Menu } from 'antd';
import type { MenuProps } from 'antd';
import {
  ApartmentOutlined,
  ClusterOutlined,
  CloseOutlined,
  CodeOutlined,
  DashboardOutlined,
  ExportOutlined,
  GithubOutlined,
  ImportOutlined,
  LogoutOutlined,
  MenuOutlined,
  MoonFilled,
  MoonOutlined,
  SafetyOutlined,
  SettingOutlined,
  SwapOutlined,
  SunOutlined,
  TeamOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { navItems, type NavKey } from '../app/navigation';
import { nextTheme, type ThemeMode } from '../app/theme';
import { useI18n } from '../i18n/I18nProvider';
import { logoutPanel } from '../shared/api';
import './Shell.css';
import { panelPath } from '../app/runtime-path';

const collapsedStorageKey = 'isSidebarCollapsed';
const repoUrl = 'https://github.com/VAMPIRE0924/TapX';
const logoutKey = '__logout__';

type IconName = 'dashboard' | 'node' | 'device' | 'listener' | 'user' | 'connector' | 'link' | 'setting' | 'kernel' | 'logout';

const iconByName: Record<IconName, ComponentType> = {
  dashboard: DashboardOutlined,
  node: ClusterOutlined,
  device: ApartmentOutlined,
  listener: ImportOutlined,
  user: TeamOutlined,
  connector: ExportOutlined,
  link: SwapOutlined,
  setting: SettingOutlined,
  kernel: ToolOutlined,
  logout: LogoutOutlined,
};

function readCollapsed(): boolean {
  try {
    return window.localStorage.getItem(collapsedStorageKey) === 'true';
  } catch {
    return false;
  }
}

interface ShellProps {
  active: NavKey;
  theme: ThemeMode;
  currentPath: string;
  children: ReactNode;
  onThemeChange: (theme: ThemeMode) => void;
  onNavigate: (path: string) => void;
}

const navIconNames: Record<NavKey, IconName> = {
  dashboard: 'dashboard',
  nodes: 'node',
  devices: 'device',
  listeners: 'listener',
  users: 'user',
  connectors: 'connector',
  links: 'link',
  settings: 'setting',
  kernels: 'kernel',
};

function ThemeIcon({ theme }: { theme: ThemeMode }) {
  if (theme === 'light') return <SunOutlined />;
  if (theme === 'dark') return <MoonOutlined />;
  return <MoonFilled />;
}

function ThemeCycleButton({ id, theme, onCycle }: { id: string; theme: ThemeMode; onCycle: () => void }) {
  const { t } = useI18n();
  return (
    <button
      id={id}
      type="button"
      className="sidebar-theme-cycle"
      aria-label={t('common.theme')}
      title={t('common.theme')}
      onClick={onCycle}
    >
      <ThemeIcon theme={theme} />
    </button>
  );
}

function VersionBadge({ collapsed }: { collapsed?: boolean }) {
  const { t } = useI18n();
  return (
    <a
      href={repoUrl}
      target="_blank"
      rel="noopener noreferrer"
      className={`sider-version${collapsed ? ' is-collapsed' : ''}`}
      aria-label={`GitHub ${t('app.version')}`}
      title={t('app.version')}
    >
      <GithubOutlined />
      {!collapsed && <span className="sider-version-text">{t('app.version')}</span>}
    </a>
  );
}

export function Shell({ active, theme, currentPath, children, onThemeChange, onNavigate }: ShellProps) {
  const { t } = useI18n();
  const [collapsed, setCollapsed] = useState(readCollapsed);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const antTheme = theme === 'light' ? 'light' : 'dark';
  const selectedKey = selectedMenuKey(active, currentPath);
  const openSubmenu = active === 'settings' ? '/settings' : active === 'kernels' ? '/kernels' : null;
  const [openKeys, setOpenKeys] = useState<string[]>(() => (openSubmenu ? [openSubmenu] : []));

  useEffect(() => {
    if (!openSubmenu) return;
    setOpenKeys((current) => current.includes(openSubmenu) ? current : [openSubmenu]);
  }, [openSubmenu]);

  const settingsChildren = useMemo<NonNullable<MenuProps['items']>>(() => [
    { key: '/settings#general', icon: <SettingOutlined />, label: t('settings.general') },
    { key: '/settings#certificate', icon: <SafetyOutlined />, label: t('settings.certificate') },
    { key: '/settings#security', icon: <SafetyOutlined />, label: t('settings.security') },
    { key: '/settings#timezone', icon: <SettingOutlined />, label: t('settings.timezone') },
  ], [t]);

  const kernelChildren = useMemo<NonNullable<MenuProps['items']>>(() => [
    { key: '/kernels#kernel', icon: <SettingOutlined />, label: t('kernel.builtin') },
    { key: '/kernels#external', icon: <ToolOutlined />, label: t('kernel.external') },
    { key: '/kernels#advanced', icon: <CodeOutlined />, label: t('kernel.advanced') },
  ], [t]);

  const menuItems = useMemo<MenuProps['items']>(() => navItems.map((item) => {
    const Icon = iconByName[navIconNames[item.key]];
    if (item.key === 'settings') {
      return { key: item.path, icon: <Icon />, label: t(item.labelKey), children: settingsChildren };
    }
    if (item.key === 'kernels') {
      return { key: item.path, icon: <Icon />, label: t(item.labelKey), children: kernelChildren };
    }
    return { key: item.path, icon: <Icon />, label: t(item.labelKey) };
  }), [kernelChildren, settingsChildren, t]);

  const utilityItems = useMemo<MenuProps['items']>(() => {
    const Icon = iconByName.logout;
    return [{ key: logoutKey, icon: <Icon />, label: t('common.logout') }];
  }, [t]);

  function handleCollapse(next: boolean, type: 'clickTrigger' | 'responsive') {
    if (type === 'clickTrigger') {
      try {
        window.localStorage.setItem(collapsedStorageKey, String(next));
      } catch {
        // Sidebar interaction remains available when storage is blocked.
      }
      setCollapsed(next);
    }
  }

  function handleMenuClick(key: string) {
    if (key === logoutKey) {
      void logoutPanel().finally(() => window.location.replace(panelPath('/login.html')));
      return;
    }
    onNavigate(key);
  }

  function cycleTheme() {
    onThemeChange(nextTheme(theme));
  }

  return (
    <Layout className="tapx-layout">
      <div className="ant-sidebar">
        <Layout.Sider
          theme={antTheme}
          width={220}
          collapsible
          collapsed={collapsed}
          breakpoint="md"
          onCollapse={handleCollapse}
        >
          <div className={`sider-brand${collapsed ? ' sider-brand-collapsed' : ''}`}>
            <div className="brand-block">
              <span className="brand-text">{collapsed ? t('app.shortBrand') : t('app.brand')}</span>
            </div>
            {!collapsed && (
              <div className="brand-actions">
                <ThemeCycleButton id="theme-cycle" theme={theme} onCycle={cycleTheme} />
              </div>
            )}
          </div>
          <Menu
            theme={antTheme}
            mode="inline"
            selectedKeys={[selectedKey]}
            openKeys={collapsed ? undefined : openKeys}
            onOpenChange={(keys) => setOpenKeys(keys as string[])}
            className="sider-nav"
            items={menuItems}
            onClick={({ key }) => handleMenuClick(String(key))}
          />
          <Menu
            theme={antTheme}
            mode="inline"
            selectedKeys={[selectedKey]}
            className="sider-utility"
            items={utilityItems}
            onClick={({ key }) => handleMenuClick(String(key))}
          />
          <div className="sider-footer">
            <VersionBadge collapsed={collapsed} />
          </div>
        </Layout.Sider>

        <Drawer
          placement="left"
          closable={false}
          open={drawerOpen}
          rootClassName={antTheme}
          size="min(82vw, 320px)"
          styles={{
            wrapper: { padding: 0 },
            body: { padding: 0, display: 'flex', flexDirection: 'column', height: '100%' },
            header: { display: 'none' },
          }}
          onClose={() => setDrawerOpen(false)}
        >
          <div className="drawer-header">
            <div className="brand-block">
              <span className="drawer-brand">{t('app.brand')}</span>
            </div>
            <div className="drawer-header-actions">
              <ThemeCycleButton id="theme-cycle-drawer" theme={theme} onCycle={cycleTheme} />
              <button className="drawer-close" type="button" aria-label={t('common.close')} onClick={() => setDrawerOpen(false)}>
                <CloseOutlined />
              </button>
            </div>
          </div>
          <Menu
            theme={antTheme}
            mode="inline"
            selectedKeys={[selectedKey]}
            openKeys={openKeys}
            onOpenChange={(keys) => setOpenKeys(keys as string[])}
            className="drawer-menu drawer-nav"
            items={menuItems}
            onClick={({ key }) => {
              handleMenuClick(String(key));
              setDrawerOpen(false);
            }}
          />
          <Menu
            theme={antTheme}
            mode="inline"
            selectedKeys={[selectedKey]}
            className="drawer-menu drawer-utility"
            items={utilityItems}
            onClick={({ key }) => {
              handleMenuClick(String(key));
              setDrawerOpen(false);
            }}
          />
          <div className="drawer-footer">
            <VersionBadge />
          </div>
        </Drawer>

        {!drawerOpen && (
          <button className="drawer-handle" type="button" aria-label={t('common.openMenu')} onClick={() => setDrawerOpen(true)}>
            <MenuOutlined />
          </button>
        )}
      </div>
      <Layout className="content-shell">
        <Layout.Content className="content-area">{children}</Layout.Content>
      </Layout>
    </Layout>
  );
}

function selectedMenuKey(active: NavKey, currentPath: string): string {
  if (active === 'settings') return `/settings${hashOrDefault(currentPath, '#general')}`;
  if (active === 'kernels') return `/kernels${hashOrDefault(currentPath, '#kernel')}`;
  return navItems.find((item) => item.key === active)?.path || '/';
}

function hashOrDefault(path: string, fallback: string): string {
  const index = path.indexOf('#');
  if (index < 0) return fallback;
  return path.slice(index) || fallback;
}
