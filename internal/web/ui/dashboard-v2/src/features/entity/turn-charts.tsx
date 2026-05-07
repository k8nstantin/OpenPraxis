import { EChart } from '@/components/echart'
import {
  useTurnTimeline,
  useTurnTools,
  useCostPerTurn,
} from '@/lib/queries/turns'

// Turn-level analytics charts. Each chart short-circuits to a quiet
// "no turn data" placeholder when the run pre-dates the turn-events
// feature so the Runs detail row stays compact.

function EmptyState({ label }: { label: string }) {
  return (
    <div className='text-muted-foreground flex h-32 items-center justify-center text-[11px]'>
      {label}
    </div>
  )
}

export function TurnTimelineChart({
  entityId,
  runUid,
}: {
  entityId: string
  runUid: string
}) {
  const { data, isLoading } = useTurnTimeline(entityId, runUid)
  if (isLoading) return <EmptyState label='loading turn timeline…' />
  if (!data || data.length === 0)
    return <EmptyState label='no turn data for this run' />

  const labels = data.map((t) => `Turn ${t.turn}`)
  const durations = data.map((t) => t.duration_ms)

  return (
    <EChart
      height={Math.max(160, 18 * data.length + 60)}
      option={{
        grid: { left: 64, right: 24, top: 12, bottom: 32 },
        tooltip: {
          trigger: 'axis',
          confine: true,
          formatter: (params: unknown) => {
            const arr = params as { name: string; value: number }[]
            const p = arr[0]
            if (!p) return ''
            const ms = p.value
            const sec = (ms / 1000).toFixed(1)
            return `${p.name}<br/>${ms.toLocaleString()} ms (${sec}s)`
          },
        },
        xAxis: {
          type: 'value',
          name: 'ms',
          nameLocation: 'end',
          axisLabel: { fontSize: 9 },
        },
        yAxis: {
          type: 'category',
          data: labels,
          inverse: true,
          axisLabel: { fontSize: 9 },
        },
        series: [
          {
            type: 'bar',
            data: durations,
            itemStyle: { color: '#a78bfa' },
            barMaxWidth: 14,
          },
        ],
      }}
    />
  )
}

// Stacked-bar (X = turn, one series per tool name).
export function ToolsPerTurnChart({
  entityId,
  runUid,
}: {
  entityId: string
  runUid: string
}) {
  const { data, isLoading } = useTurnTools(entityId, runUid)
  if (isLoading) return <EmptyState label='loading tools per turn…' />
  if (!data || data.length === 0)
    return <EmptyState label='no per-turn tool data for this run' />

  const turns = data.map((r) => `T${r.turn}`)
  const tools = Array.from(
    new Set(data.flatMap((r) => r.tools.map((t) => t.name)))
  ).sort()
  const palette = [
    '#a78bfa',
    '#38bdf8',
    '#34d399',
    '#fbbf24',
    '#f87171',
    '#f472b6',
    '#22d3ee',
    '#facc15',
    '#86efac',
    '#fca5a5',
  ]

  const series = tools.map((tool, i) => ({
    name: tool,
    type: 'bar' as const,
    stack: 'tools',
    data: data.map(
      (r) => r.tools.find((t) => t.name === tool)?.count ?? 0
    ),
    itemStyle: { color: palette[i % palette.length] },
  }))

  return (
    <EChart
      height={220}
      option={{
        grid: { left: 32, right: 16, top: 24, bottom: 56 },
        tooltip: { trigger: 'axis', confine: true, axisPointer: { type: 'shadow' } },
        legend: {
          bottom: 0,
          itemWidth: 8,
          itemHeight: 8,
          textStyle: { fontSize: 8 },
        },
        xAxis: { type: 'category', data: turns, axisLabel: { fontSize: 9 } },
        yAxis: {
          type: 'value',
          name: 'calls',
          axisLabel: { fontSize: 9 },
          splitLine: { lineStyle: { opacity: 0.15 } },
        },
        series,
      }}
    />
  )
}

