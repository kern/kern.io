/**
 * Schema compiler for GraphDB.
 *
 * Compiles TypeScript schema definitions into a JSON format that can be
 * loaded by the server at runtime. This enables defining your entire
 * graph schema in TypeScript and deploying it to the server.
 *
 * Usage:
 *   const schema = compileSchema("my-app", (s) => {
 *     s.node("user", (n) => {
 *       n.string("name", { required: true });
 *       n.string("email", { required: true, unique: true, indexed: true });
 *       n.number("age");
 *     });
 *     s.edge("follows", ["user"], ["user"]);
 *     s.unique("user", "email");
 *     s.acyclic("depends_on");
 *   });
 *
 *   // Deploy to server
 *   await client.deploySchema(schema);
 */

// --- Compiled Schema Types ---

export interface CompiledSchema {
  version: number;
  graphName: string;
  nodeTypes: CompiledNodeType[];
  edgeTypes: CompiledEdgeType[];
  invariants: CompiledInvariant[];
  pipelines: CompiledPipeline[];
  /** Schema modules for composable/conditional schema application */
  modules?: CompiledModule[];
}

/**
 * A schema module is an independently defined fragment that can be
 * conditionally applied to a graph. Modules enable:
 * - Defining separate subsystems (e.g., "users", "billing", "content")
 * - Conditionally applying invariants based on node type namespaces
 * - Nesting graphs: each module defines its own types + cross-module edges
 * - Composing multiple modules into one graph with interoperability
 */
export interface CompiledModule {
  /** Unique module identifier */
  id: string;
  /** Human-readable name */
  name: string;
  /** Optional namespace prefix applied to all types in this module */
  namespace?: string;
  /** Node types defined by this module */
  nodeTypes: CompiledNodeType[];
  /** Edge types defined by this module */
  edgeTypes: CompiledEdgeType[];
  /** Invariants scoped to this module */
  invariants: CompiledInvariant[];
  /** Pipelines scoped to this module */
  pipelines: CompiledPipeline[];
  /** Condition for when this module is active (evaluated at runtime) */
  condition?: ModuleCondition;
  /** Dependencies on other modules (by ID) */
  dependsOn?: string[];
}

export interface CompiledNodeType {
  name: string;
  properties: Record<string, CompiledProperty>;
  allowedChildren?: string[];
  allowedParents?: string[];
}

export interface CompiledProperty {
  type: string;
  required: boolean;
  indexed: boolean;
  unique: boolean;
}

export interface CompiledEdgeType {
  name: string;
  fromTypes: string[];
  toTypes: string[];
}

export interface CompiledInvariant {
  id: string;
  name: string;
  type: string;
  config: Record<string, unknown>;
}

export interface CompiledPipeline {
  id: string;
  name: string;
  stages: CompiledStage[];
}

export interface CompiledStage {
  type: string;
  sourceType?: string;
  derivedType?: string;
  transform?: SerializableTransform;
}

export interface SerializableTransform {
  propertyMap?: Record<string, string>;
  propertyDefaults?: Record<string, unknown>;
  computedProperties?: Record<string, string>;
  includeProperties?: string[];
  excludeProperties?: string[];
  conditions?: string[];
}

/**
 * Condition for when a module is active. Modules can be activated based on:
 * - Node type matching (namespace prefix)
 * - Feature flags (string tags present on the graph)
 * - Custom expressions evaluated at the server
 */
export interface ModuleCondition {
  /** Module activates when any operated-on node type starts with this prefix */
  nodeTypePrefix?: string;
  /** Module activates when all listed feature flags are present on the graph */
  featureFlags?: string[];
  /** Module activates when this CEL-like expression evaluates to true */
  expression?: string;
}

// --- Property Options ---

export interface PropertyOptions {
  required?: boolean;
  indexed?: boolean;
  unique?: boolean;
}

// --- Schema Builder ---

export class SchemaCompiler {
  private nodeTypes: CompiledNodeType[] = [];
  private edgeTypes: CompiledEdgeType[] = [];
  private invariants: CompiledInvariant[] = [];
  private pipelines: CompiledPipeline[] = [];

