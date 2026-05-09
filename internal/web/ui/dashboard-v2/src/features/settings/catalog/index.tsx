import { useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ContentSection } from '../components/content-section'

type KnobType = 'int' | 'float' | 'enum' | 'string' | 'multiselect' | 'bool'

interface KnobDef {
  key: string
  type: KnobType
  default: unknown
  description?: string
  unit?: string
  slider_min?: number
  slider_max?: number
  slider_step?: number
  enum_values?: string[]
}

const GROUPS: { label: string; prefix: string[] }[] = [
  { label: 'Execution Controls', prefix: ['max_parallel', 'max_turns', 'timeout_minutes', 'temperature', 'reasoning_effort', 'default_agent', 'default_model', 'retry_on_failure', 'approval_mode', 'allowed_tools'] },
  { label: 'Prompt Context', prefix: ['prompt_max_comment_chars', 'prompt_max_context_pct', 'compliance_checks_enabled'] },
  { label: 'Scheduler / Runner', prefix: ['scheduler_tick_seconds', 'host_sampler_tick_seconds', 'on_restart_behavior'] },
  { label: 'Branch / Worktree', prefix: ['branch_prefix', 'branch_strategy', 'branch_remote', 'worktree_base_dir'] },
  { label: 'Frontend Flags', prefix: ['frontend_dashboard_v2', 'frontend_dev_mode', 'frontend_dashboard_v2_products', 'frontend_dashboard_v2_manifests', 'frontend_dashboard_v2_tasks', 'frontend_dashboard_v2_memories', 'frontend_dashboard_v2_conversations', 'frontend_dashboard_v2_settings', 'frontend_dashboard_v2_compliance', 'frontend_dashboard_v2_overview'] },
  { label: 'Comment Attachments', prefix: ['comment_attachment_max_mb', 'comment_attachment_allowed_mimes'] },
]

function typeColor(t: KnobType): string {
  switch (t) {
    case 'int': return 'bg-blue-500/10 text-blue-400 border-blue-500/20'
    case 'float': return 'bg-purple-500/10 text-purple-400 border-purple-500/20'
    case 'enum': return 'bg-amber-500/10 text-amber-400 border-amber-500/20'
    case 'multiselect': return 'bg-green-500/10 text-green-400 border-green-500/20'
    case 'string': return 'bg-zinc-500/10 text-zinc-400 border-zinc-500/20'
    default: return 'bg-zinc-500/10 text-zinc-400 border-zinc-500/20'
  }
}

function defaultDisplay(knob: KnobDef): string {
  const d = knob.default
  if (Array.isArray(d)) return `[${(d as string[]).slice(0, 3).join(', ')}${d.length > 3 ? ', …' : ''}]`
  if (d === '' || d === null || d === undefined) return '(agent default)'
  return String(d) + (knob.unit ? ` ${knob.unit}` : '')
}

function KnobCard({ knob }: { knob: KnobDef }) {
  return (
    <div className='flex flex-col gap-1 border-b px-3 py-2.5 last:border-b-0'>
      <div className='flex items-center gap-2 flex-wrap'>
        <code className='font-mono text-sm font-semibold'>{knob.key}</code>
        <Badge variant='outline' className={`text-[10px] px-1.5 py-0 ${typeColor(knob.type)}`}>
          {knob.type}
        </Badge>
        {knob.unit ? (
          <span className='text-xs text-muted-foreground'>{knob.unit}</span>
        ) : null}
        <span className='ml-auto text-xs text-muted-foreground font-mono'>
          default: {defaultDisplay(knob)}
        </span>
      </div>
      {knob.description ? (
        <p className='text-xs text-muted-foreground leading-relaxed'>{knob.description}</p>
      ) : null}
      {knob.type === 'enum' && knob.enum_values ? (
        <div className='flex flex-wrap gap-1 mt-0.5'>
          {knob.enum_values.filter(v => v !== '').map(v => (
            <code key={v} className='text-[10px] bg-muted px-1.5 py-0.5 rounded'>
              {v}
            </code>
          ))}
          {knob.enum_values.includes('') ? (
            <code className='text-[10px] bg-muted px-1.5 py-0.5 rounded text-muted-foreground italic'>
              (empty = agent default)
            </code>
          ) : null}
        </div>
      ) : null}
      {(knob.type === 'int' || knob.type === 'float') && knob.slider_min !== undefined ? (
        <div className='text-[11px] text-muted-foreground'>
          range: {knob.slider_min} – {knob.slider_max}
          {knob.slider_step !== undefined ? ` · step ${knob.slider_step}` : ''}
        </div>
      ) : null}
    </div>
  )
}

function groupKnobs(catalog: KnobDef[]): { label: string; knobs: KnobDef[] }[] {
  const byKey = new Map(catalog.map(k => [k.key, k]))
  const used = new Set<string>()
  const groups: { label: string; knobs: KnobDef[] }[] = []

  for (const g of GROUPS) {
    const knobs = g.prefix
      .map(k => byKey.get(k))
      .filter((k): k is KnobDef => k !== undefined)
    knobs.forEach(k => used.add(k.key))
    if (knobs.length > 0) groups.push({ label: g.label, knobs })
  }

  const remaining = catalog.filter(k => !used.has(k.key))
  if (remaining.length > 0) groups.push({ label: 'Other', knobs: remaining })

  return groups
}

export function SettingsCatalog() {
  const [catalog, setCatalog] = useState<KnobDef[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    fetch('/api/settings/catalog')
      .then(r => {
        if (!r.ok) throw new Error(`catalog ${r.status}`)
        return r.json()
      })
      .then(d => setCatalog((d?.knobs ?? []) as KnobDef[]))
      .catch(e => setError(String(e)))
  }, [])

  if (error) return (
    <ContentSection title='Catalog' desc='System-default values for all settings knobs.'>
      <div className='text-sm text-rose-400'>Failed to load catalog: {error}</div>
    </ContentSection>
  )

  if (!catalog) return (
    <ContentSection title='Catalog' desc='System-default values for all settings knobs.'>
      <div className='space-y-3'>
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className='h-28 w-full' />
        ))}
      </div>
    </ContentSection>
  )

  const groups = groupKnobs(catalog)

  return (
    <ContentSection
      title='Catalog'
      desc={`System-default values for all ${catalog.length} settings knobs. Read-only — override at product, manifest, or task scope.`}
    >
      <div className='space-y-4'>
        {groups.map(g => (
          <Card key={g.label}>
            <CardHeader className='px-3 py-2 pb-0'>
              <CardTitle className='text-sm font-semibold text-muted-foreground uppercase tracking-wider'>
                {g.label}
                <span className='ml-2 font-normal normal-case text-xs'>{g.knobs.length} knobs</span>
              </CardTitle>
            </CardHeader>
            <CardContent className='p-0'>
              {g.knobs.map(k => <KnobCard key={k.key} knob={k} />)}
            </CardContent>
          </Card>
        ))}
      </div>
    </ContentSection>
  )
}
