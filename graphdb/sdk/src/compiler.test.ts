import { describe, it, expect } from "vitest";
import {
  compileSchema,
  serializeSchema,
  SchemaCompiler,
  NodeCompiler,
  PipelineCompiler,
  ModuleCompiler,
} from "./compiler";
import type {
  CompiledSchema,
  CompiledModule,
  CompiledNodeType,
  CompiledProperty,
  SerializableTransform,
} from "./compiler";

describe("compileSchema", () => {
  it("compiles a basic schema with nodes and edges", () => {
    const schema = compileSchema("my-app", (s) => {
      s.node("user", (n) => {
        n.string("name", { required: true });
        n.string("email", { required: true, unique: true, indexed: true });
        n.number("age");
      });
      s.node("post", (n) => {
        n.string("title", { required: true });
        n.string("body");
        n.boolean("published");
      });
      s.edge("authored", ["user"], ["post"]);
      s.edge("follows", ["user"], ["user"]);
    });

    expect(schema.version).toBe(1);
    expect(schema.graphName).toBe("my-app");
    expect(schema.nodeTypes).toHaveLength(2);
    expect(schema.edgeTypes).toHaveLength(2);

    const user = schema.nodeTypes[0];
    expect(user.name).toBe("user");
    expect(user.properties["name"]).toEqual({
      type: "string",
      required: true,
      indexed: false,
      unique: false,
    });
    expect(user.properties["email"]).toEqual({
      type: "string",
      required: true,
      indexed: true,
      unique: true,
    });
    expect(user.properties["age"]).toEqual({
      type: "number",
      required: false,
      indexed: false,
      unique: false,
    });

    const post = schema.nodeTypes[1];
    expect(post.properties["published"]).toEqual({
      type: "boolean",
      required: false,
      indexed: false,
      unique: false,
    });
  });

  it("compiles invariants", () => {
    const schema = compileSchema("test", (s) => {
      s.node("user", (n) => {
        n.string("email");
      });
      s.edge("follows", ["user"], ["user"]);
      s.unique("user", "email");
      s.acyclic("follows");
      s.maxCardinality("user", "follows", "out", 100);
      s.maxDepth("user", 5);
      s.childCount("user", { childType: "post", min: 0, max: 10 });
    });

    expect(schema.invariants).toHaveLength(5);

    const unique = schema.invariants[0];
    expect(unique.type).toBe("uniqueness");
    expect(unique.config).toEqual({ nodeType: "user", property: "email" });

    const acyclic = schema.invariants[1];
    expect(acyclic.type).toBe("acyclicity");
    expect(acyclic.config).toEqual({ edgeType: "follows" });

    const card = schema.invariants[2];
    expect(card.type).toBe("cardinality");
    expect(card.config).toEqual({
      nodeType: "user",
      edgeType: "follows",
      direction: "out",
      max: 100,
    });

    const depth = schema.invariants[3];
    expect(depth.type).toBe("hierarchy_depth");
    expect(depth.config).toEqual({ nodeType: "user", maxDepth: 5 });

    const childCount = schema.invariants[4];
    expect(childCount.type).toBe("child_count");
    expect(childCount.config).toEqual({
      parentType: "user",
      childType: "post",
      min: 0,
      max: 10,
    });
  });

  it("compiles pipelines", () => {
    const schema = compileSchema("test", (s) => {
      s.node("user", (n) => n.string("name"));
      s.pipeline("user-summary", "User Summaries", (p) => {
        p.map("user", "user_summary", {
          propertyMap: { displayName: "name" },
          propertyDefaults: { verified: false },
          computedProperties: { slug: "toLowerCase(name)" },
          includeProperties: ["name"],
          excludeProperties: ["internal"],
          conditions: ["name != ''"],
        });
        p.join("user", "user_with_posts");
      });
    });

    expect(schema.pipelines).toHaveLength(1);
    const pipeline = schema.pipelines[0];
    expect(pipeline.id).toBe("user-summary");
    expect(pipeline.stages).toHaveLength(2);

    const mapStage = pipeline.stages[0];
    expect(mapStage.type).toBe("map");
    expect(mapStage.sourceType).toBe("user");
    expect(mapStage.derivedType).toBe("user_summary");
    expect(mapStage.transform?.propertyMap).toEqual({ displayName: "name" });
    expect(mapStage.transform?.propertyDefaults).toEqual({ verified: false });
    expect(mapStage.transform?.computedProperties).toEqual({
      slug: "toLowerCase(name)",
    });
    expect(mapStage.transform?.includeProperties).toEqual(["name"]);
    expect(mapStage.transform?.excludeProperties).toEqual(["internal"]);
    expect(mapStage.transform?.conditions).toEqual(["name != ''"]);

    const joinStage = pipeline.stages[1];
    expect(joinStage.type).toBe("join");
  });
});

