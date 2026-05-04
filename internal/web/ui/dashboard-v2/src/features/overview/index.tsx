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
    queryKey: ['stats', 'charts'],
    queryFn: () => fetch('/api/stats/charts').then(r => r.json()) as Promise<ChartsData>,
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

// Merge git hourly data with execution_log productivity data.
// Git data is the source of truth for code metrics; execution_log fills in
// tests and any future metrics the runner captures.
function mergeProductivity(
  exec: ChartsData['productivity'],
  git: GitStats['hourly_buckets'],
): ChartsData['productivity'] {
  const map = new Map<string, ChartsData['productivity'][0]>()
  for (const b of exec) {
    map.set(b.hour, { ...b })
  }
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

function hourLabel(iso: string) {
  // "2026-05-04T14:00:00Z" → "14h"
  return iso.slice(11, 13) + 'h'
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

// ── Charts ────────────────────────────────────────────────────────────────

function ActivityChart({ data }: { data: ChartsData['activity'] }) {
  const hours  = data.map(d => hourLabel(d.hour))
  const ok     = data.map(d => d.completed)
  const failed = data.map(d => d.failed)
  return (
    <EChart height={160} option={{
      grid: { left: 32, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: hours, axisLabel: { fontSize: 9 } },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'completed', type: 'bar', stack: 'runs', data: ok,     itemStyle: { color: '#10b981' } },
        { name: 'failed',    type: 'bar', stack: 'runs', data: failed, itemStyle: { color: '#f43f5e' } },
      ],
    }} />
  )
}

function CacheHitChart({ data }: { data: ChartsData['efficiency'] }) {
  const hours = data.map(d => hourLabel(d.hour))
  const rates = data.map(d => +d.cache_hit_rate_pct.toFixed(1))
  return (
    <EChart height={160} option={{
      grid: { left: 36, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `${p[0]?.value}%` },
      xAxis: { type: 'category', data: hours, axisLabel: { fontSize: 9 } },
      yAxis: { type: 'value', min: 0, max: 100, axisLabel: { fontSize: 9, formatter: '{value}%' } },
      series: [{
        type: 'line', data: rates, smooth: true, showSymbol: false,
        lineStyle: { color: '#10b981', width: 2 },
        areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1,
          colorStops: [{ offset:0, color:'#10b981aa' },{ offset:1, color:'#10b98100' }] } },
      }],
    }} />
  )
}

