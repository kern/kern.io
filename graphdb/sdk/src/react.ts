/**
 * React hooks for GraphDB - provides Convex-compatible useQuery, useMutation, useAction.
 */

import { useCallback, useEffect, useRef, useState, createContext, useContext, createElement } from "react";
import type { ReactNode } from "react";
import { GraphDBClient, type GraphDBClientOptions } from "./client";
import type { FuncResult, Id, Node, Edge } from "./types";

// --- Context ---

const GraphDBContext = createContext<GraphDBClient | null>(null);

/** Provider props */
export interface GraphDBProviderProps {
  client: GraphDBClient;
  children: ReactNode;
}

/** Provider component — wrap your app with this */
export function GraphDBProvider({ client, children }: GraphDBProviderProps) {
  useEffect(() => {
    client.connect();
    return () => client.disconnect();
  }, [client]);

  return createElement(GraphDBContext.Provider, { value: client }, children);
}

/** Hook to get the GraphDB client */
export function useGraphDB(): GraphDBClient {
  const client = useContext(GraphDBContext);
  if (!client) {
    throw new Error("useGraphDB must be used within a GraphDBProvider");
  }
  return client;
}

// --- useQuery ---

export interface UseQueryResult<T> {
  data: T | undefined;
  error: string | undefined;
  isLoading: boolean;
}

/**
 * Reactive query hook — re-renders when query result changes.
 * Like Convex's useQuery.
 *
 * @param queryName - Name of the query function
 * @param args - Arguments to pass to the query
 */
export function useQuery<T = unknown>(
  queryName: string,
  args?: Record<string, unknown>
): UseQueryResult<T> {
  const client = useGraphDB();
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const [isLoading, setIsLoading] = useState(true);

  // Stable reference for args
  const argsRef = useRef(args);
  const argsKey = JSON.stringify(args);
  argsRef.current = args;

  useEffect(() => {
    setIsLoading(true);

    const unsubscribe = client.subscribe<T>(
      queryName,
      argsRef.current,
      (result: FuncResult<T>) => {
        if (result.error) {
          setError(result.error);
          setData(undefined);
        } else {
          setError(undefined);
          setData(result.value);
        }
        setIsLoading(false);
      }
    );

    return unsubscribe;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client, queryName, argsKey]);

  return { data, error, isLoading };
}

// --- useMutation ---

export type UseMutationResult<Args extends Record<string, unknown>, T = unknown> = [
  (args: Args) => Promise<T>,
  { isLoading: boolean; error: string | undefined }
];

/**
 * Mutation hook — returns a function to call the mutation.
 * Like Convex's useMutation.
 *
 * @param mutationName - Name of the mutation function
 */
export function useMutation<
  Args extends Record<string, unknown> = Record<string, unknown>,
  T = unknown
>(mutationName: string): UseMutationResult<Args, T> {
  const client = useGraphDB();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const mutate = useCallback(
    async (args: Args): Promise<T> => {
      setIsLoading(true);
      setError(undefined);
      try {
        const result = await client.mutation<T>(mutationName, args);
        return result;
      } catch (e: any) {
        setError(e.message);
        throw e;
      } finally {
        setIsLoading(false);
      }
    },
    [client, mutationName]
  );

  return [mutate, { isLoading, error }];
}

// --- useAction ---

export type UseActionResult<Args extends Record<string, unknown>, T = unknown> = [
  (args: Args) => Promise<T>,
  { isLoading: boolean; error: string | undefined }
];

/**
 * Action hook — returns a function to call the action.
 * Like Convex's useAction.
 */
export function useAction<
  Args extends Record<string, unknown> = Record<string, unknown>,
  T = unknown
>(actionName: string): UseActionResult<Args, T> {
  const client = useGraphDB();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const execute = useCallback(
    async (args: Args): Promise<T> => {
      setIsLoading(true);
      setError(undefined);
      try {
        const result = await client.action<T>(actionName, args);
        return result;
      } catch (e: any) {
        setError(e.message);
        throw e;
      } finally {
        setIsLoading(false);
      }
    },
    [client, actionName]
  );

  return [execute, { isLoading, error }];
}

// --- Graph-specific hooks ---

/** Hook to get a node by ID (reactive) */
export function useNode<T extends Record<string, unknown> = Record<string, unknown>>(
  id: Id | undefined
): UseQueryResult<Node<T>> {
  return useQuery<Node<T>>("graphdb:getNode", id ? { id } : undefined);
}

