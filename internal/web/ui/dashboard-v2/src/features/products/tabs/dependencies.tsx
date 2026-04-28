import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { History, Pencil, Plus, X } from 'lucide-react'
import { toast } from 'sonner'
import {
  useAddDownstreamProductDep,
  useAllManifests,
  useLinkManifest,
  useProductComments,
  useProductDependencies,
  useProductManifests,
  useProducts,
  useRemoveDownstreamProductDep,
  useRestoreDependencySnapshot,
  useUnlinkManifest,
} from '@/lib/queries/products'
import type { Comment } from '@/lib/types'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { DepPicker, type PickerRow } from '../dep-picker'

const STATUS_COLOR: Record<string, string> = {
  open: 'bg-emerald-500/15 text-emerald-500',
  in_progress: 'bg-sky-500/15 text-sky-500',
  draft: 'bg-amber-500/15 text-amber-500',
  closed: 'bg-zinc-500/15 text-zinc-400',
  archived: 'bg-zinc-500/10 text-zinc-500',
}

type ConfirmTarget =
  | {
      kind: 'remove-subproduct'
      row: PickerRow
    }
  | {
      kind: 'remove-manifest'
      row: PickerRow
    }
  | {
      kind: 'restore'
      revisionLabel: string
      snapshot: {
        downstream: string[]
        manifests: string[]
      }
    }

