/**
 * Local SQLite cache for offline-first operation + P2P sync.
 *
 * Uses expo-sqlite to persist API responses locally.
 * When online, data is fetched from the server and cached.
 * When offline, the cache serves stale data.
 *
 * P2P sync tables:
 *   sync_outbox   — queued offline mutations to push to server
 *   sync_state    — per-entity-type sync watermarks
 *   local_manifests, local_ideas, local_visceral — offline entity stores
 */

import * as SQLite from "expo-sqlite";

const DB_NAME = "openloom-cache.db";

let db: SQLite.SQLiteDatabase | null = null;

/** Open the cache database and create tables. */
export async function openDb(): Promise<SQLite.SQLiteDatabase> {
  if (db) return db;

  db = await SQLite.openDatabaseAsync(DB_NAME);

  await db.execAsync(`
    CREATE TABLE IF NOT EXISTS cache (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL,
      updated_at INTEGER NOT NULL
    );

    CREATE TABLE IF NOT EXISTS peer_config (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL
    );

    -- P2P Sync: outbox for offline mutations
    CREATE TABLE IF NOT EXISTS sync_outbox (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      entity_type TEXT NOT NULL,
      operation TEXT NOT NULL,
      entity_id TEXT,
      payload TEXT NOT NULL,
      created_at INTEGER NOT NULL,
      retries INTEGER DEFAULT 0
    );

    -- P2P Sync: watermarks per entity type
    CREATE TABLE IF NOT EXISTS sync_state (
      entity_type TEXT PRIMARY KEY,
      last_sync_at TEXT NOT NULL,
      item_count INTEGER DEFAULT 0
    );

    -- P2P Sync: local manifests for offline access
    CREATE TABLE IF NOT EXISTS local_manifests (
      id TEXT PRIMARY KEY,
      data TEXT NOT NULL,
      updated_at TEXT NOT NULL,
      synced INTEGER DEFAULT 1
    );

    -- P2P Sync: local ideas for offline access
    CREATE TABLE IF NOT EXISTS local_ideas (
      id TEXT PRIMARY KEY,
      data TEXT NOT NULL,
      updated_at TEXT NOT NULL,
      synced INTEGER DEFAULT 1
    );

    -- P2P Sync: local visceral rules for offline access
    CREATE TABLE IF NOT EXISTS local_visceral (
      id TEXT PRIMARY KEY,
      data TEXT NOT NULL,
      updated_at TEXT NOT NULL,
      synced INTEGER DEFAULT 1
    );
  `);

  return db;
}

/** Close the database. */
export async function closeDb(): Promise<void> {
  if (db) {
    await db.closeAsync();
    db = null;
  }
}

// ---------------------------------------------------------------------------
// Generic cache operations
// ---------------------------------------------------------------------------

/** Write a JSON-serializable value to cache. */
export async function cacheSet(key: string, value: unknown): Promise<void> {
  const conn = await openDb();
  const json = JSON.stringify(value);
  const now = Date.now();
  await conn.runAsync(
    "INSERT OR REPLACE INTO cache (key, value, updated_at) VALUES (?, ?, ?)",
    [key, json, now],
  );
}

/** Read a cached value. Returns null if missing or expired. */
export async function cacheGet<T>(
  key: string,
  maxAgeMs?: number,
): Promise<T | null> {
  const conn = await openDb();
  const row = await conn.getFirstAsync<{ value: string; updated_at: number }>(
    "SELECT value, updated_at FROM cache WHERE key = ?",
    [key],
  );
  if (!row) return null;

  if (maxAgeMs != null && Date.now() - row.updated_at > maxAgeMs) {
    return null; // expired
  }

  try {
    return JSON.parse(row.value) as T;
  } catch {
    return null;
  }
}

/** Delete a cached value. */
export async function cacheDel(key: string): Promise<void> {
  const conn = await openDb();
  await conn.runAsync("DELETE FROM cache WHERE key = ?", [key]);
}

/** Clear all cached data. */
export async function cacheClear(): Promise<void> {
  const conn = await openDb();
  await conn.runAsync("DELETE FROM cache");
}

/** Get cache age in ms for a key. Returns null if not cached. */
export async function cacheAge(key: string): Promise<number | null> {
  const conn = await openDb();
  const row = await conn.getFirstAsync<{ updated_at: number }>(
    "SELECT updated_at FROM cache WHERE key = ?",
    [key],
  );
  if (!row) return null;
  return Date.now() - row.updated_at;
}

