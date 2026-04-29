import { useEffect, useMemo, useState } from 'react'
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

interface LiveSample {
  ts: string
  cpu_pct: number
  rss_mb: number
  cost_usd: number
  turns: number
  actions: number
}

interface LiveTask {
  task_id: string
  title: string
  manifest_id: string
  run_id: number
  run_number: number
  started_at: string
  elapsed_sec: number
  turns: number
  lines: number
  lines_added: number
  lines_removed: number
  cost_usd: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_create_tokens: number
  cpu_pct: number
  rss_mb: number
  actions_count: number
  recent_samples: LiveSample[]
}

interface TasksStats {
  budget_exceeded: boolean
  budget_pct: number
  cost_today: number
  daily_budget: number
  running: number
  tasks_total: number
  turns_today: number
  // Cumulative rollups across every completed task_run.
  runs_total: number
  cost_total: number
  turns_total: number
  lines_total: number
  errors_total: number
  input_tokens_total: number
  output_tokens_total: number
  cache_read_tokens_total: number
  cache_create_tokens_total: number
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

function useRunningTasksLive() {
  return useQuery({
    queryKey: ['running-tasks-live'],
    queryFn: async () => {
      const r = await fetch('/api/tasks/running/live')
      if (!r.ok) throw new Error(`tasks/running/live → ${r.status}`)
      return ((await r.json()) as LiveTask[] | null) ?? []
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

// Top-level panel container — three stacked full-width cards.
// Order: Tasks → AI Stats → System Stats. Each panel gets the full
// row so chart titles + content breathe; stacking also makes the
// scrolling story coherent (you scroll through tasks, then agents,
// then host metrics, top-down).
export function RunningTasksPanel() {
  return (
    <div className='space-y-3'>
      <TasksPanel />
      <AIStatsPanel />
      <SystemPanel />
    </div>
  )
}

// ── 1. Tasks panel ───────────────────────────────────────────────────
function TasksPanel() {
  const live = useRunningTasksLive()
  const stats = useTasksStats()
  const running = live.data ?? []
  const s = stats.data

  // Aggregate live metrics across all running tasks for the header.
  const agg = useMemo(() => {
    let cpu = 0, rss = 0, cost = 0, turns = 0, actions = 0
    for (const t of running) {
      cpu += t.cpu_pct
      rss += t.rss_mb
      cost += t.cost_usd
      turns += t.turns
      actions += t.actions_count
    }
    return { cpu, rss, cost, turns, actions }
  }, [running])

  return (
    <PanelCard
      title='Tasks'
      badge='live'
      badgeTone='emerald'
      stats={[
        { label: 'Running', value: String(running.length), accent: 'emerald' },
        { label: 'Total', value: s ? String(s.tasks_total) : '—' },
        { label: '∑ CPU', value: running.length > 0 ? `${agg.cpu.toFixed(0)}%` : '—' },
        { label: '∑ RSS', value: running.length > 0 ? `${(agg.rss / 1024).toFixed(1)}G` : '—' },
        { label: '∑ Cost·run', value: running.length > 0 ? `$${agg.cost.toFixed(4)}` : '—' },
        { label: '∑ Turns', value: running.length > 0 ? String(agg.turns) : '—' },
        { label: '∑ Actions', value: running.length > 0 ? String(agg.actions) : '—' },
        {
          label: 'Budget',
          value: s ? `${(s.budget_pct ?? 0).toFixed(0)}%` : '—',
          accent: s && s.budget_pct > 80 ? 'rose' : undefined,
        },
      ]}
    >
      {running.length === 0 ? (
        <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
          <ChartTile label='Running count' note='0…N tasks live'>
            <RunningGauge running={0} max={8} />
          </ChartTile>
          <ChartTile label='Daily budget' note={`$${s?.cost_today.toFixed(2) ?? '0.00'} of $${s?.daily_budget ?? 100}`}>
            <BudgetRing used={s?.cost_today ?? 0} budget={s?.daily_budget ?? 100} />
          </ChartTile>
          <div className='text-muted-foreground col-span-full rounded-md border bg-background/40 p-6 text-center text-sm'>
            no running tasks — fire one and live tiles appear here
          </div>
        </div>
      ) : (
        <div className='space-y-3'>
          {/* One per-task tile per running task. Each tile carries the
              full live metric set: turns / actions / cost / lines /
              CPU / RSS plus three live sparklines. */}
          <div className='grid grid-cols-1 gap-3 lg:grid-cols-2'>
            {running.map((t) => (
              <RunningTaskTile key={t.task_id} t={t} />
            ))}
          </div>
          {/* Side cards next to the per-task tiles: aggregate budget
              ring + count gauge so the panel still reads as "tasks
              overview" not just per-task drill-in. */}
          <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
            <ChartTile label='Running count' note='0…N tasks live'>
              <RunningGauge running={running.length} max={Math.max(8, running.length + 2)} />
            </ChartTile>
            <ChartTile label='Daily budget' note={`$${s?.cost_today.toFixed(2) ?? '0.00'} of $${s?.daily_budget ?? 100}`}>
              <BudgetRing used={s?.cost_today ?? 0} budget={s?.daily_budget ?? 100} />
            </ChartTile>
          </div>
        </div>
      )}
    </PanelCard>
  )
}

// RunningTaskTile — one card per in-flight task. Renders:
//   - title + elapsed clock
//   - 4-up stat grid: turns / actions / cost / lines (live)
//   - three small sparklines: CPU%, RSS MB, cost trajectory
//   - token totals as small text
function RunningTaskTile({ t }: { t: LiveTask }) {
  const elapsed = formatElapsed(t.elapsed_sec)
  const samples = t.recent_samples ?? []
  return (
    <div className='rounded-md border bg-background/40 p-3'>
      <div className='mb-2 flex items-baseline justify-between gap-2'>
        <div className='min-w-0 flex-1'>
          <div className='truncate text-sm font-semibold' title={t.title}>
            {t.title}
          </div>
          <code className='text-muted-foreground font-mono text-[10px]'>
            {t.task_id.slice(0, 8)}…  ·  run #{t.run_number}  ·  {elapsed}
          </code>
        </div>
        <span className='inline-block h-2 w-2 animate-pulse rounded-full bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,.8)]' />
      </div>

      <div className='mb-2 grid grid-cols-4 gap-2 text-xs'>
        <Tile label='turns' value={String(t.turns)} accent='emerald' />
        <Tile label='actions' value={String(t.actions_count)} accent='violet' />
        <Tile label='cost' value={`$${t.cost_usd.toFixed(4)}`} accent='violet' />
        <Tile label='lines' value={String(t.lines)} />
      </div>

      <div className='grid grid-cols-3 gap-2'>
        <MiniSpark
          label='CPU'
          unit='%'
          color='#34d399'
          data={samples.map((s) => [s.ts, s.cpu_pct] as [string, number])}
          current={t.cpu_pct}
        />
        <MiniSpark
          label='RSS'
          unit='M'
          color='#a78bfa'
          data={samples.map((s) => [s.ts, s.rss_mb] as [string, number])}
          current={t.rss_mb}
        />
        <MiniSpark
          label='cost'
          unit=''
          color='#f59e0b'
          data={samples.map((s) => [s.ts, s.cost_usd] as [string, number])}
          current={t.cost_usd}
          prefix='$'
        />
      </div>

      <div className='text-muted-foreground mt-2 flex flex-wrap gap-x-3 gap-y-0.5 text-[10px]'>
        <span>in {t.input_tokens.toLocaleString()}</span>
        <span>out {t.output_tokens.toLocaleString()}</span>
        <span>cache_r {t.cache_read_tokens.toLocaleString()}</span>
        <span>cache_w {t.cache_create_tokens.toLocaleString()}</span>
        {t.lines_added > 0 && <span className='text-emerald-400'>+{t.lines_added}</span>}
        {t.lines_removed > 0 && <span className='text-rose-400'>-{t.lines_removed}</span>}
      </div>
    </div>
  )
}

function Tile({
  label,
  value,
  accent,
}: {
  label: string
  value: string
  accent?: 'emerald' | 'violet'
}) {
  return (
    <div className='rounded-md bg-zinc-900/40 px-2 py-1.5'>
      <div className='text-muted-foreground text-[9px] uppercase tracking-wider'>{label}</div>
      <div
        className={cn(
          'font-mono text-sm font-semibold tabular-nums',
          accent === 'emerald' && 'text-emerald-300',
          accent === 'violet' && 'text-violet-300'
        )}
      >
        {value}
      </div>
    </div>
  )
}

function MiniSpark({
  label,
  unit,
  color,
  data,
  current,
  prefix = '',
}: {
  label: string
  unit: string
  color: string
  data: [string, number][]
  current: number
  prefix?: string
}) {
  return (
    <div className='rounded-md bg-zinc-900/40 p-1.5'>
      <div className='flex items-baseline justify-between'>
        <span className='text-muted-foreground text-[9px] uppercase tracking-wider'>
          {label}
        </span>
        <span className='font-mono text-[10px]' style={{ color }}>
          {prefix}
          {current.toFixed(unit === '%' ? 0 : unit === 'M' ? 0 : 4)}
          {unit}
        </span>
      </div>
      <div className='h-[36px]'>
        <EChart
          height='100%'
          option={{
            grid: { left: 0, right: 0, top: 2, bottom: 0 },
            xAxis: { type: 'time', show: false },
            yAxis: { type: 'value', show: false, scale: true },
            series: [
              {
                type: 'line',
                data,
                smooth: true,
                showSymbol: false,
                lineStyle: { color, width: 1.5 },
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
      </div>
    </div>
  )
}

function formatElapsed(sec: number): string {
  if (sec < 60) return `${sec}s`
  if (sec < 3600) {
    const m = Math.floor(sec / 60)
    const s = sec % 60
    return `${m}m${s.toString().padStart(2, '0')}s`
  }
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return `${h}h${m.toString().padStart(2, '0')}m`
}

// ── 2. AI Stats panel ────────────────────────────────────────────────
function AIStatsPanel() {
  const stats = useTasksStats()
  const actions = useActionsRecent()
  const s = stats.data
  const env = actions.data

  // In-memory history of the polled cumulative totals. Each poll adds
  // a sample; the trend charts plot deltas/cumulative as time series.
  // Lives in component state so it survives between polls but resets
  // on full reload (acceptable — these are live-tail trends, not
  // long-term audit data).
  //
  // Sampler: a setInterval fires every 3s regardless of whether the
  // polled totals changed. Without this, TanStack Query's structural
  // sharing returns the same `s` reference when nothing changed and
  // a useEffect([s, env]) wouldn't re-fire — leaving the trend charts
  // perpetually at one sample.
  const [history, setHistory] = useState<{
    ts: number
    cost: number
    turns: number
    actions: number
  }[]>([])
  useEffect(() => {
    const tick = () => {
      if (!s || !env) return
      setHistory((prev) => {
        const sample = {
          ts: Date.now(),
          cost: s.cost_total,
          turns: s.turns_total,
          actions: env.total,
        }
        const next = [...prev, sample]
        // Keep last 200 samples; drops oldest. With a 3s sampler
        // that's ~10 minutes of trend.
        return next.length > 200 ? next.slice(-200) : next
      })
    }
    tick() // immediate sample so the chart isn't blank for 3s
    const id = setInterval(tick, 3000)
    return () => clearInterval(id)
  }, [s, env])

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
        { label: 'Cost·all', value: s ? `$${s.cost_total.toFixed(2)}` : '—', accent: 'violet' },
        { label: 'Turns·all', value: s ? s.turns_total.toLocaleString() : '—' },
        { label: 'Actions·all', value: env ? env.total.toLocaleString() : '—' },
        { label: 'Runs·all', value: s ? s.runs_total.toLocaleString() : '—' },
      ]}
    >
      {/* Three trend charts: cumulative cost, cumulative turns,
          cumulative actions — all over the live polling history. */}
      <div className='mb-3 grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3'>
        <ChartTile label='Cumulative cost' note='live trend'>
          <CumulativeTrend
            data={history.map((h) => [h.ts, h.cost])}
            color='#a78bfa'
            unit='$'
            currentLabel={s ? `$${s.cost_total.toFixed(2)}` : '—'}
          />
        </ChartTile>
        <ChartTile label='Cumulative turns' note='live trend'>
          <CumulativeTrend
            data={history.map((h) => [h.ts, h.turns])}
            color='#10b981'
            unit=''
            currentLabel={s ? s.turns_total.toLocaleString() : '—'}
          />
        </ChartTile>
        <ChartTile label='Cumulative actions' note='live trend'>
          <CumulativeTrend
            data={history.map((h) => [h.ts, h.actions])}
            color='#f59e0b'
            unit=''
            currentLabel={env ? env.total.toLocaleString() : '—'}
          />
        </ChartTile>
      </div>

      {/* Cache hit ratio + token split — the cache is where the real
          cost saving lives. Stacked bar shows the four token kinds;
          gauge shows the ratio cached_read / (input + cached_read). */}
      <div className='mb-3 grid grid-cols-1 gap-3 md:grid-cols-3'>
        <ChartTile label='Cache hit ratio' note='cache_read / (cache_read + cache_create)'>
          <CacheHitGauge
            cacheRead={s?.cache_read_tokens_total ?? 0}
            cacheCreate={s?.cache_create_tokens_total ?? 0}
          />
        </ChartTile>
        <ChartTile label='Token split' note='all-time' span={2}>
          <TokenSplitBar
            input={s?.input_tokens_total ?? 0}
            output={s?.output_tokens_total ?? 0}
            cacheRead={s?.cache_read_tokens_total ?? 0}
            cacheCreate={s?.cache_create_tokens_total ?? 0}
          />
        </ChartTile>
      </div>

      <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
        <ChartTile label='Actions / hour' note='last 12 hr buckets'>
          <HourlyBars data={hourly} color='#a78bfa' />
        </ChartTile>
        <ChartTile label='Top tools' note='in last 300 actions'>
          <ToolBreakdownBars data={toolBreakdown} />
        </ChartTile>
      </div>
    </PanelCard>
  )
}

// CumulativeTrend — area-line chart over time-stamped cumulative
// samples. Shows the current value as overlay text top-right.
//
// Bounds: the xAxis is hard-pinned to "now - 10min … now" so a sparse
// (or single-point) history still renders correctly at the right edge.
// Without these bounds ECharts auto-fits to a ±21h default range and
// the line vanishes into a single invisible dot.
//
// Empty state: when fewer than 2 samples are in the history we show a
// "warming up" overlay instead of an empty grid so the operator knows
// the chart hasn't broken — it's just collecting.
function CumulativeTrend({
  data,
  color,
  unit,
  currentLabel,
}: {
  data: [number, number][]
  color: string
  unit: string
  currentLabel: string
}) {
  const now = Date.now()
  const xMin = now - 10 * 60 * 1000
  const warming = data.length < 2
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 40, right: 8, top: 22, bottom: 18 },
        xAxis: {
          type: 'time',
          min: xMin,
          max: now,
          axisLabel: { fontSize: 9 },
        },
        yAxis: {
          type: 'value',
          axisLabel: {
            fontSize: 9,
            formatter: (v: number) => {
              if (v >= 1000) return (v / 1000).toFixed(1) + 'k'
              return unit + String(v)
            },
          },
          scale: true,
        },
        graphic: [
          {
            type: 'text',
            right: 8,
            top: 4,
            style: {
              text: currentLabel,
              fill: color,
              fontSize: 14,
              fontWeight: 600,
            },
          },
          ...(warming
            ? [
                {
                  type: 'text',
                  left: 'center',
                  top: 'middle',
                  style: {
                    text: `warming up · ${data.length}/200 samples`,
                    fill: '#71717a',
                    fontSize: 11,
                  },
                },
              ]
            : []),
        ],
        series: [
          {
            type: 'line',
            data,
            smooth: true,
            showSymbol: data.length < 5,
            symbolSize: 4,
            lineStyle: { color, width: 2 },
            itemStyle: { color },
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

// CacheHitGauge — radial showing what fraction of cache traffic was
// reuse (cache_read) vs new entries written (cache_create). Higher =
// the prompt cache is amortising well across runs. Below 50% is
// amber, below 25% is rose.
function CacheHitGauge({
  cacheRead,
  cacheCreate,
}: {
  cacheRead: number
  cacheCreate: number
}) {
  const denom = cacheRead + cacheCreate
  const ratio = denom > 0 ? cacheRead / denom : 0
  const pct = ratio * 100
  const color = pct >= 75 ? '#10b981' : pct >= 50 ? '#a78bfa' : pct >= 25 ? '#f59e0b' : '#f43f5e'
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
              itemStyle: { color },
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
              formatter: () => `${pct.toFixed(0)}%`,
              color,
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
              text:
                cacheRead > 1e9
                  ? `${(cacheRead / 1e9).toFixed(1)}B cached`
                  : cacheRead > 1e6
                    ? `${(cacheRead / 1e6).toFixed(1)}M cached`
                    : `${cacheRead} cached`,
              fill: '#94a3b8',
              fontSize: 10,
            },
          },
        ],
      }}
    />
  )
}

// TokenSplitBar — single horizontal stacked bar showing the four
// token kinds and their proportions. Order: cache_read | input |
// cache_create | output. Cache read first so it visually dominates
// when the cache is doing its job.
function TokenSplitBar({
  input,
  output,
  cacheRead,
  cacheCreate,
}: {
  input: number
  output: number
  cacheRead: number
  cacheCreate: number
}) {
  const total = input + output + cacheRead + cacheCreate
  if (total === 0) {
    return (
      <div className='text-muted-foreground flex h-full items-center justify-center text-xs'>
        no token data yet
      </div>
    )
  }
  const fmt = (n: number) =>
    n >= 1e9 ? `${(n / 1e9).toFixed(1)}B` : n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e3 ? `${(n / 1e3).toFixed(0)}k` : String(n)
  return (
    <EChart
      height='100%'
      option={{
        grid: { left: 8, right: 8, top: 24, bottom: 32 },
        tooltip: {
          trigger: 'axis',
          axisPointer: { type: 'shadow' },
          formatter: (params: { name: string; value: number; color: string }[]) => {
            return params
              .map(
                (p) =>
                  `<span style="color:${p.color}">${p.name}</span> ${fmt(p.value)} (${((p.value / total) * 100).toFixed(1)}%)`
              )
              .join('<br/>')
          },
        },
        legend: {
          show: true,
          bottom: 0,
          itemWidth: 8,
          itemHeight: 8,
          textStyle: { fontSize: 9, color: '#a1a1aa' },
        },
        xAxis: {
          type: 'value',
          show: false,
          max: total,
        },
        yAxis: {
          type: 'category',
          data: ['tokens'],
          show: false,
        },
        series: [
          {
            name: 'cache read',
            type: 'bar',
            stack: 'tokens',
            data: [cacheRead],
            itemStyle: { color: '#10b981' },
            label: {
              show: true,
              position: 'inside',
              fontSize: 10,
              color: '#fff',
              formatter: () => fmt(cacheRead),
            },
          },
          {
            name: 'input',
            type: 'bar',
            stack: 'tokens',
            data: [input],
            itemStyle: { color: '#3b82f6' },
            label: {
              show: true,
              position: 'inside',
              fontSize: 10,
              color: '#fff',
              formatter: () => fmt(input),
            },
          },
          {
            name: 'cache create',
            type: 'bar',
            stack: 'tokens',
            data: [cacheCreate],
            itemStyle: { color: '#f59e0b' },
            label: {
              show: true,
              position: 'inside',
              fontSize: 10,
              color: '#fff',
              formatter: () => fmt(cacheCreate),
            },
          },
          {
            name: 'output',
            type: 'bar',
            stack: 'tokens',
            data: [output],
            itemStyle: { color: '#a78bfa' },
            label: {
              show: true,
              position: 'inside',
              fontSize: 10,
              color: '#fff',
              formatter: () => fmt(output),
            },
          },
        ],
      }}
    />
  )
}

function BigNumber({
  label,
  value,
  sub,
  accent,
}: {
  label: string
  value: string
  sub?: string
  accent?: 'violet' | 'rose'
}) {
  return (
    <div className='rounded-md border bg-background/40 p-2'>
      <div className='text-muted-foreground text-[10px] uppercase tracking-wider'>
        {label}
      </div>
      <div
        className={cn(
          'mt-1 font-mono text-xl font-semibold tabular-nums',
          accent === 'violet' && 'text-violet-300',
          accent === 'rose' && 'text-rose-300'
        )}
      >
        {value}
      </div>
      {sub && (
        <div className='text-muted-foreground mt-0.5 text-[10px]'>{sub}</div>
      )}
    </div>
  )
}

function CostPerTurnGauge({ cost, turns }: { cost: number; turns: number }) {
  const cpt = turns > 0 ? cost / turns : 0
  // Most agent runs land in $0.001–$0.10 per turn. Use a log-ish gauge:
  // 0 → ∅, 0.05 → mid, 0.20+ → red. Bar fill 0..100 maps to 0..$0.20.
  const pct = Math.min(100, (cpt / 0.2) * 100)
  const color = cpt > 0.1 ? '#f43f5e' : cpt > 0.05 ? '#f59e0b' : '#a78bfa'
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
              itemStyle: { color },
            },
            axisLine: { lineStyle: { width: 12, color: [[1, '#27272a']] } },
            axisTick: { show: false },
            splitLine: { show: false },
            axisLabel: { show: false },
            pointer: { show: false },
            title: { show: false },
            detail: {
              valueAnimation: true,
              fontSize: 18,
              fontWeight: 600,
              offsetCenter: [0, '5%'],
              formatter: () => `$${cpt.toFixed(4)}`,
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
              text: 'per turn',
              fill: '#94a3b8',
              fontSize: 10,
            },
          },
        ],
      }}
    />
  )
}

// ── 3. System panel ──────────────────────────────────────────────────
function SystemPanel() {
  const samples = useSystemStats(120)
  const series = samples.data ?? []
  const last = series.length ? series[series.length - 1] : null

  return (
    <PanelCard
      title='System Stats'
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
      <div className='grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-4'>
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
