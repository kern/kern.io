// Package function provides a Convex-like function registry where
// queries, mutations, and actions are registered and executed.
package function

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
)

// FuncType is the type of a registered function.
type FuncType string

const (
	FuncQuery    FuncType = "query"
	FuncMutation FuncType = "mutation"
	FuncAction   FuncType = "action"
)

// QueryCtx is the read-only context passed to query functions.
type QueryCtx struct {
	store *graph.Store
}

// MutationCtx is the read-write context passed to mutation functions.
type MutationCtx struct {
	store     *graph.Store
	validator *invariant.Validator
	ops       []*crdt.Operation // operations produced by this mutation
}

// ActionCtx is the context for actions (can do I/O, call other functions).
type ActionCtx struct {
	store    *graph.Store
	registry *Registry
}

// QueryHandler is a query function signature.
type QueryHandler func(ctx context.Context, qctx *QueryCtx, args map[string]interface{}) (interface{}, error)

// MutationHandler is a mutation function signature.
type MutationHandler func(ctx context.Context, mctx *MutationCtx, args map[string]interface{}) (interface{}, error)

// ActionHandler is an action function signature.
type ActionHandler func(ctx context.Context, actx *ActionCtx, args map[string]interface{}) (interface{}, error)

// FuncDef is a registered function definition.
type FuncDef struct {
	Name    string   `json:"name"`
	Type    FuncType `json:"type"`
	// Argument schema (optional)
	ArgSchema map[string]string `json:"argSchema,omitempty"`

	queryHandler    QueryHandler
	mutationHandler MutationHandler
	actionHandler   ActionHandler
}

// Registry manages all registered functions.
type Registry struct {
	mu        sync.RWMutex
	functions map[string]*FuncDef
	store     *graph.Store
	validator *invariant.Validator
}

// NewRegistry creates a new function registry.
func NewRegistry(store *graph.Store, validator *invariant.Validator) *Registry {
	return &Registry{
		functions: make(map[string]*FuncDef),
		store:     store,
		validator: validator,
	}
}

// RegisterQuery registers a query function.
func (r *Registry) RegisterQuery(name string, handler QueryHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.functions[name] = &FuncDef{
		Name:         name,
		Type:         FuncQuery,
		queryHandler: handler,
	}
}

// RegisterMutation registers a mutation function.
func (r *Registry) RegisterMutation(name string, handler MutationHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.functions[name] = &FuncDef{
		Name:            name,
		Type:            FuncMutation,
		mutationHandler: handler,
	}
}

// RegisterAction registers an action function.
func (r *Registry) RegisterAction(name string, handler ActionHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.functions[name] = &FuncDef{
		Name:          name,
		Type:          FuncAction,
		actionHandler: handler,
	}
}

// Get returns a function definition by name.
func (r *Registry) Get(name string) (*FuncDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.functions[name]
	return fn, ok
}

// List returns all registered functions.
func (r *Registry) List() []*FuncDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*FuncDef, 0, len(r.functions))
	for _, fn := range r.functions {
		result = append(result, fn)
	}
	return result
}

// FuncResult is the result of executing a function.
type FuncResult struct {
	Value    interface{}     `json:"value,omitempty"`
	Error    string          `json:"error,omitempty"`
	Duration time.Duration   `json:"duration"`
}

// Call executes a function by name.
func (r *Registry) Call(ctx context.Context, name string, args map[string]interface{}) *FuncResult {
	start := time.Now()

	fn, ok := r.Get(name)
	if !ok {
		return &FuncResult{Error: fmt.Sprintf("function %q not found", name), Duration: time.Since(start)}
	}

	switch fn.Type {
	case FuncQuery:
		return r.callQuery(ctx, fn, args, start)
	case FuncMutation:
		return r.callMutation(ctx, fn, args, start)
	case FuncAction:
		return r.callAction(ctx, fn, args, start)
	default:
		return &FuncResult{Error: fmt.Sprintf("unknown function type %q", fn.Type), Duration: time.Since(start)}
	}
}

func (r *Registry) callQuery(ctx context.Context, fn *FuncDef, args map[string]interface{}, start time.Time) *FuncResult {
	qctx := &QueryCtx{store: r.store}
	val, err := fn.queryHandler(ctx, qctx, args)
	if err != nil {
		return &FuncResult{Error: err.Error(), Duration: time.Since(start)}
	}
	return &FuncResult{Value: val, Duration: time.Since(start)}
}

func (r *Registry) callMutation(ctx context.Context, fn *FuncDef, args map[string]interface{}, start time.Time) *FuncResult {
	mctx := &MutationCtx{
		store:     r.store,
		validator: r.validator,
	}
	val, err := fn.mutationHandler(ctx, mctx, args)
	if err != nil {
		return &FuncResult{Error: err.Error(), Duration: time.Since(start)}
	}

	// Validate invariants after mutation
	if r.validator != nil {
		if err := r.validator.ValidateAll(r.store.Walker()); err != nil {
			return &FuncResult{Error: fmt.Sprintf("invariant violation: %v", err), Duration: time.Since(start)}
		}
	}

	return &FuncResult{Value: val, Duration: time.Since(start)}
}

func (r *Registry) callAction(ctx context.Context, fn *FuncDef, args map[string]interface{}, start time.Time) *FuncResult {
	actx := &ActionCtx{store: r.store, registry: r}
	val, err := fn.actionHandler(ctx, actx, args)
	if err != nil {
		return &FuncResult{Error: err.Error(), Duration: time.Since(start)}
	}
	return &FuncResult{Value: val, Duration: time.Since(start)}
}

// --- QueryCtx methods (read-only) ---

func (q *QueryCtx) GetNode(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return q.store.GetNode(uid)
}

