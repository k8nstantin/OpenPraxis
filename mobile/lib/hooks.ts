/**
 * React Query hooks for all OpenLoom API endpoints.
 *
 * Convention:
 *   useXxx()          — query hook (GET)
 *   useXxxMutation()  — mutation hook (POST/PUT/DELETE)
 *
 * All query hooks use the offline-first fetchWithCache pattern
 * and invalidate related queries on mutations.
 */

import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";
import { api } from "./api";
import { fetchWithCache, localEntityGetAll } from "./db";
import { useAppStore } from "./store";
import {
  offlineCreateManifest,
  offlineUpdateManifest,
  offlineDeleteManifest,
  offlineCreateIdea,
  offlineDeleteIdea,
  offlineAddVisceralRule,
  offlineDeleteVisceralRule,
  getSyncStatus,
} from "./p2pSync";
import type {
  StatusResponse,
  Memory,
  MemorySearchRequest,
  MemorySearchResult,
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
} from "./types";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Only enable queries when connected to a peer. */
function useConnected() {
  return useAppStore((s) => s.connected);
}

// Query key factories for consistent invalidation
export const queryKeys = {
  status: ["status"] as const,
  memories: ["memories"] as const,
  memoriesByPeer: ["memories", "by-peer"] as const,
  memoriesBySession: ["memories", "by-session"] as const,
  memoryTree: ["memories", "tree"] as const,
  memory: (id: string) => ["memories", id] as const,
  conversations: ["conversations"] as const,
  conversationsByPeer: ["conversations", "by-peer"] as const,
  conversation: (id: string) => ["conversations", id] as const,
  conversationActions: (id: string) =>
    ["conversations", id, "actions"] as const,
  manifests: ["manifests"] as const,
  manifestsByPeer: ["manifests", "by-peer"] as const,
  manifest: (id: string) => ["manifests", id] as const,
  manifestTasks: (id: string) => ["manifests", id, "tasks"] as const,
  manifestIdeas: (id: string) => ["manifests", id, "ideas"] as const,
  ideas: ["ideas"] as const,
  ideasByPeer: ["ideas", "by-peer"] as const,
  idea: (id: string) => ["ideas", id] as const,
  ideaManifests: (id: string) => ["ideas", id, "manifests"] as const,
  tasks: ["tasks"] as const,
  tasksByPeer: ["tasks", "by-peer"] as const,
  runningTasks: ["tasks", "running"] as const,
  task: (id: string) => ["tasks", id] as const,
  taskOutput: (id: string) => ["tasks", id, "output"] as const,
  taskRuns: (id: string) => ["tasks", id, "runs"] as const,
  taskActions: (id: string) => ["tasks", id, "actions"] as const,
  taskAmnesia: (id: string) => ["tasks", id, "amnesia"] as const,
  taskDelusions: (id: string) => ["tasks", id, "delusions"] as const,
  actions: ["actions"] as const,
  actionsByPeer: ["actions", "by-peer"] as const,
  amnesia: ["amnesia"] as const,
  amnesiaByPeer: ["amnesia", "by-peer"] as const,
  delusions: ["delusions"] as const,
  delusionsByPeer: ["delusions", "by-peer"] as const,
  visceral: ["visceral"] as const,
  visceralByPeer: ["visceral", "by-peer"] as const,
  visceralConfirmations: ["visceral", "confirmations"] as const,
  rulePatterns: ["visceral", "patterns"] as const,
  rulePattern: (id: string) => ["visceral", "patterns", id] as const,
  markers: ["markers"] as const,
  peers: ["peers"] as const,
  agents: ["agents"] as const,
  activity: ["activity"] as const,
  activityByPeer: ["activity", "by-peer"] as const,
  recall: ["recall"] as const,
  profile: ["profile"] as const,
  settingsAgents: ["settings", "agents"] as const,
} as const;

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

