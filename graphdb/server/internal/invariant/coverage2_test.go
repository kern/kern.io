package invariant

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
)

// ---------------------------------------------------------------------------
// invariant.go: validateEdgeConstraint — edge with missing from/to node
// ---------------------------------------------------------------------------

func TestEdgeConstraintMissingFromNode(t *testing.T) {
	w := crdt.NewEGWalker("r1")

	inv := NewEdgeConstraintInvariant("ec", EdgeConstraintConfig{
		EdgeType:  "link",
		FromTypes: []string{"user"},
		ToTypes:   []string{"team"},
	})

	// Create two nodes, insert edge, then delete the from-node
	fromID, _, _ := w.InsertNode("user", nil, nil)
	toID, _, _ := w.InsertNode("team", nil, nil)
	w.InsertEdge("link", fromID, toID, nil)

	// Delete the from-node so it's not found in the validation context
	w.DeleteNode(fromID)

	ctx := NewValidationContext(w)
	// Edge with deleted from-node should be skipped (continue), not error
	err := inv.Validate(ctx)
	if err != nil {
		t.Errorf("expected edge with deleted from-node to be skipped, got: %v", err)
	}
}

func TestEdgeConstraintMissingToNode(t *testing.T) {
	w := crdt.NewEGWalker("r1")

	inv := NewEdgeConstraintInvariant("ec", EdgeConstraintConfig{
		EdgeType:  "link",
		FromTypes: []string{"user"},
		ToTypes:   []string{"team"},
	})

	fromID, _, _ := w.InsertNode("user", nil, nil)
	toID, _, _ := w.InsertNode("team", nil, nil)
	w.InsertEdge("link", fromID, toID, nil)

	// Delete the to-node
	w.DeleteNode(toID)

	ctx := NewValidationContext(w)
	// Edge with deleted to-node should be skipped
	err := inv.Validate(ctx)
	if err != nil {
		t.Errorf("expected edge with deleted to-node to be skipped, got: %v", err)
	}
}

func TestEdgeConstraintFromTypeViolation(t *testing.T) {
	w := crdt.NewEGWalker("r1")

	inv := NewEdgeConstraintInvariant("ec", EdgeConstraintConfig{
		EdgeType:  "link",
		FromTypes: []string{"user"},
		ToTypes:   []string{}, // empty means allow any
	})

	// Create edge from "team" (not in FromTypes)
	fromID, _, _ := w.InsertNode("team", nil, nil)
	toID, _, _ := w.InsertNode("user", nil, nil)
	w.InsertEdge("link", fromID, toID, nil)

	ctx := NewValidationContext(w)
	err := inv.Validate(ctx)
	if err == nil {
		t.Error("expected from-type violation")
	}
}

func TestEdgeConstraintToTypeViolation(t *testing.T) {
	w := crdt.NewEGWalker("r1")

	inv := NewEdgeConstraintInvariant("ec", EdgeConstraintConfig{
		EdgeType:  "link",
		FromTypes: []string{}, // allow any
		ToTypes:   []string{"team"},
	})

	// Create edge to "user" (not in ToTypes)
	fromID, _, _ := w.InsertNode("user", nil, nil)
	toID, _, _ := w.InsertNode("user", nil, nil)
	w.InsertEdge("link", fromID, toID, nil)

	ctx := NewValidationContext(w)
	err := inv.Validate(ctx)
	if err == nil {
		t.Error("expected to-type violation")
	}
}

// ---------------------------------------------------------------------------
// tracker.go: UnregisterInvariant — removes from all maps
// ---------------------------------------------------------------------------

func TestUnregisterInvariantComprehensive(t *testing.T) {
	dt := NewDependencyTracker()

	// Register invariants covering all dep types
	cardInv := NewCardinalityInvariant("card", CardinalityConfig{
		NodeType: "user", EdgeType: "follows", Direction: "out", Max: intPtr(5),
	})
	uniqueInv := NewUniquenessInvariant("uniq", UniquenessConfig{
		NodeType: "user", Property: "email",
	})
	edgeInv := NewEdgeConstraintInvariant("ec", EdgeConstraintConfig{
		EdgeType: "link",
	})
	customInv := NewCustomInvariant("global", "desc", func(ctx *ValidationContext) error {
		return nil
	})
	hdInv := NewHierarchyDepthInvariant("hd", HierarchyDepthConfig{
		MaxDepth: 5,
	})

	dt.RegisterInvariant(cardInv)
	dt.RegisterInvariant(uniqueInv)
	dt.RegisterInvariant(edgeInv)
	dt.RegisterInvariant(customInv)
	dt.RegisterInvariant(hdInv)

	// Add a node-specific dep
	nodeID := uuid.New()
	dt.mu.Lock()
	dt.nodeDepInvariants[nodeID] = map[string]struct{}{cardInv.ID: {}}
	dt.mu.Unlock()

	// Cache results
	dt.CacheResult(cardInv.ID, true, nil)
	dt.CacheResult(customInv.ID, true, nil)

	// Unregister all
	dt.UnregisterInvariant(cardInv.ID)
	dt.UnregisterInvariant(uniqueInv.ID)
	dt.UnregisterInvariant(edgeInv.ID)
	dt.UnregisterInvariant(customInv.ID)
	dt.UnregisterInvariant(hdInv.ID)

	// All affected queries should return empty (except possibly empty globals)
	op := &crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "user",
	}
	affected := dt.GetAffectedInvariants(op)
	if len(affected) != 0 {
		t.Errorf("expected 0 affected after unregister all, got %d", len(affected))
	}

	// Caches should be cleared
	_, _, cached := dt.GetCachedResult(cardInv.ID)
	if cached {
		t.Error("cache should be cleared after unregister")
	}
}

