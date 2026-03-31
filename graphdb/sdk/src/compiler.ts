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

  /** Build the compiled schema */
  build(graphName: string): CompiledSchema {
    return {
      version: 1,
      graphName,
      nodeTypes: this.nodeTypes,
      edgeTypes: this.edgeTypes,
      invariants: this.invariants,
      pipelines: this.pipelines,
    };
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
