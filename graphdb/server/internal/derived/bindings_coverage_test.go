package derived

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
)

// mockResolver implements BindingResolver for testing.
type mockResolver struct {
	nodes    map[uuid.UUID]*crdt.MaterializedNode
	children map[uuid.UUID][]*crdt.MaterializedNode
	parents  map[uuid.UUID]*crdt.MaterializedNode
	outEdges map[uuid.UUID][]*crdt.MaterializedEdge
	inEdges  map[uuid.UUID][]*crdt.MaterializedEdge
}

func newMockResolver() *mockResolver {
	return &mockResolver{
		nodes:    make(map[uuid.UUID]*crdt.MaterializedNode),
		children: make(map[uuid.UUID][]*crdt.MaterializedNode),
		parents:  make(map[uuid.UUID]*crdt.MaterializedNode),
		outEdges: make(map[uuid.UUID][]*crdt.MaterializedEdge),
		inEdges:  make(map[uuid.UUID][]*crdt.MaterializedEdge),
	}
}

func (r *mockResolver) addNode(n *crdt.MaterializedNode) {
	r.nodes[n.ID] = n
}

func (r *mockResolver) setParent(childID uuid.UUID, parent *crdt.MaterializedNode) {
	r.parents[childID] = parent
}

func (r *mockResolver) addChild(parentID uuid.UUID, child *crdt.MaterializedNode) {
	r.children[parentID] = append(r.children[parentID], child)
}

func (r *mockResolver) addOutEdge(nodeID uuid.UUID, edge *crdt.MaterializedEdge) {
	r.outEdges[nodeID] = append(r.outEdges[nodeID], edge)
}

func (r *mockResolver) addInEdge(nodeID uuid.UUID, edge *crdt.MaterializedEdge) {
	r.inEdges[nodeID] = append(r.inEdges[nodeID], edge)
}

func (r *mockResolver) GetNode(id uuid.UUID) (*crdt.MaterializedNode, bool) {
	n, ok := r.nodes[id]
	return n, ok
}

func (r *mockResolver) GetParent(id uuid.UUID) (*crdt.MaterializedNode, bool) {
	p, ok := r.parents[id]
	return p, ok
}

func (r *mockResolver) GetChildren(id uuid.UUID) []*crdt.MaterializedNode {
	return r.children[id]
}

func (r *mockResolver) GetOutEdges(id uuid.UUID) []*crdt.MaterializedEdge {
	return r.outEdges[id]
}

func (r *mockResolver) GetInEdges(id uuid.UUID) []*crdt.MaterializedEdge {
	return r.inEdges[id]
}

func TestBindingEngineAddAndList(t *testing.T) {
	be := NewBindingEngine()

	b1 := NewBinding("b1").FromSelf("name").To("user", "displayName").Build()
	b2 := NewBinding("b2").FromParent("title").To("item", "parentTitle").Build()

	be.Add(b1)
	be.Add(b2)

	list := be.List()
	if len(list) != 2 {
		t.Errorf("expected 2 bindings, got %d", len(list))
	}
}

func TestBindingEngineRemove(t *testing.T) {
	be := NewBindingEngine()

	b1 := NewBinding("b1").FromSelf("name").To("user", "displayName").Build()
	b2 := NewBinding("b2").FromParent("title").To("item", "parentTitle").Build()

	be.Add(b1)
	be.Add(b2)

	be.Remove("b1")

	list := be.List()
	if len(list) != 1 {
		t.Errorf("expected 1 binding after remove, got %d", len(list))
	}
	if list[0].ID != "b2" {
		t.Error("wrong binding remaining")
	}
}

func TestGetBindingsForTarget(t *testing.T) {
	be := NewBindingEngine()

	b := NewBinding("b1").FromSelf("name").To("user", "displayName").Build()
	be.Add(b)

	bindings := be.GetBindingsForTarget("user", "displayName")
	if len(bindings) != 1 {
		t.Errorf("expected 1 binding, got %d", len(bindings))
	}

	bindings = be.GetBindingsForTarget("nonexistent", "prop")
	if len(bindings) != 0 {
		t.Errorf("expected 0 bindings for nonexistent type, got %d", len(bindings))
	}
}

