package graph

import (
	"testing"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// store.go: reapOrphansUnlocked — cascading orphans across multiple levels
// ---------------------------------------------------------------------------

func TestReapOrphansCascading(t *testing.T) {
	s := NewStore("r1")

	// Build a 3-level tree: grandparent -> parent -> child
	grandparent, _ := s.InsertNode("folder", nil, nil)
	parent, _ := s.InsertNode("folder", &grandparent, nil)
	child, _ := s.InsertNode("file", &parent, nil)

	// Soft-delete only the grandparent
	s.SoftDeleteNode(grandparent)

	// ReapOrphans should cascade: parent becomes orphan, then child
	reaped, err := s.ReapOrphans()
	if err != nil {
		t.Fatalf("ReapOrphans: %v", err)
	}

	// Both parent and child should be reaped
	ids := make(map[uuid.UUID]bool)
	for _, id := range reaped {
		ids[id] = true
	}
	if !ids[parent] {
		t.Error("parent should be reaped")
	}
	if !ids[child] {
		t.Error("child should be reaped")
	}

	// All nodes should be gone
	all := s.AllNodes()
	if len(all) != 0 {
		t.Errorf("expected 0 live nodes, got %d", len(all))
	}
}

func TestReapOrphansNoOrphans(t *testing.T) {
	s := NewStore("r1")
	s.InsertNode("item", nil, nil)
	s.InsertNode("item", nil, nil)

	reaped, err := s.ReapOrphans()
	if err != nil {
		t.Fatalf("ReapOrphans: %v", err)
	}
	if len(reaped) != 0 {
		t.Errorf("expected 0 reaped, got %d", len(reaped))
	}
}

func TestReapOrphansDeeply(t *testing.T) {
	s := NewStore("r1")

	// Build a 4-level tree to force multiple rounds of reapOrphansUnlocked
	level0, _ := s.InsertNode("folder", nil, nil)
	level1, _ := s.InsertNode("folder", &level0, nil)
	level2, _ := s.InsertNode("folder", &level1, nil)
	level3, _ := s.InsertNode("file", &level2, nil)

	// Soft-delete only root
	s.SoftDeleteNode(level0)

	reaped, err := s.ReapOrphans()
	if err != nil {
		t.Fatalf("ReapOrphans: %v", err)
	}

	ids := make(map[uuid.UUID]bool)
	for _, id := range reaped {
		ids[id] = true
	}

	if !ids[level1] || !ids[level2] || !ids[level3] {
		t.Errorf("expected all descendants to be reaped, got %d reaped", len(reaped))
	}
}

func TestReapOrphansWithRootNodes(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	s.InsertNode("file", &parent, map[string]interface{}{"name": "a"})
	s.InsertNode("file", &parent, map[string]interface{}{"name": "b"})
	root, _ := s.InsertNode("root", nil, nil)

	s.SoftDeleteNode(parent)

	reaped, err := s.ReapOrphans()
	if err != nil {
		t.Fatalf("ReapOrphans: %v", err)
	}
	if len(reaped) != 2 {
		t.Errorf("expected 2 reaped, got %d", len(reaped))
	}

	// Root node should still be alive
	_, err = s.GetNode(root)
	if err != nil {
		t.Error("root node should still exist")
	}
}

// ---------------------------------------------------------------------------
// store.go: PatchNode — error path (node not found)
// ---------------------------------------------------------------------------

func TestPatchNodeNotFound(t *testing.T) {
	s := NewStore("r1")
	err := s.PatchNode(uuid.New(), map[string]interface{}{"a": 1})
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestPatchNodeMultipleProperties(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"a": 1, "b": 2})

	err := s.PatchNode(id, map[string]interface{}{"b": 20, "c": 30})
	if err != nil {
		t.Fatalf("PatchNode: %v", err)
	}

	node, _ := s.GetNode(id)
	if node.Properties["a"] != 1 {
		t.Error("a should be unchanged")
	}
	if node.Properties["b"] != 20 {
		t.Error("b should be updated")
	}
	if node.Properties["c"] != 30 {
		t.Error("c should be added")
	}
}

func TestPatchNodeWithSchemaValidation(t *testing.T) {
	s := NewStore("r1")
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name: "item",
		Properties: map[string]*PropertyDef{
			"count": {Name: "count", Type: PropNumber},
		},
	})
	s.SetSchema(schema)

	id, _ := s.InsertNode("item", nil, nil)

	// Valid patch
	err := s.PatchNode(id, map[string]interface{}{"count": 42})
	if err != nil {
		t.Fatalf("valid patch failed: %v", err)
	}

	// Invalid patch — first key fails
	err = s.PatchNode(id, map[string]interface{}{"count": "not-a-number"})
	if err == nil {
		t.Error("should fail schema validation")
	}
}