/** Hook to get nodes by type (reactive) */
export function useNodesByType<T extends Record<string, unknown> = Record<string, unknown>>(
  type: string
): UseQueryResult<Node<T>[]> {
  return useQuery<Node<T>[]>("graphdb:getNodesByType", { type });
}

/** Hook to get children of a node (reactive) */
export function useChildren<T extends Record<string, unknown> = Record<string, unknown>>(
  id: Id | undefined
): UseQueryResult<Node<T>[]> {
  return useQuery<Node<T>[]>("graphdb:getChildren", id ? { id } : undefined);
}

/** Hook to get parent of a node (reactive) */
export function useParent<T extends Record<string, unknown> = Record<string, unknown>>(
  id: Id | undefined
): UseQueryResult<Node<T>> {
  return useQuery<Node<T>>("graphdb:getParent", id ? { id } : undefined);
}

/** Hook to get outgoing edges (reactive) */
export function useOutEdges(id: Id | undefined): UseQueryResult<Edge[]> {
  return useQuery<Edge[]>("graphdb:getOutEdges", id ? { id } : undefined);
}

/** Hook to get incoming edges (reactive) */
export function useInEdges(id: Id | undefined): UseQueryResult<Edge[]> {
  return useQuery<Edge[]>("graphdb:getInEdges", id ? { id } : undefined);
}

/** Hook to get root nodes (reactive) */
export function useRoots<T extends Record<string, unknown> = Record<string, unknown>>(
): UseQueryResult<Node<T>[]> {
  return useQuery<Node<T>[]>("graphdb:getRoots", {});
}

/** Hook to get subtree (reactive) */
export function useSubtree<T extends Record<string, unknown> = Record<string, unknown>>(
  id: Id | undefined
): UseQueryResult<Node<T>[]> {
  return useQuery<Node<T>[]>("graphdb:getSubtree", id ? { id } : undefined);
}

// --- Mutation convenience hooks ---

/** Hook for inserting nodes */
export function useInsertNode() {
  return useMutation<{
    type: string;
    properties?: Record<string, unknown>;
    parentId?: Id;
  }, Id>("graphdb:insertNode");
}

/** Hook for deleting nodes */
export function useDeleteNode() {
  return useMutation<{ id: Id }>("graphdb:deleteNode");
}

/** Hook for patching nodes */
export function usePatchNode() {
  return useMutation<{
    id: Id;
    properties: Record<string, unknown>;
  }>("graphdb:patchNode");
}

/** Hook for inserting edges */
export function useInsertEdge() {
  return useMutation<{
    type: string;
    from: Id;
    to: Id;
    properties?: Record<string, unknown>;
  }, Id>("graphdb:insertEdge");
}

/** Hook for deleting edges */
export function useDeleteEdge() {
  return useMutation<{ id: Id }>("graphdb:deleteEdge");
}

/** Hook for moving nodes in the hierarchy */
export function useMoveNode() {
  return useMutation<{
    id: Id;
    parentId?: Id;
  }>("graphdb:moveNode");
}

/** Hook for soft-deleting nodes */
export function useSoftDeleteNode() {
  return useMutation<{ id: Id }>("graphdb:softDeleteNode");
}

/** Hook for cascade-deleting nodes */
export function useCascadeDeleteNode() {
  return useMutation<{ id: Id }>("graphdb:cascadeDeleteNode");
}

/** Hook for restoring soft-deleted nodes */
export function useRestoreNode() {
  return useMutation<{ id: Id }>("graphdb:restoreNode");
}

/** Hook for reordering nodes (fractional indexing) */
export function useReorderNode() {
  return useMutation<{
    id: Id;
    position: string;
  }>("graphdb:reorderNode");
}

/** Hook to get children sorted by fractional index (reactive) */
export function useOrderedChildren<T extends Record<string, unknown> = Record<string, unknown>>(
  id: Id | undefined
): UseQueryResult<Node<T>[]> {
  return useQuery<Node<T>[]>("graphdb:getOrderedChildren", id ? { id } : undefined);
}

/** Hook to get deleted nodes (reactive) */
export function useDeletedNodes(): UseQueryResult<Node[]> {
  return useQuery<Node[]>("graphdb:getDeletedNodes", {});
}

/** Hook to get graph statistics (reactive) */
export function useStats(): UseQueryResult<Record<string, unknown>> {
  return useQuery<Record<string, unknown>>("graphdb:stats", {});
}

// --- Connection status hook ---

export function useConnectionStatus(): boolean {
  const client = useGraphDB();
  const [connected, setConnected] = useState(client.isConnected());

  useEffect(() => {
    return client.onConnectionChange(setConnected);
  }, [client]);

  return connected;
}
