/**
 * WebSocket client for real-time OpenLoom events.
 *
 * Connects to ws://{peerHost}/ws and emits typed events.
 * Auto-reconnects with exponential backoff (1s → 30s cap).
 */

import type {
  WSEvent,
  WSEventType,
  WSMemoryStored,
  WSPeerJoined,
  WSPeerLeft,
  WSTaskFailed,
  WSTaskCompleted,
  WSTaskStarted,
  WSAmnesiaDetected,
  WSDelusionDetected,
  WSSyncRequested,
  WSEntityChanged,
} from "./types";

type EventHandler = (data: unknown) => void;

const RECONNECT_MIN_MS = 1_000;
const RECONNECT_MAX_MS = 30_000;
const RECONNECT_FACTOR = 2;

export class OpenLoomSocket {
  private ws: WebSocket | null = null;
  private listeners = new Map<WSEventType | "*", Set<EventHandler>>();
  private reconnectMs = RECONNECT_MIN_MS;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private intentionalClose = false;
  private _host: string;

  constructor(host: string) {
    this._host = host;
  }

  /** Current connection state. */
  get connected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  /** Change host (disconnects first). */
  setHost(host: string): void {
    if (host === this._host && this.connected) return;
    this._host = host;
    this.disconnect();
    this.connect();
  }

  /** Open the WebSocket connection. */
  connect(): void {
    if (this.ws && this.ws.readyState <= WebSocket.OPEN) return;

    this.intentionalClose = false;
    const url = `ws://${this._host}/ws`;

    try {
      this.ws = new WebSocket(url);
    } catch {
      this.scheduleReconnect();
      return;
    }

    this.ws.onopen = () => {
      this.reconnectMs = RECONNECT_MIN_MS;
      this.emit("*", { event: "ws_connected" });
    };

    this.ws.onmessage = (e: MessageEvent) => {
      try {
        const msg = JSON.parse(e.data as string) as WSEvent;
        this.emit(msg.event, msg.data);
        this.emit("*", msg);
      } catch {
        // ignore malformed messages
      }
    };

    this.ws.onclose = () => {
      this.emit("*", { event: "ws_disconnected" });
      if (!this.intentionalClose) {
        this.scheduleReconnect();
      }
    };

    this.ws.onerror = () => {
      // onclose will fire after onerror — reconnect handled there
    };
  }

  /** Cleanly disconnect. */
  disconnect(): void {
    this.intentionalClose = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  /** Subscribe to a specific event type or "*" for all. */
  on(event: WSEventType | "*", handler: EventHandler): () => void {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, new Set());
    }
    this.listeners.get(event)!.add(handler);

    // Return unsubscribe function
    return () => {
      this.listeners.get(event)?.delete(handler);
    };
  }

  /** Typed convenience subscribers. */
  onMemoryStored(handler: (data: WSMemoryStored) => void): () => void {
    return this.on("memory_stored", handler as EventHandler);
  }

  onPeerJoined(handler: (data: WSPeerJoined) => void): () => void {
    return this.on("peer_joined", handler as EventHandler);
  }

  onPeerLeft(handler: (data: WSPeerLeft) => void): () => void {
    return this.on("peer_left", handler as EventHandler);
  }

  onTaskFailed(handler: (data: WSTaskFailed) => void): () => void {
    return this.on("task_failed", handler as EventHandler);
  }

  onTaskCompleted(handler: (data: WSTaskCompleted) => void): () => void {
    return this.on("task_completed", handler as EventHandler);
  }

  onSyncRequested(handler: (data: WSSyncRequested) => void): () => void {
    return this.on("sync_requested", handler as EventHandler);
  }

  onTaskStarted(handler: (data: WSTaskStarted) => void): () => void {
    return this.on("task_started", handler as EventHandler);
  }

  onAmnesiaDetected(handler: (data: WSAmnesiaDetected) => void): () => void {
    return this.on("amnesia_detected", handler as EventHandler);
  }

  onDelusionDetected(handler: (data: WSDelusionDetected) => void): () => void {
    return this.on("delusion_detected", handler as EventHandler);
  }

  onEntityChanged(handler: (data: WSEntityChanged) => void): () => void {
    return this.on("entity_changed", handler as EventHandler);
  }

  // --- Internal ---

  private emit(event: WSEventType | "*", data: unknown): void {
    this.listeners.get(event)?.forEach((fn) => {
      try {
        fn(data);
      } catch {
        // don't let listener errors crash the socket
      }
    });
  }

  private scheduleReconnect(): void {
    if (this.intentionalClose) return;
    if (this.reconnectTimer) return;

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, this.reconnectMs);

    this.reconnectMs = Math.min(
      this.reconnectMs * RECONNECT_FACTOR,
      RECONNECT_MAX_MS,
    );
  }
}
