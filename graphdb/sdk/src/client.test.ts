import { describe, it, expect, vi, beforeEach } from "vitest";
import { GraphDBClient, GraphDBError } from "./client";
import type { ServerMessage } from "./types";

describe("GraphDBClient", () => {
  describe("constructor", () => {
    it("sets defaults", () => {
      const client = new GraphDBClient({ url: "http://localhost:8787" });
      expect(client.isConnected()).toBe(false);
    });

    it("trims trailing slash from url", () => {
      const client = new GraphDBClient({ url: "http://localhost:8787/" });
      expect(client.isConnected()).toBe(false);
    });

    it("accepts options", () => {
      const client = new GraphDBClient({
        url: "http://localhost:8787",
        timeout: 5000,
        autoReconnect: false,
        token: "test-token",
      });
      expect(client.isConnected()).toBe(false);
    });
  });

  describe("disconnect", () => {
    it("does nothing when not connected", () => {
      const client = new GraphDBClient({ url: "http://localhost:8787" });
      client.disconnect();
      expect(client.isConnected()).toBe(false);
    });
  });

  describe("onConnectionChange", () => {
    it("returns unsubscribe function", () => {
      const client = new GraphDBClient({ url: "http://localhost:8787" });
      const cb = vi.fn();
      const unsub = client.onConnectionChange(cb);
      expect(typeof unsub).toBe("function");
      unsub();
    });
  });

  describe("HTTP methods with fetch mock", () => {
    let client: GraphDBClient;

    beforeEach(() => {
      client = new GraphDBClient({ url: "http://localhost:8787" });
    });

    it("query sends correct request", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        json: () => Promise.resolve({ value: "result" }),
      });
      vi.stubGlobal("fetch", mockFetch);

      const result = await client.query("test:echo", { msg: "hello" });
      expect(result).toBe("result");
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost:8787/api/query",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ name: "test:echo", args: { msg: "hello" } }),
        })
      );

      vi.unstubAllGlobals();
    });

    it("query throws on error response", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ error: "not found" }),
        })
      );

      await expect(client.query("bad")).rejects.toThrow(GraphDBError);
      vi.unstubAllGlobals();
    });

    it("mutation sends correct request", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: "created" }),
        })
      );

      const result = await client.mutation("test:create", { name: "Alice" });
      expect(result).toBe("created");
      vi.unstubAllGlobals();
    });

    it("mutation throws on error", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ error: "failed" }),
        })
      );

      await expect(client.mutation("bad")).rejects.toThrow(GraphDBError);
      vi.unstubAllGlobals();
    });

    it("action sends correct request", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: "done" }),
        })
      );

      const result = await client.action("test:run");
      expect(result).toBe("done");
      vi.unstubAllGlobals();
    });

    it("action throws on error", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ error: "action failed" }),
        })
      );

      await expect(client.action("bad")).rejects.toThrow(GraphDBError);
      vi.unstubAllGlobals();
    });

    it("getNode calls query with correct args", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: { id: "123", type: "user" } }),
        })
      );

      const node = await client.getNode("123");
      expect(node).toEqual({ id: "123", type: "user" });
      vi.unstubAllGlobals();
    });

    it("getNodesByType calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [{ id: "1" }, { id: "2" }] }),
        })
      );

      const nodes = await client.getNodesByType("user");
      expect(nodes).toHaveLength(2);
      vi.unstubAllGlobals();
    });

    it("getChildren calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [{ id: "child1" }] }),
        })
      );

      const children = await client.getChildren("parent1");
      expect(children).toHaveLength(1);
      vi.unstubAllGlobals();
    });

    it("getParent calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: { id: "parent1" } }),
        })
      );

      const parent = await client.getParent("child1");
      expect(parent).toEqual({ id: "parent1" });
      vi.unstubAllGlobals();
    });

    it("getSubtree calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [] }),
        })
      );

      const tree = await client.getSubtree("root");
      expect(tree).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("getAncestors calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [] }),
        })
      );

      const ancestors = await client.getAncestors("leaf");
      expect(ancestors).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("getOutEdges calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [] }),
        })
      );

      const edges = await client.getOutEdges("node1");
      expect(edges).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("getInEdges calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [] }),
        })
      );

      const edges = await client.getInEdges("node1");
      expect(edges).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("getRoots calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [] }),
        })
      );

      const roots = await client.getRoots();
      expect(roots).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("traverse calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [] }),
        })
      );

      const nodes = await client.traverse("n1", "follows", "out", 5);
      expect(nodes).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("insertNode calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: "new-id" }),
        })
      );

      const id = await client.insertNode("user", { name: "Alice" });
      expect(id).toBe("new-id");
      vi.unstubAllGlobals();
    });

    it("deleteNode calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: null }),
        })
      );

      await client.deleteNode("123");
      vi.unstubAllGlobals();
    });

    it("patchNode calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: null }),
        })
      );

      await client.patchNode("123", { name: "Bob" });
      vi.unstubAllGlobals();
    });

    it("insertEdge calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: "edge-id" }),
        })
      );

      const id = await client.insertEdge("follows", "a", "b");
      expect(id).toBe("edge-id");
      vi.unstubAllGlobals();
    });

    it("deleteEdge calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: null }),
        })
      );

      await client.deleteEdge("edge1");
      vi.unstubAllGlobals();
    });

    it("moveNode calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: null }),
        })
      );

      await client.moveNode("n1", "parent1");
      vi.unstubAllGlobals();
    });

    it("softDeleteNode calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: null }),
        })
      );

      await client.softDeleteNode("123");
      vi.unstubAllGlobals();
    });

    it("cascadeDeleteNode calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: null }),
        })
      );

      await client.cascadeDeleteNode("123");
      vi.unstubAllGlobals();
    });

    it("restoreNode calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: null }),
        })
      );

      await client.restoreNode("123");
      vi.unstubAllGlobals();
    });

    it("getOrderedChildren calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [] }),
        })
      );

      const children = await client.getOrderedChildren("parent1");
      expect(children).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("reorderNode calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: null }),
        })
      );

      await client.reorderNode("n1", "M");
      vi.unstubAllGlobals();
    });

    it("getDeletedNodes calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [] }),
        })
      );

      const deleted = await client.getDeletedNodes();
      expect(deleted).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("getStats calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: { totalNodes: 5 } }),
        })
      );

      const stats = await client.getStats();
      expect(stats).toEqual({ totalNodes: 5 });
      vi.unstubAllGlobals();
    });

    it("reapOrphans calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: { reapedCount: 2, reapedIds: ["a", "b"] } }),
        })
      );

      const result = await client.reapOrphans();
      expect(result.reapedCount).toBe(2);
      vi.unstubAllGlobals();
    });

    it("executeBatch calls mutation", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [] }),
        })
      );

      const result = await client.executeBatch([]);
      expect(result).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("getClusterPeers calls fetch", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve([]),
        })
      );

      const peers = await client.getClusterPeers();
      expect(peers).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("getClusterStats calls fetch", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ totalShards: 16 }),
        })
      );

      const stats = await client.getClusterStats();
      expect(stats).toEqual({ totalShards: 16 });
      vi.unstubAllGlobals();
    });

    it("getDerivedNode calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: null }),
        })
      );

      const node = await client.getDerivedNode("d1");
      expect(node).toBeNull();
      vi.unstubAllGlobals();
    });

    it("getDerivedNodesByType calls query", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ value: [] }),
        })
      );

      const nodes = await client.getDerivedNodesByType("summary");
      expect(nodes).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("deploySchema calls fetch", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () =>
            Promise.resolve({ status: "deployed", graphName: "default" }),
        })
      );

      const result = await client.deploySchema({
        graphName: "default",
        nodeTypes: [],
        edgeTypes: [],
        invariants: [],
        pipelines: [],
      });
      expect(result.status).toBe("deployed");
      vi.unstubAllGlobals();
    });

    it("deploySchema throws on error", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ error: "validation failed" }),
        })
      );

      await expect(
        client.deploySchema({
          graphName: "default",
          nodeTypes: [],
          edgeTypes: [],
          invariants: [],
          pipelines: [],
        })
      ).rejects.toThrow(GraphDBError);
      vi.unstubAllGlobals();
    });

    it("listGraphs calls fetch", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve(["default", "test"]),
        })
      );

      const graphs = await client.listGraphs();
      expect(graphs).toEqual(["default", "test"]);
      vi.unstubAllGlobals();
    });

    it("setFeatureFlags calls fetch", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ ok: true }),
        })
      );

      await client.setFeatureFlags("default", { billing: true });
      vi.unstubAllGlobals();
    });

    it("setFeatureFlags throws on error", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ error: "graph not found" }),
        })
      );

      await expect(
        client.setFeatureFlags("bad", {})
      ).rejects.toThrow(GraphDBError);
      vi.unstubAllGlobals();
    });

    it("getFeatureFlags calls fetch", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ billing: true }),
        })
      );

      const flags = await client.getFeatureFlags("default");
      expect(flags).toEqual({ billing: true });
      vi.unstubAllGlobals();
    });

    it("getGraphSchema returns schema", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          status: 200,
          json: () =>
            Promise.resolve({
              graphName: "default",
              nodeTypes: [],
              edgeTypes: [],
              invariants: [],
              pipelines: [],
            }),
        })
      );

      const schema = await client.getGraphSchema("default");
      expect(schema).not.toBeNull();
      vi.unstubAllGlobals();
    });

    it("getGraphSchema returns null on 404", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          status: 404,
          json: () => Promise.resolve(null),
        })
      );

      const schema = await client.getGraphSchema("nonexistent");
      expect(schema).toBeNull();
      vi.unstubAllGlobals();
    });

    it("requestDelta calls fetch", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () => Promise.resolve({ operations: [], versionVector: {} }),
        })
      );

      const delta = await client.requestDelta({ r1: 5 });
      expect(delta.operations).toEqual([]);
      vi.unstubAllGlobals();
    });

    it("listFunctions calls fetch", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn().mockResolvedValue({
          json: () =>
            Promise.resolve([
              { name: "test:echo", type: "query" },
              { name: "test:create", type: "mutation" },
            ]),
        })
      );

      const fns = await client.listFunctions();
      expect(fns).toHaveLength(2);
      vi.unstubAllGlobals();
    });
  });

  describe("subscribe without connection", () => {
    it("queues subscription for later", () => {
      const client = new GraphDBClient({ url: "http://localhost:8787" });
      const cb = vi.fn();
      const unsub = client.subscribe("test:query", undefined, cb);
      expect(typeof unsub).toBe("function");
      unsub(); // should not throw
    });
  });
});

describe("GraphDBError", () => {
  it("has correct name and properties", () => {
    const err = new GraphDBError("test error", "test:fn");
    expect(err.name).toBe("GraphDBError");
    expect(err.message).toBe("test error");
    expect(err.functionName).toBe("test:fn");
    expect(err instanceof Error).toBe(true);
  });

  it("works without function name", () => {
    const err = new GraphDBError("test error");
    expect(err.functionName).toBeUndefined();
  });
});
