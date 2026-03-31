// Package graph provides the high-level graph store that wraps the CRDT layer
// and exposes a Convex-like API for queries and mutations.
package graph

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
)

// Store is the main graph database store. It wraps an EGWalker and adds
// schema awareness, indexing, and transactional semantics.
type Store struct {
	mu     sync.RWMutex
	walker *crdt.EGWalker
	schema *Schema

	// Secondary indexes: (nodeType, propertyKey, propertyValue) -> []nodeID
	indexes map[indexKey]map[string][]uuid.UUID
}

type indexKey struct {
	NodeType string
	Property string
}

// NewStore creates a new graph store.
func NewStore(replicaID string) *Store {
	return &Store{
		walker:  crdt.NewEGWalker(replicaID),
		schema:  NewSchema(),
		indexes: make(map[indexKey]map[string][]uuid.UUID),
	}
}

// Walker returns the underlying EGWalker.
func (s *Store) Walker() *crdt.EGWalker {
	return s.walker
}

// SetSchema sets the schema for the store.
func (s *Store) SetSchema(schema *Schema) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.schema = schema
}

// Schema returns the current schema.
func (s *Store) GetSchema() *Schema {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.schema
}

// --- Node operations ---

// InsertNode creates a new node, validating against the schema.
func (s *Store) InsertNode(nodeType string, parentID *uuid.UUID, properties map[string]interface{}) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Schema validation
	if s.schema != nil {
		if err := s.schema.ValidateNode(nodeType, properties); err != nil {
			return uuid.Nil, fmt.Errorf("schema validation failed: %w", err)
		}
		if parentID != nil {
			parent, ok := s.walker.GetNode(*parentID)
			if !ok {
				return uuid.Nil, fmt.Errorf("parent node %s not found", parentID)
			}
			if err := s.schema.ValidateHierarchy(parent.Type, nodeType); err != nil {
				return uuid.Nil, fmt.Errorf("hierarchy validation failed: %w", err)
			}
		}
	}

	id, _, err := s.walker.InsertNode(nodeType, parentID, properties)
	if err != nil {
		return uuid.Nil, err
	}

	// Update indexes
	s.updateIndexes(id, nodeType, properties)

	return id, nil
}

// DeleteNode deletes a node.
func (s *Store) DeleteNode(id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.walker.GetNode(id)
	if !ok {
		return fmt.Errorf("node %s not found", id)
	}

	// Check if node has children — prevent orphaning
	children := s.walker.GetChildren(id)
	if len(children) > 0 {
		return fmt.Errorf("cannot delete node %s: has %d children", id, len(children))
	}

	// Remove from indexes
	s.removeFromIndexes(id, node.Type, node.Properties)

	_, err := s.walker.DeleteNode(id)
	return err
}

// GetNode gets a node by ID.
func (s *Store) GetNode(id uuid.UUID) (*crdt.MaterializedNode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, ok := s.walker.GetNode(id)
	if !ok {
		return nil, fmt.Errorf("node %s not found", id)
	}
	return node, nil
}

// SetProperty sets a property on a node with schema validation.
func (s *Store) SetProperty(nodeID uuid.UUID, key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.walker.GetNode(nodeID)
	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}

	if s.schema != nil {
		if err := s.schema.ValidateProperty(node.Type, key, value); err != nil {
			return fmt.Errorf("schema validation failed: %w", err)
		}
	}

	// Update index
	oldValue, hadOld := node.Properties[key]
	if hadOld {
		s.removeFromIndex(nodeID, node.Type, key, oldValue)
	}

	_, err := s.walker.SetProperty(nodeID, key, value)
	if err != nil {
		return err
	}

	s.addToIndex(nodeID, node.Type, key, value)
	return nil
}

// PatchNode updates multiple properties at once.
func (s *Store) PatchNode(nodeID uuid.UUID, properties map[string]interface{}) error {
	for k, v := range properties {
		if err := s.SetProperty(nodeID, k, v); err != nil {
			return err
		}
	}
	return nil
}

// DeleteProperty deletes a property from a node.
func (s *Store) DeleteProperty(nodeID uuid.UUID, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.walker.GetNode(nodeID)
	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}

	if oldValue, ok := node.Properties[key]; ok {
		s.removeFromIndex(nodeID, node.Type, key, oldValue)
	}

	_, err := s.walker.DeleteProperty(nodeID, key)
	return err
}

// --- Edge operations ---