  /** Define a node type */
  node(name: string, builder: (n: NodeCompiler) => void): this {
    const nc = new NodeCompiler(name);
    builder(nc);
    this.nodeTypes.push(nc.build());
    return this;
  }

  /** Define an edge type */
  edge(name: string, fromTypes: string[], toTypes: string[]): this {
    this.edgeTypes.push({ name, fromTypes, toTypes });
    return this;
  }

  /** Add a uniqueness invariant */
  unique(nodeType: string, property: string): this {
    const id = `unique-${nodeType}-${property}`;
    this.invariants.push({
      id,
      name: `Unique ${nodeType}.${property}`,
      type: "uniqueness",
      config: { nodeType, property },
    });
    return this;
  }

  /** Add an acyclicity invariant on an edge type */
  acyclic(edgeType: string): this {
    const id = `acyclic-${edgeType}`;
    this.invariants.push({
      id,
      name: `Acyclic ${edgeType}`,
      type: "acyclicity",
      config: { edgeType },
    });
    return this;
  }

  /** Add a cardinality constraint */
  maxCardinality(
    nodeType: string,
    edgeType: string,
    direction: "in" | "out",
    max: number
  ): this {
    const id = `max-${nodeType}-${edgeType}-${direction}`;
    this.invariants.push({
      id,
      name: `Max ${direction} ${edgeType} on ${nodeType}`,
      type: "cardinality",
      config: { nodeType, edgeType, direction, max },
    });
    return this;
  }

  /** Add a hierarchy depth constraint */
  maxDepth(nodeType: string, max: number): this {
    const id = `max-depth-${nodeType}`;
    this.invariants.push({
      id,
      name: `Max depth for ${nodeType}`,
      type: "hierarchy_depth",
      config: { nodeType, maxDepth: max },
    });
    return this;
  }

  /** Add a child count constraint */
  childCount(
    parentType: string,
    opts: { childType?: string; min?: number; max?: number }
  ): this {
    const id = `child-count-${parentType}-${opts.childType ?? "any"}`;
    this.invariants.push({
      id,
      name: `Child count for ${parentType}`,
      type: "child_count",
      config: { parentType, ...opts },
    });
    return this;
  }

  /** Add a derivation pipeline */
  pipeline(
    id: string,
    name: string,
    builder: (p: PipelineCompiler) => void
  ): this {
    const pc = new PipelineCompiler(id, name);
    builder(pc);
    this.pipelines.push(pc.build());
    return this;
  }

  private modules: CompiledModule[] = [];

  /**
   * Define a schema module — an independent, composable schema fragment.
   *
   * Modules allow you to define subsystems separately and compose them
   * into a single graph. Each module can have its own node types, edge types,
   * invariants, and pipelines. Modules can optionally:
   * - Use a namespace prefix to avoid type name collisions
   * - Declare dependencies on other modules
   * - Be conditionally activated based on node type prefixes, feature flags,
   *   or custom expressions
   *
   * @example
   * ```ts
   * s.module("users", (m) => {
   *   m.namespace("users");
   *   m.node("profile", (n) => { n.string("name", { required: true }); });
   *   m.edge("follows", ["profile"], ["profile"]);
   *   m.unique("profile", "name");
   * });
   *
   * s.module("billing", (m) => {
   *   m.namespace("billing");
   *   m.dependsOn("users");
   *   m.node("invoice", (n) => { n.number("amount", { required: true }); });
   *   // Cross-module edge: billing references users
   *   m.edge("billed_to", ["billing:invoice"], ["users:profile"]);
   * });
   * ```
   */
  module(id: string, builder: (m: ModuleCompiler) => void): this {
    const mc = new ModuleCompiler(id);
    builder(mc);
    this.modules.push(mc.build());
    return this;
  }

  /** Build the compiled schema */
  build(graphName: string): CompiledSchema {
    const result: CompiledSchema = {
      version: 1,
      graphName,
      nodeTypes: this.nodeTypes,
      edgeTypes: this.edgeTypes,
      invariants: this.invariants,
      pipelines: this.pipelines,
    };
    if (this.modules.length > 0) {
      result.modules = this.modules;
    }
    return result;
  }
}

