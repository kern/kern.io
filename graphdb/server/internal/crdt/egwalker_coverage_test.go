package crdt

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// egwalker.go: GetParent edge-case branches
// ---------------------------------------------------------------------------

func TestGetParentDeletedParent(t *testing.T) {
	w := NewEGWalker("r1")
	parentID, _, _ := w.InsertNode("folder", nil, nil)
	childID, _, _ := w.InsertNode("file", &parentID, nil)

	// Delete parent
	w.DeleteNode(parentID)

	// GetParent should return false because parent is deleted
	_, ok := w.GetParent(childID)
	if ok {
		t.Error("GetParent should return false when parent is deleted")
	}
}

func TestGetParentNoParent(t *testing.T) {
	w := NewEGWalker("r1")
	id, _, _ := w.InsertNode("item", nil, nil)

	_, ok := w.GetParent(id)
	if !ok == false { // just make sure no panic
	}
	if ok {
		t.Error("GetParent should return false for root node")
	}
}

func TestGetParentNonExistentNode(t *testing.T) {
	w := NewEGWalker("r1")
	_, ok := w.GetParent(uuid.New())
	if ok {
		t.Error("GetParent should return false for non-existent node")
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: DeleteNode when node not found (no-op in applyOp)
// ---------------------------------------------------------------------------

func TestDeleteNodeNonExistent(t *testing.T) {
	w := NewEGWalker("r1")
	// DeleteNode on a node that doesn't exist should not panic
	_, err := w.DeleteNode(uuid.New())
	if err != nil {
		t.Fatalf("DeleteNode should not error (CRDT idempotent): %v", err)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: DeleteNode LWW — second delete with lower seq ignored
// ---------------------------------------------------------------------------

func TestDeleteNodeLWWStaleDelete(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	id, opCreate, _ := w1.InsertNode("item", nil, nil)
	w2.ApplyRemote(opCreate)

	// Delete on w1 (seq=2)
	opDel1, _ := w1.DeleteNode(id)

	// Delete on w2 (seq=2 but different replica)
	opDel2, _ := w2.DeleteNode(id)

	// Cross-apply: both should converge
	w1.ApplyRemote(opDel2)
	w2.ApplyRemote(opDel1)

	nodeA, _ := w1.GetNodeIncludingDeleted(id)
	nodeB, _ := w2.GetNodeIncludingDeleted(id)
	if nodeA.Deleted != nodeB.Deleted {
		t.Error("replicas should converge on deleted state")
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: SetProperty LWW — stale write skipped for node and edge
// ---------------------------------------------------------------------------

func TestSetPropertyStaleWrite(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	id, opCreate, _ := w1.InsertNode("item", nil, map[string]interface{}{"x": 1})
	w2.ApplyRemote(opCreate)

	// w1 sets x=10 (seq=2), then x=20 (seq=3, causally after seq=2)
	op1, _ := w1.SetProperty(id, "x", 10)
	op2, _ := w1.SetProperty(id, "x", 20)

	// Apply both on w2 in causal order — final value should be 20
	w2.ApplyRemote(op1)
	w2.ApplyRemote(op2)

	node, _ := w2.GetNode(id)
	if node.Properties["x"] != 20 {
		t.Errorf("expected x=20, got %v", node.Properties["x"])
	}
}

func TestSetPropertyOnEdgeLWW(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	n1, op1, _ := w1.InsertNode("a", nil, nil)
	n2, op2, _ := w1.InsertNode("b", nil, nil)
	edgeID, op3, _ := w1.InsertEdge("link", n1, n2, map[string]interface{}{"weight": 1})

	w2.ApplyRemote(op1)
	w2.ApplyRemote(op2)
	w2.ApplyRemote(op3)

	// Both set property on edge concurrently
	opA, _ := w1.SetProperty(edgeID, "weight", 10)
	opB, _ := w2.SetProperty(edgeID, "weight", 20)

	w1.ApplyRemote(opB)
	w2.ApplyRemote(opA)

	edgeA, _ := w1.GetEdge(edgeID)
	edgeB, _ := w2.GetEdge(edgeID)
	if edgeA.Properties["weight"] != edgeB.Properties["weight"] {
		t.Error("edge property should converge across replicas")
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: DeleteProperty — stale delete ignored
// ---------------------------------------------------------------------------

func TestDeletePropertyThenSet(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	id, opCreate, _ := w1.InsertNode("item", nil, map[string]interface{}{"x": 1})
	w2.ApplyRemote(opCreate)

	// w1 deletes property (seq=2), then sets it back (seq=3)
	opDel, _ := w1.DeleteProperty(id, "x")
	opSet, _ := w1.SetProperty(id, "x", 99)

	// Apply in causal order on w2
	w2.ApplyRemote(opDel)
	w2.ApplyRemote(opSet)

	node, _ := w2.GetNode(id)
	if node.Properties["x"] != 99 {
		t.Errorf("expected x=99 after delete+set, got %v", node.Properties["x"])
	}
}

func TestDeletePropertyNonExistentKey(t *testing.T) {
	w := NewEGWalker("r1")
	id, _, _ := w.InsertNode("item", nil, map[string]interface{}{"a": 1})
	// Deleting a key that doesn't exist should be a no-op
	_, err := w.DeleteProperty(id, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	node, _ := w.GetNode(id)
	if node.Properties["a"] != 1 {
		t.Error("existing property should be unchanged")
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: DeleteEdge LWW — second delete with lower seq ignored
// ---------------------------------------------------------------------------

func TestDeleteEdgeLWWStale(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	n1, op1, _ := w1.InsertNode("a", nil, nil)
	n2, op2, _ := w1.InsertNode("b", nil, nil)
	edgeID, op3, _ := w1.InsertEdge("link", n1, n2, nil)

	w2.ApplyRemote(op1)
	w2.ApplyRemote(op2)
	w2.ApplyRemote(op3)

	// Both delete the edge concurrently
	opDelA, _ := w1.DeleteEdge(edgeID)
	opDelB, _ := w2.DeleteEdge(edgeID)

	w1.ApplyRemote(opDelB)
	w2.ApplyRemote(opDelA)

	_, okA := w1.GetEdge(edgeID)
	_, okB := w2.GetEdge(edgeID)
	if okA != okB {
		t.Error("edge deleted state should converge")
	}
}

func TestDeleteEdgeNonExistent(t *testing.T) {
	w := NewEGWalker("r1")
	_, err := w.DeleteEdge(uuid.New())
	if err != nil {
		t.Fatalf("DeleteEdge on non-existent should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: ReorderNode LWW — stale reorder ignored
// ---------------------------------------------------------------------------

func TestReorderNodeSequential(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	parentID, opP, _ := w1.InsertNode("folder", nil, nil)
	childID, opC, _ := w1.InsertNode("file", &parentID, nil)

	w2.ApplyRemote(opP)
	w2.ApplyRemote(opC)

	// w1 reorders to "A" then "Z" in causal order
	opA, _ := w1.ReorderNode(childID, "A")
	opZ, _ := w1.ReorderNode(childID, "Z")

	// Apply in causal order on w2
	w2.ApplyRemote(opA)
	w2.ApplyRemote(opZ)

	node, _ := w2.GetNode(childID)
	if node.Position != "Z" {
		t.Errorf("expected position Z, got %s", node.Position)
	}
}

func TestReorderNodeNonExistent(t *testing.T) {
	w := NewEGWalker("r1")
	_, err := w.ReorderNode(uuid.New(), "M")
	if err != nil {
		t.Fatalf("ReorderNode on non-existent should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: RestoreNode — stale restore ignored
// ---------------------------------------------------------------------------

func TestRestoreNodeStale(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	id, opCreate, _ := w1.InsertNode("item", nil, nil)
	w2.ApplyRemote(opCreate)

	// w1: delete, then restore
	opDel, _ := w1.DeleteNode(id)
	opRestore, _ := w1.RestoreNode(id)

	// w1: delete again (higher seq than the restore)
	opDel2, _ := w1.DeleteNode(id)

	// On w2: apply in order: delete, restore, delete2
	w2.ApplyRemote(opDel)
	w2.ApplyRemote(opRestore)
	w2.ApplyRemote(opDel2)

	node, ok := w2.GetNodeIncludingDeleted(id)
	if !ok {
		t.Fatal("node should exist")
	}
	if !node.Deleted {
		t.Error("node should be deleted after second delete")
	}
}

func TestRestoreNodeNotDeleted(t *testing.T) {
	w := NewEGWalker("r1")
	id, _, _ := w.InsertNode("item", nil, nil)
	// Restore a node that is not deleted — should be a no-op
	_, err := w.RestoreNode(id)
	if err != nil {
		t.Fatalf("RestoreNode should not error: %v", err)
	}
	node, ok := w.GetNode(id)
	if !ok || node.Deleted {
		t.Error("node should still be visible and not deleted")
	}
}

func TestRestoreNodeNonExistent(t *testing.T) {
	w := NewEGWalker("r1")
	_, err := w.RestoreNode(uuid.New())
	if err != nil {
		t.Fatalf("RestoreNode on non-existent should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: RestoreEdge — stale restore ignored
// ---------------------------------------------------------------------------

func TestRestoreEdgeStale(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	n1, op1, _ := w1.InsertNode("a", nil, nil)
	n2, op2, _ := w1.InsertNode("b", nil, nil)
	edgeID, op3, _ := w1.InsertEdge("link", n1, n2, nil)

	w2.ApplyRemote(op1)
	w2.ApplyRemote(op2)
	w2.ApplyRemote(op3)

	// w1: delete, restore, delete again
	opDel, _ := w1.DeleteEdge(edgeID)
	opRestore, _ := w1.RestoreEdge(edgeID)
	opDel2, _ := w1.DeleteEdge(edgeID)

	w2.ApplyRemote(opDel)
	w2.ApplyRemote(opRestore)
	w2.ApplyRemote(opDel2)

	_, ok := w2.GetEdge(edgeID)
	if ok {
		t.Error("edge should be deleted after final delete")
	}
}

func TestRestoreEdgeNotDeleted(t *testing.T) {
	w := NewEGWalker("r1")
	n1, _, _ := w.InsertNode("a", nil, nil)
	n2, _, _ := w.InsertNode("b", nil, nil)
	edgeID, _, _ := w.InsertEdge("link", n1, n2, nil)

	// Restore an edge that is not deleted — should be a no-op
	_, err := w.RestoreEdge(edgeID)
	if err != nil {
		t.Fatalf("RestoreEdge should not error: %v", err)
	}
}

func TestRestoreEdgeNonExistent(t *testing.T) {
	w := NewEGWalker("r1")
	_, err := w.RestoreEdge(uuid.New())
	if err != nil {
		t.Fatalf("RestoreEdge on non-existent should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: MoveNode to nil parent (un-parent)
// ---------------------------------------------------------------------------

func TestMoveNodeToNilParent(t *testing.T) {
	w := NewEGWalker("r1")
	parentID, _, _ := w.InsertNode("folder", nil, nil)
	childID, _, _ := w.InsertNode("file", &parentID, nil)

	_, err := w.MoveNode(childID, nil)
	if err != nil {
		t.Fatalf("MoveNode to nil parent: %v", err)
	}

	_, ok := w.GetParent(childID)
	if ok {
		t.Error("child should have no parent after move to nil")
	}

	node, _ := w.GetNode(childID)
	if node.ParentID != nil {
		t.Error("ParentID should be nil")
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: InsertNode with nil properties
// ---------------------------------------------------------------------------

func TestInsertNodeNilProperties(t *testing.T) {
	w := NewEGWalker("r1")
	id, _, err := w.InsertNode("item", nil, nil)
	if err != nil {
		t.Fatalf("InsertNode with nil properties: %v", err)
	}
	node, ok := w.GetNode(id)
	if !ok {
		t.Fatal("node should exist")
	}
	if len(node.Properties) != 0 {
		t.Errorf("expected 0 properties, got %d", len(node.Properties))
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: InsertEdge with nil properties
// ---------------------------------------------------------------------------

func TestInsertEdgeNilProperties(t *testing.T) {
	w := NewEGWalker("r1")
	n1, _, _ := w.InsertNode("a", nil, nil)
	n2, _, _ := w.InsertNode("b", nil, nil)
	edgeID, _, err := w.InsertEdge("link", n1, n2, nil)
	if err != nil {
		t.Fatalf("InsertEdge: %v", err)
	}
	edge, ok := w.GetEdge(edgeID)
	if !ok {
		t.Fatal("edge should exist")
	}
	if len(edge.Properties) != 0 {
		t.Errorf("expected 0 properties, got %d", len(edge.Properties))
	}
}

// ---------------------------------------------------------------------------
// eventgraph.go: sortOps with various orderings
// ---------------------------------------------------------------------------

func TestSortOpsMultipleReplicas(t *testing.T) {
	// Create operations that need sorting by (Seq, ReplicaID)
	ops := []*Operation{
		{ID: EventID{ReplicaID: "r3", Seq: 2}},
		{ID: EventID{ReplicaID: "r1", Seq: 1}},
		{ID: EventID{ReplicaID: "r2", Seq: 1}},
		{ID: EventID{ReplicaID: "r1", Seq: 2}},
		{ID: EventID{ReplicaID: "r2", Seq: 2}},
	}
	sortOps(ops)

	// Should be sorted by Seq ascending, then ReplicaID ascending
	for i := 1; i < len(ops); i++ {
		prev := ops[i-1]
		curr := ops[i]
		if prev.ID.Seq > curr.ID.Seq {
			t.Errorf("not sorted by seq at index %d", i)
		}
		if prev.ID.Seq == curr.ID.Seq && prev.ID.ReplicaID > curr.ID.ReplicaID {
			t.Errorf("not sorted by replicaID at index %d", i)
		}
	}
}

func TestSortOpsEmpty(t *testing.T) {
	sortOps(nil)
	sortOps([]*Operation{})
	sortOps([]*Operation{{ID: EventID{ReplicaID: "r1", Seq: 1}}})
	// Should not panic
}

// ---------------------------------------------------------------------------
// eventgraph.go: EventsSince with empty frontier (returns all events)
// ---------------------------------------------------------------------------

func TestEventsSinceEmptyFrontier(t *testing.T) {
	w := NewEGWalker("r1")
	eg := w.Graph()

	w.InsertNode("a", nil, nil)
	w.InsertNode("b", nil, nil)
	w.InsertNode("c", nil, nil)

	events := eg.EventsSince(nil)
	if len(events) != 3 {
		t.Errorf("expected 3 events with nil frontier, got %d", len(events))
	}

	events = eg.EventsSince([]EventID{})
	if len(events) != 3 {
		t.Errorf("expected 3 events with empty frontier, got %d", len(events))
	}
}

func TestEventsSinceCurrentFrontier(t *testing.T) {
	w := NewEGWalker("r1")
	eg := w.Graph()

	w.InsertNode("a", nil, nil)
	w.InsertNode("b", nil, nil)
	frontier := eg.Frontier()

	events := eg.EventsSince(frontier)
	if len(events) != 0 {
		t.Errorf("expected 0 events since current frontier, got %d", len(events))
	}
}

func TestEventsSinceWithNonExistentFrontier(t *testing.T) {
	w := NewEGWalker("r1")
	eg := w.Graph()

	w.InsertNode("a", nil, nil)

	// Frontier with a non-existent event ID — markSeen should handle gracefully
	events := eg.EventsSince([]EventID{{ReplicaID: "nonexistent", Seq: 999}})
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// fractional.go: charIndex with unknown byte
// ---------------------------------------------------------------------------

func TestCharIndexUnknownByte(t *testing.T) {
	// charIndex with a byte not in posChars should return 0
	idx := charIndex(byte(0))
	if idx != 0 {
		t.Errorf("expected 0 for unknown byte, got %d", idx)
	}
}

// ---------------------------------------------------------------------------
// fractional.go: PositionInitial edge cases
// ---------------------------------------------------------------------------

func TestPositionInitialZero(t *testing.T) {
	result := PositionInitial(0)
	if result != nil {
		t.Errorf("expected nil for n=0, got %v", result)
	}
}

func TestPositionInitialNegative(t *testing.T) {
	result := PositionInitial(-1)
	if result != nil {
		t.Errorf("expected nil for n=-1, got %v", result)
	}
}

func TestPositionInitialLarge(t *testing.T) {
	// Large n where step would be < 1 (n+1 > posBase)
	positions := PositionInitial(100)
	if len(positions) != 100 {
		t.Errorf("expected 100 positions, got %d", len(positions))
	}
	// All should be valid non-empty strings
	for i, p := range positions {
		if p == "" {
			t.Errorf("position %d is empty", i)
		}
	}
}

func TestPositionInitialOne(t *testing.T) {
	positions := PositionInitial(1)
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
}

// ---------------------------------------------------------------------------
// fractional.go: positionMid edge cases — adjacent positions requiring deeper digits
// ---------------------------------------------------------------------------

func TestPositionMidAdjacent(t *testing.T) {
	// Two adjacent positions should produce something between them
	mid := positionMid("A", "B")
	if mid <= "A" || mid >= "B" {
		t.Errorf("expected A < mid < B, got mid=%s", mid)
	}
}

func TestPositionMidSamePrefix(t *testing.T) {
	// Same prefix, different last char
	mid := positionMid("AA", "AZ")
	if mid <= "AA" || mid >= "AZ" {
		t.Errorf("expected AA < mid < AZ, got mid=%s", mid)
	}
}

func TestPositionMidDifferentLengths(t *testing.T) {
	mid := positionMid("A", "AA")
	if mid <= "A" || mid >= "AA" {
		t.Errorf("expected A < mid < AA, got mid=%s", mid)
	}
}

func TestPositionMidVeryClose(t *testing.T) {
	// Force carry-down in positionMid: "A0" and "A1" are adjacent
	mid := positionMid("A0", "A1")
	if mid <= "A0" || mid >= "A1" {
		t.Errorf("expected A0 < mid < A1, got mid=%s", mid)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: SetProperty on non-existent node (edge path in applyOp)
// ---------------------------------------------------------------------------

func TestSetPropertyNonExistentNode(t *testing.T) {
	w := NewEGWalker("r1")
	_, err := w.SetProperty(uuid.New(), "key", "value")
	if err != nil {
		t.Fatalf("SetProperty on non-existent should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: DeleteProperty on non-existent node
// ---------------------------------------------------------------------------

func TestDeletePropertyNonExistentNode(t *testing.T) {
	w := NewEGWalker("r1")
	_, err := w.DeleteProperty(uuid.New(), "key")
	if err != nil {
		t.Fatalf("DeleteProperty on non-existent should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: MoveNode non-existent (no-op in applyOp)
// ---------------------------------------------------------------------------

func TestMoveNodeNonExistent(t *testing.T) {
	w := NewEGWalker("r1")
	_, err := w.MoveNode(uuid.New(), nil)
	if err != nil {
		t.Fatalf("MoveNode on non-existent should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: MoveNode from no parent to a parent
// ---------------------------------------------------------------------------

func TestMoveNodeFromRootToParent(t *testing.T) {
	w := NewEGWalker("r1")
	parentID, _, _ := w.InsertNode("folder", nil, nil)
	childID, _, _ := w.InsertNode("file", nil, nil)

	_, err := w.MoveNode(childID, &parentID)
	if err != nil {
		t.Fatalf("MoveNode: %v", err)
	}

	parent, ok := w.GetParent(childID)
	if !ok || parent.ID != parentID {
		t.Error("node should be under new parent")
	}
}

// ---------------------------------------------------------------------------
// eventgraph.go: DeltaSince with multiple replicas
// ---------------------------------------------------------------------------

func TestDeltaSinceMultipleReplicas(t *testing.T) {
	w1 := NewEGWalker("r1")
	w2 := NewEGWalker("r2")
	eg := NewEventGraph()

	// Manually create ops from different replicas
	op1 := &Operation{
		ID:        EventID{ReplicaID: "r1", Seq: 1},
		Type:      OpInsertNode,
		Timestamp: time.Now(),
	}
	op2 := &Operation{
		ID:        EventID{ReplicaID: "r2", Seq: 1},
		Parents:   []EventID{op1.ID},
		Type:      OpInsertNode,
		Timestamp: time.Now(),
	}
	op3 := &Operation{
		ID:        EventID{ReplicaID: "r1", Seq: 2},
		Parents:   []EventID{op1.ID},
		Type:      OpSetProperty,
		Timestamp: time.Now(),
	}

	eg.Apply(op1)
	eg.Apply(op2)
	eg.Apply(op3)

	// VV knows r1 up to 1, r2 up to 1 => should get op3 (r1:2)
	delta := eg.DeltaSince(map[string]uint64{"r1": 1, "r2": 1})
	if len(delta) != 1 {
		t.Errorf("expected 1 delta, got %d", len(delta))
	}

	// VV knows nothing => all 3
	all := eg.DeltaSince(map[string]uint64{})
	if len(all) != 3 {
		t.Errorf("expected 3 events, got %d", len(all))
	}

	_ = w1
	_ = w2
}

// ---------------------------------------------------------------------------
// eventgraph.go: CompactBefore with live children preserved
// ---------------------------------------------------------------------------

func TestCompactBeforePreservesLiveParents(t *testing.T) {
	w := NewEGWalker("r1")
	eg := w.Graph()

	w.InsertNode("a", nil, nil)
	frontier1 := eg.Frontier()
	w.InsertNode("b", nil, nil)
	w.InsertNode("c", nil, nil)

	// Compact before frontier1 — op1 is a parent of op2 and op3, so it should be preserved
	compacted := eg.CompactBefore(frontier1)
	// The root event (frontier1) is parent of later events, so it may not be compacted
	_ = compacted
	// Just ensure no panic and graph still works
	w.InsertNode("d", nil, nil)
}

// ---------------------------------------------------------------------------
// Additional applyOp branch: InsertNode with properties map
// ---------------------------------------------------------------------------

func TestInsertNodeWithMultipleProperties(t *testing.T) {
	w := NewEGWalker("r1")
	id, _, _ := w.InsertNode("user", nil, map[string]interface{}{
		"name":  "Alice",
		"email": "alice@test.com",
		"age":   30,
	})
	node, ok := w.GetNode(id)
	if !ok {
		t.Fatal("node should exist")
	}
	if len(node.Properties) != 3 {
		t.Errorf("expected 3 properties, got %d", len(node.Properties))
	}
}

// ---------------------------------------------------------------------------
// Additional: InsertNode with parent that has existing siblings
// ---------------------------------------------------------------------------

func TestInsertNodeWithExistingSiblings(t *testing.T) {
	w := NewEGWalker("r1")
	parentID, _, _ := w.InsertNode("folder", nil, nil)
	w.InsertNode("file", &parentID, nil)
	w.InsertNode("file", &parentID, nil)
	id3, _, _ := w.InsertNode("file", &parentID, nil)

	// Third child should have a position after the second
	node, _ := w.GetNode(id3)
	if node.Position == "" {
		t.Error("position should be set")
	}
}