func TestGetAffectedBindings(t *testing.T) {
	be := NewBindingEngine()

	b := NewBinding("b1").From("user", "name").To("user", "displayName").Build()
	be.Add(b)

	affected := be.GetAffectedBindings("user", "name")
	if len(affected) != 1 {
		t.Errorf("expected 1 affected, got %d", len(affected))
	}

	affected = be.GetAffectedBindings("user", "email")
	if len(affected) != 0 {
		t.Errorf("expected 0 affected, got %d", len(affected))
	}
}

func TestEvaluateBindingSelf(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	targetNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"name": "Alice"},
	}
	resolver.addNode(targetNode)

	b := NewBinding("b1").FromSelf("name").To("user", "displayName").Build()

	value, err := be.EvaluateBinding(b, targetNode, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != "Alice" {
		t.Errorf("expected 'Alice', got %v", value)
	}
}

func TestEvaluateBindingParent(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	parentNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "folder",
		Properties: map[string]interface{}{"title": "Documents"},
	}
	childNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "file",
		Properties: map[string]interface{}{},
	}
	resolver.addNode(parentNode)
	resolver.addNode(childNode)
	resolver.setParent(childNode.ID, parentNode)

	b := NewBinding("b1").FromParent("title").To("file", "parentTitle").Build()

	value, err := be.EvaluateBinding(b, childNode, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != "Documents" {
		t.Errorf("expected 'Documents', got %v", value)
	}
}

func TestEvaluateBindingAncestor(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	rootNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "org",
		Properties: map[string]interface{}{"name": "Acme"},
	}
	midNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "team",
		Properties: map[string]interface{}{},
	}
	leafNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{},
	}
	resolver.addNode(rootNode)
	resolver.addNode(midNode)
	resolver.addNode(leafNode)
	resolver.setParent(leafNode.ID, midNode)
	resolver.setParent(midNode.ID, rootNode)

	b := NewBinding("b1").FromAncestor("org", "name").To("user", "orgName").Build()

	value, err := be.EvaluateBinding(b, leafNode, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != "Acme" {
		t.Errorf("expected 'Acme', got %v", value)
	}
}

func TestEvaluateBindingRef(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	refNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"name": "Bob"},
	}
	targetNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "post",
		Properties: map[string]interface{}{"authorId": refNode.ID.String()},
	}
	resolver.addNode(refNode)
	resolver.addNode(targetNode)

	b := NewBinding("b1").FromRef("authorId", "name").To("post", "authorName").Build()

	value, err := be.EvaluateBinding(b, targetNode, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != "Bob" {
		t.Errorf("expected 'Bob', got %v", value)
	}
}

func TestEvaluateBindingChild(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	parentNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "folder",
		Properties: map[string]interface{}{},
	}
	childNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "file",
		Properties: map[string]interface{}{"status": "active"},
	}
	resolver.addNode(parentNode)
	resolver.addNode(childNode)
	resolver.addChild(parentNode.ID, childNode)

	b := NewBinding("b1").FromChild("file", "status").To("folder", "childStatus").Build()

	value, err := be.EvaluateBinding(b, parentNode, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != "active" {
		t.Errorf("expected 'active', got %v", value)
	}
}

func TestEvaluateBindingOutEdge(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	sourceNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{},
	}
	targetNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "team",
		Properties: map[string]interface{}{"teamName": "Engineering"},
	}
	resolver.addNode(sourceNode)
	resolver.addNode(targetNode)
	resolver.addOutEdge(sourceNode.ID, &crdt.MaterializedEdge{
		ID:     uuid.New(),
		Type:   "member_of",
		FromID: sourceNode.ID,
		ToID:   targetNode.ID,
	})

	b := NewBinding("b1").FromOutEdge("member_of", "teamName").To("user", "team").Build()

	value, err := be.EvaluateBinding(b, sourceNode, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != "Engineering" {
		t.Errorf("expected 'Engineering', got %v", value)
	}
}

