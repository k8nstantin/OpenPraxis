import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

// Groups for display — mirrors catalog.go order
const GROUPS: Record<string, string[]> = {
  'Execution Controls': ['max_parallel','max_turns','timeout_minutes','temperature','reasoning_effort','default_agent','default_model','retry_on_failure','dag_chain_recovery_window_minutes','approval_mode','auto_push_on_complete'],
  'Branch & Git': ['branch_prefix','branch_remote','branch_strategy','worktree_base_dir'],
  'Prompt Context': ['prompt_max_comment_chars','prompt_max_context_pct','prompt_prior_runs_limit','prompt_prior_comments_limit','prompt_build_timeout_seconds'],
  'Frontier & Scoring': ['frontier_window_days'],
  'Proposer Loop': ['proposer_enabled','proposer_trigger_failure_streak','proposer_trigger_cost_usd','proposer_min_pass_rate_delta','proposer_max_candidates'],
}

const TEMPLATE_SECTIONS = ['preamble','visceral_rules','manifest_spec','prior_context','task','instructions','git_workflow','closing_protocol']

interface KnobDef { key: string; type: string; default: any; description: string; enum_values?: string[]; unit?: string }
interface Entry { key: string; value: string; updated_at_iso?: string }

function fetchJSON<T>(path: string): Promise<T> {
  return fetch(path).then(r => r.json())
}

export function SettingsSystem() {
  const qc = useQueryClient()

  // Catalog
  const catalog = useQuery({
    queryKey: ['settings-catalog'],
    queryFn: () => fetchJSON<{ knobs: KnobDef[] }>('/api/settings/catalog').then(d => d.knobs ?? []),
    staleTime: 60_000,
  })

  // System-scope entries
  const entries = useQuery({
    queryKey: ['settings-system'],
    queryFn: () => fetchJSON<{ entries: Entry[] }>('/api/settings/system').then(d => d.entries ?? []),
    staleTime: 10_000,
  })

  const knobMap = new Map((catalog.data ?? []).map(k => [k.key, k]))
  const entryMap = new Map((entries.data ?? []).map(e => [e.key, e]))

  // Save a knob
  const save = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      fetch('/api/settings/system', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ [key]: JSON.parse(value) }),
      }).then(r => r.json()),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['settings-system'] }),
  })

  // Delete (reset to system default)
  const reset = useMutation({
    mutationFn: (key: string) =>
      fetch(`/api/settings/system/${key}`, { method: 'DELETE' }).then(r => r.json()),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['settings-system'] }),
  })

  // Templates
  const [selectedSection, setSelectedSection] = useState('git_workflow')
  const templateBody = useQuery({
    queryKey: ['template-system', selectedSection],
    queryFn: () => fetchJSON<{ body: string }>(`/api/templates?section=${selectedSection}&scope=system`).then(d => d.body ?? ''),
    staleTime: 10_000,
  })
  const [templateEdit, setTemplateEdit] = useState('')
  useEffect(() => { setTemplateEdit(templateBody.data ?? '') }, [templateBody.data])

  const saveTemplate = useMutation({
    mutationFn: () => fetch('/api/templates', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ section: selectedSection, scope: 'system', scope_id: '', body: templateEdit }),
    }).then(r => r.json()),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['template-system', selectedSection] }),
  })

  const resetTemplate = useMutation({
    mutationFn: () => fetch('/api/templates/tombstone', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ section: selectedSection, scope: 'system', scope_id: '' }),
    }).then(r => r.json()),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['template-system', selectedSection] }); setTemplateEdit('') },
  })

  if (catalog.isLoading || entries.isLoading) return (
    <div className='space-y-3 w-full'>
      {[1,2,3].map(i => <Skeleton key={i} className='h-24 w-full' />)}
    </div>
  )

  return (
    <div className='space-y-6 w-full'>
      {/* Knobs by group */}
      {Object.entries(GROUPS).map(([group, keys]) => (
        <Card key={group}>
          <CardHeader className='pb-2'>
            <CardTitle className='text-sm font-semibold uppercase tracking-wide text-muted-foreground'>{group}</CardTitle>
          </CardHeader>
          <CardContent className='space-y-3'>
            {keys.map(key => {
              const knob = knobMap.get(key)
              const entry = entryMap.get(key)
              if (!knob) return null
              const currentRaw = entry ? entry.value : JSON.stringify(knob.default)
              return <KnobRow key={key} knob={knob} currentRaw={currentRaw} hasOverride={!!entry}
                onSave={v => save.mutate({ key, value: v })}
                onReset={() => reset.mutate(key)} />
            })}
          </CardContent>
        </Card>
      ))}

      {/* Template editor */}
      <Card>
        <CardHeader className='pb-2'>
          <CardTitle className='text-sm font-semibold uppercase tracking-wide text-muted-foreground'>Prompt Templates — System Scope</CardTitle>
        </CardHeader>
        <CardContent className='space-y-3'>
          <div className='flex items-center gap-2'>
            <Select value={selectedSection} onValueChange={setSelectedSection}>
              <SelectTrigger className='w-52 h-8 text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TEMPLATE_SECTIONS.map(s => <SelectItem key={s} value={s}>{s}</SelectItem>)}
              </SelectContent>
            </Select>
            <span className='text-xs text-muted-foreground'>system scope · all agents</span>
          </div>
          {templateBody.isLoading ? <Skeleton className='h-40 w-full' /> : (
            <Textarea
              value={templateEdit}
              onChange={e => setTemplateEdit(e.target.value)}
              className='font-mono text-xs min-h-[200px]'
              placeholder='(using built-in default)'
            />
          )}
          <div className='flex gap-2'>
            <Button size='sm' onClick={() => saveTemplate.mutate()} disabled={saveTemplate.isPending}>
              {saveTemplate.isPending ? 'Saving…' : 'Save Override'}
            </Button>
            <Button size='sm' variant='outline' onClick={() => resetTemplate.mutate()} disabled={resetTemplate.isPending}>
              Reset to Default
            </Button>
          </div>
          <p className='text-xs text-muted-foreground'>
            Available variables: {'{{.BranchPrefix}}'} {'{{.BranchRemote}}'} {'{{.Branch}}'} {'{{.Task.ID}}'} {'{{.Task.Title}}'} {'{{.Manifest.Title}}'}
          </p>
        </CardContent>
      </Card>
    </div>
  )
}

