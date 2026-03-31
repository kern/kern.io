package function

import (
	"context"
	"fmt"
	"testing"

	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
)

// ---------------------------------------------------------------------------
// registry.go: Call — unknown function type branch
// ---------------------------------------------------------------------------

func TestCallUnknownFuncType(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	// Manually inject a function with unknown type
	reg.mu.Lock()
	reg.functions["test:unknown"] = &FuncDef{
		Name: "test:unknown",
		Type: FuncType("bogus"),
	}
	reg.mu.Unlock()

	result := reg.Call(context.Background(), "test:unknown", nil)
	if result.Error == "" {
		t.Error("expected error for unknown function type")
	}
}

// ---------------------------------------------------------------------------
// registry.go: Call — mutation with validator that detects violation
// ---------------------------------------------------------------------------

func TestCallMutationInvariantViolation(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()

	// Add a uniqueness invariant
	validator.Add(invariant.NewUniquenessInvariant("unique-email", invariant.UniquenessConfig{
		NodeType: "user",
		Property: "email",
	}))

	reg := NewRegistry(store, validator)

	// Pre-insert a user
	store.InsertNode("user", nil, map[string]interface{}{"email": "dup@test.com"})

	reg.RegisterMutation("test:createDup", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		return mctx.InsertNode("user", nil, map[string]interface{}{"email": "dup@test.com"})
	})

	result := reg.Call(context.Background(), "test:createDup", nil)
	if result.Error == "" {
		t.Error("expected invariant violation error")
	}
}

// ---------------------------------------------------------------------------
// registry.go: Call — mutation with nil validator (no invariant check)
// ---------------------------------------------------------------------------

func TestCallMutationNilValidator(t *testing.T) {
	store := graph.NewStore("r1")
	reg := NewRegistry(store, nil) // nil validator

	reg.RegisterMutation("test:create", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		return mctx.InsertNode("item", nil, nil)
	})

	result := reg.Call(context.Background(), "test:create", nil)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

// ---------------------------------------------------------------------------
// registry.go: GetParent — valid and nil result
// ---------------------------------------------------------------------------

func TestQueryCtxGetParentNotFound(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	id, _ := store.InsertNode("item", nil, nil)

	reg.RegisterQuery("test:getParent", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		parent, err := qctx.GetParent(args["id"].(string))
		if err != nil {
			return nil, err
		}
		return parent, nil
	})

	result := reg.Call(context.Background(), "test:getParent", map[string]interface{}{"id": id.String()})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Value != nil {
		t.Errorf("expected nil parent for root node, got %v", result.Value)
	}
}

func TestQueryCtxGetParentFound(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	parent, _ := store.InsertNode("folder", nil, nil)
	child, _ := store.InsertNode("file", &parent, nil)

	reg.RegisterQuery("test:getParent", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return qctx.GetParent(args["id"].(string))
	})

	result := reg.Call(context.Background(), "test:getParent", map[string]interface{}{"id": child.String()})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Value == nil {
		t.Error("expected non-nil parent")
	}
}

// ---------------------------------------------------------------------------
// registry.go: InsertNode error path
// ---------------------------------------------------------------------------

func TestMutationCtxInsertNodeError(t *testing.T) {
	store := graph.NewStore("r1")
	schema := graph.NewSchema()
	schema.DefineNode(&graph.NodeTypeDef{
		Name: "user",
		Properties: map[string]*graph.PropertyDef{
			"name": {Name: "name", Type: graph.PropString, Required: true},
		},
	})
	store.SetSchema(schema)

	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterMutation("test:create", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		// Missing required property
		return mctx.InsertNode("user", nil, map[string]interface{}{})
	})

	result := reg.Call(context.Background(), "test:create", nil)
	if result.Error == "" {
		t.Error("expected error for missing required property")
	}
}

// ---------------------------------------------------------------------------
// registry.go: InsertEdge error paths
// ---------------------------------------------------------------------------

func TestMutationCtxInsertEdgeStoreError(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	id, _ := store.InsertNode("a", nil, nil)

	reg.RegisterMutation("test:edge", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		// Target node doesn't exist
		return mctx.InsertEdge("link", id.String(), "00000000-0000-0000-0000-000000000001", nil)
	})

	result := reg.Call(context.Background(), "test:edge", nil)
	if result.Error == "" {
		t.Error("expected error for missing target node")
	}
}

// ---------------------------------------------------------------------------
// registry.go: Action calling other functions — error propagation
// ---------------------------------------------------------------------------

func TestActionRunQueryError(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterQuery("test:failq", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, fmt.Errorf("query failure")
	})

	reg.RegisterAction("test:act", func(ctx context.Context, actx *ActionCtx, args map[string]interface{}) (interface{}, error) {
		r := actx.RunQuery(ctx, "test:failq", nil)
		if r.Error != "" {
			return nil, fmt.Errorf("query failed: %s", r.Error)
		}
		return nil, nil
	})

	result := reg.Call(context.Background(), "test:act", nil)
	if result.Error == "" {
		t.Error("expected action to propagate query error")
	}
}

func TestActionRunMutationError(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterMutation("test:failm", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, fmt.Errorf("mutation failure")
	})

	reg.RegisterAction("test:act", func(ctx context.Context, actx *ActionCtx, args map[string]interface{}) (interface{}, error) {
		r := actx.RunMutation(ctx, "test:failm", nil)
		if r.Error != "" {
			return nil, fmt.Errorf("mutation failed: %s", r.Error)
		}
		return nil, nil
	})

	result := reg.Call(context.Background(), "test:act", nil)
	if result.Error == "" {
		t.Error("expected action to propagate mutation error")
	}
}
