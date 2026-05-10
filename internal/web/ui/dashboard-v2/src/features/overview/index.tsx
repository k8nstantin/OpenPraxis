import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EChart } from '@/components/echart'
import { cn } from '@/lib/utils'
import { useTurnActivity, } from '@/lib/queries/turns'
import { useLiveRuns, type LiveRun, type EntityKind } from '@/lib/queries/entity'

// ── Types ─────────────────────────────────────────────────────────────────

interface OverviewStats {
  window: string
  runs: number; runs_failed: number; turns: number; actions: number
  input_tokens: number; output_tokens: number
  cache_read_tokens: number; cache_create_tokens: number
  cache_hit_rate_pct: number
  interactive: { runs: number; turns: number; actions: number }
  autonomous:  { runs: number; turns: number; actions: number }
}

interface ChartsData {
  activity:     { hour: string; completed: number; failed: number; running: number }[]
  productivity: { hour: string; lines_added: number; lines_removed: number; files_changed: number; commits: number; tests_run: number; tests_passed: number; tests_failed: number }[]
  efficiency:   { hour: string; avg_turns: number; avg_actions: number; avg_actions_per_turn: number; cache_hit_rate_pct: number }[]
  tokens:       { hour: string; cache_read_tokens: number; cache_create_tokens: number; input_tokens: number; output_tokens: number }[]
  total_commits: number; total_lines_added: number; total_lines_removed: number
  total_files_changed: number; total_prs_opened: number
  total_tests_run: number; total_tests_passed: number; total_tests_failed: number
  repos_touched: number
  interactive_runs: number; autonomous_runs: number
  terminal_reasons: { reason: string; count: number }[]
  system: { hour: string; avg_cpu_pct: number; avg_mem_used_mb: number; avg_net_rx_mbps: number; avg_net_tx_mbps: number; avg_disk_read_mbps: number; avg_disk_write_mbps: number }[]
}

interface GitStats {
  since: string
  total_commits: number; total_added: number; total_removed: number; total_files: number
  hourly_buckets: { hour: string; lines_added: number; lines_removed: number; files_changed: number; commits: number }[]
}

interface SysSample {
  ts: string
  cpu_pct: number
  mem_used_mb: number; mem_total_mb: number
  net_rx_mbps: number; net_tx_mbps: number
  disk_read_mbps: number; disk_write_mbps: number
  disk_used_gb: number; disk_total_gb: number
  load_1m: number
}

// ── Queries ───────────────────────────────────────────────────────────────

function useOverviewStats() {
  return useQuery({
    queryKey: ['stats', 'overview'],
    queryFn: () => fetch('/api/stats/overview').then(r => r.json()) as Promise<OverviewStats>,
    refetchInterval: 30_000, staleTime: 15_000,
  })
}

function useChartsData() {
  return useQuery({
    queryKey: ['stats', 'charts', 24],
    queryFn: () => fetch('/api/stats/charts').then(r => r.json()) as Promise<ChartsData>,
    refetchInterval: 60_000, staleTime: 30_000,
  })
}


function todayHoursET(): number {
  return parseInt(
    new Intl.DateTimeFormat('en-US', { timeZone: TZ, hour: 'numeric', hour12: false }).format(new Date()),
    10,
  ) + 1
}

function useGitStats() {
  const h = todayHoursET()
  return useQuery({
    queryKey: ['stats', 'git', 'today', h],
    queryFn: () => fetch(`/api/stats/git?hours=${h}`).then(r => r.json()) as Promise<GitStats>,
    refetchInterval: 120_000, staleTime: 60_000,
  })
}

// Range in days → hours for system stats (1d = 24h, 0 = all time ≈ 30d)
function sysHours(days: ActivityRange): number {
  if (days === 0) return 24 * 30
  return days * 24
}

function useSysStats(hours = 24) {
  const now = new Date()
  const from = new Date(now.getTime() - hours * 60 * 60 * 1000)
  return useQuery({
    queryKey: ['system-stats', hours],
    queryFn: () => fetch(
      `/api/system-stats?from=${from.toISOString()}&to=${now.toISOString()}`
    ).then(r => r.json()).then((d: { samples: SysSample[] }) => d.samples ?? []) as Promise<SysSample[]>,
    refetchInterval: hours <= 1 ? 10_000 : 60_000,
    staleTime: hours <= 1 ? 5_000 : 30_000,
  })
}

