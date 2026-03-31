package system

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/kern/graphdb/internal/derived"
	"github.com/kern/graphdb/internal/function"
	"github.com/kern/graphdb/internal/graph"
)

// ---------------------------------------------------------------------------
// Start / Stop coverage
// ---------------------------------------------------------------------------

func TestStartOnRandomPortAndStop(t *testing.T) {
	sys := New("start-test")

	// Start on a random port in a goroutine so it doesn't block.
	errCh := make(chan error, 1)
	go func() {
		errCh <- sys.Start(":0")
	}()

	// Give the server a moment to bind.
	time.Sleep(100 * time.Millisecond)

	// Stop the system (exercises the reactor != nil path).
	sys.Stop()

	// The server should eventually return (http.ErrServerClosed or nil).
	// We don't assert on the exact error because graceful shutdown is async.
}

func TestStartWithEmptyAddr(t *testing.T) {
	// When addr is empty, Start should use the configured port.
	// We override the port to a random free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	config := DefaultConfig()
	config.Port = port
	sys := NewWithConfig("empty-addr-test", config)

	errCh := make(chan error, 1)
	go func() {
		errCh <- sys.Start("") // empty addr triggers default port path
	}()

	time.Sleep(100 * time.Millisecond)
	sys.Stop()
}

func TestStopWithReactorSet(t *testing.T) {
	// Directly set reactor via Start then Stop to cover reactor.Stop() branch.
	sys := New("reactor-stop-test")

	go func() {
		sys.Start(":0")
	}()
	time.Sleep(100 * time.Millisecond)

	// Now reactor is non-nil; Stop should call reactor.Stop()
	sys.Stop()
}

// ---------------------------------------------------------------------------
// registerSystemBuiltins — uncovered mutation paths
// ---------------------------------------------------------------------------

func TestBuiltinInsertNodeWithParentID(t *testing.T) {
	sys := New("r1")
	registerSystemBuiltins(sys)

	ctx := context.Background()

	// Insert root
	r := sys.Registry().Call(ctx, "graphdb:insertNode", map[string]interface{}{
		"type": "folder",
	})
	if r.Error != "" {
		t.Fatalf("insert root failed: %s", r.Error)
	}
	parentID := r.Value.(string)

	// Insert child with parentId (string branch in insertNode)
	r = sys.Registry().Call(ctx, "graphdb:insertNode", map[string]interface{}{
		"type":     "file",
		"parentId": parentID,
	})
	if r.Error != "" {
		t.Fatalf("insert child failed: %s", r.Error)
	}
}

func TestBuiltinMoveNodeWithParentID(t *testing.T) {
	sys := New("r1")
	registerSystemBuiltins(sys)

	ctx := context.Background()
	r := sys.Registry().Call(ctx, "graphdb:insertNode", map[string]interface{}{"type": "folder"})
	parentID := r.Value.(string)
	r = sys.Registry().Call(ctx, "graphdb:insertNode", map[string]interface{}{"type": "file"})
	childID := r.Value.(string)

	// Move with parentId (exercises the string-to-pointer branch)
	r = sys.Registry().Call(ctx, "graphdb:moveNode", map[string]interface{}{
		"id":       childID,
		"parentId": parentID,
	})
	if r.Error != "" {
		t.Fatalf("moveNode with parentId failed: %s", r.Error)
	}
}

func TestBuiltinReapOrphansWithData(t *testing.T) {
	sys := New("r1")
	registerSystemBuiltins(sys)

	ctx := context.Background()
	// Insert a parent and child
	r := sys.Registry().Call(ctx, "graphdb:insertNode", map[string]interface{}{"type": "folder"})
	parentID := r.Value.(string)
	sys.Registry().Call(ctx, "graphdb:insertNode", map[string]interface{}{
		"type":     "file",
		"parentId": parentID,
	})

	// Delete parent (orphans child)
	sys.Registry().Call(ctx, "graphdb:deleteNode", map[string]interface{}{"id": parentID})

	// Reap orphans
	r = sys.Registry().Call(ctx, "graphdb:reapOrphans", map[string]interface{}{})
	if r.Error != "" {
		t.Fatalf("reapOrphans failed: %s", r.Error)
	}
}

// ---------------------------------------------------------------------------
// applyCompiledSchema — edge types, pipelines, invariants, unknown prop type
// ---------------------------------------------------------------------------

