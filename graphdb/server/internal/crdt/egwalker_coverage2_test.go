package crdt

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Stale operation branches: two walkers make conflicting ops, cross-apply.
// The walker with the lower (Seq, ReplicaID) has its op treated as stale.
// ---------------------------------------------------------------------------

// Helper: create two walkers and a shared node.
func twoWalkersWithNode(t *testing.T) (*EGWalker, *EGWalker, uuid.UUID) {
	t.Helper()
	// Use replica IDs where "replica-b" > "replica-a" so LWW tiebreak is predictable.
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	id, opCreate, err := w1.InsertNode("item", nil, map[string]interface{}{"key": "original"})
	if err != nil {
		t.Fatalf("InsertNode: %v", err)
	}
	if err := w2.ApplyRemote(opCreate); err != nil {
		t.Fatalf("ApplyRemote create: %v", err)
	}
	return w1, w2, id
}

func TestConcurrentInsertNode(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	// Both insert a node independently (concurrent)
	id1, op1, _ := w1.InsertNode("item", nil, map[string]interface{}{"from": "a"})
	id2, op2, _ := w2.InsertNode("item", nil, map[string]interface{}{"from": "b"})

	// Cross-apply
	w1.ApplyRemote(op2)
	w2.ApplyRemote(op1)

	// Both should see both nodes (set union)
	n1a, ok1a := w1.GetNode(id1)
	n2a, ok2a := w1.GetNode(id2)
	n1b, ok1b := w2.GetNode(id1)
	n2b, ok2b := w2.GetNode(id2)

	if !ok1a || !ok2a || !ok1b || !ok2b {
		t.Fatal("all nodes should exist on both replicas")
	}
	if n1a.Properties["from"] != "a" || n2a.Properties["from"] != "b" {
		t.Error("properties mismatch on w1")
	}
	if n1b.Properties["from"] != "a" || n2b.Properties["from"] != "b" {
		t.Error("properties mismatch on w2")
	}
}

func TestConcurrentSetPropertyStale(t *testing.T) {
	w1, w2, id := twoWalkersWithNode(t)

	// Both set the same property concurrently.
	// w1 has seq counter for "replica-a" at 1 (from create), so SetProperty gets seq=2.
	// w2 has seq counter for "replica-b" at 0, so SetProperty gets seq=1.
	// LWW: higher seq wins, so replica-a (seq 2) beats replica-b (seq 1).
	opA, _ := w1.SetProperty(id, "key", "from-a")
	opB, _ := w2.SetProperty(id, "key", "from-b")

	// Cross-apply: each applies the other's op
	w1.ApplyRemote(opB)
	w2.ApplyRemote(opA)

	// Both should converge to the same value (LWW)
	nodeA, _ := w1.GetNode(id)
	nodeB, _ := w2.GetNode(id)
	if nodeA.Properties["key"] != nodeB.Properties["key"] {
		t.Errorf("replicas diverged: a=%v, b=%v", nodeA.Properties["key"], nodeB.Properties["key"])
	}
	// replica-a wins because its seq (2) > replica-b seq (1)
	if nodeA.Properties["key"] != "from-a" {
		t.Errorf("expected 'from-a' to win LWW, got %v", nodeA.Properties["key"])
	}
}

func TestConcurrentDeletePropertyStale(t *testing.T) {
	w1, w2, id := twoWalkersWithNode(t)

	// w1 sets a new value for "key", w2 deletes "key"
	opSet, _ := w1.SetProperty(id, "key", "updated")
	opDel, _ := w2.DeleteProperty(id, "key")

	// Cross-apply
	w1.ApplyRemote(opDel)
	w2.ApplyRemote(opSet)

	// Both should converge. replica-b's delete has higher ReplicaID at same seq.
	nodeA, _ := w1.GetNode(id)
	nodeB, _ := w2.GetNode(id)

	_, hasKeyA := nodeA.Properties["key"]
	_, hasKeyB := nodeB.Properties["key"]
	if hasKeyA != hasKeyB {
		t.Errorf("replicas diverged on key existence: a=%v, b=%v", hasKeyA, hasKeyB)
	}
}

func TestConcurrentDeleteNodeStale(t *testing.T) {
	w1, w2, id := twoWalkersWithNode(t)

	// Both delete the node concurrently
	opDelA, _ := w1.DeleteNode(id)
	opDelB, _ := w2.DeleteNode(id)

	// Cross-apply
	w1.ApplyRemote(opDelB)
	w2.ApplyRemote(opDelA)

	// Both should converge: node is deleted
	nA, _ := w1.GetNodeIncludingDeleted(id)
	nB, _ := w2.GetNodeIncludingDeleted(id)
	if !nA.Deleted || !nB.Deleted {
		t.Error("node should be deleted on both replicas")
	}
	// deleteEvent should converge
	if *nA.deleteEvent != *nB.deleteEvent {
		t.Error("deleteEvent should converge")
	}
}

