/**
 * Global state — Zustand store with WebSocket lifecycle and peer identity.
 *
 * Manages:
 *   - Connection status (connected / connecting / disconnected)
 *   - Peer host address
 *   - Device peer UUID (persisted in SQLite)
 *   - WebSocket instance (real-time events)
 *   - Notification token
 */

import { create } from "zustand";
import { OpenLoomSocket } from "./sync";
import { api } from "./api";
import { configGet, configSet, outboxCount } from "./db";
import {
  syncAll,
  startPeriodicSync,
  stopPeriodicSync,
  setSyncCallbacks,
  getSyncStatus,
  isSyncing,
} from "./p2pSync";
import type { StatusResponse, SyncResult } from "./types";

export type ConnectionStatus = "disconnected" | "connecting" | "connected";

interface AppState {
  // --- Connection ---
  connected: boolean;
  connectionStatus: ConnectionStatus;
  peerHost: string;
  lastError: string | null;

  // --- Peer identity ---
  peerUuid: string | null;
  peerName: string | null;
  serverStatus: StatusResponse | null;

  // --- WebSocket ---
  socket: OpenLoomSocket | null;

  // --- Push notifications ---
  pushToken: string | null;

  // --- P2P Sync ---
  syncPending: number;
  syncInProgress: boolean;
  lastSyncAt: string | null;
  lastSyncResult: SyncResult | null;
  syncError: string | null;

  // --- Actions ---
  setPeerHost: (host: string) => void;
  setPeerUuid: (uuid: string | null) => void;
  setPeerName: (name: string | null) => void;
  setPushToken: (token: string | null) => void;
  setConnected: (connected: boolean) => void;

  /** Test connection to the peer, exchange identity, start WebSocket. */
  connect: () => Promise<void>;

  /** Disconnect WebSocket and reset connection state. */
  disconnect: () => void;

  /** Load persisted config (host, UUID) from SQLite on app start. */
  hydrate: () => Promise<void>;

  /** Trigger a full P2P sync (push outbox + pull from server). */
  triggerSync: () => Promise<SyncResult>;

  /** Refresh the pending outbox count. */
  refreshSyncPending: () => Promise<void>;
}

