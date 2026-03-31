package invariant

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
)

// ---------------------------------------------------------------------------
// invariant.go coverage
// ---------------------------------------------------------------------------

func TestGetNodeAndGetInEdges(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	id1, _, _ := w.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	id2, _, _ := w.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})
	w.InsertEdge("follows", id1, id2, nil)

	ctx := NewValidationContext(w)

	// GetNode existing
	node, ok := ctx.GetNode(id1)
	if !ok || node == nil {
		t.Fatal("expected to find node")
	}

	// GetNode non-existing
	_, ok = ctx.GetNode(uuid.New())
	if ok {
		t.Fatal("expected not found for random UUID")
	}

	// GetInEdges
	inEdges := ctx.GetInEdges(id2)
	if len(inEdges) != 1 {
		t.Fatalf("expected 1 in-edge, got %d", len(inEdges))
	}
	if inEdges[0].Type != "follows" {
		t.Errorf("expected edge type 'follows', got %q", inEdges[0].Type)
	}
}

func TestNewEdgeConstraintInvariantAndValidation(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	inv := NewEdgeConstraintInvariant("user-to-team", EdgeConstraintConfig{
		EdgeType:  "member_of",
		FromTypes: []string{"user"},
		ToTypes:   []string{"team"},
	})
	v.Add(inv)

	user, _, _ := w.InsertNode("user", nil, map[string]interface{}{})
	team, _, _ := w.InsertNode("team", nil, map[string]interface{}{})

	// Valid edge: user -> team
	w.InsertEdge("member_of", user, team, nil)
	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation, got: %v", err)
	}

	// Invalid edge: team -> user (team not in FromTypes)
	w.InsertEdge("member_of", team, user, nil)
	err := v.ValidateAll(w)
	if err == nil {
		t.Error("expected edge constraint violation for invalid fromType")
	}
}

func TestEdgeConstraintInvalidToType(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	inv := NewEdgeConstraintInvariant("only-to-team", EdgeConstraintConfig{
		EdgeType: "member_of",
		ToTypes:  []string{"team"},
	})
	v.Add(inv)

	user1, _, _ := w.InsertNode("user", nil, map[string]interface{}{})
	user2, _, _ := w.InsertNode("user", nil, map[string]interface{}{})

	// Edge to wrong type
	w.InsertEdge("member_of", user1, user2, nil)
	err := v.ValidateAll(w)
	if err == nil {
		t.Error("expected edge constraint violation for invalid toType")
	}
}

func TestNewRequiredEdgeInvariantAndValidation(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	// Require every user to have an outgoing "belongs_to" edge
	inv := NewRequiredEdgeInvariant("user-must-belong", RequiredEdgeConfig{
		NodeType:  "user",
		EdgeType:  "belongs_to",
		Direction: "out",
	})
	v.Add(inv)

	user, _, _ := w.InsertNode("user", nil, map[string]interface{}{})
	team, _, _ := w.InsertNode("team", nil, map[string]interface{}{})

	// No edge yet -> violation
	err := v.ValidateAll(w)
	if err == nil {
		t.Error("expected required edge violation")
	}

	// Add the required edge
	w.InsertEdge("belongs_to", user, team, nil)
	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation after adding required edge, got: %v", err)
	}
}

func TestRequiredEdgeInDirection(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	// Require every team to have at least one incoming "belongs_to" edge
	inv := NewRequiredEdgeInvariant("team-needs-member", RequiredEdgeConfig{
		NodeType:  "team",
		EdgeType:  "belongs_to",
		Direction: "in",
	})
	v.Add(inv)

	user, _, _ := w.InsertNode("user", nil, map[string]interface{}{})
	team, _, _ := w.InsertNode("team", nil, map[string]interface{}{})

	// No incoming edge on team -> violation
	err := v.ValidateAll(w)
	if err == nil {
		t.Error("expected required edge violation for 'in' direction")
	}

	// Add incoming edge on team
	w.InsertEdge("belongs_to", user, team, nil)
	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation, got: %v", err)
	}
}

