import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { EChart } from '@/components/echart'
import { LineChart, toMs } from '@/components/charts'
import { cn } from '@/lib/utils'
import type { EntityKind } from '@/lib/queries/entity'

// Option 1: mirrors the global /stats page UX 1:1 — per-chart range
// selectors (ChartCard pattern), single ActivityChart at top driven by
// the entity-scoped efficiency series, identical KPI cards, identical
// inner-tab layout. Only difference vs /stats: data source is
// /api/entities/{id}/stats (entity-scoped) instead of /api/stats/history.

// ── Types ─────────────────────────────────────────────────────────────────

interface DayRun { day: string; completed: number; failed: number; avg_dur_sec: number; max_dur_sec: number; avg_run_number: number }
interface DayEfficiency { day: string; avg_turns: number; avg_actions: number; avg_actions_per_turn: number; avg_context_pct: number; avg_tokens_per_turn: number; avg_cache_hit_pct: number; total_compactions: number; total_errors: number; avg_ttfb_ms: number }
interface DayTokens { day: string; input_tokens: number; output_tokens: number; cache_read_tokens: number; cache_create_tokens: number; reasoning_tokens: number; tool_use_tokens: number }
interface DayProductivity { day: string; lines_added: number; lines_removed: number; files_changed: number; commits: number; tests_run: number; tests_passed: number; tests_failed: number; prs_opened: number }
interface LabelCount { label: string; count: number }
interface Totals {
  total_runs: number; total_failed: number; total_turns: number; total_actions: number
  total_compactions: number; total_errors: number
  total_input_tokens: number; total_output_tokens: number
  total_cache_read_tokens: number; total_cache_create_tokens: number
  total_lines_added: number; total_lines_removed: number
  total_files_changed: number; total_commits: number
  total_tests_run: number; total_tests_passed: number; total_tests_failed: number
  avg_cache_hit_pct: number; avg_turns: number; avg_dur_sec: number; avg_context_pct: number
}
interface EntityStatsHistory {
  runs: DayRun[]
  efficiency: DayEfficiency[]
  tokens: DayTokens[]
  productivity: DayProductivity[]
  models: LabelCount[]
  agents: LabelCount[]
  terminal_reasons: LabelCount[]
  trigger_split: LabelCount[]
  totals: Totals
}

// ── Range selector (identical to /stats RANGES) ────────────────────────────

const RANGES = [
  { label: '1d',  days: 1 },
  { label: '2d',  days: 2 },
  { label: '3d',  days: 3 },
  { label: '1w',  days: 7 },
  { label: '2w',  days: 14 },
  { label: '1m',  days: 30 },
  { label: '3m',  days: 90 },
  { label: 'All', days: 0 },
] as const
type RangeDays = typeof RANGES[number]['days']

const dayMs = (day: string) => toMs(day)

function useEntityStatsHistory(entityId: string, days: RangeDays) {
  const url = days > 0
    ? `/api/entities/${entityId}/stats?days=${days}`
    : `/api/entities/${entityId}/stats`
  return useQuery({
    queryKey: ['entity-stats', entityId, days],
    queryFn: () => fetch(url).then(r => r.json()) as Promise<EntityStatsHistory>,
    enabled: !!entityId,
    staleTime: 30_000,
  })
}

// ── Helpers ───────────────────────────────────────────────────────────────

function padDays<T extends { day: string }>(data: T[], empty: (day: string) => T): T[] {
  if (!data.length) return data
  const map = new Map(data.map(d => [d.day, d]))
  const start = new Date(data[0].day + 'T00:00:00Z')
  const end = new Date(data[data.length - 1].day + 'T00:00:00Z')
  const out: T[] = []
  for (let d = new Date(start); d <= end; d.setUTCDate(d.getUTCDate() + 1)) {
    const key = d.toISOString().slice(0, 10)
    out.push(map.get(key) ?? empty(key))
  }
  return out
}

