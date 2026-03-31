package derived

import (
	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
)

// Pipeline is an ordered sequence of derivation stages.
// Each stage reads from the source graph (or previous stage outputs)
// and produces derived nodes.
type Pipeline struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Stages []*Stage `json:"stages"`

	// Where this pipeline runs
	ExecutionMode ExecutionMode `json:"executionMode"`

	// DependsOn: source types that trigger recomputation.
	// If empty, inferred from stages.
	DependsOn []string `json:"dependsOn,omitempty"`
}

// ExecutionMode determines where derivation executes.
type ExecutionMode int

const (
	// ExecServer: derivation runs on the server only.
	ExecServer ExecutionMode = iota
	// ExecClient: derivation runs on the client only.
	// The pipeline definition is sent to the client, which executes it locally.
	ExecClient
	// ExecBoth: derivation runs on both server and client.
	// Server produces the canonical result; client runs optimistically.
	ExecBoth
)

// StageType identifies what a stage does.
type StageType int

const (
	// StageMap: 1:1 mapping of source nodes to derived nodes.
	StageMap StageType = iota
	// StageSubtreeInherit: component-instance subtree inheritance.
	StageSubtreeInherit
	// StageJoin: derive properties from related nodes (parent, refs, edges).
	StageJoin
	// StageMultiSubtree: merge multiple subtrees as children of a single node.
	StageMultiSubtree
	// StageComputed: fully custom computation via a function.
	StageComputed
)

// Stage is a single step in a derivation pipeline.
type Stage struct {
	Type        StageType `json:"type"`
	SourceType  string    `json:"sourceType,omitempty"`
	DerivedType string    `json:"derivedType,omitempty"`

	// Transform function (for Map, SubtreeInherit, Join stages)
	Transform TransformFunc `json:"-"`

	// Filter function (for all stages)
	Filter FilterFunc `json:"-"`

	// DeriveChildren: recursively derive children (for Map stage)
	DeriveChildren bool `json:"deriveChildren"`

	// ChildRules: per-child-type derivation overrides
	ChildRules map[string]*Stage `json:"childRules,omitempty"`

	// Inheritance config (for SubtreeInherit stage)
	Inheritance *SubtreeDerivation `json:"inheritance,omitempty"`

	// Join config (for Join stage)
	Join *JoinDef `json:"join,omitempty"`

	// MultiSubtree config
	MultiSubtree *MultiSubtreeDef `json:"multiSubtree,omitempty"`

	// ComputeFunc (for Computed stage)
	ComputeFunc func(ctx *ComputeContext) `json:"-"`

	// Serializable representation of this stage (for client-side execution)
	SerializableTransform *SerializableTransform `json:"serializableTransform,omitempty"`
}

// --- Join definitions ---

// JoinDef configures a Join stage: derive properties from related nodes.
type JoinDef struct {
	SourceType string         `json:"sourceType"`
	Relations  []*RelationDef `json:"relations"`
}

// RelVia specifies how to resolve a relation.
type RelVia int

const (
	RelViaProperty  RelVia = iota // follow a ref property
	RelViaParent                  // walk up to parent
	RelViaChildren                // walk down to children
	RelViaOutEdge                 // follow outgoing edges
	RelViaInEdge                  // follow incoming edges
)

// RelationDef defines a single relation to resolve during a Join.
type RelationDef struct {
	Name       string `json:"name"`       // name of the relation in derived properties
	Via        RelVia `json:"via"`        // how to resolve
	Property   string `json:"property,omitempty"`  // for RelViaProperty
	EdgeType   string `json:"edgeType,omitempty"`  // for RelViaOutEdge/InEdge
	TargetType string `json:"targetType,omitempty"` // filter by target node type
}

// --- Multi-subtree definitions ---

// MultiSubtreeDef configures merging multiple subtrees as children of a node.
type MultiSubtreeDef struct {
	ParentType string           `json:"parentType"`
	Sources    []*SubtreeSource `json:"sources"`
}

// ResolveVia specifies how to find the root nodes of a subtree source.
type ResolveVia int

