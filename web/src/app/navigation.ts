import type { TranslationKey } from '../i18n/dictionaries';

export type NavKey =
  | 'dashboard'
  | 'nodes'
  | 'devices'
  | 'listeners'
  | 'users'
  | 'connectors'
  | 'links'
  | 'settings'
  | 'kernels';

export interface NavItem {
  key: NavKey;
  labelKey: TranslationKey;
  path: string;
}

export const navItems: NavItem[] = [
  { key: 'dashboard', labelKey: 'menu.dashboard', path: '/' },
  { key: 'nodes', labelKey: 'menu.nodes', path: '/nodes' },
  { key: 'devices', labelKey: 'menu.devices', path: '/devices' },
  { key: 'listeners', labelKey: 'menu.listeners', path: '/listeners' },
  { key: 'users', labelKey: 'menu.users', path: '/users' },
  { key: 'connectors', labelKey: 'menu.connectors', path: '/connectors' },
  { key: 'links', labelKey: 'menu.links', path: '/links' },
  { key: 'settings', labelKey: 'menu.settings', path: '/settings' },
  { key: 'kernels', labelKey: 'menu.kernels', path: '/kernels' },
];

export function navKeyFromPath(pathname: string): NavKey {
  const matched = navItems.find((item) => item.path !== '/' && pathname.startsWith(item.path));
  return matched?.key || 'dashboard';
}
