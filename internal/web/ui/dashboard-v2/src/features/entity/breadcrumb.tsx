import { Link } from '@tanstack/react-router'
import { ChevronRight, Home } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { type EntityKind } from '@/lib/queries/entity'

interface Crumb { id: string; kind: string; title: string }

// Walk owns edges upward from entityId until there is no parent.
// Generic — works for any entity type at any depth, no kind checks.
function useAncestorChain(entityId: string) {
  return useQuery({
    queryKey: ['ancestor-chain', entityId],
    enabled: !!entityId,
    staleTime: 30_000,
    queryFn: async (): Promise<Crumb[]> => {
      const chain: Crumb[] = []
      let currentId = entityId
      for (let depth = 0; depth < 8; depth++) {
        const edges = await fetch(
          `/api/relationships/incoming?dst_id=${currentId}&kind=owns`
        ).then(r => r.json())
        if (!Array.isArray(edges) || edges.length === 0) break
        const srcId: string = edges[0].SrcID
        const entity = await fetch(`/api/entities/${srcId}`).then(r => r.json())
        chain.unshift({ id: srcId, kind: entity.type, title: entity.title })
        currentId = srcId
      }
      return chain
    },
  })
}

export function EntityBreadcrumb({
  entityId,
  entityTitle,
  kind,
}: {
  entityId: string
  entityTitle?: string
  kind?: EntityKind
}) {
  const { data: ancestors = [] } = useAncestorChain(entityId)

  return (
    <nav aria-label='Breadcrumb' className='text-muted-foreground flex flex-wrap items-center gap-1 text-xs'>
      <Link to='/entities' className='hover:text-foreground inline-flex items-center gap-0.5'>
        <Home className='h-3 w-3' />
      </Link>
      {ancestors.map(c => (
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
      {entityTitle && (
        <span className='inline-flex items-center gap-1'>
          <ChevronRight className='h-3 w-3 opacity-40' />
          <span className='text-foreground font-medium truncate max-w-[200px]'>{entityTitle}</span>
        </span>
      )}
    </nav>
  )
}
