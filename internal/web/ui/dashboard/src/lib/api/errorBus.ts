import type { ApiError } from './fetchJSON';

type Listener = (err: ApiError) => void;

const listeners = new Set<Listener>();

export function onApiError(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function emitApiError(err: ApiError) {
  for (const l of listeners) {
    try {
      l(err);
    } catch (e) {
      console.error('errorBus listener threw', e);
    }
  }
}
