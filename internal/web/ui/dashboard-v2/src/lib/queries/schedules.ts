import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { EntityKind } from '@/lib/queries/entity'

// Central SCD-2 schedules table — UI hooks. Mirrors the shape of the
// other queries-layer modules:
//   - scheduleKeys: stable cache keys
//   - useSchedules / useScheduleHistory / useScheduleTemplates: reads
//   - useCreateSchedule / useCloseSchedule: mutations
//
// The Schedule tab consumes these directly. Backend wires:
//   GET    /api/schedules?entity_kind=...&entity_id=...
//   GET    /api/schedules?entity_kind=...&entity_id=...&history=true
//   GET    /api/schedules/templates
//   POST   /api/schedules
//   DELETE /api/schedules/{id}?reason=...

export interface Schedule {
  id: number
  entity_kind: EntityKind
  entity_id: string
  run_at: string
  cron_expr: string
  timezone: string
  max_runs: number
  runs_so_far: number
  stop_at: string
  enabled: boolean
  metadata: string

  valid_from: string
  valid_to: string
  created_by: string
  reason: string
  created_at: string
}

export interface ScheduleTemplate {
  key: string
  label: string
  cron: string
}

export interface ScheduleTemplatesResponse {
  templates: ScheduleTemplate[]
}

export const scheduleKeys = {
  all: ['schedules'] as const,
  active: (kind: EntityKind, id: string) =>
    [...scheduleKeys.all, 'active', kind, id] as const,
  history: (kind: EntityKind, id: string) =>
    [...scheduleKeys.all, 'history', kind, id] as const,
  templates: [...['schedules' as const], 'templates'] as const,
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${path} → ${res.status}`)
  return res.json() as Promise<T>
}

// Active schedules for the entity. The list-current API returns active
// rows ordered by run_at ASC; we render the next-fire at the top.
export function useSchedules(
  kind: EntityKind,
  entityId: string | undefined
) {
  return useQuery({
    queryKey: scheduleKeys.active(kind, entityId ?? ''),
    queryFn: () =>
      fetchJSON<Schedule[]>(
        `/api/schedules?entity_kind=${kind}&entity_id=${entityId}`
      ),
    enabled: !!entityId,
    staleTime: 15 * 1000,
  })
}

// History (closed + active) for the audit-trail panel.
export function useScheduleHistory(
  kind: EntityKind,
  entityId: string | undefined
) {
  return useQuery({
    queryKey: scheduleKeys.history(kind, entityId ?? ''),
    queryFn: () =>
      fetchJSON<Schedule[]>(
        `/api/schedules?entity_kind=${kind}&entity_id=${entityId}&history=true`
      ),
    enabled: !!entityId,
    staleTime: 30 * 1000,
  })
}

// Recurrence templates — fed into the "How frequent" dropdown. Cached
// once per session (templates rarely change).
export function useScheduleTemplates() {
  return useQuery({
    queryKey: scheduleKeys.templates,
    queryFn: async () => {
      const res = await fetchJSON<ScheduleTemplatesResponse>(
        '/api/schedules/templates'
      )
      return res.templates
    },
    staleTime: 5 * 60 * 1000,
  })
}

export interface CreateScheduleInput {
  entity_kind: EntityKind
  entity_id: string
  run_at: string
  cron_expr?: string
  timezone?: string
  max_runs?: number
  stop_at?: string
  enabled?: boolean
  metadata?: string
  reason?: string
}

export function useCreateSchedule(
  kind: EntityKind,
  entityId: string | undefined
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: CreateScheduleInput) => {
      const res = await fetch('/api/schedules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!res.ok) {
        const body = await res.text().catch(() => '')
        throw new Error(`create schedule → ${res.status} ${body}`)
      }
      return (await res.json()) as Schedule
    },
    onSuccess: () => {
      if (!entityId) return
      qc.invalidateQueries({ queryKey: scheduleKeys.active(kind, entityId) })
      qc.invalidateQueries({ queryKey: scheduleKeys.history(kind, entityId) })
    },
  })
}

export function useCloseSchedule(
  kind: EntityKind,
  entityId: string | undefined
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: { id: number; reason?: string }) => {
      const url =
        '/api/schedules/' +
        input.id +
        (input.reason
          ? `?reason=${encodeURIComponent(input.reason)}`
          : '')
      const res = await fetch(url, { method: 'DELETE' })
      if (!res.ok) {
        const body = await res.text().catch(() => '')
        throw new Error(`close schedule → ${res.status} ${body}`)
      }
      return (await res.json()) as { ok: boolean; id: number }
    },
    onSuccess: () => {
      if (!entityId) return
      qc.invalidateQueries({ queryKey: scheduleKeys.active(kind, entityId) })
      qc.invalidateQueries({ queryKey: scheduleKeys.history(kind, entityId) })
    },
  })
}
