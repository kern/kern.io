package system

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kern/graphdb/internal/derived"
	"github.com/kern/graphdb/internal/function"
)

// --- Derive and DerivedType ---

func TestDerive(t *testing.T) {
	sys := New("r1")
	sys.Schema(func(sb *SchemaBuilder) {
		sb.Node("item", func(nb *NodeBuilder) {
			nb.String("name", true)
		})
		sb.Derive(derived.NewPipeline("p1", "test").
			Map("item", "d_item", nil))
	})
}

func TestDerivedType(t *testing.T) {
	sys := New("r1")
	sys.Schema(func(sb *SchemaBuilder) {
		sb.DerivedType(derived.NewDerivedNodeType("d_item").
			Field("name", derived.FieldString, true))
	})
}

// --- registerSystemBuiltins comprehensive ---

func TestRegisterSystemBuiltinsComprehensive(t *testing.T) {
	sys := New("r1")
	registerSystemBuiltins(sys)

	fns := sys.Registry().List()
	expectedNames := []string{
		"graphdb:getNode",
		"graphdb:getNodesByType",
		"graphdb:getChildren",
		"graphdb:getOrderedChildren",
		"graphdb:getParent",
		"graphdb:getRoots",
		"graphdb:getSubtree",
		"graphdb:getAncestors",
		"graphdb:getOutEdges",
		"graphdb:getInEdges",
		"graphdb:stats",
		"graphdb:getDeletedNodes",
		"graphdb:insertNode",
		"graphdb:deleteNode",
		"graphdb:softDeleteNode",
		"graphdb:cascadeDeleteNode",
		"graphdb:restoreNode",
		"graphdb:patchNode",
		"graphdb:setProperty",
		"graphdb:deleteProperty",
		"graphdb:insertEdge",
		"graphdb:deleteEdge",
		"graphdb:moveNode",
		"graphdb:reapOrphans",
	}

	nameSet := make(map[string]bool)
	for _, fn := range fns {
		nameSet[fn.Name] = true
	}

	for _, name := range expectedNames {
		if !nameSet[name] {
			t.Errorf("expected builtin %q to be registered", name)
		}
	}
}

func callOK(t *testing.T, sys *System, name string, args map[string]interface{}) interface{} {
	t.Helper()
	ctx := context.Background()
	r := sys.Registry().Call(ctx, name, args)
	if r.Error != "" {
		t.Fatalf("%s failed: %s", name, r.Error)
	}
	return r.Value
}

func TestCallBuiltinInsertAndGet(t *testing.T) {
	sys := New("r1")
	registerSystemBuiltins(sys)

	// Insert a node - returns UUID string
	result := callOK(t, sys, "graphdb:insertNode", map[string]interface{}{
		"type":       "user",
		"properties": map[string]interface{}{"name": "Alice"},
	})
	nodeID := result.(string)

	// Get the node
	callOK(t, sys, "graphdb:getNode", map[string]interface{}{"id": nodeID})

	// Get nodes by type
	callOK(t, sys, "graphdb:getNodesByType", map[string]interface{}{"type": "user"})

	// Get stats
	callOK(t, sys, "graphdb:stats", map[string]interface{}{})

	// Get roots
	callOK(t, sys, "graphdb:getRoots", map[string]interface{}{})

	// Get deleted nodes
	callOK(t, sys, "graphdb:getDeletedNodes", map[string]interface{}{})

	// Set property
	callOK(t, sys, "graphdb:setProperty", map[string]interface{}{
		"id": nodeID, "key": "email", "value": "alice@test.com",
	})

	// Delete property
	callOK(t, sys, "graphdb:deleteProperty", map[string]interface{}{
		"id": nodeID, "key": "email",
	})

	// Patch node
	callOK(t, sys, "graphdb:patchNode", map[string]interface{}{
		"id": nodeID, "properties": map[string]interface{}{"name": "Updated"},
	})

	// Soft delete
	callOK(t, sys, "graphdb:softDeleteNode", map[string]interface{}{"id": nodeID})

	// Restore
	callOK(t, sys, "graphdb:restoreNode", map[string]interface{}{"id": nodeID})

	// Reap orphans
	callOK(t, sys, "graphdb:reapOrphans", map[string]interface{}{})
}

func TestCallBuiltinEdgeOperations(t *testing.T) {
	sys := New("r1")
	registerSystemBuiltins(sys)

	id1 := callOK(t, sys, "graphdb:insertNode", map[string]interface{}{"type": "user"}).(string)
	id2 := callOK(t, sys, "graphdb:insertNode", map[string]interface{}{"type": "user"}).(string)

	callOK(t, sys, "graphdb:insertEdge", map[string]interface{}{
		"type": "follows", "from": id1, "to": id2,
	})
	callOK(t, sys, "graphdb:getOutEdges", map[string]interface{}{"id": id1})
	callOK(t, sys, "graphdb:getInEdges", map[string]interface{}{"id": id2})
}

