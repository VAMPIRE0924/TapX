import { describe, expect, it } from 'vitest';
import { defaultPanelName, panelDisplayName } from './panel-name';

describe('panel display name', () => {
  it('trims configured names and falls back to the product name', () => {
    expect(panelDisplayName('  Edge Panel  ')).toBe('Edge Panel');
    expect(panelDisplayName('')).toBe(defaultPanelName);
    expect(panelDisplayName(undefined)).toBe(defaultPanelName);
  });
});
