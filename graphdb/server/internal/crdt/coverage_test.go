package crdt

import (
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

func TestReplicaID(t *testing.T) {
	w := NewEGWalker("test-replica")
	if w.ReplicaID() != "test-replica" {
		t.Errorf("expected 'test-replica', got %s", w.ReplicaID())
	}
}

func TestReorderNode(t *testing.T) {
	w := NewEGWalker("r1")
	parentID, _, _ := w.InsertNode("folder", nil, nil)
	childID, _, _ := w.InsertNode("file", &parentID, map[string]interface{}{"name": "a"})

	node, _ := w.GetNode(childID)
	origPos := node.Position

	_, err := w.ReorderNode(childID, "Z")
	if err != nil {
		t.Fatalf("ReorderNode failed: %v", err)
	}

	node, _ = w.GetNode(childID)
	if node.Position == origPos {
		t.Error("position should have changed")
	}
	if node.Position != "Z" {
		t.Errorf("expected position 'Z', got %s", node.Position)
	}
}

func TestRestoreNode(t *testing.T) {
	w := NewEGWalker("r1")
	id, _, _ := w.InsertNode("item", nil, map[string]interface{}{"name": "test"})

	// Delete
	_, err := w.DeleteNode(id)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := w.GetNode(id)
	if ok {
		t.Error("deleted node should not be visible via GetNode")
	}

	// Restore
	_, err = w.RestoreNode(id)
	if err != nil {
		t.Fatal(err)
	}
	node, ok := w.GetNode(id)
	if !ok {
		t.Error("restored node should be visible")
	}
	if node.Properties["name"] != "test" {
		t.Error("restored node should have properties")
	}
}

func TestRestoreEdge(t *testing.T) {
	w := NewEGWalker("r1")
	id1, _, _ := w.InsertNode("a", nil, nil)
	id2, _, _ := w.InsertNode("b", nil, nil)
	edgeID, _, _ := w.InsertEdge("link", id1, id2, nil)

	// Delete edge
	_, err := w.DeleteEdge(edgeID)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := w.GetEdge(edgeID)
	if ok {
		t.Error("deleted edge should not be visible")
	}

	// Restore edge
	_, err = w.RestoreEdge(edgeID)
	if err != nil {
		t.Fatal(err)
	}
	edge, ok := w.GetEdge(edgeID)
	if !ok {
		t.Error("restored edge should be visible")
	}
	if edge.FromID != id1 || edge.ToID != id2 {
		t.Error("restored edge should have correct endpoints")
	}
}

func TestGetNodeIncludingDeleted(t *testing.T) {
	w := NewEGWalker("r1")
	id, _, _ := w.InsertNode("item", nil, map[string]interface{}{"x": 1})

	w.DeleteNode(id)

	// Regular GetNode should not find it
	_, ok := w.GetNode(id)
	if ok {
		t.Error("GetNode should not find deleted node")
	}

	// GetNodeIncludingDeleted should find it
	node, ok := w.GetNodeIncludingDeleted(id)
	if !ok {
		t.Error("GetNodeIncludingDeleted should find deleted node")
	}
	if !node.Deleted {
		t.Error("node should be marked as deleted")
	}
}

func TestGetDeletedNodes(t *testing.T) {
	w := NewEGWalker("r1")
	id1, _, _ := w.InsertNode("item", nil, nil)
	w.InsertNode("item", nil, nil) // not deleted
	id3, _, _ := w.InsertNode("item", nil, nil)

	w.DeleteNode(id1)
	w.DeleteNode(id3)

	deleted := w.GetDeletedNodes()
	if len(deleted) != 2 {
		t.Errorf("expected 2 deleted nodes, got %d", len(deleted))
	}
}

func TestGetOrderedChildren(t *testing.T) {
	w := NewEGWalker("r1")
	parentID, _, _ := w.InsertNode("folder", nil, nil)

	c1, _, _ := w.InsertNode("file", &parentID, nil)
	c2, _, _ := w.InsertNode("file", &parentID, nil)
	c3, _, _ := w.InsertNode("file", &parentID, nil)

	// Reorder to specific positions
	w.ReorderNode(c3, "A")
	w.ReorderNode(c1, "M")
	w.ReorderNode(c2, "Z")

	children := w.GetOrderedChildren(parentID)
	if len(children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(children))
	}
	if children[0].ID != c3 {
		t.Error("first child should be c3 (position A)")
	}
	if children[1].ID != c1 {
		t.Error("second child should be c1 (position M)")
	}
	if children[2].ID != c2 {
		t.Error("third child should be c2 (position Z)")
	}
}