function fmt(n: number, dec = 0) {
  if (!n) return '0'
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return n.toFixed(dec)
}

function Kpi({ label, value, sub, accent }: { label: string; value: string; sub?: string; accent?: string }) {
  return (
    <div className='space-y-0.5'>
      <div className='text-muted-foreground text-[10px] uppercase tracking-wider'>{label}</div>
      <div className={cn('font-mono text-xl font-semibold tabular-nums', accent)}>{value}</div>
      {sub && <div className='text-muted-foreground text-[10px]'>{sub}</div>}
    </div>
  )
}

function Empty() {
  return <div className='h-[180px] flex items-center justify-center text-xs text-muted-foreground'>No data yet</div>
}

// ── ChartCard — copy of /stats ChartCard, per-chart range selector ─────────

function ChartCard({ entityId, title, series }: {
  entityId: string
  title: string
  series: (entityId: string, range: RangeDays) => React.ReactNode
}) {
  const [range, setRange] = useState<RangeDays>(7)
  return (
    <Card>
      <CardHeader className='pb-1 pt-3'>
        <div className='flex items-center justify-between gap-2'>
          <CardTitle className='text-xs text-muted-foreground uppercase tracking-wider truncate'>{title}</CardTitle>
          <div className='inline-flex shrink-0 rounded border bg-muted/20 p-px text-[9px]'>
            {RANGES.map(r => (
              <button key={r.days} type='button'
                onClick={() => setRange(r.days)}
                className={cn('rounded px-1.5 py-0.5 transition-colors',
                  range === r.days
                    ? 'bg-primary/20 text-foreground font-semibold'
                    : 'text-muted-foreground hover:text-foreground'
                )}>
                {r.label}
              </button>
            ))}
          </div>
        </div>
      </CardHeader>
      <CardContent className='pb-3'>{series(entityId, range)}</CardContent>
    </Card>
  )
}

// ── Daily activity (entity-scoped) — replaces global ActivityChart ─────────

function EntityActivityChart({ entityId }: { entityId: string }) {
  const [range, setRange] = useState<RangeDays>(7)
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.efficiency?.length) {
    return (
      <Card>
        <CardHeader className='pb-1 pt-3'>
          <CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Daily activity (entity-scoped)</CardTitle>
        </CardHeader>
        <CardContent className='pb-3'><Empty /></CardContent>
      </Card>
    )
  }
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  const labels = eff.map(d => d.day.slice(5))
  return (
    <Card>
      <CardHeader className='pb-1 pt-3'>
        <div className='flex items-center justify-between gap-2'>
          <CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Daily activity</CardTitle>
          <div className='inline-flex shrink-0 rounded border bg-muted/20 p-px text-[9px]'>
            {RANGES.map(r => (
              <button key={r.days} type='button'
                onClick={() => setRange(r.days)}
                className={cn('rounded px-1.5 py-0.5 transition-colors',
                  range === r.days
                    ? 'bg-primary/20 text-foreground font-semibold'
                    : 'text-muted-foreground hover:text-foreground'
                )}>
                {r.label}
              </button>
            ))}
          </div>
        </div>
      </CardHeader>
      <CardContent className='pb-3'>
        <EChart height={220} option={{
          grid: { left: 44, right: 44, top: 12, bottom: 40 },
          tooltip: { trigger: 'axis', confine: true },
          legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9 } },
          xAxis: { type: 'category', data: labels, axisLabel: { fontSize: 9 }, boundaryGap: false },
          yAxis: { type: 'value', axisLabel: { fontSize: 9 }, splitLine: { lineStyle: { opacity: 0.15 } } },
          series: [
            { name: 'avg turns',   type: 'line', smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: '#a78bfa', width: 2 }, data: eff.map(d => +d.avg_turns.toFixed(1)) },
            { name: 'avg actions', type: 'line', smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: '#38bdf8', width: 2 }, data: eff.map(d => +d.avg_actions.toFixed(1)) },
          ],
        }} />
      </CardContent>
    </Card>
  )
}

