import * as React from 'react'
import * as TabsPrimitive from '@radix-ui/react-tabs'
import { cn } from '@/lib/utils'

function Tabs({
  className,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Root>) {
  return (
    <TabsPrimitive.Root
      data-slot='tabs'
      className={cn('flex flex-col gap-2', className)}
      {...props}
    />
  )
}

// Folder-tab visuals (vs. the default pill-row): each trigger has top
// corners rounded, border on top + sides, and the active tab merges
// with the content surface below by overlapping the list's bottom edge
// and dropping its own bottom border. TabsList is just the shelf —
// `border-b` becomes the line every inactive tab sits on, the active
// tab sticks up through it.
function TabsList({
  className,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.List>) {
  return (
    <TabsPrimitive.List
      data-slot='tabs-list'
      className={cn(
        'flex w-full items-end gap-2 border-b px-2 text-muted-foreground',
        className
      )}
      {...props}
    />
  )
}

function TabsTrigger({
  className,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Trigger>) {
  return (
    <TabsPrimitive.Trigger
      data-slot='tabs-trigger'
      className={cn(
        // Base — folder shape: top-rounded only, sides + top border,
        // small lift on hover. The `-mb-px` pulls the tab down by a
        // pixel so its bottom edge sits ON the TabsList's border-b.
        'relative -mb-px inline-flex items-center gap-1.5 whitespace-nowrap rounded-t-md border border-transparent px-3 py-1.5 text-sm font-medium transition-colors',
        // Inactive — muted bg, subtle border, hover lifts color.
        'bg-muted/40 hover:bg-muted/70 hover:text-foreground',
        // Active — match the content surface, draw top + sides, hide
        // the bottom border so the tab "merges" into the content area
        // below. Slightly bolder text + subtle inset shadow on top.
        'data-[state=active]:bg-card data-[state=active]:text-foreground',
        'data-[state=active]:border-border data-[state=active]:border-b-card',
        'data-[state=active]:font-semibold',
        // Focus + disabled.
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        'disabled:pointer-events-none disabled:opacity-50',
        "[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        className
      )}
      {...props}
    />
  )
}

function TabsContent({
  className,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Content>) {
  return (
    <TabsPrimitive.Content
      data-slot='tabs-content'
      className={cn('flex-1 outline-none', className)}
      {...props}
    />
  )
}

export { Tabs, TabsList, TabsTrigger, TabsContent }
