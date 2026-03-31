package function

import (
	"context"
	"fmt"
	"testing"

	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
)

func TestRegistryCallAction(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterAction("test:act", func(ctx context.Context, actx *ActionCtx, args map[string]interface{}) (interface{}, error) {
		return "action-result", nil
	})

	result := reg.Call(context.Background(), "test:act", nil)
	if result.Error != "" {
		t.Fatalf("call failed: %s", result.Error)
	}
	if result.Value != "action-result" {
		t.Errorf("expected 'action-result', got %v", result.Value)
	}
}

func TestRegistryCallQueryError(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterQuery("test:fail", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, fmt.Errorf("query error")
	})

	result := reg.Call(context.Background(), "test:fail", nil)
	if result.Error == "" {
		t.Error("should have error")
	}
}

func TestRegistryCallMutationError(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterMutation("test:fail", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, fmt.Errorf("mutation error")
	})

	result := reg.Call(context.Background(), "test:fail", nil)
	if result.Error == "" {
		t.Error("should have error")
	}
}

func TestRegistryCallActionError(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterAction("test:fail", func(ctx context.Context, actx *ActionCtx, args map[string]interface{}) (interface{}, error) {
		return nil, fmt.Errorf("action error")
	})

	result := reg.Call(context.Background(), "test:fail", nil)
	if result.Error == "" {
		t.Error("should have error")
	}
}

func TestQueryCtxTraverse(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	id1, _ := store.InsertNode("user", nil, nil)
	id2, _ := store.InsertNode("user", nil, nil)
	store.InsertEdge("follows", id1, id2, nil)

	reg.RegisterQuery("test:traverse", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return qctx.Traverse(args["id"].(string), "follows", "out", 5)
	})

	result := reg.Call(context.Background(), "test:traverse", map[string]interface{}{"id": id1.String()})
	if result.Error != "" {
		t.Fatalf("traverse failed: %s", result.Error)
	}
}

func TestQueryCtxFindByIndex(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	store.InsertNode("user", nil, map[string]interface{}{"email": "alice@test.com"})

	reg.RegisterQuery("test:find", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return qctx.FindByIndex("user", "email", "alice@test.com"), nil
	})

	result := reg.Call(context.Background(), "test:find", nil)
	if result.Error != "" {
		t.Fatalf("find failed: %s", result.Error)
	}
}

func TestQueryCtxAllNodesAndEdges(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	id1, _ := store.InsertNode("a", nil, nil)
	id2, _ := store.InsertNode("b", nil, nil)
	store.InsertEdge("link", id1, id2, nil)

	reg.RegisterQuery("test:all", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		nodes := qctx.AllNodes()
		edges := qctx.AllEdges()
		return map[string]interface{}{"nodes": nodes, "edges": edges}, nil
	})

	result := reg.Call(context.Background(), "test:all", nil)
	if result.Error != "" {
		t.Fatalf("failed: %s", result.Error)
	}
}

func TestMutationCtxDeleteEdge(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	id1, _ := store.InsertNode("a", nil, nil)
	id2, _ := store.InsertNode("b", nil, nil)
	edgeID, _ := store.InsertEdge("link", id1, id2, nil)

	reg.RegisterMutation("test:delEdge", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, mctx.DeleteEdge(args["edgeID"].(string))
	})

	result := reg.Call(context.Background(), "test:delEdge", map[string]interface{}{"edgeID": edgeID.String()})
	if result.Error != "" {
		t.Fatalf("delete edge failed: %s", result.Error)
	}
}

func TestMutationCtxMoveNode(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	parent, _ := store.InsertNode("folder", nil, nil)
	child, _ := store.InsertNode("file", nil, nil)

	reg.RegisterMutation("test:move", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		pid := args["parent"].(string)
		return nil, mctx.MoveNode(args["child"].(string), &pid)
	})

	result := reg.Call(context.Background(), "test:move", map[string]interface{}{
		"child":  child.String(),
		"parent": parent.String(),
	})
	if result.Error != "" {
		t.Fatalf("move failed: %s", result.Error)
	}
}

