import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { useEntityTypes, type EntityType } from '@/lib/queries/entity-types'

async function createEntityType(body: { name: string; display_name: string; description: string; color: string; icon: string }) {
  const res = await fetch('/api/entity-types', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text)
  }
  return res.json()
}

async function updateEntityType(name: string, body: { display_name?: string; description?: string; color?: string; icon?: string; new_name?: string }) {
  const res = await fetch(`/api/entity-types/${encodeURIComponent(name)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text)
  }
  return res.json()
}

function TypeRow({ et }: { et: EntityType }) {
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [displayName, setDisplayName] = useState(et.display_name)
  const [description, setDescription] = useState(et.description)
  const [color, setColor] = useState(et.color)
  const [icon, setIcon] = useState(et.icon)

  const update = useMutation({
    mutationFn: () => updateEntityType(et.name, { display_name: displayName, description, color, icon }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['entity-types'] }); setEditing(false) },
  })

  return (
    <div className='flex items-start gap-3 py-2 border-b border-white/5 last:border-0'>
      <div
        className='mt-1 h-4 w-4 rounded-full shrink-0 border border-white/20'
        style={{ backgroundColor: et.color }}
      />
      <div className='flex-1 min-w-0'>
        {editing ? (
          <div className='space-y-2'>
            <div className='grid grid-cols-2 gap-2'>
              <div>
                <label className='text-[10px] text-muted-foreground uppercase tracking-wide'>Display Name</label>
                <Input value={displayName} onChange={e => setDisplayName(e.target.value)} className='h-7 text-xs mt-0.5' />
              </div>
              <div>
                <label className='text-[10px] text-muted-foreground uppercase tracking-wide'>Icon (lucide name)</label>
                <Input value={icon} onChange={e => setIcon(e.target.value)} className='h-7 text-xs mt-0.5' />
              </div>
              <div>
                <label className='text-[10px] text-muted-foreground uppercase tracking-wide'>Color (hex)</label>
                <div className='flex items-center gap-1 mt-0.5'>
                  <input type='color' value={color} onChange={e => setColor(e.target.value)} className='h-7 w-10 rounded cursor-pointer bg-transparent border border-white/20' />
                  <Input value={color} onChange={e => setColor(e.target.value)} className='h-7 text-xs flex-1 font-mono' />
                </div>
              </div>
              <div>
                <label className='text-[10px] text-muted-foreground uppercase tracking-wide'>Description</label>
                <Input value={description} onChange={e => setDescription(e.target.value)} className='h-7 text-xs mt-0.5' />
              </div>
            </div>
            {update.isError && <p className='text-xs text-rose-400'>{String(update.error)}</p>}
            <div className='flex gap-2'>
              <Button size='sm' className='h-7 text-xs' onClick={() => update.mutate()} disabled={update.isPending}>
                {update.isPending ? 'Saving…' : 'Save'}
              </Button>
              <Button size='sm' variant='ghost' className='h-7 text-xs' onClick={() => setEditing(false)}>Cancel</Button>
            </div>
          </div>
        ) : (
          <div>
            <div className='flex items-center gap-2'>
              <code className='text-xs font-mono font-medium'>{et.name}</code>
              <span className='text-xs text-muted-foreground'>— {et.display_name}</span>
            </div>
            {et.description && <p className='text-xs text-muted-foreground mt-0.5'>{et.description}</p>}
            <p className='text-[10px] text-muted-foreground font-mono mt-0.5'>icon: {et.icon} · color: {et.color}</p>
          </div>
        )}
      </div>
      {!editing && (
        <Button size='sm' variant='ghost' className='h-7 text-xs shrink-0' onClick={() => setEditing(true)}>Edit</Button>
      )}
    </div>
  )
}

export function SettingsEntityTypes() {
  const qc = useQueryClient()
  const { data: types, isLoading } = useEntityTypes()
  const [name, setName] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [description, setDescription] = useState('')
  const [color, setColor] = useState('#6366f1')
  const [icon, setIcon] = useState('Database')

  const create = useMutation({
    mutationFn: () => createEntityType({ name: name.trim(), display_name: displayName.trim() || name.trim(), description, color, icon }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['entity-types'] })
      setName(''); setDisplayName(''); setDescription(''); setColor('#6366f1'); setIcon('Database')
    },
  })

  return (
    <div className='space-y-6 w-full'>
      {/* Existing types */}
      <Card>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm font-semibold uppercase tracking-wide text-muted-foreground'>Entity Types</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className='space-y-2'>{[1,2,3].map(i => <Skeleton key={i} className='h-8 w-full' />)}</div>
          ) : (
            <div>
              {(types ?? []).map(et => <TypeRow key={et.type_uid} et={et} />)}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Add new type */}
      <Card>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm font-semibold uppercase tracking-wide text-muted-foreground'>Add Entity Type</CardTitle>
        </CardHeader>
        <CardContent className='space-y-3'>
          <div className='grid grid-cols-2 gap-3'>
            <div>
              <label className='text-xs text-muted-foreground'>Name (internal key)</label>
              <Input value={name} onChange={e => setName(e.target.value)} placeholder='e.g. pipeline' className='mt-1 h-8 text-sm font-mono' />
            </div>
            <div>
              <label className='text-xs text-muted-foreground'>Display Name</label>
              <Input value={displayName} onChange={e => setDisplayName(e.target.value)} placeholder='e.g. Pipeline' className='mt-1 h-8 text-sm' />
            </div>
            <div>
              <label className='text-xs text-muted-foreground'>Color</label>
              <div className='flex items-center gap-2 mt-1'>
                <input type='color' value={color} onChange={e => setColor(e.target.value)} className='h-8 w-10 rounded cursor-pointer bg-transparent border border-white/20' />
                <Input value={color} onChange={e => setColor(e.target.value)} className='h-8 text-sm font-mono flex-1' />
              </div>
            </div>
            <div>
              <label className='text-xs text-muted-foreground'>Icon (lucide name)</label>
              <Input value={icon} onChange={e => setIcon(e.target.value)} placeholder='e.g. GitBranch' className='mt-1 h-8 text-sm' />
            </div>
            <div className='col-span-2'>
              <label className='text-xs text-muted-foreground'>Description</label>
              <Input value={description} onChange={e => setDescription(e.target.value)} placeholder='What is this type for?' className='mt-1 h-8 text-sm' />
            </div>
          </div>
          {create.isError && <p className='text-xs text-rose-400'>{String(create.error)}</p>}
          <Button size='sm' onClick={() => create.mutate()} disabled={!name.trim() || create.isPending}>
            {create.isPending ? 'Creating…' : 'Add Type'}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
