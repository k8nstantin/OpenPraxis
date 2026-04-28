import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { ProductsPage } from '@/features/products'

// Master-detail Products page. Both selection (`id`) and active tab
// (`tab`) live as search params so the URL is shareable / reload-safe
// and the browser-back stack actually works.
const productsSearch = z.object({
  id: z.string().optional(),
  tab: z
    .enum(['description', 'comments', 'dependencies', 'dag', 'stats'])
    .optional()
    .default('description'),
})

export const Route = createFileRoute('/_authenticated/products')({
  validateSearch: productsSearch,
  component: ProductsPage,
})
