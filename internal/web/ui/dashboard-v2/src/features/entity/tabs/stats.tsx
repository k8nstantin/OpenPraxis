import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { EntityKind } from '@/lib/queries/entity'
import {
  useStatsHistory, ChartCard,
  RunsKpiStrip, EfficiencyKpiStrip, TokensKpiStrip,
  RunsBarChart, DurationLineChart, TerminalReasonsChart, RetriesBarChart,
  TurnsLineChart, CacheHitLineChart, ContextPctLineChart, TokensPerTurnLineChart,
  ActionsPerTurnLineChart, CompactionsBarChart,
  TokenStackedBarChart, CacheRatioLineChart, OutputTokensBarChart, ReasoningTokensBarChart,
  ModelsPieChart, AgentsPieChart, TriggerSplitPieChart,
  padDays, fmt, Kpi, Empty,
} from '@/features/stats/panels'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EChart } from '@/components/echart'

// Option 2: shared chart components from features/stats/panels.tsx.
// Identical UX to /stats — each chart has its own per-chart range
// selector. Only difference: entityId is non-null, so charts hit
// /api/entities/{id}/stats instead of /api/stats/history.
//
// Productivity tab: uses execution_log directly (no git merge — git
// history isn't entity-scopable). Falls back to inline simple charts
// for lines / commits / files / tests.

function ProductivityCharts({ entityId }: { entityId: string }) {
  return (
    <>
      <ChartCard entityId={entityId} title='Lines added / removed per day'
        series={(e, r) => <LinesChart entityId={e} range={r} />} />
      <ChartCard entityId={entityId} title='Commits + files changed per day'
        series={(e, r) => <CommitsFilesChart entityId={e} range={r} />} />
      <ChartCard entityId={entityId} title='Tests passed / failed per day'
        series={(e, r) => <TestsChart entityId={e} range={r} />} />
    </>
  )
}

function LinesChart({ entityId, range }: { entityId: string | null; range: number }) {
  const { data } = useStatsHistory(entityId, range as 1 | 2 | 3 | 7 | 14 | 30 | 90 | 0)
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

function CommitsFilesChart({ entityId, range }: { entityId: string | null; range: number }) {
  const { data } = useStatsHistory(entityId, range as 1 | 2 | 3 | 7 | 14 | 30 | 90 | 0)
  if (!data?.productivity) return <Empty />
  const prod = padDays(data.productivity, d => ({ day: d, lines_added: 0, lines_removed: 0, files_changed: 0, commits: 0, tests_run: 0, tests_passed: 0, tests_failed: 0, prs_opened: 0 }))
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

function TestsChart({ entityId, range }: { entityId: string | null; range: number }) {
  const { data } = useStatsHistory(entityId, range as 1 | 2 | 3 | 7 | 14 | 30 | 90 | 0)
  if (!data?.productivity) return <Empty />
  const prod = padDays(data.productivity, d => ({ day: d, lines_added: 0, lines_removed: 0, files_changed: 0, commits: 0, tests_run: 0, tests_passed: 0, tests_failed: 0, prs_opened: 0 }))
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

function ProductivityKpis({ entityId }: { entityId: string }) {
  const { data } = useStatsHistory(entityId, 7)
  if (!data) return null
  const t = data.totals
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
  // KPI strips read from a single 7-day window for stable totals; charts
  // each have their own per-chart range selector via ChartCard.
  const { data, isLoading, isError, error } = useStatsHistory(entityId, 7)

  return (
    <div className='space-y-3'>
      <Card>
        <CardHeader className='pb-2 pt-3'>
          <CardTitle className='text-xs text-muted-foreground uppercase tracking-wider'>Scope</CardTitle>
        </CardHeader>
        <CardContent className='pb-3 text-xs text-muted-foreground'>
          This entity's run set (descendants included for manifest / product). KPI strips use a 7-day window; each chart has its own range selector.
        </CardContent>
      </Card>

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
            <RunsKpiStrip data={data} />
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <ChartCard entityId={entityId} title='Daily runs — completed vs failed' series={(e, r) => <RunsBarChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Avg duration per day (seconds)' series={(e, r) => <DurationLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Terminal reasons' series={(e, r) => <TerminalReasonsChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Avg retry number (run_number) — higher = more retries' series={(e, r) => <RetriesBarChart entityId={e} range={r} />} />
            </div>
          </TabsContent>

          <TabsContent value='efficiency' className='mt-4 space-y-4'>
            <EfficiencyKpiStrip data={data} />
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <ChartCard entityId={entityId} title='Avg turns per run' series={(e, r) => <TurnsLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Cache hit rate %' series={(e, r) => <CacheHitLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Avg context window used %' series={(e, r) => <ContextPctLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Avg tokens per turn' series={(e, r) => <TokensPerTurnLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Actions per turn (efficiency — higher = less deliberation)' series={(e, r) => <ActionsPerTurnLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Compactions per day (context resets — 0 is best)' series={(e, r) => <CompactionsBarChart entityId={e} range={r} />} />
            </div>
          </TabsContent>

          <TabsContent value='tokens' className='mt-4 space-y-4'>
            <TokensKpiStrip data={data} />
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <ChartCard entityId={entityId} title='Daily token volumes (stacked)' series={(e, r) => <TokenStackedBarChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Cache read/write ratio (higher = better cache compounding)' series={(e, r) => <CacheRatioLineChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Output tokens per day' series={(e, r) => <OutputTokensBarChart entityId={e} range={r} />} />
              <ChartCard entityId={entityId} title='Reasoning tokens per day (extended thinking)' series={(e, r) => <ReasoningTokensBarChart entityId={e} range={r} />} />
            </div>
          </TabsContent>

          <TabsContent value='productivity' className='mt-4 space-y-4'>
            <ProductivityKpis entityId={entityId} />
            <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
              <ProductivityCharts entityId={entityId} />
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
