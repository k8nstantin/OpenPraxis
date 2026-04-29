import { useQuery } from '@tanstack/react-query'
import type { EntityKind } from '@/lib/queries/entity'

// Stats tab — react-query hooks for /api/run-stats + /api/system-stats.
// Both honor a URL ?as_of=... if present (point-in-time view).
//
// Backend wires:
//   GET /api/run-stats?entity_kind=...&entity_id=...&as_of=<rfc3339>
//     → { runs: [...task_runs rows...],
//         samples_by_run: { <run_id>: [...task_run_host_samples...] } }
//   GET /api/system-stats?from=<rfc3339>&to=<rfc3339>&as_of=<rfc3339>
//     → { samples: [...system_host_samples...] }

export interface RunRow {
  id: number
  task_id: string
  run_number: number
  output: string
  status: string
  actions: number
  lines: number
  cost_usd: number
  turns: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_create_tokens: number
  model: string
  pricing_version: string
  peak_cpu_pct: number
  avg_cpu_pct: number
  peak_rss_mb: number
  started_at: string
  completed_at: string

  // Stats-tab denorm
  errors: number
  compactions: number
  files_changed: number
  exit_code: number
  cancelled_at: string
  cancelled_by: string
  duration_ms: number
  avg_rss_mb: number
  branch: string
  commit_sha: string
  commits: number
  pr_number: number
  worktree_path: string
  agent_runtime: string
  agent_version: string
  lines_added: number
  lines_removed: number
}

export interface RunHostSample {
  ts: string
  cpu_pct: number
  rss_mb: number
  cost_usd: number
  turns: number
  actions: number
  disk_used_gb: number
  disk_total_gb: number
}

export interface RunStatsResponse {
  runs: RunRow[]
  samples_by_run: Record<string, RunHostSample[]>
}

export interface SystemHostSample {
  ts: string
  cpu_pct: number
  load_1m: number
  load_5m: number
  load_15m: number
  mem_used_mb: number
  mem_total_mb: number
  swap_used_mb: number
  disk_used_gb: number
  disk_total_gb: number
  net_rx_mbps: number
  net_tx_mbps: number
}

export interface SystemStatsResponse {
  samples: SystemHostSample[]
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(`${path} → ${res.status}`)
  return res.json() as Promise<T>
}

// Reads ?as_of=... from the current URL (search OR hash). Used as a
// default for both hooks when the caller doesn't pass one explicitly.
function urlAsOf(): string | undefined {
  if (typeof window === 'undefined') return undefined
  const search = new URLSearchParams(window.location.search)
  const fromSearch = search.get('as_of')
  if (fromSearch) return fromSearch
  const hash = window.location.hash || ''
  const qIdx = hash.indexOf('?')
  if (qIdx === -1) return undefined
  const fromHash = new URLSearchParams(hash.slice(qIdx + 1)).get('as_of')
  return fromHash ?? undefined
}

export const statsKeys = {
  all: ['stats'] as const,
  runs: (kind: EntityKind, id: string, asOf?: string) =>
    [...statsKeys.all, 'runs', kind, id, asOf ?? ''] as const,
  system: (from: string, to: string, asOf?: string) =>
    [...statsKeys.all, 'system', from, to, asOf ?? ''] as const,
}

export function useRunStats(
  kind: EntityKind,
  entityId: string | undefined,
  asOf?: string
) {
  const effectiveAsOf = asOf ?? urlAsOf()
  return useQuery({
    queryKey: statsKeys.runs(kind, entityId ?? '', effectiveAsOf),
    queryFn: () => {
      const params = new URLSearchParams({
        entity_kind: kind,
        entity_id: entityId ?? '',
      })
      if (effectiveAsOf) params.set('as_of', effectiveAsOf)
      return fetchJSON<RunStatsResponse>(`/api/run-stats?${params.toString()}`)
    },
    enabled: !!entityId,
    staleTime: 15 * 1000,
  })
}

export function useSystemStats(
  fromIso: string,
  toIso: string,
  asOf?: string
) {
  const effectiveAsOf = asOf ?? urlAsOf()
  return useQuery({
    queryKey: statsKeys.system(fromIso, toIso, effectiveAsOf),
    queryFn: () => {
      const params = new URLSearchParams({ from: fromIso, to: toIso })
      if (effectiveAsOf) params.set('as_of', effectiveAsOf)
      return fetchJSON<SystemStatsResponse>(
        `/api/system-stats?${params.toString()}`
      )
    },
    refetchInterval: 30 * 1000,
    staleTime: 15 * 1000,
  })
}