func TestApplyCompiledSchemaWithEdgeTypes(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	compiled := &CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{
				"name": {Type: "string", Required: true},
			}},
			{Name: "post", Properties: map[string]CompiledProperty{
				"title": {Type: "string", Required: true},
			}},
		},
		EdgeTypes: []CompiledEdgeType{
			{Name: "authored", FromTypes: []string{"user"}, ToTypes: []string{"post"}},
		},
	}

	err := mg.applyCompiledSchema(g, compiled)
	if err != nil {
		t.Fatalf("applyCompiledSchema failed: %v", err)
	}

	schema := g.Store.GetSchema()
	if schema.EdgeTypes["authored"] == nil {
		t.Error("edge type 'authored' should be defined")
	}
}

func TestApplyCompiledSchemaWithAllPropertyTypes(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	compiled := &CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "thing", Properties: map[string]CompiledProperty{
				"s":       {Type: "string", Required: true},
				"n":       {Type: "number", Required: false},
				"b":       {Type: "boolean", Required: false},
				"a":       {Type: "array", Required: false},
				"o":       {Type: "object", Required: false},
				"r":       {Type: "ref", Required: false},
				"x":       {Type: "any", Required: false},
				"unknown": {Type: "totally_unknown_type", Required: false}, // falls back to PropAny
			}},
		},
	}

	err := mg.applyCompiledSchema(g, compiled)
	if err != nil {
		t.Fatalf("applyCompiledSchema failed: %v", err)
	}

	schema := g.Store.GetSchema()
	nd := schema.NodeTypes["thing"]
	if nd == nil {
		t.Fatal("thing node type not found")
	}
	if nd.Properties["unknown"].Type != graph.PropAny {
		t.Errorf("unknown prop type should map to PropAny, got %s", nd.Properties["unknown"].Type)
	}
}

func TestApplyCompiledSchemaWithPipelines(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	compiled := &CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "item", Properties: map[string]CompiledProperty{
				"name": {Type: "string", Required: true},
			}},
		},
		Pipelines: []CompiledPipeline{
			{
				ID:   "p1",
				Name: "Map Pipeline",
				Stages: []CompiledStage{
					{
						Type:        "map",
						SourceType:  "item",
						DerivedType: "d_item",
						Transform: &derived.SerializableTransform{
							PropertyMap: map[string]string{"name": "name"},
						},
					},
				},
			},
			{
				ID:   "p2",
				Name: "Join Pipeline",
				Stages: []CompiledStage{
					{
						Type:        "join",
						SourceType:  "item",
						DerivedType: "d_joined",
					},
				},
			},
			{
				ID:   "p3",
				Name: "Computed Pipeline",
				Stages: []CompiledStage{
					{
						Type:        "computed",
						SourceType:  "item",
						DerivedType: "d_computed",
					},
				},
			},
		},
	}

	err := mg.applyCompiledSchema(g, compiled)
	if err != nil {
		t.Fatalf("applyCompiledSchema with pipelines failed: %v", err)
	}
}

func TestApplyCompiledSchemaWithInvariants(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	compiled := &CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{
				"email": {Type: "string", Required: true},
			}},
		},
		Invariants: []CompiledInvariant{
			{
				ID:     "uniq-1",
				Type:   "uniqueness",
				Config: json.RawMessage(`{"nodeType":"user","property":"email"}`),
			},
		},
	}

	err := mg.applyCompiledSchema(g, compiled)
	if err != nil {
		t.Fatalf("applyCompiledSchema with invariants failed: %v", err)
	}

	if len(g.Validator.List()) == 0 {
		t.Error("invariant should be added")
	}
}

func TestApplyCompiledSchemaInvariantError(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	compiled := &CompiledSchema{
		Invariants: []CompiledInvariant{
			{
				ID:     "bad-1",
				Type:   "unknown_type",
				Config: json.RawMessage(`{}`),
			},
		},
	}

	err := mg.applyCompiledSchema(g, compiled)
	if err == nil {
		t.Error("expected error for unknown invariant type")
	}
}

// ---------------------------------------------------------------------------
// buildInvariant — error paths for each specific type's bad JSON
// ---------------------------------------------------------------------------