// --- Node Compiler ---

export class NodeCompiler {
  private properties: Record<string, CompiledProperty> = {};
  private allowedChildren?: string[];
  private allowedParents?: string[];

  constructor(private name: string) {}

  string(name: string, opts: PropertyOptions = {}): this {
    this.properties[name] = {
      type: "string",
      required: opts.required ?? false,
      indexed: opts.indexed ?? false,
      unique: opts.unique ?? false,
    };
    return this;
  }

  number(name: string, opts: PropertyOptions = {}): this {
    this.properties[name] = {
      type: "number",
      required: opts.required ?? false,
      indexed: opts.indexed ?? false,
      unique: opts.unique ?? false,
    };
    return this;
  }

  boolean(name: string, opts: PropertyOptions = {}): this {
    this.properties[name] = {
      type: "boolean",
      required: opts.required ?? false,
      indexed: opts.indexed ?? false,
      unique: opts.unique ?? false,
    };
    return this;
  }

  ref(name: string, opts: PropertyOptions = {}): this {
    this.properties[name] = {
      type: "ref",
      required: opts.required ?? false,
      indexed: opts.indexed ?? false,
      unique: opts.unique ?? false,
    };
    return this;
  }

  array(name: string, opts: PropertyOptions = {}): this {
    this.properties[name] = {
      type: "array",
      required: opts.required ?? false,
      indexed: opts.indexed ?? false,
      unique: opts.unique ?? false,
    };
    return this;
  }

  object(name: string, opts: PropertyOptions = {}): this {
    this.properties[name] = {
      type: "object",
      required: opts.required ?? false,
      indexed: opts.indexed ?? false,
      unique: opts.unique ?? false,
    };
    return this;
  }

  any(name: string, opts: PropertyOptions = {}): this {
    this.properties[name] = {
      type: "any",
      required: opts.required ?? false,
      indexed: opts.indexed ?? false,
      unique: opts.unique ?? false,
    };
    return this;
  }

  children(...types: string[]): this {
    this.allowedChildren = types;
    return this;
  }

  parents(...types: string[]): this {
    this.allowedParents = types;
    return this;
  }

  build(): CompiledNodeType {
    return {
      name: this.name,
      properties: this.properties,
      allowedChildren: this.allowedChildren,
      allowedParents: this.allowedParents,
    };
  }
}

// --- Module Compiler ---

export class ModuleCompiler {
  private _name: string;
  private _namespace?: string;
  private nodeTypes: CompiledNodeType[] = [];
  private edgeTypes: CompiledEdgeType[] = [];
  private invariants: CompiledInvariant[] = [];
  private pipelines: CompiledPipeline[] = [];
  private condition?: ModuleCondition;
  private deps: string[] = [];

  constructor(private id: string) {
    this._name = id;
  }

  /** Set a human-readable name */
  name(name: string): this {
    this._name = name;
    return this;
  }

  /**
   * Set a namespace prefix. When set, all node types defined in this module
   * are automatically prefixed (e.g., namespace "users" + node "profile" = "users:profile").
   * This prevents collisions when composing multiple modules.
   */
  namespace(ns: string): this {
    this._namespace = ns;
    return this;
  }

  /** Declare a dependency on another module */
  dependsOn(...moduleIds: string[]): this {
    this.deps.push(...moduleIds);
    return this;
  }

  /** Set a condition for when this module is active */
  when(condition: ModuleCondition): this {
    this.condition = condition;
    return this;
  }

  /** Activate this module only for node types with this prefix */
  whenNodeTypePrefix(prefix: string): this {
    this.condition = { ...this.condition, nodeTypePrefix: prefix };
    return this;
  }

  /** Activate this module only when these feature flags are set */
  whenFeatureFlags(...flags: string[]): this {
    this.condition = { ...this.condition, featureFlags: flags };
    return this;
  }

