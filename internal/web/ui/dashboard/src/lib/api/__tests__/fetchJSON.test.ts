import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, fetchJSON } from '../fetchJSON';
import { onApiError } from '../errorBus';

describe('fetchJSON', () => {
  const realFetch = global.fetch;
  afterEach(() => {
    global.fetch = realFetch;
    vi.restoreAllMocks();
  });

  it('returns parsed JSON on success and emits no error', async () => {
    const calls: ApiError[] = [];
    const off = onApiError((e) => calls.push(e));
    global.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ id: 'abc' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json', 'X-Request-ID': 'r-1' },
      }),
    );
    const out = await fetchJSON<{ id: string }>('/api/products/abc');
    expect(out).toEqual({ id: 'abc' });
    expect(calls).toEqual([]);
    off();
  });

  it('throws ApiError on non-2xx and broadcasts on the error bus', async () => {
    const calls: ApiError[] = [];
    const off = onApiError((e) => calls.push(e));
    global.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ error: 'forbidden' }), {
        status: 403,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    await expect(fetchJSON('/api/products/x')).rejects.toBeInstanceOf(ApiError);
    expect(calls).toHaveLength(1);
    expect(calls[0]?.status).toBe(403);
    expect(calls[0]?.message).toBe('forbidden');
    off();
  });

  it('honours silent: true (no bus emit)', async () => {
    const calls: ApiError[] = [];
    const off = onApiError((e) => calls.push(e));
    global.fetch = vi.fn(async () => new Response('boom', { status: 500 }));
    await expect(fetchJSON('/api/x', { silent: true })).rejects.toBeInstanceOf(ApiError);
    expect(calls).toEqual([]);
    off();
  });

  it('attaches an X-Request-ID header', async () => {
    let captured: HeadersInit | undefined;
    global.fetch = vi.fn(async (_input, init) => {
      captured = init?.headers;
      return new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } });
    });
    await fetchJSON('/api/x');
    expect(captured && (captured as Record<string, string>)['X-Request-ID']).toMatch(/^c-/);
  });
});