func TestBuildInvariantBadJSONForEachType(t *testing.T) {
	badJSON := json.RawMessage(`{invalid`)
	types := []string{"acyclicity", "cardinality", "edge_constraint", "hierarchy_depth", "child_count"}

	for _, typ := range types {
		t.Run(typ, func(t *testing.T) {
			_, err := buildInvariant(CompiledInvariant{
				ID:     "bad",
				Type:   typ,
				Config: badJSON,
			})
			if err == nil {
				t.Errorf("expected error for bad JSON on type %s", typ)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LoadSchema — error paths
// ---------------------------------------------------------------------------

func TestLoadSchemaInvalidJSON(t *testing.T) {
	mg := NewMultiGraph("r1")
	err := mg.LoadSchema("test", []byte(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadSchemaInvariantError(t *testing.T) {
	mg := NewMultiGraph("r1")

	schemaJSON := `{
		"nodeTypes": [],
		"invariants": [{"id": "bad", "type": "unknown_type", "config": {}}]
	}`

	err := mg.LoadSchema("test", json.RawMessage(schemaJSON))
	if err == nil {
		t.Error("expected error for unknown invariant type in LoadSchema")
	}
}

func TestLoadSchemaWithModules(t *testing.T) {
	mg := NewMultiGraph("r1")

	schema := CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "base", Properties: map[string]CompiledProperty{
				"name": {Type: "string", Required: true},
			}},
		},
		Modules: []CompiledModule{
			{
				ID:   "mod1",
				Name: "Module 1",
				NodeTypes: []CompiledNodeType{
					{Name: "extra", Properties: map[string]CompiledProperty{
						"label": {Type: "string", Required: true},
					}},
				},
			},
		},
	}

	schemaJSON, _ := json.Marshal(schema)
	err := mg.LoadSchema("modtest", schemaJSON)
	if err != nil {
		t.Fatalf("LoadSchema with modules failed: %v", err)
	}
}

func TestLoadSchemaModuleResolutionError(t *testing.T) {
	mg := NewMultiGraph("r1")

	// Modules with a cycle cause ResolveModules to fail
	schema := CompiledSchema{
		NodeTypes: []CompiledNodeType{},
		Modules: []CompiledModule{
			{ID: "a", Name: "A", DependsOn: []string{"b"}},
			{ID: "b", Name: "B", DependsOn: []string{"a"}},
		},
	}

	schemaJSON, _ := json.Marshal(schema)
	err := mg.LoadSchema("cycletest", schemaJSON)
	if err == nil {
		t.Error("expected error for module circular dependency")
	}
}

// ---------------------------------------------------------------------------
// ValidateSchemaCompatibility — edge removal and narrowing paths
// ---------------------------------------------------------------------------

func TestValidateSchemaCompatibilityRemoveEdgeType(t *testing.T) {
	mg := NewMultiGraph("r1")

	// Deploy initial schema with edge type
	schema1 := CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{
				"name": {Type: "string", Required: true},
			}},
		},
		EdgeTypes: []CompiledEdgeType{
			{Name: "follows", FromTypes: []string{"user"}, ToTypes: []string{"user"}},
		},
	}
	schemaJSON1, _ := json.Marshal(schema1)
	if err := mg.DeploySchema("edge-test", schemaJSON1); err != nil {
		t.Fatalf("initial deploy failed: %v", err)
	}

	g, _ := mg.Get("edge-test")

	// Insert two users and an edge between them
	id1, _ := g.Store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	id2, _ := g.Store.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})
	g.Store.InsertEdge("follows", id1, id2, nil)

	// Try removing the edge type — should fail because edges exist
	schema2 := CompiledSchema{
		NodeTypes: schema1.NodeTypes,
		EdgeTypes: []CompiledEdgeType{}, // removed "follows"
	}
	schemaJSON2, _ := json.Marshal(schema2)
	err := mg.ValidateSchemaCompatibility("edge-test", schemaJSON2)
	if err == nil {
		t.Error("removing edge type with existing edges should fail")
	}
}