// ── Individual chart components (each calls useEntityStatsHistory) ─────────

function RunsBarChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.runs?.length) return <Empty />
  const runs = padDays(data.runs, d => ({ day: d, completed: 0, failed: 0, avg_dur_sec: 0, max_dur_sec: 0, avg_run_number: 0 }))
  const days = runs.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 32, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'completed', type: 'bar', stack: 'r', data: runs.map(d => d.completed), itemStyle: { color: '#10b981' } },
        { name: 'failed',    type: 'bar', stack: 'r', data: runs.map(d => d.failed),    itemStyle: { color: '#f43f5e' } },
      ],
    }} />
  )
}

function DurationLineChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.runs?.length) return <Empty />
  const runs = padDays(data.runs, d => ({ day: d, completed: 0, failed: 0, avg_dur_sec: 0, max_dur_sec: 0, avg_run_number: 0 }))
  return <LineChart series={[{ name: 'avg dur', data: runs.map(d => [dayMs(d.day), +d.avg_dur_sec.toFixed(1)] as [number, number]), color: '#a78bfa', area: true }]} yLeft={{ unit: 's' }} />
}

function TerminalReasonsChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.terminal_reasons?.length) return <Empty />
  return (
    <EChart height={180} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      series: [{
        type: 'pie', radius: ['40%', '65%'], center: ['50%', '42%'],
        data: data.terminal_reasons.map(d => ({
          name: d.label || 'success',
          value: d.count,
          itemStyle: { color: d.label === 'success' || d.label === '' ? '#10b981' : d.label === 'max_turns' ? '#f59e0b' : '#f43f5e' },
        })),
        label: { show: false },
      }],
    }} />
  )
}

function RetriesBarChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.runs?.length) return <Empty />
  const runs = padDays(data.runs, d => ({ day: d, completed: 0, failed: 0, avg_dur_sec: 0, max_dur_sec: 0, avg_run_number: 0 }))
  const days = runs.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 32, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [{ type: 'bar', data: runs.map(d => +d.avg_run_number.toFixed(1)), itemStyle: { color: '#6366f1' } }],
    }} />
  )
}

function TurnsLineChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  return <LineChart series={[{ name: 'avg turns', data: eff.map(d => [dayMs(d.day), +d.avg_turns.toFixed(1)] as [number, number]), color: '#a78bfa', area: true }]} />
}

function CacheHitLineChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  return <LineChart series={[{ name: 'cache hit', data: eff.map(d => [dayMs(d.day), +d.avg_cache_hit_pct.toFixed(1)] as [number, number]), color: '#10b981', area: true }]} yLeft={{ min: 0, max: 100, unit: '%' }} />
}

function ContextPctLineChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  return <LineChart series={[{ name: 'ctx window', data: eff.map(d => [dayMs(d.day), +d.avg_context_pct.toFixed(1)] as [number, number]), color: '#f59e0b', area: true }]} yLeft={{ min: 0, max: 100, unit: '%' }} />
}

function TokensPerTurnLineChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  return <LineChart series={[{ name: 'tok/turn', data: eff.map(d => [dayMs(d.day), +d.avg_tokens_per_turn.toFixed(0)] as [number, number]), color: '#38bdf8' }]} />
}

function ActionsPerTurnLineChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  return <LineChart series={[{ name: 'actions/turn', data: eff.map(d => [dayMs(d.day), +d.avg_actions_per_turn.toFixed(2)] as [number, number]), color: '#6366f1' }]} />
}

function CompactionsBarChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  const days = eff.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 32, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [{ type: 'bar', data: eff.map(d => d.total_compactions), itemStyle: { color: '#f59e0b' } }],
    }} />
  )
}

function TokenStackedBarChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.tokens?.length) return <Empty />
  const tok = padDays(data.tokens, d => ({ day: d, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0, reasoning_tokens: 0, tool_use_tokens: 0 }))
  const days = tok.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9, formatter: (v: number) => v >= 1e6 ? (v/1e6).toFixed(1)+'M' : v >= 1e3 ? (v/1e3).toFixed(0)+'k' : String(v) } },
      series: [
        { name: 'cache read',  type: 'bar', stack: 'tok', data: tok.map(d => d.cache_read_tokens),   itemStyle: { color: '#10b981' } },
        { name: 'input',       type: 'bar', stack: 'tok', data: tok.map(d => d.input_tokens),        itemStyle: { color: '#38bdf8' } },
        { name: 'output',      type: 'bar', stack: 'tok', data: tok.map(d => d.output_tokens),       itemStyle: { color: '#a78bfa' } },
        { name: 'cache write', type: 'bar', stack: 'tok', data: tok.map(d => d.cache_create_tokens), itemStyle: { color: '#f59e0b' } },
      ],
    }} />
  )
}

function CacheRatioLineChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.tokens?.length) return <Empty />
  const tok = padDays(data.tokens, d => ({ day: d, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0, reasoning_tokens: 0, tool_use_tokens: 0 }))
  return <LineChart
    series={[{ name: 'cache reuse', data: tok.map(d => {
      const total = d.cache_read_tokens + d.cache_create_tokens
      return [dayMs(d.day), total > 0 ? +((d.cache_read_tokens / total) * 100).toFixed(1) : 0] as [number, number]
    }), color: '#10b981', area: true }]}
    yLeft={{ min: 0, max: 100, unit: '%' }}
  />
}

function OutputTokensBarChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.tokens?.length) return <Empty />
  const tok = padDays(data.tokens, d => ({ day: d, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0, reasoning_tokens: 0, tool_use_tokens: 0 }))
  const days = tok.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9, formatter: (v: number) => v >= 1e3 ? (v/1e3).toFixed(0)+'k' : String(v) } },
      series: [{ type: 'bar', data: tok.map(d => d.output_tokens), itemStyle: { color: '#a78bfa' } }],
    }} />
  )
}

function ReasoningTokensBarChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.tokens?.length) return <Empty />
  const tok = padDays(data.tokens, d => ({ day: d, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0, reasoning_tokens: 0, tool_use_tokens: 0 }))
  const days = tok.map(d => d.day.slice(5))
  if (!tok.some(d => d.reasoning_tokens > 0)) {
    return <div className='h-[180px] flex items-center justify-center text-xs text-muted-foreground'>No reasoning tokens yet (Opus extended thinking)</div>
  }
  return (
    <EChart height={180} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [{ type: 'bar', data: tok.map(d => d.reasoning_tokens), itemStyle: { color: '#f43f5e' } }],
    }} />
  )
}

function LinesBarChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.productivity) return <Empty />
  const prod = padDays(data.productivity, d => ({ day: d, lines_added: 0, lines_removed: 0, files_changed: 0, commits: 0, tests_run: 0, tests_passed: 0, tests_failed: 0, prs_opened: 0 }))
  const hasData = prod.some(d => d.lines_added > 0 || d.commits > 0 || d.tests_run > 0)
  if (!hasData) return <Empty />
  const days = prod.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [
        { name: 'added',   type: 'bar', data: prod.map(d => d.lines_added),    itemStyle: { color: '#10b981' }, stack: 'lines' },
        { name: 'removed', type: 'bar', data: prod.map(d => -d.lines_removed), itemStyle: { color: '#f43f5e' }, stack: 'lines' },
      ],
    }} />
  )
}

function CommitsFilesBarChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.productivity) return <Empty />
  const prod = padDays(data.productivity, d => ({ day: d, lines_added: 0, lines_removed: 0, files_changed: 0, commits: 0, tests_run: 0, tests_passed: 0, tests_failed: 0, prs_opened: 0 }))
  const hasData = prod.some(d => d.lines_added > 0 || d.commits > 0 || d.tests_run > 0)
  if (!hasData) return <Empty />
  const days = prod.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 32, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'commits', type: 'bar', data: prod.map(d => d.commits),       itemStyle: { color: '#6366f1' } },
        { name: 'files',   type: 'bar', data: prod.map(d => d.files_changed), itemStyle: { color: '#38bdf8' } },
      ],
    }} />
  )
}

function TestsBarChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.productivity) return <Empty />
  const prod = padDays(data.productivity, d => ({ day: d, lines_added: 0, lines_removed: 0, files_changed: 0, commits: 0, tests_run: 0, tests_passed: 0, tests_failed: 0, prs_opened: 0 }))
  const hasData = prod.some(d => d.lines_added > 0 || d.commits > 0 || d.tests_run > 0)
  if (!hasData) return <Empty />
  const days = prod.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 32, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'passed', type: 'bar', stack: 't', data: prod.map(d => d.tests_passed), itemStyle: { color: '#10b981' } },
        { name: 'failed', type: 'bar', stack: 't', data: prod.map(d => d.tests_failed), itemStyle: { color: '#f43f5e' } },
      ],
    }} />
  )
}

function ModelsPieChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.models?.length) return <Empty />
  const modelColors: Record<string, string> = {
    'claude-opus-4-7': '#f43f5e', 'claude-sonnet-4-6': '#a78bfa',
    'claude-haiku-4-5': '#38bdf8', 'unknown': '#71717a',
  }
  return (
    <EChart height={180} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      series: [{ type: 'pie', radius: ['40%', '65%'], center: ['50%', '42%'], data: data.models.map(d => ({ name: d.label, value: d.count, itemStyle: { color: modelColors[d.label] ?? '#71717a' } })), label: { show: false } }],
    }} />
  )
}

function AgentsPieChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.agents?.length) return <Empty />
  return (
    <EChart height={180} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      series: [{ type: 'pie', radius: ['40%', '65%'], center: ['50%', '42%'], data: data.agents.map(d => ({ name: d.label, value: d.count })), label: { show: false } }],
    }} />
  )
}

function TriggerSplitPieChart({ entityId, range }: { entityId: string; range: RangeDays }) {
  const { data } = useEntityStatsHistory(entityId, range)
  if (!data?.trigger_split?.length) return <Empty />
  return (
    <EChart height={180} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      series: [{ type: 'pie', radius: ['40%', '65%'], center: ['50%', '42%'], data: data.trigger_split.map(d => ({ name: d.label, value: d.count, itemStyle: { color: d.label === 'interactive' ? '#38bdf8' : d.label === 'manual' ? '#10b981' : '#a78bfa' } })), label: { show: false } }],
    }} />
  )
}

// ── KPI strip (uses page-level range) ──────────────────────────────────────

