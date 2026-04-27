import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Badge } from '@/components/ui/Badge';
import { useTypeahead } from '@/lib/hooks/useTypeahead';
import { useProductSearch } from '@/lib/queries/products';
import type { Product } from '@/lib/types';

export function ProductsSidebarSearch() {
  const fetcher = useProductSearch();
  const ta = useTypeahead<Product>({ fetcher, minChars: 1, debounceMs: 200 });
  const [focused, setFocused] = useState(false);
  const navigate = useNavigate();
  const showResults = focused && ta.query.length > 0;

  return (
    <div className="products-sidebar__search">
      <input
        type="search"
        className="ui-input"
        placeholder="Search products…"
        value={ta.query}
        onChange={(e) => ta.setQuery(e.target.value)}
        onFocus={() => setFocused(true)}
        onBlur={() => setTimeout(() => setFocused(false), 150)}
        aria-label="Search products"
      />
      {showResults && (
        <ul className="products-sidebar__search-results" role="listbox">
          {ta.loading && <li className="products-sidebar__search-empty">Searching…</li>}
          {!ta.loading && ta.results.length === 0 && (
            <li className="products-sidebar__search-empty">No products found</li>
          )}
          {!ta.loading &&
            ta.results.map((p) => (
              <li key={p.id}>
                <button
                  type="button"
                  className="products-sidebar__search-row"
                  onMouseDown={(e) => {
                    e.preventDefault();
                    navigate(`/products/${p.id}`);
                    ta.reset();
                  }}
                >
                  <span className="marker">{p.marker}</span>
                  <Badge tone={p.status === 'open' ? 'success' : p.status === 'closed' ? 'neutral' : 'info'}>
                    {p.status}
                  </Badge>
                  <span className="products-sidebar__search-title">{p.title}</span>
                </button>
              </li>
            ))}
        </ul>
      )}
    </div>
  );
}
