import { useQuery } from '@tanstack/react-query'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EChart } from '@/components/echart'
import { cn } from '@/lib/utils'

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
  activity:     { hour: string; completed: number; failed: number }[]
  productivity: { hour: string; lines_added: number; lines_removed: number; files_changed: number; commits: number; tests_run: number; tests_passed: number; tests_failed: number }[]
  efficiency:   { hour: string; avg_turns: number; avg_actions_per_turn: number; cache_hit_rate_pct: number }[]
  tokens:       { hour: string; cache_read_tokens: number; cache_create_tokens: number; input_tokens: number; output_tokens: number }[]
  total_commits: number; total_lines_added: number; total_lines_removed: number
  total_files_changed: number; total_prs_opened: number
  total_tests_run: number; total_tests_passed: number; total_tests_failed: number
  repos_touched: number
  interactive_runs: number; autonomous_runs: number
  terminal_reasons: { reason: string; count: number }[]
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
    queryFn: () => fetch('/api/stats/charts?hours=24').then(r => r.json()) as Promise<ChartsData>,
    refetchInterval: 60_000, staleTime: 30_000,
  })
}

function useGitStats() {
  return useQuery({
    queryKey: ['stats', 'git'],
    queryFn: () => fetch('/api/stats/git?hours=24').then(r => r.json()) as Promise<GitStats>,
    refetchInterval: 120_000, staleTime: 60_000,
  })
}

function useSysStats() {
  const now = new Date()
  const from = new Date(now.getTime() - 60 * 60 * 1000) // last 60 min
  return useQuery({
    queryKey: ['system-stats', 'overview'],
    queryFn: () => fetch(
      `/api/system-stats?from=${from.toISOString()}&to=${now.toISOString()}`
    ).then(r => r.json()).then((d: { samples: SysSample[] }) => d.samples ?? []) as Promise<SysSample[]>,
    refetchInterval: 10_000, staleTime: 5_000,
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

// "2026-05-04T14:00:00Z" → "Mon 14h" or just "14h"
function hourLabel(iso: string) {
  const d = new Date(iso)
  const h = d.getUTCHours().toString().padStart(2, '0')
  // Show day prefix at midnight
  if (d.getUTCHours() === 0) {
    return ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'][d.getUTCDay()] + ' 0h'
  }
  return h + 'h'
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

// ── Execution charts ───────────────────────────────────────────────────────

function ActivityChart({ data }: { data: ChartsData['activity'] }) {
  const hours  = data.map(d => hourLabel(d.hour))
  return (
    <EChart height={160} option={{
      grid: { left: 32, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: hours, axisLabel: { fontSize: 9 } },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'completed', type: 'bar', stack: 'runs', data: data.map(d => d.completed), itemStyle: { color: '#10b981' } },
        { name: 'failed',    type: 'bar', stack: 'runs', data: data.map(d => d.failed),    itemStyle: { color: '#f43f5e' } },
      ],
    }} />
  )
}

function CacheHitChart({ data }: { data: ChartsData['efficiency'] }) {
  return (
    <EChart height={160} option={{
      grid: { left: 36, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `${p[0]?.value}%` },
      xAxis: { type: 'category', data: data.map(d => hourLabel(d.hour)), axisLabel: { fontSize: 9 } },
      yAxis: { type: 'value', min: 0, max: 100, axisLabel: { fontSize: 9, formatter: '{value}%' } },
      series: [{ type: 'line', data: data.map(d => +d.cache_hit_rate_pct.toFixed(1)), smooth: true, showSymbol: false,
        lineStyle: { color: '#10b981', width: 2 },
        areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1,
          colorStops: [{ offset:0, color:'#10b981aa' },{ offset:1, color:'#10b98100' }] } } }],
    }} />
  )
}

function AvgTurnsChart({ data }: { data: ChartsData['efficiency'] }) {
  return (
    <EChart height={160} option={{
      grid: { left: 36, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: data.map(d => hourLabel(d.hour)), axisLabel: { fontSize: 9 } },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [
        { name: 'turns/run',    type: 'line', data: data.map(d => +d.avg_turns.toFixed(1)), smooth: true, showSymbol: false, lineStyle: { color: '#a78bfa', width: 2 } },
        { name: 'actions/turn', type: 'line', data: data.map(d => +d.avg_actions_per_turn.toFixed(1)), smooth: true, showSymbol: false, lineStyle: { color: '#38bdf8', width: 2 } },
      ],
    }} />
  )
}

