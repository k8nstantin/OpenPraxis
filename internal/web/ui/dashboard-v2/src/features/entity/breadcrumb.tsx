import { Link } from '@tanstack/react-router'
import { ChevronRight, Home } from 'lucide-react'
import {
  useEntity,
  useEntityHierarchy,
  type EntityKind,
} from '@/lib/queries/entity'
import type { HierarchyNode, Manifest, Task } from '@/lib/types'

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
      className='text-muted-foreground flex items-center gap-1.5 text-sm'
    >
      <Link
        to='/products'
        className='hover:text-foreground inline-flex items-center gap-1'
      >
        <Home className='h-3.5 w-3.5' />
        Products
      </Link>
      {path && path.length > 0 ? (
        path.map((node) => (
          <span key={node.id} className='inline-flex items-center gap-1.5'>
            <ChevronRight className='h-3.5 w-3.5 opacity-50' />
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
          <ChevronRight className='h-3.5 w-3.5 opacity-50' />
          <span className='text-foreground font-medium'>{productTitle}</span>
        </span>
      ) : null}
    </nav>
  )
}

// Tasks: flat 2-crumb breadcrumb (Tasks › <TaskTitle>). The task's
// manifest_id is available but we keep v1 deliberately compact —
// landing on a task leaf rarely benefits from drilling up via the
// breadcrumb when the parent manifest is one click away in the
// Manifests menu.
function TaskBreadcrumb({
  taskId,
  taskTitle,
}: {
  taskId: string
  taskTitle?: string
}) {
  const task = useEntity('task', taskId)
  const t = task.data as Task | undefined
  return (
    <nav
      aria-label='Breadcrumb'
      className='text-muted-foreground flex items-center gap-1.5 text-sm'
    >
      <Link
        to='/tasks'
        className='hover:text-foreground inline-flex items-center gap-1'
      >
        <Home className='h-3.5 w-3.5' />
        Tasks
      </Link>
      {t?.manifest_id ? (
        <span className='inline-flex items-center gap-1.5'>
          <ChevronRight className='h-3.5 w-3.5 opacity-50' />
          <Link
            to='/manifests'
            search={{ id: t.manifest_id, tab: 'main' }}
            className='hover:text-foreground'
          >
            {t.manifest_id.slice(0, 12)}
          </Link>
        </span>
      ) : null}
      <span className='inline-flex items-center gap-1.5'>
        <ChevronRight className='h-3.5 w-3.5 opacity-50' />
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
  // Read this manifest to learn its parent product_id; then read the
  // product to get its title for the crumb. Both queries are cached so
  // navigating around doesn't re-fetch.
  const manifest = useEntity('manifest', manifestId)
  const m = manifest.data as Manifest | undefined
  const projectId = m?.project_id
  const parent = useEntity('product', projectId || undefined)
  const parentTitle = parent.data?.title

  return (
    <nav
      aria-label='Breadcrumb'
      className='text-muted-foreground flex items-center gap-1.5 text-sm'
    >
      <Link
        to='/products'
        className='hover:text-foreground inline-flex items-center gap-1'
      >
        <Home className='h-3.5 w-3.5' />
        Products
      </Link>
      {projectId ? (
        <span className='inline-flex items-center gap-1.5'>
          <ChevronRight className='h-3.5 w-3.5 opacity-50' />
          <Link
            to='/products'
            search={{ id: projectId, tab: 'main' }}
            className='hover:text-foreground'
          >
            {parentTitle ?? projectId.slice(0, 12)}
          </Link>
        </span>
      ) : null}
      <span className='inline-flex items-center gap-1.5'>
        <ChevronRight className='h-3.5 w-3.5 opacity-50' />
        <Link
          to='/manifests'
          className='hover:text-foreground'
        >
          Manifests
        </Link>
      </span>
      <span className='inline-flex items-center gap-1.5'>
        <ChevronRight className='h-3.5 w-3.5 opacity-50' />
        <span className='text-foreground font-medium'>
          {manifestTitle ?? manifestId.slice(0, 12)}
        </span>
      </span>
    </nav>
  )
}
