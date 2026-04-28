import { Link } from '@tanstack/react-router'
import {
  useProductDependencies,
  useProductManifests,
} from '@/lib/queries/products'
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

// Dependencies tab — what this product is composed of, in two
// sections:
//
//   1. Sub-products  — clickable; click → drill into that sub-product
//      (URL ?id=<sub-id>, breadcrumb extends, list pane swaps).
//
//   2. Manifests     — read-only rows (manifest detail surface lands
//      with its own top-level menu).
//
// Edge editor (add / remove / reparent) deferred. Upstream "this
// product depends on a parent" lands once we have a parent-lookup
// endpoint or the breadcrumb hierarchy walker is generalized.
export function DependenciesTab({ productId }: { productId: string }) {
  const subs = useProductDependencies(productId)
  const manifests = useProductManifests(productId)

  return (
    <div className='space-y-3'>
      <Card>
        <CardHeader className='py-3'>
          <CardTitle className='flex items-center justify-between text-sm font-medium'>
            <span>Sub-products</span>
            <Badge variant='outline' className='text-[10px]'>
              {subs.data?.length ?? 0}
            </Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className='pt-0 pb-3'>
          {subs.isLoading ? (
            <Skeleton className='h-12 w-full' />
          ) : subs.isError ? (
            <div className='text-sm text-rose-400'>
              Failed: {String(subs.error)}
            </div>
          ) : !subs.data || subs.data.length === 0 ? (
            <div className='text-muted-foreground text-sm'>
              No sub-products. This product is a leaf.
            </div>
          ) : (
            <div className='space-y-1 text-sm'>
              {subs.data.map((d) => (
                <Link
                  key={d.id}
                  to='/products'
                  search={{ id: d.id, tab: 'description' }}
                  className='hover:bg-accent flex items-center justify-between gap-2 rounded-md px-2 py-1.5'
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
        <CardHeader className='py-3'>
          <CardTitle className='flex items-center justify-between text-sm font-medium'>
            <span>Manifests</span>
            <Badge variant='outline' className='text-[10px]'>
              {manifests.data?.length ?? 0}
            </Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className='pt-0 pb-3'>
          {manifests.isLoading ? (
            <Skeleton className='h-12 w-full' />
          ) : manifests.isError ? (
            <div className='text-sm text-rose-400'>
              Failed: {String(manifests.error)}
            </div>
          ) : !manifests.data || manifests.data.length === 0 ? (
            <div className='text-muted-foreground text-sm'>
              No manifests linked.
            </div>
          ) : (
            <div className='space-y-1 text-sm'>
              {manifests.data.map((m) => (
                <div
                  key={m.id}
                  className='flex items-center justify-between gap-2 rounded-md px-2 py-1.5'
                >
                  <div className='min-w-0'>
                    <div className='truncate font-medium'>{m.title}</div>
                    <code className='text-muted-foreground font-mono text-[11px]'>
                      {m.marker}
                    </code>
                  </div>
                  <Badge
                    variant='secondary'
                    className={`shrink-0 text-[10px] uppercase ${STATUS_COLOR[m.status] ?? 'bg-zinc-500/15'}`}
                  >
                    {m.status}
                  </Badge>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