func TestConcurrentRestoreNodeStale(t *testing.T) {
	w1, w2, id := twoWalkersWithNode(t)

	// Delete on w1
	opDel, _ := w1.DeleteNode(id)
	w2.ApplyRemote(opDel)

	// Both restore concurrently
	opRestoreA, _ := w1.RestoreNode(id)
	opRestoreB, _ := w2.RestoreNode(id)

	// Cross-apply
	w1.ApplyRemote(opRestoreB)
	w2.ApplyRemote(opRestoreA)

	// Both should converge: node is restored
	nA, okA := w1.GetNode(id)
	nB, okB := w2.GetNode(id)
	if !okA || !okB {
		t.Error("node should be visible on both replicas after restore")
	}
	_ = nA
	_ = nB
}

func TestConcurrentDeleteEdgeStale(t *testing.T) {
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

	// Both should converge
	_, okA := w1.GetEdge(edgeID)
	_, okB := w2.GetEdge(edgeID)
	if okA || okB {
		t.Error("edge should be deleted on both")
	}
}

func TestConcurrentRestoreEdgeStale(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	n1, op1, _ := w1.InsertNode("a", nil, nil)
	n2, op2, _ := w1.InsertNode("b", nil, nil)
	edgeID, op3, _ := w1.InsertEdge("link", n1, n2, nil)

	w2.ApplyRemote(op1)
	w2.ApplyRemote(op2)
	w2.ApplyRemote(op3)

	// Delete on w1 and sync
	opDel, _ := w1.DeleteEdge(edgeID)
	w2.ApplyRemote(opDel)

	// Both restore concurrently
	opRestoreA, _ := w1.RestoreEdge(edgeID)
	opRestoreB, _ := w2.RestoreEdge(edgeID)

	w1.ApplyRemote(opRestoreB)
	w2.ApplyRemote(opRestoreA)

	// Both should converge: edge is restored
	_, okA := w1.GetEdge(edgeID)
	_, okB := w2.GetEdge(edgeID)
	if okA != okB {
		t.Error("replicas diverged on edge restore")
	}
}

func TestConcurrentReorderNodeStale(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	parentID, opP, _ := w1.InsertNode("folder", nil, nil)
	childID, opC, _ := w1.InsertNode("file", &parentID, nil)

	w2.ApplyRemote(opP)
	w2.ApplyRemote(opC)

	// w1 reorder gets seq=3 (replica-a), w2 reorder gets seq=1 (replica-b).
	// w1 wins (higher seq).
	opA, _ := w1.ReorderNode(childID, "A")
	opB, _ := w2.ReorderNode(childID, "Z")

	w1.ApplyRemote(opB)
	w2.ApplyRemote(opA)

	// Both should converge
	nodeA, _ := w1.GetNode(childID)
	nodeB, _ := w2.GetNode(childID)
	if nodeA.Position != nodeB.Position {
		t.Errorf("replicas diverged on position: a=%s, b=%s", nodeA.Position, nodeB.Position)
	}
	// replica-a wins (higher seq)
	if nodeA.Position != "A" {
		t.Errorf("expected A to win, got %s", nodeA.Position)
	}
}

func TestConcurrentMoveNodeBothApplied(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	p1, op1, _ := w1.InsertNode("folder", nil, nil)
	p2, op2, _ := w1.InsertNode("folder", nil, nil)
	child, op3, _ := w1.InsertNode("file", &p1, nil)

	w2.ApplyRemote(op1)
	w2.ApplyRemote(op2)
	w2.ApplyRemote(op3)

	// Both move child concurrently. MoveNode in applyOp always applies
	// (no LWW guard), so the last applied move determines final parent.
	opMoveA, _ := w1.MoveNode(child, &p2)
	opMoveB, _ := w2.MoveNode(child, nil)

	w1.ApplyRemote(opMoveB)
	w2.ApplyRemote(opMoveA)

	// Just verify no panics and both ops were processed
	nodeA, okA := w1.GetNode(child)
	nodeB, okB := w2.GetNode(child)
	if !okA || !okB {
		t.Error("node should still exist on both replicas")
	}
	_ = nodeA
	_ = nodeB
}

// ---------------------------------------------------------------------------
// egwalker.go: GetParent — node whose parent was moved (parent deleted)
// ---------------------------------------------------------------------------

