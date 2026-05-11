import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { Plus } from 'lucide-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { EntityKind } from '@/lib/queries/entity'

const ENTITY_TYPES: { value: EntityKind; label: string; description: string }[] = [
  { value: 'skill',    label: 'Skill',    description: 'Governance rule or coding practice loaded into agent context' },
  { value: 'product',  label: 'Product',  description: 'Top-level product with manifests and tasks' },
  { value: 'manifest', label: 'Manifest', description: 'Spec or plan — owns a set of tasks' },
  { value: 'task',     label: 'Task',     description: 'Atomic execution unit run by an agent' },
  { value: 'idea',     label: 'Idea',     description: 'Unstructured concept or feature request' },
  { value: 'RAG',      label: 'RAG',      description: 'Retrieval-augmented generation knowledge source' },
]

async function createEntity(type: EntityKind, title: string) {
  const res = await fetch('/api/entities', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ type, title, status: 'draft' }),
  })
  if (!res.ok) throw new Error(`create entity → ${res.status}`)
  return res.json()
}

export function AddEntityButton() {
  const [open, setOpen] = useState(false)
  const [type, setType] = useState<EntityKind>('product')
  const [title, setTitle] = useState('')
  const qc = useQueryClient()
  const navigate = useNavigate()

  const create = useMutation({
    mutationFn: () => createEntity(type, title.trim()),
    onSuccess: (entity) => {
      qc.invalidateQueries({ queryKey: ['entity-tree'] })
      setOpen(false)
      setTitle('')
      navigate({
        to: '/entities/$uid',
        params: { uid: entity.entity_uid },
        search: { kind: type, tab: 'main' },
      })
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (title.trim()) create.mutate()
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <button
          type='button'
          className='flex items-center gap-1 rounded px-2 py-1 text-xs text-muted-foreground hover:bg-white/10 hover:text-foreground transition-colors'
          title='Add Entity'
        >
          <Plus className='h-3.5 w-3.5' />
          Add
        </button>
      </DialogTrigger>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>Add Entity</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className='space-y-4 pt-2'>
          <div className='space-y-1.5'>
            <label className='text-xs font-medium text-muted-foreground uppercase tracking-wide'>Type</label>
            <Select value={type} onValueChange={(v) => setType(v as EntityKind)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ENTITY_TYPES.map((t) => (
                  <SelectItem key={t.value} value={t.value}>
                    <div>
                      <div className='font-medium'>{t.label}</div>
                      <div className='text-xs text-muted-foreground'>{t.description}</div>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className='space-y-1.5'>
            <label className='text-xs font-medium text-muted-foreground uppercase tracking-wide'>Title</label>
            <Input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder={`Name this ${type}…`}
              autoFocus
            />
          </div>
          <div className='flex justify-end gap-2'>
            <Button type='button' variant='ghost' size='sm' onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type='submit' size='sm' disabled={!title.trim() || create.isPending}>
              {create.isPending ? 'Creating…' : 'Create'}
            </Button>
          </div>
          {create.isError && (
            <p className='text-xs text-rose-400'>{String(create.error)}</p>
          )}
        </form>
      </DialogContent>
    </Dialog>
  )
}