// Cost-per-turn card — single number summarising the run's avg cost
// per assistant turn. Sources: completed-row cost_usd / turns.
export function CostPerTurnCard({
  entityId,
  runUid,
}: {
  entityId: string
  runUid: string
}) {
  const { data, isLoading } = useCostPerTurn(entityId, runUid)
  if (isLoading)
    return (
      <div className='text-muted-foreground rounded border bg-card/40 p-3 text-[11px]'>
        loading…
      </div>
    )
  const first = data?.[0]
  const cost = first?.cost_per_turn_avg ?? 0
  const turns = data?.length ?? 0
  const inTok = first?.input_tokens ?? 0
  const outTok = first?.output_tokens ?? 0

  const fmtCost =
    cost === 0
      ? '—'
      : cost < 0.01
        ? `$${cost.toFixed(4)}`
        : `$${cost.toFixed(3)}`

  return (
    <div className='rounded border bg-card/40 p-3'>
      <div className='text-muted-foreground text-[10px] uppercase tracking-wider'>
        Cost per turn
      </div>
      <div className='mt-1 text-lg font-semibold tabular-nums'>{fmtCost}</div>
      <div className='text-muted-foreground mt-0.5 text-[10px]'>
        {turns} turn{turns === 1 ? '' : 's'} · {inTok.toLocaleString()} in /{' '}
        {outTok.toLocaleString()} out
      </div>
    </div>
  )
}

// Tool-call density heatmap — X = turn, Y = tool, value = call count.
export function ToolDensityHeatmap({
  entityId,
  runUid,
}: {
  entityId: string
  runUid: string
}) {
  const { data, isLoading } = useTurnTools(entityId, runUid)
  if (isLoading) return <EmptyState label='loading tool density…' />
  if (!data || data.length === 0)
    return <EmptyState label='no tool density data' />

  const turnLabels = data.map((r) => `T${r.turn}`)
  const tools = Array.from(
    new Set(data.flatMap((r) => r.tools.map((t) => t.name)))
  ).sort()

  const cells: [number, number, number][] = []
  let maxCount = 1
  data.forEach((row, xi) => {
    row.tools.forEach((tc) => {
      const yi = tools.indexOf(tc.name)
      if (yi < 0) return
      cells.push([xi, yi, tc.count])
      if (tc.count > maxCount) maxCount = tc.count
    })
  })

  return (
    <EChart
      height={Math.max(160, tools.length * 22 + 80)}
      option={{
        grid: { left: 88, right: 32, top: 12, bottom: 32 },
        tooltip: {
          confine: true,
          formatter: (params: unknown) => {
            const p = params as { value: [number, number, number] }
            const [x, y, v] = p.value
            return `${tools[y]} · ${turnLabels[x]}<br/>${v} call${v === 1 ? '' : 's'}`
          },
        },
        xAxis: {
          type: 'category',
          data: turnLabels,
          axisLabel: { fontSize: 9 },
          splitArea: { show: true },
        },
        yAxis: {
          type: 'category',
          data: tools,
          axisLabel: { fontSize: 9 },
          splitArea: { show: true },
        },
        visualMap: {
          min: 0,
          max: maxCount,
          calculable: false,
          orient: 'vertical',
          right: 4,
          top: 'middle',
          itemWidth: 8,
          itemHeight: 80,
          textStyle: { fontSize: 8 },
          inRange: { color: ['#1e293b', '#a78bfa'] },
        },
        series: [
          {
            name: 'tool calls',
            type: 'heatmap',
            data: cells,
            label: { show: false },
          },
        ],
      }}
    />
  )
}

// Composite block rendered inside an expanded RunsTab row.
export function TurnAnalyticsBlock({
  entityId,
  runUid,
}: {
  entityId: string
  runUid: string
}) {
  return (
    <div className='space-y-3'>
      <div className='grid gap-3 md:grid-cols-3'>
        <div className='md:col-span-1'>
          <CostPerTurnCard entityId={entityId} runUid={runUid} />
        </div>
        <div className='md:col-span-2'>
          <div className='text-muted-foreground mb-1 text-[10px] uppercase tracking-wider'>
            Turn timeline
          </div>
          <TurnTimelineChart entityId={entityId} runUid={runUid} />
        </div>
      </div>
      <div className='grid gap-3 md:grid-cols-2'>
        <div>
          <div className='text-muted-foreground mb-1 text-[10px] uppercase tracking-wider'>
            Tools per turn
          </div>
          <ToolsPerTurnChart entityId={entityId} runUid={runUid} />
        </div>
        <div>
          <div className='text-muted-foreground mb-1 text-[10px] uppercase tracking-wider'>
            Tool density
          </div>
          <ToolDensityHeatmap entityId={entityId} runUid={runUid} />
        </div>
      </div>
    </div>
  )
}
