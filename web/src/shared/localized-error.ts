import type { TranslationKey, TranslationValues } from '../i18n/dictionaries';

type Translate = (key: TranslationKey, values?: TranslationValues) => string;

export class LocalizedError extends Error {
  constructor(readonly translationKey: TranslationKey, readonly values?: TranslationValues) {
    super(translationKey);
    this.name = 'LocalizedError';
  }
}

export function errorMessage(error: unknown, t: Translate, fallback: TranslationKey): string {
  if (error instanceof LocalizedError) return t(error.translationKey, error.values);
  if (error instanceof Error) return error.message;
  return t(fallback);
}
