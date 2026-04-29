import { useMemo, useState } from 'react'
import { CalendarIcon, History as HistoryIcon, Plus, X } from 'lucide-react'
import { toast } from 'sonner'
import {
  useCloseSchedule,
  useCreateSchedule,
  useScheduleHistory,
  useSchedules,
  type Schedule,
} from '@/lib/queries/schedules'
import type { EntityKind } from '@/lib/queries/entity'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  RadioGroup,
  RadioGroupItem,
} from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'

// Schedule tab — replaces the SchedulePlaceholder in detail-pane.tsx.
// Lists active schedules, lists history, and exposes a "+ New schedule"
// modal/inline form that reads recurrence presets from
// /api/schedules/templates so the catalog can grow without a frontend
// edit. Closing an active schedule prompts a confirmation dialog
// (same AlertDialog pattern as status-control + dependencies).

interface ScheduleTabProps {
  kind: EntityKind
  entityId: string
}

export function ScheduleTab({ kind, entityId }: ScheduleTabProps) {
  const active = useSchedules(kind, entityId)
  const history = useScheduleHistory(kind, entityId)
  const closing = useCloseSchedule(kind, entityId)
  const [composing, setComposing] = useState(false)
  const [confirmCloseId, setConfirmCloseId] = useState<number | null>(null)

  // History excludes still-active rows so the two panels don't double-render.
  const closedHistory = useMemo<Schedule[]>(() => {
    if (!history.data) return []
    return history.data.filter((s) => s.valid_to !== '')
  }, [history.data])

  const onConfirmClose = async () => {
    if (confirmCloseId == null) return
    try {
      await closing.mutateAsync({
        id: confirmCloseId,
        reason: 'closed via portal',
      })
      toast.success('Schedule closed.')
    } catch (e) {
      toast.error(`Close failed: ${String(e)}`)
    }
    setConfirmCloseId(null)
  }

  return (
    <div className='space-y-4'>
      <div className='flex items-center justify-between gap-3'>
        <div className='text-muted-foreground text-sm'>
          {active.isLoading
            ? 'Loading active schedules…'
            : `${active.data?.length ?? 0} active`}
        </div>
        {!composing ? (
          <Button
            type='button'
            data-testid='new-schedule-button'
            size='sm'
            variant='outline'
            onClick={() => setComposing(true)}
            className='h-8 text-xs'
          >
            <Plus className='mr-1 h-3 w-3' />
            New schedule
          </Button>
        ) : null}
      </div>

      {composing ? (
        <NewScheduleForm
          kind={kind}
          entityId={entityId}
          onCancel={() => setComposing(false)}
          onSaved={() => setComposing(false)}
        />
      ) : null}

      <section className='space-y-2'>
        <h3 className='text-sm font-semibold tracking-tight'>Active</h3>
        {active.isLoading ? (
          <div className='space-y-2'>
            <Skeleton className='h-16 w-full' />
            <Skeleton className='h-16 w-full' />
          </div>
        ) : active.isError ? (
          <Card>
            <CardContent className='p-3 text-sm text-rose-400'>
              Failed: {String(active.error)}
            </CardContent>
          </Card>
        ) : (active.data?.length ?? 0) === 0 ? (
          <Card>
            <CardContent className='text-muted-foreground p-3 text-sm'>
              No active schedules. Click <strong>+ New schedule</strong> to add one.
            </CardContent>
          </Card>
        ) : (
          <div className='space-y-2'>
            {active.data?.map((s) => (
              <ActiveScheduleRow
                key={s.id}
                schedule={s}
                onCloseClick={() => setConfirmCloseId(s.id)}
              />
            ))}
          </div>
        )}
      </section>

      <section className='space-y-2'>
        <h3 className='flex items-center gap-2 text-sm font-semibold tracking-tight'>
          <HistoryIcon className='h-3.5 w-3.5' />
          History
        </h3>
        {history.isLoading ? (
          <Skeleton className='h-12 w-full' />
        ) : closedHistory.length === 0 ? (
          <Card>
            <CardContent className='text-muted-foreground p-3 text-xs'>
              No closed schedules.
            </CardContent>
          </Card>
        ) : (
          <div className='space-y-1'>
            {closedHistory.map((s) => (
              <HistoryRow key={s.id} schedule={s} />
            ))}
          </div>
        )}
      </section>

      <AlertDialog
        open={confirmCloseId != null}
        onOpenChange={(open) => {
          if (!open) setConfirmCloseId(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Close this schedule?</AlertDialogTitle>
            <AlertDialogDescription>
              The row stays in history with{' '}
              <code className='font-mono text-xs'>valid_to</code> set to now —
              never deleted. Future fires from this rule will not run.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={onConfirmClose}>
              Close schedule
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

// ── Active row card ─────────────────────────────────────────────────

function ActiveScheduleRow({
  schedule,
  onCloseClick,
}: {
  schedule: Schedule
  onCloseClick: () => void
}) {
  const label = describeRecurrence(schedule.cron_expr)
  const runs =
    schedule.max_runs > 0
      ? `${schedule.runs_so_far}/${schedule.max_runs}`
      : `${schedule.runs_so_far}`
  return (
    <Card>
      <CardContent className='space-y-1 p-3'>
        <div className='flex items-center justify-between gap-2'>
          <div className='flex items-center gap-2 text-sm'>
            <Badge variant='outline' className='text-[10px]'>
              {label}
            </Badge>
            <span className='text-muted-foreground'>·</span>
            <code className='font-mono text-xs'>
              {schedule.timezone || 'UTC'}
            </code>
            <span className='text-muted-foreground'>·</span>
            <span className='text-muted-foreground text-xs'>
              runs {runs}
            </span>
            {!schedule.enabled ? (
              <Badge variant='secondary' className='ml-1 text-[10px]'>
                Disabled
              </Badge>
            ) : null}
          </div>
          <Button
            type='button'
            size='sm'
            variant='ghost'
            onClick={onCloseClick}
            className='h-7 text-xs'
          >
            <X className='mr-1 h-3 w-3' />
            Close
          </Button>
        </div>
        <div className='text-muted-foreground text-xs'>
          next run:{' '}
          <span className='text-foreground font-mono'>
            {fmtDateTime(schedule.run_at)}
          </span>
          {schedule.stop_at ? (
            <>
              <span className='mx-1'>·</span>
              <span>
                ends {fmtDate(schedule.stop_at)}
              </span>
            </>
          ) : null}
          {schedule.cron_expr ? (
            <>
              <span className='mx-1'>·</span>
              <code className='font-mono'>{schedule.cron_expr}</code>
            </>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}

function HistoryRow({ schedule }: { schedule: Schedule }) {
  const label = describeRecurrence(schedule.cron_expr)
  const validFrom = fmtDate(schedule.valid_from)
  const validTo = schedule.valid_to ? fmtDate(schedule.valid_to) : 'now'
  return (
    <div className='text-muted-foreground rounded-md border bg-card/40 px-3 py-2 text-xs'>
      <span className='text-foreground'>{label}</span>
      <span className='mx-1'>·</span>
      valid {validFrom} → {validTo}
      {schedule.reason ? (
        <>
          <span className='mx-1'>·</span>
          <span className='italic'>"{schedule.reason}"</span>
        </>
      ) : null}
    </div>
  )
}

// ── New-schedule form ───────────────────────────────────────────────

type EndCondition = 'never' | 'after_n' | 'on_date'

function NewScheduleForm({
  kind,
  entityId,
  onCancel,
  onSaved,
}: {
  kind: EntityKind
  entityId: string
  onCancel: () => void
  onSaved: () => void
}) {
  const create = useCreateSchedule(kind, entityId)

  const [mode, setMode] = useState<'recurring' | 'one_shot'>('recurring')
  const [intervalValue, setIntervalValue] = useState<string>('1')
  const [intervalUnit, setIntervalUnit] = useState<
    'second' | 'minute' | 'hour' | 'day' | 'week'
  >('hour')
  const [date, setDate] = useState<Date | undefined>(() => {
    const tomorrow = new Date()
    tomorrow.setDate(tomorrow.getDate() + 1)
    tomorrow.setHours(9, 0, 0, 0)
    return tomorrow
  })
  const [time, setTime] = useState<string>('09:00')
  const [tz, setTz] = useState<string>('UTC')
  const [endCondition, setEndCondition] = useState<EndCondition>('never')
  const [maxRuns, setMaxRuns] = useState<string>('1')
  const [stopDate, setStopDate] = useState<Date | undefined>(undefined)
  const [calOpen, setCalOpen] = useState(false)
  const [stopCalOpen, setStopCalOpen] = useState(false)

  const onSave = async () => {
    if (!date) {
      toast.error('Please pick a date.')
      return
    }
    const runAt = combineDateTime(date, time)
    if (!runAt) {
      toast.error('Invalid time — use HH:MM (24h).')
      return
    }
    // One-shot fires once at runAt — cron_expr is empty so the runner
    // (post-cutover) treats it as a single-fire schedule.
    let reccLabel = ''
    if (mode === 'recurring') {
      const ivl = Number.parseInt(intervalValue, 10)
      if (!Number.isFinite(ivl) || ivl <= 0) {
        toast.error('Interval must be a positive integer.')
        return
      }
      reccLabel = `every ${ivl} ${intervalUnit}${ivl === 1 ? '' : 's'}`
    }

    let max = 0
    let stopAt = ''
    if (mode === 'one_shot') {
      // Single fire — no end condition. max_runs=1, stop_at empty.
      max = 1
    } else if (endCondition === 'after_n') {
      const parsed = Number.parseInt(maxRuns, 10)
      if (!Number.isFinite(parsed) || parsed <= 0) {
        toast.error('Max runs must be a positive integer.')
        return
      }
      max = parsed
    } else if (endCondition === 'on_date') {
      if (!stopDate) {
        toast.error('Pick an end date.')
        return
      }
      stopAt = stopDate.toISOString()
    }

    try {
      await create.mutateAsync({
        entity_kind: kind,
        entity_id: entityId,
        run_at: runAt,
        cron_expr: reccLabel,
        timezone: tz || 'UTC',
        max_runs: max,
        stop_at: stopAt,
        enabled: true,
        reason: 'created via portal',
      })
      toast.success('Schedule saved.')
      onSaved()
    } catch (e) {
      toast.error(`Save failed: ${String(e)}`)
    }
  }

  return (
    <Card data-testid='new-schedule-form'>
      <CardContent className='space-y-4 p-4'>
        <div className='space-y-2'>
          <Label>Schedule type</Label>
          <RadioGroup
            value={mode}
            onValueChange={(v) => setMode(v as 'recurring' | 'one_shot')}
            className='flex gap-6'
          >
            <div className='flex items-center gap-2'>
              <RadioGroupItem value='recurring' id='mode-recurring' />
              <Label htmlFor='mode-recurring' className='font-normal'>
                Recurring
              </Label>
            </div>
            <div className='flex items-center gap-2'>
              <RadioGroupItem value='one_shot' id='mode-oneshot' />
              <Label htmlFor='mode-oneshot' className='font-normal'>
                Run once at a specific time
              </Label>
            </div>
          </RadioGroup>
        </div>

        {mode === 'recurring' ? (
          <div className='space-y-2'>
            <Label>Run every</Label>
            <div className='flex items-center gap-2'>
              <Input
                type='number'
                min={1}
                value={intervalValue}
                onChange={(e) => setIntervalValue(e.target.value)}
                className='w-24'
              />
              <Select
                value={intervalUnit}
                onValueChange={(v) =>
                  setIntervalUnit(
                    v as 'second' | 'minute' | 'hour' | 'day' | 'week'
                  )
                }
              >
                <SelectTrigger className='w-40'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='second'>Seconds</SelectItem>
                  <SelectItem value='minute'>Minutes</SelectItem>
                  <SelectItem value='hour'>Hours</SelectItem>
                  <SelectItem value='day'>Days</SelectItem>
                  <SelectItem value='week'>Weeks</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        ) : null}

        <div className='grid grid-cols-2 gap-3'>
          <div className='space-y-2'>
            <Label>Start date</Label>
            <Popover open={calOpen} onOpenChange={setCalOpen}>
              <PopoverTrigger asChild>
                <Button
                  type='button'
                  variant='outline'
                  className='w-full justify-start text-left font-normal'
                >
                  <CalendarIcon className='mr-2 h-4 w-4' />
                  {date ? fmtDate(date.toISOString()) : 'Pick a date'}
                </Button>
              </PopoverTrigger>
              <PopoverContent className='w-auto p-0' align='start'>
                <Calendar
                  mode='single'
                  selected={date}
                  onSelect={(d) => {
                    setDate(d ?? undefined)
                    setCalOpen(false)
                  }}
                />
              </PopoverContent>
            </Popover>
          </div>
          <div className='space-y-2'>
            <Label htmlFor='sched-time'>Time (HH:MM, 24h)</Label>
            <Input
              id='sched-time'
              value={time}
              onChange={(e) => setTime(e.target.value)}
              placeholder='09:00'
            />
          </div>
        </div>

        <div className='space-y-2'>
          <Label htmlFor='sched-tz'>Timezone</Label>
          <Input
            id='sched-tz'
            value={tz}
            onChange={(e) => setTz(e.target.value)}
            placeholder='UTC'
          />
          <p className='text-muted-foreground text-xs'>
            IANA name (e.g. <code className='font-mono'>America/New_York</code>).
            Default UTC.
          </p>
        </div>

        {mode === 'recurring' ? (
        <div className='space-y-2'>
          <Label>End condition</Label>
          <RadioGroup
            value={endCondition}
            onValueChange={(v) => setEndCondition(v as EndCondition)}
            className='space-y-2'
          >
            <div className='flex items-center gap-2'>
              <RadioGroupItem value='never' id='end-never' />
              <Label htmlFor='end-never' className='font-normal'>
                Never
              </Label>
            </div>
            <div className='flex items-center gap-2'>
              <RadioGroupItem value='after_n' id='end-after' />
              <Label htmlFor='end-after' className='font-normal'>
                After
              </Label>
              <Input
                type='number'
                value={maxRuns}
                onChange={(e) => setMaxRuns(e.target.value)}
                disabled={endCondition !== 'after_n'}
                className='h-8 w-20'
                min={1}
              />
              <span className='text-muted-foreground text-xs'>runs</span>
            </div>
            <div className='flex items-center gap-2'>
              <RadioGroupItem value='on_date' id='end-date' />
              <Label htmlFor='end-date' className='font-normal'>
                On date
              </Label>
              <Popover open={stopCalOpen} onOpenChange={setStopCalOpen}>
                <PopoverTrigger asChild>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={endCondition !== 'on_date'}
                    className='h-8 justify-start font-normal'
                  >
                    <CalendarIcon className='mr-1 h-3 w-3' />
                    {stopDate ? fmtDate(stopDate.toISOString()) : 'Pick'}
                  </Button>
                </PopoverTrigger>
                <PopoverContent className='w-auto p-0' align='start'>
                  <Calendar
                    mode='single'
                    selected={stopDate}
                    onSelect={(d) => {
                      setStopDate(d ?? undefined)
                      setStopCalOpen(false)
                    }}
                  />
                </PopoverContent>
              </Popover>
            </div>
          </RadioGroup>
        </div>
        ) : null}

        <div className='flex items-center justify-end gap-2 pt-2'>
          {create.isError ? (
            <span className='mr-auto text-xs text-rose-400'>
              {String(create.error)}
            </span>
          ) : null}
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={onCancel}
            disabled={create.isPending}
          >
            Cancel
          </Button>
          <Button
            type='button'
            size='sm'
            onClick={onSave}
            disabled={create.isPending}
          >
            {create.isPending ? 'Saving…' : 'Save'}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

// ── Helpers ─────────────────────────────────────────────────────────

// describeRecurrence renders a friendly label for the recurrence
// stored on a schedule row. New rows write the friendly label
// directly (e.g. "every 4 hours"). Legacy/backfilled rows may carry
// a cron expression — translate the common ones to a human label.
function describeRecurrence(label: string): string {
  if (label === '') return '—'
  const cronMap: Record<string, string> = {
    '*/15 * * * *': 'every 15 minutes',
    '0 * * * *': 'every hour',
    '0 */4 * * *': 'every 4 hours',
    '0 9 * * *': 'daily',
    '0 9 * * 1': 'weekly',
    '0 9 1 * *': 'monthly',
    once: 'one-shot (legacy)',
  }
  return cronMap[label] ?? label
}

// combineDateTime merges a date (from <Calendar>) with HH:MM 24h text
// into an RFC3339 string. Returns null on parse error so the caller can
// show a toast instead of POSTing garbage.
function combineDateTime(date: Date, hhmm: string): string | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec(hhmm.trim())
  if (!m) return null
  const hh = Number.parseInt(m[1], 10)
  const mm = Number.parseInt(m[2], 10)
  if (hh < 0 || hh > 23 || mm < 0 || mm > 59) return null
  const merged = new Date(date)
  merged.setHours(hh, mm, 0, 0)
  return merged.toISOString()
}

function fmtDateTime(ts: string): string {
  if (!ts) return '—'
  const d = new Date(ts)
  if (!Number.isFinite(d.getTime())) return ts
  return d.toLocaleString()
}

function fmtDate(ts: string): string {
  if (!ts) return '—'
  const d = new Date(ts)
  if (!Number.isFinite(d.getTime())) return ts
  return d.toLocaleDateString()
}
