package function

import (
	"context"
	"fmt"
	"testing"

	"github.com/kern/graphdb/internal/crdt"
	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
)

func setup() (*Registry, *graph.Store) {
	store := graph.NewStore("test-replica")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)
	return reg, store
}

func TestRegisterAndCallQuery(t *testing.T) {
	reg, store := setup()

	// Insert some test data
	store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	store.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})

	reg.RegisterQuery("listUsers", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return qctx.GetNodesByType("user"), nil
	})

	result := reg.Call(context.Background(), "listUsers", nil)
	if result.Error != "" {
		t.Fatalf("query failed: %s", result.Error)
	}

	nodes, ok := result.Value.([]*crdt.MaterializedNode)
	if !ok {
		t.Fatalf("expected []*MaterializedNode, got %T", result.Value)
	}
	if len(nodes) != 2 {
		t.Errorf("expected 2 users, got %d", len(nodes))
	}
}

func TestRegisterAndCallMutation(t *testing.T) {
	reg, _ := setup()

	reg.RegisterMutation("createUser", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		name := args["name"].(string)
		id, err := mctx.InsertNode("user", nil, map[string]interface{}{"name": name})
		return id, err
	})

	result := reg.Call(context.Background(), "createUser", map[string]interface{}{"name": "Charlie"})
	if result.Error != "" {
		t.Fatalf("mutation failed: %s", result.Error)
	}

	// Verify the node was created
	reg.RegisterQuery("getUser", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return qctx.GetNode(args["id"].(string))
	})

	getResult := reg.Call(context.Background(), "getUser", map[string]interface{}{
		"id": result.Value.(string),
	})
	if getResult.Error != "" {
		t.Fatalf("get user failed: %s", getResult.Error)
	}
}

func TestMutationWithInvariantViolation(t *testing.T) {
	store := graph.NewStore("test-replica")
	validator := invariant.NewValidator()

	validator.Add(invariant.NewUniquenessInvariant("unique-email", invariant.UniquenessConfig{
		NodeType: "user",
		Property: "email",
	}))

	reg := NewRegistry(store, validator)

	reg.RegisterMutation("createUser", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		id, err := mctx.InsertNode("user", nil, map[string]interface{}{
			"name":  args["name"],
			"email": args["email"],
		})
		return id, err
	})

	// First user
	result := reg.Call(context.Background(), "createUser", map[string]interface{}{
		"name": "Alice", "email": "alice@example.com",
	})
	if result.Error != "" {
		t.Fatalf("first create failed: %s", result.Error)
	}

	// Duplicate email should fail invariant
	result = reg.Call(context.Background(), "createUser", map[string]interface{}{
		"name": "Bob", "email": "alice@example.com",
	})
	if result.Error == "" {
		t.Error("expected invariant violation for duplicate email")
	}
}

func TestCallNonExistentFunction(t *testing.T) {
	reg, _ := setup()
	result := reg.Call(context.Background(), "nonexistent", nil)
	if result.Error == "" {
		t.Error("expected error for nonexistent function")
	}
}

func TestAction(t *testing.T) {
	reg, _ := setup()

	reg.RegisterMutation("createUser", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		id, err := mctx.InsertNode("user", nil, map[string]interface{}{"name": args["name"]})
		return id, err
	})

	reg.RegisterQuery("listUsers", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return qctx.GetNodesByType("user"), nil
	})

	reg.RegisterAction("seedUsers", func(ctx context.Context, actx *ActionCtx, args map[string]interface{}) (interface{}, error) {
		names := []string{"Alice", "Bob", "Charlie"}
		for _, name := range names {
			r := actx.RunMutation(ctx, "createUser", map[string]interface{}{"name": name})
			if r.Error != "" {
				return nil, fmt.Errorf("failed to create %s: %s", name, r.Error)
			}
		}
		result := actx.RunQuery(ctx, "listUsers", nil)
		return result.Value, nil
	})

	result := reg.Call(context.Background(), "seedUsers", nil)
	if result.Error != "" {
		t.Fatalf("action failed: %s", result.Error)
	}

	nodes, ok := result.Value.([]*crdt.MaterializedNode)
	if !ok {
		t.Fatalf("expected []*MaterializedNode, got %T", result.Value)
	}
	if len(nodes) != 3 {
		t.Errorf("expected 3 users, got %d", len(nodes))
	}
}

func TestListFunctions(t *testing.T) {
	reg, _ := setup()

	reg.RegisterQuery("q1", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) { return nil, nil })
	reg.RegisterMutation("m1", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) { return nil, nil })
	reg.RegisterAction("a1", func(ctx context.Context, actx *ActionCtx, args map[string]interface{}) (interface{}, error) { return nil, nil })

	fns := reg.List()
	if len(fns) != 3 {
		t.Errorf("expected 3 functions, got %d", len(fns))
	}
}

