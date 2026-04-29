import { useMemo, useState } from 'react'
import {
  useRunStats,
  useSystemStats,
  type RunHostSample,
  type RunRow,
  type SystemHostSample,
} from '@/lib/queries/stats'
import type { EntityKind } from '@/lib/queries/entity'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { EChart } from '@/components/echart'

// Stats tab — three stacked panels backed by /api/run-stats and
// /api/system-stats. Cumulative rolls up across runs (across descendants
// for product / manifest scope, this task's runs only on task scope).
// Per-run drills into a single run's host samples + summary numbers.
// System Capacity reads system_host_samples between [from, to].
//
// Single chart library (echarts + echarts-for-react). The EChart wrapper
// themes via CSS vars so the panels match the rest of the dashboard.

interface StatsTabProps {
  kind: EntityKind
  entityId: string
}

export function StatsTab({ kind, entityId }: StatsTabProps) {
  const runStats = useRunStats(kind, entityId)
  const runs = useMemo(() => runStats.data?.runs ?? [], [runStats.data])
  const samplesByRun = runStats.data?.samples_by_run ?? {}

  return (
    <div data-testid='stats-tab' className='space-y-6'>
      <Panel title='Cumulative'>
        {runStats.isLoading ? (
          <Skeleton className='h-64 w-full' />
        ) : runs.length === 0 ? (
          <Empty msg='No runs yet for this entity.' />
        ) : (
          <CumulativePanel runs={runs} kind={kind} />
        )}
      </Panel>
      <Panel title='Per-run'>
        {runStats.isLoading ? (
          <Skeleton className='h-64 w-full' />
        ) : runs.length === 0 ? (
          <Empty msg='No runs yet for this entity.' />
        ) : (
          <PerRunPanel runs={runs} samplesByRun={samplesByRun} />
        )}
      </Panel>
      <Panel title='System capacity'>
        <SystemPanel />
      </Panel>
    </div>
  )
}

function Panel({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section data-testid={`stats-panel-${title.toLowerCase().replace(/ /g, '-')}`}>
      <h2 className='mb-3 text-lg font-semibold tracking-tight'>{title}</h2>
      <Card>
        <CardContent className='p-4'>{children}</CardContent>
      </Card>
    </section>
  )
}

function Empty({ msg }: { msg: string }) {
  return <p className='text-muted-foreground text-sm'>{msg}</p>
}

// ── Cumulative ─────────────────────────────────────────────────────────

