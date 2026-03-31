package crdt

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// EGWalker implements the eg-walker algorithm for computing
// the materialized state of the graph from the event graph.
//
// The key insight of eg-walker is that instead of maintaining
// complex CRDT metadata at each node, we store a simple log
// of operations (the event graph) and "walk" it to compute state.
//
// For concurrent operations, we use these resolution strategies:
//   - Node/Edge insert: both survive (set union)
//   - Node/Edge delete: delete wins if concurrent with property update;
//     but insert-after-delete resurrects (last-writer-wins on existence)
//   - Properties: last-writer-wins register using Lamport timestamps
//   - Move/re-parent: last-writer-wins, with cycle detection
type EGWalker struct {
	mu        sync.RWMutex
	graph     *EventGraph
	replicaID string

	// Materialized state caches — rebuilt by walking the event graph
	nodes      map[uuid.UUID]*MaterializedNode
	edges      map[uuid.UUID]*MaterializedEdge
	parentMap  map[uuid.UUID]uuid.UUID    // child -> parent
	childMap   map[uuid.UUID][]uuid.UUID  // parent -> children
	edgeIndex  map[uuid.UUID][]uuid.UUID  // node -> outgoing edge IDs
	edgeRevIdx map[uuid.UUID][]uuid.UUID  // node -> incoming edge IDs
	typeIndex  map[string][]uuid.UUID     // nodeType -> node IDs

	// Dirty flag: if true, materialized state needs recompute
	dirty bool
}

// MaterializedNode is the computed state of a node after walking the event graph.
type MaterializedNode struct {
	ID         uuid.UUID              `json:"id"`
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	ParentID   *uuid.UUID             `json:"parentId,omitempty"`
	CreatedAt  time.Time              `json:"createdAt"`
	UpdatedAt  time.Time              `json:"updatedAt"`
	Deleted    bool                   `json:"deleted"`
	// Track which event last wrote each property (for LWW resolution)
	propVersions map[string]EventID
	// Track the event that created/deleted this node
	createEvent EventID
	deleteEvent *EventID
}

// MaterializedEdge is the computed state of an edge.
type MaterializedEdge struct {
	ID         uuid.UUID              `json:"id"`
	Type       string                 `json:"type"`
	FromID     uuid.UUID              `json:"fromId"`
	ToID       uuid.UUID              `json:"toId"`
	Properties map[string]interface{} `json:"properties"`
	CreatedAt  time.Time              `json:"createdAt"`
	Deleted    bool                   `json:"deleted"`
	propVersions map[string]EventID
	createEvent  EventID
	deleteEvent  *EventID
}

// NewEGWalker creates a new eg-walker instance.
func NewEGWalker(replicaID string) *EGWalker {
	w := &EGWalker{
		graph:      NewEventGraph(),
		replicaID:  replicaID,
		nodes:      make(map[uuid.UUID]*MaterializedNode),
		edges:      make(map[uuid.UUID]*MaterializedEdge),
		parentMap:  make(map[uuid.UUID]uuid.UUID),
		childMap:   make(map[uuid.UUID][]uuid.UUID),
		edgeIndex:  make(map[uuid.UUID][]uuid.UUID),
		edgeRevIdx: make(map[uuid.UUID][]uuid.UUID),
		typeIndex:  make(map[string][]uuid.UUID),
		dirty:      false,
	}

	// Register listener to incrementally update state
	w.graph.AddListener(func(op *Operation) {
		w.applyOp(op)
	})

	return w
}

// Graph returns the underlying event graph.
func (w *EGWalker) Graph() *EventGraph {
	return w.graph
}

// ReplicaID returns this walker's replica ID.
func (w *EGWalker) ReplicaID() string {
	return w.replicaID
}

// newOp creates a new operation with the current frontier as parents.
func (w *EGWalker) newOp(opType OpType) *Operation {
	return &Operation{
		ID: EventID{
			ReplicaID: w.replicaID,
			Seq:       w.graph.NextSeq(w.replicaID),
		},
		Parents:   w.graph.Frontier(),
		Type:      opType,
		Timestamp: time.Now(),
	}
}