function AvgTurnsChart({ data }: { data: ChartsData['efficiency'] }) {
  const hours = data.map(d => hourLabel(d.hour))
  const turns = data.map(d => +d.avg_turns.toFixed(1))
  const apts  = data.map(d => +d.avg_actions_per_turn.toFixed(1))
  return (
    <EChart height={160} option={{
      grid: { left: 36, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      xAxis: { type: 'category', data: hours, axisLabel: { fontSize: 9 } },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [
        { name: 'turns/run',    type: 'line', data: turns, smooth: true, showSymbol: false, lineStyle: { color: '#a78bfa', width: 2 } },
        { name: 'actions/turn', type: 'line', data: apts,  smooth: true, showSymbol: false, lineStyle: { color: '#38bdf8', width: 2 } },
      ],
    }} />
  )
}

function LinesChart({ data }: { data: ChartsData['productivity'] }) {
  const hours   = data.map(d => hourLabel(d.hour))
  const added   = data.map(d => d.lines_added)
  const removed = data.map(d => -d.lines_removed)
  return (
    <EChart height={160} option={{
      grid: { left: 40, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
      xAxis: { type: 'category', data: hours, axisLabel: { fontSize: 9 } },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [
        { name: 'added',   type: 'bar', data: added,   itemStyle: { color: '#10b981' }, stack: 'lines' },
        { name: 'removed', type: 'bar', data: removed, itemStyle: { color: '#f43f5e' }, stack: 'lines' },
      ],
    }} />
  )
}

function CommitsChart({ data }: { data: ChartsData['productivity'] }) {
  const hours   = data.map(d => hourLabel(d.hour))
  const commits = data.map(d => d.commits)
  const tests   = data.map(d => d.tests_run)
  return (
    <EChart height={160} option={{
      grid: { left: 32, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis' },
      xAxis: { type: 'category', data: hours, axisLabel: { fontSize: 9 } },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'commits', type: 'bar', data: commits, itemStyle: { color: '#6366f1' } },
        { name: 'tests',   type: 'bar', data: tests,   itemStyle: { color: '#f59e0b' } },
      ],
    }} />
  )
}

function TokenRatioChart({ data }: { data: ChartsData['tokens'] }) {
  const hours  = data.map(d => hourLabel(d.hour))
  const ratios = data.map(d => {
    const total = d.cache_read_tokens + d.cache_create_tokens
    return total > 0 ? +(d.cache_read_tokens / total * 100).toFixed(1) : 0
  })
  return (
    <EChart height={160} option={{
      grid: { left: 36, right: 8, top: 8, bottom: 24 },
      tooltip: { trigger: 'axis', formatter: (p: {value:number}[]) => `cache reuse ${p[0]?.value}%` },
      xAxis: { type: 'category', data: hours, axisLabel: { fontSize: 9 } },
      yAxis: { type: 'value', min: 0, max: 100, axisLabel: { fontSize: 9, formatter: '{value}%' } },
      series: [{
        name: 'cache reuse', type: 'line', data: ratios, smooth: true, showSymbol: false,
        lineStyle: { color: '#10b981', width: 2 },
        areaStyle: { color: { type: 'linear', x:0,y:0,x2:0,y2:1,
          colorStops: [{ offset:0, color:'#10b98155' },{ offset:1, color:'#10b98100' }] } },
      }],
    }} />
  )
}

function TerminalReasonsChart({ data }: { data: ChartsData['terminal_reasons'] }) {
  const colors: Record<string, string> = {
    success: '#10b981', max_turns: '#f59e0b', error: '#f43f5e',
    process_error: '#f43f5e', timeout: '#fb923c', deliverable_missing: '#a78bfa',
  }
  const items = data.map(d => ({
    name: d.reason || 'success', value: d.count,
    itemStyle: { color: colors[d.reason] ?? '#71717a' },
  }))
  return (
    <EChart height={160} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      legend: { bottom: 0, itemWidth: 8, itemHeight: 8, textStyle: { fontSize: 9, color: '#a1a1aa' } },
      series: [{
        type: 'pie', radius: ['40%', '65%'], center: ['50%', '45%'],
        data: items, label: { show: false },
      }],
    }} />
  )
}

function SplitChart({ interactive, autonomous }: { interactive: number; autonomous: number }) {
  return (
    <EChart height={160} option={{
      tooltip: { trigger: 'item', formatter: '{b}: {c} ({d}%)' },
      series: [{
        type: 'pie', radius: ['40%', '65%'], center: ['50%', '45%'],
        data: [
          { name: 'interactive', value: interactive, itemStyle: { color: '#38bdf8' } },
          { name: 'autonomous',  value: autonomous,  itemStyle: { color: '#a78bfa' } },
        ],
        label: { show: false },
      }],
    }} />
  )
}

// ── Page ──────────────────────────────────────────────────────────────────

