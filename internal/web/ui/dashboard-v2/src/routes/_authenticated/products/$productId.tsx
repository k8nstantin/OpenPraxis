import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { ProductDetail } from '@/features/products/detail'

// Tab is captured as a search param (?tab=description) rather than a
// path segment so it can be persisted across product switches and
// keyboard-shortcut navigation without remounting the whole detail.
// Defaults to "main" — the operator's primary lens.
const productDetailSearch = z.object({
  tab: z
    .enum(['main', 'description', 'stats', 'comments', 'dependencies', 'dag'])
    .optional()
    .default('main'),
})

export const Route = createFileRoute('/_authenticated/products/$productId')({
  validateSearch: productDetailSearch,
  component: ProductDetail,
})
