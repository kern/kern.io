package graph

import (
	"testing"
)

func TestReorderAndGetOrderedChildren(t *testing.T) {
	s := NewStore("r1")
	parentID, _ := s.InsertNode("folder", nil, nil)

	c1, _ := s.InsertNode("file", &parentID, nil)
	c2, _ := s.InsertNode("file", &parentID, nil)
	c3, _ := s.InsertNode("file", &parentID, nil)

	s.ReorderNode(c1, "M")
	s.ReorderNode(c2, "Z")
	s.ReorderNode(c3, "A")

	ordered := s.GetOrderedChildren(parentID)
	if len(ordered) != 3 {
		t.Fatalf("expected 3 ordered children, got %d", len(ordered))
	}
	if ordered[0].ID != c3 {
		t.Error("first should be c3 (A)")
	}
	if ordered[2].ID != c2 {
		t.Error("last should be c2 (Z)")
	}
}

func TestReorderBetween(t *testing.T) {
	s := NewStore("r1")
	parentID, _ := s.InsertNode("folder", nil, nil)

	c1, _ := s.InsertNode("file", &parentID, nil)
	c2, _ := s.InsertNode("file", &parentID, nil)

	s.ReorderNode(c1, "A")
	s.ReorderNode(c2, "Z")

	c3, _ := s.InsertNode("file", &parentID, nil)
	s.ReorderBetween(c3, &c1, &c2)

	ordered := s.GetOrderedChildren(parentID)
	if len(ordered) != 3 {
		t.Fatalf("expected 3 children, got %d", len(ordered))
	}
	// c3 should be between c1 and c2
	if ordered[0].ID != c1 || ordered[2].ID != c2 {
		t.Error("c1 should be first, c2 last")
	}
	if ordered[1].ID != c3 {
		t.Error("c3 should be in the middle")
	}
}

func TestSoftDeleteNode(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"name": "test"})

	err := s.SoftDeleteNode(id)
	if err != nil {
		t.Fatal(err)
	}

	// Should not be visible normally
	_, err = s.GetNode(id)
	if err == nil {
		t.Error("soft-deleted node should not be visible via GetNode")
	}

	// Should appear in deleted list
	deleted := s.GetDeletedNodes()
	if len(deleted) != 1 {
		t.Errorf("expected 1 deleted node, got %d", len(deleted))
	}
}

func TestCascadeDeleteNode(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	child1, _ := s.InsertNode("file", &parent, nil)
	child2, _ := s.InsertNode("file", &parent, nil)
	grandchild, _ := s.InsertNode("file", &child1, nil)

	err := s.CascadeDeleteNode(parent)
	if err != nil {
		t.Fatal(err)
	}

	_ = child1
	_ = child2
	_ = grandchild

	// Verify all nodes are gone
	all := s.AllNodes()
	if len(all) != 0 {
		t.Errorf("expected 0 nodes after cascade delete, got %d", len(all))
	}
}

func TestRestoreNodeStore(t *testing.T) {
	s := NewStore("r1")
	id, _ := s.InsertNode("item", nil, map[string]interface{}{"name": "hello"})

	s.SoftDeleteNode(id)
	err := s.RestoreNode(id)
	if err != nil {
		t.Fatal(err)
	}

	node, err := s.GetNode(id)
	if err != nil {
		t.Fatal("restored node should be visible")
	}
	if node.Properties["name"] != "hello" {
		t.Error("properties should be preserved")
	}
}

func TestReapOrphans(t *testing.T) {
	s := NewStore("r1")
	parent, _ := s.InsertNode("folder", nil, nil)
	s.InsertNode("file", &parent, nil)
	s.InsertNode("file", &parent, nil)

	// Soft-delete parent, making children orphans
	s.SoftDeleteNode(parent)

	reaped, err := s.ReapOrphans()
	if err != nil {
		t.Fatal(err)
	}
	if len(reaped) < 2 {
		t.Errorf("expected at least 2 reaped nodes, got %d", len(reaped))
	}
}

func TestExecuteBatch(t *testing.T) {
	s := NewStore("r1")

	ops := []BatchOp{
		{Type: BatchInsertNode, NodeType: "user", Properties: map[string]interface{}{"name": "Alice"}},
		{Type: BatchInsertNode, NodeType: "user", Properties: map[string]interface{}{"name": "Bob"}},
	}

	result, err := s.ExecuteBatch(ops)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(result.Results))
	}

	all := s.AllNodes()
	if len(all) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(all))
	}
}