const (
	ResolveProperty      ResolveVia = iota // single ref property
	ResolvePropertySlice                   // array of ref properties
	ResolveEdge                            // follow edges
	ResolveChildren                        // direct children matching a type
	ResolveQuery                           // custom query function
)

// SubtreeSource defines one source of subtree nodes to merge.
type SubtreeSource struct {
	Name       string     `json:"name"`
	ResolveVia ResolveVia `json:"resolveVia"`
	Property   string     `json:"property,omitempty"`
	EdgeType   string     `json:"edgeType,omitempty"`
	SourceType string     `json:"sourceType,omitempty"`
	DerivedType string    `json:"derivedType,omitempty"`

	IncludeChildren bool `json:"includeChildren"`

	// Filtering and transformation
	Filter         FilterFunc    `json:"-"`
	Transform      TransformFunc `json:"-"`
	ChildFilter    FilterFunc    `json:"-"`
	ChildTransform TransformFunc `json:"-"`
	ChildTypeMapping map[string]string `json:"childTypeMapping,omitempty"`

	// Custom query for ResolveQuery
	QueryFunc func(parent *crdt.MaterializedNode, ctx *DerivationContext) []*crdt.MaterializedNode `json:"-"`
}

// ComputeContext provides full read/write access to both graphs
// during a Computed stage. This is the escape hatch for
// derivations that don't fit the declarative model.
//
// NOTE: ComputeContext is used inside Recompute which already holds
// the store's write lock. Methods here access store internals directly
// without re-acquiring the lock.
type ComputeContext struct {
	*DerivationContext
	store      *Store
	pipelineID string
}

// Emit adds a derived node to the store.
func (cc *ComputeContext) Emit(node *DerivedNode) {
	node.DerivationID = cc.pipelineID
	cc.store.insertDerivedNode(node)
}

// EmitEdge adds a derived edge to the store.
func (cc *ComputeContext) EmitEdge(edge *DerivedEdge) {
	cc.store.insertDerivedEdge(edge)
}

// GetDerivedNodesByType returns already-derived nodes of a type.
// This accesses the internal map directly (no lock) since we're
// called within Recompute which already holds the write lock.
func (cc *ComputeContext) GetDerivedNodesByType(derivedType string) []*DerivedNode {
	var result []*DerivedNode
	for _, id := range cc.store.typeIndex[derivedType] {
		if n, ok := cc.store.nodes[id]; ok {
			result = append(result, n)
		}
	}
	return result
}

// GetDerivedChildren returns children of a derived node (no lock).
func (cc *ComputeContext) GetDerivedChildren(id uuid.UUID) []*DerivedNode {
	var result []*DerivedNode
	for _, childID := range cc.store.childMap[id] {
		if n, ok := cc.store.nodes[childID]; ok {
			result = append(result, n)
		}
	}
	return result
}


// --- Serializable transform for client-side execution ---

// SerializableTransform is a JSON-serializable representation of a transform
// that can be sent to the client for client-side derivation.
type SerializableTransform struct {
	// PropertyMap: simple key remapping. Key = derived property, Value = source property.
	PropertyMap map[string]string `json:"propertyMap,omitempty"`

	// PropertyDefaults: default values for missing properties.
	PropertyDefaults map[string]interface{} `json:"propertyDefaults,omitempty"`

	// ComputedProperties: properties computed from expressions.
	// Key = derived property, Value = expression string.
	// Supported expressions:
	//   "source.propName" - direct property access
	//   "parent.propName" - parent property access
	//   "count(children)" - count of children
	//   "count(outEdges.edgeType)" - count of outgoing edges of type
	//   "concat(source.firstName, ' ', source.lastName)" - string concat
	//   "ref(source.componentId).propName" - follow ref and get property
	ComputedProperties map[string]string `json:"computedProperties,omitempty"`

	// IncludeProperties: only include these properties (whitelist).
	// Empty = include all.
	IncludeProperties []string `json:"includeProperties,omitempty"`

	// ExcludeProperties: exclude these properties (blacklist).
	ExcludeProperties []string `json:"excludeProperties,omitempty"`

	// Conditions: only derive if these conditions are met.
	// Each condition is an expression that must evaluate to true.
	Conditions []string `json:"conditions,omitempty"`
}

