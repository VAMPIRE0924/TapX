import { afterEach, describe, expect, it, vi } from 'vitest';
import { integrations } from './integrations';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('managed node integrations', () => {
  it('routes account operations through the selected node', async () => {
    const requests: Array<{ path: string; method: string; body: string }> = [];
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      requests.push({
        path: String(input),
        method: init?.method || 'GET',
        body: String(init?.body || ''),
      });
      return jsonResponse(null);
    }));

    await integrations.warp.data('node edge');
    await integrations.nord.servers(228, 'node edge');
    await integrations.nord.login('remote-token', 'node edge');
    await integrations.warp.remove('local');

    expect(requests).toEqual([
      { path: '/api/nodes/node%20edge/integrations/warp/data', method: 'GET', body: '' },
      { path: '/api/nodes/node%20edge/integrations/nord/servers?countryId=228', method: 'GET', body: '' },
      { path: '/api/nodes/node%20edge/integrations/nord/login', method: 'POST', body: '{"token":"remote-token"}' },
      { path: '/api/integrations/warp/delete', method: 'DELETE', body: '' },
    ]);
  });
});

function jsonResponse(data: unknown): Response {
  return new Response(JSON.stringify({ ok: true, data }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}
