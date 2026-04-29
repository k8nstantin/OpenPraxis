import { useEffect, useMemo, useRef, useState } from 'react'
import { Virtuoso, type VirtuosoHandle } from 'react-virtuoso'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import {
  ChevronRight,
  Copy,
  Download,
  Pause,
  Play,
  Wrench,
  CircleAlert,
  CircleCheck,
  History,
  Radio,
} from 'lucide-react'
import { useTaskOutput, useTaskRuns } from '@/lib/queries/entity'
import {
  parseTaskOutput,
  type OutputBlock,
  type TextBlock,
  type ToolUseBlock,
  type ToolResultBlock,
  type ResultBlock,
} from '@/lib/parse-output'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

// Live Output tab — task-only.
//
// Single scrolling feed that shows the **current run live** at the top
// (polled at 750ms while runner reports running:true, off the 200-line
// ring buffer at /api/tasks/{id}/output) followed by **every previous
// completed run** in DESC order (newest first), each delimited by a
// run-divider card with run #, status, cost, turns, timestamps.
//
// All runs render in the same Claude-Code-style structured block
// format: text → markdown card, tool_use → expandable chip,
// tool_result → output panel, result → run summary card.
//
// Rationale: a single chronological feed is the cleanest mental model
// for "show me everything this task has ever done." Run dividers act
// as section anchors so it's still trivial to find a specific run.

interface LiveOutputTabProps {
  entityId: string
}

interface RunDividerItem {
  type: 'run_divider'
  runNumber: number
  status: string
  costUsd: number
  turns: number
  lines: number
  startedAt?: string
  completedAt?: string
  isLive?: boolean
}

type FeedItem = OutputBlock | RunDividerItem

