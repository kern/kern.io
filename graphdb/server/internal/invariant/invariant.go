// Package invariant provides a system for defining and validating
// graph-wide invariants. These are constraints that span multiple
// nodes and edges, validated server-side on every mutation.
package invariant

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
)

// InvariantType identifies the kind of invariant.
type InvariantType string

const (
	// Cardinality: constrains the count of edges/children
	InvariantCardinality InvariantType = "cardinality"
	// Uniqueness: ensures a property is unique across nodes of a type
	InvariantUniqueness InvariantType = "uniqueness"
	// EdgeConstraint: constrains edge relationships between types
	InvariantEdgeConstraint InvariantType = "edge_constraint"
	// RequiredEdge: a node must have at least one edge of a given type
	InvariantRequiredEdge InvariantType = "required_edge"
	// Acyclicity: prevents cycles in edges of a given type
	InvariantAcyclicity InvariantType = "acyclicity"
	// Custom: user-defined predicate function
	InvariantCustom InvariantType = "custom"
	// HierarchyDepth: max depth in the hierarchy tree
	InvariantHierarchyDepth InvariantType = "hierarchy_depth"
	// ChildCount: constrains number of children a node type can have
	InvariantChildCount InvariantType = "child_count"
)

// Invariant is a constraint on the graph.
type Invariant struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Type        InvariantType `json:"type"`
	Description string        `json:"description"`
	Config      interface{}   `json:"config"` // type-specific configuration

	// For custom invariants: a Go function that validates the graph
	customFn func(ctx *ValidationContext) error
}

// CardinalityConfig configures a cardinality invariant.
type CardinalityConfig struct {
	NodeType  string `json:"nodeType"`
	EdgeType  string `json:"edgeType"`
	Direction string `json:"direction"` // "in" or "out"
	Min       *int   `json:"min,omitempty"`
	Max       *int   `json:"max,omitempty"`
}

// UniquenessConfig configures a uniqueness invariant.
type UniquenessConfig struct {
	NodeType string `json:"nodeType"`
	Property string `json:"property"`
}

// EdgeConstraintConfig configures an edge constraint invariant.
type EdgeConstraintConfig struct {
	EdgeType  string   `json:"edgeType"`
	FromTypes []string `json:"fromTypes"`
	ToTypes   []string `json:"toTypes"`
}

// RequiredEdgeConfig configures a required edge invariant.
type RequiredEdgeConfig struct {
	NodeType  string `json:"nodeType"`
	EdgeType  string `json:"edgeType"`
	Direction string `json:"direction"` // "in" or "out"
}

// AcyclicityConfig configures an acyclicity invariant.
type AcyclicityConfig struct {
	EdgeType string `json:"edgeType"`
}

// HierarchyDepthConfig configures a hierarchy depth invariant.
type HierarchyDepthConfig struct {
	NodeType string `json:"nodeType,omitempty"` // empty = all types
	MaxDepth int    `json:"maxDepth"`
}

// ChildCountConfig configures a child count invariant.
type ChildCountConfig struct {
	ParentType string `json:"parentType"`
	ChildType  string `json:"childType,omitempty"` // empty = all children
	Min        *int   `json:"min,omitempty"`
	Max        *int   `json:"max,omitempty"`
}

// ValidationContext provides access to the graph state during validation.
type ValidationContext struct {
	walker *crdt.EGWalker
}

// NewValidationContext creates a new validation context.
func NewValidationContext(walker *crdt.EGWalker) *ValidationContext {
	return &ValidationContext{walker: walker}
}

// GetNode returns a node by ID.
func (vc *ValidationContext) GetNode(id uuid.UUID) (*crdt.MaterializedNode, bool) {
	return vc.walker.GetNode(id)
}

// GetNodesByType returns all nodes of a type.
func (vc *ValidationContext) GetNodesByType(nodeType string) []*crdt.MaterializedNode {
	return vc.walker.GetNodesByType(nodeType)
}

// GetOutEdges returns outgoing edges.
func (vc *ValidationContext) GetOutEdges(nodeID uuid.UUID) []*crdt.MaterializedEdge {
	return vc.walker.GetOutEdges(nodeID)
}

// GetInEdges returns incoming edges.
func (vc *ValidationContext) GetInEdges(nodeID uuid.UUID) []*crdt.MaterializedEdge {
	return vc.walker.GetInEdges(nodeID)
}

// GetChildren returns children of a node.
func (vc *ValidationContext) GetChildren(nodeID uuid.UUID) []*crdt.MaterializedNode {
	return vc.walker.GetChildren(nodeID)
}

// GetParent returns the parent of a node.
func (vc *ValidationContext) GetParent(nodeID uuid.UUID) (*crdt.MaterializedNode, bool) {
	return vc.walker.GetParent(nodeID)
}

// AllNodes returns all nodes.
func (vc *ValidationContext) AllNodes() []*crdt.MaterializedNode {
	return vc.walker.AllNodes()
}

