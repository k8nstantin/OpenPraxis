/**
 * OpenLoom REST API client — fully typed, covers all server endpoints.
 * Base URL is read from Zustand store (peerHost).
 */

import { useAppStore } from "./store";
import type {
  StatusResponse,
  Memory,
  MemorySearchRequest,
  MemorySearchResult,
  TreeNode,
  Conversation,
  ConversationSearchRequest,
  ConversationSearchResult,
  Action,
  Manifest,
  ManifestCreateRequest,
  ManifestSearchRequest,
  ManifestUpdateRequest,
  Idea,
  IdeaCreateRequest,
  Task,
  TaskCreateRequest,
  TaskStartRequest,
  TaskOutputResponse,
  TaskRun,
  Amnesia,
  Delusion,
  Marker,
  VisceralConfirmation,
  RulePattern,
  RulePatternUpdateRequest,
  PeersResponse,
  RecallItem,
  LinkRequest,
  ProfileResponse,
  ProfileUpdateRequest,
  AgentInfo,
  ByPeerResponse,
  PeerRegistration,
} from "./types";

// ---------------------------------------------------------------------------
// Core fetch wrapper
// ---------------------------------------------------------------------------

function getBaseUrl(): string {
  const host = useAppStore.getState().peerHost;
  return `http://${host}`;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/**
 * Generic fetch wrapper with JSON serialization and error handling.
 * Timeout defaults to 10 seconds.
 */
export async function apiFetch<T>(
  path: string,
  options?: RequestInit & { timeout?: number },
): Promise<T> {
  const url = `${getBaseUrl()}${path}`;
  const { timeout = 10_000, ...fetchOpts } = options ?? {};

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeout);

  try {
    const res = await fetch(url, {
      ...fetchOpts,
      signal: controller.signal,
      headers: {
        "Content-Type": "application/json",
        ...fetchOpts.headers,
      },
    });

    if (!res.ok) {
      let msg = res.statusText;
      try {
        const body = await res.json();
        if (body?.error) msg = body.error;
      } catch {
        // keep statusText
      }
      throw new ApiError(res.status, `API ${res.status}: ${msg}`);
    }

    return (await res.json()) as T;
  } finally {
    clearTimeout(timer);
  }
}

/** POST helper with JSON body. */
function post<T>(path: string, body?: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: "POST",
    body: body != null ? JSON.stringify(body) : undefined,
  });
}

/** PUT helper with JSON body. */
function put<T>(path: string, body: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: "PUT",
    body: JSON.stringify(body),
  });
}

/** DELETE helper. */
function del<T>(path: string): Promise<T> {
  return apiFetch<T>(path, { method: "DELETE" });
}

// ---------------------------------------------------------------------------
// API namespace — every server endpoint
// ---------------------------------------------------------------------------

