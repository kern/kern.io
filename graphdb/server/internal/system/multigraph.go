package system

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kern/graphdb/internal/derived"
	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
)

// MultiGraph manages multiple named graph instances, each with its own
// schema, derivations, invariants, and derived store.
// This is the server-side equivalent of multi-tenant graph storage.
type MultiGraph struct {
	mu        sync.RWMutex
	replicaID string
	graphs    map[string]*GraphInstance
}

// GraphInstance is a single named graph with all its associated state.
type GraphInstance struct {
	Name         string                         `json:"name"`
	Store        *graph.Store                   `json:"-"`
	DerivedStore *derived.Store                 `json:"-"`
	Validator    *invariant.Validator            `json:"-"`
	IncValidator *invariant.IncrementalValidator `json:"-"`
	SchemaJSON   json.RawMessage                `json:"schema,omitempty"`
	// FeatureFlags enable conditional module activation
	FeatureFlags map[string]bool                `json:"featureFlags,omitempty"`
	// ActiveModules tracks which modules have been applied
	ActiveModules []string                      `json:"activeModules,omitempty"`
}

// NewMultiGraph creates a new multi-graph manager.
func NewMultiGraph(replicaID string) *MultiGraph {
	return &MultiGraph{
		replicaID: replicaID,
		graphs:    make(map[string]*GraphInstance),
	}
}

// Create creates a new named graph.
func (mg *MultiGraph) Create(name string) (*GraphInstance, error) {
	mg.mu.Lock()
	defer mg.mu.Unlock()

	if _, exists := mg.graphs[name]; exists {
		return nil, fmt.Errorf("graph %q already exists", name)
	}

	store := graph.NewStore(mg.replicaID)
	instance := &GraphInstance{
		Name:         name,
		Store:        store,
		DerivedStore: derived.NewStore(store),
		Validator:    invariant.NewValidator(),
		IncValidator: invariant.NewIncrementalValidator(),
	}
	mg.graphs[name] = instance
	return instance, nil
}

// Get returns a named graph instance.
func (mg *MultiGraph) Get(name string) (*GraphInstance, bool) {
	mg.mu.RLock()
	defer mg.mu.RUnlock()
	g, ok := mg.graphs[name]
	return g, ok
}

// GetOrCreate returns an existing graph or creates a new one.
func (mg *MultiGraph) GetOrCreate(name string) *GraphInstance {
	mg.mu.Lock()
	defer mg.mu.Unlock()

	if g, ok := mg.graphs[name]; ok {
		return g
	}

	store := graph.NewStore(mg.replicaID)
	instance := &GraphInstance{
		Name:         name,
		Store:        store,
		DerivedStore: derived.NewStore(store),
		Validator:    invariant.NewValidator(),
		IncValidator: invariant.NewIncrementalValidator(),
	}
	mg.graphs[name] = instance
	return instance
}

// Delete removes a named graph and stops its derived store.
func (mg *MultiGraph) Delete(name string) error {
	mg.mu.Lock()
	defer mg.mu.Unlock()

	g, ok := mg.graphs[name]
	if !ok {
		return fmt.Errorf("graph %q not found", name)
	}
	g.DerivedStore.Stop()
	delete(mg.graphs, name)
	return nil
}

// List returns the names of all graphs.
func (mg *MultiGraph) List() []string {
	mg.mu.RLock()
	defer mg.mu.RUnlock()
	names := make([]string, 0, len(mg.graphs))
	for name := range mg.graphs {
		names = append(names, name)
	}
	return names
}

// Count returns the number of graphs.
func (mg *MultiGraph) Count() int {
	mg.mu.RLock()
	defer mg.mu.RUnlock()
	return len(mg.graphs)
}

// LoadSchema applies a compiled schema definition to a graph.
// The schema JSON is produced by the TypeScript schema compiler.
func (mg *MultiGraph) LoadSchema(graphName string, schemaJSON []byte) error {
	g := mg.GetOrCreate(graphName)

	var compiled CompiledSchema
	if err := json.Unmarshal(schemaJSON, &compiled); err != nil {
		return fmt.Errorf("invalid schema JSON: %w", err)
	}

	if err := mg.applyCompiledSchema(g, &compiled); err != nil {
		return err
	}

	// Resolve and apply conditional modules
	if len(compiled.Modules) > 0 {
		if err := mg.ResolveModules(graphName, compiled.Modules); err != nil {
			return fmt.Errorf("module resolution: %w", err)
		}
	}

	return nil
}

