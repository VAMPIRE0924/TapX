import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  getClientShare,
  getStats,
  getSystemInterfaces,
  resetClientTraffic,
  resetConnectorTraffic,
  resetListenerTraffic,
  testConnector,
} from './api';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('managed node object APIs', () => {
  it('routes remote object operations through the selected node', async () => {
    const paths: string[] = [];
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      paths.push(path);
      if (path.endsWith('/connectors/test')) {
        return jsonResponse({ result: { id: 'same', kind: 'channel', target: 'remote', network: 'udp', confirmed: true, active: true, message: 'ok' } });
      }
      if (path.includes('/share/clients/')) return jsonResponse({ share: { link: 'raw://remote' } });
      if (path.endsWith('/stats')) return jsonResponse({ byEndpoint: [] });
      if (path.endsWith('/system/interfaces')) return jsonResponse({ interfaces: ['eth0'] });
      return jsonResponse({ config: { Clients: [], Connectors: [], Listeners: [] } });
    }));

    await getStats('node-edge');
    await getSystemInterfaces('node-edge');
    await getClientShare('client same', 'node-edge');
    await testConnector('connector same', 'channel', undefined, 'node-edge');
    await resetClientTraffic('client same', 'node-edge');
    await resetConnectorTraffic('connector same', 'node-edge');
    await resetListenerTraffic('listener same', 'node-edge');

    expect(paths).toEqual([
      '/api/nodes/node-edge/stats',
      '/api/nodes/node-edge/system/interfaces',
      '/api/nodes/node-edge/share/clients/client%20same',
      '/api/nodes/node-edge/connectors/test',
      '/api/nodes/node-edge/clients/client%20same/traffic/reset',
      '/api/nodes/node-edge/connectors/connector%20same/traffic/reset',
      '/api/nodes/node-edge/listeners/listener%20same/traffic/reset',
    ]);
  });

  it('keeps local object operations on local APIs', async () => {
    const paths: string[] = [];
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      paths.push(path);
      if (path.endsWith('/connectors/test')) {
        return jsonResponse({ result: { id: 'connector-a', kind: 'channel', target: 'local', network: 'tcp', confirmed: true, active: true, message: 'ok' } });
      }
      return jsonResponse({ config: {} });
    }));

    await testConnector('connector-a', 'channel', undefined, 'local');
    await resetConnectorTraffic('connector-a', 'local');

    expect(paths).toEqual([
      '/api/connectors/test',
      '/api/connectors/connector-a/traffic/reset',
    ]);
  });
});

function jsonResponse(value: unknown): Response {
  return new Response(JSON.stringify(value), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}
