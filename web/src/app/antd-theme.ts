import { theme as antdTheme, type ThemeConfig } from 'antd';
import type { ThemeMode } from './theme';

const statisticTokens = {
  contentFontSize: 17,
  titleFontSize: 11,
};

const darkTokens = {
  colorBgBase: '#1a1b1f',
  colorBgLayout: '#1a1b1f',
  colorBgContainer: '#23252b',
  colorBgElevated: '#2d2f37',
};

const deepTokens = {
  colorBgBase: '#000',
  colorBgLayout: '#000',
  colorBgContainer: '#101013',
  colorBgElevated: '#1a1a1e',
};

const darkLayoutTokens = {
  bodyBg: '#1a1b1f',
  headerBg: '#15161a',
  headerColor: '#ffffff',
  footerBg: '#1a1b1f',
  siderBg: '#15161a',
  triggerBg: '#23252b',
  triggerColor: '#ffffff',
};

const deepLayoutTokens = {
  bodyBg: '#000',
  headerBg: '#050507',
  headerColor: '#ffffff',
  footerBg: '#000',
  siderBg: '#050507',
  triggerBg: '#1a1a1e',
  triggerColor: '#ffffff',
};

const darkMenuTokens = {
  darkItemBg: '#15161a',
  darkSubMenuItemBg: '#1a1b1f',
  darkPopupBg: '#23252b',
};

const deepMenuTokens = {
  darkItemBg: '#050507',
  darkSubMenuItemBg: '#000',
  darkPopupBg: '#101013',
};

export function buildThemeConfig(mode: ThemeMode): ThemeConfig {
  if (mode === 'light') {
    return {
      algorithm: antdTheme.defaultAlgorithm,
      components: {
        Statistic: statisticTokens,
      },
    };
  }

  const deep = mode === 'deep';
  return {
    algorithm: antdTheme.darkAlgorithm,
    token: deep ? deepTokens : darkTokens,
    components: {
      Layout: deep ? deepLayoutTokens : darkLayoutTokens,
      Menu: deep ? deepMenuTokens : darkMenuTokens,
      Card: {
        colorBorderSecondary: deep ? 'rgba(255, 255, 255, 0.04)' : 'rgba(255, 255, 255, 0.06)',
      },
      Statistic: statisticTokens,
    },
  };
}