// ---------------------------------------------------------------------------
// Peer config persistence (survives app restarts)
// ---------------------------------------------------------------------------

/** Save a peer config value (host, UUID, etc.). */
export async function configSet(key: string, value: string): Promise<void> {
  const conn = await openDb();
  await conn.runAsync(
    "INSERT OR REPLACE INTO peer_config (key, value) VALUES (?, ?)",
    [key, value],
  );
}

/** Read a peer config value. */
export async function configGet(key: string): Promise<string | null> {
  const conn = await openDb();
  const row = await conn.getFirstAsync<{ value: string }>(
    "SELECT value FROM peer_config WHERE key = ?",
    [key],
  );
  return row?.value ?? null;
}

// ---------------------------------------------------------------------------
// Sync outbox operations (offline mutation queue)
// ---------------------------------------------------------------------------

export type SyncEntityType = "manifest" | "idea" | "visceral";
export type SyncOperation = "create" | "update" | "delete";

export interface OutboxEntry {
  id: number;
  entity_type: SyncEntityType;
  operation: SyncOperation;
  entity_id: string | null;
  payload: string;
  created_at: number;
  retries: number;
}

/** Queue an offline mutation for later sync. */
export async function outboxPush(
  entityType: SyncEntityType,
  operation: SyncOperation,
  entityId: string | null,
  payload: unknown,
): Promise<void> {
  const conn = await openDb();
  await conn.runAsync(
    `INSERT INTO sync_outbox (entity_type, operation, entity_id, payload, created_at)
     VALUES (?, ?, ?, ?, ?)`,
    [entityType, operation, entityId, JSON.stringify(payload), Date.now()],
  );
}

/** Get all pending outbox entries, ordered oldest first. */
export async function outboxGetAll(): Promise<OutboxEntry[]> {
  const conn = await openDb();
  return conn.getAllAsync<OutboxEntry>(
    "SELECT * FROM sync_outbox ORDER BY created_at ASC",
  );
}

/** Remove a successfully synced outbox entry. */
export async function outboxRemove(id: number): Promise<void> {
  const conn = await openDb();
  await conn.runAsync("DELETE FROM sync_outbox WHERE id = ?", [id]);
}

/** Increment retry count for a failed outbox entry. */
export async function outboxRetry(id: number): Promise<void> {
  const conn = await openDb();
  await conn.runAsync(
    "UPDATE sync_outbox SET retries = retries + 1 WHERE id = ?",
    [id],
  );
}

/** Get outbox count (for badge display). */
export async function outboxCount(): Promise<number> {
  const conn = await openDb();
  const row = await conn.getFirstAsync<{ cnt: number }>(
    "SELECT COUNT(*) as cnt FROM sync_outbox",
  );
  return row?.cnt ?? 0;
}

/** Clear outbox entries that have exceeded max retries. */
export async function outboxPurgeStale(maxRetries = 10): Promise<number> {
  const conn = await openDb();
  const result = await conn.runAsync(
    "DELETE FROM sync_outbox WHERE retries >= ?",
    [maxRetries],
  );
  return result.changes;
}

// ---------------------------------------------------------------------------
// Sync state (watermarks)
// ---------------------------------------------------------------------------

/** Get the last sync timestamp for an entity type. */
export async function syncStateGet(
  entityType: SyncEntityType,
): Promise<{ lastSyncAt: string; itemCount: number } | null> {
  const conn = await openDb();
  const row = await conn.getFirstAsync<{
    last_sync_at: string;
    item_count: number;
  }>("SELECT last_sync_at, item_count FROM sync_state WHERE entity_type = ?", [
    entityType,
  ]);
  if (!row) return null;
  return { lastSyncAt: row.last_sync_at, itemCount: row.item_count };
}

/** Update the sync watermark for an entity type. */
export async function syncStateSet(
  entityType: SyncEntityType,
  lastSyncAt: string,
  itemCount: number,
): Promise<void> {
  const conn = await openDb();
  await conn.runAsync(
    `INSERT OR REPLACE INTO sync_state (entity_type, last_sync_at, item_count)
     VALUES (?, ?, ?)`,
    [entityType, lastSyncAt, itemCount],
  );
}

// ---------------------------------------------------------------------------
// Local entity stores (offline-first storage)
// ---------------------------------------------------------------------------

type LocalTable = "local_manifests" | "local_ideas" | "local_visceral";

