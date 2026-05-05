import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { EChart } from '@/components/echart'
import { cn } from '@/lib/utils'
import { ActivityChart } from '@/features/overview'

// ── Types ─────────────────────────────────────────────────────────────────

interface DaySystem { day: string; avg_cpu_pct: number; avg_net_rx_mbps: number; avg_net_tx_mbps: number; avg_disk_read_mbps: number; avg_disk_write_mbps: number }

interface StatsHistory {
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

// ── Query ─────────────────────────────────────────────────────────────────

const RANGES = [
  { label: '1d',   days: 1 },
  { label: '2d',   days: 2 },
  { label: '3d',   days: 3 },
  { label: '1w',   days: 7 },
  { label: '2w',   days: 14 },
  { label: '1m',   days: 30 },
  { label: '3m',   days: 90 },
  { label: 'All',  days: 0 },
] as const

type RangeDays = typeof RANGES[number]['days']

function useStatsHistory(days: RangeDays) {
  const url = days > 0 ? `/api/stats/history?days=${days}` : '/api/stats/history'
  return useQuery({
    queryKey: ['stats', 'history', days],
    queryFn: () => fetch(url).then(r => r.json()) as Promise<StatsHistory>,
    staleTime: 30_000,
  })
}

interface GitDay { day: string; lines_added: number; lines_removed: number; files_changed: number; commits: number }
interface GitHistory { total_commits: number; total_added: number; total_removed: number; total_files: number; hourly_buckets: { hour: string; lines_added: number; lines_removed: number; files_changed: number; commits: number }[] }

function useGitHistory(days: RangeDays) {
  const param = days === 0 ? 'all=1' : `days=${days}`
  return useQuery({
    queryKey: ['stats', 'git', days],
    queryFn: async () => {
      const d = await fetch(`/api/stats/git?${param}`).then(r => r.json()) as GitHistory
      // Roll hourly buckets up to daily
      const byDay = new Map<string, GitDay>()
      for (const b of (d.hourly_buckets ?? [])) {
        const day = b.hour.slice(0, 10)
        const ex = byDay.get(day) ?? { day, lines_added: 0, lines_removed: 0, files_changed: 0, commits: 0 }
        ex.lines_added   += b.lines_added
        ex.lines_removed += b.lines_removed
        ex.files_changed += b.files_changed
        ex.commits       += b.commits
        byDay.set(day, ex)
      }
      return { ...d, daily: [...byDay.values()].sort((a, b) => a.day < b.day ? -1 : 1) }
    },
    staleTime: 120_000,
  })
}

// Merge git daily data with execution_log productivity data.
function mergeProductivity(exec: DayProductivity[], git: GitDay[]): DayProductivity[] {
  const map = new Map<string, DayProductivity>()
  for (const b of exec) map.set(b.day, { ...b })
  for (const g of git) {
    const ex = map.get(g.day)
    if (ex) {
      ex.lines_added   = Math.max(ex.lines_added,   g.lines_added)
      ex.lines_removed = Math.max(ex.lines_removed, g.lines_removed)
      ex.files_changed = Math.max(ex.files_changed, g.files_changed)
      ex.commits       = Math.max(ex.commits,       g.commits)
    } else {
      map.set(g.day, { day: g.day, lines_added: g.lines_added, lines_removed: g.lines_removed, files_changed: g.files_changed, commits: g.commits, tests_run: 0, tests_passed: 0, tests_failed: 0, prs_opened: 0 })
    }
  }
  return [...map.values()].sort((a, b) => a.day < b.day ? -1 : 1)
}

// ── Helpers ───────────────────────────────────────────────────────────────

// Pad sparse daily arrays to a continuous range so charts don't have gaps.
function padDays<T extends { day: string }>(data: T[], empty: (day: string) => T): T[] {
  if (!data.length) return data
  const map = new Map(data.map(d => [d.day, d]))
  const start = new Date(data[0].day + 'T00:00:00Z')
  const end   = new Date(data[data.length - 1].day + 'T00:00:00Z')
  const out: T[] = []
  for (let d = new Date(start); d <= end; d.setUTCDate(d.getUTCDate() + 1)) {
    const key = d.toISOString().slice(0, 10)
    out.push(map.get(key) ?? empty(key))
  }
  return out
}

function fmt(n: number, dec = 0) {
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

// Chart with its own range selector — each chart is independently zoomable.
function ChartCard({ title, series }: {
  title: string
  series: (range: RangeDays) => React.ReactNode
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
      <CardContent className='pb-3'>{series(range)}</CardContent>
    </Card>
  )
}

function RangeBar({ range, onRange }: { range: RangeDays; onRange: (r: RangeDays) => void }) {
  return (
    <div className='flex items-center gap-2 mb-4'>
      <div className='inline-flex rounded-md border bg-card p-0.5 text-xs'>
        {RANGES.map(r => (
          <button key={r.days} type='button'
            onClick={() => onRange(r.days)}
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
  )
}

function Empty() {
  return <div className='h-[180px] flex items-center justify-center text-xs text-muted-foreground'>No data yet</div>
}

// ── Runs chart components ─────────────────────────────────────────────────

function RunsBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.runs.length) return <Empty />
  const runs = padDays(data.runs, d => ({ day: d, completed: 0, failed: 0, avg_dur_sec: 0, max_dur_sec: 0, avg_run_number: 0 }))
  const days = runs.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 32, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'completed', type: 'bar', stack: 'r', data: runs.map(d => d.completed), itemStyle: { color: '#10b981' } },
        { name: 'failed',    type: 'bar', stack: 'r', data: runs.map(d => d.failed),    itemStyle: { color: '#f43f5e' } },
      ],
    }} />
  )
}

function DurationLineChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.runs.length) return <Empty />
  const runs = padDays(data.runs, d => ({ day: d, completed: 0, failed: 0, avg_dur_sec: 0, max_dur_sec: 0, avg_run_number: 0 }))
  const days = runs.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9, formatter: '{value}s' } },
      series: [{ type: 'line', data: runs.map(d => +d.avg_dur_sec.toFixed(1)), smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: '#a78bfa', width: 2 }, areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1, colorStops: [{ offset:0, color:'#a78bfa44' },{ offset:1, color:'#a78bfa00' }] } } }],
    }} />
  )
}

function TerminalReasonsChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.terminal_reasons.length) return <Empty />
  return (
    <EChart height={180} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      series: [{ type: 'pie', radius: ['40%', '65%'], center: ['50%', '42%'], data: data.terminal_reasons.map(d => ({ name: d.label || 'success', value: d.count, itemStyle: { color: d.label === 'success' || d.label === '' ? '#10b981' : d.label === 'max_turns' ? '#f59e0b' : '#f43f5e' } })), label: { show: false } }],
    }} />
  )
}

function RetriesBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.runs.length) return <Empty />
  const runs = padDays(data.runs, d => ({ day: d, completed: 0, failed: 0, avg_dur_sec: 0, max_dur_sec: 0, avg_run_number: 0 }))
  const days = runs.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 32, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [{ type: 'bar', data: runs.map(d => +d.avg_run_number.toFixed(1)), itemStyle: { color: '#6366f1' } }],
    }} />
  )
}

// ── Efficiency chart components ───────────────────────────────────────────

function TurnsLineChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.efficiency.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  const days = eff.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 36, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [{ type: 'line', data: eff.map(d => +d.avg_turns.toFixed(1)), smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: '#a78bfa', width: 2 }, areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1, colorStops: [{ offset:0, color:'#a78bfa44' },{ offset:1, color:'#a78bfa00' }] } } }],
    }} />
  )
}

function CacheHitLineChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.efficiency.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  const days = eff.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 36, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `${p[0]?.value}%` },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', min: 0, max: 100, axisLabel: { fontSize: 9, formatter: '{value}%' } },
      series: [{ type: 'line', data: eff.map(d => +d.avg_cache_hit_pct.toFixed(1)), smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: '#10b981', width: 2 }, areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1, colorStops: [{ offset:0, color:'#10b98155' },{ offset:1, color:'#10b98100' }] } } }],
    }} />
  )
}

function ContextPctLineChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.efficiency.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  const days = eff.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 36, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', min: 0, max: 100, axisLabel: { fontSize: 9, formatter: '{value}%' } },
      series: [{ type: 'line', data: eff.map(d => +d.avg_context_pct.toFixed(1)), smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: '#f59e0b', width: 2 }, areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1, colorStops: [{ offset:0, color:'#f59e0b44' },{ offset:1, color:'#f59e0b00' }] } } }],
    }} />
  )
}

function TokensPerTurnLineChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.efficiency.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  const days = eff.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [{ type: 'line', data: eff.map(d => +d.avg_tokens_per_turn.toFixed(0)), smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: '#38bdf8', width: 2 } }],
    }} />
  )
}

function ActionsPerTurnLineChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.efficiency.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  const days = eff.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 36, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [{ type: 'line', data: eff.map(d => +d.avg_actions_per_turn.toFixed(2)), smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: '#6366f1', width: 2 } }],
    }} />
  )
}

function CompactionsBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.efficiency.length) return <Empty />
  const eff = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  const days = eff.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 32, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [{ type: 'bar', data: eff.map(d => d.total_compactions), itemStyle: { color: '#f59e0b' } }],
    }} />
  )
}

// ── Tokens chart components ───────────────────────────────────────────────

function TokenStackedBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.tokens.length) return <Empty />
  const tok = padDays(data.tokens, d => ({ day: d, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0, reasoning_tokens: 0, tool_use_tokens: 0 }))
  const days = tok.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
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

function CacheRatioLineChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.tokens.length) return <Empty />
  const tok = padDays(data.tokens, d => ({ day: d, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0, reasoning_tokens: 0, tool_use_tokens: 0 }))
  const days = tok.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 36, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `${p[0]?.value?.toFixed(1)}%` },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', min: 0, max: 100, axisLabel: { fontSize: 9, formatter: '{value}%' } },
      series: [{ type: 'line', data: tok.map(d => { const t = d.cache_read_tokens + d.cache_create_tokens; return t > 0 ? +((d.cache_read_tokens/t)*100).toFixed(1) : 0 }), smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: '#10b981', width: 2 }, areaStyle: { color: { type:'linear',x:0,y:0,x2:0,y2:1, colorStops:[{offset:0,color:'#10b98155'},{offset:1,color:'#10b98100'}] } } }],
    }} />
  )
}

function OutputTokensBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.tokens.length) return <Empty />
  const tok = padDays(data.tokens, d => ({ day: d, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0, reasoning_tokens: 0, tool_use_tokens: 0 }))
  const days = tok.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9, formatter: (v:number) => v>=1e3?(v/1e3).toFixed(0)+'k':String(v) } },
      series: [{ type: 'bar', data: tok.map(d => d.output_tokens), itemStyle: { color: '#a78bfa' } }],
    }} />
  )
}

function ReasoningTokensBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.tokens.length) return <Empty />
  const tok = padDays(data.tokens, d => ({ day: d, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0, reasoning_tokens: 0, tool_use_tokens: 0 }))
  const days = tok.map(d => d.day.slice(5))
  if (!tok.some(d => d.reasoning_tokens > 0)) {
    return <div className='h-[180px] flex items-center justify-center text-xs text-muted-foreground'>No reasoning tokens yet (Opus extended thinking)</div>
  }
  return (
    <EChart height={180} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [{ type: 'bar', data: tok.map(d => d.reasoning_tokens), itemStyle: { color: '#f43f5e' } }],
    }} />
  )
}

// ── Productivity chart components ─────────────────────────────────────────

function LinesBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  const git = useGitHistory(range)
  if (!data) return <Empty />
  const merged = mergeProductivity(data.productivity, git.data?.daily ?? [])
  const prod = padDays(merged.length ? merged : data.productivity, d => ({ day: d, lines_added: 0, lines_removed: 0, files_changed: 0, commits: 0, tests_run: 0, tests_passed: 0, tests_failed: 0, prs_opened: 0 }))
  const hasData = prod.some(d => d.lines_added > 0 || d.commits > 0 || d.tests_run > 0)
  if (!hasData) return <Empty />
  const days = prod.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [
        { name: 'added',   type: 'bar', data: prod.map(d => d.lines_added),    itemStyle: { color: '#10b981' }, stack: 'lines' },
        { name: 'removed', type: 'bar', data: prod.map(d => -d.lines_removed), itemStyle: { color: '#f43f5e' }, stack: 'lines' },
      ],
    }} />
  )
}

function CommitsFilesBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  const git = useGitHistory(range)
  if (!data) return <Empty />
  const merged = mergeProductivity(data.productivity, git.data?.daily ?? [])
  const prod = padDays(merged.length ? merged : data.productivity, d => ({ day: d, lines_added: 0, lines_removed: 0, files_changed: 0, commits: 0, tests_run: 0, tests_passed: 0, tests_failed: 0, prs_opened: 0 }))
  const hasData = prod.some(d => d.lines_added > 0 || d.commits > 0 || d.tests_run > 0)
  if (!hasData) return <Empty />
  const days = prod.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 32, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'commits', type: 'bar', data: prod.map(d => d.commits),       itemStyle: { color: '#6366f1' } },
        { name: 'files',   type: 'bar', data: prod.map(d => d.files_changed), itemStyle: { color: '#38bdf8' } },
      ],
    }} />
  )
}

function TestsBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  const git = useGitHistory(range)
  if (!data) return <Empty />
  const merged = mergeProductivity(data.productivity, git.data?.daily ?? [])
  const prod = padDays(merged.length ? merged : data.productivity, d => ({ day: d, lines_added: 0, lines_removed: 0, files_changed: 0, commits: 0, tests_run: 0, tests_passed: 0, tests_failed: 0, prs_opened: 0 }))
  const hasData = prod.some(d => d.lines_added > 0 || d.commits > 0 || d.tests_run > 0)
  if (!hasData) return <Empty />
  const days = prod.map(d => d.day.slice(5))
  return (
    <EChart height={180} option={{
      grid: { left: 32, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: \1, axisLabel: { fontSize: 9  }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'passed', type: 'bar', stack: 't', data: prod.map(d => d.tests_passed), itemStyle: { color: '#10b981' } },
        { name: 'failed', type: 'bar', stack: 't', data: prod.map(d => d.tests_failed), itemStyle: { color: '#f43f5e' } },
      ],
    }} />
  )
}

// ── Agents chart components ───────────────────────────────────────────────

function ModelsPieChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.models.length) return <Empty />
  const modelColors: Record<string, string> = {
    'claude-opus-4-7': '#f43f5e', 'claude-sonnet-4-6': '#a78bfa',
    'claude-haiku-4-5': '#38bdf8', 'unknown': '#71717a',
  }
  return (
    <EChart height={180} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      series: [{ type: 'pie', radius: ['40%','65%'], center: ['50%','42%'], data: data.models.map(d => ({ name: d.label, value: d.count, itemStyle: { color: modelColors[d.label] ?? '#71717a' } })), label: { show: false } }],
    }} />
  )
}

function AgentsPieChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.agents.length) return <Empty />
  return (
    <EChart height={180} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      series: [{ type: 'pie', radius: ['40%','65%'], center: ['50%','42%'], data: data.agents.map(d => ({ name: d.label, value: d.count })), label: { show: false } }],
    }} />
  )
}

function TriggerSplitPieChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(range)
  if (!data?.trigger_split.length) return <Empty />
  return (
    <EChart height={180} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      series: [{ type: 'pie', radius: ['40%','65%'], center: ['50%','42%'], data: data.trigger_split.map(d => ({ name: d.label, value: d.count, itemStyle: { color: d.label === 'interactive' ? '#38bdf8' : d.label === 'manual' ? '#10b981' : '#a78bfa' } })), label: { show: false } }],
    }} />
  )
}

// ── Tab: Runs ─────────────────────────────────────────────────────────────

function RunsTab({ data, range, onRange }: { data: StatsHistory; range: RangeDays; onRange: (r: RangeDays) => void }) {
  const t = data.totals
  return (
    <div className='space-y-4'>
      <div className='grid grid-cols-2 gap-3 md:grid-cols-5'>
        <Card><CardContent className='pt-4'><Kpi label='Total runs' value={String(t.total_runs + t.total_failed)} sub={`${t.total_failed} failed`} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Success rate' value={`${t.total_runs + t.total_failed > 0 ? ((t.total_runs / (t.total_runs + t.total_failed)) * 100).toFixed(0) : 0}%`} accent='text-emerald-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Avg duration' value={`${t.avg_dur_sec.toFixed(0)}s`} sub={`${(t.avg_dur_sec/60).toFixed(1)} min`} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Avg turns' value={t.avg_turns.toFixed(1)} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Errors' value={String(t.total_errors)} accent={t.total_errors > 0 ? 'text-rose-400' : undefined} /></CardContent></Card>
      </div>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
        <ChartCard title='Daily runs — completed vs failed' series={(r) => <RunsBarChart range={r} />} />
        <ChartCard title='Avg duration per day (seconds)' series={(r) => <DurationLineChart range={r} />} />
        <ChartCard title='Terminal reasons' series={(r) => <TerminalReasonsChart range={r} />} />
        <ChartCard title='Avg retry number (run_number) — higher = more retries' series={(r) => <RetriesBarChart range={r} />} />
      </div>
    </div>
  )
}

