package graph

import (
	"testing"
)

func TestStoreInsertAndGet(t *testing.T) {
	s := NewStore("replica-1")

	id, err := s.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	if err != nil {
		t.Fatalf("InsertNode failed: %v", err)
	}

	node, err := s.GetNode(id)
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	if node.Properties["name"] != "Alice" {
		t.Errorf("expected name Alice, got %v", node.Properties["name"])
	}
}

func TestStoreSchemaValidation(t *testing.T) {
	s := NewStore("replica-1")

	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name: "user",
		Properties: map[string]*PropertyDef{
			"name":  {Name: "name", Type: PropString, Required: true},
			"email": {Name: "email", Type: PropString, Required: true},
		},
	})
	s.SetSchema(schema)

	// Missing required field should fail
	_, err := s.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	if err == nil {
		t.Error("expected validation error for missing required field")
	}

	// All required fields should pass
	_, err = s.InsertNode("user", nil, map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
	})
	if err != nil {
		t.Fatalf("InsertNode with valid data failed: %v", err)
	}
}

func TestStoreHierarchyValidation(t *testing.T) {
	s := NewStore("replica-1")

	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{
		Name:            "org",
		AllowedChildren: []string{"team"},
	})
	schema.DefineNode(&NodeTypeDef{
		Name: "team",
	})
	schema.DefineNode(&NodeTypeDef{
		Name: "project",
	})
	s.SetSchema(schema)

	orgID, _ := s.InsertNode("org", nil, map[string]interface{}{})

	// Allowed child type
	_, err := s.InsertNode("team", &orgID, map[string]interface{}{})
	if err != nil {
		t.Fatalf("InsertNode with allowed child type failed: %v", err)
	}

	// Disallowed child type
	_, err = s.InsertNode("project", &orgID, map[string]interface{}{})
	if err == nil {
		t.Error("expected hierarchy validation error")
	}
}

func TestStoreEdgeValidation(t *testing.T) {
	s := NewStore("replica-1")

	schema := NewSchema()
	schema.DefineNode(&NodeTypeDef{Name: "user"})
	schema.DefineNode(&NodeTypeDef{Name: "document"})
	schema.DefineEdge(&EdgeTypeDef{
		Name:      "authored",
		FromTypes: []string{"user"},
		ToTypes:   []string{"document"},
	})
	s.SetSchema(schema)

	userID, _ := s.InsertNode("user", nil, map[string]interface{}{})
	docID, _ := s.InsertNode("document", nil, map[string]interface{}{})

	// Valid edge
	_, err := s.InsertEdge("authored", userID, docID, nil)
	if err != nil {
		t.Fatalf("InsertEdge failed: %v", err)
	}

	// Invalid edge (wrong direction)
	_, err = s.InsertEdge("authored", docID, userID, nil)
	if err == nil {
		t.Error("expected edge validation error")
	}
}

func TestStoreDeleteNodeWithChildren(t *testing.T) {
	s := NewStore("replica-1")

	parentID, _ := s.InsertNode("org", nil, map[string]interface{}{})
	s.InsertNode("team", &parentID, map[string]interface{}{})

	err := s.DeleteNode(parentID)
	if err == nil {
		t.Error("expected error when deleting node with children")
	}
}

func TestStoreTraverse(t *testing.T) {
	s := NewStore("replica-1")

	a, _ := s.InsertNode("user", nil, map[string]interface{}{"name": "A"})
	b, _ := s.InsertNode("user", nil, map[string]interface{}{"name": "B"})
	c, _ := s.InsertNode("user", nil, map[string]interface{}{"name": "C"})

	s.InsertEdge("follows", a, b, nil)
	s.InsertEdge("follows", b, c, nil)

	nodes, err := s.Traverse(a, "follows", "out", 10)
	if err != nil {
		t.Fatalf("Traverse failed: %v", err)
	}
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes in traversal, got %d", len(nodes))
	}
}

func TestStoreGetSubtree(t *testing.T) {
	s := NewStore("replica-1")

	root, _ := s.InsertNode("org", nil, map[string]interface{}{"name": "Root"})
	child1, _ := s.InsertNode("team", &root, map[string]interface{}{"name": "Team1"})
	s.InsertNode("member", &child1, map[string]interface{}{"name": "Alice"})
	s.InsertNode("team", &root, map[string]interface{}{"name": "Team2"})

	subtree, err := s.GetSubtree(root)
	if err != nil {
		t.Fatalf("GetSubtree failed: %v", err)
	}
	if len(subtree) != 4 {
		t.Errorf("expected 4 nodes in subtree, got %d", len(subtree))
	}
}

func TestStoreGetAncestors(t *testing.T) {
	s := NewStore("replica-1")

	root, _ := s.InsertNode("org", nil, map[string]interface{}{})
	child, _ := s.InsertNode("team", &root, map[string]interface{}{})
	grandchild, _ := s.InsertNode("member", &child, map[string]interface{}{})

	ancestors := s.GetAncestors(grandchild)
	if len(ancestors) != 2 {
		t.Errorf("expected 2 ancestors, got %d", len(ancestors))
	}
}

func TestStoreFindByIndex(t *testing.T) {
	s := NewStore("replica-1")

	s.InsertNode("user", nil, map[string]interface{}{"name": "Alice", "role": "admin"})
	s.InsertNode("user", nil, map[string]interface{}{"name": "Bob", "role": "admin"})
	s.InsertNode("user", nil, map[string]interface{}{"name": "Charlie", "role": "user"})

	admins := s.FindByIndex("user", "role", "admin")
	if len(admins) != 2 {
		t.Errorf("expected 2 admins, got %d", len(admins))
	}
}