func TestEvaluateBindingInEdge(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	fromNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"name": "Charlie"},
	}
	toNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "post",
		Properties: map[string]interface{}{},
	}
	resolver.addNode(fromNode)
	resolver.addNode(toNode)
	resolver.addInEdge(toNode.ID, &crdt.MaterializedEdge{
		ID:     uuid.New(),
		Type:   "authored",
		FromID: fromNode.ID,
		ToID:   toNode.ID,
	})

	b := NewBinding("b1").FromInEdge("authored", "name").To("post", "authorName").Build()

	value, err := be.EvaluateBinding(b, toNode, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != "Charlie" {
		t.Errorf("expected 'Charlie', got %v", value)
	}
}

func TestEvaluateBindingAbsolute(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	sourceNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "config",
		Properties: map[string]interface{}{"theme": "dark"},
	}
	targetNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{},
	}
	resolver.addNode(sourceNode)
	resolver.addNode(targetNode)

	b := NewBinding("b1").FromNode(sourceNode.ID.String(), "theme").To("user", "theme").Build()

	value, err := be.EvaluateBinding(b, targetNode, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != "dark" {
		t.Errorf("expected 'dark', got %v", value)
	}
}

func TestEvaluateBindingSibling(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	parentNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "folder",
		Properties: map[string]interface{}{},
	}
	siblingNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "config",
		Properties: map[string]interface{}{"color": "blue"},
	}
	targetNode := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "item",
		Properties: map[string]interface{}{},
	}
	resolver.addNode(parentNode)
	resolver.addNode(siblingNode)
	resolver.addNode(targetNode)
	resolver.setParent(targetNode.ID, parentNode)
	resolver.addChild(parentNode.ID, siblingNode)
	resolver.addChild(parentNode.ID, targetNode)

	b := NewBinding("b1").FromSibling("config", "color").To("item", "color").Build()

	value, err := be.EvaluateBinding(b, targetNode, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != "blue" {
		t.Errorf("expected 'blue', got %v", value)
	}
}

func TestEvaluateAllForNode(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"name": "Alice", "email": "alice@test.com"},
	}
	resolver.addNode(node)

	b1 := NewBinding("b1").FromSelf("name").To("user", "displayName").Build()
	b2 := NewBinding("b2").FromSelf("email").To("user", "contactEmail").Build()
	be.Add(b1)
	be.Add(b2)

	results := be.EvaluateAllForNode(node, resolver)
	if results["displayName"] != "Alice" {
		t.Errorf("expected 'Alice', got %v", results["displayName"])
	}
	if results["contactEmail"] != "alice@test.com" {
		t.Errorf("expected 'alice@test.com', got %v", results["contactEmail"])
	}
}

func TestEvaluateAllForNodeWithTargetNodeID(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node1 := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"name": "Alice"},
	}
	node2 := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"name": "Bob"},
	}
	resolver.addNode(node1)
	resolver.addNode(node2)

	// This binding targets a specific node
	b := NewBinding("b1").FromSelf("name").To("user", "displayName").Build()
	b.Target.NodeID = node1.ID.String()
	be.Add(b)

	// Should apply to node1
	results := be.EvaluateAllForNode(node1, resolver)
	if results["displayName"] != "Alice" {
		t.Error("should apply to targeted node")
	}

	// Should not apply to node2
	results = be.EvaluateAllForNode(node2, resolver)
	if results["displayName"] != nil {
		t.Error("should not apply to non-targeted node")
	}
}

func TestSerialize(t *testing.T) {
	be := NewBindingEngine()

	b1 := NewBinding("b1").FromSelf("name").To("user", "display").ClientSide().Build()
	b2 := NewBinding("b2").FromSelf("name").To("user", "display2").Build() // server only
	b3 := NewBinding("b3").FromSelf("name").To("user", "display3").BothSides().Build()

	be.Add(b1)
	be.Add(b2)
	be.Add(b3)

	serialized := be.Serialize()
	if len(serialized) != 2 {
		t.Errorf("expected 2 client-visible bindings, got %d", len(serialized))
	}
}

