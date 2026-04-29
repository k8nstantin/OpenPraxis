import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { ApiHealthCheck } from './api-health-check'
import { RunningTasksPanel } from './running-tasks-panel'

// Overview tab — morning landing for the operator. The headline panel
// is RunningTasksPanel: 8 chart variants over the live task + system
// metrics so the operator can pick which views earn a permanent slot.
// Wiring health check stays below as a quick smoke probe.
export function Overview() {
  return (
    <>
      <Header />
      <Main>
        <div className='mb-2 flex items-center justify-between space-y-2'>
          <h1 className='text-2xl font-bold tracking-tight'>Overview</h1>
        </div>
        <div className='space-y-4'>
          <RunningTasksPanel />
          <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
            <ApiHealthCheck />
          </div>
        </div>
      </Main>
    </>
  )
}
