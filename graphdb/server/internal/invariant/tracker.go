package invariant

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
)

// DependencyTracker tracks which invariants depend on which nodes/properties,
// so that when a change occurs, only the affected invariants are re-evaluated.
type DependencyTracker struct {
	mu sync.RWMutex

	// nodeType -> invariant IDs that depend on this type
	typeDepInvariants map[string]map[string]struct{}

	// edgeType -> invariant IDs that depend on this edge type
	edgeDepInvariants map[string]map[string]struct{}

	// (nodeType, property) -> invariant IDs that read this property
	propDepInvariants map[depKey]map[string]struct{}

	// nodeID -> invariant IDs that specifically reference this node
	nodeDepInvariants map[uuid.UUID]map[string]struct{}

	// invariantID -> last validation result (cached)
	resultCache map[string]*cachedResult

	// Global invariants that must always be checked
	globalInvariants map[string]struct{}
}

type depKey struct {
	NodeType string
	Property string
}

type cachedResult struct {
	valid bool
	err   error
	// Hash of inputs that produced this result
	inputHash uint64
}

// NewDependencyTracker creates a new dependency tracker.
func NewDependencyTracker() *DependencyTracker {
	return &DependencyTracker{
		typeDepInvariants: make(map[string]map[string]struct{}),
		edgeDepInvariants: make(map[string]map[string]struct{}),
		propDepInvariants: make(map[depKey]map[string]struct{}),
		nodeDepInvariants: make(map[uuid.UUID]map[string]struct{}),
		resultCache:       make(map[string]*cachedResult),
		globalInvariants:  make(map[string]struct{}),
	}
}

// RegisterInvariant analyzes an invariant and tracks its dependencies.
func (dt *DependencyTracker) RegisterInvariant(inv *Invariant) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	switch inv.Type {
	case InvariantCardinality:
		cfg := inv.Config.(CardinalityConfig)
		dt.addTypeDep(cfg.NodeType, inv.ID)
		dt.addEdgeDep(cfg.EdgeType, inv.ID)

	case InvariantUniqueness:
		cfg := inv.Config.(UniquenessConfig)
		dt.addTypeDep(cfg.NodeType, inv.ID)
		dt.addPropDep(cfg.NodeType, cfg.Property, inv.ID)

	case InvariantEdgeConstraint:
		cfg := inv.Config.(EdgeConstraintConfig)
		dt.addEdgeDep(cfg.EdgeType, inv.ID)

	case InvariantRequiredEdge:
		cfg := inv.Config.(RequiredEdgeConfig)
		dt.addTypeDep(cfg.NodeType, inv.ID)
		dt.addEdgeDep(cfg.EdgeType, inv.ID)

	case InvariantAcyclicity:
		cfg := inv.Config.(AcyclicityConfig)
		dt.addEdgeDep(cfg.EdgeType, inv.ID)

	case InvariantHierarchyDepth:
		cfg := inv.Config.(HierarchyDepthConfig)
		if cfg.NodeType != "" {
			dt.addTypeDep(cfg.NodeType, inv.ID)
		} else {
			dt.globalInvariants[inv.ID] = struct{}{}
		}

	case InvariantChildCount:
		cfg := inv.Config.(ChildCountConfig)
		dt.addTypeDep(cfg.ParentType, inv.ID)
		if cfg.ChildType != "" {
			dt.addTypeDep(cfg.ChildType, inv.ID)
		}

	case InvariantCustom:
		// Custom invariants are always re-checked (conservative)
		dt.globalInvariants[inv.ID] = struct{}{}
	}
}

// UnregisterInvariant removes an invariant from tracking.
func (dt *DependencyTracker) UnregisterInvariant(id string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	// Remove from all indexes
	for _, m := range dt.typeDepInvariants {
		delete(m, id)
	}
	for _, m := range dt.edgeDepInvariants {
		delete(m, id)
	}
	for _, m := range dt.propDepInvariants {
		delete(m, id)
	}
	for _, m := range dt.nodeDepInvariants {
		delete(m, id)
	}
	delete(dt.globalInvariants, id)
	delete(dt.resultCache, id)
}

