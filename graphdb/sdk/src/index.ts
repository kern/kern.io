/**
 * @graphdb/sdk - TypeScript SDK for GraphDB
 *
 * A reactive graph database with CRDT support, hierarchical nodes,
 * and server-side invariant validation.
 *
 * @packageDocumentation
 */

// Core client
export { GraphDBClient, GraphDBError } from "./client";
export type { GraphDBClientOptions } from "./client";

// Types
export type {
  Id,
  Node,
  Edge,
  EventId,
  Operation,
  OpType,
  FuncResult,
  FuncDef,
  SubscriptionCallback,
  Schema,
  NodeTypeDef,
  EdgeTypeDef,
  PropertyDef,
  PropertyType,
  InvariantDef,
  InvariantType,
  QueryCtx,
  MutationCtx,
  ActionCtx,
  GraphReader,
  GraphWriter,
} from "./types";

// Schema DSL
export {
  defineSchema,
  defineNode,
  defineEdge,
  s,
  PropertyBuilder,
  NodeTypeBuilder,
  EdgeTypeBuilder,
} from "./schema";

// Invariant builders
export {
  uniqueProperty,
  maxOutEdges,
  maxInEdges,
  requiredEdge,
  acyclic,
  maxDepth,
  maxChildren,
  minChildren,
  edgeConstraint,
} from "./schema";

// Function definitions
export {
  query,
  mutation,
  action,
} from "./functions";
export type {
  QueryFunction,
  MutationFunction,
  ActionFunction,
  FunctionReference,
  QueryReturn,
  QueryArgs,
  MutationReturn,
  MutationArgs,
  ActionReturn,
  ActionArgs,
} from "./functions";
