/**
 * Schema definition DSL for GraphDB.
 *
 * Provides a fluent, type-safe API for defining graph schemas,
 * similar to Convex's schema definition system but adapted
 * for hierarchical graph structures.
 *
 * @example
 * ```ts
 * import { defineSchema, defineNode, defineEdge, s } from "@graphdb/sdk";
 *
 * export default defineSchema({
 *   nodes: {
 *     organization: defineNode({
 *       name: s.string(),
 *       domain: s.string().optional(),
 *     }).allowChildren("team", "member"),
 *
 *     team: defineNode({
 *       name: s.string(),
 *       description: s.string().optional(),
 *     }).allowChildren("member"),
 *
 *     member: defineNode({
 *       name: s.string(),
 *       email: s.string().unique().indexed(),
 *       role: s.string(),
 *     }),
 *   },
 *
 *   edges: {
 *     manages: defineEdge("team", "member"),
 *     reports_to: defineEdge("member", "member"),
 *   },
 *
 *   invariants: [
 *     uniqueProperty("member", "email"),
 *     maxChildren("team", 50),
 *     acyclic("reports_to"),
 *   ],
 * });
 * ```
 */

import type {
  Schema,
  NodeTypeDef,
  EdgeTypeDef,
  PropertyDef,
  PropertyType,
  InvariantDef,
  InvariantType,
} from "./types";

// --- Property builder ---

export class PropertyBuilder {
  private _type: PropertyType;
  private _required = true;
  private _indexed = false;
  private _unique = false;

  constructor(type: PropertyType) {
    this._type = type;
  }

  /** Mark this property as optional */
  optional(): this {
    this._required = false;
    return this;
  }

  /** Add a secondary index on this property */
  indexed(): this {
    this._indexed = true;
    return this;
  }

  /** Enforce uniqueness across all nodes of this type */
  unique(): this {
    this._unique = true;
    return this;
  }

  /** @internal Build the property definition */
  _build(name: string): PropertyDef {
    return {
      name,
      type: this._type,
      required: this._required,
      indexed: this._indexed,
      unique: this._unique,
    };
  }
}

/** Schema field type helpers (like Convex's `v` validator) */
export const s = {
  /** String property */
  string: () => new PropertyBuilder("string"),
  /** Number property */
  number: () => new PropertyBuilder("number"),
  /** Boolean property */
  boolean: () => new PropertyBuilder("boolean"),
  /** Array property */
  array: () => new PropertyBuilder("array"),
  /** Object/map property */
  object: () => new PropertyBuilder("object"),
  /** Reference to another node (stores a node ID) */
  ref: () => new PropertyBuilder("ref"),
  /** Any type (no validation) */
  any: () => new PropertyBuilder("any"),
};

// --- Node type builder ---

export class NodeTypeBuilder {
  private properties: Record<string, PropertyBuilder>;
  private _allowedChildren: string[] = [];
  private _allowedParents: string[] = [];

  constructor(properties: Record<string, PropertyBuilder>) {
    this.properties = properties;
  }

  /** Specify which node types can be children of this node */
  allowChildren(...types: string[]): this {
    this._allowedChildren.push(...types);
    return this;
  }

  /** Specify which node types can be parents of this node */
  allowParents(...types: string[]): this {
    this._allowedParents.push(...types);
    return this;
  }

  /** @internal Build the node type definition */
  _build(name: string): NodeTypeDef {
    const properties: Record<string, PropertyDef> = {};
    for (const [key, builder] of Object.entries(this.properties)) {
      properties[key] = builder._build(key);
    }
    return {
      name,
      properties,
      allowedChildren:
        this._allowedChildren.length > 0 ? this._allowedChildren : undefined,
      allowedParents:
        this._allowedParents.length > 0 ? this._allowedParents : undefined,
    };
  }
}

/** Define a node type with properties */
export function defineNode(
  properties: Record<string, PropertyBuilder>
): NodeTypeBuilder {
  return new NodeTypeBuilder(properties);
}

// --- Edge type builder ---

export class EdgeTypeBuilder {
  private _fromTypes: string[];
  private _toTypes: string[];
  private _properties: Record<string, PropertyBuilder> = {};

  constructor(fromTypes: string | string[], toTypes: string | string[]) {
    this._fromTypes = Array.isArray(fromTypes) ? fromTypes : [fromTypes];
    this._toTypes = Array.isArray(toTypes) ? toTypes : [toTypes];
  }

  /** Add properties to this edge type */
  withProperties(properties: Record<string, PropertyBuilder>): this {
    this._properties = properties;
    return this;
  }

  /** @internal Build the edge type definition */
  _build(name: string): EdgeTypeDef {
    const properties: Record<string, PropertyDef> = {};
    for (const [key, builder] of Object.entries(this._properties)) {
      properties[key] = builder._build(key);
    }
    return {
      name,
      fromTypes: this._fromTypes,
      toTypes: this._toTypes,
      properties:
        Object.keys(properties).length > 0 ? properties : undefined,
    };
  }
}

/** Define an edge type between node types */
export function defineEdge(
  fromTypes: string | string[],
  toTypes: string | string[]
): EdgeTypeBuilder {
  return new EdgeTypeBuilder(fromTypes, toTypes);
}

