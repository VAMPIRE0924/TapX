import { describe, expect, it } from 'vitest';
import { responseError } from './http-error';

describe('responseError', () => {
  it('extracts the backend JSON error without exposing its envelope', async () => {
    const error = await responseError(new Response(JSON.stringify({ ok: false, error: 'invalid username or password' }), { status: 401 }), 'login');
    expect(error.message).toBe('invalid username or password');
  });

  it('keeps concise plain-text errors', async () => {
    const error = await responseError(new Response('invalid route binding', { status: 400 }), 'config');
    expect(error.message).toBe('invalid route binding');
  });

  it('does not surface an HTML error document', async () => {
    const error = await responseError(new Response('<html>proxy failure</html>', { status: 502 }), 'runtime');
    expect(error.message).toBe('runtime (502)');
  });
});
