package derived

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
	"github.com/kern/graphdb/internal/graph"
)

// DerivedNode is a node in the derived graph.
type DerivedNode struct {
	ID           uuid.UUID              `json:"id"`
	DerivedType  string                 `json:"derivedType"`
	SourceID     *uuid.UUID             `json:"sourceId,omitempty"`   // persistent source node, if any
	SourceType   string                 `json:"sourceType,omitempty"` // type of the source node
	Properties   map[string]interface{} `json:"properties"`
	ParentID     *uuid.UUID             `json:"parentId,omitempty"`
	CreatedAt    time.Time              `json:"createdAt"`
	UpdatedAt    time.Time              `json:"updatedAt"`
	// Which derivation produced this node
	DerivationID string `json:"derivationId,omitempty"`
	// For inheritance: which source subtree was inherited
	InheritedFrom *uuid.UUID `json:"inheritedFrom,omitempty"`
}

// DerivedEdge is an edge in the derived graph.
type DerivedEdge struct {
	ID         uuid.UUID              `json:"id"`
	Type       string                 `json:"type"`
	FromID     uuid.UUID              `json:"fromId"`
	ToID       uuid.UUID              `json:"toId"`
	Properties map[string]interface{} `json:"properties"`
	SourceID   *uuid.UUID             `json:"sourceId,omitempty"`
}

// DerivedSchema holds all type definitions for the derived graph.
type DerivedSchema struct {
	Types map[string]*DerivedNodeType `json:"types"`
}

// NewDerivedSchema creates a new empty derived schema.
func NewDerivedSchema() *DerivedSchema {
	return &DerivedSchema{
		Types: make(map[string]*DerivedNodeType),
	}
}

// Register adds a derived node type to the schema.
func (ds *DerivedSchema) Register(nodeType *DerivedNodeType) {
	ds.Types[nodeType.Name] = nodeType
}

// Get returns a derived node type by name.
func (ds *DerivedSchema) Get(name string) (*DerivedNodeType, bool) {
	t, ok := ds.Types[name]
	return t, ok
}

// Store is the materialized derived graph. It sits on top of graph.Store
// and reactively recomputes derived nodes when the source graph changes.
type Store struct {
	mu sync.RWMutex

	source *graph.Store
	schema *DerivedSchema

	// Materialized derived nodes
	nodes     map[uuid.UUID]*DerivedNode
	edges     map[uuid.UUID]*DerivedEdge
	parentMap map[uuid.UUID]uuid.UUID
	childMap  map[uuid.UUID][]uuid.UUID
	typeIndex map[string][]uuid.UUID

	// Source -> derived node mapping (one source can produce many derived nodes)
	sourceToDerived map[uuid.UUID][]uuid.UUID
	// Derived -> source mapping
	derivedToSource map[uuid.UUID]uuid.UUID

	// Registered derivation pipelines
	pipelines []*Pipeline

	// Change listeners
	listeners []func(event ChangeEvent)

	// Async recomputation
	pendingOps chan *crdt.Operation
	stopCh     chan struct{}
}

// ChangeEvent describes what changed in the derived graph.
type ChangeEvent struct {
	Type      ChangeType
	NodeID    uuid.UUID
	Node      *DerivedNode
	OldNode   *DerivedNode
	PipelineID string
}

type ChangeType int

const (
	ChangeInsert ChangeType = iota
	ChangeUpdate
	ChangeDelete
)

// NewStore creates a new derived store backed by a persistent source store.
func NewStore(source *graph.Store) *Store {
	ds := &Store{
		source:          source,
		schema:          NewDerivedSchema(),
		nodes:           make(map[uuid.UUID]*DerivedNode),
		edges:           make(map[uuid.UUID]*DerivedEdge),
		parentMap:       make(map[uuid.UUID]uuid.UUID),
		childMap:        make(map[uuid.UUID][]uuid.UUID),
		typeIndex:       make(map[string][]uuid.UUID),
		sourceToDerived: make(map[uuid.UUID][]uuid.UUID),
		derivedToSource: make(map[uuid.UUID]uuid.UUID),
	}

	// Listen for changes on the source graph to trigger recomputation.
	// IMPORTANT: The listener fires inside EGWalker.InsertNode which holds
	// the walker's write lock, so we MUST NOT read the walker synchronously.
	// Instead, queue the operation and process it asynchronously.
	ds.pendingOps = make(chan *crdt.Operation, 1024)
	source.Walker().Graph().AddListener(func(op *crdt.Operation) {
		select {
		case ds.pendingOps <- op:
		default:
			// Channel full — mark for full recompute
		}
	})

	go ds.processLoop()

	return ds
}

