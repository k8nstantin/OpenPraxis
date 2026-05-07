// Portal V2 type extension layer.
//
// Go-backed shapes come from `@/lib/api/generated.ts` (emitted by tygo,
// see Makefile `types` target). Do not hand-edit those — re-run `make types`.
// This file re-exports the generated types and adds frontend-only extensions.

import type {
  Entity as GeneratedEntity,
} from '@/lib/api/generated'

// ── Re-export Go-generated types ──────────────────────────────────────
export type { Row as ExecutionRow, Action } from '@/lib/api/generated'

// ── Frontend-only narrowed unions ─────────────────────────────────────
export type EntityType = 'skill' | 'product' | 'manifest' | 'task' | 'idea'

export type EntityStatus =
  | 'draft'
  | 'active'
  | 'closed'
  | 'archived'
  | string

export type CommentType =
  | 'prompt'
  | 'comment'
  | string

// ── Entity with frontend extensions ───────────────────────────────────
// Narrows `type` to EntityType and adds aggregate stats fields the API
// attaches when fetching product/manifest summaries.
export interface Entity extends Omit<GeneratedEntity, 'type'> {
  type: EntityType
  total_manifests?: number
  total_tasks?: number
  total_turns?: number
  total_actions?: number
  total_tokens?: number
}

// ── Frontend-only types (no Go equivalent) ────────────────────────────
export interface OutputChunk {
  id: string
  run_uid: string
  seq: number
  chunk: string
  created_at: string
}

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

// ── Legacy aliases (backward compat) ─────────────────────────────────
export type Product = Entity
export type Manifest = Entity
export type Task = Entity
export type Idea = Entity
