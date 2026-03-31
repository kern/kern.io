package function

import (
	"context"
	"testing"

	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterQuery("test:getUser", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return "user", nil
	})

	fn, ok := reg.Get("test:getUser")
	if !ok {
		t.Fatal("registered function should be found")
	}
	if fn.Type != FuncQuery {
		t.Error("should be a query")
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("non-existent function should not be found")
	}
}

func TestRegistryList(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterQuery("q1", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, nil
	})
	reg.RegisterMutation("m1", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, nil
	})

	fns := reg.List()
	if len(fns) != 2 {
		t.Errorf("expected 2 functions, got %d", len(fns))
	}
}

func TestRegistryCallQuery(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterQuery("test:echo", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return args["msg"], nil
	})

	result := reg.Call(context.Background(), "test:echo", map[string]interface{}{"msg": "hello"})
	if result.Error != "" {
		t.Fatalf("call failed: %s", result.Error)
	}
	if result.Value != "hello" {
		t.Errorf("expected 'hello', got %v", result.Value)
	}
}

func TestRegistryCallMutation(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterMutation("test:create", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		return mctx.InsertNode("item", nil, map[string]interface{}{"name": args["name"]})
	})

	result := reg.Call(context.Background(), "test:create", map[string]interface{}{"name": "test"})
	if result.Error != "" {
		t.Fatalf("call failed: %s", result.Error)
	}
	if result.Value == nil {
		t.Error("should return node ID")
	}
}

func TestRegistryCallNonExistent(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	result := reg.Call(context.Background(), "nonexistent", nil)
	if result.Error == "" {
		t.Error("calling non-existent function should return error")
	}
}

func TestQueryCtxOperations(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	// Insert test data
	parentID, _ := store.InsertNode("folder", nil, map[string]interface{}{"name": "root"})
	childID, _ := store.InsertNode("file", &parentID, map[string]interface{}{"name": "child"})
	id1, _ := store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	id2, _ := store.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})
	store.InsertEdge("follows", id1, id2, nil)

	// Test through query function
	reg.RegisterQuery("test:all", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		// GetNode
		node, err := qctx.GetNode(parentID.String())
		if err != nil || node == nil {
			return nil, err
		}

		// GetChildren
		children, err := qctx.GetChildren(parentID.String())
		if err != nil || children == nil {
			return "GetChildren failed", nil
		}

		// GetParent
		parent, err := qctx.GetParent(childID.String())
		if err != nil || parent == nil {
			return "GetParent failed", nil
		}

		// GetSubtree
		subtree, err := qctx.GetSubtree(parentID.String())
		if err != nil || subtree == nil {
			return "GetSubtree failed", nil
		}

		// GetAncestors
		ancestors, err := qctx.GetAncestors(childID.String())
		if err != nil || ancestors == nil {
			return "GetAncestors failed", nil
		}

		// GetNodesByType
		users := qctx.GetNodesByType("user")
		if users == nil {
			return "GetNodesByType failed", nil
		}

		// GetRoots
		roots := qctx.GetRoots()
		if roots == nil {
			return "GetRoots failed", nil
		}

		// GetOutEdges
		edges, err := qctx.GetOutEdges(id1.String())
		if err != nil {
			return "GetOutEdges failed", nil
		}
		_ = edges

		// GetInEdges
		inEdges, err := qctx.GetInEdges(id2.String())
		if err != nil {
			return "GetInEdges failed", nil
		}
		_ = inEdges

		// Stats
		stats := qctx.Stats()
		if stats == nil {
			return "Stats failed", nil
		}

		// GetOrderedChildren
		ordered, err := qctx.GetOrderedChildren(parentID.String())
		if err != nil {
			return "GetOrderedChildren failed", nil
		}
		_ = ordered

		// GetDeletedNodes
		deleted := qctx.GetDeletedNodes()
		_ = deleted

		return "all passed", nil
	})

	result := reg.Call(context.Background(), "test:all", nil)
	if result.Error != "" {
		t.Fatalf("call failed: %s", result.Error)
	}
	if result.Value != "all passed" {
		t.Errorf("expected 'all passed', got %v", result.Value)
	}
}

func TestMutationCtxOperations(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterMutation("test:mutate", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		// InsertNode
		id, err := mctx.InsertNode("item", nil, map[string]interface{}{"name": "test"})
		if err != nil {
			return nil, err
		}

		// PatchNode
		if err = mctx.PatchNode(id, map[string]interface{}{"name": "updated"}); err != nil {
			return nil, err
		}

		// SetProperty
		if err = mctx.SetProperty(id, "color", "blue"); err != nil {
			return nil, err
		}

		// DeleteProperty
		if err = mctx.DeleteProperty(id, "color"); err != nil {
			return nil, err
		}

		// InsertNode for edge targets
		id2, err := mctx.InsertNode("item", nil, nil)
		if err != nil {
			return nil, err
		}

		// InsertEdge
		if _, err = mctx.InsertEdge("link", id, id2, nil); err != nil {
			return nil, err
		}

		// MoveNode
		if err = mctx.MoveNode(id2, nil); err != nil {
			return nil, err
		}

		// SoftDeleteNode
		id3, _ := mctx.InsertNode("temp", nil, nil)
		if err = mctx.SoftDeleteNode(id3); err != nil {
			return nil, err
		}

		// RestoreNode
		if err = mctx.RestoreNode(id3); err != nil {
			return nil, err
		}

		// CascadeDeleteNode
		if err = mctx.CascadeDeleteNode(id3); err != nil {
			return nil, err
		}

		// DeleteNode
		if err = mctx.DeleteNode(id2); err != nil {
			return nil, err
		}

		return "mutations passed", nil
	})

	result := reg.Call(context.Background(), "test:mutate", nil)
	if result.Error != "" {
		t.Fatalf("mutations failed: %s", result.Error)
	}
	if result.Value != "mutations passed" {
		t.Errorf("expected 'mutations passed', got %v", result.Value)
	}
}

func TestRegistryCallDuration(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterQuery("test:fast", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, nil
	})

	result := reg.Call(context.Background(), "test:fast", nil)
	if result.Duration <= 0 {
		t.Error("duration should be positive")
	}
}
