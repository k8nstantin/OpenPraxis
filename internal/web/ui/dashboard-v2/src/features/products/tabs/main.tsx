import { useState } from 'react'
import { Pencil } from 'lucide-react'
import {
  useProduct,
  useProductDescriptionHistory,
  useProductHierarchy,
  useUpdateProduct,
} from '@/lib/queries/products'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DescriptionView } from '@/components/description-view'
import { MarkdownEditor } from '@/components/markdown-editor'

// Main tab — stats grid + description editor + description revision
// history. Same Markup ↔ Rendered toggle as Portal A; Cmd-Enter saves,
// Escape cancels. PUT /api/products/{id} drops a new SCD-2 description
// revision row server-side, surfaced in the history card below.
export function MainTab({ productId }: { productId: string }) {
  const product = useProduct(productId)
  const history = useProductDescriptionHistory(productId)
  const hierarchy = useProductHierarchy(productId)
  const update = useUpdateProduct(productId)

  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')

  const startEdit = () => {
    setDraft(product.data?.description ?? '')
    setEditing(true)
  }
  const cancel = () => {
    setEditing(false)
    setDraft('')
  }
  const save = async () => {
    try {
      await update.mutateAsync({ description: draft })
      setEditing(false)
      setDraft('')
    } catch (e) {
      console.error(e)
    }
  }

  const p = product.data
  const subProductCount = hierarchy.data?.sub_products?.length ?? 0
  const created = p?.created_at ? new Date(p.created_at) : null
  const updated = p?.updated_at ? new Date(p.updated_at) : null

  return (
    <div className='space-y-3'>
      <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-4'>
        {product.isLoading || !p ? (
          Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className='h-20 w-full' />
          ))
        ) : (
          <>
            <Stat label='Total cost' value={fmtCost(p.total_cost ?? 0)} />
            <Stat label='Manifests' value={String(p.total_manifests ?? 0)} />
            <Stat label='Tasks' value={String(p.total_tasks ?? 0)} />
            <Stat label='Turns' value={String(p.total_turns ?? 0)} />
          </>
        )}
      </div>

      {p ? (
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
                      <Badge
                        key={t}
                        variant='secondary'
                        className='text-[10px]'
                      >
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
                value={
                  <code className='font-mono text-xs'>{p.source_node}</code>
                }
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
      ) : null}

      <Card>
        <CardContent className='space-y-2 p-3'>
          {!editing && !product.isLoading ? (
            <div className='flex justify-end'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='h-7 px-2 text-xs'
                onClick={startEdit}
              >
                <Pencil className='mr-1 h-3 w-3' />
                Edit
              </Button>
            </div>
          ) : null}
          {product.isLoading ? (
            <Skeleton className='h-24 w-full' />
          ) : editing ? (
            <div className='space-y-2'>
              <MarkdownEditor
                value={draft}
                onChange={setDraft}
                onSave={save}
                onCancel={cancel}
                autoFocus
                placeholder='Product description in markdown…'
              />
              <div className='flex items-center justify-end gap-2'>
                {update.isError ? (
                  <span className='mr-auto text-xs text-rose-400'>
                    Save failed: {String(update.error)}
                  </span>
                ) : null}
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  onClick={cancel}
                  disabled={update.isPending}
                >
                  Cancel
                </Button>
                <Button
                  type='button'
                  size='sm'
                  onClick={save}
                  disabled={update.isPending}
                >
                  {update.isPending ? 'Saving…' : 'Save'}
                </Button>
              </div>
            </div>
          ) : (
            <DescriptionView
              raw={product.data?.description}
              rendered={
                (product.data as Record<string, unknown> | undefined)?.[
                  'description_html'
                ] as string | undefined
              }
            />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardContent className='space-y-2 p-3'>
          <div className='flex items-center justify-between'>
            <span className='text-muted-foreground text-xs uppercase tracking-wider'>
              Revisions
            </span>
            <Badge variant='outline' className='text-[10px]'>
              {history.data?.length ?? 0}
            </Badge>
          </div>
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

function fmtTime(ts: number | string): string {
  const t = typeof ts === 'number' ? ts * 1000 : Date.parse(ts)
  if (!Number.isFinite(t)) return '—'
  return new Date(t).toLocaleString()
}