func TestValidateUnknownType(t *testing.T) {
	inv := &Invariant{
		ID:   "test-unknown",
		Name: "unknown",
		Type: InvariantType("bogus_type"),
	}
	w := crdt.NewEGWalker("r1")
	ctx := NewValidationContext(w)

	err := inv.Validate(ctx)
	if err == nil {
		t.Fatal("expected error for unknown invariant type")
	}
	if !strings.Contains(err.Error(), "unknown invariant type") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCardinalityInDirection(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewCardinalityInvariant("max-in-follows", CardinalityConfig{
		NodeType:  "user",
		EdgeType:  "follows",
		Direction: "in",
		Max:       intPtr(1),
	}))

	u1, _, _ := w.InsertNode("user", nil, map[string]interface{}{})
	u2, _, _ := w.InsertNode("user", nil, map[string]interface{}{})
	u3, _, _ := w.InsertNode("user", nil, map[string]interface{}{})

	// u2 and u3 follow u1 -> u1 has 2 in-edges
	w.InsertEdge("follows", u2, u1, nil)
	w.InsertEdge("follows", u3, u1, nil)

	err := v.ValidateAll(w)
	if err == nil {
		t.Error("expected cardinality violation for 'in' direction")
	}
}

func TestCardinalityMinViolation(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewCardinalityInvariant("min-in-edges", CardinalityConfig{
		NodeType:  "user",
		EdgeType:  "follows",
		Direction: "in",
		Min:       intPtr(2),
	}))

	u1, _, _ := w.InsertNode("user", nil, map[string]interface{}{})
	u2, _, _ := w.InsertNode("user", nil, map[string]interface{}{})

	// u1 has only 1 in-edge, min is 2
	w.InsertEdge("follows", u2, u1, nil)

	err := v.ValidateAll(w)
	if err == nil {
		t.Error("expected cardinality min violation")
	}
}

func TestUniquenessWithMissingProperty(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewUniquenessInvariant("unique-email", UniquenessConfig{
		NodeType: "user",
		Property: "email",
	}))

	// One user with email, one without -> should pass (missing property skipped)
	w.InsertNode("user", nil, map[string]interface{}{"email": "a@b.com"})
	w.InsertNode("user", nil, map[string]interface{}{"name": "NoEmail"})

	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation when property is missing, got: %v", err)
	}
}

func TestHierarchyDepthWithNodeTypeFilter(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	// Limit depth only for "folder" type nodes
	v.Add(NewHierarchyDepthInvariant("folder-depth", HierarchyDepthConfig{
		NodeType: "folder",
		MaxDepth: 1,
	}))

	root, _, _ := w.InsertNode("folder", nil, map[string]interface{}{})
	child, _, _ := w.InsertNode("folder", &root, map[string]interface{}{})

	// folder at depth 1 -> ok
	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation at depth 1, got: %v", err)
	}

	// folder at depth 2 -> exceeds limit
	w.InsertNode("folder", &child, map[string]interface{}{})
	err := v.ValidateAll(w)
	if err == nil {
		t.Error("expected hierarchy depth violation for folder type")
	}

	// A non-folder deep node should be ignored
	w2 := crdt.NewEGWalker("r2")
	v2 := NewValidator()
	v2.Add(NewHierarchyDepthInvariant("folder-depth-only", HierarchyDepthConfig{
		NodeType: "folder",
		MaxDepth: 1,
	}))
	r, _, _ := w2.InsertNode("org", nil, map[string]interface{}{})
	c, _, _ := w2.InsertNode("org", &r, map[string]interface{}{})
	w2.InsertNode("org", &c, map[string]interface{}{})

	if err := v2.ValidateAll(w2); err != nil {
		t.Errorf("non-folder nodes should be ignored, got: %v", err)
	}
}

func TestChildCountMinViolation(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewChildCountInvariant("min-children", ChildCountConfig{
		ParentType: "org",
		Min:        intPtr(2),
	}))

	org, _, _ := w.InsertNode("org", nil, map[string]interface{}{})
	w.InsertNode("team", &org, map[string]interface{}{})

	// Only 1 child, min is 2
	err := v.ValidateAll(w)
	if err == nil {
		t.Error("expected child count min violation")
	}
}