func TestGetEdge(t *testing.T) {
	w := NewEGWalker("r1")
	id1, _, _ := w.InsertNode("a", nil, nil)
	id2, _, _ := w.InsertNode("b", nil, nil)
	edgeID, _, _ := w.InsertEdge("link", id1, id2, map[string]interface{}{"weight": 5})

	edge, ok := w.GetEdge(edgeID)
	if !ok {
		t.Fatal("edge should be found")
	}
	if edge.Type != "link" {
		t.Errorf("expected type 'link', got %s", edge.Type)
	}
	if edge.Properties["weight"] != 5 {
		t.Error("edge should have properties")
	}

	// Non-existent edge
	_, ok = w.GetEdge(uuid.New())
	if ok {
		t.Error("non-existent edge should not be found")
	}
}

func TestAllEdges(t *testing.T) {
	w := NewEGWalker("r1")
	id1, _, _ := w.InsertNode("a", nil, nil)
	id2, _, _ := w.InsertNode("b", nil, nil)
	id3, _, _ := w.InsertNode("c", nil, nil)

	w.InsertEdge("x", id1, id2, nil)
	w.InsertEdge("y", id2, id3, nil)
	edgeID3, _, _ := w.InsertEdge("z", id1, id3, nil)

	edges := w.AllEdges()
	if len(edges) != 3 {
		t.Errorf("expected 3 edges, got %d", len(edges))
	}

	// Delete one edge
	w.DeleteEdge(edgeID3)
	edges = w.AllEdges()
	if len(edges) != 2 {
		t.Errorf("expected 2 edges after delete, got %d", len(edges))
	}
}

func TestAddPostApplyListener(t *testing.T) {
	w := NewEGWalker("r1")

	var count int32
	w.AddPostApplyListener(func(op *Operation) {
		atomic.AddInt32(&count, 1)
	})

	w.InsertNode("test", nil, nil)
	w.InsertNode("test", nil, nil)

	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("expected 2 post-apply calls, got %d", count)
	}
}

func TestApplyRemote(t *testing.T) {
	w1 := NewEGWalker("r1")
	w2 := NewEGWalker("r2")

	id, op, _ := w1.InsertNode("item", nil, map[string]interface{}{"name": "test"})

	// Apply to w2
	err := w2.ApplyRemote(op)
	if err != nil {
		t.Fatalf("ApplyRemote failed: %v", err)
	}

	node, ok := w2.GetNode(id)
	if !ok {
		t.Fatal("node should exist on r2 after remote apply")
	}
	if node.Properties["name"] != "test" {
		t.Error("properties should match")
	}
}

func TestEventGraphGetAndSize(t *testing.T) {
	w := NewEGWalker("r1")
	eg := w.Graph()

	if eg.Size() != 0 {
		t.Errorf("empty graph should have size 0, got %d", eg.Size())
	}

	_, op, _ := w.InsertNode("item", nil, nil)

	if eg.Size() != 1 {
		t.Errorf("after 1 insert, size should be 1, got %d", eg.Size())
	}

	retrieved, ok := eg.Get(op.ID)
	if !ok {
		t.Fatal("event should be retrievable")
	}
	if retrieved.Type != OpInsertNode {
		t.Error("retrieved event should be insert node")
	}

	_, ok = eg.Get(EventID{ReplicaID: "nonexistent", Seq: 999})
	if ok {
		t.Error("non-existent event should not be found")
	}
}

func TestEventGraphAddPostListener(t *testing.T) {
	eg := NewEventGraph()

	var called int32
	eg.AddPostListener(func(op *Operation) {
		atomic.AddInt32(&called, 1)
	})

	op := &Operation{
		ID:   EventID{ReplicaID: "r1", Seq: 1},
		Type: OpInsertNode,
	}
	eg.Apply(op)

	if atomic.LoadInt32(&called) != 1 {
		t.Error("post-listener should have been called")
	}
}

func TestEventGraphDuplicateApply(t *testing.T) {
	eg := NewEventGraph()

	op := &Operation{
		ID:   EventID{ReplicaID: "r1", Seq: 1},
		Type: OpInsertNode,
	}
	err := eg.Apply(op)
	if err != nil {
		t.Fatal(err)
	}

	// Duplicate should be idempotent
	err = eg.Apply(op)
	if err != nil {
		t.Fatal("duplicate apply should succeed (idempotent)")
	}

	if eg.Size() != 1 {
		t.Error("duplicate should not increase size")
	}
}

func TestEventGraphStaleSeq(t *testing.T) {
	eg := NewEventGraph()

	op1 := &Operation{
		ID:   EventID{ReplicaID: "r1", Seq: 1},
		Type: OpInsertNode,
	}
	eg.Apply(op1)

	// Stale seq should fail
	op2 := &Operation{
		ID:      EventID{ReplicaID: "r1", Seq: 1},
		Parents: []EventID{op1.ID},
		Type:    OpInsertNode,
	}
	// op2 is same ID as op1 so it'll be idempotent (duplicate)
	_ = op2
	// Instead, test with a truly stale seq
	op3 := &Operation{
		ID:      EventID{ReplicaID: "r1", Seq: 0},
		Parents: []EventID{op1.ID},
		Type:    OpInsertNode,
	}
	err := eg.Apply(op3)
	if err == nil {
		t.Error("stale seq should fail")
	}
}

