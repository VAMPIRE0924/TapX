import { zhCN } from './messages/zh-CN';
import { enUS } from './messages/en-US';

export { zhCN };

export const languageRegistry = {
  'zh-CN': { nativeName: '中文', antd: 'zh-CN', dayjs: 'zh-cn' },
  'en-US': { nativeName: 'English', antd: 'en-US', dayjs: 'en' },
} as const;

export type LanguageCode = keyof typeof languageRegistry;

export const fallbackLanguage: LanguageCode = 'zh-CN';

export const languageOptions: Array<{ value: LanguageCode; label: string }> = Object.entries(languageRegistry)
  .map(([value, metadata]) => ({ value: value as LanguageCode, label: metadata.nativeName }));

export type TranslationKey = keyof typeof zhCN;
export type Dictionary = Record<TranslationKey, string>;

export const dictionaries: Record<LanguageCode, Dictionary> = {
  'zh-CN': zhCN,
  'en-US': enUS,
};

export type TranslationValues = Record<string, string | number>;

export function translate(language: LanguageCode, key: TranslationKey, values?: TranslationValues): string {
  const template = dictionaries[language]?.[key] || dictionaries[fallbackLanguage][key] || key;
  if (!values) return template;
  return template.replace(/\{([A-Za-z0-9_]+)\}/g, (match, name: string) => (
    Object.prototype.hasOwnProperty.call(values, name) ? String(values[name]) : match
  ));
}