func TestChildCountEmptyChildType(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	// Empty ChildType -> count all children regardless of type
	v.Add(NewChildCountInvariant("min-any-children", ChildCountConfig{
		ParentType: "org",
		ChildType:  "",
		Min:        intPtr(2),
	}))

	org, _, _ := w.InsertNode("org", nil, map[string]interface{}{})
	w.InsertNode("team", &org, map[string]interface{}{})

	// 1 child of type "team", min is 2 (all types counted)
	err := v.ValidateAll(w)
	if err == nil {
		t.Error("expected child count min violation with empty ChildType")
	}

	// Add another child of a different type
	w.InsertNode("department", &org, map[string]interface{}{})
	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation with 2 children, got: %v", err)
	}
}

func TestContainsHelper(t *testing.T) {
	// Direct test of contains() function
	if !contains([]string{"a", "b", "c"}, "b") {
		t.Error("expected contains to find 'b'")
	}
	if contains([]string{"a", "b", "c"}, "d") {
		t.Error("expected contains to NOT find 'd'")
	}
	// Wildcard
	if !contains([]string{"*"}, "anything") {
		t.Error("expected wildcard to match anything")
	}
	if contains([]string{}, "a") {
		t.Error("expected empty slice to not contain anything")
	}
}

// ---------------------------------------------------------------------------
// validator.go coverage
// ---------------------------------------------------------------------------

func TestValidatorRemove(t *testing.T) {
	v := NewValidator()

	inv1 := NewUniquenessInvariant("inv1", UniquenessConfig{NodeType: "a", Property: "p"})
	inv2 := NewUniquenessInvariant("inv2", UniquenessConfig{NodeType: "b", Property: "q"})
	v.Add(inv1)
	v.Add(inv2)

	if len(v.List()) != 2 {
		t.Fatalf("expected 2 invariants, got %d", len(v.List()))
	}

	v.Remove(inv1.ID)
	list := v.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 invariant after remove, got %d", len(list))
	}
	if list[0].ID != inv2.ID {
		t.Errorf("expected remaining invariant to be inv2")
	}

	// Remove non-existent ID (no-op, should not panic)
	v.Remove("nonexistent-id")
	if len(v.List()) != 1 {
		t.Error("removing nonexistent ID should not change list")
	}
}

func TestValidatorList(t *testing.T) {
	v := NewValidator()
	list := v.List()
	if len(list) != 0 {
		t.Error("expected empty list for new validator")
	}

	inv := NewUniquenessInvariant("inv", UniquenessConfig{NodeType: "a", Property: "p"})
	v.Add(inv)

	list = v.List()
	if len(list) != 1 {
		t.Fatal("expected 1 invariant")
	}
	// Ensure it is a copy
	list[0] = nil
	if v.List()[0] == nil {
		t.Error("List should return a copy, not the internal slice")
	}
}

func TestValidationErrorSingleViolation(t *testing.T) {
	ve := &ValidationError{
		Violations: []Violation{
			{InvariantID: "1", InvariantName: "test", Message: "single failure"},
		},
	}
	if ve.Error() != "single failure" {
		t.Errorf("single violation error = %q, want %q", ve.Error(), "single failure")
	}
}

func TestValidationErrorMultipleViolations(t *testing.T) {
	ve := &ValidationError{
		Violations: []Violation{
			{Message: "fail 1"},
			{Message: "fail 2"},
			{Message: "fail 3"},
		},
	}
	expected := "3 invariant violations"
	if ve.Error() != expected {
		t.Errorf("multi violation error = %q, want %q", ve.Error(), expected)
	}
}

