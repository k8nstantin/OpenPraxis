import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Boxes, Search } from 'lucide-react'
import { useProducts } from '@/lib/queries/products'
import type { Product } from '@/lib/types'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

const STATUS_COLOR: Record<string, string> = {
  open: 'bg-emerald-500/15 text-emerald-500',
  in_progress: 'bg-sky-500/15 text-sky-500',
  draft: 'bg-amber-500/15 text-amber-500',
  closed: 'bg-zinc-500/15 text-zinc-400',
  archived: 'bg-zinc-500/10 text-zinc-500',
  cancelled: 'bg-rose-500/15 text-rose-500',
}

// Top-level Products page — grid of all products on the current node
// plus discovered peer products (mDNS). Click a card to drill into
// its detail (which has its own 6-tab strip).
//
// "Top-level" here means "render every product in the catalog,"
// not "only umbrella products with no parent" — the breadcrumb on
// the detail page handles ancestor navigation. A future iteration
// can add a filter for "only umbrellas" if the flat list gets noisy.
export function ProductsList() {
  const { data, isLoading, isError, error } = useProducts()
  const [query, setQuery] = useState('')

  const filtered = useMemo<Product[]>(() => {
    if (!data) return []
    const q = query.trim().toLowerCase()
    if (!q) return data
    return data.filter(
      (p) =>
        p.title.toLowerCase().includes(q) ||
        p.marker.toLowerCase().includes(q) ||
        (p.tags ?? []).some((t) => t.toLowerCase().includes(q))
    )
  }, [data, query])

  return (
    <>
      <Header />
      <Main>
        <div className='mb-4 flex items-end justify-between gap-4'>
          <div>
            <h1 className='text-2xl font-bold tracking-tight'>Products</h1>
            <p className='text-muted-foreground text-sm'>
              {isLoading
                ? 'Loading…'
                : isError
                  ? `Failed: ${String(error)}`
                  : `${filtered.length} product${filtered.length === 1 ? '' : 's'}`}
            </p>
          </div>
          <div className='relative w-72'>
            <Search className='text-muted-foreground absolute top-1/2 left-2 h-4 w-4 -translate-y-1/2' />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder='Filter title, marker, tag…'
              className='pl-8'
            />
          </div>
        </div>

        {isLoading ? (
          <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className='h-32 w-full' />
            ))}
          </div>
        ) : filtered.length === 0 ? (
          <Card>
            <CardContent className='text-muted-foreground flex flex-col items-center gap-3 py-12 text-sm'>
              <Boxes className='h-10 w-10 opacity-30' />
              {query
                ? 'No products match the filter.'
                : 'No products yet.'}
            </CardContent>
          </Card>
        ) : (
          <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-3'>
            {filtered.map((p) => (
              <Link
                key={p.id}
                to='/products/$productId'
                params={{ productId: p.id }}
                search={{ tab: 'main' }}
                className='block'
              >
                <Card className='hover:border-primary/40 h-full transition-colors'>
                  <CardHeader>
                    <div className='flex items-start justify-between gap-2'>
                      <CardTitle className='line-clamp-2 text-base'>
                        {p.title}
                      </CardTitle>
                      <Badge
                        variant='secondary'
                        className={cn(
                          'shrink-0 text-[10px] uppercase',
                          STATUS_COLOR[p.status] ?? 'bg-zinc-500/15'
                        )}
                      >
                        {p.status}
                      </Badge>
                    </div>
                    <code className='text-muted-foreground font-mono text-[11px]'>
                      {p.marker}
                    </code>
                  </CardHeader>
                  <CardContent className='space-y-2'>
                    <div className='text-muted-foreground flex flex-wrap gap-3 text-xs'>
                      <span>
                        <span className='font-medium text-foreground'>
                          {p.total_manifests ?? 0}
                        </span>{' '}
                        manifest{(p.total_manifests ?? 0) === 1 ? '' : 's'}
                      </span>
                      <span>
                        <span className='font-medium text-foreground'>
                          {p.total_tasks ?? 0}
                        </span>{' '}
                        task{(p.total_tasks ?? 0) === 1 ? '' : 's'}
                      </span>
                      {p.total_cost ? (
                        <span>
                          <span className='font-medium text-foreground'>
                            ${(p.total_cost ?? 0).toFixed(2)}
                          </span>
                        </span>
                      ) : null}
                    </div>
                    {p.tags && p.tags.length > 0 ? (
                      <div className='flex flex-wrap gap-1'>
                        {p.tags.slice(0, 4).map((t) => (
                          <Badge
                            key={t}
                            variant='outline'
                            className='text-[10px]'
                          >
                            {t}
                          </Badge>
                        ))}
                      </div>
                    ) : null}
                  </CardContent>
                </Card>
              </Link>
            ))}
          </div>
        )}
      </Main>
    </>
  )
}
