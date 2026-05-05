import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { formatDistanceToNow, fromUnixTime } from 'date-fns'
import { ChevronDown, ChevronRight, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import type { ExecutionRow } from '@/lib/types'

export const Route = createFileRoute('/_authenticated/activity')({
  component: ActivityPage,
})

interface RecentRun {
  run_uid: string
  entity_uid: string
  entity_title: string
  entity_type: string
  event: 'started' | 'sample' | 'completed' | 'failed'
  trigger: string
  model: string
  agent_runtime: string
  terminal_reason: string
  started_at: number
  duration_ms: number
  turns: number
  actions: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_hit_rate_pct: number
  lines_added: number
  lines_removed: number
  commits: number
  error: string
  created_at: string
}

function fmtDur(ms: number) {
  if (!ms) return '—'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}

function fmtTok(n: number) {
  if (!n) return '—'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`
  return String(n)
}

function fmtAgo(ts: number | string) {
  try {
    const d = typeof ts === 'number' ? fromUnixTime(ts / 1000) : new Date(ts as string)
    return formatDistanceToNow(d, { addSuffix: true })
  } catch {
    return '?'
  }
}

function StatusBadge({ run }: { run: RecentRun }) {
  if (run.event === 'started' || run.event === 'sample') {
    return (
      <Badge variant='outline' className='border-blue-400 text-blue-400 gap-1 flex-shrink-0'>
        <Loader2 className='h-3 w-3 animate-spin' />
        running
      </Badge>
    )
  }
  if (run.event === 'failed') {
    return <Badge variant='destructive' className='flex-shrink-0'>failed</Badge>
  }
  const label = run.terminal_reason && run.terminal_reason !== 'success' ? run.terminal_reason : 'done'
  return (
    <Badge variant='outline' className='border-emerald-500 text-emerald-500 flex-shrink-0' title={label}>
      {label}
    </Badge>
  )
}

function LiveStats({ runUid }: { runUid: string }) {
  const { data } = useQuery<ExecutionRow[]>({
    queryKey: ['execution', runUid],
    queryFn: () => fetch(`/api/execution/${runUid}`).then((r) => r.json()),
    refetchInterval: 2000,
    staleTime: 0,
  })
  const rows = data ?? []
  const latest = rows[rows.length - 1]
  if (!latest) return <Skeleton className='h-8 w-full mt-2' />
  return (
    <div className='mt-2 grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs'>
      {[
        ['turns', latest.turns],
        ['actions', latest.actions],
        ['ctx %', latest.context_window_pct?.toFixed(0) + '%'],
        ['cache hit', latest.cache_hit_rate_pct?.toFixed(0) + '%'],
      ].map(([label, val]) => (
        <div key={String(label)} className='bg-muted/40 rounded p-2'>
          <div className='text-muted-foreground'>{label}</div>
          <div className='font-mono font-medium'>{val || '—'}</div>
        </div>
      ))}
      <div className='col-span-2 sm:col-span-4 text-muted-foreground font-mono'>
        {rows.length} events recorded · live
      </div>
    </div>
  )
}

function CompletedStats({ run }: { run: RecentRun }) {
  const totalTok = run.input_tokens + run.output_tokens
  return (
    <div className='mt-2 grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs'>
      {[
        ['duration', fmtDur(run.duration_ms)],
        ['turns', run.turns],
        ['actions', run.actions],
        ['tokens', fmtTok(totalTok)],
        ['cache hit', run.cache_hit_rate_pct ? run.cache_hit_rate_pct.toFixed(0) + '%' : '—'],
        ['lines +', run.lines_added || '—'],
        ['lines −', run.lines_removed || '—'],
        ['commits', run.commits || '—'],
      ].map(([label, val]) => (
        <div key={String(label)} className='bg-muted/40 rounded p-2'>
          <div className='text-muted-foreground'>{label}</div>
          <div className='font-mono font-medium'>{val}</div>
        </div>
      ))}
      {run.error && (
        <div className='col-span-2 sm:col-span-4 text-rose-400 font-mono text-xs truncate'>
          {run.error}
        </div>
      )}
      {run.model && (
        <div className='col-span-2 sm:col-span-4 text-muted-foreground'>
          {run.model.replace('claude-', '')}
          {run.trigger ? ` · trigger: ${run.trigger}` : ''}
        </div>
      )}
    </div>
  )
}

function RunRow({ run }: { run: RecentRun }) {
  const [expanded, setExpanded] = useState(false)
  const isRunning = run.event === 'started' || run.event === 'sample'
  const ChevIcon = expanded ? ChevronDown : ChevronRight

  return (
    <div className='border-b last:border-0'>
      <button
        className='w-full text-left flex items-center gap-3 py-3 hover:bg-muted/40 transition-colors'
        onClick={() => setExpanded((v) => !v)}
      >
        <ChevIcon className='h-4 w-4 text-muted-foreground flex-shrink-0' />
        <StatusBadge run={run} />
        <div className='flex-1 min-w-0'>
          <span className='text-sm font-medium truncate block'>{run.entity_title}</span>
          <span className='text-xs text-muted-foreground'>
            {fmtAgo(run.started_at || run.created_at)}
            {!isRunning && run.duration_ms ? ` · ${fmtDur(run.duration_ms)}` : ''}
            {run.turns > 0 ? ` · ${run.turns}t` : ''}
          </span>
        </div>
        {run.entity_type && (
          <Badge variant='outline' className='text-xs flex-shrink-0'>{run.entity_type}</Badge>
        )}
      </button>

      {expanded && (
        <div className='px-8 pb-3'>
          {isRunning ? <LiveStats runUid={run.run_uid} /> : <CompletedStats run={run} />}
        </div>
      )}
    </div>
  )
}

function ActivityPage() {
  const { data: runs, isLoading, isError } = useQuery<RecentRun[]>({
    queryKey: ['execution-recent'],
    queryFn: () => fetch('/api/execution/recent?limit=100').then((r) => r.json()),
    refetchInterval: 10_000,
    staleTime: 5_000,
  })

  return (
    <>
      <Header />
      <Main>
        <div className='mb-4 flex items-center justify-between'>
          <h1 className='text-2xl font-bold tracking-tight'>Activity</h1>
          {runs && (
            <span className='text-sm text-muted-foreground'>{runs.length} runs</span>
          )}
        </div>

        {isLoading && (
          <div className='space-y-2'>
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className='h-14 w-full' />
            ))}
          </div>
        )}

        {isError && (
          <div className='text-sm text-rose-400'>Failed to load activity.</div>
        )}

        {runs?.length === 0 && (
          <div className='text-muted-foreground text-sm py-12 text-center'>
            No runs yet.
          </div>
        )}

        {runs && runs.length > 0 && (
          <div className='rounded-md border divide-y-0'>
            {runs.map((run) => (
              <RunRow key={run.run_uid} run={run} />
            ))}
          </div>
        )}
      </Main>
    </>
  )
}