func TestAffectsTypeAllBranches(t *testing.T) {
	tests := []struct {
		name     string
		inv      *Invariant
		nodeType string
		edgeType string
		want     bool
	}{
		{
			name:     "cardinality matches nodeType",
			inv:      NewCardinalityInvariant("c", CardinalityConfig{NodeType: "user", EdgeType: "follows", Direction: "out"}),
			nodeType: "user",
			edgeType: "",
			want:     true,
		},
		{
			name:     "cardinality matches edgeType",
			inv:      NewCardinalityInvariant("c", CardinalityConfig{NodeType: "user", EdgeType: "follows", Direction: "out"}),
			nodeType: "",
			edgeType: "follows",
			want:     true,
		},
		{
			name:     "cardinality no match",
			inv:      NewCardinalityInvariant("c", CardinalityConfig{NodeType: "user", EdgeType: "follows", Direction: "out"}),
			nodeType: "team",
			edgeType: "likes",
			want:     false,
		},
		{
			name:     "uniqueness matches",
			inv:      NewUniquenessInvariant("u", UniquenessConfig{NodeType: "user", Property: "email"}),
			nodeType: "user",
			edgeType: "",
			want:     true,
		},
		{
			name:     "uniqueness no match",
			inv:      NewUniquenessInvariant("u", UniquenessConfig{NodeType: "user", Property: "email"}),
			nodeType: "team",
			edgeType: "",
			want:     false,
		},
		{
			name:     "edge constraint matches",
			inv:      NewEdgeConstraintInvariant("ec", EdgeConstraintConfig{EdgeType: "member_of"}),
			nodeType: "",
			edgeType: "member_of",
			want:     true,
		},
		{
			name:     "edge constraint no match",
			inv:      NewEdgeConstraintInvariant("ec", EdgeConstraintConfig{EdgeType: "member_of"}),
			nodeType: "",
			edgeType: "follows",
			want:     false,
		},
		{
			name:     "required edge matches nodeType",
			inv:      NewRequiredEdgeInvariant("re", RequiredEdgeConfig{NodeType: "user", EdgeType: "belongs_to", Direction: "out"}),
			nodeType: "user",
			edgeType: "",
			want:     true,
		},
		{
			name:     "required edge matches edgeType",
			inv:      NewRequiredEdgeInvariant("re", RequiredEdgeConfig{NodeType: "user", EdgeType: "belongs_to", Direction: "out"}),
			nodeType: "",
			edgeType: "belongs_to",
			want:     true,
		},
		{
			name:     "required edge no match",
			inv:      NewRequiredEdgeInvariant("re", RequiredEdgeConfig{NodeType: "user", EdgeType: "belongs_to", Direction: "out"}),
			nodeType: "team",
			edgeType: "follows",
			want:     false,
		},
		{
			name:     "acyclicity matches",
			inv:      NewAcyclicityInvariant("ac", AcyclicityConfig{EdgeType: "depends_on"}),
			nodeType: "",
			edgeType: "depends_on",
			want:     true,
		},
		{
			name:     "acyclicity no match",
			inv:      NewAcyclicityInvariant("ac", AcyclicityConfig{EdgeType: "depends_on"}),
			nodeType: "",
			edgeType: "follows",
			want:     false,
		},
		{
			name:     "hierarchy depth empty nodeType (global)",
			inv:      NewHierarchyDepthInvariant("hd", HierarchyDepthConfig{MaxDepth: 5}),
			nodeType: "anything",
			edgeType: "",
			want:     true,
		},
		{
			name:     "hierarchy depth specific nodeType match",
			inv:      NewHierarchyDepthInvariant("hd", HierarchyDepthConfig{NodeType: "folder", MaxDepth: 5}),
			nodeType: "folder",
			edgeType: "",
			want:     true,
		},
		{
			name:     "hierarchy depth specific nodeType no match",
			inv:      NewHierarchyDepthInvariant("hd", HierarchyDepthConfig{NodeType: "folder", MaxDepth: 5}),
			nodeType: "file",
			edgeType: "",
			want:     false,
		},
		{
			name:     "child count matches parentType",
			inv:      NewChildCountInvariant("cc", ChildCountConfig{ParentType: "org", ChildType: "team"}),
			nodeType: "org",
			edgeType: "",
			want:     true,
		},
		{
			name:     "child count matches childType",
			inv:      NewChildCountInvariant("cc", ChildCountConfig{ParentType: "org", ChildType: "team"}),
			nodeType: "team",
			edgeType: "",
			want:     true,
		},
		{
			name:     "child count no match",
			inv:      NewChildCountInvariant("cc", ChildCountConfig{ParentType: "org", ChildType: "team"}),
			nodeType: "user",
			edgeType: "",
			want:     false,
		},
		{
			name: "custom always true",
			inv:  NewCustomInvariant("cust", "desc", nil),
			nodeType: "anything",
			edgeType: "anything",
			want:     true,
		},
		{
			name: "default/unknown type always true",
			inv: &Invariant{
				Type: InvariantType("unknown_type"),
			},
			nodeType: "",
			edgeType: "",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.inv.affectsType(tt.nodeType, tt.edgeType)
			if got != tt.want {
				t.Errorf("affectsType(%q, %q) = %v, want %v", tt.nodeType, tt.edgeType, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// tracker.go coverage
// ---------------------------------------------------------------------------

func TestRegisterEdgeConstraintInvariant(t *testing.T) {
	dt := NewDependencyTracker()

	inv := NewEdgeConstraintInvariant("ec", EdgeConstraintConfig{
		EdgeType:  "member_of",
		FromTypes: []string{"user"},
		ToTypes:   []string{"team"},
	})
	dt.RegisterInvariant(inv)

	op := &crdt.Operation{
		Type:     crdt.OpInsertEdge,
		EdgeType: "member_of",
	}
	affected := dt.GetAffectedInvariants(op)
	found := false
	for _, id := range affected {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("edge constraint invariant should be affected by member_of edge insert")
	}
}

func TestRegisterRequiredEdgeInvariant(t *testing.T) {
	dt := NewDependencyTracker()

	inv := NewRequiredEdgeInvariant("re", RequiredEdgeConfig{
		NodeType:  "user",
		EdgeType:  "belongs_to",
		Direction: "out",
	})
	dt.RegisterInvariant(inv)

	// Affected by node type
	op := &crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "user",
	}
	affected := dt.GetAffectedInvariants(op)
	found := false
	for _, id := range affected {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("required edge invariant should be affected by user node insert")
	}

	// Affected by edge type
	op = &crdt.Operation{
		Type:     crdt.OpInsertEdge,
		EdgeType: "belongs_to",
	}
	affected = dt.GetAffectedInvariants(op)
	found = false
	for _, id := range affected {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("required edge invariant should be affected by belongs_to edge insert")
	}
}

func TestRegisterHierarchyDepthWithNodeType(t *testing.T) {
	dt := NewDependencyTracker()

	inv := NewHierarchyDepthInvariant("hd-folder", HierarchyDepthConfig{
		NodeType: "folder",
		MaxDepth: 3,
	})
	dt.RegisterInvariant(inv)

	// Should be in type deps, not global
	op := &crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "folder",
	}
	affected := dt.GetAffectedInvariants(op)
	found := false
	for _, id := range affected {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("hierarchy depth invariant with nodeType should be affected by folder insert")
	}

	// Unrelated type should NOT trigger it
	op2 := &crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "file",
	}
	affected2 := dt.GetAffectedInvariants(op2)
	for _, id := range affected2 {
		if id == inv.ID {
			t.Error("hierarchy depth invariant for 'folder' should NOT be affected by 'file' insert")
		}
	}
}

func TestRegisterChildCountInvariant(t *testing.T) {
	dt := NewDependencyTracker()

	inv := NewChildCountInvariant("cc", ChildCountConfig{
		ParentType: "org",
		ChildType:  "team",
		Max:        intPtr(5),
	})
	dt.RegisterInvariant(inv)

	// Affected by parent type
	affected := dt.GetAffectedInvariants(&crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "org",
	})
	found := false
	for _, id := range affected {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("child count invariant should be affected by org node insert")
	}

	// Affected by child type
	affected = dt.GetAffectedInvariants(&crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "team",
	})
	found = false
	for _, id := range affected {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("child count invariant should be affected by team node insert")
	}
}

