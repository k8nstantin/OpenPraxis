import { useEffect, useRef, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Gauge } from '@/components/gauge'
import { ContentSection } from '../components/content-section'

// ------ shared types ---------------------------------------------------------

type KnobType = 'int' | 'float' | 'enum' | 'string' | 'multiselect' | 'bool'
type KnobValue = string | number | boolean | string[]
type ScopeType = 'product' | 'manifest' | 'task'

interface KnobDef {
  key: string
  type: KnobType
  default: KnobValue
  description?: string
  unit?: string
  slider_min?: number
  slider_max?: number
  slider_step?: number
  enum_values?: string[]
}

interface ExplicitEntry {
  key: string
  value: string
  updated_at_iso?: string
  updated_by?: string
}

interface EntityResult {
  entity_uid: string
  title: string
  type: string
  status: string
}

// ------ helpers --------------------------------------------------------------

function scopePath(scope: ScopeType): string {
  if (scope === 'product') return '/api/products'
  if (scope === 'task') return '/api/tasks'
  return '/api/manifests'
}

function safeParse(s: string): KnobValue {
  try { return JSON.parse(s) as KnobValue } catch { return s }
}

function numericOr(v: KnobValue, fallback: KnobValue): number {
  if (typeof v === 'number' && Number.isFinite(v)) return v
  if (typeof fallback === 'number' && Number.isFinite(fallback)) return fallback
  return 0
}

// ------ EntityCombobox -------------------------------------------------------