// CompiledSchema is the JSON format produced by the TypeScript schema compiler.
// It contains all node types, edge types, invariants, and derivation pipeline
// definitions needed to configure a graph instance.
type CompiledSchema struct {
	Version    int                     `json:"version"`
	GraphName  string                  `json:"graphName"`
	NodeTypes  []CompiledNodeType      `json:"nodeTypes"`
	EdgeTypes  []CompiledEdgeType      `json:"edgeTypes"`
	Invariants []CompiledInvariant     `json:"invariants"`
	Pipelines  []CompiledPipeline      `json:"pipelines"`
	Modules    []CompiledModule        `json:"modules,omitempty"`
}

// CompiledNodeType is a node type from the compiled schema.
type CompiledNodeType struct {
	Name           string                       `json:"name"`
	Properties     map[string]CompiledProperty  `json:"properties"`
	AllowedChildren []string                    `json:"allowedChildren,omitempty"`
	AllowedParents  []string                    `json:"allowedParents,omitempty"`
}

// CompiledProperty is a property definition.
type CompiledProperty struct {
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Indexed  bool   `json:"indexed"`
	Unique   bool   `json:"unique"`
}

// CompiledEdgeType is an edge type from the compiled schema.
type CompiledEdgeType struct {
	Name      string   `json:"name"`
	FromTypes []string `json:"fromTypes"`
	ToTypes   []string `json:"toTypes"`
}

// CompiledInvariant is an invariant from the compiled schema.
type CompiledInvariant struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Config json.RawMessage `json:"config"`
}

// CompiledPipeline is a derivation pipeline definition.
type CompiledPipeline struct {
	ID     string               `json:"id"`
	Name   string               `json:"name"`
	Stages []CompiledStage      `json:"stages"`
}

// CompiledModule is an independently defined schema fragment that can be
// conditionally applied to a graph. Modules enable composable subsystems.
type CompiledModule struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Namespace  string              `json:"namespace,omitempty"`
	NodeTypes  []CompiledNodeType  `json:"nodeTypes"`
	EdgeTypes  []CompiledEdgeType  `json:"edgeTypes"`
	Invariants []CompiledInvariant `json:"invariants"`
	Pipelines  []CompiledPipeline  `json:"pipelines"`
	Condition  *ModuleCondition    `json:"condition,omitempty"`
	DependsOn  []string            `json:"dependsOn,omitempty"`
}

// ModuleCondition determines when a module is active.
type ModuleCondition struct {
	// Module activates when node type starts with this prefix
	NodeTypePrefix string `json:"nodeTypePrefix,omitempty"`
	// Module activates when all listed feature flags are present
	FeatureFlags []string `json:"featureFlags,omitempty"`
	// Custom expression (CEL-like) evaluated at runtime
	Expression string `json:"expression,omitempty"`
}

// CompiledStage is a pipeline stage definition.
type CompiledStage struct {
	Type       string                         `json:"type"` // "map", "join", "computed"
	SourceType string                         `json:"sourceType,omitempty"`
	DerivedType string                        `json:"derivedType,omitempty"`
	Transform  *derived.SerializableTransform `json:"transform,omitempty"`
}

// propTypeMap maps string property types to graph.PropType constants.
var propTypeMap = map[string]graph.PropertyType{
	"string":  graph.PropString,
	"number":  graph.PropNumber,
	"boolean": graph.PropBool,
	"array":   graph.PropArray,
	"object":  graph.PropObject,
	"ref":     graph.PropRef,
	"any":     graph.PropAny,
}

func (mg *MultiGraph) applyCompiledSchema(g *GraphInstance, compiled *CompiledSchema) error {
	schema := graph.NewSchema()

	// Apply node types
	for _, nt := range compiled.NodeTypes {
		props := make(map[string]*graph.PropertyDef)
		for name, p := range nt.Properties {
			propType, ok := propTypeMap[p.Type]
			if !ok {
				propType = graph.PropAny
			}
			props[name] = &graph.PropertyDef{
				Name:     name,
				Type:     propType,
				Required: p.Required,
				Indexed:  p.Indexed,
				Unique:   p.Unique,
			}
		}
		schema.DefineNode(&graph.NodeTypeDef{
			Name:            nt.Name,
			Properties:      props,
			AllowedChildren: nt.AllowedChildren,
			AllowedParents:  nt.AllowedParents,
		})
	}

	// Apply edge types
	for _, et := range compiled.EdgeTypes {
		schema.DefineEdge(&graph.EdgeTypeDef{
			Name:      et.Name,
			FromTypes: et.FromTypes,
			ToTypes:   et.ToTypes,
		})
	}

	g.Store.SetSchema(schema)

	// Apply invariants
	for _, ci := range compiled.Invariants {
		inv, err := buildInvariant(ci)
		if err != nil {
			return fmt.Errorf("invariant %s: %w", ci.ID, err)
		}
		g.Validator.Add(inv)
		g.IncValidator.Add(inv)
	}

	// Apply derivation pipelines with serializable transforms
	for _, cp := range compiled.Pipelines {
		pipeline := derived.NewPipeline(cp.ID, cp.Name)
		for _, stage := range cp.Stages {
			s := &derived.Stage{
				SourceType:            stage.SourceType,
				DerivedType:           stage.DerivedType,
				SerializableTransform: stage.Transform,
			}
			switch stage.Type {
			case "map":
				s.Type = derived.StageMap
			case "join":
				s.Type = derived.StageJoin
			case "computed":
				s.Type = derived.StageComputed
			}
			pipeline.AddStage(s)
		}
		g.DerivedStore.RegisterPipeline(pipeline)
	}

	// Store schema JSON for retrieval
	schemaBytes, _ := json.Marshal(compiled)
	g.SchemaJSON = schemaBytes

	return nil
}

