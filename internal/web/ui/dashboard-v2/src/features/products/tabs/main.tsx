import { useEffect, useState } from 'react'
import { Pencil } from 'lucide-react'
import {
  useProduct,
  useProductDescriptionHistory,
  useUpdateProduct,
} from '@/lib/queries/products'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DescriptionView } from '@/components/description-view'
import { Gauge } from '@/components/gauge'
import { MarkdownEditor } from '@/components/markdown-editor'

// Main tab — stats grid + repo card + description editor + revision
// history. Same Markup ↔ Rendered toggle as Portal A; Cmd-Enter saves,
// Escape cancels. PUT /api/products/{id} drops a new SCD-2 description
// revision row server-side, surfaced in the history card below.
//
// Stats: 6 compact cards in operator-priority order — Estimated Cost,
// Actual, Turns, Actions, Tokens, Model. Estimated/Actions/Tokens are
// pending server-side wiring (cost-prediction product 019db4ba is the
// dependency); they render as "—" until those numeric fields surface
// on the product API.
export function MainTab({ productId }: { productId: string }) {
  const product = useProduct(productId)
  const history = useProductDescriptionHistory(productId)
  const update = useUpdateProduct(productId)
  const [repoInfo, setRepoInfo] = useState<Record<string, string | number>>(
    {}
  )

  useEffect(() => {
    let cancelled = false
    fetch(`/api/products/${productId}/settings`)
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => {
        if (cancelled || !d) return
        const out: Record<string, string | number> = {}
        for (const e of (d.entries ?? []) as {
          key: string
          value: string
        }[]) {
          try {
            out[e.key] = JSON.parse(e.value)
          } catch {
            out[e.key] = e.value
          }
        }
        setRepoInfo(out)
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [productId])

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
  const created = p?.created_at ? new Date(p.created_at) : null
  const updated = p?.updated_at ? new Date(p.updated_at) : null

  // Resolved daily budget from settings — defaults to the catalog
  // default (100 USD) when the operator hasn't set a product-scope
  // override. Drives both cost gauges' red-line + tone.
  const budgetRaw = repoInfo.daily_budget_usd
  const budget = typeof budgetRaw === 'number' && budgetRaw > 0 ? budgetRaw : 100
  const actual = p?.total_cost ?? 0
  const costMax = budget * 1.5
  const costTone = (v: number) =>
    v >= budget
      ? 'text-rose-500'
      : v >= budget * 0.8
        ? 'text-amber-500'
        : 'text-emerald-500'

  return (
    <div className='space-y-2'>
      <div className='grid grid-cols-3 gap-1 lg:grid-cols-6'>
        {product.isLoading || !p ? (
          Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className='h-6 w-full' />
          ))
        ) : (
          <>
            <Stat label='Estimated Cost' value='—' />
            <Stat label='Actual' value={fmtCost(actual)} />
            <Stat label='Turns' value={String(p.total_turns ?? 0)} />
            <Stat label='Actions' value='—' />
            <Stat label='Tokens' value='—' />
            <Stat
              label='Model'
              value={repoInfo.default_model || 'default'}
            />
          </>
        )}
      </div>

      {p ? (
        <div className='grid grid-cols-2 gap-2'>
          <div className={`flex flex-col items-stretch ${costTone(0)} opacity-50`}>
            <Gauge
              label='estimated cost'
              value={0}
              min={0}
              max={costMax}
              unit='USD'
              redLine={budget}
            />
            <div className='text-muted-foreground px-1 pt-0.5 text-center text-[10px]'>
              pending estimator
            </div>
          </div>
          <div className={`flex flex-col items-stretch ${costTone(actual)}`}>
            <Gauge
              label='actual cost'
              value={actual}
              min={0}
              max={costMax}
              unit='USD'
              redLine={budget}
            />
            <div className='text-muted-foreground px-1 pt-0.5 text-center text-[10px]'>
              red line = daily budget ({fmtCost(budget)})
            </div>
          </div>
        </div>
      ) : null}

      {p ? (
        <Card className='gap-0 py-0'>
          <CardContent className='space-y-0.5 px-3 py-2 text-sm'>
            <Row
              label='Repo'
              value={
                repoInfo.worktree_base_dir ? (
                  <code className='font-mono text-xs'>
                    {repoInfo.worktree_base_dir}
                  </code>
                ) : (
                  <span className='text-muted-foreground'>
                    (worktree base from settings)
                  </span>
                )
              }
            />
            <Row
              label='Branch prefix'
              value={
                <code className='font-mono text-xs'>
                  {repoInfo.branch_prefix || 'openpraxis'}
                </code>
              }
            />
            <Row
              label='Agent'
              value={
                <code className='font-mono text-xs'>
                  {repoInfo.default_agent || 'claude-code'}
                </code>
              }
            />
            <Row
              label='Status'
              value={
                <Badge variant='outline' className='text-[10px] uppercase'>
                  {p.status}
                </Badge>
              }
            />
            {created ? (
              <Row label='Created' value={created.toLocaleString()} />
            ) : null}
            {updated ? (
              <Row label='Updated' value={updated.toLocaleString()} />
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      <Card className='gap-0 py-0'>
        <CardContent className='space-y-2 px-3 py-2'>
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

      <Card className='gap-0 py-0'>
        <CardContent className='space-y-1 px-3 py-2'>
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
    <div className='bg-card flex items-center justify-between gap-2 rounded-md border px-2 py-1'>
      <span className='text-muted-foreground text-[9px] uppercase tracking-wider'>
        {label}
      </span>
      <span
        className='truncate font-mono text-xs font-semibold'
        title={value}
      >
        {value}
      </span>
    </div>
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

