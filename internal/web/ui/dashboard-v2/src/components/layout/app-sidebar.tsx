import { useLayout } from '@/context/layout-provider'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
} from '@/components/ui/sidebar'
import { sidebarData } from './data/sidebar-data'
import { NavUser } from './nav-user'
import { TeamSwitcher } from './team-switcher'
import { EntityTree } from '@/features/entity/tree/EntityTree'

export function AppSidebar() {
  const { collapsible, variant } = useLayout()
  return (
    <Sidebar collapsible={collapsible} variant={variant}>
      <SidebarHeader>
        <TeamSwitcher teams={sidebarData.teams} />
      </SidebarHeader>
      <SidebarContent className='overflow-x-hidden flex flex-col'>
        {/* All navigation through a single arborist tree: Skills, Entities, Nav pages */}
        <SidebarGroup className='flex flex-col flex-1 min-h-0 p-0'>
          <SidebarGroupContent className='flex flex-col flex-1 min-h-0'>
            <EntityTree />
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <NavUser user={sidebarData.user} />
      </SidebarFooter>
    </Sidebar>
  )
}