func buildInvariant(ci CompiledInvariant) (*invariant.Invariant, error) {
	switch ci.Type {
	case "uniqueness":
		var cfg invariant.UniquenessConfig
		if err := json.Unmarshal(ci.Config, &cfg); err != nil {
			return nil, err
		}
		return invariant.NewUniquenessInvariant(ci.ID, cfg), nil

	case "acyclicity":
		var cfg invariant.AcyclicityConfig
		if err := json.Unmarshal(ci.Config, &cfg); err != nil {
			return nil, err
		}
		return invariant.NewAcyclicityInvariant(ci.ID, cfg), nil

	case "cardinality":
		var cfg invariant.CardinalityConfig
		if err := json.Unmarshal(ci.Config, &cfg); err != nil {
			return nil, err
		}
		return invariant.NewCardinalityInvariant(ci.ID, cfg), nil

	case "edge_constraint":
		var cfg invariant.EdgeConstraintConfig
		if err := json.Unmarshal(ci.Config, &cfg); err != nil {
			return nil, err
		}
		return invariant.NewEdgeConstraintInvariant(ci.ID, cfg), nil

	case "hierarchy_depth":
		var cfg invariant.HierarchyDepthConfig
		if err := json.Unmarshal(ci.Config, &cfg); err != nil {
			return nil, err
		}
		return invariant.NewHierarchyDepthInvariant(ci.ID, cfg), nil

	case "child_count":
		var cfg invariant.ChildCountConfig
		if err := json.Unmarshal(ci.Config, &cfg); err != nil {
			return nil, err
		}
		return invariant.NewChildCountInvariant(ci.ID, cfg), nil

	default:
		return nil, fmt.Errorf("unknown invariant type: %s", ci.Type)
	}
}

