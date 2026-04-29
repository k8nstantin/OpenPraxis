import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { EChart } from '@/components/echart'
import { cn } from '@/lib/utils'

// Front-page panels — three side-by-side cards over live data:
//   - TasksPanel:   what's running RIGHT NOW (count, list, budget)
//   - AIStatsPanel: cumulative agent metrics (turns, actions, cost trend)
//   - SystemPanel:  host-level CPU/mem/load/io
//
// Pollers:
//   /api/tasks?status=running   1.5s   running list
//   /api/tasks/stats            3s     counts + budget + turns_today
//   /api/system-stats?limit=120 2s     host time series
//   /api/actions/search         3s     total + recent for action volume

interface RunningTask {
  id: string
  title: string
  status: string
  manifest_id?: string
}

interface TasksStats {
  budget_exceeded: boolean
  budget_pct: number
  cost_today: number
  daily_budget: number
  running: number
  tasks_total: number
  turns_today: number
}

interface SystemSample {
  ts: string
  cpu_pct: number
  load_1m: number
  load_5m: number
  load_15m: number
  mem_used_mb: number
  mem_total_mb: number
  swap_used_mb: number
  disk_used_gb: number
  disk_total_gb: number
  net_rx_mbps: number
  net_tx_mbps: number
  disk_read_mbps: number
  disk_write_mbps: number
}

interface ActionRow {
  id: string
  tool_name: string
  source_node: string
  created_at: string
}

interface ActionsSearchEnvelope {
  items: ActionRow[] | null
  total: number
}

function useRunningTasks() {
  return useQuery({
    queryKey: ['running-tasks-list'],
    queryFn: async () => {
      const r = await fetch('/api/tasks?status=running')
      if (!r.ok) throw new Error(`tasks → ${r.status}`)
      return ((await r.json()) as RunningTask[] | null) ?? []
    },
    refetchInterval: 1500,
    staleTime: 0,
  })
}

function useTasksStats() {
  return useQuery({
    queryKey: ['tasks-stats'],
    queryFn: () => fetch('/api/tasks/stats').then((r) => r.json() as Promise<TasksStats>),
    refetchInterval: 3000,
    staleTime: 0,
  })
}

function useSystemStats(window: number = 120) {
  return useQuery({
    queryKey: ['system-stats', window],
    queryFn: async () => {
      const r = await fetch(`/api/system-stats?limit=${window}`)
      if (!r.ok) throw new Error(`system-stats → ${r.status}`)
      const env = (await r.json()) as { samples: SystemSample[] }
      return env.samples ?? []
    },
    refetchInterval: 2000,
    staleTime: 0,
  })
}

function useActionsRecent() {
  return useQuery({
    queryKey: ['actions-recent-300'],
    queryFn: async () => {
      const r = await fetch('/api/actions/search?limit=300')
      if (!r.ok) throw new Error(`actions/search → ${r.status}`)
      return (await r.json()) as ActionsSearchEnvelope
    },
    refetchInterval: 3000,
    staleTime: 0,
  })
}

// Top-level panel container — three side-by-side cards on wide screens,
// stacked on narrow.
export function RunningTasksPanel() {
  return (
    <div className='grid grid-cols-1 gap-3 xl:grid-cols-3'>
      <TasksPanel />
      <AIStatsPanel />
      <SystemPanel />
    </div>
  )
}

// ── 1. Tasks panel ───────────────────────────────────────────────────
function TasksPanel() {
  const tasks = useRunningTasks()
  const stats = useTasksStats()
  const running = tasks.data ?? []
  const s = stats.data

  return (
    <PanelCard
      title='Tasks'
      badge='live'
      badgeTone='emerald'
      stats={[
        { label: 'Running', value: String(running.length), accent: 'emerald' },
        { label: 'Total', value: s ? String(s.tasks_total) : '—' },
        {
          label: 'Budget',
          value: s ? `${(s.budget_pct ?? 0).toFixed(0)}%` : '—',
          accent: s && s.budget_pct > 80 ? 'rose' : undefined,
        },
      ]}
    >
      <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
        <ChartTile label='Running count' note='0…N tasks live'>
          <RunningGauge running={running.length} max={Math.max(8, running.length + 2)} />
        </ChartTile>
        <ChartTile label='Daily budget' note={`$${s?.cost_today.toFixed(2) ?? '0.00'} of $${s?.daily_budget ?? 100}`}>
          <BudgetRing used={s?.cost_today ?? 0} budget={s?.daily_budget ?? 100} />
        </ChartTile>
        <ChartTile label='Running list' note='one bar per task' span={2}>
          <RunningTaskBars running={running} />
        </ChartTile>
      </div>
    </PanelCard>
  )
}

