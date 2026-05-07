import { useState, Fragment } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useEntityRuns, type EntityKind } from '@/lib/queries/entity'
import type { ExecutionRow } from '@/lib/types'
import { TurnAnalyticsBlock } from '@/features/entity/turn-charts'
import { Skeleton } from '@/components/ui/skeleton'

interface LiveRun {
  run_uid: string; entity_uid: string; entity_title: string
  elapsed_sec: number; turns: number; actions: number
  cost_usd: number; model: string
}
function useLiveRuns() {
  return useQuery({
    queryKey: ['execution-live'],
    queryFn: (): Promise<LiveRun[]> => fetch('/api/execution/live').then(r => r.json()),
    refetchInterval: (q) => ((q.state.data?.length ?? 0) > 0 ? 4000 : 10000),
    refetchIntervalInBackground: false,
  })
}

interface ActionRow {
  id: string; task_id: string; tool_name: string
  tool_input: string; tool_response: string; turn_number: number; created_at: string
}

// Live output: poll entity actions while run is active (every 3s), static otherwise.
// run_uid scopes actions to the current run only — no historical bleed.
function useEntityActions(entityId: string, runUid: string, isLive: boolean) {
  return useQuery({
    queryKey: ['entity-actions', entityId, runUid, isLive],
    queryFn: (): Promise<ActionRow[]> =>
      fetch(`/api/entities/${entityId}/actions?limit=200&run_uid=${runUid}`).then(r => r.json()),
    enabled: !!entityId,
    refetchInterval: isLive ? 3000 : false,
    staleTime: isLive ? 0 : 60_000,
  })
}