function CumulativePanel({
  runs,
  kind,
}: {
  runs: RunRow[]
  kind: EntityKind
}) {
  // Stable left-to-right order by run number / started_at.
  const ordered = useMemo(
    () =>
      [...runs].sort((a, b) => {
        const ta = new Date(a.started_at).getTime()
        const tb = new Date(b.started_at).getTime()
        return ta - tb
      }),
    [runs]
  )

  const xs = ordered.map((r) => `#${r.run_number}`)
  const costs = ordered.map((r) => round2(r.cost_usd))
  const cumulative = ordered.reduce<number[]>((acc, r) => {
    const next = (acc[acc.length - 1] ?? 0) + r.cost_usd
    acc.push(round2(next))
    return acc
  }, [])

  const inputs = ordered.map((r) => r.input_tokens)
  const outputs = ordered.map((r) => r.output_tokens)
  const cacheRead = ordered.map((r) => r.cache_read_tokens)
  const cacheCreate = ordered.map((r) => r.cache_create_tokens)

  const cacheHitPct = ordered.map((r) => {
    const total =
      r.input_tokens + r.output_tokens + r.cache_read_tokens + r.cache_create_tokens
    return total === 0 ? 0 : round2((r.cache_read_tokens / total) * 100)
  })

  const durations = ordered.map((r) => round2(r.duration_ms / 1000))

  const statusCounts = ordered.reduce<Record<string, number>>((acc, r) => {
    const key = r.cancelled_at ? 'cancelled' : r.status || 'unknown'
    acc[key] = (acc[key] ?? 0) + 1
    return acc
  }, {})

  const errors = ordered.map((r) => r.errors)
  const compactions = ordered.map((r) => r.compactions)

  // Top-10 tasks by cost (product / manifest scope only). Skipped on
  // task scope where every run shares one task_id.
  const topTasks = useMemo(() => {
    if (kind === 'task') return null
    const byTask = new Map<string, number>()
    for (const r of ordered) {
      byTask.set(r.task_id, (byTask.get(r.task_id) ?? 0) + r.cost_usd)
    }
    return [...byTask.entries()]
      .map(([id, cost]) => ({ id, cost: round2(cost) }))
      .sort((a, b) => b.cost - a.cost)
      .slice(0, 10)
  }, [ordered, kind])

  return (
    <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
      <ChartCell label='Cost per run'>
        <EChart
          option={{
            xAxis: { type: 'category', data: xs },
            yAxis: { type: 'value', name: 'USD' },
            series: [{ type: 'line', data: costs, smooth: true, color: '#10b981' }],
          }}
        />
      </ChartCell>

      <ChartCell label='Cumulative cost'>
        <EChart
          option={{
            xAxis: { type: 'category', data: xs },
            yAxis: { type: 'value', name: 'USD' },
            series: [
              {
                type: 'line',
                data: cumulative,
                smooth: true,
                areaStyle: { opacity: 0.3 },
                color: '#3b82f6',
              },
            ],
          }}
        />
      </ChartCell>

      <ChartCell label='Token mix per run'>
        <EChart
          option={{
            tooltip: { trigger: 'axis' },
            legend: { data: ['input', 'output', 'cache_read', 'cache_create'] },
            xAxis: { type: 'category', data: xs },
            yAxis: { type: 'value' },
            series: [
              { name: 'input', type: 'line', stack: 'tokens', areaStyle: {}, data: inputs },
              { name: 'output', type: 'line', stack: 'tokens', areaStyle: {}, data: outputs },
              { name: 'cache_read', type: 'line', stack: 'tokens', areaStyle: {}, data: cacheRead },
              { name: 'cache_create', type: 'line', stack: 'tokens', areaStyle: {}, data: cacheCreate },
            ],
          }}
        />
      </ChartCell>

      <ChartCell label='Cache-hit %'>
        <EChart
          option={{
            xAxis: { type: 'category', data: xs },
            yAxis: { type: 'value', max: 100, name: '%' },
            series: [{ type: 'line', smooth: true, data: cacheHitPct, color: '#8b5cf6' }],
          }}
        />
      </ChartCell>

      <ChartCell label='Duration per run'>
        <EChart
          option={{
            xAxis: { type: 'category', data: xs },
            yAxis: { type: 'value', name: 'seconds' },
            series: [{ type: 'line', smooth: true, data: durations, color: '#f59e0b' }],
          }}
        />
      </ChartCell>

      <ChartCell label='Status breakdown'>
        <EChart
          option={{
            tooltip: { trigger: 'item' },
            legend: { bottom: 0 },
            series: [
              {
                type: 'pie',
                radius: ['40%', '70%'],
                data: Object.entries(statusCounts).map(([name, value]) => ({
                  name,
                  value,
                })),
                label: { color: 'inherit' },
              },
            ],
          }}
        />
      </ChartCell>

      {topTasks && topTasks.length > 0 ? (
        <ChartCell label='Top 10 tasks by cost'>
          <EChart
            option={{
              xAxis: { type: 'value', name: 'USD' },
              yAxis: {
                type: 'category',
                data: topTasks.map((t) => t.id.slice(0, 12)),
                inverse: true,
              },
              series: [
                {
                  type: 'bar',
                  data: topTasks.map((t) => t.cost),
                  color: '#22c55e',
                },
              ],
            }}
          />
        </ChartCell>
      ) : null}

      <ChartCell label='Errors + Compactions per run'>
        <EChart
          option={{
            tooltip: { trigger: 'axis' },
            legend: { data: ['errors', 'compactions'] },
            xAxis: { type: 'category', data: xs },
            yAxis: { type: 'value' },
            series: [
              { name: 'errors', type: 'line', data: errors, color: '#ef4444' },
              {
                name: 'compactions',
                type: 'line',
                data: compactions,
                color: '#0ea5e9',
              },
            ],
          }}
        />
      </ChartCell>
    </div>
  )
}

// ── Per-run ───────────────────────────────────────────────────────────

