package system

import (
	"encoding/json"
	"testing"
)

func TestMultiGraphCreateAndGet(t *testing.T) {
	mg := NewMultiGraph("test-replica")

	g1, err := mg.Create("users")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if g1.Name != "users" {
		t.Errorf("expected name 'users', got %s", g1.Name)
	}

	g2, err := mg.Create("products")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	_ = g2

	if mg.Count() != 2 {
		t.Errorf("expected 2 graphs, got %d", mg.Count())
	}

	// Get existing
	got, ok := mg.Get("users")
	if !ok || got != g1 {
		t.Error("Get should return the same instance")
	}

	// Get non-existent
	_, ok = mg.Get("nonexistent")
	if ok {
		t.Error("Get should return false for non-existent graph")
	}

	// Duplicate creation should fail
	_, err = mg.Create("users")
	if err == nil {
		t.Error("duplicate create should fail")
	}
}

func TestMultiGraphGetOrCreate(t *testing.T) {
	mg := NewMultiGraph("test-replica")

	g1 := mg.GetOrCreate("test")
	g2 := mg.GetOrCreate("test")

	if g1 != g2 {
		t.Error("GetOrCreate should return same instance")
	}
	if mg.Count() != 1 {
		t.Errorf("expected 1 graph, got %d", mg.Count())
	}
}

func TestMultiGraphDelete(t *testing.T) {
	mg := NewMultiGraph("test-replica")

	mg.Create("temp")
	if mg.Count() != 1 {
		t.Fatal("expected 1 graph")
	}

	if err := mg.Delete("temp"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if mg.Count() != 0 {
		t.Error("expected 0 graphs after delete")
	}

	if err := mg.Delete("nonexistent"); err == nil {
		t.Error("deleting non-existent graph should fail")
	}
}

func TestMultiGraphList(t *testing.T) {
	mg := NewMultiGraph("test-replica")

	mg.Create("alpha")
	mg.Create("beta")
	mg.Create("gamma")

	names := mg.List()
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d", len(names))
	}
}

func TestMultiGraphIndependentStores(t *testing.T) {
	mg := NewMultiGraph("test-replica")

	g1 := mg.GetOrCreate("graph1")
	g2 := mg.GetOrCreate("graph2")

	// Insert into g1
	id1, _ := g1.Store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})

	// g2 should not have it
	_, err := g2.Store.GetNode(id1)
	if err == nil {
		t.Error("graph2 should not have graph1's nodes")
	}

	// Insert into g2
	_, _ = g2.Store.InsertNode("product", nil, map[string]interface{}{"name": "Widget"})

	// g1 should have 1 node, g2 should have 1 node
	if len(g1.Store.AllNodes()) != 1 {
		t.Errorf("graph1 should have 1 node, got %d", len(g1.Store.AllNodes()))
	}
	if len(g2.Store.AllNodes()) != 1 {
		t.Errorf("graph2 should have 1 node, got %d", len(g2.Store.AllNodes()))
	}
}

