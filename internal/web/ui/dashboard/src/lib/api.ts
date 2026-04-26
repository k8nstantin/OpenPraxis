// Tiny typed HTTP client. All dashboard reads / writes go through
// fetchJSON so retries, error normalisation, and instrumentation have
// exactly one home.

export class ApiError extends Error {
  status: number;
  body: unknown;
  constructor(status: number, body: unknown, message: string) {
    super(message);
    this.status = status;
    this.body = body;
    this.name = 'ApiError';
  }
}

export async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { Accept: 'application/json', ...(init?.headers || {}) },
    ...init,
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
    throw new ApiError(res.status, body, msg);
  }
  return body as T;
}