describe("NodeCompiler", () => {
  it("supports all property types", () => {
    const nc = new NodeCompiler("test");
    nc.string("s")
      .number("n")
      .boolean("b")
      .ref("r")
      .array("a")
      .object("o")
      .any("x");

    const built = nc.build();
    expect(built.properties["s"].type).toBe("string");
    expect(built.properties["n"].type).toBe("number");
    expect(built.properties["b"].type).toBe("boolean");
    expect(built.properties["r"].type).toBe("ref");
    expect(built.properties["a"].type).toBe("array");
    expect(built.properties["o"].type).toBe("object");
    expect(built.properties["x"].type).toBe("any");
  });

  it("supports children and parents constraints", () => {
    const nc = new NodeCompiler("folder");
    nc.children("file", "folder");
    nc.parents("folder");

    const built = nc.build();
    expect(built.allowedChildren).toEqual(["file", "folder"]);
    expect(built.allowedParents).toEqual(["folder"]);
  });

  it("defaults all options to false", () => {
    const nc = new NodeCompiler("test");
    nc.string("name");
    const built = nc.build();
    expect(built.properties["name"].required).toBe(false);
    expect(built.properties["name"].indexed).toBe(false);
    expect(built.properties["name"].unique).toBe(false);
  });
});

describe("SchemaCompiler", () => {
  it("supports method chaining", () => {
    const compiler = new SchemaCompiler();
    const result = compiler
      .node("user", (n) => n.string("name"))
      .edge("follows", ["user"], ["user"])
      .unique("user", "name")
      .acyclic("follows")
      .maxCardinality("user", "follows", "in", 50)
      .maxDepth("user", 3)
      .childCount("user", { min: 1 });

    expect(result).toBe(compiler);
  });

  it("childCount without childType defaults to 'any'", () => {
    const schema = compileSchema("test", (s) => {
      s.childCount("parent", { max: 5 });
    });
    expect(schema.invariants[0].id).toContain("any");
  });
});

describe("ModuleCompiler", () => {
  it("builds a module with namespace prefixing", () => {
    const mc = new ModuleCompiler("users");
    mc.namespace("users")
      .name("User System")
      .node("profile", (n) => n.string("name", { required: true }))
      .edge("follows", ["users:profile"], ["users:profile"])
      .unique("profile", "name")
      .acyclic("follows")
      .maxCardinality("profile", "follows", "out", 100);

    const module = mc.build();
    expect(module.id).toBe("users");
    expect(module.name).toBe("User System");
    expect(module.namespace).toBe("users");
    expect(module.nodeTypes[0].name).toBe("users:profile");
    expect(module.invariants[0].config).toEqual({
      nodeType: "users:profile",
      property: "name",
    });
    expect(module.invariants[1].type).toBe("acyclicity");
    expect(module.invariants[2].config).toEqual({
      nodeType: "users:profile",
      edgeType: "follows",
      direction: "out",
      max: 100,
    });
  });

  it("supports conditional activation with feature flags", () => {
    const mc = new ModuleCompiler("premium");
    mc.whenFeatureFlags("premium", "v2");
    const module = mc.build();
    expect(module.condition?.featureFlags).toEqual(["premium", "v2"]);
  });

  it("supports conditional activation with node type prefix", () => {
    const mc = new ModuleCompiler("billing");
    mc.whenNodeTypePrefix("billing:");
    const module = mc.build();
    expect(module.condition?.nodeTypePrefix).toBe("billing:");
  });

  it("supports custom condition", () => {
    const mc = new ModuleCompiler("test");
    mc.when({ expression: "graph.nodeCount > 100" });
    const module = mc.build();
    expect(module.condition?.expression).toBe("graph.nodeCount > 100");
  });

  it("supports dependencies", () => {
    const mc = new ModuleCompiler("billing");
    mc.dependsOn("users", "core");
    const module = mc.build();
    expect(module.dependsOn).toEqual(["users", "core"]);
  });

  it("omits dependsOn when empty", () => {
    const mc = new ModuleCompiler("standalone");
    const module = mc.build();
    expect(module.dependsOn).toBeUndefined();
  });

  it("supports pipelines with module-scoped IDs", () => {
    const mc = new ModuleCompiler("analytics");
    mc.pipeline("summary", "Summaries", (p) => {
      p.map("event", "event_summary");
    });
    const module = mc.build();
    expect(module.pipelines[0].id).toBe("analytics-summary");
  });

  it("works without namespace", () => {
    const mc = new ModuleCompiler("simple");
    mc.node("item", (n) => n.string("name"));
    mc.unique("item", "name");
    const module = mc.build();
    expect(module.nodeTypes[0].name).toBe("item");
    expect(module.invariants[0].config).toEqual({
      nodeType: "item",
      property: "name",
    });
  });
});

