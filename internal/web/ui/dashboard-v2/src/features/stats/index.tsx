import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Card, CardContent } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { EChart } from '@/components/echart'
import { cn } from '@/lib/utils'
import { ActivityChart } from '@/features/overview'
import {
  // types
  type StatsHistory, type DayProductivity, type RangeDays,
  // constants
  RANGES,
  // hooks
  useStatsHistory,
  // helpers
  padDays, fmt, Kpi, Empty, ChartCard,
  // KPI strips
  RunsKpiStrip, EfficiencyKpiStrip, TokensKpiStrip,
  // chart bodies
  RunsBarChart, DurationLineChart, TerminalReasonsChart, RetriesBarChart,
  TurnsLineChart, CacheHitLineChart, ContextPctLineChart, TokensPerTurnLineChart,
  ActionsPerTurnLineChart, CompactionsBarChart,
  TokenStackedBarChart, CacheRatioLineChart, OutputTokensBarChart, ReasoningTokensBarChart,
  ModelsPieChart, AgentsPieChart, TriggerSplitPieChart,
} from './panels'

// /stats page — global view. Wires shared chart components from panels.tsx
// with entityId=null. The only /stats-specific bits kept inline:
//   - ActivityChart at top (uses /api/stats/charts hourly + history daily)
//   - Productivity tab merges git history into execution_log productivity
//     (git is global, not entity-scopable, so this stays here)

// ── /stats-specific: git history merge for Productivity tab ───────────────

interface GitDay { day: string; lines_added: number; lines_removed: number; files_changed: number; commits: number }
interface GitHistory { total_commits: number; total_added: number; total_removed: number; total_files: number; hourly_buckets: { hour: string; lines_added: number; lines_removed: number; files_changed: number; commits: number }[] }

function useGitHistory(days: RangeDays) {
  const param = days === 0 ? 'all=1' : `days=${days}`
  return useQuery({
    queryKey: ['stats', 'git', days],
    queryFn: async () => {
      const d = await fetch(`/api/stats/git?${param}`).then(r => r.json()) as GitHistory
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

// /stats-specific Productivity charts — these merge git data with execution_log
// productivity, so they can't be shared with entity-scoped Stats.

function LinesBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(null, range)
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
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
      series: [
        { name: 'added',   type: 'bar', data: prod.map(d => d.lines_added),    itemStyle: { color: '#10b981' }, stack: 'lines' },
        { name: 'removed', type: 'bar', data: prod.map(d => -d.lines_removed), itemStyle: { color: '#f43f5e' }, stack: 'lines' },
      ],
    }} />
  )
}

function CommitsFilesBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(null, range)
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
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'commits', type: 'bar', data: prod.map(d => d.commits),       itemStyle: { color: '#6366f1' } },
        { name: 'files',   type: 'bar', data: prod.map(d => d.files_changed), itemStyle: { color: '#38bdf8' } },
      ],
    }} />
  )
}

function TestsBarChart({ range }: { range: RangeDays }) {
  const { data } = useStatsHistory(null, range)
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
      xAxis: { type: 'category', data: days, axisLabel: { fontSize: 9 }, boundaryGap: false },
      yAxis: { type: 'value', axisLabel: { fontSize: 9 }, minInterval: 1 },
      series: [
        { name: 'passed', type: 'bar', stack: 't', data: prod.map(d => d.tests_passed), itemStyle: { color: '#10b981' } },
        { name: 'failed', type: 'bar', stack: 't', data: prod.map(d => d.tests_failed), itemStyle: { color: '#f43f5e' } },
      ],
    }} />
  )
}

// ── Tabs ──────────────────────────────────────────────────────────────────

function RunsTab({ data }: { data: StatsHistory }) {
  return (
    <div className='space-y-4'>
      <RunsKpiStrip data={data} />
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
        <ChartCard entityId={null} title='Daily runs — completed vs failed' series={(e, r) => <RunsBarChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Avg duration per day (seconds)'  series={(e, r) => <DurationLineChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Terminal reasons'                 series={(e, r) => <TerminalReasonsChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Avg retry number (run_number) — higher = more retries' series={(e, r) => <RetriesBarChart entityId={e} range={r} />} />
      </div>
    </div>
  )
}

function EfficiencyTab({ data }: { data: StatsHistory }) {
  return (
    <div className='space-y-4'>
      <EfficiencyKpiStrip data={data} />
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
        <ChartCard entityId={null} title='Avg turns per run'                                    series={(e, r) => <TurnsLineChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Cache hit rate %'                                     series={(e, r) => <CacheHitLineChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Avg context window used %'                            series={(e, r) => <ContextPctLineChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Avg tokens per turn'                                  series={(e, r) => <TokensPerTurnLineChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Actions per turn (efficiency — higher = less deliberation)' series={(e, r) => <ActionsPerTurnLineChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Compactions per day (context resets — 0 is best)'     series={(e, r) => <CompactionsBarChart entityId={e} range={r} />} />
      </div>
    </div>
  )
}

function TokensTab({ data }: { data: StatsHistory }) {
  return (
    <div className='space-y-4'>
      <TokensKpiStrip data={data} />
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
        <ChartCard entityId={null} title='Daily token volumes (stacked)'                                   series={(e, r) => <TokenStackedBarChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Cache read/write ratio (higher = better cache compounding)'      series={(e, r) => <CacheRatioLineChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Output tokens per day'                                           series={(e, r) => <OutputTokensBarChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Reasoning tokens per day (Opus extended thinking)'               series={(e, r) => <ReasoningTokensBarChart entityId={e} range={r} />} />
      </div>
    </div>
  )
}

function ProductivityTab({ data, range }: { data: StatsHistory; range: RangeDays }) {
  const git = useGitHistory(range)
  const t = data.totals
  const g = git.data
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
        <ChartCard entityId={null} title='Lines added / removed per day'      series={(_e, r) => <LinesBarChart range={r} />} />
        <ChartCard entityId={null} title='Commits + files changed per day'     series={(_e, r) => <CommitsFilesBarChart range={r} />} />
        <ChartCard entityId={null} title='Tests run / passed / failed per day' series={(_e, r) => <TestsBarChart range={r} />} />
      </div>
    </div>
  )
}

function AgentsTab() {
  return (
    <div className='space-y-4'>
      <div className='grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3'>
        <ChartCard entityId={null} title='Runs by model'              series={(e, r) => <ModelsPieChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Runs by agent runtime'      series={(e, r) => <AgentsPieChart entityId={e} range={r} />} />
        <ChartCard entityId={null} title='Interactive vs autonomous'  series={(e, r) => <TriggerSplitPieChart entityId={e} range={r} />} />
      </div>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────

export function StatsPage() {
  const [range, setRange] = useState<RangeDays>(7)
  const { data, isLoading } = useStatsHistory(null, range)

  return (
    <>
      <Header />
      <Main>
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
            <TabsContent value='runs'         className='mt-4'><RunsTab         data={data} /></TabsContent>
            <TabsContent value='efficiency'   className='mt-4'><EfficiencyTab   data={data} /></TabsContent>
            <TabsContent value='tokens'       className='mt-4'><TokensTab       data={data} /></TabsContent>
            <TabsContent value='productivity' className='mt-4'><ProductivityTab data={data} range={range} /></TabsContent>
            <TabsContent value='agents'       className='mt-4'><AgentsTab /></TabsContent>
          </Tabs>
        ) : null}
      </Main>
    </>
  )
}