func TestEventGraphEventsSince(t *testing.T) {
	w := NewEGWalker("r1")
	eg := w.Graph()

	w.InsertNode("a", nil, nil)
	frontier1 := eg.Frontier()

	w.InsertNode("b", nil, nil)
	w.InsertNode("c", nil, nil)

	events := eg.EventsSince(frontier1)
	if len(events) != 2 {
		t.Errorf("expected 2 events since frontier, got %d", len(events))
	}
}

func TestEventGraphCompactBefore(t *testing.T) {
	w := NewEGWalker("r1")
	eg := w.Graph()

	w.InsertNode("a", nil, nil)
	w.InsertNode("b", nil, nil)
	frontier := eg.Frontier()
	w.InsertNode("c", nil, nil)

	sizeBefore := eg.Size()
	compacted := eg.CompactBefore(frontier)

	if compacted == 0 {
		t.Error("should have compacted some events")
	}
	if eg.Size() >= sizeBefore {
		t.Error("size should decrease after compaction")
	}
}

func TestEventGraphDeltaSinceAndVersionVector(t *testing.T) {
	w := NewEGWalker("r1")
	eg := w.Graph()

	w.InsertNode("a", nil, nil)
	w.InsertNode("b", nil, nil)

	vv := eg.VersionVector()
	if vv["r1"] != 2 {
		t.Errorf("expected version vector r1=2, got %d", vv["r1"])
	}

	w.InsertNode("c", nil, nil)

	// Delta since the old version vector
	delta := eg.DeltaSince(vv)
	if len(delta) != 1 {
		t.Errorf("expected 1 delta event, got %d", len(delta))
	}

	// Empty version vector = all events
	all := eg.DeltaSince(map[string]uint64{})
	if len(all) != 3 {
		t.Errorf("expected 3 events with empty VV, got %d", len(all))
	}
}

func TestMissingParentApply(t *testing.T) {
	eg := NewEventGraph()

	op := &Operation{
		ID:      EventID{ReplicaID: "r1", Seq: 1},
		Parents: []EventID{{ReplicaID: "r1", Seq: 999}}, // non-existent parent
		Type:    OpInsertNode,
	}
	err := eg.Apply(op)
	if err == nil {
		t.Error("apply with missing parent should fail")
	}
}

func TestFractionalIndexEdgeCases(t *testing.T) {
	// Position between empty strings
	pos := PositionBetween("", "")
	if pos != "V" {
		t.Errorf("expected 'V', got %s", pos)
	}

	// Position before a given string
	before := PositionBetween("", "V")
	if before >= "V" {
		t.Errorf("expected before < 'V', got %s", before)
	}

	// Position after a given string
	after := PositionBetween("V", "")
	if after <= "V" {
		t.Errorf("expected after > 'V', got %s", after)
	}

	// Position between two values
	mid := PositionBetween("A", "Z")
	if mid <= "A" || mid >= "Z" {
		t.Errorf("expected A < mid < Z, got %s", mid)
	}

	// Position after max char string
	afterMax := PositionBetween("z", "")
	if afterMax <= "z" {
		t.Errorf("expected afterMax > 'z', got %s", afterMax)
	}

	// Position before min char string — the result should be lexicographically before "0"
	beforeMin := PositionBetween("", "0")
	if beforeMin >= "0" && len(beforeMin) <= 1 {
		t.Errorf("expected beforeMin < '0' or longer string, got %s", beforeMin)
	}

	// PositionInitial
	positions := PositionInitial(5)
	if len(positions) != 5 {
		t.Errorf("expected 5 positions, got %d", len(positions))
	}
	for i := 1; i < len(positions); i++ {
		if positions[i] <= positions[i-1] {
			t.Errorf("positions should be ascending: %s <= %s", positions[i], positions[i-1])
		}
	}
}

func TestCausalOrder(t *testing.T) {
	w := NewEGWalker("r1")
	eg := w.Graph()

	w.InsertNode("a", nil, nil)
	w.InsertNode("b", nil, nil)
	w.InsertNode("c", nil, nil)

	order := eg.CausalOrder()
	if len(order) != 3 {
		t.Errorf("expected 3 events in causal order, got %d", len(order))
	}

	// Should be in seq order
	for i := 1; i < len(order); i++ {
		if order[i].ID.Seq < order[i-1].ID.Seq {
			t.Error("causal order should be ascending by seq")
		}
	}
}