// ── 2. AI Stats panel ────────────────────────────────────────────────
function AIStatsPanel() {
  const stats = useTasksStats()
  const actions = useActionsRecent()
  const s = stats.data
  const env = actions.data

  // Bucket recent actions per hour for the volume bar chart.
  const hourly = useMemo(() => {
    const buckets = new Map<string, number>()
    for (const a of env?.items ?? []) {
      // YYYY-MM-DD HH key
      const k = a.created_at.slice(0, 13)
      buckets.set(k, (buckets.get(k) ?? 0) + 1)
    }
    return Array.from(buckets.entries())
      .sort((a, b) => (a[0] < b[0] ? -1 : 1))
      .slice(-12) // last 12 hours
  }, [env])

  // Bucket actions by tool_name for the breakdown.
  const toolBreakdown = useMemo(() => {
    const t = new Map<string, number>()
    for (const a of env?.items ?? []) {
      t.set(a.tool_name, (t.get(a.tool_name) ?? 0) + 1)
    }
    return Array.from(t.entries())
      .sort((a, b) => b[1] - a[1])
      .slice(0, 8)
  }, [env])

  return (
    <PanelCard
      title='AI Stats'
      badge='cumulative'
      badgeTone='violet'
      stats={[
        { label: 'Actions·all', value: env ? env.total.toLocaleString() : '—', accent: 'violet' },
        { label: 'Turns·today', value: s ? String(s.turns_today) : '—' },
        { label: 'Cost·today', value: s ? `$${s.cost_today.toFixed(2)}` : '—' },
      ]}
    >
      <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
        <ChartTile label='Actions / hour' note='last 12 hr buckets' span={2}>
          <HourlyBars data={hourly} color='#a78bfa' />
        </ChartTile>
        <ChartTile label='Top tools' note='in last 300 actions'>
          <ToolBreakdownBars data={toolBreakdown} />
        </ChartTile>
        <ChartTile label='Turns ratio' note='today / budget'>
          <TurnsRatio
            turns={s?.turns_today ?? 0}
            budget={s?.daily_budget ?? 100}
            cost={s?.cost_today ?? 0}
          />
        </ChartTile>
      </div>
    </PanelCard>
  )
}

// ── 3. System panel ──────────────────────────────────────────────────
function SystemPanel() {
  const samples = useSystemStats(120)
  const series = samples.data ?? []
  const last = series.length ? series[series.length - 1] : null

  return (
    <PanelCard
      title='System'
      badge='host'
      badgeTone='sky'
      stats={[
        {
          label: 'CPU',
          value: last ? `${last.cpu_pct.toFixed(1)}%` : '—',
          accent: last && last.cpu_pct > 80 ? 'rose' : 'sky',
        },
        {
          label: 'Mem',
          value: last ? `${(last.mem_used_mb / 1024).toFixed(1)}G` : '—',
        },
        { label: 'Load·1m', value: last ? last.load_1m.toFixed(2) : '—' },
      ]}
    >
      <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
        <ChartTile label='CPU' note='last 4 min'>
          <SystemSparkline series={series} field='cpu_pct' color='#34d399' unit='%' />
        </ChartTile>
        <ChartTile label='Memory' note='used over time'>
          <SystemArea series={series} />
        </ChartTile>
        <ChartTile label='Load avg' note='1m / 5m / 15m'>
          <LoadLines series={series} />
        </ChartTile>
        <ChartTile label='Throughput' note='disk + net mbps'>
          <ThroughputBars series={series} />
        </ChartTile>
      </div>
    </PanelCard>
  )
}