// GetAffectedInvariants returns the IDs of invariants that need
// re-evaluation after a specific operation.
func (dt *DependencyTracker) GetAffectedInvariants(op *crdt.Operation) []string {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	affected := make(map[string]struct{})

	// Always include global invariants
	for id := range dt.globalInvariants {
		affected[id] = struct{}{}
	}

	switch op.Type {
	case crdt.OpInsertNode, crdt.OpDeleteNode:
		// Affects all invariants depending on this node type
		if deps, ok := dt.typeDepInvariants[op.NodeType]; ok {
			for id := range deps {
				affected[id] = struct{}{}
			}
		}
		// Also affects node-specific invariants
		if deps, ok := dt.nodeDepInvariants[op.TargetID]; ok {
			for id := range deps {
				affected[id] = struct{}{}
			}
		}

	case crdt.OpSetProperty, crdt.OpDeleteProperty:
		// Look up the node to find its type
		// (We use nodeType from the node that owns this property)
		if deps, ok := dt.nodeDepInvariants[op.TargetID]; ok {
			for id := range deps {
				affected[id] = struct{}{}
			}
		}
		// Also check property-specific dependencies
		// We need to check all type combos since we may not know the node type here
		for dk, deps := range dt.propDepInvariants {
			if dk.Property == op.Key {
				for id := range deps {
					affected[id] = struct{}{}
				}
			}
		}

	case crdt.OpInsertEdge, crdt.OpDeleteEdge:
		if deps, ok := dt.edgeDepInvariants[op.EdgeType]; ok {
			for id := range deps {
				affected[id] = struct{}{}
			}
		}

	case crdt.OpMoveNode:
		// Hierarchy change: affects depth and child count invariants
		for _, m := range dt.typeDepInvariants {
			for id := range m {
				affected[id] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(affected))
	for id := range affected {
		result = append(result, id)
	}
	return result
}

// InvalidateCache marks cached results as stale for affected invariants.
func (dt *DependencyTracker) InvalidateCache(op *crdt.Operation) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	affected := dt.getAffectedUnlocked(op)
	for _, id := range affected {
		delete(dt.resultCache, id)
	}
}

// GetCachedResult returns a cached validation result if available.
func (dt *DependencyTracker) GetCachedResult(id string) (bool, error, bool) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	if cached, ok := dt.resultCache[id]; ok {
		return cached.valid, cached.err, true
	}
	return false, nil, false
}

// CacheResult stores a validation result.
func (dt *DependencyTracker) CacheResult(id string, valid bool, err error) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	dt.resultCache[id] = &cachedResult{valid: valid, err: err}
}

func (dt *DependencyTracker) getAffectedUnlocked(op *crdt.Operation) []string {
	affected := make(map[string]struct{})
	for id := range dt.globalInvariants {
		affected[id] = struct{}{}
	}
	switch op.Type {
	case crdt.OpInsertNode, crdt.OpDeleteNode:
		if deps, ok := dt.typeDepInvariants[op.NodeType]; ok {
			for id := range deps {
				affected[id] = struct{}{}
			}
		}
	case crdt.OpInsertEdge, crdt.OpDeleteEdge:
		if deps, ok := dt.edgeDepInvariants[op.EdgeType]; ok {
			for id := range deps {
				affected[id] = struct{}{}
			}
		}
	case crdt.OpSetProperty, crdt.OpDeleteProperty:
		for dk, deps := range dt.propDepInvariants {
			if dk.Property == op.Key {
				for id := range deps {
					affected[id] = struct{}{}
				}
			}
		}
	}
	result := make([]string, 0, len(affected))
	for id := range affected {
		result = append(result, id)
	}
	return result
}