  /** Define a node type within this module */
  node(name: string, builder: (n: NodeCompiler) => void): this {
    const fullName = this._namespace ? `${this._namespace}:${name}` : name;
    const nc = new NodeCompiler(fullName);
    builder(nc);
    this.nodeTypes.push(nc.build());
    return this;
  }

  /** Define an edge type within this module */
  edge(name: string, fromTypes: string[], toTypes: string[]): this {
    this.edgeTypes.push({ name, fromTypes, toTypes });
    return this;
  }

  /** Add a uniqueness invariant */
  unique(nodeType: string, property: string): this {
    const fullType = this._namespace ? `${this._namespace}:${nodeType}` : nodeType;
    const id = `${this.id}-unique-${fullType}-${property}`;
    this.invariants.push({
      id,
      name: `Unique ${fullType}.${property}`,
      type: "uniqueness",
      config: { nodeType: fullType, property },
    });
    return this;
  }

  /** Add an acyclicity invariant */
  acyclic(edgeType: string): this {
    const id = `${this.id}-acyclic-${edgeType}`;
    this.invariants.push({
      id,
      name: `Acyclic ${edgeType}`,
      type: "acyclicity",
      config: { edgeType },
    });
    return this;
  }

  /** Add a cardinality constraint */
  maxCardinality(
    nodeType: string,
    edgeType: string,
    direction: "in" | "out",
    max: number
  ): this {
    const fullType = this._namespace ? `${this._namespace}:${nodeType}` : nodeType;
    const id = `${this.id}-max-${fullType}-${edgeType}-${direction}`;
    this.invariants.push({
      id,
      name: `Max ${direction} ${edgeType} on ${fullType}`,
      type: "cardinality",
      config: { nodeType: fullType, edgeType, direction, max },
    });
    return this;
  }

  /** Add a derivation pipeline */
  pipeline(id: string, name: string, builder: (p: PipelineCompiler) => void): this {
    const pc = new PipelineCompiler(`${this.id}-${id}`, name);
    builder(pc);
    this.pipelines.push(pc.build());
    return this;
  }

  build(): CompiledModule {
    return {
      id: this.id,
      name: this._name,
      namespace: this._namespace,
      nodeTypes: this.nodeTypes,
      edgeTypes: this.edgeTypes,
      invariants: this.invariants,
      pipelines: this.pipelines,
      condition: this.condition,
      dependsOn: this.deps.length > 0 ? this.deps : undefined,
    };
  }
}

// --- Pipeline Compiler ---

export class PipelineCompiler {
  private stages: CompiledStage[] = [];

  constructor(
    private id: string,
    private name: string
  ) {}

  /** Add a 1:1 map stage with a serializable transform */
  map(
    sourceType: string,
    derivedType: string,
    transform?: SerializableTransform
  ): this {
    this.stages.push({
      type: "map",
      sourceType,
      derivedType,
      transform,
    });
    return this;
  }

  /** Add a join stage */
  join(
    sourceType: string,
    derivedType: string,
    transform?: SerializableTransform
  ): this {
    this.stages.push({
      type: "join",
      sourceType,
      derivedType,
      transform,
    });
    return this;
  }

  build(): CompiledPipeline {
    return {
      id: this.id,
      name: this.name,
      stages: this.stages,
    };
  }
}

// --- Top-level compile function ---

/**
 * Compile a schema definition into a deployable JSON format.
 *
 * @example
 * ```ts
 * const schema = compileSchema("my-app", (s) => {
 *   s.node("user", (n) => {
 *     n.string("name", { required: true });
 *     n.string("email", { required: true, unique: true, indexed: true });
 *   });
 *   s.edge("follows", ["user"], ["user"]);
 *   s.unique("user", "email");
 * });
 * ```
 */
export function compileSchema(
  graphName: string,
  builder: (s: SchemaCompiler) => void
): CompiledSchema {
  const compiler = new SchemaCompiler();
  builder(compiler);
  return compiler.build(graphName);
}

/**
 * Serialize a compiled schema to JSON string for deployment.
 */
export function serializeSchema(schema: CompiledSchema): string {
  return JSON.stringify(schema, null, 2);
}
