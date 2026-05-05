import { createFileRoute } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { formatDistanceToNow } from 'date-fns'
import { Brain, MessageSquare } from 'lucide-react'
import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'

export const Route = createFileRoute('/_authenticated/activity')({
  component: ActivityPage,
})

interface ActivityItem {
  id: string
  time: string
  type: 'memory' | 'conversation' | string
  title: string
  detail: string
  session: string
}

function useActivity() {
  return useQuery<ActivityItem[]>({
    queryKey: ['activity'],
    queryFn: () => fetch('/api/activity').then((r) => r.json()),
    refetchInterval: 15_000,
    staleTime: 10_000,
  })
}

function ItemIcon({ type }: { type: string }) {
  if (type === 'memory') return <Brain className='h-4 w-4 text-violet-400 flex-shrink-0 mt-0.5' />
  return <MessageSquare className='h-4 w-4 text-blue-400 flex-shrink-0 mt-0.5' />
}

function ActivityRow({ item }: { item: ActivityItem }) {
  const ago = (() => {
    try {
      return formatDistanceToNow(new Date(item.time), { addSuffix: true })
    } catch {
      return item.time
    }
  })()

  return (
    <div className='flex items-start gap-3 py-3 border-b last:border-0 hover:bg-muted/30 px-4 -mx-4 transition-colors'>
      <ItemIcon type={item.type} />
      <div className='flex-1 min-w-0'>
        <div className='flex items-center gap-2 flex-wrap'>
          <span className='text-sm font-medium truncate'>{item.title}</span>
          {item.detail && (
            <span className='text-xs text-muted-foreground truncate'>{item.detail}</span>
          )}
        </div>
        <div className='flex items-center gap-2 mt-0.5'>
          <span className='text-xs text-muted-foreground'>{ago}</span>
          {item.session && (
            <Badge variant='outline' className='text-xs px-1.5 py-0 h-4'>
              {item.session}
            </Badge>
          )}
        </div>
      </div>
    </div>
  )
}

function ActivityPage() {
  const { data, isLoading, isError } = useActivity()

  return (
    <>
      <Header />
      <Main>
        <div className='mb-4 flex items-center justify-between'>
          <h1 className='text-2xl font-bold tracking-tight'>Activity</h1>
          {data && (
            <span className='text-sm text-muted-foreground'>{data.length} items</span>
          )}
        </div>

        {isLoading && (
          <div className='space-y-3'>
            {Array.from({ length: 10 }).map((_, i) => (
              <Skeleton key={i} className='h-12 w-full' />
            ))}
          </div>
        )}

        {isError && (
          <div className='text-sm text-rose-400'>Failed to load activity.</div>
        )}

        {data && data.length === 0 && (
          <div className='text-muted-foreground text-sm py-12 text-center'>
            No activity yet.
          </div>
        )}

        {data && data.length > 0 && (
          <div className='px-4'>
            {data.map((item) => (
              <ActivityRow key={item.id} item={item} />
            ))}
          </div>
        )}
      </Main>
    </>
  )
}
