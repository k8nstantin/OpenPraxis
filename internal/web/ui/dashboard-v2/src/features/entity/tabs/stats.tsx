import { useMemo, useState } from 'react'
import {
  useRunStats,
  useSystemStats,
  type RunHostSample,
  type RunRow,
  type SystemHostSample,
} from '@/lib/queries/stats'
import type { EntityKind } from '@/lib/queries/entity'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { EChart } from '@/components/echart'

// Stats tab — three stacked panels backed by /api/run-stats and
// /api/system-stats. Cumulative rolls up across runs (across descendants
// for product / manifest scope, this task's runs only on task scope).
// Per-run drills into a single run's host samples + summary numbers.
// System Capacity reads system_host_samples between [from, to].
//
// Single chart library (echarts + echarts-for-react). The EChart wrapper
// themes via CSS vars so the panels match the rest of the dashboard.

interface StatsTabProps {
  kind: EntityKind
  entityId: string
}

export function StatsTab({ kind, entityId }: StatsTabProps) {
  const runStats = useRunStats(kind, entityId)
  const runs = useMemo(() => runStats.data?.runs ?? [], [runStats.data])
  const samplesByRun = runStats.data?.samples_by_run ?? {}
  const [view, setView] = useState<'cumulative' | 'per_run'>('cumulative')

  const body = runStats.isLoading ? (
    <Skeleton className='h-64 w-full' />
  ) : runs.length === 0 ? (
    <Empty msg='No runs yet for this entity.' />
  ) : view === 'cumulative' ? (
    <CumulativePanel runs={runs} kind={kind} />
  ) : (
    <PerRunPanel runs={runs} samplesByRun={samplesByRun} />
  )

  return (
    <div data-testid='stats-tab' className='space-y-3'>
      {/* Sub-tab segmented control — single tab strip across the whole
          Stats area. System Capacity moves to its own top-level menu. */}
      <div className='inline-flex rounded-md border bg-card p-1 text-sm'>
        <button
          type='button'
          onClick={() => setView('cumulative')}
          className={
            'rounded px-3 py-1 transition-colors ' +
            (view === 'cumulative'
              ? 'bg-primary/15 text-foreground font-semibold'
              : 'text-muted-foreground hover:text-foreground')
          }
        >
          Cumulative
        </button>
        <button
          type='button'
          onClick={() => setView('per_run')}
          className={
            'rounded px-3 py-1 transition-colors ' +
            (view === 'per_run'
              ? 'bg-primary/15 text-foreground font-semibold'
              : 'text-muted-foreground hover:text-foreground')
          }
        >
          Per run
        </button>
      </div>
      {body}
    </div>
  )
}

function Panel({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section data-testid={`stats-panel-${title.toLowerCase().replace(/ /g, '-')}`}>
      <h2 className='mb-3 text-lg font-semibold tracking-tight'>{title}</h2>
      <Card>
        <CardContent className='p-4'>{children}</CardContent>
      </Card>
    </section>
  )
}

function Empty({ msg }: { msg: string }) {
  return <p className='text-muted-foreground text-sm'>{msg}</p>
}

// ── Cumulative ─────────────────────────────────────────────────────────