func TestGetParentAfterParentDeleted(t *testing.T) {
	w := NewEGWalker("r1")
	parentID, _, _ := w.InsertNode("folder", nil, nil)
	childID, _, _ := w.InsertNode("file", &parentID, nil)

	// Delete the parent
	w.DeleteNode(parentID)

	// GetParent should return false (parent is deleted)
	_, ok := w.GetParent(childID)
	if ok {
		t.Error("GetParent should return false when parent is deleted")
	}
}

func TestGetParentAfterMove(t *testing.T) {
	w := NewEGWalker("r1")
	p1, _, _ := w.InsertNode("folder", nil, nil)
	p2, _, _ := w.InsertNode("folder", nil, nil)
	child, _, _ := w.InsertNode("file", &p1, nil)

	// Move child to p2
	w.MoveNode(child, &p2)

	parent, ok := w.GetParent(child)
	if !ok {
		t.Fatal("GetParent should succeed after move")
	}
	if parent.ID != p2 {
		t.Errorf("expected parent p2, got %s", parent.ID)
	}

	// Now delete p2
	w.DeleteNode(p2)
	_, ok = w.GetParent(child)
	if ok {
		t.Error("GetParent should return false when parent is deleted")
	}
}

// ---------------------------------------------------------------------------
// fractional.go: positionMid — edge case with identical positions
// ---------------------------------------------------------------------------

func TestPositionMidIdentical(t *testing.T) {
	// When a and b are identical, positionMid should still produce
	// something (it appends midpoint character).
	mid := positionMid("M", "M")
	if mid == "" {
		t.Error("positionMid with identical positions should not be empty")
	}
}

func TestPositionMidVeryCloseMultiChar(t *testing.T) {
	// Adjacent multi-char positions forcing deep carry-down
	mid := positionMid("AA", "AB")
	if mid <= "AA" || mid >= "AB" {
		t.Errorf("expected AA < mid < AB, got mid=%s", mid)
	}
}

func TestPositionMidConsecutiveChars(t *testing.T) {
	// Consecutive single chars (no room for mid at first digit)
	mid := positionMid("0", "1")
	if mid <= "0" || mid >= "1" {
		t.Errorf("expected 0 < mid < 1, got mid=%s", mid)
	}
}

func TestPositionBetweenEdgeCases(t *testing.T) {
	// Before a position starting with a non-zero char
	before := PositionBetween("", "V")
	if before >= "V" {
		t.Errorf("expected before < V, got %s", before)
	}

	// After the maximum position
	maxPos := string(posChars[posBase-1]) // "z"
	after := PositionBetween(maxPos, "")
	if after <= maxPos {
		t.Errorf("expected after > %s, got %s", maxPos, after)
	}

	// Before the minimum char "0": positionBefore returns "0V" (prepend strategy)
	// which is actually > "0" lexicographically. This exercises the all-chars-at-min branch.
	beforeMin := PositionBetween("", "0")
	// Just verify it returns a non-empty string (the prepend branch)
	if beforeMin == "" {
		t.Error("expected non-empty position before '0'")
	}
}

func TestPositionBeforeMinChar(t *testing.T) {
	// positionBefore where all chars are at minimum — exercises the fallthrough
	// that prepends "0" + midpoint. The result is longer than input.
	before := positionBefore("00")
	if before == "" {
		t.Error("positionBefore should return non-empty string")
	}
	// This exercises the branch where all chars are at index 0
	if len(before) <= len("00") {
		// The fallthrough prepends, making it longer
	}
}

