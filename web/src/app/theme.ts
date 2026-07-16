export type ThemeMode = 'light' | 'dark' | 'deep';

const darkStorageKey = 'dark-mode';
const ultraStorageKey = 'isUltraDarkThemeEnabled';

export function readTheme(): ThemeMode {
  try {
    const isDark = window.localStorage.getItem(darkStorageKey);
    const isUltra = window.localStorage.getItem(ultraStorageKey);
    if (isDark === 'false') return 'light';
    if (isUltra === 'true') return 'deep';
  } catch {
    // Browser storage is optional; dark remains the stable default.
  }
  return 'dark';
}

export function writeTheme(theme: ThemeMode): void {
  try {
    window.localStorage.setItem(darkStorageKey, String(theme !== 'light'));
    window.localStorage.setItem(ultraStorageKey, String(theme === 'deep'));
  } catch {
    // Applying a theme must not depend on persistent browser storage.
  }
  document.documentElement.dataset.uiTheme = theme;
}

export function nextTheme(theme: ThemeMode): ThemeMode {
  if (theme === 'light') return 'dark';
  if (theme === 'dark') return 'deep';
  return 'light';
}
