import React, { useEffect, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowRight, ChevronRight, GitBranch, Laptop, Moon, Sun } from 'lucide-react'
import { useSearch } from '@/context/search-provider'
import { useTheme } from '@/context/theme-provider'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import { sidebarData } from './layout/data/sidebar-data'
import { ScrollArea } from './ui/scroll-area'
import { KIND_ICON } from '@/features/entity/tree/EntityTreeNode'
import type { Entity } from '@/lib/types'

// UX knobs for the inline entity search inside the ⌘K palette.
const ENTITY_QUERY_MIN_LEN = 2
const ENTITY_QUERY_DEBOUNCE_MS = 200
const ENTITY_QUERY_LIMIT = 20

export function CommandMenu() {
  const navigate = useNavigate()
  const { setTheme } = useTheme()
  const { open, setOpen } = useSearch()

  const [query, setQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query), ENTITY_QUERY_DEBOUNCE_MS)
    return () => clearTimeout(t)
  }, [query])

  // Reset the query when the palette closes so the next open starts fresh.
  useEffect(() => {
    if (!open) setQuery('')
  }, [open])

  const { data: entityResults } = useQuery<Entity[]>({
    queryKey: ['command-menu', 'entity-search', debouncedQuery],
    queryFn: async () => {
      const url = `/api/entities/search?q=${encodeURIComponent(debouncedQuery)}&limit=${ENTITY_QUERY_LIMIT}`
      const res = await fetch(url)
      if (!res.ok) throw new Error(`entity search → ${res.status}`)
      return (await res.json()) as Entity[]
    },
    enabled: debouncedQuery.length >= ENTITY_QUERY_MIN_LEN,
    staleTime: 30 * 1000,
  })

  const runCommand = React.useCallback(
    (command: () => unknown) => {
      setOpen(false)
      command()
    },
    [setOpen]
  )

  return (
    <CommandDialog modal open={open} onOpenChange={setOpen}>
      <CommandInput
        placeholder='Type a command or search...'
        value={query}
        onValueChange={setQuery}
      />
      <CommandList>
        <ScrollArea type='hover' className='h-72 pe-1'>
          <CommandEmpty>No results found.</CommandEmpty>
          {entityResults && entityResults.length > 0 && (
            <>
              <CommandGroup heading='Entities'>
                {entityResults.map((e) => {
                  const Icon = KIND_ICON[e.type] ?? GitBranch
                  return (
                    <CommandItem
                      key={e.entity_uid}
                      value={`${e.type} ${e.title} ${e.entity_uid}`}
                      onSelect={() => {
                        runCommand(() =>
                          navigate({
                            to: '/entities/$uid',
                            params: { uid: e.entity_uid },
                          })
                        )
                      }}
                    >
                      <Icon className='size-4 shrink-0 text-muted-foreground' />
                      <span className='flex-1 min-w-0 truncate'>{e.title}</span>
                      <span className='ml-2 text-[10px] text-muted-foreground capitalize shrink-0'>
                        {e.type}
                      </span>
                    </CommandItem>
                  )
                })}
              </CommandGroup>
              <CommandSeparator />
            </>
          )}
          {sidebarData.navGroups.map((group) => (
            <CommandGroup key={group.title} heading={group.title}>
              {group.items.map((navItem, i) => {
                if (navItem.url)
                  return (
                    <CommandItem
                      key={`${navItem.url}-${i}`}
                      value={navItem.title}
                      onSelect={() => {
                        runCommand(() => navigate({ to: navItem.url }))
                      }}
                    >
                      <div className='flex size-4 items-center justify-center'>
                        <ArrowRight className='size-2 text-muted-foreground/80' />
                      </div>
                      {navItem.title}
                    </CommandItem>
                  )

                return navItem.items?.map((subItem, i) => (
                  <CommandItem
                    key={`${navItem.title}-${subItem.url}-${i}`}
                    value={`${navItem.title}-${subItem.url}`}
                    onSelect={() => {
                      runCommand(() => navigate({ to: subItem.url }))
                    }}
                  >
                    <div className='flex size-4 items-center justify-center'>
                      <ArrowRight className='size-2 text-muted-foreground/80' />
                    </div>
                    {navItem.title} <ChevronRight /> {subItem.title}
                  </CommandItem>
                ))
              })}
            </CommandGroup>
          ))}
          <CommandSeparator />
          <CommandGroup heading='Theme'>
            <CommandItem onSelect={() => runCommand(() => setTheme('light'))}>
              <Sun /> <span>Light</span>
            </CommandItem>
            <CommandItem onSelect={() => runCommand(() => setTheme('dark'))}>
              <Moon className='scale-90' />
              <span>Dark</span>
            </CommandItem>
            <CommandItem onSelect={() => runCommand(() => setTheme('system'))}>
              <Laptop />
              <span>System</span>
            </CommandItem>
          </CommandGroup>
        </ScrollArea>
      </CommandList>
    </CommandDialog>
  )
}