export const api = {
  // --- Status ---
  getStatus: () => apiFetch<StatusResponse>("/api/status"),

  // --- Memories ---
  getMemories: (prefix = "/") =>
    apiFetch<Memory[]>(`/api/memories?prefix=${encodeURIComponent(prefix)}`),
  getMemory: (id: string) => apiFetch<Memory>(`/api/memories/${id}`),
  deleteMemory: (id: string) =>
    del<{ status: string }>(`/api/memories/${id}`),
  searchMemories: (req: MemorySearchRequest) =>
    post<MemorySearchResult[]>("/api/memories/search", req),
  getMemoryTree: () => apiFetch<TreeNode>("/api/memories/tree"),
  getMemoriesBySession: () =>
    apiFetch<ByPeerResponse<Memory>>("/api/memories/by-session"),
  getMemoriesByPeer: () =>
    apiFetch<ByPeerResponse<Memory>>("/api/memories/by-peer"),

  // --- Conversations ---
  getConversations: (status?: string, limit = 20) => {
    const params = new URLSearchParams();
    if (status) params.set("status", status);
    params.set("limit", String(limit));
    return apiFetch<Conversation[]>(`/api/conversations?${params}`);
  },
  getConversation: (id: string) =>
    apiFetch<Conversation>(`/api/conversations/${id}`),
  getConversationsByPeer: () =>
    apiFetch<ByPeerResponse<Conversation>>("/api/conversations/by-peer"),
  searchConversations: (req: ConversationSearchRequest) =>
    post<ConversationSearchResult[]>("/api/conversations/search", req),
  getConversationActions: (id: string) =>
    apiFetch<Action[]>(`/api/conversations/${id}/actions`),

  // --- Manifests ---
  getManifests: (status?: string, limit = 50) => {
    const params = new URLSearchParams();
    if (status) params.set("status", status);
    params.set("limit", String(limit));
    return apiFetch<Manifest[]>(`/api/manifests?${params}`);
  },
  getManifest: (id: string) => apiFetch<Manifest>(`/api/manifests/${id}`),
  getManifestsByPeer: () =>
    apiFetch<ByPeerResponse<Manifest>>("/api/manifests/by-peer"),
  createManifest: (req: ManifestCreateRequest) =>
    post<Manifest>("/api/manifests", req),
  updateManifest: (id: string, req: ManifestUpdateRequest) =>
    put<Manifest>(`/api/manifests/${id}`, req),
  deleteManifest: (id: string) =>
    del<{ status: string }>(`/api/manifests/${id}`),
  searchManifests: (req: ManifestSearchRequest) =>
    post<Manifest[]>("/api/manifests/search", req),
  getManifestTasks: (id: string) =>
    apiFetch<Task[]>(`/api/manifests/${id}/tasks`),
  getManifestIdeas: (id: string) =>
    apiFetch<Idea[]>(`/api/manifests/${id}/ideas`),

  // --- Ideas ---
  getIdeas: (status?: string, limit = 50) => {
    const params = new URLSearchParams();
    if (status) params.set("status", status);
    params.set("limit", String(limit));
    return apiFetch<Idea[]>(`/api/ideas?${params}`);
  },
  getIdea: (id: string) => apiFetch<Idea>(`/api/ideas/${id}`),
  getIdeasByPeer: () =>
    apiFetch<ByPeerResponse<Idea>>("/api/ideas/by-peer"),
  createIdea: (req: IdeaCreateRequest) => post<Idea>("/api/ideas", req),
  deleteIdea: (id: string) =>
    del<{ status: string }>(`/api/ideas/${id}`),
  getIdeaManifests: (id: string) =>
    apiFetch<Manifest[]>(`/api/ideas/${id}/manifests`),

  // --- Idea <-> Manifest linking ---
  linkIdeaManifest: (req: LinkRequest) =>
    post<{ status: string }>("/api/link", req),
  unlinkIdeaManifest: (req: LinkRequest) =>
    post<{ status: string }>("/api/unlink", req),

  // --- Tasks ---
  getTasks: (status?: string, limit = 50) => {
    const params = new URLSearchParams();
    if (status) params.set("status", status);
    params.set("limit", String(limit));
    return apiFetch<Task[]>(`/api/tasks?${params}`);
  },
  getTask: (id: string) => apiFetch<Task>(`/api/tasks/${id}`),
  getTasksByPeer: () =>
    apiFetch<ByPeerResponse<Task>>("/api/tasks/by-peer"),
  getRunningTasks: () => apiFetch<Task[]>("/api/tasks/running"),
  createTask: (req: TaskCreateRequest) => post<Task>("/api/tasks", req),
  deleteTask: (id: string) =>
    del<{ status: string }>(`/api/tasks/${id}`),
  startTask: (id: string, req?: TaskStartRequest) =>
    post<{ status: string; schedule: string }>(
      `/api/tasks/${id}/start`,
      req,
    ),
  cancelTask: (id: string) =>
    post<{ status: string }>(`/api/tasks/${id}/cancel`),
  killTask: (id: string) =>
    post<{ status: string }>(`/api/tasks/${id}/kill`),
  getTaskOutput: (id: string) =>
    apiFetch<TaskOutputResponse>(`/api/tasks/${id}/output`),
  getTaskRuns: (id: string, limit = 50) =>
    apiFetch<TaskRun[]>(`/api/tasks/${id}/runs?limit=${limit}`),
  getTaskRun: (id: string, runId: number) =>
    apiFetch<TaskRun>(`/api/tasks/${id}/runs/${runId}`),
  getTaskActions: (id: string) =>
    apiFetch<Action[]>(`/api/tasks/${id}/actions`),
  getTaskAmnesia: (id: string) =>
    apiFetch<Amnesia[]>(`/api/tasks/${id}/amnesia`),
  getTaskDelusions: (id: string) =>
    apiFetch<Delusion[]>(`/api/tasks/${id}/delusions`),

  // --- Actions ---
  getActions: (limit = 50) =>
    apiFetch<Action[]>(`/api/actions?limit=${limit}`),
  getAction: (id: string) => apiFetch<Action>(`/api/actions/${id}`),
  getActionsByPeer: () =>
    apiFetch<ByPeerResponse<Action>>("/api/actions/by-peer"),

  // --- Amnesia ---
  getAmnesia: (status?: string, limit = 50) => {
    const params = new URLSearchParams();
    if (status) params.set("status", status);
    params.set("limit", String(limit));
    return apiFetch<Amnesia[]>(`/api/amnesia?${params}`);
  },
  getAmnesiaByPeer: () =>
    apiFetch<ByPeerResponse<Amnesia>>("/api/amnesia/by-peer"),
  confirmAmnesia: (id: number) =>
    post<{ status: string }>(`/api/amnesia/${id}/confirm`),
  dismissAmnesia: (id: number) =>
    post<{ status: string }>(`/api/amnesia/${id}/dismiss`),

  // --- Delusions ---
  getDelusions: (status?: string, limit = 50) => {
    const params = new URLSearchParams();
    if (status) params.set("status", status);
    params.set("limit", String(limit));
    return apiFetch<Delusion[]>(`/api/delusions?${params}`);
  },
  getDelusionsByPeer: () =>
    apiFetch<ByPeerResponse<Delusion>>("/api/delusions/by-peer"),
  confirmDelusion: (id: number) =>
    post<{ status: string }>(`/api/delusions/${id}/confirm`),
  dismissDelusion: (id: number) =>
    post<{ status: string }>(`/api/delusions/${id}/dismiss`),

  // --- Visceral Rules ---
  getVisceralRules: () => apiFetch<Memory[]>("/api/visceral"),
  getVisceralByPeer: () =>
    apiFetch<ByPeerResponse<Memory>>("/api/visceral/by-peer"),
  addVisceralRule: (rule: string) =>
    post<Memory>("/api/visceral", { rule }),
  deleteVisceralRule: (id: string) =>
    del<{ status: string }>(`/api/visceral/${id}`),
  getVisceralConfirmations: (limit = 20) =>
    apiFetch<VisceralConfirmation[]>(
      `/api/visceral/confirmations?limit=${limit}`,
    ),
  getRulePatterns: () => apiFetch<RulePattern[]>("/api/visceral/patterns"),
  getRulePattern: (ruleId: string) =>
    apiFetch<RulePattern>(`/api/visceral/patterns/${ruleId}`),
  updateRulePattern: (ruleId: string, req: RulePatternUpdateRequest) =>
    put<RulePattern>(`/api/visceral/patterns/${ruleId}`, req),

  // --- Markers ---
  getMarkers: (status?: string, limit = 50) => {
    const params = new URLSearchParams();
    if (status) params.set("status", status);
    params.set("limit", String(limit));
    return apiFetch<Marker[]>(`/api/markers?${params}`);
  },
  markSeen: (id: string) =>
    post<{ status: string }>(`/api/markers/${id}/seen`),
  markDone: (id: string) =>
    post<{ status: string }>(`/api/markers/${id}/done`),

  // --- Peers ---
  getPeers: () => apiFetch<PeersResponse>("/api/peers"),
  getAgents: () => apiFetch<AgentInfo[]>("/api/agents"),

  // --- Activity ---
  getActivity: () => apiFetch<unknown[]>("/api/activity"),
  getActivityByPeer: () =>
    apiFetch<ByPeerResponse<unknown>>("/api/activity/by-peer"),

  // --- Recall (soft-deleted items) ---
  getRecall: () => apiFetch<RecallItem[]>("/api/recall"),
  restore: (type: string, id: string) =>
    post<{ status: string }>(`/api/recall/${type}/${id}/restore`),

  // --- Settings ---
  getProfile: () => apiFetch<ProfileResponse>("/api/settings/profile"),
  updateProfile: (req: ProfileUpdateRequest) =>
    put<{ status: string }>("/api/settings/profile", req),
  getSettingsAgents: () => apiFetch<AgentInfo[]>("/api/settings/agents"),
  connectAgent: (id: string) =>
    post<{ status: string; agent: string }>(
      `/api/settings/agents/${id}/connect`,
    ),
  disconnectAgent: (id: string) =>
    post<{ status: string }>(`/api/settings/agents/${id}/disconnect`),

  // --- Sync ---

  /** Register this device as a peer node. */
  registerPeer: (reg: PeerRegistration) =>
    post<{ status: string; node_id: string }>("/api/peers/register", reg),

  /**
   * Fetch all manifests (used for sync pull).
   * Returns the full list; client-side diffing handles deltas.
   */
  syncPullManifests: (limit = 500) =>
    apiFetch<Manifest[]>(`/api/manifests?limit=${limit}`),

  /** Fetch all ideas (used for sync pull). */
  syncPullIdeas: (limit = 500) =>
    apiFetch<Idea[]>(`/api/ideas?limit=${limit}`),

  /** Fetch all visceral rules (used for sync pull). */
  syncPullVisceral: () => apiFetch<Memory[]>("/api/visceral"),

  // --- Utility ---

  /** Quick connectivity check — resolves true if /api/status responds. */
  ping: async (): Promise<boolean> => {
    try {
      await apiFetch<StatusResponse>("/api/status", { timeout: 5_000 });
      return true;
    } catch {
      return false;
    }
  },
} as const;
