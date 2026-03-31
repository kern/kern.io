package graph

import (
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// store.go: reapOrphansUnlocked — orphaned nodes whose parent is missing
// (not just deleted). Also tests the "no more orphans" termination branch.
// ---------------------------------------------------------------------------

func TestReapOrphansUnlockedParentMissing(t *testing.T) {
	// Fabricate a scenario where a node's ParentID refers to a node
	// that is completely missing from the graph (never inserted).
	// We do this by inserting a child under a parent, then manipulating
	// the walker so the parent is gone but the child still references it.
	s := NewStore("r1")

	// Build: root -> mid -> leaf
	root, _ := s.InsertNode("folder", nil, nil)
	mid, _ := s.InsertNode("folder", &root, nil)
	_, _ = s.InsertNode("file", &mid, nil)

	// Soft-delete root (makes root.Deleted = true)
	s.SoftDeleteNode(root)

	// Reap should find mid as orphan (parent root is deleted),
	// then on the recursive call find leaf (parent mid is now deleted).
	reaped, err := s.ReapOrphans()
	if err != nil {
		t.Fatalf("ReapOrphans: %v", err)
	}
	if len(reaped) < 2 {
		t.Errorf("expected at least 2 reaped, got %d", len(reaped))
	}
	if len(s.AllNodes()) != 0 {
		t.Errorf("expected 0 live nodes, got %d", len(s.AllNodes()))
	}
}

func TestReapOrphansUnlockedTerminatesWhenNoOrphans(t *testing.T) {
	s := NewStore("r1")

	// Build: root -> child
	root, _ := s.InsertNode("folder", nil, nil)
	s.InsertNode("file", &root, nil)

	// Soft-delete root, reap. After first round all orphans are reaped;
	// the recursive call should find no more and terminate.
	s.SoftDeleteNode(root)

	reaped, err := s.ReapOrphans()
	if err != nil {
		t.Fatalf("ReapOrphans: %v", err)
	}
	if len(reaped) != 1 {
		t.Errorf("expected 1 reaped, got %d", len(reaped))
	}
}

// ---------------------------------------------------------------------------
// store.go: ExecuteBatch — cover all remaining batch operation types
// ---------------------------------------------------------------------------

func TestExecuteBatchAllOpTypes(t *testing.T) {
	s := NewStore("r1")

	// 1. BatchInsertNode
	parent, _ := s.InsertNode("folder", nil, map[string]interface{}{"name": "root"})
	child, _ := s.InsertNode("file", &parent, map[string]interface{}{"x": 1})
	other, _ := s.InsertNode("ext", nil, nil)

	// Create an edge to later delete
	edgeID, _ := s.InsertEdge("link", child, other, nil)

	// Big batch covering all op types
	result, err := s.ExecuteBatch([]BatchOp{
		// BatchInsertEdge
		{Type: BatchInsertEdge, EdgeType: "ref", FromID: parent, ToID: other, Properties: map[string]interface{}{"w": 1}},
		// BatchSetProperty
		{Type: BatchSetProperty, NodeID: child, Key: "y", Value: 2},
		// BatchDeleteProperty
		{Type: BatchDeleteProperty, NodeID: child, Key: "x"},
		// BatchMoveNode — move child to be root
		{Type: BatchMoveNode, NodeID: child, ParentID: nil},
		// BatchReorderNode
		{Type: BatchReorderNode, NodeID: child, Position: "M"},
		// BatchDeleteEdge
		{Type: BatchDeleteEdge, EdgeID: edgeID},
	})
	if err != nil {
		t.Fatalf("ExecuteBatch: %v", err)
	}

	// Verify edge was created
	if result.Results[0].ResultID == uuid.Nil {
		t.Error("BatchInsertEdge should produce a ResultID")
	}

	// Verify property set
	n, _ := s.GetNode(child)
	if n.Properties["y"] != 2 {
		t.Errorf("expected y=2, got %v", n.Properties["y"])
	}
	// Verify property deleted
	if _, ok := n.Properties["x"]; ok {
		t.Error("property x should be deleted")
	}
	// Verify position
	if n.Position != "M" {
		t.Errorf("expected position M, got %s", n.Position)
	}
}

func TestExecuteBatchSoftDeleteAndRestore(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"name": "test"})

	// Soft-delete via direct call (BatchDeleteNode)
	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchDeleteNode, NodeID: id},
	})
	if err != nil {
		t.Fatalf("BatchDeleteNode: %v", err)
	}

	// Node should be deleted
	_, err = s.GetNode(id)
	if err == nil {
		t.Error("node should be deleted")
	}

	// Restore via batch
	_, err = s.ExecuteBatch([]BatchOp{
		{Type: BatchRestoreNode, NodeID: id},
	})
	if err != nil {
		t.Fatalf("BatchRestoreNode: %v", err)
	}

	// Node should be back
	_, err = s.GetNode(id)
	if err != nil {
		t.Error("node should be restored")
	}
}

