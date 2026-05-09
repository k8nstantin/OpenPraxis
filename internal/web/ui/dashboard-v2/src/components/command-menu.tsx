import React from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { ArrowRight, ChevronRight, Laptop, Moon, Sun } from 'lucide-react'
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
import { EntityKindIcon } from '@/features/entity/tree/EntityTreeNode'
import type { Entity } from '@/lib/types'
import { sidebarData } from './layout/data/sidebar-data'
import { ScrollArea } from './ui/scroll-area'

const ENTITY_QUERY_MIN_LEN = 2
const ENTITY_QUERY_DEBOUNCE_MS = 200
const ENTITY_QUERY_LIMIT = 20

export function CommandMenu() {
  const navigate = useNavigate()
  const { setTheme } = useTheme()
  const { open, setOpen } = useSearch()
  const [query, setQuery] = React.useState('')
  const [debouncedQuery, setDebouncedQuery] = React.useState('')

  React.useEffect(() => {
    const t = setTimeout(
      () => setDebouncedQuery(query.trim()),
      ENTITY_QUERY_DEBOUNCE_MS
    )
    return () => clearTimeout(t)
  }, [query])

  const enabled = debouncedQuery.length >= ENTITY_QUERY_MIN_LEN
  const { data: results } = useQuery({
    queryKey: ['entity-search-cmd', debouncedQuery],
    queryFn: async (): Promise<Entity[]> => {
      const url = `/api/entities/search?q=${encodeURIComponent(debouncedQuery)}&limit=${ENTITY_QUERY_LIMIT}`
      const res = await fetch(url)
      if (!res.ok) throw new Error(`${url} → ${res.status}`)
      return (await res.json()) as Entity[]
    },
    enabled,
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
        placeholder='Search entities or type a command...'
        value={query}
        onValueChange={setQuery}
      />
      <CommandList>
        <ScrollArea type='hover' className='h-72 pe-1'>
          <CommandEmpty>No results found.</CommandEmpty>
          {enabled && results && results.length > 0 && (
            <CommandGroup heading='Entities'>
              {results.map((entity) => (
                <CommandItem
                  key={entity.entity_uid}
                  value={`${entity.type} ${entity.title} ${entity.entity_uid}`}
                  onSelect={() =>
                    runCommand(() =>
                      navigate({
                        to: '/entities/$uid',
                        params: { uid: entity.entity_uid },
                      })
                    )
                  }
                >
                  <EntityKindIcon
                    kind={entity.type}
                    className='mr-2 h-4 w-4 text-muted-foreground'
                  />
                  <span className='flex-1 truncate'>{entity.title}</span>
                  <span className='ml-2 text-xs text-muted-foreground capitalize'>
                    {entity.type}
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
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
