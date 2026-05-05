import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { formatDistanceToNow, format, fromUnixTime } from 'date-fns'
import { ChevronDown, ChevronRight, Loader2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import type { EntityKind } from '@/lib/queries/entity'
import type { ExecutionRow } from '@/lib/types'

// One summarised run, derived from all its execution_log rows.
interface RunSummary {
  run_uid: string
  run_number: number
  trigger: string
  model: string
  agent_runtime: string
  terminal_reason: string
  error: string
  started_at: number
  completed_at: number
  duration_ms: number
  status: 'running' | 'completed' | 'failed'
  turns: number
  actions: number
  lines_added: number
  lines_removed: number
  commits: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_hit_rate_pct: number
  context_window_pct: number
  tests_run: number
  tests_passed: number
  tests_failed: number
  events: ExecutionRow[]
}

function groupRuns(rows: ExecutionRow[]): RunSummary[] {
  const map = new Map<string, ExecutionRow[]>()
  for (const row of rows) {
    const arr = map.get(row.run_uid) ?? []
    arr.push(row)
    map.set(row.run_uid, arr)
  }

  const summaries: RunSummary[] = []
  for (const [runUid, events] of map) {
    events.sort((a, b) => a.created_at.localeCompare(b.created_at))
    const terminal = [...events].reverse().find(
      (e) => e.event === 'completed' || e.event === 'failed',
    )
    const startedRow = events.find((e) => e.event === 'started')
    const ref = terminal ?? events[events.length - 1]

    const status: RunSummary['status'] = terminal
      ? (terminal.event as 'completed' | 'failed')
      : 'running'

    summaries.push({
      run_uid: runUid,
      run_number: ref.run_number,
      trigger: ref.trigger || startedRow?.trigger || '',
      model: ref.model || startedRow?.model || '',
      agent_runtime: ref.agent_runtime || startedRow?.agent_runtime || '',
      terminal_reason: ref.terminal_reason || '',
      error: ref.error || '',
      started_at: startedRow?.started_at ?? ref.started_at,
      completed_at: ref.completed_at,
      duration_ms: ref.duration_ms,
      status,
      turns: ref.turns,
      actions: ref.actions,
      lines_added: ref.lines_added,
      lines_removed: ref.lines_removed,
      commits: ref.commits,
      input_tokens: ref.input_tokens,
      output_tokens: ref.output_tokens,
      cache_read_tokens: ref.cache_read_tokens,
      cache_hit_rate_pct: ref.cache_hit_rate_pct,
      context_window_pct: ref.context_window_pct,
      tests_run: ref.tests_run,
      tests_passed: ref.tests_passed,
      tests_failed: ref.tests_failed,
      events,
    })
  }

  summaries.sort((a, b) => (b.started_at || 0) - (a.started_at || 0))
  return summaries
}

function fmtDuration(ms: number): string {
  if (ms <= 0) return '—'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rem = s % 60
  return rem > 0 ? `${m}m ${rem}s` : `${m}m`
}

function fmtTokens(n: number): string {
  if (n === 0) return '—'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function fmtDate(unixMs: number): string {
  if (!unixMs) return '—'
  const d = fromUnixTime(unixMs / 1000)
  const ago = formatDistanceToNow(d, { addSuffix: true })
  return ago
}

function fmtAbsDate(unixMs: number): string {
  if (!unixMs) return ''
  return format(fromUnixTime(unixMs / 1000), 'MMM d, HH:mm:ss')
}

function StatusBadge({ status, terminal_reason }: { status: RunSummary['status']; terminal_reason: string }) {
  if (status === 'running') {
    return (
      <Badge variant='outline' className='border-blue-400 text-blue-400 gap-1'>
        <Loader2 className='h-3 w-3 animate-spin' />
        running
      </Badge>
    )
  }
  if (status === 'failed') {
    return <Badge variant='destructive'>failed</Badge>
  }
  const reason = terminal_reason && terminal_reason !== 'success' ? terminal_reason : ''
  return (
    <Badge
      variant='outline'
      className='border-emerald-500 text-emerald-500'
      title={reason}
    >
      {reason || 'completed'}
    </Badge>
  )
}

function EventRow({ row }: { row: ExecutionRow }) {
  const eventColor =
    row.event === 'started'
      ? 'text-blue-400'
      : row.event === 'completed'
        ? 'text-emerald-400'
        : row.event === 'failed'
          ? 'text-rose-400'
          : 'text-muted-foreground'

  return (
    <div className='grid grid-cols-[7rem_6rem_1fr] gap-2 px-2 py-1 font-mono text-xs hover:bg-muted/30 rounded'>
      <span className='text-muted-foreground truncate' title={row.created_at}>
        {format(new Date(row.created_at), 'HH:mm:ss.SSS')}
      </span>
      <span className={`font-medium ${eventColor}`}>{row.event}</span>
      <span className='text-muted-foreground truncate'>
        {row.event === 'started' && row.model ? `model=${row.model}` : ''}
        {(row.event === 'sample' || row.event === 'completed' || row.event === 'failed') && row.turns > 0
          ? `turns=${row.turns} actions=${row.actions} ctx=${row.context_window_pct.toFixed(0)}%`
          : ''}
        {row.event === 'failed' && row.error ? ` err=${row.error.slice(0, 80)}` : ''}
        {row.event === 'completed' && row.terminal_reason && row.terminal_reason !== 'success'
          ? ` reason=${row.terminal_reason}`
          : ''}
      </span>
    </div>
  )
}

function RunRow({ run }: { run: RunSummary }) {
  const [expanded, setExpanded] = useState(false)
  const totalTokens = run.input_tokens + run.output_tokens
  const Icon = expanded ? ChevronDown : ChevronRight

  return (
    <div className='border-b last:border-0'>
      <button
        className='w-full text-left px-3 py-2 hover:bg-muted/40 flex items-center gap-3 transition-colors'
        onClick={() => setExpanded((v) => !v)}
      >
        <Icon className='h-4 w-4 text-muted-foreground flex-shrink-0' />

        <StatusBadge status={run.status} terminal_reason={run.terminal_reason} />

        <span className='text-muted-foreground text-xs min-w-[7rem]' title={fmtAbsDate(run.started_at)}>
          {fmtDate(run.started_at)}
        </span>

        <span className='text-xs font-mono min-w-[4rem]' title='duration'>
          {run.status === 'running' ? <span className='text-blue-400'>live</span> : fmtDuration(run.duration_ms)}
        </span>

        <span className='text-xs font-mono min-w-[5rem]' title='turns / actions'>
          {run.turns > 0 ? `${run.turns}t ${run.actions}a` : '—'}
        </span>

        <span className='text-xs font-mono min-w-[6rem]' title='tokens (in+out)'>
          {fmtTokens(totalTokens)}
        </span>

        {run.cache_hit_rate_pct > 0 && (
          <span className='text-xs text-emerald-400 font-mono min-w-[3rem]' title='cache hit %'>
            {run.cache_hit_rate_pct.toFixed(0)}%↩
          </span>
        )}

        {run.lines_added > 0 || run.lines_removed > 0 ? (
          <span className='text-xs font-mono' title='lines added/removed'>
            <span className='text-emerald-400'>+{run.lines_added}</span>
            {run.lines_removed > 0 && <span className='text-rose-400'> −{run.lines_removed}</span>}
          </span>
        ) : null}

        {run.commits > 0 && (
          <span className='text-xs text-muted-foreground font-mono' title='commits'>
            {run.commits}c
          </span>
        )}

        {run.model ? (
          <span className='text-xs text-muted-foreground ml-auto truncate max-w-[12rem]' title={run.model}>
            {run.model.replace('claude-', '')}
          </span>
        ) : null}
      </button>

      {expanded && (
        <div className='bg-muted/20 border-t px-4 py-2 space-y-0.5'>
          <div className='text-muted-foreground text-xs mb-2 flex gap-4'>
            {run.agent_runtime && <span>runtime: {run.agent_runtime}</span>}
            {run.trigger && <span>trigger: {run.trigger}</span>}
            {run.context_window_pct > 0 && <span>ctx: {run.context_window_pct.toFixed(0)}%</span>}
            {run.tests_run > 0 && (
              <span>
                tests: {run.tests_passed}/{run.tests_run}
                {run.tests_failed > 0 && <span className='text-rose-400'> ({run.tests_failed} failed)</span>}
              </span>
            )}
            <span className='font-mono'>run #{run.run_number}</span>
            <span className='font-mono text-muted-foreground/60' title={run.run_uid}>
              {run.run_uid.slice(0, 8)}
            </span>
          </div>
          {run.events.map((e) => (
            <EventRow key={e.id} row={e} />
          ))}
        </div>
      )}
    </div>
  )
}

export function RunsTab({
  kind: _kind,
  entityId,
}: {
  kind: EntityKind
  entityId: string
}) {
  const { data: rows, isLoading, isError, error } = useQuery<ExecutionRow[]>({
    queryKey: ['entity', entityId, 'runs'],
    queryFn: () =>
      fetch(`/api/entities/${entityId}/runs`).then((r) => r.json()),
    enabled: !!entityId,
    refetchInterval: (q) => {
      const rows = q.state.data ?? []
      const runUids = new Set(rows.map((r) => r.run_uid))
      const started = new Set(rows.filter((r) => r.event === 'started').map((r) => r.run_uid))
      const terminal = new Set(
        rows
          .filter((r) => r.event === 'completed' || r.event === 'failed')
          .map((r) => r.run_uid),
      )
      const hasRunning = [...runUids].some((uid) => started.has(uid) && !terminal.has(uid))
      return hasRunning ? 2000 : false
    },
    refetchIntervalInBackground: false,
    staleTime: 5_000,
  })

  const runs = useMemo(() => groupRuns(rows ?? []), [rows])

  if (isLoading) {
    return (
      <div className='space-y-2'>
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className='h-10 w-full' />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className='text-sm text-rose-400'>
        Failed to load runs: {String(error)}
      </div>
    )
  }

  if (runs.length === 0) {
    return (
      <div className='text-muted-foreground text-sm py-8 text-center'>
        No runs yet.
      </div>
    )
  }

  return (
    <div className='rounded-md border divide-y-0 text-sm'>
      <div className='grid grid-cols-[7rem_6rem_7rem_4rem_5rem_6rem_1fr] gap-3 px-3 py-1.5 text-muted-foreground text-xs border-b bg-muted/30 font-medium'>
        <span className='col-start-2'>status</span>
        <span>when</span>
        <span>dur</span>
        <span>t/a</span>
        <span>tokens</span>
        <span>model</span>
      </div>
      {runs.map((run) => (
        <RunRow key={run.run_uid} run={run} />
      ))}
    </div>
  )
}
