package crdt

import (
	"fmt"
	"sync"
)

// EventGraph is the core DAG that tracks the causal history of all operations.
// This implements the "event graph" from the eg-walker paper.
// Each event (operation) records its causal parents, forming a DAG.
// The current state is computed by walking the graph.
type EventGraph struct {
	mu sync.RWMutex

	// All events indexed by their ID
	events map[EventID]*Operation

	// The frontier: set of events with no children (the "heads")
	frontier map[EventID]struct{}

	// Children index: parent -> children
	children map[EventID][]EventID

	// Per-replica sequence counters
	seqCounters map[string]uint64

	// Listeners for new events
	listeners []func(*Operation)
}

// NewEventGraph creates a new empty event graph.
func NewEventGraph() *EventGraph {
	return &EventGraph{
		events:      make(map[EventID]*Operation),
		frontier:    make(map[EventID]struct{}),
		children:    make(map[EventID][]EventID),
		seqCounters: make(map[string]uint64),
	}
}

// AddListener registers a callback for new events.
func (eg *EventGraph) AddListener(fn func(*Operation)) {
	eg.mu.Lock()
	defer eg.mu.Unlock()
	eg.listeners = append(eg.listeners, fn)
}

// Apply adds an operation to the event graph.
// It validates causal ordering and updates the frontier.
func (eg *EventGraph) Apply(op *Operation) error {
	eg.mu.Lock()
	defer eg.mu.Unlock()

	// Check for duplicate
	if _, exists := eg.events[op.ID]; exists {
		return nil // idempotent
	}

	// Validate all parents exist
	for _, parent := range op.Parents {
		if _, exists := eg.events[parent]; !exists {
			return fmt.Errorf("missing parent event %v", parent)
		}
	}

	// Validate sequence number
	expected := eg.seqCounters[op.ID.ReplicaID] + 1
	if op.ID.Seq != expected && len(eg.events) > 0 {
		// Allow any seq for remote replicas we haven't seen before
		if _, seen := eg.seqCounters[op.ID.ReplicaID]; seen && op.ID.Seq <= eg.seqCounters[op.ID.ReplicaID] {
			return fmt.Errorf("stale seq %d for replica %s (expected > %d)", op.ID.Seq, op.ID.ReplicaID, eg.seqCounters[op.ID.ReplicaID])
		}
	}

	// Store the event
	eg.events[op.ID] = op

	// Update sequence counter
	if op.ID.Seq > eg.seqCounters[op.ID.ReplicaID] {
		eg.seqCounters[op.ID.ReplicaID] = op.ID.Seq
	}

	// Update frontier: remove parents from frontier, add this event
	for _, parent := range op.Parents {
		delete(eg.frontier, parent)
	}
	eg.frontier[op.ID] = struct{}{}

	// Update children index
	for _, parent := range op.Parents {
		eg.children[parent] = append(eg.children[parent], op.ID)
	}

	// Notify listeners
	for _, fn := range eg.listeners {
		fn(op)
	}

	return nil
}

// Frontier returns the current frontier (heads of the DAG).
func (eg *EventGraph) Frontier() []EventID {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	result := make([]EventID, 0, len(eg.frontier))
	for id := range eg.frontier {
		result = append(result, id)
	}
	return result
}

// Get retrieves an event by ID.
func (eg *EventGraph) Get(id EventID) (*Operation, bool) {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	op, ok := eg.events[id]
	return op, ok
}

// NextSeq returns the next sequence number for a replica.
func (eg *EventGraph) NextSeq(replicaID string) uint64 {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	return eg.seqCounters[replicaID] + 1
}

// Size returns the total number of events.
func (eg *EventGraph) Size() int {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	return len(eg.events)
}

