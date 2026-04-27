import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { _resetFlagCache, flagKey, tabIsOnReactV2 } from '../featureFlags';

describe('featureFlags', () => {
  beforeEach(() => _resetFlagCache());
  afterEach(() => vi.restoreAllMocks());

  it('flagKey produces frontend_dashboard_v2_<tab>', () => {
    expect(flagKey('products')).toBe('frontend_dashboard_v2_products');
    expect(flagKey('manifests')).toBe('frontend_dashboard_v2_manifests');
  });

  it('returns true when the resolved knob is "true"', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      text: () => Promise.resolve(JSON.stringify([
        { key: 'frontend_dashboard_v2_products', value: 'true', source: 'system' },
      ])),
      status: 200,
      statusText: 'OK',
    }));
    const on = await tabIsOnReactV2('products');
    expect(on).toBe(true);
  });

  it('returns false when the knob is missing or "false"', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      text: () => Promise.resolve(JSON.stringify([
        { key: 'frontend_dashboard_v2_products', value: 'false', source: 'system' },
      ])),
      status: 200,
      statusText: 'OK',
    }));
    const on = await tabIsOnReactV2('products');
    expect(on).toBe(false);
  });
});
