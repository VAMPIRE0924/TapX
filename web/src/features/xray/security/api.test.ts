import { afterEach, describe, expect, it, vi } from 'vitest';
import { getPanelObject, postPanelObject, postPanelResult } from './api';

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

describe('Xray security API', () => {
  it('unwraps required panel objects', async () => {
    globalThis.fetch = vi.fn(async () => new Response(JSON.stringify({ success: true, obj: { value: 1 } }), {
      status: 200, headers: { 'Content-Type': 'application/json' },
    }));
    await expect(getPanelObject<{ value: number }>('/test')).resolves.toEqual({ value: 1 });
    await expect(postPanelObject<{ value: number }>('/test', {})).resolves.toEqual({ value: 1 });
  });

  it('accepts boxed and legacy unboxed scan results', async () => {
    globalThis.fetch = vi.fn(async () => new Response(JSON.stringify([{ target: 'example.com:443' }]), { status: 200 }));
    await expect(postPanelResult<Array<{ target: string }>>('/scan', {})).resolves.toEqual([{ target: 'example.com:443' }]);
  });

  it('surfaces panel errors', async () => {
    globalThis.fetch = vi.fn(async () => new Response(JSON.stringify({ success: false, msg: 'failed' }), { status: 400 }));
    await expect(getPanelObject('/test')).rejects.toThrow('failed');
  });
});
