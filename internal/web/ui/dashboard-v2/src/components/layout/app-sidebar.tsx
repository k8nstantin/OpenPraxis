import { sidebarData } from './data/sidebar-data'
import { NavUser } from './nav-user'
import { TeamSwitcher } from './team-switcher'
import { EntityTree } from '@/features/entity/tree/EntityTree'

export function AppSidebar() {
  return (
    <div className='flex h-full w-full flex-col bg-sidebar text-sidebar-foreground border-r border-border overflow-hidden'>
      <div className='shrink-0 p-2'>
        <TeamSwitcher teams={sidebarData.teams} />
      </div>
      <div className='flex flex-col flex-1 min-h-0 overflow-hidden px-1 py-1'>
        <EntityTree />
      </div>
      <div className='shrink-0 p-2'>
        <NavUser user={sidebarData.user} />
      </div>
    </div>
  )
}