// ValidateSchemaCompatibility checks that a new schema is backward-compatible
// with the existing schema on a graph. Returns an error describing the first
// incompatibility found, or nil if the new schema is safe to apply.
//
// Rules:
//   - Cannot remove a node type that has existing nodes
//   - Cannot remove a required property from a node type
//   - Cannot change a property's type
//   - Cannot make an optional property required (existing nodes may lack it)
//   - Cannot remove an edge type that has existing edges
//   - Cannot narrow edge from/to type constraints if existing edges would violate them
func (mg *MultiGraph) ValidateSchemaCompatibility(graphName string, newSchemaJSON []byte) error {
	mg.mu.RLock()
	g, exists := mg.graphs[graphName]
	mg.mu.RUnlock()

	if !exists {
		// No existing graph — any schema is compatible
		return nil
	}

	var newSchema CompiledSchema
	if err := json.Unmarshal(newSchemaJSON, &newSchema); err != nil {
		return fmt.Errorf("invalid schema JSON: %w", err)
	}

	oldSchema := g.Store.GetSchema()
	if oldSchema == nil {
		// No existing schema — any schema is compatible
		return nil
	}

	// Check node type compatibility
	for oldName, oldDef := range oldSchema.NodeTypes {
		newDef := findNodeType(newSchema.NodeTypes, oldName)
		if newDef == nil {
			// Node type removed — check if any nodes of this type exist
			nodes := g.Store.GetNodesByType(oldName)
			if len(nodes) > 0 {
				return fmt.Errorf("cannot remove node type %q: %d existing nodes", oldName, len(nodes))
			}
			continue
		}

		// Check property compatibility
		for propName, oldProp := range oldDef.Properties {
			newProp, propExists := newDef.Properties[propName]
			if !propExists {
				if oldProp.Required {
					return fmt.Errorf("cannot remove required property %q from node type %q", propName, oldName)
				}
				// Removing an optional property is allowed
				continue
			}

			// Check type change
			if string(oldProp.Type) != newProp.Type {
				return fmt.Errorf("cannot change type of property %q on node type %q from %s to %s",
					propName, oldName, oldProp.Type, newProp.Type)
			}

			// Check required escalation (optional -> required)
			if !oldProp.Required && newProp.Required {
				nodes := g.Store.GetNodesByType(oldName)
				if len(nodes) > 0 {
					return fmt.Errorf("cannot make property %q required on node type %q: %d existing nodes may lack this property",
						propName, oldName, len(nodes))
				}
			}
		}
	}

	// Check edge type compatibility
	for oldName, oldDef := range oldSchema.EdgeTypes {
		newDef := findEdgeType(newSchema.EdgeTypes, oldName)
		if newDef == nil {
			// Edge type removed — check if any edges of this type exist
			// We check all nodes for outgoing edges of this type
			allNodes := g.Store.AllNodes()
			for _, node := range allNodes {
				edges := g.Store.GetOutEdges(node.ID)
				for _, edge := range edges {
					if edge.Type == oldName {
						return fmt.Errorf("cannot remove edge type %q: existing edges found", oldName)
					}
				}
			}
			continue
		}

		// Check narrowing of from/to constraints
		if len(newDef.FromTypes) > 0 && len(oldDef.FromTypes) == 0 {
			// Adding constraints where there were none — check existing edges
			if err := mg.checkEdgeConstraintNarrowing(g, oldName, newDef.FromTypes, newDef.ToTypes); err != nil {
				return err
			}
		} else if len(newDef.FromTypes) > 0 {
			// Check that new from types are a superset of types in use
			if err := mg.checkEdgeConstraintNarrowing(g, oldName, newDef.FromTypes, newDef.ToTypes); err != nil {
				return err
			}
		}
	}

	return nil
}

func findNodeType(types []CompiledNodeType, name string) *CompiledNodeType {
	for i := range types {
		if types[i].Name == name {
			return &types[i]
		}
	}
	return nil
}

func findEdgeType(types []CompiledEdgeType, name string) *CompiledEdgeType {
	for i := range types {
		if types[i].Name == name {
			return &types[i]
		}
	}
	return nil
}

func (mg *MultiGraph) checkEdgeConstraintNarrowing(g *GraphInstance, edgeType string, allowedFrom, allowedTo []string) error {
	allNodes := g.Store.AllNodes()
	fromSet := make(map[string]bool, len(allowedFrom))
	for _, t := range allowedFrom {
		fromSet[t] = true
	}
	toSet := make(map[string]bool, len(allowedTo))
	for _, t := range allowedTo {
		toSet[t] = true
	}

	for _, node := range allNodes {
		edges := g.Store.GetOutEdges(node.ID)
		for _, edge := range edges {
			if edge.Type != edgeType {
				continue
			}
			if len(allowedFrom) > 0 && !fromSet[node.Type] {
				return fmt.Errorf("edge type %q constraint narrowing: existing edge from node type %q not in allowed fromTypes",
					edgeType, node.Type)
			}
			// Check the target node type
			targetNode, err := g.Store.GetNode(edge.ToID)
			if err == nil && len(allowedTo) > 0 && !toSet[targetNode.Type] {
				return fmt.Errorf("edge type %q constraint narrowing: existing edge to node type %q not in allowed toTypes",
					edgeType, targetNode.Type)
			}
		}
	}
	return nil
}

// DeploySchema validates backward compatibility and then applies the schema.
func (mg *MultiGraph) DeploySchema(graphName string, schemaJSON []byte) error {
	if err := mg.ValidateSchemaCompatibility(graphName, schemaJSON); err != nil {
		return fmt.Errorf("schema incompatible: %w", err)
	}
	return mg.LoadSchema(graphName, schemaJSON)
}

// SetFeatureFlags sets feature flags on a graph instance. Flags determine
// which conditional modules are active.
func (mg *MultiGraph) SetFeatureFlags(graphName string, flags map[string]bool) error {
	mg.mu.RLock()
	g, ok := mg.graphs[graphName]
	mg.mu.RUnlock()
	if !ok {
		return fmt.Errorf("graph %q not found", graphName)
	}
	g.FeatureFlags = flags
	return nil
}

// GetFeatureFlags returns the feature flags for a graph.
func (mg *MultiGraph) GetFeatureFlags(graphName string) (map[string]bool, error) {
	mg.mu.RLock()
	g, ok := mg.graphs[graphName]
	mg.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("graph %q not found", graphName)
	}
	return g.FeatureFlags, nil
}

