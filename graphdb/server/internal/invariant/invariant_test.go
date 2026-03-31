package invariant

import (
	"fmt"
	"testing"

	"github.com/kern/graphdb/internal/crdt"
)

func intPtr(n int) *int { return &n }

func TestCardinalityInvariant(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewCardinalityInvariant("max-follows", CardinalityConfig{
		NodeType:  "user",
		EdgeType:  "follows",
		Direction: "out",
		Max:       intPtr(2),
	}))

	u1, _, _ := w.InsertNode("user", nil, map[string]interface{}{})
	u2, _, _ := w.InsertNode("user", nil, map[string]interface{}{})
	u3, _, _ := w.InsertNode("user", nil, map[string]interface{}{})
	u4, _, _ := w.InsertNode("user", nil, map[string]interface{}{})

	w.InsertEdge("follows", u1, u2, nil)
	w.InsertEdge("follows", u1, u3, nil)

	// Should pass: 2 edges (max is 2)
	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation, got: %v", err)
	}

	// Add third edge
	w.InsertEdge("follows", u1, u4, nil)

	// Should fail: 3 edges (max is 2)
	if err := v.ValidateAll(w); err == nil {
		t.Error("expected cardinality violation")
	}
}

func TestUniquenessInvariant(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewUniquenessInvariant("unique-email", UniquenessConfig{
		NodeType: "user",
		Property: "email",
	}))

	w.InsertNode("user", nil, map[string]interface{}{"email": "alice@example.com"})

	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation, got: %v", err)
	}

	// Duplicate email
	w.InsertNode("user", nil, map[string]interface{}{"email": "alice@example.com"})

	if err := v.ValidateAll(w); err == nil {
		t.Error("expected uniqueness violation")
	}
}

func TestAcyclicityInvariant(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewAcyclicityInvariant("no-dep-cycles", AcyclicityConfig{
		EdgeType: "depends_on",
	}))

	a, _, _ := w.InsertNode("task", nil, map[string]interface{}{})
	b, _, _ := w.InsertNode("task", nil, map[string]interface{}{})
	c, _, _ := w.InsertNode("task", nil, map[string]interface{}{})

	w.InsertEdge("depends_on", a, b, nil)
	w.InsertEdge("depends_on", b, c, nil)

	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation, got: %v", err)
	}

	// Create cycle: c -> a
	w.InsertEdge("depends_on", c, a, nil)

	if err := v.ValidateAll(w); err == nil {
		t.Error("expected acyclicity violation")
	}
}

func TestChildCountInvariant(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewChildCountInvariant("max-teams", ChildCountConfig{
		ParentType: "org",
		ChildType:  "team",
		Max:        intPtr(2),
	}))

	org, _, _ := w.InsertNode("org", nil, map[string]interface{}{})
	w.InsertNode("team", &org, map[string]interface{}{})
	w.InsertNode("team", &org, map[string]interface{}{})

	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation, got: %v", err)
	}

	w.InsertNode("team", &org, map[string]interface{}{})

	if err := v.ValidateAll(w); err == nil {
		t.Error("expected child count violation")
	}
}

func TestCustomInvariant(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewCustomInvariant("no-empty-names", "All users must have non-empty names", func(ctx *ValidationContext) error {
		users := ctx.GetNodesByType("user")
		for _, u := range users {
			name, ok := u.Properties["name"]
			if !ok || name == "" {
				return fmt.Errorf("user %s has empty name", u.ID)
			}
		}
		return nil
	}))

	w.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation, got: %v", err)
	}

	w.InsertNode("user", nil, map[string]interface{}{"name": ""})
	if err := v.ValidateAll(w); err == nil {
		t.Error("expected custom invariant violation")
	}
}

func TestHierarchyDepthInvariant(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewHierarchyDepthInvariant("max-depth", HierarchyDepthConfig{
		MaxDepth: 2,
	}))

	root, _, _ := w.InsertNode("a", nil, map[string]interface{}{})
	child, _, _ := w.InsertNode("b", &root, map[string]interface{}{})
	grandchild, _, _ := w.InsertNode("c", &child, map[string]interface{}{})

	// Depth 2 is ok
	if err := v.ValidateAll(w); err != nil {
		t.Errorf("expected no violation, got: %v", err)
	}

	// Depth 3 exceeds limit
	w.InsertNode("d", &grandchild, map[string]interface{}{})
	if err := v.ValidateAll(w); err == nil {
		t.Error("expected hierarchy depth violation")
	}
}

func TestValidatorSubset(t *testing.T) {
	w := crdt.NewEGWalker("r1")
	v := NewValidator()

	v.Add(NewUniquenessInvariant("unique-email", UniquenessConfig{
		NodeType: "user",
		Property: "email",
	}))
	v.Add(NewAcyclicityInvariant("no-cycles", AcyclicityConfig{
		EdgeType: "depends_on",
	}))

	w.InsertNode("user", nil, map[string]interface{}{"email": "a@b.com"})
	w.InsertNode("user", nil, map[string]interface{}{"email": "a@b.com"})

	// ValidateSubset with nodeType="user" should catch uniqueness
	err := v.ValidateSubset(w, "user", "")
	if err == nil {
		t.Error("expected uniqueness violation in subset check")
	}

	// ValidateSubset with edgeType="depends_on" should not catch uniqueness
	err = v.ValidateSubset(w, "", "depends_on")
	if err != nil {
		t.Errorf("unexpected violation for unrelated subset: %v", err)
	}
}

