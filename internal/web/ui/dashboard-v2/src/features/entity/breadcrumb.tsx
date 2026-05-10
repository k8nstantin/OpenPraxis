import { Link } from '@tanstack/react-router'
import { ChevronRight, Home } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { type EntityKind } from '@/lib/queries/entity'

// Fetch the incoming owns edge for an entity — returns the parent {id, kind, title} or null
function useParent(entityId: string) {
  return useQuery({
    queryKey: ['parent-owns', entityId],
    enabled: !!entityId,
    staleTime: 30_000,
    queryFn: async () => {
      const edges = await fetch(`/api/relationships/incoming?dst_id=${entityId}&kind=owns`).then(r => r.json())
      if (!Array.isArray(edges) || edges.length === 0) return null
      const srcId: string = edges[0].SrcID
      const entity = await fetch(`/api/entities/${srcId}`).then(r => r.json())
      return { id: srcId, kind: entity.type as string, title: entity.title as string }
    },
  })
}

interface Crumb { id: string; kind: string; title: string }

function Crumbs({ crumbs, currentTitle }: { crumbs: Crumb[]; currentTitle?: string }) {
  return (
    <nav aria-label='Breadcrumb' className='text-muted-foreground flex flex-wrap items-center gap-1 text-xs'>
      <Link to='/entities' className='hover:text-foreground inline-flex items-center gap-0.5'>
        <Home className='h-3 w-3' />
      </Link>
      {crumbs.map(c => (
        <span key={c.id} className='inline-flex items-center gap-1'>
          <ChevronRight className='h-3 w-3 opacity-40' />
          <Link
            to='/entities/$uid'
            params={{ uid: c.id }}
            search={{ kind: c.kind as EntityKind, tab: 'main' }}
            className='hover:text-foreground truncate max-w-[160px]'
          >
            {c.title}
          </Link>
        </span>
      ))}
      {currentTitle && (
        <span className='inline-flex items-center gap-1'>
          <ChevronRight className='h-3 w-3 opacity-40' />
          <span className='text-foreground font-medium truncate max-w-[200px]'>{currentTitle}</span>
        </span>
      )}
    </nav>
  )
}

// Task: walks up two levels (manifest → product)
function TaskBreadcrumb({ taskId, taskTitle }: { taskId: string; taskTitle?: string }) {
  const manifest = useParent(taskId)
  const product = useParent(manifest.data?.id ?? '')

  const crumbs: Crumb[] = []
  if (product.data) crumbs.push(product.data)
  if (manifest.data) crumbs.push(manifest.data)

  return <Crumbs crumbs={crumbs} currentTitle={taskTitle} />
}

// Manifest: walks up one level (product)
function ManifestBreadcrumb({ manifestId, manifestTitle }: { manifestId: string; manifestTitle?: string }) {
  const product = useParent(manifestId)
  return <Crumbs crumbs={product.data ? [product.data] : []} currentTitle={manifestTitle} />
}

// Product: may have a parent product (sub-product)
function ProductBreadcrumb({ productId, productTitle }: { productId: string; productTitle?: string }) {
  const parent = useParent(productId)
  return <Crumbs crumbs={parent.data ? [parent.data] : []} currentTitle={productTitle} />
}

export function EntityBreadcrumb({
  kind,
  entityId,
  entityTitle,
}: {
  kind: EntityKind
  entityId: string
  entityTitle?: string
}) {
  if (kind === 'task') return <TaskBreadcrumb taskId={entityId} taskTitle={entityTitle} />
  if (kind === 'manifest') return <ManifestBreadcrumb manifestId={entityId} manifestTitle={entityTitle} />
  if (kind === 'product') return <ProductBreadcrumb productId={entityId} productTitle={entityTitle} />
  // skill / idea — flat
  return (
    <nav className='text-muted-foreground flex items-center gap-1 text-xs'>
      <Link to='/entities' className='hover:text-foreground inline-flex items-center gap-0.5'>
        <Home className='h-3 w-3' />
      </Link>
      {entityTitle && (
        <span className='inline-flex items-center gap-1'>
          <ChevronRight className='h-3 w-3 opacity-40' />
          <span className='text-foreground font-medium'>{entityTitle}</span>
        </span>
      )}
    </nav>
  )
}
