import { describe, expect, it } from 'vitest';
import type { PanelLogEvent } from '../../shared/api';
import { logMatchesScope } from './DashboardDialogs';

function event(action: string, message: string): PanelLogEvent {
  return { seq: 1, time: '', level: 'info', action, message };
}

describe('dashboard component log scopes', () => {
  it('keeps management events out of the TapX component log', () => {
    expect(logMatchesScope(event('auth.login', 'login succeeded'), 'tapx')).toBe(false);
    expect(logMatchesScope(event('backup.export', 'database backup exported'), 'tapx')).toBe(false);
    expect(logMatchesScope(event('runtime.component.stop', 'tapx'), 'tapx')).toBe(true);
    expect(logMatchesScope(event('runtime.apply', 'applied generation 2'), 'tapx')).toBe(true);
  });

  it('separates embedded and external Xray events', () => {
    const embedded = event('runtime.component.restart', 'embedded-xray');
    const external = event('runtime.component.stop', 'external-xray');
    const binary = event('xray.binary.update', 'updated external xray');

    expect(logMatchesScope(embedded, 'embedded-xray')).toBe(true);
    expect(logMatchesScope(embedded, 'external-xray')).toBe(false);
    expect(logMatchesScope(external, 'external-xray')).toBe(true);
    expect(logMatchesScope(external, 'embedded-xray')).toBe(false);
    expect(logMatchesScope(binary, 'external-xray')).toBe(true);
  });
});
