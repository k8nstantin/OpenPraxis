import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/entities/$uid')({
  component: EntityDetail,
})

function EntityDetail() {
  return (
    <div className='flex h-full items-center justify-center text-sm text-muted-foreground'>
      Loading…
    </div>
  )
}