// InsertEdge creates an edge between two nodes.
func (s *Store) InsertEdge(edgeType string, fromID, toID uuid.UUID, properties map[string]interface{}) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate both endpoints exist
	fromNode, ok := s.walker.GetNode(fromID)
	if !ok {
		return uuid.Nil, fmt.Errorf("source node %s not found", fromID)
	}
	toNode, ok := s.walker.GetNode(toID)
	if !ok {
		return uuid.Nil, fmt.Errorf("target node %s not found", toID)
	}

	// Schema validation
	if s.schema != nil {
		if err := s.schema.ValidateEdge(edgeType, fromNode.Type, toNode.Type); err != nil {
			return uuid.Nil, fmt.Errorf("schema validation failed: %w", err)
		}
	}

	id, _, err := s.walker.InsertEdge(edgeType, fromID, toID, properties)
	if err != nil {
		return uuid.Nil, err
	}

	return id, nil
}

// DeleteEdge deletes an edge.
func (s *Store) DeleteEdge(id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.walker.GetEdge(id)
	if !ok {
		return fmt.Errorf("edge %s not found", id)
	}

	_, err := s.walker.DeleteEdge(id)
	return err
}

// --- Hierarchy operations ---

// MoveNode re-parents a node in the hierarchy.
func (s *Store) MoveNode(nodeID uuid.UUID, newParentID *uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	node, ok := s.walker.GetNode(nodeID)
	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}

	if newParentID != nil {
		parent, ok := s.walker.GetNode(*newParentID)
		if !ok {
			return fmt.Errorf("new parent node %s not found", newParentID)
		}
		if s.schema != nil {
			if err := s.schema.ValidateHierarchy(parent.Type, node.Type); err != nil {
				return fmt.Errorf("hierarchy validation failed: %w", err)
			}
		}
	}

	_, err := s.walker.MoveNode(nodeID, newParentID)
	return err
}

// GetChildren returns children of a node.
func (s *Store) GetChildren(parentID uuid.UUID) []*crdt.MaterializedNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.walker.GetChildren(parentID)
}

// GetParent returns the parent of a node.
func (s *Store) GetParent(nodeID uuid.UUID) (*crdt.MaterializedNode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.walker.GetParent(nodeID)
}

// GetRoots returns all root nodes.
func (s *Store) GetRoots() []*crdt.MaterializedNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.walker.GetRoots()
}

// GetSubtree returns a node and all its descendants.
func (s *Store) GetSubtree(nodeID uuid.UUID) ([]*crdt.MaterializedNode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, ok := s.walker.GetNode(nodeID)
	if !ok {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	var result []*crdt.MaterializedNode
	var walk func(id uuid.UUID)
	walk = func(id uuid.UUID) {
		n, ok := s.walker.GetNode(id)
		if !ok {
			return
		}
		result = append(result, n)
		for _, child := range s.walker.GetChildren(id) {
			walk(child.ID)
		}
	}
	_ = node
	walk(nodeID)
	return result, nil
}

// GetAncestors returns the path from a node to the root.
func (s *Store) GetAncestors(nodeID uuid.UUID) []*crdt.MaterializedNode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*crdt.MaterializedNode
	current := nodeID
	for {
		parent, ok := s.walker.GetParent(current)
		if !ok {
			break
		}
		result = append(result, parent)
		current = parent.ID
	}
	return result
}

// --- Query operations ---

// GetNodesByType returns all nodes of a given type.
func (s *Store) GetNodesByType(nodeType string) []*crdt.MaterializedNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.walker.GetNodesByType(nodeType)
}

// GetOutEdges returns outgoing edges from a node.
func (s *Store) GetOutEdges(nodeID uuid.UUID) []*crdt.MaterializedEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.walker.GetOutEdges(nodeID)
}

// GetInEdges returns incoming edges to a node.
func (s *Store) GetInEdges(nodeID uuid.UUID) []*crdt.MaterializedEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.walker.GetInEdges(nodeID)
}

// GetOutEdgesByType returns outgoing edges of a specific type from a node.
func (s *Store) GetOutEdgesByType(nodeID uuid.UUID, edgeType string) []*crdt.MaterializedEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*crdt.MaterializedEdge
	for _, edge := range s.walker.GetOutEdges(nodeID) {
		if edge.Type == edgeType {
			result = append(result, edge)
		}
	}
	return result
}

// GetInEdgesByType returns incoming edges of a specific type to a node.
func (s *Store) GetInEdgesByType(nodeID uuid.UUID, edgeType string) []*crdt.MaterializedEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*crdt.MaterializedEdge
	for _, edge := range s.walker.GetInEdges(nodeID) {
		if edge.Type == edgeType {
			result = append(result, edge)
		}
	}
	return result
}