export function useStatus(
  opts?: Partial<UseQueryOptions<StatusResponse>>,
) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.status,
    queryFn: () =>
      fetchWithCache("status", api.getStatus).then((r) => r.data),
    enabled: connected,
    refetchInterval: 10_000,
    ...opts,
  });
}

// ---------------------------------------------------------------------------
// Memories
// ---------------------------------------------------------------------------

export function useMemories(prefix = "/") {
  const connected = useConnected();
  return useQuery({
    queryKey: [...queryKeys.memories, prefix],
    queryFn: () =>
      fetchWithCache(`memories:${prefix}`, () =>
        api.getMemories(prefix),
      ).then((r) => r.data),
    enabled: connected,
  });
}

export function useMemory(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.memory(id),
    queryFn: () =>
      fetchWithCache(`memory:${id}`, () => api.getMemory(id)).then(
        (r) => r.data,
      ),
    enabled: connected && !!id,
  });
}

export function useMemoriesByPeer() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.memoriesByPeer,
    queryFn: () =>
      fetchWithCache("memories:by-peer", api.getMemoriesByPeer).then(
        (r) => r.data,
      ),
    enabled: connected,
  });
}

export function useMemoriesBySession() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.memoriesBySession,
    queryFn: () =>
      fetchWithCache("memories:by-session", api.getMemoriesBySession).then(
        (r) => r.data,
      ),
    enabled: connected,
  });
}

export function useMemoryTree() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.memoryTree,
    queryFn: () =>
      fetchWithCache("memories:tree", api.getMemoryTree).then((r) => r.data),
    enabled: connected,
  });
}

export function useSearchMemories() {
  return useMutation({
    mutationFn: (req: MemorySearchRequest) => api.searchMemories(req),
  });
}

export function useDeleteMemory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteMemory(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.memories });
      qc.invalidateQueries({ queryKey: queryKeys.memoriesByPeer });
      qc.invalidateQueries({ queryKey: queryKeys.recall });
    },
  });
}

// ---------------------------------------------------------------------------
// Conversations
// ---------------------------------------------------------------------------

export function useConversations(status?: string, limit?: number) {
  const connected = useConnected();
  return useQuery({
    queryKey: [...queryKeys.conversations, status, limit],
    queryFn: () =>
      fetchWithCache(
        `conversations:${status ?? "all"}:${limit ?? 20}`,
        () => api.getConversations(status, limit),
      ).then((r) => r.data),
    enabled: connected,
  });
}

export function useConversation(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.conversation(id),
    queryFn: () =>
      fetchWithCache(`conversation:${id}`, () =>
        api.getConversation(id),
      ).then((r) => r.data),
    enabled: connected && !!id,
  });
}

export function useConversationsByPeer() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.conversationsByPeer,
    queryFn: () =>
      fetchWithCache(
        "conversations:by-peer",
        api.getConversationsByPeer,
      ).then((r) => r.data),
    enabled: connected,
  });
}

export function useConversationActions(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.conversationActions(id),
    queryFn: () => api.getConversationActions(id),
    enabled: connected && !!id,
  });
}

export function useSearchConversations() {
  return useMutation({
    mutationFn: (req: ConversationSearchRequest) =>
      api.searchConversations(req),
  });
}

// ---------------------------------------------------------------------------
// Manifests
// ---------------------------------------------------------------------------

export function useManifests(status?: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: [...queryKeys.manifests, status],
    queryFn: () =>
      fetchWithCache(`manifests:${status ?? "all"}`, () =>
        api.getManifests(status),
      ).then((r) => r.data),
    enabled: connected,
  });
}

export function useManifest(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.manifest(id),
    queryFn: () =>
      fetchWithCache(`manifest:${id}`, () => api.getManifest(id)).then(
        (r) => r.data,
      ),
    enabled: connected && !!id,
  });
}

