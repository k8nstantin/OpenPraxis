import { Link } from '@tanstack/react-router'
import { ChevronRight, Home } from 'lucide-react'
import {
  KIND,
  useEntityHierarchy,
  type EntityKind,
} from '@/lib/queries/entity'
import type { HierarchyNode } from '@/lib/types'

// Home path for the breadcrumb root crumb, keyed by entity kind. All
// entity kinds now resolve to the universal /entities listing — adding
// a new kind requires one line here.
const KIND_HOME_PATH: Record<EntityKind, string> = {
  [KIND.product]: '/entities',
  [KIND.manifest]: '/entities',
  [KIND.task]: '/entities',
  [KIND.skill]: '/entities',
  [KIND.idea]: '/entities',
}

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

// Generic entity breadcrumb. Branches on kind:
//   product  → Products › <ProductTitle>
//   manifest → Products › <ParentProductTitle> › Manifests › <ManifestTitle>
//
// Manifest's parent product is resolved via manifest.project_id; the
// product detail load surfaces its title for the parent crumb. If the
// manifest isn't linked to a product (project_id empty), the parent
// crumb is dropped.
export function EntityBreadcrumb({
  kind,
  entityId,
  entityTitle,
}: {
  kind: EntityKind
  entityId: string
  entityTitle?: string
}) {
  if (kind === 'product') {
    return (
      <ProductBreadcrumb
        productId={entityId}
        productTitle={entityTitle}
      />
    )
  }
  if (kind === 'task') {
    return <TaskBreadcrumb taskId={entityId} taskTitle={entityTitle} />
  }
  return (
    <ManifestBreadcrumb
      manifestId={entityId}
      manifestTitle={entityTitle}
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
  const hierarchy = useEntityHierarchy('product', productId)
  const path = hierarchy.data ? findPath(hierarchy.data, productId) : null

  return (
    <nav
      aria-label='Breadcrumb'
      className='text-muted-foreground flex items-center gap-1 text-xs'
    >
      <Link
        to={KIND_HOME_PATH[KIND.product]}
        className='hover:text-foreground inline-flex items-center gap-1'
      >
        <Home className='h-3 w-3' />
        Products
      </Link>
      {path && path.length > 0 ? (
        path.map((node) => (
          <span key={node.id} className='inline-flex items-center gap-1.5'>
            <ChevronRight className='h-3 w-3 opacity-50' />
            {node.id === productId ? (
              <span className='text-foreground font-medium'>{node.title}</span>
            ) : (
              <Link
                to='/products'
                search={{ id: node.id, tab: 'main' }}
                className='hover:text-foreground'
              >
                {node.title}
              </Link>
            )}
          </span>
        ))
      ) : productTitle ? (
        <span className='inline-flex items-center gap-1.5'>
          <ChevronRight className='h-3 w-3 opacity-50' />
          <span className='text-foreground font-medium'>{productTitle}</span>
        </span>
      ) : null}
    </nav>
  )
}

// Tasks: flat 2-crumb breadcrumb (Tasks › <TaskTitle>). The parent
// manifest relationship lives in the relationships table, not on the
// entity. The breadcrumb shows Tasks › <title> only; drill up via
// the Manifests menu to reach the parent.
function TaskBreadcrumb({
  taskId,
  taskTitle,
}: {
  taskId: string
  taskTitle?: string
}) {
  return (
    <nav
      aria-label='Breadcrumb'
      className='text-muted-foreground flex items-center gap-1 text-xs'
    >
      <Link
        to={KIND_HOME_PATH[KIND.task]}
        className='hover:text-foreground inline-flex items-center gap-1'
      >
        <Home className='h-3 w-3' />
        Tasks
      </Link>
      <span className='inline-flex items-center gap-1.5'>
        <ChevronRight className='h-3 w-3 opacity-50' />
        <span className='text-foreground font-medium'>
          {taskTitle ?? taskId.slice(0, 12)}
        </span>
      </span>
    </nav>
  )
}

function ManifestBreadcrumb({
  manifestId,
  manifestTitle,
}: {
  manifestId: string
  manifestTitle?: string
}) {
  // Parent product is resolved via relationships, not a field on the entity.
  // Show Manifests › <title> — operator can navigate to Products to drill up.
  return (
    <nav
      aria-label='Breadcrumb'
      className='text-muted-foreground flex items-center gap-1 text-xs'
    >
      <Link
        to={KIND_HOME_PATH[KIND.manifest]}
        className='hover:text-foreground inline-flex items-center gap-1'
      >
        <Home className='h-3 w-3' />
        Manifests
      </Link>
      <span className='inline-flex items-center gap-1.5'>
        <ChevronRight className='h-3 w-3 opacity-50' />
        <span className='text-foreground font-medium'>
          {manifestTitle ?? manifestId.slice(0, 12)}
        </span>
      </span>
    </nav>
  )
}
