import { describe, it, expect } from "vitest";
import {
  OpType,
  GossipMessageType,
} from "./types";
import type {
  Id,
  Node,
  Edge,
  EventId,
  Operation,
  FuncResult,
  FuncDef,
  ClientMessage,
  ServerMessage,
  PropertyDef,
  NodeTypeDef,
  EdgeTypeDef,
  Schema,
  InvariantDef,
  QueryCtx,
  MutationCtx,
  ActionCtx,
  GraphReader,
  GraphWriter,
  DerivedNode,
  DerivedEdge,
  BatchOp,
  BatchResult,
  VersionVector,
  GossipMessage,
  PeerInfo,
  DeltaPayload,
  ClusterStats,
  ShardInfo,
} from "./types";

describe("OpType enum", () => {
  it("has correct values", () => {
    expect(OpType.InsertNode).toBe(0);
    expect(OpType.DeleteNode).toBe(1);
    expect(OpType.SetProperty).toBe(2);
    expect(OpType.DeleteProperty).toBe(3);
    expect(OpType.InsertEdge).toBe(4);
    expect(OpType.DeleteEdge).toBe(5);
    expect(OpType.MoveNode).toBe(6);
    expect(OpType.ReorderNode).toBe(7);
    expect(OpType.RestoreNode).toBe(8);
    expect(OpType.RestoreEdge).toBe(9);
  });
});

describe("GossipMessageType enum", () => {
  it("has correct values", () => {
    expect(GossipMessageType.Offer).toBe(0);
    expect(GossipMessageType.Delta).toBe(3);
    expect(GossipMessageType.Heartbeat).toBe(7);
    expect(GossipMessageType.DeadLetter).toBe(8);
  });
});

describe("type contracts (compile-time checks)", () => {
  it("Node interface works", () => {
    const node: Node<{ name: string }> = {
      id: "123",
      type: "user",
      properties: { name: "Alice" },
      position: "M",
      createdAt: "2024-01-01",
      updatedAt: "2024-01-01",
    };
    expect(node.id).toBe("123");
    expect(node.properties.name).toBe("Alice");
  });

  it("Edge interface works", () => {
    const edge: Edge = {
      id: "e1",
      type: "follows",
      fromId: "a",
      toId: "b",
      properties: {},
      createdAt: "2024-01-01",
    };
    expect(edge.type).toBe("follows");
  });

  it("Operation interface works", () => {
    const op: Operation = {
      id: { replicaId: "r1", seq: 1 },
      parents: [],
      type: OpType.InsertNode,
      targetId: "123",
      nodeType: "user",
      timestamp: "2024-01-01",
    };
    expect(op.type).toBe(OpType.InsertNode);
  });

  it("FuncResult interface works", () => {
    const result: FuncResult<string> = { value: "hello" };
    expect(result.value).toBe("hello");

    const errResult: FuncResult = { error: "failed" };
    expect(errResult.error).toBe("failed");
  });

  it("BatchOp interface works", () => {
    const op: BatchOp = { op: "insert-node", nodeType: "user" };
    expect(op.op).toBe("insert-node");
  });

  it("DerivedNode interface works", () => {
    const dn: DerivedNode<{ count: number }> = {
      id: "d1",
      derivedType: "summary",
      properties: { count: 42 },
      createdAt: "2024-01-01",
      updatedAt: "2024-01-01",
    };
    expect(dn.properties.count).toBe(42);
  });

  it("GossipMessage interface works", () => {
    const msg: GossipMessage = {
      type: GossipMessageType.Heartbeat,
      from: "r1",
      payload: null,
      timestamp: "2024-01-01",
      seqNo: 1,
    };
    expect(msg.type).toBe(GossipMessageType.Heartbeat);
  });

  it("VersionVector type works", () => {
    const vv: VersionVector = { r1: 5, r2: 3 };
    expect(vv.r1).toBe(5);
  });

  it("Schema type works", () => {
    const schema: Schema = {
      nodeTypes: {
        user: {
          name: "user",
          properties: {
            name: { name: "name", type: "string", required: true },
          },
        },
      },
      edgeTypes: {
        follows: {
          name: "follows",
          fromTypes: ["user"],
          toTypes: ["user"],
        },
      },
    };
    expect(Object.keys(schema.nodeTypes)).toHaveLength(1);
  });
});
