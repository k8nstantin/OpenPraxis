import { useEffect, useMemo, useState } from 'react';

export interface UseTypeaheadResult<T> {
  query: string;
  setQuery: (q: string) => void;
  results: T[];
}

// Debounced client-side typeahead. Pass the full universe and a match
// predicate; results recompute on every debounced query change. Server-
// side typeahead is left to the caller — wrap fetchJSON in their own
// debounced effect.
export function useTypeahead<T>(
  items: T[] | undefined,
  matcher: (item: T, q: string) => boolean,
  delayMs = 120,
): UseTypeaheadResult<T> {
  const [query, setQuery] = useState('');
  const [debounced, setDebounced] = useState('');

  useEffect(() => {
    const t = setTimeout(() => setDebounced(query), delayMs);
    return () => clearTimeout(t);
  }, [query, delayMs]);

  const results = useMemo(() => {
    if (!items) return [];
    if (!debounced) return items;
    return items.filter((it) => matcher(it, debounced));
  }, [items, matcher, debounced]);

  return { query, setQuery, results };
}
