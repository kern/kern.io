// Package sync provides real-time subscriptions and reactivity,
// similar to Convex's reactive query system.
package sync

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kern/graphdb/internal/crdt"
	function "github.com/kern/graphdb/internal/function"
)

// SubscriptionID uniquely identifies a subscription.
type SubscriptionID uint64

var nextSubID uint64

func newSubID() SubscriptionID {
	return SubscriptionID(atomic.AddUint64(&nextSubID, 1))
}

// Subscription represents a reactive query subscription.
// When the result of the query changes, the subscriber is notified.
type Subscription struct {
	ID       SubscriptionID
	QueryName string
	Args      map[string]interface{}
	ClientID  string

	// Last result hash — used to detect changes
	lastHash string

	// Callback when the result changes
	onChange func(result *function.FuncResult)

	// Cancel function
	cancel context.CancelFunc
}

// Reactor manages subscriptions and re-evaluates queries when
// the underlying data changes. This is the core of the reactive system.
type Reactor struct {
	mu            sync.RWMutex
	subscriptions map[SubscriptionID]*Subscription
	clientSubs    map[string][]SubscriptionID // clientID -> subscriptions
	registry      *function.Registry
	walker        *crdt.EGWalker

	// Debounce: batch re-evaluations
	dirty     chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewReactor creates a new reactor.
func NewReactor(registry *function.Registry, walker *crdt.EGWalker) *Reactor {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Reactor{
		subscriptions: make(map[SubscriptionID]*Subscription),
		clientSubs:    make(map[string][]SubscriptionID),
		registry:      registry,
		walker:        walker,
		dirty:         make(chan struct{}, 1),
		ctx:           ctx,
		cancel:        cancel,
	}

	// Listen for changes on the event graph
	walker.Graph().AddListener(func(op *crdt.Operation) {
		r.markDirty()
	})

	// Start the re-evaluation loop
	go r.evalLoop()

	return r
}

// Subscribe creates a new reactive subscription to a query.
func (r *Reactor) Subscribe(clientID, queryName string, args map[string]interface{}, onChange func(result *function.FuncResult)) SubscriptionID {
	r.mu.Lock()
	defer r.mu.Unlock()

	subCtx, subCancel := context.WithCancel(r.ctx)
	_ = subCtx

	sub := &Subscription{
		ID:        newSubID(),
		QueryName: queryName,
		Args:      args,
		ClientID:  clientID,
		onChange:  onChange,
		cancel:    subCancel,
	}

	r.subscriptions[sub.ID] = sub
	r.clientSubs[clientID] = append(r.clientSubs[clientID], sub.ID)

	// Immediately evaluate and send initial result
	go func() {
		result := r.registry.Call(context.Background(), queryName, args)
		sub.lastHash = hashResult(result)
		onChange(result)
	}()

	return sub.ID
}

// Unsubscribe removes a subscription.
func (r *Reactor) Unsubscribe(id SubscriptionID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sub, ok := r.subscriptions[id]
	if !ok {
		return
	}
	sub.cancel()
	delete(r.subscriptions, id)

	// Remove from client index
	subs := r.clientSubs[sub.ClientID]
	for i, sid := range subs {
		if sid == id {
			r.clientSubs[sub.ClientID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
}

// UnsubscribeClient removes all subscriptions for a client.
func (r *Reactor) UnsubscribeClient(clientID string) {
	r.mu.Lock()
	ids := make([]SubscriptionID, len(r.clientSubs[clientID]))
	copy(ids, r.clientSubs[clientID])
	r.mu.Unlock()

	for _, id := range ids {
		r.Unsubscribe(id)
	}
}

// Stop shuts down the reactor.
func (r *Reactor) Stop() {
	r.cancel()
}

// markDirty signals that subscriptions need re-evaluation.
func (r *Reactor) markDirty() {
	select {
	case r.dirty <- struct{}{}:
	default:
		// already marked
	}
}

// evalLoop periodically re-evaluates all subscriptions when data changes.
func (r *Reactor) evalLoop() {
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-r.dirty:
			// Debounce: wait a tiny bit for more changes to batch
			time.Sleep(1 * time.Millisecond)
			r.reEvaluateAll()
		}
	}
}

// reEvaluateAll re-runs all subscribed queries and notifies if changed.
func (r *Reactor) reEvaluateAll() {
	r.mu.RLock()
	subs := make([]*Subscription, 0, len(r.subscriptions))
	for _, sub := range r.subscriptions {
		subs = append(subs, sub)
	}
	r.mu.RUnlock()

	for _, sub := range subs {
		result := r.registry.Call(context.Background(), sub.QueryName, sub.Args)
		newHash := hashResult(result)

		if newHash != sub.lastHash {
			sub.lastHash = newHash
			sub.onChange(result)
		}
	}
}

// ActiveSubscriptions returns the count of active subscriptions.
func (r *Reactor) ActiveSubscriptions() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subscriptions)
}

// ClientSubscriptions returns subscription IDs for a client.
func (r *Reactor) ClientSubscriptions(clientID string) []SubscriptionID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]SubscriptionID, len(r.clientSubs[clientID]))
	copy(result, r.clientSubs[clientID])
	return result
}

func hashResult(result *function.FuncResult) string {
	b, _ := json.Marshal(result)
	return string(b)
}
