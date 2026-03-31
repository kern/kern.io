package graph

import (
	"testing"

	"github.com/google/uuid"
)

func TestSetProperty(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"name": "original"})

	if err := s.SetProperty(id, "name", "updated"); err != nil {
		t.Fatal(err)
	}

	node, _ := s.GetNode(id)
	if node.Properties["name"] != "updated" {
		t.Errorf("expected 'updated', got %v", node.Properties["name"])
	}

	// Set new property
	if err := s.SetProperty(id, "color", "blue"); err != nil {
		t.Fatal(err)
	}

	node, _ = s.GetNode(id)
	if node.Properties["color"] != "blue" {
		t.Error("new property should be set")
	}
}

func TestSetPropertyNotFound(t *testing.T) {
	s := NewStore("r1")
	err := s.SetProperty(uuid.New(), "key", "val")
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestPatchNode(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"a": 1})

	err := s.PatchNode(id, map[string]interface{}{"a": 2, "b": 3})
	if err != nil {
		t.Fatal(err)
	}

	node, _ := s.GetNode(id)
	if node.Properties["a"] != 2 {
		t.Error("a should be updated")
	}
	if node.Properties["b"] != 3 {
		t.Error("b should be added")
	}
}

func TestDeleteProperty(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"name": "test", "color": "red"})

	err := s.DeleteProperty(id, "color")
	if err != nil {
		t.Fatal(err)
	}

	node, _ := s.GetNode(id)
	if _, ok := node.Properties["color"]; ok {
		t.Error("color should be deleted")
	}
}

