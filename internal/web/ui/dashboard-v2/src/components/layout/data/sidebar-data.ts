import {
  Activity,
  AlertTriangle,
  BarChart3,
  Brain,
  CalendarClock,
  Command,
  GitBranch,
  Inbox,
  LayoutDashboard,
  ScrollText,
  Settings,
  TrendingUp,
} from 'lucide-react'
import { type SidebarData } from '../types'

// OpenPraxis Portal V2 — 8 main tabs + 1 settings.
//
// Routes are file-based (TanStack Router) under src/routes/_authenticated/.
// Each url here maps to a route file.
export const sidebarData: SidebarData = {
  user: {
    name: 'operator',
    email: 'operator@openpraxis',
    avatar: '/avatars/shadcn.jpg',
  },
  teams: [
    {
      name: 'OpenPraxis',
      logo: Command,
      plan: 'Portal V2',
    },
  ],
  navGroups: [
    {
      title: 'Operations',
      items: [
        {
          title: 'Overview',
          url: '/',
          icon: LayoutDashboard,
        },
        {
          title: 'Actions Log',
          url: '/actions',
          icon: ScrollText,
        },
        {
          title: 'Entities',
          url: '/entities',
          icon: GitBranch,
        },
        {
          title: 'Schedules',
          url: '/schedules',
          icon: CalendarClock,
        },
        {
          title: 'Inbox',
          url: '/inbox',
          icon: Inbox,
        },
        {
          title: 'Recall',
          url: '/recall',
          icon: Brain,
        },
      ],
    },
    {
      title: 'Governance',
      items: [
        {
          title: 'Stats',
          url: '/stats',
          icon: BarChart3,
        },
        {
          title: 'Productivity',
          url: '/productivity',
          icon: TrendingUp,
        },
        {
          title: 'Audit',
          url: '/audit',
          icon: AlertTriangle,
        },
        {
          title: 'Activity',
          url: '/activity',
          icon: Activity,
        },
      ],
    },
    {
      title: 'Configuration',
      items: [
        {
          title: 'Settings',
          url: '/settings',
          icon: Settings,
        },
      ],
    },
  ],
}
