import dayjs from 'dayjs';
import 'dayjs/locale/en';
import 'dayjs/locale/zh-cn';
import enUS from 'antd/locale/en_US';
import zhCN from 'antd/locale/zh_CN';
import type { LanguageCode } from './dictionaries';

const antdLocales = {
  'zh-CN': zhCN,
  'en-US': enUS,
} satisfies Record<LanguageCode, typeof zhCN>;

const dateLocales: Record<LanguageCode, string> = {
  'zh-CN': 'zh-cn',
  'en-US': 'en',
};

export function antdLocale(language: LanguageCode) {
  return antdLocales[language] || antdLocales['zh-CN'];
}

export function applyDateLocale(language: LanguageCode) {
  dayjs.locale(dateLocales[language] || dateLocales['zh-CN']);
}
