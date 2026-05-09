import { useEffect, useRef, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { ContentSection } from '../components/content-section'

interface EntityResult {
  entity_uid: string
  title: string
  type: string
  status: string
}

interface ResolvedEntry {
  key: string
  value: string
  source: 'task' | 'manifest' | 'product' | 'system'
  source_id: string
}

interface ResolvedResponse {
  task_id: string
  resolved: Record<string, ResolvedEntry>
}

const GROUPS: { label: string; keys: string[] }[] = [
  { label: 'Execution Controls', keys: ['max_parallel', 'max_turns', 'timeout_minutes', 'temperature', 'reasoning_effort', 'default_agent', 'default_model', 'retry_on_failure', 'approval_mode', 'allowed_tools'] },
  { label: 'Prompt Context', keys: ['prompt_max_comment_chars', 'prompt_max_context_pct', 'compliance_checks_enabled'] },
  { label: 'Scheduler / Runner', keys: ['scheduler_tick_seconds', 'host_sampler_tick_seconds', 'on_restart_behavior'] },
  { label: 'Branch / Worktree', keys: ['branch_prefix', 'branch_strategy', 'branch_remote', 'worktree_base_dir'] },
  { label: 'Frontend Flags', keys: ['frontend_dashboard_v2', 'frontend_dev_mode', 'frontend_dashboard_v2_products', 'frontend_dashboard_v2_manifests', 'frontend_dashboard_v2_tasks', 'frontend_dashboard_v2_memories', 'frontend_dashboard_v2_conversations', 'frontend_dashboard_v2_settings', 'frontend_dashboard_v2_compliance', 'frontend_dashboard_v2_overview'] },
  { label: 'Comment Attachments', keys: ['comment_attachment_max_mb', 'comment_attachment_allowed_mimes'] },
]

function sourceBadge(source: ResolvedEntry['source']) {
  switch (source) {
    case 'task': return 'bg-rose-500/10 text-rose-400 border-rose-500/20'
    case 'manifest': return 'bg-amber-500/10 text-amber-400 border-amber-500/20'
    case 'product': return 'bg-blue-500/10 text-blue-400 border-blue-500/20'
    case 'system': return 'bg-zinc-500/10 text-zinc-400 border-zinc-500/20'
    default: return 'bg-zinc-500/10 text-zinc-400 border-zinc-500/20'
  }
}

function valueDisplay(value: string): string {
  try {
    const parsed = JSON.parse(value)
    if (Array.isArray(parsed)) {
      if (parsed.length === 0) return '[]'
      if (parsed.length <= 4) return `[${parsed.join(', ')}]`
      return `[${parsed.slice(0, 4).join(', ')}, …+${parsed.length - 4}]`
    }
    return String(parsed)
  } catch {
    return value
  }
}

// ------ TaskCombobox ---------------------------------------------------------

function TaskCombobox({
  value,
  onChange,
}: {
  value: EntityResult | null
  onChange: (e: EntityResult | null) => void
}) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<EntityResult[]>([])
  const [open, setOpen] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const wrapRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (timer.current) clearTimeout(timer.current)
    if (!query) { setResults([]); return }
    timer.current = setTimeout(() => {
      fetch(`/api/entities/search?q=${encodeURIComponent(query)}&type=task`)
        .then(r => r.json())
        .then((d: EntityResult[]) => setResults(Array.isArray(d) ? d.slice(0, 10) : []))
        .catch(() => setResults([]))
    }, 200)
  }, [query])

  useEffect(() => { setQuery(value ? value.title : '') }, [value])

  useEffect(() => {
    const h = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [])

  return (
    <div className='relative w-full max-w-lg' ref={wrapRef}>
      <div className='flex gap-2'>
        <Input
          className='h-8 text-sm'
          placeholder='Search tasks by name or UUID…'
          value={query}
          onChange={e => { setQuery(e.target.value); setOpen(true) }}
          onFocus={() => query && setOpen(true)}
        />
        {value ? (
          <Button type='button' variant='ghost' size='sm' className='h-8 px-2 text-xs'
            onClick={() => { onChange(null); setQuery(''); setResults([]) }}>
            Clear
          </Button>
        ) : null}
      </div>
      {open && results.length > 0 ? (
        <div className='absolute z-50 mt-1 w-full rounded-md border bg-popover shadow-lg'>
          {results.map(r => (
            <button key={r.entity_uid} type='button'
              className='flex w-full items-center gap-2 px-3 py-2 text-sm hover:bg-accent text-left'
              onClick={() => { onChange(r); setQuery(r.title); setOpen(false); setResults([]) }}>
              <span className='flex-1 truncate font-medium'>{r.title}</span>
              <span className='text-xs text-muted-foreground font-mono shrink-0'>{r.entity_uid.slice(0, 8)}</span>
              <Badge variant='outline' className='text-[10px] px-1 py-0 shrink-0'>{r.status}</Badge>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}

// ------ ResolvedTable --------------------------------------------------------

function ResolvedTable({ data }: { data: ResolvedResponse }) {
  const byKey = data.resolved
  const allKeys = Object.keys(byKey)
  const [showSystemOnly, setShowSystemOnly] = useState(false)

  const systemCount = allKeys.filter(k => byKey[k]?.source === 'system').length
  const overrideCount = allKeys.length - systemCount

  const groups: { label: string; entries: ResolvedEntry[] }[] = []
  const used = new Set<string>()

  for (const g of GROUPS) {
    const entries = g.keys
      .map(k => byKey[k])
      .filter((e): e is ResolvedEntry => e !== undefined)
    entries.forEach(e => used.add(e.key))
    if (entries.length > 0) groups.push({ label: g.label, entries })
  }

  const remaining = allKeys.filter(k => !used.has(k)).map(k => byKey[k]).filter((e): e is ResolvedEntry => e !== undefined)
  if (remaining.length > 0) groups.push({ label: 'Other', entries: remaining })

  const filteredGroups = showSystemOnly
    ? groups.map(g => ({ ...g, entries: g.entries.filter(e => e.source !== 'system') })).filter(g => g.entries.length > 0)
    : groups

  return (
    <div className='mt-4 space-y-4'>
      <div className='flex items-center gap-3 text-xs text-muted-foreground'>
        <span>{overrideCount} explicit override{overrideCount !== 1 ? 's' : ''}</span>
        <span>·</span>
        <span>{systemCount} at system default</span>
        <button
          type='button'
          className='ml-auto text-xs underline underline-offset-2 hover:text-foreground'
          onClick={() => setShowSystemOnly(v => !v)}
        >
          {showSystemOnly ? 'show all' : 'show overrides only'}
        </button>
      </div>

      {filteredGroups.map(g => (
        <Card key={g.label}>
          <CardHeader className='px-3 py-2 pb-0'>
            <CardTitle className='text-sm font-semibold text-muted-foreground uppercase tracking-wider'>
              {g.label}
            </CardTitle>
          </CardHeader>
          <CardContent className='p-0'>
            {g.entries.map(entry => (
              <div key={entry.key} className='flex items-start gap-3 px-3 py-2 border-b last:border-b-0'>
                <code className='font-mono text-sm font-medium w-60 shrink-0 mt-0.5'>{entry.key}</code>
                <div className='flex-1 min-w-0'>
                  <div className='font-mono text-sm truncate' title={entry.value}>
                    {valueDisplay(entry.value)}
                  </div>
                </div>
                <div className='flex items-center gap-2 shrink-0'>
                  <Badge variant='outline' className={`text-[10px] px-1.5 py-0 ${sourceBadge(entry.source)}`}>
                    {entry.source}
                  </Badge>
                  {entry.source !== 'system' ? (
                    <span className='text-[10px] text-muted-foreground font-mono'>
                      {entry.source_id.slice(0, 8)}
                    </span>
                  ) : null}
                </div>
              </div>
            ))}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

// ------ Main export ----------------------------------------------------------

export function SettingsResolved() {
  const [task, setTask] = useState<EntityResult | null>(null)
  const [data, setData] = useState<ResolvedResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!task) { setData(null); setError(null); return }
    setLoading(true)
    setError(null)
    fetch(`/api/tasks/${task.entity_uid}/settings/resolved`)
      .then(r => { if (!r.ok) throw new Error(`resolved ${r.status}`); return r.json() })
      .then(d => setData(d as ResolvedResponse))
      .catch(e => setError(String(e)))
      .finally(() => setLoading(false))
  }, [task])

  return (
    <ContentSection
      title='Resolution Chain'
      desc='For a task, see the effective value of every knob and which scope provided it (task → manifest → product → system).'
    >
      <div className='space-y-3'>
        <TaskCombobox value={task} onChange={setTask} />
        {!task ? (
          <div className='text-xs text-muted-foreground'>
            Select a task above to see its full resolved settings chain.
          </div>
        ) : null}
        {loading ? (
          <div className='space-y-2 mt-4'>
            {Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className='h-10 w-full' />)}
          </div>
        ) : null}
        {error ? <div className='text-sm text-rose-400 mt-4'>Error: {error}</div> : null}
        {data && !loading ? <ResolvedTable data={data} /> : null}
      </div>
    </ContentSection>
  )
}
