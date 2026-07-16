import { afterEach, describe, expect, it, vi } from 'vitest';
import { restartRuntimeComponent, stopRuntimeComponent } from './api';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('runtime component actions', () => {
  it('targets only the requested kernel component', async () => {
    const requests: Array<{ path: string; method: string }> = [];
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({ path: String(input), method: init?.method || 'GET' });
      return new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }));

    await restartRuntimeComponent('embedded-xray');
    await stopRuntimeComponent('external-xray');
    await restartRuntimeComponent('tapx');

    expect(requests).toEqual([
      { path: '/api/runtime/components/embedded-xray/restart', method: 'POST' },
      { path: '/api/runtime/components/external-xray/stop', method: 'POST' },
      { path: '/api/runtime/components/tapx/restart', method: 'POST' },
    ]);
  });
});