func TestDeletePropertyNotFound(t *testing.T) {
	s := NewStore("r1")
	err := s.DeleteProperty(uuid.New(), "key")
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestDeleteEdge(t *testing.T) {
	s := NewStore("r1")
	id1, _ := s.InsertNode("a", nil, nil)
	id2, _ := s.InsertNode("b", nil, nil)
	edgeID, _ := s.InsertEdge("link", id1, id2, nil)

	err := s.DeleteEdge(edgeID)
	if err != nil {
		t.Fatal(err)
	}

	// Edge should be gone
	edges := s.AllEdges()
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

func TestDeleteEdgeNotFound(t *testing.T) {
	s := NewStore("r1")
	err := s.DeleteEdge(uuid.New())
	if err == nil {
		t.Error("expected error for non-existent edge")
	}
}

func TestMoveNode(t *testing.T) {
	s := NewStore("r1")
	parent1, _ := s.InsertNode("folder", nil, nil)
	parent2, _ := s.InsertNode("folder", nil, nil)
	child, _ := s.InsertNode("file", &parent1, nil)

	err := s.MoveNode(child, &parent2)
	if err != nil {
		t.Fatal(err)
	}

	children := s.GetChildren(parent2)
	if len(children) != 1 {
		t.Error("child should be under parent2")
	}
}

func TestMoveNodeNotFound(t *testing.T) {
	s := NewStore("r1")
	err := s.MoveNode(uuid.New(), nil)
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestMoveNodeParentNotFound(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("file", nil, nil)
	badParent := uuid.New()
	err := s.MoveNode(id, &badParent)
	if err == nil {
		t.Error("expected error for non-existent parent")
	}
}

func TestGetChildren(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	s.InsertNode("file", &parent, nil)
	s.InsertNode("file", &parent, nil)

	children := s.GetChildren(parent)
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestGetParent(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	child, _ := s.InsertNode("file", &parent, nil)

	p, ok := s.GetParent(child)
	if !ok {
		t.Fatal("should have parent")
	}
	if p.ID != parent {
		t.Error("wrong parent")
	}
}

func TestGetRoots(t *testing.T) {
	s := NewStore("r1")
	s.InsertNode("a", nil, nil)
	s.InsertNode("b", nil, nil)
	parent, _ := s.InsertNode("c", nil, nil)
	s.InsertNode("d", &parent, nil)

	roots := s.GetRoots()
	if len(roots) != 3 {
		t.Errorf("expected 3 roots, got %d", len(roots))
	}
}

func TestGetNodesByType(t *testing.T) {
	s := NewStore("r1")
	s.InsertNode("user", nil, nil)
	s.InsertNode("user", nil, nil)
	s.InsertNode("post", nil, nil)

	users := s.GetNodesByType("user")
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestGetOutEdges(t *testing.T) {
	s := NewStore("r1")
	id1, _ := s.InsertNode("a", nil, nil)
	id2, _ := s.InsertNode("b", nil, nil)
	s.InsertEdge("link", id1, id2, nil)

	edges := s.GetOutEdges(id1)
	if len(edges) != 1 {
		t.Errorf("expected 1 out edge, got %d", len(edges))
	}
}

func TestGetInEdges(t *testing.T) {
	s := NewStore("r1")
	id1, _ := s.InsertNode("a", nil, nil)
	id2, _ := s.InsertNode("b", nil, nil)
	s.InsertEdge("link", id1, id2, nil)

	edges := s.GetInEdges(id2)
	if len(edges) != 1 {
		t.Errorf("expected 1 in edge, got %d", len(edges))
	}
}

func TestGetOutEdgesByType(t *testing.T) {
	s := NewStore("r1")
	id1, _ := s.InsertNode("a", nil, nil)
	id2, _ := s.InsertNode("b", nil, nil)
	s.InsertEdge("link", id1, id2, nil)
	s.InsertEdge("other", id1, id2, nil)

	edges := s.GetOutEdgesByType(id1, "link")
	if len(edges) != 1 {
		t.Errorf("expected 1 'link' edge, got %d", len(edges))
	}
}

func TestGetInEdgesByType(t *testing.T) {
	s := NewStore("r1")
	id1, _ := s.InsertNode("a", nil, nil)
	id2, _ := s.InsertNode("b", nil, nil)
	s.InsertEdge("link", id1, id2, nil)
	s.InsertEdge("other", id1, id2, nil)

	edges := s.GetInEdgesByType(id2, "link")
	if len(edges) != 1 {
		t.Errorf("expected 1 'link' edge, got %d", len(edges))
	}
}

func TestTraverseOut(t *testing.T) {
	s := NewStore("r1")
	a, _ := s.InsertNode("n", nil, nil)
	b, _ := s.InsertNode("n", nil, nil)
	c, _ := s.InsertNode("n", nil, nil)
	s.InsertEdge("link", a, b, nil)
	s.InsertEdge("link", b, c, nil)

	nodes, err := s.Traverse(a, "link", "out", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Errorf("expected 3 reachable nodes, got %d", len(nodes))
	}
}

func TestTraverseIn(t *testing.T) {
	s := NewStore("r1")
	a, _ := s.InsertNode("n", nil, nil)
	b, _ := s.InsertNode("n", nil, nil)
	s.InsertEdge("link", a, b, nil)

	nodes, _ := s.Traverse(b, "link", "in", 10)
	if len(nodes) != 2 {
		t.Errorf("expected 2 reachable nodes via in-edges, got %d", len(nodes))
	}
}

func TestTraverseBoth(t *testing.T) {
	s := NewStore("r1")
	a, _ := s.InsertNode("n", nil, nil)
	b, _ := s.InsertNode("n", nil, nil)
	c, _ := s.InsertNode("n", nil, nil)
	s.InsertEdge("link", a, b, nil)
	s.InsertEdge("link", c, b, nil)

	nodes, _ := s.Traverse(b, "link", "both", 10)
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes via both, got %d", len(nodes))
	}
}

func TestTraverseMaxDepth(t *testing.T) {
	s := NewStore("r1")
	a, _ := s.InsertNode("n", nil, nil)
	b, _ := s.InsertNode("n", nil, nil)
	c, _ := s.InsertNode("n", nil, nil)
	s.InsertEdge("link", a, b, nil)
	s.InsertEdge("link", b, c, nil)

	nodes, _ := s.Traverse(a, "link", "out", 1)
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes with maxDepth=1, got %d", len(nodes))
	}
}

func TestTraverseFilterEdgeType(t *testing.T) {
	s := NewStore("r1")
	a, _ := s.InsertNode("n", nil, nil)
	b, _ := s.InsertNode("n", nil, nil)
	c, _ := s.InsertNode("n", nil, nil)
	s.InsertEdge("link", a, b, nil)
	s.InsertEdge("other", a, c, nil)

	nodes, _ := s.Traverse(a, "link", "out", 10)
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes for 'link' type only, got %d", len(nodes))
	}
}

func TestAllEdges(t *testing.T) {
	s := NewStore("r1")
	a, _ := s.InsertNode("n", nil, nil)
	b, _ := s.InsertNode("n", nil, nil)
	s.InsertEdge("link", a, b, nil)

	edges := s.AllEdges()
	if len(edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(edges))
	}
}

func TestFindByIndex(t *testing.T) {
	s := NewStore("r1")
	s.InsertNode("user", nil, map[string]interface{}{"email": "alice@test.com"})
	s.InsertNode("user", nil, map[string]interface{}{"email": "bob@test.com"})

	results := s.FindByIndex("user", "email", "alice@test.com")
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	results = s.FindByIndex("user", "email", "nonexistent@test.com")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestDeleteNodeStore(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"name": "test"})

	err := s.DeleteNode(id)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.GetNode(id)
	if err == nil {
		t.Error("deleted node should not be found")
	}
}

