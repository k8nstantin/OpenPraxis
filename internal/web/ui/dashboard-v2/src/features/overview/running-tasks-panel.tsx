import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { EChart } from '@/components/echart'
import { cn } from '@/lib/utils'

// Running-tasks chart panel — eight different ways to look at the
// same live data so the operator can pick which views earn a
// permanent slot on the front page. All charts share three pollers:
//   - GET /api/tasks?status=running    (1.5s)  → list of running tasks
//   - GET /api/tasks/stats              (3s)   → counts + budget
//   - GET /api/system-stats?limit=120  (2s)   → host CPU/load/mem time series
//
// Goal of the layout is comparative: render every variant simultaneously
// at the same height so judgments are about the chart kind, not the
// data. Once the user picks favourites we collapse to ~3 and promote
// them to the real overview header strip.

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

interface SystemStatsResponse {
  samples: SystemSample[]
}

function useRunningTasks() {
  return useQuery({
    queryKey: ['running-tasks-list'],
    queryFn: async () => {
      const res = await fetch('/api/tasks?status=running')
      if (!res.ok) throw new Error(`tasks → ${res.status}`)
      const j = (await res.json()) as RunningTask[] | null
      return j ?? []
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
      const env = (await r.json()) as SystemStatsResponse
      return env.samples ?? []
    },
    refetchInterval: 2000,
    staleTime: 0,
  })
}

export function RunningTasksPanel() {
  const tasks = useRunningTasks()
  const stats = useTasksStats()
  const samples = useSystemStats(120)

  const running = tasks.data ?? []
  const s = stats.data
  const series = samples.data ?? []

  return (
    <Card className='gap-0 py-0'>
      <CardHeader className='flex flex-row items-center justify-between gap-2 border-b py-3'>
        <div className='flex items-center gap-2'>
          <CardTitle className='text-base'>Running Tasks — chart options</CardTitle>
          <Badge
            variant='outline'
            className='text-[10px] border-emerald-500/40 text-emerald-300'
          >
            live
          </Badge>
        </div>
        <div className='flex items-baseline gap-3 text-xs'>
          <Stat label='Running' value={String(running.length)} accent />
          <Stat label='Total' value={s ? String(s.tasks_total) : '—'} />
          <Stat
            label='Cost today'
            value={s ? `$${s.cost_today.toFixed(2)} / $${s.daily_budget}` : '—'}
            tone={s && s.budget_pct > 80 ? 'rose' : undefined}
          />
          <Stat label='Turns today' value={s ? String(s.turns_today) : '—'} />
        </div>
      </CardHeader>
      <CardContent className='p-3'>
        <div className='grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4'>
          <ChartTile
            label='1 — Running count gauge'
            note='liquid-fill ring; 0…N tasks'
          >
            <RunningGauge running={running.length} max={Math.max(8, running.length + 2)} />
          </ChartTile>

          <ChartTile
            label='2 — Budget burn ring'
            note='daily $ used vs daily budget'
          >
            <BudgetRing
              used={s?.cost_today ?? 0}
              budget={s?.daily_budget ?? 100}
            />
          </ChartTile>

          <ChartTile
            label='3 — System CPU sparkline'
            note='last ~4 min'
          >
            <SystemSparkline series={series} field='cpu_pct' color='#34d399' unit='%' />
          </ChartTile>

          <ChartTile
            label='4 — System memory area'
            note='used / total over time'
          >
            <SystemArea series={series} />
          </ChartTile>

          <ChartTile
            label='5 — Load average lines'
            note='1m / 5m / 15m'
          >
            <LoadLines series={series} />
          </ChartTile>

          <ChartTile
            label='6 — Disk + network bars'
            note='live throughput'
          >
            <ThroughputBars series={series} />
          </ChartTile>

          <ChartTile
            label='7 — Running task list'
            note='one bar per task'
          >
            <RunningTaskBars running={running} />
          </ChartTile>

          <ChartTile
            label='8 — CPU radial heat'
            note='last 60 ticks of CPU'
          >
            <CPURadialHeat series={series} />
          </ChartTile>
        </div>
      </CardContent>
    </Card>
  )
}

function ChartTile({
  label,
  note,
  children,
}: {
  label: string
  note?: string
  children: React.ReactNode
}) {
  return (
    <div className='flex flex-col gap-1 rounded-md border bg-background/40 p-2'>
      <div className='flex items-baseline justify-between'>
        <span className='text-xs font-medium'>{label}</span>
        {note && <span className='text-muted-foreground text-[10px]'>{note}</span>}
      </div>
      <div className='h-[180px]'>{children}</div>
    </div>
  )
}