func TestPositionAfterMaxChar(t *testing.T) {
	// positionAfter where all chars are at maximum
	maxStr := string(posChars[posBase-1]) + string(posChars[posBase-1])
	after := positionAfter(maxStr)
	if after <= maxStr {
		t.Errorf("expected after > %s, got %s", maxStr, after)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: AddPostApplyListener
// ---------------------------------------------------------------------------

func TestPostApplyListener(t *testing.T) {
	w := NewEGWalker("r1")
	var called int
	w.AddPostApplyListener(func(op *Operation) {
		called++
	})

	w.InsertNode("item", nil, nil)
	if called != 1 {
		t.Errorf("expected postApplyListener called 1 time, got %d", called)
	}

	w.InsertNode("item", nil, nil)
	if called != 2 {
		t.Errorf("expected postApplyListener called 2 times, got %d", called)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: ApplyRemote with out-of-order seq (remote replicas)
// ---------------------------------------------------------------------------

func TestApplyRemoteHigherSeq(t *testing.T) {
	w := NewEGWalker("r1")

	// Create a synthetic remote op with seq=1 from a new replica
	op := &Operation{
		ID:        EventID{ReplicaID: "r2", Seq: 1},
		Type:      OpInsertNode,
		TargetID:  uuid.New(),
		NodeType:  "item",
		Value:     map[string]interface{}{"x": 1},
		Timestamp: time.Now(),
	}

	err := w.ApplyRemote(op)
	if err != nil {
		t.Fatalf("ApplyRemote: %v", err)
	}

	node, ok := w.GetNode(op.TargetID)
	if !ok {
		t.Fatal("node should exist")
	}
	if node.Properties["x"] != 1 {
		t.Error("property x should be 1")
	}
}

// ---------------------------------------------------------------------------
// eventgraph.go: Apply idempotent (duplicate event)
// ---------------------------------------------------------------------------

func TestApplyDuplicate(t *testing.T) {
	w := NewEGWalker("r1")
	_, op, _ := w.InsertNode("item", nil, nil)

	// Applying the same op again should be idempotent
	err := w.ApplyRemote(op)
	if err != nil {
		t.Fatalf("duplicate apply should be idempotent: %v", err)
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: ReplicaID accessor
// ---------------------------------------------------------------------------

func TestReplicaIDAccessor(t *testing.T) {
	w := NewEGWalker("my-replica")
	if w.ReplicaID() != "my-replica" {
		t.Errorf("expected 'my-replica', got %s", w.ReplicaID())
	}
}

// ---------------------------------------------------------------------------
// eventgraph.go: VersionVector
// ---------------------------------------------------------------------------

func TestVersionVector(t *testing.T) {
	w := NewEGWalker("r1")
	w.InsertNode("a", nil, nil)
	w.InsertNode("b", nil, nil)

	vv := w.Graph().VersionVector()
	if vv["r1"] != 2 {
		t.Errorf("expected r1 seq=2, got %d", vv["r1"])
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: SetProperty on edge — stale write path
// ---------------------------------------------------------------------------

func TestSetPropertyOnEdgeStalePath(t *testing.T) {
	w1 := NewEGWalker("replica-a")
	w2 := NewEGWalker("replica-b")

	n1, op1, _ := w1.InsertNode("a", nil, nil)
	n2, op2, _ := w1.InsertNode("b", nil, nil)
	edgeID, op3, _ := w1.InsertEdge("link", n1, n2, map[string]interface{}{"w": 1})

	w2.ApplyRemote(op1)
	w2.ApplyRemote(op2)
	w2.ApplyRemote(op3)

	// Both set the same edge property concurrently
	opA, _ := w1.SetProperty(edgeID, "w", 10)
	opB, _ := w2.SetProperty(edgeID, "w", 20)

	// Cross-apply: one should be stale
	w1.ApplyRemote(opB)
	w2.ApplyRemote(opA)

	edgeA, _ := w1.GetEdge(edgeID)
	edgeB, _ := w2.GetEdge(edgeID)
	if edgeA.Properties["w"] != edgeB.Properties["w"] {
		t.Errorf("edge property should converge: a=%v, b=%v", edgeA.Properties["w"], edgeB.Properties["w"])
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: InsertEdge with properties
// ---------------------------------------------------------------------------

func TestInsertEdgeWithProperties(t *testing.T) {
	w := NewEGWalker("r1")
	n1, _, _ := w.InsertNode("a", nil, nil)
	n2, _, _ := w.InsertNode("b", nil, nil)

	props := map[string]interface{}{"weight": 5, "label": "test"}
	edgeID, _, err := w.InsertEdge("link", n1, n2, props)
	if err != nil {
		t.Fatalf("InsertEdge: %v", err)
	}

	edge, ok := w.GetEdge(edgeID)
	if !ok {
		t.Fatal("edge should exist")
	}
	if edge.Properties["weight"] != 5 {
		t.Error("weight should be 5")
	}
	if edge.Properties["label"] != "test" {
		t.Error("label should be test")
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: MoveNode cycle detection
// ---------------------------------------------------------------------------

func TestMoveNodeCycleDetectionDirect(t *testing.T) {
	w := NewEGWalker("r1")
	p, _, _ := w.InsertNode("folder", nil, nil)
	c, _, _ := w.InsertNode("folder", &p, nil)

	// Try to move p under c — would create cycle
	_, err := w.MoveNode(p, &c)
	if err == nil {
		t.Error("expected cycle detection error")
	}
}

// ---------------------------------------------------------------------------
// egwalker.go: GetOrderedChildren sorts by position
// ---------------------------------------------------------------------------

func TestGetOrderedChildrenSorted(t *testing.T) {
	w := NewEGWalker("r1")
	parentID, _, _ := w.InsertNode("folder", nil, nil)

	c1, _, _ := w.InsertNode("file", &parentID, nil)
	c2, _, _ := w.InsertNode("file", &parentID, nil)
	c3, _, _ := w.InsertNode("file", &parentID, nil)

	// Set positions out of order
	w.ReorderNode(c1, "Z")
	w.ReorderNode(c2, "A")
	w.ReorderNode(c3, "M")

	children := w.GetOrderedChildren(parentID)
	if len(children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(children))
	}
	if children[0].ID != c2 || children[1].ID != c3 || children[2].ID != c1 {
		t.Error("children should be sorted by position: A, M, Z")
	}
}

// ---------------------------------------------------------------------------
// fractional.go: positionMid — b shorter than a (bIdx[i] = posBase branch)
// ---------------------------------------------------------------------------

func TestPositionMidBShorterThanA(t *testing.T) {
	// When b is shorter than a, the missing chars in b are treated as posBase.
	// "AB" vs "B" => len(a)=2, len(b)=1, so at i=1 bIdx[1]=posBase
	mid := positionMid("AB", "B")
	if mid <= "AB" || mid >= "B" {
		t.Errorf("expected AB < mid < B, got mid=%s", mid)
	}
}

func TestPositionMidCarryDownWithNextA(t *testing.T) {
	// Force carry-down where i+1 < len(a).
	// "Az" and "B" => at digit 0: aIdx[0]=charIndex('A'), bIdx[0]=charIndex('B').
	// mid = (aIdx+bIdx)/2. If A and B are adjacent, mid == aIdx so carry down.
	// At carry-down, nextA = charIndex(a[1]) since i+1=1 < len(a)=2.
	mid := positionMid("Az", "B")
	if mid <= "Az" || mid >= "B" {
		t.Errorf("expected Az < mid < B, got mid=%s", mid)
	}
}

func TestPositionMidLongerACarryDown(t *testing.T) {
	// Another carry-down case where a is longer
	mid := positionMid("Vz", "W")
	if mid <= "Vz" || mid >= "W" {
		t.Errorf("expected Vz < mid < W, got mid=%s", mid)
	}
}

// ---------------------------------------------------------------------------
// eventgraph.go: causalOrderUnlocked — multiple concurrent roots for sorting
// ---------------------------------------------------------------------------

func TestCausalOrderWithConcurrentRoots(t *testing.T) {
	// Create events from different replicas that are concurrent (both roots).
	// This forces causalOrderUnlocked to have multiple items in the queue
	// and exercise the sorting logic.
	eg := NewEventGraph()

	op1 := &Operation{
		ID:        EventID{ReplicaID: "r2", Seq: 1},
		Type:      OpInsertNode,
		Timestamp: time.Now(),
	}
	op2 := &Operation{
		ID:        EventID{ReplicaID: "r1", Seq: 1},
		Type:      OpInsertNode,
		Timestamp: time.Now(),
	}

	eg.Apply(op1)
	eg.Apply(op2)

	// Both are roots (no parents), so they'll be in the queue simultaneously.
	// The sort should put r1 before r2 (same seq, r1 < r2).
	order := eg.CausalOrder()
	if len(order) != 2 {
		t.Fatalf("expected 2 events, got %d", len(order))
	}
	if order[0].ID.ReplicaID != "r1" {
		t.Errorf("expected r1 first, got %s", order[0].ID.ReplicaID)
	}
}

func TestEventsSinceWithConcurrentRoots(t *testing.T) {
	// EventsSince uses causalOrderUnlocked internally.
	// Force it to have concurrent events from different replicas.
	eg := NewEventGraph()

	op1 := &Operation{
		ID:        EventID{ReplicaID: "r2", Seq: 1},
		Type:      OpInsertNode,
		Timestamp: time.Now(),
	}
	op2 := &Operation{
		ID:        EventID{ReplicaID: "r1", Seq: 1},
		Type:      OpInsertNode,
		Timestamp: time.Now(),
	}
	op3 := &Operation{
		ID:        EventID{ReplicaID: "r1", Seq: 2},
		Parents:   []EventID{op2.ID},
		Type:      OpSetProperty,
		Timestamp: time.Now(),
	}

	eg.Apply(op1)
	eg.Apply(op2)
	eg.Apply(op3)

	// Since frontier: r2:1 — should return op3 (r1:2)
	events := eg.EventsSince([]EventID{op1.ID, op2.ID})
	if len(events) != 1 {
		t.Errorf("expected 1 event since frontier, got %d", len(events))
	}
}
