import {
  useProduct,
  useProductDescriptionHistory,
} from '@/lib/queries/products'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'

// Description tab — current body up top, revision history below.
// Revisions are SCD-2 description rows from /api/descriptions/.
export function DescriptionTab({ productId }: { productId: string }) {
  const product = useProduct(productId)
  const history = useProductDescriptionHistory(productId)

  return (
    <div className='space-y-3'>
      <Card>
        <CardHeader className='py-3'>
          <CardTitle className='text-sm font-medium'>
            Current description
          </CardTitle>
        </CardHeader>
        <CardContent className='pt-0 pb-3'>
          {product.isLoading ? (
            <Skeleton className='h-24 w-full' />
          ) : product.data?.description ? (
            <pre className='font-mono text-sm whitespace-pre-wrap break-words'>
              {product.data.description}
            </pre>
          ) : (
            <div className='text-muted-foreground text-sm italic'>
              No description set.
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className='py-3'>
          <CardTitle className='flex items-center justify-between text-sm font-medium'>
            <span>Revision history</span>
            <Badge variant='outline' className='text-[10px]'>
              {history.data?.length ?? 0}
            </Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className='pt-0 pb-3'>
          {history.isLoading ? (
            <Skeleton className='h-12 w-full' />
          ) : !history.data || history.data.length === 0 ? (
            <div className='text-muted-foreground text-sm'>
              No prior revisions recorded.
            </div>
          ) : (
            <div className='divide-y'>
              {history.data.map((rev) => (
                <div key={rev.id} className='space-y-1 py-2 text-sm'>
                  <div className='flex items-center justify-between'>
                    <code className='font-mono text-[11px]'>
                      {rev.author.slice(0, 16)}
                    </code>
                    <span className='text-muted-foreground text-xs'>
                      {fmtTime(rev.created_at)}
                    </span>
                  </div>
                  <pre className='text-muted-foreground line-clamp-3 font-mono text-xs whitespace-pre-wrap break-words'>
                    {rev.body}
                  </pre>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function fmtTime(ts: number | string): string {
  const t = typeof ts === 'number' ? ts * 1000 : Date.parse(ts)
  if (!Number.isFinite(t)) return '—'
  return new Date(t).toLocaleString()
}
