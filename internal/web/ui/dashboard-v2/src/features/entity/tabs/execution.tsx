import { useEffect, useRef, useState } from 'react'
import type { EntityKind } from '@/lib/queries/entity'
import { Card, CardContent } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Gauge } from '@/components/gauge'

// React port of internal/web/ui/components/knobs.js. Same data shape,
// same control mix (range mirrored to a number input, enum select,
// string text input, multiselect = comma-separated tags), same
// debounced PUT, same DELETE-to-reset flow. Generic over scope (product
// or manifest); the only change is the URL prefix.
//
//   GET    /api/settings/catalog
//   GET    /api/{kind}s/{id}/settings
//   PUT    /api/{kind}s/{id}/settings   body { <key>: <value> }
//   DELETE /api/{kind}s/{id}/settings/{key}

type KnobType = 'int' | 'float' | 'enum' | 'string' | 'multiselect' | 'bool'
type KnobValue = string | number | boolean | string[]

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
  value: string // JSON-encoded
}

function basePath(kind: EntityKind): string {
  return kind === 'product' ? '/api/products' : '/api/manifests'
}

export function ExecutionTab({
  kind,
  entityId,
}: {
  kind: EntityKind
  entityId: string
}) {
  const [catalog, setCatalog] = useState<KnobDef[] | null>(null)
  const [explicit, setExplicit] = useState<Map<string, ExplicitEntry>>(
    new Map()
  )
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const reload = async () => {
    setLoading(true)
    setError(null)
    try {
      const [catRes, expRes] = await Promise.all([
        fetch('/api/settings/catalog'),
        fetch(`${basePath(kind)}/${entityId}/settings`),
      ])
      if (!catRes.ok) throw new Error(`catalog → ${catRes.status}`)
      if (!expRes.ok) throw new Error(`settings → ${expRes.status}`)
      const cat = await catRes.json()
      const exp = await expRes.json()
      setCatalog((cat?.knobs ?? []) as KnobDef[])
      const map = new Map<string, ExplicitEntry>()
      for (const e of (exp?.entries ?? []) as ExplicitEntry[]) {
        map.set(e.key, e)
      }
      setExplicit(map)
    } catch (e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    reload()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kind, entityId])

  if (loading) {
    return (
      <div className='space-y-2'>
        {Array.from({ length: 8 }).map((_, i) => (
          <Skeleton key={i} className='h-14 w-full' />
        ))}
      </div>
    )
  }
  if (error) {
    return (
      <div className='text-sm text-rose-400'>
        Failed to load execution controls: {error}
      </div>
    )
  }
  if (!catalog) return null

  const numericKnobs = catalog.filter(
    (k) => k.type === 'int' || k.type === 'float'
  )
  const otherKnobs = catalog.filter(
    (k) => k.type !== 'int' && k.type !== 'float'
  )

  return (
    <div className='space-y-3'>
      {numericKnobs.length > 0 ? (
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5'>
          {numericKnobs.map((knob) => (
            <NumericKnobCell
              key={knob.key}
              knob={knob}
              explicit={explicit.get(knob.key)}
              kind={kind}
              entityId={entityId}
              onChanged={reload}
            />
          ))}
        </div>
      ) : null}
      {otherKnobs.length > 0 ? (
        <Card>
          <CardContent className='divide-y p-0'>
            {otherKnobs.map((knob) => (
              <KnobRow
                key={knob.key}
                knob={knob}
                explicit={explicit.get(knob.key)}
                kind={kind}
                entityId={entityId}
                onChanged={reload}
              />
            ))}
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}

function NumericKnobCell({
  knob,
  explicit,
  kind,
  entityId,
  onChanged,
}: {
  knob: KnobDef
  explicit?: ExplicitEntry
  kind: EntityKind
  entityId: string
  onChanged: () => void
}) {
  const isExplicit = !!explicit
  const initial: KnobValue = explicit ? safeParse(explicit.value) : knob.default
  const [value, setValue] = useState<KnobValue>(initial)
  const [status, setStatus] = useState<'idle' | 'saving' | 'ok' | 'err'>('idle')
  const [errMsg, setErrMsg] = useState<string | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    setValue(initial)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [explicit?.value, knob.default])

  const isInt = knob.type === 'int'
  const min = knob.slider_min ?? 0
  const max = knob.slider_max ?? 100
  const step = knob.slider_step ?? (isInt ? 1 : 0.01)
  const num = numericOr(value, knob.default)
  const def = numericOr(knob.default, min)
  const range = max - min
  const dev = range > 0 ? Math.abs(num - def) / range : 0
  const tone =
    dev <= 0.1
      ? 'text-emerald-500'
      : dev <= 0.3
        ? 'text-amber-500'
        : 'text-rose-500'

  const scheduleSave = (next: number) => {
    const v = isInt ? Math.round(next) : next
    setValue(v)
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => doSave(v), 400)
  }

  const doSave = async (next: KnobValue) => {
    setStatus('saving')
    setErrMsg(null)
    try {
      const res = await fetch(`${basePath(kind)}/${entityId}/settings`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ [knob.key]: next }),
      })
      const data = await res.json()
      const result = data?.results?.[0] ?? { ok: false, error: 'no result' }
      if (!result.ok) {
        setStatus('err')
        setErrMsg(result.error || 'save failed')
        return
      }
      setStatus('ok')
      onChanged()
      setTimeout(() => setStatus('idle'), 1500)
    } catch (e) {
      setStatus('err')
      setErrMsg(String(e))
    }
  }

  const reset = async () => {
    try {
      const res = await fetch(
        `${basePath(kind)}/${entityId}/settings/${encodeURIComponent(knob.key)}`,
        { method: 'DELETE' }
      )
      if (!res.ok) {
        const txt = await res.text()
        throw new Error(txt || `HTTP ${res.status}`)
      }
      onChanged()
    } catch (e) {
      setStatus('err')
      setErrMsg(String(e))
    }
  }

  return (
    <div className={`flex flex-col items-stretch gap-1 ${tone}`}>
      <Gauge
        label={knob.key}
        value={num}
        min={min}
        max={max}
        step={step}
        unit={knob.unit}
        defaultValue={def}
        onChange={scheduleSave}
      />
      <div className='flex items-center justify-between gap-1 px-1'>
        <SaveStatus status={status} title={errMsg ?? ''} />
        {isExplicit ? (
          <Button
            type='button'
            variant='ghost'
            size='sm'
            className='text-muted-foreground h-6 px-2 text-xs'
            onClick={reset}
            title={`Reset to default (${knob.default})`}
          >
            Reset
          </Button>
        ) : (
          <span />
        )}
      </div>
    </div>
  )
}