// AllEdges returns all edges.
func (vc *ValidationContext) AllEdges() []*crdt.MaterializedEdge {
	return vc.walker.AllEdges()
}

// --- Invariant constructors ---

// NewCardinalityInvariant creates a cardinality invariant.
func NewCardinalityInvariant(name string, config CardinalityConfig) *Invariant {
	return &Invariant{
		ID:     uuid.New().String(),
		Name:   name,
		Type:   InvariantCardinality,
		Config: config,
	}
}

// NewUniquenessInvariant creates a uniqueness invariant.
func NewUniquenessInvariant(name string, config UniquenessConfig) *Invariant {
	return &Invariant{
		ID:     uuid.New().String(),
		Name:   name,
		Type:   InvariantUniqueness,
		Config: config,
	}
}

// NewEdgeConstraintInvariant creates an edge constraint invariant.
func NewEdgeConstraintInvariant(name string, config EdgeConstraintConfig) *Invariant {
	return &Invariant{
		ID:     uuid.New().String(),
		Name:   name,
		Type:   InvariantEdgeConstraint,
		Config: config,
	}
}

// NewRequiredEdgeInvariant creates a required edge invariant.
func NewRequiredEdgeInvariant(name string, config RequiredEdgeConfig) *Invariant {
	return &Invariant{
		ID:     uuid.New().String(),
		Name:   name,
		Type:   InvariantRequiredEdge,
		Config: config,
	}
}

// NewAcyclicityInvariant creates an acyclicity invariant.
func NewAcyclicityInvariant(name string, config AcyclicityConfig) *Invariant {
	return &Invariant{
		ID:     uuid.New().String(),
		Name:   name,
		Type:   InvariantAcyclicity,
		Config: config,
	}
}

// NewHierarchyDepthInvariant creates a hierarchy depth invariant.
func NewHierarchyDepthInvariant(name string, config HierarchyDepthConfig) *Invariant {
	return &Invariant{
		ID:     uuid.New().String(),
		Name:   name,
		Type:   InvariantHierarchyDepth,
		Config: config,
	}
}

// NewChildCountInvariant creates a child count invariant.
func NewChildCountInvariant(name string, config ChildCountConfig) *Invariant {
	return &Invariant{
		ID:     uuid.New().String(),
		Name:   name,
		Type:   InvariantChildCount,
		Config: config,
	}
}

// NewCustomInvariant creates a custom invariant with a user-defined function.
func NewCustomInvariant(name, description string, fn func(ctx *ValidationContext) error) *Invariant {
	return &Invariant{
		ID:          uuid.New().String(),
		Name:        name,
		Type:        InvariantCustom,
		Description: description,
		customFn:    fn,
	}
}

// Validate checks this invariant against the current graph state.
func (inv *Invariant) Validate(ctx *ValidationContext) error {
	switch inv.Type {
	case InvariantCardinality:
		return inv.validateCardinality(ctx)
	case InvariantUniqueness:
		return inv.validateUniqueness(ctx)
	case InvariantEdgeConstraint:
		return inv.validateEdgeConstraint(ctx)
	case InvariantRequiredEdge:
		return inv.validateRequiredEdge(ctx)
	case InvariantAcyclicity:
		return inv.validateAcyclicity(ctx)
	case InvariantHierarchyDepth:
		return inv.validateHierarchyDepth(ctx)
	case InvariantChildCount:
		return inv.validateChildCount(ctx)
	case InvariantCustom:
		if inv.customFn != nil {
			return inv.customFn(ctx)
		}
		return nil
	default:
		return fmt.Errorf("unknown invariant type: %s", inv.Type)
	}
}

func (inv *Invariant) validateCardinality(ctx *ValidationContext) error {
	config := inv.Config.(CardinalityConfig)
	nodes := ctx.GetNodesByType(config.NodeType)

	for _, node := range nodes {
		var edges []*crdt.MaterializedEdge
		switch config.Direction {
		case "out":
			edges = ctx.GetOutEdges(node.ID)
		case "in":
			edges = ctx.GetInEdges(node.ID)
		}

		// Filter by edge type
		count := 0
		for _, edge := range edges {
			if edge.Type == config.EdgeType {
				count++
			}
		}

		if config.Min != nil && count < *config.Min {
			return fmt.Errorf("invariant %q violated: node %s (%s) has %d %s edges (min: %d)",
				inv.Name, node.ID, config.NodeType, count, config.EdgeType, *config.Min)
		}
		if config.Max != nil && count > *config.Max {
			return fmt.Errorf("invariant %q violated: node %s (%s) has %d %s edges (max: %d)",
				inv.Name, node.ID, config.NodeType, count, config.EdgeType, *config.Max)
		}
	}
	return nil
}

func (inv *Invariant) validateUniqueness(ctx *ValidationContext) error {
	config := inv.Config.(UniquenessConfig)
	nodes := ctx.GetNodesByType(config.NodeType)

	seen := make(map[interface{}]uuid.UUID)
	for _, node := range nodes {
		val, ok := node.Properties[config.Property]
		if !ok {
			continue
		}
		if existing, exists := seen[val]; exists {
			return fmt.Errorf("invariant %q violated: duplicate %s=%v on nodes %s and %s",
				inv.Name, config.Property, val, existing, node.ID)
		}
		seen[val] = node.ID
	}
	return nil
}

