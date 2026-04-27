import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { StatusPivots } from '../StatusPivots';

function wrap(ui: React.ReactElement) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('StatusPivots', () => {
  const originalFetch = global.fetch;
  beforeEach(() => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      text: async () => JSON.stringify({ id: 'p1', status: 'open' }),
    } as Response);
  });
  afterEach(() => {
    global.fetch = originalFetch;
  });

  it('renders all four status options with the current one active', () => {
    wrap(<StatusPivots productId="p1" current="open" />);
    expect(screen.getByText('draft')).toBeTruthy();
    const open = screen.getByText('open');
    expect(open.getAttribute('aria-pressed')).toBe('true');
    const draft = screen.getByText('draft');
    expect(draft.getAttribute('aria-pressed')).toBe('false');
  });

  it('PUTs the new status on click of an inactive pivot', async () => {
    wrap(<StatusPivots productId="p1" current="open" />);
    fireEvent.click(screen.getByText('closed'));
    // useMutation fires fetch async — flush microtasks once.
    await Promise.resolve();
    expect((global.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0]).toBe('/api/products/p1');
    const init = (global.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual({ status: 'closed' });
  });

  it('does not fire when clicking the active pivot', () => {
    wrap(<StatusPivots productId="p1" current="open" />);
    fireEvent.click(screen.getByText('open'));
    expect(global.fetch).not.toHaveBeenCalled();
  });
});