export function LiveOutputTab({ entityId }: LiveOutputTabProps) {
  const live = useTaskOutput(entityId)
  const runs = useTaskRuns(entityId)

  const isRunning = live.data?.running ?? false
  const liveLines = live.data?.lines ?? []
  const runRows = runs.data ?? []

  // Merge: current/live run on top → completed runs DESC.
  const feed = useMemo<FeedItem[]>(() => {
    const items: FeedItem[] = []

    if (isRunning && liveLines.length > 0) {
      const nextRunNum = (runRows[0]?.run_number ?? 0) + 1
      items.push({
        type: 'run_divider',
        runNumber: nextRunNum,
        status: 'running',
        costUsd: 0,
        turns: 0,
        lines: liveLines.length,
        isLive: true,
      })
      items.push(...parseTaskOutput(liveLines).blocks)
    }

    for (const r of runRows) {
      items.push({
        type: 'run_divider',
        runNumber: r.run_number,
        status: r.status,
        costUsd: r.cost_usd,
        turns: r.turns,
        lines: r.lines,
        startedAt: r.started_at,
        completedAt: r.completed_at,
      })
      if (r.output) {
        items.push(...parseTaskOutput(r.output.split('\n')).blocks)
      }
    }

    return items
  }, [isRunning, liveLines, runRows])

  // Header summary numbers — use live during a run, last completed run
  // otherwise (so an idle task still shows useful header stats).
  const headerStats = useMemo(() => {
    if (isRunning && liveLines.length > 0) {
      const lp = parseTaskOutput(liveLines)
      const turns = lp.blocks.reduce(
        (m, b) =>
          b.type === 'text' || b.type === 'tool_use' ? Math.max(m, b.turn) : m,
        0
      )
      return {
        turns,
        lines: lp.totalLines,
        cost: 0,
      }
    }
    if (runRows[0]) {
      return {
        turns: runRows[0].turns,
        lines: runRows[0].lines,
        cost: runRows[0].cost_usd,
      }
    }
    return { turns: 0, lines: 0, cost: 0 }
  }, [isRunning, liveLines, runRows])

  // Cumulative cost across all runs — useful for tasks that loop.
  const totalCost = useMemo(
    () => runRows.reduce((s, r) => s + (r.cost_usd ?? 0), 0),
    [runRows]
  )

  // Stick-to-bottom only matters during streaming. Disabled by default
  // for completed feeds (user lands at the top, scrolls down through
  // history naturally).
  const ref = useRef<VirtuosoHandle | null>(null)
  const [follow, setFollow] = useState(true)
  useEffect(() => {
    if (isRunning && follow && ref.current) {
      // Scroll to the latest live block (the live section is at the top
      // of the feed; new blocks slot into it). Find the last block that
      // belongs to the live run — that's everything before the first
      // non-live divider.
      let lastLive = -1
      for (let i = 0; i < feed.length; i++) {
        const item = feed[i]
        if (item.type === 'run_divider' && !item.isLive) break
        lastLive = i
      }
      if (lastLive > 0) {
        ref.current.scrollToIndex({ index: lastLive, align: 'end' })
      }
    }
  }, [feed, isRunning, follow])

  // All hooks above this line — early returns below stay rule-of-hooks safe.
  const allRaw = useRawDump(isRunning, liveLines, runRows)

  if ((live.isLoading || runs.isLoading) && !live.data && !runs.data) {
    return (
      <Card>
        <CardContent className='space-y-3 p-4'>
          <Skeleton className='h-6 w-48' />
          <Skeleton className='h-4 w-full' />
          <Skeleton className='h-4 w-full' />
          <Skeleton className='h-4 w-3/4' />
        </CardContent>
      </Card>
    )
  }

  if (feed.length === 0) {
    return (
      <Card>
        <CardContent className='text-muted-foreground p-8 text-center text-sm'>
          No output yet — run this task to see streaming agent output here.
        </CardContent>
      </Card>
    )
  }

  return (
    <Card className='gap-0 py-0'>
      <CardContent className='p-0'>
        <div className='flex items-center justify-between border-b px-4 py-2'>
          <div className='flex items-center gap-3'>
            <StatusPill running={isRunning} />
            <Stat label='Turns' value={String(headerStats.turns)} />
            <Stat label='Lines' value={String(headerStats.lines)} />
            <Stat label='Runs' value={String(runRows.length + (isRunning ? 1 : 0))} />
            <Stat
              label='Cost·all'
              value={totalCost > 0 ? `$${totalCost.toFixed(4)}` : '—'}
            />
          </div>
          <div className='flex items-center gap-1'>
            {isRunning && (
              <Button
                size='sm'
                variant='ghost'
                onClick={() => setFollow((f) => !f)}
                className='h-7 gap-1 text-xs'
                title={follow ? 'Pause auto-scroll' : 'Resume auto-scroll'}
              >
                {follow ? (
                  <Pause className='h-3.5 w-3.5' />
                ) : (
                  <Play className='h-3.5 w-3.5' />
                )}
                {follow ? 'Following' : 'Paused'}
              </Button>
            )}
            <Button
              size='sm'
              variant='ghost'
              onClick={() => navigator.clipboard.writeText(allRaw)}
              className='h-7 gap-1 text-xs'
              title='Copy raw stream-json (all runs)'
            >
              <Copy className='h-3.5 w-3.5' />
              Copy
            </Button>
            <Button
              size='sm'
              variant='ghost'
              onClick={() => downloadBlob(allRaw, `task-${entityId}.jsonl`)}
              className='h-7 gap-1 text-xs'
              title='Download all runs as .jsonl'
            >
              <Download className='h-3.5 w-3.5' />
              Save
            </Button>
          </div>
        </div>

        <div className='h-[calc(100vh-22rem)] min-h-[480px]'>
          <Virtuoso
            ref={ref}
            data={feed}
            followOutput={isRunning && follow ? 'smooth' : false}
            atBottomStateChange={(at) => {
              if (at && !follow) setFollow(true)
            }}
            itemContent={(_, item) => <FeedRow item={item} />}
            className='[&_>_div]:px-3'
          />
        </div>
      </CardContent>
    </Card>
  )
}

// useRawDump — concat live lines + every run's stored output into a
// single .jsonl blob for copy / download.
function useRawDump(
  isRunning: boolean,
  liveLines: string[],
  runRows: { output: string; run_number: number }[]
): string {
  return useMemo(() => {
    const parts: string[] = []
    if (isRunning && liveLines.length > 0) {
      parts.push(`// === live (in flight) ===`)
      parts.push(...liveLines)
    }
    for (const r of runRows) {
      parts.push(`// === run ${r.run_number} ===`)
      if (r.output) parts.push(r.output)
    }
    return parts.join('\n')
  }, [isRunning, liveLines, runRows])
}

