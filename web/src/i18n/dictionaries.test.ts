import { describe, expect, it } from 'vitest';
import { dictionaries, fallbackLanguage, languageOptions, translate } from './dictionaries';

describe('i18n registry', () => {
  it('exposes every registered language in the selector', () => {
    expect(languageOptions.map((item) => item.value)).toEqual(['zh-CN', 'en-US']);
  });

  it('translates keys and interpolates values', () => {
    expect(translate('en-US', 'dashboard.devices')).toBe('Devices');
    expect(translate('en-US', 'listener.fallback.destination', { port: 443 })).toContain('Automatic');
  });

  it('keeps every registered language complete', () => {
    const requiredKeys = Object.keys(dictionaries[fallbackLanguage]);
    for (const { value } of languageOptions) {
      const missing = requiredKeys.filter((key) => !dictionaries[value]?.[key as keyof typeof dictionaries[typeof fallbackLanguage]]);
      expect(missing, `${value} is missing translation keys`).toEqual([]);
    }
  });

  it('does not mention removed user-level Flow controls', () => {
    expect(translate('zh-CN', 'user.bulk.adjustHelp')).not.toContain('Flow');
    expect(translate('en-US', 'user.bulk.adjustHelp')).not.toContain('Flow');
  });

  it('does not describe removed sharing features in listener help', () => {
    expect(translate('zh-CN', 'listener.shareAddressStrategyHelp')).not.toMatch(/二维码|订阅/);
    expect(translate('en-US', 'listener.shareAddressStrategyHelp')).not.toMatch(/QR|subscription/i);
  });
});