// Dependencies tab — view + edit, with confirmation dialogs on every
// mutating action and a toast on success/failure. Revision history
// items are clickable → Restore prompt → applies the prior snapshot
// back as current state via diff + replay.
export function DependenciesTab({ productId }: { productId: string }) {
  const subs = useProductDependencies(productId)
  const manifests = useProductManifests(productId)
  const allProducts = useProducts()
  const allManifests = useAllManifests()
  const comments = useProductComments(productId)

  const [editing, setEditing] = useState(false)
  const [picker, setPicker] = useState<null | 'subproduct' | 'manifest'>(null)
  const [confirm, setConfirm] = useState<ConfirmTarget | null>(null)

  const addSub = useAddDownstreamProductDep(productId)
  const remSub = useRemoveDownstreamProductDep(productId)
  const linkM = useLinkManifest(productId)
  const unlinkM = useUnlinkManifest(productId)
  const restore = useRestoreDependencySnapshot(productId)

  const subRows: PickerRow[] = (subs.data ?? []).map((d) => ({
    id: d.id,
    marker: d.marker,
    title: d.title,
    status: d.status,
  }))
  const manifestRows: PickerRow[] = (manifests.data ?? []).map((m) => ({
    id: m.id,
    marker: m.marker,
    title: m.title,
    status: m.status,
  }))

  const snapshot = useMemo(
    () => ({
      upstream: [],
      downstream: subRows.map((r) => r.id),
      manifests: manifestRows.map((r) => r.id),
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [subs.data, manifests.data]
  )

  const pickerCandidates = useMemo<PickerRow[]>(() => {
    if (picker === 'manifest') {
      return (allManifests.data ?? [])
        .filter((m) => !manifestRows.some((r) => r.id === m.id))
        .map((m) => ({
          id: m.id,
          marker: m.marker,
          title: m.title,
          status: m.status,
        }))
    }
    if (picker === 'subproduct') {
      const exclude = new Set([productId, ...subRows.map((r) => r.id)])
      return (allProducts.data ?? [])
        .filter((p) => !exclude.has(p.id))
        .map((p) => ({
          id: p.id,
          marker: p.marker,
          title: p.title,
          status: p.status,
        }))
    }
    return []
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [picker, allProducts.data, allManifests.data, subs.data, manifests.data])

  const revisions = useMemo<Comment[]>(() => {
    if (!comments.data) return []
    return comments.data
      .filter((c) => (c.body ?? '').includes('<dependency_revision>'))
      .sort((a, b) => {
        const ta = typeof a.created_at === 'number' ? a.created_at : 0
        const tb = typeof b.created_at === 'number' ? b.created_at : 0
        return tb - ta
      })
  }, [comments.data])

  const onConfirm = async () => {
    if (!confirm) return
    try {
      if (confirm.kind === 'remove-subproduct') {
        await remSub.mutateAsync({ target: confirm.row, snapshot })
        toast.success(`Removed sub-product "${confirm.row.title}"`)
      } else if (confirm.kind === 'remove-manifest') {
        await unlinkM.mutateAsync({ target: confirm.row, snapshot })
        toast.success(`Unlinked manifest "${confirm.row.title}"`)
      } else if (confirm.kind === 'restore') {
        const result = await restore.mutateAsync({
          snapshot: confirm.snapshot,
          revisionLabel: confirm.revisionLabel,
          currentDownstream: subRows,
          currentManifests: manifestRows,
        })
        const summary = result
          ? `+${result.addedSubs + result.addedManifests} −${result.removedSubs + result.removedManifests}`
          : 'snapshot applied'
        toast.success(`Restored to ${confirm.revisionLabel} (${summary})`)
      }
    } catch (e) {
      toast.error(`Failed: ${String(e)}`)
    }
    setConfirm(null)
  }

  return (
    <div className='space-y-3'>
      <Card>
        <CardContent className='space-y-1 p-3 text-sm'>
          <p>
            <span className='font-medium'>
              Products and sub-products both own manifests.
            </span>{' '}
            Each manifest contains tasks (atomic work units agents
            execute). The same editor applies at every level of the tree.
          </p>
          <p className='text-muted-foreground font-mono text-xs'>
            Product ── owns ──▶ Sub-products · Manifests &nbsp; │
            &nbsp; Manifest ── owns ──▶ Tasks (separate menu)
          </p>
        </CardContent>
      </Card>

      <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
        <DepSection
          title='Sub-products'
          subtitle='Products nested under this one. Each is itself a product and can own its own sub-products + manifests. Click a row to drill in.'
          editing={editing}
          rows={subRows}
          loading={subs.isLoading}
          error={subs.error}
          onAdd={() => setPicker('subproduct')}
          onRemove={(row) => setConfirm({ kind: 'remove-subproduct', row })}
          rowHref='/products'
        />
        <DepSection
          title='Manifests'
          subtitle='Plans owned directly by this product (or sub-product) at this level of the tree. Each manifest contains tasks (separate menu).'
          editing={editing}
          rows={manifestRows}
          loading={manifests.isLoading}
          error={manifests.error}
          onAdd={() => setPicker('manifest')}
          onRemove={(row) => setConfirm({ kind: 'remove-manifest', row })}
        />
      </div>

      <div className='flex justify-end'>
        {editing ? (
          <Button
            variant='secondary'
            size='sm'
            onClick={() => setEditing(false)}
          >
            Done
          </Button>
        ) : (
          <Button
            variant='outline'
            size='sm'
            onClick={() => setEditing(true)}
          >
            <Pencil className='mr-1 h-3 w-3' />
            Edit dependencies
          </Button>
        )}
      </div>

      <Card>
        <CardHeader className='py-3'>
          <CardTitle className='flex items-center justify-between text-sm font-medium'>
            <span>
              Revision history
              <Badge variant='outline' className='ml-2 text-[10px]'>
                {revisions.length}
              </Badge>
            </span>
          </CardTitle>
          <p className='text-muted-foreground pt-1 text-xs'>
            Every add / remove logs a snapshot here. Click a row to
            restore that state — the diff is applied as a sequence of
            add/remove ops and a new revision is logged.
          </p>
        </CardHeader>
        <CardContent className='pt-0 pb-3'>
          {comments.isLoading ? (
            <Skeleton className='h-12 w-full' />
          ) : revisions.length === 0 ? (
            <div className='text-muted-foreground text-sm'>
              No dependency changes recorded yet.
            </div>
          ) : (
            <div className='divide-y'>
              {revisions.map((rev) => (
                <RevisionRow
                  key={rev.id}
                  rev={rev}
                  onRestore={(snap, label) =>
                    setConfirm({
                      kind: 'restore',
                      snapshot: snap,
                      revisionLabel: label,
                    })
                  }
                />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <DepPicker
        open={picker !== null}
        onOpenChange={(open) => setPicker(open ? picker : null)}
        title={
          picker === 'subproduct' ? 'Add sub-product' : 'Link a manifest'
        }
        description={
          picker === 'subproduct'
            ? 'Pick a product to nest under this one.'
            : 'Pick a manifest to re-parent under this product.'
        }
        rows={pickerCandidates}
        loading={
          picker === 'manifest'
            ? allManifests.isLoading
            : allProducts.isLoading
        }
        onPick={async (row) => {
          try {
            if (picker === 'subproduct') {
              await addSub.mutateAsync({ target: row, snapshot })
              toast.success(`Added sub-product "${row.title}"`)
            } else if (picker === 'manifest') {
              await linkM.mutateAsync({ target: row, snapshot })
              toast.success(`Linked manifest "${row.title}"`)
            }
          } catch (e) {
            toast.error(`Failed: ${String(e)}`)
          }
        }}
      />

      <AlertDialog
        open={confirm !== null}
        onOpenChange={(open) => !open && setConfirm(null)}
      >
        <AlertDialogContent>
          {confirm?.kind === 'remove-subproduct' ? (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  Remove sub-product?
                </AlertDialogTitle>
                <AlertDialogDescription>
                  Remove <span className='font-medium'>{confirm.row.title}</span>{' '}
                  from this product? It stays as a standalone product —
                  this only severs the parent edge. Action is logged
                  in revision history; you can restore.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={onConfirm}>
                  Remove
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          ) : confirm?.kind === 'remove-manifest' ? (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Unlink manifest?</AlertDialogTitle>
                <AlertDialogDescription>
                  Unlink <span className='font-medium'>{confirm.row.title}</span>{' '}
                  from this product? Tasks owned by this manifest stay
                  with the manifest but won't show up under this
                  product anymore. Action is logged in revision
                  history; you can restore.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={onConfirm}>
                  Unlink
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          ) : confirm?.kind === 'restore' ? (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Restore to this revision?</AlertDialogTitle>
                <AlertDialogDescription>
                  Apply the snapshot from{' '}
                  <span className='font-medium'>{confirm.revisionLabel}</span>{' '}
                  as current state. Items removed since then get
                  re-added; items added since get removed. The restore
                  itself is logged as a new revision so you can move
                  forward or back again.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={onConfirm}>
                  Restore
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          ) : null}
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function DepSection({
  title,
  subtitle,
  editing,
  rows,
  loading,
  error,
  onAdd,
  onRemove,
  rowHref,
}: {
  title: string
  subtitle: string
  editing: boolean
  rows: PickerRow[]
  loading: boolean
  error: unknown
  onAdd: () => void
  onRemove: (row: PickerRow) => void
  rowHref?: '/products'
}) {
  return (
    <Card>
      <CardHeader className='py-3'>
        <CardTitle className='flex items-center justify-between text-sm font-medium'>
          <span>
            {title}
            <Badge variant='outline' className='ml-2 text-[10px]'>
              {rows.length}
            </Badge>
          </span>
          {editing ? (
            <Button
              variant='outline'
              size='sm'
              className='h-7 px-2 text-xs'
              onClick={onAdd}
            >
              <Plus className='mr-1 h-3 w-3' />
              Add
            </Button>
          ) : null}
        </CardTitle>
        <p className='text-muted-foreground pt-1 text-xs'>{subtitle}</p>
      </CardHeader>
      <CardContent className='pt-0 pb-3'>
        {loading ? (
          <Skeleton className='h-16 w-full' />
        ) : error ? (
          <div className='text-sm text-rose-400'>Failed: {String(error)}</div>
        ) : rows.length === 0 ? (
          <div className='text-muted-foreground text-sm'>None linked.</div>
        ) : (
          <div className='space-y-1 text-sm'>
            {rows.map((r) => (
              <div
                key={r.id}
                className='hover:bg-accent flex items-center justify-between gap-2 rounded-md px-2 py-1.5'
              >
                {rowHref ? (
                  <Link
                    to={rowHref}
                    search={{ id: r.id, tab: 'main' }}
                    className='min-w-0 flex-1'
                  >
                    <div className='truncate font-medium'>{r.title}</div>
                    <code className='text-muted-foreground font-mono text-[11px] block truncate'>
                      {r.id}
                    </code>
                  </Link>
                ) : (
                  <div className='min-w-0 flex-1'>
                    <div className='truncate font-medium'>{r.title}</div>
                    <code className='text-muted-foreground font-mono text-[11px] block truncate'>
                      {r.id}
                    </code>
                  </div>
                )}
                <Badge
                  variant='secondary'
                  className={`shrink-0 text-[10px] uppercase ${STATUS_COLOR[r.status] ?? 'bg-zinc-500/15'}`}
                >
                  {r.status}
                </Badge>
                {editing ? (
                  <Button
                    variant='ghost'
                    size='sm'
                    className='h-6 w-6 shrink-0 p-0'
                    onClick={() => onRemove(r)}
                  >
                    <X className='h-3 w-3' />
                  </Button>
                ) : null}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

interface ParsedRevision {
  op?: string
  kind?: string
  target?: { title?: string }
  snapshot?: { downstream?: string[]; manifests?: string[] }
}

function RevisionRow({
  rev,
  onRestore,
}: {
  rev: Comment
  onRestore: (
    snap: { downstream: string[]; manifests: string[] },
    label: string
  ) => void
}) {
  const body = rev.body ?? ''
  const match = body.match(
    /<dependency_revision>\s*([\s\S]*?)\s*<\/dependency_revision>/
  )
  let parsed: ParsedRevision = {}
  if (match) {
    try {
      parsed = JSON.parse(match[1])
    } catch {
      // keep summary fallback
    }
  }
  const op = parsed.op
  const kind = parsed.kind
  const targetTitle = parsed.target?.title ?? '?'
  const kindLabel =
    kind === 'product_downstream'
      ? 'sub-product'
      : kind === 'product_upstream'
        ? 'upstream dep'
        : kind === 'manifest'
          ? 'manifest'
          : kind === 'snapshot'
            ? 'snapshot'
            : 'item'
  const summary =
    op === 'restore'
      ? `Restored to ${targetTitle}`
      : op && kind
        ? `${op === 'add' ? 'Added' : 'Removed'} ${kindLabel} "${targetTitle}"`
        : 'Dependency change'

  const ts = fmtTime(rev.created_at)

  const canRestore =
    !!parsed.snapshot &&
    Array.isArray(parsed.snapshot.downstream) &&
    Array.isArray(parsed.snapshot.manifests)

  return (
    <div className='hover:bg-accent flex items-center justify-between gap-2 rounded-md px-2 py-2 text-sm'>
      <div className='min-w-0 flex-1'>
        <div className='flex items-center gap-2'>
          <code className='font-mono text-[11px]'>
            {rev.author.slice(0, 16)}
          </code>
          <span className='text-muted-foreground text-xs'>{ts}</span>
        </div>
        <div className='text-foreground text-xs'>{summary}</div>
      </div>
      {canRestore ? (
        <Button
          variant='outline'
          size='sm'
          className='h-7 shrink-0 px-2 text-xs'
          onClick={() =>
            onRestore(
              {
                downstream: parsed.snapshot!.downstream ?? [],
                manifests: parsed.snapshot!.manifests ?? [],
              },
              ts
            )
          }
        >
          <History className='mr-1 h-3 w-3' />
          Restore
        </Button>
      ) : null}
    </div>
  )
}

function fmtTime(ts: number | string): string {
  const t = typeof ts === 'number' ? ts * 1000 : Date.parse(ts)
  if (!Number.isFinite(t)) return '—'
  return new Date(t).toLocaleString()
}