function KnobRow({
  knob,
  explicit,
  kind,
  entityId,
  onChanged,
}: {
  knob: KnobDef
  explicit?: ExplicitEntry
  kind: EntityKind
  entityId: string
  onChanged: () => void
}) {
  const isExplicit = !!explicit
  const initial: KnobValue = explicit ? safeParse(explicit.value) : knob.default
  const [value, setValue] = useState<KnobValue>(initial)
  const [status, setStatus] = useState<'idle' | 'saving' | 'ok' | 'err'>('idle')
  const [errMsg, setErrMsg] = useState<string | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    setValue(initial)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [explicit?.value, knob.default])

  const scheduleSave = (next: KnobValue) => {
    setValue(next)
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => doSave(next), 400)
  }

  const doSave = async (next: KnobValue) => {
    setStatus('saving')
    setErrMsg(null)
    try {
      const res = await fetch(`${basePath(kind)}/${entityId}/settings`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ [knob.key]: next }),
      })
      const data = await res.json()
      const result = data?.results?.[0] ?? { ok: false, error: 'no result' }
      if (!result.ok) {
        setStatus('err')
        setErrMsg(result.error || 'save failed')
        return
      }
      setStatus('ok')
      onChanged()
      setTimeout(() => setStatus('idle'), 1500)
    } catch (e) {
      setStatus('err')
      setErrMsg(String(e))
    }
  }

  const reset = async () => {
    try {
      const res = await fetch(
        `${basePath(kind)}/${entityId}/settings/${encodeURIComponent(knob.key)}`,
        { method: 'DELETE' }
      )
      if (!res.ok) {
        const txt = await res.text()
        throw new Error(txt || `HTTP ${res.status}`)
      }
      onChanged()
    } catch (e) {
      setStatus('err')
      setErrMsg(String(e))
    }
  }

  const warnings = computeWarnings(knob, value)
  const noun = kind === 'product' ? 'product' : 'manifest'

  return (
    <div className='flex flex-col gap-2 p-3'>
      <div className='flex items-center gap-3'>
        <code
          className='font-mono text-sm font-medium'
          title={knob.description ?? ''}
        >
          {knob.key}
        </code>
        <div className='flex flex-1 items-center gap-2'>
          <Control knob={knob} value={value} onChange={scheduleSave} />
        </div>
        <SaveStatus status={status} title={errMsg ?? ''} />
        {isExplicit ? (
          <Button
            type='button'
            variant='ghost'
            size='sm'
            className='h-7 px-2 text-xs'
            onClick={reset}
            title='Reset to inherited value'
          >
            Reset
          </Button>
        ) : null}
      </div>
      <div className='text-muted-foreground flex items-center gap-3 text-xs'>
        <span>{isExplicit ? `set at ${noun}` : 'system default'}</span>
        {(knob.type === 'int' || knob.type === 'float') &&
        knob.slider_max !== undefined ? (
          <span>
            system maximum {knob.slider_max}
            {knob.unit ? ` ${knob.unit}` : ''} · default{' '}
            {String(knob.default)}
          </span>
        ) : null}
        {warnings.map((w, i) => (
          <span key={i} className='text-amber-400'>
            {w}
          </span>
        ))}
      </div>
      {knob.description ? (
        <div className='text-muted-foreground text-xs'>{knob.description}</div>
      ) : null}
    </div>
  )
}

