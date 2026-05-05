import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { formatDistanceToNow, format } from 'date-fns'
import { Loader2 } from 'lucide-react'
import { useMemo } from 'react'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'

export const Route = createFileRoute('/_authenticated/activity')({
  component: ActivityPage,
})

interface ActivityEvent {
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

function realDate(ev: ActivityEvent): Date {
  // started_at is Unix ms; prefer it over created_at which may be a migration timestamp.
  if (ev.started_at && ev.started_at > 0) return new Date(ev.started_at)
  return new Date(ev.created_at)
}

function fmtAgo(ev: ActivityEvent) {
  try { return formatDistanceToNow(realDate(ev), { addSuffix: true }) }
  catch { return '' }
}

function fmtTime(ev: ActivityEvent) {
  try { return format(realDate(ev), 'MMM d, HH:mm') }
  catch { return '' }
}

function fmtDur(ms: number) {
  if (!ms) return ''
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}

function fmtTok(n: number) {
  if (!n) return ''
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`
  return String(n)
}

function EventBadge({ event, isRunning }: { event: ActivityEvent['event']; isRunning: boolean }) {
  if (event === 'started')
    return isRunning
      ? <Badge variant='outline' className='border-blue-400 text-blue-400 w-20 justify-center text-xs gap-1'><Loader2 className='h-3 w-3 animate-spin' />running</Badge>
      : <Badge variant='outline' className='border-muted-foreground text-muted-foreground w-20 justify-center text-xs'>running</Badge>
  if (event === 'completed')
    return <Badge variant='outline' className='border-emerald-500 text-emerald-500 w-20 justify-center text-xs'>done</Badge>
  return <Badge variant='destructive' className='w-20 justify-center text-xs'>failed</Badge>
}

function EventRow({ ev, activeRunUids }: { ev: ActivityEvent; activeRunUids: Set<string> }) {
  const totalTok = ev.input_tokens + ev.output_tokens
  const isRunning = ev.event === 'started' && activeRunUids.has(ev.run_uid)

  return (
    <div className='flex items-center gap-3 py-2 border-b last:border-0 hover:bg-muted/20 px-1 text-sm'>
      <span className='text-muted-foreground font-mono text-xs w-24 flex-shrink-0' title={ev.created_at}>
        {fmtTime(ev)}
      </span>

      <EventBadge event={ev.event} isRunning={isRunning} />

      <span className='flex-1 min-w-0 truncate font-medium' title={ev.entity_title}>
        {ev.entity_title}
      </span>

      <span className='text-muted-foreground font-mono text-xs min-w-[4rem] text-right'>
        {ev.turns > 0 ? `${ev.turns}t` : ''}
        {ev.actions > 0 ? ` ${ev.actions}a` : ''}
      </span>

      {totalTok > 0 && (
        <span className='text-muted-foreground font-mono text-xs min-w-[3.5rem] text-right'>
          {fmtTok(totalTok)}
        </span>
      )}

      {ev.cache_hit_rate_pct > 0 && (
        <span className='text-emerald-400 font-mono text-xs w-10 text-right'>
          {ev.cache_hit_rate_pct.toFixed(0)}%
        </span>
      )}

      {ev.duration_ms > 0 && (
        <span className='text-muted-foreground font-mono text-xs w-12 text-right'>
          {fmtDur(ev.duration_ms)}
        </span>
      )}

      {ev.lines_added > 0 && (
        <span className='font-mono text-xs'>
          <span className='text-emerald-400'>+{ev.lines_added}</span>
          {ev.lines_removed > 0 && <span className='text-rose-400'> −{ev.lines_removed}</span>}
        </span>
      )}

      {ev.entity_type && ev.entity_type !== 'interactive' && (
        <Badge variant='outline' className='text-xs px-1.5 h-4 flex-shrink-0'>{ev.entity_type}</Badge>
      )}

      <span className='text-muted-foreground text-xs w-20 text-right flex-shrink-0'>
        {fmtAgo(ev)}
      </span>
    </div>
  )
}

function ActivityPage() {
  const { data: events, isLoading, isError } = useQuery<ActivityEvent[]>({
    queryKey: ['activity-events'],
    queryFn: () => fetch('/api/execution/recent?limit=200').then((r) => r.json()),
    refetchInterval: 5_000,
    staleTime: 3_000,
  })

  return (
    <>
      <Header />
      <Main>
        <div className='mb-4 flex items-center justify-between'>
          <h1 className='text-2xl font-bold tracking-tight'>Activity</h1>
          {events && (
            <span className='text-sm text-muted-foreground'>{events.length} events</span>
          )}
        </div>

        {isLoading && (
          <div className='space-y-1'>
            {Array.from({ length: 12 }).map((_, i) => (
              <Skeleton key={i} className='h-9 w-full' />
            ))}
          </div>
        )}

        {isError && (
          <div className='text-sm text-rose-400'>Failed to load activity.</div>
        )}

        {events?.length === 0 && (
          <div className='text-muted-foreground text-sm py-12 text-center'>No activity yet.</div>
        )}

        {events && events.length > 0 && (
          <ActiveFeed events={events} />
        )}
      </Main>
    </>
  )
}

function ActiveFeed({ events }: { events: ActivityEvent[] }) {
  // A run is active if it has a started row but no terminal (completed/failed) in this window.
  const activeRunUids = useMemo(() => {
    const terminal = new Set(
      events.filter((e) => e.event === 'completed' || e.event === 'failed').map((e) => e.run_uid)
    )
    return new Set(
      events
        .filter((e) => e.event === 'started' && !terminal.has(e.run_uid))
        .map((e) => e.run_uid)
    )
  }, [events])
  const deduped = events

  return (
    <div className='rounded-md border'>
      <div className='flex items-center gap-3 px-1 py-1.5 border-b bg-muted/30 text-xs text-muted-foreground font-medium'>
        <span className='w-24 flex-shrink-0'>time</span>
        <span className='w-20 flex-shrink-0'>event</span>
        <span className='flex-1'>entity</span>
        <span className='min-w-[4rem] text-right'>turns/actions</span>
        <span className='min-w-[3.5rem] text-right'>tokens</span>
        <span className='w-10 text-right'>cache</span>
        <span className='w-12 text-right'>dur</span>
        <span className='w-16'>lines</span>
        <span className='w-16'></span>
        <span className='w-20 text-right'>ago</span>
      </div>
      {events.map((ev, i) => (
        <EventRow key={`${ev.run_uid}-${ev.created_at}-${i}`} ev={ev} activeRunUids={activeRunUids} />
      ))}
    </div>
  )
}
