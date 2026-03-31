import { describe, it, expect } from "vitest";
import {
  defineSchema,
  defineNode,
  defineEdge,
  s,
  PropertyBuilder,
  NodeTypeBuilder,
  EdgeTypeBuilder,
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

describe("PropertyBuilder", () => {
  it("creates a string property, required by default", () => {
    const prop = s.string()._build("name");
    expect(prop).toEqual({
      name: "name",
      type: "string",
      required: true,
      indexed: false,
      unique: false,
    });
  });

  it("supports optional()", () => {
    const prop = s.string().optional()._build("bio");
    expect(prop.required).toBe(false);
  });

  it("supports indexed()", () => {
    const prop = s.string().indexed()._build("email");
    expect(prop.indexed).toBe(true);
  });

  it("supports unique()", () => {
    const prop = s.string().unique()._build("slug");
    expect(prop.unique).toBe(true);
  });

  it("supports chaining all modifiers", () => {
    const prop = s.string().optional().indexed().unique()._build("code");
    expect(prop.required).toBe(false);
    expect(prop.indexed).toBe(true);
    expect(prop.unique).toBe(true);
  });

  it("supports all types via s helper", () => {
    expect(s.string()._build("a").type).toBe("string");
    expect(s.number()._build("b").type).toBe("number");
    expect(s.boolean()._build("c").type).toBe("boolean");
    expect(s.array()._build("d").type).toBe("array");
    expect(s.object()._build("e").type).toBe("object");
    expect(s.ref()._build("f").type).toBe("ref");
    expect(s.any()._build("g").type).toBe("any");
  });
});

describe("NodeTypeBuilder", () => {
  it("builds a node type with properties", () => {
    const builder = defineNode({
      name: s.string(),
      age: s.number().optional(),
    });
    const def = builder._build("user");
    expect(def.name).toBe("user");
    expect(Object.keys(def.properties)).toHaveLength(2);
    expect(def.properties["name"].required).toBe(true);
    expect(def.properties["age"].required).toBe(false);
  });

  it("supports allowChildren", () => {
    const builder = defineNode({ name: s.string() }).allowChildren(
      "post",
      "comment"
    );
    const def = builder._build("user");
    expect(def.allowedChildren).toEqual(["post", "comment"]);
  });

  it("supports allowParents", () => {
    const builder = defineNode({ name: s.string() }).allowParents("org");
    const def = builder._build("team");
    expect(def.allowedParents).toEqual(["org"]);
  });

  it("omits allowedChildren/Parents when empty", () => {
    const def = defineNode({ name: s.string() })._build("item");
    expect(def.allowedChildren).toBeUndefined();
    expect(def.allowedParents).toBeUndefined();
  });
});

describe("EdgeTypeBuilder", () => {
  it("builds edge with single from/to types", () => {
    const edge = defineEdge("user", "post")._build("authored");
    expect(edge.name).toBe("authored");
    expect(edge.fromTypes).toEqual(["user"]);
    expect(edge.toTypes).toEqual(["post"]);
  });

  it("builds edge with array from/to types", () => {
    const edge = defineEdge(
      ["user", "admin"],
      ["post", "page"]
    )._build("manages");
    expect(edge.fromTypes).toEqual(["user", "admin"]);
    expect(edge.toTypes).toEqual(["post", "page"]);
  });

  it("supports edge properties", () => {
    const edge = defineEdge("user", "user")
      .withProperties({ weight: s.number(), label: s.string().optional() })
      ._build("follows");
    expect(edge.properties).toBeDefined();
    expect(edge.properties!["weight"].required).toBe(true);
    expect(edge.properties!["label"].required).toBe(false);
  });

  it("omits properties when empty", () => {
    const edge = defineEdge("a", "b")._build("link");
    expect(edge.properties).toBeUndefined();
  });
});

describe("defineSchema", () => {
  it("builds a complete schema", () => {
    const schema = defineSchema({
      nodes: {
        user: defineNode({
          name: s.string(),
          email: s.string().unique().indexed(),
        }),
        post: defineNode({
          title: s.string(),
          body: s.string().optional(),
        }),
      },
      edges: {
        authored: defineEdge("user", "post"),
        follows: defineEdge("user", "user"),
      },
    });

    expect(Object.keys(schema.nodeTypes)).toEqual(["user", "post"]);
    expect(Object.keys(schema.edgeTypes)).toEqual(["authored", "follows"]);
    expect(schema.nodeTypes["user"].properties["email"].unique).toBe(true);
    expect(schema.nodeTypes["user"].properties["email"].indexed).toBe(true);
  });

  it("works without edges", () => {
    const schema = defineSchema({
      nodes: {
        item: defineNode({ name: s.string() }),
      },
    });
    expect(Object.keys(schema.edgeTypes)).toHaveLength(0);
  });
});

describe("invariant builders", () => {
  it("uniqueProperty", () => {
    const inv = uniqueProperty("user", "email")._build();
    expect(inv.type).toBe("uniqueness");
    expect(inv.config).toEqual({ nodeType: "user", property: "email" });
    expect(inv.description).toContain("unique");
  });

  it("maxOutEdges", () => {
    const inv = maxOutEdges("user", "follows", 100)._build();
    expect(inv.type).toBe("cardinality");
    expect(inv.config).toEqual({
      nodeType: "user",
      edgeType: "follows",
      direction: "out",
      max: 100,
    });
  });

  it("maxInEdges", () => {
    const inv = maxInEdges("post", "likes", 10000)._build();
    expect(inv.type).toBe("cardinality");
    expect(inv.config.direction).toBe("in");
  });

  it("requiredEdge with default direction", () => {
    const inv = requiredEdge("user", "profile")._build();
    expect(inv.type).toBe("required_edge");
    expect(inv.config.direction).toBe("out");
  });

  it("requiredEdge with explicit direction", () => {
    const inv = requiredEdge("user", "org", "in")._build();
    expect(inv.config.direction).toBe("in");
  });

  it("acyclic", () => {
    const inv = acyclic("depends_on")._build();
    expect(inv.type).toBe("acyclicity");
    expect(inv.config).toEqual({ edgeType: "depends_on" });
  });

  it("maxDepth with nodeType", () => {
    const inv = maxDepth(5, "category")._build();
    expect(inv.type).toBe("hierarchy_depth");
    expect(inv.config).toEqual({ nodeType: "category", maxDepth: 5 });
  });

  it("maxDepth without nodeType", () => {
    const inv = maxDepth(10)._build();
    expect(inv.config.nodeType).toBe("");
  });

  it("maxChildren", () => {
    const inv = maxChildren("team", 50, "member")._build();
    expect(inv.type).toBe("child_count");
    expect(inv.config).toEqual({
      parentType: "team",
      childType: "member",
      max: 50,
    });
  });

  it("maxChildren without childType", () => {
    const inv = maxChildren("folder", 100)._build();
    expect(inv.config.childType).toBe("");
  });

  it("minChildren", () => {
    const inv = minChildren("team", 1, "member")._build();
    expect(inv.type).toBe("child_count");
    expect(inv.config).toEqual({
      parentType: "team",
      childType: "member",
      min: 1,
    });
  });

  it("minChildren without childType", () => {
    const inv = minChildren("org", 1)._build();
    expect(inv.config.childType).toBe("");
  });

  it("edgeConstraint", () => {
    const inv = edgeConstraint(
      "manages",
      ["admin", "manager"],
      ["user"]
    )._build();
    expect(inv.type).toBe("edge_constraint");
    expect(inv.config).toEqual({
      edgeType: "manages",
      fromTypes: ["admin", "manager"],
      toTypes: ["user"],
    });
    expect(inv.description).toContain("admin|manager");
  });
});
