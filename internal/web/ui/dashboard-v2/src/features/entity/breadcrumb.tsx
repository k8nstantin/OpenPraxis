import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, Home } from 'lucide-react'
import {
  useEntityByUid,
  useEntityHierarchy,
  type EntityKind,
} from '@/lib/queries/entity'
import { EDGE, KIND } from '@/lib/queries/entity-tree'
import type { Entity, HierarchyNode } from '@/lib/types'

// Walk the hierarchy tree to find the path from the root to the
// target entity id. Returns null if the target isn't reachable.
function findPath(
  root: HierarchyNode | undefined,
  targetId: string
): HierarchyNode[] | null {
  if (!root) return null
  if (root.id === targetId) return [root]
  const children = [...(root.sub_products ?? []), ...(root.children ?? [])]
  for (const c of children) {
    const sub = findPath(c, targetId)
    if (sub) return [root, ...sub]
  }
  return null
}

// Index-page label and path for each kind. The home crumb in the
// per-kind breadcrumbs links here; entity-ancestor crumbs always link
// to /entities/$uid.
const KIND_HOME_LABEL: Record<EntityKind, string> = {
  [KIND.product]: 'Products',
  [KIND.manifest]: 'Manifests',
  [KIND.task]: 'Tasks',
  [KIND.skill]: 'Skills',
  [KIND.idea]: 'Ideas',
}

const KIND_HOME_PATH: Record<EntityKind, string> = {
  [KIND.product]: '/products',
  [KIND.manifest]: '/manifests',
  [KIND.task]: '/tasks',
  [KIND.skill]: '/skills',
  [KIND.idea]: '/ideas',
}

// Kind-aware dispatcher — used by the legacy per-kind detail pages
// (EntityPage). Branches on kind:
//   product            → walks the product hierarchy via useEntityHierarchy
//   manifest|task|idea|skill → flat "<KindName> › <EntityTitle>"
//
// Internal entity-ancestor links always navigate to /entities/$uid.
export function EntityBreadcrumb({
  kind,
  entityId,
  entityTitle,
}: {
  kind: EntityKind
  entityId: string
  entityTitle?: string
}) {
  if (kind === KIND.product) {
    return (
      <ProductBreadcrumb
        productId={entityId}
        productTitle={entityTitle}
      />
    )
  }
  return (
    <FlatBreadcrumb
      kind={kind}
      entityId={entityId}
      entityTitle={entityTitle}
    />
  )
}

function ProductBreadcrumb({
  productId,
  productTitle,
}: {
  productId: string
  productTitle?: string
}) {
  const hierarchy = useEntityHierarchy(KIND.product, productId)
  const path = hierarchy.data ? findPath(hierarchy.data, productId) : null

  return (
    <nav
      aria-label='Breadcrumb'
      className='text-muted-foreground flex items-center gap-1.5 text-sm'
    >
      <Link
        to={KIND_HOME_PATH[KIND.product]}
        className='hover:text-foreground inline-flex items-center gap-1'
      >
        <Home className='h-3.5 w-3.5' />
        {KIND_HOME_LABEL[KIND.product]}
      </Link>
      {path && path.length > 0 ? (
        path.map((node) => (
          <span key={node.id} className='inline-flex items-center gap-1.5'>
            <ChevronRight className='h-3.5 w-3.5 opacity-50' />
            {node.id === productId ? (
              <span className='text-foreground font-medium'>{node.title}</span>
            ) : (
              <Link
                to='/entities/$uid'
                params={{ uid: node.id }}
                className='hover:text-foreground'
              >
                {node.title}
              </Link>
            )}
          </span>
        ))
      ) : productTitle ? (
        <span className='inline-flex items-center gap-1.5'>
          <ChevronRight className='h-3.5 w-3.5 opacity-50' />
          <span className='text-foreground font-medium'>{productTitle}</span>
        </span>
      ) : null}
    </nav>
  )
}