function EntityCombobox({
  scopeType,
  value,
  onChange,
}: {
  scopeType: ScopeType
  value: EntityResult | null
  onChange: (e: EntityResult | null) => void
}) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<EntityResult[]>([])
  const [open, setOpen] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const wrapRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (timer.current) clearTimeout(timer.current)
    if (!query || query.length < 1) {
      setResults([])
      return
    }
    timer.current = setTimeout(() => {
      fetch(`/api/entities/search?q=${encodeURIComponent(query)}&type=${scopeType}`)
        .then(r => r.json())
        .then((d: EntityResult[]) => setResults(Array.isArray(d) ? d.slice(0, 10) : []))
        .catch(() => setResults([]))
    }, 200)
  }, [query, scopeType])

  useEffect(() => {
    setQuery(value ? value.title : '')
  }, [value])

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  const handleSelect = (e: EntityResult) => {
    onChange(e)
    setQuery(e.title)
    setOpen(false)
    setResults([])
  }

  const handleClear = () => {
    onChange(null)
    setQuery('')
    setResults([])
  }

  return (
    <div className='relative w-full max-w-lg' ref={wrapRef}>
      <div className='flex gap-2'>
        <Input
          className='h-8 text-sm'
          placeholder={`Search ${scopeType}s by name or UUID…`}
          value={query}
          onChange={e => { setQuery(e.target.value); setOpen(true) }}
          onFocus={() => query && setOpen(true)}
        />
        {value ? (
          <Button type='button' variant='ghost' size='sm' className='h-8 px-2 text-xs' onClick={handleClear}>
            Clear
          </Button>
        ) : null}
      </div>
      {open && results.length > 0 ? (
        <div className='absolute z-50 mt-1 w-full rounded-md border bg-popover shadow-lg'>
          {results.map(r => (
            <button
              key={r.entity_uid}
              type='button'
              className='flex w-full items-center gap-2 px-3 py-2 text-sm hover:bg-accent text-left'
              onClick={() => handleSelect(r)}
            >
              <span className='flex-1 truncate font-medium'>{r.title}</span>
              <span className='text-xs text-muted-foreground font-mono shrink-0'>
                {r.entity_uid.slice(0, 8)}
              </span>
              <Badge variant='outline' className='text-[10px] px-1 py-0 shrink-0'>
                {r.status}
              </Badge>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  )
}

// ------ KnobRow / NumericKnobCell (scope-aware) ------------------------------

function SaveStatus({ status, title }: { status: 'idle' | 'saving' | 'ok' | 'err'; title: string }) {
  if (status === 'idle') return <span className='w-4' />
  if (status === 'saving') return <span className='text-muted-foreground w-4 text-xs'>…</span>
  if (status === 'ok') return <span className='w-4 text-xs text-emerald-400'>✓</span>
  return <span className='w-4 text-xs text-rose-400' title={title}>✗</span>
}

function Control({ knob, value, onChange }: { knob: KnobDef; value: KnobValue; onChange: (v: KnobValue) => void }) {
  switch (knob.type) {
    case 'int':
    case 'float': {
      const isInt = knob.type === 'int'
      const min = knob.slider_min ?? 0
      const max = knob.slider_max ?? 100
      const step = knob.slider_step ?? (isInt ? 1 : 0.01)
      const num = numericOr(value, knob.default)
      return (
        <div className='flex flex-1 justify-end'>
          <div className='w-44'>
            <Gauge value={num} min={min} max={max} step={step} unit={knob.unit}
              onChange={v => onChange(isInt ? Math.round(v) : v)} />
          </div>
        </div>
      )
    }
    case 'enum': {
      const v = typeof value === 'string' ? value : String(value ?? '')
      const items = (knob.enum_values ?? []).filter(ev => ev !== '')
      return (
        <Select value={v} onValueChange={onChange}>
          <SelectTrigger className='h-8 w-48 text-sm'>
            <SelectValue placeholder='(default)' />
          </SelectTrigger>
          <SelectContent>
            {items.map(ev => <SelectItem key={ev} value={ev}>{ev}</SelectItem>)}
          </SelectContent>
        </Select>
      )
    }
    case 'bool': {
      const v = (value === true || value === 'true') ? 'true' : 'false'
      return (
        <Select value={v} onValueChange={s => onChange(s === 'true')}>
          <SelectTrigger className='h-8 w-32 text-sm'><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value='true'>true</SelectItem>
            <SelectItem value='false'>false</SelectItem>
          </SelectContent>
        </Select>
      )
    }
    case 'multiselect': {
      const arr = Array.isArray(value) ? value : []
      return (
        <Input type='text' className='h-8 max-w-2xl text-sm'
          value={arr.map(String).join(', ')}
          placeholder='comma-separated values'
          onChange={e => onChange(e.target.value.split(',').map(s => s.trim()).filter(Boolean))} />
      )
    }
    default:
      return (
        <Input type='text' className='h-8 max-w-md text-sm'
          value={typeof value === 'string' ? value : ''}
          placeholder={knob.default ? `default: ${knob.default}` : ''}
          onChange={e => onChange(e.target.value)} />
      )
  }
}

function ScopeKnobRow({
  knob, explicit, scopeType, entityId, onChanged,
}: {
  knob: KnobDef
  explicit?: ExplicitEntry
  scopeType: ScopeType
  entityId: string
  onChanged: () => void
}) {
  const isExplicit = !!explicit
  const initial: KnobValue = explicit ? safeParse(explicit.value) : knob.default
  const [value, setValue] = useState<KnobValue>(initial)
  const [status, setStatus] = useState<'idle' | 'saving' | 'ok' | 'err'>('idle')
  const [errMsg, setErrMsg] = useState<string | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => { setValue(initial) }, [explicit?.value, knob.default]) // eslint-disable-line

  const scheduleSave = (next: KnobValue) => {
    setValue(next)
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => doSave(next), 400)
  }

  const doSave = async (next: KnobValue) => {
    setStatus('saving')
    setErrMsg(null)
    try {
      const res = await fetch(`${scopePath(scopeType)}/${entityId}/settings`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ [knob.key]: next }),
      })
      const data = await res.json()
      const result = data?.results?.[0] ?? { ok: false, error: 'no result' }
      if (!result.ok) { setStatus('err'); setErrMsg(result.error || 'save failed'); return }
      setStatus('ok')
      onChanged()
      setTimeout(() => setStatus('idle'), 1500)
    } catch (e) { setStatus('err'); setErrMsg(String(e)) }
  }

  const reset = async () => {
    try {
      const res = await fetch(`${scopePath(scopeType)}/${entityId}/settings/${encodeURIComponent(knob.key)}`, { method: 'DELETE' })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      onChanged()
    } catch (e) { setStatus('err'); setErrMsg(String(e)) }
  }

  return (
    <div className='flex flex-col gap-1.5 p-3 border-b last:border-b-0'>
      <div className='flex items-center gap-3'>
        <code className='font-mono text-sm font-medium' title={knob.description ?? ''}>{knob.key}</code>
        <div className='flex flex-1 items-center gap-2'>
          <Control knob={knob} value={value} onChange={scheduleSave} />
        </div>
        <SaveStatus status={status} title={errMsg ?? ''} />
        {isExplicit ? (
          <Button type='button' variant='ghost' size='sm' className='h-7 px-2 text-xs' onClick={reset} title='Reset to inherited value'>
            Reset
          </Button>
        ) : null}
      </div>
      <div className='flex items-center gap-2 text-xs text-muted-foreground'>
        {isExplicit ? (
          <Badge variant='outline' className='text-[10px] px-1.5 py-0 bg-amber-500/10 text-amber-400 border-amber-500/20'>
            explicit · {scopeType}
          </Badge>
        ) : (
          <Badge variant='outline' className='text-[10px] px-1.5 py-0'>
            inherited / system default
          </Badge>
        )}
        {knob.unit ? <span>{knob.unit}</span> : null}
        {knob.description ? <span className='truncate max-w-sm'>{knob.description}</span> : null}
      </div>
    </div>
  )
}

