/**
 * P2P Sync Engine — offline-first bidirectional sync.
 *
 * The phone is a peer node with its own UUID. Data syncs between
 * phone and laptop via the OpenLoom server.
 *
 * Sync flow:
 *   1. PUSH — Flush outbox (offline mutations) to server
 *   2. PULL — Fetch all entities from server, merge into local store
 *   3. Conflict resolution — last-write-wins based on updated_at
 *
 * Synced entity types: manifests, ideas, visceral rules.
 */

import { api } from "./api";
import {
  outboxGetAll,
  outboxRemove,
  outboxRetry,
  outboxCount,
  outboxPurgeStale,
  syncStateGet,
  syncStateSet,
  localEntityBulkSet,
  localEntityGetAll,
  localEntityGet,
  localEntityRemove,
  type SyncEntityType,
} from "./db";
import type {
  Manifest,
  Idea,
  Memory,
  ManifestCreateRequest,
  ManifestUpdateRequest,
  IdeaCreateRequest,
  SyncResult,
} from "./types";

const MAX_OUTBOX_RETRIES = 10;

// ---------------------------------------------------------------------------
// Sync orchestrator
// ---------------------------------------------------------------------------

export interface SyncCallbacks {
  onSyncStart?: () => void;
  onSyncComplete?: (result: SyncResult) => void;
  onSyncError?: (error: string) => void;
  onPendingCountChanged?: (count: number) => void;
}

let syncInProgress = false;
let syncTimer: ReturnType<typeof setInterval> | null = null;
let callbacks: SyncCallbacks = {};

/** Configure sync callbacks (called once from store/layout). */
export function setSyncCallbacks(cb: SyncCallbacks): void {
  callbacks = cb;
}

/** Full bidirectional sync: push outbox, then pull all entity types. */
export async function syncAll(): Promise<SyncResult> {
  if (syncInProgress) {
    return { pushed: 0, pulled: 0, conflicts: 0, errors: ["Sync already in progress"] };
  }

  syncInProgress = true;
  callbacks.onSyncStart?.();

  const result: SyncResult = { pushed: 0, pulled: 0, conflicts: 0, errors: [] };

  try {
    // Phase 1: Push outbox
    const pushResult = await flushOutbox();
    result.pushed = pushResult.pushed;
    result.errors.push(...pushResult.errors);

    // Phase 2: Pull all entity types
    const [manifestResult, ideaResult, visceralResult] = await Promise.allSettled([
      pullManifests(),
      pullIdeas(),
      pullVisceral(),
    ]);

    for (const r of [manifestResult, ideaResult, visceralResult]) {
      if (r.status === "fulfilled") {
        result.pulled += r.value.pulled;
        result.conflicts += r.value.conflicts;
      } else {
        result.errors.push(r.reason?.message ?? "Pull failed");
      }
    }

    // Update pending count after sync
    const pending = await outboxCount();
    callbacks.onPendingCountChanged?.(pending);
  } catch (err) {
    const msg = err instanceof Error ? err.message : "Sync failed";
    result.errors.push(msg);
    callbacks.onSyncError?.(msg);
  } finally {
    syncInProgress = false;
    callbacks.onSyncComplete?.(result);
  }

  return result;
}

/** Check if a sync is currently running. */
export function isSyncing(): boolean {
  return syncInProgress;
}

// ---------------------------------------------------------------------------
// Periodic sync
// ---------------------------------------------------------------------------

const DEFAULT_SYNC_INTERVAL_MS = 30_000; // 30 seconds

/** Start periodic sync. */
export function startPeriodicSync(intervalMs = DEFAULT_SYNC_INTERVAL_MS): void {
  stopPeriodicSync();
  syncTimer = setInterval(() => {
    syncAll().catch(() => {
      // Silent fail on periodic sync — will retry next interval
    });
  }, intervalMs);
}

/** Stop periodic sync. */
export function stopPeriodicSync(): void {
  if (syncTimer) {
    clearInterval(syncTimer);
    syncTimer = null;
  }
}

// ---------------------------------------------------------------------------
// Phase 1: Push (flush outbox)
// ---------------------------------------------------------------------------

interface PushResult {
  pushed: number;
  errors: string[];
}

async function flushOutbox(): Promise<PushResult> {
  const entries = await outboxGetAll();
  let pushed = 0;
  const errors: string[] = [];

  // Purge entries that have exceeded max retries
  await outboxPurgeStale(MAX_OUTBOX_RETRIES);

  for (const entry of entries) {
    try {
      const payload = JSON.parse(entry.payload);

      switch (entry.entity_type) {
        case "manifest":
          await pushManifest(entry.operation, entry.entity_id, payload);
          break;
        case "idea":
          await pushIdea(entry.operation, entry.entity_id, payload);
          break;
        case "visceral":
          await pushVisceral(entry.operation, entry.entity_id, payload);
          break;
      }

      await outboxRemove(entry.id);
      pushed++;
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Push failed";
      errors.push(`${entry.entity_type}/${entry.operation}: ${msg}`);
      await outboxRetry(entry.id);
    }
  }

  return { pushed, errors };
}