// Schema returns the derived schema.
func (ds *Store) Schema() *DerivedSchema {
	return ds.schema
}

// Source returns the underlying persistent store.
func (ds *Store) Source() *graph.Store {
	return ds.source
}

// RegisterType adds a derived node type definition.
func (ds *Store) RegisterType(nodeType *DerivedNodeType) {
	ds.schema.Register(nodeType)
}

// RegisterPipeline adds a derivation pipeline.
func (ds *Store) RegisterPipeline(p *Pipeline) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.pipelines = append(ds.pipelines, p)
}

// OnChange registers a listener for derived graph changes.
func (ds *Store) OnChange(fn func(ChangeEvent)) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.listeners = append(ds.listeners, fn)
}

// Recompute rebuilds the entire derived graph from scratch.
// Call this after registering all pipelines, or to force a full refresh.
func (ds *Store) Recompute() {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// Clear everything
	ds.nodes = make(map[uuid.UUID]*DerivedNode)
	ds.edges = make(map[uuid.UUID]*DerivedEdge)
	ds.parentMap = make(map[uuid.UUID]uuid.UUID)
	ds.childMap = make(map[uuid.UUID][]uuid.UUID)
	ds.typeIndex = make(map[string][]uuid.UUID)
	ds.sourceToDerived = make(map[uuid.UUID][]uuid.UUID)
	ds.derivedToSource = make(map[uuid.UUID]uuid.UUID)

	// Run each pipeline
	ctx := ds.buildContext()
	for _, p := range ds.pipelines {
		ds.runPipeline(p, ctx)
	}
}

// --- Read operations ---

// GetNode returns a derived node by ID.
func (ds *Store) GetNode(id uuid.UUID) (*DerivedNode, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	n, ok := ds.nodes[id]
	return n, ok
}

// GetNodesByType returns all derived nodes of a given type.
func (ds *Store) GetNodesByType(derivedType string) []*DerivedNode {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	var result []*DerivedNode
	for _, id := range ds.typeIndex[derivedType] {
		if n, ok := ds.nodes[id]; ok {
			result = append(result, n)
		}
	}
	return result
}

// GetChildren returns children of a derived node.
func (ds *Store) GetChildren(id uuid.UUID) []*DerivedNode {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	var result []*DerivedNode
	for _, childID := range ds.childMap[id] {
		if n, ok := ds.nodes[childID]; ok {
			result = append(result, n)
		}
	}
	return result
}

// GetParent returns the parent of a derived node.
func (ds *Store) GetParent(id uuid.UUID) (*DerivedNode, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	parentID, ok := ds.parentMap[id]
	if !ok {
		return nil, false
	}
	n, ok := ds.nodes[parentID]
	return n, ok
}

// GetSubtree returns a derived node and all its descendants.
func (ds *Store) GetSubtree(id uuid.UUID) []*DerivedNode {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	var result []*DerivedNode
	var walk func(nodeID uuid.UUID)
	walk = func(nodeID uuid.UUID) {
		n, ok := ds.nodes[nodeID]
		if !ok {
			return
		}
		result = append(result, n)
		for _, childID := range ds.childMap[nodeID] {
			walk(childID)
		}
	}
	walk(id)
	return result
}

// GetDerivedForSource returns all derived nodes produced from a source node.
func (ds *Store) GetDerivedForSource(sourceID uuid.UUID) []*DerivedNode {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	var result []*DerivedNode
	for _, id := range ds.sourceToDerived[sourceID] {
		if n, ok := ds.nodes[id]; ok {
			result = append(result, n)
		}
	}
	return result
}

// GetEdge returns a derived edge by ID.
func (ds *Store) GetEdge(id uuid.UUID) (*DerivedEdge, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	e, ok := ds.edges[id]
	return e, ok
}

// GetOutEdges returns outgoing edges from a derived node.
func (ds *Store) GetOutEdges(id uuid.UUID) []*DerivedEdge {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	var result []*DerivedEdge
	for _, e := range ds.edges {
		if e.FromID == id {
			result = append(result, e)
		}
	}
	return result
}

// GetInEdges returns incoming edges to a derived node.
func (ds *Store) GetInEdges(id uuid.UUID) []*DerivedEdge {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	var result []*DerivedEdge
	for _, e := range ds.edges {
		if e.ToID == id {
			result = append(result, e)
		}
	}
	return result
}