function KpiStrip({ data, tab }: { data: EntityStatsHistory; tab: 'runs' | 'efficiency' | 'tokens' | 'productivity' }) {
  const t = data.totals
  if (tab === 'runs') {
    const successPct = t.total_runs + t.total_failed > 0
      ? ((t.total_runs / (t.total_runs + t.total_failed)) * 100).toFixed(0) : '0'
    return (
      <div className='grid grid-cols-2 gap-3 md:grid-cols-5'>
        <Card><CardContent className='pt-4'><Kpi label='Total runs' value={String(t.total_runs + t.total_failed)} sub={`${t.total_failed} failed`} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Success rate' value={`${successPct}%`} accent='text-emerald-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Avg duration' value={`${t.avg_dur_sec.toFixed(0)}s`} sub={`${(t.avg_dur_sec/60).toFixed(1)} min`} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Avg turns' value={t.avg_turns.toFixed(1)} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Errors' value={String(t.total_errors)} accent={t.total_errors > 0 ? 'text-rose-400' : undefined} /></CardContent></Card>
      </div>
    )
  }
  if (tab === 'efficiency') {
    return (
      <div className='grid grid-cols-2 gap-3 md:grid-cols-5'>
        <Card><CardContent className='pt-4'><Kpi label='Avg turns/run' value={t.avg_turns.toFixed(1)} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Cache hit' value={`${t.avg_cache_hit_pct.toFixed(0)}%`} accent='text-emerald-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Avg context %' value={`${t.avg_context_pct.toFixed(0)}%`} accent={t.avg_context_pct > 80 ? 'text-amber-400' : undefined} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Compactions' value={String(t.total_compactions)} sub='context resets' accent={t.total_compactions > 0 ? 'text-amber-400' : undefined} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Total errors' value={String(t.total_errors)} accent={t.total_errors > 10 ? 'text-rose-400' : undefined} /></CardContent></Card>
      </div>
    )
  }
  if (tab === 'tokens') {
    const totalAll = t.total_input_tokens + t.total_output_tokens + t.total_cache_read_tokens + t.total_cache_create_tokens
    return (
      <div className='grid grid-cols-2 gap-3 md:grid-cols-5'>
        <Card><CardContent className='pt-4'><Kpi label='Input tokens'  value={fmt(t.total_input_tokens)}  accent='text-sky-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Output tokens' value={fmt(t.total_output_tokens)} accent='text-violet-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Cache read'    value={fmt(t.total_cache_read_tokens)} sub='reused' accent='text-emerald-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Cache write'   value={fmt(t.total_cache_create_tokens)} sub='written' accent='text-amber-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Total tokens'  value={fmt(totalAll)} /></CardContent></Card>
      </div>
    )
  }
  // productivity
  return (
    <div className='grid grid-cols-2 gap-3 md:grid-cols-4 lg:grid-cols-8'>
      <Card><CardContent className='pt-4'><Kpi label='Lines added'   value={fmt(t.total_lines_added)}   accent='text-emerald-400' /></CardContent></Card>
      <Card><CardContent className='pt-4'><Kpi label='Lines removed' value={fmt(t.total_lines_removed)} accent='text-rose-400' /></CardContent></Card>
      <Card><CardContent className='pt-4'><Kpi label='Files changed' value={fmt(t.total_files_changed)} /></CardContent></Card>
      <Card><CardContent className='pt-4'><Kpi label='Commits'       value={String(t.total_commits)} accent='text-blue-400' /></CardContent></Card>
      <Card><CardContent className='pt-4'><Kpi label='PRs opened'    value={String(data.productivity?.reduce((a, d) => a + d.prs_opened, 0) ?? 0)} accent='text-violet-400' /></CardContent></Card>
      <Card><CardContent className='pt-4'><Kpi label='Tests run'     value={String(t.total_tests_run)} /></CardContent></Card>
      <Card><CardContent className='pt-4'><Kpi label='Tests passed'  value={String(t.total_tests_passed)} accent='text-emerald-400' /></CardContent></Card>
      <Card><CardContent className='pt-4'><Kpi label='Tests failed'  value={String(t.total_tests_failed)} accent={t.total_tests_failed > 0 ? 'text-rose-400' : undefined} /></CardContent></Card>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────

export function StatsTab({ kind: _kind, entityId }: { kind: EntityKind; entityId: string }) {
  // Page-level range drives only the KPI strips. Each chart card has its own
  // range selector, identical to /stats. Matches /stats UX 1:1.
  const [range, setRange] = useState<RangeDays>(7)
  const { data, isLoading, isError, error } = useEntityStatsHistory(entityId, range)

  return (
    <div className='space-y-3'>
      <div className='flex items-center justify-between'>
        <div className='text-muted-foreground text-xs'>
          Scope: this entity's run set (descendants included for manifest / product). KPIs use the range here; each chart has its own range below.
        </div>
        <div className='inline-flex rounded-md border bg-card p-0.5 text-xs'>
          {RANGES.map(r => (
            <button key={r.days} type='button'
              onClick={() => setRange(r.days)}
              className={cn('rounded px-3 py-1 transition-colors',
                range === r.days
                  ? 'bg-primary/15 text-foreground font-semibold'
                  : 'text-muted-foreground hover:text-foreground'
              )}>
              {r.label}
            </button>
          ))}
        </div>
      </div>

      <EntityActivityChart entityId={entityId} />

      {isLoading ? (
        <div className='text-muted-foreground p-6 text-center text-sm'>Loading entity stats…</div>
      ) : isError ? (
        <div className='p-4 text-sm text-rose-400'>Failed to load: {String(error)}</div>
      ) : data ? (
        <Tabs defaultValue='runs'>
          <TabsList>
            <TabsTrigger value='runs'>Runs</TabsTrigger>
            <TabsTrigger value='efficiency'>Efficiency</TabsTrigger>
            <TabsTrigger value='tokens'>Tokens</TabsTrigger>
            <TabsTrigger value='productivity'>Productivity</TabsTrigger>
            <TabsTrigger value='agents'>Agents</TabsTrigger>
          </TabsList>

          <TabsContent value='runs' className='mt-4 space-y-4'>
            <KpiStrip data={data} tab='runs' />
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <ChartCard entityId={entityId} title='Daily runs — completed vs failed' series={(e, r) => <RunsBarChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Avg duration per day (seconds)' series={(e, r) => <DurationLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Terminal reasons' series={(e, r) => <TerminalReasonsChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Avg retry number (run_number) — higher = more retries' series={(e, r) => <RetriesBarChart entityId={e} range={r} />} />
            </div>
          </TabsContent>

          <TabsContent value='efficiency' className='mt-4 space-y-4'>
            <KpiStrip data={data} tab='efficiency' />
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <ChartCard entityId={entityId} title='Avg turns per run' series={(e, r) => <TurnsLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Cache hit rate %' series={(e, r) => <CacheHitLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Avg context window used %' series={(e, r) => <ContextPctLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Avg tokens per turn' series={(e, r) => <TokensPerTurnLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Actions per turn (higher = less deliberation)' series={(e, r) => <ActionsPerTurnLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Compactions per day (context resets — 0 is best)' series={(e, r) => <CompactionsBarChart entityId={e} range={r} />} />
            </div>
          </TabsContent>

          <TabsContent value='tokens' className='mt-4 space-y-4'>
            <KpiStrip data={data} tab='tokens' />
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <ChartCard entityId={entityId} title='Daily token volumes (stacked)' series={(e, r) => <TokenStackedBarChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Cache read/write ratio (higher = better cache compounding)' series={(e, r) => <CacheRatioLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Output tokens per day' series={(e, r) => <OutputTokensBarChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Reasoning tokens per day (extended thinking)' series={(e, r) => <ReasoningTokensBarChart entityId={e} range={r} />} />
            </div>
          </TabsContent>

          <TabsContent value='productivity' className='mt-4 space-y-4'>
            <KpiStrip data={data} tab='productivity' />
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <ChartCard entityId={entityId} title='Lines added / removed per day' series={(e, r) => <LinesBarChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Commits + files changed per day' series={(e, r) => <CommitsFilesBarChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Tests passed / failed per day' series={(e, r) => <TestsBarChart entityId={e} range={r} />} />
            </div>
          </TabsContent>

          <TabsContent value='agents' className='mt-4 space-y-4'>
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3'>
              <ChartCard entityId={entityId} title='Runs by model' series={(e, r) => <ModelsPieChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Runs by agent runtime' series={(e, r) => <AgentsPieChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Interactive vs autonomous' series={(e, r) => <TriggerSplitPieChart entityId={e} range={r} />} />
            </div>
          </TabsContent>
        </Tabs>
      ) : null}
    </div>
  )
}
