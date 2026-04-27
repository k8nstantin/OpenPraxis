// Smoke tests for the Products tab. These guard against regressions in
// the parity migration without depending on a live backend — fetch is
// stubbed so the page can mount under jsdom + react-query.
import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { setApiErrorToasts } from '../../lib/api/client';
import Products from '../Products';

interface FetchMock {
  url: string;
  init?: RequestInit;
}

let calls: FetchMock[] = [];
let responder: (url: string, init?: RequestInit) => unknown = () => null;

beforeEach(() => {
  calls = [];
  setApiErrorToasts(false);
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    calls.push({ url, init });
    const data = responder(url, init);
    return new Response(JSON.stringify(data ?? null), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function mount(path = '/products') {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/products" element={<Products />} />
          <Route path="/products/:id" element={<Products />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('Products page', () => {
  it('renders sidebar header + new product button', async () => {
    responder = () => [];
    mount();
    expect(screen.getByText('Products')).toBeTruthy();
    expect(screen.getByText('+ New Product')).toBeTruthy();
    expect(screen.getByText('Select a product to view details.')).toBeTruthy();
  });

  it('renders peer groups and products', async () => {
    responder = (url) => {
      if (url === '/api/products/by-peer') {
        return [
          {
            peer_id: 'peer-aaaa-bbbb-cccc',
            count: 1,
            products: [
              {
                id: 'p1',
                marker: '019dc464-430',
                title: 'Sample Product',
                status: 'open',
                total_manifests: 0,
              },
            ],
          },
        ];
      }
      return null;
    };
    mount();
    await waitFor(() => expect(screen.getByText('Sample Product')).toBeTruthy());
    expect(screen.getByText('019dc464-430')).toBeTruthy();
  });

  it('shows search results when typing in the sidebar search', async () => {
    responder = (url) => {
      if (url === '/api/products/by-peer') return [];
      if (url.startsWith('/api/products/search')) {
        return [
          { id: 'p2', marker: '019dc464-431', title: 'Hit', status: 'draft' },
        ];
      }
      return null;
    };
    mount();
    const search = screen.getByPlaceholderText(/Search products by id/);
    fireEvent.change(search, { target: { value: 'hit' } });
    await waitFor(() => expect(screen.getByText('Hit')).toBeTruthy(), { timeout: 1000 });
  });

  it('loads detail when /products/:id', async () => {
    responder = (url) => {
      if (url === '/api/products/by-peer') return [];
      if (url === '/api/products/p1') {
        return {
          id: 'p1',
          marker: '019dc464-430',
          title: 'Detail Product',
          status: 'open',
          tags: ['tag1'],
          total_manifests: 2,
          total_tasks: 5,
          total_turns: 12,
          total_cost: 1.5,
        };
      }
      if (url.includes('/manifests')) return [];
      if (url.includes('/ideas')) return null;
      if (url.includes('/comments')) return { comments: [] };
      if (url.includes('/dependencies')) return { deps: null, dependents: null };
      return null;
    };
    mount('/products/p1');
    await waitFor(() => expect(screen.getByRole('heading', { level: 1, name: 'Detail Product' })).toBeTruthy());
    expect(screen.getByText('+ New Manifest')).toBeTruthy();
    expect(screen.getByText('+ Link Manifest')).toBeTruthy();
    expect(screen.getByText('+ Depends On')).toBeTruthy();
    expect(screen.getByText('+ Link Idea')).toBeTruthy();
    expect(screen.getByText('◈ Product DAG')).toBeTruthy();
    expect(screen.getByTestId('status-pivot-open').classList.contains('is-active')).toBe(true);
    expect(screen.getByTestId('status-pivot-draft').classList.contains('is-active')).toBe(false);
  });
});
