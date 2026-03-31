/**
 * GraphDB Client - TypeScript SDK
 *
 * Provides a Convex-compatible client API for connecting to a GraphDB server.
 * Supports queries, mutations, actions, reactive subscriptions, and CRDT sync.
 */

import type {
  Id,
  Node,
  Edge,
  FuncResult,
  FuncDef,
  ClientMessage,
  ServerMessage,
  SubscriptionCallback,
  Operation,
  EventId,
} from "./types";

interface PendingRequest {
  resolve: (value: ServerMessage) => void;
  reject: (reason: Error) => void;
  timeout: ReturnType<typeof setTimeout>;
}

interface ActiveSubscription {
  queryName: string;
  args?: Record<string, unknown>;
  callback: SubscriptionCallback;
  serverSubId?: number;
}

export interface GraphDBClientOptions {
  /** Server URL (e.g., "http://localhost:8787") */
  url: string;
  /** Request timeout in milliseconds (default: 30000) */
  timeout?: number;
  /** Auto-reconnect WebSocket (default: true) */
  autoReconnect?: boolean;
  /** Authentication token */
  token?: string;
}

/**
 * GraphDBClient is the main client for interacting with a GraphDB server.
 * It mirrors the Convex client API with reactive subscriptions.
 */
export class GraphDBClient {
  private url: string;
  private wsUrl: string;
  private timeout: number;
  private autoReconnect: boolean;
  private token?: string;

  private ws: WebSocket | null = null;
  private requestId = 0;
  private pendingRequests = new Map<string, PendingRequest>();
  private subscriptions = new Map<number, ActiveSubscription>();
  private nextSubId = 1;
  private connected = false;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private connectionListeners: Array<(connected: boolean) => void> = [];

  constructor(options: GraphDBClientOptions) {
    this.url = options.url.replace(/\/$/, "");
    this.wsUrl = this.url.replace(/^http/, "ws") + "/ws";
    this.timeout = options.timeout ?? 30000;
    this.autoReconnect = options.autoReconnect ?? true;
    this.token = options.token;
  }

  // --- Connection Management ---

  /** Connect the WebSocket for real-time subscriptions */
  connect(): void {
    if (this.ws) return;

    this.ws = new WebSocket(this.wsUrl);

    this.ws.onopen = () => {
      this.connected = true;
      this.notifyConnectionListeners(true);

      // Re-subscribe to all active subscriptions
      for (const [localId, sub] of this.subscriptions) {
        this.sendSubscribe(localId, sub);
      }
    };

    this.ws.onmessage = (event) => {
      const msg: ServerMessage = JSON.parse(event.data);
      this.handleMessage(msg);
    };

    this.ws.onclose = () => {
      this.connected = false;
      this.ws = null;
      this.notifyConnectionListeners(false);

      // Reject all pending requests
      for (const [, req] of this.pendingRequests) {
        clearTimeout(req.timeout);
        req.reject(new Error("WebSocket disconnected"));
      }
      this.pendingRequests.clear();

      // Auto-reconnect
      if (this.autoReconnect) {
        this.reconnectTimer = setTimeout(() => this.connect(), 1000);
      }
    };

    this.ws.onerror = () => {
      // onclose will fire after this
    };
  }

  /** Disconnect the WebSocket */
  disconnect(): void {
    this.autoReconnect = false;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.connected = false;
  }

  /** Check if WebSocket is connected */
  isConnected(): boolean {
    return this.connected;
  }

  /** Listen for connection state changes */
  onConnectionChange(callback: (connected: boolean) => void): () => void {
    this.connectionListeners.push(callback);
    return () => {
      this.connectionListeners = this.connectionListeners.filter(
        (l) => l !== callback
      );
    };
  }

  // --- HTTP API (stateless calls) ---