func TestValidateSchemaCompatibilityRemoveEdgeTypeNoEdges(t *testing.T) {
	mg := NewMultiGraph("r1")

	schema1 := CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{
				"name": {Type: "string", Required: true},
			}},
		},
		EdgeTypes: []CompiledEdgeType{
			{Name: "follows", FromTypes: []string{"user"}, ToTypes: []string{"user"}},
		},
	}
	schemaJSON1, _ := json.Marshal(schema1)
	mg.DeploySchema("edge-test2", schemaJSON1)

	// Insert nodes but no edges
	g, _ := mg.Get("edge-test2")
	g.Store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})

	// Remove edge type — should succeed because no edges exist
	schema2 := CompiledSchema{
		NodeTypes: schema1.NodeTypes,
		EdgeTypes: []CompiledEdgeType{},
	}
	schemaJSON2, _ := json.Marshal(schema2)
	err := mg.ValidateSchemaCompatibility("edge-test2", schemaJSON2)
	if err != nil {
		t.Errorf("removing unused edge type should succeed: %v", err)
	}
}

func TestValidateSchemaCompatibilityNarrowEdgeConstraints(t *testing.T) {
	mg := NewMultiGraph("r1")

	// Initial schema with unconstrained edge (empty fromTypes/toTypes)
	schema1 := CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{"name": {Type: "string"}}},
			{Name: "post", Properties: map[string]CompiledProperty{"title": {Type: "string"}}},
		},
		EdgeTypes: []CompiledEdgeType{
			{Name: "likes", FromTypes: []string{}, ToTypes: []string{}}, // unconstrained
		},
	}
	schemaJSON1, _ := json.Marshal(schema1)
	mg.DeploySchema("narrow-test", schemaJSON1)

	g, _ := mg.Get("narrow-test")
	id1, _ := g.Store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	id2, _ := g.Store.InsertNode("post", nil, map[string]interface{}{"title": "Hello"})
	g.Store.InsertEdge("likes", id1, id2, nil)

	// Narrow to only user->user (existing user->post edge violates toTypes)
	schema2 := CompiledSchema{
		NodeTypes: schema1.NodeTypes,
		EdgeTypes: []CompiledEdgeType{
			{Name: "likes", FromTypes: []string{"user"}, ToTypes: []string{"user"}},
		},
	}
	schemaJSON2, _ := json.Marshal(schema2)
	err := mg.ValidateSchemaCompatibility("narrow-test", schemaJSON2)
	if err == nil {
		t.Error("narrowing edge constraints to exclude existing edges should fail")
	}
}

func TestValidateSchemaCompatibilityNarrowFromTypes(t *testing.T) {
	mg := NewMultiGraph("r1")

	schema1 := CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{"name": {Type: "string"}}},
			{Name: "bot", Properties: map[string]CompiledProperty{"name": {Type: "string"}}},
		},
		EdgeTypes: []CompiledEdgeType{
			{Name: "likes", FromTypes: []string{"user", "bot"}, ToTypes: []string{"user", "bot"}},
		},
	}
	schemaJSON1, _ := json.Marshal(schema1)
	mg.DeploySchema("narrow-from", schemaJSON1)

	g, _ := mg.Get("narrow-from")
	id1, _ := g.Store.InsertNode("bot", nil, map[string]interface{}{"name": "Bot1"})
	id2, _ := g.Store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	g.Store.InsertEdge("likes", id1, id2, nil)

	// Narrow from types to only "user" — existing "bot" edge violates
	schema2 := CompiledSchema{
		NodeTypes: schema1.NodeTypes,
		EdgeTypes: []CompiledEdgeType{
			{Name: "likes", FromTypes: []string{"user"}, ToTypes: []string{"user", "bot"}},
		},
	}
	schemaJSON2, _ := json.Marshal(schema2)
	err := mg.ValidateSchemaCompatibility("narrow-from", schemaJSON2)
	if err == nil {
		t.Error("narrowing fromTypes to exclude existing edge source should fail")
	}
}

func TestValidateSchemaCompatibilityEdgeConstraintOK(t *testing.T) {
	mg := NewMultiGraph("r1")

	schema1 := CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{"name": {Type: "string"}}},
		},
		EdgeTypes: []CompiledEdgeType{
			{Name: "follows", FromTypes: []string{"user"}, ToTypes: []string{"user"}},
		},
	}
	schemaJSON1, _ := json.Marshal(schema1)
	mg.DeploySchema("ok-test", schemaJSON1)

	g, _ := mg.Get("ok-test")
	id1, _ := g.Store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	id2, _ := g.Store.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})
	g.Store.InsertEdge("follows", id1, id2, nil)

	// Same constraints — should pass
	schema2 := CompiledSchema{
		NodeTypes: schema1.NodeTypes,
		EdgeTypes: []CompiledEdgeType{
			{Name: "follows", FromTypes: []string{"user"}, ToTypes: []string{"user"}},
		},
	}
	schemaJSON2, _ := json.Marshal(schema2)
	err := mg.ValidateSchemaCompatibility("ok-test", schemaJSON2)
	if err != nil {
		t.Errorf("same edge constraints should pass: %v", err)
	}
}