function mergeProductivity(
  exec: ChartsData['productivity'],
  git: GitStats['hourly_buckets'],
): ChartsData['productivity'] {
  const map = new Map<string, ChartsData['productivity'][0]>()
  for (const b of exec) map.set(b.hour, { ...b })
  for (const g of git) {
    const existing = map.get(g.hour)
    if (existing) {
      existing.lines_added   = Math.max(existing.lines_added,   g.lines_added)
      existing.lines_removed = Math.max(existing.lines_removed, g.lines_removed)
      existing.files_changed = Math.max(existing.files_changed, g.files_changed)
      existing.commits       = Math.max(existing.commits,       g.commits)
    } else {
      map.set(g.hour, { hour: g.hour, lines_added: g.lines_added, lines_removed: g.lines_removed,
        files_changed: g.files_changed, commits: g.commits, tests_run: 0, tests_passed: 0, tests_failed: 0 })
    }
  }
  return [...map.values()].sort((a, b) => a.hour < b.hour ? -1 : 1)
}

// ── Helpers ───────────────────────────────────────────────────────────────

function fmt(n: number, dec = 0) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
  if (n >= 1_000)     return (n / 1_000).toFixed(1) + 'k'
  return n.toFixed(dec)
}

const TZ = 'America/New_York'

// ET offset in hours. May–Nov = EDT (−4), Dec–Mar = EST (−5).
function etOffset(): number {
  const d = new Date()
  const jan = new Date(d.getFullYear(), 0, 1)
  const jul = new Date(d.getFullYear(), 6, 1)
  const stdOff = Math.max(jan.getTimezoneOffset(), jul.getTimezoneOffset())
  return -(stdOff > 300 ? 5 : 4) // 300 = EST, <300 = EDT
}

// Convert a UTC ISO bucket string → Eastern-time hour label "9h", "23h" etc.
function hourLabel(iso: string): string {
  const utcH = parseInt(iso.slice(11, 13), 10) // "2026-05-05T09:00:00Z" → 9
  const etH = ((utcH + etOffset()) + 24) % 24
  return `${etH}h`
}

// Format a UTC ISO string as HH:MM:SS in Eastern time for system-stats axes.
function timeLabel(ts: string): string {
  const d = new Date(ts)
  const utcH = d.getUTCHours()
  const utcM = d.getUTCMinutes()
  const utcS = d.getUTCSeconds()
  const etH = ((utcH + etOffset()) + 24) % 24
  return `${etH.toString().padStart(2,'0')}:${utcM.toString().padStart(2,'0')}:${utcS.toString().padStart(2,'0')}`
}

// ── Stat card ─────────────────────────────────────────────────────────────

function Stat({ label, value, sub, accent }: {
  label: string; value: string; sub?: string
  accent?: 'green' | 'amber' | 'rose' | 'violet' | 'sky' | 'blue'
}) {
  return (
    <div className='space-y-0.5'>
      <div className='text-muted-foreground text-xs uppercase tracking-wider'>{label}</div>
      <div className={cn('font-mono text-2xl font-semibold tabular-nums',
        accent === 'green'  && 'text-emerald-400',
        accent === 'amber'  && 'text-amber-400',
        accent === 'rose'   && 'text-rose-400',
        accent === 'violet' && 'text-violet-400',
        accent === 'sky'    && 'text-sky-400',
        accent === 'blue'   && 'text-blue-400',
      )}>{value}</div>
      {sub && <div className='text-muted-foreground text-xs'>{sub}</div>}
    </div>
  )
}

function Empty() {
  return <div className='h-[160px] flex items-center justify-center text-xs text-muted-foreground'>No data</div>
}

// Common axis label config — show every other label so 24 buckets don't crowd
const xLabelOpts = { fontSize: 9, interval: 0 }

// ── Execution charts ───────────────────────────────────────────────────────


const ACTIVITY_RANGES = [
  { label: '1d', days: 1 },
  { label: '2d', days: 2 },
  { label: '3d', days: 3 },
  { label: '1w', days: 7 },
  { label: '2w', days: 14 },
  { label: '1m', days: 30 },
  { label: '3m', days: 90 },
  { label: 'All', days: 0 },
] as const
type ActivityRange = typeof ACTIVITY_RANGES[number]['days']

interface ActivityRow {
  label: string
  turns: number; actions: number
  totalTurns: number
  linesAdded: number; linesRemoved: number; files: number
  netRx: number; netTx: number
}

function buildActivityRows(
  efficiency: { avg_turns: number; avg_actions: number; avg_net_rx_mbps?: number; avg_net_tx_mbps?: number }[],
  labels: string[],
  getLinesAdded: (i: number) => number,
  getLinesRemoved: (i: number) => number,
  getFiles: (i: number) => number,
  getNetRx: (i: number) => number,
  getNetTx: (i: number) => number,
  getTotalTurns: (i: number) => number = () => 0,
): ActivityRow[] {
  return efficiency.map((e, i) => ({
    label: labels[i],
    turns: +e.avg_turns.toFixed(0),
    actions: +e.avg_actions.toFixed(0),
    totalTurns: getTotalTurns(i),
    linesAdded: getLinesAdded(i),
    linesRemoved: getLinesRemoved(i),
    files: getFiles(i),
    netRx: +getNetRx(i).toFixed(2),
    netTx: +getNetTx(i).toFixed(2),
  }))
}

