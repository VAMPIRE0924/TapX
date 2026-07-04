import { useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Button, Drawer, Layout, Menu, Select } from 'antd';
import type { MenuProps } from 'antd';
import {
  ApiOutlined,
  ApartmentOutlined,
  CloseOutlined,
  CloudServerOutlined,
  DashboardOutlined,
  ExportOutlined,
  GlobalOutlined,
  ImportOutlined,
  KeyOutlined,
  MenuOutlined,
  MoonOutlined,
  PoweroffOutlined,
  SafetyOutlined,
  SettingOutlined,
  SunOutlined,
  SwapOutlined,
  TeamOutlined,
  ToolOutlined,
} from '@ant-design/icons';

import { useI18n } from '@/i18n';
import { useTheme } from '@/theme';
import './AppSidebar.css';

const LOGOUT_KEY = '__logout__';

interface AppSidebarProps {
  onLogout: () => Promise<void>;
}

export function AppSidebar({ onLogout }: AppSidebarProps) {
  const { t, lang, setLang } = useI18n();
  const { isDark, toggleTheme } = useTheme();
  const navigate = useNavigate();
  const location = useLocation();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(false);

  const items = useMemo<MenuProps['items']>(() => [
    { key: '/', icon: <DashboardOutlined />, label: t('dashboard') },
    { key: '/listeners', icon: <ImportOutlined />, label: t('listeners') },
    { key: '/clients', icon: <TeamOutlined />, label: t('clients') },
    { key: '/connectors', icon: <ExportOutlined />, label: t('connectors') },
    { key: '/routes', icon: <SwapOutlined />, label: t('routes') },
    { key: '/devices', icon: <ApartmentOutlined />, label: t('devices') },
    { key: '/vkeys', icon: <KeyOutlined />, label: t('vkeys') },
    { key: '/addresses', icon: <SafetyOutlined />, label: t('addressLimits') },
    {
      key: '/xray',
      icon: <ToolOutlined />,
      label: t('xray'),
      children: [
        { key: '/xray#profiles', icon: <CloudServerOutlined />, label: 'Profiles' },
        { key: '/xray#binary', icon: <ApiOutlined />, label: t('xrayBinary') },
        { key: '/xray#template', icon: <GlobalOutlined />, label: 'Template' },
      ],
    },
    { key: '/settings', icon: <SettingOutlined />, label: t('settings') },
    { key: '/system', icon: <PoweroffOutlined />, label: t('system') },
    { type: 'divider' },
    { key: LOGOUT_KEY, icon: <PoweroffOutlined />, label: t('logout') },
  ], [t]);

  async function click(key: string) {
    if (key === LOGOUT_KEY) {
      await onLogout();
      return;
    }
    navigate(key);
    setDrawerOpen(false);
  }

  const selectedKey = location.pathname === '/xray'
    ? `/xray${location.hash || '#profiles'}`
    : location.pathname || '/';

  const menu = (
    <>
      <div className={`sider-brand${collapsed ? ' sider-brand-collapsed' : ''}`}>
        <div className="brand-block">
          <span className="brand-mark">TX</span>
          {!collapsed && <span className="brand-text">TapX</span>}
        </div>
        {!collapsed && (
          <div className="brand-actions">
            <Select
              size="small"
              value={lang}
              popupMatchSelectWidth={false}
              options={[{ value: 'zh', label: '中文' }, { value: 'en', label: 'EN' }]}
              onChange={(value) => setLang(value)}
            />
            <Button size="small" shape="circle" icon={isDark ? <MoonOutlined /> : <SunOutlined />} onClick={toggleTheme} />
          </div>
        )}
      </div>
      <Menu
        theme={isDark ? 'dark' : 'light'}
        mode="inline"
        selectedKeys={[selectedKey]}
        defaultOpenKeys={location.pathname === '/xray' ? ['/xray'] : []}
        items={items}
        className="sider-nav"
        onClick={({ key }) => click(String(key))}
      />
    </>
  );

  return (
    <div className="tapx-sidebar">
      <Layout.Sider
        theme={isDark ? 'dark' : 'light'}
        width={236}
        collapsible
        collapsed={collapsed}
        breakpoint="md"
        onCollapse={(next) => setCollapsed(next)}
      >
        {menu}
      </Layout.Sider>
      <Button className="drawer-handle" shape="circle" icon={<MenuOutlined />} onClick={() => setDrawerOpen(true)} />
      <Drawer
        placement="left"
        closable={false}
        open={drawerOpen}
        width="min(84vw, 320px)"
        styles={{ body: { padding: 0 } }}
        onClose={() => setDrawerOpen(false)}
      >
        <div className="drawer-header">
          <span className="drawer-brand">TapX</span>
          <Button shape="circle" icon={<CloseOutlined />} onClick={() => setDrawerOpen(false)} />
        </div>
        {menu}
      </Drawer>
    </div>
  );
}