func TestValidateSchemaCompatibilityNilOldSchema(t *testing.T) {
	mg := NewMultiGraph("r1")

	// Create a graph but don't deploy any schema to it
	g := mg.GetOrCreate("nil-schema")

	// Explicitly set schema to nil to exercise the oldSchema==nil branch
	g.Store.SetSchema(nil)

	// Any new schema should be compatible
	schema := CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{"name": {Type: "string"}}},
		},
	}
	schemaJSON, _ := json.Marshal(schema)
	err := mg.ValidateSchemaCompatibility("nil-schema", schemaJSON)
	if err != nil {
		t.Errorf("nil old schema should accept any new schema: %v", err)
	}
}

// ---------------------------------------------------------------------------
// checkEdgeConstraintNarrowing — direct tests
// ---------------------------------------------------------------------------

func TestCheckEdgeConstraintNarrowingNoEdges(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	// No edges — should succeed
	err := mg.checkEdgeConstraintNarrowing(g, "follows", []string{"user"}, []string{"user"})
	if err != nil {
		t.Errorf("no edges should pass: %v", err)
	}
}

func TestCheckEdgeConstraintNarrowingDifferentEdgeType(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	id1, _ := g.Store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	id2, _ := g.Store.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})
	g.Store.InsertEdge("blocks", id1, id2, nil)

	// Checking "follows" constraints — "blocks" edge should be skipped
	err := mg.checkEdgeConstraintNarrowing(g, "follows", []string{"user"}, []string{"user"})
	if err != nil {
		t.Errorf("different edge type should be skipped: %v", err)
	}
}

func TestCheckEdgeConstraintNarrowingFromViolation(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	id1, _ := g.Store.InsertNode("bot", nil, map[string]interface{}{})
	id2, _ := g.Store.InsertNode("user", nil, map[string]interface{}{})
	g.Store.InsertEdge("follows", id1, id2, nil)

	// "bot" not in allowedFrom
	err := mg.checkEdgeConstraintNarrowing(g, "follows", []string{"user"}, []string{"user"})
	if err == nil {
		t.Error("expected from-type violation")
	}
}

func TestCheckEdgeConstraintNarrowingToViolation(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	id1, _ := g.Store.InsertNode("user", nil, map[string]interface{}{})
	id2, _ := g.Store.InsertNode("bot", nil, map[string]interface{}{})
	g.Store.InsertEdge("follows", id1, id2, nil)

	// "bot" not in allowedTo
	err := mg.checkEdgeConstraintNarrowing(g, "follows", []string{"user"}, []string{"user"})
	if err == nil {
		t.Error("expected to-type violation")
	}
}

func TestCheckEdgeConstraintNarrowingEmptyFrom(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	id1, _ := g.Store.InsertNode("user", nil, map[string]interface{}{})
	id2, _ := g.Store.InsertNode("bot", nil, map[string]interface{}{})
	g.Store.InsertEdge("follows", id1, id2, nil)

	// Empty allowedFrom means no from-type check, but toTypes is restricted
	err := mg.checkEdgeConstraintNarrowing(g, "follows", []string{}, []string{"user"})
	if err == nil {
		t.Error("expected to-type violation even with empty fromTypes")
	}
}

// ---------------------------------------------------------------------------
// findEdgeType — not found path
// ---------------------------------------------------------------------------

func TestFindEdgeTypeNotFound(t *testing.T) {
	result := findEdgeType([]CompiledEdgeType{
		{Name: "follows"},
		{Name: "likes"},
	}, "nonexistent")
	if result != nil {
		t.Error("expected nil for non-existent edge type")
	}
}

func TestFindEdgeTypeFound(t *testing.T) {
	result := findEdgeType([]CompiledEdgeType{
		{Name: "follows"},
		{Name: "likes"},
	}, "likes")
	if result == nil {
		t.Error("expected to find 'likes'")
	}
}

// ---------------------------------------------------------------------------
// ApplyModule — dependency satisfied path
// ---------------------------------------------------------------------------