// AllNodes returns all derived nodes.
func (ds *Store) AllNodes() []*DerivedNode {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	result := make([]*DerivedNode, 0, len(ds.nodes))
	for _, n := range ds.nodes {
		result = append(result, n)
	}
	return result
}

// --- Internal: node insertion/update/removal ---

func (ds *Store) insertDerivedNode(node *DerivedNode) {
	ds.nodes[node.ID] = node
	ds.typeIndex[node.DerivedType] = append(ds.typeIndex[node.DerivedType], node.ID)

	if node.SourceID != nil {
		ds.sourceToDerived[*node.SourceID] = append(ds.sourceToDerived[*node.SourceID], node.ID)
		ds.derivedToSource[node.ID] = *node.SourceID
	}

	if node.ParentID != nil {
		ds.parentMap[node.ID] = *node.ParentID
		ds.childMap[*node.ParentID] = append(ds.childMap[*node.ParentID], node.ID)
	}

	ds.notify(ChangeEvent{Type: ChangeInsert, NodeID: node.ID, Node: node})
}

func (ds *Store) updateDerivedNode(node *DerivedNode) {
	old := ds.nodes[node.ID]
	ds.nodes[node.ID] = node
	ds.notify(ChangeEvent{Type: ChangeUpdate, NodeID: node.ID, Node: node, OldNode: old})
}

func (ds *Store) removeDerivedNode(id uuid.UUID) {
	node, ok := ds.nodes[id]
	if !ok {
		return
	}

	// Remove children recursively
	children := make([]uuid.UUID, len(ds.childMap[id]))
	copy(children, ds.childMap[id])
	for _, childID := range children {
		ds.removeDerivedNode(childID)
	}

	// Remove from indexes
	delete(ds.nodes, id)
	if node.ParentID != nil {
		cs := ds.childMap[*node.ParentID]
		for i, cid := range cs {
			if cid == id {
				ds.childMap[*node.ParentID] = append(cs[:i], cs[i+1:]...)
				break
			}
		}
		delete(ds.parentMap, id)
	}
	if node.SourceID != nil {
		ids := ds.sourceToDerived[*node.SourceID]
		for i, did := range ids {
			if did == id {
				ds.sourceToDerived[*node.SourceID] = append(ids[:i], ids[i+1:]...)
				break
			}
		}
		delete(ds.derivedToSource, id)
	}
	// Remove from type index
	tids := ds.typeIndex[node.DerivedType]
	for i, tid := range tids {
		if tid == id {
			ds.typeIndex[node.DerivedType] = append(tids[:i], tids[i+1:]...)
			break
		}
	}

	ds.notify(ChangeEvent{Type: ChangeDelete, NodeID: id, OldNode: node})
}

func (ds *Store) insertDerivedEdge(edge *DerivedEdge) {
	ds.edges[edge.ID] = edge
}

func (ds *Store) notify(event ChangeEvent) {
	for _, fn := range ds.listeners {
		fn(event)
	}
}

// --- Internal: derivation context ---

func (ds *Store) buildContext() *DerivationContext {
	walker := ds.source.Walker()
	return &DerivationContext{
		sourceNodes:       walker.GetNode,
		sourceByType:      walker.GetNodesByType,
		sourceChildren:    walker.GetChildren,
		sourceParent:      walker.GetParent,
		sourceOutEdges:    walker.GetOutEdges,
		sourceInEdges:     walker.GetInEdges,
		derivedNodes:      ds.nodes,
		sourceToDerivedID: ds.buildSourceToDerivedIDMap(),
	}
}

func (ds *Store) buildSourceToDerivedIDMap() map[uuid.UUID]uuid.UUID {
	m := make(map[uuid.UUID]uuid.UUID)
	for sourceID, derivedIDs := range ds.sourceToDerived {
		if len(derivedIDs) > 0 {
			m[sourceID] = derivedIDs[0]
		}
	}
	return m
}

// --- Internal: pipeline execution ---

func (ds *Store) runPipeline(p *Pipeline, ctx *DerivationContext) {
	for _, stage := range p.Stages {
		ds.runStage(p.ID, stage, ctx)
	}
}

