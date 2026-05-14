import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EChart } from '@/components/echart'
import { LineChart, toMs } from '@/components/charts'
import { cn } from '@/lib/utils'

// Shared stats panels — consumed by both the global /stats page and the
// per-entity Stats tab. Each chart component takes (entityId: string|null,
// range) and fetches via useStatsHistory which switches endpoint:
//   entityId=null → /api/stats/history
//   entityId=<uid> → /api/entities/<uid>/stats
//
// The /stats page-specific Productivity charts merge git history with
// execution_log to fill in older rows; those variants live inline in
// features/stats/index.tsx since git history is not entity-scopable. All
// other charts are shared here.

// ── Types (mirror handlers_stats_history.go) ───────────────────────────────

export interface DayRun { day: string; completed: number; failed: number; avg_dur_sec: number; max_dur_sec: number; avg_run_number: number }
export interface DayEfficiency { day: string; avg_turns: number; avg_actions: number; avg_actions_per_turn: number; avg_context_pct: number; avg_tokens_per_turn: number; avg_cache_hit_pct: number; total_compactions: number; total_errors: number; avg_ttfb_ms: number }
export interface DayTokens { day: string; input_tokens: number; output_tokens: number; cache_read_tokens: number; cache_create_tokens: number; reasoning_tokens: number; tool_use_tokens: number }
export interface DayProductivity { day: string; lines_added: number; lines_removed: number; files_changed: number; commits: number; tests_run: number; tests_passed: number; tests_failed: number; prs_opened: number }
export interface LabelCount { label: string; count: number }
export interface DaySystem { day: string; avg_cpu_pct: number; avg_net_rx_mbps: number; avg_net_tx_mbps: number; avg_disk_read_mbps: number; avg_disk_write_mbps: number }
export interface Totals {
  total_runs: number; total_failed: number; total_turns: number; total_actions: number
  total_compactions: number; total_errors: number
  total_input_tokens: number; total_output_tokens: number
  total_cache_read_tokens: number; total_cache_create_tokens: number
  total_lines_added: number; total_lines_removed: number
  total_files_changed: number; total_commits: number
  total_tests_run: number; total_tests_passed: number; total_tests_failed: number
  avg_cache_hit_pct: number; avg_turns: number; avg_dur_sec: number; avg_context_pct: number
}
export interface StatsHistory {
  runs: DayRun[]
  efficiency: DayEfficiency[]
  tokens: DayTokens[]
  productivity: DayProductivity[]
  models: LabelCount[]
  agents: LabelCount[]
  terminal_reasons: LabelCount[]
  trigger_split: LabelCount[]
  system_daily: DaySystem[]
  totals: Totals
}

// ── Range constants ────────────────────────────────────────────────────────

export const RANGES = [
  { label: '1d',  days: 1 },
  { label: '2d',  days: 2 },
  { label: '3d',  days: 3 },
  { label: '1w',  days: 7 },
  { label: '2w',  days: 14 },
  { label: '1m',  days: 30 },
  { label: '3m',  days: 90 },
  { label: 'All', days: 0 },
] as const

export type RangeDays = typeof RANGES[number]['days']

export const dayMs = (day: string) => toMs(day)

// ── Hook — switches endpoint based on entityId ────────────────────────────

export function useStatsHistory(entityId: string | null, days: RangeDays) {
  let url: string
  let key: (string | number)[]
  if (entityId) {
    url = days > 0 ? `/api/entities/${entityId}/stats?days=${days}` : `/api/entities/${entityId}/stats`
    key = ['entity-stats', entityId, days]
  } else {
    url = days > 0 ? `/api/stats/history?days=${days}` : `/api/stats/history`
    key = ['stats', 'history', days]
  }
  return useQuery({
    queryKey: key,
    queryFn: () => fetch(url).then(r => r.json()) as Promise<StatsHistory>,
    staleTime: 30_000,
  })
}

// ── Helpers ───────────────────────────────────────────────────────────────