func TestApplyModuleWithSatisfiedDependency(t *testing.T) {
	mg := NewMultiGraph("r1")
	g := mg.GetOrCreate("test")

	// Apply first module
	mod1 := &CompiledModule{
		ID:   "base",
		Name: "Base Module",
		NodeTypes: []CompiledNodeType{
			{Name: "item", Properties: map[string]CompiledProperty{"name": {Type: "string"}}},
		},
	}
	if err := mg.ApplyModule("test", mod1); err != nil {
		t.Fatalf("apply base module failed: %v", err)
	}

	// Apply dependent module
	mod2 := &CompiledModule{
		ID:        "ext",
		Name:      "Extension Module",
		DependsOn: []string{"base"},
		NodeTypes: []CompiledNodeType{
			{Name: "extended_item", Properties: map[string]CompiledProperty{"label": {Type: "string"}}},
		},
	}
	if err := mg.ApplyModule("test", mod2); err != nil {
		t.Fatalf("apply dependent module failed: %v", err)
	}

	if len(g.ActiveModules) != 2 {
		t.Errorf("expected 2 active modules, got %d", len(g.ActiveModules))
	}
}

func TestApplyModuleSchemaError(t *testing.T) {
	mg := NewMultiGraph("r1")
	mg.GetOrCreate("test")

	// Module with a bad invariant that will cause applyCompiledSchema to fail
	mod := &CompiledModule{
		ID:   "bad",
		Name: "Bad Module",
		Invariants: []CompiledInvariant{
			{ID: "inv-bad", Type: "unknown_type", Config: json.RawMessage(`{}`)},
		},
	}
	err := mg.ApplyModule("test", mod)
	if err == nil {
		t.Error("expected error from bad invariant in module")
	}
}

// ---------------------------------------------------------------------------
// ResolveModules — empty modules + applyCompiledSchema error path
// ---------------------------------------------------------------------------

func TestResolveModulesEmpty(t *testing.T) {
	mg := NewMultiGraph("r1")
	err := mg.ResolveModules("test", nil)
	if err != nil {
		t.Errorf("empty modules should not error: %v", err)
	}
}

func TestResolveModulesSchemaError(t *testing.T) {
	mg := NewMultiGraph("r1")
	mg.GetOrCreate("test")

	modules := []CompiledModule{
		{
			ID:   "bad",
			Name: "Bad Module",
			Invariants: []CompiledInvariant{
				{ID: "inv-bad", Type: "unknown_type", Config: json.RawMessage(`{}`)},
			},
		},
	}
	err := mg.ResolveModules("test", modules)
	if err == nil {
		t.Error("expected error from bad invariant during ResolveModules")
	}
}

// ---------------------------------------------------------------------------
// topologicalSortModules — unknown dependency
// ---------------------------------------------------------------------------

func TestTopologicalSortUnknownDep(t *testing.T) {
	modules := []CompiledModule{
		{ID: "a", Name: "A", DependsOn: []string{"external-not-in-batch"}},
	}
	sorted, err := topologicalSortModules(modules)
	if err != nil {
		t.Errorf("external dep should be tolerated: %v", err)
	}
	if len(sorted) != 1 {
		t.Errorf("expected 1 sorted module, got %d", len(sorted))
	}
}

// ---------------------------------------------------------------------------
// corsMiddleware — already covered but let's ensure the handler chain works
// ---------------------------------------------------------------------------

func TestCorsMiddlewarePassthrough(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	})

	handler := corsMiddleware(inner)

	// POST request should pass through to inner handler
	req, _ := http.NewRequest("POST", "/api/data", nil)
	w := &fakeResponseWriter{headers: http.Header{}}
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("inner handler should have been called")
	}
}

type fakeResponseWriter struct {
	headers    http.Header
	statusCode int
}

func (f *fakeResponseWriter) Header() http.Header         { return f.headers }
func (f *fakeResponseWriter) Write(b []byte) (int, error)  { return len(b), nil }
func (f *fakeResponseWriter) WriteHeader(code int)          { f.statusCode = code }

// ---------------------------------------------------------------------------
// Start — exercise cluster endpoints
// ---------------------------------------------------------------------------

