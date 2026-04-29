import { useEffect, useMemo, useRef, useState } from 'react'
import { Virtuoso } from 'react-virtuoso'
import {
  ChevronRight,
  Copy,
  Download,
  Search,
  ScrollText,
  ShieldAlert,
  ShieldCheck,
  Wrench,
  X,
} from 'lucide-react'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { useActions, type ActionRow } from '@/lib/queries/actions'
import { cn } from '@/lib/utils'

// Actions Log — auditable timeline of every tool call across every
// peer, agent, and session.
//
// Principles (not optional, this is the audit surface):
//  - Show every action that exists. Filters are additive, never lossy.
//  - Source attribution per row: action id, source_node, session_id,
//    task_id, full RFC3339 timestamp on hover.
//  - Search is server-side via /api/actions/search?q= (single endpoint
//    handles both browse and search — empty q == "all paged").
//  - Pagination is deterministic: offset + limit + total. The total
//    count is always visible so operators know what's missing.
//  - Errors highlighted but never hidden from the default view.
//  - Export to .jsonl for offline replay (keys not in the UI shape).

const PAGE_SIZE = 50

export function ActionsLogPage() {
  const [q, setQ] = useState('')
  const [debounced, setDebounced] = useState('')
  const [accumulated, setAccumulated] = useState<ActionRow[]>([])
  const [offset, setOffset] = useState(0)

  // Debounce the search input so we don't hammer the backend on every
  // keystroke. 300ms is the sweet spot between perceived responsiveness
  // and not firing 8 queries while typing "manifest_create".
  useEffect(() => {
    const t = setTimeout(() => setDebounced(q.trim()), 300)
    return () => clearTimeout(t)
  }, [q])

  // Reset accumulated rows + offset when the search query changes.
  // Also re-anchor to a fresh fetch.
  useEffect(() => {
    setAccumulated([])
    setOffset(0)
  }, [debounced])

  const page = useActions({ q: debounced, offset, limit: PAGE_SIZE })

  // Accumulate pages so "Load more" works without losing earlier rows.
  // Keyed by (q, offset) — the queryKey already covers cache-side; here
  // we're just stitching the pages on the client for the visible feed.
  useEffect(() => {
    if (!page.data) return
    setAccumulated((prev) => {
      // If offset===0 (new query), replace. Otherwise append.
      if (offset === 0) return page.data!.items ?? []
      const existing = new Set(prev.map((a) => a.id))
      const additions = (page.data!.items ?? []).filter(
        (a) => !existing.has(a.id)
      )
      return prev.concat(additions)
    })
  }, [page.data, offset])

  const total = page.data?.total ?? 0
  const hasMore = page.data?.has_more ?? false

  const loadMore = () => {
    if (page.isFetching || !hasMore) return
    setOffset(accumulated.length)
  }

  // Export current accumulated page as .jsonl — one action per line.
  // Doesn't dump the full table (could be 14k rows / many MB); operator
  // exports what they're looking at. To dump the whole table they can
  // hit /api/actions/search?q=&limit=50000 directly.
  const onExport = () => {
    const lines = accumulated.map((a) => JSON.stringify(a))
    const blob = new Blob([lines.join('\n')], { type: 'application/x-ndjson' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `actions-log-${new Date().toISOString().slice(0, 10)}.jsonl`
    link.click()
    URL.revokeObjectURL(url)
  }

  // Stats row — unique source nodes + tool names visible in the
  // current accumulated set. Cheap O(N) over a few hundred rows.
  const stats = useMemo(() => {
    const peers = new Set<string>()
    const tools = new Set<string>()
    let oldest = ''
    let newest = ''
    let errors = 0
    for (const a of accumulated) {
      if (a.source_node) peers.add(a.source_node)
      if (a.tool_name) tools.add(a.tool_name)
      if (!oldest || a.created_at < oldest) oldest = a.created_at
      if (!newest || a.created_at > newest) newest = a.created_at
      if (looksLikeError(a)) errors++
    }
    return {
      peers: peers.size,
      tools: tools.size,
      oldest,
      newest,
      errors,
    }
  }, [accumulated])

  return (
    <>
      <Header>
        <div className='flex items-center gap-2'>
          <ScrollText className='h-5 w-5 text-violet-400' />
          <h1 className='text-base font-semibold'>Actions Log</h1>
          <Badge
            variant='outline'
            className='border-violet-500/40 text-violet-300 text-[10px]'
          >
            audit · append-only
          </Badge>
        </div>
      </Header>
      <Main>
        <div className='mb-4 space-y-3'>
          <p className='text-muted-foreground text-sm'>
            Every tool call across every peer, agent, and session — filterable,
            searchable, exportable. Pagination is server-side; total count
            below reflects every matching row, not just what's loaded.
          </p>

          <div className='flex flex-wrap items-center gap-2'>
            <div className='relative min-w-[280px] flex-1 max-w-xl'>
              <Search className='text-muted-foreground absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2' />
              <Input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder='Search tool name, input, response, or action id…'
                className='pl-8 pr-8'
              />
              {q && (
                <button
                  type='button'
                  onClick={() => setQ('')}
                  className='text-muted-foreground hover:text-foreground absolute right-2 top-1/2 -translate-y-1/2'
                  title='Clear search'
                >
                  <X className='h-4 w-4' />
                </button>
              )}
            </div>
            <Button
              size='sm'
              variant='outline'
              onClick={onExport}
              disabled={accumulated.length === 0}
              className='gap-1.5'
              title='Download visible rows as .jsonl'
            >
              <Download className='h-3.5 w-3.5' />
              Export .jsonl
            </Button>
            <Button
              size='sm'
              variant='outline'
              disabled
              className='gap-1.5 opacity-60'
              title='Hash-chain verification ships in AAL/M1 (product 019dd94e-99f)'
            >
              <ShieldCheck className='h-3.5 w-3.5' />
              Verify chain
            </Button>
          </div>

          <div className='flex flex-wrap items-baseline gap-x-4 gap-y-1 text-xs'>
            <Stat
              label='Total matching'
              value={total.toLocaleString()}
              accent
            />
            <Stat label='Loaded' value={accumulated.length.toLocaleString()} />
            <Stat label='Peers' value={String(stats.peers)} />
            <Stat label='Tools' value={String(stats.tools)} />
            <Stat
              label='Errors'
              value={stats.errors > 0 ? String(stats.errors) : '0'}
              tone={stats.errors > 0 ? 'rose' : undefined}
            />
            <Stat
              label='Range'
              value={
                stats.oldest && stats.newest
                  ? `${formatTimestamp(stats.oldest)} – ${formatTimestamp(stats.newest)}`
                  : '—'
              }
            />
          </div>
        </div>

        {accumulated.length === 0 && page.isLoading ? (
          <Card>
            <CardContent className='space-y-3 p-4'>
              <Skeleton className='h-12 w-full' />
              <Skeleton className='h-12 w-full' />
              <Skeleton className='h-12 w-full' />
              <Skeleton className='h-12 w-full' />
            </CardContent>
          </Card>
        ) : accumulated.length === 0 ? (
          <Card>
            <CardContent className='text-muted-foreground p-8 text-center text-sm'>
              {debounced
                ? `No actions match "${debounced}".`
                : 'No actions recorded yet.'}
            </CardContent>
          </Card>
        ) : (
          <Card className='gap-0 py-0'>
            <CardContent className='p-0'>
              <div className='h-[calc(100vh-22rem)] min-h-[480px]'>
                <Virtuoso
                  data={accumulated}
                  endReached={() => loadMore()}
                  itemContent={(_, row) => <ActionRowCard row={row} />}
                  className='[&_>_div]:px-3'
                />
              </div>
              <div className='border-t px-4 py-2 flex items-center justify-between gap-2'>
                <span className='text-muted-foreground text-xs'>
                  Showing {accumulated.length.toLocaleString()} of{' '}
                  {total.toLocaleString()}
                  {debounced && (
                    <>
                      {' '}
                      matching <code className='font-mono'>{debounced}</code>
                    </>
                  )}
                </span>
                <Button
                  size='sm'
                  variant='ghost'
                  onClick={loadMore}
                  disabled={!hasMore || page.isFetching}
                  className='text-xs'
                >
                  {page.isFetching
                    ? 'Loading…'
                    : hasMore
                      ? `Load ${Math.min(PAGE_SIZE, total - accumulated.length)} more`
                      : 'All loaded'}
                </Button>
              </div>
            </CardContent>
          </Card>
        )}
      </Main>
    </>
  )
}

// Single action card — fixed-height-ish row that expands when the user
// clicks the input/response panes. Layout choices:
//  - Left edge: rose if looks-like-error, else zinc. Single-glance
//    error scanning down the feed.
//  - Header line: id · timestamp · source · session · task · tool.
//    Every attribution field present even when empty (rendered as —)
//    so the column structure is consistent across rows — important
//    for an audit surface.
//  - Body: collapsed by default to keep the list scannable; expand on
//    click. Default-closed means the operator deliberately chooses to
//    inspect a row, which matches the audit-review workflow.
function ActionRowCard({ row }: { row: ActionRow }) {
  const [open, setOpen] = useState(false)
  const isErr = looksLikeError(row)

  return (
    <div
      className={cn(
        'my-1 rounded-md border-l-2',
        isErr ? 'border-rose-500/70 bg-rose-500/[.04]' : 'border-zinc-600/60'
      )}
    >
      <button
        type='button'
        onClick={() => setOpen((o) => !o)}
        className='flex w-full items-start gap-2 px-3 py-2 text-left hover:bg-zinc-500/[.05]'
      >
        <ChevronRight
          className={cn(
            'text-muted-foreground mt-0.5 h-3.5 w-3.5 transition-transform',
            open && 'rotate-90'
          )}
        />
        {isErr ? (
          <ShieldAlert className='mt-0.5 h-3.5 w-3.5 text-rose-400' />
        ) : (
          <Wrench className='mt-0.5 h-3.5 w-3.5 text-sky-400' />
        )}
        <div className='min-w-0 flex-1'>
          <div className='flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-xs'>
            <code className='text-muted-foreground font-mono text-[10px]'>
              #{row.id}
            </code>
            <span
              className='text-muted-foreground tabular-nums'
              title={row.created_at}
            >
              {formatTimestamp(row.created_at)}
            </span>
            <code className='font-mono text-xs font-semibold text-sky-300'>
              {row.tool_name || '?'}
            </code>
            {row.task_id && (
              <Badge
                variant='outline'
                className='font-mono text-[10px] tracking-tight'
                title={`task_id: ${row.task_id}`}
              >
                T:{shortId(row.task_id)}
              </Badge>
            )}
            <Badge
              variant='outline'
              className='font-mono text-[10px] tracking-tight'
              title={`session_id: ${row.session_id}`}
            >
              S:{shortId(row.session_id)}
            </Badge>
            <Badge
              variant='outline'
              className='font-mono text-[10px] tracking-tight'
              title={`source_node: ${row.source_node}`}
            >
              N:{shortId(row.source_node) || 'local'}
            </Badge>
          </div>

          <div className='text-muted-foreground mt-1 truncate font-mono text-[11px]'>
            {previewInput(row.tool_input)}
          </div>

          {row.snippet_html && (
            <div
              className='mt-1 font-mono text-[11px] [&_mark]:rounded [&_mark]:bg-amber-500/30 [&_mark]:px-0.5 [&_mark]:text-amber-100'
              // The backend produces this with HTML-escaping on the
              // surrounding text and only the <mark> tags injected, so
              // dangerouslySetInnerHTML is safe for our shape. See
              // handlers_search.go:highlightSnippet.
              dangerouslySetInnerHTML={{ __html: row.snippet_html }}
            />
          )}
        </div>
      </button>

      {open && (
        <div className='space-y-2 border-t border-zinc-800/50 px-3 pb-3 pt-2'>
          <Pane
            label='tool_input'
            content={prettify(row.tool_input)}
            tone='sky'
          />
          <Pane
            label='tool_response'
            content={prettify(row.tool_response)}
            tone={isErr ? 'rose' : 'zinc'}
          />
          <div className='flex flex-wrap gap-x-4 gap-y-0.5 text-[10px] text-muted-foreground'>
            {row.cwd && (
              <span title='cwd'>
                cwd: <code className='font-mono'>{row.cwd}</code>
              </span>
            )}
            <span title={row.created_at}>
              at: <code className='font-mono'>{row.created_at}</code>
            </span>
            <button
              type='button'
              onClick={(e) => {
                e.stopPropagation()
                navigator.clipboard.writeText(JSON.stringify(row, null, 2))
              }}
              className='flex items-center gap-1 hover:text-foreground'
              title='Copy this row as JSON'
            >
              <Copy className='h-3 w-3' />
              copy row
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

function Pane({
  label,
  content,
  tone,
}: {
  label: string
  content: string
  tone: 'sky' | 'zinc' | 'rose'
}) {
  const border =
    tone === 'sky'
      ? 'border-sky-500/40'
      : tone === 'rose'
        ? 'border-rose-500/50'
        : 'border-zinc-700'
  return (
    <div className={cn('overflow-hidden rounded-md border', border)}>
      <div className='bg-zinc-900/50 px-2 py-1 text-[10px] uppercase tracking-wider text-muted-foreground'>
        {label}
      </div>
      <pre className='max-h-[40vh] overflow-auto bg-zinc-950 p-2 text-[11px] leading-relaxed font-mono whitespace-pre-wrap break-words'>
        {content || '—'}
      </pre>
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
    <div className='flex items-baseline gap-1.5'>
      <span className='text-muted-foreground'>{label}</span>
      <span
        className={cn(
          'font-mono font-medium tabular-nums',
          accent && 'text-violet-300',
          tone === 'rose' && 'text-rose-300'
        )}
      >
        {value}
      </span>
    </div>
  )
}

// Heuristics for "this action looks like an error response." We don't
// have an explicit error column on the actions table (yet) — the
// Tamper-Evidence product or a future M-row could add one — so for
// now we sniff the response payload. False positives are tolerated;
// we never HIDE errors, just visually flag them.
function looksLikeError(a: ActionRow): boolean {
  const r = a.tool_response
  if (!r) return false
  if (r.length > 5_000) return false // truncated to 5k; can't tell
  // The most common error shapes in this codebase:
  //  - Bash tool: stderr non-empty + interrupted=true
  //  - MCP tool: text starting with "Error:"
  //  - Generic: '"is_error":true' present in JSON-encoded result
  if (/"is_error"\s*:\s*true/.test(r)) return true
  if (/"interrupted"\s*:\s*true/.test(r)) return true
  if (/^Error: /.test(r)) return true
  if (/Tool .* failed:/i.test(r)) return true
  return false
}

function previewInput(s: string): string {
  if (!s) return '—'
  // Try to extract the most informative field from the JSON input
  try {
    const o = JSON.parse(s) as Record<string, unknown>
    for (const k of [
      'command',
      'file_path',
      'pattern',
      'query',
      'q',
      'prompt',
      'description',
      'content',
      'title',
    ]) {
      const v = o[k]
      if (typeof v === 'string' && v) return truncate(v, 160)
    }
  } catch {
    // not JSON — fall through
  }
  return truncate(s, 160)
}

function prettify(s: string): string {
  if (!s) return ''
  try {
    return JSON.stringify(JSON.parse(s), null, 2)
  } catch {
    return s
  }
}

function truncate(s: string, max: number): string {
  return s.length > max ? s.slice(0, max) + '…' : s
}

function shortId(id: string): string {
  if (!id) return ''
  if (id.length <= 12) return id
  return id.slice(0, 8)
}

function formatTimestamp(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const now = new Date()
  const sameDay =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate()
  if (sameDay) {
    return d.toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  }
  return d.toLocaleDateString(undefined, {
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

// Default export so the route file can import as default — matches the
// pattern used by /features/overview etc.
export default ActionsLogPage
