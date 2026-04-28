import { useProduct, useProductHierarchy } from '@/lib/queries/products'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'

// Stats tab — at-a-glance numbers for this product. Aggregates roll
// up from manifests + tasks underneath. Detailed time-series charts
// (cost/day, throughput) land in a follow-up wired to host_samples.
export function StatsTab({ productId }: { productId: string }) {
  const product = useProduct(productId)
  const hierarchy = useProductHierarchy(productId)

  if (product.isLoading) {
    return (
      <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-4'>
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className='h-20 w-full' />
        ))}
      </div>
    )
  }
  if (product.isError || !product.data) {
    return <div className='text-sm text-rose-400'>Failed to load product.</div>
  }
  const p = product.data
  const subProductCount = hierarchy.data?.sub_products?.length ?? 0
  const created = p.created_at ? new Date(p.created_at) : null
  const updated = p.updated_at ? new Date(p.updated_at) : null

  return (
    <div className='space-y-3'>
      <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-4'>
        <Stat label='Total cost' value={fmtCost(p.total_cost ?? 0)} />
        <Stat label='Manifests' value={String(p.total_manifests ?? 0)} />
        <Stat label='Tasks' value={String(p.total_tasks ?? 0)} />
        <Stat label='Turns' value={String(p.total_turns ?? 0)} />
      </div>

      <Card>
        <CardContent className='space-y-2 p-3 text-sm'>
          <Row label='Sub-products' value={String(subProductCount)} />
          <Row
            label='Status'
            value={
              <Badge variant='outline' className='text-[10px] uppercase'>
                {p.status}
              </Badge>
            }
          />
          {p.tags && p.tags.length > 0 ? (
            <Row
              label='Tags'
              value={
                <div className='flex flex-wrap justify-end gap-1'>
                  {p.tags.map((t) => (
                    <Badge key={t} variant='secondary' className='text-[10px]'>
                      {t}
                    </Badge>
                  ))}
                </div>
              }
            />
          ) : null}
          {p.source_node ? (
            <Row
              label='Source node'
              value={<code className='font-mono text-xs'>{p.source_node}</code>}
            />
          ) : null}
          {created ? (
            <Row label='Created' value={created.toLocaleString()} />
          ) : null}
          {updated ? (
            <Row label='Updated' value={updated.toLocaleString()} />
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardContent className='flex flex-col items-start gap-0.5 p-3'>
        <span className='text-muted-foreground text-[10px] uppercase tracking-wider'>
          {label}
        </span>
        <span className='font-mono text-xl font-semibold'>{value}</span>
      </CardContent>
    </Card>
  )
}

function Row({
  label,
  value,
}: {
  label: string
  value: React.ReactNode
}) {
  return (
    <div className='flex items-center justify-between gap-3'>
      <span className='text-muted-foreground'>{label}</span>
      <div className='text-foreground'>{value}</div>
    </div>
  )
}

function fmtCost(n: number): string {
  return '$' + n.toFixed(2)
}
