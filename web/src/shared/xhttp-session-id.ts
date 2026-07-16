import type { TranslationKey, TranslationValues } from '../i18n/dictionaries';

type Translate = (key: TranslationKey, values?: TranslationValues) => string;

export function validateSessionIDTable(_rule: unknown, value: unknown, t?: Translate): Promise<void> {
  const text = typeof value === 'string' ? value : '';
  if (!text) return Promise.resolve();
  // Xray requires the custom session ID character table to be ASCII-only.
  // eslint-disable-next-line no-control-regex
  if (/[^\x00-\x7f]/.test(text)) {
    return Promise.reject(new Error(t ? t('xray.sessionIdAscii') : 'sessionIDTable must contain ASCII characters only'));
  }
  return Promise.resolve();
}

export function validateSessionIDLength(_rule: unknown, value: unknown, t?: Translate): Promise<void> {
  const text = typeof value === 'string' ? value.trim() : '';
  if (!text) return Promise.resolve();
  if (!/^\d+(?:-\d+)?$/.test(text)) {
    return Promise.reject(new Error(t ? t('xray.sessionIdLengthInvalid') : 'Enter a length or range, for example 8 or 8-16'));
  }
  const [fromText, toText] = text.split('-');
  const from = Number(fromText);
  const to = toText == null ? from : Number(toText);
  if (!Number.isFinite(from) || from <= 0) {
    return Promise.reject(new Error(t ? t('xray.sessionIdLengthPositive') : 'sessionIDLength minimum must be greater than 0'));
  }
  if (!Number.isFinite(to) || to < from) {
    return Promise.reject(new Error(t ? t('xray.sessionIdLengthOrder') : 'sessionIDLength upper bound must be greater than or equal to the lower bound'));
  }
  return Promise.resolve();
}
