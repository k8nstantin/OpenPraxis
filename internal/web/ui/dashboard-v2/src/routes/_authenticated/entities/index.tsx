import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/entities/')({
  component: EntitiesIndex,
})

function EntitiesIndex() {
  return (
    <div className='flex h-full items-center justify-center text-sm text-muted-foreground'>
      Select an entity from the tree
    </div>
  )
}