func TestRegisterChildCountInvariantEmptyChildType(t *testing.T) {
	dt := NewDependencyTracker()

	inv := NewChildCountInvariant("cc-all", ChildCountConfig{
		ParentType: "org",
		ChildType:  "", // empty = all children
		Max:        intPtr(10),
	})
	dt.RegisterInvariant(inv)

	// Should be affected by parent type
	affected := dt.GetAffectedInvariants(&crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "org",
	})
	found := false
	for _, id := range affected {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("child count invariant with empty childType should be affected by org insert")
	}
}

func TestUnregisterInvariant(t *testing.T) {
	dt := NewDependencyTracker()

	inv := NewCardinalityInvariant("card", CardinalityConfig{
		NodeType:  "user",
		EdgeType:  "follows",
		Direction: "out",
		Max:       intPtr(5),
	})
	dt.RegisterInvariant(inv)

	// Cache a result for it
	dt.CacheResult(inv.ID, true, nil)

	// Verify it is tracked
	affected := dt.GetAffectedInvariants(&crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "user",
	})
	if len(affected) == 0 {
		t.Fatal("expected invariant to be tracked")
	}

	// Unregister
	dt.UnregisterInvariant(inv.ID)

	// Should no longer be affected
	affected = dt.GetAffectedInvariants(&crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "user",
	})
	for _, id := range affected {
		if id == inv.ID {
			t.Error("unregistered invariant should not appear in affected list")
		}
	}

	// Cache should also be cleared
	_, _, cached := dt.GetCachedResult(inv.ID)
	if cached {
		t.Error("cache should be cleared after unregister")
	}
}