export function useManifestsByPeer() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.manifestsByPeer,
    queryFn: () =>
      fetchWithCache("manifests:by-peer", api.getManifestsByPeer).then(
        (r) => r.data,
      ),
    enabled: connected,
  });
}

export function useManifestTasks(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.manifestTasks(id),
    queryFn: () => api.getManifestTasks(id),
    enabled: connected && !!id,
  });
}

export function useManifestIdeas(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.manifestIdeas(id),
    queryFn: () => api.getManifestIdeas(id),
    enabled: connected && !!id,
  });
}

export function useCreateManifest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: ManifestCreateRequest) => api.createManifest(req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.manifests });
      qc.invalidateQueries({ queryKey: queryKeys.manifestsByPeer });
    },
  });
}

export function useUpdateManifest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: ManifestUpdateRequest }) =>
      api.updateManifest(id, req),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: queryKeys.manifests });
      qc.invalidateQueries({ queryKey: queryKeys.manifestsByPeer });
      qc.invalidateQueries({ queryKey: queryKeys.manifest(id) });
    },
  });
}

export function useDeleteManifest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteManifest(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.manifests });
      qc.invalidateQueries({ queryKey: queryKeys.manifestsByPeer });
      qc.invalidateQueries({ queryKey: queryKeys.recall });
    },
  });
}

export function useSearchManifests() {
  return useMutation({
    mutationFn: (req: ManifestSearchRequest) => api.searchManifests(req),
  });
}

// ---------------------------------------------------------------------------
// Ideas
// ---------------------------------------------------------------------------

export function useIdeas(status?: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: [...queryKeys.ideas, status],
    queryFn: () =>
      fetchWithCache(`ideas:${status ?? "all"}`, () =>
        api.getIdeas(status),
      ).then((r) => r.data),
    enabled: connected,
  });
}

export function useIdea(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.idea(id),
    queryFn: () =>
      fetchWithCache(`idea:${id}`, () => api.getIdea(id)).then(
        (r) => r.data,
      ),
    enabled: connected && !!id,
  });
}

export function useIdeasByPeer() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.ideasByPeer,
    queryFn: () =>
      fetchWithCache("ideas:by-peer", api.getIdeasByPeer).then(
        (r) => r.data,
      ),
    enabled: connected,
  });
}

export function useIdeaManifests(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.ideaManifests(id),
    queryFn: () => api.getIdeaManifests(id),
    enabled: connected && !!id,
  });
}

export function useCreateIdea() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: IdeaCreateRequest) => api.createIdea(req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.ideas });
      qc.invalidateQueries({ queryKey: queryKeys.ideasByPeer });
    },
  });
}

export function useDeleteIdea() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteIdea(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.ideas });
      qc.invalidateQueries({ queryKey: queryKeys.ideasByPeer });
      qc.invalidateQueries({ queryKey: queryKeys.recall });
    },
  });
}

export function useLinkIdeaManifest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: LinkRequest) => api.linkIdeaManifest(req),
    onSuccess: (_data, req) => {
      qc.invalidateQueries({
        queryKey: queryKeys.ideaManifests(req.idea_id),
      });
      qc.invalidateQueries({
        queryKey: queryKeys.manifestIdeas(req.manifest_id),
      });
    },
  });
}

export function useUnlinkIdeaManifest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: LinkRequest) => api.unlinkIdeaManifest(req),
    onSuccess: (_data, req) => {
      qc.invalidateQueries({
        queryKey: queryKeys.ideaManifests(req.idea_id),
      });
      qc.invalidateQueries({
        queryKey: queryKeys.manifestIdeas(req.manifest_id),
      });
    },
  });
}

// ---------------------------------------------------------------------------
// Tasks
// ---------------------------------------------------------------------------

export function useTasks(status?: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: [...queryKeys.tasks, status],
    queryFn: () =>
      fetchWithCache(`tasks:${status ?? "all"}`, () =>
        api.getTasks(status),
      ).then((r) => r.data),
    enabled: connected,
  });
}

