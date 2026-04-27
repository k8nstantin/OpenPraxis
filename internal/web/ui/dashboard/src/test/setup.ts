import '@testing-library/react';

// jsdom doesn't ship ResizeObserver / IntersectionObserver — Radix
// (Popover/Tooltip/Dialog) uses ResizeObserver via `useSize` to track
// trigger geometry. Stub both so portal-based primitives can mount
// inside our axe a11y suite without environment errors.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
class IntersectionObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() { return []; }
  root = null;
  rootMargin = '';
  thresholds: number[] = [];
}
if (typeof globalThis.ResizeObserver === 'undefined') {
  // @ts-expect-error — installing a minimal stub on globalThis.
  globalThis.ResizeObserver = ResizeObserverStub;
}
if (typeof globalThis.IntersectionObserver === 'undefined') {
  // @ts-expect-error — installing a minimal stub on globalThis.
  globalThis.IntersectionObserver = IntersectionObserverStub;
}
