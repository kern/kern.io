package cluster

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGetNodes(t *testing.T) {
	ring := NewRing(1)

	// Empty ring
	nodes := ring.GetNodes()
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}

	ring.AddNode(&ClusterNode{ID: "n1", Address: "localhost:8001"})
	ring.AddNode(&ClusterNode{ID: "n2", Address: "localhost:8002"})
	ring.AddNode(&ClusterNode{ID: "n3", Address: "localhost:8003"})

	nodes = ring.GetNodes()
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodes))
	}

	found := map[string]bool{}
	for _, n := range nodes {
		found[n.ID] = true
	}
	if !found["n1"] || !found["n2"] || !found["n3"] {
		t.Error("expected all three nodes")
	}
}

func TestRemoveNode(t *testing.T) {
	ring := NewRing(1)

	ring.AddNode(&ClusterNode{ID: "n1", Address: "localhost:8001"})
	ring.AddNode(&ClusterNode{ID: "n2", Address: "localhost:8002"})

	if ring.NodeCount() != 2 {
		t.Errorf("expected 2 nodes, got %d", ring.NodeCount())
	}

	ring.RemoveNode("n1")

	if ring.NodeCount() != 1 {
		t.Errorf("expected 1 node after removal, got %d", ring.NodeCount())
	}

	nodes := ring.GetNodes()
	if len(nodes) != 1 || nodes[0].ID != "n2" {
		t.Error("only n2 should remain")
	}

	// All keys should route to n2 now
	for i := 0; i < 50; i++ {
		node := ring.GetNode(fmt.Sprintf("key-%d", i))
		if node != "n2" {
			t.Errorf("expected all keys on n2, got %s", node)
		}
	}
}

func TestRemoveNodeNonexistent(t *testing.T) {
	ring := NewRing(1)
	ring.AddNode(&ClusterNode{ID: "n1", Address: "localhost:8001"})

	// Should not panic
	ring.RemoveNode("nonexistent")
	if ring.NodeCount() != 1 {
		t.Error("node count should still be 1")
	}
}

func TestNewRingMinReplicaCount(t *testing.T) {
	// replicaCount < 1 should be clamped to 1
	ring := NewRing(0)
	if ring.replicaCount != 1 {
		t.Errorf("expected replicaCount=1 when 0 passed, got %d", ring.replicaCount)
	}

	ring2 := NewRing(-5)
	if ring2.replicaCount != 1 {
		t.Errorf("expected replicaCount=1 when negative, got %d", ring2.replicaCount)
	}

	ring3 := NewRing(3)
	if ring3.replicaCount != 3 {
		t.Errorf("expected replicaCount=3, got %d", ring3.replicaCount)
	}
}

func TestGetNodeEmptyRing(t *testing.T) {
	ring := NewRing(1)
	node := ring.GetNode("test-key")
	if node != "" {
		t.Errorf("expected empty string for empty ring, got %s", node)
	}
}

func TestGetReplicaNodesEmptyRing(t *testing.T) {
	ring := NewRing(2)
	nodes := ring.GetReplicaNodes("test-key")
	if nodes != nil {
		t.Errorf("expected nil for empty ring, got %v", nodes)
	}
}

func TestGetReplicaNodesFewerNodesThanReplicas(t *testing.T) {
	ring := NewRing(5) // Want 6 total (1 primary + 5 replicas) but only have 2 nodes

	ring.AddNode(&ClusterNode{ID: "n1", Address: "localhost:8001"})
	ring.AddNode(&ClusterNode{ID: "n2", Address: "localhost:8002"})

	nodes := ring.GetReplicaNodes("test-key")
	if len(nodes) != 2 {
		t.Errorf("should return all available nodes (2), got %d", len(nodes))
	}
}

