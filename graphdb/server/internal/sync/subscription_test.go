package sync

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kern/graphdb/internal/function"
	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
)

func setup() (*Reactor, *graph.Store, *function.Registry) {
	store := graph.NewStore("test-replica")
	validator := invariant.NewValidator()
	reg := function.NewRegistry(store, validator)
	reactor := NewReactor(reg, store.Walker())
	return reactor, store, reg
}

func TestSubscribeGetsInitialResult(t *testing.T) {
	reactor, store, reg := setup()
	defer reactor.Stop()

	store.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})

	reg.RegisterQuery("listUsers", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return qctx.GetNodesByType("user"), nil
	})

	got := make(chan *function.FuncResult, 1)
	reactor.Subscribe("client-1", "listUsers", nil, func(result *function.FuncResult) {
		got <- result
	})

	select {
	case result := <-got:
		if result.Error != "" {
			t.Fatalf("subscription error: %s", result.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for initial result")
	}
}

func TestSubscribeNotifiesOnChange(t *testing.T) {
	reactor, store, reg := setup()
	defer reactor.Stop()

	reg.RegisterQuery("countUsers", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		nodes := qctx.GetNodesByType("user")
		return len(nodes.([]interface{})), nil
	})

	// Actually register a simpler query that returns a count
	reg.RegisterQuery("countUsers", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return qctx.AllNodes(), nil
	})

	var callCount int64
	reactor.Subscribe("client-1", "countUsers", nil, func(result *function.FuncResult) {
		atomic.AddInt64(&callCount, 1)
	})

	// Wait for initial result
	time.Sleep(50 * time.Millisecond)

	// Make a mutation that should trigger re-evaluation
	store.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})

	// Wait for re-evaluation
	time.Sleep(50 * time.Millisecond)

	count := atomic.LoadInt64(&callCount)
	if count < 2 {
		t.Errorf("expected at least 2 callbacks (initial + change), got %d", count)
	}
}

func TestUnsubscribe(t *testing.T) {
	reactor, _, reg := setup()
	defer reactor.Stop()

	reg.RegisterQuery("q", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, nil
	})

	subID := reactor.Subscribe("client-1", "q", nil, func(result *function.FuncResult) {})
	time.Sleep(10 * time.Millisecond)

	if reactor.ActiveSubscriptions() != 1 {
		t.Errorf("expected 1 subscription, got %d", reactor.ActiveSubscriptions())
	}

	reactor.Unsubscribe(subID)
	if reactor.ActiveSubscriptions() != 0 {
		t.Errorf("expected 0 subscriptions, got %d", reactor.ActiveSubscriptions())
	}
}

func TestUnsubscribeClient(t *testing.T) {
	reactor, _, reg := setup()
	defer reactor.Stop()

	reg.RegisterQuery("q1", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, nil
	})
	reg.RegisterQuery("q2", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, nil
	})

	reactor.Subscribe("client-1", "q1", nil, func(result *function.FuncResult) {})
	reactor.Subscribe("client-1", "q2", nil, func(result *function.FuncResult) {})
	reactor.Subscribe("client-2", "q1", nil, func(result *function.FuncResult) {})
	time.Sleep(10 * time.Millisecond)

	if reactor.ActiveSubscriptions() != 3 {
		t.Errorf("expected 3 subscriptions, got %d", reactor.ActiveSubscriptions())
	}

	reactor.UnsubscribeClient("client-1")
	if reactor.ActiveSubscriptions() != 1 {
		t.Errorf("expected 1 subscription after client disconnect, got %d", reactor.ActiveSubscriptions())
	}
}