func TestDeleteNodeNotFound(t *testing.T) {
	s := NewStore("r1")
	err := s.DeleteNode(uuid.New())
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestInsertNodeWithSchemaValidation(t *testing.T) {
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
	_, err := s.InsertNode("user", nil, map[string]interface{}{})
	if err == nil {
		t.Error("should fail validation")
	}

	// Valid
	_, err = s.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	if err != nil {
		t.Errorf("should pass: %v", err)
	}
}

func TestInsertEdgeWithSchemaValidation(t *testing.T) {
	s := NewStore("r1")
	schema := NewSchema()
	schema.DefineEdge(&EdgeTypeDef{
		Name:      "follows",
		FromTypes: []string{"user"},
		ToTypes:   []string{"user"},
	})
	s.SetSchema(schema)

	u1, _ := s.InsertNode("user", nil, nil)
	u2, _ := s.InsertNode("user", nil, nil)
	p1, _ := s.InsertNode("post", nil, nil)

	_, err := s.InsertEdge("follows", u1, u2, nil)
	if err != nil {
		t.Errorf("should pass: %v", err)
	}

	_, err = s.InsertEdge("follows", u1, p1, nil)
	if err == nil {
		t.Error("should fail: post not in ToTypes")
	}
}

func TestInsertEdgeSourceNotFound(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("a", nil, nil)
	_, err := s.InsertEdge("link", uuid.New(), id, nil)
	if err == nil {
		t.Error("expected error for missing source")
	}
}

func TestInsertEdgeTargetNotFound(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("a", nil, nil)
	_, err := s.InsertEdge("link", id, uuid.New(), nil)
	if err == nil {
		t.Error("expected error for missing target")
	}
}

func TestMoveNodeWithSchemaValidation(t *testing.T) {
	s := NewStore("r1")
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name:            "folder",
		AllowedChildren: []string{"file"},
	})
	s.SetSchema(schema)

	folder, _ := s.InsertNode("folder", nil, nil)
	file, _ := s.InsertNode("file", nil, nil)
	otherFolder, _ := s.InsertNode("folder", nil, nil)

	// Valid move
	err := s.MoveNode(file, &folder)
	if err != nil {
		t.Errorf("should pass: %v", err)
	}

	// folder -> folder not allowed
	err = s.MoveNode(otherFolder, &folder)
	if err == nil {
		t.Error("should fail: folder not in AllowedChildren")
	}
}

func TestSetPropertyWithSchemaValidation(t *testing.T) {
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

	err := s.SetProperty(id, "count", "not-a-number")
	if err == nil {
		t.Error("should fail: string for number property")
	}

	err = s.SetProperty(id, "count", 42)
	if err != nil {
		t.Errorf("should pass: %v", err)
	}
}

func TestRestoreNodeNotDeleted(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, nil)
	err := s.RestoreNode(id)
	if err == nil {
		t.Error("should fail: node is not deleted")
	}
}

func TestRestoreNodeNotFound(t *testing.T) {
	s := NewStore("r1")
	err := s.RestoreNode(uuid.New())
	if err == nil {
		t.Error("should fail: node not found")
	}
}

func TestBatchDeleteNode(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, nil)

	ops := []BatchOp{
		{Type: BatchDeleteNode, NodeID: id},
	}
	_, err := s.ExecuteBatch(ops)
	if err != nil {
		t.Fatal(err)
	}

	all := s.AllNodes()
	if len(all) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(all))
	}
}

func TestBatchSetProperty(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, nil)

	ops := []BatchOp{
		{Type: BatchSetProperty, NodeID: id, Key: "name", Value: "test"},
	}
	_, err := s.ExecuteBatch(ops)
	if err != nil {
		t.Fatal(err)
	}

	node, _ := s.GetNode(id)
	if node.Properties["name"] != "test" {
		t.Error("property should be set")
	}
}

func TestBatchDeleteProperty(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"color": "red"})

	ops := []BatchOp{
		{Type: BatchDeleteProperty, NodeID: id, Key: "color"},
	}
	_, err := s.ExecuteBatch(ops)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBatchInsertEdge(t *testing.T) {
	s := NewStore("r1")
	id1, _ := s.InsertNode("a", nil, nil)
	id2, _ := s.InsertNode("b", nil, nil)

	ops := []BatchOp{
		{Type: BatchInsertEdge, EdgeType: "link", FromID: id1, ToID: id2},
	}
	result, err := s.ExecuteBatch(ops)
	if err != nil {
		t.Fatal(err)
	}
	if result.Results[0].ResultID == uuid.Nil {
		t.Error("should have edge result ID")
	}
}

