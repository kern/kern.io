package derived

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
)

// Binding represents a unidirectional data binding from a source property
// to a target property. When the source changes, the target is recomputed.
// Bindings can include transformations.
type Binding struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`

	// Source specification
	Source BindingSource `json:"source"`

	// Target specification
	Target BindingTarget `json:"target"`

	// Transform applied to the source value before setting on target.
	// If nil, value is copied directly.
	Transform BindingTransformFunc `json:"-"`

	// Serializable transform for client-side execution
	SerializableTransform *BindingExpression `json:"expression,omitempty"`

	// Execution mode
	Mode ExecutionMode `json:"mode"`
}

// BindingSource specifies where the bound value comes from.
type BindingSource struct {
	// NodeID: specific node (empty = relative binding)
	NodeID string `json:"nodeId,omitempty"`
	// NodeType: bind to all nodes of this type (for pattern-based bindings)
	NodeType string `json:"nodeType,omitempty"`
	// Resolve: how to find the source node relative to the target
	Resolve BindingResolve `json:"resolve"`
	// Property: the source property name
	Property string `json:"property"`
	// For edge-based resolution
	EdgeType string `json:"edgeType,omitempty"`
	// RefProperty: for resolve=ref, which property on the target contains the ref
	RefProperty string `json:"refProperty,omitempty"`
}

// BindingTarget specifies where the bound value goes.
type BindingTarget struct {
	// NodeID: specific node (empty = applies to all matching nodes)
	NodeID string `json:"nodeId,omitempty"`
	// NodeType: target node type (for pattern-based bindings)
	NodeType string `json:"nodeType,omitempty"`
	// Property: the target property name
	Property string `json:"property"`
}

// BindingResolve determines how to find the source node.
type BindingResolve int

const (
	// BindSelf: source property is on the same node.
	BindSelf BindingResolve = iota
	// BindParent: source is the parent node.
	BindParent
	// BindAncestor: walk up the tree to find a node of the source type.
	BindAncestor
	// BindRef: follow a reference property on the target to find the source.
	BindRef
	// BindChild: source is a child node (first matching child of type).
	BindChild
	// BindOutEdge: follow an outgoing edge to find the source.
	BindOutEdge
	// BindInEdge: follow an incoming edge to find the source.
	BindInEdge
	// BindAbsolute: source is a specific node by ID.
	BindAbsolute
	// BindSibling: source is a sibling (same parent) matching criteria.
	BindSibling
)

// BindingTransformFunc transforms a value during binding propagation.
type BindingTransformFunc func(value interface{}, sourceNode, targetNode *crdt.MaterializedNode) interface{}

// BindingExpression is a serializable transform expression for client-side binding.
type BindingExpression struct {
	// Type of expression
	Type ExprType `json:"type"`
	// Value for literal expressions
	Value interface{} `json:"value,omitempty"`
	// Path for property access (e.g. "source.name")
	Path string `json:"path,omitempty"`
	// Operator for binary expressions
	Op string `json:"op,omitempty"`
	// Sub-expressions for composite expressions
	Args []*BindingExpression `json:"args,omitempty"`
}

// ExprType is the type of a binding expression.
type ExprType int

const (
	ExprLiteral    ExprType = iota // constant value
	ExprPath                       // property path (e.g., "source.name")
	ExprConcat                     // string concatenation
	ExprAdd                        // numeric addition
	ExprMul                        // numeric multiplication
	ExprCoalesce                   // first non-nil value
	ExprConditional                // if/then/else
	ExprMap                        // map over array
	ExprCount                      // count elements
	ExprFormat                     // string format
)

// BindingEngine manages property bindings and propagates changes.
type BindingEngine struct {
	mu       sync.RWMutex
	bindings []*Binding

	// Index: targetNodeType -> property -> bindings
	targetIndex map[string]map[string][]*Binding
	// Index: sourceNodeID -> bindings (for absolute bindings)
	sourceIndex map[string][]*Binding

	// Dependency graph for incremental evaluation
	// Key: (nodeType, property) -> which bindings to re-evaluate
	deps map[depKey][]*Binding
}

type depKey struct {
	NodeType string
	Property string
}

// NewBindingEngine creates a new binding engine.
func NewBindingEngine() *BindingEngine {
	return &BindingEngine{
		targetIndex: make(map[string]map[string][]*Binding),
		sourceIndex: make(map[string][]*Binding),
		deps:        make(map[depKey][]*Binding),
	}
}

// Add registers a new binding.
func (be *BindingEngine) Add(b *Binding) {
	be.mu.Lock()
	defer be.mu.Unlock()

	be.bindings = append(be.bindings, b)

	// Index by target
	if b.Target.NodeType != "" {
		if be.targetIndex[b.Target.NodeType] == nil {
			be.targetIndex[b.Target.NodeType] = make(map[string][]*Binding)
		}
		be.targetIndex[b.Target.NodeType][b.Target.Property] = append(
			be.targetIndex[b.Target.NodeType][b.Target.Property], b)
	}

	// Index by source for absolute bindings
	if b.Source.NodeID != "" {
		be.sourceIndex[b.Source.NodeID] = append(be.sourceIndex[b.Source.NodeID], b)
	}

	// Track dependency
	dk := depKey{NodeType: b.Source.NodeType, Property: b.Source.Property}
	be.deps[dk] = append(be.deps[dk], b)
}

// Remove removes a binding by ID.
func (be *BindingEngine) Remove(id string) {
	be.mu.Lock()
	defer be.mu.Unlock()

	for i, b := range be.bindings {
		if b.ID == id {
			be.bindings = append(be.bindings[:i], be.bindings[i+1:]...)
			be.rebuildIndexes()
			return
		}
	}
}

func (be *BindingEngine) rebuildIndexes() {
	be.targetIndex = make(map[string]map[string][]*Binding)
	be.sourceIndex = make(map[string][]*Binding)
	be.deps = make(map[depKey][]*Binding)

	for _, b := range be.bindings {
		if b.Target.NodeType != "" {
			if be.targetIndex[b.Target.NodeType] == nil {
				be.targetIndex[b.Target.NodeType] = make(map[string][]*Binding)
			}
			be.targetIndex[b.Target.NodeType][b.Target.Property] = append(
				be.targetIndex[b.Target.NodeType][b.Target.Property], b)
		}
		if b.Source.NodeID != "" {
			be.sourceIndex[b.Source.NodeID] = append(be.sourceIndex[b.Source.NodeID], b)
		}
		dk := depKey{NodeType: b.Source.NodeType, Property: b.Source.Property}
		be.deps[dk] = append(be.deps[dk], b)
	}
}

// GetBindingsForTarget returns all bindings that target a given node type and property.
func (be *BindingEngine) GetBindingsForTarget(nodeType, property string) []*Binding {
	be.mu.RLock()
	defer be.mu.RUnlock()
	if propBindings, ok := be.targetIndex[nodeType]; ok {
		return propBindings[property]
	}
	return nil
}

// GetAffectedBindings returns bindings that need re-evaluation when
// a source node of the given type changes a specific property.
// This is the incremental update path.
func (be *BindingEngine) GetAffectedBindings(nodeType, property string) []*Binding {
	be.mu.RLock()
	defer be.mu.RUnlock()
	dk := depKey{NodeType: nodeType, Property: property}
	return be.deps[dk]
}

// EvaluateBinding resolves a single binding for a specific target node.
func (be *BindingEngine) EvaluateBinding(b *Binding, targetNode *crdt.MaterializedNode, resolver BindingResolver) (interface{}, error) {
	// Find the source node
	sourceNode, err := be.resolveSourceNode(b, targetNode, resolver)
	if err != nil {
		return nil, err
	}
	if sourceNode == nil {
		return nil, nil
	}

	// Get the source value
	value, ok := sourceNode.Properties[b.Source.Property]
	if !ok {
		return nil, nil
	}

	// Apply transform
	if b.Transform != nil {
		value = b.Transform(value, sourceNode, targetNode)
	} else if b.SerializableTransform != nil {
		value = be.evaluateExpression(b.SerializableTransform, value, sourceNode, targetNode)
	}

	return value, nil
}

// EvaluateAllForNode evaluates all bindings that target a specific node.
func (be *BindingEngine) EvaluateAllForNode(node *crdt.MaterializedNode, resolver BindingResolver) map[string]interface{} {
	be.mu.RLock()
	defer be.mu.RUnlock()

	results := make(map[string]interface{})

	propBindings, ok := be.targetIndex[node.Type]
	if !ok {
		return results
	}

	for prop, bindings := range propBindings {
		for _, b := range bindings {
			// Check if this binding applies to this specific node
			if b.Target.NodeID != "" && b.Target.NodeID != node.ID.String() {
				continue
			}

			value, err := be.EvaluateBinding(b, node, resolver)
			if err == nil && value != nil {
				results[prop] = value
			}
		}
	}

	return results
}

// List returns all registered bindings.
func (be *BindingEngine) List() []*Binding {
	be.mu.RLock()
	defer be.mu.RUnlock()
	result := make([]*Binding, len(be.bindings))
	copy(result, be.bindings)
	return result
}

// Serialize returns all bindings that should be sent to the client.
func (be *BindingEngine) Serialize() []*Binding {
	be.mu.RLock()
	defer be.mu.RUnlock()

	var result []*Binding
	for _, b := range be.bindings {
		if b.Mode == ExecClient || b.Mode == ExecBoth {
			result = append(result, b)
		}
	}
	return result
}

// BindingResolver provides graph access for resolving bindings.
type BindingResolver interface {
	GetNode(id uuid.UUID) (*crdt.MaterializedNode, bool)
	GetParent(id uuid.UUID) (*crdt.MaterializedNode, bool)
	GetChildren(id uuid.UUID) []*crdt.MaterializedNode
	GetOutEdges(id uuid.UUID) []*crdt.MaterializedEdge
	GetInEdges(id uuid.UUID) []*crdt.MaterializedEdge
}

func (be *BindingEngine) resolveSourceNode(b *Binding, targetNode *crdt.MaterializedNode, resolver BindingResolver) (*crdt.MaterializedNode, error) {
	switch b.Source.Resolve {
	case BindSelf:
		return targetNode, nil

	case BindParent:
		parent, ok := resolver.GetParent(targetNode.ID)
		if !ok {
			return nil, nil
		}
		return parent, nil

	case BindAncestor:
		current := targetNode.ID
		for {
			parent, ok := resolver.GetParent(current)
			if !ok {
				return nil, nil
			}
			if b.Source.NodeType == "" || parent.Type == b.Source.NodeType {
				return parent, nil
			}
			current = parent.ID
		}

	case BindRef:
		refProp := b.Source.RefProperty
		if refProp == "" {
			refProp = b.Source.Property + "Id"
		}
		refID, ok := targetNode.Properties[refProp].(string)
		if !ok {
			return nil, nil
		}
		uid, err := uuid.Parse(refID)
		if err != nil {
			return nil, nil
		}
		node, ok := resolver.GetNode(uid)
		if !ok {
			return nil, nil
		}
		return node, nil

	case BindChild:
		children := resolver.GetChildren(targetNode.ID)
		for _, child := range children {
			if b.Source.NodeType == "" || child.Type == b.Source.NodeType {
				return child, nil
			}
		}
		return nil, nil

	case BindOutEdge:
		edges := resolver.GetOutEdges(targetNode.ID)
		for _, edge := range edges {
			if b.Source.EdgeType == "" || edge.Type == b.Source.EdgeType {
				node, ok := resolver.GetNode(edge.ToID)
				if ok && (b.Source.NodeType == "" || node.Type == b.Source.NodeType) {
					return node, nil
				}
			}
		}
		return nil, nil

	case BindInEdge:
		edges := resolver.GetInEdges(targetNode.ID)
		for _, edge := range edges {
			if b.Source.EdgeType == "" || edge.Type == b.Source.EdgeType {
				node, ok := resolver.GetNode(edge.FromID)
				if ok && (b.Source.NodeType == "" || node.Type == b.Source.NodeType) {
					return node, nil
				}
			}
		}
		return nil, nil

	case BindAbsolute:
		uid, err := uuid.Parse(b.Source.NodeID)
		if err != nil {
			return nil, fmt.Errorf("invalid source node ID: %s", b.Source.NodeID)
		}
		node, ok := resolver.GetNode(uid)
		if !ok {
			return nil, nil
		}
		return node, nil

	case BindSibling:
		parent, ok := resolver.GetParent(targetNode.ID)
		if !ok {
			return nil, nil
		}
		siblings := resolver.GetChildren(parent.ID)
		for _, sibling := range siblings {
			if sibling.ID == targetNode.ID {
				continue
			}
			if b.Source.NodeType == "" || sibling.Type == b.Source.NodeType {
				return sibling, nil
			}
		}
		return nil, nil
	}

	return nil, fmt.Errorf("unknown resolve type: %d", b.Source.Resolve)
}

func (be *BindingEngine) evaluateExpression(expr *BindingExpression, sourceValue interface{}, sourceNode, targetNode *crdt.MaterializedNode) interface{} {
	switch expr.Type {
	case ExprLiteral:
		return expr.Value

	case ExprPath:
		return resolvePropertyPath(expr.Path, sourceNode, targetNode)

	case ExprConcat:
		var parts []string
		for _, arg := range expr.Args {
			v := be.evaluateExpression(arg, sourceValue, sourceNode, targetNode)
			parts = append(parts, fmt.Sprintf("%v", v))
		}
		return strings.Join(parts, "")

	case ExprCoalesce:
		for _, arg := range expr.Args {
			v := be.evaluateExpression(arg, sourceValue, sourceNode, targetNode)
			if v != nil {
				return v
			}
		}
		return nil

	case ExprConditional:
		if len(expr.Args) >= 3 {
			cond := be.evaluateExpression(expr.Args[0], sourceValue, sourceNode, targetNode)
			if isTruthy(cond) {
				return be.evaluateExpression(expr.Args[1], sourceValue, sourceNode, targetNode)
			}
			return be.evaluateExpression(expr.Args[2], sourceValue, sourceNode, targetNode)
		}
		return nil

	default:
		return sourceValue
	}
}

func resolvePropertyPath(path string, sourceNode, targetNode *crdt.MaterializedNode) interface{} {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) != 2 {
		return nil
	}

	var node *crdt.MaterializedNode
	switch parts[0] {
	case "source":
		node = sourceNode
	case "target":
		node = targetNode
	default:
		return nil
	}

	if node == nil {
		return nil
	}

	return node.Properties[parts[1]]
}

func isTruthy(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val != ""
	case int:
		return val != 0
	case float64:
		return val != 0
	default:
		return true
	}
}

// --- Binding constructors ---

// NewBinding creates a new property binding.
func NewBinding(id string) *BindingBuilder {
	return &BindingBuilder{
		binding: &Binding{
			ID:   id,
			Mode: ExecServer,
		},
	}
}

// BindingBuilder provides a fluent API for creating bindings.
type BindingBuilder struct {
	binding *Binding
}

// From sets the binding source.
func (bb *BindingBuilder) From(nodeType, property string) *BindingBuilder {
	bb.binding.Source.NodeType = nodeType
	bb.binding.Source.Property = property
	return bb
}

// FromSelf binds from the same node's property.
func (bb *BindingBuilder) FromSelf(property string) *BindingBuilder {
	bb.binding.Source.Resolve = BindSelf
	bb.binding.Source.Property = property
	return bb
}

// FromParent binds from the parent node's property.
func (bb *BindingBuilder) FromParent(property string) *BindingBuilder {
	bb.binding.Source.Resolve = BindParent
	bb.binding.Source.Property = property
	return bb
}

// FromAncestor binds from an ancestor of a specific type.
func (bb *BindingBuilder) FromAncestor(nodeType, property string) *BindingBuilder {
	bb.binding.Source.Resolve = BindAncestor
	bb.binding.Source.NodeType = nodeType
	bb.binding.Source.Property = property
	return bb
}

// FromRef binds by following a reference property.
func (bb *BindingBuilder) FromRef(refProperty, sourceProperty string) *BindingBuilder {
	bb.binding.Source.Resolve = BindRef
	bb.binding.Source.RefProperty = refProperty
	bb.binding.Source.Property = sourceProperty
	return bb
}

// FromChild binds from a child node of a specific type.
func (bb *BindingBuilder) FromChild(childType, property string) *BindingBuilder {
	bb.binding.Source.Resolve = BindChild
	bb.binding.Source.NodeType = childType
	bb.binding.Source.Property = property
	return bb
}

// FromOutEdge binds from a node reached via an outgoing edge.
func (bb *BindingBuilder) FromOutEdge(edgeType, property string) *BindingBuilder {
	bb.binding.Source.Resolve = BindOutEdge
	bb.binding.Source.EdgeType = edgeType
	bb.binding.Source.Property = property
	return bb
}

// FromInEdge binds from a node reached via an incoming edge.
func (bb *BindingBuilder) FromInEdge(edgeType, property string) *BindingBuilder {
	bb.binding.Source.Resolve = BindInEdge
	bb.binding.Source.EdgeType = edgeType
	bb.binding.Source.Property = property
	return bb
}

// FromNode binds from a specific node by ID.
func (bb *BindingBuilder) FromNode(nodeID, property string) *BindingBuilder {
	bb.binding.Source.Resolve = BindAbsolute
	bb.binding.Source.NodeID = nodeID
	bb.binding.Source.Property = property
	return bb
}

// FromSibling binds from a sibling node of a specific type.
func (bb *BindingBuilder) FromSibling(nodeType, property string) *BindingBuilder {
	bb.binding.Source.Resolve = BindSibling
	bb.binding.Source.NodeType = nodeType
	bb.binding.Source.Property = property
	return bb
}

// To sets the binding target.
func (bb *BindingBuilder) To(nodeType, property string) *BindingBuilder {
	bb.binding.Target.NodeType = nodeType
	bb.binding.Target.Property = property
	return bb
}

// ToNode sets the binding target to a specific node.
func (bb *BindingBuilder) ToNode(nodeID, property string) *BindingBuilder {
	bb.binding.Target.NodeID = nodeID
	bb.binding.Target.Property = property
	return bb
}

// WithTransform adds a server-side transform function.
func (bb *BindingBuilder) WithTransform(fn BindingTransformFunc) *BindingBuilder {
	bb.binding.Transform = fn
	return bb
}

// WithExpression adds a serializable expression (works client-side too).
func (bb *BindingBuilder) WithExpression(expr *BindingExpression) *BindingBuilder {
	bb.binding.SerializableTransform = expr
	return bb
}

// ClientSide marks this binding for client-side execution.
func (bb *BindingBuilder) ClientSide() *BindingBuilder {
	bb.binding.Mode = ExecClient
	return bb
}

// BothSides marks this binding for both server and client execution.
func (bb *BindingBuilder) BothSides() *BindingBuilder {
	bb.binding.Mode = ExecBoth
	return bb
}

// Build finalizes the binding.
func (bb *BindingBuilder) Build() *Binding {
	return bb.binding
}

// --- Expression constructors ---

func Literal(v interface{}) *BindingExpression {
	return &BindingExpression{Type: ExprLiteral, Value: v}
}

func Path(path string) *BindingExpression {
	return &BindingExpression{Type: ExprPath, Path: path}
}

func Concat(args ...*BindingExpression) *BindingExpression {
	return &BindingExpression{Type: ExprConcat, Args: args}
}

func Coalesce(args ...*BindingExpression) *BindingExpression {
	return &BindingExpression{Type: ExprCoalesce, Args: args}
}

func IfThenElse(cond, then, elseExpr *BindingExpression) *BindingExpression {
	return &BindingExpression{Type: ExprConditional, Args: []*BindingExpression{cond, then, elseExpr}}
}