function StatusPill({ running }: { running: boolean }) {
  return (
    <div className='flex items-center gap-1.5 text-xs'>
      <span
        className={cn(
          'inline-block h-2 w-2 rounded-full',
          running
            ? 'bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,.8)] animate-pulse'
            : 'bg-zinc-500'
        )}
      />
      <span className='font-medium'>{running ? 'Running' : 'Idle'}</span>
    </div>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className='flex items-baseline gap-1 text-xs'>
      <span className='text-muted-foreground'>{label}</span>
      <span className='font-mono font-medium tabular-nums'>{value}</span>
    </div>
  )
}

function FeedRow({ item }: { item: FeedItem }) {
  if (item.type === 'run_divider') return <RunDividerRow d={item} />
  if (item.type === 'text') return <TextRow b={item} />
  if (item.type === 'tool_use') return <ToolUseRow b={item} />
  if (item.type === 'tool_result') return <ToolResultRow b={item} />
  return <ResultRow b={item} />
}

function RunDividerRow({ d }: { d: RunDividerItem }) {
  // Strong horizontal anchor so users can scan to a run boundary at a
  // glance. Color the header by status so the eye picks out failed
  // runs against successful ones.
  const tone =
    d.isLive
      ? 'border-emerald-500/60 bg-emerald-500/[.08]'
      : d.status === 'failed' || d.status === 'error'
        ? 'border-rose-500/60 bg-rose-500/[.06]'
        : d.status === 'completed'
          ? 'border-violet-500/40 bg-violet-500/[.06]'
          : 'border-zinc-500/40 bg-zinc-500/[.04]'

  const Icon = d.isLive ? Radio : History
  const duration = formatDuration(d.startedAt, d.completedAt)

  return (
    <div className={cn('my-3 rounded-md border px-3 py-2', tone)}>
      <div className='flex items-center justify-between gap-2'>
        <div className='flex items-center gap-2'>
          <Icon
            className={cn(
              'h-3.5 w-3.5',
              d.isLive ? 'animate-pulse text-emerald-400' : 'text-zinc-400'
            )}
          />
          <span className='text-sm font-semibold'>
            {d.isLive ? `Run #${d.runNumber} — live` : `Run #${d.runNumber}`}
          </span>
          <Badge
            variant='outline'
            className={cn(
              'text-[10px] capitalize',
              d.isLive
                ? 'border-emerald-500/60 text-emerald-200'
                : d.status === 'failed' || d.status === 'error'
                  ? 'border-rose-500/60 text-rose-300'
                  : d.status === 'completed'
                    ? 'border-violet-500/50 text-violet-200'
                    : 'border-zinc-500/50 text-zinc-300'
            )}
          >
            {d.status}
          </Badge>
        </div>
        <div className='text-muted-foreground flex items-baseline gap-3 text-[11px]'>
          {d.startedAt && (
            <span title={d.startedAt}>{formatTimestamp(d.startedAt)}</span>
          )}
          {duration && <span>· {duration}</span>}
          <Stat label='turns' value={String(d.turns)} />
          <Stat label='lines' value={String(d.lines)} />
          <Stat
            label='cost'
            value={d.costUsd > 0 ? `$${d.costUsd.toFixed(4)}` : '—'}
          />
        </div>
      </div>
    </div>
  )
}

function TurnBadge({ turn }: { turn: number }) {
  return (
    <Badge
      variant='outline'
      className='font-mono text-[10px] tracking-tight'
    >
      T{turn}
    </Badge>
  )
}

function TextRow({ b }: { b: TextBlock }) {
  return (
    <div className='border-l-2 border-emerald-500/60 bg-emerald-500/[.04] py-2 pl-3 pr-2 my-1 rounded-r-md'>
      <div className='mb-1 flex items-center gap-2'>
        <TurnBadge turn={b.turn} />
        <span className='text-muted-foreground text-[10px] uppercase tracking-wider'>
          assistant
        </span>
      </div>
      <div className='prose prose-invert prose-sm max-w-none text-sm leading-relaxed [&_code]:rounded [&_code]:bg-zinc-800 [&_code]:px-1 [&_code]:py-0.5 [&_pre]:rounded-md [&_pre]:bg-zinc-900 [&_pre]:p-2 [&_pre]:text-xs'>
        <ReactMarkdown remarkPlugins={[remarkGfm]}>{b.text}</ReactMarkdown>
      </div>
    </div>
  )
}

