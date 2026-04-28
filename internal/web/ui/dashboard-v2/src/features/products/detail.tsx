import { useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { useProduct } from '@/lib/queries/products'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ProductsBreadcrumb } from './breadcrumb'
import { CommentsTab } from './tabs/comments'
import { DAGTab } from './tabs/dag'
import { DependenciesTab } from './tabs/dependencies'
import { DescriptionTab } from './tabs/description'
import { MainTab } from './tabs/main'
import { StatsTab } from './tabs/stats'

const TAB_IDS = [
  'main',
  'description',
  'stats',
  'comments',
  'dependencies',
  'dag',
] as const

const STATUS_COLOR: Record<string, string> = {
  open: 'bg-emerald-500/15 text-emerald-500',
  in_progress: 'bg-sky-500/15 text-sky-500',
  draft: 'bg-amber-500/15 text-amber-500',
  closed: 'bg-zinc-500/15 text-zinc-400',
  archived: 'bg-zinc-500/10 text-zinc-500',
  cancelled: 'bg-rose-500/15 text-rose-500',
}

// Per-product detail page. Six tabs, each its own component, all
// reading from react-query so switching tabs doesn't refetch unless
// the cache is stale. Title + status + breadcrumb live ABOVE the
// tab strip — they're the "where am I" anchor regardless of which
// tab is active.
export function ProductDetail() {
  const { productId } = useParams({ from: '/_authenticated/products/$productId' })
  const search = useSearch({ from: '/_authenticated/products/$productId' })
  const navigate = useNavigate({ from: '/_authenticated/products/$productId' })
  const product = useProduct(productId)

  const setTab = (tab: (typeof TAB_IDS)[number]) => {
    navigate({ search: { tab } })
  }

  return (
    <>
      <Header />
      <Main>
        <ProductsBreadcrumb
          productId={productId}
          productTitle={product.data?.title}
        />

        <div className='mt-3 mb-4'>
          {product.isLoading ? (
            <Skeleton className='h-8 w-1/2' />
          ) : product.isError ? (
            <div className='text-sm text-rose-400'>
              Failed to load: {String(product.error)}
            </div>
          ) : product.data ? (
            <div className='flex items-start justify-between gap-3'>
              <div>
                <h1 className='text-2xl font-bold tracking-tight'>
                  {product.data.title}
                </h1>
                <code className='text-muted-foreground font-mono text-xs'>
                  {product.data.marker}
                </code>
              </div>
              <Badge
                variant='secondary'
                className={`shrink-0 uppercase ${STATUS_COLOR[product.data.status] ?? 'bg-zinc-500/15'}`}
              >
                {product.data.status}
              </Badge>
            </div>
          ) : null}
        </div>

        <Tabs
          value={search.tab ?? 'main'}
          onValueChange={(v) => setTab(v as (typeof TAB_IDS)[number])}
          className='space-y-4'
        >
          <TabsList>
            <TabsTrigger value='main'>Main</TabsTrigger>
            <TabsTrigger value='description'>Description</TabsTrigger>
            <TabsTrigger value='stats'>Stats</TabsTrigger>
            <TabsTrigger value='comments'>Comments</TabsTrigger>
            <TabsTrigger value='dependencies'>Dependencies</TabsTrigger>
            <TabsTrigger value='dag'>DAG</TabsTrigger>
          </TabsList>

          <TabsContent value='main'>
            <MainTab productId={productId} />
          </TabsContent>
          <TabsContent value='description'>
            <DescriptionTab productId={productId} />
          </TabsContent>
          <TabsContent value='stats'>
            <StatsTab productId={productId} />
          </TabsContent>
          <TabsContent value='comments'>
            <CommentsTab productId={productId} />
          </TabsContent>
          <TabsContent value='dependencies'>
            <DependenciesTab productId={productId} />
          </TabsContent>
          <TabsContent value='dag'>
            <DAGTab productId={productId} />
          </TabsContent>
        </Tabs>
      </Main>
    </>
  )
}