function LiveOutput({ entityId, runUid }: { entityId: string; runUid: string }) {
  const { data, isLoading } = useEntityActions(entityId, runUid, true)
  if (isLoading) return <div className='text-muted-foreground p-3 text-xs'>Loading output…</div>
  if (!data?.length) return <div className='text-muted-foreground p-3 text-xs'>Waiting for agent to make tool calls…</div>
  return (
    <div className='max-h-96 overflow-y-auto font-mono text-xs'>
      {[...data].reverse().map((a) => (
        <div key={a.id} className='border-b border-white/5 px-3 py-1.5'>
          <div className='flex items-center gap-2'>
            <span className='text-blue-400 font-medium'>{a.tool_name}</span>
            {a.turn_number > 0 && <span className='text-muted-foreground text-[10px]'>turn {a.turn_number}</span>}
            <span className='text-muted-foreground ml-auto text-[10px]'>
              {new Date(Date.parse(a.created_at)).toLocaleTimeString()}
            </span>
          </div>
          {a.tool_input && (
            <div className='text-muted-foreground mt-0.5 truncate text-[10px]'>
              {a.tool_input.slice(0, 120)}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

interface TaskRunGroup { task_id: string; task_title: string; runs: ExecutionRow[] }
interface ManifestGroup { manifest_id: string; manifest_title: string; tasks: TaskRunGroup[] }

function fmtCost(usd: number) {
  if (!usd) return '—'
  return usd < 0.01 ? `$${usd.toFixed(4)}` : `$${usd.toFixed(3)}`
}
function fmtTime(v: string | number) {
  const ms = typeof v === 'number' ? v * 1000 : Date.parse(v)
  return isFinite(ms) ? new Date(ms).toLocaleString() : '—'
}

const thCls = 'text-muted-foreground px-3 py-2 text-left text-[11px] font-medium uppercase tracking-wide'
const tdCls = 'px-3 py-2 text-xs'

function RunRow({ run, entityId, selectedRunUid, onSelect, onSelectHistory }: {
  run: ExecutionRow; entityId: string; selectedRunUid: string | null
  onSelect: (uid: string | null) => void; onSelectHistory?: (uid: string) => void
}) {
  const expanded = selectedRunUid === run.run_uid
  return (
    <Fragment key={run.run_uid}>
      <tr
        className={`cursor-pointer border-b transition-colors hover:bg-white/5 ${expanded ? 'bg-white/5' : ''}`}
        onClick={() => { onSelect(expanded ? null : run.run_uid); onSelectHistory?.(run.run_uid) }}
      >
        <td className={tdCls}><span className={
          run.event === 'completed' ? 'text-emerald-400' :
          run.event === 'failed' ? 'text-rose-400' : 'text-amber-400'
        }>{run.event}</span></td>
        <td className={`${tdCls} font-mono text-[10px] opacity-50`}>{run.run_uid.slice(0,12)}</td>
        <td className={tdCls}>{run.turns || '—'}</td>
        <td className={tdCls}>{run.actions || '—'}</td>
        <td className={tdCls}>{fmtCost(run.cost_usd)}</td>
        <td className={`${tdCls} text-[10px] opacity-50`}>{run.model || '—'}</td>
        <td className={`${tdCls} text-[10px] opacity-50`}>{fmtTime(run.created_at)}</td>
      </tr>
      {expanded && (
        <tr><td colSpan={7} className='border-b bg-white/3 px-4 py-3'>
          <TurnAnalyticsBlock entityId={entityId} runUid={run.run_uid} />
        </td></tr>
      )}
    </Fragment>
  )
}

function RunTable({ runs, entityId, selectedRunUid, onSelect, onSelectHistory }: {
  runs: ExecutionRow[]; entityId: string; selectedRunUid: string | null
  onSelect: (uid: string | null) => void; onSelectHistory?: (uid: string) => void
}) {
  // One row per run_uid — prefer terminal event (completed/failed) over started.
  const history = (() => {
    const byRun = new Map<string, ExecutionRow>()
    for (const r of runs) {
      if (!['completed','failed','started'].includes(r.event)) continue
      const existing = byRun.get(r.run_uid)
      if (!existing || (r.event !== 'started' && existing.event === 'started')) {
        byRun.set(r.run_uid, r)
      }
    }
    return [...byRun.values()].sort((a,b) =>
      new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
    )
  })()
  if (history.length === 0) return <div className='text-muted-foreground px-3 py-2 text-xs'>No runs.</div>
  return (
    <table className='w-full'>
      <thead><tr className='border-b'>
        <th className={thCls}>Status</th><th className={thCls}>Run</th>
        <th className={thCls}>Turns</th><th className={thCls}>Actions</th>
        <th className={thCls}>Cost</th><th className={thCls}>Model</th>
        <th className={thCls}>Time</th>
      </tr></thead>
      <tbody>
        {history.map(run => (
          <RunRow key={run.run_uid} run={run} entityId={entityId}
            selectedRunUid={selectedRunUid} onSelect={onSelect} onSelectHistory={onSelectHistory} />
        ))}
      </tbody>
    </table>
  )
}

interface RunsTabProps {
  kind: EntityKind; entityId: string
  onSelectLive?: (uid: string) => void
  onSelectHistory?: (uid: string) => void
}

export function RunsTab({ kind, entityId, onSelectLive, onSelectHistory }: RunsTabProps) {
  const runs = useEntityRuns(kind, entityId)
  const live = useLiveRuns()
  const [selectedRunUid, setSelectedRunUid] = useState<string | null>(null)

  const liveRun = live.data?.find(r => r.entity_uid === entityId && r.entity_uid !== 'stdio')

  if (runs.isLoading) return <div className='space-y-1 p-2'><Skeleton className='h-8 w-full' /><Skeleton className='h-8 w-full' /></div>
  if (runs.isError) return <div className='p-4 text-sm text-rose-400'>Failed to load runs</div>

  const data = runs.data as any

  return (
    <div className='space-y-4 overflow-x-auto'>
      {/* Live run */}
      {liveRun && (
        <div className='rounded border border-emerald-500/30 bg-emerald-500/5'>
          {/* Header */}
          <div className='flex items-center gap-3 px-3 py-2'>
            <span className='relative flex h-2.5 w-2.5'>
              <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75' />
              <span className='relative inline-flex h-2.5 w-2.5 rounded-full bg-emerald-500' />
            </span>
            <span className='text-sm font-medium text-emerald-400'>Running</span>
            <span className='text-muted-foreground text-xs'>{liveRun.elapsed_sec}s</span>
            <span className='font-bold text-emerald-400'>{liveRun.turns} turns</span>
            <span className='text-blue-400'>{liveRun.actions} actions</span>
            {liveRun.model && <span className='text-muted-foreground text-[10px]'>{liveRun.model}</span>}
            <span className='text-muted-foreground ml-auto font-mono text-[10px]'>{liveRun.run_uid.slice(0,12)}</span>
          </div>
          {/* Live output — polls actions every 3s */}
          <div className='border-t border-emerald-500/20'>
            <LiveOutput entityId={liveRun.entity_uid} runUid={liveRun.run_uid} />
          </div>
        </div>
      )}

      {/* Task / skill / idea — flat atomic run list */}
      {(kind === 'task' || kind === 'skill' || kind === 'idea') && Array.isArray(data) && (
        <RunTable runs={data as ExecutionRow[]} entityId={entityId}
          selectedRunUid={selectedRunUid} onSelect={setSelectedRunUid} onSelectHistory={onSelectHistory} />
      )}

      {/* Manifest — grouped by task */}
      {kind === 'manifest' && Array.isArray(data) && (data as TaskRunGroup[]).map(tg => (
        <div key={tg.task_id}>
          <div className='text-muted-foreground mb-1 px-1 text-[11px] font-medium uppercase tracking-wide'>{tg.task_title}</div>
          <RunTable runs={tg.runs} entityId={tg.task_id}
            selectedRunUid={selectedRunUid} onSelect={setSelectedRunUid} onSelectHistory={onSelectHistory} />
        </div>
      ))}

      {/* Product — grouped by manifest → task */}
      {kind === 'product' && Array.isArray(data) && (data as ManifestGroup[]).map(mg => (
        <div key={mg.manifest_id} className='rounded border border-white/10'>
          <div className='border-b border-white/10 px-3 py-2 text-xs font-semibold'>{mg.manifest_title}</div>
          <div className='space-y-3 p-2'>
            {mg.tasks.map(tg => (
              <div key={tg.task_id}>
                <div className='text-muted-foreground mb-1 px-1 text-[11px] uppercase tracking-wide'>{tg.task_title}</div>
                <RunTable runs={tg.runs} entityId={tg.task_id}
                  selectedRunUid={selectedRunUid} onSelect={setSelectedRunUid} onSelectHistory={onSelectHistory} />
              </div>
            ))}
          </div>
        </div>
      ))}

      {!liveRun && (!data || (Array.isArray(data) && data.length === 0)) && (
        <div className='text-muted-foreground p-6 text-center text-sm'>No runs yet.</div>
      )}
    </div>
  )
}