func (dt *DependencyTracker) addTypeDep(nodeType, invariantID string) {
	if dt.typeDepInvariants[nodeType] == nil {
		dt.typeDepInvariants[nodeType] = make(map[string]struct{})
	}
	dt.typeDepInvariants[nodeType][invariantID] = struct{}{}
}

func (dt *DependencyTracker) addEdgeDep(edgeType, invariantID string) {
	if dt.edgeDepInvariants[edgeType] == nil {
		dt.edgeDepInvariants[edgeType] = make(map[string]struct{})
	}
	dt.edgeDepInvariants[edgeType][invariantID] = struct{}{}
}

func (dt *DependencyTracker) addPropDep(nodeType, property, invariantID string) {
	dk := depKey{NodeType: nodeType, Property: property}
	if dt.propDepInvariants[dk] == nil {
		dt.propDepInvariants[dk] = make(map[string]struct{})
	}
	dt.propDepInvariants[dk][invariantID] = struct{}{}
}

// IncrementalValidator wraps Validator with dependency tracking for efficient
// incremental validation. Instead of checking all invariants on every change,
// it only checks the ones whose dependencies were affected.
type IncrementalValidator struct {
	validator *Validator
	tracker   *DependencyTracker
	// index: invariantID -> *Invariant
	invIndex map[string]*Invariant
}

// NewIncrementalValidator creates an incremental validator.
func NewIncrementalValidator() *IncrementalValidator {
	return &IncrementalValidator{
		validator: NewValidator(),
		tracker:   NewDependencyTracker(),
		invIndex:  make(map[string]*Invariant),
	}
}

// Add registers an invariant with dependency tracking.
func (iv *IncrementalValidator) Add(inv *Invariant) {
	iv.validator.Add(inv)
	iv.tracker.RegisterInvariant(inv)
	iv.invIndex[inv.ID] = inv
}

// Remove removes an invariant.
func (iv *IncrementalValidator) Remove(id string) {
	iv.validator.Remove(id)
	iv.tracker.UnregisterInvariant(id)
	delete(iv.invIndex, id)
}

// ValidateIncremental checks only the invariants affected by an operation.
// This is O(affected) instead of O(all invariants).
func (iv *IncrementalValidator) ValidateIncremental(walker *crdt.EGWalker, op *crdt.Operation) error {
	// Invalidate cached results for affected invariants
	iv.tracker.InvalidateCache(op)

	// Get affected invariant IDs
	affectedIDs := iv.tracker.GetAffectedInvariants(op)
	if len(affectedIDs) == 0 {
		return nil
	}

	ctx := NewValidationContext(walker)
	var violations []Violation

	for _, id := range affectedIDs {
		// Check cache first
		if valid, err, cached := iv.tracker.GetCachedResult(id); cached {
			if !valid {
				violations = append(violations, Violation{
					InvariantID:   id,
					InvariantName: iv.invIndex[id].Name,
					Message:       err.Error(),
				})
			}
			continue
		}

		inv, ok := iv.invIndex[id]
		if !ok {
			continue
		}

		err := inv.Validate(ctx)
		if err != nil {
			violations = append(violations, Violation{
				InvariantID:   id,
				InvariantName: inv.Name,
				Message:       err.Error(),
			})
			iv.tracker.CacheResult(id, false, err)
		} else {
			iv.tracker.CacheResult(id, true, nil)
		}
	}

	if len(violations) > 0 {
		return &ValidationError{Violations: violations}
	}
	return nil
}

// ValidateAll checks all invariants (non-incremental fallback).
func (iv *IncrementalValidator) ValidateAll(walker *crdt.EGWalker) error {
	return iv.validator.ValidateAll(walker)
}

// List returns all registered invariants.
func (iv *IncrementalValidator) List() []*Invariant {
	return iv.validator.List()
}

// Tracker returns the dependency tracker.
func (iv *IncrementalValidator) Tracker() *DependencyTracker {
	return iv.tracker
}

// ensure imports are used
var _ = fmt.Sprintf
var _ = uuid.Nil
