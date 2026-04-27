import { describe, expect, it } from 'vitest';
import { TAB_ROUTES, FLAG_TO_PATH } from '../routes';

describe('routes registry', () => {
  it('every tab declares a flag key + path', () => {
    for (const t of TAB_ROUTES) {
      expect(t.path).toMatch(/^\//);
      expect(t.flagKey).toMatch(/^frontend_dashboard_v2/);
    }
  });

  it('FLAG_TO_PATH maps products flag to /products', () => {
    expect(FLAG_TO_PATH['frontend_dashboard_v2_products']).toBe('/products');
  });
});
