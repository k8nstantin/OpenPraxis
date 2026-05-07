import { useMemo, useEffect, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { EChart } from '@/components/echart'
import {
  useEntityGraph,
  useLiveRuns,
  type EntityKind,
  type GraphNode,
  type GraphEdge,
} from '@/lib/queries/entity'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

// DAG tab — Apache ECharts `graph` series with force-directed layout.
// Read-only canonical view of the relationships SCD-2 graph rooted at
// this entity. Force layout handles arbitrary topologies (mixed
// ownership + dependency edges, cycles, varying fan-out) without
// blowing up the canvas like a strict hierarchical layout would.
//
//   - Categories per kind (product / manifest / task) — gives every
//     node an auto-color from the category palette + a legend toggle
//     to hide/show by kind.
//   - Symbols per kind: rect (product), diamond (manifest), circle
//     (task). Distinguishable at every zoom level.
//   - Edge styling per kind: solid blue for owns, dashed amber for
//     depends_on. Legend distinguishes the two on hover.
//   - emphasis.focus = 'adjacency' — hover a node, everything else
//     fades; only that node's neighbors stay full-color.
//   - roam: true → drag to pan, wheel to zoom, fits the whole graph.
//   - Click → navigate into that entity's own DAG.

interface DAGTabProps {
  kind: EntityKind
  entityId: string
}

const STATUS_BORDER: Record<string, string> = {
  draft: '#f59e0b',
  active: '#10b981',
  closed: '#71717a',
  archived: '#52525b',
}

const KIND_COLOR: Record<string, string> = {
  skill:    '#f59e0b', // amber-400 — root of the DAG, stands out
  product:  '#3b82f6', // blue-500
  manifest: '#a78bfa', // violet-400
  task:     '#10b981', // emerald-500
}

const KIND_SYMBOL: Record<string, string> = {
  skill:    'star',       // star = top-level governance node
  product:  'roundRect',
  manifest: 'diamond',
  task:     'circle',
}

const KIND_SIZE: Record<string, number> = {
  skill:    42,  // largest — root of the DAG
  product:  32,
  manifest: 26,
  task:     18,
}

interface ChartNode {
  id: string
  name: string
  symbol?: string
  symbolSize?: number
  category?: number
  itemStyle?: { color?: string; borderColor?: string; borderWidth?: number }
  label?: { fontWeight?: number | string }
  // raw — picked off in click handler
  _kind: string
  _status: string
}

interface ChartLink {
  source: string
  target: string
  lineStyle?: { color?: string; type?: string; width?: number; opacity?: number }
  symbol?: [string, string]
  symbolSize?: [number, number]
  _kind: string
}

function build(
  rootId: string,
  nodes: GraphNode[],
  edges: GraphEdge[],
  runningIds: Set<string>,
  completedIds: Set<string>
): { data: ChartNode[]; links: ChartLink[]; categories: { name: string }[] } {
  const categories = [{ name: 'skill' }, { name: 'product' }, { name: 'manifest' }, { name: 'task' }]
  const catIndex: Record<string, number> = { skill: 0, product: 1, manifest: 2, task: 3 }
  // Per-node radial-gradient fill. ECharts accepts {type:'radial',
  // colorStops:[]} on itemStyle.color directly. Center bright →
  // periphery deeper; gives each node a soft "glow from within" look.
  const grad = (hex: string) => ({
    type: 'radial' as const,
    x: 0.5,
    y: 0.5,
    r: 0.7,
    colorStops: [
      { offset: 0, color: hex + 'ff' },
      { offset: 0.85, color: hex + 'aa' },
      { offset: 1, color: hex + '60' },
    ],
  })
  const data: ChartNode[] = nodes.map((n) => {
    const isRunning = runningIds.has(n.id)
    const isDone   = completedIds.has(n.id)
    const baseSize = n.id === rootId ? KIND_SIZE[n.kind] + 12 : KIND_SIZE[n.kind]
    return {
      id: n.id,
      name: n.title,
      symbol: KIND_SYMBOL[n.kind] ?? 'circle',
      symbolSize: isRunning ? baseSize + 10 : baseSize,
      category: catIndex[n.kind] ?? 0,
      itemStyle: {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        color: (isRunning
          ? grad('#8b5cf6')
          : isDone
            ? grad('#10b981')
            : grad(KIND_COLOR[n.kind] ?? '#3b82f6')) as any,
        borderColor: isRunning ? '#ffffff' : isDone ? '#34d399' : (STATUS_BORDER[n.status] ?? '#71717a'),
        borderWidth: isRunning ? 3 : (isDone ? 2.5 : (n.id === rootId ? 3 : 1.5)),
        shadowBlur: isRunning ? 48 : isDone ? 20 : (n.id === rootId ? 24 : 8),
        shadowColor: isRunning ? '#8b5cf6' : isDone ? '#10b981' : (KIND_COLOR[n.kind] ?? '#3b82f6'),
        shadowOffsetX: 0,
        shadowOffsetY: 0,
        opacity: (!isRunning && !isDone && completedIds.size > 0) ? 0.55 : 1,
      },
      label: {
        fontWeight: (isRunning || isDone || n.id === rootId) ? 'bold' : 'normal',
        color: isRunning ? '#c4b5fd' : isDone ? '#6ee7b7' : '#e5e7eb',
      },
      _kind: n.kind,
      _status: n.status,
    }
  })

  const links: ChartLink[] = edges.map((e) => {
    const isOwns = e.kind === 'owns'
    return {
      source: e.source,
      target: e.target,
      lineStyle: {
        color: isOwns ? '#3b82f6' : '#f59e0b',
        type: isOwns ? 'solid' : 'dashed',
        width: 1.4,
        opacity: 0.8,
      },
      symbol: ['none', 'arrow'],
      symbolSize: [4, 8],
      _kind: e.kind,
    }
  })
  const traversedLinks: ChartLink[] = links.map((l) => {
    const srcDone = completedIds.has(l.source as string)
    const srcRun  = runningIds.has(l.source as string)
    if (srcDone || srcRun) {
      return {
        ...l,
        lineStyle: {
          ...l.lineStyle,
          color: srcDone ? '#34d399' : '#a78bfa',
          width: 2.2,
          opacity: 1,
        },
      }
    }
    return { ...l, lineStyle: { ...l.lineStyle, opacity: completedIds.size > 0 ? 0.3 : 0.8 } }
  })
  return { data, links: traversedLinks, categories }
}

export function DAGTab({ kind, entityId }: DAGTabProps) {
  const graph = useEntityGraph(kind, entityId, 10)
  const navigate = useNavigate()
  const { data: liveRuns } = useLiveRuns()

  // First render: notMerge=true so the force layout unfolds with animation.
  // After 2.5s (layout settled), switch to notMerge=false so live progress
  // updates (running/completed node styles) don't restart the simulation.
  const [settled, setSettled] = useState(false)
  useEffect(() => {
    if (!graph.data) return
    const t = setTimeout(() => setSettled(true), 2500)
    return () => clearTimeout(t)
  }, [graph.data])

  const runningIds = useMemo(
    () => new Set((liveRuns ?? []).map((r) => r.entity_uid)),
    [liveRuns]
  )

  // Derive completed node IDs: any node in the graph that has a completed
  // run (fetch /runs and check for a completed event). We batch by querying
  // each task node's runs — only task nodes actually execute.
  const taskNodeIds = useMemo(
    () => (graph.data?.nodes ?? []).filter((n) => n.kind === 'task').map((n) => n.id),
    [graph.data]
  )
  const [completedIds, setCompletedIds] = useState<Set<string>>(new Set())
  useEffect(() => {
    if (taskNodeIds.length === 0) return
    let cancelled = false
    Promise.all(
      taskNodeIds.map((id) =>
        fetch(`/api/entities/${id}/runs`)
          .then((r) => r.json())
          .then((rows: { event: string }[]) => ({
            id,
            done: Array.isArray(rows) && rows.some((r) => r.event === 'completed'),
          }))
          .catch(() => ({ id, done: false }))
      )
    ).then((results) => {
      if (cancelled) return
      setCompletedIds(new Set(results.filter((r) => r.done).map((r) => r.id)))
    })
    return () => { cancelled = true }
  }, [taskNodeIds, runningIds]) // re-check when a run starts/ends

  const built = useMemo(() => {
    if (!graph.data) return null
    return build(entityId, graph.data.nodes, graph.data.edges, runningIds, completedIds)
  }, [graph.data, entityId, runningIds, completedIds])

  if (graph.isLoading) {
    return (
      <Card>
        <CardContent className='p-3'>
          <Skeleton className='h-96 w-full' />
        </CardContent>
      </Card>
    )
  }
  if (graph.isError || !graph.data) {
    return (
      <div className='text-sm text-rose-400'>
        Failed to load graph: {String(graph.error ?? 'no data')}
      </div>
    )
  }
  if (!built || built.data.length === 0) {
    return (
      <Card>
        <CardContent className='text-muted-foreground p-6 text-sm'>
          No graph yet — this entity has no descendants in the relationships
          table.
        </CardContent>
      </Card>
    )
  }

  return (
    <Card className='gap-0 py-0'>
      <CardContent className='p-0'>
        <div className='bg-background h-[calc(100vh-15rem)] min-h-[600px] w-full overflow-hidden rounded-md'>
          <EChart
            height='100%'
            notMerge={!settled}
            option={{
              tooltip: {
                trigger: 'item',
                formatter: (p: { dataType?: string; data: ChartNode | ChartLink }) => {
                  if (p.dataType === 'edge') {
                    const e = p.data as ChartLink
                    return `<div style="font-size:11px"><b>${e._kind}</b><br/>${(e.source as string).slice(0, 12)}… → ${(e.target as string).slice(0, 12)}…</div>`
                  }
                  const d = p.data as ChartNode
                  return `<div style="max-width:280px">
                    <div style="font-weight:600">${d.name}</div>
                    <div style="font-size:11px;opacity:.7">${d._kind} · ${d._status}</div>
                    <div style="font-size:11px;opacity:.5;margin-top:2px">${d.id.slice(0, 12)}…</div>
                  </div>`
                },
              },
              legend: [
                { data: built.categories.map((c) => c.name), top: 8, textStyle: { color: '#e5e7eb' } },
              ],
              animationDurationUpdate: 600,
              animationEasingUpdate: 'quinticInOut',
              series: [
                {
                  type: 'graph',
                  layout: 'force',
                  data: built.data,
                  links: built.links,
                  categories: built.categories,
                  roam: true,
                  draggable: true,
                  label: {
                    show: true,
                    position: 'right',
                    fontSize: 11,
                    color: '#e5e7eb',
                    formatter: (p: { data: ChartNode }) => {
                      const t = p.data.name
                      return t.length > 32 ? t.slice(0, 30) + '…' : t
                    },
                  },
                  edgeLabel: { show: false },
                  emphasis: {
                    focus: 'adjacency',
                    label: { show: true, fontWeight: 'bold' },
                    lineStyle: { width: 2.5 },
                  },
                  // Force tuning is node-count dependent. Sparse graphs
                  // (≤ 10 nodes) need MUCH higher repulsion to spread
                  // across the canvas; dense graphs (50+ nodes) need
                  // lower repulsion or they fly apart. Auto-scale.
                  force: {
                    repulsion: built.data.length <= 10 ? 1500 : built.data.length <= 30 ? 600 : 320,
                    edgeLength: built.data.length <= 10 ? [180, 280] : [100, 180],
                    gravity: 0.08,
                    friction: 0.6,
                    layoutAnimation: !settled,
                  },
                  center: ['50%', '50%'],
                  lineStyle: { width: 1.4, curveness: 0.12, opacity: 0.8 },
                },
              ],
            }}
            onEvents={{
              click: (params: unknown) => {
                const p = params as { dataType?: string; data?: ChartNode }
                if (p.dataType !== 'node' || !p.data) return
                const d = p.data
                if (d.id === entityId) return
                const target =
                  d._kind === 'product'
                    ? '/products'
                    : d._kind === 'manifest'
                      ? '/manifests'
                      : '/tasks'
                // eslint-disable-next-line @typescript-eslint/no-explicit-any
                navigate({
                  to: target,
                  search: { id: d.id, tab: 'dag' },
                } as any)
              },
            }}
          />
        </div>
      </CardContent>
    </Card>
  )
}
