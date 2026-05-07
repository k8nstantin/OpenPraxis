import { useQuery } from '@tanstack/react-query'

// Turn-level analytics queries. Backed by:
//   GET /api/entities/{id}/turn-timeline?run_uid=...
//   GET /api/entities/{id}/turn-tools?run_uid=...
//   GET /api/entities/{id}/cost-per-turn?run_uid=...
//   GET /api/stats/turn-activity?since=<hours>

export interface TurnTimelineRow {
  turn: number
  started_at: string
  duration_ms: number
}

export interface TurnToolCount {
  name: string
  count: number
}

export interface TurnToolsRow {
  turn: number
  tools: TurnToolCount[]
}

export interface TurnActivityRow {
  hour: string
  turns: number
}

export interface CostPerTurnRow {
  turn_number: number
  turn_started_at: string
  cost_per_turn_avg: number
  input_tokens: number
  output_tokens: number
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${path} → ${res.status}`)
  return res.json() as Promise<T>
}

export const turnKeys = {
  timeline: (entityId: string, runUid: string) =>
    ['turns', 'timeline', entityId, runUid] as const,
  tools: (entityId: string, runUid: string) =>
    ['turns', 'tools', entityId, runUid] as const,
  cost: (entityId: string, runUid: string) =>
    ['turns', 'cost', entityId, runUid] as const,
  activity: (hours: number) => ['turns', 'activity', hours] as const,
}

export function useTurnTimeline(
  entityId: string | undefined,
  runUid: string | undefined
) {
  return useQuery({
    queryKey: turnKeys.timeline(entityId ?? '', runUid ?? ''),
    queryFn: () =>
      fetchJSON<TurnTimelineRow[]>(
        `/api/entities/${entityId}/turn-timeline?run_uid=${runUid}`
      ),
    enabled: !!entityId && !!runUid,
    staleTime: 15 * 1000,
  })
}

export function useTurnTools(
  entityId: string | undefined,
  runUid: string | undefined
) {
  return useQuery({
    queryKey: turnKeys.tools(entityId ?? '', runUid ?? ''),
    queryFn: () =>
      fetchJSON<TurnToolsRow[]>(
        `/api/entities/${entityId}/turn-tools?run_uid=${runUid}`
      ),
    enabled: !!entityId && !!runUid,
    staleTime: 15 * 1000,
  })
}

export function useCostPerTurn(
  entityId: string | undefined,
  runUid: string | undefined
) {
  return useQuery({
    queryKey: turnKeys.cost(entityId ?? '', runUid ?? ''),
    queryFn: () =>
      fetchJSON<CostPerTurnRow[]>(
        `/api/entities/${entityId}/cost-per-turn?run_uid=${runUid}`
      ),
    enabled: !!entityId && !!runUid,
    staleTime: 15 * 1000,
  })
}

export function useTurnActivity(hours = 24) {
  return useQuery({
    queryKey: turnKeys.activity(hours),
    queryFn: () =>
      fetchJSON<TurnActivityRow[]>(`/api/stats/turn-activity?since=${hours}`),
    staleTime: 60 * 1000,
  })
}