func (ds *Store) runStage(pipelineID string, stage *Stage, ctx *DerivationContext) {
	switch stage.Type {
	case StageMap:
		ds.runMapStage(pipelineID, stage, ctx)
	case StageSubtreeInherit:
		ds.runSubtreeInheritStage(pipelineID, stage, ctx)
	case StageJoin:
		ds.runJoinStage(pipelineID, stage, ctx)
	case StageMultiSubtree:
		ds.runMultiSubtreeStage(pipelineID, stage, ctx)
	case StageComputed:
		ds.runComputedStage(pipelineID, stage, ctx)
	}
}

func (ds *Store) runMapStage(pipelineID string, stage *Stage, ctx *DerivationContext) {
	sourceNodes := ctx.SourceNodesByType(stage.SourceType)
	for _, sn := range sourceNodes {
		if stage.Filter != nil && !stage.Filter(sn) {
			continue
		}

		props := sn.Properties
		if stage.Transform != nil {
			props = stage.Transform(sn, ctx)
		}

		// Validate and apply defaults if we have a type def
		if typeDef, ok := ds.schema.Get(stage.DerivedType); ok {
			props = typeDef.ApplyDefaults(props)
		}

		derivedID := uuid.New()
		var parentID *uuid.UUID
		if sn.ParentID != nil {
			// Check if parent was already derived
			if parentDerivedIDs, ok := ds.sourceToDerived[*sn.ParentID]; ok && len(parentDerivedIDs) > 0 {
				pid := parentDerivedIDs[0]
				parentID = &pid
			}
		}

		node := &DerivedNode{
			ID:           derivedID,
			DerivedType:  stage.DerivedType,
			SourceID:     &sn.ID,
			SourceType:   sn.Type,
			Properties:   props,
			ParentID:     parentID,
			CreatedAt:    sn.CreatedAt,
			UpdatedAt:    sn.UpdatedAt,
			DerivationID: pipelineID,
		}
		ds.insertDerivedNode(node)

		// Recursively derive children if requested
		if stage.DeriveChildren {
			ds.deriveChildrenRecursive(pipelineID, stage, sn.ID, derivedID, ctx)
		}
	}
}

func (ds *Store) deriveChildrenRecursive(pipelineID string, stage *Stage, sourceParentID, derivedParentID uuid.UUID, ctx *DerivationContext) {
	children := ctx.SourceChildren(sourceParentID)
	for _, child := range children {
		// Find the appropriate rule for this child type
		childStage := stage
		if stage.ChildRules != nil {
			if cs, ok := stage.ChildRules[child.Type]; ok {
				childStage = cs
			}
		}

		if childStage.Filter != nil && !childStage.Filter(child) {
			continue
		}

		props := child.Properties
		if childStage.Transform != nil {
			props = childStage.Transform(child, ctx)
		}

		derivedType := childStage.DerivedType
		if derivedType == "" {
			derivedType = child.Type
		}

		if typeDef, ok := ds.schema.Get(derivedType); ok {
			props = typeDef.ApplyDefaults(props)
		}

		childID := uuid.New()
		pid := derivedParentID
		childNode := &DerivedNode{
			ID:           childID,
			DerivedType:  derivedType,
			SourceID:     &child.ID,
			SourceType:   child.Type,
			Properties:   props,
			ParentID:     &pid,
			CreatedAt:    child.CreatedAt,
			UpdatedAt:    child.UpdatedAt,
			DerivationID: pipelineID,
		}
		ds.insertDerivedNode(childNode)

		// Recurse
		ds.deriveChildrenRecursive(pipelineID, childStage, child.ID, childID, ctx)
	}
}