export function padDays<T extends { day: string }>(data: T[], empty: (day: string) => T): T[] {
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

export function fmt(n: number, dec = 0) {
  if (!n) return '0'
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return n.toFixed(dec)
}

export function Kpi({ label, value, sub, accent }: { label: string; value: string; sub?: string; accent?: string }) {
  return (
    <div className='space-y-0.5'>
      <div className='text-muted-foreground text-[10px] uppercase tracking-wider'>{label}</div>
      <div className={cn('font-mono text-xl font-semibold tabular-nums', accent)}>{value}</div>
      {sub && <div className='text-muted-foreground text-[10px]'>{sub}</div>}
    </div>
  )
}

export function Empty() {
  return <div className='h-[180px] flex items-center justify-center text-xs text-muted-foreground'>No data yet</div>
}

export function ChartCard({ entityId, title, series }: {
  entityId: string | null
  title: string
  series: (entityId: string | null, range: RangeDays) => React.ReactNode
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

// ── KPI strips ────────────────────────────────────────────────────────────

export function RunsKpiStrip({ data }: { data: StatsHistory }) {
  const t = data.totals
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

export function EfficiencyKpiStrip({ data }: { data: StatsHistory }) {
  const t = data.totals
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

export function TokensKpiStrip({ data }: { data: StatsHistory }) {
  const t = data.totals
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

// ── Runs charts ───────────────────────────────────────────────────────────

export function RunsBarChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
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

export function DurationLineChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.runs?.length) return <Empty />
  const runs = padDays(data.runs, d => ({ day: d, completed: 0, failed: 0, avg_dur_sec: 0, max_dur_sec: 0, avg_run_number: 0 }))
  return <LineChart series={[{ name: 'avg dur', data: runs.map(d => [dayMs(d.day), +d.avg_dur_sec.toFixed(1)] as [number, number]), color: '#a78bfa', area: true }]} yLeft={{ unit: 's' }} />
}

export function TerminalReasonsChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
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

export function RetriesBarChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
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

// ── Efficiency charts ─────────────────────────────────────────────────────

const EMPTY_EFF = (d: string): DayEfficiency => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 })

export function TurnsLineChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, EMPTY_EFF)
  return <LineChart series={[{ name: 'avg turns', data: eff.map(d => [dayMs(d.day), +d.avg_turns.toFixed(1)] as [number, number]), color: '#a78bfa', area: true }]} />
}

export function CacheHitLineChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, EMPTY_EFF)
  return <LineChart series={[{ name: 'cache hit', data: eff.map(d => [dayMs(d.day), +d.avg_cache_hit_pct.toFixed(1)] as [number, number]), color: '#10b981', area: true }]} yLeft={{ min: 0, max: 100, unit: '%' }} />
}

export function ContextPctLineChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, EMPTY_EFF)
  return <LineChart series={[{ name: 'ctx window', data: eff.map(d => [dayMs(d.day), +d.avg_context_pct.toFixed(1)] as [number, number]), color: '#f59e0b', area: true }]} yLeft={{ min: 0, max: 100, unit: '%' }} />
}

export function TokensPerTurnLineChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, EMPTY_EFF)
  return <LineChart series={[{ name: 'tok/turn', data: eff.map(d => [dayMs(d.day), +d.avg_tokens_per_turn.toFixed(0)] as [number, number]), color: '#38bdf8' }]} />
}

export function ActionsPerTurnLineChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, EMPTY_EFF)
  return <LineChart series={[{ name: 'actions/turn', data: eff.map(d => [dayMs(d.day), +d.avg_actions_per_turn.toFixed(2)] as [number, number]), color: '#6366f1' }]} />
}

export function CompactionsBarChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.efficiency?.length) return <Empty />
  const eff = padDays(data.efficiency, EMPTY_EFF)
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

// ── Token charts ──────────────────────────────────────────────────────────

const EMPTY_TOK = (d: string): DayTokens => ({ day: d, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0, reasoning_tokens: 0, tool_use_tokens: 0 })

export function TokenStackedBarChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.tokens?.length) return <Empty />
  const tok = padDays(data.tokens, EMPTY_TOK)
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

export function CacheRatioLineChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.tokens?.length) return <Empty />
  const tok = padDays(data.tokens, EMPTY_TOK)
  return <LineChart
    series={[{ name: 'cache reuse', data: tok.map(d => {
      const total = d.cache_read_tokens + d.cache_create_tokens
      return [dayMs(d.day), total > 0 ? +((d.cache_read_tokens / total) * 100).toFixed(1) : 0] as [number, number]
    }), color: '#10b981', area: true }]}
    yLeft={{ min: 0, max: 100, unit: '%' }}
  />
}

export function OutputTokensBarChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.tokens?.length) return <Empty />
  const tok = padDays(data.tokens, EMPTY_TOK)
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

export function ReasoningTokensBarChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.tokens?.length) return <Empty />
  const tok = padDays(data.tokens, EMPTY_TOK)
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

// ── Agents pies ────────────────────────────────────────────────────────────

export function ModelsPieChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
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

export function AgentsPieChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.agents?.length) return <Empty />
  return (
    <EChart height={180} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      series: [{ type: 'pie', radius: ['40%', '65%'], center: ['50%', '42%'], data: data.agents.map(d => ({ name: d.label, value: d.count })), label: { show: false } }],
    }} />
  )
}

export function TriggerSplitPieChart({ entityId, range }: { entityId: string | null; range: RangeDays }) {
  const { data } = useStatsHistory(entityId, range)
  if (!data?.trigger_split?.length) return <Empty />
  return (
    <EChart height={180} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      series: [{ type: 'pie', radius: ['40%', '65%'], center: ['50%', '42%'], data: data.trigger_split.map(d => ({ name: d.label, value: d.count, itemStyle: { color: d.label === 'interactive' ? '#38bdf8' : d.label === 'manual' ? '#10b981' : '#a78bfa' } })), label: { show: false } }],
    }} />
  )
}
