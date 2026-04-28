import { Network } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// DAG tab — visual graph of THIS product's neighborhood. Cytoscape
// (or react-flow) lazy boundary lands in a follow-up; the canvas is
// the heaviest tab and we want the rest of the Products surface
// shipped + verified before we pull in that bundle.
//
// Until then this card explains what's coming and links the operator
// to the legacy Portal A DAG viewer for the same product.
export function DAGTab({ productId }: { productId: string }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2 text-base'>
          <Network className='h-4 w-4 opacity-60' />
          Product DAG
        </CardTitle>
      </CardHeader>
      <CardContent className='space-y-3 text-sm'>
        <p className='text-muted-foreground'>
          The DAG view (pan / zoom canvas of this product's neighborhood
          — itself, sub-products, manifests, dependency edges) lands in
          a follow-up. The canvas library is a heavy lazy boundary so
          we ship the lighter tabs first and pull it in on its own
          merge.
        </p>
        <p className='text-muted-foreground'>
          Need the graph today? Open the Portal A view at{' '}
          <a
            className='text-primary underline'
            href={`http://localhost:8765/?product=${productId}`}
            target='_blank'
            rel='noreferrer'
          >
            http://localhost:8765/?product={productId.slice(0, 12)}…
          </a>
        </p>
      </CardContent>
    </Card>
  )
}
