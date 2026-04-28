import { Link } from '@tanstack/react-router'
import {
  useProduct,
  useProductHierarchy,
  useProductIdeas,
  useProductManifests,
} from '@/lib/queries/products'
import type { HierarchyNode, Idea, Manifest } from '@/lib/types'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

const STATUS_COLOR: Record<string, string> = {
  open: 'bg-emerald-500/15 text-emerald-500',
  in_progress: 'bg-sky-500/15 text-sky-500',
  draft: 'bg-amber-500/15 text-amber-500',
  closed: 'bg-zinc-500/15 text-zinc-400',
  archived: 'bg-zinc-500/10 text-zinc-500',
  cancelled: 'bg-rose-500/15 text-rose-500',
}

// Main tab — the "where do I go next from here" view. Three sections:
//   1. Sub-products (clickable → drill in via breadcrumb)
//   2. Linked manifests (read-only preview for now; manifest detail
//      is its own future top-level tab)
//   3. Linked ideas (read-only preview)
//
// Description body, stats, comments, dependencies, DAG live in their
// own tabs — the operator picks the lens, doesn't scroll a wall.
export function MainTab({ productId }: { productId: string }) {
  const product = useProduct(productId)
  const hierarchy = useProductHierarchy(productId)
  const manifests = useProductManifests(productId)
  const ideas = useProductIdeas(productId)

  // Sub-products come from the hierarchy endpoint's `sub_products`
  // array on the root node (the product itself). If the response is
  // missing that field we fall back to an empty list.
  const subProducts: HierarchyNode[] = hierarchy.data?.sub_products ?? []

  return (
    <div className='space-y-4'>
      <SubProductsCard
        loading={hierarchy.isLoading}
        items={subProducts}
      />
      <ManifestsCard
        loading={manifests.isLoading}
        error={manifests.isError ? String(manifests.error) : undefined}
        items={manifests.data ?? []}
        totalFromProduct={product.data?.total_manifests}
      />
      <IdeasCard
        loading={ideas.isLoading}
        error={ideas.isError ? String(ideas.error) : undefined}
        items={ideas.data ?? []}
      />
    </div>
  )
}

function SubProductsCard({
  loading,
  items,
}: {
  loading: boolean
  items: HierarchyNode[]
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center justify-between text-base'>
          <span>Sub-products</span>
          <Badge variant='outline' className='text-[10px]'>
            {items.length}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className='h-10 w-full' />
        ) : items.length === 0 ? (
          <div className='text-muted-foreground text-sm'>
            No sub-products. Drill into a sub-product to see its own children.
          </div>
        ) : (
          <div className='grid gap-2 sm:grid-cols-2'>
            {items.map((sp) => (
              <Link
                key={sp.id}
                to='/products'
                search={{ id: sp.id, tab: 'main' }}
                className='hover:border-primary/50 group flex items-center justify-between gap-2 rounded-md border p-3 text-sm transition-colors'
              >
                <div className='min-w-0'>
                  <div className='truncate font-medium group-hover:underline'>
                    {sp.title}
                  </div>
                  <code className='text-muted-foreground font-mono text-[11px]'>
                    {sp.marker}
                  </code>
                </div>
                <Badge
                  variant='secondary'
                  className={`shrink-0 text-[10px] uppercase ${STATUS_COLOR[sp.status] ?? 'bg-zinc-500/15'}`}
                >
                  {sp.status}
                </Badge>
              </Link>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function ManifestsCard({
  loading,
  error,
  items,
  totalFromProduct,
}: {
  loading: boolean
  error?: string
  items: Manifest[]
  totalFromProduct?: number
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center justify-between text-base'>
          <span>Manifests</span>
          <Badge variant='outline' className='text-[10px]'>
            {items.length || totalFromProduct || 0}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className='h-10 w-full' />
        ) : error ? (
          <div className='text-sm text-rose-400'>Failed: {error}</div>
        ) : items.length === 0 ? (
          <div className='text-muted-foreground text-sm'>
            No manifests linked.
          </div>
        ) : (
          <div className='space-y-1.5 text-sm'>
            {items.map((m) => (
              <div
                key={m.id}
                className='flex items-center justify-between gap-2 py-1'
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
  )
}

function IdeasCard({
  loading,
  error,
  items,
}: {
  loading: boolean
  error?: string
  items: Idea[]
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center justify-between text-base'>
          <span>Ideas</span>
          <Badge variant='outline' className='text-[10px]'>
            {items.length}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className='h-10 w-full' />
        ) : error ? (
          <div className='text-sm text-rose-400'>Failed: {error}</div>
        ) : items.length === 0 ? (
          <div className='text-muted-foreground text-sm'>No ideas linked.</div>
        ) : (
          <div className='space-y-1.5 text-sm'>
            {items.map((i) => (
              <div key={i.id} className='py-1'>
                <div className='font-medium'>{i.title}</div>
                {i.description ? (
                  <div className='text-muted-foreground line-clamp-2 text-xs'>
                    {i.description}
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