func TestEvaluateBindingWithTransform(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"name": "alice"},
	}
	resolver.addNode(node)

	b := NewBinding("b1").FromSelf("name").To("user", "upperName").
		WithTransform(func(value interface{}, source, target *crdt.MaterializedNode) interface{} {
			return "UPPER:" + value.(string)
		}).Build()

	value, err := be.EvaluateBinding(b, node, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != "UPPER:alice" {
		t.Errorf("expected 'UPPER:alice', got %v", value)
	}
}

func TestEvaluateBindingWithExpression(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"firstName": "Alice", "lastName": "Smith"},
	}
	resolver.addNode(node)

	b := NewBinding("b1").FromSelf("firstName").To("user", "fullName").
		WithExpression(Concat(Path("source.firstName"), Literal(" "), Path("source.lastName"))).
		Build()

	value, err := be.EvaluateBinding(b, node, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != "Alice Smith" {
		t.Errorf("expected 'Alice Smith', got %v", value)
	}
}

func TestExpressionLiteral(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"name": "test"},
	}

	b := NewBinding("b1").FromSelf("name").To("user", "constant").
		WithExpression(Literal("constant-value")).Build()

	value, _ := be.EvaluateBinding(b, node, resolver)
	if value != "constant-value" {
		t.Errorf("expected 'constant-value', got %v", value)
	}
}

func TestExpressionCoalesce(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"name": "test"},
	}

	b := NewBinding("b1").FromSelf("name").To("user", "val").
		WithExpression(Coalesce(Path("source.missing"), Literal("default"))).Build()

	value, _ := be.EvaluateBinding(b, node, resolver)
	if value != "default" {
		t.Errorf("expected 'default', got %v", value)
	}
}

func TestExpressionConditional(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"active": true},
	}

	b := NewBinding("b1").FromSelf("active").To("user", "status").
		WithExpression(IfThenElse(Path("source.active"), Literal("active"), Literal("inactive"))).Build()

	value, _ := be.EvaluateBinding(b, node, resolver)
	if value != "active" {
		t.Errorf("expected 'active', got %v", value)
	}
}

func TestExpressionConditionalFalse(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"active": false},
	}

	b := NewBinding("b1").FromSelf("active").To("user", "status").
		WithExpression(IfThenElse(Path("source.active"), Literal("yes"), Literal("no"))).Build()

	value, _ := be.EvaluateBinding(b, node, resolver)
	if value != "no" {
		t.Errorf("expected 'no', got %v", value)
	}
}

func TestResolvePropertyPath(t *testing.T) {
	source := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Properties: map[string]interface{}{"name": "Alice"},
	}
	target := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Properties: map[string]interface{}{"color": "blue"},
	}

	if v := resolvePropertyPath("source.name", source, target); v != "Alice" {
		t.Errorf("expected 'Alice', got %v", v)
	}
	if v := resolvePropertyPath("target.color", source, target); v != "blue" {
		t.Errorf("expected 'blue', got %v", v)
	}
	if v := resolvePropertyPath("unknown.field", source, target); v != nil {
		t.Errorf("expected nil, got %v", v)
	}
	if v := resolvePropertyPath("noDot", source, target); v != nil {
		t.Errorf("expected nil for invalid path, got %v", v)
	}
}

func TestIsTruthy(t *testing.T) {
	tests := []struct {
		val    interface{}
		expect bool
	}{
		{nil, false},
		{true, true},
		{false, false},
		{"hello", true},
		{"", false},
		{0, false},
		{1, true},
		{0.0, false},
		{1.5, true},
		{[]string{"a"}, true}, // non-nil defaults to true
	}

	for _, tt := range tests {
		if got := isTruthy(tt.val); got != tt.expect {
			t.Errorf("isTruthy(%v) = %v, want %v", tt.val, got, tt.expect)
		}
	}
}

