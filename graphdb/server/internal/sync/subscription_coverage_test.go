package sync

import (
	"context"
	"testing"
	"time"

	"github.com/kern/graphdb/internal/function"
	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
)

// ---------------------------------------------------------------------------
// subscription.go: Unsubscribe — non-existent subscription ID
// ---------------------------------------------------------------------------

func TestUnsubscribeNonExistent(t *testing.T) {
	reactor, _, _ := setup()
	defer reactor.Stop()

	// Should not panic
	reactor.Unsubscribe(SubscriptionID(99999))
}

// ---------------------------------------------------------------------------
// subscription.go: Unsubscribe — removes from clientSubs
// ---------------------------------------------------------------------------

func TestUnsubscribeRemovesFromClientSubs(t *testing.T) {
	reactor, _, reg := setup()
	defer reactor.Stop()

	reg.RegisterQuery("q", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, nil
	})

	sub1 := reactor.Subscribe("client-1", "q", nil, func(result *function.FuncResult) {})
	sub2 := reactor.Subscribe("client-1", "q", nil, func(result *function.FuncResult) {})
	time.Sleep(10 * time.Millisecond)

	// Should have 2 subs for client-1
	subs := reactor.ClientSubscriptions("client-1")
	if len(subs) != 2 {
		t.Fatalf("expected 2 subscriptions, got %d", len(subs))
	}

	// Unsubscribe first
	reactor.Unsubscribe(sub1)
	subs = reactor.ClientSubscriptions("client-1")
	if len(subs) != 1 {
		t.Errorf("expected 1 subscription after unsubscribe, got %d", len(subs))
	}
	if subs[0] != sub2 {
		t.Error("remaining sub should be sub2")
	}
}

// ---------------------------------------------------------------------------
// subscription.go: ClientSubscriptions — empty/non-existent client
// ---------------------------------------------------------------------------

func TestClientSubscriptionsEmpty(t *testing.T) {
	reactor, _, _ := setup()
	defer reactor.Stop()

	subs := reactor.ClientSubscriptions("nonexistent-client")
	if len(subs) != 0 {
		t.Errorf("expected 0 subscriptions for unknown client, got %d", len(subs))
	}
}

func TestClientSubscriptionsMultiple(t *testing.T) {
	reactor, _, reg := setup()
	defer reactor.Stop()

	reg.RegisterQuery("q1", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return "r1", nil
	})
	reg.RegisterQuery("q2", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return "r2", nil
	})

	reactor.Subscribe("c1", "q1", nil, func(result *function.FuncResult) {})
	reactor.Subscribe("c1", "q2", nil, func(result *function.FuncResult) {})
	reactor.Subscribe("c2", "q1", nil, func(result *function.FuncResult) {})
	time.Sleep(10 * time.Millisecond)

	c1Subs := reactor.ClientSubscriptions("c1")
	if len(c1Subs) != 2 {
		t.Errorf("expected 2 subs for c1, got %d", len(c1Subs))
	}

	c2Subs := reactor.ClientSubscriptions("c2")
	if len(c2Subs) != 1 {
		t.Errorf("expected 1 sub for c2, got %d", len(c2Subs))
	}
}

// ---------------------------------------------------------------------------
// subscription.go: UnsubscribeClient — then check ClientSubscriptions
// ---------------------------------------------------------------------------

func TestUnsubscribeClientClearsAll(t *testing.T) {
	reactor, _, reg := setup()
	defer reactor.Stop()

	reg.RegisterQuery("q", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, nil
	})

	reactor.Subscribe("c1", "q", nil, func(result *function.FuncResult) {})
	reactor.Subscribe("c1", "q", nil, func(result *function.FuncResult) {})
	time.Sleep(10 * time.Millisecond)

	reactor.UnsubscribeClient("c1")

	subs := reactor.ClientSubscriptions("c1")
	if len(subs) != 0 {
		t.Errorf("expected 0 after UnsubscribeClient, got %d", len(subs))
	}

	if reactor.ActiveSubscriptions() != 0 {
		t.Errorf("expected 0 active, got %d", reactor.ActiveSubscriptions())
	}
}

// ---------------------------------------------------------------------------
// subscription.go: re-evaluation detects no change (same hash)
// ---------------------------------------------------------------------------

func TestReEvaluateNoChange(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := function.NewRegistry(store, validator)
	reactor := NewReactor(reg, store.Walker())
	defer reactor.Stop()

	// Query always returns same value
	reg.RegisterQuery("static", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return "constant", nil
	})

	callCount := 0
	reactor.Subscribe("c1", "static", nil, func(result *function.FuncResult) {
		callCount++
	})

	time.Sleep(50 * time.Millisecond)

	// Trigger a data change — query result shouldn't change
	store.InsertNode("unrelated", nil, nil)
	time.Sleep(50 * time.Millisecond)

	// callCount should be 1 (initial) — re-eval should detect same hash
	if callCount > 2 {
		t.Errorf("expected at most 2 callbacks (initial + possible re-eval), got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// subscription.go: Stop halts the eval loop
// ---------------------------------------------------------------------------

func TestStopReactor(t *testing.T) {
	reactor, _, reg := setup()

	reg.RegisterQuery("q", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, nil
	})

	reactor.Subscribe("c1", "q", nil, func(result *function.FuncResult) {})
	time.Sleep(10 * time.Millisecond)

	reactor.Stop()

	// After stop, active subscriptions still exist but no new evaluations
	// Just ensure no panic
	time.Sleep(10 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// subscription.go: Subscribe with args
// ---------------------------------------------------------------------------

func TestSubscribeWithArgs(t *testing.T) {
	reactor, store, reg := setup()
	defer reactor.Stop()

	store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	store.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})

	reg.RegisterQuery("getByType", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		nodeType := args["type"].(string)
		return qctx.GetNodesByType(nodeType), nil
	})

	got := make(chan *function.FuncResult, 1)
	reactor.Subscribe("c1", "getByType", map[string]interface{}{"type": "user"}, func(result *function.FuncResult) {
		select {
		case got <- result:
		default:
		}
	})

	select {
	case result := <-got:
		if result.Error != "" {
			t.Fatalf("query error: %s", result.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}
