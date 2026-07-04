import { createContext, useContext, useMemo, useState } from 'react';
import type { ReactNode } from 'react';

type Lang = 'en' | 'zh';
type Dict = Record<string, string>;

const en: Dict = {
  add: 'Add',
  advanced: 'Advanced',
  addressLimits: 'Address Limits',
  apply: 'Apply',
  backup: 'Backup',
  cancel: 'Cancel',
  clients: 'Clients',
  clone: 'Clone',
  configJson: 'Config JSON',
  connectors: 'Outbounds',
  dashboard: 'Dashboard',
  delete: 'Delete',
  devices: 'Devices',
  diagnostics: 'Diagnostics',
  download: 'Download',
  edit: 'Edit',
  enabled: 'Enabled',
  export: 'Export',
  import: 'Import',
  language: 'Language',
  listeners: 'Inbounds',
  logs: 'Logs',
  login: 'Sign In',
  logout: 'Logout',
  noData: 'No data',
  objects: 'Objects',
  password: 'Password',
  rawTcp: 'Raw TCP',
  rawUdp: 'Raw UDP',
  refresh: 'Refresh',
  routes: 'Routes',
  runtime: 'Runtime',
  save: 'Save',
  settings: 'Settings',
  start: 'Start',
  status: 'Status',
  stop: 'Stop',
  system: 'System',
  templates: 'Templates',
  upload: 'Upload',
  username: 'Username',
  validate: 'Validate',
  view: 'View',
  vkeys: 'vKeys',
  xray: 'Xray',
  xrayBinary: 'Xray Binary',
};

const zh: Dict = {
  add: '新增',
  advanced: '高级',
  addressLimits: '地址限制',
  apply: '应用',
  backup: '备份',
  cancel: '取消',
  clients: '客户端',
  clone: '复制',
  configJson: '配置 JSON',
  connectors: '出站',
  dashboard: '仪表盘',
  delete: '删除',
  devices: '设备',
  diagnostics: '诊断',
  download: '下载',
  edit: '编辑',
  enabled: '启用',
  export: '导出',
  import: '导入',
  language: '语言',
  listeners: '入站',
  logs: '日志',
  login: '登录',
  logout: '退出',
  noData: '暂无数据',
  objects: '对象',
  password: '密码',
  rawTcp: '裸 TCP',
  rawUdp: '裸 UDP',
  refresh: '刷新',
  routes: '路由',
  runtime: '运行时',
  save: '保存',
  settings: '设置',
  start: '启动',
  status: '状态',
  stop: '停止',
  system: '系统',
  templates: '模板',
  upload: '上传',
  username: '用户名',
  validate: '校验',
  view: '查看',
  vkeys: 'vKey',
  xray: 'Xray',
  xrayBinary: 'Xray 内核',
};

interface I18nValue {
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: (key: string) => string;
}

const I18nContext = createContext<I18nValue | null>(null);

function initialLang(): Lang {
  const stored = localStorage.getItem('tapx-panel-lang');
  if (stored === 'en' || stored === 'zh') return stored;
  return navigator.language.toLowerCase().startsWith('zh') ? 'zh' : 'en';
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(() => initialLang());
  const dict = lang === 'zh' ? zh : en;
  const value = useMemo<I18nValue>(() => ({
    lang,
    setLang(next) {
      localStorage.setItem('tapx-panel-lang', next);
      setLangState(next);
    },
    t(key) {
      return dict[key] || key;
    },
  }), [dict, lang]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error('useI18n must be used inside I18nProvider');
  return ctx;
}