func TestIsLocalShardOwnerAndReplica(t *testing.T) {
	// Use 64 shards and 3 nodes with replicaCount=0 to ensure some are remote
	sm := NewShardManager("node1", 64, 0)
	sm.Init()

	sm.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001", Status: NodeHealthy})
	sm.AddNode(&ClusterNode{ID: "node2", Address: "localhost:8002", Status: NodeHealthy})
	sm.AddNode(&ClusterNode{ID: "node3", Address: "localhost:8003", Status: NodeHealthy})

	// Test many UUIDs - some should be local, some not
	localCount := 0
	remoteCount := 0
	for i := 0; i < 200; i++ {
		id := uuid.New()
		if sm.IsLocalShard(id) {
			localCount++
		} else {
			remoteCount++
		}
	}

	if localCount == 0 {
		t.Error("expected some UUIDs to be local")
	}
	if remoteCount == 0 {
		t.Error("expected some UUIDs to be remote")
	}
}

func TestIsLocalShardAsReplica(t *testing.T) {
	sm := NewShardManager("node2", 4, 1)
	sm.Init()

	sm.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001", Status: NodeHealthy})
	sm.AddNode(&ClusterNode{ID: "node2", Address: "localhost:8002", Status: NodeHealthy})

	// With replicaCount=1, node2 should have shards either as owner or replica
	localShards := sm.GetLocalShards()
	if len(localShards) == 0 {
		t.Error("node2 should have some local shards (owned or replicated)")
	}
}

func TestIsLocalShardNoMatch(t *testing.T) {
	// Create a shard manager with no nodes
	sm := NewShardManager("node1", 4, 1)
	sm.Init()

	// Don't add node1 to the ring, so no shard has node1 as owner
	sm.ring.AddNode(&ClusterNode{ID: "node999", Address: "localhost:9999", Status: NodeHealthy})
	sm.AssignShards()

	// node1 shouldn't own any shards
	id := uuid.New()
	if sm.IsLocalShard(id) {
		t.Error("node1 should not own any shards")
	}
}

func TestShardManagerRemoveNode(t *testing.T) {
	sm := NewShardManager("node1", 16, 1)
	sm.Init()

	sm.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001", Status: NodeHealthy})
	sm.AddNode(&ClusterNode{ID: "node2", Address: "localhost:8002", Status: NodeHealthy})

	stats := sm.GetShardStats()
	if stats["nodeCount"].(int) != 2 {
		t.Errorf("expected 2 nodes, got %v", stats["nodeCount"])
	}

	sm.RemoveNode("node2")

	stats = sm.GetShardStats()
	if stats["nodeCount"].(int) != 1 {
		t.Errorf("expected 1 node after removal, got %v", stats["nodeCount"])
	}

	// All shards should be on node1 now
	dist := stats["distribution"].(map[NodeID]int)
	if dist["node1"] != 16 {
		t.Errorf("expected all 16 shards on node1, got %d", dist["node1"])
	}
}

func TestShardManagerRouteNodeConsistency(t *testing.T) {
	sm := NewShardManager("node1", 8, 1)
	sm.Init()

	sm.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001", Status: NodeHealthy})
	sm.AddNode(&ClusterNode{ID: "node2", Address: "localhost:8002", Status: NodeHealthy})

	id := uuid.New()
	node1 := sm.RouteNode(id)
	node2 := sm.RouteNode(id)

	if node1 != node2 {
		t.Error("same UUID should consistently route to the same node")
	}
}

func TestShardManagerGetLocalShardsWithReplicas(t *testing.T) {
	sm := NewShardManager("node1", 8, 2) // 2 replicas
	sm.Init()

	sm.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001", Status: NodeHealthy})
	sm.AddNode(&ClusterNode{ID: "node2", Address: "localhost:8002", Status: NodeHealthy})
	sm.AddNode(&ClusterNode{ID: "node3", Address: "localhost:8003", Status: NodeHealthy})

	localShards := sm.GetLocalShards()
	// With 3 nodes and replicaCount=2 (so 3 copies total), node1 should have all shards
	if len(localShards) != 8 {
		t.Errorf("with 3 nodes and replicaCount=2, node1 should have all 8 shards (owned + replicated), got %d", len(localShards))
	}
}