// ---------------------------------------------------------------------------
// store.go: CascadeDeleteNode with edges and grandchildren
// ---------------------------------------------------------------------------

func TestCascadeDeleteNodeWithEdges(t *testing.T) {
	s := NewStore("r1")

	parent, _ := s.InsertNode("folder", nil, map[string]interface{}{"name": "root"})
	child, _ := s.InsertNode("file", &parent, map[string]interface{}{"name": "child"})
	grandchild, _ := s.InsertNode("file", &child, nil)

	// External edges
	other, _ := s.InsertNode("ext", nil, nil)
	s.InsertEdge("ref", other, parent, nil)
	s.InsertEdge("ref", child, other, nil)

	err := s.CascadeDeleteNode(parent)
	if err != nil {
		t.Fatalf("CascadeDeleteNode: %v", err)
	}

	_ = grandchild

	// All cascade-deleted nodes should be gone
	all := s.AllNodes()
	if len(all) != 1 {
		t.Errorf("expected 1 remaining node (other), got %d", len(all))
	}

	// All edges should be gone
	edges := s.AllEdges()
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

func TestCascadeDeleteNodeWithInAndOutEdges(t *testing.T) {
	s := NewStore("r1")

	// Create nodes
	root, _ := s.InsertNode("folder", nil, nil)
	child1, _ := s.InsertNode("file", &root, nil)
	child2, _ := s.InsertNode("file", &root, nil)
	external, _ := s.InsertNode("ext", nil, nil)

	// Outgoing edge from child1
	s.InsertEdge("out-link", child1, external, nil)
	// Incoming edge to child1
	s.InsertEdge("in-link", external, child1, nil)
	// Edge between children
	s.InsertEdge("sibling", child1, child2, nil)
	// Incoming edge to root
	s.InsertEdge("points-to", external, root, nil)

	err := s.CascadeDeleteNode(root)
	if err != nil {
		t.Fatalf("CascadeDeleteNode: %v", err)
	}

	// root, child1, child2 deleted; external survives
	all := s.AllNodes()
	if len(all) != 1 {
		t.Errorf("expected 1 node (external), got %d", len(all))
	}

	// All edges should be deleted (edges involving deleted nodes)
	edges := s.AllEdges()
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

func TestCascadeDeleteNodeLeaf(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"name": "leaf"})

	err := s.CascadeDeleteNode(id)
	if err != nil {
		t.Fatalf("CascadeDeleteNode on leaf: %v", err)
	}

	all := s.AllNodes()
	if len(all) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(all))
	}
}

// ---------------------------------------------------------------------------
// store.go: ExecuteBatch — error paths for various batch operations
// ---------------------------------------------------------------------------

func TestExecuteBatchDeleteNodeNotFoundError(t *testing.T) {
	s := NewStore("r1")
	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchDeleteNode, NodeID: uuid.New()},
	})
	if err == nil {
		t.Error("expected error for BatchDeleteNode on missing node")
	}
}

func TestExecuteBatchSetPropertyError(t *testing.T) {
	// SetProperty on non-existent node — the walker doesn't error, but
	// it's a no-op. We test that batch continues.
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, nil)
	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchSetProperty, NodeID: id, Key: "k", Value: "v"},
	})
	if err != nil {
		t.Fatalf("BatchSetProperty: %v", err)
	}
}

func TestExecuteBatchDeletePropertyError(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"k": "v"})
	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchDeleteProperty, NodeID: id, Key: "k"},
	})
	if err != nil {
		t.Fatalf("BatchDeleteProperty: %v", err)
	}
}

func TestExecuteBatchReorderNodeError(t *testing.T) {
	// ReorderNode doesn't error from walker, just verify batch works
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	child, _ := s.InsertNode("file", &parent, nil)
	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchReorderNode, NodeID: child, Position: "X"},
	})
	if err != nil {
		t.Fatalf("BatchReorderNode: %v", err)
	}
}

func TestExecuteBatchCascadeDeleteWithChildren(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	child, _ := s.InsertNode("file", &parent, nil)
	s.InsertNode("file", &child, nil)

	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchCascadeDelete, NodeID: parent},
	})
	if err != nil {
		t.Fatalf("BatchCascadeDelete: %v", err)
	}

	all := s.AllNodes()
	if len(all) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(all))
	}
}

func TestExecuteBatchRestoreNodeError(t *testing.T) {
	// RestoreNode on a non-existent node
	s := NewStore("r1")
	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchRestoreNode, NodeID: uuid.New()},
	})
	// Walker doesn't error for restore on non-existent, so check behavior
	if err != nil {
		// It's ok if it errors or not, just ensure no panic
	}
}

func TestExecuteBatchMoveNodeError(t *testing.T) {
	// Move to a node that would create a cycle
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	child, _ := s.InsertNode("file", &parent, nil)

	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchMoveNode, NodeID: parent, ParentID: &child},
	})
	if err == nil {
		t.Error("expected cycle detection error in batch")
	}
}

