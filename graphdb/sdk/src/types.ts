// Core types for the GraphDB SDK

/** Unique identifier for a node or edge (UUID string) */
export type Id = string;

/** A node in the graph */
export interface Node<T extends Record<string, unknown> = Record<string, unknown>> {
  id: Id;
  type: string;
  properties: T;
  parentId?: Id;
  position: string;
  createdAt: string;
  updatedAt: string;
  deleted?: boolean;
  deletedAt?: string;
}

/** An edge in the graph */
export interface Edge<T extends Record<string, unknown> = Record<string, unknown>> {
  id: Id;
  type: string;
  fromId: Id;
  toId: Id;
  properties: T;
  createdAt: string;
}

/** Event ID in the CRDT event graph */
export interface EventId {
  replicaId: string;
  seq: number;
}

/** A CRDT operation */
export interface Operation {
  id: EventId;
  parents: EventId[];
  type: OpType;
  targetId: Id;
  key?: string;
  value?: unknown;
  edgeFrom?: Id;
  edgeTo?: Id;
  edgeType?: string;
  nodeType?: string;
  parentRef?: Id;
  timestamp: string;
}

export enum OpType {
  InsertNode = 0,
  DeleteNode = 1,
  SetProperty = 2,
  DeleteProperty = 3,
  InsertEdge = 4,
  DeleteEdge = 5,
  MoveNode = 6,
  ReorderNode = 7,
  RestoreNode = 8,
  RestoreEdge = 9,
}

/** Result of a function call */
export interface FuncResult<T = unknown> {
  value?: T;
  error?: string;
  duration?: string;
}

/** Function definition */
export interface FuncDef {
  name: string;
  type: "query" | "mutation" | "action";
}

/** Subscription callback */
export type SubscriptionCallback<T = unknown> = (result: FuncResult<T>) => void;

/** WebSocket message types */
export type ClientMessageType =
  | "call"
  | "subscribe"
  | "unsubscribe"
  | "sync"
  | "authenticate";

export type ServerMessageType =
  | "result"
  | "subscription_update"
  | "error"
  | "sync_response"
  | "authenticated";

export interface ClientMessage {
  type: ClientMessageType;
  requestId: string;
  functionName?: string;
  args?: Record<string, unknown>;
  subscriptionId?: number;
  token?: string;
  operations?: Operation[];
  frontier?: EventId[];
}

export interface ServerMessage {
  type: ServerMessageType;
  requestId?: string;
  subscriptionId?: number;
  value?: unknown;
  error?: string;
  operations?: Operation[];
  frontier?: EventId[];
}

// --- Schema types ---

export type PropertyType =
  | "string"
  | "number"
  | "boolean"
  | "array"
  | "object"
  | "ref"
  | "any";

export interface PropertyDef {
  name: string;
  type: PropertyType;
  required?: boolean;
  indexed?: boolean;
  unique?: boolean;
}

export interface NodeTypeDef {
  name: string;
  properties: Record<string, PropertyDef>;
  allowedChildren?: string[];
  allowedParents?: string[];
}

export interface EdgeTypeDef {
  name: string;
  fromTypes: string[];
  toTypes: string[];
  properties?: Record<string, PropertyDef>;
}

export interface Schema {
  nodeTypes: Record<string, NodeTypeDef>;
  edgeTypes: Record<string, EdgeTypeDef>;
}

// --- Invariant types ---

export type InvariantType =
  | "cardinality"
  | "uniqueness"
  | "edge_constraint"
  | "required_edge"
  | "acyclicity"
  | "hierarchy_depth"
  | "child_count"
  | "custom";

export interface InvariantDef {
  id: string;
  name: string;
  type: InvariantType;
  description?: string;
  config?: unknown;
}

// --- Function definition helpers (Convex-compatible API) ---

/** Context for query functions */
export interface QueryCtx {
  graph: GraphReader;
}

/** Context for mutation functions */
export interface MutationCtx {
  graph: GraphReader & GraphWriter;
}

/** Context for action functions */
export interface ActionCtx {
  runQuery: <T = unknown>(name: string, args?: Record<string, unknown>) => Promise<T>;
  runMutation: <T = unknown>(name: string, args?: Record<string, unknown>) => Promise<T>;
}