function Control({
  knob,
  value,
  onChange,
}: {
  knob: KnobDef
  value: KnobValue
  onChange: (v: KnobValue) => void
}) {
  switch (knob.type) {
    case 'int':
    case 'float':
      return <RangeNumber knob={knob} value={value} onChange={onChange} />
    case 'enum':
      return <EnumSelect knob={knob} value={value} onChange={onChange} />
    case 'bool':
      return <BoolSelect value={value} onChange={onChange} />
    case 'multiselect':
      return <MultiselectInput value={value} onChange={onChange} />
    case 'string':
    default:
      return (
        <Input
          type='text'
          className='h-8 max-w-md text-sm'
          value={typeof value === 'string' ? value : ''}
          placeholder={knob.default ? `default: ${knob.default}` : ''}
          onChange={(e) => onChange(e.target.value)}
        />
      )
  }
}

function RangeNumber({
  knob,
  value,
  onChange,
}: {
  knob: KnobDef
  value: KnobValue
  onChange: (v: KnobValue) => void
}) {
  const isInt = knob.type === 'int'
  const min = knob.slider_min ?? 0
  const max = knob.slider_max ?? 100
  const step = knob.slider_step ?? (isInt ? 1 : 0.01)
  const num = numericOr(value, knob.default)
  return (
    <div className='flex flex-1 justify-end'>
      <div className='w-44'>
        <Gauge
          value={num}
          min={min}
          max={max}
          step={step}
          unit={knob.unit}
          onChange={(v) => onChange(isInt ? Math.round(v) : v)}
        />
      </div>
    </div>
  )
}

function EnumSelect({
  knob,
  value,
  onChange,
}: {
  knob: KnobDef
  value: KnobValue
  onChange: (v: KnobValue) => void
}) {
  const v = typeof value === 'string' ? value : String(value ?? '')
  const items = (knob.enum_values ?? []).filter((ev) => ev !== '')
  return (
    <Select value={v} onValueChange={onChange}>
      <SelectTrigger className='h-8 w-48 text-sm'>
        <SelectValue placeholder='(default)' />
      </SelectTrigger>
      <SelectContent>
        {items.map((ev) => (
          <SelectItem key={ev} value={ev}>
            {ev}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function BoolSelect({
  value,
  onChange,
}: {
  value: KnobValue
  onChange: (v: KnobValue) => void
}) {
  const v = value === true || value === 'true' ? 'true' : 'false'
  return (
    <Select value={v} onValueChange={(s) => onChange(s === 'true')}>
      <SelectTrigger className='h-8 w-32 text-sm'>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value='true'>true</SelectItem>
        <SelectItem value='false'>false</SelectItem>
      </SelectContent>
    </Select>
  )
}

function MultiselectInput({
  value,
  onChange,
}: {
  value: KnobValue
  onChange: (v: KnobValue) => void
}) {
  const arr = Array.isArray(value) ? value : []
  const csv = arr.map(String).join(', ')
  return (
    <Input
      type='text'
      className='h-8 max-w-2xl text-sm'
      value={csv}
      placeholder='comma-separated values'
      onChange={(e) => {
        const next = e.target.value
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean)
        onChange(next)
      }}
    />
  )
}

function SaveStatus({
  status,
  title,
}: {
  status: 'idle' | 'saving' | 'ok' | 'err'
  title: string
}) {
  if (status === 'idle') return <span className='w-4' />
  if (status === 'saving')
    return <span className='text-muted-foreground w-4 text-xs'>…</span>
  if (status === 'ok')
    return <span className='w-4 text-xs text-emerald-400'>✓</span>
  return (
    <span className='w-4 text-xs text-rose-400' title={title}>
      ✗
    </span>
  )
}

function safeParse(s: string): KnobValue {
  try {
    return JSON.parse(s) as KnobValue
  } catch {
    return s
  }
}

function numericOr(v: KnobValue, fallback: KnobValue): number {
  if (typeof v === 'number' && Number.isFinite(v)) return v
  if (typeof fallback === 'number' && Number.isFinite(fallback)) return fallback
  return 0
}

function computeWarnings(knob: KnobDef, value: KnobValue): string[] {
  const out: string[] = []
  if (knob.key === 'max_parallel' && typeof value === 'number') {
    const cores =
      typeof navigator !== 'undefined' ? navigator.hardwareConcurrency || 0 : 0
    if (cores > 0 && value > cores) {
      out.push(`Exceeds CPU count (${cores}); tasks will queue`)
    }
  }
  if (knob.key === 'temperature' && typeof value === 'number' && value > 1.5) {
    out.push('High temperature rarely helps for coding')
  }
  if (
    knob.key === 'daily_budget_usd' &&
    typeof value === 'number' &&
    value > 90
  ) {
    out.push('Within $10 of visceral rule cap ($100)')
  }
  return out
}