// ------ ScopeKnobs (full knob list for a selected entity) --------------------

function ScopeKnobs({ catalog, scopeType, entityId }: {
  catalog: KnobDef[]
  scopeType: ScopeType
  entityId: string
}) {
  const [explicit, setExplicit] = useState<Map<string, ExplicitEntry>>(new Map())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const reload = async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetch(`${scopePath(scopeType)}/${entityId}/settings`)
      if (!res.ok) throw new Error(`settings → ${res.status}`)
      const data = await res.json()
      const m = new Map<string, ExplicitEntry>()
      for (const e of (data?.entries ?? []) as ExplicitEntry[]) m.set(e.key, e)
      setExplicit(m)
    } catch (e) { setError(String(e)) }
    finally { setLoading(false) }
  }

  useEffect(() => { reload() }, [scopeType, entityId]) // eslint-disable-line

  if (loading) return <div className='space-y-2 mt-4'>{Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className='h-14 w-full' />)}</div>
  if (error) return <div className='text-sm text-rose-400 mt-4'>Failed to load settings: {error}</div>

  const explicitCount = explicit.size

  return (
    <div className='mt-4 space-y-1'>
      <div className='text-xs text-muted-foreground mb-2'>
        {explicitCount} explicit override{explicitCount !== 1 ? 's' : ''} · {catalog.length - explicitCount} inherited from parent or system default
      </div>
      <Card>
        <CardContent className='p-0'>
          {catalog.map(k => (
            <ScopeKnobRow
              key={k.key}
              knob={k}
              explicit={explicit.get(k.key)}
              scopeType={scopeType}
              entityId={entityId}
              onChanged={reload}
            />
          ))}
        </CardContent>
      </Card>
    </div>
  )
}

// ------ Main export ----------------------------------------------------------

export function SettingsScope() {
  const [scopeType, setScopeType] = useState<ScopeType>('product')
  const [entity, setEntity] = useState<EntityResult | null>(null)
  const [catalog, setCatalog] = useState<KnobDef[] | null>(null)
  const [catalogError, setCatalogError] = useState<string | null>(null)

  useEffect(() => {
    fetch('/api/settings/catalog')
      .then(r => { if (!r.ok) throw new Error(`catalog ${r.status}`); return r.json() })
      .then(d => setCatalog((d?.knobs ?? []) as KnobDef[]))
      .catch(e => setCatalogError(String(e)))
  }, [])

  const handleScopeChange = (s: string) => {
    setScopeType(s as ScopeType)
    setEntity(null)
  }

  return (
    <ContentSection
      title='Scope Editor'
      desc='Override settings at product, manifest, or task scope. Explicit values shadow the inherited chain below.'
    >
      <div className='space-y-3'>
        {catalogError ? (
          <div className='text-sm text-rose-400'>Catalog error: {catalogError}</div>
        ) : null}
        <div className='flex items-center gap-3'>
          <Select value={scopeType} onValueChange={handleScopeChange}>
            <SelectTrigger className='h-8 w-32 text-sm'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='product'>Product</SelectItem>
              <SelectItem value='manifest'>Manifest</SelectItem>
              <SelectItem value='task'>Task</SelectItem>
            </SelectContent>
          </Select>
          <EntityCombobox scopeType={scopeType} value={entity} onChange={setEntity} />
        </div>
        {entity ? (
          <div className='text-xs text-muted-foreground font-mono'>
            {scopeType}/{entity.entity_uid}
          </div>
        ) : (
          <div className='text-xs text-muted-foreground'>
            Select a {scopeType} above to view and edit its settings overrides.
          </div>
        )}
        {entity && catalog ? (
          <ScopeKnobs catalog={catalog} scopeType={scopeType} entityId={entity.entity_uid} />
        ) : null}
        {entity && !catalog ? (
          <Skeleton className='h-10 w-full' />
        ) : null}
      </div>
    </ContentSection>
  )
}