func (q *QueryCtx) GetNodesByType(nodeType string) interface{} {
	return q.store.GetNodesByType(nodeType)
}

func (q *QueryCtx) GetChildren(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return q.store.GetChildren(uid), nil
}

func (q *QueryCtx) GetParent(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	node, ok := q.store.GetParent(uid)
	if !ok {
		return nil, nil
	}
	return node, nil
}

func (q *QueryCtx) GetOutEdges(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return q.store.GetOutEdges(uid), nil
}

func (q *QueryCtx) GetInEdges(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return q.store.GetInEdges(uid), nil
}

func (q *QueryCtx) GetSubtree(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return q.store.GetSubtree(uid)
}

func (q *QueryCtx) GetAncestors(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return q.store.GetAncestors(uid), nil
}

func (q *QueryCtx) Traverse(id, edgeType, direction string, maxDepth int) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return q.store.Traverse(uid, edgeType, direction, maxDepth)
}

func (q *QueryCtx) FindByIndex(nodeType, property string, value interface{}) interface{} {
	return q.store.FindByIndex(nodeType, property, value)
}

func (q *QueryCtx) GetRoots() interface{} {
	return q.store.GetRoots()
}

func (q *QueryCtx) AllNodes() interface{} {
	return q.store.AllNodes()
}

func (q *QueryCtx) AllEdges() interface{} {
	return q.store.AllEdges()
}

func (q *QueryCtx) GetOrderedChildren(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return q.store.GetOrderedChildren(uid), nil
}

func (q *QueryCtx) GetDeletedNodes() interface{} {
	return q.store.GetDeletedNodes()
}

func (q *QueryCtx) Stats() interface{} {
	nodes := q.store.AllNodes()
	edges := q.store.AllEdges()
	deleted := q.store.GetDeletedNodes()
	roots := q.store.GetRoots()
	typeCounts := make(map[string]int)
	for _, n := range nodes {
		typeCounts[n.Type]++
	}
	return map[string]interface{}{
		"totalNodes":   len(nodes),
		"totalEdges":   len(edges),
		"deletedNodes": len(deleted),
		"rootNodes":    len(roots),
		"nodesByType":  typeCounts,
	}
}

// --- MutationCtx methods (read-write) ---

func (m *MutationCtx) InsertNode(nodeType string, parentID *string, properties map[string]interface{}) (string, error) {
	var pid *uuid.UUID
	if parentID != nil {
		uid, err := parseUUID(*parentID)
		if err != nil {
			return "", err
		}
		pid = &uid
	}
	id, err := m.store.InsertNode(nodeType, pid, properties)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func (m *MutationCtx) DeleteNode(id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return err
	}
	return m.store.DeleteNode(uid)
}

func (m *MutationCtx) SetProperty(nodeID, key string, value interface{}) error {
	uid, err := parseUUID(nodeID)
	if err != nil {
		return err
	}
	return m.store.SetProperty(uid, key, value)
}

func (m *MutationCtx) PatchNode(nodeID string, properties map[string]interface{}) error {
	uid, err := parseUUID(nodeID)
	if err != nil {
		return err
	}
	return m.store.PatchNode(uid, properties)
}

func (m *MutationCtx) DeleteProperty(nodeID, key string) error {
	uid, err := parseUUID(nodeID)
	if err != nil {
		return err
	}
	return m.store.DeleteProperty(uid, key)
}

func (m *MutationCtx) InsertEdge(edgeType, fromID, toID string, properties map[string]interface{}) (string, error) {
	from, err := parseUUID(fromID)
	if err != nil {
		return "", err
	}
	to, err := parseUUID(toID)
	if err != nil {
		return "", err
	}
	id, err := m.store.InsertEdge(edgeType, from, to, properties)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func (m *MutationCtx) DeleteEdge(id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return err
	}
	return m.store.DeleteEdge(uid)
}

func (m *MutationCtx) MoveNode(nodeID string, newParentID *string) error {
	uid, err := parseUUID(nodeID)
	if err != nil {
		return err
	}
	var pid *uuid.UUID
	if newParentID != nil {
		p, err := parseUUID(*newParentID)
		if err != nil {
			return err
		}
		pid = &p
	}
	return m.store.MoveNode(uid, pid)
}

func (m *MutationCtx) RestoreNode(id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return err
	}
	return m.store.RestoreNode(uid)
}

func (m *MutationCtx) SoftDeleteNode(id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return err
	}
	return m.store.SoftDeleteNode(uid)
}

func (m *MutationCtx) CascadeDeleteNode(id string) error {
	uid, err := parseUUID(id)
	if err != nil {
		return err
	}
	return m.store.CascadeDeleteNode(uid)
}

// MutationCtx also has all QueryCtx read methods
func (m *MutationCtx) GetNode(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return m.store.GetNode(uid)
}

func (m *MutationCtx) GetNodesByType(nodeType string) interface{} {
	return m.store.GetNodesByType(nodeType)
}

func (m *MutationCtx) GetChildren(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return m.store.GetChildren(uid), nil
}

func (m *MutationCtx) GetOutEdges(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return m.store.GetOutEdges(uid), nil
}

func (m *MutationCtx) GetInEdges(id string) (interface{}, error) {
	uid, err := parseUUID(id)
	if err != nil {
		return nil, err
	}
	return m.store.GetInEdges(uid), nil
}

// --- ActionCtx methods ---

func (a *ActionCtx) RunQuery(ctx context.Context, name string, args map[string]interface{}) *FuncResult {
	return a.registry.Call(ctx, name, args)
}

func (a *ActionCtx) RunMutation(ctx context.Context, name string, args map[string]interface{}) *FuncResult {
	return a.registry.Call(ctx, name, args)
}

// --- Helpers ---

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