export function Overview() {
  const { data: s } = useOverviewStats()
  const { data: c } = useChartsData()
  const { data: g } = useGitStats()

  // Use git data for code metrics (it's the authoritative source)
  // Fall back to execution_log totals if git is unavailable
  const commits      = (g?.total_commits     ?? 0) || (c?.total_commits      ?? 0)
  const linesAdded   = (g?.total_added       ?? 0) || (c?.total_lines_added  ?? 0)
  const linesRemoved = (g?.total_removed     ?? 0) || (c?.total_lines_removed ?? 0)
  const files        = (g?.total_files       ?? 0) || (c?.total_files_changed ?? 0)

  // Merge git + execution_log productivity for charts
  const productivity = mergeProductivity(c?.productivity ?? [], g?.hourly_buckets ?? [])

  const totalTok = (s?.input_tokens ?? 0) + (s?.output_tokens ?? 0) +
                   (s?.cache_read_tokens ?? 0) + (s?.cache_create_tokens ?? 0)

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

          {/* Row 2 — productivity stats */}
          <div className='grid grid-cols-2 gap-3 md:grid-cols-4'>
            <Card><CardContent className='pt-4'><Stat label='Repos touched' value={String(c?.repos_touched ?? 0)} /></CardContent></Card>
            <Card><CardContent className='pt-4'><Stat label='PRs opened'    value={String(c?.total_prs_opened ?? 0)} accent='violet' /></CardContent></Card>
            <Card><CardContent className='pt-4'><Stat label='Interactive'   value={String(c?.interactive_runs ?? 0)} sub='sessions' accent='sky' /></CardContent></Card>
            <Card><CardContent className='pt-4'><Stat label='Autonomous'    value={String(c?.autonomous_runs ?? 0)} sub='runs' accent='violet' /></CardContent></Card>
          </div>

          {/* Row 3 — activity + cache hit */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Runs per hour</CardTitle></CardHeader>
              <CardContent className='pb-3'>
                {c?.activity?.length ? <ActivityChart data={c.activity} /> : <Empty />}
              </CardContent>
            </Card>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Cache hit rate %</CardTitle></CardHeader>
              <CardContent className='pb-3'>
                {c?.efficiency?.length ? <CacheHitChart data={c.efficiency} /> : <Empty />}
              </CardContent>
            </Card>
          </div>

          {/* Row 4 — efficiency + token ratio */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Avg turns/run · actions/turn</CardTitle></CardHeader>
              <CardContent className='pb-3'>
                {c?.efficiency?.length ? <AvgTurnsChart data={c.efficiency} /> : <Empty />}
              </CardContent>
            </Card>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Cache reuse ratio</CardTitle></CardHeader>
              <CardContent className='pb-3'>
                {c?.tokens?.length ? <TokenRatioChart data={c.tokens} /> : <Empty />}
              </CardContent>
            </Card>
          </div>

          {/* Row 5 — productivity charts */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Lines added / removed</CardTitle></CardHeader>
              <CardContent className='pb-3'>
                {productivity.length ? <LinesChart data={productivity} /> : <Empty />}
              </CardContent>
            </Card>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Commits · tests per hour</CardTitle></CardHeader>
              <CardContent className='pb-3'>
                {productivity.length ? <CommitsChart data={productivity} /> : <Empty />}
              </CardContent>
            </Card>
          </div>

          {/* Row 6 — quality split */}
          <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Terminal reasons</CardTitle></CardHeader>
              <CardContent className='pb-3'>
                {c?.terminal_reasons?.length ? <TerminalReasonsChart data={c.terminal_reasons} /> : <Empty />}
              </CardContent>
            </Card>
            <Card>
              <CardHeader className='pb-1 pt-3'><CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Interactive vs autonomous</CardTitle></CardHeader>
              <CardContent className='pb-3'>
                {(c?.interactive_runs ?? 0) + (c?.autonomous_runs ?? 0) > 0
                  ? <SplitChart interactive={c?.interactive_runs ?? 0} autonomous={c?.autonomous_runs ?? 0} />
                  : <Empty />}
              </CardContent>
            </Card>
          </div>

          {/* Token split bar */}
          {totalTok > 0 && (
            <Card>
              <CardContent className='pt-3 pb-3'>
                <div className='flex items-center justify-between text-xs text-muted-foreground mb-1.5'>
                  <span>Token split · {fmt(totalTok)} total</span>
                  <span>cache read {(s?.cache_hit_rate_pct ?? 0).toFixed(0)}%</span>
                </div>
                <div className='h-3 rounded-full overflow-hidden flex gap-px bg-zinc-900'>
                  {[
                    { v: s?.cache_read_tokens ?? 0,   c: 'bg-emerald-500', l: 'cache read' },
                    { v: s?.input_tokens ?? 0,         c: 'bg-sky-500',     l: 'input' },
                    { v: s?.output_tokens ?? 0,        c: 'bg-violet-500',  l: 'output' },
                    { v: s?.cache_create_tokens ?? 0,  c: 'bg-amber-500',   l: 'cache write' },
                  ].map(({ v, c, l }) => (
                    <div key={l} className={c} style={{ width: `${totalTok > 0 ? (v/totalTok)*100 : 0}%` }} title={`${l}: ${fmt(v)}`} />
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
      </Main>
    </>
  )
}

function Empty() {
  return <div className='h-[160px] flex items-center justify-center text-xs text-muted-foreground'>No data yet</div>
}