func (ds *Store) runSubtreeInheritStage(pipelineID string, stage *Stage, ctx *DerivationContext) {
	inh := stage.Inheritance
	if inh == nil {
		return
	}

	// Find all instance nodes
	instances := ctx.SourceNodesByType(inh.InstanceType)
	for _, inst := range instances {
		if stage.Filter != nil && !stage.Filter(inst) {
			continue
		}

		// Get the component reference
		refID, ok := inst.Properties[inh.RefProperty].(string)
		if !ok {
			continue
		}
		componentID, err := uuid.Parse(refID)
		if err != nil {
			continue
		}

		component, ok := ctx.SourceNode(componentID)
		if !ok {
			continue
		}

		// Create the derived instance node with merged properties
		mergedProps := MergeProperties(component.Properties, inst.Properties, inh.Strategy)

		// Remove excluded properties
		for _, ex := range inh.ExcludeProperties {
			delete(mergedProps, ex)
		}

		// Apply custom transform
		if inh.PropertyTransform != nil {
			mergedProps = inh.PropertyTransform(inst, ctx)
		}
		if stage.Transform != nil {
			mergedProps = stage.Transform(inst, ctx)
		}

		derivedType := stage.DerivedType
		if derivedType == "" {
			derivedType = inst.Type
		}

		if typeDef, ok := ds.schema.Get(derivedType); ok {
			mergedProps = typeDef.ApplyDefaults(mergedProps)
		}

		instDerivedID := uuid.New()
		var parentID *uuid.UUID
		if inst.ParentID != nil {
			if pdIDs, ok := ds.sourceToDerived[*inst.ParentID]; ok && len(pdIDs) > 0 {
				pid := pdIDs[0]
				parentID = &pid
			}
		}

		instNode := &DerivedNode{
			ID:            instDerivedID,
			DerivedType:   derivedType,
			SourceID:      &inst.ID,
			SourceType:    inst.Type,
			Properties:    mergedProps,
			ParentID:      parentID,
			CreatedAt:     inst.CreatedAt,
			UpdatedAt:     inst.UpdatedAt,
			DerivationID:  pipelineID,
			InheritedFrom: &componentID,
		}
		ds.insertDerivedNode(instNode)

		// If SubtreeInherit: clone the component's subtree under the instance
		if inh.Strategy == InheritSubtree {
			ds.inheritSubtree(pipelineID, inh, componentID, inst.ID, instDerivedID, ctx)
		}
	}
}

// inheritSubtree clones a source subtree under a derived parent,
// merging with any instance-level overrides.
func (ds *Store) inheritSubtree(pipelineID string, inh *SubtreeDerivation, componentID, instanceID, derivedParentID uuid.UUID, ctx *DerivationContext) {
	componentChildren := ctx.SourceChildren(componentID)
	instanceChildren := ctx.SourceChildren(instanceID)

	// Build a map of instance children by type+name for matching overrides
	instanceOverrides := make(map[string]*crdt.MaterializedNode)
	for _, ic := range instanceChildren {
		key := ic.Type
		if name, ok := ic.Properties["name"].(string); ok {
			key += ":" + name
		}
		instanceOverrides[key] = ic
	}

	for _, cc := range componentChildren {
		// Check child type filter
		if len(inh.ChildTypeMapping) > 0 {
			if _, ok := inh.ChildTypeMapping[cc.Type]; !ok {
				continue
			}
		}

		// Check for instance override
		key := cc.Type
		if name, ok := cc.Properties["name"].(string); ok {
			key += ":" + name
		}

		props := copyProps(cc.Properties)
		var sourceID uuid.UUID
		if override, ok := instanceOverrides[key]; ok {
			props = MergeProperties(cc.Properties, override.Properties, inh.Strategy)
			sourceID = override.ID
		} else {
			sourceID = cc.ID
		}

		derivedType := cc.Type
		if mapped, ok := inh.ChildTypeMapping[cc.Type]; ok {
			derivedType = mapped
		}

		childID := uuid.New()
		pid := derivedParentID
		sid := sourceID
		childNode := &DerivedNode{
			ID:            childID,
			DerivedType:   derivedType,
			SourceID:      &sid,
			SourceType:    cc.Type,
			Properties:    props,
			ParentID:      &pid,
			CreatedAt:     cc.CreatedAt,
			UpdatedAt:     time.Now(),
			DerivationID:  pipelineID,
			InheritedFrom: &componentID,
		}
		ds.insertDerivedNode(childNode)

		// Recurse into the component child's subtree
		ds.inheritSubtree(pipelineID, inh, cc.ID, instanceID, childID, ctx)
	}

	// Also add instance-only children (not in component)
	for _, ic := range instanceChildren {
		key := ic.Type
		if name, ok := ic.Properties["name"].(string); ok {
			key += ":" + name
		}
		// Skip if already merged above
		found := false
		for _, cc := range componentChildren {
			ccKey := cc.Type
			if name, ok := cc.Properties["name"].(string); ok {
				ccKey += ":" + name
			}
			if ccKey == key {
				found = true
				break
			}
		}
		if found {
			continue
		}

		childID := uuid.New()
		pid := derivedParentID
		sid := ic.ID
		childNode := &DerivedNode{
			ID:           childID,
			DerivedType:  ic.Type,
			SourceID:     &sid,
			SourceType:   ic.Type,
			Properties:   copyProps(ic.Properties),
			ParentID:     &pid,
			CreatedAt:    ic.CreatedAt,
			UpdatedAt:    ic.UpdatedAt,
			DerivationID: pipelineID,
		}
		ds.insertDerivedNode(childNode)
	}
}

