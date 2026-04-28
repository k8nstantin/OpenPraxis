import { useEntity, type EntityKind } from '@/lib/queries/entity'
import { Boxes, CheckSquare, FileText } from 'lucide-react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { EntityBreadcrumb } from './breadcrumb'
import { EntityStatusControl } from './status-control'
import { CommentsTab } from './tabs/comments'
import { DAGTab } from './tabs/dag'
import { DependenciesTab } from './tabs/dependencies'
import { ExecutionTab } from './tabs/execution'
import { MainTab } from './tabs/main'
import { ScheduleTab } from './tabs/schedule'

// Seven tabs in operator-priority order — same set across product /
// manifest / task. Schedule + Stats are placeholders pending the
// central schedules + run_stats backend (next PR); they render a
// "pending backend" stub so the tab strip is complete now.
const TAB_IDS = [
  'main',
  'execution',
  'comments',
  'dependencies',
  'dag',
  'schedule',
  'stats',
] as const

export type EntityTabId = (typeof TAB_IDS)[number]

// Right-pane content for the master-detail layout. Renders breadcrumb +
// title + status + 5-tab strip for the selected entity. No outer
// Header/Main — the parent EntityPage owns those wrappers.
export function EntityDetailPane({
  kind,
  entityId,
  tab,
  onTabChange,
}: {
  kind: EntityKind
  entityId?: string
  tab: EntityTabId
  onTabChange: (tab: EntityTabId) => void
}) {
  const entity = useEntity(kind, entityId)
  const Icon =
    kind === 'product' ? Boxes : kind === 'task' ? CheckSquare : FileText
  const noun =
    kind === 'product' ? 'product' : kind === 'task' ? 'task' : 'manifest'

  if (!entityId) {
    return (
      <div className='text-muted-foreground flex h-full flex-col items-center justify-center gap-3 p-6 text-center'>
        <Icon className='h-12 w-12 opacity-30' />
        <div className='text-sm'>Pick a {noun} from the list to see its tabs.</div>
      </div>
    )
  }

  return (
    <div className='flex h-full min-h-0 flex-col'>
      <ScrollArea className='min-h-0 flex-1'>
        <div className='space-y-4 p-4'>
          <EntityBreadcrumb
            kind={kind}
            entityId={entityId}
            entityTitle={entity.data?.title}
          />

          <div>
            {entity.isLoading ? (
              <Skeleton className='h-8 w-1/2' />
            ) : entity.isError ? (
              <div className='text-sm text-rose-400'>
                Failed to load: {String(entity.error)}
              </div>
            ) : entity.data ? (
              <div className='flex items-start justify-between gap-3'>
                <div>
                  <h1 className='text-2xl font-bold tracking-tight'>
                    {entity.data.title}
                  </h1>
                  <code className='text-muted-foreground font-mono text-xs'>
                    {entity.data.id}
                  </code>
                </div>
                <EntityStatusControl
                  kind={kind}
                  entityId={entityId}
                  status={entity.data.status}
                  entityTitle={entity.data.title}
                />
              </div>
            ) : null}
          </div>

          <Tabs
            value={tab}
            onValueChange={(v) => onTabChange(v as EntityTabId)}
            className='space-y-2'
          >
            <TabsList>
              <TabsTrigger value='main'>Main</TabsTrigger>
              <TabsTrigger value='execution'>Execution Control</TabsTrigger>
              <TabsTrigger value='comments'>Comments</TabsTrigger>
              <TabsTrigger value='dependencies'>Dependencies</TabsTrigger>
              <TabsTrigger value='dag'>DAG</TabsTrigger>
              <TabsTrigger value='schedule'>Schedule</TabsTrigger>
              <TabsTrigger value='stats'>Stats</TabsTrigger>
            </TabsList>

            <TabsContent value='main'>
              <MainTab kind={kind} entityId={entityId} />
            </TabsContent>
            <TabsContent value='execution'>
              <ExecutionTab kind={kind} entityId={entityId} />
            </TabsContent>
            <TabsContent value='comments'>
              <CommentsTab kind={kind} entityId={entityId} />
            </TabsContent>
            <TabsContent value='dependencies'>
              <DependenciesTab kind={kind} entityId={entityId} />
            </TabsContent>
            <TabsContent value='dag'>
              <DAGTab kind={kind} entityId={entityId} />
            </TabsContent>
            <TabsContent value='schedule'>
              <ScheduleTab kind={kind} entityId={entityId} />
            </TabsContent>
            <TabsContent value='stats'>
              <StatsPlaceholder />
            </TabsContent>
          </Tabs>
        </div>
      </ScrollArea>
    </div>
  )
}

// Placeholder until the central `run_stats` table + per-entity
// rollup queries land. ECharts visuals lock in here once data is wired.
function StatsPlaceholder() {
  return (
    <div className='text-muted-foreground rounded-md border bg-card p-6 text-sm'>
      <div className='mb-2 font-medium text-foreground'>Stats</div>
      <p>
        Per-run charts (cost, turns, actions, tokens, cpu%, rss) for this
        entity. Backed by <code className='font-mono text-xs'>task_runs</code>
        {' + '}<code className='font-mono text-xs'>task_run_host_samples</code>
        ; aggregated across descendants on products / manifests, individual
        runs on tasks. ECharts. Pending the backend-decoupling PR.
      </p>
    </div>
  )
}
