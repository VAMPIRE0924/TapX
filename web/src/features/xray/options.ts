import type { TranslationKey, TranslationValues } from '../../i18n/dictionaries';

type Translate = (key: TranslationKey, values?: TranslationValues) => string;

export function getTlsVersionOptions(t: Translate) {
  return ['', '1.0', '1.1', '1.2', '1.3'].map((value) => ({ value, label: value || t('xray.default') }));
}

const utlsValues = [
  '',
  'chrome',
  'firefox',
  'safari',
  'ios',
  'android',
  'edge',
  '360',
  'qq',
  'random',
  'randomized',
  'randomizednoalpn',
  'unsafe',
];

export function getUtlsOptions(t: Translate) {
  return utlsValues.map((value) => ({ value, label: value || t('xray.none') }));
}

export const alpnOptions = ['h3', 'h2', 'http/1.1'].map((value) => ({ value, label: value }));

const tlsCipherValues = [
  { value: '', label: '' },
  { value: 'TLS_AES_128_GCM_SHA256', label: 'AES_128_GCM' },
  { value: 'TLS_AES_256_GCM_SHA384', label: 'AES_256_GCM' },
  { value: 'TLS_CHACHA20_POLY1305_SHA256', label: 'CHACHA20_POLY1305' },
  { value: 'TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA', label: 'ECDHE_ECDSA_AES_128_CBC' },
  { value: 'TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA', label: 'ECDHE_ECDSA_AES_256_CBC' },
  { value: 'TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA', label: 'ECDHE_RSA_AES_128_CBC' },
  { value: 'TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA', label: 'ECDHE_RSA_AES_256_CBC' },
  { value: 'TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256', label: 'ECDHE_ECDSA_AES_128_GCM' },
  { value: 'TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384', label: 'ECDHE_ECDSA_AES_256_GCM' },
  { value: 'TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256', label: 'ECDHE_RSA_AES_128_GCM' },
  { value: 'TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384', label: 'ECDHE_RSA_AES_256_GCM' },
  { value: 'TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256', label: 'ECDHE_ECDSA_CHACHA20_POLY1305' },
  { value: 'TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256', label: 'ECDHE_RSA_CHACHA20_POLY1305' },
];

export function getTlsCipherOptions(t: Translate) {
  return tlsCipherValues.map((option) => option.value ? option : { ...option, label: t('xray.auto') });
}

export const tlsUsageOptions = ['encipherment', 'verify', 'issue'].map((value) => ({ value, label: value }));

export const targetStrategyOptions = [
  'AsIs',
  'UseIP',
  'UseIPv6v4',
  'UseIPv6',
  'UseIPv4v6',
  'UseIPv4',
  'ForceIP',
  'ForceIPv6v4',
  'ForceIPv6',
  'ForceIPv4v6',
  'ForceIPv4',
].map((value) => ({ value, label: value }));
