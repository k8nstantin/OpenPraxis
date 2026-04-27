import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ProductEditForm } from '../ProductEditForm';
import type { Product } from '@/lib/types';

const sample: Product = {
  id: 'p1',
  marker: 'p1xxx',
  title: 'Hello',
  description: 'old desc',
  status: 'open',
  tags: ['one', 'two'],
};

function wrap(ui: React.ReactElement) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('ProductEditForm', () => {
  const originalFetch = global.fetch;
  beforeEach(() => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      text: async () => JSON.stringify({ ...sample, title: 'New' }),
    } as Response);
  });
  afterEach(() => {
    global.fetch = originalFetch;
  });

  it('pre-fills title, description, tags', () => {
    wrap(<ProductEditForm product={sample} onCancel={() => {}} onSaved={() => {}} />);
    expect((screen.getByLabelText(/Title/i) as HTMLInputElement).value).toBe('Hello');
    expect((screen.getByLabelText(/Description/i) as HTMLTextAreaElement).value).toBe('old desc');
    expect((screen.getByLabelText(/Tags/i) as HTMLInputElement).value).toBe('one, two');
  });

  it('submits PUT with parsed tag array on save', async () => {
    const onSaved = vi.fn();
    wrap(<ProductEditForm product={sample} onCancel={() => {}} onSaved={onSaved} />);
    fireEvent.change(screen.getByLabelText(/Title/i), { target: { value: 'New' } });
    fireEvent.change(screen.getByLabelText(/Tags/i), { target: { value: 'a, b ,, c' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect((global.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0]).toBe('/api/products/p1');
    const init = (global.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual({
      title: 'New',
      description: 'old desc',
      tags: ['a', 'b', 'c'],
    });
  });

  it('blocks submit with empty title', () => {
    wrap(<ProductEditForm product={sample} onCancel={() => {}} onSaved={() => {}} />);
    fireEvent.change(screen.getByLabelText(/Title/i), { target: { value: '   ' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(global.fetch).not.toHaveBeenCalled();
    expect(screen.getByText('Title is required')).toBeTruthy();
  });
});
