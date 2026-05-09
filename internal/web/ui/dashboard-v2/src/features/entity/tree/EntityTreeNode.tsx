import {
  Boxes,
  CheckSquare,
  ChevronRight,
  FileText,
  Lightbulb,
  Wand2,
  type LucideIcon,
} from 'lucide-react'
import type { NodeRendererProps } from 'react-arborist'
import { cn } from '@/lib/utils'
import type { EntityKind } from '@/lib/queries/entity'
import {
  KIND,
  STATUS,
  type TreeNode,
  type TreeStatus,
} from '@/lib/queries/entity-tree'

// Icon per kind. Record<EntityKind, …> forces every kind to be mapped —
// adding a new kind to EntityKind makes this fail to typecheck until an
// icon is supplied. Single source of truth for kind→icon visualization.
const KIND_ICON: Record<EntityKind, LucideIcon> = {
  [KIND.skill]: Wand2,
  [KIND.idea]: Lightbulb,
  [KIND.product]: Boxes,
  [KIND.manifest]: FileText,
  [KIND.task]: CheckSquare,
}

export function EntityKindIcon({
  kind,
  className,
}: {
  kind: EntityKind
  className?: string
}) {
  const Icon = KIND_ICON[kind]
  return <Icon className={className} />
}

// Status dot — only meaningful for executable entities (task is the
// source of truth; manifest/product are derived upward). Skills and
// ideas have no execution surface so render nothing.
const STATUS_VISIBLE_FOR: ReadonlySet<EntityKind> = new Set<EntityKind>([
  KIND.task,
  KIND.manifest,
  KIND.product,
])

type StatusVisual = { glyph: string; className: string }

const STATUS_VISUALS: Record<TreeStatus, StatusVisual> = {
  [STATUS.running]: { glyph: '●', className: 'animate-pulse text-blue-400' },
  [STATUS.completed]: { glyph: '✓', className: 'text-emerald-500' },
  [STATUS.failed]: { glyph: '✗', className: 'text-red-500' },
  [STATUS.active]: { glyph: '⚠', className: 'text-amber-400' },
  [STATUS.draft]: { glyph: '○', className: 'text-muted-foreground' },
  [STATUS.closed]: { glyph: '○', className: 'text-muted-foreground' },
  [STATUS.archived]: { glyph: '○', className: 'text-muted-foreground' },
}

export function StatusDot({
  status,
  kind,
}: {
  status: TreeStatus
  kind: EntityKind
}) {
  if (!STATUS_VISIBLE_FOR.has(kind)) return null
  const visual = STATUS_VISUALS[status]
  return (
    <span
      aria-label={`status: ${status}`}
      className={cn('text-xs leading-none shrink-0', visual.className)}
    >
      {visual.glyph}
    </span>
  )
}

export function EntityTreeNode({
  node,
  style,
  dragHandle,
}: NodeRendererProps<TreeNode>) {
  const isSelected = node.isSelected
  const data = node.data
  const childCount = data.children?.length ?? 0

  return (
    <div
      ref={dragHandle}
      style={style}
      onClick={() => (node.isInternal ? node.toggle() : node.select())}
      className={cn(
        'flex items-center gap-1.5 px-2 py-0.5 rounded-sm cursor-pointer text-sm',
        'hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
        isSelected && 'bg-sidebar-accent text-sidebar-accent-foreground font-medium',
      )}
    >
      {node.isInternal ? (
        <ChevronRight
          className={cn(
            'h-3 w-3 shrink-0 text-muted-foreground transition-transform',
            node.isOpen && 'rotate-90',
          )}
        />
      ) : (
        <span className='w-3' />
      )}

      <EntityKindIcon
        kind={data.kind}
        className='h-3.5 w-3.5 shrink-0 text-muted-foreground'
      />

      <span className='truncate flex-1 min-w-0'>{data.name}</span>

      <StatusDot status={data.status} kind={data.kind} />

      {node.isInternal && !node.isOpen && childCount > 0 && (
        <span className='text-xs text-muted-foreground ml-auto'>
          {childCount}
        </span>
      )}
    </div>
  )
}