export function useTask(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.task(id),
    queryFn: () =>
      fetchWithCache(`task:${id}`, () => api.getTask(id)).then(
        (r) => r.data,
      ),
    enabled: connected && !!id,
  });
}

export function useTasksByPeer() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.tasksByPeer,
    queryFn: () =>
      fetchWithCache("tasks:by-peer", api.getTasksByPeer).then(
        (r) => r.data,
      ),
    enabled: connected,
  });
}

export function useRunningTasks() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.runningTasks,
    queryFn: () =>
      fetchWithCache("tasks:running", api.getRunningTasks).then(
        (r) => r.data,
      ),
    enabled: connected,
    refetchInterval: 3_000, // poll running tasks frequently
  });
}

export function useTaskOutput(id: string, enabled = true) {
  const connected = useConnected();
  return useQuery<TaskOutputResponse>({
    queryKey: queryKeys.taskOutput(id),
    queryFn: () => api.getTaskOutput(id),
    enabled: connected && !!id && enabled,
    refetchInterval: (query) => {
      // Poll every 2s while running, stop when done
      const data = query.state.data as TaskOutputResponse | undefined;
      return data?.running === false ? false : 2_000;
    },
  });
}

export function useTaskRuns(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.taskRuns(id),
    queryFn: () => api.getTaskRuns(id),
    enabled: connected && !!id,
  });
}

export function useTaskActions(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.taskActions(id),
    queryFn: () => api.getTaskActions(id),
    enabled: connected && !!id,
  });
}

export function useTaskAmnesia(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.taskAmnesia(id),
    queryFn: () => api.getTaskAmnesia(id),
    enabled: connected && !!id,
  });
}

export function useTaskDelusions(id: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.taskDelusions(id),
    queryFn: () => api.getTaskDelusions(id),
    enabled: connected && !!id,
  });
}

export function useCreateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: TaskCreateRequest) => api.createTask(req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.tasks });
      qc.invalidateQueries({ queryKey: queryKeys.tasksByPeer });
    },
  });
}

export function useDeleteTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteTask(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.tasks });
      qc.invalidateQueries({ queryKey: queryKeys.tasksByPeer });
      qc.invalidateQueries({ queryKey: queryKeys.recall });
    },
  });
}

export function useStartTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req?: TaskStartRequest }) =>
      api.startTask(id, req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.tasks });
      qc.invalidateQueries({ queryKey: queryKeys.runningTasks });
    },
  });
}

export function useCancelTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.cancelTask(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.tasks });
      qc.invalidateQueries({ queryKey: queryKeys.runningTasks });
    },
  });
}

export function useKillTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.killTask(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.tasks });
      qc.invalidateQueries({ queryKey: queryKeys.runningTasks });
    },
  });
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

export function useActions(limit?: number) {
  const connected = useConnected();
  return useQuery({
    queryKey: [...queryKeys.actions, limit],
    queryFn: () => api.getActions(limit),
    enabled: connected,
  });
}

export function useActionsByPeer() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.actionsByPeer,
    queryFn: () => api.getActionsByPeer(),
    enabled: connected,
  });
}

// ---------------------------------------------------------------------------
// Amnesia
// ---------------------------------------------------------------------------

export function useAmnesia(status?: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: [...queryKeys.amnesia, status],
    queryFn: () => api.getAmnesia(status),
    enabled: connected,
  });
}

export function useAmnesiaByPeer() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.amnesiaByPeer,
    queryFn: () => api.getAmnesiaByPeer(),
    enabled: connected,
  });
}

export function useConfirmAmnesia() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.confirmAmnesia(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.amnesia });
    },
  });
}

export function useDismissAmnesia() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.dismissAmnesia(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.amnesia });
    },
  });
}

// ---------------------------------------------------------------------------
// Delusions
// ---------------------------------------------------------------------------

