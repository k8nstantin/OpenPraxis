import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  CalendarClock,
  CheckCircle,
  Clock,
  Plus,
  Trash2,
  XCircle,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'

// Wire shape from GET /api/schedules and GET /api/schedules/history
interface Schedule {
  id: number
  entity_kind: string
  entity_id: string
  run_at: string
  cron_expr: string
  timezone: string
  max_runs: number
  runs_so_far: number
  stop_at: string
  enabled: boolean
  metadata: string
  valid_from: string
  valid_to: string
  created_by: string
  reason: string
  created_at: string
}

interface ScheduleTemplate {
  key: string
  label: string
  cron: string
  description?: string
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${path} → ${res.status}`)
  return res.json() as Promise<T>
}

function useActiveSchedules() {
  return useQuery({
    queryKey: ['schedules', 'active'],
    queryFn: () => fetchJSON<Schedule[]>('/api/schedules'),
    refetchInterval: 10_000,
  })
}

function useScheduleHistory() {
  return useQuery({
    queryKey: ['schedules', 'history'],
    queryFn: () => fetchJSON<Schedule[]>('/api/schedules/history'),
    refetchInterval: 30_000,
  })
}

function useScheduleTemplates() {
  return useQuery({
    queryKey: ['schedules', 'templates'],
    queryFn: async () => {
      const d = await fetchJSON<{ templates: ScheduleTemplate[] }>('/api/schedules/templates')
      return d.templates
    },
    staleTime: Infinity,
  })
}

function useCloseSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      fetch(`/api/schedules/${id}?reason=closed+via+ui`, { method: 'DELETE' }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['schedules'] })
    },
  })
}

function useCreateSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: object) =>
      fetch('/api/schedules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      }).then(async (r) => {
        if (!r.ok) {
          const t = await r.text()
          throw new Error(t || `HTTP ${r.status}`)
        }
        return r.json()
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['schedules'] })
    },
  })
}

function formatCron(cron: string, runAt: string): string {
  if (cron && cron !== 'once' && cron !== '') return cron
  if (runAt) return `once @ ${new Date(runAt).toLocaleString()}`
  return '—'
}

function scheduleLabel(s: Schedule): string {
  return formatCron(s.cron_expr, s.run_at)
}

// ── Schedule table ────────────────────────────────────────────────────────
function ScheduleRow({ s, onClose }: { s: Schedule; onClose?: () => void }) {
  return (
    <tr className='border-t hover:bg-muted/30 transition-colors text-sm'>
      <td className='px-3 py-2 font-mono text-xs text-muted-foreground'>
        {(s.entity_id ?? '').slice(0, 18)}…
      </td>
      <td className='px-3 py-2 text-muted-foreground text-xs'>{s.entity_kind}</td>
      <td className='px-3 py-2'>{scheduleLabel(s)}</td>
      <td className='px-3 py-2 text-muted-foreground'>{s.timezone || 'UTC'}</td>
      <td className='px-3 py-2'>
        {s.runs_so_far}
        {s.max_runs > 0 ? ` / ${s.max_runs}` : ''}
      </td>
      <td className='px-3 py-2'>
        {s.enabled ? (
          <span className='flex items-center gap-1 text-emerald-600 text-xs'>
            <CheckCircle className='h-3 w-3' /> enabled
          </span>
        ) : (
          <span className='flex items-center gap-1 text-muted-foreground text-xs'>
            <XCircle className='h-3 w-3' /> disabled
          </span>
        )}
      </td>
      {onClose && (
        <td className='px-3 py-2'>
          <button
            onClick={onClose}
            className='p-1 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive'
            title='Close schedule'
          >
            <Trash2 className='h-3.5 w-3.5' />
          </button>
        </td>
      )}
      {!onClose && (
        <td className='px-3 py-2 text-muted-foreground text-xs'>
          {s.reason || '—'}
        </td>
      )}
    </tr>
  )
}

function ScheduleTable({
  rows,
  showClose,
  onClose,
  loading,
}: {
  rows: Schedule[]
  showClose?: boolean
  onClose?: (id: number) => void
  loading?: boolean
}) {
  if (loading) return <Skeleton className='h-32 w-full' />
  if (!rows.length)
    return (
      <div className='py-8 text-center text-sm text-muted-foreground'>
        No schedules found.
      </div>
    )
  return (
    <div className='rounded-md border overflow-hidden'>
      <table className='w-full text-sm'>
        <thead className='bg-muted/50 text-muted-foreground text-xs'>
          <tr>
            <th className='px-3 py-2 text-left font-medium'>Entity</th>
            <th className='px-3 py-2 text-left font-medium'>Kind</th>
            <th className='px-3 py-2 text-left font-medium'>Schedule</th>
            <th className='px-3 py-2 text-left font-medium'>Timezone</th>
            <th className='px-3 py-2 text-left font-medium'>Runs</th>
            <th className='px-3 py-2 text-left font-medium'>Status</th>
            <th className='px-3 py-2 text-left font-medium'>{showClose ? '' : 'Reason'}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((s) => (
            <ScheduleRow
              key={`${s.id}-${s.valid_from}`}
              s={s}
              onClose={showClose && onClose ? () => onClose(s.id) : undefined}
            />
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ── New schedule form ─────────────────────────────────────────────────────
function NewScheduleForm({ onDone }: { onDone: () => void }) {
  const templates = useScheduleTemplates()
  const create = useCreateSchedule()
  const [entityKind, setEntityKind] = useState('task')
  const [entityId, setEntityId] = useState('')
  const [templateKey, setTemplateKey] = useState('once')
  const [runAt, setRunAt] = useState('')
  const [cronExpr, setCronExpr] = useState('')
  const [timezone, setTimezone] = useState('UTC')
  const [maxRuns, setMaxRuns] = useState(0)
  const [error, setError] = useState('')

  const selectedTemplate = templates.data?.find((t) => t.key === templateKey)
  const isOneShot = templateKey === 'once'
  const isCustom = templateKey === 'custom'

  const handleSubmit = async () => {
    setError('')
    if (!entityId.trim()) { setError('Entity ID is required'); return }
    const body: Record<string, unknown> = {
      entity_kind: entityKind,
      entity_id: entityId.trim(),
      timezone,
      max_runs: maxRuns,
    }
    if (isOneShot) {
      if (!runAt) { setError('Run-at datetime is required for one-shot'); return }
      body.run_at = new Date(runAt).toISOString()
    } else if (isCustom) {
      if (!cronExpr.trim()) { setError('Cron expression is required'); return }
      body.cron_expr = cronExpr.trim()
    } else {
      body.cron_expr = selectedTemplate?.cron ?? ''
    }
    try {
      await create.mutateAsync(body)
      onDone()
    } catch (e) {
      setError(String(e))
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className='text-sm'>New Schedule</CardTitle>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='grid grid-cols-2 gap-3'>
          <div className='space-y-1'>
            <Label className='text-xs'>Entity Kind</Label>
            <Select value={entityKind} onValueChange={setEntityKind}>
              <SelectTrigger className='h-8 text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {['product', 'manifest', 'task', 'skill', 'idea'].map((k) => (
                  <SelectItem key={k} value={k} className='text-xs'>{k}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className='space-y-1'>
            <Label className='text-xs'>Entity ID</Label>
            <Input
              className='h-8 text-xs font-mono'
              placeholder='entity_uid (full UUID)'
              value={entityId}
              onChange={(e) => setEntityId(e.target.value)}
            />
          </div>
        </div>

        <div className='grid grid-cols-2 gap-3'>
          <div className='space-y-1'>
            <Label className='text-xs'>Recurrence</Label>
            <Select value={templateKey} onValueChange={setTemplateKey}>
              <SelectTrigger className='h-8 text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {templates.data?.map((t) => (
                  <SelectItem key={t.key} value={t.key} className='text-xs'>{t.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className='space-y-1'>
            <Label className='text-xs'>Timezone</Label>
            <Input
              className='h-8 text-xs'
              placeholder='UTC'
              value={timezone}
              onChange={(e) => setTimezone(e.target.value)}
            />
          </div>
        </div>

        {isOneShot && (
          <div className='space-y-1'>
            <Label className='text-xs'>Run at</Label>
            <Input
              type='datetime-local'
              className='h-8 text-xs'
              value={runAt}
              onChange={(e) => setRunAt(e.target.value)}
            />
          </div>
        )}

        {isCustom && (
          <div className='space-y-1'>
            <Label className='text-xs'>Cron expression</Label>
            <Input
              className='h-8 text-xs font-mono'
              placeholder='0 9 * * 1'
              value={cronExpr}
              onChange={(e) => setCronExpr(e.target.value)}
            />
          </div>
        )}

        <div className='space-y-1'>
          <Label className='text-xs'>Max runs (0 = unlimited)</Label>
          <Input
            type='number'
            min={0}
            className='h-8 text-xs w-32'
            value={maxRuns}
            onChange={(e) => setMaxRuns(parseInt(e.target.value) || 0)}
          />
        </div>

        {error && <p className='text-xs text-destructive'>{error}</p>}

        <div className='flex gap-2'>
          <Button size='sm' onClick={handleSubmit} disabled={create.isPending}>
            {create.isPending ? 'Saving…' : 'Create schedule'}
          </Button>
          <Button size='sm' variant='ghost' onClick={onDone}>Cancel</Button>
        </div>
      </CardContent>
    </Card>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────
function SchedulesPage() {
  const active = useActiveSchedules()
  const history = useScheduleHistory()
  const close = useCloseSchedule()
  const [showForm, setShowForm] = useState(false)

  return (
    <>
      <Header />
      <Main>
        <div className='mb-4 flex items-center justify-between'>
          <div className='flex items-center gap-2'>
            <CalendarClock className='h-5 w-5 text-muted-foreground' />
            <h1 className='text-2xl font-bold tracking-tight'>Schedules</h1>
          </div>
          <Button size='sm' onClick={() => setShowForm((v) => !v)}>
            <Plus className='h-4 w-4 mr-1' />
            New schedule
          </Button>
        </div>

        {showForm && (
          <div className='mb-4'>
            <NewScheduleForm onDone={() => setShowForm(false)} />
          </div>
        )}

        <Tabs defaultValue='active'>
          <TabsList>
            <TabsTrigger value='active' className='gap-1.5'>
              <CheckCircle className='h-3.5 w-3.5' />
              Active
              {active.data && (
                <Badge variant='secondary' className='text-[10px] px-1 py-0'>
                  {active.data.length}
                </Badge>
              )}
            </TabsTrigger>
            <TabsTrigger value='history' className='gap-1.5'>
              <Clock className='h-3.5 w-3.5' />
              History
            </TabsTrigger>
          </TabsList>

          <TabsContent value='active' className='mt-3'>
            <ScheduleTable
              rows={active.data ?? []}
              showClose
              onClose={(id) => close.mutate(id)}
              loading={active.isLoading}
            />
          </TabsContent>

          <TabsContent value='history' className='mt-3'>
            <ScheduleTable
              rows={history.data ?? []}
              loading={history.isLoading}
            />
          </TabsContent>
        </Tabs>
      </Main>
    </>
  )
}

export const Route = createFileRoute('/_authenticated/schedules')({
  component: SchedulesPage,
})