function tableForEntity(entityType: SyncEntityType): LocalTable {
  switch (entityType) {
    case "manifest":
      return "local_manifests";
    case "idea":
      return "local_ideas";
    case "visceral":
      return "local_visceral";
  }
}

/** Upsert an entity into the local store. */
export async function localEntitySet<T>(
  entityType: SyncEntityType,
  id: string,
  data: T,
  updatedAt: string,
  synced = true,
): Promise<void> {
  const conn = await openDb();
  const table = tableForEntity(entityType);
  await conn.runAsync(
    `INSERT OR REPLACE INTO ${table} (id, data, updated_at, synced) VALUES (?, ?, ?, ?)`,
    [id, JSON.stringify(data), updatedAt, synced ? 1 : 0],
  );
}

/** Get an entity from the local store. */
export async function localEntityGet<T>(
  entityType: SyncEntityType,
  id: string,
): Promise<T | null> {
  const conn = await openDb();
  const table = tableForEntity(entityType);
  const row = await conn.getFirstAsync<{ data: string }>(
    `SELECT data FROM ${table} WHERE id = ?`,
    [id],
  );
  if (!row) return null;
  try {
    return JSON.parse(row.data) as T;
  } catch {
    return null;
  }
}

/** Get all entities from a local store. */
export async function localEntityGetAll<T>(
  entityType: SyncEntityType,
): Promise<T[]> {
  const conn = await openDb();
  const table = tableForEntity(entityType);
  const rows = await conn.getAllAsync<{ data: string }>(
    `SELECT data FROM ${table} ORDER BY updated_at DESC`,
  );
  return rows.map((r) => JSON.parse(r.data) as T);
}

/** Remove an entity from the local store. */
export async function localEntityRemove(
  entityType: SyncEntityType,
  id: string,
): Promise<void> {
  const conn = await openDb();
  const table = tableForEntity(entityType);
  await conn.runAsync(`DELETE FROM ${table} WHERE id = ?`, [id]);
}

/** Bulk upsert entities (used during sync pull). */
export async function localEntityBulkSet<T extends { id: string }>(
  entityType: SyncEntityType,
  items: T[],
  getUpdatedAt: (item: T) => string,
): Promise<void> {
  const conn = await openDb();
  const table = tableForEntity(entityType);

  // Use a transaction for atomicity
  await conn.execAsync("BEGIN TRANSACTION");
  try {
    for (const item of items) {
      await conn.runAsync(
        `INSERT OR REPLACE INTO ${table} (id, data, updated_at, synced) VALUES (?, ?, ?, 1)`,
        [item.id, JSON.stringify(item), getUpdatedAt(item)],
      );
    }
    await conn.execAsync("COMMIT");
  } catch (err) {
    await conn.execAsync("ROLLBACK");
    throw err;
  }
}

/** Count unsynced local entities. */
export async function localEntityUnsyncedCount(
  entityType: SyncEntityType,
): Promise<number> {
  const conn = await openDb();
  const table = tableForEntity(entityType);
  const row = await conn.getFirstAsync<{ cnt: number }>(
    `SELECT COUNT(*) as cnt FROM ${table} WHERE synced = 0`,
  );
  return row?.cnt ?? 0;
}

// ---------------------------------------------------------------------------
// Fetch-with-cache pattern
// ---------------------------------------------------------------------------

/**
 * Fetch from the API, cache the result, and return it.
 * If the fetch fails (offline), return the cached value.
 *
 * @param cacheKey - Unique cache key for this request
 * @param fetcher - Function that performs the API call
 * @param maxAgeMs - Max cache age before considering stale (default: 5 min)
 */
export async function fetchWithCache<T>(
  cacheKey: string,
  fetcher: () => Promise<T>,
  maxAgeMs = 5 * 60 * 1000,
): Promise<{ data: T; fromCache: boolean }> {
  try {
    const data = await fetcher();
    await cacheSet(cacheKey, data);
    return { data, fromCache: false };
  } catch {
    // Offline or error — try cache
    const cached = await cacheGet<T>(cacheKey, maxAgeMs);
    if (cached != null) {
      return { data: cached, fromCache: true };
    }
    // Try stale cache (no age limit)
    const stale = await cacheGet<T>(cacheKey);
    if (stale != null) {
      return { data: stale, fromCache: true };
    }
    throw new Error(`Offline and no cached data for ${cacheKey}`);
  }
}