func TestIncrementalValidatorRemove(t *testing.T) {
	iv := NewIncrementalValidator()

	inv := NewUniquenessInvariant("u", UniquenessConfig{NodeType: "user", Property: "email"})
	iv.Add(inv)

	if len(iv.List()) != 1 {
		t.Fatal("expected 1 invariant")
	}

	iv.Remove(inv.ID)

	if len(iv.List()) != 0 {
		t.Error("expected 0 invariants after remove")
	}
}

func TestIncrementalValidatorValidateAll(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	iv := NewIncrementalValidator()

	iv.Add(NewUniquenessInvariant("u", UniquenessConfig{NodeType: "user", Property: "email"}))

	w.InsertNode("user", nil, map[string]interface{}{"email": "a@b.com"})
	if err := iv.ValidateAll(w); err != nil {
		t.Errorf("expected no violation, got: %v", err)
	}

	w.InsertNode("user", nil, map[string]interface{}{"email": "a@b.com"})
	err := iv.ValidateAll(w)
	if err == nil {
		t.Error("expected uniqueness violation from ValidateAll")
	}
}

func TestIncrementalValidatorList(t *testing.T) {
	iv := NewIncrementalValidator()

	inv1 := NewUniquenessInvariant("u1", UniquenessConfig{NodeType: "a", Property: "p"})
	inv2 := NewAcyclicityInvariant("a1", AcyclicityConfig{EdgeType: "e"})
	iv.Add(inv1)
	iv.Add(inv2)

	list := iv.List()
	if len(list) != 2 {
		t.Errorf("expected 2 invariants, got %d", len(list))
	}
}

func TestGetAffectedInvariantsOpMoveNode(t *testing.T) {
	dt := NewDependencyTracker()

	inv1 := NewChildCountInvariant("cc", ChildCountConfig{
		ParentType: "org",
		ChildType:  "team",
		Max:        intPtr(5),
	})
	inv2 := NewHierarchyDepthInvariant("hd", HierarchyDepthConfig{
		NodeType: "folder",
		MaxDepth: 3,
	})
	inv3 := NewUniquenessInvariant("u", UniquenessConfig{
		NodeType: "user",
		Property: "email",
	})

	dt.RegisterInvariant(inv1)
	dt.RegisterInvariant(inv2)
	dt.RegisterInvariant(inv3)

	// OpMoveNode should affect all type-dep invariants
	op := &crdt.Operation{
		Type:     crdt.OpMoveNode,
		TargetID: uuid.New(),
	}
	affected := dt.GetAffectedInvariants(op)

	// Should include inv1 (org type dep), inv2 (folder type dep), inv3 (user type dep)
	// because OpMoveNode iterates all typeDepInvariants
	ids := make(map[string]bool)
	for _, id := range affected {
		ids[id] = true
	}
	if !ids[inv1.ID] {
		t.Error("OpMoveNode should affect child count invariant")
	}
	if !ids[inv2.ID] {
		t.Error("OpMoveNode should affect hierarchy depth invariant")
	}
	if !ids[inv3.ID] {
		t.Error("OpMoveNode should affect uniqueness invariant (all type deps)")
	}
}

