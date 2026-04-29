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
} from 'lucide-react'
import {
  useTaskOutput,
  useTaskLatestRun,
} from '@/lib/queries/entity'
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

// Live Output tab — task-only. Stream-json events from the running
// agent, rendered as a Claude-Code-style structured event log.
//
// Source-switching:
//   - while runner reports `running:true` we poll /api/tasks/{id}/output
//     (200-line ring buffer)
//   - once running flips false (or was never true) we fall back to the
//     latest run's `output` TEXT blob (full transcript)
//
// Renderer:
//   - virtuoso list, stick-to-bottom while running, freeze on user scroll
//   - one card style per block kind: text (markdown), tool_use
//     (collapsible chip), tool_result (with error ring), result (summary)

interface LiveOutputTabProps {
  entityId: string
}

export function LiveOutputTab({ entityId }: LiveOutputTabProps) {
  const live = useTaskOutput(entityId)
  const lastRun = useTaskLatestRun(entityId)

  // Source selection: while live.running stay on live; otherwise fall
  // back to last run blob. Avoids a flash of empty when the runner is
  // idle but a previous run exists.
  const lines = useMemo<string[]>(() => {
    if (live.data?.running && live.data.lines.length > 0) return live.data.lines
    if (live.data?.lines && live.data.lines.length > 0) return live.data.lines
    if (lastRun.data?.output) return lastRun.data.output.split('\n')
    return []
  }, [live.data, lastRun.data])

  const parsed = useMemo(() => parseTaskOutput(lines), [lines])

  const running = live.data?.running ?? false
  const totalLines = parsed.totalLines
  const turns = useMemo(
    () =>
      parsed.blocks.reduce(
        (m, b) => (b.type === 'text' || b.type === 'tool_use' ? Math.max(m, b.turn) : m),
        0
      ),
    [parsed.blocks]
  )
  const cost = useMemo(() => {
    const last = [...parsed.blocks]
      .reverse()
      .find((b) => b.type === 'result') as ResultBlock | undefined
    return last?.costUsd ?? lastRun.data?.cost_usd ?? 0
  }, [parsed.blocks, lastRun.data])

  // Stick-to-bottom behavior. Virtuoso provides atBottomStateChange; we
  // freeze auto-scroll when the user scrolls up, resume when they scroll
  // back to the bottom OR click the "Resume follow" button.
  const ref = useRef<VirtuosoHandle | null>(null)
  const [follow, setFollow] = useState(true)
  useEffect(() => {
    if (follow && ref.current) {
      ref.current.scrollToIndex({
        index: parsed.blocks.length - 1,
        align: 'end',
      })
    }
  }, [parsed.blocks.length, follow])

  if (live.isLoading && !live.data && !lastRun.data) {
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

  if (!running && parsed.blocks.length === 0) {
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
            <StatusPill running={running} />
            <Stat label='Turns' value={String(turns)} />
            <Stat label='Lines' value={String(totalLines)} />
            <Stat
              label='Cost'
              value={cost > 0 ? `$${cost.toFixed(4)}` : '—'}
            />
          </div>
          <div className='flex items-center gap-1'>
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
            <Button
              size='sm'
              variant='ghost'
              onClick={() => navigator.clipboard.writeText(lines.join('\n'))}
              className='h-7 gap-1 text-xs'
              title='Copy raw stream-json'
            >
              <Copy className='h-3.5 w-3.5' />
              Copy
            </Button>
            <Button
              size='sm'
              variant='ghost'
              onClick={() => downloadBlob(lines.join('\n'), `task-${entityId}.jsonl`)}
              className='h-7 gap-1 text-xs'
              title='Download as .jsonl'
            >
              <Download className='h-3.5 w-3.5' />
              Save
            </Button>
          </div>
        </div>

        <div className='h-[calc(100vh-22rem)] min-h-[480px]'>
          <Virtuoso
            ref={ref}
            data={parsed.blocks}
            followOutput={follow ? 'smooth' : false}
            atBottomStateChange={(at) => {
              if (at && !follow) setFollow(true)
            }}
            itemContent={(_, b) => <BlockRow block={b} />}
            className='[&_>_div]:px-3'
          />
        </div>
      </CardContent>
    </Card>
  )
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

function BlockRow({ block }: { block: OutputBlock }) {
  if (block.type === 'text') return <TextRow b={block} />
  if (block.type === 'tool_use') return <ToolUseRow b={block} />
  if (block.type === 'tool_result') return <ToolResultRow b={block} />
  return <ResultRow b={block} />
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
