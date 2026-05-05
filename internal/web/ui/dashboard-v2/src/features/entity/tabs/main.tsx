import { useEffect, useState } from 'react'
import {
  useEntity,
  type EntityKind,
} from '@/lib/queries/entity'
import type { Entity } from '@/lib/types'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Gauge } from '@/components/gauge'
import { ContentBlock } from '@/components/content-block'

// Label shown in ContentBlock per entity type
const CONTENT_LABEL: Record<string, string> = {
  product:  'Description',
  manifest: 'Declaration',
  task:     'Instructions',
  skill:    'Description',
  idea:     'Description',
}

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

  const e = entity.data as Entity | undefined
  const created = e?.created_at ? new Date(e.created_at) : null
  const updatedDate = e?.valid_from ? new Date(e.valid_from) : null
  // history is now shown inside ContentBlock — no separate query needed

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

      {/* ContentBlock: Description / Declaration / Instructions */}
      <ContentBlock
        entityId={entityId}
        kind={kind}
        label={CONTENT_LABEL[kind] ?? 'Description'}
        placeholder={`Write ${(CONTENT_LABEL[kind] ?? 'description').toLowerCase()} here… Markdown supported, drag/paste files to attach`}
      />

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
