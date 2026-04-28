import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useProductDependencies } from '@/lib/queries/products'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

const STATUS_COLOR: Record<string, string> = {
  open: 'bg-emerald-500/15 text-emerald-500',
  in_progress: 'bg-sky-500/15 text-sky-500',
  draft: 'bg-amber-500/15 text-amber-500',
  closed: 'bg-zinc-500/15 text-zinc-400',
  archived: 'bg-zinc-500/10 text-zinc-500',
}

// Dependencies tab — read-only edge list for now. Add/remove editor
// and the "build dependencies" affordance land in a follow-up. The
// shape is intentionally split into upstream (this product depends on
// →) and downstream (← these products depend on this) so the operator
// sees both directions of the edge graph.
//
// Today's `/api/products/{id}/dependencies` returns the upstream
// list. Downstream isn't a separate endpoint — populated when the
// hierarchy walker exposes it. Until then the downstream section is
// hidden so we don't show a misleading empty card.
export function DependenciesTab({ productId }: { productId: string }) {
  const upstream = useProductDependencies(productId)

  return (
    <div className='space-y-4'>
      <Card>
        <CardHeader>
          <CardTitle className='flex items-center justify-between text-base'>
            <span className='inline-flex items-center gap-2'>
              <ArrowRight className='h-4 w-4 opacity-60' />
              This product depends on
            </span>
            <Badge variant='outline' className='text-[10px]'>
              {upstream.data?.length ?? 0}
            </Badge>
          </CardTitle>
        </CardHeader>
        <CardContent>
          {upstream.isLoading ? (
            <Skeleton className='h-16 w-full' />
          ) : upstream.isError ? (
            <div className='text-sm text-rose-400'>
              Failed: {String(upstream.error)}
            </div>
          ) : !upstream.data || upstream.data.length === 0 ? (
            <div className='text-muted-foreground text-sm'>
              No dependencies declared.
            </div>
          ) : (
            <div className='space-y-1.5 text-sm'>
              {upstream.data.map((d) => (
                <Link
                  key={d.id}
                  to='/products'
                  search={{ id: d.id, tab: 'main' }}
                  className='hover:bg-accent flex items-center justify-between gap-2 rounded-md px-3 py-2'
                >
                  <div className='min-w-0'>
                    <div className='truncate font-medium'>{d.title}</div>
                    <code className='text-muted-foreground font-mono text-[11px]'>
                      {d.marker}
                    </code>
                  </div>
                  <Badge
                    variant='secondary'
                    className={`shrink-0 text-[10px] uppercase ${STATUS_COLOR[d.status] ?? 'bg-zinc-500/15'}`}
                  >
                    {d.status}
                  </Badge>
                </Link>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='text-base text-muted-foreground'>
            Edit dependencies
          </CardTitle>
        </CardHeader>
        <CardContent className='text-muted-foreground text-sm'>
          Add / remove edges lands in a follow-up alongside the create
          / edit dialogs. For now this view is read-only.
        </CardContent>
      </Card>
    </div>
  )
}