// Traverse follows edges from a starting node. Returns all reachable nodes.
func (s *Store) Traverse(startID uuid.UUID, edgeType string, direction string, maxDepth int) ([]*crdt.MaterializedNode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	visited := make(map[uuid.UUID]bool)
	var result []*crdt.MaterializedNode

	var walk func(id uuid.UUID, depth int)
	walk = func(id uuid.UUID, depth int) {
		if depth > maxDepth || visited[id] {
			return
		}
		visited[id] = true

		node, ok := s.walker.GetNode(id)
		if !ok {
			return
		}
		result = append(result, node)

		var edges []*crdt.MaterializedEdge
		switch direction {
		case "out":
			edges = s.walker.GetOutEdges(id)
		case "in":
			edges = s.walker.GetInEdges(id)
		case "both":
			edges = append(s.walker.GetOutEdges(id), s.walker.GetInEdges(id)...)
		}

		for _, edge := range edges {
			if edgeType != "" && edge.Type != edgeType {
				continue
			}
			var nextID uuid.UUID
			if edge.FromID == id {
				nextID = edge.ToID
			} else {
				nextID = edge.FromID
			}
			walk(nextID, depth+1)
		}
	}

	walk(startID, 0)
	return result, nil
}

// FindByIndex looks up nodes by a secondary index.
func (s *Store) FindByIndex(nodeType, property string, value interface{}) []*crdt.MaterializedNode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := indexKey{NodeType: nodeType, Property: property}
	valStr := indexValueString(value)
	ids, ok := s.indexes[key][valStr]
	if !ok {
		return nil
	}

	var result []*crdt.MaterializedNode
	for _, id := range ids {
		if node, ok := s.walker.GetNode(id); ok {
			result = append(result, node)
		}
	}
	return result
}

// AllNodes returns all non-deleted nodes.
func (s *Store) AllNodes() []*crdt.MaterializedNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.walker.AllNodes()
}

// AllEdges returns all non-deleted edges.
func (s *Store) AllEdges() []*crdt.MaterializedEdge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.walker.AllEdges()
}

// --- Reorder ---

// ReorderNode changes a node's position among its siblings.
func (s *Store) ReorderNode(nodeID uuid.UUID, position string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.walker.GetNode(nodeID)
	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}
	_, err := s.walker.ReorderNode(nodeID, position)
	return err
}

// ReorderBetween places a node between two siblings (or at start/end).
func (s *Store) ReorderBetween(nodeID uuid.UUID, afterID, beforeID *uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.walker.GetNode(nodeID)
	if !ok {
		return fmt.Errorf("node %s not found", nodeID)
	}
	var afterPos, beforePos string
	if afterID != nil {
		if n, ok := s.walker.GetNode(*afterID); ok {
			afterPos = n.Position
		}
	}
	if beforeID != nil {
		if n, ok := s.walker.GetNode(*beforeID); ok {
			beforePos = n.Position
		}
	}
	pos := crdt.PositionBetween(afterPos, beforePos)
	_, err := s.walker.ReorderNode(nodeID, pos)
	return err
}

// GetOrderedChildren returns children sorted by fractional index.
func (s *Store) GetOrderedChildren(parentID uuid.UUID) []*crdt.MaterializedNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.walker.GetOrderedChildren(parentID)
}

// --- Soft Delete / Restore ---

// SoftDeleteNode soft-deletes a node (keeps it in the graph, marked as deleted).
func (s *Store) SoftDeleteNode(id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.walker.GetNode(id)
	if !ok {
		return fmt.Errorf("node %s not found", id)
	}
	_, err := s.walker.DeleteNode(id)
	return err
}

// CascadeDeleteNode soft-deletes a node and all its descendants.
func (s *Store) CascadeDeleteNode(id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var cascadeDelete func(nodeID uuid.UUID) error
	cascadeDelete = func(nodeID uuid.UUID) error {
		// Delete children first
		children := s.walker.GetChildren(nodeID)
		for _, child := range children {
			if err := cascadeDelete(child.ID); err != nil {
				return err
			}
		}
		// Delete associated edges
		for _, edge := range s.walker.GetOutEdges(nodeID) {
			if _, err := s.walker.DeleteEdge(edge.ID); err != nil {
				return err
			}
		}
		for _, edge := range s.walker.GetInEdges(nodeID) {
			if _, err := s.walker.DeleteEdge(edge.ID); err != nil {
				return err
			}
		}
		// Remove from indexes
		if node, ok := s.walker.GetNode(nodeID); ok {
			s.removeFromIndexes(nodeID, node.Type, node.Properties)
		}
		_, err := s.walker.DeleteNode(nodeID)
		return err
	}

	return cascadeDelete(id)
}

