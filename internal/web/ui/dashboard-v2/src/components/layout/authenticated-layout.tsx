import { Outlet } from '@tanstack/react-router'
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { SidebarProvider } from '@/components/ui/sidebar'
import { AppSidebar } from '@/components/layout/app-sidebar'
import { SkipToMain } from '@/components/skip-to-main'

type AuthenticatedLayoutProps = {
  children?: React.ReactNode
}

export function AuthenticatedLayout({ children }: AuthenticatedLayoutProps) {
  return (
    <SearchProvider>
      <LayoutProvider>
        <SidebarProvider>
          <SkipToMain />
          <div className='flex h-svh w-full overflow-hidden bg-background'>
            <PanelGroup direction='horizontal' autoSaveId='op-layout' className='h-full'>
              <Panel defaultSize={18} minSize={12} maxSize={40} className='flex flex-col'>
                <AppSidebar />
              </Panel>
              <PanelResizeHandle className='group bg-border hover:bg-primary/40 data-[resize-handle-state=drag]:bg-primary relative w-1 cursor-col-resize transition-colors'>
                <div className='absolute top-1/2 left-1/2 flex -translate-x-1/2 -translate-y-1/2 flex-col gap-0.5 opacity-0 transition-opacity group-hover:opacity-100'>
                  <span className='block h-0.5 w-0.5 rounded-full bg-foreground/60' />
                  <span className='block h-0.5 w-0.5 rounded-full bg-foreground/60' />
                  <span className='block h-0.5 w-0.5 rounded-full bg-foreground/60' />
                </div>
              </PanelResizeHandle>
              <Panel className='flex flex-col overflow-y-auto @container/content'>
                {children ?? <Outlet />}
              </Panel>
            </PanelGroup>
          </div>
        </SidebarProvider>
      </LayoutProvider>
    </SearchProvider>
  )
}
