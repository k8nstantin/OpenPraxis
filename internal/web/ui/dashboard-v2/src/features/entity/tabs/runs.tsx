import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useEntityRuns, type EntityKind } from '@/lib/queries/entity'
import { Skeleton } from '@/components/ui/skeleton'

interface LiveRun {
  run_uid: string
  entity_uid: string
  entity_title: string
  elapsed_sec: number
  turns: number
  actions: number
  cost_usd: number
  model: string
}

function useLiveRuns() {
  return useQuery({
    queryKey: ['execution-live'],
    queryFn: (): Promise<LiveRun[]> => fetch('/api/execution/live').then(r => r.json()),
    refetchInterval: 4000,
  })
}

function fmtCost(usd: number) {
  if (!usd) return '—'
  return usd < 0.01 ? `$${usd.toFixed(4)}` : `$${usd.toFixed(3)}`
}

function fmtTime(v: string | number) {
  const ms = typeof v === 'number' ? v * 1000 : Date.parse(v)
  if (!isFinite(ms)) return '—'
  return new Date(ms).toLocaleString()
}

interface RunsTabProps {
  kind: EntityKind
  entityId: string
  onSelectLive?: (runUid: string) => void
  onSelectHistory?: (runUid: string) => void
}

export function RunsTab({ kind, entityId, onSelectLive, onSelectHistory }: RunsTabProps) {
  const runs = useEntityRuns(kind, entityId)
  const live = useLiveRuns()
  const [selectedRunUid, setSelectedRunUid] = useState<string | null>(null)

  const liveRun = live.data?.find(r => r.entity_uid === entityId && r.entity_uid !== 'stdio')

  // Deduplicate historical runs — one row per run_uid, prefer terminal event
  const history = (() => {
    if (!runs.data) return []
    const map = new Map<string, typeof runs.data[0]>()
    for (const row of runs.data) {
      if (!['completed', 'failed', 'started'].includes(row.event)) continue
      const existing = map.get(row.run_uid)
      if (!existing || (row.event !== 'started' && existing.event === 'started')) {
        map.set(row.run_uid, row)
      }
    }
    return [...map.values()]
      .filter(r => !liveRun || r.run_uid !== liveRun.run_uid)
      .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
  })()

  if (runs.isLoading) return (
    <div className='space-y-1 p-2'>
      <Skeleton className='h-8 w-full' />
      <Skeleton className='h-8 w-full' />
      <Skeleton className='h-8 w-full' />
    </div>
  )

  const thCls = 'text-muted-foreground px-3 py-2 text-left text-[11px] font-medium uppercase tracking-wide'
  const tdCls = 'px-3 py-2 text-xs'

  return (
    <div className='overflow-x-auto'>
      <table className='w-full'>
        <thead>
          <tr className='border-b'>
            <th className={thCls}>Status</th>
            <th className={thCls}>Run</th>
            <th className={thCls}>Turns</th>
            <th className={thCls}>Actions</th>
            <th className={thCls}>Cost</th>
            <th className={thCls}>Model</th>
            <th className={thCls}>Time</th>
          </tr>
        </thead>
        <tbody>
          {/* Live run — always first, highlighted */}
          {liveRun && (
            <tr
              className='cursor-pointer border-b border-emerald-500/20 bg-emerald-500/10 hover:bg-emerald-500/20'
              onClick={() => onSelectLive?.(liveRun.run_uid)}
            >
              <td className={tdCls}>
                <span className='flex items-center gap-1.5'>
                  <span className='relative flex h-2 w-2'>
                    <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75' />
                    <span className='relative inline-flex h-2 w-2 rounded-full bg-emerald-500' />
                  </span>
                  <span className='font-medium text-emerald-400'>running</span>
                </span>
              </td>
              <td className={`${tdCls} font-mono text-[10px] opacity-60`}>{liveRun.run_uid.slice(0, 12)}</td>
              <td className={`${tdCls} font-bold text-emerald-400`}>{liveRun.turns}</td>
              <td className={`${tdCls} text-blue-400`}>{liveRun.actions}</td>
              <td className={tdCls}>{fmtCost(liveRun.cost_usd)}</td>
              <td className={`${tdCls} text-[10px] opacity-60`}>{liveRun.model || '—'}</td>
              <td className={`${tdCls} text-[10px] opacity-50`}>{liveRun.elapsed_sec}s ago</td>
            </tr>
          )}

          {/* Historical runs */}
          {history.map(run => (
            <>
            <tr
              key={run.run_uid}
              className={`cursor-pointer border-b transition-colors hover:bg-white/5 ${selectedRunUid === run.run_uid ? 'bg-white/5' : ''}`}
              onClick={() => setSelectedRunUid(selectedRunUid === run.run_uid ? null : run.run_uid)}
            >
              <td className={tdCls}>
                <span className={
                  run.event === 'completed' ? 'text-emerald-400' :
                  run.event === 'failed'    ? 'text-rose-400' :
                  'text-amber-400'
                }>
                  {run.event}
                </span>
              </td>
              <td className={`${tdCls} font-mono text-[10px] opacity-50`}>{run.run_uid.slice(0, 12)}</td>
              <td className={tdCls}>{run.turns || '—'}</td>
              <td className={tdCls}>{run.actions || '—'}</td>
              <td className={tdCls}>{fmtCost(run.cost_usd)}</td>
              <td className={`${tdCls} text-[10px] opacity-50`}>{(run as any).model || '—'}</td>
              <td className={`${tdCls} text-[10px] opacity-50`}>{fmtTime(run.created_at)}</td>
            </tr>
            {/* Expanded analytics section — T3 adds Turn Timeline, T4 adds Tools/Heatmap, T5 adds Cost/Turn */}
            {selectedRunUid === run.run_uid && (
              <tr key={`${run.run_uid}-detail`}>
                <td colSpan={7} className='border-b bg-white/3 px-4 py-3'>
                  {/* TURN_ANALYTICS_PLACEHOLDER — agents add charts here */}
                  <div className='text-muted-foreground text-xs'>Turn analytics loading…</div>
                </td>
              </tr>
            )}
            </>
          ))}

          {history.length === 0 && !liveRun && (
            <tr>
              <td colSpan={7} className='text-muted-foreground px-3 py-6 text-center text-sm'>
                No runs yet.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  )
}
