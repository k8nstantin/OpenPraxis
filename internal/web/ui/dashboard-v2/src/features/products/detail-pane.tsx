import { useProduct } from '@/lib/queries/products'
import { Boxes } from 'lucide-react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ProductsBreadcrumb } from './breadcrumb'
import { ProductStatusControl } from './status-control'
import { CommentsTab } from './tabs/comments'
import { DAGTab } from './tabs/dag'
import { DependenciesTab } from './tabs/dependencies'
import { ExecutionTab } from './tabs/execution'
import { MainTab } from './tabs/main'

// Five tabs in operator-priority order. Main combines stats + the
// editable description (with revision history). Execution exposes the
// full settings catalog scoped to this product.
const TAB_IDS = [
  'main',
  'execution',
  'comments',
  'dependencies',
  'dag',
] as const

export type ProductsTabId = (typeof TAB_IDS)[number]

// Right-pane content for the master-detail Products layout. Renders
// breadcrumb + title + status + 5-tab strip for the selected product.
// No outer Header/Main — the parent ProductsPage owns those wrappers.
export function ProductDetailPane({
  productId,
  tab,
  onTabChange,
}: {
  productId?: string
  tab: ProductsTabId
  onTabChange: (tab: ProductsTabId) => void
}) {
  const product = useProduct(productId)

  if (!productId) {
    return (
      <div className='text-muted-foreground flex h-full flex-col items-center justify-center gap-3 p-6 text-center'>
        <Boxes className='h-12 w-12 opacity-30' />
        <div className='text-sm'>
          Pick a product from the list to see its tabs.
        </div>
      </div>
    )
  }

  return (
    <div className='flex h-full min-h-0 flex-col'>
      <ScrollArea className='min-h-0 flex-1'>
        <div className='space-y-4 p-4'>
          <ProductsBreadcrumb
            productId={productId}
            productTitle={product.data?.title}
          />

          <div>
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
                    {product.data.id}
                  </code>
                </div>
                <ProductStatusControl
                  productId={productId}
                  status={product.data.status}
                  productTitle={product.data.title}
                />
              </div>
            ) : null}
          </div>

          <Tabs
            value={tab}
            onValueChange={(v) => onTabChange(v as ProductsTabId)}
            className='space-y-2'
          >
            <TabsList>
              <TabsTrigger value='main'>Main</TabsTrigger>
              <TabsTrigger value='execution'>Execution Control</TabsTrigger>
              <TabsTrigger value='comments'>Comments</TabsTrigger>
              <TabsTrigger value='dependencies'>Dependencies</TabsTrigger>
              <TabsTrigger value='dag'>DAG</TabsTrigger>
            </TabsList>

            <TabsContent value='main'>
              <MainTab productId={productId} />
            </TabsContent>
            <TabsContent value='execution'>
              <ExecutionTab productId={productId} />
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
        </div>
      </ScrollArea>
    </div>
  )
}
