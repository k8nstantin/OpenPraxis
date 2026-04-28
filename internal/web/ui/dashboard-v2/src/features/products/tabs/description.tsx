import { useState } from 'react'
import { Pencil } from 'lucide-react'
import {
  useProduct,
  useProductDescriptionHistory,
  useUpdateProduct,
} from '@/lib/queries/products'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DescriptionView } from '@/components/description-view'
import { MarkdownEditor } from '@/components/markdown-editor'

// Description tab — view (markup ↔ rendered toggle, same as Portal A)
// + inline edit. Click pencil → swap to MarkdownEditor with the same
// toolbar + shortcuts Portal A uses (Cmd-Enter saves, Escape cancels).
// Save → PUT /api/products/{id}; SCD-2 description revision row
// auto-created server-side, surfaces in revision history below.
export function DescriptionTab({ productId }: { productId: string }) {
  const product = useProduct(productId)
  const history = useProductDescriptionHistory(productId)
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

  return (
    <div className='space-y-3'>
      <Card>
        <CardHeader className='flex flex-row items-center justify-between space-y-0 py-3'>
          <CardTitle className='text-sm font-medium'>
            Current description
          </CardTitle>
          {!editing && !product.isLoading ? (
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
          ) : null}
        </CardHeader>
        <CardContent className='pt-0 pb-3'>
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