function ActivityChartInner({ rows }: { rows: ActivityRow[] }) {
  const labels = rows.map(r => r.label)
  // Single axis — turns and actions on the same scale
  const hasTotalTurns = rows.some(r => r.totalTurns > 0)
  const series = [
    { name: 'turns',   data: rows.map(r => r.turns),   color: '#a78bfa' },
    { name: 'actions', data: rows.map(r => r.actions),  color: '#38bdf8' },
    ...(hasTotalTurns ? [{ name: 'total turns', data: rows.map(r => r.totalTurns), color: '#fbbf24' }] : []),
  ]
  const axisMax = Math.max(1, ...series.flatMap(s => s.data))

  return (
    <EChart height={260} option={{
      grid: { left: 44, right: 56, top: 12, bottom: 44 },
      tooltip: {
        trigger: 'axis', confine: true,
        position: (pt: number[], _p: unknown, _d: unknown, _r: unknown, sz: {contentSize:number[];viewSize:number[]}) => {
          const [x] = pt; const [w] = sz.contentSize; const [vw] = sz.viewSize
          return [x > vw / 2 ? x - w - 20 : x + 20, 12]
        },
      },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 8 } },
      xAxis: { type: 'category', data: labels, axisLabel: { fontSize: 8 }, boundaryGap: false },
      yAxis: { type: 'value' as const, min: 0, max: Math.ceil(axisMax * 1.1), splitNumber: 5, axisLabel: { fontSize: 8 }, splitLine: { lineStyle: { opacity: 0.15 } } },
      series: series.map(s => ({ name: s.name, type: 'line' as const, data: s.data, smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: s.color, width: 2 } })),
    }} />
  )
}

// Self-contained activity chart: 1d=hourly today, 2d+/All=daily history.
export function ActivityChart({ defaultRange = 1 }: { defaultRange?: ActivityRange } = {}) {
  const [range, setRange] = useState<ActivityRange>(defaultRange)

  // Hourly data (1d only)
  const { data: charts } = useQuery<ChartsData>({
    queryKey: ['stats', 'charts'],
    queryFn: () => fetch('/api/stats/charts').then(r => r.json()),
    enabled: range === 1,
    refetchInterval: 60_000, staleTime: 30_000,
  })

  // Hourly turn-activity overlay (1d only)
  const { data: turnActivity } = useTurnActivity(24)

  // Daily history data (2d+)
  const { data: hist } = useQuery<{
    efficiency: { day: string; avg_turns: number; avg_actions: number }[]
    productivity: { day: string; lines_added: number; lines_removed: number; files_changed: number }[]
    system_daily: { day: string; avg_net_rx_mbps: number; avg_net_tx_mbps: number }[]
  }>({
    queryKey: ['stats', 'history', range],
    queryFn: () => fetch(range === 0 ? '/api/stats/history' : `/api/stats/history?days=${range}`).then(r => r.json()),
    enabled: range !== 1,
    refetchInterval: 60_000, staleTime: 30_000,
  })

  let rows: ActivityRow[] = []

  if (range === 1 && charts) {
    const sys = charts.system ?? []
    const sysMap = new Map(sys.map(s => [s.hour, s]))
    const prodMap = new Map((charts.productivity ?? []).map(p => [p.hour, p]))
    const turnMap = new Map((turnActivity ?? []).map(t => [t.hour, t.turns]))
    rows = buildActivityRows(
      charts.efficiency,
      charts.efficiency.map(d => hourLabel(d.hour)),
      i => prodMap.get(charts.efficiency[i].hour)?.lines_added ?? 0,
      i => prodMap.get(charts.efficiency[i].hour)?.lines_removed ?? 0,
      i => prodMap.get(charts.efficiency[i].hour)?.files_changed ?? 0,
      i => sysMap.get(charts.efficiency[i].hour)?.avg_net_rx_mbps ?? 0,
      i => sysMap.get(charts.efficiency[i].hour)?.avg_net_tx_mbps ?? 0,
      i => turnMap.get(charts.efficiency[i].hour) ?? 0,
    )
  } else if (range !== 1 && hist) {
    const sysMap = new Map((hist.system_daily ?? []).map(s => [s.day, s]))
    const prodMap = new Map((hist.productivity ?? []).map(p => [p.day, p]))
    rows = buildActivityRows(
      hist.efficiency,
      hist.efficiency.map(d => d.day.slice(5)),
      i => prodMap.get(hist.efficiency[i].day)?.lines_added ?? 0,
      i => prodMap.get(hist.efficiency[i].day)?.lines_removed ?? 0,
      i => prodMap.get(hist.efficiency[i].day)?.files_changed ?? 0,
      i => sysMap.get(hist.efficiency[i].day)?.avg_net_rx_mbps ?? 0,
      i => sysMap.get(hist.efficiency[i].day)?.avg_net_tx_mbps ?? 0,
    )
  }

  return (
    <Card>
      <CardHeader className='pb-1 pt-3'>
        <div className='flex items-center justify-between gap-2'>
          <CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>
            Activity · turns · actions · lines · net · {range === 1 ? 'today' : range === 0 ? 'all time' : `${range}d`}
          </CardTitle>
          <div className='inline-flex shrink-0 rounded border bg-muted/20 p-px text-[9px]'>
            {ACTIVITY_RANGES.map(r => (
              <button key={r.days} type='button' onClick={() => setRange(r.days)}
                className={cn('rounded px-1.5 py-0.5 transition-colors',
                  range === r.days ? 'bg-primary/20 text-foreground font-semibold' : 'text-muted-foreground hover:text-foreground'
                )}>{r.label}</button>
            ))}
          </div>
        </div>
      </CardHeader>
      <CardContent className='pb-3'>
        {rows.length > 0 ? <ActivityChartInner rows={rows} /> : <div className='h-[260px] flex items-center justify-center text-xs text-muted-foreground'>Loading…</div>}
      </CardContent>
    </Card>
  )
}