describe("compileSchema with modules", () => {
  it("includes modules in compiled output", () => {
    const schema = compileSchema("app", (s) => {
      s.module("users", (m) => {
        m.namespace("users");
        m.node("profile", (n) => n.string("name", { required: true }));
      });
      s.module("billing", (m) => {
        m.namespace("billing");
        m.dependsOn("users");
        m.node("invoice", (n) => n.number("amount", { required: true }));
        m.edge("billed_to", ["billing:invoice"], ["users:profile"]);
      });
    });

    expect(schema.modules).toHaveLength(2);
    expect(schema.modules![0].id).toBe("users");
    expect(schema.modules![1].id).toBe("billing");
    expect(schema.modules![1].dependsOn).toEqual(["users"]);
  });

  it("omits modules array when empty", () => {
    const schema = compileSchema("simple", (s) => {
      s.node("item", (n) => n.string("name"));
    });
    expect(schema.modules).toBeUndefined();
  });
});

describe("serializeSchema", () => {
  it("produces valid JSON", () => {
    const schema = compileSchema("test", (s) => {
      s.node("user", (n) => {
        n.string("name", { required: true });
      });
      s.edge("follows", ["user"], ["user"]);
      s.unique("user", "name");
    });

    const json = serializeSchema(schema);
    const parsed = JSON.parse(json) as CompiledSchema;

    expect(parsed.version).toBe(1);
    expect(parsed.graphName).toBe("test");
    expect(parsed.nodeTypes).toHaveLength(1);
    expect(parsed.edgeTypes).toHaveLength(1);
    expect(parsed.invariants).toHaveLength(1);
  });

  it("round-trips through JSON", () => {
    const original = compileSchema("roundtrip", (s) => {
      s.node("a", (n) => {
        n.string("x", { required: true, indexed: true, unique: true });
        n.number("y");
        n.boolean("z");
        n.ref("r");
        n.array("arr");
        n.object("obj");
        n.any("wild");
      });
      s.edge("link", ["a"], ["a"]);
      s.unique("a", "x");
      s.acyclic("link");
      s.pipeline("p1", "Test Pipeline", (p) => {
        p.map("a", "a_derived", { propertyMap: { foo: "x" } });
        p.join("a", "a_joined");
      });
      s.module("mod", (m) => {
        m.namespace("ns");
        m.node("item", (n) => n.string("val"));
        m.whenFeatureFlags("flag1");
        m.dependsOn("other");
      });
    });

    const json = serializeSchema(original);
    const parsed = JSON.parse(json) as CompiledSchema;

    expect(parsed.nodeTypes[0].properties["x"]).toEqual({
      type: "string",
      required: true,
      indexed: true,
      unique: true,
    });
    expect(parsed.pipelines[0].stages).toHaveLength(2);
    expect(parsed.modules![0].namespace).toBe("ns");
    expect(parsed.modules![0].condition?.featureFlags).toEqual(["flag1"]);
  });
});

describe("PipelineCompiler", () => {
  it("supports map and join stages", () => {
    const pc = new PipelineCompiler("p1", "Pipeline 1");
    pc.map("source", "derived", { propertyMap: { a: "b" } });
    pc.join("source2", "derived2");

    const built = pc.build();
    expect(built.id).toBe("p1");
    expect(built.name).toBe("Pipeline 1");
    expect(built.stages).toHaveLength(2);
    expect(built.stages[0].type).toBe("map");
    expect(built.stages[1].type).toBe("join");
  });

  it("map stage without transform", () => {
    const pc = new PipelineCompiler("p2", "Simple");
    pc.map("a", "b");
    const built = pc.build();
    expect(built.stages[0].transform).toBeUndefined();
  });
});