// ── Tab: Efficiency ───────────────────────────────────────────────────────

function EfficiencyTab({ data, range, onRange }: { data: StatsHistory; range: RangeDays; onRange: (r: RangeDays) => void }) {
  const t = data.totals
  return (
    <div className='space-y-4'>
      <div className='grid grid-cols-2 gap-3 md:grid-cols-5'>
        <Card><CardContent className='pt-4'><Kpi label='Avg turns/run' value={t.avg_turns.toFixed(1)} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Cache hit' value={`${t.avg_cache_hit_pct.toFixed(0)}%`} accent='text-emerald-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Avg context %' value={`${t.avg_context_pct.toFixed(0)}%`} accent={t.avg_context_pct > 80 ? 'text-amber-400' : undefined} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Compactions' value={String(t.total_compactions)} sub='context resets' accent={t.total_compactions > 0 ? 'text-amber-400' : undefined} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Total errors' value={String(t.total_errors)} accent={t.total_errors > 10 ? 'text-rose-400' : undefined} /></CardContent></Card>
      </div>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
        <ChartCard title='Avg turns per run' series={(r) => <TurnsLineChart range={r} />} />
        <ChartCard title='Cache hit rate %' series={(r) => <CacheHitLineChart range={r} />} />
        <ChartCard title='Avg context window used %' series={(r) => <ContextPctLineChart range={r} />} />
        <ChartCard title='Avg tokens per turn' series={(r) => <TokensPerTurnLineChart range={r} />} />
        <ChartCard title='Actions per turn (efficiency — higher = less deliberation)' series={(r) => <ActionsPerTurnLineChart range={r} />} />
        <ChartCard title='Compactions per day (context resets — 0 is best)' series={(r) => <CompactionsBarChart range={r} />} />
      </div>
    </div>
  )
}

// ── Tab: Tokens ───────────────────────────────────────────────────────────

function TokensTab({ data, range, onRange }: { data: StatsHistory; range: RangeDays; onRange: (r: RangeDays) => void }) {
  const t = data.totals
  const totalAll = t.total_input_tokens + t.total_output_tokens + t.total_cache_read_tokens + t.total_cache_create_tokens
  return (
    <div className='space-y-4'>
      <div className='grid grid-cols-2 gap-3 md:grid-cols-5'>
        <Card><CardContent className='pt-4'><Kpi label='Input tokens' value={fmt(t.total_input_tokens)} accent='text-sky-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Output tokens' value={fmt(t.total_output_tokens)} accent='text-violet-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Cache read' value={fmt(t.total_cache_read_tokens)} sub='reused' accent='text-emerald-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Cache write' value={fmt(t.total_cache_create_tokens)} sub='written' accent='text-amber-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Total tokens' value={fmt(totalAll)} /></CardContent></Card>
      </div>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
        <ChartCard title='Daily token volumes (stacked)' series={(r) => <TokenStackedBarChart range={r} />} />
        <ChartCard title='Cache read/write ratio (higher = better cache compounding)' series={(r) => <CacheRatioLineChart range={r} />} />
        <ChartCard title='Output tokens per day' series={(r) => <OutputTokensBarChart range={r} />} />
        <ChartCard title='Reasoning tokens per day (Opus extended thinking)' series={(r) => <ReasoningTokensBarChart range={r} />} />
      </div>
    </div>
  )
}

// ── Tab: Productivity ─────────────────────────────────────────────────────

