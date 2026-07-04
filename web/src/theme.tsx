import { createContext, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { theme as antdTheme } from 'antd';
import type { ThemeConfig } from 'antd';

const STORAGE_DARK = 'tapx-dark-mode';
const STORAGE_ULTRA = 'tapx-ultra-dark-mode';

function readBool(key: string, fallback: boolean) {
  const raw = localStorage.getItem(key);
  if (raw == null) return fallback;
  return raw === 'true';
}

function applyDom(isDark: boolean, isUltra: boolean) {
  document.body.className = isDark ? 'dark' : 'light';
  document.documentElement.toggleAttribute('data-theme-ultra', isUltra);
}

interface ThemeValue {
  isDark: boolean;
  isUltra: boolean;
  toggleTheme: () => void;
  toggleUltra: () => void;
  antdThemeConfig: ThemeConfig;
}

const ThemeContext = createContext<ThemeValue | null>(null);

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [isDark, setDark] = useState(() => readBool(STORAGE_DARK, false));
  const [isUltra, setUltra] = useState(() => readBool(STORAGE_ULTRA, false));

  useEffect(() => {
    applyDom(isDark, isUltra);
    localStorage.setItem(STORAGE_DARK, String(isDark));
    localStorage.setItem(STORAGE_ULTRA, String(isUltra));
  }, [isDark, isUltra]);

  const antdThemeConfig = useMemo<ThemeConfig>(() => {
    if (!isDark) return { algorithm: antdTheme.defaultAlgorithm };
    return {
      algorithm: antdTheme.darkAlgorithm,
      token: isUltra
        ? { colorBgBase: '#000000', colorBgLayout: '#000000', colorBgContainer: '#101013' }
        : { colorBgBase: '#1a1b1f', colorBgLayout: '#1a1b1f', colorBgContainer: '#23252b' },
      components: {
        Layout: {
          siderBg: isUltra ? '#050507' : '#15161a',
          triggerBg: isUltra ? '#101013' : '#23252b',
        },
        Menu: {
          darkItemBg: isUltra ? '#050507' : '#15161a',
          darkSubMenuItemBg: isUltra ? '#000000' : '#1a1b1f',
        },
      },
    };
  }, [isDark, isUltra]);

  const value = useMemo<ThemeValue>(() => ({
    isDark,
    isUltra,
    toggleTheme: () => setDark((v) => !v),
    toggleUltra: () => setUltra((v) => !v),
    antdThemeConfig,
  }), [antdThemeConfig, isDark, isUltra]);

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used inside ThemeProvider');
  return ctx;
}