async function pushManifest(
  operation: string,
  entityId: string | null,
  payload: unknown,
): Promise<void> {
  switch (operation) {
    case "create":
      await api.createManifest(payload as ManifestCreateRequest);
      break;
    case "update":
      if (!entityId) throw new Error("Missing entity_id for manifest update");
      await api.updateManifest(entityId, payload as ManifestUpdateRequest);
      break;
    case "delete":
      if (!entityId) throw new Error("Missing entity_id for manifest delete");
      await api.deleteManifest(entityId);
      break;
  }
}

async function pushIdea(
  operation: string,
  entityId: string | null,
  payload: unknown,
): Promise<void> {
  switch (operation) {
    case "create":
      await api.createIdea(payload as IdeaCreateRequest);
      break;
    case "delete":
      if (!entityId) throw new Error("Missing entity_id for idea delete");
      await api.deleteIdea(entityId);
      break;
  }
}

async function pushVisceral(
  operation: string,
  entityId: string | null,
  payload: unknown,
): Promise<void> {
  switch (operation) {
    case "create": {
      const p = payload as { rule: string };
      await api.addVisceralRule(p.rule);
      break;
    }
    case "delete":
      if (!entityId) throw new Error("Missing entity_id for visceral delete");
      await api.deleteVisceralRule(entityId);
      break;
  }
}

// ---------------------------------------------------------------------------
// Phase 2: Pull (fetch from server, merge into local store)
// ---------------------------------------------------------------------------

interface PullResult {
  pulled: number;
  conflicts: number;
}

async function pullManifests(): Promise<PullResult> {
  const remote = await api.syncPullManifests();
  const localItems = await localEntityGetAll<Manifest>("manifest");

  const { merged, conflicts } = mergeEntities(localItems, remote, "manifest");

  await localEntityBulkSet<Manifest>(
    "manifest",
    merged,
    (m) => m.updated_at,
  );

  await syncStateSet("manifest", new Date().toISOString(), merged.length);

  return { pulled: merged.length, conflicts };
}

async function pullIdeas(): Promise<PullResult> {
  const remote = await api.syncPullIdeas();
  const localItems = await localEntityGetAll<Idea>("idea");

  const { merged, conflicts } = mergeEntities(localItems, remote, "idea");

  await localEntityBulkSet<Idea>(
    "idea",
    merged,
    (i) => i.updated_at,
  );

  await syncStateSet("idea", new Date().toISOString(), merged.length);

  return { pulled: merged.length, conflicts };
}

async function pullVisceral(): Promise<PullResult> {
  const remote = await api.syncPullVisceral();
  const localItems = await localEntityGetAll<Memory>("visceral");

  const { merged, conflicts } = mergeEntities(localItems, remote, "visceral");

  await localEntityBulkSet<Memory>(
    "visceral",
    merged,
    (v) => v.updated_at,
  );

  await syncStateSet("visceral", new Date().toISOString(), merged.length);

  return { pulled: merged.length, conflicts };
}

// ---------------------------------------------------------------------------
// Conflict resolution — last-write-wins
// ---------------------------------------------------------------------------

interface MergeResult<T> {
  merged: T[];
  conflicts: number;
}

function mergeEntities<T extends { id: string; updated_at: string }>(
  local: T[],
  remote: T[],
  _entityType: SyncEntityType,
): MergeResult<T> {
  const localMap = new Map(local.map((item) => [item.id, item]));
  const remoteMap = new Map(remote.map((item) => [item.id, item]));
  const merged: T[] = [];
  let conflicts = 0;

  // Process all remote items (they take precedence in last-write-wins
  // when timestamps match, since server is source of truth)
  for (const [id, remoteItem] of remoteMap) {
    const localItem = localMap.get(id);

    if (!localItem) {
      // New item from server — add it
      merged.push(remoteItem);
    } else {
      // Both exist — compare timestamps
      const localTime = new Date(localItem.updated_at).getTime();
      const remoteTime = new Date(remoteItem.updated_at).getTime();

      if (remoteTime >= localTime) {
        // Remote wins (or equal — server is source of truth)
        merged.push(remoteItem);
        if (localTime !== remoteTime && localTime > 0) {
          conflicts++;
        }
      } else {
        // Local wins — keep local version (it's newer, will be pushed next sync)
        merged.push(localItem);
        conflicts++;
      }
    }
    localMap.delete(id);
  }

  // Any remaining local items not on server — keep them
  // (they're either new local items or deleted on server)
  for (const [, localItem] of localMap) {
    merged.push(localItem);
  }

  return { merged, conflicts };
}

