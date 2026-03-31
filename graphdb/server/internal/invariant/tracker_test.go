package invariant

import (
	"testing"

	"github.com/kern/graphdb/internal/crdt"
)

func TestDependencyTrackerInvariantRouting(t *testing.T) {
	dt := NewDependencyTracker()

	// Register invariants with different dependency patterns
	uniqueEmail := NewUniquenessInvariant("unique-email", UniquenessConfig{
		NodeType: "user",
		Property: "email",
	})
	noCycles := NewAcyclicityInvariant("no-dep-cycles", AcyclicityConfig{
		EdgeType: "depends_on",
	})
	maxFollows := NewCardinalityInvariant("max-follows", CardinalityConfig{
		NodeType:  "user",
		EdgeType:  "follows",
		Direction: "out",
		Max:       intPtr(10),
	})

	dt.RegisterInvariant(uniqueEmail)
	dt.RegisterInvariant(noCycles)
	dt.RegisterInvariant(maxFollows)

	// Insert user node: should affect uniqueEmail and maxFollows (both depend on "user" type)
	op := &crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "user",
	}
	affected := dt.GetAffectedInvariants(op)
	if len(affected) < 2 {
		t.Errorf("expected at least 2 affected invariants for user insert, got %d", len(affected))
	}

	// Insert edge of type "depends_on": should affect noCycles
	op = &crdt.Operation{
		Type:     crdt.OpInsertEdge,
		EdgeType: "depends_on",
	}
	affected = dt.GetAffectedInvariants(op)
	foundCycles := false
	for _, id := range affected {
		if id == noCycles.ID {
			foundCycles = true
		}
	}
	if !foundCycles {
		t.Error("depends_on edge insert should affect acyclicity invariant")
	}

	// Property change on "email": should affect uniqueEmail
	op = &crdt.Operation{
		Type: crdt.OpSetProperty,
		Key:  "email",
	}
	affected = dt.GetAffectedInvariants(op)
	foundEmail := false
	for _, id := range affected {
		if id == uniqueEmail.ID {
			foundEmail = true
		}
	}
	if !foundEmail {
		t.Error("email property change should affect uniqueness invariant")
	}
}

func TestDependencyTrackerCaching(t *testing.T) {
	dt := NewDependencyTracker()

	inv := NewUniquenessInvariant("unique-email", UniquenessConfig{
		NodeType: "user",
		Property: "email",
	})
	dt.RegisterInvariant(inv)

	// Cache a result
	dt.CacheResult(inv.ID, true, nil)

	valid, err, cached := dt.GetCachedResult(inv.ID)
	if !cached {
		t.Fatal("expected cached result")
	}
	if !valid || err != nil {
		t.Error("expected valid cached result")
	}

	// Invalidate via operation
	dt.InvalidateCache(&crdt.Operation{
		Type: crdt.OpSetProperty,
		Key:  "email",
	})

	_, _, cached = dt.GetCachedResult(inv.ID)
	if cached {
		t.Error("cache should be invalidated after relevant operation")
	}
}

func TestIncrementalValidator(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	iv := NewIncrementalValidator()

	iv.Add(NewUniquenessInvariant("unique-email", UniquenessConfig{
		NodeType: "user",
		Property: "email",
	}))
	iv.Add(NewAcyclicityInvariant("no-cycles", AcyclicityConfig{
		EdgeType: "depends_on",
	}))

	// Insert first user
	_, op1, _ := w.InsertNode("user", nil, map[string]interface{}{"email": "alice@example.com"})
	err := iv.ValidateIncremental(w, op1)
	if err != nil {
		t.Errorf("expected no violation, got: %v", err)
	}

	// Insert second user with same email
	_, op2, _ := w.InsertNode("user", nil, map[string]interface{}{"email": "alice@example.com"})
	err = iv.ValidateIncremental(w, op2)
	if err == nil {
		t.Error("expected uniqueness violation")
	}

	// Insert an edge (should only check acyclicity, not uniqueness)
	a, _, _ := w.InsertNode("task", nil, map[string]interface{}{})
	b, _, _ := w.InsertNode("task", nil, map[string]interface{}{})
	_, edgeOp, _ := w.InsertEdge("depends_on", a, b, nil)
	err = iv.ValidateIncremental(w, edgeOp)
	if err != nil {
		t.Errorf("expected no violation for acyclic edge, got: %v", err)
	}
}

func TestIncrementalValidatorUnrelatedChanges(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	iv := NewIncrementalValidator()

	// Only track uniqueness on "user" type
	iv.Add(NewUniquenessInvariant("unique-email", UniquenessConfig{
		NodeType: "user",
		Property: "email",
	}))

	// Insert a completely unrelated node type — should not trigger validation
	_, op, _ := w.InsertNode("product", nil, map[string]interface{}{"name": "Widget"})
	affected := iv.Tracker().GetAffectedInvariants(op)

	// Should have 0 affected (no invariant depends on "product" type)
	if len(affected) != 0 {
		t.Errorf("expected 0 affected invariants for unrelated type, got %d", len(affected))
	}
}