// ── primitives ───────────────────────────────────────────────────────
function PanelCard({
  title,
  badge,
  badgeTone,
  stats,
  children,
}: {
  title: string
  badge?: string
  badgeTone?: 'emerald' | 'violet' | 'sky'
  stats?: { label: string; value: string; accent?: 'emerald' | 'violet' | 'sky' | 'rose' }[]
  children: React.ReactNode
}) {
  const badgeBorder =
    badgeTone === 'emerald'
      ? 'border-emerald-500/40 text-emerald-300'
      : badgeTone === 'violet'
        ? 'border-violet-500/40 text-violet-300'
        : 'border-sky-500/40 text-sky-300'
  return (
    <Card className='gap-0 py-0'>
      <CardHeader className='flex flex-col gap-2 border-b py-3'>
        <div className='flex items-center justify-between gap-2'>
          <div className='flex items-center gap-2'>
            <CardTitle className='text-sm'>{title}</CardTitle>
            {badge && (
              <Badge variant='outline' className={cn('text-[10px]', badgeBorder)}>
                {badge}
              </Badge>
            )}
          </div>
        </div>
        {stats && (
          <div className='flex flex-wrap items-baseline gap-x-3 gap-y-1 text-xs'>
            {stats.map((st) => (
              <Stat key={st.label} {...st} />
            ))}
          </div>
        )}
      </CardHeader>
      <CardContent className='p-3'>{children}</CardContent>
    </Card>
  )
}

function ChartTile({
  label,
  note,
  span,
  children,
}: {
  label: string
  note?: string
  span?: number
  children: React.ReactNode
}) {
  return (
    <div
      className={cn(
        'flex flex-col gap-1 rounded-md border bg-background/40 p-2',
        span === 2 && 'md:col-span-2'
      )}
    >
      <div className='flex items-baseline justify-between'>
        <span className='text-xs font-medium'>{label}</span>
        {note && <span className='text-muted-foreground text-[10px]'>{note}</span>}
      </div>
      <div className='h-[160px]'>{children}</div>
    </div>
  )
}

function Stat({
  label,
  value,
  accent,
}: {
  label: string
  value: string
  accent?: 'emerald' | 'violet' | 'sky' | 'rose'
}) {
  return (
    <div className='flex items-baseline gap-1'>
      <span className='text-muted-foreground'>{label}</span>
      <span
        className={cn(
          'font-mono font-medium tabular-nums',
          accent === 'emerald' && 'text-emerald-300',
          accent === 'violet' && 'text-violet-300',
          accent === 'sky' && 'text-sky-300',
          accent === 'rose' && 'text-rose-300'
        )}
      >
        {value}
      </span>
    </div>
  )
}

// ── chart components (unchanged from prior iteration except formatting) ─

function RunningGauge({ running, max }: { running: number; max: number }) {
  return (
    <EChart
      height='100%'
      option={{
        series: [
          {
            type: 'gauge',
            startAngle: 220,
            endAngle: -40,
            min: 0,
            max,
            itemStyle: { color: '#10b981' },
            progress: { show: true, width: 12 },
            axisLine: { lineStyle: { width: 12, color: [[1, '#27272a']] } },
            axisTick: { show: false },
            splitLine: { length: 6, lineStyle: { color: '#71717a' } },
            axisLabel: { distance: 14, color: '#a1a1aa', fontSize: 9 },
            pointer: { show: false },
            anchor: { show: false },
            title: { show: false },
            detail: {
              valueAnimation: true,
              formatter: '{value}',
              fontSize: 24,
              fontWeight: 600,
              color: '#10b981',
              offsetCenter: [0, '20%'],
            },
            data: [{ value: running, name: 'running' }],
          },
        ],
      }}
    />
  )
}

function BudgetRing({ used, budget }: { used: number; budget: number }) {
  const pct = budget > 0 ? Math.min(100, (used / budget) * 100) : 0
  const color = pct > 80 ? '#f43f5e' : pct > 50 ? '#f59e0b' : '#10b981'
  return (
    <EChart
      height='100%'
      option={{
        series: [
          {
            type: 'gauge',
            startAngle: 90,
            endAngle: -270,
            min: 0,
            max: 100,
            radius: '90%',
            progress: { show: true, width: 12, roundCap: true, itemStyle: { color } },
            axisLine: { lineStyle: { width: 12, color: [[1, '#27272a']] } },
            axisTick: { show: false },
            splitLine: { show: false },
            axisLabel: { show: false },
            pointer: { show: false },
            title: { show: false },
            detail: {
              valueAnimation: true,
              fontSize: 20,
              fontWeight: 600,
              offsetCenter: [0, '0%'],
              formatter: () => `$${used.toFixed(2)}`,
              color: '#e5e7eb',
            },
            data: [{ value: pct }],
          },
        ],
      }}
    />
  )
}

