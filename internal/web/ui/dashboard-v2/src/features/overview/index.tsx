import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ApiHealthCheck } from './api-health-check'

// Overview tab — morning landing for the operator. Currently shows the
// placeholder + the chunk-3 backend wiring health check. Real content
// (alerts, ship feed, budget gauge, productivity strip, message of the
// day) lands in chunk 4 once the spec is locked.
export function Overview() {
  return (
    <>
      <Header />
      <Main>
        <div className='mb-2 flex items-center justify-between space-y-2'>
          <h1 className='text-2xl font-bold tracking-tight'>Overview</h1>
        </div>
        <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
          <Card>
            <CardHeader>
              <CardTitle>Pending</CardTitle>
            </CardHeader>
            <CardContent className='text-muted-foreground space-y-3 text-sm'>
              <p>
                Morning landing — alerts, where I left off, overnight ship
                feed, budget gauge, productivity strip, message of the day.
              </p>
              <p>
                Real content ships in chunk 4. The wiring health check on
                the right proves the React shell on :9766 talks to the same
                backend Portal A on :8765 talks to.
              </p>
            </CardContent>
          </Card>
          <ApiHealthCheck />
        </div>
      </Main>
    </>
  )
}