export function useDelusions(status?: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: [...queryKeys.delusions, status],
    queryFn: () => api.getDelusions(status),
    enabled: connected,
  });
}

export function useDelusionsByPeer() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.delusionsByPeer,
    queryFn: () => api.getDelusionsByPeer(),
    enabled: connected,
  });
}

export function useConfirmDelusion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.confirmDelusion(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.delusions });
    },
  });
}

export function useDismissDelusion() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => api.dismissDelusion(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.delusions });
    },
  });
}

// ---------------------------------------------------------------------------
// Visceral Rules
// ---------------------------------------------------------------------------

export function useVisceralRules() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.visceral,
    queryFn: () =>
      fetchWithCache("visceral", api.getVisceralRules).then((r) => r.data),
    enabled: connected,
  });
}

export function useVisceralByPeer() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.visceralByPeer,
    queryFn: () =>
      fetchWithCache("visceral:by-peer", api.getVisceralByPeer).then(
        (r) => r.data,
      ),
    enabled: connected,
  });
}

export function useAddVisceralRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (rule: string) => api.addVisceralRule(rule),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.visceral });
      qc.invalidateQueries({ queryKey: queryKeys.visceralByPeer });
    },
  });
}

export function useDeleteVisceralRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteVisceralRule(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.visceral });
      qc.invalidateQueries({ queryKey: queryKeys.visceralByPeer });
    },
  });
}

export function useVisceralConfirmations(limit?: number) {
  const connected = useConnected();
  return useQuery({
    queryKey: [...queryKeys.visceralConfirmations, limit],
    queryFn: () => api.getVisceralConfirmations(limit),
    enabled: connected,
  });
}

export function useRulePatterns() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.rulePatterns,
    queryFn: () => api.getRulePatterns(),
    enabled: connected,
  });
}

export function useRulePattern(ruleId: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.rulePattern(ruleId),
    queryFn: () => api.getRulePattern(ruleId),
    enabled: connected && !!ruleId,
  });
}

export function useUpdateRulePattern() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      ruleId,
      req,
    }: {
      ruleId: string;
      req: RulePatternUpdateRequest;
    }) => api.updateRulePattern(ruleId, req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.rulePatterns });
    },
  });
}

// ---------------------------------------------------------------------------
// Markers
// ---------------------------------------------------------------------------

export function useMarkers(status?: string) {
  const connected = useConnected();
  return useQuery({
    queryKey: [...queryKeys.markers, status],
    queryFn: () =>
      fetchWithCache(`markers:${status ?? "all"}`, () =>
        api.getMarkers(status),
      ).then((r) => r.data),
    enabled: connected,
  });
}

export function useMarkSeen() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.markSeen(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.markers });
      qc.invalidateQueries({ queryKey: queryKeys.status });
    },
  });
}

export function useMarkDone() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.markDone(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.markers });
      qc.invalidateQueries({ queryKey: queryKeys.status });
    },
  });
}

// ---------------------------------------------------------------------------
// Peers
// ---------------------------------------------------------------------------

export function usePeers() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.peers,
    queryFn: () =>
      fetchWithCache("peers", api.getPeers).then((r) => r.data),
    enabled: connected,
    refetchInterval: 15_000,
  });
}

// ---------------------------------------------------------------------------
// Activity
// ---------------------------------------------------------------------------

export function useActivity() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.activity,
    queryFn: () =>
      fetchWithCache("activity", api.getActivity).then((r) => r.data),
    enabled: connected,
  });
}

export function useActivityByPeer() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.activityByPeer,
    queryFn: () =>
      fetchWithCache("activity:by-peer", api.getActivityByPeer).then(
        (r) => r.data,
      ),
    enabled: connected,
  });
}

// ---------------------------------------------------------------------------
// Recall
// ---------------------------------------------------------------------------