func TestRingAddNodeSorted(t *testing.T) {
	ring := NewRing(1)

	ring.AddNode(&ClusterNode{ID: "n1", Address: "localhost:8001"})
	ring.AddNode(&ClusterNode{ID: "n2", Address: "localhost:8002"})

	// Verify ring is sorted
	ring.mu.RLock()
	for i := 1; i < len(ring.ring); i++ {
		if ring.ring[i].hash < ring.ring[i-1].hash {
			t.Error("ring should be sorted by hash")
		}
	}
	ring.mu.RUnlock()
}

func TestHashKeyDeterministic(t *testing.T) {
	h1 := hashKey("test-key")
	h2 := hashKey("test-key")
	if h1 != h2 {
		t.Error("hashKey should be deterministic")
	}

	h3 := hashKey("different-key")
	if h1 == h3 {
		t.Error("different keys should (likely) produce different hashes")
	}
}

func TestNodeStatusConstants(t *testing.T) {
	// Verify status constants exist
	if NodeHealthy != 0 {
		t.Error("NodeHealthy should be 0")
	}
	if NodeSuspect != 1 {
		t.Error("NodeSuspect should be 1")
	}
	if NodeDead != 2 {
		t.Error("NodeDead should be 2")
	}
	if NodeJoining != 3 {
		t.Error("NodeJoining should be 3")
	}
	if NodeLeaving != 4 {
		t.Error("NodeLeaving should be 4")
	}
}

func TestShardStateConstants(t *testing.T) {
	if ShardActive != 0 {
		t.Error("ShardActive should be 0")
	}
	if ShardMigrating != 1 {
		t.Error("ShardMigrating should be 1")
	}
	if ShardReadOnly != 2 {
		t.Error("ShardReadOnly should be 2")
	}
	if ShardInactive != 3 {
		t.Error("ShardInactive should be 3")
	}
}

func TestClusterNodeFields(t *testing.T) {
	now := time.Now()
	node := &ClusterNode{
		ID:            "test-node",
		Address:       "localhost:9000",
		JoinedAt:      now,
		LastHeartbeat: now,
		ShardCount:    5,
		Status:        NodeHealthy,
		Weight:        10,
	}

	if node.ID != "test-node" {
		t.Error("ID mismatch")
	}
	if node.Weight != 10 {
		t.Error("Weight mismatch")
	}
}

func TestShardManagerInitKeyRanges(t *testing.T) {
	sm := NewShardManager("node1", 4, 1)
	sm.Init()

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if len(sm.shards) != 4 {
		t.Fatalf("expected 4 shards, got %d", len(sm.shards))
	}

	// Last shard should have max = 0xFFFFFFFF
	lastShard := sm.shards[ShardID(3)]
	if lastShard.KeyRangeMax != 0xFFFFFFFF {
		t.Errorf("last shard max should be 0xFFFFFFFF, got %d", lastShard.KeyRangeMax)
	}

	// First shard should start at 0
	firstShard := sm.shards[ShardID(0)]
	if firstShard.KeyRangeMin != 0 {
		t.Errorf("first shard min should be 0, got %d", firstShard.KeyRangeMin)
	}
}

func TestGetShardStatsWithMigrations(t *testing.T) {
	sm := NewShardManager("node1", 4, 1)
	sm.Init()
	sm.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001", Status: NodeHealthy})

	stats := sm.GetShardStats()
	if stats["migrations"].(int) != 0 {
		t.Error("expected 0 migrations initially")
	}
}

func TestRingGetNodeWraparound(t *testing.T) {
	ring := NewRing(1)
	ring.AddNode(&ClusterNode{ID: "n1", Address: "localhost:8001"})

	// All keys should route to n1 since it's the only node
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("wrap-test-%d", i)
		node := ring.GetNode(key)
		if node != "n1" {
			t.Errorf("expected n1, got %s for key %s", node, key)
		}
	}
}