function ProductivityTab({ data, range, onRange }: { data: StatsHistory; range: RangeDays; onRange: (r: RangeDays) => void }) {
  const git = useGitHistory(range)
  const t = data.totals
  const g = git.data
  // Use git totals when execution_log totals are zero (historical data)
  const linesAdded   = t.total_lines_added   || (g?.total_added   ?? 0)
  const linesRemoved = t.total_lines_removed || (g?.total_removed ?? 0)
  const filesChanged = t.total_files_changed || (g?.total_files   ?? 0)
  const commits      = t.total_commits       || (g?.total_commits ?? 0)
  return (
    <div className='space-y-4'>
      <div className='grid grid-cols-2 gap-3 md:grid-cols-4 lg:grid-cols-8'>
        <Card><CardContent className='pt-4'><Kpi label='Lines added'   value={fmt(linesAdded)}   accent='text-emerald-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Lines removed' value={fmt(linesRemoved)} accent='text-rose-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Files changed' value={fmt(filesChanged)} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Commits'       value={String(commits)} accent='text-blue-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='PRs opened'    value={String(t.total_lines_added > 0 ? t.total_commits : 0)} accent='text-violet-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Tests run'     value={String(t.total_tests_run)} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Tests passed'  value={String(t.total_tests_passed)} accent='text-emerald-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Tests failed'  value={String(t.total_tests_failed)} accent={t.total_tests_failed > 0 ? 'text-rose-400' : undefined} /></CardContent></Card>
      </div>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
        <ChartCard title='Lines added / removed per day' series={(r) => <LinesBarChart range={r} />} />
        <ChartCard title='Commits + files changed per day' series={(r) => <CommitsFilesBarChart range={r} />} />
        <ChartCard title='Tests run / passed / failed per day' series={(r) => <TestsBarChart range={r} />} />
      </div>
    </div>
  )
}

// ── Tab: Agents ───────────────────────────────────────────────────────────

function AgentsTab({ data, range, onRange }: { data: StatsHistory; range: RangeDays; onRange: (r: RangeDays) => void }) {
  return (
    <div className='space-y-4'>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3'>
        <ChartCard title='Runs by model' series={(r) => <ModelsPieChart range={r} />} />
        <ChartCard title='Runs by agent runtime' series={(r) => <AgentsPieChart range={r} />} />
        <ChartCard title='Interactive vs autonomous' series={(r) => <TriggerSplitPieChart range={r} />} />
      </div>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────

// ── Activity overview chart (daily, same series as Overview hourly) ───────────


export function StatsPage() {
  const [range, setRange] = useState<RangeDays>(7)
  const { data, isLoading } = useStatsHistory(range)

  return (
    <>
      <Header />
      <Main>
        {/* Page header with range selector — one selector controls all tabs */}
        <div className='mb-4 flex items-center justify-between'>
          <h1 className='text-2xl font-bold tracking-tight'>Stats</h1>
          <div className='flex items-center gap-3'>
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
            <span className='text-muted-foreground text-xs'>execution_log</span>
          </div>
        </div>

        <div className='mb-4'>
          <ActivityChart defaultRange={7} />
        </div>

        {isLoading ? (
          <div className='text-muted-foreground text-sm'>Loading…</div>
        ) : data ? (
          <Tabs defaultValue='runs'>
            <TabsList>
              <TabsTrigger value='runs'>Runs</TabsTrigger>
              <TabsTrigger value='efficiency'>Efficiency</TabsTrigger>
              <TabsTrigger value='tokens'>Tokens</TabsTrigger>
              <TabsTrigger value='productivity'>Productivity</TabsTrigger>
              <TabsTrigger value='agents'>Agents</TabsTrigger>
            </TabsList>
            <TabsContent value='runs'         className='mt-4'><RunsTab         data={data} range={range} onRange={setRange} /></TabsContent>
            <TabsContent value='efficiency'   className='mt-4'><EfficiencyTab   data={data} range={range} onRange={setRange} /></TabsContent>
            <TabsContent value='tokens'       className='mt-4'><TokensTab       data={data} range={range} onRange={setRange} /></TabsContent>
            <TabsContent value='productivity' className='mt-4'><ProductivityTab data={data} range={range} onRange={setRange} /></TabsContent>
            <TabsContent value='agents'       className='mt-4'><AgentsTab       data={data} range={range} onRange={setRange} /></TabsContent>
          </Tabs>
        ) : null}
      </Main>
    </>
  )
}
