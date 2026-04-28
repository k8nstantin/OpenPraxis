import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

// Three-way wiring proof for chunk 3:
//   1. GET  /api/status   — proves same-origin GET works on :9766
//   2. GET  /api/tasks    — proves list endpoints return same shape as :8765
//   3. WSS  /ws           — proves WebSocket upgrade works on the V2 port
//
// Renders inline on the Overview placeholder. Replaces with real
// content once the Overview tab proper is built (chunk 4+).
export function ApiHealthCheck() {
  const status = useQuery({
    queryKey: ['health', 'status'],
    queryFn: async () => {
      const res = await fetch('/api/status')
      if (!res.ok) throw new Error(`status ${res.status}`)
      return res.json() as Promise<Record<string, unknown>>
    },
    staleTime: 30 * 1000,
  })

  const tasks = useQuery({
    queryKey: ['health', 'tasks-sample'],
    queryFn: async () => {
      const res = await fetch('/api/tasks?limit=3')
      if (!res.ok) throw new Error(`tasks ${res.status}`)
      return res.json() as Promise<Array<{ id: string; title?: string; status?: string }>>
    },
    staleTime: 30 * 1000,
  })

  // Open one WebSocket connection on mount. Don't subscribe to any
  // event types — we just want to prove the upgrade succeeds. Close
  // on unmount so dev-mode HMR doesn't leak handles.
  const [wsState, setWsState] = useState<'connecting' | 'open' | 'error' | 'closed'>('connecting')
  useEffect(() => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${proto}//${window.location.host}/ws`)
    ws.addEventListener('open', () => setWsState('open'))
    ws.addEventListener('error', () => setWsState('error'))
    ws.addEventListener('close', () => setWsState((s) => (s === 'open' ? 'closed' : s)))
    return () => ws.close()
  }, [])

  return (
    <Card>
      <CardHeader>
        <CardTitle className='text-base'>Backend wiring (chunk 3 health check)</CardTitle>
      </CardHeader>
      <CardContent className='space-y-2 text-sm'>
        <Row
          label='GET /api/status'
          state={
            status.isLoading
              ? 'pending'
              : status.isError
                ? 'fail'
                : 'ok'
          }
          detail={
            status.isError
              ? String(status.error)
              : status.data
                ? `${Object.keys(status.data).length} keys`
                : '—'
          }
        />
        <Row
          label='GET /api/tasks?limit=3'
          state={
            tasks.isLoading
              ? 'pending'
              : tasks.isError
                ? 'fail'
                : 'ok'
          }
          detail={
            tasks.isError
              ? String(tasks.error)
              : tasks.data
                ? `${tasks.data.length} tasks`
                : '—'
          }
        />
        <Row
          label='WS /ws'
          state={
            wsState === 'connecting'
              ? 'pending'
              : wsState === 'open' || wsState === 'closed'
                ? 'ok'
                : 'fail'
          }
          detail={wsState}
        />
      </CardContent>
    </Card>
  )
}

function Row({
  label,
  state,
  detail,
}: {
  label: string
  state: 'pending' | 'ok' | 'fail'
  detail: string
}) {
  const color =
    state === 'ok'
      ? 'bg-emerald-500/15 text-emerald-500'
      : state === 'fail'
        ? 'bg-rose-500/15 text-rose-500'
        : 'bg-amber-500/15 text-amber-500'
  return (
    <div className='flex items-center justify-between gap-3'>
      <code className='font-mono text-xs'>{label}</code>
      <div className='flex items-center gap-2'>
        <span className='text-muted-foreground text-xs'>{detail}</span>
        <Badge className={color} variant='secondary'>
          {state}
        </Badge>
      </div>
    </div>
  )
}