export function useRecall() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.recall,
    queryFn: () =>
      fetchWithCache("recall", api.getRecall).then((r) => r.data),
    enabled: connected,
  });
}

export function useRestore() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ type, id }: { type: string; id: string }) =>
      api.restore(type, id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.recall });
      // Invalidate all resource types since we don't know which was restored
      qc.invalidateQueries({ queryKey: queryKeys.memories });
      qc.invalidateQueries({ queryKey: queryKeys.manifests });
      qc.invalidateQueries({ queryKey: queryKeys.ideas });
      qc.invalidateQueries({ queryKey: queryKeys.tasks });
    },
  });
}

// ---------------------------------------------------------------------------
// Settings / Profile
// ---------------------------------------------------------------------------

export function useProfile() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.profile,
    queryFn: () =>
      fetchWithCache("profile", api.getProfile).then((r) => r.data),
    enabled: connected,
  });
}

export function useUpdateProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: ProfileUpdateRequest) => api.updateProfile(req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.profile });
      qc.invalidateQueries({ queryKey: queryKeys.status });
    },
  });
}

export function useSettingsAgents() {
  const connected = useConnected();
  return useQuery({
    queryKey: queryKeys.settingsAgents,
    queryFn: () => api.getSettingsAgents(),
    enabled: connected,
  });
}

export function useConnectAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.connectAgent(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.settingsAgents });
    },
  });
}

export function useDisconnectAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.disconnectAgent(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.settingsAgents });
    },
  });
}

// ---------------------------------------------------------------------------
// P2P Sync
// ---------------------------------------------------------------------------

/** Get current sync status (pending count, last sync per entity type). */
export function useSyncStatus() {
  return useQuery({
    queryKey: ["sync", "status"],
    queryFn: getSyncStatus,
    refetchInterval: 10_000,
  });
}

/** Trigger a manual sync. */
export function useTriggerSync() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => useAppStore.getState().triggerSync(),
    onSuccess: () => {
      // Invalidate all entity queries after sync
      qc.invalidateQueries({ queryKey: queryKeys.manifests });
      qc.invalidateQueries({ queryKey: queryKeys.manifestsByPeer });
      qc.invalidateQueries({ queryKey: queryKeys.ideas });
      qc.invalidateQueries({ queryKey: queryKeys.ideasByPeer });
      qc.invalidateQueries({ queryKey: queryKeys.visceral });
      qc.invalidateQueries({ queryKey: queryKeys.visceralByPeer });
      qc.invalidateQueries({ queryKey: ["sync", "status"] });
    },
  });
}

/** Get locally cached manifests (offline-first). */
export function useLocalManifests() {
  return useQuery({
    queryKey: ["local", "manifests"],
    queryFn: () => localEntityGetAll<Manifest>("manifest"),
  });
}

/** Get locally cached ideas (offline-first). */
export function useLocalIdeas() {
  return useQuery({
    queryKey: ["local", "ideas"],
    queryFn: () => localEntityGetAll<Idea>("idea"),
  });
}

/** Get locally cached visceral rules (offline-first). */
export function useLocalVisceral() {
  return useQuery({
    queryKey: ["local", "visceral"],
    queryFn: () => localEntityGetAll<Memory>("visceral"),
  });
}

/** Offline-aware manifest creation. */
export function useOfflineCreateManifest() {
  const qc = useQueryClient();
  const connected = useConnected();
  return useMutation({
    mutationFn: (req: ManifestCreateRequest) =>
      offlineCreateManifest(req, connected),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.manifests });
      qc.invalidateQueries({ queryKey: queryKeys.manifestsByPeer });
      qc.invalidateQueries({ queryKey: ["local", "manifests"] });
      qc.invalidateQueries({ queryKey: ["sync", "status"] });
    },
  });
}

