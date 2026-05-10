import { createFileRoute, redirect } from '@tanstack/react-router'
import { Outlet } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/settings')({
  beforeLoad: ({ location }) => {
    if (location.pathname === '/settings' || location.pathname === '/settings/') {
      throw redirect({ to: '/settings/system' })
    }
  },
  component: () => <Outlet />,
})
