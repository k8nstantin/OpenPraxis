import { createFileRoute, useNavigate } from '@tanstack/react-router'
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
  terminal_reason: string
  started_at: number
  duration_ms: number
  turns: number
  actions: number
  input_tokens: number
  output_tokens: number
  cache_hit_rate_pct: number
  lines_added: number
  lines_removed: number
  error: string
  created_at: string
}

function realDate(ev: ActivityEvent): Date {
  if (ev.started_at && ev.started_at > 0) return new Date(ev.started_at)
  return new Date(ev.created_at)
}

function fmtDur(ms: number): string {
  if (!ms) return '—'
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}

function fmtTok(n: number): string {
  if (!n) return '—'
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`
  return String(n)
}

function entityPath(type: string, uid: string): string | null {
  const map: Record<string, string> = {
    task: '/tasks', product: '/products', manifest: '/manifests',
    skill: '/skills', idea: '/ideas',
  }
  const base = map[type]
  if (!base) return null
  return `${base}?id=${uid}&tab=runs`
}

// Grid columns: time | status | entity | turns | actions | tokens | cache | dur | lines | ago
const GRID = 'grid grid-cols-[9rem_5rem_1fr_4rem_4rem_5rem_4rem_5rem_7rem_7rem] gap-x-3 items-center'

function EventRow({ ev, isRunning }: { ev: ActivityEvent; isRunning: boolean }) {
  const navigate = useNavigate()
  const totalTok = ev.input_tokens + ev.output_tokens
  const path = entityPath(ev.entity_type, ev.entity_uid)

  const handleClick = () => {
    if (!path) return
    navigate({ to: path } as Parameters<typeof navigate>[0])
  }

  return (
    <div
      className={`${GRID} py-2 px-2 border-b last:border-0 text-xs hover:bg-muted/30 transition-colors ${path ? 'cursor-pointer' : ''}`}
      onClick={path ? handleClick : undefined}
    >
      {/* time */}
      <span className='text-muted-foreground font-mono truncate' title={ev.created_at}>
        {format(realDate(ev), 'MMM d, HH:mm')}
      </span>

      {/* status */}
      {ev.event === 'started' && isRunning ? (
        <Badge variant='outline' className='border-blue-400 text-blue-400 gap-1 justify-center text-xs'>
          <Loader2 className='h-3 w-3 animate-spin' />running
        </Badge>
      ) : ev.event === 'started' ? (
        <Badge variant='outline' className='border-sky-600 text-sky-600 justify-center text-xs'>running</Badge>
      ) : ev.event === 'completed' ? (
        <Badge variant='outline' className='border-emerald-500 text-emerald-500 justify-center text-xs'>done</Badge>
      ) : (
        <Badge variant='destructive' className='justify-center text-xs'>failed</Badge>
      )}

      {/* entity */}
      <span className='font-medium truncate' title={ev.entity_title}>
        {ev.entity_title}
        {ev.entity_type && ev.entity_type !== 'interactive' && (
          <span className='text-muted-foreground font-normal ml-1'>· {ev.entity_type}</span>
        )}
      </span>

      {/* turns */}
      <span className='font-mono text-muted-foreground text-right'>
        {ev.turns > 0 ? ev.turns : '—'}
      </span>

      {/* actions */}
      <span className='font-mono text-muted-foreground text-right'>
        {ev.actions > 0 ? ev.actions : '—'}
      </span>

      {/* tokens */}
      <span className='font-mono text-muted-foreground text-right'>
        {fmtTok(totalTok)}
      </span>

      {/* cache */}
      <span className={`font-mono text-right ${ev.cache_hit_rate_pct > 0 ? 'text-emerald-400' : 'text-muted-foreground'}`}>
        {ev.cache_hit_rate_pct > 0 ? `${ev.cache_hit_rate_pct.toFixed(0)}%` : '—'}
      </span>

      {/* duration */}
      <span className='font-mono text-muted-foreground text-right'>
        {fmtDur(ev.duration_ms)}
      </span>

      {/* lines */}
      <span className='font-mono text-right'>
        {ev.lines_added > 0
          ? <><span className='text-emerald-400'>+{ev.lines_added}</span>{ev.lines_removed > 0 && <span className='text-rose-400'> −{ev.lines_removed}</span>}</>
          : <span className='text-muted-foreground'>—</span>
        }
      </span>

      {/* ago */}
      <span className='text-muted-foreground text-right truncate'>
        {formatDistanceToNow(realDate(ev), { addSuffix: true })}
      </span>
    </div>
  )
}

function ActiveFeed({ events }: { events: ActivityEvent[] }) {
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

  return (
    <div className='rounded-md border text-sm'>
      {/* header row */}
      <div className={`${GRID} px-2 py-1.5 border-b bg-muted/30 text-xs text-muted-foreground font-medium`}>
        <span>time</span>
        <span>status</span>
        <span>entity</span>
        <span className='text-right'>turns</span>
        <span className='text-right'>actions</span>
        <span className='text-right'>tokens</span>
        <span className='text-right'>cache</span>
        <span className='text-right'>dur</span>
        <span className='text-right'>lines</span>
        <span className='text-right'>ago</span>
      </div>
      {events
        .filter((ev) => ev.event !== 'started' || activeRunUids.has(ev.run_uid))
        .map((ev, i) => (
          <EventRow
            key={`${ev.run_uid}-${ev.event}-${i}`}
            ev={ev}
            isRunning={activeRunUids.has(ev.run_uid)}
          />
        ))}
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
          {events && <span className='text-sm text-muted-foreground'>{events.length} events</span>}
        </div>

        {isLoading && (
          <div className='space-y-1'>
            {Array.from({ length: 12 }).map((_, i) => (
              <Skeleton key={i} className='h-9 w-full' />
            ))}
          </div>
        )}

        {isError && <div className='text-sm text-rose-400'>Failed to load activity.</div>}

        {events?.length === 0 && (
          <div className='text-muted-foreground text-sm py-12 text-center'>No activity yet.</div>
        )}

        {events && events.length > 0 && <ActiveFeed events={events} />}
      </Main>
    </>
  )
}
