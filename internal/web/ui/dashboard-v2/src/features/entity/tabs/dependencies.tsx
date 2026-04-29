import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { History, Pencil, Plus, X } from 'lucide-react'
import { toast } from 'sonner'
import {
  useAddDownstreamDep,
  useAddUpstreamDep,
  useAllManifests,
  useAllProducts,
  useEntity,
  useEntityChildren,
  useEntityComments,
  useEntityDependencies,
  useLinkManifest,
  useRemoveDownstreamDep,
  useRemoveUpstreamDep,
  useRestoreEntityDependencySnapshot,
  useUnlinkManifest,
  type EntityKind,
} from '@/lib/queries/entity'
import type { Comment, Manifest } from '@/lib/types'
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
  | { kind: 'remove-downstream'; row: PickerRow }
  | { kind: 'remove-manifest'; row: PickerRow }
  | { kind: 'remove-upstream'; row: PickerRow }
  | {
      kind: 'restore'
      revisionLabel: string
      snapshot: { downstream: string[]; manifests: string[] }
    }

// Dependencies tab — generic over products + manifests.
//
// Products surface two sections: Sub-products (downstream products
// depending on this) + Manifests (owned children).
//
// Manifests surface one section: Depends on (upstream manifests this
// blocks-on). Manifests don't own manifests, so the second section is
// dropped. Children (tasks) are out of scope here — they get their own
// menu later.
export function DependenciesTab({
  kind,
  entityId,
}: {
  kind: EntityKind
  entityId: string
}) {
  const isProduct = kind === 'product'
  const deps = useEntityDependencies(kind, entityId)
  const children = useEntityChildren(kind, entityId)
  const allProducts = useAllProducts()
  const allManifests = useAllManifests()
  const comments = useEntityComments(kind, entityId)

  const [editing, setEditing] = useState(false)
  const [picker, setPicker] = useState<
    null | 'subproduct' | 'manifest-link' | 'manifest-upstream'
  >(null)
  const [confirm, setConfirm] = useState<ConfirmTarget | null>(null)

  // Mutation hooks. Products use downstream (sub-products) + manifest-
  // re-parent. Manifests use upstream (depends-on) only.
  const addDownstream = useAddDownstreamDep(kind, entityId)
  const remDownstream = useRemoveDownstreamDep(kind, entityId)
  const addUpstream = useAddUpstreamDep(kind, entityId)
  const remUpstream = useRemoveUpstreamDep(kind, entityId)
  const linkM = useLinkManifest(entityId)
  const unlinkM = useUnlinkManifest(entityId)
  const restore = useRestoreEntityDependencySnapshot(kind, entityId)

  // Products: deps = sub-products (downstream).
  // Manifests: deps = upstream (depends-on).
  const depRows: PickerRow[] = (deps.data ?? []).map((d) => ({
    id: d.id,
    marker: d.marker,
    title: d.title,
    status: d.status,
  }))
  const childRows: PickerRow[] = (children.data ?? []).map(
    (m: { id: string; marker?: string; title?: string; status?: string }) => ({
      id: m.id,
      marker: m.marker ?? m.id.slice(0, 12),
      title: m.title ?? m.id.slice(0, 12),
      status: m.status ?? '',
    })
  )

  const snapshot = useMemo(
    () => ({
      upstream: isProduct ? [] : depRows.map((r) => r.id),
      downstream: isProduct ? depRows.map((r) => r.id) : [],
      manifests: isProduct ? childRows.map((r) => r.id) : [],
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [deps.data, children.data, isProduct]
  )

  const pickerCandidates = useMemo<PickerRow[]>(() => {
    if (picker === 'manifest-link') {
      return (allManifests.data ?? [])
        .filter((m) => !childRows.some((r) => r.id === m.id))
        .map((m) => ({
          id: m.id,
          marker: m.marker,
          title: m.title,
          status: m.status,
        }))
    }
    if (picker === 'subproduct') {
      const exclude = new Set([entityId, ...depRows.map((r) => r.id)])
      return (allProducts.data ?? [])
        .filter((p) => !exclude.has(p.id))
        .map((p) => ({
          id: p.id,
          marker: p.marker,
          title: p.title,
          status: p.status,
        }))
    }
    if (picker === 'manifest-upstream') {
      const exclude = new Set([entityId, ...depRows.map((r) => r.id)])
      return (allManifests.data ?? [])
        .filter((m) => !exclude.has(m.id))
        .map((m) => ({
          id: m.id,
          marker: m.marker,
          title: m.title,
          status: m.status,
        }))
    }
    return []
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [picker, allProducts.data, allManifests.data, deps.data, children.data])

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
      if (confirm.kind === 'remove-downstream') {
        await remDownstream.mutateAsync({ target: confirm.row, snapshot })
        toast.success(`Removed sub-product "${confirm.row.title}"`)
      } else if (confirm.kind === 'remove-manifest') {
        await unlinkM.mutateAsync({ target: confirm.row, snapshot })
        toast.success(`Unlinked manifest "${confirm.row.title}"`)
      } else if (confirm.kind === 'remove-upstream') {
        await remUpstream.mutateAsync({ target: confirm.row, snapshot })
        toast.success(`Removed upstream "${confirm.row.title}"`)
      } else if (confirm.kind === 'restore') {
        const result = await restore.mutateAsync({
          snapshot: confirm.snapshot,
          revisionLabel: confirm.revisionLabel,
          currentDownstream: depRows,
          currentManifests: childRows,
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
          {isProduct ? (
            <>
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
            </>
          ) : (
            <>
              <p>
                <span className='font-medium'>
                  Manifests can depend on other manifests.
                </span>{' '}
                A manifest stays in <em>waiting</em> until everything in
                "Depends on" reaches a terminal status (closed / archive).
                Tasks owned by this manifest live in the Tasks menu.
              </p>
              <p className='text-muted-foreground font-mono text-xs'>
                Manifest ── depends on ──▶ Upstream manifests &nbsp; │
                &nbsp; Manifest ── owns ──▶ Tasks
              </p>
            </>
          )}
        </CardContent>
      </Card>

      <div
        className={
          isProduct
            ? 'grid grid-cols-1 gap-3 md:grid-cols-2'
            : 'grid grid-cols-1 gap-3'
        }
      >
        {isProduct ? (
          <>
            <DepSection
              title='Sub-products'
              subtitle='Products nested under this one. Each is itself a product and can own its own sub-products + manifests. Click a row to drill in.'
              editing={editing}
              rows={depRows}
              loading={deps.isLoading}
              error={deps.error}
              onAdd={() => setPicker('subproduct')}
              onRemove={(row) =>
                setConfirm({ kind: 'remove-downstream', row })
              }
              rowHref='/products'
            />
            <DepSection
              title='Manifests'
              subtitle='Plans owned directly by this product (or sub-product) at this level of the tree. Each manifest contains tasks (separate menu).'
              editing={editing}
              rows={childRows}
              loading={children.isLoading}
              error={children.error}
              onAdd={() => setPicker('manifest-link')}
              onRemove={(row) =>
                setConfirm({ kind: 'remove-manifest', row })
              }
              rowHref='/manifests'
            />
          </>
        ) : (
          <DepSection
            title='Depends on'
            subtitle='Upstream manifests this one waits on. Each must reach a terminal status before this manifest can flip to scheduled.'
            editing={editing}
            rows={depRows}
            loading={deps.isLoading}
            error={deps.error}
            onAdd={() => setPicker('manifest-upstream')}
            onRemove={(row) => setConfirm({ kind: 'remove-upstream', row })}
            rowHref='/manifests'
          />
        )}
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
          picker === 'subproduct'
            ? 'Add sub-product'
            : picker === 'manifest-link'
              ? 'Link a manifest'
              : 'Add upstream manifest'
        }
        description={
          picker === 'subproduct'
            ? 'Pick a product to nest under this one.'
            : picker === 'manifest-link'
              ? 'Pick a manifest to re-parent under this product.'
              : 'Pick a manifest this one will depend on. It must finish first.'
        }
        rows={pickerCandidates}
        loading={
          picker === 'subproduct'
            ? allProducts.isLoading
            : allManifests.isLoading
        }
        onPick={async (row) => {
          try {
            if (picker === 'subproduct') {
              await addDownstream.mutateAsync({ target: row, snapshot })
              toast.success(`Added sub-product "${row.title}"`)
            } else if (picker === 'manifest-link') {
              await linkM.mutateAsync({ target: row, snapshot })
              toast.success(`Linked manifest "${row.title}"`)
            } else if (picker === 'manifest-upstream') {
              await addUpstream.mutateAsync({ target: row, snapshot })
              toast.success(`Added upstream "${row.title}"`)
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
          {confirm?.kind === 'remove-downstream' ? (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Remove sub-product?</AlertDialogTitle>
                <AlertDialogDescription>
                  Remove <span className='font-medium'>{confirm.row.title}</span>
                  {' '}from this product? It stays as a standalone product —
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
                  Unlink <span className='font-medium'>{confirm.row.title}</span>
                  {' '}from this product? Tasks owned by this manifest stay
                  with the manifest but won't show up under this
                  product anymore. Action is logged in revision history.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={onConfirm}>
                  Unlink
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          ) : confirm?.kind === 'remove-upstream' ? (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Remove upstream dep?</AlertDialogTitle>
                <AlertDialogDescription>
                  Sever the depends-on edge from this manifest to{' '}
                  <span className='font-medium'>{confirm.row.title}</span>.
                  This manifest's waiting tasks may unblock as a result.
                  Action is logged in revision history.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={onConfirm}>
                  Remove
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
  rowHref?: '/products' | '/manifests'
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
          : kind === 'manifest_upstream'
            ? 'upstream manifest'
            : kind === 'manifest_downstream'
              ? 'downstream manifest'
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

// Suppress unused-import warning — `Manifest` is referenced indirectly
// via the queries layer's typing; keep the import so future tweaks
// don't have to re-add it.
export type { Manifest }

function fmtTime(ts: number | string): string {
  const t = typeof ts === 'number' ? ts * 1000 : Date.parse(ts)
  if (!Number.isFinite(t)) return '—'
  return new Date(t).toLocaleString()
}
