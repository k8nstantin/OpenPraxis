// Hand-written API response types for Portal V2.
//
// Long term these are generated from Go structs via tygo (see Portal A's
// `tools/tygo/config.yaml`). For now we hand-roll the fields each tab
// actually reads — narrowing-by-need keeps the type surface honest and
// keeps drift visible in code review.

export type EntityType = 'skill' | 'product' | 'manifest' | 'task' | 'idea'

export type EntityStatus =
  | 'draft'
  | 'active'
  | 'closed'
  | 'archived'
  | string

export interface Entity {
  row_id: number
  entity_uid: string
  type: EntityType
  title: string
  status: EntityStatus
  tags: string[]
  valid_from: string
  valid_to: string
  changed_by: string
  change_reason: string
  created_at: string
  // Aggregated stats fields
  total_manifests?: number
  total_tasks?: number
  total_turns?: number
  total_actions?: number
  total_tokens?: number
}

export interface ExecutionRow {
  id: string
  run_uid: string
  entity_uid: string
  event: 'started' | 'sample' | 'completed' | 'failed'
  run_number: number
  trigger: string
  terminal_reason: string
  started_at: number
  completed_at: number
  duration_ms: number
  ttfb_ms: number
  exit_code: number | null
  error: string
  provider: string
  model: string
  agent_runtime: string
  agent_version: string
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_create_tokens: number
  reasoning_tokens: number
  cache_hit_rate_pct: number
  context_window_pct: number
  tokens_per_turn: number
  turns: number
  actions: number
  errors: number
  compactions: number
  lines_added: number
  lines_removed: number
  files_changed: number
  commits: number
  pr_number: number | null
  branch: string
  cpu_pct: number
  rss_mb: number
  peak_cpu_pct: number
  peak_rss_mb: number
  tests_run: number
  tests_passed: number
  tests_failed: number
  session_id: string
  created_at: string
}

export type CommentType =
  | 'execution_review'
  | 'description_revision'
  | 'agent_note'
  | 'user_note'
  | 'watcher_finding'
  | 'decision'
  | 'link'
  | string

export interface Comment {
  id: string
  target_type: 'product' | 'manifest' | 'task' | 'idea'
  target_id: string
  author: string
  type: CommentType
  body: string
  body_html?: string
  created_at: number | string
  created_at_iso?: string
}

export interface ProductDependency {
  id: string
  title: string
  status: EntityStatus
  // Note: id here IS the entity_uid — the dependency endpoints return {id,title,status}
}

// Hierarchy endpoint response — recursive shape for breadcrumb building.
export interface HierarchyNode {
  id: string
  title: string
  type: 'product' | 'manifest' | 'task'
  status: EntityStatus
  meta?: Record<string, unknown>
  children?: HierarchyNode[]
  sub_products?: HierarchyNode[]
}

// Legacy type aliases kept for backward compat with any remaining consumers.
// All three now map to Entity — callers should migrate to Entity directly.
export type Product = Entity
export type Manifest = Entity
export type Task = Entity
export type Idea = Entity