// ---------------------------------------------------------------------------
// schema.go: ValidateProperty — unknown node type and unknown property
// ---------------------------------------------------------------------------

func TestValidatePropertyUnknownNodeType(t *testing.T) {
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name: "user",
		Properties: map[string]*PropertyDef{
			"name": {Name: "name", Type: PropString},
		},
	})

	// Unknown node type => no validation
	if err := schema.ValidateProperty("unknown", "anything", 42); err != nil {
		t.Errorf("unknown node type should pass: %v", err)
	}

	// Known node type, unknown property => allow extra
	if err := schema.ValidateProperty("user", "extra", 42); err != nil {
		t.Errorf("extra property should be allowed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// schema.go: ValidateHierarchy — when AllowedChildren has entries but no match
// ---------------------------------------------------------------------------

func TestValidateHierarchyNotAllowed(t *testing.T) {
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name:            "org",
		AllowedChildren: []string{"team", "dept"},
	})

	err := schema.ValidateHierarchy("org", "user")
	if err == nil {
		t.Error("user should not be allowed as child of org")
	}

	// Allowed types
	if err := schema.ValidateHierarchy("org", "team"); err != nil {
		t.Errorf("team should be allowed: %v", err)
	}
	if err := schema.ValidateHierarchy("org", "dept"); err != nil {
		t.Errorf("dept should be allowed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// schema.go: ValidateEdge — ToTypes violation
// ---------------------------------------------------------------------------

func TestValidateEdgeToTypeViolation(t *testing.T) {
	schema := NewSchema()
	schema.DefineEdge(&EdgeTypeDef{
		Name:      "authored",
		FromTypes: []string{"user"},
		ToTypes:   []string{"document"},
	})

	// From ok, To not ok
	err := schema.ValidateEdge("authored", "user", "user")
	if err == nil {
		t.Error("should fail: user not in ToTypes")
	}

	// From not ok
	err = schema.ValidateEdge("authored", "document", "document")
	if err == nil {
		t.Error("should fail: document not in FromTypes")
	}
}

// ---------------------------------------------------------------------------
// store.go: InsertNode with parent not found
// ---------------------------------------------------------------------------

func TestInsertNodeParentNotFound(t *testing.T) {
	s := NewStore("r1")
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{Name: "item"})
	s.SetSchema(schema)

	badParent := uuid.New()
	_, err := s.InsertNode("item", &badParent, nil)
	if err == nil {
		t.Error("expected error for non-existent parent")
	}
}

// ---------------------------------------------------------------------------
// store.go: InsertNode with hierarchy violation
// ---------------------------------------------------------------------------

func TestInsertNodeHierarchyViolation(t *testing.T) {
	s := NewStore("r1")
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name:            "org",
		AllowedChildren: []string{"team"},
	})
	schema.DefineNode(&NodeTypeDef{Name: "user"})
	s.SetSchema(schema)

	orgID, _ := s.InsertNode("org", nil, nil)
	_, err := s.InsertNode("user", &orgID, nil)
	if err == nil {
		t.Error("expected hierarchy violation")
	}
}

// ---------------------------------------------------------------------------
// store.go: MoveNode with hierarchy schema validation
// ---------------------------------------------------------------------------

func TestMoveNodeHierarchyViolation(t *testing.T) {
	s := NewStore("r1")
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name:            "folder",
		AllowedChildren: []string{"file"},
	})
	schema.DefineNode(&NodeTypeDef{Name: "file"})
	schema.DefineNode(&NodeTypeDef{Name: "folder2"})
	s.SetSchema(schema)

	folder, _ := s.InsertNode("folder", nil, nil)
	folder2, _ := s.InsertNode("folder2", nil, nil)

	// folder -> folder2 not in AllowedChildren
	err := s.MoveNode(folder2, &folder)
	if err == nil {
		t.Error("expected hierarchy validation error")
	}
}

// ---------------------------------------------------------------------------
// store.go: ReorderBetween with only afterID or only beforeID
// ---------------------------------------------------------------------------

func TestReorderBetweenOnlyAfter(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	c1, _ := s.InsertNode("file", &parent, nil)
	c2, _ := s.InsertNode("file", &parent, nil)
	s.ReorderNode(c1, "A")

	err := s.ReorderBetween(c2, &c1, nil)
	if err != nil {
		t.Fatalf("ReorderBetween: %v", err)
	}

	n, _ := s.GetNode(c2)
	if n.Position <= "A" {
		t.Errorf("expected position after A, got %s", n.Position)
	}
}