function RunningTaskBars({ running }: { running: RunningTask[] }) {
  if (running.length === 0) {
    return (
      <div className='text-muted-foreground flex h-full items-center justify-center text-xs'>
        no running tasks
      </div>
    )
  }
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 100, right: 24, top: 8, bottom: 18 },
        xAxis: { type: 'value', show: false },
        yAxis: {
          type: 'category',
          data: running.map((t) => truncate(t.title, 20)),
          axisLabel: { fontSize: 10 },
        },
        series: [
          {
            type: 'bar',
            data: running.map(() => 1),
            itemStyle: { color: '#10b981' },
            barWidth: 12,
            label: {
              show: true,
              position: 'right',
              fontSize: 10,
              formatter: 'running',
              color: '#10b981',
            },
          },
        ],
      }}
    />
  )
}

function HourlyBars({
  data,
  color,
}: {
  data: [string, number][]
  color: string
}) {
  if (data.length === 0) {
    return (
      <div className='text-muted-foreground flex h-full items-center justify-center text-xs'>
        no recent actions
      </div>
    )
  }
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 32, right: 8, top: 16, bottom: 24 },
        tooltip: { trigger: 'axis' },
        xAxis: {
          type: 'category',
          data: data.map(([k]) => k.slice(11, 13) + 'h'),
          axisLabel: { fontSize: 9 },
        },
        yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
        series: [
          {
            type: 'bar',
            data: data.map(([, v]) => v),
            itemStyle: { color },
            barWidth: '70%',
          },
        ],
      }}
    />
  )
}

function ToolBreakdownBars({ data }: { data: [string, number][] }) {
  if (data.length === 0) {
    return (
      <div className='text-muted-foreground flex h-full items-center justify-center text-xs'>
        no recent actions
      </div>
    )
  }
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 110, right: 24, top: 4, bottom: 18 },
        tooltip: { trigger: 'item' },
        xAxis: { type: 'value', show: false },
        yAxis: {
          type: 'category',
          data: data.map(([k]) => truncate(k, 20)),
          axisLabel: { fontSize: 10 },
          inverse: true,
        },
        series: [
          {
            type: 'bar',
            data: data.map(([, v]) => v),
            itemStyle: { color: '#a78bfa' },
            barWidth: 10,
            label: {
              show: true,
              position: 'right',
              fontSize: 10,
              color: '#a1a1aa',
            },
          },
        ],
      }}
    />
  )
}

function TurnsRatio({
  turns,
  budget,
  cost,
}: {
  turns: number
  budget: number
  cost: number
}) {
  const costPct = budget > 0 ? Math.min(100, (cost / budget) * 100) : 0
  return (
    <EChart
      height='100%'
      option={{
        series: [
          {
            type: 'gauge',
            startAngle: 200,
            endAngle: -20,
            min: 0,
            max: 100,
            progress: {
              show: true,
              width: 12,
              roundCap: true,
              itemStyle: { color: '#a78bfa' },
            },
            axisLine: { lineStyle: { width: 12, color: [[1, '#27272a']] } },
            axisTick: { show: false },
            splitLine: { show: false },
            axisLabel: { show: false },
            pointer: { show: false },
            title: { show: false },
            detail: {
              valueAnimation: true,
              fontSize: 22,
              fontWeight: 600,
              offsetCenter: [0, '5%'],
              formatter: () => String(turns),
              color: '#e5e7eb',
            },
            data: [{ value: costPct }],
          },
        ],
        graphic: [
          {
            type: 'text',
            left: 'center',
            top: '70%',
            style: {
              text: 'turns today',
              fill: '#94a3b8',
              fontSize: 10,
            },
          },
        ],
      }}
    />
  )
}