func TestBindingBuilderAllMethods(t *testing.T) {
	// Test all builder methods compile and chain properly
	b := NewBinding("test").
		From("user", "name").
		To("user", "displayName").
		Build()
	if b.Source.NodeType != "user" || b.Source.Property != "name" {
		t.Error("From not set correctly")
	}

	b = NewBinding("test2").
		FromSelf("name").
		ToNode("node-id", "displayName").
		Build()
	if b.Source.Resolve != BindSelf {
		t.Error("FromSelf not set correctly")
	}
	if b.Target.NodeID != "node-id" {
		t.Error("ToNode not set correctly")
	}

	b = NewBinding("test3").
		FromAncestor("org", "name").
		To("user", "orgName").
		Build()
	if b.Source.Resolve != BindAncestor {
		t.Error("FromAncestor not set correctly")
	}

	b = NewBinding("test4").
		FromRef("authorId", "name").
		To("post", "authorName").
		Build()
	if b.Source.Resolve != BindRef {
		t.Error("FromRef not set correctly")
	}

	b = NewBinding("test5").
		FromChild("file", "status").
		To("folder", "childStatus").
		Build()
	if b.Source.Resolve != BindChild {
		t.Error("FromChild not set correctly")
	}

	b = NewBinding("test6").
		FromOutEdge("follows", "name").
		To("user", "following").
		Build()
	if b.Source.Resolve != BindOutEdge {
		t.Error("FromOutEdge not set correctly")
	}

	b = NewBinding("test7").
		FromInEdge("follows", "name").
		To("user", "follower").
		Build()
	if b.Source.Resolve != BindInEdge {
		t.Error("FromInEdge not set correctly")
	}

	b = NewBinding("test8").
		FromNode("node-123", "value").
		To("user", "config").
		Build()
	if b.Source.Resolve != BindAbsolute || b.Source.NodeID != "node-123" {
		t.Error("FromNode not set correctly")
	}

	b = NewBinding("test9").
		FromSibling("config", "color").
		To("item", "color").
		Build()
	if b.Source.Resolve != BindSibling {
		t.Error("FromSibling not set correctly")
	}
}

func TestEvaluateBindingMissingProperty(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{},
	}
	resolver.addNode(node)

	b := NewBinding("b1").FromSelf("nonexistent").To("user", "val").Build()

	value, err := be.EvaluateBinding(b, node, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != nil {
		t.Errorf("expected nil for missing property, got %v", value)
	}
}

func TestEvaluateBindingNoParent(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{},
	}
	resolver.addNode(node)

	b := NewBinding("b1").FromParent("name").To("user", "parentName").Build()

	value, err := be.EvaluateBinding(b, node, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if value != nil {
		t.Errorf("expected nil when no parent, got %v", value)
	}
}

func TestEvaluateBindingBadRefID(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "post",
		Properties: map[string]interface{}{"authorId": "not-a-uuid"},
	}
	resolver.addNode(node)

	b := NewBinding("b1").FromRef("authorId", "name").To("post", "authorName").Build()

	value, _ := be.EvaluateBinding(b, node, resolver)
	if value != nil {
		t.Errorf("expected nil for bad ref ID, got %v", value)
	}
}

func TestEvaluateBindingAbsoluteBadID(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{},
	}

	b := NewBinding("b1").FromNode("not-a-uuid", "prop").To("user", "val").Build()

	_, err := be.EvaluateBinding(b, node, resolver)
	if err == nil {
		t.Error("expected error for bad absolute ID")
	}
}

func TestExpressionDefault(t *testing.T) {
	be := NewBindingEngine()
	resolver := newMockResolver()

	node := &crdt.MaterializedNode{
		ID:         uuid.New(),
		Type:       "user",
		Properties: map[string]interface{}{"name": "source-val"},
	}

	// Use an expression type that falls to default (e.g., ExprAdd which has no impl)
	b := NewBinding("b1").FromSelf("name").To("user", "val").
		WithExpression(&BindingExpression{Type: ExprAdd}).Build()

	value, _ := be.EvaluateBinding(b, node, resolver)
	// Default returns sourceValue
	if value != "source-val" {
		t.Errorf("expected 'source-val', got %v", value)
	}
}