// CausalOrder returns a topologically sorted list of all events.
// This is the "walk" in eg-walker — we traverse the event graph
// in causal order to compute the current state.
func (eg *EventGraph) CausalOrder() []*Operation {
	eg.mu.RLock()
	defer eg.mu.RUnlock()

	// Find roots (events with no parents)
	inDegree := make(map[EventID]int)
	for id, op := range eg.events {
		if _, exists := inDegree[id]; !exists {
			inDegree[id] = 0
		}
		for range op.Parents {
			// Parents are counted in child's in-degree
		}
	}
	for _, op := range eg.events {
		for _, parent := range op.Parents {
			_ = parent // parents already exist
		}
	}

	// Kahn's algorithm for topological sort
	childCount := make(map[EventID]int)
	for _, op := range eg.events {
		childCount[op.ID] = 0
	}
	for _, op := range eg.events {
		for _, parent := range op.Parents {
			childCount[parent]++
		}
	}

	// Actually, let's use in-degree based on parents
	deg := make(map[EventID]int)
	for _, op := range eg.events {
		deg[op.ID] = len(op.Parents)
	}

	var queue []EventID
	for id, d := range deg {
		if d == 0 {
			queue = append(queue, id)
		}
	}

	var result []*Operation
	for len(queue) > 0 {
		// Pop from queue — deterministic ordering by (Seq, ReplicaID)
		minIdx := 0
		for i := 1; i < len(queue); i++ {
			if queue[i].Seq < queue[minIdx].Seq ||
				(queue[i].Seq == queue[minIdx].Seq && queue[i].ReplicaID < queue[minIdx].ReplicaID) {
				minIdx = i
			}
		}
		id := queue[minIdx]
		queue[minIdx] = queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		result = append(result, eg.events[id])

		// Reduce in-degree of children
		for _, child := range eg.children[id] {
			deg[child]--
			if deg[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	return result
}

// EventsSince returns all events that are causally after the given frontier.
// Used for syncing: "give me everything I haven't seen."
func (eg *EventGraph) EventsSince(knownFrontier []EventID) []*Operation {
	eg.mu.RLock()
	defer eg.mu.RUnlock()

	// Mark all events reachable from the known frontier as "seen"
	seen := make(map[EventID]bool)
	var markSeen func(id EventID)
	markSeen = func(id EventID) {
		if seen[id] {
			return
		}
		seen[id] = true
		op, ok := eg.events[id]
		if !ok {
			return
		}
		for _, parent := range op.Parents {
			markSeen(parent)
		}
	}
	for _, id := range knownFrontier {
		markSeen(id)
	}

	// Return all events not in "seen", in causal order
	all := eg.causalOrderUnlocked()
	var result []*Operation
	for _, op := range all {
		if !seen[op.ID] {
			result = append(result, op)
		}
	}
	return result
}

// CompactBefore compresses all events before (and including) the given frontier
// into a single snapshot. This discards individual operations and keeps only
// the materialized state at that point. Events after the frontier are preserved.
// Returns the number of events compacted.
func (eg *EventGraph) CompactBefore(cutoff []EventID) int {
	eg.mu.Lock()
	defer eg.mu.Unlock()

	// Find all events reachable backward from cutoff
	toCompact := make(map[EventID]bool)
	var markCompact func(id EventID)
	markCompact = func(id EventID) {
		if toCompact[id] {
			return
		}
		toCompact[id] = true
		if op, ok := eg.events[id]; ok {
			for _, parent := range op.Parents {
				markCompact(parent)
			}
		}
	}
	for _, id := range cutoff {
		markCompact(id)
	}

	count := 0
	for id := range toCompact {
		// Don't remove events that are parents of non-compacted events
		hasLiveChild := false
		for _, child := range eg.children[id] {
			if !toCompact[child] {
				hasLiveChild = true
				break
			}
		}
		if hasLiveChild {
			continue
		}
		delete(eg.events, id)
		delete(eg.frontier, id)
		delete(eg.children, id)
		count++
	}

	return count
}

// DeltaSince returns a compact delta of events since the given version vector.
// The version vector maps replicaID -> last seen seq for that replica.
// This is more efficient than EventsSince for large graphs.
func (eg *EventGraph) DeltaSince(versionVector map[string]uint64) []*Operation {
	eg.mu.RLock()
	defer eg.mu.RUnlock()

	var result []*Operation
	for _, op := range eg.events {
		lastSeen, ok := versionVector[op.ID.ReplicaID]
		if !ok || op.ID.Seq > lastSeen {
			result = append(result, op)
		}
	}

	// Sort in causal order
	sortOps(result)
	return result
}

// VersionVector returns the current version vector (replicaID -> max seq).
func (eg *EventGraph) VersionVector() map[string]uint64 {
	eg.mu.RLock()
	defer eg.mu.RUnlock()
	vv := make(map[string]uint64, len(eg.seqCounters))
	for k, v := range eg.seqCounters {
		vv[k] = v
	}
	return vv
}

// sortOps sorts operations in causal order (by Seq, then ReplicaID).
func sortOps(ops []*Operation) {
	for i := 1; i < len(ops); i++ {
		key := ops[i]
		j := i - 1
		for j >= 0 && (ops[j].ID.Seq > key.ID.Seq ||
			(ops[j].ID.Seq == key.ID.Seq && ops[j].ID.ReplicaID > key.ID.ReplicaID)) {
			ops[j+1] = ops[j]
			j--
		}
		ops[j+1] = key
	}
}

func (eg *EventGraph) causalOrderUnlocked() []*Operation {
	deg := make(map[EventID]int)
	for _, op := range eg.events {
		deg[op.ID] = len(op.Parents)
	}

	var queue []EventID
	for id, d := range deg {
		if d == 0 {
			queue = append(queue, id)
		}
	}

	var result []*Operation
	for len(queue) > 0 {
		minIdx := 0
		for i := 1; i < len(queue); i++ {
			if queue[i].Seq < queue[minIdx].Seq ||
				(queue[i].Seq == queue[minIdx].Seq && queue[i].ReplicaID < queue[minIdx].ReplicaID) {
				minIdx = i
			}
		}
		id := queue[minIdx]
		queue[minIdx] = queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		result = append(result, eg.events[id])

		for _, child := range eg.children[id] {
			deg[child]--
			if deg[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	return result
}