export const useAppStore = create<AppState>((set, get) => ({
  // Initial state
  connected: false,
  connectionStatus: "disconnected",
  peerHost: "localhost:8765",
  lastError: null,
  peerUuid: null,
  peerName: null,
  serverStatus: null,
  socket: null,
  pushToken: null,
  syncPending: 0,
  syncInProgress: false,
  lastSyncAt: null,
  lastSyncResult: null,
  syncError: null,

  // Simple setters
  setPeerHost: (peerHost) => {
    set({ peerHost });
    configSet("peer_host", peerHost).catch(() => {});
  },

  setPeerUuid: (peerUuid) => {
    set({ peerUuid });
    if (peerUuid) {
      configSet("peer_uuid", peerUuid).catch(() => {});
    }
  },

  setPeerName: (peerName) => {
    set({ peerName });
    if (peerName) {
      configSet("peer_name", peerName).catch(() => {});
    }
  },

  setPushToken: (pushToken) => set({ pushToken }),

  setConnected: (connected) =>
    set({
      connected,
      connectionStatus: connected ? "connected" : "disconnected",
    }),

  // --- Connect ---
  connect: async () => {
    const { peerHost, socket: existingSocket } = get();
    set({ connectionStatus: "connecting", lastError: null });

    try {
      // 1. Ping the server
      const status = await api.getStatus();

      // 2. Generate or load peer UUID
      let { peerUuid } = get();
      if (!peerUuid) {
        // Generate a UUID for this device
        peerUuid = generateUUID();
        set({ peerUuid });
        await configSet("peer_uuid", peerUuid);
      }

      // 3. Store server info
      set({
        connected: true,
        connectionStatus: "connected",
        serverStatus: status,
        lastError: null,
      });

      // 4. Start WebSocket
      let sock = existingSocket;
      if (!sock) {
        sock = new OpenLoomSocket(peerHost);
        set({ socket: sock });
      } else {
        sock.setHost(peerHost);
      }

      // Wire WebSocket connection state back to the store
      sock.on("*", (evt: unknown) => {
        const msg = evt as { event?: string };
        if (msg?.event === "ws_disconnected") {
          // Don't set disconnected immediately — WS reconnects automatically.
          // Only mark disconnected if ping also fails.
          api.ping().then((ok) => {
            if (!ok) {
              set({ connected: false, connectionStatus: "disconnected" });
              stopPeriodicSync();
            }
          });
        }
      });

      // Wire sync events — trigger sync when server signals entity changes
      sock.on("entity_changed", () => {
        // Remote entity changed — pull fresh data
        syncAll().catch(() => {});
      });
      sock.on("sync_requested", () => {
        syncAll().catch(() => {});
      });

      sock.connect();

      // 5. Register sync callbacks
      setSyncCallbacks({
        onSyncStart: () => set({ syncInProgress: true, syncError: null }),
        onSyncComplete: (result) =>
          set({
            syncInProgress: false,
            lastSyncAt: new Date().toISOString(),
            lastSyncResult: result,
            syncError: result.errors.length > 0 ? result.errors[0] : null,
          }),
        onSyncError: (error) =>
          set({ syncInProgress: false, syncError: error }),
        onPendingCountChanged: (count) => set({ syncPending: count }),
      });

      // 6. Initial sync on connect
      const pending = await outboxCount();
      set({ syncPending: pending });
      syncAll().catch(() => {});

      // 7. Start periodic sync (every 30s)
      startPeriodicSync();

      // 8. Try to register as a peer node (best-effort)
      try {
        await api.registerPeer({
          node_id: peerUuid,
          display_name: get().peerName ?? "Mobile",
          device_type: "mobile",
          platform: "ios",
        });
      } catch {
        // Server may not support peer registration yet — that's fine
      }
    } catch (err) {
      const msg =
        err instanceof Error ? err.message : "Connection failed";
      set({
        connected: false,
        connectionStatus: "disconnected",
        lastError: msg,
      });
      throw err;
    }
  },

  // --- Disconnect ---
  disconnect: () => {
    const { socket } = get();
    if (socket) {
      socket.disconnect();
    }
    stopPeriodicSync();
    set({
      connected: false,
      connectionStatus: "disconnected",
      serverStatus: null,
      lastError: null,
      syncInProgress: false,
    });
  },

  // --- Hydrate from SQLite on app start ---
  hydrate: async () => {
    try {
      const host = await configGet("peer_host");
      const uuid = await configGet("peer_uuid");
      const name = await configGet("peer_name");

      const pending = await outboxCount();

      set({
        peerHost: host || "localhost:8765",
        peerUuid: uuid || null,
        peerName: name || null,
        syncPending: pending,
      });
    } catch {
      // SQLite not ready yet — use defaults
    }
  },

  // --- P2P Sync ---
  triggerSync: async () => {
    if (!get().connected) {
      return { pushed: 0, pulled: 0, conflicts: 0, errors: ["Not connected"] };
    }
    return syncAll();
  },

  refreshSyncPending: async () => {
    const pending = await outboxCount();
    set({ syncPending: pending });
  },
}));

// ---------------------------------------------------------------------------
// UUID generator (no external dependency)
// ---------------------------------------------------------------------------

function generateUUID(): string {
  // crypto.randomUUID is available in React Native Hermes
  if (typeof crypto !== "undefined" && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  // Fallback: v4 UUID
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(
    /[xy]/g,
    (c) => {
      const r = (Math.random() * 16) | 0;
      const v = c === "x" ? r : (r & 0x3) | 0x8;
      return v.toString(16);
    },
  );
}
