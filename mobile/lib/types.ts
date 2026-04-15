/**
 * OpenLoom API type definitions — mirrors Go structs from internal/.
 * All timestamps are RFC3339 strings unless noted.
 */

// --- Memory ---

export interface Memory {
  id: string;
  path: string;
  l0: string; // one-liner summary
  l1: string; // paragraph (used for embedding)
  l2: string; // full content
  type: MemoryType;
  tags: string[];
  source_agent: string;
  source_node: string;
  scope: MemoryScope;
  project: string;
  domain: string;
  created_at: string;
  updated_at: string;
  accessed_at: string;
  access_count: number;
}

export type MemoryType =
  | "insight"
  | "decision"
  | "pattern"
  | "bug"
  | "context"
  | "reference"
  | "visceral";

export type MemoryScope = "personal" | "project" | "team" | "global";

export interface MemorySearchRequest {
  query: string;
  scope?: string;
  project?: string;
  domain?: string;
  limit?: number;
}

export interface MemorySearchResult {
  distance: number;
  score: number;
  memory: Memory;
}

export interface TreeNode {
  name: string;
  path: string;
  children?: TreeNode[];
  memory?: Memory;
}

// --- Conversation ---

export interface Conversation {
  id: string;
  title: string;
  summary: string;
  agent: string;
  project: string;
  tags: string[];
  turns: Turn[];
  turn_count: number;
  source_node: string;
  created_at: string;
  updated_at: string;
  accessed_at: string;
  access_count: number;
}

export interface Turn {
  role: "user" | "assistant";
  content: string;
  model?: string;
}

export interface ConversationSearchRequest {
  query: string;
  agent?: string;
  project?: string;
  limit?: number;
}

export interface ConversationSearchResult {
  conversation: Conversation;
  distance: number;
  score: number;
}

// --- Manifest ---