func (inv *Invariant) validateEdgeConstraint(ctx *ValidationContext) error {
	config := inv.Config.(EdgeConstraintConfig)
	edges := ctx.AllEdges()

	for _, edge := range edges {
		if edge.Type != config.EdgeType {
			continue
		}

		fromNode, ok := ctx.GetNode(edge.FromID)
		if !ok {
			continue
		}
		toNode, ok := ctx.GetNode(edge.ToID)
		if !ok {
			continue
		}

		if len(config.FromTypes) > 0 && !contains(config.FromTypes, fromNode.Type) {
			return fmt.Errorf("invariant %q violated: edge %s (%s) from node type %s not allowed (allowed: %v)",
				inv.Name, edge.ID, config.EdgeType, fromNode.Type, config.FromTypes)
		}
		if len(config.ToTypes) > 0 && !contains(config.ToTypes, toNode.Type) {
			return fmt.Errorf("invariant %q violated: edge %s (%s) to node type %s not allowed (allowed: %v)",
				inv.Name, edge.ID, config.EdgeType, toNode.Type, config.ToTypes)
		}
	}
	return nil
}

func (inv *Invariant) validateRequiredEdge(ctx *ValidationContext) error {
	config := inv.Config.(RequiredEdgeConfig)
	nodes := ctx.GetNodesByType(config.NodeType)

	for _, node := range nodes {
		var edges []*crdt.MaterializedEdge
		switch config.Direction {
		case "out":
			edges = ctx.GetOutEdges(node.ID)
		case "in":
			edges = ctx.GetInEdges(node.ID)
		}

		found := false
		for _, edge := range edges {
			if edge.Type == config.EdgeType {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("invariant %q violated: node %s (%s) missing required %s %s edge",
				inv.Name, node.ID, config.NodeType, config.Direction, config.EdgeType)
		}
	}
	return nil
}

func (inv *Invariant) validateAcyclicity(ctx *ValidationContext) error {
	config := inv.Config.(AcyclicityConfig)
	edges := ctx.AllEdges()

	// Build adjacency list for edges of this type
	adj := make(map[uuid.UUID][]uuid.UUID)
	for _, edge := range edges {
		if edge.Type == config.EdgeType {
			adj[edge.FromID] = append(adj[edge.FromID], edge.ToID)
		}
	}

	// DFS cycle detection
	white := 0 // unvisited
	gray := 1  // in current path
	black := 2 // fully processed
	colors := make(map[uuid.UUID]int)

	var hasCycle func(id uuid.UUID) bool
	hasCycle = func(id uuid.UUID) bool {
		colors[id] = gray
		for _, next := range adj[id] {
			if colors[next] == gray {
				return true // back edge = cycle
			}
			if colors[next] == white {
				if hasCycle(next) {
					return true
				}
			}
		}
		colors[id] = black
		return false
	}

	_ = white
	for id := range adj {
		if colors[id] == white {
			if hasCycle(id) {
				return fmt.Errorf("invariant %q violated: cycle detected in %s edges", inv.Name, config.EdgeType)
			}
		}
	}
	return nil
}

func (inv *Invariant) validateHierarchyDepth(ctx *ValidationContext) error {
	config := inv.Config.(HierarchyDepthConfig)
	nodes := ctx.AllNodes()

	for _, node := range nodes {
		if config.NodeType != "" && node.Type != config.NodeType {
			continue
		}

		// Count depth by walking up to root
		depth := 0
		current := node.ID
		for {
			parent, ok := ctx.GetParent(current)
			if !ok {
				break
			}
			depth++
			if depth > config.MaxDepth {
				return fmt.Errorf("invariant %q violated: node %s exceeds max hierarchy depth of %d",
					inv.Name, node.ID, config.MaxDepth)
			}
			current = parent.ID
		}
	}
	return nil
}

func (inv *Invariant) validateChildCount(ctx *ValidationContext) error {
	config := inv.Config.(ChildCountConfig)
	parents := ctx.GetNodesByType(config.ParentType)

	for _, parent := range parents {
		children := ctx.GetChildren(parent.ID)

		count := 0
		for _, child := range children {
			if config.ChildType == "" || child.Type == config.ChildType {
				count++
			}
		}

		if config.Min != nil && count < *config.Min {
			return fmt.Errorf("invariant %q violated: node %s (%s) has %d children (min: %d)",
				inv.Name, parent.ID, config.ParentType, count, *config.Min)
		}
		if config.Max != nil && count > *config.Max {
			return fmt.Errorf("invariant %q violated: node %s (%s) has %d children (max: %d)",
				inv.Name, parent.ID, config.ParentType, count, *config.Max)
		}
	}
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item || s == "*" {
			return true
		}
	}
	return false
}