// Flat breadcrumb — kind home root, then the entity title. Used for
// every kind that doesn't have a dedicated hierarchy walker (manifest,
// task, idea, skill). Entity-ancestor relationships are not surfaced
// here; operators drill up via the kind-scoped index.
function FlatBreadcrumb({
  kind,
  entityId,
  entityTitle,
}: {
  kind: EntityKind
  entityId: string
  entityTitle?: string
}) {
  return (
    <nav
      aria-label='Breadcrumb'
      className='text-muted-foreground flex items-center gap-1.5 text-sm'
    >
      <Link
        to={KIND_HOME_PATH[kind]}
        className='hover:text-foreground inline-flex items-center gap-1'
      >
        <Home className='h-3.5 w-3.5' />
        {KIND_HOME_LABEL[kind]}
      </Link>
      <span className='inline-flex items-center gap-1.5'>
        <ChevronRight className='h-3.5 w-3.5 opacity-50' />
        <span className='text-foreground font-medium'>
          {entityTitle ?? entityId.slice(0, 12)}
        </span>
      </span>
    </nav>
  )
}

// ── Universal breadcrumb ─────────────────────────────────────────────
//
// Walks the `owns` chain from this entity up to the root. Each ancestor
// is rendered as a link to /entities/$uid; the leaf (current entity) is
// rendered as plain text. Used by the /entities/$uid route — kind
// doesn't matter, the same walker handles every entity type.

interface RelationshipRow {
  SrcID: string
  SrcKind: EntityKind
  DstID: string
  DstKind: EntityKind
  Kind: string
}

interface AncestorCrumb {
  id: string
  title: string
}

const UNIVERSAL_WALK_DEPTH_CAP = 16

async function fetchAncestry(uid: string): Promise<AncestorCrumb[]> {
  const chain: AncestorCrumb[] = []
  let cursor: string | undefined = uid
  const seen = new Set<string>()
  for (let i = 0; i < UNIVERSAL_WALK_DEPTH_CAP && cursor; i++) {
    if (seen.has(cursor)) break
    seen.add(cursor)
    const incoming = await fetch(
      `/api/relationships/incoming?dst_id=${cursor}&kind=${EDGE.owns}`
    )
    if (!incoming.ok) break
    const rows = ((await incoming.json()) as RelationshipRow[] | null) ?? []
    if (rows.length === 0) break
    const parentId = rows[0].SrcID
    const parentRes = await fetch(`/api/entities/${parentId}`)
    if (!parentRes.ok) break
    const parent = (await parentRes.json()) as Entity
    chain.push({ id: parent.entity_uid, title: parent.title })
    cursor = parent.entity_uid
  }
  return chain.reverse()
}

function useUniversalAncestry(uid: string | undefined) {
  return useQuery({
    queryKey: ['entity', 'ancestry', uid ?? ''] as const,
    queryFn: () => fetchAncestry(uid as string),
    enabled: !!uid,
    staleTime: 30 * 1000,
  })
}

export function UniversalBreadcrumb({ uid }: { uid: string }) {
  const entity = useEntityByUid(uid)
  const ancestry = useUniversalAncestry(uid)
  const ancestors = ancestry.data ?? []

  return (
    <nav
      aria-label='Breadcrumb'
      className='text-muted-foreground flex flex-wrap items-center gap-1.5 text-sm'
    >
      <Link
        to='/entities'
        className='hover:text-foreground inline-flex items-center gap-1'
      >
        <Home className='h-3.5 w-3.5' />
        Entities
      </Link>
      {ancestors.map((node) => (
        <span key={node.id} className='inline-flex items-center gap-1.5'>
          <ChevronRight className='h-3.5 w-3.5 opacity-50' />
          <Link
            to='/entities/$uid'
            params={{ uid: node.id }}
            className='hover:text-foreground'
          >
            {node.title}
          </Link>
        </span>
      ))}
      <span className='inline-flex items-center gap-1.5'>
        <ChevronRight className='h-3.5 w-3.5 opacity-50' />
        <span className='text-foreground font-medium'>
          {entity.data?.title ?? uid.slice(0, 12)}
        </span>
      </span>
    </nav>
  )
}