function CacheHitChart({ data }: { data: ChartsData['efficiency'] }) {
  return (
    <EChart height={160} option={{
      grid: { left: 36, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `${p[0]?.value}%` },
      xAxis: { type: 'category', data: data.map(d => hourLabel(d.hour)), axisLabel: xLabelOpts , boundaryGap: false },
      yAxis: { type: 'value', min: 0, max: 100, axisLabel: { fontSize: 9, formatter: '{value}%' } },
      series: [{ type: 'line', data: data.map(d => +d.cache_hit_rate_pct.toFixed(1)), smooth: true, smoothMonotone: 'x', showSymbol: false,
        lineStyle: { color: '#10b981', width: 2 },
        areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1,
          colorStops: [{ offset:0, color:'#10b981aa' },{ offset:1, color:'#10b98100' }] } } }],
    }} />
  )
}

function AvgTurnsChart({ data }: { data: ChartsData['efficiency'] }) {
  return (
    <EChart height={160} option={{
      grid: { left: 36, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: data.map(d => hourLabel(d.hour)), axisLabel: xLabelOpts , boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [
        { name: 'turns/run',    type: 'line', data: data.map(d => +d.avg_turns.toFixed(1)), smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: '#a78bfa', width: 2 } },
        { name: 'actions/turn', type: 'line', data: data.map(d => +d.avg_actions_per_turn.toFixed(1)), smooth: true, smoothMonotone: 'x', showSymbol: false, lineStyle: { color: '#38bdf8', width: 2 } },
      ],
    }} />
  )
}

function LinesChart({ data }: { data: ChartsData['productivity'] }) {
  return (
    <EChart height={160} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: data.map(d => hourLabel(d.hour)), axisLabel: xLabelOpts , boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [
        { name: 'added',   type: 'bar', data: data.map(d => d.lines_added),    itemStyle: { color: '#10b981' }, stack: 'lines' },
        { name: 'removed', type: 'bar', data: data.map(d => -d.lines_removed), itemStyle: { color: '#f43f5e' }, stack: 'lines' },
      ],
    }} />
  )
}

function CommitsChart({ data }: { data: ChartsData['productivity'] }) {
  return (
    <EChart height={160} option={{
      grid: { left: 32, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: data.map(d => hourLabel(d.hour)), axisLabel: xLabelOpts , boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'commits', type: 'bar', data: data.map(d => d.commits),   itemStyle: { color: '#6366f1' } },
        { name: 'tests',   type: 'bar', data: data.map(d => d.tests_run), itemStyle: { color: '#f59e0b' } },
      ],
    }} />
  )
}

function TerminalReasonsChart({ data }: { data: ChartsData['terminal_reasons'] }) {
  const colors: Record<string, string> = {
    success: '#10b981', max_turns: '#f59e0b', error: '#f43f5e',
    process_error: '#f43f5e', timeout: '#fb923c', deliverable_missing: '#a78bfa',
  }
  return (
    <EChart height={160} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9 } },
      series: [{ type: 'pie', radius: ['40%', '65%'], center: ['50%', '45%'],
        data: data.map(d => ({ name: d.reason || 'success', value: d.count,
          itemStyle: { color: colors[d.reason] ?? '#71717a' } })),
        label: { show: false } }],
    }} />
  )
}