func TestSchemaBackwardCompatibility(t *testing.T) {
	mg := NewMultiGraph("test-replica")

	// Deploy initial schema
	schema1 := CompiledSchema{
		Version:   1,
		GraphName: "compat-test",
		NodeTypes: []CompiledNodeType{
			{
				Name: "user",
				Properties: map[string]CompiledProperty{
					"name":  {Type: "string", Required: true},
					"email": {Type: "string", Required: true, Unique: true},
					"bio":   {Type: "string", Required: false},
				},
			},
			{
				Name: "post",
				Properties: map[string]CompiledProperty{
					"title": {Type: "string", Required: true},
				},
			},
		},
		EdgeTypes: []CompiledEdgeType{
			{Name: "authored", FromTypes: []string{"user"}, ToTypes: []string{"post"}},
		},
	}

	schemaJSON1, _ := json.Marshal(schema1)
	if err := mg.DeploySchema("compat-test", schemaJSON1); err != nil {
		t.Fatalf("initial deploy failed: %v", err)
	}

	// Insert some data
	g, _ := mg.Get("compat-test")
	g.Store.InsertNode("user", nil, map[string]interface{}{
		"name":  "Alice",
		"email": "alice@test.com",
	})
	g.Store.InsertNode("post", nil, map[string]interface{}{
		"title": "Hello World",
	})

	// Test: removing required property should fail
	schema2 := schema1
	schema2.NodeTypes = []CompiledNodeType{
		{
			Name: "user",
			Properties: map[string]CompiledProperty{
				"email": {Type: "string", Required: true, Unique: true},
				// "name" removed — should fail because it was required
			},
		},
		schema1.NodeTypes[1],
	}
	schemaJSON2, _ := json.Marshal(schema2)
	err := mg.ValidateSchemaCompatibility("compat-test", schemaJSON2)
	if err == nil {
		t.Error("removing required property should fail compatibility check")
	}

	// Test: changing property type should fail
	schema3 := schema1
	schema3.NodeTypes = []CompiledNodeType{
		{
			Name: "user",
			Properties: map[string]CompiledProperty{
				"name":  {Type: "number", Required: true}, // changed from string to number
				"email": {Type: "string", Required: true, Unique: true},
				"bio":   {Type: "string", Required: false},
			},
		},
		schema1.NodeTypes[1],
	}
	schemaJSON3, _ := json.Marshal(schema3)
	err = mg.ValidateSchemaCompatibility("compat-test", schemaJSON3)
	if err == nil {
		t.Error("changing property type should fail compatibility check")
	}

	// Test: making optional property required should fail (existing data)
	schema4 := schema1
	schema4.NodeTypes = []CompiledNodeType{
		{
			Name: "user",
			Properties: map[string]CompiledProperty{
				"name":  {Type: "string", Required: true},
				"email": {Type: "string", Required: true, Unique: true},
				"bio":   {Type: "string", Required: true}, // was optional, now required
			},
		},
		schema1.NodeTypes[1],
	}
	schemaJSON4, _ := json.Marshal(schema4)
	err = mg.ValidateSchemaCompatibility("compat-test", schemaJSON4)
	if err == nil {
		t.Error("making optional property required should fail when nodes exist")
	}

	// Test: removing node type with existing nodes should fail
	schema5 := schema1
	schema5.NodeTypes = []CompiledNodeType{
		schema1.NodeTypes[0], // keep user
		// remove post — but posts exist
	}
	schemaJSON5, _ := json.Marshal(schema5)
	err = mg.ValidateSchemaCompatibility("compat-test", schemaJSON5)
	if err == nil {
		t.Error("removing node type with existing nodes should fail")
	}

	// Test: adding a new optional property should succeed
	schema6 := schema1
	schema6.NodeTypes = []CompiledNodeType{
		{
			Name: "user",
			Properties: map[string]CompiledProperty{
				"name":   {Type: "string", Required: true},
				"email":  {Type: "string", Required: true, Unique: true},
				"bio":    {Type: "string", Required: false},
				"avatar": {Type: "string", Required: false}, // new optional property
			},
		},
		schema1.NodeTypes[1],
	}
	schema6.EdgeTypes = schema1.EdgeTypes
	schemaJSON6, _ := json.Marshal(schema6)
	err = mg.ValidateSchemaCompatibility("compat-test", schemaJSON6)
	if err != nil {
		t.Errorf("adding optional property should be compatible: %v", err)
	}

	// Test: adding a new node type should succeed
	schema7 := schema1
	schema7.NodeTypes = append(schema1.NodeTypes, CompiledNodeType{
		Name: "comment",
		Properties: map[string]CompiledProperty{
			"text": {Type: "string", Required: true},
		},
	})
	schema7.EdgeTypes = schema1.EdgeTypes
	schemaJSON7, _ := json.Marshal(schema7)
	err = mg.ValidateSchemaCompatibility("compat-test", schemaJSON7)
	if err != nil {
		t.Errorf("adding new node type should be compatible: %v", err)
	}

	// Test: removing optional property should succeed
	schema8 := schema1
	schema8.NodeTypes = []CompiledNodeType{
		{
			Name: "user",
			Properties: map[string]CompiledProperty{
				"name":  {Type: "string", Required: true},
				"email": {Type: "string", Required: true, Unique: true},
				// "bio" removed — was optional, should be fine
			},
		},
		schema1.NodeTypes[1],
	}
	schema8.EdgeTypes = schema1.EdgeTypes
	schemaJSON8, _ := json.Marshal(schema8)
	err = mg.ValidateSchemaCompatibility("compat-test", schemaJSON8)
	if err != nil {
		t.Errorf("removing optional property should be compatible: %v", err)
	}

	// Test: compatibility check on non-existent graph should pass
	err = mg.ValidateSchemaCompatibility("nonexistent", schemaJSON1)
	if err != nil {
		t.Errorf("new graph should always be compatible: %v", err)
	}
}

func TestMultiGraphLoadSchema(t *testing.T) {
	mg := NewMultiGraph("test-replica")

	schema := CompiledSchema{
		Version:   1,
		GraphName: "test-app",
		NodeTypes: []CompiledNodeType{
			{
				Name: "user",
				Properties: map[string]CompiledProperty{
					"name":  {Type: "string", Required: true},
					"email": {Type: "string", Required: true, Unique: true, Indexed: true},
					"age":   {Type: "number", Required: false},
				},
			},
			{
				Name: "post",
				Properties: map[string]CompiledProperty{
					"title": {Type: "string", Required: true},
					"body":  {Type: "string", Required: false},
				},
			},
		},
		EdgeTypes: []CompiledEdgeType{
			{Name: "authored", FromTypes: []string{"user"}, ToTypes: []string{"post"}},
		},
		Invariants: []CompiledInvariant{
			{
				ID:     "unique-email",
				Name:   "Unique Email",
				Type:   "uniqueness",
				Config: json.RawMessage(`{"nodeType":"user","property":"email"}`),
			},
		},
	}

	schemaJSON, _ := json.Marshal(schema)
	err := mg.LoadSchema("test-app", schemaJSON)
	if err != nil {
		t.Fatalf("LoadSchema failed: %v", err)
	}

	g, ok := mg.Get("test-app")
	if !ok {
		t.Fatal("graph should exist after LoadSchema")
	}

	// Test that schema validation works
	_, err = g.Store.InsertNode("user", nil, map[string]interface{}{
		"name":  "Alice",
		"email": "alice@test.com",
	})
	if err != nil {
		t.Fatalf("insert should succeed: %v", err)
	}

	// Test that invariant was loaded
	invs := g.Validator.List()
	if len(invs) != 1 {
		t.Errorf("expected 1 invariant, got %d", len(invs))
	}

	// Schema JSON should be stored
	if g.SchemaJSON == nil {
		t.Error("SchemaJSON should be stored")
	}
}