// RestoreNode un-deletes a soft-deleted node.
func (s *Store) RestoreNode(id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	node, ok := s.walker.GetNodeIncludingDeleted(id)
	if !ok {
		return fmt.Errorf("node %s not found", id)
	}
	if !node.Deleted {
		return fmt.Errorf("node %s is not deleted", id)
	}
	_, err := s.walker.RestoreNode(id)
	if err != nil {
		return err
	}
	// Re-add to indexes
	s.updateIndexes(id, node.Type, node.Properties)
	return nil
}

// GetDeletedNodes returns all soft-deleted nodes.
func (s *Store) GetDeletedNodes() []*crdt.MaterializedNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.walker.GetDeletedNodes()
}

// --- Orphan Reaping ---

// ReapOrphans finds nodes whose parents are deleted and soft-deletes them.
// Returns the IDs of reaped nodes.
func (s *Store) ReapOrphans() ([]uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var reaped []uuid.UUID
	// Iterate all live nodes, check if parent is deleted
	for _, node := range s.walker.AllNodes() {
		if node.ParentID == nil {
			continue
		}
		parent, ok := s.walker.GetNode(*node.ParentID)
		if !ok || parent.Deleted {
			// Parent is deleted or missing — this is an orphan
			s.removeFromIndexes(node.ID, node.Type, node.Properties)
			if _, err := s.walker.DeleteNode(node.ID); err != nil {
				return reaped, err
			}
			reaped = append(reaped, node.ID)
		}
	}

	// Repeat until no more orphans found (cascading orphans)
	if len(reaped) > 0 {
		more, err := s.reapOrphansUnlocked()
		if err != nil {
			return reaped, err
		}
		reaped = append(reaped, more...)
	}

	return reaped, nil
}

func (s *Store) reapOrphansUnlocked() ([]uuid.UUID, error) {
	var reaped []uuid.UUID
	for _, node := range s.walker.AllNodes() {
		if node.ParentID == nil {
			continue
		}
		parent, ok := s.walker.GetNode(*node.ParentID)
		if !ok || parent.Deleted {
			s.removeFromIndexes(node.ID, node.Type, node.Properties)
			if _, err := s.walker.DeleteNode(node.ID); err != nil {
				return reaped, err
			}
			reaped = append(reaped, node.ID)
		}
	}
	if len(reaped) > 0 {
		more, err := s.reapOrphansUnlocked()
		if err != nil {
			return reaped, err
		}
		reaped = append(reaped, more...)
	}
	return reaped, nil
}

// --- Batch Operations ---

// BatchOpType identifies a batch operation.
type BatchOpType int

const (
	BatchInsertNode BatchOpType = iota
	BatchDeleteNode
	BatchSetProperty
	BatchDeleteProperty
	BatchInsertEdge
	BatchDeleteEdge
	BatchMoveNode
	BatchReorderNode
	BatchRestoreNode
	BatchCascadeDelete
)

// BatchOp is a single operation within a batch.
type BatchOp struct {
	Type       BatchOpType
	NodeType   string
	ParentID   *uuid.UUID
	Properties map[string]interface{}
	NodeID     uuid.UUID
	Key        string
	Value      interface{}
	EdgeType   string
	FromID     uuid.UUID
	ToID       uuid.UUID
	EdgeID     uuid.UUID
	Position   string

	// Output: filled after execution
	ResultID uuid.UUID
}

// BatchResult holds the result of a batch execution.
type BatchResult struct {
	Results []BatchOp
}