function KnobRow({ knob, currentRaw, hasOverride, onSave, onReset }: {
  knob: KnobDef; currentRaw: string; hasOverride: boolean
  onSave: (v: string) => void; onReset: () => void
}) {
  const [editing, setEditing] = useState(false)
  const [val, setVal] = useState(currentRaw)

  let displayVal: string
  try { displayVal = JSON.stringify(JSON.parse(currentRaw)) } catch { displayVal = currentRaw }

  return (
    <div className='flex items-start gap-3 py-1 border-b border-white/5 last:border-0'>
      <div className='flex-1 min-w-0'>
        <div className='flex items-center gap-2 mb-0.5'>
          <code className='text-xs font-mono'>{knob.key}</code>
          {hasOverride && <Badge variant='secondary' className='text-[10px] h-4'>overridden</Badge>}
          {knob.unit && <span className='text-[10px] text-muted-foreground'>{knob.unit}</span>}
        </div>
        <p className='text-xs text-muted-foreground'>{knob.description}</p>
      </div>
      <div className='flex items-center gap-1.5 shrink-0'>
        {editing ? (
          <>
            {knob.enum_values ? (
              <Select value={val.replace(/"/g,'')} onValueChange={v => setVal(JSON.stringify(v))}>
                <SelectTrigger className='h-7 w-36 text-xs'><SelectValue /></SelectTrigger>
                <SelectContent>{knob.enum_values.map(v => <SelectItem key={v} value={v}>{v}</SelectItem>)}</SelectContent>
              </Select>
            ) : (
              <Input value={val.replace(/^"|"$/g,'')} onChange={e => setVal(knob.type === 'string' ? JSON.stringify(e.target.value) : e.target.value)}
                className='h-7 w-36 text-xs font-mono' />
            )}
            <Button size='sm' className='h-7 text-xs px-2' onClick={() => { onSave(val); setEditing(false) }}>Save</Button>
            <Button size='sm' variant='ghost' className='h-7 text-xs px-2' onClick={() => setEditing(false)}>✕</Button>
          </>
        ) : (
          <>
            <code className='text-xs bg-white/5 px-2 py-0.5 rounded font-mono'>{displayVal}</code>
            <Button size='sm' variant='ghost' className='h-7 text-xs px-2' onClick={() => { setVal(currentRaw); setEditing(true) }}>Edit</Button>
            {hasOverride && <Button size='sm' variant='ghost' className='h-7 text-xs px-2 text-muted-foreground' onClick={onReset}>↩</Button>}
          </>
        )}
      </div>
    </div>
  )
}