// --- Pipeline builders ---

// NewPipeline creates a new derivation pipeline.
func NewPipeline(id, name string) *Pipeline {
	return &Pipeline{
		ID:            id,
		Name:          name,
		ExecutionMode: ExecServer,
	}
}

// WithMode sets the execution mode.
func (p *Pipeline) WithMode(mode ExecutionMode) *Pipeline {
	p.ExecutionMode = mode
	return p
}

// Map adds a 1:1 mapping stage.
func (p *Pipeline) Map(sourceType, derivedType string, transform TransformFunc) *Pipeline {
	p.Stages = append(p.Stages, &Stage{
		Type:        StageMap,
		SourceType:  sourceType,
		DerivedType: derivedType,
		Transform:   transform,
	})
	return p
}

// MapWithChildren adds a mapping stage that also recursively derives children.
func (p *Pipeline) MapWithChildren(sourceType, derivedType string, transform TransformFunc) *Pipeline {
	p.Stages = append(p.Stages, &Stage{
		Type:           StageMap,
		SourceType:     sourceType,
		DerivedType:    derivedType,
		Transform:      transform,
		DeriveChildren: true,
	})
	return p
}

// MapFiltered adds a mapping stage with a filter.
func (p *Pipeline) MapFiltered(sourceType, derivedType string, filter FilterFunc, transform TransformFunc) *Pipeline {
	p.Stages = append(p.Stages, &Stage{
		Type:        StageMap,
		SourceType:  sourceType,
		DerivedType: derivedType,
		Filter:      filter,
		Transform:   transform,
	})
	return p
}

// Inherit adds a subtree inheritance stage.
func (p *Pipeline) Inherit(derivedType string, inh *SubtreeDerivation) *Pipeline {
	p.Stages = append(p.Stages, &Stage{
		Type:        StageSubtreeInherit,
		DerivedType: derivedType,
		Inheritance: inh,
	})
	return p
}

// InheritWithTransform adds a subtree inheritance stage with a custom transform.
func (p *Pipeline) InheritWithTransform(derivedType string, inh *SubtreeDerivation, transform TransformFunc) *Pipeline {
	p.Stages = append(p.Stages, &Stage{
		Type:        StageSubtreeInherit,
		DerivedType: derivedType,
		Inheritance: inh,
		Transform:   transform,
	})
	return p
}

// Join adds a join stage that resolves related nodes.
func (p *Pipeline) Join(derivedType string, join *JoinDef, transform TransformFunc) *Pipeline {
	p.Stages = append(p.Stages, &Stage{
		Type:        StageJoin,
		DerivedType: derivedType,
		Join:        join,
		Transform:   transform,
	})
	return p
}

// MultiSubtree adds a multi-subtree merging stage.
func (p *Pipeline) MultiSubtree(derivedType string, ms *MultiSubtreeDef) *Pipeline {
	p.Stages = append(p.Stages, &Stage{
		Type:         StageMultiSubtree,
		DerivedType:  derivedType,
		MultiSubtree: ms,
	})
	return p
}

// MultiSubtreeWithTransform adds a multi-subtree stage with a parent transform.
func (p *Pipeline) MultiSubtreeWithTransform(derivedType string, ms *MultiSubtreeDef, transform TransformFunc) *Pipeline {
	p.Stages = append(p.Stages, &Stage{
		Type:         StageMultiSubtree,
		DerivedType:  derivedType,
		MultiSubtree: ms,
		Transform:    transform,
	})
	return p
}

// Compute adds a fully custom computation stage.
func (p *Pipeline) Compute(fn func(ctx *ComputeContext)) *Pipeline {
	p.Stages = append(p.Stages, &Stage{
		Type:        StageComputed,
		ComputeFunc: fn,
	})
	return p
}

// AddStage adds a manually constructed stage.
func (p *Pipeline) AddStage(stage *Stage) *Pipeline {
	p.Stages = append(p.Stages, stage)
	return p
}
