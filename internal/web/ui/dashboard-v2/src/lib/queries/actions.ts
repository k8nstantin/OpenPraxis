import { useQuery } from '@tanstack/react-query'

// Actions Log queries — single canonical endpoint for the audit
// surface. `/api/actions/search` was extended (this PR) to treat
// empty `q` as "browse every action paged" so one hook covers both
// browse and search modes. No client-side filtering — pagination is
// server-side, deterministic, and the returned `total` is the source
// of truth for "how many actions match my query."

export interface ActionRow {
  id: string
  session_id: string
  source_node: string
  task_id: string
  tool_name: string
  tool_input: string
  tool_response: string
  cwd: string
  created_at: string
  // Set when the request had a non-empty q. Pre-rendered <mark>-tagged
  // HTML snippet for display; treat as pre-sanitized server output.
  snippet_html?: string
}

export interface ActionsSearchResponse {
  items: ActionRow[] | null
  total: number
  offset: number
  limit: number
  has_more: boolean
}

export interface UseActionsParams {
  q: string
  offset: number
  limit: number
}

export function useActions(params: UseActionsParams) {
  return useQuery({
    queryKey: ['actions-log', params.q, params.offset, params.limit],
    queryFn: async () => {
      const url = new URL('/api/actions/search', window.location.origin)
      if (params.q) url.searchParams.set('q', params.q)
      url.searchParams.set('limit', String(params.limit))
      url.searchParams.set('offset', String(params.offset))
      const res = await fetch(url.toString().replace(window.location.origin, ''))
      if (!res.ok) throw new Error(`actions/search → ${res.status}`)
      const env = (await res.json()) as ActionsSearchResponse
      if (!env.items) env.items = []
      return env
    },
    // Live-tail behaviour: poll the FIRST page every 2s so new actions
    // surface as they're recorded by the hook. Deeper pages don't poll
    // (the operator is browsing history; refetch would be jarring) — we
    // only auto-refresh at offset 0.
    refetchInterval: params.offset === 0 ? 2000 : false,
    refetchIntervalInBackground: false,
    staleTime: 0,
  })
}
