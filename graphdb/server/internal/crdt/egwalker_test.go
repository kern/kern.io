package crdt

import (
	"testing"

	"github.com/google/uuid"
)

func TestInsertAndGetNode(t *testing.T) {
	w := NewEGWalker("replica-1")

	id, _, err := w.InsertNode("user", nil, map[string]interface{}{
		"name": "Alice",
		"age":  30,
	})
	if err != nil {
		t.Fatalf("InsertNode failed: %v", err)
	}

	node, ok := w.GetNode(id)
	if !ok {
		t.Fatal("GetNode returned false")
	}
	if node.Type != "user" {
		t.Errorf("expected type 'user', got %q", node.Type)
	}
	if node.Properties["name"] != "Alice" {
		t.Errorf("expected name 'Alice', got %v", node.Properties["name"])
	}
	if node.ParentID != nil {
		t.Error("expected nil parent")
	}
}

func TestDeleteNode(t *testing.T) {
	w := NewEGWalker("replica-1")

	id, _, err := w.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})
	if err != nil {
		t.Fatalf("InsertNode failed: %v", err)
	}

	_, err = w.DeleteNode(id)
	if err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}

	_, ok := w.GetNode(id)
	if ok {
		t.Error("expected GetNode to return false for deleted node")
	}
}

func TestSetProperty(t *testing.T) {
	w := NewEGWalker("replica-1")

	id, _, _ := w.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})

	_, err := w.SetProperty(id, "email", "alice@example.com")
	if err != nil {
		t.Fatalf("SetProperty failed: %v", err)
	}

	node, _ := w.GetNode(id)
	if node.Properties["email"] != "alice@example.com" {
		t.Errorf("expected email 'alice@example.com', got %v", node.Properties["email"])
	}
}

func TestHierarchy(t *testing.T) {
	w := NewEGWalker("replica-1")

	parentID, _, _ := w.InsertNode("org", nil, map[string]interface{}{"name": "Acme"})
	childID, _, err := w.InsertNode("team", &parentID, map[string]interface{}{"name": "Engineering"})
	if err != nil {
		t.Fatalf("InsertNode with parent failed: %v", err)
	}

	// Check parent
	parent, ok := w.GetParent(childID)
	if !ok {
		t.Fatal("GetParent returned false")
	}
	if parent.ID != parentID {
		t.Errorf("expected parent ID %s, got %s", parentID, parent.ID)
	}

	// Check children
	children := w.GetChildren(parentID)
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	if children[0].ID != childID {
		t.Errorf("expected child ID %s, got %s", childID, children[0].ID)
	}

	// Check roots
	roots := w.GetRoots()
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
}

func TestEdges(t *testing.T) {
	w := NewEGWalker("replica-1")

	user1, _, _ := w.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	user2, _, _ := w.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})

	edgeID, _, err := w.InsertEdge("follows", user1, user2, map[string]interface{}{"since": "2024"})
	if err != nil {
		t.Fatalf("InsertEdge failed: %v", err)
	}

	// Check outgoing edges
	outEdges := w.GetOutEdges(user1)
	if len(outEdges) != 1 {
		t.Fatalf("expected 1 outgoing edge, got %d", len(outEdges))
	}
	if outEdges[0].Type != "follows" {
		t.Errorf("expected edge type 'follows', got %q", outEdges[0].Type)
	}

	// Check incoming edges
	inEdges := w.GetInEdges(user2)
	if len(inEdges) != 1 {
		t.Fatalf("expected 1 incoming edge, got %d", len(inEdges))
	}

	// Delete edge
	_, err = w.DeleteEdge(edgeID)
	if err != nil {
		t.Fatalf("DeleteEdge failed: %v", err)
	}

	outEdges = w.GetOutEdges(user1)
	if len(outEdges) != 0 {
		t.Errorf("expected 0 outgoing edges after delete, got %d", len(outEdges))
	}
}

func TestMoveNode(t *testing.T) {
	w := NewEGWalker("replica-1")

	parent1, _, _ := w.InsertNode("org", nil, map[string]interface{}{"name": "Org1"})
	parent2, _, _ := w.InsertNode("org", nil, map[string]interface{}{"name": "Org2"})
	child, _, _ := w.InsertNode("team", &parent1, map[string]interface{}{"name": "Team"})

	_, err := w.MoveNode(child, &parent2)
	if err != nil {
		t.Fatalf("MoveNode failed: %v", err)
	}

	// Old parent should have no children
	if len(w.GetChildren(parent1)) != 0 {
		t.Error("old parent still has children")
	}

	// New parent should have the child
	children := w.GetChildren(parent2)
	if len(children) != 1 || children[0].ID != child {
		t.Error("child not moved to new parent")
	}
}