/** Offline-aware manifest update. */
export function useOfflineUpdateManifest() {
  const qc = useQueryClient();
  const connected = useConnected();
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: ManifestUpdateRequest }) =>
      offlineUpdateManifest(id, req, connected),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: queryKeys.manifests });
      qc.invalidateQueries({ queryKey: queryKeys.manifestsByPeer });
      qc.invalidateQueries({ queryKey: queryKeys.manifest(id) });
      qc.invalidateQueries({ queryKey: ["local", "manifests"] });
      qc.invalidateQueries({ queryKey: ["sync", "status"] });
    },
  });
}

/** Offline-aware manifest deletion. */
export function useOfflineDeleteManifest() {
  const qc = useQueryClient();
  const connected = useConnected();
  return useMutation({
    mutationFn: (id: string) => offlineDeleteManifest(id, connected),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.manifests });
      qc.invalidateQueries({ queryKey: queryKeys.manifestsByPeer });
      qc.invalidateQueries({ queryKey: queryKeys.recall });
      qc.invalidateQueries({ queryKey: ["local", "manifests"] });
      qc.invalidateQueries({ queryKey: ["sync", "status"] });
    },
  });
}

/** Offline-aware idea creation. */
export function useOfflineCreateIdea() {
  const qc = useQueryClient();
  const connected = useConnected();
  return useMutation({
    mutationFn: (req: IdeaCreateRequest) =>
      offlineCreateIdea(req, connected),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.ideas });
      qc.invalidateQueries({ queryKey: queryKeys.ideasByPeer });
      qc.invalidateQueries({ queryKey: ["local", "ideas"] });
      qc.invalidateQueries({ queryKey: ["sync", "status"] });
    },
  });
}

/** Offline-aware idea deletion. */
export function useOfflineDeleteIdea() {
  const qc = useQueryClient();
  const connected = useConnected();
  return useMutation({
    mutationFn: (id: string) => offlineDeleteIdea(id, connected),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.ideas });
      qc.invalidateQueries({ queryKey: queryKeys.ideasByPeer });
      qc.invalidateQueries({ queryKey: queryKeys.recall });
      qc.invalidateQueries({ queryKey: ["local", "ideas"] });
      qc.invalidateQueries({ queryKey: ["sync", "status"] });
    },
  });
}

/** Offline-aware visceral rule creation. */
export function useOfflineAddVisceralRule() {
  const qc = useQueryClient();
  const connected = useConnected();
  return useMutation({
    mutationFn: (rule: string) =>
      offlineAddVisceralRule(rule, connected),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.visceral });
      qc.invalidateQueries({ queryKey: queryKeys.visceralByPeer });
      qc.invalidateQueries({ queryKey: ["local", "visceral"] });
      qc.invalidateQueries({ queryKey: ["sync", "status"] });
    },
  });
}

/** Offline-aware visceral rule deletion. */
export function useOfflineDeleteVisceralRule() {
  const qc = useQueryClient();
  const connected = useConnected();
  return useMutation({
    mutationFn: (id: string) =>
      offlineDeleteVisceralRule(id, connected),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.visceral });
      qc.invalidateQueries({ queryKey: queryKeys.visceralByPeer });
      qc.invalidateQueries({ queryKey: ["local", "visceral"] });
      qc.invalidateQueries({ queryKey: ["sync", "status"] });
    },
  });
}

// ---------------------------------------------------------------------------
// Notification Preferences
// ---------------------------------------------------------------------------

import {
  getAllPreferences,
  setEnabled as setNotifEnabled,
  type NotificationPreferenceKey,
} from "./notifications";

/** Get all notification preferences. */
export function useNotificationPreferences() {
  return useQuery({
    queryKey: ["notification-preferences"],
    queryFn: getAllPreferences,
  });
}

/** Toggle a notification preference on/off. */
export function useToggleNotificationPref() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      key,
      enabled,
    }: {
      key: NotificationPreferenceKey;
      enabled: boolean;
    }) => setNotifEnabled(key, enabled),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notification-preferences"] });
    },
  });
}