function SystemSparkline({
  series,
  field,
  color,
  unit,
}: {
  series: SystemSample[]
  field: keyof SystemSample
  color: string
  unit?: string
}) {
  const data = series.map((s) => [s.ts, Number(s[field] ?? 0)])
  const last = data.length ? data[data.length - 1][1] : 0
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 36, right: 8, top: 24, bottom: 18 },
        xAxis: { type: 'time', axisLabel: { fontSize: 9 } },
        yAxis: {
          type: 'value',
          axisLabel: { fontSize: 9, formatter: `{value}${unit ?? ''}` },
        },
        graphic: [
          {
            type: 'text',
            right: 8,
            top: 4,
            style: {
              text: `${(last as number).toFixed(1)}${unit ?? ''}`,
              fill: color,
              fontSize: 14,
              fontWeight: 600,
            },
          },
        ],
        series: [
          {
            type: 'line',
            data,
            smooth: true,
            showSymbol: false,
            lineStyle: { color, width: 2 },
            areaStyle: {
              color: {
                type: 'linear' as const,
                x: 0,
                y: 0,
                x2: 0,
                y2: 1,
                colorStops: [
                  { offset: 0, color: color + 'aa' },
                  { offset: 1, color: color + '00' },
                ],
              },
            },
          },
        ],
      }}
    />
  )
}

function SystemArea({ series }: { series: SystemSample[] }) {
  const used = series.map((s) => [s.ts, s.mem_used_mb / 1024])
  const total = series.length ? series[series.length - 1].mem_total_mb / 1024 : 16
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 36, right: 8, top: 18, bottom: 18 },
        xAxis: { type: 'time', axisLabel: { fontSize: 9 } },
        yAxis: {
          type: 'value',
          max: total,
          axisLabel: { fontSize: 9, formatter: '{value}G' },
        },
        series: [
          {
            type: 'line',
            data: used,
            smooth: true,
            showSymbol: false,
            lineStyle: { color: '#a78bfa', width: 1.5 },
            areaStyle: {
              color: {
                type: 'linear' as const,
                x: 0,
                y: 0,
                x2: 0,
                y2: 1,
                colorStops: [
                  { offset: 0, color: '#a78bfaaa' },
                  { offset: 1, color: '#a78bfa00' },
                ],
              },
            },
          },
        ],
      }}
    />
  )
}

function LoadLines({ series }: { series: SystemSample[] }) {
  const colors = { '1m': '#3b82f6', '5m': '#10b981', '15m': '#f59e0b' }
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 32, right: 8, top: 24, bottom: 18 },
        legend: {
          show: true,
          top: 0,
          right: 0,
          itemWidth: 8,
          itemHeight: 8,
          textStyle: { fontSize: 9, color: '#a1a1aa' },
        },
        xAxis: { type: 'time', axisLabel: { fontSize: 9 } },
        yAxis: { type: 'value', axisLabel: { fontSize: 9 } },
        series: [
          {
            name: '1m',
            type: 'line',
            data: series.map((s) => [s.ts, s.load_1m]),
            smooth: true,
            showSymbol: false,
            lineStyle: { color: colors['1m'], width: 1.5 },
          },
          {
            name: '5m',
            type: 'line',
            data: series.map((s) => [s.ts, s.load_5m]),
            smooth: true,
            showSymbol: false,
            lineStyle: { color: colors['5m'], width: 1.5 },
          },
          {
            name: '15m',
            type: 'line',
            data: series.map((s) => [s.ts, s.load_15m]),
            smooth: true,
            showSymbol: false,
            lineStyle: { color: colors['15m'], width: 1.5 },
          },
        ],
      }}
    />
  )
}

function ThroughputBars({ series }: { series: SystemSample[] }) {
  const last = series.length ? series[series.length - 1] : null
  const data = last
    ? [
        { name: 'disk read', value: last.disk_read_mbps, color: '#0ea5e9' },
        { name: 'disk write', value: last.disk_write_mbps, color: '#6366f1' },
        { name: 'net rx', value: last.net_rx_mbps, color: '#10b981' },
        { name: 'net tx', value: last.net_tx_mbps, color: '#f59e0b' },
      ]
    : []
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 80, right: 24, top: 8, bottom: 18 },
        xAxis: {
          type: 'value',
          axisLabel: { fontSize: 9, formatter: '{value}M' },
        },
        yAxis: {
          type: 'category',
          data: data.map((d) => d.name),
          axisLabel: { fontSize: 10 },
        },
        series: [
          {
            type: 'bar',
            data: data.map((d) => ({ value: d.value, itemStyle: { color: d.color } })),
            label: {
              show: true,
              position: 'right',
              fontSize: 10,
              formatter: (p: { value: number }) => p.value.toFixed(2),
            },
          },
        ],
      }}
    />
  )
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}