function SplitChart({ interactive, autonomous }: { interactive: number; autonomous: number }) {
  return (
    <EChart height={160} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      series: [{ type: 'pie', radius: ['40%', '65%'], center: ['50%', '45%'],
        data: [
          { name: 'interactive', value: interactive, itemStyle: { color: '#38bdf8' } },
          { name: 'autonomous',  value: autonomous,  itemStyle: { color: '#a78bfa' } },
        ], label: { show: false } }],
    }} />
  )
}

// ── System stats charts ────────────────────────────────────────────────────

// Keep full 5s resolution — ECharts canvas handles 720 points fine.
// interval:'auto' on x-axis ensures labels don't crowd.
function downsample(samples: SysSample[], targetPoints = 720): SysSample[] {
  if (samples.length <= targetPoints) return samples
  const step = Math.ceil(samples.length / targetPoints)
  return samples.filter((_, i) => i % step === 0)
}


function CPUChart({ samples }: { samples: SysSample[] }) {
  const ds = downsample(samples)
  return (
    <EChart height={160} option={{
      grid: { left: 36, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `CPU ${p[0]?.value?.toFixed(1)}%` },
      xAxis: { type: 'category', data: ds.map(s => timeLabel(s.ts)), axisLabel: { fontSize: 9, interval: 'auto', showMaxLabel: true, rotate: 0  } },
      yAxis: { type: 'value', min: 0, max: 100, axisLabel: { fontSize: 9, formatter: '{value}%' } },
      series: [{
        name: 'CPU %', type: 'line', data: ds.map(s => +s.cpu_pct.toFixed(1)),
        smooth: true, showSymbol: false, lineStyle: { color: '#f59e0b', width: 1.5 },
        areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1,
          colorStops: [{ offset:0, color:'#f59e0b55' }, { offset:1, color:'#f59e0b00' }] } },
      }],
    }} />
  )
}

function MemoryChart({ samples }: { samples: SysSample[] }) {
  const ds = downsample(samples)
  const total = ds[0]?.mem_total_mb ?? 16384
  return (
    <EChart height={160} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `RAM ${((p[0]?.value ?? 0)/1024).toFixed(1)} GB` },
      xAxis: { type: 'category', data: ds.map(s => timeLabel(s.ts)), axisLabel: { fontSize: 9, interval: 'auto', showMaxLabel: true, rotate: 0  } },
      yAxis: { type: 'value', min: 0, max: total, axisLabel: { fontSize: 9, formatter: (v: number) => `${(v/1024).toFixed(0)}G` } },
      series: [{
        name: 'RAM used', type: 'line', data: ds.map(s => +s.mem_used_mb.toFixed(0)),
        smooth: true, showSymbol: false, lineStyle: { color: '#38bdf8', width: 1.5 },
        areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1,
          colorStops: [{ offset:0, color:'#38bdf855' }, { offset:1, color:'#38bdf800' }] } },
      }],
    }} />
  )
}

function NetworkChart({ samples }: { samples: SysSample[] }) {
  const ds = downsample(samples)
  return (
    <EChart height={160} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9 } },
      xAxis: { type: 'category', data: ds.map(s => timeLabel(s.ts)), axisLabel: { fontSize: 9, interval: 'auto', showMaxLabel: true, rotate: 0  } },
      yAxis: { type: 'value', axisLabel: { fontSize: 9, formatter: '{value}M' } },
      series: [
        { name: 'rx', type: 'line', data: ds.map(s => +s.net_rx_mbps.toFixed(2)), smooth: true, showSymbol: false, lineStyle: { color: '#10b981', width: 1.5 } },
        { name: 'tx', type: 'line', data: ds.map(s => +s.net_tx_mbps.toFixed(2)), smooth: true, showSymbol: false, lineStyle: { color: '#a78bfa', width: 1.5 } },
      ],
    }} />
  )
}

function DiskIOChart({ samples }: { samples: SysSample[] }) {
  const ds = downsample(samples)
  return (
    <EChart height={160} option={{
      grid: { left: 40, right: 16, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9 } },
      xAxis: { type: 'category', data: ds.map(s => timeLabel(s.ts)), axisLabel: { fontSize: 9, interval: 'auto', showMaxLabel: true, rotate: 0  } },
      yAxis: { type: 'value', axisLabel: { fontSize: 9, formatter: '{value}M' } },
      series: [
        { name: 'read',  type: 'line', data: ds.map(s => +s.disk_read_mbps.toFixed(2)),  smooth: true, showSymbol: false, lineStyle: { color: '#f59e0b', width: 1.5 } },
        { name: 'write', type: 'line', data: ds.map(s => +s.disk_write_mbps.toFixed(2)), smooth: true, showSymbol: false, lineStyle: { color: '#f43f5e', width: 1.5 } },
      ],
    }} />
  )
}