func TestGetAffectedInvariantsDeleteNode(t *testing.T) {
	dt := NewDependencyTracker()

	inv := NewUniquenessInvariant("u", UniquenessConfig{
		NodeType: "user",
		Property: "email",
	})
	dt.RegisterInvariant(inv)

	nodeID := uuid.New()
	op := &crdt.Operation{
		Type:     crdt.OpDeleteNode,
		TargetID: nodeID,
		NodeType: "user",
	}
	affected := dt.GetAffectedInvariants(op)

	found := false
	for _, id := range affected {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("DeleteNode with matching nodeType should affect uniqueness invariant")
	}
}

func TestGetAffectedInvariantsDeleteEdge(t *testing.T) {
	dt := NewDependencyTracker()

	inv := NewAcyclicityInvariant("ac", AcyclicityConfig{EdgeType: "depends_on"})
	dt.RegisterInvariant(inv)

	op := &crdt.Operation{
		Type:     crdt.OpDeleteEdge,
		EdgeType: "depends_on",
	}
	affected := dt.GetAffectedInvariants(op)

	found := false
	for _, id := range affected {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("DeleteEdge should affect acyclicity invariant")
	}
}

func TestGetAffectedInvariantsDeleteProperty(t *testing.T) {
	dt := NewDependencyTracker()

	inv := NewUniquenessInvariant("u", UniquenessConfig{
		NodeType: "user",
		Property: "email",
	})
	dt.RegisterInvariant(inv)

	op := &crdt.Operation{
		Type: crdt.OpDeleteProperty,
		Key:  "email",
	}
	affected := dt.GetAffectedInvariants(op)

	found := false
	for _, id := range affected {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("DeleteProperty on 'email' should affect uniqueness invariant")
	}
}

func TestGlobalInvariantsAlwaysIncluded(t *testing.T) {
	dt := NewDependencyTracker()

	// Custom invariants are global
	customInv := NewCustomInvariant("global", "always check", func(ctx *ValidationContext) error {
		return nil
	})
	dt.RegisterInvariant(customInv)

	// HierarchyDepth with empty NodeType is global
	hdInv := NewHierarchyDepthInvariant("hd-all", HierarchyDepthConfig{MaxDepth: 10})
	dt.RegisterInvariant(hdInv)

	// Any operation should include globals
	op := &crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "unrelated",
	}
	affected := dt.GetAffectedInvariants(op)

	ids := make(map[string]bool)
	for _, id := range affected {
		ids[id] = true
	}
	if !ids[customInv.ID] {
		t.Error("custom (global) invariant should always be affected")
	}
	if !ids[hdInv.ID] {
		t.Error("hierarchy depth without nodeType (global) should always be affected")
	}
}

func TestEdgeConstraintValidationPassesForNonMatchingEdgeType(t *testing.T) {
	w := crdt.NewEGWalker("r1")

	inv := NewEdgeConstraintInvariant("ec", EdgeConstraintConfig{
		EdgeType:  "member_of",
		FromTypes: []string{"user"},
		ToTypes:   []string{"team"},
	})

	u1, _, _ := w.InsertNode("user", nil, map[string]interface{}{})
	u2, _, _ := w.InsertNode("user", nil, map[string]interface{}{})

	// Different edge type should be ignored
	w.InsertEdge("follows", u1, u2, nil)

	ctx := NewValidationContext(w)
	if err := inv.Validate(ctx); err != nil {
		t.Errorf("non-matching edge type should pass, got: %v", err)
	}
}

