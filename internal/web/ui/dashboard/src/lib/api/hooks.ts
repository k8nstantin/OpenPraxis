import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationOptions,
  type UseQueryOptions,
  type QueryKey,
} from '@tanstack/react-query';
import { fetchJSON, type ApiError, type FetchOptions } from './fetchJSON';

export interface UseApiQueryOpts<T> extends Omit<UseQueryOptions<T, ApiError, T, QueryKey>, 'queryKey' | 'queryFn'> {
  fetchOpts?: FetchOptions;
}

// Thin React Query wrapper around fetchJSON. Lets every tab type the
// row shape once and infer through.
export function useApiQuery<T>(key: QueryKey, path: string | (() => string), opts?: UseApiQueryOpts<T>) {
  return useQuery<T, ApiError, T, QueryKey>({
    queryKey: key,
    queryFn: () => fetchJSON<T>(typeof path === 'function' ? path() : path, opts?.fetchOpts),
    ...opts,
  });
}

export interface UseApiMutationOpts<T, V> extends Omit<UseMutationOptions<T, ApiError, V>, 'mutationFn'> {
  /** Query keys to invalidate after a successful mutation. */
  invalidate?: QueryKey[];
  /** Apply an optimistic update; return the rollback fn if the mutation fails. */
  optimistic?: (variables: V) => void | (() => void);
}

export function useApiMutation<T, V = unknown>(
  request: (variables: V) => { path: string; init?: FetchOptions },
  opts?: UseApiMutationOpts<T, V>,
) {
  const qc = useQueryClient();
  const { invalidate, optimistic, ...rest } = opts ?? {};
  return useMutation<T, ApiError, V, { rollback?: () => void }>({
    ...rest,
    mutationFn: async (variables) => {
      const { path, init } = request(variables);
      return fetchJSON<T>(path, init);
    },
    onMutate: (variables) => {
      const rollback = optimistic ? optimistic(variables) || undefined : undefined;
      return { rollback };
    },
    onError: (_err, _variables, ctx) => {
      ctx?.rollback?.();
    },
    onSuccess: () => {
      if (invalidate) for (const k of invalidate) qc.invalidateQueries({ queryKey: k });
    },
  });
}
