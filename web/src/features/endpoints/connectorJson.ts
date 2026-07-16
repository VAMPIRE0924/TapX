import type { TranslationKey, TranslationValues } from '../../i18n/dictionaries';

type Translate = (key: TranslationKey, values?: TranslationValues) => string;

export function mergeConnectorJson<T extends object>(base: T, text: string, t?: Translate): T {
  const parsed = JSON.parse(text) as unknown;
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error(t ? t('connector.jsonObjectRequired') : 'Connector JSON must be an object');
  }
  return Object.assign({}, base, parsed) as T;
}

export function connectorIDConflicts(id: string, editingID: string | undefined, existingIDs: string[]): boolean {
  return existingIDs.some((existingID) => existingID === id && existingID !== editingID);
}
