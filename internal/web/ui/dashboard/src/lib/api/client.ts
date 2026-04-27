// Typed HTTP client. Single home for retries, error normalisation, request-ID
// propagation, and global error → toast surfacing. Tabs should use
// `useApiQuery` / `useApiMutation` from `./queries` rather than calling
// fetchJSON directly.
import { toast } from '@/components/ui/Toast';

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

let toastErrors = true;

// Tests can disable the toast side-effect so error states render as plain
// data without polluting the test DOM.
export function setApiErrorToasts(enabled: boolean) {
  toastErrors = enabled;
}

let nextRequestId = 0;
function makeRequestId(): string {
  // 32-bit counter prefixed with epoch ms so server logs can correlate to
  // a single browser session even under heavy concurrent dashboard use.
  nextRequestId = (nextRequestId + 1) >>> 0;
  return `${Date.now().toString(36)}-${nextRequestId.toString(36)}`;
}

export interface FetchOptions extends RequestInit {
  requestId?: string;
  silent?: boolean;
}

export async function fetchJSON<T>(path: string, init: FetchOptions = {}): Promise<T> {
  const { requestId, silent, headers, ...rest } = init;
  const reqId = requestId ?? makeRequestId();
  const res = await fetch(path, {
    headers: {
      Accept: 'application/json',
      'X-Request-ID': reqId,
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
    const msg =
      typeof body === 'object' && body && 'error' in (body as object)
        ? String((body as { error: unknown }).error)
        : `HTTP ${res.status} ${res.statusText}`;
    const err = new ApiError(res.status, body, msg, reqId);
    if (!silent && toastErrors) {
      toast.error(msg, { description: `${path} · req=${reqId}` });
    }
    throw err;
  }
  return body as T;
}
