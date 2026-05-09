import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/entities/')({
  component: EntitiesIndex,
})

function EntitiesIndex() {
  return (
    <div className='text-muted-foreground flex h-full items-center justify-center'>
      Select an entity from the tree
    </div>
  )
}
