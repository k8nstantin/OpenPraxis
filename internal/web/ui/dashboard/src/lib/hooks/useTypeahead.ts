// Generic typeahead state machine. Tabs feed it a fetcher that produces a
// list of suggestions for a query; the hook owns query state, debounce,
// loading flag, and an active highlight index for keyboard navigation.
import { useEffect, useRef, useState } from 'react';

export interface UseTypeaheadArgs<T> {
  fetcher: (query: string) => Promise<T[]>;
  debounceMs?: number;
  minChars?: number;
}

export interface TypeaheadState<T> {
  query: string;
  setQuery: (q: string) => void;
  results: T[];
  loading: boolean;
  error: Error | null;
  active: number;
  setActive: (i: number) => void;
  reset: () => void;
}

export function useTypeahead<T>({ fetcher, debounceMs = 200, minChars = 1 }: UseTypeaheadArgs<T>): TypeaheadState<T> {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [active, setActive] = useState(0);
  const requestSeq = useRef(0);

  useEffect(() => {
    if (query.length < minChars) {
      setResults([]);
      setError(null);
      return;
    }
    const seq = ++requestSeq.current;
    setLoading(true);
    const timer = setTimeout(async () => {
      try {
        const data = await fetcher(query);
        if (seq === requestSeq.current) {
          setResults(data);
          setError(null);
        }
      } catch (err) {
        if (seq === requestSeq.current) {
          setError(err instanceof Error ? err : new Error(String(err)));
          setResults([]);
        }
      } finally {
        if (seq === requestSeq.current) setLoading(false);
      }
    }, debounceMs);
    return () => clearTimeout(timer);
  }, [query, fetcher, debounceMs, minChars]);

  return {
    query,
    setQuery,
    results,
    loading,
    error,
    active,
    setActive,
    reset: () => {
      setQuery('');
      setResults([]);
      setError(null);
      setActive(0);
    },
  };
}
