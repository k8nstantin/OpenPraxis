// Tiny hook for "last viewed product" persistence. Uses localStorage
// so the operator returns to wherever they were instead of an empty
// pane on every reload. Operators don't work on 20 products at a time
// — they live in one and switch occasionally.
//
// Single key (no per-tab tracking) — coming back to Products lands on
// the most recently selected product, regardless of which tab was
// active. The tab itself is in the URL search param; this hook only
// remembers the product id.

const KEY = 'portal-v2.products.lastViewedId'

export function readLastViewedProductId(): string | null {
  try {
    return localStorage.getItem(KEY)
  } catch {
    // SSR / private mode / quota — graceful degrade to empty state
    return null
  }
}

export function writeLastViewedProductId(id: string | null): void {
  try {
    if (id === null) {
      localStorage.removeItem(KEY)
    } else {
      localStorage.setItem(KEY, id)
    }
  } catch {
    // ignore — non-fatal
  }
}