export interface Manifest {
  id: string;
  marker: string;
  title: string;
  description: string;
  content: string; // full markdown spec
  status: ManifestStatus;
  jira_refs: string[];
  tags: string[];
  author: string;
  source_node: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export type ManifestStatus = "draft" | "active" | "completed" | "archived";

export interface ManifestCreateRequest {
  title: string;
  description?: string;
  content?: string;
  status?: ManifestStatus;
  jira_refs?: string[];
  tags?: string[];
}

export interface ManifestUpdateRequest {
  title?: string;
  description?: string;
  content?: string;
  status?: ManifestStatus;
  jira_refs?: string[];
  tags?: string[];
}

export interface ManifestSearchRequest {
  query: string;
}

// --- Idea ---

export interface Idea {
  id: string;
  marker: string;
  title: string;
  description: string;
  status: IdeaStatus;
  priority: IdeaPriority;
  tags: string[];
  author: string;
  source_node: string;
  created_at: string;
  updated_at: string;
}

export type IdeaStatus = "new" | "planned" | "in-progress" | "done" | "rejected";
export type IdeaPriority = "low" | "medium" | "high" | "critical";

export interface IdeaCreateRequest {
  title: string;
  description?: string;
  priority?: IdeaPriority;
  tags?: string[];
}

// --- Task ---

export interface Task {
  id: string;
  marker: string;
  manifest_id: string;
  title: string;
  description: string;
  schedule: string; // "once", "5m", "1h", cron expression
  status: TaskStatus;
  agent: string;
  source_node: string;
  created_by: string;
  max_turns: number;
  depends_on: string;
  run_count: number;
  last_run_at: string;
  next_run_at: string;
  last_output: string;
  created_at: string;
  updated_at: string;
}

export type TaskStatus =
  | "pending"
  | "scheduled"
  | "running"
  | "completed"
  | "failed"
  | "cancelled";

export interface TaskCreateRequest {
  manifest_id: string;
  title: string;
  description?: string;
  schedule?: string;
  agent?: string;
  max_turns?: number;
  depends_on?: string;
}

export interface TaskRun {
  id: number;
  task_id: string;
  run_number: number;
  output: string;
  status: string;
  actions: number;
  lines: number;
  started_at: string;
  completed_at: string;
}

export interface TaskStartRequest {
  schedule?: string;
}

export interface TaskOutputResponse {
  lines: string[];
  running: boolean;
}

// --- Action ---

export interface Action {
  id: string;
  session_id: string;
  source_node: string;
  task_id: string;
  tool_name: string;
  tool_input: string;
  tool_response: string;
  cwd: string;
  created_at: string;
}

// --- Amnesia (visceral rule violation) ---

export interface Amnesia {
  id: number;
  session_id: string;
  source_node: string;
  action_id: string;
  task_id: string;
  rule_id: string;
  rule_marker: string;
  rule_text: string;
  tool_name: string;
  tool_input: string;
  score: number;
  match_type: "similarity" | "forbidden_pattern";
  matched_pattern: string;
  status: "flagged" | "confirmed" | "dismissed";
  created_at: string;
}

// --- Delusion (manifest deviation) ---

export interface Delusion {
  id: number;
  session_id: string;
  source_node: string;
  action_id: string;
  task_id: string;
  manifest_id: string;
  manifest_marker: string;
  manifest_title: string;
  tool_name: string;
  tool_input: string;
  score: number;
  reason: string;
  status: "flagged" | "confirmed" | "dismissed";
  created_at: string;
}

// --- Marker ---

export interface Marker {
  id: string;
  target_id: string;
  target_type: "memory" | "conversation";
  target_path: string;
  from_node: string;
  to_node: string;
  message: string;
  priority: "normal" | "high" | "urgent";
  status: "pending" | "seen" | "done";
  created_at: string;
  seen_at?: string;
  done_at?: string;
}

// --- Visceral ---

export interface VisceralConfirmation {
  id: number;
  session_id: string;
  rules_count: number;
  created_at: string;
}

export interface RulePattern {
  id: number;
  rule_id: string;
  polarity: "prohibition" | "instruction" | "permission";
  required_patterns: string[];
  forbidden_patterns: string[];
  auto_extracted: boolean;
  created_at: string;
  updated_at: string;
}

export interface RulePatternUpdateRequest {
  required_patterns?: string[];
  forbidden_patterns?: string[];
}

// --- Peer ---

export interface PeerInfo {
  node_id: string;
  ip: string;
  port: number;
  memories: number;
  last_sync: string;
  sync_count: number;
  first_seen: string;
  status: "connected" | "syncing" | "stale";
}

export interface PeersResponse {
  peers: PeerInfo[];
  local_node: string;
}

// --- Status ---

export interface StatusResponse {
  node: string;
  display_name: string;
  email: string;
  avatar: string;
  memories: number;
  conversations: number;
  markers: number;
  sessions: number;
  peers: number;
  uptime: string;
  embedding: string;
}

// --- Settings ---

export interface ProfileResponse {
  uuid: string;
  display_name: string;
  email: string;
  avatar: string;
}

export interface ProfileUpdateRequest {
  display_name: string;
  email: string;
  avatar: string;
}

export interface AgentInfo {
  id: string;
  name: string;
  connected: boolean;
}

// --- Recall ---

export interface RecallItem {
  id: string;
  marker: string;
  type: "memory" | "manifest" | "idea" | "task";
  title: string;
}

// --- Link ---

export interface LinkRequest {
  idea_id: string;
  manifest_id: string;
}

// --- P2P Sync ---

export type SyncEntityType = "manifest" | "idea" | "visceral";
export type SyncOperation = "create" | "update" | "delete";

export interface SyncStatus {
  lastSyncAt: string | null;
  pendingCount: number;
  syncing: boolean;
  lastError: string | null;
  entityCounts: Record<SyncEntityType, number>;
}

export interface SyncResult {
  pushed: number;
  pulled: number;
  conflicts: number;
  errors: string[];
}

export interface PeerRegistration {
  node_id: string;
  display_name: string;
  device_type: "mobile" | "desktop";
  platform: string;
}

export interface SyncPullResponse<T> {
  items: T[];
  total: number;
  sync_token: string;
}

// --- WebSocket Events ---

export type WSEventType =
  | "memory_stored"
  | "peer_joined"
  | "peer_left"
  | "task_failed"
  | "task_completed"
  | "task_started"
  | "amnesia_detected"
  | "delusion_detected"
  | "sync_requested"
  | "entity_changed";

export interface WSEvent<T = unknown> {
  event: WSEventType;
  data: T;
}

export interface WSMemoryStored {
  id: string;
}

export interface WSPeerJoined {
  node_id: string;
  ip: string;
  port: number;
}

export interface WSPeerLeft {
  node_id: string;
}

export interface WSTaskFailed {
  task_id: string;
  error: string;
}

export interface WSTaskCompleted {
  task_id: string;
  output: string;
}

export interface WSSyncRequested {
  from_node: string;
  entity_type: SyncEntityType;
}

export interface WSTaskStarted {
  task_id: string;
  title: string;
}

export interface WSAmnesiaDetected {
  amnesia_id: number;
  task_id: string;
  rule_text: string;
  tool_name: string;
  score: number;
}

export interface WSDelusionDetected {
  delusion_id: number;
  task_id: string;
  manifest_title: string;
  reason: string;
  tool_name: string;
  score: number;
}

export interface WSEntityChanged {
  entity_type: SyncEntityType;
  entity_id: string;
  operation: SyncOperation;
  source_node: string;
}

// --- Generic API types ---

export interface ApiError {
  error: string;
}

/** Grouped-by-peer response shape: { [source_node]: T[] } */
export type ByPeerResponse<T> = Record<string, T[]>;
