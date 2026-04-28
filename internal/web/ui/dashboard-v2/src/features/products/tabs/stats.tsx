import { useProduct } from '@/lib/queries/products'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

// Stats tab — at-a-glance numbers for this product. Aggregates roll
// up from manifests + tasks underneath. More detailed time-series
// charts (cost/day, throughput) land in a follow-up once we wire
// host_samples queries here.
export function StatsTab({ productId }: { productId: string }) {
  const product = useProduct(productId)

  if (product.isLoading) {
    return (
      <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className='h-24 w-full' />
        ))}
      </div>
    )
  }
  if (product.isError || !product.data) {
    return (
      <div className='text-sm text-rose-400'>
        Failed to load product.
      </div>
    )
  }
  const p = product.data

  return (
    <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
      <Stat label='Total cost' value={fmtCost(p.total_cost ?? 0)} />
      <Stat label='Manifests' value={String(p.total_manifests ?? 0)} />
      <Stat label='Tasks' value={String(p.total_tasks ?? 0)} />
      <Stat label='Total turns' value={String(p.total_turns ?? 0)} />
    </div>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardContent className='flex flex-col items-start gap-1 p-4'>
        <span className='text-muted-foreground text-xs uppercase tracking-wider'>
          {label}
        </span>
        <span className='font-mono text-2xl font-semibold'>{value}</span>
      </CardContent>
    </Card>
  )
}

function fmtCost(n: number): string {
  return '$' + n.toFixed(2)
}