// InsertNode creates a new node in the graph.
func (w *EGWalker) InsertNode(nodeType string, parentID *uuid.UUID, properties map[string]interface{}) (uuid.UUID, *Operation, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	id := uuid.New()
	op := w.newOp(OpInsertNode)
	op.TargetID = id
	op.NodeType = nodeType
	op.ParentRef = parentID
	op.Value = properties

	if err := w.graph.Apply(op); err != nil {
		return uuid.Nil, nil, err
	}

	return id, op, nil
}

// DeleteNode marks a node as deleted.
func (w *EGWalker) DeleteNode(id uuid.UUID) (*Operation, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	op := w.newOp(OpDeleteNode)
	op.TargetID = id

	if err := w.graph.Apply(op); err != nil {
		return nil, err
	}

	return op, nil
}

// SetProperty sets a property on a node.
func (w *EGWalker) SetProperty(nodeID uuid.UUID, key string, value interface{}) (*Operation, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	op := w.newOp(OpSetProperty)
	op.TargetID = nodeID
	op.Key = key
	op.Value = value

	if err := w.graph.Apply(op); err != nil {
		return nil, err
	}

	return op, nil
}

// DeleteProperty removes a property from a node.
func (w *EGWalker) DeleteProperty(nodeID uuid.UUID, key string) (*Operation, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	op := w.newOp(OpDeleteProperty)
	op.TargetID = nodeID
	op.Key = key

	if err := w.graph.Apply(op); err != nil {
		return nil, err
	}

	return op, nil
}

// InsertEdge creates an edge between two nodes.
func (w *EGWalker) InsertEdge(edgeType string, fromID, toID uuid.UUID, properties map[string]interface{}) (uuid.UUID, *Operation, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	id := uuid.New()
	op := w.newOp(OpInsertEdge)
	op.TargetID = id
	op.EdgeType = edgeType
	op.EdgeFrom = fromID
	op.EdgeTo = toID
	op.Value = properties

	if err := w.graph.Apply(op); err != nil {
		return uuid.Nil, nil, err
	}

	return id, op, nil
}

// DeleteEdge marks an edge as deleted.
func (w *EGWalker) DeleteEdge(id uuid.UUID) (*Operation, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	op := w.newOp(OpDeleteEdge)
	op.TargetID = id

	if err := w.graph.Apply(op); err != nil {
		return nil, err
	}

	return op, nil
}

// MoveNode re-parents a node in the hierarchy.
func (w *EGWalker) MoveNode(nodeID uuid.UUID, newParentID *uuid.UUID) (*Operation, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Cycle detection: walk up from newParentID, make sure we don't hit nodeID
	if newParentID != nil {
		if w.wouldCreateCycle(nodeID, *newParentID) {
			return nil, fmt.Errorf("moving node %s under %s would create a cycle", nodeID, *newParentID)
		}
	}

	op := w.newOp(OpMoveNode)
	op.TargetID = nodeID
	op.ParentRef = newParentID

	if err := w.graph.Apply(op); err != nil {
		return nil, err
	}

	return op, nil
}

// ApplyRemote applies an operation from a remote replica.
func (w *EGWalker) ApplyRemote(op *Operation) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.graph.Apply(op)
}

// wouldCreateCycle checks if re-parenting would create a cycle.
func (w *EGWalker) wouldCreateCycle(nodeID, newParentID uuid.UUID) bool {
	current := newParentID
	for {
		if current == nodeID {
			return true
		}
		parent, ok := w.parentMap[current]
		if !ok {
			return false
		}
		current = parent
	}
}

