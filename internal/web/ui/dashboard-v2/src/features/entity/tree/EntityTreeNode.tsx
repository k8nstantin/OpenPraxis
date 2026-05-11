import type { ElementType } from 'react'
import type { NodeApi, NodeRendererProps } from 'react-arborist'
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Boxes,
  Brain,
  CalendarClock,
  CheckSquare,
  ChevronRight,
  Database,
  FileText,
  GitBranch,
  Inbox,
  LayoutDashboard,
  Lightbulb,
  ScrollText,
  Settings,
  TrendingUp,
  Wand2,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { TreeStatus, type TreeNode } from '@/lib/queries/entity-tree'

export const KIND_ICON: Record<string, ElementType> = {
  skill: Wand2,
  idea: Lightbulb,
  product: Boxes,
  manifest: FileText,
  task: CheckSquare,
  RAG: Database,
}

// Lucide icons available for dynamic entity types — keyed by the icon
// name string stored in entity_types.icon. Extend this map when new
// icon names are used in Settings › Entity Types.
export const LUCIDE_BY_NAME: Record<string, ElementType> = {
  Activity, AlertTriangle, BarChart3, Boxes, Brain, CalendarClock,
  CheckSquare, Database, FileText, GitBranch, Inbox, LayoutDashboard,
  Lightbulb, ScrollText, Settings, TrendingUp, Wand2,
}

// Resolve the icon for any entity kind — checks KIND_ICON first,
// then LUCIDE_BY_NAME (for dynamic types with an icon name from entity_types),
// then falls back to GitBranch.
export function kindIcon(kind: string, iconName?: string): ElementType {
  return KIND_ICON[kind] ?? (iconName ? LUCIDE_BY_NAME[iconName] : undefined) ?? GitBranch
}

// Icons for synthetic page nav nodes — keyed by node ID.
const PAGE_NAV_ICON: Record<string, ElementType> = {
  __page_overview__: LayoutDashboard,
  __page_actions__: ScrollText,
  __page_schedules__: CalendarClock,
  __page_inbox__: Inbox,
  __page_recall__: Brain,
  __page_stats__: BarChart3,
  __page_productivity__: TrendingUp,
  __page_audit__: AlertTriangle,
  __page_activity__: Activity,
  __page_settings__: Settings,
  __page_entity_types__: Database,
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
  [TreeStatus.Active]: '●',
  [TreeStatus.Draft]: '○',
  [TreeStatus.Closed]: '○',
  [TreeStatus.Archived]: '○',
}

// Opens a node and recursively opens all its descendants so the full subtree
// is visible in one click (product → manifests → tasks all revealed at once).
function openSubtree(n: NodeApi<TreeNode>) {
  n.open()
  for (const child of n.children ?? []) {
    openSubtree(child)
  }
}

export function EntityTreeNode({ node, style, dragHandle }: NodeRendererProps<TreeNode>) {
  // Group separator header (kind === '__group__').
  // Plain div — SidebarGroupLabel targets a sidebar ancestor that doesn't exist
  // inside react-arborist's virtualized list DOM.
  if (node.data.kind === '__group__') {
    return (
      <div
        style={style}
        onClick={() => node.toggle()}
        className='flex items-center pl-3 pr-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground select-none cursor-pointer hover:text-foreground'
      >
        <ChevronRight className={cn('h-3 w-3 mr-1 shrink-0 transition-transform', node.isOpen && 'rotate-90')} />
        {node.data.name}
        <span className='ml-1 text-[9px] opacity-50'>({node.children?.length ?? 0})</span>
      </div>
    )
  }

  // Page nav leaf node — no status glyph, icon from PAGE_NAV_ICON map.
  if (node.data.kind === 'page') {
    const Icon = PAGE_NAV_ICON[node.id] ?? LayoutDashboard
    return (
      <div
        ref={dragHandle}
        style={style}
        onClick={() => node.select()}
        className={cn(
          'flex items-center gap-1 px-1 rounded-sm cursor-pointer select-none',
          'hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
          node.isSelected && 'bg-sidebar-accent text-sidebar-accent-foreground',
        )}
      >
        <span className='w-3 shrink-0' />
        <Icon className='h-3.5 w-3.5 shrink-0 text-muted-foreground' />
        <span className='flex-1 min-w-0 text-[13px] truncate'>{node.data.name}</span>
      </div>
    )
  }

  const Icon = kindIcon(node.data.kind, node.data.iconName)
  const glyph = STATUS_GLYPH[node.data.status] ?? '○'
  const color = STATUS_COLOR[node.data.status] ?? 'text-muted-foreground'
  const isPulsing = node.data.status === TreeStatus.Running

  return (
    <div
      ref={dragHandle}
      style={style}
      onClick={() => {
        if (node.isInternal) {
          node.isOpen ? node.close() : openSubtree(node)
        } else {
          node.select()
        }
      }}
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
      <span className='flex-1 min-w-0 text-[13px] truncate'>{node.data.name}</span>
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