// ---------------------------------------------------------------------------
// tracker.go: ValidateIncremental — no affected invariants (short circuit)
// ---------------------------------------------------------------------------

func TestValidateIncrementalNoAffected(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	iv := NewIncrementalValidator()

	// Only add a uniqueness invariant for "user"
	iv.Add(NewUniquenessInvariant("u", UniquenessConfig{
		NodeType: "user", Property: "email",
	}))

	// Insert a completely unrelated type
	_, op, _ := w.InsertNode("product", nil, nil)

	// Should return nil (no affected invariants)
	err := iv.ValidateIncremental(w, op)
	if err != nil {
		t.Errorf("expected nil for unrelated op, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// tracker.go: ValidateIncremental — cached valid result
// ---------------------------------------------------------------------------

func TestValidateIncrementalCachedValid(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	iv := NewIncrementalValidator()

	// Custom invariant (global, always checked)
	inv := NewCustomInvariant("always", "always check", func(ctx *ValidationContext) error {
		return nil
	})
	iv.Add(inv)

	// First call — validates and caches
	_, op1, _ := w.InsertNode("item", nil, nil)
	err := iv.ValidateIncremental(w, op1)
	if err != nil {
		t.Fatalf("first validation: %v", err)
	}

	// Manually set cache to valid
	iv.tracker.CacheResult(inv.ID, true, nil)

	// Second call with different op but cache is still valid
	_, op2, _ := w.InsertNode("item", nil, nil)
	// InvalidateCache will clear it, but let's verify the flow
	err = iv.ValidateIncremental(w, op2)
	if err != nil {
		t.Errorf("second validation: %v", err)
	}
}

// ---------------------------------------------------------------------------
// tracker.go: ValidateIncremental — cached invalid result
// ---------------------------------------------------------------------------

func TestValidateIncrementalCachedInvalid(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	iv := NewIncrementalValidator()

	inv := NewCustomInvariant("failing", "always fails", func(ctx *ValidationContext) error {
		return fmt.Errorf("always fail")
	})
	iv.Add(inv)

	_, op1, _ := w.InsertNode("item", nil, nil)
	err := iv.ValidateIncremental(w, op1)
	if err == nil {
		t.Error("expected violation from custom invariant")
	}

	// The result should now be cached as invalid.
	// Next validation: InvalidateCache clears it, then re-validates.
	_, op2, _ := w.InsertNode("item", nil, nil)
	err = iv.ValidateIncremental(w, op2)
	if err == nil {
		t.Error("expected violation again after re-validation")
	}
}

// ---------------------------------------------------------------------------
// tracker.go: ValidateIncremental — invariant not in invIndex (removed between calls)
// ---------------------------------------------------------------------------

func TestValidateIncrementalMissingInvIndex(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	iv := NewIncrementalValidator()

	inv := NewCustomInvariant("temp", "temp", func(ctx *ValidationContext) error {
		return nil
	})
	iv.Add(inv)

	// Remove from invIndex but leave in tracker (simulate race)
	delete(iv.invIndex, inv.ID)

	_, op, _ := w.InsertNode("item", nil, nil)
	err := iv.ValidateIncremental(w, op)
	if err != nil {
		t.Errorf("should gracefully skip missing inv: %v", err)
	}
}

// ---------------------------------------------------------------------------
// tracker.go: getAffectedUnlocked — all op types
// ---------------------------------------------------------------------------

func TestGetAffectedUnlockedAllOpTypes(t *testing.T) {
	dt := NewDependencyTracker()

	inv1 := NewUniquenessInvariant("u", UniquenessConfig{NodeType: "user", Property: "email"})
	inv2 := NewAcyclicityInvariant("ac", AcyclicityConfig{EdgeType: "dep"})
	dt.RegisterInvariant(inv1)
	dt.RegisterInvariant(inv2)

	// InsertNode
	dt.InvalidateCache(&crdt.Operation{Type: crdt.OpInsertNode, NodeType: "user"})

	// DeleteNode
	dt.InvalidateCache(&crdt.Operation{Type: crdt.OpDeleteNode, NodeType: "user"})

	// InsertEdge
	dt.InvalidateCache(&crdt.Operation{Type: crdt.OpInsertEdge, EdgeType: "dep"})

	// DeleteEdge
	dt.InvalidateCache(&crdt.Operation{Type: crdt.OpDeleteEdge, EdgeType: "dep"})

	// SetProperty
	dt.InvalidateCache(&crdt.Operation{Type: crdt.OpSetProperty, Key: "email"})

	// DeleteProperty
	dt.InvalidateCache(&crdt.Operation{Type: crdt.OpDeleteProperty, Key: "email"})

	// MoveNode — falls through to default in getAffectedUnlocked
	dt.InvalidateCache(&crdt.Operation{Type: crdt.OpMoveNode})

	// No panics = pass
}