func (ds *Store) runJoinStage(pipelineID string, stage *Stage, ctx *DerivationContext) {
	if stage.Join == nil {
		return
	}
	j := stage.Join

	sourceNodes := ctx.SourceNodesByType(j.SourceType)
	for _, sn := range sourceNodes {
		if stage.Filter != nil && !stage.Filter(sn) {
			continue
		}

		// Resolve related nodes
		related := ds.resolveRelated(sn, j, ctx)

		// Build properties from source + related
		props := make(map[string]interface{})
		for k, v := range sn.Properties {
			props[k] = v
		}
		for relName, relNodes := range related {
			if len(relNodes) == 1 {
				props[relName] = relNodes[0].Properties
			} else if len(relNodes) > 1 {
				relProps := make([]interface{}, len(relNodes))
				for i, rn := range relNodes {
					relProps[i] = rn.Properties
				}
				props[relName] = relProps
			}
		}

		if stage.Transform != nil {
			props = stage.Transform(sn, ctx)
		}

		derivedType := stage.DerivedType
		if typeDef, ok := ds.schema.Get(derivedType); ok {
			props = typeDef.ApplyDefaults(props)
		}

		derivedID := uuid.New()
		node := &DerivedNode{
			ID:           derivedID,
			DerivedType:  derivedType,
			SourceID:     &sn.ID,
			SourceType:   sn.Type,
			Properties:   props,
			CreatedAt:    sn.CreatedAt,
			UpdatedAt:    sn.UpdatedAt,
			DerivationID: pipelineID,
		}
		ds.insertDerivedNode(node)
	}
}

func (ds *Store) resolveRelated(node *crdt.MaterializedNode, j *JoinDef, ctx *DerivationContext) map[string][]*crdt.MaterializedNode {
	result := make(map[string][]*crdt.MaterializedNode)

	for _, rel := range j.Relations {
		switch rel.Via {
		case RelViaProperty:
			// Follow a ref property to a target node
			if refID, ok := node.Properties[rel.Property].(string); ok {
				if uid, err := uuid.Parse(refID); err == nil {
					if target, ok := ctx.SourceNode(uid); ok {
						result[rel.Name] = append(result[rel.Name], target)
					}
				}
			}
		case RelViaParent:
			if parent, ok := ctx.SourceParent(node.ID); ok {
				result[rel.Name] = append(result[rel.Name], parent)
			}
		case RelViaChildren:
			children := ctx.SourceChildren(node.ID)
			for _, child := range children {
				if rel.TargetType == "" || child.Type == rel.TargetType {
					result[rel.Name] = append(result[rel.Name], child)
				}
			}
		case RelViaOutEdge:
			edges := ctx.SourceOutEdges(node.ID)
			for _, edge := range edges {
				if rel.EdgeType == "" || edge.Type == rel.EdgeType {
					if target, ok := ctx.SourceNode(edge.ToID); ok {
						if rel.TargetType == "" || target.Type == rel.TargetType {
							result[rel.Name] = append(result[rel.Name], target)
						}
					}
				}
			}
		case RelViaInEdge:
			edges := ctx.SourceInEdges(node.ID)
			for _, edge := range edges {
				if rel.EdgeType == "" || edge.Type == rel.EdgeType {
					if target, ok := ctx.SourceNode(edge.FromID); ok {
						if rel.TargetType == "" || target.Type == rel.TargetType {
							result[rel.Name] = append(result[rel.Name], target)
						}
					}
				}
			}
		}
	}
	return result
}

func (ds *Store) runMultiSubtreeStage(pipelineID string, stage *Stage, ctx *DerivationContext) {
	ms := stage.MultiSubtree
	if ms == nil {
		return
	}

	// Find all target parent nodes
	parents := ctx.SourceNodesByType(ms.ParentType)
	for _, parent := range parents {
		if stage.Filter != nil && !stage.Filter(parent) {
			continue
		}

		// Create the derived parent node
		parentProps := parent.Properties
		if stage.Transform != nil {
			parentProps = stage.Transform(parent, ctx)
		}

		derivedType := stage.DerivedType
		if derivedType == "" {
			derivedType = parent.Type
		}

		derivedParentID := uuid.New()
		parentNode := &DerivedNode{
			ID:           derivedParentID,
			DerivedType:  derivedType,
			SourceID:     &parent.ID,
			SourceType:   parent.Type,
			Properties:   parentProps,
			CreatedAt:    parent.CreatedAt,
			UpdatedAt:    parent.UpdatedAt,
			DerivationID: pipelineID,
		}
		ds.insertDerivedNode(parentNode)

		// For each subtree source, derive it as children of this parent
		for _, src := range ms.Sources {
			ds.deriveMultiSubtreeSource(pipelineID, src, parent, derivedParentID, ctx)
		}
	}
}