func TestStartAndQueryClusterEndpoints(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	sys := New("cluster-test")

	errCh := make(chan error, 1)
	go func() {
		errCh <- sys.Start(addr)
	}()

	time.Sleep(200 * time.Millisecond)

	// Query cluster endpoints to exercise the handler closures
	resp, err := http.Get("http://" + addr + "/api/cluster/peers")
	if err == nil {
		resp.Body.Close()
	}
	resp, err = http.Get("http://" + addr + "/api/cluster/shards")
	if err == nil {
		resp.Body.Close()
	}

	sys.Stop()
}

// ---------------------------------------------------------------------------
// Indexed on nonexistent property
// ---------------------------------------------------------------------------

func TestIndexedNonexistentProperty(t *testing.T) {
	sys := New("r1")
	sys.Schema(func(sb *SchemaBuilder) {
		sb.Node("item", func(nb *NodeBuilder) {
			nb.String("name", true)
			nb.Indexed("nonexistent") // should be a no-op, not panic
		})
	})
}

// ---------------------------------------------------------------------------
// DeploySchema with incompatible schema
// ---------------------------------------------------------------------------

func TestDeploySchemaIncompatible(t *testing.T) {
	mg := NewMultiGraph("r1")

	schema1 := CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{
				"name": {Type: "string", Required: true},
			}},
		},
	}
	schemaJSON1, _ := json.Marshal(schema1)
	mg.DeploySchema("compat", schemaJSON1)

	// Insert data
	g, _ := mg.Get("compat")
	g.Store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})

	// Try deploying a schema that removes required property
	schema2 := CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{}},
		},
	}
	schemaJSON2, _ := json.Marshal(schema2)
	err := mg.DeploySchema("compat", schemaJSON2)
	if err == nil {
		t.Error("should fail for incompatible schema")
	}
}

// ---------------------------------------------------------------------------
// System Action registration
// ---------------------------------------------------------------------------

func TestActionRegistration(t *testing.T) {
	sys := New("r1")
	sys.Action("doSomething", func(ctx context.Context, a *function.ActionCtx, args map[string]interface{}) (interface{}, error) {
		return "done", nil
	})

	fn, ok := sys.Registry().Get("doSomething")
	if !ok {
		t.Fatal("action should be registered")
	}
	if fn.Type != function.FuncAction {
		t.Error("should be action type")
	}
}

// ---------------------------------------------------------------------------
// Builtin deleteEdge
// ---------------------------------------------------------------------------

func TestBuiltinDeleteEdge(t *testing.T) {
	sys := New("r1")
	registerSystemBuiltins(sys)

	ctx := context.Background()
	r := sys.Registry().Call(ctx, "graphdb:insertNode", map[string]interface{}{"type": "user"})
	id1 := r.Value.(string)
	r = sys.Registry().Call(ctx, "graphdb:insertNode", map[string]interface{}{"type": "user"})
	id2 := r.Value.(string)

	r = sys.Registry().Call(ctx, "graphdb:insertEdge", map[string]interface{}{
		"type": "follows", "from": id1, "to": id2,
	})
	edgeID := r.Value.(string)

	r = sys.Registry().Call(ctx, "graphdb:deleteEdge", map[string]interface{}{"id": edgeID})
	if r.Error != "" {
		t.Fatalf("deleteEdge failed: %s", r.Error)
	}
}

// ---------------------------------------------------------------------------
// ValidateSchemaCompatibility — remove node type with zero existing nodes
// ---------------------------------------------------------------------------

func TestValidateSchemaCompatibilityRemoveEmptyNodeType(t *testing.T) {
	mg := NewMultiGraph("r1")

	schema1 := CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{"name": {Type: "string", Required: true}}},
			{Name: "orphan_type", Properties: map[string]CompiledProperty{"x": {Type: "string"}}},
		},
	}
	schemaJSON1, _ := json.Marshal(schema1)
	mg.DeploySchema("rm-empty", schemaJSON1)

	// Insert only "user" nodes, not "orphan_type"
	g, _ := mg.Get("rm-empty")
	g.Store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})

	// Remove orphan_type — should succeed (0 nodes)
	schema2 := CompiledSchema{
		NodeTypes: []CompiledNodeType{
			{Name: "user", Properties: map[string]CompiledProperty{"name": {Type: "string", Required: true}}},
		},
	}
	schemaJSON2, _ := json.Marshal(schema2)
	err := mg.ValidateSchemaCompatibility("rm-empty", schemaJSON2)
	if err != nil {
		t.Errorf("removing node type with no existing nodes should succeed: %v", err)
	}
}
