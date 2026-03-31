package derived

import (
	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
)

// DerivationRule defines how to compute derived nodes from source nodes.
// Rules are composable: you can chain transforms, filter, and restructure.
type DerivationRule struct {
	Name string `json:"name"`

	// SourceType: the persistent node type this rule applies to.
	SourceType string `json:"sourceType"`

	// DerivedType: the output derived node type.
	DerivedType string `json:"derivedType"`

	// Transform maps a source node's properties to derived properties.
	// If nil, properties are copied 1:1.
	Transform TransformFunc `json:"-"`

	// Filter decides whether a source node should produce a derived node.
	// If nil, all source nodes of SourceType are derived.
	Filter FilterFunc `json:"-"`

	// DeriveChildren: if true, recursively derive children using
	// ChildRules (or the same rule if ChildRules is empty).
	DeriveChildren bool `json:"deriveChildren"`

	// ChildRules: per-child-type derivation rules. Key is source child type.
	// If a child type is not in this map and DeriveChildren is true,
	// the child is copied as-is with the same type.
	ChildRules map[string]*DerivationRule `json:"childRules,omitempty"`

	// EdgeRules: rules for deriving edges associated with matched nodes.
	EdgeRules []*EdgeDerivationRule `json:"edgeRules,omitempty"`
}

// TransformFunc converts source node properties into derived properties.
// It receives the source node and a context providing access to the
// full source graph (for lookups, joins, etc.)
type TransformFunc func(source *crdt.MaterializedNode, ctx *DerivationContext) map[string]interface{}

// FilterFunc decides if a source node should be derived.
type FilterFunc func(source *crdt.MaterializedNode) bool

// EdgeDerivationRule defines how edges are derived.
type EdgeDerivationRule struct {
	SourceEdgeType  string `json:"sourceEdgeType"`
	DerivedEdgeType string `json:"derivedEdgeType"`
	// Transform for edge properties
	Transform func(edge *crdt.MaterializedEdge, ctx *DerivationContext) map[string]interface{} `json:"-"`
}

// DerivationContext provides read access to both the source graph
// and the derived graph during derivation. This lets transforms
// reference other nodes, follow edges, etc.
type DerivationContext struct {
	// Source graph access
	sourceNodes   func(id uuid.UUID) (*crdt.MaterializedNode, bool)
	sourceByType  func(nodeType string) []*crdt.MaterializedNode
	sourceChildren func(id uuid.UUID) []*crdt.MaterializedNode
	sourceParent  func(id uuid.UUID) (*crdt.MaterializedNode, bool)
	sourceOutEdges func(id uuid.UUID) []*crdt.MaterializedEdge
	sourceInEdges  func(id uuid.UUID) []*crdt.MaterializedEdge

	// Derived graph access (already-computed derived nodes)
	derivedNodes  map[uuid.UUID]*DerivedNode

	// Source-to-derived ID mapping
	sourceToDerivedID map[uuid.UUID]uuid.UUID
}

// SourceNode gets a node from the persistent source graph.
func (dc *DerivationContext) SourceNode(id uuid.UUID) (*crdt.MaterializedNode, bool) {
	return dc.sourceNodes(id)
}

// SourceNodesByType gets all source nodes of a type.
func (dc *DerivationContext) SourceNodesByType(nodeType string) []*crdt.MaterializedNode {
	return dc.sourceByType(nodeType)
}

// SourceChildren gets children of a source node.
func (dc *DerivationContext) SourceChildren(id uuid.UUID) []*crdt.MaterializedNode {
	return dc.sourceChildren(id)
}

// SourceParent gets the parent of a source node.
func (dc *DerivationContext) SourceParent(id uuid.UUID) (*crdt.MaterializedNode, bool) {
	return dc.sourceParent(id)
}

// SourceOutEdges gets outgoing edges from a source node.
func (dc *DerivationContext) SourceOutEdges(id uuid.UUID) []*crdt.MaterializedEdge {
	return dc.sourceOutEdges(id)
}

// SourceInEdges gets incoming edges to a source node.
func (dc *DerivationContext) SourceInEdges(id uuid.UUID) []*crdt.MaterializedEdge {
	return dc.sourceInEdges(id)
}

// DerivedNodeForSource returns the derived node produced from a source node.
func (dc *DerivationContext) DerivedNodeForSource(sourceID uuid.UUID) (*DerivedNode, bool) {
	derivedID, ok := dc.sourceToDerivedID[sourceID]
	if !ok {
		return nil, false
	}
	dn, ok := dc.derivedNodes[derivedID]
	return dn, ok
}

// SubtreeDerivation defines how an instance node inherits a subtree
// from a component/template node. This is the key pattern for
// component-instance relationships.
type SubtreeDerivation struct {
	Name string `json:"name"`

	// InstanceType: the node type that inherits (e.g., "instance")
	InstanceType string `json:"instanceType"`

	// SourceType: the node type to inherit from (e.g., "component")
	SourceType string `json:"sourceType"`

	// RefProperty: property on the instance that references the source
	// (e.g., "componentId" containing the UUID of the component)
	RefProperty string `json:"refProperty"`

	// Strategy determines how properties are merged
	Strategy InheritStrategy `json:"strategy"`

	// ChildTypeMapping: optionally remap child types during inheritance.
	// Key = source child type, Value = derived child type.
	// If empty, child types are preserved.
	ChildTypeMapping map[string]string `json:"childTypeMapping,omitempty"`

	// ExcludeProperties: source properties to NOT inherit
	ExcludeProperties []string `json:"excludeProperties,omitempty"`

	// PropertyTransform: custom transform applied after merge
	PropertyTransform TransformFunc `json:"-"`
}

// MergeProperties combines source properties with instance overrides
// according to the specified strategy.
func MergeProperties(source, overrides map[string]interface{}, strategy InheritStrategy) map[string]interface{} {
	switch strategy {
	case InheritProperties, InheritSubtree:
		// Overlay: overrides win for conflicting keys
		result := make(map[string]interface{}, len(source)+len(overrides))
		for k, v := range source {
			result[k] = v
		}
		for k, v := range overrides {
			result[k] = v
		}
		return result

	case InheritMerge:
		// Deep merge: recursively merge maps
		return deepMerge(source, overrides)

	default:
		return overrides
	}
}

func deepMerge(base, overlay map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(base)+len(overlay))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		if baseMap, ok := result[k].(map[string]interface{}); ok {
			if overlayMap, ok := v.(map[string]interface{}); ok {
				result[k] = deepMerge(baseMap, overlayMap)
				continue
			}
		}
		result[k] = v
	}
	return result
}
