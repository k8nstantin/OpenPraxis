import type { EntityKind } from './entity'

export type EntityHit = {
  kind: EntityKind
  id: string
  title: string
}

type Row = {
  id: string
  title?: string
  description?: string
  status?: string
}

async function searchOne(kind: EntityKind, query: string): Promise<EntityHit[]> {
  const path =
    kind === 'product'
      ? '/api/products/search'
      : kind === 'manifest'
        ? '/api/manifests/search'
        : '/api/tasks/search'
  const url = `${path}?q=${encodeURIComponent(query)}&limit=8`
  try {
    const res = await fetch(url)
    if (!res.ok) return []
    const data = (await res.json()) as Row[] | { results?: Row[] }
    const rows: Row[] = Array.isArray(data) ? data : (data.results ?? [])
    return rows
      .filter((r) => r && r.id)
      .map((r) => ({
        kind,
        id: r.id,
        title: r.title ?? r.id.slice(0, 8),
      }))
  } catch {
    return []
  }
}

// searchEntities — fan-out to all three entity-search endpoints. Used
// by the BlockNote `@`-mention picker + the slash-menu "Link to …"
// items. Empty query returns empty.
export async function searchEntities(query: string): Promise<EntityHit[]> {
  const q = query.trim()
  if (!q) return []
  const [p, m, t] = await Promise.all([
    searchOne('product', q),
    searchOne('manifest', q),
    searchOne('task', q),
  ])
  return [...p, ...m, ...t]
}

export function entityHref(hit: EntityHit): string {
  const slug =
    hit.kind === 'product'
      ? 'products'
      : hit.kind === 'manifest'
        ? 'manifests'
        : 'tasks'
  return `/${slug}/${hit.id}`
}

export async function getEntityTitle(
  kind: EntityKind,
  id: string
): Promise<string | null> {
  const slug =
    kind === 'product' ? 'products' : kind === 'manifest' ? 'manifests' : 'tasks'
  try {
    const res = await fetch(`/api/${slug}/${id}`)
    if (!res.ok) return null
    const data = (await res.json()) as Row
    return data.title ?? null
  } catch {
    return null
  }
}
