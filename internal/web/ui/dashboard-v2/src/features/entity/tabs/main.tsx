import { useEffect, useRef, useState } from 'react'
import { Pencil, Plus } from 'lucide-react'
import {
  useEntity,
  useEntityDescriptionHistory,
  useUpdateEntity,
  type EntityKind,
} from '@/lib/queries/entity'
import type { Entity } from '@/lib/types'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DescriptionView } from '@/components/description-view'
import { Gauge } from '@/components/gauge'
import {
  BlockNoteComposer,
  type BlockNoteComposerHandle,
} from '@/components/blocknote-composer'

// Main tab — stats grid + repo card + description editor + revision
// history. Same Markup ↔ Rendered toggle on description view; Cmd-Enter
// saves; Escape cancels. PUT /api/entities/:id drops a new SCD-2
// description revision row server-side, surfaced in the history card
// below.
//
// Stats: 5 compact gauges in operator-priority order — Estimated Cost,
// Actual, Turns, Actions, Tokens. Same byte-identical Gauge layout
// across products and manifests; the cumulative numbers come straight
// off the entity (server-side aggregates).

// Legacy settings path — still needed until settings are migrated to /api/entities.
function settingsPathFor(kind: EntityKind, entityId: string): string {
  const slug = kind === 'product' ? 'products' : kind === 'task' ? 'tasks' : 'manifests'
  return `/api/${slug}/${entityId}/settings`
}

export function MainTab({
  kind,
  entityId,
}: {
  kind: EntityKind
  entityId: string
}) {
  const entity = useEntity(kind, entityId)
  const history = useEntityDescriptionHistory(kind, entityId)
  const update = useUpdateEntity(kind, entityId)
  const [repoInfo, setRepoInfo] = useState<Record<string, string | number>>(
    {}
  )

  // Load entity-scoped settings (resolved/explicit) so the repo card
  // can show worktree base + branch prefix + agent. Same shape on
  // /api/products/{id}/settings and /api/manifests/{id}/settings.
  useEffect(() => {
    let cancelled = false
    fetch(settingsPathFor(kind, entityId))
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
  }, [kind, entityId])

  const [editing, setEditing] = useState(false)
  const [initialDraft, setInitialDraft] = useState('')
  const [saving, setSaving] = useState(false)
  const composerRef = useRef<BlockNoteComposerHandle>(null)

  const e = entity.data as Entity | undefined

  // Description lives in comments (description_revision type). The Main tab
  // shows the latest description_revision body from the history endpoint;
  // currentText is derived from the most recent revision if any.
  const latestRevision = history.data?.[0]
  const currentText = (latestRevision?.body ?? '').trim()
  const hasDesc = currentText.length > 0

  const startEdit = () => {
    setInitialDraft(currentText)
    setEditing(true)
  }
  const cancel = () => {
    setEditing(false)
    composerRef.current?.clear()
  }
  const save = async () => {
    if (!composerRef.current) return
    setSaving(true)
    try {
      const draft = await composerRef.current.getMarkdown()
      // Description is stored as a description_revision comment; the PUT
      // endpoint drops a new SCD-2 revision row server-side. We send the
      // body as a top-level `description` field which the backend records
      // as a new revision.
      await update.mutateAsync({ description: draft })
      setEditing(false)
    } catch (e) {
      console.error(e)
    } finally {
      setSaving(false)
    }
  }

  const created = e?.created_at ? new Date(e.created_at) : null
  const updatedDate = e?.valid_from ? new Date(e.valid_from) : null

  // Description body comes from the latest description_revision comment.
  const description = latestRevision?.body

  return (
    <div className='space-y-2'>
      {e ? (
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-3'>
          <div className='flex flex-col items-stretch text-emerald-500'>
            <Gauge
              label='turns'
              value={e.total_turns ?? 0}
              min={0}
              max={Math.max(50, (e.total_turns ?? 0) * 1.5)}
            />
          </div>
          <div className='flex flex-col items-stretch text-emerald-500'>
            <Gauge
              label='actions'
              value={e.total_actions ?? 0}
              min={0}
              max={Math.max(100, (e.total_actions ?? 0) * 1.5)}
            />
          </div>
          <div className='flex flex-col items-stretch text-emerald-500'>
            <Gauge
              label='tokens'
              value={e.total_tokens ?? 0}
              min={0}
              max={Math.max(10000, (e.total_tokens ?? 0) * 1.5)}
            />
          </div>
        </div>
      ) : (
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5'>
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className='h-24 w-full' />
          ))}
        </div>
      )}

      {e ? (
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
                  {e.status}
                </Badge>
              }
            />
            {created ? (
              <Row label='Created' value={created.toLocaleString()} />
            ) : null}
            {updatedDate ? (
              <Row label='Updated' value={updatedDate.toLocaleString()} />
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      <Card className='gap-0 py-0'>
        <CardContent className='space-y-2 px-3 py-2'>
          {!editing && !entity.isLoading ? (
            <div className='flex justify-end'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='h-7 px-2 text-xs'
                onClick={startEdit}
              >
                {hasDesc ? (
                  <>
                    <Pencil className='mr-1 h-3 w-3' />
                    Edit
                  </>
                ) : (
                  <>
                    <Plus className='mr-1 h-3 w-3' />
                    Add Description
                  </>
                )}
              </Button>
            </div>
          ) : null}
          {entity.isLoading ? (
            <Skeleton className='h-24 w-full' />
          ) : editing ? (
            <div className='space-y-2'>
              <BlockNoteComposer
                ref={composerRef}
                initialMarkdown={initialDraft}
                onSave={save}
                onCancel={cancel}
                placeholder={
                  kind === 'product'
                    ? 'Product description… drop files, paste images, type / for blocks'
                    : 'Manifest spec… drop files, paste images, type / for blocks'
                }
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
                  disabled={saving}
                >
                  Cancel
                </Button>
                <Button
                  type='button'
                  size='sm'
                  onClick={save}
                  disabled={saving}
                >
                  {saving ? 'Saving…' : 'Save'}
                </Button>
              </div>
            </div>
          ) : (
            <DescriptionView raw={description} />
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
                      {(rev.author ?? '').slice(0, 16)}
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

function fmtTime(ts: number | string): string {
  const t = typeof ts === 'number' ? ts * 1000 : Date.parse(ts)
  if (!Number.isFinite(t)) return '—'
  return new Date(t).toLocaleString()
}