// ExecuteBatch executes multiple operations atomically.
// If any operation fails, previously applied operations in this batch
// are still applied (CRDT operations are idempotent and conflict-free).
func (s *Store) ExecuteBatch(ops []BatchOp) (*BatchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := make([]BatchOp, len(ops))
	copy(results, ops)

	for i := range results {
		op := &results[i]
		switch op.Type {
		case BatchInsertNode:
			id, _, err := s.walker.InsertNode(op.NodeType, op.ParentID, op.Properties)
			if err != nil {
				return &BatchResult{Results: results}, fmt.Errorf("batch op %d (InsertNode): %w", i, err)
			}
			op.ResultID = id
			s.updateIndexes(id, op.NodeType, op.Properties)

		case BatchDeleteNode:
			node, ok := s.walker.GetNode(op.NodeID)
			if !ok {
				return &BatchResult{Results: results}, fmt.Errorf("batch op %d (DeleteNode): node %s not found", i, op.NodeID)
			}
			s.removeFromIndexes(op.NodeID, node.Type, node.Properties)
			if _, err := s.walker.DeleteNode(op.NodeID); err != nil {
				return &BatchResult{Results: results}, fmt.Errorf("batch op %d (DeleteNode): %w", i, err)
			}

		case BatchSetProperty:
			if _, err := s.walker.SetProperty(op.NodeID, op.Key, op.Value); err != nil {
				return &BatchResult{Results: results}, fmt.Errorf("batch op %d (SetProperty): %w", i, err)
			}

		case BatchDeleteProperty:
			if _, err := s.walker.DeleteProperty(op.NodeID, op.Key); err != nil {
				return &BatchResult{Results: results}, fmt.Errorf("batch op %d (DeleteProperty): %w", i, err)
			}

		case BatchInsertEdge:
			id, _, err := s.walker.InsertEdge(op.EdgeType, op.FromID, op.ToID, op.Properties)
			if err != nil {
				return &BatchResult{Results: results}, fmt.Errorf("batch op %d (InsertEdge): %w", i, err)
			}
			op.ResultID = id

		case BatchDeleteEdge:
			if _, err := s.walker.DeleteEdge(op.EdgeID); err != nil {
				return &BatchResult{Results: results}, fmt.Errorf("batch op %d (DeleteEdge): %w", i, err)
			}

		case BatchMoveNode:
			if _, err := s.walker.MoveNode(op.NodeID, op.ParentID); err != nil {
				return &BatchResult{Results: results}, fmt.Errorf("batch op %d (MoveNode): %w", i, err)
			}

		case BatchReorderNode:
			if _, err := s.walker.ReorderNode(op.NodeID, op.Position); err != nil {
				return &BatchResult{Results: results}, fmt.Errorf("batch op %d (ReorderNode): %w", i, err)
			}

		case BatchRestoreNode:
			if _, err := s.walker.RestoreNode(op.NodeID); err != nil {
				return &BatchResult{Results: results}, fmt.Errorf("batch op %d (RestoreNode): %w", i, err)
			}

		case BatchCascadeDelete:
			var cascadeDelete func(nodeID uuid.UUID) error
			cascadeDelete = func(nodeID uuid.UUID) error {
				for _, child := range s.walker.GetChildren(nodeID) {
					if err := cascadeDelete(child.ID); err != nil {
						return err
					}
				}
				for _, edge := range s.walker.GetOutEdges(nodeID) {
					if _, err := s.walker.DeleteEdge(edge.ID); err != nil {
						return err
					}
				}
				for _, edge := range s.walker.GetInEdges(nodeID) {
					if _, err := s.walker.DeleteEdge(edge.ID); err != nil {
						return err
					}
				}
				if node, ok := s.walker.GetNode(nodeID); ok {
					s.removeFromIndexes(nodeID, node.Type, node.Properties)
				}
				_, err := s.walker.DeleteNode(nodeID)
				return err
			}
			if err := cascadeDelete(op.NodeID); err != nil {
				return &BatchResult{Results: results}, fmt.Errorf("batch op %d (CascadeDelete): %w", i, err)
			}
		}
	}

	return &BatchResult{Results: results}, nil
}

// --- Sync ---

// ApplyRemote applies a remote operation.
func (s *Store) ApplyRemote(op *crdt.Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.walker.ApplyRemote(op)
}

// --- Index helpers ---

func (s *Store) updateIndexes(nodeID uuid.UUID, nodeType string, properties map[string]interface{}) {
	for k, v := range properties {
		s.addToIndex(nodeID, nodeType, k, v)
	}
}

func (s *Store) addToIndex(nodeID uuid.UUID, nodeType, property string, value interface{}) {
	key := indexKey{NodeType: nodeType, Property: property}
	valStr := indexValueString(value)
	if s.indexes[key] == nil {
		s.indexes[key] = make(map[string][]uuid.UUID)
	}
	s.indexes[key][valStr] = append(s.indexes[key][valStr], nodeID)
}

func (s *Store) removeFromIndex(nodeID uuid.UUID, nodeType, property string, value interface{}) {
	key := indexKey{NodeType: nodeType, Property: property}
	valStr := indexValueString(value)
	ids := s.indexes[key][valStr]
	for i, id := range ids {
		if id == nodeID {
			s.indexes[key][valStr] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
}

func (s *Store) removeFromIndexes(nodeID uuid.UUID, nodeType string, properties map[string]interface{}) {
	for k, v := range properties {
		s.removeFromIndex(nodeID, nodeType, k, v)
	}
}

func indexValueString(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
