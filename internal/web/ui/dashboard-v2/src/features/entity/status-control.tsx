import { useState } from 'react'
import { ChevronDown, Check } from 'lucide-react'
import { toast } from 'sonner'
import {
  useChangeEntityStatus,
  type EntityKind,
} from '@/lib/queries/entity'
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

const STATUSES = [
  { id: 'draft', label: 'Draft', help: 'Not yet open for work.' },
  { id: 'open', label: 'Open', help: 'Active — agents can pick up tasks.' },
  {
    id: 'closed',
    label: 'Closed',
    help: 'Completed. No new work expected; history preserved.',
  },
  {
    id: 'archived',
    label: 'Archived',
    help: 'Long-term inactive. Hidden from default views (still queryable).',
  },
] as const

type StatusId = (typeof STATUSES)[number]['id']

const STATUS_COLOR: Record<string, string> = {
  draft: 'bg-amber-500/15 text-amber-500',
  open: 'bg-emerald-500/15 text-emerald-500',
  in_progress: 'bg-sky-500/15 text-sky-500',
  closed: 'bg-zinc-500/15 text-zinc-400',
  archived: 'bg-zinc-500/10 text-zinc-500',
  cancelled: 'bg-rose-500/15 text-rose-500',
}

// Status pill that doubles as a dropdown — same lifecycle for products
// and manifests. Confirmation dialog spells out the consequence; toast
// on success/failure.
export function EntityStatusControl({
  kind,
  entityId,
  status,
  entityTitle,
}: {
  kind: EntityKind
  entityId: string
  status: string
  entityTitle: string
}) {
  const update = useChangeEntityStatus(kind, entityId)
  const [target, setTarget] = useState<StatusId | null>(null)

  const onConfirm = async () => {
    if (!target) return
    try {
      await update.mutateAsync({ status: target })
      toast.success(
        `Status of "${entityTitle}" → ${STATUSES.find((s) => s.id === target)?.label}`
      )
    } catch (e) {
      toast.error(`Status change failed: ${String(e)}`)
    }
    setTarget(null)
  }

  const targetMeta = target ? STATUSES.find((s) => s.id === target) : null
  const noun =
    kind === 'product' ? 'product' : kind === 'task' ? 'task' : 'manifest'

  return (
    <div className='shrink-0'>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type='button'
            className={cn(
              'inline-flex items-center gap-1 rounded-md px-2 py-1 text-[10px] font-medium uppercase transition-colors',
              STATUS_COLOR[status] ?? 'bg-zinc-500/15'
            )}
            aria-label='Change status'
          >
            {status}
            <ChevronDown className='h-3 w-3' />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' className='w-56'>
          {STATUSES.map((s) => (
            <DropdownMenuItem
              key={s.id}
              onClick={() => {
                if (s.id === status) return
                setTarget(s.id)
              }}
              className='flex items-start gap-2'
            >
              <div className='flex w-4 shrink-0 items-center justify-center pt-0.5'>
                {s.id === status ? <Check className='h-3.5 w-3.5' /> : null}
              </div>
              <div className='flex-1'>
                <div className='flex items-center gap-2'>
                  <Badge
                    variant='secondary'
                    className={cn('text-[10px] uppercase', STATUS_COLOR[s.id])}
                  >
                    {s.label}
                  </Badge>
                </div>
                <p className='text-muted-foreground pt-0.5 text-[11px]'>
                  {s.help}
                </p>
              </div>
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>

      <AlertDialog
        open={target !== null}
        onOpenChange={(open) => !open && setTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Change status to {targetMeta?.label}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              "{entityTitle}" will move from{' '}
              <span className='font-medium'>{status}</span> →{' '}
              <span className='font-medium'>{targetMeta?.label}</span>.{' '}
              {targetMeta?.help}
              {target === 'archived' ? (
                <>
                  {' '}
                  Children stay attached but the {noun} won't show in
                  default {noun} lists.
                </>
              ) : null}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={update.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={onConfirm}
              disabled={update.isPending}
            >
              {update.isPending ? 'Saving…' : 'Confirm'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