function PerRunPanel({
  runs,
  samplesByRun,
}: {
  runs: RunRow[]
  samplesByRun: Record<string, RunHostSample[]>
}) {
  const ordered = useMemo(
    () =>
      [...runs].sort(
        (a, b) =>
          new Date(b.started_at).getTime() - new Date(a.started_at).getTime()
      ),
    [runs]
  )
  const [selected, setSelected] = useState<string>(
    ordered[0] ? String(ordered[0].id) : ''
  )
  const run = useMemo(
    () => ordered.find((r) => String(r.id) === selected),
    [ordered, selected]
  )
  const samples = useMemo(
    () => (run ? (samplesByRun[String(run.id)] ?? []) : []),
    [run, samplesByRun]
  )

  if (!run) return <Empty msg='No run selected.' />

  return (
    <div className='space-y-4'>
      <div className='flex items-center gap-3'>
        <span className='text-muted-foreground text-sm'>Run:</span>
        <Select value={selected} onValueChange={setSelected}>
          <SelectTrigger className='w-64'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {ordered.map((r) => (
              <SelectItem key={r.id} value={String(r.id)}>
                #{r.run_number} — {fmtTime(r.started_at)} ({r.status})
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <RunSummary run={run} />

      <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
        <ChartCell label='Token mix'>
          <EChart
            height={140}
            option={{
              tooltip: { trigger: 'axis' },
              legend: { data: ['input', 'output', 'cache_read', 'cache_create'] },
              xAxis: { type: 'value' },
              yAxis: { type: 'category', data: ['tokens'] },
              series: [
                { name: 'input', type: 'bar', stack: 'tokens', data: [run.input_tokens] },
                { name: 'output', type: 'bar', stack: 'tokens', data: [run.output_tokens] },
                {
                  name: 'cache_read',
                  type: 'bar',
                  stack: 'tokens',
                  data: [run.cache_read_tokens],
                },
                {
                  name: 'cache_create',
                  type: 'bar',
                  stack: 'tokens',
                  data: [run.cache_create_tokens],
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Cache-hit'>
          <EChart
            height={200}
            option={{
              tooltip: { trigger: 'item' },
              series: [
                {
                  type: 'pie',
                  radius: ['55%', '75%'],
                  label: {
                    show: true,
                    position: 'center',
                    formatter: () =>
                      `${cacheHit(run).toFixed(1)}%`,
                    fontSize: 22,
                    color: 'inherit',
                  },
                  data: [
                    { name: 'cache_read', value: run.cache_read_tokens },
                    {
                      name: 'other',
                      value:
                        run.input_tokens +
                        run.output_tokens +
                        run.cache_create_tokens,
                    },
                  ],
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Cumulative cost timeline'>
          <EChart
            option={{
              xAxis: { type: 'time' },
              yAxis: { type: 'value', name: 'USD' },
              series: [
                {
                  type: 'line',
                  smooth: true,
                  areaStyle: { opacity: 0.3 },
                  data: samples.map((s) => [s.ts, round2(s.cost_usd)]),
                  color: '#10b981',
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='CPU %'>
          <EChart
            option={{
              xAxis: { type: 'time' },
              yAxis: { type: 'value', name: '%' },
              series: [
                {
                  type: 'line',
                  smooth: true,
                  data: samples.map((s) => [s.ts, round2(s.cpu_pct)]),
                  color: '#f97316',
                  markPoint: { data: [{ type: 'max', name: 'peak' }] },
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='RSS (MB)'>
          <EChart
            option={{
              xAxis: { type: 'time' },
              yAxis: { type: 'value', name: 'MB' },
              series: [
                {
                  type: 'line',
                  smooth: true,
                  data: samples.map((s) => [s.ts, round2(s.rss_mb)]),
                  color: '#a855f7',
                  markPoint: { data: [{ type: 'max', name: 'peak' }] },
                },
              ],
            }}
          />
        </ChartCell>

        <ChartCell label='Turns + Actions timeline'>
          <EChart
            option={{
              tooltip: { trigger: 'axis' },
              legend: { data: ['turns', 'actions'] },
              xAxis: { type: 'time' },
              yAxis: { type: 'value' },
              series: [
                {
                  name: 'turns',
                  type: 'line',
                  data: samples.map((s) => [s.ts, s.turns]),
                  color: '#0ea5e9',
                },
                {
                  name: 'actions',
                  type: 'line',
                  data: samples.map((s) => [s.ts, s.actions]),
                  color: '#22c55e',
                },
              ],
            }}
          />
        </ChartCell>
      </div>

      <GitOutputCard run={run} />
    </div>
  )
}

function RunSummary({ run }: { run: RunRow }) {
  return (
    <div className='grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-9'>
      <Stat label='status'>
        <Badge variant='outline'>
          {run.cancelled_at ? 'cancelled' : run.status || '—'}
        </Badge>
      </Stat>
      <Stat label='cost'>${round2(run.cost_usd)}</Stat>
      <Stat label='turns'>{run.turns}</Stat>
      <Stat label='actions'>{run.actions}</Stat>
      <Stat label='duration'>{fmtDuration(run.duration_ms)}</Stat>
      <Stat label='exit_code'>{run.exit_code}</Stat>
      <Stat label='model'>{run.model || '—'}</Stat>
      <Stat label='runtime'>{run.agent_runtime || '—'}</Stat>
      <Stat label='version'>{run.agent_version || '—'}</Stat>
    </div>
  )
}

function Stat({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className='border-border bg-card flex flex-col rounded-md border p-2'>
      <span className='text-muted-foreground text-[10px] uppercase tracking-wide'>
        {label}
      </span>
      <span className='text-sm font-medium'>{children}</span>
    </div>
  )
}

function GitOutputCard({ run }: { run: RunRow }) {
  const [expanded, setExpanded] = useState(false)
  const truncated = run.output && run.output.length > 4000
  const visible = expanded
    ? run.output
    : (run.output ?? '').slice(0, 4000)

  return (
    <Card>
      <CardContent className='space-y-3 p-4'>
        <div className='flex flex-wrap gap-x-6 gap-y-1 text-sm'>
          <Field label='branch' value={run.branch || '—'} />
          <Field
            label='commit'
            value={
              run.commit_sha
                ? run.commit_sha.slice(0, 12)
                : '—'
            }
          />
          {run.pr_number > 0 ? (
            <Field
              label='PR'
              value={`#${run.pr_number}`}
            />
          ) : null}
          <Field
            label='lines'
            value={`+${run.lines_added} / -${run.lines_removed}`}
          />
          <Field
            label='files_changed'
            value={String(run.files_changed)}
          />
          <Field label='commits' value={String(run.commits)} />
        </div>
        {run.output ? (
          <div>
            <div className='mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground'>
              Output
            </div>
            <pre className='border-border bg-muted/40 max-h-[400px] overflow-auto rounded border p-3 text-xs'>
              {visible}
            </pre>
            {truncated ? (
              <Button
                size='sm'
                variant='ghost'
                onClick={() => setExpanded((v) => !v)}
                className='mt-2'
              >
                {expanded ? 'Show less' : 'Show all'}
              </Button>
            ) : null}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <span className='text-sm'>
      <span className='text-muted-foreground'>{label}:</span>{' '}
      <span className='font-mono text-xs'>{value}</span>
    </span>
  )
}

// ── System capacity ───────────────────────────────────────────────────

const WINDOWS = [
  { key: '1h', label: '1h', ms: 1 * 60 * 60 * 1000 },
  { key: '6h', label: '6h', ms: 6 * 60 * 60 * 1000 },
  { key: '24h', label: '24h', ms: 24 * 60 * 60 * 1000 },
  { key: '7d', label: '7d', ms: 7 * 24 * 60 * 60 * 1000 },
] as const

function SystemPanel() {
  const [windowKey, setWindowKey] = useState<string>('1h')
  const range = useMemo(() => {
    const cfg = WINDOWS.find((w) => w.key === windowKey) ?? WINDOWS[0]
    const to = new Date()
    const from = new Date(to.getTime() - cfg.ms)
    return {
      from: from.toISOString(),
      to: to.toISOString(),
    }
  }, [windowKey])

  const sys = useSystemStats(range.from, range.to)
  const samples = sys.data?.samples ?? []
  const latest = samples[samples.length - 1]

  return (
    <div className='space-y-4'>
      <div className='flex items-center gap-2'>
        <span className='text-muted-foreground text-sm'>Window:</span>
        {WINDOWS.map((w) => (
          <Button
            key={w.key}
            size='sm'
            variant={windowKey === w.key ? 'default' : 'outline'}
            onClick={() => setWindowKey(w.key)}
          >
            {w.label}
          </Button>
        ))}
      </div>

      {sys.isLoading ? (
        <Skeleton className='h-64 w-full' />
      ) : samples.length === 0 ? (
        <Empty msg='No system samples in this window yet — wait ~30s after restart for the SystemSampler to fill rows.' />
      ) : (
        <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
          <ChartCell label='CPU %'>
            <EChart
              option={{
                xAxis: { type: 'time' },
                yAxis: { type: 'value', name: '%' },
                series: [
                  {
                    type: 'line',
                    smooth: true,
                    data: samples.map((s) => [s.ts, round2(s.cpu_pct)]),
                    color: '#f97316',
                    markPoint: { data: [{ type: 'max', name: 'peak' }] },
                  },
                ],
              }}
            />
          </ChartCell>

          <ChartCell label='Load average'>
            <EChart
              option={{
                tooltip: { trigger: 'axis' },
                legend: { data: ['load_1m', 'load_5m', 'load_15m'] },
                xAxis: { type: 'time' },
                yAxis: { type: 'value' },
                series: [
                  {
                    name: 'load_1m',
                    type: 'line',
                    data: samples.map((s) => [s.ts, round2(s.load_1m)]),
                  },
                  {
                    name: 'load_5m',
                    type: 'line',
                    data: samples.map((s) => [s.ts, round2(s.load_5m)]),
                  },
                  {
                    name: 'load_15m',
                    type: 'line',
                    data: samples.map((s) => [s.ts, round2(s.load_15m)]),
                  },
                ],
              }}
            />
          </ChartCell>

          <ChartCell label='Memory used vs total (MB)'>
            <EChart
              option={{
                tooltip: { trigger: 'axis' },
                legend: { data: ['used', 'free'] },
                xAxis: { type: 'time' },
                yAxis: { type: 'value', name: 'MB' },
                series: [
                  {
                    name: 'used',
                    type: 'line',
                    stack: 'mem',
                    areaStyle: {},
                    data: samples.map((s) => [s.ts, round2(s.mem_used_mb)]),
                  },
                  {
                    name: 'free',
                    type: 'line',
                    stack: 'mem',
                    areaStyle: {},
                    data: samples.map((s) => [
                      s.ts,
                      round2(Math.max(s.mem_total_mb - s.mem_used_mb, 0)),
                    ]),
                  },
                ],
              }}
            />
          </ChartCell>

          <ChartCell label='Disk used vs total (GB)'>
            <EChart
              option={{
                tooltip: { trigger: 'axis' },
                legend: { data: ['used', 'free'] },
                xAxis: { type: 'time' },
                yAxis: { type: 'value', name: 'GB' },
                series: [
                  {
                    name: 'used',
                    type: 'line',
                    stack: 'disk',
                    areaStyle: {},
                    data: samples.map((s) => [s.ts, round2(s.disk_used_gb)]),
                  },
                  {
                    name: 'free',
                    type: 'line',
                    stack: 'disk',
                    areaStyle: {},
                    data: samples.map((s) => [
                      s.ts,
                      round2(Math.max(s.disk_total_gb - s.disk_used_gb, 0)),
                    ]),
                  },
                ],
              }}
            />
          </ChartCell>

          <ChartCell label='Network throughput (Mbps)'>
            <EChart
              option={{
                tooltip: { trigger: 'axis' },
                legend: { data: ['rx', 'tx'] },
                xAxis: { type: 'time' },
                yAxis: { type: 'value', name: 'Mbps' },
                series: [
                  {
                    name: 'rx',
                    type: 'line',
                    data: samples.map((s) => [s.ts, round2(s.net_rx_mbps)]),
                  },
                  {
                    name: 'tx',
                    type: 'line',
                    data: samples.map((s) => [s.ts, round2(s.net_tx_mbps)]),
                  },
                ],
              }}
            />
          </ChartCell>

          <CurrentSnapshotCard latest={latest} />
        </div>
      )}
    </div>
  )
}

function CurrentSnapshotCard({ latest }: { latest?: SystemHostSample }) {
  if (!latest) return null
  const memPct =
    latest.mem_total_mb > 0
      ? (latest.mem_used_mb / latest.mem_total_mb) * 100
      : 0
  const diskPct =
    latest.disk_total_gb > 0
      ? (latest.disk_used_gb / latest.disk_total_gb) * 100
      : 0
  return (
    <div>
      <div className='text-muted-foreground mb-1 text-sm font-medium'>
        Current snapshot
      </div>
      <div className='grid grid-cols-3 gap-3'>
        <Stat label='cpu %'>{round2(latest.cpu_pct)}</Stat>
        <Stat label='mem %'>{round2(memPct)}</Stat>
        <Stat label='disk %'>{round2(diskPct)}</Stat>
      </div>
    </div>
  )
}

// ── Helpers ───────────────────────────────────────────────────────────

function ChartCell({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className='border-border bg-card rounded-md border p-3'>
      <div className='text-muted-foreground mb-1 text-xs font-medium uppercase tracking-wide'>
        {label}
      </div>
      {children}
    </div>
  )
}

function round2(n: number): number {
  return Math.round(n * 100) / 100
}

function cacheHit(r: RunRow): number {
  const total =
    r.input_tokens + r.output_tokens + r.cache_read_tokens + r.cache_create_tokens
  if (total === 0) return 0
  return (r.cache_read_tokens / total) * 100
}

function fmtTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function fmtDuration(ms: number): string {
  if (!ms) return '—'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rs = s % 60
  if (m < 60) return `${m}m ${rs}s`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}
