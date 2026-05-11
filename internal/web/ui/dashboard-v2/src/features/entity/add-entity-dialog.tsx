import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { useQueryClient } from '@tanstack/react-query'
import { useEntityTypes } from '@/lib/queries/entity-types'
import { useCreateEntity } from '@/lib/queries/entity'
import type { AnyEntityKind } from '@/lib/queries/entity'
import type { Entity } from '@/lib/types'

interface AddEntityDialogProps {
  open: boolean
  onOpenChange: (next: boolean) => void
  onCreated?: (entity: Entity) => void
}

// AddEntityDialog is a generic "create entity" dialog that sources
// the available type options from the DB-driven /api/entity-types
// endpoint rather than a hardcoded array. While the types are loading
// the Select trigger shows a skeleton. The created entity is forwarded
// via onCreated so callers can navigate or update local state.
export function AddEntityDialog({
  open,
  onOpenChange,
  onCreated,
}: AddEntityDialogProps) {
  const qc = useQueryClient()
  const { data: entityTypes, isLoading: typesLoading } = useEntityTypes()
  const [selectedType, setSelectedType] = useState<string>('')
  const [title, setTitle] = useState('')

  // useCreateEntity requires the kind up front; we update it dynamically
  // as the user picks a type. Fallback to 'task' to satisfy the hook's
  // type requirement while no type is selected.
  const create = useCreateEntity((selectedType || 'task') as AnyEntityKind)

  const handleSubmit = async () => {
    if (!selectedType || !title.trim()) return
    try {
      const entity = await create.mutateAsync({
        type: selectedType as AnyEntityKind,
        title: title.trim(),
        status: 'draft',
      })
      // Invalidate the entity-types cache so sidebar and other consumers
      // reflect any newly registered type without waiting for staleTime.
      qc.invalidateQueries({ queryKey: ['entity-types'] })
      setTitle('')
      setSelectedType('')
      onOpenChange(false)
      onCreated?.(entity)
    } catch {
      // create.isError surfaces the error in the UI
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-[400px]'>
        <DialogHeader>
          <DialogTitle>New entity</DialogTitle>
        </DialogHeader>

        <div className='space-y-4 py-2'>
          <div className='space-y-1.5'>
            <Label htmlFor='entity-type' className='text-xs'>
              Type
            </Label>
            {typesLoading ? (
              <Skeleton className='h-9 w-full' />
            ) : (
              <Select value={selectedType} onValueChange={setSelectedType}>
                <SelectTrigger id='entity-type' className='h-9 text-sm'>
                  <SelectValue placeholder='Select a type…' />
                </SelectTrigger>
                <SelectContent>
                  {(entityTypes ?? []).map((et) => (
                    <SelectItem key={et.name} value={et.name}>
                      {et.display_name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>

          <div className='space-y-1.5'>
            <Label htmlFor='entity-title' className='text-xs'>
              Title
            </Label>
            <Input
              id='entity-title'
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleSubmit()
                if (e.key === 'Escape') onOpenChange(false)
              }}
              placeholder='Title…'
              className='h-9 text-sm'
              autoFocus
            />
          </div>
        </div>

        <DialogFooter>
          <Button
            variant='ghost'
            onClick={() => onOpenChange(false)}
            className='text-sm'>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={!selectedType || !title.trim() || create.isPending}
            className='text-sm'>
            {create.isPending ? 'Creating…' : 'Create'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
