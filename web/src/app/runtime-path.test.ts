import { describe, expect, it } from 'vitest';
import { appPathname, normalizeBasePath, panelPath } from './runtime-path';

describe('runtime panel paths', () => {
  it('normalizes root and nested panel base paths', () => {
    expect(normalizeBasePath(undefined)).toBe('');
    expect(normalizeBasePath('/')).toBe('');
    expect(normalizeBasePath('tapx/')).toBe('/tapx');
    expect(normalizeBasePath('/tapx-secret///')).toBe('/tapx-secret');
  });

  it('prefixes browser and API paths under a configured panel path', () => {
    expect(panelPath('/api/config', '/tapx-secret')).toBe('/tapx-secret/api/config');
    expect(panelPath('/login.html', '/tapx-secret')).toBe('/tapx-secret/login.html');
    expect(appPathname('/tapx-secret/devices', '/tapx-secret')).toBe('/devices');
    expect(appPathname('/tapx-secret/', '/tapx-secret')).toBe('/');
  });

  it('leaves root-hosted paths unchanged', () => {
    expect(panelPath('/api/config', '')).toBe('/api/config');
    expect(appPathname('/devices', '')).toBe('/devices');
  });
});