// ApplyModule applies a single module to a graph if its conditions are met.
func (mg *MultiGraph) ApplyModule(graphName string, module *CompiledModule) error {
	mg.mu.RLock()
	g, ok := mg.graphs[graphName]
	mg.mu.RUnlock()
	if !ok {
		return fmt.Errorf("graph %q not found", graphName)
	}

	if !mg.isModuleActive(g, module) {
		return fmt.Errorf("module %q conditions not met", module.ID)
	}

	// Check dependencies
	for _, dep := range module.DependsOn {
		found := false
		for _, active := range g.ActiveModules {
			if active == dep {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("module %q depends on %q which is not active", module.ID, dep)
		}
	}

	// Apply module's schema elements
	miniSchema := &CompiledSchema{
		NodeTypes:  module.NodeTypes,
		EdgeTypes:  module.EdgeTypes,
		Invariants: module.Invariants,
		Pipelines:  module.Pipelines,
	}
	if err := mg.applyCompiledSchema(g, miniSchema); err != nil {
		return fmt.Errorf("module %q: %w", module.ID, err)
	}

	g.ActiveModules = append(g.ActiveModules, module.ID)
	return nil
}

// isModuleActive checks whether a module's conditions are satisfied.
func (mg *MultiGraph) isModuleActive(g *GraphInstance, module *CompiledModule) bool {
	if module.Condition == nil {
		return true // No condition = always active
	}

	cond := module.Condition

	// Check feature flags
	if len(cond.FeatureFlags) > 0 {
		if g.FeatureFlags == nil {
			return false
		}
		for _, flag := range cond.FeatureFlags {
			if !g.FeatureFlags[flag] {
				return false
			}
		}
	}

	// NodeTypePrefix and Expression conditions are always considered met
	// at the schema level — they are evaluated at operation time by the
	// invariant/validation layer. The schema elements are always loaded,
	// but the invariants only fire when the prefix matches.
	return true
}

// ResolveModules processes all modules in a compiled schema: checks conditions,
// resolves dependencies via topological sort, and applies them in order.
func (mg *MultiGraph) ResolveModules(graphName string, modules []CompiledModule) error {
	if len(modules) == 0 {
		return nil
	}

	g := mg.GetOrCreate(graphName)

	// Build dependency graph and sort topologically
	sorted, err := topologicalSortModules(modules)
	if err != nil {
		return err
	}

	for _, module := range sorted {
		m := module // capture
		if !mg.isModuleActive(g, &m) {
			continue // Skip inactive modules
		}

		// Check dependencies are satisfied
		for _, dep := range m.DependsOn {
			found := false
			for _, active := range g.ActiveModules {
				if active == dep {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("module %q depends on %q which was not activated", m.ID, dep)
			}
		}

		miniSchema := &CompiledSchema{
			NodeTypes:  m.NodeTypes,
			EdgeTypes:  m.EdgeTypes,
			Invariants: m.Invariants,
			Pipelines:  m.Pipelines,
		}
		if err := mg.applyCompiledSchema(g, miniSchema); err != nil {
			return fmt.Errorf("module %q: %w", m.ID, err)
		}
		g.ActiveModules = append(g.ActiveModules, m.ID)
	}

	return nil
}

// topologicalSortModules sorts modules by dependency order.
func topologicalSortModules(modules []CompiledModule) ([]CompiledModule, error) {
	byID := make(map[string]*CompiledModule, len(modules))
	for i := range modules {
		byID[modules[i].ID] = &modules[i]
	}

	visited := make(map[string]bool)
	visiting := make(map[string]bool) // cycle detection
	var sorted []CompiledModule

	var visit func(id string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("circular dependency detected involving module %q", id)
		}
		visiting[id] = true

		m, ok := byID[id]
		if !ok {
			return fmt.Errorf("unknown module dependency %q", id)
		}

		for _, dep := range m.DependsOn {
			if _, exists := byID[dep]; exists {
				if err := visit(dep); err != nil {
					return err
				}
			}
			// External deps (not in this batch) are assumed satisfied
		}

		visiting[id] = false
		visited[id] = true
		sorted = append(sorted, *m)
		return nil
	}

	for _, m := range modules {
		if err := visit(m.ID); err != nil {
			return nil, err
		}
	}

	return sorted, nil
}

// StopAll stops all graph instances.
func (mg *MultiGraph) StopAll() {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	for _, g := range mg.graphs {
		g.DerivedStore.Stop()
	}
}
