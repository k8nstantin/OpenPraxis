import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
  flexRender,
} from '@tanstack/react-table'
import { formatDistanceToNow, format } from 'date-fns'
import { Loader2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { DataTableColumnHeader } from '@/components/data-table/column-header'

export const Route = createFileRoute('/_authenticated/activity')({
  component: ActivityPage,
})

interface ActivityEvent {
  run_uid: string
  entity_uid: string
  entity_title: string
  entity_type: string
  event: 'started' | 'completed' | 'failed'
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
  // computed
  isRunning?: boolean
  realDate?: Date
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
  return base ? `${base}?id=${uid}&tab=runs` : null
}

function StatusCell({ ev }: { ev: ActivityEvent }) {
  if (ev.event === 'started' && ev.isRunning)
    return (
      <Badge variant='outline' className='border-blue-400 text-blue-400 gap-1 whitespace-nowrap'>
        <Loader2 className='h-3 w-3 animate-spin' />running
      </Badge>
    )
  if (ev.event === 'completed')
    return <Badge variant='outline' className='border-emerald-500 text-emerald-500'>done</Badge>
  if (ev.event === 'failed')
    return <Badge variant='destructive'>failed</Badge>
  return null
}

export function ActivityPage() {
  const navigate = useNavigate()
  const [sorting, setSorting] = useState<SortingState>([])

  const { data: raw, isLoading, isError } = useQuery<ActivityEvent[]>({
    queryKey: ['activity-events'],
    queryFn: () => fetch('/api/execution/recent?limit=200').then((r) => r.json()),
    refetchInterval: 5_000,
    staleTime: 3_000,
  })

  // Compute active run_uids, attach to rows, drop stale started rows
  const events = useMemo(() => {
    if (!raw) return []
    const terminal = new Set(
      raw.filter((e) => e.event === 'completed' || e.event === 'failed').map((e) => e.run_uid)
    )
    const activeRunUids = new Set(
      raw.filter((e) => e.event === 'started' && !terminal.has(e.run_uid)).map((e) => e.run_uid)
    )
    return raw
      .filter((e) => e.event !== 'started' || activeRunUids.has(e.run_uid))
      .map((e) => ({ ...e, isRunning: activeRunUids.has(e.run_uid), realDate: realDate(e) }))
  }, [raw])

  const columns: ColumnDef<ActivityEvent>[] = useMemo(() => [
    {
      id: 'time',
      accessorFn: (row) => row.realDate,
      header: ({ column }) => <DataTableColumnHeader column={column} title='Time' />,
      cell: ({ row }) => (
        <span className='font-mono text-xs text-muted-foreground whitespace-nowrap'>
          {format(row.original.realDate!, 'MMM d, HH:mm')}
          <span className='block text-[10px]'>{formatDistanceToNow(row.original.realDate!, { addSuffix: true })}</span>
        </span>
      ),
      sortingFn: 'datetime',
    },
    {
      id: 'status',
      accessorKey: 'event',
      header: 'Status',
      cell: ({ row }) => <StatusCell ev={row.original} />,
      enableSorting: false,
    },
    {
      id: 'entity',
      accessorKey: 'entity_title',
      header: ({ column }) => <DataTableColumnHeader column={column} title='Entity' />,
      cell: ({ row }) => (
        <span className='font-medium truncate block max-w-xs' title={row.original.entity_title}>
          {row.original.entity_title}
          {row.original.entity_type && row.original.entity_type !== 'interactive' && (
            <span className='text-muted-foreground font-normal ml-1.5 text-xs'>
              {row.original.entity_type}
            </span>
          )}
        </span>
      ),
    },
    {
      id: 'turns',
      accessorKey: 'turns',
      header: ({ column }) => <DataTableColumnHeader column={column} title='Turns' className='justify-end' />,
      cell: ({ getValue }) => {
        const v = getValue() as number
        return <span className='font-mono text-xs text-right block'>{v > 0 ? v : '—'}</span>
      },
    },
    {
      id: 'actions',
      accessorKey: 'actions',
      header: ({ column }) => <DataTableColumnHeader column={column} title='Actions' className='justify-end' />,
      cell: ({ getValue }) => {
        const v = getValue() as number
        return <span className='font-mono text-xs text-right block'>{v > 0 ? v : '—'}</span>
      },
    },
    {
      id: 'tokens',
      accessorFn: (row) => row.input_tokens + row.output_tokens,
      header: ({ column }) => <DataTableColumnHeader column={column} title='Tokens' className='justify-end' />,
      cell: ({ getValue }) => (
        <span className='font-mono text-xs text-right block text-muted-foreground'>
          {fmtTok(getValue() as number)}
        </span>
      ),
    },
    {
      id: 'cache',
      accessorKey: 'cache_hit_rate_pct',
      header: ({ column }) => <DataTableColumnHeader column={column} title='Cache' className='justify-end' />,
      cell: ({ getValue }) => {
        const v = getValue() as number
        return (
          <span className={`font-mono text-xs text-right block ${v > 0 ? 'text-emerald-400' : 'text-muted-foreground'}`}>
            {v > 0 ? `${v.toFixed(0)}%` : '—'}
          </span>
        )
      },
    },
    {
      id: 'duration',
      accessorKey: 'duration_ms',
      header: ({ column }) => <DataTableColumnHeader column={column} title='Dur' className='justify-end' />,
      cell: ({ getValue }) => (
        <span className='font-mono text-xs text-right block text-muted-foreground'>
          {fmtDur(getValue() as number)}
        </span>
      ),
    },
    {
      id: 'lines',
      accessorKey: 'lines_added',
      header: ({ column }) => <DataTableColumnHeader column={column} title='Lines' className='justify-end' />,
      cell: ({ row }) => (
        <span className='font-mono text-xs text-right block'>
          {row.original.lines_added > 0 ? (
            <>
              <span className='text-emerald-400'>+{row.original.lines_added}</span>
              {row.original.lines_removed > 0 && (
                <span className='text-rose-400'> −{row.original.lines_removed}</span>
              )}
            </>
          ) : <span className='text-muted-foreground'>—</span>}
        </span>
      ),
    },
  ], [])

  const table = useReactTable({
    data: events,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  })

  return (
    <>
      <Header />
      <Main>
        <div className='mb-4 flex items-center justify-between'>
          <h1 className='text-2xl font-bold tracking-tight'>Activity</h1>
          {events.length > 0 && (
            <span className='text-sm text-muted-foreground'>{events.length} events</span>
          )}
        </div>

        {isLoading && (
          <div className='space-y-1'>
            {Array.from({ length: 10 }).map((_, i) => (
              <Skeleton key={i} className='h-10 w-full' />
            ))}
          </div>
        )}

        {isError && <div className='text-sm text-rose-400'>Failed to load activity.</div>}

        {!isLoading && events.length === 0 && (
          <div className='text-muted-foreground text-sm py-12 text-center'>No activity yet.</div>
        )}

        {events.length > 0 && (
          <div className='rounded-md border'>
            <Table>
              <TableHeader>
                {table.getHeaderGroups().map((hg) => (
                  <TableRow key={hg.id}>
                    {hg.headers.map((header) => (
                      <TableHead key={header.id}>
                        {header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
                      </TableHead>
                    ))}
                  </TableRow>
                ))}
              </TableHeader>
              <TableBody>
                {table.getRowModel().rows.map((row) => {
                  const path = entityPath(row.original.entity_type, row.original.entity_uid)
                  return (
                    <TableRow
                      key={row.id}
                      className={path ? 'cursor-pointer' : ''}
                      onClick={path ? () => navigate({ to: path } as Parameters<typeof navigate>[0]) : undefined}
                    >
                      {row.getVisibleCells().map((cell) => (
                        <TableCell key={cell.id}>
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </TableCell>
                      ))}
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        )}
      </Main>
    </>
  )
}
