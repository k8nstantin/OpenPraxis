// Last-viewed-id persistence for the master-detail entity page. Same
// shape as Portal A's per-view memory: a tiny localStorage shim so
// operators land back on the entity they were last looking at instead
// of an empty pane on every reload.
//
// Keyed by entity kind so /products and /manifests don't trample each
// other — switching between the two menus restores the right id.

import type { EntityKind } from '@/lib/queries/entity'

function key(kind: EntityKind): string {
  return `portal-v2.${kind}.lastViewedId`
}

export function readLastViewedId(kind: EntityKind): string | null {
  try {
    return localStorage.getItem(key(kind))
  } catch {
    return null
  }
}

export function writeLastViewedId(
  kind: EntityKind,
  id: string | null
): void {
  try {
    if (id === null) {
      localStorage.removeItem(key(kind))
    } else {
      localStorage.setItem(key(kind), id)
    }
  } catch {
    // ignore — non-fatal
  }
}