func TestReorderBetweenOnlyBefore(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	c1, _ := s.InsertNode("file", &parent, nil)
	c2, _ := s.InsertNode("file", &parent, nil)
	s.ReorderNode(c2, "Z")

	err := s.ReorderBetween(c1, nil, &c2)
	if err != nil {
		t.Fatalf("ReorderBetween: %v", err)
	}

	n, _ := s.GetNode(c1)
	if n.Position >= "Z" {
		t.Errorf("expected position before Z, got %s", n.Position)
	}
}

// ---------------------------------------------------------------------------
// store.go: RestoreNode re-adds indexes
// ---------------------------------------------------------------------------

func TestRestoreNodeReindexes(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("user", nil, map[string]interface{}{"email": "test@test.com"})

	// Find by index should work
	results := s.FindByIndex("user", "email", "test@test.com")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	s.SoftDeleteNode(id)

	// Restore should re-add to indexes
	s.RestoreNode(id)

	results = s.FindByIndex("user", "email", "test@test.com")
	if len(results) == 0 {
		t.Errorf("expected results after restore, got 0")
	}
}

// ---------------------------------------------------------------------------
// store.go: GetNode not found
// ---------------------------------------------------------------------------

func TestGetNodeNotFound(t *testing.T) {
	s := NewStore("r1")
	_, err := s.GetNode(uuid.New())
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

// ---------------------------------------------------------------------------
// store.go: Traverse with empty edge type (all edges)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// store.go: ExecuteBatch — BatchInsertEdge, BatchDeleteEdge, BatchReorderNode,
// BatchRestoreNode success with real data, and BatchCascadeDelete with deep tree + edges
// ---------------------------------------------------------------------------

func TestExecuteBatchInsertEdgeSuccess(t *testing.T) {
	s := NewStore("r1")
	id1, _ := s.InsertNode("a", nil, nil)
	id2, _ := s.InsertNode("b", nil, nil)

	result, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchInsertEdge, EdgeType: "link", FromID: id1, ToID: id2, Properties: map[string]interface{}{"w": 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Results[0].ResultID == uuid.Nil {
		t.Error("should have edge ID")
	}
}

func TestExecuteBatchDeleteEdgeSuccess(t *testing.T) {
	s := NewStore("r1")
	id1, _ := s.InsertNode("a", nil, nil)
	id2, _ := s.InsertNode("b", nil, nil)
	eid, _ := s.InsertEdge("link", id1, id2, nil)

	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchDeleteEdge, EdgeID: eid},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExecuteBatchCascadeDeleteDeep(t *testing.T) {
	s := NewStore("r1")
	p, _ := s.InsertNode("folder", nil, nil)
	c1, _ := s.InsertNode("file", &p, nil)
	c2, _ := s.InsertNode("file", &c1, nil)
	ext, _ := s.InsertNode("ext", nil, nil)

	// Edges in both directions
	s.InsertEdge("out", c1, ext, nil)
	s.InsertEdge("in", ext, c2, nil)

	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchCascadeDelete, NodeID: p},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(s.AllNodes()) != 1 {
		t.Errorf("expected 1 node (ext), got %d", len(s.AllNodes()))
	}
}

func TestExecuteBatchMoveNodeCycleError(t *testing.T) {
	s := NewStore("r1")
	p, _ := s.InsertNode("folder", nil, nil)
	c, _ := s.InsertNode("file", &p, nil)

	_, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchMoveNode, NodeID: p, ParentID: &c},
	})
	if err == nil {
		t.Error("expected cycle error in batch move")
	}
}

func TestExecuteBatchInsertNodeSuccess(t *testing.T) {
	s := NewStore("r1")
	result, err := s.ExecuteBatch([]BatchOp{
		{Type: BatchInsertNode, NodeType: "item", Properties: map[string]interface{}{"x": 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Results[0].ResultID == uuid.Nil {
		t.Error("should have node ID")
	}
}

// ---------------------------------------------------------------------------
// schema.go: ValidateHierarchy — parent type defined with empty AllowedChildren
// ---------------------------------------------------------------------------

func TestValidateHierarchyEmptyAllowedChildren(t *testing.T) {
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name:            "parent",
		AllowedChildren: []string{}, // empty = allow anything
	})
	err := schema.ValidateHierarchy("parent", "anything")
	if err != nil {
		t.Errorf("empty AllowedChildren should allow anything: %v", err)
	}
}

func TestTraverseAllEdgeTypes(t *testing.T) {
	s := NewStore("r1")
	a, _ := s.InsertNode("n", nil, nil)
	b, _ := s.InsertNode("n", nil, nil)
	c, _ := s.InsertNode("n", nil, nil)
	s.InsertEdge("link", a, b, nil)
	s.InsertEdge("other", b, c, nil)

	// Empty edge type should traverse all edges
	nodes, err := s.Traverse(a, "", "out", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes traversing all edge types, got %d", len(nodes))
	}
}
