import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { EChart } from '@/components/echart'
import { cn } from '@/lib/utils'

// ── Types ─────────────────────────────────────────────────────────────────

interface StatsHistory {
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

function ChartCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Card>
      <CardHeader className='pb-1 pt-3'>
        <CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>{title}</CardTitle>
      </CardHeader>
      <CardContent className='pb-3'>{children}</CardContent>
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

// ── Tab: Runs ─────────────────────────────────────────────────────────────

function RunsTab({ data, range, onRange }: { data: StatsHistory; range: RangeDays; onRange: (r: RangeDays) => void }) {
  const runs = padDays(data.runs, d => ({ day: d, completed: 0, failed: 0, avg_dur_sec: 0, max_dur_sec: 0, avg_run_number: 0 }))
  const days = runs.map(d => d.day.slice(5)) // MM-DD
  const completed = runs.map(d => d.completed)
  const failed    = runs.map(d => d.failed)
  const duration  = runs.map(d => +d.avg_dur_sec.toFixed(1))
  const retries   = runs.map(d => +d.avg_run_number.toFixed(1))
  const t = data.totals

  return (
    <div className='space-y-4'>
      <RangeBar range={range} onRange={onRange} />
      <div className='grid grid-cols-2 gap-3 md:grid-cols-5'>
        <Card><CardContent className='pt-4'><Kpi label='Total runs' value={String(t.total_runs + t.total_failed)} sub={`${t.total_failed} failed`} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Success rate' value={`${t.total_runs + t.total_failed > 0 ? ((t.total_runs / (t.total_runs + t.total_failed)) * 100).toFixed(0) : 0}%`} accent='text-emerald-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Avg duration' value={`${t.avg_dur_sec.toFixed(0)}s`} sub={`${(t.avg_dur_sec/60).toFixed(1)} min`} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Avg turns' value={t.avg_turns.toFixed(1)} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Errors' value={String(t.total_errors)} accent={t.total_errors > 0 ? 'text-rose-400' : undefined} /></CardContent></Card>
      </div>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
        <ChartCard title='Daily runs — completed vs failed'>
          {data.runs.length ? <EChart height={180} option={{
            grid: { left: 32, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
            series: [
              { name: 'completed', type: 'bar', stack: 'r', data: completed, itemStyle: { color: '#10b981' } },
              { name: 'failed',    type: 'bar', stack: 'r', data: failed,    itemStyle: { color: '#f43f5e' } },
            ],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Avg duration per day (seconds)'>
          {data.runs.length ? <EChart height={180} option={{
            grid: { left: 40, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis' },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', axisLabel: { fontSize: 9, formatter: '{value}s' } },
            series: [{ type: 'line', data: duration, smooth: true, showSymbol: false, lineStyle: { color: '#a78bfa', width: 2 }, areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1, colorStops: [{ offset:0, color:'#a78bfa44' },{ offset:1, color:'#a78bfa00' }] } } }],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Terminal reasons'>
          {data.terminal_reasons.length ? <EChart height={180} option={{
            tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
            legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
            series: [{ type: 'pie', radius: ['40%', '65%'], center: ['50%', '42%'], data: data.terminal_reasons.map(d => ({ name: d.label || 'success', value: d.count, itemStyle: { color: d.label === 'success' || d.label === '' ? '#10b981' : d.label === 'max_turns' ? '#f59e0b' : '#f43f5e' } })), label: { show: false } }],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Avg retry number (run_number) — higher = more retries'>
          {data.runs.length ? <EChart height={180} option={{
            grid: { left: 32, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis' },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
            series: [{ type: 'bar', data: retries, itemStyle: { color: '#6366f1' } }],
          }} /> : <Empty />}
        </ChartCard>
      </div>
    </div>
  )
}

// ── Tab: Efficiency ───────────────────────────────────────────────────────

function EfficiencyTab({ data, range, onRange }: { data: StatsHistory; range: RangeDays; onRange: (r: RangeDays) => void }) {
  const eff  = padDays(data.efficiency, d => ({ day: d, avg_turns: 0, avg_actions: 0, avg_actions_per_turn: 0, avg_context_pct: 0, avg_tokens_per_turn: 0, avg_cache_hit_pct: 0, total_compactions: 0, total_errors: 0, avg_ttfb_ms: 0 }))
  const days = eff.map(d => d.day.slice(5))
  const t = data.totals
  return (
    <div className='space-y-4'>
      <RangeBar range={range} onRange={onRange} />
      <div className='grid grid-cols-2 gap-3 md:grid-cols-5'>
        <Card><CardContent className='pt-4'><Kpi label='Avg turns/run' value={t.avg_turns.toFixed(1)} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Cache hit' value={`${t.avg_cache_hit_pct.toFixed(0)}%`} accent='text-emerald-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Avg context %' value={`${t.avg_context_pct.toFixed(0)}%`} accent={t.avg_context_pct > 80 ? 'text-amber-400' : undefined} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Compactions' value={String(t.total_compactions)} sub='context resets' accent={t.total_compactions > 0 ? 'text-amber-400' : undefined} /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Total errors' value={String(t.total_errors)} accent={t.total_errors > 10 ? 'text-rose-400' : undefined} /></CardContent></Card>
      </div>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
        <ChartCard title='Avg turns per run'>
          {eff.length ? <EChart height={180} option={{
            grid: { left: 36, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis' },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
            series: [{ type: 'line', data: eff.map(d => +d.avg_turns.toFixed(1)), smooth: true, showSymbol: false, lineStyle: { color: '#a78bfa', width: 2 }, areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1, colorStops: [{ offset:0, color:'#a78bfa44' },{ offset:1, color:'#a78bfa00' }] } } }],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Cache hit rate %'>
          {eff.length ? <EChart height={180} option={{
            grid: { left: 36, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `${p[0]?.value}%` },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', min: 0, max: 100, axisLabel: { fontSize: 9, formatter: '{value}%' } },
            series: [{ type: 'line', data: eff.map(d => +d.avg_cache_hit_pct.toFixed(1)), smooth: true, showSymbol: false, lineStyle: { color: '#10b981', width: 2 }, areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1, colorStops: [{ offset:0, color:'#10b98155' },{ offset:1, color:'#10b98100' }] } } }],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Avg context window used %'>
          {eff.length ? <EChart height={180} option={{
            grid: { left: 36, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis' },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', min: 0, max: 100, axisLabel: { fontSize: 9, formatter: '{value}%' } },
            series: [{ type: 'line', data: eff.map(d => +d.avg_context_pct.toFixed(1)), smooth: true, showSymbol: false, lineStyle: { color: '#f59e0b', width: 2 }, areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1, colorStops: [{ offset:0, color:'#f59e0b44' },{ offset:1, color:'#f59e0b00' }] } } }],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Avg tokens per turn'>
          {eff.length ? <EChart height={180} option={{
            grid: { left: 40, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis' },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
            series: [{ type: 'line', data: eff.map(d => +d.avg_tokens_per_turn.toFixed(0)), smooth: true, showSymbol: false, lineStyle: { color: '#38bdf8', width: 2 } }],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Actions per turn (efficiency — higher = less deliberation)'>
          {eff.length ? <EChart height={180} option={{
            grid: { left: 36, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis' },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
            series: [{ type: 'line', data: eff.map(d => +d.avg_actions_per_turn.toFixed(2)), smooth: true, showSymbol: false, lineStyle: { color: '#6366f1', width: 2 } }],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Compactions per day (context resets — 0 is best)'>
          {eff.length ? <EChart height={180} option={{
            grid: { left: 32, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis' },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
            series: [{ type: 'bar', data: eff.map(d => d.total_compactions), itemStyle: { color: '#f59e0b' } }],
          }} /> : <Empty />}
        </ChartCard>
      </div>
    </div>
  )
}

// ── Tab: Tokens ───────────────────────────────────────────────────────────

function TokensTab({ data, range, onRange }: { data: StatsHistory; range: RangeDays; onRange: (r: RangeDays) => void }) {
  const tok  = padDays(data.tokens, d => ({ day: d, input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0, reasoning_tokens: 0, tool_use_tokens: 0 }))
  const days = tok.map(d => d.day.slice(5))
  const t = data.totals
  const totalAll = t.total_input_tokens + t.total_output_tokens + t.total_cache_read_tokens + t.total_cache_create_tokens
  return (
    <div className='space-y-4'>
      <RangeBar range={range} onRange={onRange} />
      <div className='grid grid-cols-2 gap-3 md:grid-cols-5'>
        <Card><CardContent className='pt-4'><Kpi label='Input tokens' value={fmt(t.total_input_tokens)} accent='text-sky-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Output tokens' value={fmt(t.total_output_tokens)} accent='text-violet-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Cache read' value={fmt(t.total_cache_read_tokens)} sub='reused' accent='text-emerald-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Cache write' value={fmt(t.total_cache_create_tokens)} sub='written' accent='text-amber-400' /></CardContent></Card>
        <Card><CardContent className='pt-4'><Kpi label='Total tokens' value={fmt(totalAll)} /></CardContent></Card>
      </div>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
        <ChartCard title='Daily token volumes (stacked)'>
          {tok.length ? <EChart height={180} option={{
            grid: { left: 40, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', axisLabel: { fontSize: 9, formatter: (v: number) => v >= 1e6 ? (v/1e6).toFixed(1)+'M' : v >= 1e3 ? (v/1e3).toFixed(0)+'k' : String(v) } },
            series: [
              { name: 'cache read',  type: 'bar', stack: 'tok', data: tok.map(d => d.cache_read_tokens),   itemStyle: { color: '#10b981' } },
              { name: 'input',       type: 'bar', stack: 'tok', data: tok.map(d => d.input_tokens),        itemStyle: { color: '#38bdf8' } },
              { name: 'output',      type: 'bar', stack: 'tok', data: tok.map(d => d.output_tokens),       itemStyle: { color: '#a78bfa' } },
              { name: 'cache write', type: 'bar', stack: 'tok', data: tok.map(d => d.cache_create_tokens), itemStyle: { color: '#f59e0b' } },
            ],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Cache read/write ratio (higher = better cache compounding)'>
          {tok.length ? <EChart height={180} option={{
            grid: { left: 36, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `${p[0]?.value?.toFixed(1)}%` },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', min: 0, max: 100, axisLabel: { fontSize: 9, formatter: '{value}%' } },
            series: [{ type: 'line', data: tok.map(d => { const t = d.cache_read_tokens + d.cache_create_tokens; return t > 0 ? +((d.cache_read_tokens/t)*100).toFixed(1) : 0 }), smooth: true, showSymbol: false, lineStyle: { color: '#10b981', width: 2 }, areaStyle: { color: { type:'linear',x:0,y:0,x2:0,y2:1, colorStops:[{offset:0,color:'#10b98155'},{offset:1,color:'#10b98100'}] } } }],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Output tokens per day'>
          {tok.length ? <EChart height={180} option={{
            grid: { left: 40, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis' },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', axisLabel: { fontSize: 9, formatter: (v:number) => v>=1e3?(v/1e3).toFixed(0)+'k':String(v) } },
            series: [{ type: 'bar', data: tok.map(d => d.output_tokens), itemStyle: { color: '#a78bfa' } }],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Reasoning tokens per day (Opus extended thinking)'>
          {tok.some(d => d.reasoning_tokens > 0) ? <EChart height={180} option={{
            grid: { left: 40, right: 8, top: 8, bottom: 24 },
            tooltip: { trigger: 'axis' },
            xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
            yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
            series: [{ type: 'bar', data: tok.map(d => d.reasoning_tokens), itemStyle: { color: '#f43f5e' } }],
          }} /> : <div className='h-[180px] flex items-center justify-center text-xs text-muted-foreground'>No reasoning tokens yet (Opus extended thinking)</div>}
        </ChartCard>
      </div>
    </div>
  )
}

// ── Tab: Productivity ─────────────────────────────────────────────────────

function ProductivityTab({ data, range, onRange }: { data: StatsHistory; range: RangeDays; onRange: (r: RangeDays) => void }) {
  const git = useGitHistory(range)
  const merged = mergeProductivity(data.productivity, git.data?.daily ?? [])
  const prod = padDays(merged.length ? merged : data.productivity, d => ({ day: d, lines_added: 0, lines_removed: 0, files_changed: 0, commits: 0, tests_run: 0, tests_passed: 0, tests_failed: 0, prs_opened: 0 }))
  const days = prod.map(d => d.day.slice(5))
  const t = data.totals
  const g = git.data
  // Use git totals when execution_log totals are zero (historical data)
  const linesAdded   = t.total_lines_added   || (g?.total_added   ?? 0)
  const linesRemoved = t.total_lines_removed || (g?.total_removed ?? 0)
  const filesChanged = t.total_files_changed || (g?.total_files   ?? 0)
  const commits      = t.total_commits       || (g?.total_commits ?? 0)
  const hasData = prod.some(d => d.lines_added > 0 || d.commits > 0 || d.tests_run > 0)
  return (
    <div className='space-y-4'>
      <RangeBar range={range} onRange={onRange} />
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
      {!hasData && (
        <Card><CardContent className='py-8 text-center text-sm text-muted-foreground'>
          Productivity data populates as the runner executes tasks with git commits and test runs.
          Fields captured: lines_added, lines_removed, files_changed, commits, pr_number, tests_run, tests_passed, tests_failed.
        </CardContent></Card>
      )}
      {hasData && (
        <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
          <ChartCard title='Lines added / removed per day'>
            <EChart height={180} option={{
              grid: { left: 40, right: 8, top: 8, bottom: 24 },
              tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
              xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
              yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
              series: [
                { name: 'added',   type: 'bar', data: prod.map(d => d.lines_added),    itemStyle: { color: '#10b981' }, stack: 'lines' },
                { name: 'removed', type: 'bar', data: prod.map(d => -d.lines_removed), itemStyle: { color: '#f43f5e' }, stack: 'lines' },
              ],
            }} />
          </ChartCard>
          <ChartCard title='Commits + files changed per day'>
            <EChart height={180} option={{
              grid: { left: 32, right: 8, top: 8, bottom: 24 },
              tooltip: { trigger: 'axis' },
              xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
              yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
              series: [
                { name: 'commits', type: 'bar', data: prod.map(d => d.commits),       itemStyle: { color: '#6366f1' } },
                { name: 'files',   type: 'bar', data: prod.map(d => d.files_changed), itemStyle: { color: '#38bdf8' } },
              ],
            }} />
          </ChartCard>
          <ChartCard title='Tests run / passed / failed per day'>
            <EChart height={180} option={{
              grid: { left: 32, right: 8, top: 8, bottom: 24 },
              tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
              xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 } },
              yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
              series: [
                { name: 'passed', type: 'bar', stack: 't', data: prod.map(d => d.tests_passed), itemStyle: { color: '#10b981' } },
                { name: 'failed', type: 'bar', stack: 't', data: prod.map(d => d.tests_failed), itemStyle: { color: '#f43f5e' } },
              ],
            }} />
          </ChartCard>
        </div>
      )}
    </div>
  )
}

// ── Tab: Agents ───────────────────────────────────────────────────────────

function AgentsTab({ data, range, onRange }: { data: StatsHistory; range: RangeDays; onRange: (r: RangeDays) => void }) {
  const modelColors: Record<string, string> = {
    'claude-opus-4-7': '#f43f5e', 'claude-sonnet-4-6': '#a78bfa',
    'claude-haiku-4-5': '#38bdf8', 'unknown': '#71717a',
  }
  return (
    <div className='space-y-4'>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3'>
        <ChartCard title='Runs by model'>
          {data.models.length ? <EChart height={180} option={{
            tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
            legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
            series: [{ type: 'pie', radius: ['40%','65%'], center: ['50%','42%'], data: data.models.map(d => ({ name: d.label, value: d.count, itemStyle: { color: modelColors[d.label] ?? '#71717a' } })), label: { show: false } }],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Runs by agent runtime'>
          {data.agents.length ? <EChart height={180} option={{
            tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
            legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
            series: [{ type: 'pie', radius: ['40%','65%'], center: ['50%','42%'], data: data.agents.map(d => ({ name: d.label, value: d.count })), label: { show: false } }],
          }} /> : <Empty />}
        </ChartCard>
        <ChartCard title='Interactive vs autonomous'>
          {data.trigger_split.length ? <EChart height={180} option={{
            tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
            legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
            series: [{ type: 'pie', radius: ['40%','65%'], center: ['50%','42%'], data: data.trigger_split.map(d => ({ name: d.label, value: d.count, itemStyle: { color: d.label === 'interactive' ? '#38bdf8' : d.label === 'manual' ? '#10b981' : '#a78bfa' } })), label: { show: false } }],
          }} /> : <Empty />}
        </ChartCard>
      </div>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────

// Each tab owns its data + range independently.
function TabPane({ children }: { children: React.ReactNode }) {
  return <>{children}</>
}

function RunsPane() {
  const [range, setRange] = useState<RangeDays>(0)
  const { data } = useStatsHistory(range)
  return data ? <RunsTab data={data} range={range} onRange={setRange} /> : null
}
function EfficiencyPane() {
  const [range, setRange] = useState<RangeDays>(0)
  const { data } = useStatsHistory(range)
  return data ? <EfficiencyTab data={data} range={range} onRange={setRange} /> : null
}
function TokensPane() {
  const [range, setRange] = useState<RangeDays>(0)
  const { data } = useStatsHistory(range)
  return data ? <TokensTab data={data} range={range} onRange={setRange} /> : null
}
function ProductivityPane() {
  const [range, setRange] = useState<RangeDays>(0)
  const { data } = useStatsHistory(range)
  return data ? <ProductivityTab data={data} range={range} onRange={setRange} /> : null
}
function AgentsPane() {
  const [range, setRange] = useState<RangeDays>(0)
  const { data } = useStatsHistory(range)
  return data ? <AgentsTab data={data} range={range} onRange={setRange} /> : null
}

export function StatsPage() {
  return (
    <>
      <Header />
      <Main>
        <div className='mb-4 flex items-baseline justify-between'>
          <h1 className='text-2xl font-bold tracking-tight'>Stats</h1>
          <span className='text-muted-foreground text-xs'>execution_log</span>
        </div>
        <Tabs defaultValue='runs'>
          <TabsList>
            <TabsTrigger value='runs'>Runs</TabsTrigger>
            <TabsTrigger value='efficiency'>Efficiency</TabsTrigger>
            <TabsTrigger value='tokens'>Tokens</TabsTrigger>
            <TabsTrigger value='productivity'>Productivity</TabsTrigger>
            <TabsTrigger value='agents'>Agents</TabsTrigger>
          </TabsList>
          <TabsContent value='runs'         className='mt-4'><RunsPane /></TabsContent>
          <TabsContent value='efficiency'   className='mt-4'><EfficiencyPane /></TabsContent>
          <TabsContent value='tokens'       className='mt-4'><TokensPane /></TabsContent>
          <TabsContent value='productivity' className='mt-4'><ProductivityPane /></TabsContent>
          <TabsContent value='agents'       className='mt-4'><AgentsPane /></TabsContent>
        </Tabs>
      </Main>
    </>
  )
}