// applyOp incrementally updates the materialized state from a single new operation.
// This is the core of the eg-walker: each op type has a specific merge semantic.
func (w *EGWalker) applyOp(op *Operation) {
	switch op.Type {
	case OpInsertNode:
		props := make(map[string]interface{})
		if op.Value != nil {
			if m, ok := op.Value.(map[string]interface{}); ok {
				for k, v := range m {
					props[k] = v
				}
			}
		}
		propVersions := make(map[string]EventID)
		for k := range props {
			propVersions[k] = op.ID
		}
		node := &MaterializedNode{
			ID:           op.TargetID,
			Type:         op.NodeType,
			Properties:   props,
			ParentID:     op.ParentRef,
			CreatedAt:    op.Timestamp,
			UpdatedAt:    op.Timestamp,
			Deleted:      false,
			propVersions: propVersions,
			createEvent:  op.ID,
		}
		w.nodes[op.TargetID] = node

		if op.ParentRef != nil {
			w.parentMap[op.TargetID] = *op.ParentRef
			w.childMap[*op.ParentRef] = append(w.childMap[*op.ParentRef], op.TargetID)
		}

		w.typeIndex[op.NodeType] = append(w.typeIndex[op.NodeType], op.TargetID)

	case OpDeleteNode:
		if node, ok := w.nodes[op.TargetID]; ok {
			// Delete wins: LWW based on event ordering
			if node.deleteEvent == nil || w.eventAfter(op.ID, *node.deleteEvent) {
				node.Deleted = true
				node.deleteEvent = &op.ID
				node.UpdatedAt = op.Timestamp
			}
		}

	case OpSetProperty:
		if node, ok := w.nodes[op.TargetID]; ok {
			// LWW register: the event with the higher (Seq, ReplicaID) wins
			if existing, ok := node.propVersions[op.Key]; ok {
				if !w.eventAfter(op.ID, existing) {
					return // stale write
				}
			}
			node.Properties[op.Key] = op.Value
			node.propVersions[op.Key] = op.ID
			node.UpdatedAt = op.Timestamp
		} else if edge, ok := w.edges[op.TargetID]; ok {
			if existing, ok := edge.propVersions[op.Key]; ok {
				if !w.eventAfter(op.ID, existing) {
					return
				}
			}
			edge.Properties[op.Key] = op.Value
			edge.propVersions[op.Key] = op.ID
		}

	case OpDeleteProperty:
		if node, ok := w.nodes[op.TargetID]; ok {
			if existing, ok := node.propVersions[op.Key]; ok {
				if w.eventAfter(op.ID, existing) {
					delete(node.Properties, op.Key)
					node.propVersions[op.Key] = op.ID
					node.UpdatedAt = op.Timestamp
				}
			}
		}

	case OpInsertEdge:
		props := make(map[string]interface{})
		if op.Value != nil {
			if m, ok := op.Value.(map[string]interface{}); ok {
				for k, v := range m {
					props[k] = v
				}
			}
		}
		propVersions := make(map[string]EventID)
		for k := range props {
			propVersions[k] = op.ID
		}
		edge := &MaterializedEdge{
			ID:           op.TargetID,
			Type:         op.EdgeType,
			FromID:       op.EdgeFrom,
			ToID:         op.EdgeTo,
			Properties:   props,
			CreatedAt:    op.Timestamp,
			Deleted:      false,
			propVersions: propVersions,
			createEvent:  op.ID,
		}
		w.edges[op.TargetID] = edge
		w.edgeIndex[op.EdgeFrom] = append(w.edgeIndex[op.EdgeFrom], op.TargetID)
		w.edgeRevIdx[op.EdgeTo] = append(w.edgeRevIdx[op.EdgeTo], op.TargetID)

	case OpDeleteEdge:
		if edge, ok := w.edges[op.TargetID]; ok {
			if edge.deleteEvent == nil || w.eventAfter(op.ID, *edge.deleteEvent) {
				edge.Deleted = true
				edge.deleteEvent = &op.ID
			}
		}

	case OpMoveNode:
		if node, ok := w.nodes[op.TargetID]; ok {
			// Remove from old parent
			if oldParent, ok := w.parentMap[op.TargetID]; ok {
				children := w.childMap[oldParent]
				for i, child := range children {
					if child == op.TargetID {
						w.childMap[oldParent] = append(children[:i], children[i+1:]...)
						break
					}
				}
			}

			// Set new parent
			if op.ParentRef != nil {
				node.ParentID = op.ParentRef
				w.parentMap[op.TargetID] = *op.ParentRef
				w.childMap[*op.ParentRef] = append(w.childMap[*op.ParentRef], op.TargetID)
			} else {
				node.ParentID = nil
				delete(w.parentMap, op.TargetID)
			}
			node.UpdatedAt = op.Timestamp
		}
	}
}