func TestMutationCtxMoveNodeToRoot(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	parent, _ := store.InsertNode("folder", nil, nil)
	child, _ := store.InsertNode("file", &parent, nil)

	reg.RegisterMutation("test:moveRoot", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, mctx.MoveNode(args["child"].(string), nil)
	})

	result := reg.Call(context.Background(), "test:moveRoot", map[string]interface{}{"child": child.String()})
	if result.Error != "" {
		t.Fatalf("move to root failed: %s", result.Error)
	}
}

func TestMutationCtxGetNodeMethods(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	parent, _ := store.InsertNode("folder", nil, nil)
	child, _ := store.InsertNode("file", &parent, nil)
	id2, _ := store.InsertNode("file", nil, nil)
	store.InsertEdge("link", child, id2, nil)

	reg.RegisterMutation("test:read", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		// GetNode
		_, err := mctx.GetNode(args["child"].(string))
		if err != nil {
			return nil, err
		}

		// GetNodesByType
		nodes := mctx.GetNodesByType("file")
		if nodes == nil {
			return nil, fmt.Errorf("GetNodesByType returned nil")
		}

		// GetChildren
		_, err = mctx.GetChildren(args["parent"].(string))
		if err != nil {
			return nil, err
		}

		// GetOutEdges
		_, err = mctx.GetOutEdges(args["child"].(string))
		if err != nil {
			return nil, err
		}

		// GetInEdges
		_, err = mctx.GetInEdges(args["id2"].(string))
		if err != nil {
			return nil, err
		}

		return "ok", nil
	})

	result := reg.Call(context.Background(), "test:read", map[string]interface{}{
		"parent": parent.String(),
		"child":  child.String(),
		"id2":    id2.String(),
	})
	if result.Error != "" {
		t.Fatalf("read methods failed: %s", result.Error)
	}
}

func TestMutationCtxInsertNodeWithParent(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	parent, _ := store.InsertNode("folder", nil, nil)

	reg.RegisterMutation("test:insertChild", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		pid := args["parent"].(string)
		return mctx.InsertNode("file", &pid, nil)
	})

	result := reg.Call(context.Background(), "test:insertChild", map[string]interface{}{
		"parent": parent.String(),
	})
	if result.Error != "" {
		t.Fatalf("insert child failed: %s", result.Error)
	}
}

func TestActionCtxRunQueryAndMutation(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterQuery("test:q", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		return "query-result", nil
	})
	reg.RegisterMutation("test:m", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		return "mutation-result", nil
	})
	reg.RegisterAction("test:orchestrate", func(ctx context.Context, actx *ActionCtx, args map[string]interface{}) (interface{}, error) {
		qResult := actx.RunQuery(ctx, "test:q", nil)
		if qResult.Error != "" {
			return nil, fmt.Errorf("query failed: %s", qResult.Error)
		}
		mResult := actx.RunMutation(ctx, "test:m", nil)
		if mResult.Error != "" {
			return nil, fmt.Errorf("mutation failed: %s", mResult.Error)
		}
		return map[string]interface{}{"q": qResult.Value, "m": mResult.Value}, nil
	})

	result := reg.Call(context.Background(), "test:orchestrate", nil)
	if result.Error != "" {
		t.Fatalf("action orchestration failed: %s", result.Error)
	}
}

func TestQueryCtxInvalidUUID(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterQuery("test:badId", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		_, err := qctx.GetNode("not-a-uuid")
		return nil, err
	})

	result := reg.Call(context.Background(), "test:badId", nil)
	if result.Error == "" {
		t.Error("should fail for bad UUID")
	}
}

