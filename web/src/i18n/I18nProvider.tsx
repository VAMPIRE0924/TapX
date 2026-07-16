import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { fallbackLanguage, languageRegistry, translate, type LanguageCode, type TranslationKey, type TranslationValues } from './dictionaries';
import { applyDateLocale } from './locales';

const languageStorageKey = 'tapx.language';

interface I18nContextValue {
  language: LanguageCode;
  setLanguage: (language: LanguageCode) => void;
  t: (key: TranslationKey, values?: TranslationValues) => string;
}

const I18nContext = createContext<I18nContextValue | null>(null);

export function I18nProvider({ children }: { children: ReactNode }) {
  const [language, setLanguageState] = useState<LanguageCode>(readLanguage);

  const setLanguage = useCallback((next: LanguageCode) => {
    setLanguageState(next);
    try {
      window.localStorage.setItem(languageStorageKey, next);
    } catch {
      // Language persistence is optional; UI still works without localStorage.
    }
  }, []);

  useEffect(() => {
    applyDateLocale(language);
    document.documentElement.lang = language;
  }, [language]);

  const t = useCallback((key: TranslationKey, values?: TranslationValues) => {
    return translate(language, key, values);
  }, [language]);

  const value = useMemo(() => ({ language, setLanguage, t }), [language, setLanguage, t]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) throw new Error('useI18n must be used inside I18nProvider');
  return context;
}

function readLanguage(): LanguageCode {
  try {
    const stored = window.localStorage.getItem(languageStorageKey);
    if (stored && stored in languageRegistry) return stored as LanguageCode;
  } catch {
    return fallbackLanguage;
  }
  return fallbackLanguage;
}
