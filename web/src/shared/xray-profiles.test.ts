import { describe, expect, it } from 'vitest';
import type { TapxXrayProfile } from './api';
import { removeUnusedXrayProfiles, upsertXrayProfile } from './xray-profiles';

const profile = (ID: string): TapxXrayProfile => ({ ID, Enabled: true });

describe('Xray profile ownership', () => {
  it('inserts and replaces profiles by ID', () => {
    expect(upsertXrayProfile([profile('a')], profile('b')).map((item) => item.ID)).toEqual(['a', 'b']);
    expect(upsertXrayProfile([{ ...profile('a'), Enabled: false }], profile('a'))).toEqual([profile('a')]);
  });

  it('removes only unreferenced candidate profiles', () => {
    const profiles = [profile('listener'), profile('connector'), profile('unused'), profile('unrelated')];
    const result = removeUnusedXrayProfiles(profiles, ['listener', 'connector', 'unused'], {
      listeners: [{ ID: 'l1', XrayProfileID: 'listener' }],
      connectors: [{ ID: 'c1', XrayProfileID: 'connector' }],
    });
    expect(result.map((item) => item.ID)).toEqual(['listener', 'connector', 'unrelated']);
  });

  it('upserts a same-id profile only on its owning node', () => {
    const profiles = [
      { ...profile('shared'), ManagedNodeID: 'local', Name: 'local' },
      { ...profile('shared'), ManagedNodeID: 'node-edge', Name: 'old-remote' },
    ] as never;
    const result = upsertXrayProfile(profiles, {
      ...profile('shared'), ManagedNodeID: 'node-edge', Name: 'remote',
    } as never) as Array<TapxXrayProfile & { ManagedNodeID?: string }>;

    expect(result).toHaveLength(2);
    expect(result.find((item) => item.ManagedNodeID === 'local')?.Name).toBe('local');
    expect(result.find((item) => item.ManagedNodeID === 'node-edge')?.Name).toBe('remote');
  });
});
