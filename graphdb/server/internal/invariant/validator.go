package invariant

import (
	"fmt"
	"sync"

	"github.com/kern/graphdb/internal/crdt"
)

// Validator manages a set of invariants and validates them against the graph.
type Validator struct {
	mu         sync.RWMutex
	invariants []*Invariant
}

// NewValidator creates a new validator.
func NewValidator() *Validator {
	return &Validator{}
}

// Add registers an invariant.
func (v *Validator) Add(inv *Invariant) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.invariants = append(v.invariants, inv)
}

// Remove removes an invariant by ID.
func (v *Validator) Remove(id string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i, inv := range v.invariants {
		if inv.ID == id {
			v.invariants = append(v.invariants[:i], v.invariants[i+1:]...)
			return
		}
	}
}

// List returns all registered invariants.
func (v *Validator) List() []*Invariant {
	v.mu.RLock()
	defer v.mu.RUnlock()
	result := make([]*Invariant, len(v.invariants))
	copy(result, v.invariants)
	return result
}

// ValidationError contains all invariant violations from a single validation pass.
type ValidationError struct {
	Violations []Violation `json:"violations"`
}

// Violation is a single invariant violation.
type Violation struct {
	InvariantID   string `json:"invariantId"`
	InvariantName string `json:"invariantName"`
	Message       string `json:"message"`
}

func (e *ValidationError) Error() string {
	if len(e.Violations) == 1 {
		return e.Violations[0].Message
	}
	return fmt.Sprintf("%d invariant violations", len(e.Violations))
}

// ValidateAll checks all invariants against the current graph state.
// Returns nil if all invariants pass.
func (v *Validator) ValidateAll(walker *crdt.EGWalker) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	ctx := NewValidationContext(walker)
	var violations []Violation

	for _, inv := range v.invariants {
		if err := inv.Validate(ctx); err != nil {
			violations = append(violations, Violation{
				InvariantID:   inv.ID,
				InvariantName: inv.Name,
				Message:       err.Error(),
			})
		}
	}

	if len(violations) > 0 {
		return &ValidationError{Violations: violations}
	}
	return nil
}

// ValidateSubset checks only the invariants that might be affected by an operation
// on a specific node type or edge type. This is an optimization to avoid
// re-checking all invariants on every mutation.
func (v *Validator) ValidateSubset(walker *crdt.EGWalker, nodeType, edgeType string) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	ctx := NewValidationContext(walker)
	var violations []Violation

	for _, inv := range v.invariants {
		if !inv.affectsType(nodeType, edgeType) {
			continue
		}
		if err := inv.Validate(ctx); err != nil {
			violations = append(violations, Violation{
				InvariantID:   inv.ID,
				InvariantName: inv.Name,
				Message:       err.Error(),
			})
		}
	}

	if len(violations) > 0 {
		return &ValidationError{Violations: violations}
	}
	return nil
}

// affectsType returns true if the invariant could be affected by changes
// to the given node type or edge type.
func (inv *Invariant) affectsType(nodeType, edgeType string) bool {
	switch inv.Type {
	case InvariantCardinality:
		cfg := inv.Config.(CardinalityConfig)
		return cfg.NodeType == nodeType || cfg.EdgeType == edgeType
	case InvariantUniqueness:
		cfg := inv.Config.(UniquenessConfig)
		return cfg.NodeType == nodeType
	case InvariantEdgeConstraint:
		cfg := inv.Config.(EdgeConstraintConfig)
		return cfg.EdgeType == edgeType
	case InvariantRequiredEdge:
		cfg := inv.Config.(RequiredEdgeConfig)
		return cfg.NodeType == nodeType || cfg.EdgeType == edgeType
	case InvariantAcyclicity:
		cfg := inv.Config.(AcyclicityConfig)
		return cfg.EdgeType == edgeType
	case InvariantHierarchyDepth:
		cfg := inv.Config.(HierarchyDepthConfig)
		return cfg.NodeType == "" || cfg.NodeType == nodeType
	case InvariantChildCount:
		cfg := inv.Config.(ChildCountConfig)
		return cfg.ParentType == nodeType || cfg.ChildType == nodeType
	case InvariantCustom:
		return true // always check custom invariants
	default:
		return true
	}
}