// --- Schema builder ---

export interface SchemaDefinition {
  nodes: Record<string, NodeTypeBuilder>;
  edges?: Record<string, EdgeTypeBuilder>;
  invariants?: InvariantBuilder[];
}

/**
 * Define the complete graph schema.
 * This is the top-level function for defining your graph structure.
 */
export function defineSchema(def: SchemaDefinition): Schema {
  const nodeTypes: Record<string, NodeTypeDef> = {};
  for (const [name, builder] of Object.entries(def.nodes)) {
    nodeTypes[name] = builder._build(name);
  }

  const edgeTypes: Record<string, EdgeTypeDef> = {};
  if (def.edges) {
    for (const [name, builder] of Object.entries(def.edges)) {
      edgeTypes[name] = builder._build(name);
    }
  }

  return { nodeTypes, edgeTypes };
}

// --- Invariant builders ---

export interface InvariantBuilder {
  _build(): InvariantDef;
}

class InvariantBuilderImpl implements InvariantBuilder {
  constructor(private def: InvariantDef) {}
  _build(): InvariantDef {
    return this.def;
  }
}

/** Ensure a property is unique across all nodes of a type */
export function uniqueProperty(
  nodeType: string,
  property: string
): InvariantBuilder {
  return new InvariantBuilderImpl({
    id: "",
    name: `unique_${nodeType}_${property}`,
    type: "uniqueness",
    description: `Property '${property}' must be unique across all '${nodeType}' nodes`,
    config: { nodeType, property },
  });
}

/** Limit the number of outgoing edges of a type */
export function maxOutEdges(
  nodeType: string,
  edgeType: string,
  max: number
): InvariantBuilder {
  return new InvariantBuilderImpl({
    id: "",
    name: `max_out_${nodeType}_${edgeType}`,
    type: "cardinality",
    description: `'${nodeType}' can have at most ${max} outgoing '${edgeType}' edges`,
    config: { nodeType, edgeType, direction: "out", max },
  });
}

/** Limit the number of incoming edges of a type */
export function maxInEdges(
  nodeType: string,
  edgeType: string,
  max: number
): InvariantBuilder {
  return new InvariantBuilderImpl({
    id: "",
    name: `max_in_${nodeType}_${edgeType}`,
    type: "cardinality",
    description: `'${nodeType}' can have at most ${max} incoming '${edgeType}' edges`,
    config: { nodeType, edgeType, direction: "in", max },
  });
}

/** Require at least one edge of a type */
export function requiredEdge(
  nodeType: string,
  edgeType: string,
  direction: "in" | "out" = "out"
): InvariantBuilder {
  return new InvariantBuilderImpl({
    id: "",
    name: `required_${direction}_${nodeType}_${edgeType}`,
    type: "required_edge",
    description: `'${nodeType}' must have at least one ${direction}going '${edgeType}' edge`,
    config: { nodeType, edgeType, direction },
  });
}

/** Prevent cycles in edges of a given type */
export function acyclic(edgeType: string): InvariantBuilder {
  return new InvariantBuilderImpl({
    id: "",
    name: `acyclic_${edgeType}`,
    type: "acyclicity",
    description: `'${edgeType}' edges must not form cycles`,
    config: { edgeType },
  });
}

/** Limit hierarchy depth */
export function maxDepth(maxDepthValue: number, nodeType?: string): InvariantBuilder {
  return new InvariantBuilderImpl({
    id: "",
    name: `max_depth${nodeType ? `_${nodeType}` : ""}`,
    type: "hierarchy_depth",
    description: `Hierarchy depth must not exceed ${maxDepthValue}`,
    config: { nodeType: nodeType ?? "", maxDepth: maxDepthValue },
  });
}

/** Limit the number of children a node type can have */
export function maxChildren(
  parentType: string,
  max: number,
  childType?: string
): InvariantBuilder {
  return new InvariantBuilderImpl({
    id: "",
    name: `max_children_${parentType}${childType ? `_${childType}` : ""}`,
    type: "child_count",
    description: `'${parentType}' can have at most ${max} ${childType ?? ""} children`,
    config: { parentType, childType: childType ?? "", max },
  });
}

/** Require a minimum number of children */
export function minChildren(
  parentType: string,
  min: number,
  childType?: string
): InvariantBuilder {
  return new InvariantBuilderImpl({
    id: "",
    name: `min_children_${parentType}${childType ? `_${childType}` : ""}`,
    type: "child_count",
    description: `'${parentType}' must have at least ${min} ${childType ?? ""} children`,
    config: { parentType, childType: childType ?? "", min },
  });
}

/** Constrain which node types an edge can connect */
export function edgeConstraint(
  edgeType: string,
  fromTypes: string[],
  toTypes: string[]
): InvariantBuilder {
  return new InvariantBuilderImpl({
    id: "",
    name: `edge_constraint_${edgeType}`,
    type: "edge_constraint",
    description: `'${edgeType}' edges can only connect ${fromTypes.join("|")} -> ${toTypes.join("|")}`,
    config: { edgeType, fromTypes, toTypes },
  });
}