function Stat({
  label,
  value,
  accent,
  tone,
}: {
  label: string
  value: string
  accent?: boolean
  tone?: 'rose'
}) {
  return (
    <div className='flex items-baseline gap-1'>
      <span className='text-muted-foreground'>{label}</span>
      <span
        className={cn(
          'font-mono font-medium tabular-nums',
          accent && 'text-emerald-300',
          tone === 'rose' && 'text-rose-300'
        )}
      >
        {value}
      </span>
    </div>
  )
}

// ── Chart 1 — Running gauge ──────────────────────────────────────────
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
            progress: { show: true, width: 14 },
            axisLine: { lineStyle: { width: 14, color: [[1, '#27272a']] } },
            axisTick: { show: false },
            splitLine: { length: 8, lineStyle: { color: '#71717a' } },
            axisLabel: { distance: 18, color: '#a1a1aa', fontSize: 9 },
            pointer: { show: false },
            anchor: { show: false },
            title: { show: false },
            detail: {
              valueAnimation: true,
              formatter: '{value}',
              fontSize: 28,
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

// ── Chart 2 — Budget burn ring ────────────────────────────────────────
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
            progress: {
              show: true,
              width: 14,
              roundCap: true,
              itemStyle: { color },
            },
            axisLine: { lineStyle: { width: 14, color: [[1, '#27272a']] } },
            axisTick: { show: false },
            splitLine: { show: false },
            axisLabel: { show: false },
            pointer: { show: false },
            title: { show: false },
            detail: {
              valueAnimation: true,
              fontSize: 24,
              fontWeight: 600,
              offsetCenter: [0, '0%'],
              formatter: () => `$${used.toFixed(2)}`,
              color: '#e5e7eb',
            },
            data: [{ value: pct }],
          },
        ],
        graphic: [
          {
            type: 'text',
            left: 'center',
            top: '70%',
            style: {
              text: `of $${budget} (${pct.toFixed(0)}%)`,
              fill: '#94a3b8',
              fontSize: 11,
            },
          },
        ],
      }}
    />
  )
}

// ── Chart 3 — System sparkline (one metric) ───────────────────────────
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
        xAxis: {
          type: 'time',
          axisLabel: { fontSize: 9 },
        },
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

// ── Chart 4 — Memory area used vs total ──────────────────────────────
function SystemArea({ series }: { series: SystemSample[] }) {
  const used = series.map((s) => [s.ts, s.mem_used_mb / 1024])
  const total = series.length ? series[series.length - 1].mem_total_mb / 1024 : 16
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 40, right: 8, top: 18, bottom: 18 },
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

// ── Chart 5 — Load average lines ─────────────────────────────────────
function LoadLines({ series }: { series: SystemSample[] }) {
  const colors = { '1m': '#3b82f6', '5m': '#10b981', '15m': '#f59e0b' }
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 36, right: 8, top: 24, bottom: 18 },
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

// ── Chart 6 — Throughput bars (disk r/w + net rx/tx) ─────────────────
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
            data: data.map((d) => ({
              value: d.value,
              itemStyle: { color: d.color },
            })),
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

// ── Chart 7 — Running task bars (count of runs / activity per task) ──
function RunningTaskBars({ running }: { running: RunningTask[] }) {
  if (running.length === 0) {
    return (
      <div className='text-muted-foreground flex h-full items-center justify-center text-xs'>
        no running tasks
      </div>
    )
  }
  // For v1 just show a 1-per-task bar with title — value is a constant
  // 1 (presence indicator). Once we wire per-task host samples this
  // becomes lines/sec or CPU%.
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 100, right: 24, top: 8, bottom: 18 },
        xAxis: { type: 'value', show: false },
        yAxis: {
          type: 'category',
          data: running.map((t) => truncate(t.title, 18)),
          axisLabel: { fontSize: 10 },
        },
        series: [
          {
            type: 'bar',
            data: running.map(() => 1),
            itemStyle: { color: '#10b981' },
            barWidth: 14,
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

// ── Chart 8 — CPU radial heat (recent ticks as radial segments) ──────
function CPURadialHeat({ series }: { series: SystemSample[] }) {
  const recent = series.slice(-60)
  return (
    <EChart
      height='100%'
      option={{
        polar: { radius: ['30%', '85%'] },
        angleAxis: {
          type: 'category',
          data: recent.map((_, i) => String(i)),
          axisLabel: { show: false },
          axisTick: { show: false },
          axisLine: { show: false },
        },
        radiusAxis: {
          max: 100,
          axisLabel: { show: false },
          axisLine: { show: false },
          splitLine: { show: false },
        },
        series: [
          {
            type: 'bar',
            coordinateSystem: 'polar',
            data: recent.map((s) => ({
              value: s.cpu_pct,
              itemStyle: { color: heatColor(s.cpu_pct) },
            })),
            barWidth: '95%',
          },
        ],
      }}
    />
  )
}

function heatColor(pct: number): string {
  if (pct < 20) return '#10b981'
  if (pct < 50) return '#3b82f6'
  if (pct < 75) return '#f59e0b'
  return '#f43f5e'
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}