/** Read-only graph operations */
export interface GraphReader {
  get: <T extends Record<string, unknown> = Record<string, unknown>>(id: Id) => Promise<Node<T> | null>;
  getByType: <T extends Record<string, unknown> = Record<string, unknown>>(type: string) => Promise<Node<T>[]>;
  getChildren: <T extends Record<string, unknown> = Record<string, unknown>>(id: Id) => Promise<Node<T>[]>;
  getParent: <T extends Record<string, unknown> = Record<string, unknown>>(id: Id) => Promise<Node<T> | null>;
  getSubtree: <T extends Record<string, unknown> = Record<string, unknown>>(id: Id) => Promise<Node<T>[]>;
  getAncestors: <T extends Record<string, unknown> = Record<string, unknown>>(id: Id) => Promise<Node<T>[]>;
  getOutEdges: (id: Id, edgeType?: string) => Promise<Edge[]>;
  getInEdges: (id: Id, edgeType?: string) => Promise<Edge[]>;
  getRoots: <T extends Record<string, unknown> = Record<string, unknown>>() => Promise<Node<T>[]>;
  traverse: <T extends Record<string, unknown> = Record<string, unknown>>(
    id: Id,
    edgeType: string,
    direction?: "in" | "out" | "both",
    maxDepth?: number
  ) => Promise<Node<T>[]>;
  findByIndex: <T extends Record<string, unknown> = Record<string, unknown>>(
    type: string,
    property: string,
    value: unknown
  ) => Promise<Node<T>[]>;
}

/** Write graph operations */
export interface GraphWriter {
  insertNode: (
    type: string,
    properties?: Record<string, unknown>,
    parentId?: Id
  ) => Promise<Id>;
  deleteNode: (id: Id) => Promise<void>;
  softDeleteNode: (id: Id) => Promise<void>;
  cascadeDeleteNode: (id: Id) => Promise<void>;
  restoreNode: (id: Id) => Promise<void>;
  patchNode: (id: Id, properties: Record<string, unknown>) => Promise<void>;
  setProperty: (id: Id, key: string, value: unknown) => Promise<void>;
  deleteProperty: (id: Id, key: string) => Promise<void>;
  insertEdge: (
    type: string,
    fromId: Id,
    toId: Id,
    properties?: Record<string, unknown>
  ) => Promise<Id>;
  deleteEdge: (id: Id) => Promise<void>;
  moveNode: (id: Id, newParentId?: Id) => Promise<void>;
  reorderNode: (id: Id, position: string) => Promise<void>;
  reorderBetween: (id: Id, afterId?: Id, beforeId?: Id) => Promise<void>;
}

// --- Derived graph types ---

/** A derived node produced by a derivation pipeline */
export interface DerivedNode<T extends Record<string, unknown> = Record<string, unknown>> {
  id: Id;
  derivedType: string;
  sourceId?: Id;
  sourceType?: string;
  properties: T;
  parentId?: Id;
  createdAt: string;
  updatedAt: string;
  derivationId?: string;
  inheritedFrom?: Id;
}

/** A derived edge */
export interface DerivedEdge<T extends Record<string, unknown> = Record<string, unknown>> {
  id: Id;
  type: string;
  fromId: Id;
  toId: Id;
  properties: T;
  sourceId?: Id;
}

// --- Batch operation types ---

export type BatchOpType =
  | "insert-node"
  | "delete-node"
  | "set-property"
  | "delete-property"
  | "insert-edge"
  | "delete-edge"
  | "move-node"
  | "reorder-node"
  | "restore-node"
  | "cascade-delete";

export interface BatchOp {
  op: BatchOpType;
  nodeType?: string;
  parentId?: Id;
  properties?: Record<string, unknown>;
  nodeId?: Id;
  edgeId?: Id;
  key?: string;
  value?: unknown;
  edgeType?: string;
  fromId?: Id;
  toId?: Id;
  position?: string;
}

export interface BatchResult {
  index: number;
  type: string;
  resultId?: Id;
}

// --- Version vector for sync ---

export type VersionVector = Record<string, number>;

// --- Gossip protocol types ---

export enum GossipMessageType {
  Offer = 0,
  Answer = 1,
  ICECandidate = 2,
  Delta = 3,
  VersionVector = 4,
  DeltaRequest = 5,
  PeerList = 6,
  Heartbeat = 7,
  DeadLetter = 8,
}

export interface GossipMessage {
  type: GossipMessageType;
  from: string;
  to?: string;
  payload: unknown;
  timestamp: string;
  seqNo: number;
}

export interface PeerInfo {
  id: string;
  replicaId: string;
  connectedAt: string;
  lastHeartbeat: string;
  versionVector: VersionVector;
}

export interface DeltaPayload {
  operations: Operation[];
  versionVector: VersionVector;
}

// --- Cluster types ---

export interface ClusterStats {
  totalShards: number;
  nodeCount: number;
  distribution: Record<string, number>;
  migrations: number;
}

export interface ShardInfo {
  id: number;
  state: number;
  owner: string;
  replicas: string[];
  nodeCount: number;
}
