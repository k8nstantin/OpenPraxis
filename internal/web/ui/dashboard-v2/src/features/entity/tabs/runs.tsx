import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useEntityRuns, type EntityKind } from '@/lib/queries/entity'
import { EChart } from '@/components/echart'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'

interface LiveRun {
  run_uid: string
  entity_uid: string
  entity_title: string
  entity_type: string
  elapsed_sec: number
  turns: number
  actions: number
  cost_usd: number
  model: string
}

function useLiveRuns() {
  return useQuery({
    queryKey: ['execution-live'],
    queryFn: () => fetch('/api/execution/live').then(r => r.json()) as Promise<LiveRun[]>,
    refetchInterval: 4000,
  })
}

function fmtDuration(ms: number) {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`
}

function fmtCost(usd: number) {
  if (!usd) return '—'
  return usd < 0.01 ? `$${usd.toFixed(4)}` : `$${usd.toFixed(3)}`
}

interface RunsTabProps {
  kind: EntityKind
  entityId: string
}

export function RunsTab({ kind, entityId }: RunsTabProps) {
  const runs = useEntityRuns(kind, entityId)
  const live = useLiveRuns()
  const [selectedRunUid, setSelectedRunUid] = useState<string | null>(null)

  // This entity's live run (if any)
  const liveRun = live.data?.find(r => r.entity_uid === entityId && r.entity_uid !== 'stdio')

  // Group historical rows by run_uid — keep terminal event per run
  const history = (() => {
    if (!runs.data) return []
    const map = new Map<string, typeof runs.data[0]>()
    for (const row of runs.data) {
      if (['completed','failed','started'].includes(row.event)) {
        const existing = map.get(row.run_uid)
        // Prefer terminal events over started
        if (!existing || (row.event !== 'started' && existing.event === 'started')) {
          map.set(row.run_uid, row)
        }
      }
    }
    return [...map.values()]
      .filter(r => !liveRun || r.run_uid !== liveRun.run_uid)
      .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
  })()

  // ECharts gauge for live run
  const liveGaugeOption = liveRun ? {
    series: [
      {
        type: 'gauge',
        startAngle: 200, endAngle: -20,
        min: 0, max: Math.max(50, liveRun.turns + 10),
        radius: '85%',
        pointer: { show: false },
        progress: { show: true, overlap: false, roundCap: true, clip: false,
          itemStyle: { color: '#10b981' } },
        axisLine: { lineStyle: { width: 10 } },
        splitLine: { show: false },
        axisTick: { show: false },
        axisLabel: { show: false },
        detail: {
          valueAnimation: true,
          formatter: '{value}',
          fontSize: 18, fontWeight: 'bold', color: '#10b981',
          offsetCenter: [0, '10%'],
        },
        title: { show: true, offsetCenter: [0, '50%'], fontSize: 10, color: '#9ca3af' },
        data: [{ value: liveRun.turns, name: 'turns' }],
      },
      {
        type: 'gauge',
        startAngle: 200, endAngle: -20,
        min: 0, max: Math.max(100, liveRun.actions + 20),
        radius: '60%',
        pointer: { show: false },
        progress: { show: true, overlap: false, roundCap: true, clip: false,
          itemStyle: { color: '#3b82f6' } },
        axisLine: { lineStyle: { width: 8 } },
        splitLine: { show: false },
        axisTick: { show: false },
        axisLabel: { show: false },
        detail: {
          valueAnimation: true,
          formatter: '{value}',
          fontSize: 14, fontWeight: 'bold', color: '#3b82f6',
          offsetCenter: [0, '10%'],
        },
        title: { show: true, offsetCenter: [0, '55%'], fontSize: 10, color: '#9ca3af' },
        data: [{ value: liveRun.actions, name: 'actions' }],
      },
    ],
  } : null

  // ECharts bar for history summary
  const recentRuns = history.slice(0, 10).reverse()
  const historyChartOption = recentRuns.length > 1 ? {
    tooltip: { trigger: 'axis' as const },
    xAxis: { type: 'category' as const, data: recentRuns.map((_, i) => `Run ${i + 1}`),
      axisLabel: { show: false } },
    yAxis: [
      { type: 'value' as const, name: 'turns', position: 'left' as const, axisLabel: { fontSize: 9 } },
      { type: 'value' as const, name: 'actions', position: 'right' as const, axisLabel: { fontSize: 9 } },
    ],
    series: [
      { name: 'turns', type: 'bar' as const, data: recentRuns.map(r => r.turns || 0),
        itemStyle: { color: '#10b981' } },
      { name: 'actions', type: 'bar' as const, yAxisIndex: 1,
        data: recentRuns.map(r => r.actions || 0),
        itemStyle: { color: '#3b82f6', opacity: 0.6 } },
    ],
  } : null

  if (runs.isLoading) return (
    <div className='space-y-2 p-2'>
      <Skeleton className='h-32 w-full' />
      <Skeleton className='h-10 w-full' />
    </div>
  )

  return (
    <div className='space-y-4'>

      {/* ── Live Run ── */}
      {liveRun && (
        <Card className='border-emerald-500/30 bg-emerald-500/5'>
          <CardContent className='p-3'>
            <div className='mb-2 flex items-center gap-2'>
              <span className='relative flex h-2.5 w-2.5'>
                <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75' />
                <span className='relative inline-flex h-2.5 w-2.5 rounded-full bg-emerald-500' />
              </span>
              <span className='text-sm font-medium text-emerald-400'>Running</span>
              <span className='text-muted-foreground text-xs'>{liveRun.elapsed_sec}s elapsed</span>
              {liveRun.model && <Badge variant='outline' className='text-[10px]'>{liveRun.model}</Badge>}
              <span className='text-muted-foreground ml-auto font-mono text-[10px]'>{liveRun.run_uid.slice(0,12)}</span>
            </div>
            <div className='flex items-center gap-4'>
              <EChart option={liveGaugeOption!} style={{ height: 140, width: 180 }} />
              <div className='space-y-1 text-sm'>
                <div className='flex gap-2'>
                  <span className='text-muted-foreground w-16 text-xs'>Turns</span>
                  <span className='font-bold text-emerald-400'>{liveRun.turns}</span>
                </div>
                <div className='flex gap-2'>
                  <span className='text-muted-foreground w-16 text-xs'>Actions</span>
                  <span className='font-bold text-blue-400'>{liveRun.actions}</span>
                </div>
                <div className='flex gap-2'>
                  <span className='text-muted-foreground w-16 text-xs'>Cost</span>
                  <span className='font-mono text-xs'>{fmtCost(liveRun.cost_usd)}</span>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* ── History summary chart ── */}
      {historyChartOption && (
        <Card className='gap-0 py-0'>
          <CardContent className='p-2'>
            <div className='text-muted-foreground mb-1 text-xs'>Last {recentRuns.length} runs</div>
            <EChart option={historyChartOption} style={{ height: 80 }} />
          </CardContent>
        </Card>
      )}

      {/* ── Run list ── */}
      <div className='space-y-1'>
        {history.length === 0 && !liveRun && (
          <div className='text-muted-foreground p-4 text-center text-sm'>No runs yet.</div>
        )}
        {history.map(run => (
          <button
            key={run.run_uid}
            onClick={() => setSelectedRunUid(selectedRunUid === run.run_uid ? null : run.run_uid)}
            className={`w-full rounded border px-3 py-2 text-left text-xs transition-colors ${
              selectedRunUid === run.run_uid
                ? 'border-blue-500/40 bg-blue-500/10'
                : 'border-transparent hover:bg-white/5'
            }`}
          >
            <div className='flex items-center gap-2'>
              <Badge variant='outline' className={`text-[9px] uppercase ${
                run.event === 'completed' ? 'border-emerald-500/40 text-emerald-400' :
                run.event === 'failed' ? 'border-rose-500/40 text-rose-400' :
                'border-amber-500/40 text-amber-400'
              }`}>{run.event}</Badge>
              <span className='font-mono text-[10px] opacity-50'>{run.run_uid.slice(0,12)}</span>
              <span className='text-muted-foreground ml-auto text-[10px]'>
                {new Date(typeof run.created_at === 'number' ? run.created_at * 1000 : run.created_at).toLocaleString()}
              </span>
            </div>
            <div className='text-muted-foreground mt-0.5 flex gap-3 text-[10px]'>
              {(run.turns || 0) > 0 && <span>{run.turns} turns</span>}
              {(run.actions || 0) > 0 && <span>{run.actions} actions</span>}
              {run.cost_usd > 0 && <span>{fmtCost(run.cost_usd)}</span>}
            </div>

            {/* Expanded: turn analytics charts injected by T3/T4/T5 */}
            {selectedRunUid === run.run_uid && (
              <div className='mt-3 space-y-2 border-t pt-3'>
                {/* Turn Timeline — added by T3 */}
                {/* Tools per Turn — added by T4 */}
                {/* Heatmap — added by T4 */}
                {/* Cost/turn — added by T5 */}
                <div className='text-muted-foreground text-center text-xs'>
                  Turn analytics coming soon
                </div>
              </div>
            )}
          </button>
        ))}
      </div>
    </div>
  )
}