func TestMutationCtxInvalidUUIDs(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterMutation("test:badIds", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		// DeleteNode bad UUID
		if err := mctx.DeleteNode("bad"); err == nil {
			return nil, fmt.Errorf("DeleteNode should fail")
		}
		// SetProperty bad UUID
		if err := mctx.SetProperty("bad", "k", "v"); err == nil {
			return nil, fmt.Errorf("SetProperty should fail")
		}
		// PatchNode bad UUID
		if err := mctx.PatchNode("bad", nil); err == nil {
			return nil, fmt.Errorf("PatchNode should fail")
		}
		// DeleteProperty bad UUID
		if err := mctx.DeleteProperty("bad", "k"); err == nil {
			return nil, fmt.Errorf("DeleteProperty should fail")
		}
		// InsertEdge bad from
		if _, err := mctx.InsertEdge("t", "bad", "bad", nil); err == nil {
			return nil, fmt.Errorf("InsertEdge should fail")
		}
		// DeleteEdge bad UUID
		if err := mctx.DeleteEdge("bad"); err == nil {
			return nil, fmt.Errorf("DeleteEdge should fail")
		}
		// MoveNode bad UUID
		if err := mctx.MoveNode("bad", nil); err == nil {
			return nil, fmt.Errorf("MoveNode should fail")
		}
		// RestoreNode bad UUID
		if err := mctx.RestoreNode("bad"); err == nil {
			return nil, fmt.Errorf("RestoreNode should fail")
		}
		// SoftDeleteNode bad UUID
		if err := mctx.SoftDeleteNode("bad"); err == nil {
			return nil, fmt.Errorf("SoftDeleteNode should fail")
		}
		// CascadeDeleteNode bad UUID
		if err := mctx.CascadeDeleteNode("bad"); err == nil {
			return nil, fmt.Errorf("CascadeDeleteNode should fail")
		}
		// InsertNode bad parent UUID
		badPID := "bad"
		if _, err := mctx.InsertNode("t", &badPID, nil); err == nil {
			return nil, fmt.Errorf("InsertNode should fail with bad parent")
		}
		// MoveNode bad parent UUID
		badP := "bad"
		if err := mctx.MoveNode("00000000-0000-0000-0000-000000000000", &badP); err == nil {
			return nil, fmt.Errorf("MoveNode should fail with bad parent")
		}
		// GetNode bad UUID
		if _, err := mctx.GetNode("bad"); err == nil {
			return nil, fmt.Errorf("GetNode should fail")
		}
		// GetChildren bad UUID
		if _, err := mctx.GetChildren("bad"); err == nil {
			return nil, fmt.Errorf("GetChildren should fail")
		}
		// GetOutEdges bad UUID
		if _, err := mctx.GetOutEdges("bad"); err == nil {
			return nil, fmt.Errorf("GetOutEdges should fail")
		}
		// GetInEdges bad UUID
		if _, err := mctx.GetInEdges("bad"); err == nil {
			return nil, fmt.Errorf("GetInEdges should fail")
		}
		return "all bad uuid checks passed", nil
	})

	result := reg.Call(context.Background(), "test:badIds", nil)
	if result.Error != "" {
		t.Fatalf("bad uuid checks failed: %s", result.Error)
	}
}

func TestQueryCtxBadUUIDs(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	reg.RegisterQuery("test:badQueryIds", func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error) {
		if _, err := qctx.GetChildren("bad"); err == nil {
			return nil, fmt.Errorf("GetChildren should fail")
		}
		if _, err := qctx.GetParent("bad"); err == nil {
			return nil, fmt.Errorf("GetParent should fail")
		}
		if _, err := qctx.GetOutEdges("bad"); err == nil {
			return nil, fmt.Errorf("GetOutEdges should fail")
		}
		if _, err := qctx.GetInEdges("bad"); err == nil {
			return nil, fmt.Errorf("GetInEdges should fail")
		}
		if _, err := qctx.GetSubtree("bad"); err == nil {
			return nil, fmt.Errorf("GetSubtree should fail")
		}
		if _, err := qctx.GetAncestors("bad"); err == nil {
			return nil, fmt.Errorf("GetAncestors should fail")
		}
		if _, err := qctx.Traverse("bad", "t", "out", 1); err == nil {
			return nil, fmt.Errorf("Traverse should fail")
		}
		if _, err := qctx.GetOrderedChildren("bad"); err == nil {
			return nil, fmt.Errorf("GetOrderedChildren should fail")
		}
		return "all checks passed", nil
	})

	result := reg.Call(context.Background(), "test:badQueryIds", nil)
	if result.Error != "" {
		t.Fatalf("bad query uuid checks failed: %s", result.Error)
	}
}

func TestInsertEdgeBadToUUID(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := NewRegistry(store, validator)

	id, _ := store.InsertNode("a", nil, nil)

	reg.RegisterMutation("test:badTo", func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error) {
		_, err := mctx.InsertEdge("t", id.String(), "bad-uuid", nil)
		return nil, err
	})

	result := reg.Call(context.Background(), "test:badTo", nil)
	if result.Error == "" {
		t.Error("should fail for bad to UUID")
	}
}
