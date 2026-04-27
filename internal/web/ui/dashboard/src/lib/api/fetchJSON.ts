import { emitApiError } from './errorBus';

// Tiny typed HTTP client. Every dashboard read / write goes through
// fetchJSON so retries, error normalisation, request-ID propagation,
// and instrumentation have exactly one home.

export class ApiError extends Error {
  status: number;
  body: unknown;
  requestId?: string;
  constructor(status: number, body: unknown, message: string, requestId?: string) {
    super(message);
    this.status = status;
    this.body = body;
    this.requestId = requestId;
    this.name = 'ApiError';
  }
}

function newRequestId(): string {
  // Short, sortable, unique-enough for client-side correlation.
  return `c-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

export interface FetchOptions extends RequestInit {
  /** Skip the global error-bus emit (e.g. when the caller handles its own errors). */
  silent?: boolean;
}

export async function fetchJSON<T>(path: string, init?: FetchOptions): Promise<T> {
  const requestId = newRequestId();
  const { silent, headers, ...rest } = init ?? {};
  const res = await fetch(path, {
    headers: {
      Accept: 'application/json',
      'X-Request-ID': requestId,
      ...(headers || {}),
    },
    ...rest,
  });
  const text = await res.text();
  let body: unknown = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = text;
    }
  }
  if (!res.ok) {
    const msg = typeof body === 'object' && body && 'error' in (body as object)
      ? String((body as { error: unknown }).error)
      : `HTTP ${res.status} ${res.statusText}`;
    const err = new ApiError(res.status, body, msg, res.headers.get('X-Request-ID') ?? requestId);
    if (!silent) emitApiError(err);
    throw err;
  }
  return body as T;
}
