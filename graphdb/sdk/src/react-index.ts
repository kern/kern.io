/**
 * React bindings for @graphdb/sdk
 *
 * @packageDocumentation
 */

// Re-export everything from the main SDK
export * from "./index";

// React-specific exports
export {
  GraphDBProvider,
  useGraphDB,
  useQuery,
  useMutation,
  useAction,
  useNode,
  useNodesByType,
  useChildren,
  useParent,
  useOutEdges,
  useInEdges,
  useRoots,
  useSubtree,
  useInsertNode,
  useDeleteNode,
  usePatchNode,
  useInsertEdge,
  useDeleteEdge,
  useMoveNode,
  useConnectionStatus,
} from "./react";

export type {
  GraphDBProviderProps,
  UseQueryResult,
  UseMutationResult,
  UseActionResult,
} from "./react";
