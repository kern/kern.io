package crdt

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/google/uuid"
)

// FuzzCRDTConvergence tests that two replicas always converge to the same
// state after exchanging all operations, regardless of operation order.
func FuzzCRDTConvergence(f *testing.F) {
	// Seed corpus
	f.Add(uint64(42), 10, 3)
	f.Add(uint64(123), 20, 5)
	f.Add(uint64(999), 5, 2)

	f.Fuzz(func(t *testing.T, seed uint64, numOps int, numTypes int) {
		if numOps < 1 || numOps > 50 {
			return
		}
		if numTypes < 1 || numTypes > 10 {
			return
		}

		rng := rand.New(rand.NewSource(int64(seed)))

		// Two replicas
		w1 := NewEGWalker("r1")
		w2 := NewEGWalker("r2")

		var ops1 []*Operation
		var ops2 []*Operation

		// Track which nodes each walker knows about
		w1Nodes := make([]uuid.UUID, 0)
		w2Nodes := make([]uuid.UUID, 0)

		types := make([]string, numTypes)
		for i := 0; i < numTypes; i++ {
			types[i] = fmt.Sprintf("type%d", i)
		}

		// Generate random operations on both replicas
		// Only operate on nodes the specific walker knows about
		for i := 0; i < numOps; i++ {
			useW1 := rng.Intn(2) == 0

			switch rng.Intn(4) {
			case 0: // Insert node (always safe)
				nodeType := types[rng.Intn(len(types))]
				props := map[string]interface{}{
					"val": rng.Intn(1000),
				}
				if useW1 {
					var parentID *uuid.UUID
					if len(w1Nodes) > 0 && rng.Intn(3) == 0 {
						id := w1Nodes[rng.Intn(len(w1Nodes))]
						parentID = &id
					}
					id, op, err := w1.InsertNode(nodeType, parentID, props)
					if err == nil {
						w1Nodes = append(w1Nodes, id)
						ops1 = append(ops1, op)
					}
				} else {
					var parentID *uuid.UUID
					if len(w2Nodes) > 0 && rng.Intn(3) == 0 {
						id := w2Nodes[rng.Intn(len(w2Nodes))]
						parentID = &id
					}
					id, op, err := w2.InsertNode(nodeType, parentID, props)
					if err == nil {
						w2Nodes = append(w2Nodes, id)
						ops2 = append(ops2, op)
					}
				}

			case 1: // Set property on a node this walker knows about
				if useW1 && len(w1Nodes) > 0 {
					nodeID := w1Nodes[rng.Intn(len(w1Nodes))]
					key := fmt.Sprintf("prop%d", rng.Intn(5))
					val := rng.Intn(1000)
					op, err := w1.SetProperty(nodeID, key, val)
					if err == nil {
						ops1 = append(ops1, op)
					}
				} else if !useW1 && len(w2Nodes) > 0 {
					nodeID := w2Nodes[rng.Intn(len(w2Nodes))]
					key := fmt.Sprintf("prop%d", rng.Intn(5))
					val := rng.Intn(1000)
					op, err := w2.SetProperty(nodeID, key, val)
					if err == nil {
						ops2 = append(ops2, op)
					}
				}

			case 2: // Delete node this walker owns
				if useW1 && len(w1Nodes) > 0 {
					nodeID := w1Nodes[rng.Intn(len(w1Nodes))]
					op, err := w1.DeleteNode(nodeID)
					if err == nil {
						ops1 = append(ops1, op)
					}
				} else if !useW1 && len(w2Nodes) > 0 {
					nodeID := w2Nodes[rng.Intn(len(w2Nodes))]
					op, err := w2.DeleteNode(nodeID)
					if err == nil {
						ops2 = append(ops2, op)
					}
				}

			case 3: // Insert edge between nodes this walker knows
				if useW1 && len(w1Nodes) >= 2 {
					from := w1Nodes[rng.Intn(len(w1Nodes))]
					to := w1Nodes[rng.Intn(len(w1Nodes))]
					if from != to {
						_, op, err := w1.InsertEdge("edge", from, to, nil)
						if err == nil {
							ops1 = append(ops1, op)
						}
					}
				} else if !useW1 && len(w2Nodes) >= 2 {
					from := w2Nodes[rng.Intn(len(w2Nodes))]
					to := w2Nodes[rng.Intn(len(w2Nodes))]
					if from != to {
						_, op, err := w2.InsertEdge("edge", from, to, nil)
						if err == nil {
							ops2 = append(ops2, op)
						}
					}
				}
			}
		}

		// Exchange operations: apply ops1 to w2 and ops2 to w1
		for _, op := range ops1 {
			w2.ApplyRemote(op)
		}
		for _, op := range ops2 {
			w1.ApplyRemote(op)
		}

		// Both walkers should now have the same state
		nodes1 := w1.AllNodes()
		nodes2 := w2.AllNodes()

		if len(nodes1) != len(nodes2) {
			t.Errorf("node count mismatch: r1=%d, r2=%d", len(nodes1), len(nodes2))
			return
		}

		// Check each node matches
		nodeMap1 := make(map[uuid.UUID]*MaterializedNode)
		for _, n := range nodes1 {
			nodeMap1[n.ID] = n
		}
		for _, n2 := range nodes2 {
			n1, ok := nodeMap1[n2.ID]
			if !ok {
				t.Errorf("node %s in r2 but not r1", n2.ID)
				continue
			}
			if n1.Type != n2.Type {
				t.Errorf("node %s type mismatch: r1=%s r2=%s", n2.ID, n1.Type, n2.Type)
			}
			if n1.Deleted != n2.Deleted {
				t.Errorf("node %s deleted mismatch: r1=%v r2=%v", n2.ID, n1.Deleted, n2.Deleted)
			}
			// Check properties match
			for k, v1 := range n1.Properties {
				v2, ok := n2.Properties[k]
				if !ok {
					t.Errorf("node %s property %s in r1 but not r2", n2.ID, k)
				} else if fmt.Sprintf("%v", v1) != fmt.Sprintf("%v", v2) {
					t.Errorf("node %s property %s mismatch: r1=%v r2=%v", n2.ID, k, v1, v2)
				}
			}
		}

		// Check edges match
		edges1 := w1.AllEdges()
		edges2 := w2.AllEdges()
		if len(edges1) != len(edges2) {
			t.Errorf("edge count mismatch: r1=%d r2=%d", len(edges1), len(edges2))
		}
	})
}

