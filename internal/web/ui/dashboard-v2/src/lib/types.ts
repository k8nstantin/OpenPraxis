// Hand-written API response types for Portal V2.
//
// Long term these are generated from Go structs via tygo (see Portal A's
// `tools/tygo/config.yaml`). For now we hand-roll the fields each tab
// actually reads — narrowing-by-need keeps the type surface honest and
// keeps drift visible in code review.

export type EntityStatus =
  | 'draft'
  | 'open'
  | 'in_progress'
  | 'closed'
  | 'archived'
  | 'cancelled'
  | string

export interface Product {
  id: string
  marker: string
  title: string
  description?: string
  status: EntityStatus
  tags?: string[]
  source_node?: string
  created_at: string
  updated_at: string
  total_cost?: number
  total_manifests?: number
  total_tasks?: number
  total_turns?: number
  total_actions?: number
  total_tokens?: number
}

export interface Manifest {
  id: string
  marker: string
  title: string
  description?: string
  status: EntityStatus
  project_id?: string
  tags?: string[]
  source_node?: string
  created_at: string
  updated_at: string
  total_tasks?: number
  total_turns?: number
  total_cost?: number
  total_actions?: number
  total_tokens?: number
}

export interface Task {
  id: string
  marker: string
  manifest_id?: string
  title: string
  description?: string
  description_html?: string
  status: EntityStatus
  agent?: string
  schedule?: string
  depends_on?: string
  block_reason?: string
  source_node?: string
  tags?: string[]
  created_at: string
  updated_at: string
  total_cost?: number
  total_turns?: number
  total_actions?: number
  total_tokens?: number
}

export interface Idea {
  id: string
  marker: string
  title: string
  description?: string
  status?: string
  source_node?: string
  created_at: string
  updated_at: string
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
  marker: string
  title: string
  status: EntityStatus
}

// Hierarchy endpoint response — recursive shape for breadcrumb building.
export interface HierarchyNode {
  id: string
  marker: string
  title: string
  type: 'product' | 'manifest' | 'task'
  status: EntityStatus
  meta?: Record<string, unknown>
  children?: HierarchyNode[]
  sub_products?: HierarchyNode[]
}