func TestCallBuiltinHierarchy(t *testing.T) {
	sys := New("r1")
	registerSystemBuiltins(sys)

	parentID := callOK(t, sys, "graphdb:insertNode", map[string]interface{}{"type": "folder"}).(string)
	childID := callOK(t, sys, "graphdb:insertNode", map[string]interface{}{
		"type": "file", "parentId": parentID,
	}).(string)

	callOK(t, sys, "graphdb:getChildren", map[string]interface{}{"id": parentID})
	callOK(t, sys, "graphdb:getOrderedChildren", map[string]interface{}{"id": parentID})
	callOK(t, sys, "graphdb:getParent", map[string]interface{}{"id": childID})
	callOK(t, sys, "graphdb:getSubtree", map[string]interface{}{"id": parentID})
	callOK(t, sys, "graphdb:getAncestors", map[string]interface{}{"id": childID})
	callOK(t, sys, "graphdb:moveNode", map[string]interface{}{"id": childID})
	callOK(t, sys, "graphdb:deleteNode", map[string]interface{}{"id": childID})
	callOK(t, sys, "graphdb:cascadeDeleteNode", map[string]interface{}{"id": parentID})
}

// --- buildInvariant ---

func TestBuildInvariantAllTypes(t *testing.T) {
	tests := []struct {
		name   string
		ci     CompiledInvariant
		wantOk bool
	}{
		{
			name: "uniqueness",
			ci: CompiledInvariant{
				ID:   "uniq-1",
				Type: "uniqueness",
				Config: json.RawMessage(`{"nodeType":"user","property":"email"}`),
			},
			wantOk: true,
		},
		{
			name: "acyclicity",
			ci: CompiledInvariant{
				ID:   "acyc-1",
				Type: "acyclicity",
				Config: json.RawMessage(`{"edgeType":"depends_on"}`),
			},
			wantOk: true,
		},
		{
			name: "cardinality",
			ci: CompiledInvariant{
				ID:   "card-1",
				Type: "cardinality",
				Config: json.RawMessage(`{"nodeType":"user","edgeType":"follows","direction":"out","max":100}`),
			},
			wantOk: true,
		},
		{
			name: "edge_constraint",
			ci: CompiledInvariant{
				ID:   "ec-1",
				Type: "edge_constraint",
				Config: json.RawMessage(`{"edgeType":"follows","fromTypes":["user"],"toTypes":["user"]}`),
			},
			wantOk: true,
		},
		{
			name: "hierarchy_depth",
			ci: CompiledInvariant{
				ID:   "hd-1",
				Type: "hierarchy_depth",
				Config: json.RawMessage(`{"maxDepth":10}`),
			},
			wantOk: true,
		},
		{
			name: "child_count",
			ci: CompiledInvariant{
				ID:   "cc-1",
				Type: "child_count",
				Config: json.RawMessage(`{"parentType":"folder","max":50}`),
			},
			wantOk: true,
		},
		{
			name: "unknown",
			ci: CompiledInvariant{
				ID:   "unk-1",
				Type: "unknown_type",
				Config: json.RawMessage(`{}`),
			},
			wantOk: false,
		},
		{
			name: "bad json",
			ci: CompiledInvariant{
				ID:   "bad-1",
				Type: "uniqueness",
				Config: json.RawMessage(`{invalid`),
			},
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, err := buildInvariant(tt.ci)
			if tt.wantOk {
				if err != nil {
					t.Errorf("expected success, got error: %v", err)
				}
				if inv == nil {
					t.Error("expected invariant, got nil")
				}
			} else {
				if err == nil {
					t.Error("expected error")
				}
			}
		})
	}
}

// --- ApplyModule ---

func TestApplyModule(t *testing.T) {
	mg := NewMultiGraph("r1")
	mg.GetOrCreate("test")

	module := &CompiledModule{
		ID:   "mod-1",
		Name: "test-module",
		NodeTypes: []CompiledNodeType{
			{Name: "widget", Properties: map[string]CompiledProperty{"label": {Type: "string", Required: true}}},
		},
	}

	err := mg.ApplyModule("test", module)
	if err != nil {
		t.Errorf("ApplyModule failed: %v", err)
	}
}

func TestApplyModuleNotFound(t *testing.T) {
	mg := NewMultiGraph("r1")
	err := mg.ApplyModule("nonexistent", &CompiledModule{ID: "m1"})
	if err == nil {
		t.Error("expected error for non-existent graph")
	}
}

func TestApplyModuleWithCondition(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")
	g.FeatureFlags = map[string]bool{"dark-mode": true}

	module := &CompiledModule{
		ID:   "mod-1",
		Name: "dark-module",
		Condition: &ModuleCondition{
			FeatureFlags: []string{"dark-mode"},
		},
	}

	err := mg.ApplyModule("test", module)
	if err != nil {
		t.Errorf("ApplyModule with met condition failed: %v", err)
	}
}