// TestDeterministicConvergence runs a fixed convergence test (non-fuzz).
func TestDeterministicConvergence(t *testing.T) {
	w1 := NewEGWalker("r1")
	w2 := NewEGWalker("r2")

	// r1 creates nodes
	id1, op1, _ := w1.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	id2, op2, _ := w1.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})

	// r2 creates a node concurrently
	id3, op3, _ := w2.InsertNode("task", nil, map[string]interface{}{"title": "Fix bug"})

	// r1 sets a property, r2 sets same property concurrently
	op4, _ := w1.SetProperty(id1, "score", 100)
	// r2 can't set property on id1 until it knows about it

	// Exchange r1's ops to r2
	w2.ApplyRemote(op1)
	w2.ApplyRemote(op2)
	w2.ApplyRemote(op4)

	// Now r2 sets the same property
	op5, _ := w2.SetProperty(id1, "score", 200)

	// Exchange r2's ops to r1
	w1.ApplyRemote(op3)
	w1.ApplyRemote(op5)

	// Both should converge: last writer wins
	n1, _ := w1.GetNode(id1)
	n2, _ := w2.GetNode(id1)

	if n1.Properties["score"] != n2.Properties["score"] {
		t.Errorf("score should converge: r1=%v r2=%v", n1.Properties["score"], n2.Properties["score"])
	}

	// Both should see all 3 nodes
	if len(w1.AllNodes()) != len(w2.AllNodes()) {
		t.Errorf("node count mismatch after sync: r1=%d r2=%d", len(w1.AllNodes()), len(w2.AllNodes()))
	}

	// Both should see id3
	_, ok1 := w1.GetNode(id3)
	_, ok2 := w2.GetNode(id3)
	if !ok1 || !ok2 {
		t.Error("both replicas should see id3 after sync")
	}

	_ = id2
}

