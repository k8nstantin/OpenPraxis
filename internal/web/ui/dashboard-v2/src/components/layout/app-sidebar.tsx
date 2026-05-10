import { useLayout } from '@/context/layout-provider'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarRail,
} from '@/components/ui/sidebar'
import { sidebarData } from './data/sidebar-data'
import { NavGroup } from './nav-group'
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
        {/* Entity tree: Skills + Products→Manifests→Tasks hierarchy */}
        <SidebarGroup className='flex flex-col flex-1 min-h-0 p-0'>
          <SidebarGroupContent className='flex flex-col flex-1 min-h-0'>
            <EntityTree />
          </SidebarGroupContent>
        </SidebarGroup>
        {/* Page nav: Overview, Stats, Settings etc — no entity type links */}
        <div className='shrink-0'>
          {sidebarData.navGroups.map((props) => (
            <NavGroup key={props.title} {...props} />
          ))}
        </div>
      </SidebarContent>
      <SidebarFooter>
        <NavUser user={sidebarData.user} />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
