import { Header } from '@/components/layout/header'
import { Main } from '@/components/layout/main'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

// Placeholder for tabs whose content hasn't been built yet. Lands as a
// real route in chunk 2 so the sidebar nav + hash-routing all work end
// to end; the per-tab feature components replace this in subsequent
// chunks (Overview first, then Active / Catalog / etc.).
export function TabPlaceholder({
  name,
  description,
}: {
  name: string
  description: string
}) {
  return (
    <>
      <Header />
      <Main>
        <div className='mb-2 flex items-center justify-between space-y-2'>
          <h1 className='text-2xl font-bold tracking-tight'>{name}</h1>
        </div>
        <Card>
          <CardHeader>
            <CardTitle>Pending</CardTitle>
          </CardHeader>
          <CardContent className='space-y-3 text-sm text-muted-foreground'>
            <p>{description}</p>
            <p>
              This route is part of the chunk-2 scaffold. The real content
              ships in a follow-up chunk dedicated to this tab.
            </p>
          </CardContent>
        </Card>
      </Main>
    </>
  )
}
