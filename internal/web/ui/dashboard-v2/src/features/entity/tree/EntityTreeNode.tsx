import type { ElementType } from 'react'
import type { NodeRendererProps } from 'react-arborist'
import {
  Boxes,
  CheckSquare,
  ChevronRight,
  FileText,
  GitBranch,
  Lightbulb,
  Wand2,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import type { EntityKind } from '@/lib/queries/entity'
import { TreeStatus, type TreeNode } from '@/lib/queries/entity-tree'

const KIND_ICON: Record<EntityKind, ElementType> = {
  skill: Wand2,
  idea: Lightbulb,
  product: Boxes,
  manifest: FileText,
  task: CheckSquare,
}

const STATUS_COLOR: Partial<Record<string, string>> = {
  [TreeStatus.Running]: 'text-blue-400',
  [TreeStatus.Completed]: 'text-emerald-500',
  [TreeStatus.Failed]: 'text-red-500',
  [TreeStatus.Active]: 'text-amber-400',
}

const STATUS_GLYPH: Partial<Record<string, string>> = {
  [TreeStatus.Running]: '●',
  [TreeStatus.Completed]: '✓',
  [TreeStatus.Failed]: '✗',
  [TreeStatus.Active]: '⚠',
  [TreeStatus.Draft]: '○',
  [TreeStatus.Closed]: '○',
  [TreeStatus.Archived]: '○',
}

// Sentinel prefix for synthetic group-header rows (e.g. __skills__, __lifecycle__).
// The Tree data wrapper in EntityTree.tsx is the only place that creates these.
const GROUP_PREFIX = '__'

export function EntityTreeNode({ node, style, dragHandle }: NodeRendererProps<TreeNode>) {
  // Group separator header. Render as a plain styled div — using
  // SidebarGroupLabel here would target a sidebar wrapper ancestor that does
  // not exist inside react-arborist's virtualized list DOM.
  if (node.id.startsWith(GROUP_PREFIX)) {
    return (
      <div
        style={style}
        onClick={() => node.toggle()}
        className='flex items-center px-2 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground select-none cursor-pointer hover:text-foreground'
      >
        {node.data.name}
      </div>
    )
  }

  const Icon = KIND_ICON[node.data.kind] ?? GitBranch
  const glyph = STATUS_GLYPH[node.data.status] ?? '○'
  const color = STATUS_COLOR[node.data.status] ?? 'text-muted-foreground'
  const isPulsing = node.data.status === TreeStatus.Running

  return (
    <div
      ref={dragHandle}
      style={style}
      onClick={() => (node.isInternal ? node.toggle() : node.select())}
      className={cn(
        'flex items-center gap-1 px-1 rounded-sm cursor-pointer select-none',
        'hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
        node.isSelected && 'bg-sidebar-accent text-sidebar-accent-foreground',
      )}
    >
      <ChevronRight
        className={cn(
          'h-3 w-3 shrink-0 text-muted-foreground/60 transition-transform',
          node.isInternal ? 'opacity-100' : 'opacity-0',
          node.isInternal && node.isOpen && 'rotate-90',
        )}
      />
      <Icon className='h-3.5 w-3.5 shrink-0 text-muted-foreground' />
      <span className='flex-1 min-w-0 text-xs truncate'>{node.data.name}</span>
      <span
        className={cn(
          'text-[10px] shrink-0 ml-1',
          color,
          isPulsing && 'animate-pulse',
        )}
      >
        {glyph}
      </span>
    </div>
  )
}
