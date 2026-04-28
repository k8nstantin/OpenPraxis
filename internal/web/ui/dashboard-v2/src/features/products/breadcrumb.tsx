import { Link } from '@tanstack/react-router'
import { ChevronRight, Home } from 'lucide-react'
import { useProductHierarchy } from '@/lib/queries/products'
import type { HierarchyNode } from '@/lib/types'

// Walk the hierarchy tree to find the path from the root to the
// target product id. Returns null if the target isn't reachable
// from the supplied root (e.g., the operator deep-linked into a
// product whose root we haven't fetched).
function findPath(
  root: HierarchyNode | undefined,
  targetId: string
): HierarchyNode[] | null {
  if (!root) return null
  if (root.id === targetId) return [root]
  const children = [...(root.sub_products ?? []), ...(root.children ?? [])]
  for (const c of children) {
    const sub = findPath(c, targetId)
    if (sub) return [root, ...sub]
  }
  return null
}

// ProductsBreadcrumb renders the ancestor chain. If we can't compute
// the chain (e.g., deep-linked into a product whose umbrella we don't
// know yet), we fall back to "Products / <current>" — still useful as
// a navigation affordance up to the top-level list.
export function ProductsBreadcrumb({
  productId,
  productTitle,
}: {
  productId: string
  productTitle?: string
}) {
  // Best-effort: try the supplied product's hierarchy as a root. If the
  // product itself IS a root (umbrella), the path is just [self]; if
  // it's nested, this won't resolve and we render the fallback.
  const hierarchy = useProductHierarchy(productId)
  const path = hierarchy.data ? findPath(hierarchy.data, productId) : null

  return (
    <nav
      aria-label='Breadcrumb'
      className='text-muted-foreground flex items-center gap-1.5 text-sm'
    >
      <Link
        to='/products'
        className='hover:text-foreground inline-flex items-center gap-1'
      >
        <Home className='h-3.5 w-3.5' />
        Products
      </Link>
      {path && path.length > 0 ? (
        path.map((node) => (
          <span key={node.id} className='inline-flex items-center gap-1.5'>
            <ChevronRight className='h-3.5 w-3.5 opacity-50' />
            {node.id === productId ? (
              <span className='text-foreground font-medium'>{node.title}</span>
            ) : (
              <Link
                to='/products/$productId'
                params={{ productId: node.id }}
                search={{ tab: 'main' }}
                className='hover:text-foreground'
              >
                {node.title}
              </Link>
            )}
          </span>
        ))
      ) : productTitle ? (
        <span className='inline-flex items-center gap-1.5'>
          <ChevronRight className='h-3.5 w-3.5 opacity-50' />
          <span className='text-foreground font-medium'>{productTitle}</span>
        </span>
      ) : null}
    </nav>
  )
}