func TestApplyModuleConditionNotMet(t *testing.T) {
	mg := NewMultiGraph("r1")
	mg.GetOrCreate("test")

	module := &CompiledModule{
		ID:   "mod-1",
		Name: "dark-module",
		Condition: &ModuleCondition{
			FeatureFlags: []string{"dark-mode"},
		},
	}

	err := mg.ApplyModule("test", module)
	if err == nil {
		t.Error("expected error when conditions not met")
	}
}

func TestApplyModuleMissingDependency(t *testing.T) {
	mg := NewMultiGraph("r1")
	mg.GetOrCreate("test")

	module := &CompiledModule{
		ID:        "mod-2",
		Name:      "depends-module",
		DependsOn: []string{"mod-1"},
	}

	err := mg.ApplyModule("test", module)
	if err == nil {
		t.Error("expected error for missing dependency")
	}
}

// --- StopAll ---

func TestStopAll(t *testing.T) {
	mg := NewMultiGraph("r1")
	mg.GetOrCreate("g1")
	mg.GetOrCreate("g2")
	mg.StopAll() // should not panic
}

// --- DeploySchema ---

func TestDeploySchema(t *testing.T) {
	mg := NewMultiGraph("r1")

	schemaJSON := `{
		"nodeTypes": [{"name": "user", "properties": {"name": {"type": "string", "required": true}}}],
		"edgeTypes": [{"name": "follows", "fromTypes": ["user"], "toTypes": ["user"]}],
		"invariants": [{"id": "uniq-email", "type": "uniqueness", "config": {"nodeType": "user", "property": "email"}}]
	}`

	err := mg.DeploySchema("app", json.RawMessage(schemaJSON))
	if err != nil {
		t.Fatalf("DeploySchema failed: %v", err)
	}
}

func TestDeploySchemaInvalidJSON(t *testing.T) {
	mg := NewMultiGraph("r1")
	err := mg.DeploySchema("app", json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- ValidateSchemaCompatibility ---

func TestValidateSchemaCompatibilityNewGraph(t *testing.T) {
	mg := NewMultiGraph("r1")
	err := mg.ValidateSchemaCompatibility("new-graph", json.RawMessage(`{}`))
	if err != nil {
		t.Error("new graph should accept any schema")
	}
}

func TestValidateSchemaCompatibilityInvalidJSON(t *testing.T) {
	mg := NewMultiGraph("r1")
	mg.GetOrCreate("test")

	// Deploy initial schema
	mg.DeploySchema("test", json.RawMessage(`{"nodeTypes": [{"name": "user", "properties": []}]}`))

	err := mg.ValidateSchemaCompatibility("test", json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- isModuleActive ---

func TestIsModuleActiveNoCondition(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	active := mg.isModuleActive(g, &CompiledModule{ID: "m1"})
	if !active {
		t.Error("module without condition should be active")
	}
}

func TestIsModuleActiveNilFlags(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	active := mg.isModuleActive(g, &CompiledModule{
		ID: "m1",
		Condition: &ModuleCondition{
			FeatureFlags: []string{"flag1"},
		},
	})
	if active {
		t.Error("module should not be active when graph has nil flags")
	}
}

// --- Stop with reactor and derivedStore ---

func TestStopWithSubsystems(t *testing.T) {
	sys := New("r1")
	// Stop should be safe with reactor=nil and derivedStore set
	sys.Stop()
}

// --- Schema with edge ---

func TestSchemaEdgeRegistration(t *testing.T) {
	sys := New("r1")
	sys.Schema(func(sb *SchemaBuilder) {
		sb.Node("user", func(nb *NodeBuilder) {
			nb.String("name", true)
		})
		sb.Node("post", func(nb *NodeBuilder) {
			nb.String("title", true)
		})
		sb.Edge("authored", []string{"user"}, []string{"post"})
	})

	schema := sys.Store().GetSchema()
	if schema.EdgeTypes["authored"] == nil {
		t.Error("authored edge type should be defined")
	}
}

// --- Query/Mutation/Action with builtins ---

func TestUserDefinedFunctions(t *testing.T) {
	sys := New("r1")

	sys.Query("myQuery", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return "query-result", nil
	})

	sys.Mutation("myMutation", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return "mutation-result", nil
	})

	sys.Action("myAction", func(ctx context.Context, a *function.ActionCtx, args map[string]interface{}) (interface{}, error) {
		return "action-result", nil
	})

	ctx := context.Background()

	r := sys.Registry().Call(ctx, "myQuery", nil)
	if r.Error != "" || r.Value != "query-result" {
		t.Error("query should work")
	}

	r = sys.Registry().Call(ctx, "myMutation", nil)
	if r.Error != "" || r.Value != "mutation-result" {
		t.Error("mutation should work")
	}

	r = sys.Registry().Call(ctx, "myAction", nil)
	if r.Error != "" || r.Value != "action-result" {
		t.Error("action should work")
	}
}
