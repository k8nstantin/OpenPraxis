// JSON shapes mirrored from the Go API. Kept narrow to what the migrated
// tabs actually read — fields the legacy UI ignores are omitted here too.
// Tab manifests should add fields as they migrate.

export interface PeerProductGroup {
  peer_id: string;
  count: number;
  products: Product[];
}

export interface Product {
  id: string;
  marker: string;
  title: string;
  description?: string;
  description_html?: string;
  status: string;
  tags?: string[];
  source_node?: string;
  total_manifests?: number;
  total_tasks?: number;
  total_turns?: number;
  total_cost?: number;
  sub_products?: Product[];
  created_at?: string;
  updated_at?: string;
}

export interface Manifest {
  id: string;
  marker: string;
  title: string;
  status: string;
  description?: string;
  description_html?: string;
  project_id?: string;
  total_tasks?: number;
  total_turns?: number;
  total_cost?: number;
  updated_at?: string;
  depends_on?: string;
  meta?: Record<string, unknown>;
  children?: Task[];
  tags?: string[];
}

export interface Idea {
  id: string;
  marker: string;
  title: string;
  description?: string;
  status: string;
  priority?: string;
  project_id?: string;
  created_at?: string;
  updated_at?: string;
}

export interface ProductDep {
  id: string;
  marker: string;
  title: string;
  status: string;
}

export interface ProductDependencies {
  deps: ProductDep[] | null;
  dependents: ProductDep[] | null;
}

export interface CommentEnvelope {
  comments: Comment[] | null;
}

export interface Task {
  id: string;
  marker: string;
  title: string;
  status: string;
  description?: string;
  description_html?: string;
  depends_on?: string;
  manifest_id?: string;
  meta?: Record<string, unknown>;
}

export interface ProductHierarchy {
  id: string;
  marker: string;
  title: string;
  status: string;
  meta?: Record<string, unknown>;
  children?: Manifest[];
  sub_products?: ProductHierarchy[];
}

export interface Comment {
  id: string;
  body?: string;
  body_html?: string;
  author?: string;
  type?: string;
  created_at?: string;
  updated_at?: string;
  deleted_at?: string;
  target_type?: 'product' | 'manifest' | 'task';
  target_id?: string;
}

/** Settings catalog entry mirrored from Go's settings.KnobDef. */
export interface KnobDef {
  key: string;
  type: 'int' | 'float' | 'string' | 'enum' | 'multiselect';
  slider_min?: number;
  slider_max?: number;
  slider_step?: number;
  enum_values?: string[];
  default: unknown;
  description: string;
  unit?: string;
}

/** Per-scope resolved knob from /api/settings/resolve. */
export interface ResolvedKnob {
  key: string;
  value: unknown;
  source: 'task' | 'manifest' | 'product' | 'system';
}