  /** Execute a query function via HTTP */
  async query<T = unknown>(
    name: string,
    args?: Record<string, unknown>
  ): Promise<T> {
    const response = await fetch(`${this.url}/api/query`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, args: args ?? {} }),
    });

    const result = await response.json();
    if (result.error) {
      throw new GraphDBError(result.error, name);
    }
    return result.value as T;
  }

  /** Execute a mutation function via HTTP */
  async mutation<T = unknown>(
    name: string,
    args?: Record<string, unknown>
  ): Promise<T> {
    const response = await fetch(`${this.url}/api/mutation`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, args: args ?? {} }),
    });

    const result = await response.json();
    if (result.error) {
      throw new GraphDBError(result.error, name);
    }
    return result.value as T;
  }

  /** Execute an action function via HTTP */
  async action<T = unknown>(
    name: string,
    args?: Record<string, unknown>
  ): Promise<T> {
    const response = await fetch(`${this.url}/api/action`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, args: args ?? {} }),
    });

    const result = await response.json();
    if (result.error) {
      throw new GraphDBError(result.error, name);
    }
    return result.value as T;
  }

  // --- WebSocket API (real-time) ---

  /** Execute a function via WebSocket (lower latency than HTTP) */
  async call<T = unknown>(
    name: string,
    args?: Record<string, unknown>
  ): Promise<T> {
    const msg = await this.sendRequest({
      type: "call",
      requestId: this.nextRequestId(),
      functionName: name,
      args: args ?? {},
    });

    if (msg.error) {
      throw new GraphDBError(msg.error, name);
    }
    return msg.value as T;
  }

  /**
   * Subscribe to a reactive query. The callback fires whenever the
   * query result changes (like Convex's useQuery).
   *
   * Returns an unsubscribe function.
   */
  subscribe<T = unknown>(
    queryName: string,
    args: Record<string, unknown> | undefined,
    callback: SubscriptionCallback<T>
  ): () => void {
    const localId = this.nextSubId++;
    const sub: ActiveSubscription = {
      queryName,
      args,
      callback: callback as SubscriptionCallback,
    };
    this.subscriptions.set(localId, sub);

    if (this.connected) {
      this.sendSubscribe(localId, sub);
    }

    return () => {
      const removed = this.subscriptions.get(localId);
      this.subscriptions.delete(localId);
      if (removed?.serverSubId && this.connected) {
        this.sendRaw({
          type: "unsubscribe",
          requestId: this.nextRequestId(),
          subscriptionId: removed.serverSubId,
        });
      }
    };
  }

  // --- CRDT Sync ---

  /** Send local CRDT operations and receive missing remote operations */
  async sync(
    operations: Operation[],
    frontier: EventId[]
  ): Promise<{ operations: Operation[]; frontier: EventId[] }> {
    const msg = await this.sendRequest({
      type: "sync",
      requestId: this.nextRequestId(),
      operations: operations as unknown as undefined,
      frontier: frontier as unknown as undefined,
    });

    return {
      operations: (msg.operations as Operation[]) ?? [],
      frontier: (msg.frontier as EventId[]) ?? [],
    };
  }

  // --- Convenience Graph Methods ---

  /** Get a node by ID */
  async getNode<T extends Record<string, unknown> = Record<string, unknown>>(
    id: Id
  ): Promise<Node<T> | null> {
    return this.query<Node<T> | null>("graphdb:getNode", { id });
  }

  /** Get all nodes of a type */
  async getNodesByType<
    T extends Record<string, unknown> = Record<string, unknown>
  >(type: string): Promise<Node<T>[]> {
    return this.query<Node<T>[]>("graphdb:getNodesByType", { type });
  }

  /** Get children of a node */
  async getChildren<
    T extends Record<string, unknown> = Record<string, unknown>
  >(id: Id): Promise<Node<T>[]> {
    return this.query<Node<T>[]>("graphdb:getChildren", { id });
  }

  /** Get parent of a node */
  async getParent<T extends Record<string, unknown> = Record<string, unknown>>(
    id: Id
  ): Promise<Node<T> | null> {
    return this.query<Node<T> | null>("graphdb:getParent", { id });
  }

  /** Get the full subtree under a node */
  async getSubtree<
    T extends Record<string, unknown> = Record<string, unknown>
  >(id: Id): Promise<Node<T>[]> {
    return this.query<Node<T>[]>("graphdb:getSubtree", { id });
  }

  /** Get ancestors of a node (path to root) */
  async getAncestors<
    T extends Record<string, unknown> = Record<string, unknown>
  >(id: Id): Promise<Node<T>[]> {
    return this.query<Node<T>[]>("graphdb:getAncestors", { id });
  }

  /** Get outgoing edges from a node */
  async getOutEdges(id: Id): Promise<Edge[]> {
    return this.query<Edge[]>("graphdb:getOutEdges", { id });
  }

  /** Get incoming edges to a node */
  async getInEdges(id: Id): Promise<Edge[]> {
    return this.query<Edge[]>("graphdb:getInEdges", { id });
  }

  /** Get all root nodes */
  async getRoots<T extends Record<string, unknown> = Record<string, unknown>>(
  ): Promise<Node<T>[]> {
    return this.query<Node<T>[]>("graphdb:getRoots", {});
  }

  /** Traverse the graph following edges */
  async traverse<T extends Record<string, unknown> = Record<string, unknown>>(
    id: Id,
    edgeType: string,
    direction: "in" | "out" | "both" = "out",
    maxDepth = 10
  ): Promise<Node<T>[]> {
    return this.query<Node<T>[]>("graphdb:traverse", {
      id,
      edgeType,
      direction,
      maxDepth,
    });
  }

  /** Insert a new node */
  async insertNode(
    type: string,
    properties?: Record<string, unknown>,
    parentId?: Id
  ): Promise<Id> {
    return this.mutation<Id>("graphdb:insertNode", {
      type,
      properties: properties ?? {},
      parentId,
    });
  }

  /** Delete a node */
  async deleteNode(id: Id): Promise<void> {
    await this.mutation("graphdb:deleteNode", { id });
  }

  /** Update properties on a node */
  async patchNode(
    id: Id,
    properties: Record<string, unknown>
  ): Promise<void> {
    await this.mutation("graphdb:patchNode", { id, properties });
  }

  /** Insert an edge between two nodes */
  async insertEdge(
    type: string,
    fromId: Id,
    toId: Id,
    properties?: Record<string, unknown>
  ): Promise<Id> {
    return this.mutation<Id>("graphdb:insertEdge", {
      type,
      from: fromId,
      to: toId,
      properties: properties ?? {},
    });
  }

  /** Delete an edge */
  async deleteEdge(id: Id): Promise<void> {
    await this.mutation("graphdb:deleteEdge", { id });
  }

  /** Move a node to a new parent */
  async moveNode(id: Id, newParentId?: Id): Promise<void> {
    await this.mutation("graphdb:moveNode", { id, parentId: newParentId });
  }

  /** Soft-delete a node (marks as deleted but keeps in graph) */
  async softDeleteNode(id: Id): Promise<void> {
    await this.mutation("graphdb:softDeleteNode", { id });
  }

  /** Cascade-delete a node and all its descendants */
  async cascadeDeleteNode(id: Id): Promise<void> {
    await this.mutation("graphdb:cascadeDeleteNode", { id });
  }

  /** Restore a soft-deleted node */
  async restoreNode(id: Id): Promise<void> {
    await this.mutation("graphdb:restoreNode", { id });
  }

  /** Get children sorted by fractional index position */
  async getOrderedChildren<T extends Record<string, unknown> = Record<string, unknown>>(
    id: Id
  ): Promise<Node<T>[]> {
    return this.query<Node<T>[]>("graphdb:getOrderedChildren", { id });
  }

  /** Reorder a node to a specific fractional index position */
  async reorderNode(id: Id, position: string): Promise<void> {
    await this.mutation("graphdb:reorderNode", { id, position });
  }

  /** Get deleted nodes */
  async getDeletedNodes(): Promise<Node[]> {
    return this.query<Node[]>("graphdb:getDeletedNodes", {});
  }

  /** Get graph statistics */
  async getStats(): Promise<Record<string, unknown>> {
    return this.query<Record<string, unknown>>("graphdb:stats", {});
  }

  /** Reap orphaned nodes (nodes whose parents are deleted) */
  async reapOrphans(): Promise<{ reapedCount: number; reapedIds: Id[] }> {
    return this.mutation("graphdb:reapOrphans", {});
  }

  /** Execute a batch of operations */
  async executeBatch(operations: import("./types").BatchOp[]): Promise<import("./types").BatchResult[]> {
    return this.mutation("graphdb:batch", { operations });
  }

  /** Get cluster peer info */
  async getClusterPeers(): Promise<import("./types").PeerInfo[]> {
    const response = await fetch(`${this.url}/api/cluster/peers`);
    return response.json();
  }

  /** Get cluster shard stats */
  async getClusterStats(): Promise<import("./types").ClusterStats> {
    const response = await fetch(`${this.url}/api/cluster/shards`);
    return response.json();
  }

  /** Get a derived node by ID */
  async getDerivedNode<T extends Record<string, unknown> = Record<string, unknown>>(
    id: Id
  ): Promise<import("./types").DerivedNode<T> | null> {
    return this.query("graphdb:getDerivedNode", { id });
  }

  /** Get derived nodes by type */
  async getDerivedNodesByType<T extends Record<string, unknown> = Record<string, unknown>>(
    type: string
  ): Promise<import("./types").DerivedNode<T>[]> {
    return this.query("graphdb:getDerivedNodesByType", { type });
  }

  /** Deploy a compiled schema to the server for a given graph */
  async deploySchema(schema: import("./compiler").CompiledSchema): Promise<{ status: string; graphName: string }> {
    const response = await fetch(`${this.url}/api/schema/deploy`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(schema),
    });

    const result = await response.json();
    if (result.error) {
      throw new GraphDBError(result.error, "deploySchema");
    }
    return result;
  }

  /** List all deployed graphs */
  async listGraphs(): Promise<string[]> {
    const response = await fetch(`${this.url}/api/graphs`);
    return response.json();
  }

  /** Set feature flags on a graph (controls conditional module activation) */
  async setFeatureFlags(graphName: string, flags: Record<string, boolean>): Promise<void> {
    const response = await fetch(`${this.url}/api/graphs/flags`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ graphName, flags }),
    });
    const result = await response.json();
    if (result.error) {
      throw new GraphDBError(result.error, "setFeatureFlags");
    }
  }

  /** Get feature flags for a graph */
  async getFeatureFlags(graphName: string): Promise<Record<string, boolean>> {
    const response = await fetch(`${this.url}/api/graphs/flags?name=${encodeURIComponent(graphName)}`);
    return response.json();
  }

  /** Get the schema JSON for a deployed graph */
  async getGraphSchema(graphName: string): Promise<import("./compiler").CompiledSchema | null> {
    const response = await fetch(`${this.url}/api/graphs/schema?name=${encodeURIComponent(graphName)}`);
    if (response.status === 404) return null;
    return response.json();
  }

  /** Request a sync delta from the server based on local version vector */
  async requestDelta(versionVector: import("./types").VersionVector): Promise<import("./types").DeltaPayload> {
    const response = await fetch(`${this.url}/api/sync/delta`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ versionVector }),
    });
    return response.json();
  }

  /** List all registered functions */
  async listFunctions(): Promise<FuncDef[]> {
    const response = await fetch(`${this.url}/api/functions`);
    return response.json();
  }

  // --- Internal ---

  private handleMessage(msg: ServerMessage): void {
    if (msg.type === "subscription_update") {
      // Route to subscription callback
      for (const [, sub] of this.subscriptions) {
        if (sub.serverSubId === msg.subscriptionId) {
          sub.callback({
            value: msg.value,
            error: msg.error,
          });
          break;
        }
      }
      return;
    }

    // Route to pending request
    if (msg.requestId) {
      const pending = this.pendingRequests.get(msg.requestId);
      if (pending) {
        clearTimeout(pending.timeout);
        this.pendingRequests.delete(msg.requestId);
        pending.resolve(msg);
      }
    }
  }

  private sendSubscribe(localId: number, sub: ActiveSubscription): void {
    const requestId = this.nextRequestId();
    this.sendRaw({
      type: "subscribe",
      requestId,
      functionName: sub.queryName,
      args: sub.args ?? {},
      subscriptionId: localId,
    });

    // The server will respond with the server-side subscription ID
    const pending: PendingRequest = {
      resolve: (msg) => {
        if (msg.subscriptionId) {
          sub.serverSubId = msg.subscriptionId;
        }
      },
      reject: () => {},
      timeout: setTimeout(() => {
        this.pendingRequests.delete(requestId);
      }, this.timeout),
    };
    this.pendingRequests.set(requestId, pending);
  }

  private sendRequest(msg: ClientMessage): Promise<ServerMessage> {
    return new Promise((resolve, reject) => {
      if (!this.ws || !this.connected) {
        reject(new Error("WebSocket not connected"));
        return;
      }

      const timeout = setTimeout(() => {
        this.pendingRequests.delete(msg.requestId);
        reject(new Error(`Request ${msg.requestId} timed out`));
      }, this.timeout);

      this.pendingRequests.set(msg.requestId, { resolve, reject, timeout });
      this.sendRaw(msg);
    });
  }

  private sendRaw(msg: ClientMessage): void {
    if (this.ws && this.connected) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  private nextRequestId(): string {
    return `req-${++this.requestId}`;
  }

  private notifyConnectionListeners(connected: boolean): void {
    for (const listener of this.connectionListeners) {
      listener(connected);
    }
  }
}

/** Error thrown by GraphDB operations */
export class GraphDBError extends Error {
  constructor(
    message: string,
    public readonly functionName?: string
  ) {
    super(message);
    this.name = "GraphDBError";
  }
}