func TestBatchDeleteEdge(t *testing.T) {
	s := NewStore("r1")
	id1, _ := s.InsertNode("a", nil, nil)
	id2, _ := s.InsertNode("b", nil, nil)
	edgeID, _ := s.InsertEdge("link", id1, id2, nil)

	ops := []BatchOp{
		{Type: BatchDeleteEdge, EdgeID: edgeID},
	}
	_, err := s.ExecuteBatch(ops)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBatchMoveNode(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	child, _ := s.InsertNode("file", nil, nil)

	ops := []BatchOp{
		{Type: BatchMoveNode, NodeID: child, ParentID: &parent},
	}
	_, err := s.ExecuteBatch(ops)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBatchReorderNode(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	child, _ := s.InsertNode("file", &parent, nil)

	ops := []BatchOp{
		{Type: BatchReorderNode, NodeID: child, Position: "M"},
	}
	_, err := s.ExecuteBatch(ops)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBatchRestoreNode(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, nil)
	s.SoftDeleteNode(id)

	ops := []BatchOp{
		{Type: BatchRestoreNode, NodeID: id},
	}
	_, err := s.ExecuteBatch(ops)
	if err != nil {
		t.Fatal(err)
	}

	node, getErr := s.GetNode(id)
	if getErr != nil {
		t.Fatal("should be restored")
	}
	_ = node
}

func TestBatchCascadeDelete(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	child, _ := s.InsertNode("file", &parent, nil)
	s.InsertEdge("ref", parent, child, nil)

	ops := []BatchOp{
		{Type: BatchCascadeDelete, NodeID: parent},
	}
	_, err := s.ExecuteBatch(ops)
	if err != nil {
		t.Fatal(err)
	}

	all := s.AllNodes()
	if len(all) != 0 {
		t.Errorf("expected 0 nodes after cascade, got %d", len(all))
	}
}

func TestBatchDeleteNodeNotFound(t *testing.T) {
	s := NewStore("r1")
	ops := []BatchOp{
		{Type: BatchDeleteNode, NodeID: uuid.New()},
	}
	_, err := s.ExecuteBatch(ops)
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestReorderNodeNotFound(t *testing.T) {
	s := NewStore("r1")
	err := s.ReorderNode(uuid.New(), "M")
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestReorderBetweenNotFound(t *testing.T) {
	s := NewStore("r1")
	err := s.ReorderBetween(uuid.New(), nil, nil)
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestSoftDeleteNodeNotFound(t *testing.T) {
	s := NewStore("r1")
	err := s.SoftDeleteNode(uuid.New())
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestGetSubtreeNotFound(t *testing.T) {
	s := NewStore("r1")
	_, err := s.GetSubtree(uuid.New())
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestGetSubtreeWithChildren(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	s.InsertNode("file", &parent, nil)
	s.InsertNode("file", &parent, nil)

	subtree, err := s.GetSubtree(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(subtree) != 3 {
		t.Errorf("expected 3 nodes in subtree, got %d", len(subtree))
	}
}

func TestGetAncestors(t *testing.T) {
	s := NewStore("r1")
	root, _ := s.InsertNode("folder", nil, nil)
	mid, _ := s.InsertNode("folder", &root, nil)
	leaf, _ := s.InsertNode("file", &mid, nil)

	ancestors := s.GetAncestors(leaf)
	if len(ancestors) != 2 {
		t.Errorf("expected 2 ancestors, got %d", len(ancestors))
	}
}

func TestApplyRemote(t *testing.T) {
	s := NewStore("r1")
	s2 := NewStore("r2")

	id, _ := s.InsertNode("item", nil, map[string]interface{}{"name": "test"})

	// Get ops from s's walker
	w := s.Walker()
	eg := w.Graph()
	ops := eg.EventsSince(nil)

	for _, op := range ops {
		if err := s2.ApplyRemote(op); err != nil {
			t.Fatalf("ApplyRemote failed: %v", err)
		}
	}

	node, err := s2.GetNode(id)
	if err != nil {
		t.Fatal("node should exist on s2")
	}
	if node.Properties["name"] != "test" {
		t.Error("properties should match")
	}
}

func TestGetDeletedNodesStore(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, nil)
	s.SoftDeleteNode(id)

	deleted := s.GetDeletedNodes()
	if len(deleted) != 1 {
		t.Errorf("expected 1 deleted, got %d", len(deleted))
	}
}
