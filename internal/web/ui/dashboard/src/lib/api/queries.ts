// Thin wrappers over @tanstack/react-query that pin every dashboard read /
// write to the typed `fetchJSON<T>` client. Tabs avoid touching the raw
// `useQuery` / `useMutation` APIs so retry / cache / error policy lives in
// one place.
import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryKey,
  type UseMutationOptions,
  type UseQueryOptions,
} from '@tanstack/react-query';
import { fetchJSON, type FetchOptions } from './client';

export interface UseApiQueryArgs<T> extends Omit<UseQueryOptions<T, Error, T, QueryKey>, 'queryKey' | 'queryFn'> {
  key: QueryKey;
  path: string;
  fetchOptions?: FetchOptions;
}

export function useApiQuery<T>({ key, path, fetchOptions, ...opts }: UseApiQueryArgs<T>) {
  return useQuery<T, Error>({
    queryKey: key,
    queryFn: () => fetchJSON<T>(path, fetchOptions),
    ...opts,
  });
}

export interface UseApiMutationArgs<T, V>
  extends Omit<UseMutationOptions<T, Error, V>, 'mutationFn'> {
  path: string | ((vars: V) => string);
  method?: 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  body?: (vars: V) => unknown;
  invalidate?: QueryKey[];
  /**
   * Optimistic update: produce the next cache value for `optimisticKey`
   * given the current value and the mutation variables. The previous
   * value is captured and restored on error.
   */
  optimistic?: {
    key: QueryKey;
    update: (prev: T | undefined, vars: V) => T;
  };
}

export function useApiMutation<T = unknown, V = void>({
  path,
  method = 'POST',
  body,
  invalidate,
  optimistic,
  ...opts
}: UseApiMutationArgs<T, V>) {
  const qc = useQueryClient();
  return useMutation<T, Error, V>({
    mutationFn: (vars) => {
      const url = typeof path === 'function' ? path(vars) : path;
      const init: FetchOptions = { method };
      if (body && method !== 'DELETE') {
        init.body = JSON.stringify(body(vars));
        init.headers = { 'Content-Type': 'application/json' };
      }
      return fetchJSON<T>(url, init);
    },
    onMutate: async (vars) => {
      if (!optimistic) return undefined;
      await qc.cancelQueries({ queryKey: optimistic.key });
      const prev = qc.getQueryData<T>(optimistic.key);
      qc.setQueryData<T>(optimistic.key, optimistic.update(prev, vars));
      return { prev } as const;
    },
    onError: (_err, _vars, ctx) => {
      if (ctx && optimistic) {
        const previous = (ctx as { prev?: T }).prev;
        qc.setQueryData(optimistic.key, previous);
      }
    },
    onSettled: () => {
      if (invalidate) for (const k of invalidate) qc.invalidateQueries({ queryKey: k });
    },
    ...opts,
  });
}