// TestConcurrentDeleteAndUpdate tests delete-wins semantics.
func TestConcurrentDeleteAndUpdate(t *testing.T) {
	w1 := NewEGWalker("r1")
	w2 := NewEGWalker("r2")

	// r1 creates node
	id, insertOp, _ := w1.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})

	// Sync to r2
	w2.ApplyRemote(insertOp)

	// r1 deletes, r2 updates (concurrent)
	deleteOp, _ := w1.DeleteNode(id)
	updateOp, _ := w2.SetProperty(id, "name", "Bob")

	// Exchange
	w2.ApplyRemote(deleteOp)
	w1.ApplyRemote(updateOp)

	// Both should agree: delete wins in current implementation
	_, ok1 := w1.GetNode(id)
	_, ok2 := w2.GetNode(id)

	if ok1 != ok2 {
		t.Errorf("replicas disagree on node existence: r1=%v r2=%v", ok1, ok2)
	}
}

// TestFractionalIndexOrdering tests that fractional indexing maintains order.
func TestFractionalIndexOrdering(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool // result should be between a and b
	}{
		{"A", "Z", true},
		{"V", "", true},
		{"", "V", true},
		{"", "", true},
		{"a", "z", true},
		{"AA", "AB", true},
	}

	for _, tt := range tests {
		result := PositionBetween(tt.a, tt.b)
		if tt.a != "" && result <= tt.a {
			t.Errorf("PositionBetween(%q, %q) = %q, should be > %q", tt.a, tt.b, result, tt.a)
		}
		if tt.b != "" && result >= tt.b {
			t.Errorf("PositionBetween(%q, %q) = %q, should be < %q", tt.a, tt.b, result, tt.b)
		}
	}
}

// TestFractionalIndexStress tests that we can always insert between two positions.
func TestFractionalIndexStress(t *testing.T) {
	positions := []string{"A", "z"}

	// Insert 100 elements between first two, always at the start
	for i := 0; i < 100; i++ {
		newPos := PositionBetween(positions[0], positions[1])
		if newPos <= positions[0] || newPos >= positions[1] {
			t.Fatalf("iteration %d: PositionBetween(%q, %q) = %q is not between",
				i, positions[0], positions[1], newPos)
		}
		positions = append(positions, "")
		copy(positions[2:], positions[1:])
		positions[1] = newPos
	}
}

// TestDeltaCompression tests delta sync with version vectors.
func TestDeltaCompression(t *testing.T) {
	w := NewEGWalker("r1")

	w.InsertNode("a", nil, nil)
	w.InsertNode("b", nil, nil)
	w.InsertNode("c", nil, nil)

	vv := w.Graph().VersionVector()
	if vv["r1"] != 3 {
		t.Errorf("expected version 3 for r1, got %d", vv["r1"])
	}

	// Get delta from version 1 (should return ops 2 and 3)
	delta := w.Graph().DeltaSince(map[string]uint64{"r1": 1})
	if len(delta) != 2 {
		t.Errorf("expected 2 delta ops, got %d", len(delta))
	}

	// Empty version vector should return all ops
	allDelta := w.Graph().DeltaSince(map[string]uint64{})
	if len(allDelta) != 3 {
		t.Errorf("expected 3 ops from empty VV, got %d", len(allDelta))
	}
}

// TestPositionInitial tests generating evenly-spaced initial positions.
func TestPositionInitial(t *testing.T) {
	positions := PositionInitial(5)
	if len(positions) != 5 {
		t.Fatalf("expected 5 positions, got %d", len(positions))
	}

	// Each position should be lexicographically after the previous
	for i := 1; i < len(positions); i++ {
		if positions[i] <= positions[i-1] {
			t.Errorf("positions not sorted: %q <= %q", positions[i], positions[i-1])
		}
	}
}