function ToolUseRow({ b }: { b: ToolUseBlock }) {
  const [open, setOpen] = useState(false)
  return (
    <div className='border-l-2 border-sky-500/60 bg-sky-500/[.04] my-1 rounded-r-md'>
      <button
        type='button'
        onClick={() => setOpen((o) => !o)}
        className='flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-sky-500/[.08]'
      >
        <ChevronRight
          className={cn(
            'text-muted-foreground h-3.5 w-3.5 transition-transform',
            open && 'rotate-90'
          )}
        />
        <Wrench className='h-3.5 w-3.5 text-sky-400' />
        <TurnBadge turn={b.turn} />
        <code className='font-mono text-xs font-semibold text-sky-300'>
          {b.name}
        </code>
        <code className='text-muted-foreground truncate font-mono text-xs'>
          {b.inputPreview}
        </code>
      </button>
      {open && (
        <pre className='mx-3 mb-2 overflow-x-auto rounded-md bg-zinc-900 p-2 text-[11px] leading-relaxed'>
          {JSON.stringify(b.inputRaw, null, 2)}
        </pre>
      )}
    </div>
  )
}

function ToolResultRow({ b }: { b: ToolResultBlock }) {
  const [open, setOpen] = useState(false)
  const truncated = b.preview.length > 240
  const head = truncated ? b.preview.slice(0, 240) + '…' : b.preview
  return (
    <div
      className={cn(
        'my-1 rounded-r-md border-l-2 px-3 py-1.5',
        b.isError
          ? 'border-rose-500/70 bg-rose-500/[.05]'
          : 'border-zinc-600/60 bg-zinc-500/[.04]'
      )}
    >
      <div className='mb-1 flex items-center gap-2'>
        {b.isError ? (
          <CircleAlert className='h-3.5 w-3.5 text-rose-400' />
        ) : (
          <CircleCheck className='h-3.5 w-3.5 text-zinc-400' />
        )}
        <span className='text-muted-foreground text-[10px] uppercase tracking-wider'>
          {b.isError ? 'tool error' : 'tool result'}
        </span>
        {truncated && (
          <button
            type='button'
            onClick={() => setOpen((o) => !o)}
            className='text-muted-foreground hover:text-foreground ml-auto text-[10px] underline-offset-2 hover:underline'
          >
            {open ? 'collapse' : 'expand'}
          </button>
        )}
      </div>
      <pre className='font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-words'>
        {open ? b.preview : head}
      </pre>
    </div>
  )
}

function ResultRow({ b }: { b: ResultBlock }) {
  return (
    <div className='my-2 rounded-md border border-violet-500/40 bg-violet-500/[.06] px-3 py-2'>
      <div className='mb-1 flex items-center gap-2'>
        <span className='text-[10px] font-semibold uppercase tracking-wider text-violet-300'>
          run finished
        </span>
        <Badge
          variant='outline'
          className='border-violet-500/40 text-violet-200'
        >
          {b.terminalReason}
        </Badge>
      </div>
      <div className='flex items-baseline gap-4 text-xs'>
        <Stat label='Turns' value={String(b.numTurns)} />
        <Stat label='Cost' value={`$${b.costUsd.toFixed(4)}`} />
      </div>
    </div>
  )
}

function downloadBlob(content: string, filename: string) {
  const blob = new Blob([content], { type: 'application/x-ndjson' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

function formatTimestamp(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  // Local time, day-aware: show "HH:MM" if today, else "Mon DD HH:MM".
  const now = new Date()
  const sameDay =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate()
  if (sameDay) {
    return d.toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
    })
  }
  return d.toLocaleDateString(undefined, {
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function formatDuration(start?: string, end?: string): string {
  if (!start || !end) return ''
  const s = new Date(start).getTime()
  const e = new Date(end).getTime()
  if (Number.isNaN(s) || Number.isNaN(e) || e < s) return ''
  const ms = e - s
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  const m = Math.floor(ms / 60_000)
  const sec = Math.floor((ms % 60_000) / 1000)
  return `${m}m${sec}s`
}