// ── System stats — self-contained card with range selector ────────────────

export function SystemStatsChart({ defaultRange = 1 }: { defaultRange?: ActivityRange } = {}) {
  const [range, setRange] = useState<ActivityRange>(defaultRange)
  const { data: samples = [] } = useSysStats(sysHours(range))
  const latestSys = samples.length ? samples[samples.length - 1] : null

  const rangeLabel = range === 1 ? 'today' : range === 0 ? 'all time' : `${range}d`

  return (
    <Card>
      <CardHeader className='pb-1 pt-3'>
        <div className='flex items-center justify-between gap-2'>
          <CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>
            System · cpu · ram · network · disk · {rangeLabel}
          </CardTitle>
          <div className='inline-flex shrink-0 rounded border bg-muted/20 p-px text-[9px]'>
            {ACTIVITY_RANGES.map(r => (
              <button key={r.days} type='button' onClick={() => setRange(r.days)}
                className={cn('rounded px-1.5 py-0.5 transition-colors',
                  range === r.days ? 'bg-primary/20 text-foreground font-semibold' : 'text-muted-foreground hover:text-foreground'
                )}>{r.label}</button>
            ))}
          </div>
        </div>
      </CardHeader>
      <CardContent className='pb-3 space-y-1'>
        {/* Live snapshot */}
        {latestSys && (
          <div className='grid grid-cols-3 gap-2 md:grid-cols-6 mb-2'>
            {[
              { l: 'CPU',    v: `${latestSys.cpu_pct.toFixed(1)}%`,           accent: 'text-amber-400' },
              { l: 'RAM',    v: `${(latestSys.mem_used_mb/1024).toFixed(1)}G`, accent: 'text-sky-400' },
              { l: 'Net RX', v: `${latestSys.net_rx_mbps.toFixed(1)} M`,       accent: 'text-emerald-400' },
              { l: 'Net TX', v: `${latestSys.net_tx_mbps.toFixed(1)} M`,       accent: 'text-violet-400' },
              { l: 'Disk R', v: `${latestSys.disk_read_mbps.toFixed(1)} M`,    accent: 'text-amber-400' },
              { l: 'Disk W', v: `${latestSys.disk_write_mbps.toFixed(1)} M`,   accent: 'text-rose-400' },
            ].map(({ l, v, accent }) => (
              <div key={l} className='text-center'>
                <div className='text-muted-foreground text-[10px]'>{l}</div>
                <div className={cn('font-mono text-sm font-semibold', accent)}>{v}</div>
              </div>
            ))}
          </div>
        )}
        {samples.length > 0 ? (
          <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
            <CPUChart samples={samples} />
            <MemoryChart samples={samples} />
            <NetworkChart samples={samples} />
            <DiskIOChart samples={samples} />
          </div>
        ) : (
          <div className='h-[160px] flex items-center justify-center text-xs text-muted-foreground'>Loading…</div>
        )}
      </CardContent>
    </Card>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────

function fmtElapsed(sec: number) {
  if (sec < 60) return `${sec}s`
  if (sec < 3600) return `${Math.floor(sec / 60)}m ${sec % 60}s`
  return `${Math.floor(sec / 3600)}h ${Math.floor((sec % 3600) / 60)}m`
}

function RunningAgentCard({ run }: { run: LiveRun }) {
  const navigate = useNavigate()
  const handleClick = () => {
    if (run.entity_uid && run.entity_uid !== 'stdio') {
      navigate({ to: '/entities/$uid', params: { uid: run.entity_uid }, search: { kind: run.entity_type as EntityKind, tab: 'runs' } })
    }
  }
  return (
    <div
      onClick={handleClick}
      className={cn(
        'rounded-lg border border-emerald-500/40 bg-emerald-500/5 p-3 cursor-pointer',
        'hover:border-emerald-500/70 hover:bg-emerald-500/10 transition-colors',
        'flex flex-col gap-2 min-w-0',
      )}
    >
      <div className='flex items-center gap-2'>
        <span className='relative flex h-2.5 w-2.5 shrink-0'>
          <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75' />
          <span className='relative inline-flex h-2.5 w-2.5 rounded-full bg-emerald-500' />
        </span>
        <span className='text-xs font-medium text-emerald-400 truncate flex-1'>
          {run.entity_title || run.entity_uid?.slice(0, 12) || 'stdio'}
        </span>
      </div>
      <div className='grid grid-cols-3 gap-1 text-center'>
        <div>
          <div className='font-mono text-sm font-bold tabular-nums'>{run.turns}</div>
          <div className='text-[10px] text-muted-foreground uppercase tracking-wide'>turns</div>
        </div>
        <div>
          <div className='font-mono text-sm font-bold tabular-nums'>{run.actions}</div>
          <div className='text-[10px] text-muted-foreground uppercase tracking-wide'>actions</div>
        </div>
        <div>
          <div className='font-mono text-sm font-bold tabular-nums'>{fmtElapsed(run.elapsed_sec)}</div>
          <div className='text-[10px] text-muted-foreground uppercase tracking-wide'>elapsed</div>
        </div>
      </div>
      {run.model && (
        <div className='text-[10px] text-muted-foreground truncate'>{run.model}</div>
      )}
    </div>
  )
}

function RunningAgents() {
  const { data: runs } = useLiveRuns()
  const active = (runs ?? []).filter(r => r.entity_uid && r.entity_uid !== 'stdio')
  return (
    <div>
      <div className='flex items-center gap-2 mb-2'>
        <h2 className='text-sm font-semibold tracking-tight'>Running Agents</h2>
        {active.length > 0 ? (
          <span className='rounded-full bg-emerald-500/20 text-emerald-400 text-[10px] font-medium px-2 py-0.5'>
            {active.length} active
          </span>
        ) : (
          <span className='text-[10px] text-muted-foreground'>none running</span>
        )}
      </div>
      {active.length > 0 ? (
        <div className='grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6'>
          {active.map(r => <RunningAgentCard key={r.run_uid} run={r} />)}
        </div>
      ) : (
        <div className='rounded-lg border border-dashed border-white/10 p-4 text-center text-xs text-muted-foreground'>
          No agents running — idle
        </div>
      )}
    </div>
  )
}

export function Overview() {
  const { data: s } = useOverviewStats()
  const { data: c } = useChartsData()
  const { data: g } = useGitStats()
  const { data: sys } = useSysStats()

  const commits      = (g?.total_commits  ?? 0) || (c?.total_commits      ?? 0)
  const linesAdded   = (g?.total_added    ?? 0) || (c?.total_lines_added  ?? 0)
  const linesRemoved = (g?.total_removed  ?? 0) || (c?.total_lines_removed ?? 0)
  const files        = (g?.total_files    ?? 0) || (c?.total_files_changed ?? 0)
  const productivity = mergeProductivity(c?.productivity ?? [], g?.hourly_buckets ?? [])
  const totalTok = (s?.input_tokens ?? 0) + (s?.output_tokens ?? 0) +
                   (s?.cache_read_tokens ?? 0) + (s?.cache_create_tokens ?? 0)

  // Current system snapshot (latest sample)
  const latestSys = sys?.length ? sys[sys.length - 1] : null

  return (
    <>
      <Header />
      <Main>
        <div className='mb-4 flex items-baseline justify-between'>
          <h1 className='text-2xl font-bold tracking-tight'>Overview</h1>
          <span className='text-muted-foreground text-xs'>last 24 h · execution_log</span>
        </div>

        <div className='space-y-4'>

          {/* Running Agents — live task cards, auto-refreshes */}
          <RunningAgents />

          {/* Row 1 — headline stats */}
          <div className='grid grid-cols-2 gap-3 md:grid-cols-4 lg:grid-cols-8'>
            <Card><CardContent className='pt-4'><Stat label='Runs'    value={String((s?.runs ?? 0) + (s?.runs_failed ?? 0))} sub={s?.runs_failed ? `${s.runs_failed} failed` : 'all ok'} /></CardContent></Card>
            <Card><CardContent className='pt-4'><Stat label='Turns'   value={fmt(s?.turns ?? 0)} /></CardContent></Card>
            <Card><CardContent className='pt-4'><Stat label='Actions' value={fmt(s?.actions ?? 0)} /></CardContent></Card>
            <Card><CardContent className='pt-4'><Stat label='Cache'   value={`${(s?.cache_hit_rate_pct ?? 0).toFixed(0)}%`} sub='hit rate' accent='green' /></CardContent></Card>
            <Card><CardContent className='pt-4'><Stat label='Commits'  value={String(commits)} accent='blue' /></CardContent></Card>
            <Card><CardContent className='pt-4'><Stat label='Lines +'  value={fmt(linesAdded)} sub={`-${fmt(linesRemoved)}`} accent='green' /></CardContent></Card>
            <Card><CardContent className='pt-4'><Stat label='Files'    value={fmt(files)} /></CardContent></Card>
            <Card><CardContent className='pt-4'><Stat label='Tests'    value={String(c?.total_tests_run ?? 0)} sub={c?.total_tests_failed ? `${c.total_tests_failed} failed` : 'all passed'} accent={c?.total_tests_failed ? 'rose' : 'green'} /></CardContent></Card>
          </div>

          {/* Row 2 — system snapshot (live cards only — charts in SystemStatsChart below) */}
          {latestSys && (
            <div className='grid grid-cols-2 gap-3 md:grid-cols-4 lg:grid-cols-6'>
              <Card><CardContent className='pt-4'><Stat label='CPU' value={`${latestSys.cpu_pct.toFixed(1)}%`} sub={`load ${latestSys.load_1m.toFixed(2)}`} accent='amber' /></CardContent></Card>
              <Card><CardContent className='pt-4'><Stat label='RAM' value={`${(latestSys.mem_used_mb/1024).toFixed(1)}G`} sub={`of ${(latestSys.mem_total_mb/1024).toFixed(0)}G`} accent='sky' /></CardContent></Card>
              <Card><CardContent className='pt-4'><Stat label='Net RX' value={`${latestSys.net_rx_mbps.toFixed(1)}`} sub='Mbps' accent='green' /></CardContent></Card>
              <Card><CardContent className='pt-4'><Stat label='Net TX' value={`${latestSys.net_tx_mbps.toFixed(1)}`} sub='Mbps' accent='violet' /></CardContent></Card>
              <Card><CardContent className='pt-4'><Stat label='Disk R' value={`${latestSys.disk_read_mbps.toFixed(1)}`} sub='MB/s' accent='amber' /></CardContent></Card>
              <Card><CardContent className='pt-4'><Stat label='Disk W' value={`${latestSys.disk_write_mbps.toFixed(1)}`} sub='MB/s' accent='rose' /></CardContent></Card>
            </div>
          )}

          {/* Row 3 — activity chart (self-contained, same as Stats page) */}
          <ActivityChart />

          {/* Row 4 — commits + lines */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Commits · tests · 24h</CardTitle></CardHeader>
              <CardContent className='pb-3'>{productivity.length ? <CommitsChart data={productivity} /> : <Empty />}</CardContent>
            </Card>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Lines added / removed · 24h</CardTitle></CardHeader>
              <CardContent className='pb-3'>{productivity.length ? <LinesChart data={productivity} /> : <Empty />}</CardContent>
            </Card>
          </div>

          {/* Row 5 — system charts with range selector */}
          <SystemStatsChart />

          {/* Row 6 — commits + quality split */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Commits · tests · 24h</CardTitle></CardHeader>
              <CardContent className='pb-3'>{productivity.length ? <CommitsChart data={productivity} /> : <Empty />}</CardContent>
            </Card>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Terminal reasons · 24h</CardTitle></CardHeader>
              <CardContent className='pb-3'>{c?.terminal_reasons?.length ? <TerminalReasonsChart data={c.terminal_reasons} /> : <Empty />}</CardContent>
            </Card>
          </div>

          {/* Row 7 — interactive/autonomous + token bar */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Interactive vs autonomous · 24h</CardTitle></CardHeader>
              <CardContent className='pb-3'>
                {(c?.interactive_runs ?? 0) + (c?.autonomous_runs ?? 0) > 0
                  ? <SplitChart interactive={c?.interactive_runs ?? 0} autonomous={c?.autonomous_runs ?? 0} />
                  : <Empty />}
              </CardContent>
            </Card>
            {totalTok > 0 && (
              <Card>
                <CardContent className='pt-4 pb-3'>
                  <div className='flex items-center justify-between text-xs text-muted-foreground mb-1.5'>
                    <span>Token split · {fmt(totalTok)} total</span>
                    <span>cache {(s?.cache_hit_rate_pct ?? 0).toFixed(0)}%</span>
                  </div>
                  <div className='h-3 rounded-full overflow-hidden flex gap-px bg-zinc-900'>
                    {[
                      { v: s?.cache_read_tokens ?? 0,  c: 'bg-emerald-500', l: 'cache read' },
                      { v: s?.input_tokens ?? 0,        c: 'bg-sky-500',     l: 'input' },
                      { v: s?.output_tokens ?? 0,       c: 'bg-violet-500',  l: 'output' },
                      { v: s?.cache_create_tokens ?? 0, c: 'bg-amber-500',   l: 'cache write' },
                    ].map(({ v, c: cls, l }) => (
                      <div key={l} className={cls} style={{ width: `${totalTok > 0 ? (v/totalTok)*100 : 0}%` }} title={`${l}: ${fmt(v)}`} />
                    ))}
                  </div>
                  <div className='flex gap-3 mt-1.5 text-[10px] text-muted-foreground'>
                    <span><span className='text-emerald-500'>■</span> cache read</span>
                    <span><span className='text-sky-500'>■</span> input</span>
                    <span><span className='text-violet-500'>■</span> output</span>
                    <span><span className='text-amber-500'>■</span> cache write</span>
                  </div>
                </CardContent>
              </Card>
            )}
          </div>

        </div>
      </Main>
    </>
  )
}
