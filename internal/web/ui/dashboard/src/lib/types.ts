// JSON shapes mirrored from the Go API. Kept narrow to what the
// Products tab actually reads — fields the legacy UI ignores are
// omitted here too. Add fields as new tabs migrate.

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
  project_id?: string;
  total_tasks?: number;
  total_turns?: number;
  total_cost?: number;
  updated_at?: string;
  depends_on?: string;
  meta?: Record<string, unknown>;
  children?: Task[];
}

export interface Task {
  id: string;
  marker: string;
  title: string;
  status: string;
  depends_on?: string;
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