func TestExecuteBatchCascadeDeleteWithEdges(t *testing.T) {
	s := NewStore("r1")

	root, _ := s.InsertNode("folder", nil, nil)
	child, _ := s.InsertNode("file", &root, nil)
	ext, _ := s.InsertNode("ext", nil, nil)

	// Edges in both directions on the cascade-deleted nodes
	s.InsertEdge("out", root, ext, nil)
	s.InsertEdge("in", ext, child, nil)
	s.InsertEdge("sibling", child, root, nil)

	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchCascadeDelete, NodeID: root},
	})
	if err != nil {
		t.Fatalf("BatchCascadeDelete: %v", err)
	}

	// Only ext survives
	if len(s.AllNodes()) != 1 {
		t.Errorf("expected 1 node (ext), got %d", len(s.AllNodes()))
	}
	// All edges should be gone
	if len(s.AllEdges()) != 0 {
		t.Errorf("expected 0 edges, got %d", len(s.AllEdges()))
	}
}

func TestExecuteBatchPatchNode(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"a": 1, "b": 2})

	// Patch via batch: set multiple properties
	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchSetProperty, NodeID: id, Key: "a", Value: 10},
		{Type: BatchSetProperty, NodeID: id, Key: "c", Value: 30},
	})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}

	n, _ := s.GetNode(id)
	if n.Properties["a"] != 10 {
		t.Error("a should be 10")
	}
	if n.Properties["c"] != 30 {
		t.Error("c should be 30")
	}
}

// ---------------------------------------------------------------------------
// store.go: CascadeDeleteNode — with in-edges and out-edges specifically
// ---------------------------------------------------------------------------

func TestCascadeDeleteNodeInAndOutEdgesComprehensive(t *testing.T) {
	s := NewStore("r1")

	// Create tree with edges
	root, _ := s.InsertNode("folder", nil, nil)
	child, _ := s.InsertNode("file", &root, nil)
	grandchild, _ := s.InsertNode("file", &child, nil)
	ext1, _ := s.InsertNode("ext", nil, nil)
	ext2, _ := s.InsertNode("ext", nil, nil)

	// Outgoing from grandchild
	s.InsertEdge("out-ref", grandchild, ext1, nil)
	// Incoming to child
	s.InsertEdge("in-ref", ext2, child, nil)
	// Cross-edge between grandchild and ext2
	s.InsertEdge("cross", ext2, grandchild, nil)

	err := s.CascadeDeleteNode(root)
	if err != nil {
		t.Fatalf("CascadeDeleteNode: %v", err)
	}

	// root, child, grandchild deleted; ext1, ext2 survive
	all := s.AllNodes()
	if len(all) != 2 {
		t.Errorf("expected 2 remaining nodes, got %d", len(all))
	}

	// All edges should be deleted
	edges := s.AllEdges()
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

// ---------------------------------------------------------------------------
// store.go: SoftDeleteNode — node not found error
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// store.go: MoveNode — new parent not found
// ---------------------------------------------------------------------------

func TestMoveNodeNewParentNotFound(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, nil)
	missing := uuid.New()
	err := s.MoveNode(id, &missing)
	if err == nil {
		t.Error("expected error for non-existent new parent")
	}
}


// ---------------------------------------------------------------------------
// store.go: DeleteNode — has children error
// ---------------------------------------------------------------------------

func TestDeleteNodeWithChildren(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	s.InsertNode("file", &parent, nil)

	err := s.DeleteNode(parent)
	if err == nil {
		t.Error("expected error: node has children")
	}
}

// ---------------------------------------------------------------------------
// store.go: InsertEdge with schema validation error
// ---------------------------------------------------------------------------

func TestInsertEdgeSchemaViolation(t *testing.T) {
	s := NewStore("r1")
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{Name: "user"})
	schema.DefineNode(&NodeTypeDef{Name: "product"})
	schema.DefineEdge(&EdgeTypeDef{
		Name:      "owns",
		FromTypes: []string{"user"},
		ToTypes:   []string{"product"},
	})
	s.SetSchema(schema)

	u, _ := s.InsertNode("user", nil, nil)
	u2, _ := s.InsertNode("user", nil, nil)

	// user -> user not allowed by schema (ToTypes only allows product)
	_, err := s.InsertEdge("owns", u, u2, nil)
	if err == nil {
		t.Error("expected schema validation error for edge")
	}
}

// ---------------------------------------------------------------------------
// store.go: InsertNode schema validation error
// ---------------------------------------------------------------------------

func TestInsertNodeSchemaValidationError(t *testing.T) {
	s := NewStore("r1")
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name: "user",
		Properties: map[string]*PropertyDef{
			"name": {Name: "name", Type: PropString, Required: true},
		},
	})
	s.SetSchema(schema)

	// Missing required property
	_, err := s.InsertNode("user", nil, nil)
	if err == nil {
		t.Error("expected schema validation error")
	}
}

// ---------------------------------------------------------------------------
// store.go: SetProperty with schema validation error
// ---------------------------------------------------------------------------

func TestSetPropertySchemaValidationError(t *testing.T) {
	s := NewStore("r1")
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name: "user",
		Properties: map[string]*PropertyDef{
			"age": {Name: "age", Type: PropNumber},
		},
	})
	s.SetSchema(schema)

	id, _ := s.InsertNode("user", nil, nil)
	err := s.SetProperty(id, "age", "not-a-number")
	if err == nil {
		t.Error("expected schema validation error")
	}
}

// ---------------------------------------------------------------------------
// store.go: GetSchema / SetSchema
// ---------------------------------------------------------------------------

func TestGetSetSchema(t *testing.T) {
	s := NewStore("r1")
	schema := NewSchema()
	s.SetSchema(schema)
	got := s.GetSchema()
	if got != schema {
		t.Error("GetSchema should return the set schema")
	}
}