func TestCustomInvariantNilFn(t *testing.T) {
	inv := NewCustomInvariant("noop", "no function", nil)

	w := crdt.NewEGWalker("r1")
	ctx := NewValidationContext(w)

	if err := inv.Validate(ctx); err != nil {
		t.Errorf("nil customFn should pass validation, got: %v", err)
	}
}

func TestValidateSubsetWithMultipleViolations(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewUniquenessInvariant("unique-name", UniquenessConfig{
		NodeType: "user",
		Property: "name",
	}))
	v.Add(NewChildCountInvariant("min-children", ChildCountConfig{
		ParentType: "user",
		Min:        intPtr(1),
	}))

	// Create duplicate names and no children -> two violations
	w.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	w.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})

	err := v.ValidateSubset(w, "user", "")
	if err == nil {
		t.Fatal("expected violations")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if len(ve.Violations) < 2 {
		t.Errorf("expected at least 2 violations, got %d", len(ve.Violations))
	}
}

func TestAllNodesAndAllEdges(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	ctx := NewValidationContext(w)

	// Empty graph
	if len(ctx.AllNodes()) != 0 {
		t.Error("expected 0 nodes in empty graph")
	}
	if len(ctx.AllEdges()) != 0 {
		t.Error("expected 0 edges in empty graph")
	}

	n1, _, _ := w.InsertNode("a", nil, map[string]interface{}{})
	n2, _, _ := w.InsertNode("b", nil, map[string]interface{}{})
	w.InsertEdge("e", n1, n2, nil)

	if len(ctx.AllNodes()) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(ctx.AllNodes()))
	}
	if len(ctx.AllEdges()) != 1 {
		t.Errorf("expected 1 edge, got %d", len(ctx.AllEdges()))
	}
}

func TestGetParent(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	ctx := NewValidationContext(w)

	root, _, _ := w.InsertNode("a", nil, map[string]interface{}{})
	child, _, _ := w.InsertNode("b", &root, map[string]interface{}{})

	parent, ok := ctx.GetParent(child)
	if !ok {
		t.Fatal("expected to find parent")
	}
	if parent.ID != root {
		t.Errorf("expected parent ID %s, got %s", root, parent.ID)
	}

	_, ok = ctx.GetParent(root)
	if ok {
		t.Error("root node should not have a parent")
	}
}

func TestGetChildren(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	ctx := NewValidationContext(w)

	root, _, _ := w.InsertNode("a", nil, map[string]interface{}{})
	w.InsertNode("b", &root, map[string]interface{}{})
	w.InsertNode("c", &root, map[string]interface{}{})

	children := ctx.GetChildren(root)
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestNodeDepInvariantsAffected(t *testing.T) {
	dt := NewDependencyTracker()

	inv := NewUniquenessInvariant("u", UniquenessConfig{
		NodeType: "user",
		Property: "email",
	})
	dt.RegisterInvariant(inv)

	// Manually add a node-specific dependency
	nodeID := uuid.New()
	dt.mu.Lock()
	dt.nodeDepInvariants[nodeID] = map[string]struct{}{inv.ID: {}}
	dt.mu.Unlock()

	// OpInsertNode with matching TargetID should pick it up
	op := &crdt.Operation{
		Type:     crdt.OpInsertNode,
		NodeType: "other",
		TargetID: nodeID,
	}
	affected := dt.GetAffectedInvariants(op)
	found := false
	for _, id := range affected {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("node-specific dep should be affected when TargetID matches")
	}

	// OpSetProperty with matching TargetID
	op2 := &crdt.Operation{
		Type:     crdt.OpSetProperty,
		TargetID: nodeID,
		Key:      "other_key",
	}
	affected2 := dt.GetAffectedInvariants(op2)
	found = false
	for _, id := range affected2 {
		if id == inv.ID {
			found = true
		}
	}
	if !found {
		t.Error("node-specific dep should be affected for OpSetProperty with matching TargetID")
	}
}