function LinesChart({ data }: { data: ChartsData['productivity'] }) {
  return (
    <EChart height={160} option={{
      grid: { left: 40, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: data.map(d => hourLabel(d.hour)), axisLabel: { fontSize: 9 } },
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
      grid: { left: 32, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: data.map(d => hourLabel(d.hour)), axisLabel: { fontSize: 9 } },
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

function downsample(samples: SysSample[], targetPoints = 120): SysSample[] {
  if (samples.length <= targetPoints) return samples
  const step = Math.ceil(samples.length / targetPoints)
  return samples.filter((_, i) => i % step === 0)
}

function timeLabel(ts: string) {
  const d = new Date(ts)
  return d.getUTCHours().toString().padStart(2,'0') + ':' + d.getUTCMinutes().toString().padStart(2,'0')
}

function CPUChart({ samples }: { samples: SysSample[] }) {
  const ds = downsample(samples)
  return (
    <EChart height={160} option={{
      grid: { left: 36, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `CPU ${p[0]?.value?.toFixed(1)}%` },
      xAxis: { type: 'category', data: ds.map(s => timeLabel(s.ts)), axisLabel: { fontSize: 9 } },
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
      grid: { left: 40, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `RAM ${((p[0]?.value ?? 0)/1024).toFixed(1)} GB` },
      xAxis: { type: 'category', data: ds.map(s => timeLabel(s.ts)), axisLabel: { fontSize: 9 } },
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
      grid: { left: 40, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9 } },
      xAxis: { type: 'category', data: ds.map(s => timeLabel(s.ts)), axisLabel: { fontSize: 9 } },
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
      grid: { left: 40, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9 } },
      xAxis: { type: 'category', data: ds.map(s => timeLabel(s.ts)), axisLabel: { fontSize: 9 } },
      yAxis: { type: 'value', axisLabel: { fontSize: 9, formatter: '{value}M' } },
      series: [
        { name: 'read',  type: 'line', data: ds.map(s => +s.disk_read_mbps.toFixed(2)),  smooth: true, showSymbol: false, lineStyle: { color: '#f59e0b', width: 1.5 } },
        { name: 'write', type: 'line', data: ds.map(s => +s.disk_write_mbps.toFixed(2)), smooth: true, showSymbol: false, lineStyle: { color: '#f43f5e', width: 1.5 } },
      ],
    }} />
  )
}

// ── Page ──────────────────────────────────────────────────────────────────

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

          {/* Row 2 — system snapshot */}
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

          {/* Row 3 — activity + cache */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Runs per hour · 24h</CardTitle></CardHeader>
              <CardContent className='pb-3'>{c?.activity?.length ? <ActivityChart data={c.activity} /> : <Empty />}</CardContent>
            </Card>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Cache hit rate % · 24h</CardTitle></CardHeader>
              <CardContent className='pb-3'>{c?.efficiency?.length ? <CacheHitChart data={c.efficiency} /> : <Empty />}</CardContent>
            </Card>
          </div>

          {/* Row 4 — efficiency + token ratio */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Avg turns · actions/turn · 24h</CardTitle></CardHeader>
              <CardContent className='pb-3'>{c?.efficiency?.length ? <AvgTurnsChart data={c.efficiency} /> : <Empty />}</CardContent>
            </Card>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Lines added / removed · 24h</CardTitle></CardHeader>
              <CardContent className='pb-3'>{productivity.length ? <LinesChart data={productivity} /> : <Empty />}</CardContent>
            </Card>
          </div>

          {/* Row 5 — system charts */}
          {sys && sys.length > 0 && (
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <Card>
                <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>CPU % · last 60 min</CardTitle></CardHeader>
                <CardContent className='pb-3'><CPUChart samples={sys} /></CardContent>
              </Card>
              <Card>
                <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>RAM · last 60 min</CardTitle></CardHeader>
                <CardContent className='pb-3'><MemoryChart samples={sys} /></CardContent>
              </Card>
            </div>
          )}
          {sys && sys.length > 0 && (
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <Card>
                <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Network RX / TX · last 60 min</CardTitle></CardHeader>
                <CardContent className='pb-3'><NetworkChart samples={sys} /></CardContent>
              </Card>
              <Card>
                <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Disk read / write · last 60 min</CardTitle></CardHeader>
                <CardContent className='pb-3'><DiskIOChart samples={sys} /></CardContent>
              </Card>
            </div>
          )}

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
