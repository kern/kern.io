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

func TestModuleComposition(t *testing.T) {
	mg := NewMultiGraph("test-replica")

	// Deploy a schema with two composable modules
	schema := CompiledSchema{
		Version:   1,
		GraphName: "modular-app",
		NodeTypes: []CompiledNodeType{}, // base schema has no types
		Modules: []CompiledModule{
			{
				ID:        "users",
				Name:      "User System",
				Namespace: "users",
				NodeTypes: []CompiledNodeType{
					{
						Name: "users:profile",
						Properties: map[string]CompiledProperty{
							"name":  {Type: "string", Required: true},
							"email": {Type: "string", Required: true, Unique: true},
						},
					},
				},
				EdgeTypes: []CompiledEdgeType{
					{Name: "follows", FromTypes: []string{"users:profile"}, ToTypes: []string{"users:profile"}},
				},
			},
			{
				ID:        "billing",
				Name:      "Billing System",
				Namespace: "billing",
				DependsOn: []string{"users"},
				NodeTypes: []CompiledNodeType{
					{
						Name: "billing:invoice",
						Properties: map[string]CompiledProperty{
							"amount": {Type: "number", Required: true},
						},
					},
				},
				EdgeTypes: []CompiledEdgeType{
					// Cross-module edge
					{Name: "billed_to", FromTypes: []string{"billing:invoice"}, ToTypes: []string{"users:profile"}},
				},
			},
		},
	}

	schemaJSON, _ := json.Marshal(schema)
	err := mg.LoadSchema("modular-app", schemaJSON)
	if err != nil {
		t.Fatalf("LoadSchema with modules failed: %v", err)
	}

	g, ok := mg.Get("modular-app")
	if !ok {
		t.Fatal("graph should exist")
	}

	// Both modules should be active
	if len(g.ActiveModules) != 2 {
		t.Errorf("expected 2 active modules, got %d: %v", len(g.ActiveModules), g.ActiveModules)
	}

	// Insert nodes using both module types
	_, err = g.Store.InsertNode("users:profile", nil, map[string]interface{}{
		"name":  "Alice",
		"email": "alice@test.com",
	})
	if err != nil {
		t.Fatalf("insert users:profile failed: %v", err)
	}

	_, err = g.Store.InsertNode("billing:invoice", nil, map[string]interface{}{
		"amount": 99.99,
	})
	if err != nil {
		t.Fatalf("insert billing:invoice failed: %v", err)
	}
}

func TestModuleConditionalActivation(t *testing.T) {
	mg := NewMultiGraph("test-replica")

	// Create graph with feature flags
	g := mg.GetOrCreate("flagged-app")
	g.FeatureFlags = map[string]bool{"premium": true}

	modules := []CompiledModule{
		{
			ID:   "base",
			Name: "Base",
			NodeTypes: []CompiledNodeType{
				{
					Name:       "item",
					Properties: map[string]CompiledProperty{"name": {Type: "string", Required: true}},
				},
			},
		},
		{
			ID:   "premium-features",
			Name: "Premium Features",
			Condition: &ModuleCondition{
				FeatureFlags: []string{"premium"},
			},
			NodeTypes: []CompiledNodeType{
				{
					Name:       "premium_item",
					Properties: map[string]CompiledProperty{"tier": {Type: "string", Required: true}},
				},
			},
		},
		{
			ID:   "beta-features",
			Name: "Beta Features",
			Condition: &ModuleCondition{
				FeatureFlags: []string{"beta"},
			},
			NodeTypes: []CompiledNodeType{
				{
					Name:       "beta_item",
					Properties: map[string]CompiledProperty{"test": {Type: "string"}},
				},
			},
		},
	}

	err := mg.ResolveModules("flagged-app", modules)
	if err != nil {
		t.Fatalf("ResolveModules failed: %v", err)
	}

	// base and premium should be active, beta should not
	if len(g.ActiveModules) != 2 {
		t.Errorf("expected 2 active modules, got %d: %v", len(g.ActiveModules), g.ActiveModules)
	}

	// Premium type should be usable
	_, err = g.Store.InsertNode("premium_item", nil, map[string]interface{}{"tier": "gold"})
	if err != nil {
		t.Fatalf("insert premium_item failed: %v", err)
	}
}

func TestModuleDependencyCycle(t *testing.T) {
	mg := NewMultiGraph("test-replica")
	mg.GetOrCreate("cycle-test")

	modules := []CompiledModule{
		{ID: "a", Name: "A", DependsOn: []string{"b"}},
		{ID: "b", Name: "B", DependsOn: []string{"a"}},
	}

	err := mg.ResolveModules("cycle-test", modules)
	if err == nil {
		t.Error("expected circular dependency error")
	}
}

func TestModuleMissingDependency(t *testing.T) {
	mg := NewMultiGraph("test-replica")
	mg.GetOrCreate("dep-test")

	modules := []CompiledModule{
		{
			ID:        "child",
			Name:      "Child",
			DependsOn: []string{"nonexistent-parent"},
		},
	}

	err := mg.ResolveModules("dep-test", modules)
	if err == nil {
		t.Error("expected missing dependency error")
	}
}

func TestFeatureFlags(t *testing.T) {
	mg := NewMultiGraph("test-replica")
	mg.GetOrCreate("flags-test")

	// Set flags
	err := mg.SetFeatureFlags("flags-test", map[string]bool{"alpha": true, "beta": false})
	if err != nil {
		t.Fatalf("SetFeatureFlags failed: %v", err)
	}

	flags, err := mg.GetFeatureFlags("flags-test")
	if err != nil {
		t.Fatalf("GetFeatureFlags failed: %v", err)
	}
	if !flags["alpha"] {
		t.Error("expected alpha flag to be true")
	}
	if flags["beta"] {
		t.Error("expected beta flag to be false")
	}

	// Non-existent graph
	err = mg.SetFeatureFlags("nonexistent", map[string]bool{})
	if err == nil {
		t.Error("expected error for non-existent graph")
	}
	_, err = mg.GetFeatureFlags("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent graph")
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