// eventAfter returns true if a is causally after (or concurrent but wins by LWW) b.
func (w *EGWalker) eventAfter(a, b EventID) bool {
	if a.Seq != b.Seq {
		return a.Seq > b.Seq
	}
	return a.ReplicaID > b.ReplicaID
}

// GetNode returns a materialized node by ID.
func (w *EGWalker) GetNode(id uuid.UUID) (*MaterializedNode, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	node, ok := w.nodes[id]
	if !ok || node.Deleted {
		return nil, false
	}
	return node, true
}

// GetEdge returns a materialized edge by ID.
func (w *EGWalker) GetEdge(id uuid.UUID) (*MaterializedEdge, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	edge, ok := w.edges[id]
	if !ok || edge.Deleted {
		return nil, false
	}
	return edge, true
}

// GetChildren returns the children of a node in the hierarchy.
func (w *EGWalker) GetChildren(parentID uuid.UUID) []*MaterializedNode {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var result []*MaterializedNode
	for _, childID := range w.childMap[parentID] {
		if node, ok := w.nodes[childID]; ok && !node.Deleted {
			result = append(result, node)
		}
	}
	return result
}

// GetParent returns the parent of a node.
func (w *EGWalker) GetParent(nodeID uuid.UUID) (*MaterializedNode, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	parentID, ok := w.parentMap[nodeID]
	if !ok {
		return nil, false
	}
	node, ok := w.nodes[parentID]
	if !ok || node.Deleted {
		return nil, false
	}
	return node, true
}

// GetOutEdges returns outgoing edges from a node.
func (w *EGWalker) GetOutEdges(nodeID uuid.UUID) []*MaterializedEdge {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var result []*MaterializedEdge
	for _, edgeID := range w.edgeIndex[nodeID] {
		if edge, ok := w.edges[edgeID]; ok && !edge.Deleted {
			result = append(result, edge)
		}
	}
	return result
}

// GetInEdges returns incoming edges to a node.
func (w *EGWalker) GetInEdges(nodeID uuid.UUID) []*MaterializedEdge {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var result []*MaterializedEdge
	for _, edgeID := range w.edgeRevIdx[nodeID] {
		if edge, ok := w.edges[edgeID]; ok && !edge.Deleted {
			result = append(result, edge)
		}
	}
	return result
}

// GetNodesByType returns all non-deleted nodes of a given type.
func (w *EGWalker) GetNodesByType(nodeType string) []*MaterializedNode {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var result []*MaterializedNode
	for _, id := range w.typeIndex[nodeType] {
		if node, ok := w.nodes[id]; ok && !node.Deleted {
			result = append(result, node)
		}
	}
	return result
}

// GetRoots returns all root nodes (nodes with no parent).
func (w *EGWalker) GetRoots() []*MaterializedNode {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var result []*MaterializedNode
	for _, node := range w.nodes {
		if !node.Deleted && node.ParentID == nil {
			result = append(result, node)
		}
	}
	return result
}

// AllNodes returns all non-deleted nodes.
func (w *EGWalker) AllNodes() []*MaterializedNode {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var result []*MaterializedNode
	for _, node := range w.nodes {
		if !node.Deleted {
			result = append(result, node)
		}
	}
	return result
}

// AllEdges returns all non-deleted edges.
func (w *EGWalker) AllEdges() []*MaterializedEdge {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var result []*MaterializedEdge
	for _, edge := range w.edges {
		if !edge.Deleted {
			result = append(result, edge)
		}
	}
	return result
}