func TestSchemaValidation(t *testing.T) {
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name: "user",
		Properties: map[string]*PropertyDef{
			"name":  {Name: "name", Type: PropString, Required: true},
			"email": {Name: "email", Type: PropString, Required: true, Unique: true},
			"age":   {Name: "age", Type: PropNumber},
		},
		AllowedChildren: []string{"post"},
	})
	schema.DefineNode(&NodeTypeDef{
		Name: "post",
		Properties: map[string]*PropertyDef{
			"title": {Name: "title", Type: PropString, Required: true},
		},
	})
	schema.DefineEdge(&EdgeTypeDef{
		Name:      "authored",
		FromTypes: []string{"user"},
		ToTypes:   []string{"post"},
	})

	// Valid node
	err := schema.ValidateNode("user", map[string]interface{}{"name": "Alice", "email": "a@b.com"})
	if err != nil {
		t.Errorf("valid node should pass: %v", err)
	}

	// Missing required property
	err = schema.ValidateNode("user", map[string]interface{}{"name": "Alice"})
	if err == nil {
		t.Error("missing required 'email' should fail")
	}

	// Wrong type
	err = schema.ValidateNode("user", map[string]interface{}{"name": 123, "email": "a@b.com"})
	if err == nil {
		t.Error("wrong type for 'name' should fail")
	}

	// Unknown node type passes (no schema restriction)
	err = schema.ValidateNode("unknown", map[string]interface{}{"anything": true})
	if err != nil {
		t.Error("unknown type should pass without schema")
	}

	// Hierarchy validation
	err = schema.ValidateHierarchy("user", "post")
	if err != nil {
		t.Errorf("user -> post should be valid: %v", err)
	}
	err = schema.ValidateHierarchy("user", "user")
	if err == nil {
		t.Error("user -> user should fail (not in allowedChildren)")
	}

	// Edge validation
	err = schema.ValidateEdge("authored", "user", "post")
	if err != nil {
		t.Errorf("authored: user->post should be valid: %v", err)
	}
	err = schema.ValidateEdge("authored", "post", "user")
	if err == nil {
		t.Error("authored: post->user should fail")
	}

	// Property type validations
	err = schema.ValidateProperty("user", "age", "not a number")
	if err == nil {
		t.Error("string value for number property should fail")
	}
	err = schema.ValidateProperty("user", "age", 25)
	if err != nil {
		t.Errorf("int value for number should pass: %v", err)
	}
	err = schema.ValidateProperty("user", "age", 25.5)
	if err != nil {
		t.Errorf("float value for number should pass: %v", err)
	}
}

func TestSetSchemaAndGetSchema(t *testing.T) {
	s := NewStore("r1")

	if s.GetSchema() == nil {
		t.Error("default schema should not be nil")
	}

	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name: "item",
		Properties: map[string]*PropertyDef{
			"name": {Name: "name", Type: PropString, Required: true},
		},
	})
	s.SetSchema(schema)

	got := s.GetSchema()
	if got.NodeTypes["item"] == nil {
		t.Error("schema should have 'item' node type")
	}
}

func TestStoreWalker(t *testing.T) {
	s := NewStore("r1")
	w := s.Walker()
	if w == nil {
		t.Error("walker should not be nil")
	}
}

func TestValidatePropertyTypes(t *testing.T) {
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name: "test",
		Properties: map[string]*PropertyDef{
			"str":  {Name: "str", Type: PropString},
			"num":  {Name: "num", Type: PropNumber},
			"bool": {Name: "bool", Type: PropBool},
			"arr":  {Name: "arr", Type: PropArray},
			"obj":  {Name: "obj", Type: PropObject},
			"ref":  {Name: "ref", Type: PropRef},
			"any":  {Name: "any", Type: PropAny},
		},
	})

	// Valid types
	if err := schema.ValidateProperty("test", "str", "hello"); err != nil {
		t.Error(err)
	}
	if err := schema.ValidateProperty("test", "bool", true); err != nil {
		t.Error(err)
	}
	if err := schema.ValidateProperty("test", "arr", []interface{}{"a"}); err != nil {
		t.Error(err)
	}
	if err := schema.ValidateProperty("test", "obj", map[string]interface{}{"k": "v"}); err != nil {
		t.Error(err)
	}
	if err := schema.ValidateProperty("test", "ref", "uuid-string"); err != nil {
		t.Error(err)
	}
	if err := schema.ValidateProperty("test", "any", 42); err != nil {
		t.Error(err)
	}

	// Invalid types
	if err := schema.ValidateProperty("test", "bool", "not-bool"); err == nil {
		t.Error("string for bool should fail")
	}
	if err := schema.ValidateProperty("test", "arr", "not-array"); err == nil {
		t.Error("string for array should fail")
	}
	if err := schema.ValidateProperty("test", "obj", "not-object"); err == nil {
		t.Error("string for object should fail")
	}
	if err := schema.ValidateProperty("test", "ref", 123); err == nil {
		t.Error("number for ref should fail")
	}
}

func TestValidateEdgeUnknownType(t *testing.T) {
	schema := NewSchema()
	// Unknown edge type should pass
	err := schema.ValidateEdge("unknown", "a", "b")
	if err != nil {
		t.Error("unknown edge type should pass")
	}
}

func TestValidateHierarchyNoSchema(t *testing.T) {
	schema := NewSchema()
	err := schema.ValidateHierarchy("unknown", "anything")
	if err != nil {
		t.Error("no schema for parent type should allow anything")
	}
}

func TestValidateEdgeWildcard(t *testing.T) {
	schema := NewSchema()
	schema.DefineEdge(&EdgeTypeDef{
		Name:      "any_edge",
		FromTypes: []string{"*"},
		ToTypes:   []string{"*"},
	})
	err := schema.ValidateEdge("any_edge", "foo", "bar")
	if err != nil {
		t.Error("wildcard edge should allow any types")
	}
}

func TestValidateHierarchyWildcard(t *testing.T) {
	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name:            "container",
		AllowedChildren: []string{"*"},
	})
	err := schema.ValidateHierarchy("container", "anything")
	if err != nil {
		t.Error("wildcard children should allow any child type")
	}
}