func (ds *Store) deriveMultiSubtreeSource(pipelineID string, src *SubtreeSource, parent *crdt.MaterializedNode, derivedParentID uuid.UUID, ctx *DerivationContext) {
	var sourceRoots []*crdt.MaterializedNode

	switch src.ResolveVia {
	case ResolveProperty:
		// Property on parent references a source node
		if refID, ok := parent.Properties[src.Property].(string); ok {
			if uid, err := uuid.Parse(refID); err == nil {
				if node, ok := ctx.SourceNode(uid); ok {
					sourceRoots = append(sourceRoots, node)
				}
			}
		}
	case ResolvePropertySlice:
		// Property on parent is a list of references
		if refs, ok := parent.Properties[src.Property].([]interface{}); ok {
			for _, ref := range refs {
				if refStr, ok := ref.(string); ok {
					if uid, err := uuid.Parse(refStr); err == nil {
						if node, ok := ctx.SourceNode(uid); ok {
							sourceRoots = append(sourceRoots, node)
						}
					}
				}
			}
		}
	case ResolveEdge:
		edges := ctx.SourceOutEdges(parent.ID)
		for _, edge := range edges {
			if src.EdgeType == "" || edge.Type == src.EdgeType {
				if node, ok := ctx.SourceNode(edge.ToID); ok {
					if src.SourceType == "" || node.Type == src.SourceType {
						sourceRoots = append(sourceRoots, node)
					}
				}
			}
		}
	case ResolveChildren:
		children := ctx.SourceChildren(parent.ID)
		for _, child := range children {
			if src.SourceType == "" || child.Type == src.SourceType {
				sourceRoots = append(sourceRoots, child)
			}
		}
	case ResolveQuery:
		// Use a query function to find source nodes
		if src.QueryFunc != nil {
			sourceRoots = src.QueryFunc(parent, ctx)
		}
	}

	// Apply filter
	for _, root := range sourceRoots {
		if src.Filter != nil && !src.Filter(root) {
			continue
		}

		// Create derived child from this source subtree root
		props := root.Properties
		if src.Transform != nil {
			props = src.Transform(root, ctx)
		}

		derivedType := src.DerivedType
		if derivedType == "" {
			derivedType = root.Type
		}

		childID := uuid.New()
		pid := derivedParentID
		sid := root.ID
		childNode := &DerivedNode{
			ID:           childID,
			DerivedType:  derivedType,
			SourceID:     &sid,
			SourceType:   root.Type,
			Properties:   props,
			ParentID:     &pid,
			CreatedAt:    root.CreatedAt,
			UpdatedAt:    root.UpdatedAt,
			DerivationID: pipelineID,
		}
		ds.insertDerivedNode(childNode)

		// Recursively clone children if requested
		if src.IncludeChildren {
			ds.cloneSubtreeChildren(pipelineID, root.ID, childID, src, ctx)
		}
	}
}

func (ds *Store) cloneSubtreeChildren(pipelineID string, sourceParentID, derivedParentID uuid.UUID, src *SubtreeSource, ctx *DerivationContext) {
	children := ctx.SourceChildren(sourceParentID)
	for _, child := range children {
		if src.ChildFilter != nil && !src.ChildFilter(child) {
			continue
		}

		props := child.Properties
		if src.ChildTransform != nil {
			props = src.ChildTransform(child, ctx)
		}

		derivedType := child.Type
		if src.ChildTypeMapping != nil {
			if mapped, ok := src.ChildTypeMapping[child.Type]; ok {
				derivedType = mapped
			}
		}

		childID := uuid.New()
		pid := derivedParentID
		sid := child.ID
		childNode := &DerivedNode{
			ID:           childID,
			DerivedType:  derivedType,
			SourceID:     &sid,
			SourceType:   child.Type,
			Properties:   props,
			ParentID:     &pid,
			CreatedAt:    child.CreatedAt,
			UpdatedAt:    child.UpdatedAt,
			DerivationID: pipelineID,
		}
		ds.insertDerivedNode(childNode)

		ds.cloneSubtreeChildren(pipelineID, child.ID, childID, src, ctx)
	}
}

