import { createFileRoute } from '@tanstack/react-router'
import { ProductsList } from '@/features/products/list'

export const Route = createFileRoute('/_authenticated/products')({
  component: ProductsList,
})
