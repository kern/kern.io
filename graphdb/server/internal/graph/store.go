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