func (ds *Store) runComputedStage(pipelineID string, stage *Stage, ctx *DerivationContext) {
	if stage.ComputeFunc == nil {
		return
	}

	// ComputeFunc has full control: it reads from source, writes to derived
	computeCtx := &ComputeContext{
		DerivationContext: ctx,
		store:             ds,
		pipelineID:        pipelineID,
	}
	stage.ComputeFunc(computeCtx)
}

// Stop shuts down the derived store's background processing.
func (ds *Store) Stop() {
	if ds.stopCh != nil {
		close(ds.stopCh)
	}
}

// processLoop drains pending operations and recomputes affected pipelines.
// Runs in a background goroutine to avoid deadlocks with the walker's mutex.
func (ds *Store) processLoop() {
	ds.stopCh = make(chan struct{})
	for {
		select {
		case <-ds.stopCh:
			return
		case op := <-ds.pendingOps:
			ds.onSourceChange(op)
			// Drain any additional pending ops in a batch
		drain:
			for {
				select {
				case op = <-ds.pendingOps:
					ds.onSourceChange(op)
				default:
					break drain
				}
			}
		}
	}
}

// --- Internal: reactive recomputation ---

func (ds *Store) onSourceChange(op *crdt.Operation) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// Determine which pipelines are affected
	ctx := ds.buildContext()
	for _, p := range ds.pipelines {
		if ds.pipelineAffectedBy(p, op) {
			ds.clearPipelineResults(p.ID)
			ds.runPipeline(p, ctx)
		}
	}
}

func (ds *Store) pipelineAffectedBy(p *Pipeline, op *crdt.Operation) bool {
	for _, stage := range p.Stages {
		if stage.SourceType == op.NodeType {
			return true
		}
		if stage.Inheritance != nil {
			if stage.Inheritance.InstanceType == op.NodeType || stage.Inheritance.SourceType == op.NodeType {
				return true
			}
		}
		if stage.MultiSubtree != nil && stage.MultiSubtree.ParentType == op.NodeType {
			return true
		}
		if stage.Join != nil && stage.Join.SourceType == op.NodeType {
			return true
		}
		// For property changes on existing nodes, check if the target is a source
		if op.Type == crdt.OpSetProperty || op.Type == crdt.OpDeleteProperty {
			if _, ok := ds.sourceToDerived[op.TargetID]; ok {
				return true
			}
		}
	}
	return false
}

func (ds *Store) clearPipelineResults(pipelineID string) {
	var toRemove []uuid.UUID
	for id, node := range ds.nodes {
		if node.DerivationID == pipelineID {
			toRemove = append(toRemove, id)
		}
	}
	for _, id := range toRemove {
		// Simple removal without recursive (we're clearing all pipeline results)
		node := ds.nodes[id]
		delete(ds.nodes, id)
		if node.ParentID != nil {
			cs := ds.childMap[*node.ParentID]
			for i, cid := range cs {
				if cid == id {
					ds.childMap[*node.ParentID] = append(cs[:i], cs[i+1:]...)
					break
				}
			}
			delete(ds.parentMap, id)
		}
		if node.SourceID != nil {
			ids := ds.sourceToDerived[*node.SourceID]
			for i, did := range ids {
				if did == id {
					ds.sourceToDerived[*node.SourceID] = append(ids[:i], ids[i+1:]...)
					break
				}
			}
			delete(ds.derivedToSource, id)
		}
		tids := ds.typeIndex[node.DerivedType]
		for i, tid := range tids {
			if tid == id {
				ds.typeIndex[node.DerivedType] = append(tids[:i], tids[i+1:]...)
				break
			}
		}
	}

	// Also clear edges from this pipeline
	var edgesToRemove []uuid.UUID
	for id, edge := range ds.edges {
		if edge.SourceID != nil {
			if _, ok := ds.derivedToSource[*edge.SourceID]; !ok {
				edgesToRemove = append(edgesToRemove, id)
			}
		}
	}
	for _, id := range edgesToRemove {
		delete(ds.edges, id)
	}
}

func copyProps(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	result := make(map[string]interface{}, len(src))
	for k, v := range src {
		result[k] = v
	}
	return result
}

// Ensure uuid.Parse is importable
var _ = fmt.Sprintf
