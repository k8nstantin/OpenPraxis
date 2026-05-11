import { useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { BlockNoteComposer, type BlockNoteComposerHandle } from '@/components/blocknote-composer'
import { BlockNoteReadView } from '@/components/blocknote-read-view'
import { useEntityTypes, type EntityType } from '@/lib/queries/entity-types'

async function createEntityType(body: { name: string; display_name: string; description: string; color: string; icon: string }) {
  const res = await fetch('/api/entity-types', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

async function updateEntityType(name: string, body: { display_name?: string; description?: string; color?: string; icon?: string; new_name?: string }) {
  const res = await fetch(`/api/entity-types/${encodeURIComponent(name)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

function TypeRow({ et }: { et: EntityType }) {
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [viewSource, setViewSource] = useState(false)
  const [displayName, setDisplayName] = useState(et.display_name)
  const [color, setColor] = useState(et.color)
  const [icon, setIcon] = useState(et.icon)
  const descRef = useRef<BlockNoteComposerHandle>(null)

  const update = useMutation({
    mutationFn: async () => {
      const description = descRef.current ? (await descRef.current.getMarkdown()).trim() : et.description
      return updateEntityType(et.name, { display_name: displayName, description, color, icon })
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['entity-types'] }); setEditing(false) },
  })

  return (
    <div className='flex items-start gap-3 py-3 border-b border-white/5 last:border-0'>
      <div
        className='mt-1 h-4 w-4 rounded-full shrink-0 border border-white/20'
        style={{ backgroundColor: et.color }}
      />
      <div className='flex-1 min-w-0'>
        {editing ? (
          <div className='space-y-3'>
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
                <label className='text-[10px] text-muted-foreground uppercase tracking-wide'>Color</label>
                <div className='flex items-center gap-1 mt-0.5'>
                  <input type='color' value={color} onChange={e => setColor(e.target.value)} className='h-7 w-10 rounded cursor-pointer bg-transparent border border-white/20' />
                  <Input value={color} onChange={e => setColor(e.target.value)} className='h-7 text-xs flex-1 font-mono' />
                </div>
              </div>
            </div>
            <div>
              <label className='text-[10px] text-muted-foreground uppercase tracking-wide'>Description</label>
              <div className='rounded-lg border bg-card mt-0.5'>
                <BlockNoteComposer ref={descRef} initialMarkdown={et.description ?? ''} />
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
            <div className='mt-1.5 rounded-lg border bg-card'>
              <div className='flex items-center gap-2 px-3 py-1.5 border-b border-white/5'>
                <span className='text-[10px] font-medium text-muted-foreground uppercase tracking-wide flex-1'>Description</span>
                <button type='button' onClick={() => setViewSource(false)}
                  className={`text-[10px] px-2 py-0.5 rounded transition-colors ${!viewSource ? 'bg-white/10 text-foreground' : 'text-muted-foreground hover:text-foreground'}`}>
                  Rendered
                </button>
                <button type='button' onClick={() => setViewSource(true)}
                  className={`text-[10px] px-2 py-0.5 rounded transition-colors ${viewSource ? 'bg-white/10 text-foreground' : 'text-muted-foreground hover:text-foreground'}`}>
                  Source
                </button>
              </div>
              <div className='px-3 py-2'>
                {viewSource
                  ? <pre className='whitespace-pre-wrap text-xs font-mono text-muted-foreground min-h-[2rem]'>{et.description || '(no description)'}</pre>
                  : et.description
                    ? <BlockNoteReadView markdown={et.description} />
                    : <span className='text-xs text-muted-foreground italic'>No description</span>
                }
              </div>
            </div>
            <p className='text-[10px] text-muted-foreground font-mono mt-1'>icon: {et.icon} · color: {et.color}</p>
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
  const [color, setColor] = useState('#6366f1')
  const [icon, setIcon] = useState('Database')
  const newDescRef = useRef<BlockNoteComposerHandle>(null)

  const create = useMutation({
    mutationFn: async () => {
      const description = newDescRef.current ? (await newDescRef.current.getMarkdown()).trim() : ''
      return createEntityType({ name: name.trim(), display_name: displayName.trim() || name.trim(), description, color, icon })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['entity-types'] })
      setName(''); setDisplayName(''); setColor('#6366f1'); setIcon('Database')
      newDescRef.current?.clear()
    },
  })

  return (
    <div className='space-y-6 w-full'>
      <Card>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm font-semibold uppercase tracking-wide text-muted-foreground'>Entity Types</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className='space-y-2'>{[1,2,3].map(i => <Skeleton key={i} className='h-8 w-full' />)}</div>
          ) : (
            (types ?? []).map(et => <TypeRow key={et.type_uid} et={et} />)
          )}
        </CardContent>
      </Card>

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
          </div>
          <div>
            <label className='text-xs text-muted-foreground'>Description</label>
            <div className='rounded-lg border bg-card mt-1'>
              <BlockNoteComposer ref={newDescRef} initialMarkdown='' />
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
