// Core types for the GraphDB SDK

/** Unique identifier for a node or edge (UUID string) */
export type Id = string;

/** A node in the graph */
export interface Node<T extends Record<string, unknown> = Record<string, unknown>> {
  id: Id;
  type: string;
  properties: T;
  parentId?: Id;
  createdAt: string;
  updatedAt: string;
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
}
