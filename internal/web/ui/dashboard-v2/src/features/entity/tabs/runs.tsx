import { useState } from 'react'
import { useEntityRuns, type EntityKind } from '@/lib/queries/entity'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'

interface RunsTabProps {
  kind: EntityKind
  entityId: string
}

// RunsTab shows the execution history for an entity.
// Turn-level charts (timeline, tools-per-turn, heatmap, cost/turn)
// are wired in by the Turn-Level Analytics product tasks (T3/T4/T5).
export function RunsTab({ kind, entityId }: RunsTabProps) {
  const runs = useEntityRuns(kind, entityId)
  const [selectedRunUid, setSelectedRunUid] = useState<string | null>(null)

  // Group rows by run_uid — keep only the terminal event per run
  const runList = (() => {
    if (!runs.data) return []
    const seen = new Map<string, typeof runs.data[0]>()
    for (const row of runs.data) {
      if (row.event === 'completed' || row.event === 'failed') {
        if (!seen.has(row.run_uid)) seen.set(row.run_uid, row)
      }
    }
    // Also capture started-only runs (in progress)
    for (const row of runs.data) {
      if (row.event === 'started' && !seen.has(row.run_uid)) {
        seen.set(row.run_uid, row)
      }
    }
    return [...seen.values()].sort((a, b) =>
      new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
    )
  })()

  if (runs.isLoading) return <div className='space-y-2 p-2'><Skeleton className='h-10 w-full' /><Skeleton className='h-10 w-full' /></div>
  if (runs.isError) return <div className='p-4 text-sm text-rose-400'>Failed to load runs</div>
  if (runList.length === 0) return <div className='text-muted-foreground p-4 text-sm'>No runs yet.</div>

  return (
    <div className='flex gap-3'>
      {/* Run list */}
      <div className='w-56 shrink-0 space-y-1'>
        {runList.map((run) => (
          <button
            key={run.run_uid}
            onClick={() => setSelectedRunUid(run.run_uid)}
            className={`w-full rounded border px-2 py-1.5 text-left text-xs transition-colors ${
              selectedRunUid === run.run_uid
                ? 'border-emerald-500/50 bg-emerald-500/10'
                : 'border-transparent hover:bg-white/5'
            }`}
          >
            <div className='flex items-center justify-between gap-1'>
              <span className='font-mono text-[10px] opacity-60'>{run.run_uid.slice(0, 8)}</span>
              <Badge
                variant='outline'
                className={`text-[9px] uppercase ${
                  run.event === 'completed' ? 'border-emerald-500/50 text-emerald-400' :
                  run.event === 'failed' ? 'border-rose-500/50 text-rose-400' :
                  'border-amber-500/50 text-amber-400'
                }`}
              >
                {run.event}
              </Badge>
            </div>
            <div className='mt-0.5 text-[10px] opacity-50'>
              {new Date(typeof run.created_at === 'number' ? run.created_at * 1000 : run.created_at).toLocaleString()}
            </div>
            {run.turns > 0 && <div className='mt-0.5 text-[10px] opacity-40'>{run.turns} turns · {run.actions} actions</div>}
          </button>
        ))}
      </div>

      {/* Run detail — charts added by T3/T4/T5 */}
      <div className='min-w-0 flex-1'>
        {selectedRunUid ? (
          <Card className='gap-0 py-0'>
            <CardContent className='p-3'>
              <div className='text-muted-foreground mb-2 font-mono text-xs'>{selectedRunUid}</div>
              {/* Turn Timeline chart — added by T3 */}
              {/* Tools per Turn chart — added by T4 */}
              {/* Tool Density Heatmap — added by T4 */}
              {/* Cost per Turn — added by T5 */}
              <div className='text-muted-foreground text-sm'>
                Turn analytics charts load here once turn data exists for this run.
              </div>
            </CardContent>
          </Card>
        ) : (
          <div className='text-muted-foreground flex h-32 items-center justify-center text-sm'>
            Select a run to see turn analytics
          </div>
        )}
      </div>
    </div>
  )
}
