/**
 * Function definition helpers for GraphDB.
 *
 * Provides a Convex-compatible API for defining queries, mutations, and actions
 * that run on the server.
 *
 * @example
 * ```ts
 * import { query, mutation, action } from "@graphdb/sdk";
 *
 * export const listUsers = query(async ({ graph }) => {
 *   return await graph.getByType("user");
 * });
 *
 * export const createUser = mutation(async ({ graph }, { name, email }) => {
 *   return await graph.insertNode("user", { name, email });
 * });
 *
 * export const importUsers = action(async ({ runMutation }, { users }) => {
 *   for (const user of users) {
 *     await runMutation("createUser", user);
 *   }
 * });
 * ```
 */

import type { QueryCtx, MutationCtx, ActionCtx } from "./types";

/** A query function definition */
export interface QueryFunction<Args = Record<string, unknown>, Returns = unknown> {
  _type: "query";
  _handler: (ctx: QueryCtx, args: Args) => Promise<Returns>;
  _args?: ArgValidator<Args>;
}

/** A mutation function definition */
export interface MutationFunction<Args = Record<string, unknown>, Returns = unknown> {
  _type: "mutation";
  _handler: (ctx: MutationCtx, args: Args) => Promise<Returns>;
  _args?: ArgValidator<Args>;
}

/** An action function definition */
export interface ActionFunction<Args = Record<string, unknown>, Returns = unknown> {
  _type: "action";
  _handler: (ctx: ActionCtx, args: Args) => Promise<Returns>;
  _args?: ArgValidator<Args>;
}

type AnyFunction = QueryFunction | MutationFunction | ActionFunction;

/** Argument validator (simplified version of Convex's `v` validators) */
export interface ArgValidator<T = unknown> {
  validate(args: unknown): T;
}

// --- Query definition ---

interface QueryOptions<Args, Returns> {
  args?: ArgValidator<Args>;
  handler: (ctx: QueryCtx, args: Args) => Promise<Returns>;
}

/**
 * Define a query function.
 * Queries are read-only and automatically reactive when used with useQuery.
 */
export function query<Args extends Record<string, unknown> = Record<string, unknown>, Returns = unknown>(
  handlerOrOptions: ((ctx: QueryCtx, args: Args) => Promise<Returns>) | QueryOptions<Args, Returns>
): QueryFunction<Args, Returns> {
  if (typeof handlerOrOptions === "function") {
    return {
      _type: "query",
      _handler: handlerOrOptions,
    };
  }
  return {
    _type: "query",
    _handler: handlerOrOptions.handler,
    _args: handlerOrOptions.args,
  };
}

// --- Mutation definition ---

interface MutationOptions<Args, Returns> {
  args?: ArgValidator<Args>;
  handler: (ctx: MutationCtx, args: Args) => Promise<Returns>;
}

/**
 * Define a mutation function.
 * Mutations can read and write, and are transactional.
 * Invariants are validated after every mutation.
 */
export function mutation<Args extends Record<string, unknown> = Record<string, unknown>, Returns = unknown>(
  handlerOrOptions: ((ctx: MutationCtx, args: Args) => Promise<Returns>) | MutationOptions<Args, Returns>
): MutationFunction<Args, Returns> {
  if (typeof handlerOrOptions === "function") {
    return {
      _type: "mutation",
      _handler: handlerOrOptions,
    };
  }
  return {
    _type: "mutation",
    _handler: handlerOrOptions.handler,
    _args: handlerOrOptions.args,
  };
}

// --- Action definition ---

interface ActionOptions<Args, Returns> {
  args?: ArgValidator<Args>;
  handler: (ctx: ActionCtx, args: Args) => Promise<Returns>;
}

/**
 * Define an action function.
 * Actions can do I/O (fetch, etc.) and call other queries/mutations.
 * They are NOT reactive and NOT transactional.
 */
export function action<Args extends Record<string, unknown> = Record<string, unknown>, Returns = unknown>(
  handlerOrOptions: ((ctx: ActionCtx, args: Args) => Promise<Returns>) | ActionOptions<Args, Returns>
): ActionFunction<Args, Returns> {
  if (typeof handlerOrOptions === "function") {
    return {
      _type: "action",
      _handler: handlerOrOptions,
    };
  }
  return {
    _type: "action",
    _handler: handlerOrOptions.handler,
    _args: handlerOrOptions.args,
  };
}

// --- Type helpers for referencing functions ---

/** Extract the return type of a query function */
export type QueryReturn<Q extends QueryFunction<any, any>> =
  Q extends QueryFunction<any, infer R> ? R : never;

/** Extract the args type of a query function */
export type QueryArgs<Q extends QueryFunction<any, any>> =
  Q extends QueryFunction<infer A, any> ? A : never;

/** Extract the return type of a mutation function */
export type MutationReturn<M extends MutationFunction<any, any>> =
  M extends MutationFunction<any, infer R> ? R : never;

/** Extract the args type of a mutation function */
export type MutationArgs<M extends MutationFunction<any, any>> =
  M extends MutationFunction<infer A, any> ? A : never;

/** Extract the return type of an action function */
export type ActionReturn<A extends ActionFunction<any, any>> =
  A extends ActionFunction<any, infer R> ? R : never;

/** Extract the args type of an action function */
export type ActionArgs<A extends ActionFunction<any, any>> =
  A extends ActionFunction<infer Ar, any> ? Ar : never;

/** Union of all function types */
export type FunctionReference = QueryFunction | MutationFunction | ActionFunction;
