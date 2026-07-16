import { describe, expect, it } from 'vitest';
import { applyBulkUserAdjustment, buildBulkNames, changeListenerIDs, parseBulkExpiry } from './userBulk';

describe('user bulk operations', () => {
  it('builds random names with a prefix and quantity', () => {
    expect(buildBulkNames({ method: 1, first: 1, last: 1, prefix: 'edge-', postfix: '', quantity: 2 }, () => 'random'))
      .toEqual(['randomedge-', 'randomedge-']);
  });

  it('builds deterministic numbered names without a random segment', () => {
    expect(buildBulkNames({ method: 4, first: 2, last: 4, prefix: 'user-', postfix: '@tapx', quantity: 1 }, () => 'unused'))
      .toEqual(['user-2@tapx', 'user-3@tapx', 'user-4@tapx']);
  });

  it('attaches without duplicating listener ids', () => {
    expect(changeListenerIDs(['a', 'b'], ['b', 'c'], 'attach')).toEqual(['a', 'b', 'c']);
  });

  it('detaches only selected listener ids', () => {
    expect(changeListenerIDs(['a', 'b', 'c'], ['b'], 'detach')).toEqual(['a', 'c']);
  });

  it('parses optional and explicit expiry values', () => {
    expect(parseBulkExpiry('')).toBe(0);
    expect(parseBulkExpiry('invalid')).toBeNull();
    expect(parseBulkExpiry('2026-12-31 00:00:00')).toBeGreaterThan(0);
  });

  it('adjusts quota and sets both TapX bandwidth limits', () => {
    const result = applyBulkUserAdjustment({
      ExpiresAt: 1_800_000_000,
      TrafficCap: 10 * 1024 * 1024 * 1024,
      UploadRateLimit: 1,
      DownloadRateLimit: 1,
    }, {
      addDays: 2,
      addTrafficGB: 5,
      uploadRateMbps: 20,
      downloadRateMbps: 50,
    }, 1_700_000_000);

    expect(result.ExpiresAt).toBe(1_800_172_800);
    expect(result.TrafficCap).toBe(15 * 1024 * 1024 * 1024);
    expect(result.UploadRateLimit).toBe(20_000_000);
    expect(result.DownloadRateLimit).toBe(50_000_000);
  });

  it('keeps blank limits and uses zero to remove a limit', () => {
    const result = applyBulkUserAdjustment({
      UploadRateLimit: 20_000_000,
      DownloadRateLimit: 50_000_000,
    }, {
      addDays: 0,
      addTrafficGB: 0,
      uploadRateMbps: null,
      downloadRateMbps: 0,
    }, 1_700_000_000);

    expect(result.UploadRateLimit).toBe(20_000_000);
    expect(result.DownloadRateLimit).toBe(0);
  });
});