// ---------------------------------------------------------------------------
// Offline-aware mutations (queue if offline, direct if online)
// ---------------------------------------------------------------------------

export async function offlineCreateManifest(
  req: ManifestCreateRequest,
  isOnline: boolean,
): Promise<Manifest | null> {
  if (isOnline) {
    try {
      return await api.createManifest(req);
    } catch {
      // Fall through to offline queue
    }
  }

  // Queue for later sync
  const { outboxPush } = await import("./db");
  await outboxPush("manifest", "create", null, req);
  const pending = await outboxCount();
  callbacks.onPendingCountChanged?.(pending);
  return null;
}

export async function offlineUpdateManifest(
  id: string,
  req: ManifestUpdateRequest,
  isOnline: boolean,
): Promise<Manifest | null> {
  if (isOnline) {
    try {
      return await api.updateManifest(id, req);
    } catch {
      // Fall through to offline queue
    }
  }

  const { outboxPush } = await import("./db");
  await outboxPush("manifest", "update", id, req);
  const pending = await outboxCount();
  callbacks.onPendingCountChanged?.(pending);
  return null;
}

export async function offlineDeleteManifest(
  id: string,
  isOnline: boolean,
): Promise<boolean> {
  if (isOnline) {
    try {
      await api.deleteManifest(id);
      await localEntityRemove("manifest", id);
      return true;
    } catch {
      // Fall through to offline queue
    }
  }

  const { outboxPush } = await import("./db");
  await outboxPush("manifest", "delete", id, {});
  await localEntityRemove("manifest", id);
  const pending = await outboxCount();
  callbacks.onPendingCountChanged?.(pending);
  return true;
}

export async function offlineCreateIdea(
  req: IdeaCreateRequest,
  isOnline: boolean,
): Promise<Idea | null> {
  if (isOnline) {
    try {
      return await api.createIdea(req);
    } catch {
      // Fall through to offline queue
    }
  }

  const { outboxPush } = await import("./db");
  await outboxPush("idea", "create", null, req);
  const pending = await outboxCount();
  callbacks.onPendingCountChanged?.(pending);
  return null;
}

export async function offlineDeleteIdea(
  id: string,
  isOnline: boolean,
): Promise<boolean> {
  if (isOnline) {
    try {
      await api.deleteIdea(id);
      await localEntityRemove("idea", id);
      return true;
    } catch {
      // Fall through to offline queue
    }
  }

  const { outboxPush } = await import("./db");
  await outboxPush("idea", "delete", id, {});
  await localEntityRemove("idea", id);
  const pending = await outboxCount();
  callbacks.onPendingCountChanged?.(pending);
  return true;
}

export async function offlineAddVisceralRule(
  rule: string,
  isOnline: boolean,
): Promise<Memory | null> {
  if (isOnline) {
    try {
      return await api.addVisceralRule(rule);
    } catch {
      // Fall through to offline queue
    }
  }

  const { outboxPush } = await import("./db");
  await outboxPush("visceral", "create", null, { rule });
  const pending = await outboxCount();
  callbacks.onPendingCountChanged?.(pending);
  return null;
}

export async function offlineDeleteVisceralRule(
  id: string,
  isOnline: boolean,
): Promise<boolean> {
  if (isOnline) {
    try {
      await api.deleteVisceralRule(id);
      await localEntityRemove("visceral", id);
      return true;
    } catch {
      // Fall through to offline queue
    }
  }

  const { outboxPush } = await import("./db");
  await outboxPush("visceral", "delete", id, {});
  await localEntityRemove("visceral", id);
  const pending = await outboxCount();
  callbacks.onPendingCountChanged?.(pending);
  return true;
}

// ---------------------------------------------------------------------------
// Sync status helpers
// ---------------------------------------------------------------------------

export async function getSyncStatus(): Promise<{
  pending: number;
  manifests: { lastSync: string | null; count: number };
  ideas: { lastSync: string | null; count: number };
  visceral: { lastSync: string | null; count: number };
}> {
  const [pending, manifestState, ideaState, visceralState] = await Promise.all([
    outboxCount(),
    syncStateGet("manifest"),
    syncStateGet("idea"),
    syncStateGet("visceral"),
  ]);

  return {
    pending,
    manifests: {
      lastSync: manifestState?.lastSyncAt ?? null,
      count: manifestState?.itemCount ?? 0,
    },
    ideas: {
      lastSync: ideaState?.lastSyncAt ?? null,
      count: ideaState?.itemCount ?? 0,
    },
    visceral: {
      lastSync: visceralState?.lastSyncAt ?? null,
      count: visceralState?.itemCount ?? 0,
    },
  };
}