function CumulativePanel({
  runs,
  kind,
}: {
  runs: RunRow[]
  kind: EntityKind
}) {
  // Stable left-to-right order by run number / started_at.
  const ordered = useMemo(
    () =>
      [...runs].sort((a, b) => {
        const ta = new Date(a.started_at).getTime()
        const tb = new Date(b.started_at).getTime()
        return ta - tb
      }),
    [runs]
  )

  const xs = ordered.map((r) => `#${r.run_number}`)
  const costs = ordered.map((r) => round2(r.cost_usd))
  const cumulative = ordered.reduce<number[]>((acc, r) => {
    const next = (acc[acc.length - 1] ?? 0) + r.cost_usd
    acc.push(round2(next))
    return acc
  }, [])

  const inputs = ordered.map((r) => r.input_tokens)
  const outputs = ordered.map((r) => r.output_tokens)
  const cacheRead = ordered.map((r) => r.cache_read_tokens)
  const cacheCreate = ordered.map((r) => r.cache_create_tokens)

  const cacheHitPct = ordered.map((r) => {
    const total =
      r.input_tokens + r.output_tokens + r.cache_read_tokens + r.cache_create_tokens
    return total === 0 ? 0 : round2((r.cache_read_tokens / total) * 100)
  })

  const durations = ordered.map((r) => round2(r.duration_ms / 1000))

  const statusCounts = ordered.reduce<Record<string, number>>((acc, r) => {
    const key = r.cancelled_at ? 'cancelled' : r.status || 'unknown'
    acc[key] = (acc[key] ?? 0) + 1
    return acc
  }, {})

  const errors = ordered.map((r) => r.errors)
  const compactions = ordered.map((r) => r.compactions)

  // Senior-dev derived metrics — what you actually want to watch when
  // iterating on agent runs:
  //   - cost efficiency ($/turn, $/action) — am I getting cheaper per
  //     unit of useful work over time?
  //   - tokens-per-turn — verbosity trend; is the agent tighter or
  //     getting chattier?
  //   - code churn — lines_added vs lines_removed per run; how much
  //     real code is moving vs spinning?
  //   - files touched + commits per run — breadth of impact.
  //   - compaction rate (compactions / turns) — context-window
  //     pressure signal; if this climbs the agent is hitting limits.
  const costPerTurn = ordered.map((r) =>
    r.turns > 0 ? round2(r.cost_usd / r.turns) : 0
  )
  const costPerAction = ordered.map((r) =>
    r.actions > 0 ? round2(r.cost_usd / r.actions) : 0
  )
  const tokensPerTurn = ordered.map((r) => {
    const total =
      r.input_tokens +
      r.output_tokens +
      r.cache_read_tokens +
      r.cache_create_tokens
    return r.turns > 0 ? Math.round(total / r.turns) : 0
  })
  const linesAdded = ordered.map((r) => (r as { lines_added?: number }).lines_added ?? 0)
  const linesRemoved = ordered.map(
    (r) => -((r as { lines_removed?: number }).lines_removed ?? 0)
  )
  const filesChanged = ordered.map((r) => r.files_changed ?? 0)
  const commitsPerRun = ordered.map((r) => (r as { commits?: number }).commits ?? 0)
  const compactionRate = ordered.map((r) =>
    r.turns > 0 ? round2((r.compactions / r.turns) * 100) : 0
  )

  // Cost-by-model — group runs by model and sum cost. Stacked bar per
  // run gives a "what model handled this run" + total at-a-glance read.
  const modelKeys = Array.from(new Set(ordered.map((r) => r.model || '—')))
  const costByModel: Record<string, number[]> = {}
  for (const k of modelKeys) {
    costByModel[k] = ordered.map((r) =>
      (r.model || '—') === k ? round2(r.cost_usd) : 0
    )
  }

  // Top-10 tasks by cost (product / manifest scope only). Skipped on
  // task scope where every run shares one task_id.
  const topTasks = useMemo(() => {
    if (kind === 'task') return null
    const byTask = new Map<string, number>()
    for (const r of ordered) {
      byTask.set(r.task_id, (byTask.get(r.task_id) ?? 0) + r.cost_usd)
    }
    return [...byTask.entries()]
      .map(([id, cost]) => ({ id, cost: round2(cost) }))
      .sort((a, b) => b.cost - a.cost)
      .slice(0, 10)
  }, [ordered, kind])

  // Aggregates for the latest-run column + footer summary.
  const totalRuns = ordered.length
  const latest = ordered[ordered.length - 1]
  const totalCost = cumulative[cumulative.length - 1] ?? 0
  const totalTurns = ordered.reduce((s, r) => s + r.turns, 0)
  const totalActions = ordered.reduce((s, r) => s + r.actions, 0)
  const totalLines = ordered.reduce((s, r) => s + r.lines, 0)
  const peakCpu = Math.max(0, ...ordered.map((r) => r.peak_cpu_pct ?? 0))
  const peakRss = Math.max(0, ...ordered.map((r) => r.peak_rss_mb ?? 0))
  const cpuPcts = ordered.map((r) => round2(r.avg_cpu_pct ?? 0))
  const rssMbs = ordered.map((r) => round2(r.peak_rss_mb ?? 0))

  // Latest-run token mix totals for the horizontal bar.
  const latestMix = latest
    ? {
        input: latest.input_tokens,
        output: latest.output_tokens,
        cache_read: latest.cache_read_tokens,
        cache_create: latest.cache_create_tokens,
      }
    : { input: 0, output: 0, cache_read: 0, cache_create: 0 }
  const latestTotalTokens =
    latestMix.input + latestMix.output + latestMix.cache_read + latestMix.cache_create
  const latestCacheHitPct =
    latestTotalTokens === 0
      ? 0
      : Math.round((latestMix.cache_read / latestTotalTokens) * 100)

  // Shared chart config — distilled from apache/echarts-examples canonical
  // patterns (area-stack-gradient.ts, line-marker.ts, mix-line-bar.ts).
  //
  //   - tooltip.axisPointer.type='cross' for crosshair guides
  //   - toolbox feature { dataZoom (yAxis locked), restore, saveAsImage }
  //   - dataZoom inside + slider so brush-zoom works wheel + drag
  //   - LinearGradient via plain object form (same as constructor at runtime)
  //   - emphasis.focus='series' so hovering one stacked series dims others
  //
  const tooltipCross = {
    trigger: 'axis' as const,
    axisPointer: {
      type: 'cross' as const,
      crossStyle: { color: '#94a3b8' },
      label: { backgroundColor: '#1f2937' },
    },
  }
  const toolboxStd = {
    feature: {
      dataZoom: { yAxisIndex: 'none' as const },
      restore: {},
      saveAsImage: {},
    },
    right: 8,
    top: 4,
    itemSize: 12,
  }
  const dataZoomBrush = [
    { type: 'inside' as const },
    { type: 'slider' as const, height: 16, bottom: 4 },
  ]
  const grad = (hex: string) => ({
    type: 'linear' as const,
    x: 0,
    y: 0,
    x2: 0,
    y2: 1,
    colorStops: [
      { offset: 0, color: hex + 'cc' },
      { offset: 1, color: hex + '00' },
    ],
  })

  return (
    <div className='space-y-4'>
      <div className='flex items-baseline gap-3'>
        <div className='text-sm font-semibold'>
          Cumulative — {totalRuns} run{totalRuns === 1 ? '' : 's'}
        </div>
        {latest ? (
          <div className='text-muted-foreground text-xs'>
            latest #{latest.run_number} · {fmtRelTime(latest.started_at)} ·{' '}
            {fmtDuration(latest.duration_ms)} · cumulative cost $
            {totalCost.toFixed(2)} · cache-hit {latestCacheHitPct}%
          </div>
        ) : null}
      </div>

      <div className='grid grid-cols-2 gap-4'>
        <ChartCell label='Cost per run'>
          <EChart
            option={{
              tooltip: tooltipCross,
              toolbox: toolboxStd,
              grid: { left: 50, right: 24, top: 24, bottom: 36 },
              xAxis: { type: 'category', data: xs, boundaryGap: false },
              yAxis: {
                type: 'value',
                name: 'USD',
                axisLabel: { formatter: '${value}' },
              },
              dataZoom: dataZoomBrush,
              series: [
                {
                  type: 'line',
                  smooth: true,
                  data: costs,
                  color: '#10b981',
                  symbol: 'circle',
                  symbolSize: 6,
                  areaStyle: { color: grad('#10b981') },
                  markPoint: {
                    symbol: 'pin',
                    symbolSize: 36,
                    data: [
                      { type: 'max', name: 'peak', label: { color: '#fff' } },
                    ],
                  },
                  markLine: {
                    data: [{ type: 'average', name: 'avg' }],
                    label: { formatter: 'avg ${c|0}' },
                    lineStyle: { color: '#10b981', type: 'dashed' },
                  },
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Cumulative cost'>
          <EChart
            option={{
              tooltip: tooltipCross,
              toolbox: toolboxStd,
              grid: { left: 50, right: 24, top: 24, bottom: 36 },
              xAxis: { type: 'category', data: xs, boundaryGap: false },
              yAxis: {
                type: 'value',
                name: 'USD',
                axisLabel: { formatter: '${value}' },
              },
              dataZoom: dataZoomBrush,
              series: [
                {
                  type: 'line',
                  smooth: true,
                  data: cumulative,
                  color: '#3b82f6',
                  symbol: 'circle',
                  symbolSize: 6,
                  areaStyle: { color: grad('#3b82f6') },
                  emphasis: { focus: 'series' },
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Token mix per run'>
          <EChart
            option={{
              tooltip: tooltipCross,
              toolbox: toolboxStd,
              legend: { data: ['input', 'output', 'cache_read', 'cache_create'], top: 0 },
              grid: { left: 60, right: 24, top: 30, bottom: 36 },
              xAxis: { type: 'category', data: xs, boundaryGap: false },
              yAxis: { type: 'value', axisLabel: { formatter: fmtTokens } },
              dataZoom: dataZoomBrush,
              series: [
                {
                  name: 'input',
                  type: 'line',
                  stack: 'tokens',
                  smooth: true,
                  showSymbol: false,
                  lineStyle: { width: 0 },
                  emphasis: { focus: 'series' },
                  areaStyle: { opacity: 0.85, color: grad('#fbbf24') },
                  color: '#fbbf24',
                  data: inputs,
                },
                {
                  name: 'output',
                  type: 'line',
                  stack: 'tokens',
                  smooth: true,
                  showSymbol: false,
                  lineStyle: { width: 0 },
                  emphasis: { focus: 'series' },
                  areaStyle: { opacity: 0.85, color: grad('#3b82f6') },
                  color: '#3b82f6',
                  data: outputs,
                },
                {
                  name: 'cache_read',
                  type: 'line',
                  stack: 'tokens',
                  smooth: true,
                  showSymbol: false,
                  lineStyle: { width: 0 },
                  emphasis: { focus: 'series' },
                  areaStyle: { opacity: 0.85, color: grad('#10b981') },
                  color: '#10b981',
                  data: cacheRead,
                },
                {
                  name: 'cache_create',
                  type: 'line',
                  stack: 'tokens',
                  smooth: true,
                  showSymbol: false,
                  lineStyle: { width: 0 },
                  emphasis: { focus: 'series' },
                  areaStyle: { opacity: 0.85, color: grad('#dc2626') },
                  color: '#dc2626',
                  data: cacheCreate,
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Cache-hit %'>
          <EChart
            option={{
              tooltip: tooltipCross,
              toolbox: toolboxStd,
              grid: { left: 50, right: 24, top: 24, bottom: 36 },
              xAxis: { type: 'category', data: xs, boundaryGap: false },
              yAxis: {
                type: 'value',
                max: 100,
                axisLabel: { formatter: '{value}%' },
              },
              dataZoom: dataZoomBrush,
              series: [
                {
                  type: 'line',
                  smooth: true,
                  data: cacheHitPct,
                  color: '#8b5cf6',
                  symbol: 'circle',
                  symbolSize: 6,
                  areaStyle: { color: grad('#8b5cf6') },
                  markLine: {
                    data: [
                      { yAxis: 80, name: 'good' },
                      { type: 'average', name: 'avg' },
                    ],
                    lineStyle: { type: 'dashed' },
                  },
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Duration per run'>
          <EChart
            option={{
              tooltip: tooltipCross,
              toolbox: toolboxStd,
              grid: { left: 50, right: 24, top: 24, bottom: 36 },
              xAxis: { type: 'category', data: xs, boundaryGap: false },
              yAxis: {
                type: 'value',
                name: 'sec',
                axisLabel: { formatter: '{value}s' },
              },
              dataZoom: dataZoomBrush,
              series: [
                {
                  type: 'line',
                  smooth: true,
                  data: durations,
                  color: '#f59e0b',
                  symbol: 'circle',
                  symbolSize: 6,
                  areaStyle: { color: grad('#f59e0b') },
                  markPoint: {
                    symbol: 'pin',
                    symbolSize: 36,
                    data: [{ type: 'max', name: 'longest' }],
                  },
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Errors + Compactions per run'>
          <EChart
            option={{
              tooltip: tooltipCross,
              toolbox: toolboxStd,
              legend: { data: ['errors', 'compactions'], top: 0 },
              grid: { left: 50, right: 24, top: 30, bottom: 36 },
              xAxis: { type: 'category', data: xs, boundaryGap: false },
              yAxis: { type: 'value' },
              dataZoom: dataZoomBrush,
              series: [
                { name: 'errors', type: 'bar', data: errors, color: '#ef4444', barWidth: '40%' },
                { name: 'compactions', type: 'bar', data: compactions, color: '#0ea5e9', barWidth: '40%' },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Cost efficiency ($ per turn / per action)'>
          <EChart
            option={{
              tooltip: tooltipCross,
              toolbox: toolboxStd,
              legend: { data: ['$/turn', '$/action'], top: 0 },
              grid: { left: 60, right: 24, top: 30, bottom: 36 },
              xAxis: { type: 'category', data: xs, boundaryGap: false },
              yAxis: { type: 'value', axisLabel: { formatter: '${value}' } },
              dataZoom: dataZoomBrush,
              series: [
                {
                  name: '$/turn',
                  type: 'line',
                  smooth: true,
                  data: costPerTurn,
                  color: '#22d3ee',
                  symbol: 'circle',
                  symbolSize: 5,
                  areaStyle: { color: grad('#22d3ee') },
                  markLine: { data: [{ type: 'average', name: 'avg' }], lineStyle: { type: 'dashed', color: '#22d3ee' } },
                },
                {
                  name: '$/action',
                  type: 'line',
                  smooth: true,
                  data: costPerAction,
                  color: '#a78bfa',
                  symbol: 'circle',
                  symbolSize: 5,
                  markLine: { data: [{ type: 'average', name: 'avg' }], lineStyle: { type: 'dashed', color: '#a78bfa' } },
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Tokens per turn (verbosity)'>
          <EChart
            option={{
              tooltip: tooltipCross,
              toolbox: toolboxStd,
              grid: { left: 60, right: 24, top: 24, bottom: 36 },
              xAxis: { type: 'category', data: xs, boundaryGap: false },
              yAxis: { type: 'value', axisLabel: { formatter: fmtTokens } },
              dataZoom: dataZoomBrush,
              series: [
                {
                  type: 'line',
                  smooth: true,
                  data: tokensPerTurn,
                  color: '#f472b6',
                  symbol: 'circle',
                  symbolSize: 6,
                  areaStyle: { color: grad('#f472b6') },
                  markPoint: {
                    symbol: 'pin',
                    symbolSize: 36,
                    data: [{ type: 'max', name: 'chattiest' }],
                  },
                  markLine: {
                    data: [{ type: 'average', name: 'avg' }],
                    lineStyle: { type: 'dashed' },
                  },
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Code churn — lines added / removed per run'>
          <EChart
            option={{
              tooltip: tooltipCross,
              toolbox: toolboxStd,
              legend: { data: ['added', 'removed'], top: 0 },
              grid: { left: 50, right: 24, top: 30, bottom: 36 },
              xAxis: { type: 'category', data: xs, boundaryGap: false },
              yAxis: { type: 'value' },
              dataZoom: dataZoomBrush,
              series: [
                { name: 'added', type: 'bar', stack: 'churn', data: linesAdded, color: '#10b981', barWidth: '50%' },
                { name: 'removed', type: 'bar', stack: 'churn', data: linesRemoved, color: '#ef4444', barWidth: '50%' },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Files changed + commits per run'>
          <EChart
            option={{
              tooltip: tooltipCross,
              toolbox: toolboxStd,
              legend: { data: ['files', 'commits'], top: 0 },
              grid: { left: 40, right: 40, top: 30, bottom: 36 },
              xAxis: { type: 'category', data: xs, boundaryGap: false },
              yAxis: [
                { type: 'value', name: 'files' },
                { type: 'value', name: 'commits' },
              ],
              dataZoom: dataZoomBrush,
              series: [
                {
                  name: 'files',
                  type: 'bar',
                  data: filesChanged,
                  color: '#0ea5e9',
                  barWidth: '40%',
                },
                {
                  name: 'commits',
                  type: 'line',
                  yAxisIndex: 1,
                  data: commitsPerRun,
                  color: '#22c55e',
                  smooth: true,
                  symbol: 'diamond',
                  symbolSize: 8,
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Context pressure — compactions / turn (%)'>
          <EChart
            option={{
              tooltip: tooltipCross,
              toolbox: toolboxStd,
              grid: { left: 50, right: 24, top: 24, bottom: 36 },
              xAxis: { type: 'category', data: xs, boundaryGap: false },
              yAxis: {
                type: 'value',
                axisLabel: { formatter: '{value}%' },
              },
              dataZoom: dataZoomBrush,
              series: [
                {
                  type: 'line',
                  smooth: true,
                  data: compactionRate,
                  color: '#fb7185',
                  symbol: 'circle',
                  symbolSize: 6,
                  areaStyle: { color: grad('#fb7185') },
                  markLine: {
                    data: [
                      { yAxis: 5, name: 'caution' },
                      { yAxis: 10, name: 'high' },
                    ],
                    lineStyle: { type: 'dashed' },
                  },
                },
              ],
            }}
          />
        </ChartCell>

        {modelKeys.length > 1 ? (
          <ChartCell label='Cost by model'>
            <EChart
              option={{
                tooltip: tooltipCross,
              toolbox: toolboxStd,
                legend: { top: 0 },
                grid: { left: 50, right: 24, top: 30, bottom: 36 },
                xAxis: {
                  type: 'category',
                  data: xs,
                  boundaryGap: false,
                  axisLabel: { interval: 0 },
                },
                yAxis: { type: 'value', axisLabel: { formatter: '${value}' } },
                dataZoom: dataZoomBrush,
                series: modelKeys.map((m, i) => ({
                  name: m,
                  type: 'bar',
                  stack: 'cost',
                  data: costByModel[m],
                  barWidth: '60%',
                  color: ['#10b981', '#3b82f6', '#a78bfa', '#f59e0b', '#ef4444'][
                    i % 5
                  ],
                })),
              }}
            />
          </ChartCell>
        ) : null}

        {Object.keys(statusCounts).length > 0 ? (
          <ChartCell label='Status breakdown'>
            <EChart
              option={{
                tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
                legend: { bottom: 0 },
                series: [
                  {
                    type: 'pie',
                    radius: ['45%', '72%'],
                    avoidLabelOverlap: true,
                    data: Object.entries(statusCounts).map(([name, value]) => ({
                      name,
                      value,
                      itemStyle: {
                        color:
                          name === 'success'
                            ? '#10b981'
                            : name === 'failed'
                              ? '#ef4444'
                              : name === 'cancelled'
                                ? '#f59e0b'
                                : '#71717a',
                      },
                    })),
                    label: { formatter: '{b}\n{c}' },
                  },
                ],
              }}
            />
          </ChartCell>
        ) : null}

        {topTasks && topTasks.length > 0 ? (
          <ChartCell label='Top 10 tasks by cost'>
            <EChart
              option={{
                tooltip: tooltipCross,
              toolbox: toolboxStd,
                grid: { left: 100, right: 60, top: 12, bottom: 30 },
                xAxis: {
                  type: 'value',
                  name: 'USD',
                  axisLabel: { formatter: '${value}' },
                },
                yAxis: {
                  type: 'category',
                  data: topTasks.map((t) => t.id.slice(0, 12)),
                  inverse: true,
                },
                series: [
                  {
                    type: 'bar',
                    data: topTasks.map((t) => t.cost),
                    color: '#22c55e',
                    label: {
                      show: true,
                      position: 'right',
                      formatter: (p: { value: number }) => '$' + p.value.toFixed(2),
                    },
                  },
                ],
              }}
            />
          </ChartCell>
        ) : null}
      </div>

      {/* Footer */}
      <div className='border-border text-muted-foreground border-t pt-2 text-[11px]'>
        Turns {fmtCount(totalTurns)} · Actions {fmtCount(totalActions)} · Lines{' '}
        {fmtCount(totalLines)} · Peak CPU {Math.round(peakCpu)}% · Peak RSS{' '}
        {Math.round(peakRss)} MB
        {latest?.model ? ` · ${latest.model}` : ''}
        {latest?.pricing_version ? ` · pricing ${latest.pricing_version}` : ''}
      </div>
    </div>
  )
}

// Inline-SVG sparkline row — label / line / latest value. Same DNA as
// Portal A's run-stats.js. Pure SVG, no library, scales with container.
function SparkRow({
  label,
  values,
  latest,
  color,
}: {
  label: string
  values: number[]
  latest: string
  color: string
}) {
  if (!values.length) {
    return (
      <div className='flex items-center gap-3 text-xs'>
        <span className='text-muted-foreground w-16 shrink-0'>{label}</span>
        <span className='text-muted-foreground flex-1'>—</span>
        <span className='font-mono'>—</span>
      </div>
    )
  }
  const W = 600
  const H = 28
  const padX = 4
  const padY = 4
  const minV = Math.min(...values)
  let maxV = Math.max(...values)
  if (maxV === minV) maxV = minV + 1
  const stepX = values.length > 1 ? (W - padX * 2) / (values.length - 1) : 0
  const pts = values.map((v, i) => {
    const x = padX + i * stepX
    const y = H - padY - ((v - minV) / (maxV - minV)) * (H - padY * 2)
    return [x, y] as [number, number]
  })
  const path = pts.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(' ')
  return (
    <div className='flex items-center gap-3 text-xs'>
      <span className='text-muted-foreground w-16 shrink-0'>{label}</span>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        preserveAspectRatio='none'
        className='block h-7 flex-1'
        aria-label={label}
      >
        <polyline
          points={path}
          fill='none'
          stroke={color}
          strokeWidth={1.5}
          vectorEffect='non-scaling-stroke'
        />
        {pts.length === 1 ? (
          <circle cx={pts[0][0]} cy={pts[0][1]} r={2.5} fill={color} />
        ) : (
          <circle cx={pts[pts.length - 1][0]} cy={pts[pts.length - 1][1]} r={2.5} fill={color} />
        )}
      </svg>
      <span className='font-mono w-20 shrink-0 text-right tabular-nums'>{latest}</span>
    </div>
  )
}

// Token mix horizontal stacked bar — input / output / cache-read /
// cache-create as flexbox segments sized by token count, label inside
// each segment when wide enough.
function TokenMixBar({
  mix,
}: {
  mix: { input: number; output: number; cache_read: number; cache_create: number }
}) {
  const total = mix.input + mix.output + mix.cache_read + mix.cache_create
  if (total === 0) return null
  const segs = [
    { label: 'input', n: mix.input, color: '#fbbf24' },
    { label: 'output', n: mix.output, color: '#3b82f6' },
    { label: 'cache-read', n: mix.cache_read, color: '#10b981' },
    { label: 'cache-create', n: mix.cache_create, color: '#dc2626' },
  ].filter((s) => s.n > 0)
  return (
    <div className='flex h-7 w-full overflow-hidden rounded'>
      {segs.map((s) => {
        const pct = (s.n / total) * 100
        return (
          <div
            key={s.label}
            className='flex items-center justify-center text-[10px] font-medium text-black/80'
            style={{ width: `${pct}%`, backgroundColor: s.color }}
            title={`${s.label}: ${s.n.toLocaleString()}`}
          >
            {pct >= 8 ? `${s.label} ${fmtTokens(s.n)}` : ''}
          </div>
        )
      })}
    </div>
  )
}

// Cache-hit ring — small donut (60×60) with the % rendered in the
// middle. Pure SVG so it scales with the container without ECharts.
function CacheHitRing({ pct }: { pct: number }) {
  const R = 22
  const C = 28
  const circumference = 2 * Math.PI * R
  const dash = (Math.max(0, Math.min(100, pct)) / 100) * circumference
  return (
    <div className='flex w-20 shrink-0 flex-col items-center justify-center gap-0.5'>
      <svg viewBox='0 0 56 56' className='h-14 w-14' aria-label='cache hit'>
        <circle
          cx={C}
          cy={C}
          r={R}
          fill='none'
          stroke='currentColor'
          strokeOpacity={0.15}
          strokeWidth={6}
        />
        <circle
          cx={C}
          cy={C}
          r={R}
          fill='none'
          stroke='#3b82f6'
          strokeWidth={6}
          strokeDasharray={`${dash} ${circumference}`}
          strokeLinecap='round'
          transform={`rotate(-90 ${C} ${C})`}
        />
        <text
          x={C}
          y={C + 4}
          textAnchor='middle'
          fontSize='13'
          fontWeight='bold'
          fill='currentColor'
        >
          {pct}%
        </text>
      </svg>
      <span className='text-muted-foreground text-[9px] uppercase tracking-wider'>
        cache hit
      </span>
    </div>
  )
}

function fmtRelTime(iso: string): string {
  if (!iso) return '—'
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return '—'
  const diff = Date.now() - t
  const s = Math.floor(diff / 1000)
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  return `${d}d ago`
}

// ── Per-run ───────────────────────────────────────────────────────────

function PerRunPanel({
  runs,
  samplesByRun,
}: {
  runs: RunRow[]
  samplesByRun: Record<string, RunHostSample[]>
}) {
  const ordered = useMemo(
    () =>
      [...runs].sort(
        (a, b) =>
          new Date(b.started_at).getTime() - new Date(a.started_at).getTime()
      ),
    [runs]
  )
  const [selected, setSelected] = useState<string>(
    ordered[0] ? String(ordered[0].id) : ''
  )
  const run = useMemo(
    () => ordered.find((r) => String(r.id) === selected),
    [ordered, selected]
  )
  const samples = useMemo(
    () => (run ? (samplesByRun[String(run.id)] ?? []) : []),
    [run, samplesByRun]
  )

  if (!run) return <Empty msg='No run selected.' />

  return (
    <div className='space-y-4'>
      <div className='flex items-center gap-3'>
        <span className='text-muted-foreground text-sm'>Run:</span>
        <Select value={selected} onValueChange={setSelected}>
          <SelectTrigger className='w-64'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {ordered.map((r) => (
              <SelectItem key={r.id} value={String(r.id)}>
                #{r.run_number} — {fmtTime(r.started_at)} ({r.status})
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <RunSummary run={run} />

      <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
        <ChartCell label='Token mix'>
          <EChart
            height={140}
            option={{
              tooltip: { trigger: 'axis' },
              legend: { data: ['input', 'output', 'cache_read', 'cache_create'] },
              xAxis: { type: 'value' },
              yAxis: { type: 'category', data: ['tokens'] },
              series: [
                { name: 'input', type: 'bar', stack: 'tokens', data: [run.input_tokens] },
                { name: 'output', type: 'bar', stack: 'tokens', data: [run.output_tokens] },
                {
                  name: 'cache_read',
                  type: 'bar',
                  stack: 'tokens',
                  data: [run.cache_read_tokens],
                },
                {
                  name: 'cache_create',
                  type: 'bar',
                  stack: 'tokens',
                  data: [run.cache_create_tokens],
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Cache-hit'>
          <EChart
            height={200}
            option={{
              tooltip: { trigger: 'item' },
              series: [
                {
                  type: 'pie',
                  radius: ['55%', '75%'],
                  label: {
                    show: true,
                    position: 'center',
                    formatter: () =>
                      `${cacheHit(run).toFixed(1)}%`,
                    fontSize: 22,
                    color: 'inherit',
                  },
                  data: [
                    { name: 'cache_read', value: run.cache_read_tokens },
                    {
                      name: 'other',
                      value:
                        run.input_tokens +
                        run.output_tokens +
                        run.cache_create_tokens,
                    },
                  ],
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Cumulative cost timeline'>
          <EChart
            option={{
              xAxis: { type: 'time' },
              yAxis: { type: 'value', name: 'USD' },
              series: [
                {
                  type: 'line',
                  smooth: true,
                  areaStyle: { opacity: 0.3 },
                  data: samples.map((s) => [s.ts, round2(s.cost_usd)]),
                  color: '#10b981',
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='CPU %'>
          <EChart
            option={{
              xAxis: { type: 'time' },
              yAxis: { type: 'value', name: '%' },
              series: [
                {
                  type: 'line',
                  smooth: true,
                  data: samples.map((s) => [s.ts, round2(s.cpu_pct)]),
                  color: '#f97316',
                  markPoint: { data: [{ type: 'max', name: 'peak' }] },
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='RSS (MB)'>
          <EChart
            option={{
              xAxis: { type: 'time' },
              yAxis: { type: 'value', name: 'MB' },
              series: [
                {
                  type: 'line',
                  smooth: true,
                  data: samples.map((s) => [s.ts, round2(s.rss_mb)]),
                  color: '#a855f7',
                  markPoint: { data: [{ type: 'max', name: 'peak' }] },
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Turns + Actions timeline'>
          <EChart
            option={{
              tooltip: { trigger: 'axis' },
              legend: { data: ['turns', 'actions'] },
              xAxis: { type: 'time' },
              yAxis: { type: 'value' },
              series: [
                {
                  name: 'turns',
                  type: 'line',
                  data: samples.map((s) => [s.ts, s.turns]),
                  color: '#0ea5e9',
                },
                {
                  name: 'actions',
                  type: 'line',
                  data: samples.map((s) => [s.ts, s.actions]),
                  color: '#22c55e',
                },
              ],
            }}
          />
        </ChartCell>
      </div>

      <GitOutputCard run={run} />
    </div>
  )
}

function RunSummary({ run }: { run: RunRow }) {
  return (
    <div className='grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-9'>
      <Stat label='status'>
        <Badge variant='outline'>
          {run.cancelled_at ? 'cancelled' : run.status || '—'}
        </Badge>
      </Stat>
      <Stat label='cost'>${round2(run.cost_usd)}</Stat>
      <Stat label='turns'>{run.turns}</Stat>
      <Stat label='actions'>{run.actions}</Stat>
      <Stat label='duration'>{fmtDuration(run.duration_ms)}</Stat>
      <Stat label='exit_code'>{run.exit_code}</Stat>
      <Stat label='model'>{run.model || '—'}</Stat>
      <Stat label='runtime'>{run.agent_runtime || '—'}</Stat>
      <Stat label='version'>{run.agent_version || '—'}</Stat>
    </div>
  )
}

function Stat({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className='border-border bg-card flex flex-col rounded-md border p-2'>
      <span className='text-muted-foreground text-[10px] uppercase tracking-wide'>
        {label}
      </span>
      <span className='text-sm font-medium'>{children}</span>
    </div>
  )
}

function GitOutputCard({ run }: { run: RunRow }) {
  const [expanded, setExpanded] = useState(false)
  const truncated = run.output && run.output.length > 4000
  const visible = expanded
    ? run.output
    : (run.output ?? '').slice(0, 4000)

  return (
    <Card>
      <CardContent className='space-y-3 p-4'>
        <div className='flex flex-wrap gap-x-6 gap-y-1 text-sm'>
          <Field label='branch' value={run.branch || '—'} />
          <Field
            label='commit'
            value={
              run.commit_sha
                ? run.commit_sha.slice(0, 12)
                : '—'
            }
          />
          {run.pr_number > 0 ? (
            <Field
              label='PR'
              value={`#${run.pr_number}`}
            />
          ) : null}
          <Field
            label='lines'
            value={`+${run.lines_added} / -${run.lines_removed}`}
          />
          <Field
            label='files_changed'
            value={String(run.files_changed)}
          />
          <Field label='commits' value={String(run.commits)} />
        </div>
        {run.output ? (
          <div>
            <div className='mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground'>
              Output
            </div>
            <pre className='border-border bg-muted/40 max-h-[400px] overflow-auto rounded border p-3 text-xs'>
              {visible}
            </pre>
            {truncated ? (
              <Button
                size='sm'
                variant='ghost'
                onClick={() => setExpanded((v) => !v)}
                className='mt-2'
              >
                {expanded ? 'Show less' : 'Show all'}
              </Button>
            ) : null}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <span className='text-sm'>
      <span className='text-muted-foreground'>{label}:</span>{' '}
      <span className='font-mono text-xs'>{value}</span>
    </span>
  )
}

// ── System capacity ───────────────────────────────────────────────────

const WINDOWS = [
  { key: '1h', label: '1h', ms: 1 * 60 * 60 * 1000 },
  { key: '6h', label: '6h', ms: 6 * 60 * 60 * 1000 },
  { key: '24h', label: '24h', ms: 24 * 60 * 60 * 1000 },
  { key: '7d', label: '7d', ms: 7 * 24 * 60 * 60 * 1000 },
] as const

function SystemPanel() {
  const [windowKey, setWindowKey] = useState<string>('1h')
  const range = useMemo(() => {
    const cfg = WINDOWS.find((w) => w.key === windowKey) ?? WINDOWS[0]
    const to = new Date()
    const from = new Date(to.getTime() - cfg.ms)
    return {
      from: from.toISOString(),
      to: to.toISOString(),
    }
  }, [windowKey])

  const sys = useSystemStats(range.from, range.to)
  const samples = sys.data?.samples ?? []
  const latest = samples[samples.length - 1]

  return (
    <div className='space-y-4'>
      <div className='flex items-center gap-2'>
        <span className='text-muted-foreground text-sm'>Window:</span>
        {WINDOWS.map((w) => (
          <Button
            key={w.key}
            size='sm'
            variant={windowKey === w.key ? 'default' : 'outline'}
            onClick={() => setWindowKey(w.key)}
          >
            {w.label}
          </Button>
        ))}
      </div>

      {sys.isLoading ? (
        <Skeleton className='h-64 w-full' />
      ) : samples.length === 0 ? (
        <Empty msg='No system samples in this window yet — wait ~30s after restart for the SystemSampler to fill rows.' />
      ) : (
        <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
          <ChartCell label='CPU %'>
            <EChart
              option={{
                xAxis: { type: 'time' },
                yAxis: { type: 'value', name: '%' },
                series: [
                  {
                    type: 'line',
                    smooth: true,
                    data: samples.map((s) => [s.ts, round2(s.cpu_pct)]),
                    color: '#f97316',
                    markPoint: { data: [{ type: 'max', name: 'peak' }] },
                  },
                ],
              }}
            />
          </ChartCell>

          <ChartCell label='Load average'>
            <EChart
              option={{
                tooltip: { trigger: 'axis' },
                legend: { data: ['load_1m', 'load_5m', 'load_15m'] },
                xAxis: { type: 'time' },
                yAxis: { type: 'value' },
                series: [
                  {
                    name: 'load_1m',
                    type: 'line',
                    data: samples.map((s) => [s.ts, round2(s.load_1m)]),
                  },
                  {
                    name: 'load_5m',
                    type: 'line',
                    data: samples.map((s) => [s.ts, round2(s.load_5m)]),
                  },
                  {
                    name: 'load_15m',
                    type: 'line',
                    data: samples.map((s) => [s.ts, round2(s.load_15m)]),
                  },
                ],
              }}
            />
          </ChartCell>

          <ChartCell label='Memory used vs total (MB)'>
            <EChart
              option={{
                tooltip: { trigger: 'axis' },
                legend: { data: ['used', 'free'] },
                xAxis: { type: 'time' },
                yAxis: { type: 'value', name: 'MB' },
                series: [
                  {
                    name: 'used',
                    type: 'line',
                    stack: 'mem',
                    areaStyle: {},
                    data: samples.map((s) => [s.ts, round2(s.mem_used_mb)]),
                  },
                  {
                    name: 'free',
                    type: 'line',
                    stack: 'mem',
                    areaStyle: {},
                    data: samples.map((s) => [
                      s.ts,
                      round2(Math.max(s.mem_total_mb - s.mem_used_mb, 0)),
                    ]),
                  },
                ],
              }}
            />
          </ChartCell>

          <ChartCell label='Disk used vs total (GB)'>
            <EChart
              option={{
                tooltip: { trigger: 'axis' },
                legend: { data: ['used', 'free'] },
                xAxis: { type: 'time' },
                yAxis: { type: 'value', name: 'GB' },
                series: [
                  {
                    name: 'used',
                    type: 'line',
                    stack: 'disk',
                    areaStyle: {},
                    data: samples.map((s) => [s.ts, round2(s.disk_used_gb)]),
                  },
                  {
                    name: 'free',
                    type: 'line',
                    stack: 'disk',
                    areaStyle: {},
                    data: samples.map((s) => [
                      s.ts,
                      round2(Math.max(s.disk_total_gb - s.disk_used_gb, 0)),
                    ]),
                  },
                ],
              }}
            />
          </ChartCell>

          <ChartCell label='Network throughput (Mbps)'>
            <EChart
              option={{
                tooltip: { trigger: 'axis' },
                legend: { data: ['rx', 'tx'] },
                xAxis: { type: 'time' },
                yAxis: { type: 'value', name: 'Mbps' },
                series: [
                  {
                    name: 'rx',
                    type: 'line',
                    data: samples.map((s) => [s.ts, round2(s.net_rx_mbps)]),
                  },
                  {
                    name: 'tx',
                    type: 'line',
                    data: samples.map((s) => [s.ts, round2(s.net_tx_mbps)]),
                  },
                ],
              }}
            />
          </ChartCell>

          <ChartCell label='Disk I/O (MB/s)'>
            <EChart
              option={{
                tooltip: { trigger: 'axis' },
                legend: { data: ['read', 'write'] },
                xAxis: { type: 'time' },
                yAxis: { type: 'value', name: 'MB/s' },
                series: [
                  {
                    name: 'read',
                    type: 'line',
                    smooth: true,
                    data: samples.map((s) => [s.ts, round2(s.disk_read_mbps ?? 0)]),
                    color: '#06b6d4',
                  },
                  {
                    name: 'write',
                    type: 'line',
                    smooth: true,
                    data: samples.map((s) => [s.ts, round2(s.disk_write_mbps ?? 0)]),
                    color: '#f97316',
                  },
                ],
              }}
            />
          </ChartCell>

          <CurrentSnapshotCard latest={latest} />
        </div>
      )}
    </div>
  )
}

function CurrentSnapshotCard({ latest }: { latest?: SystemHostSample }) {
  if (!latest) return null
  const memPct =
    latest.mem_total_mb > 0
      ? (latest.mem_used_mb / latest.mem_total_mb) * 100
      : 0
  const diskPct =
    latest.disk_total_gb > 0
      ? (latest.disk_used_gb / latest.disk_total_gb) * 100
      : 0
  return (
    <div>
      <div className='text-muted-foreground mb-1 text-sm font-medium'>
        Current snapshot
      </div>
      <div className='grid grid-cols-3 gap-3'>
        <Stat label='cpu %'>{round2(latest.cpu_pct)}</Stat>
        <Stat label='mem %'>{round2(memPct)}</Stat>
        <Stat label='disk %'>{round2(diskPct)}</Stat>
      </div>
    </div>
  )
}

// ── Helpers ───────────────────────────────────────────────────────────

function ChartCell({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className='border-border bg-card rounded-md border p-3'>
      <div className='text-muted-foreground mb-1 text-xs font-medium uppercase tracking-wide'>
        {label}
      </div>
      {children}
    </div>
  )
}

function round2(n: number): number {
  return Math.round(n * 100) / 100
}

function cacheHit(r: RunRow): number {
  const total =
    r.input_tokens + r.output_tokens + r.cache_read_tokens + r.cache_create_tokens
  if (total === 0) return 0
  return (r.cache_read_tokens / total) * 100
}

// Compact-number formatters for chart axis tick labels and data
// labels — keeps "22000000" from sprawling across the gutter.
function fmtTokens(n: number): string {
  if (!Number.isFinite(n)) return ''
  const abs = Math.abs(n)
  if (abs >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (abs >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return String(n)
}

function fmtCount(n: number): string {
  if (!Number.isFinite(n)) return ''
  const abs = Math.abs(n)
  if (abs >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (abs >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return String(Math.round(n))
}

function fmtTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function fmtDuration(ms: number): string {
  if (!ms) return '—'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rs = s % 60
  if (m < 60) return `${m}m ${rs}s`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}
