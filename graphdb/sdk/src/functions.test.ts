import { describe, it, expect } from "vitest";
import { query, mutation, action } from "./functions";
import type {
  QueryFunction,
  MutationFunction,
  ActionFunction,
  QueryReturn,
  QueryArgs,
  MutationReturn,
  MutationArgs,
  ActionReturn,
  ActionArgs,
  FunctionReference,
} from "./functions";
import type { QueryCtx, MutationCtx, ActionCtx } from "./types";

describe("query", () => {
  it("creates a query from a handler function", () => {
    const q = query(async (ctx, args) => {
      return "result";
    });
    expect(q._type).toBe("query");
    expect(q._handler).toBeInstanceOf(Function);
    expect(q._args).toBeUndefined();
  });

  it("creates a query from an options object", () => {
    const validator = { validate: (args: unknown) => args as { id: string } };
    const q = query({
      args: validator,
      handler: async (ctx, args) => {
        return args.id;
      },
    });
    expect(q._type).toBe("query");
    expect(q._args).toBe(validator);
  });

  it("handler is callable", async () => {
    const q = query(async (_ctx, args: { x: number }) => args.x * 2);
    const result = await q._handler({} as QueryCtx, { x: 5 });
    expect(result).toBe(10);
  });
});

describe("mutation", () => {
  it("creates a mutation from a handler function", () => {
    const m = mutation(async (ctx, args) => {
      return "created";
    });
    expect(m._type).toBe("mutation");
    expect(m._handler).toBeInstanceOf(Function);
    expect(m._args).toBeUndefined();
  });

  it("creates a mutation from an options object", () => {
    const validator = {
      validate: (args: unknown) => args as { name: string },
    };
    const m = mutation({
      args: validator,
      handler: async (ctx, args) => args.name,
    });
    expect(m._type).toBe("mutation");
    expect(m._args).toBe(validator);
  });

  it("handler is callable", async () => {
    const m = mutation(
      async (_ctx, args: { name: string }) => `Hello ${args.name}`
    );
    const result = await m._handler({} as MutationCtx, { name: "World" });
    expect(result).toBe("Hello World");
  });
});

describe("action", () => {
  it("creates an action from a handler function", () => {
    const a = action(async (ctx, args) => {
      return "done";
    });
    expect(a._type).toBe("action");
    expect(a._handler).toBeInstanceOf(Function);
    expect(a._args).toBeUndefined();
  });

  it("creates an action from an options object", () => {
    const validator = { validate: (args: unknown) => args as { url: string } };
    const a = action({
      args: validator,
      handler: async (ctx, args) => args.url,
    });
    expect(a._type).toBe("action");
    expect(a._args).toBe(validator);
  });

  it("handler is callable", async () => {
    const a = action(async (_ctx, args: { n: number }) => args.n + 1);
    const result = await a._handler({} as ActionCtx, { n: 41 });
    expect(result).toBe(42);
  });
});

describe("type helpers", () => {
  it("type inference works correctly", () => {
    // These are compile-time checks; if they compile, the types work
    const q = query(async (_ctx, args: { id: string }) => ({ name: "test" }));

    // Type assertions (compile-time only)
    type QR = QueryReturn<typeof q>;
    type QA = QueryArgs<typeof q>;

    const m = mutation(async (_ctx, args: { name: string }) => "id-123");
    type MR = MutationReturn<typeof m>;
    type MA = MutationArgs<typeof m>;

    const a = action(async (_ctx, args: { url: string }) => true);
    type AR = ActionReturn<typeof a>;
    type AA = ActionArgs<typeof a>;

    // All function types are FunctionReferences
    const refs: FunctionReference[] = [q, m, a];
    expect(refs).toHaveLength(3);
  });
});