func TestMoveNodeCycleDetection(t *testing.T) {
	w := NewEGWalker("replica-1")

	parent, _, _ := w.InsertNode("org", nil, map[string]interface{}{})
	child, _, _ := w.InsertNode("team", &parent, map[string]interface{}{})

	// Try to make parent a child of its own child — should fail
	_, err := w.MoveNode(parent, &child)
	if err == nil {
		t.Error("expected cycle detection error")
	}
}

func TestGetNodesByType(t *testing.T) {
	w := NewEGWalker("replica-1")

	w.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	w.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})
	w.InsertNode("org", nil, map[string]interface{}{"name": "Acme"})

	users := w.GetNodesByType("user")
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}

	orgs := w.GetNodesByType("org")
	if len(orgs) != 1 {
		t.Errorf("expected 1 org, got %d", len(orgs))
	}
}

func TestConcurrentPropertyWrites(t *testing.T) {
	// Simulate two replicas making concurrent writes to the same property.
	// The one with the higher (Seq, ReplicaID) should win (LWW).
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	// Create a node on replica-a
	id, op1, _ := w1.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})

	// Apply the creation on replica-b
	err := w2.ApplyRemote(op1)
	if err != nil {
		t.Fatalf("ApplyRemote failed: %v", err)
	}

	// Both replicas concurrently set the same property
	opA, _ := w1.SetProperty(id, "name", "Alice-A")
	opB, _ := w2.SetProperty(id, "name", "Alice-B")

	// Cross-apply
	w1.ApplyRemote(opB)
	w2.ApplyRemote(opA)

	// Both should converge to the same value
	nodeA, _ := w1.GetNode(id)
	nodeB, _ := w2.GetNode(id)

	if nodeA.Properties["name"] != nodeB.Properties["name"] {
		t.Errorf("replicas diverged: replica-a=%v, replica-b=%v",
			nodeA.Properties["name"], nodeB.Properties["name"])
	}
}

func TestEventGraphCausalOrder(t *testing.T) {
	eg := NewEventGraph()

	op1 := &Operation{
		ID:      EventID{ReplicaID: "r1", Seq: 1},
		Parents: nil,
		Type:    OpInsertNode,
	}
	op2 := &Operation{
		ID:      EventID{ReplicaID: "r1", Seq: 2},
		Parents: []EventID{op1.ID},
		Type:    OpSetProperty,
	}
	op3 := &Operation{
		ID:      EventID{ReplicaID: "r2", Seq: 1},
		Parents: []EventID{op1.ID},
		Type:    OpSetProperty,
	}

	eg.Apply(op1)
	eg.Apply(op2)
	eg.Apply(op3)

	order := eg.CausalOrder()
	if len(order) != 3 {
		t.Fatalf("expected 3 events, got %d", len(order))
	}
	// op1 should be first
	if order[0].ID != op1.ID {
		t.Errorf("expected op1 first, got %v", order[0].ID)
	}
}

func TestEventGraphFrontier(t *testing.T) {
	eg := NewEventGraph()

	op1 := &Operation{ID: EventID{ReplicaID: "r1", Seq: 1}, Parents: nil, Type: OpInsertNode}
	op2 := &Operation{ID: EventID{ReplicaID: "r1", Seq: 2}, Parents: []EventID{op1.ID}, Type: OpSetProperty}

	eg.Apply(op1)
	frontier := eg.Frontier()
	if len(frontier) != 1 || frontier[0] != op1.ID {
		t.Error("frontier after op1 should be [op1]")
	}

	eg.Apply(op2)
	frontier = eg.Frontier()
	if len(frontier) != 1 || frontier[0] != op2.ID {
		t.Error("frontier after op2 should be [op2]")
	}
}

func TestDeleteProperty(t *testing.T) {
	w := NewEGWalker("replica-1")

	id, _, _ := w.InsertNode("user", nil, map[string]interface{}{"name": "Alice", "age": 30})

	_, err := w.DeleteProperty(id, "age")
	if err != nil {
		t.Fatalf("DeleteProperty failed: %v", err)
	}

	node, _ := w.GetNode(id)
	if _, ok := node.Properties["age"]; ok {
		t.Error("property 'age' should have been deleted")
	}
	if node.Properties["name"] != "Alice" {
		t.Error("property 'name' should still exist")
	}
}

// Ensure uuid is used
var _ = uuid.New
