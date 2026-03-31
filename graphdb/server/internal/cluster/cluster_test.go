package cluster

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRingConsistentHashing(t *testing.T) {
	ring := NewRing(2)

	ring.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001", Weight: 1})
	ring.AddNode(&ClusterNode{ID: "node2", Address: "localhost:8002", Weight: 1})
	ring.AddNode(&ClusterNode{ID: "node3", Address: "localhost:8003", Weight: 1})

	// Same key should always map to same node
	key := "test-key-123"
	node1 := ring.GetNode(key)
	node2 := ring.GetNode(key)
	if node1 != node2 {
		t.Error("consistent hashing should return same node for same key")
	}

	// Should return a valid node
	if node1 == "" {
		t.Error("should return a node")
	}
}

func TestRingNodeRemoval(t *testing.T) {
	ring := NewRing(1)

	ring.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001"})
	ring.AddNode(&ClusterNode{ID: "node2", Address: "localhost:8002"})

	// Record assignments for 100 keys
	assignments := make(map[string]NodeID)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		assignments[key] = ring.GetNode(key)
	}

	// Remove node2
	ring.RemoveNode("node2")

	// All keys previously on node2 should move to node1
	// All keys previously on node1 should stay
	for key, oldNode := range assignments {
		newNode := ring.GetNode(key)
		if oldNode == "node1" && newNode != "node1" {
			t.Errorf("key %s should stay on node1, moved to %s", key, newNode)
		}
		if newNode != "node1" {
			t.Errorf("after removing node2, all keys should be on node1, got %s", newNode)
		}
	}
}

func TestRingReplicaNodes(t *testing.T) {
	ring := NewRing(2) // 2 replicas

	ring.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001"})
	ring.AddNode(&ClusterNode{ID: "node2", Address: "localhost:8002"})
	ring.AddNode(&ClusterNode{ID: "node3", Address: "localhost:8003"})

	key := "test-replica-key"
	nodes := ring.GetReplicaNodes(key)

	// Should return 3 nodes (1 primary + 2 replicas)
	if len(nodes) != 3 {
		t.Errorf("expected 3 replica nodes, got %d", len(nodes))
	}

	// All nodes should be unique
	seen := make(map[NodeID]bool)
	for _, n := range nodes {
		if seen[n] {
			t.Errorf("duplicate node in replica set: %s", n)
		}
		seen[n] = true
	}
}

func TestRingUUIDRouting(t *testing.T) {
	ring := NewRing(1)

	ring.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001"})
	ring.AddNode(&ClusterNode{ID: "node2", Address: "localhost:8002"})

	id := uuid.New()
	node := ring.GetNodeForUUID(id)
	if node == "" {
		t.Error("should return a node for UUID")
	}

	// Same UUID should always route to same node
	node2 := ring.GetNodeForUUID(id)
	if node != node2 {
		t.Error("same UUID should route to same node")
	}
}

func TestShardManagerInit(t *testing.T) {
	sm := NewShardManager("local-node", 16, 2)
	sm.Init()

	sm.AddNode(&ClusterNode{
		ID:       "local-node",
		Address:  "localhost:8001",
		JoinedAt: time.Now(),
		Status:   NodeHealthy,
	})
	sm.AddNode(&ClusterNode{
		ID:       "remote-node",
		Address:  "localhost:8002",
		JoinedAt: time.Now(),
		Status:   NodeHealthy,
	})

	localShards := sm.GetLocalShards()
	if len(localShards) == 0 {
		t.Error("local node should own some shards")
	}

	stats := sm.GetShardStats()
	if stats["totalShards"].(int) != 16 {
		t.Errorf("expected 16 shards, got %v", stats["totalShards"])
	}
	if stats["nodeCount"].(int) != 2 {
		t.Errorf("expected 2 nodes, got %v", stats["nodeCount"])
	}
}

func TestShardManagerRouting(t *testing.T) {
	sm := NewShardManager("node1", 8, 1)
	sm.Init()

	sm.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001", Status: NodeHealthy})
	sm.AddNode(&ClusterNode{ID: "node2", Address: "localhost:8002", Status: NodeHealthy})

	id := uuid.New()
	node := sm.RouteNode(id)
	if node == "" {
		t.Error("should route to a node")
	}

	// Check IsLocalShard
	isLocal := sm.IsLocalShard(id)
	expectedLocal := (node == "node1")
	// If the shard is on a replica, isLocal could also be true
	if !isLocal && expectedLocal {
		t.Error("IsLocalShard mismatch with RouteNode")
	}
}

func TestShardManagerRebalance(t *testing.T) {
	sm := NewShardManager("node1", 64, 1) // 64 shards for better distribution
	sm.Init()

	sm.AddNode(&ClusterNode{ID: "node1", Address: "localhost:8001", Status: NodeHealthy})

	// All shards should be on node1
	local := sm.GetLocalShards()
	if len(local) != 64 {
		t.Errorf("with single node, all 64 shards should be local, got %d", len(local))
	}

	// Add second node
	sm.AddNode(&ClusterNode{ID: "node2", Address: "localhost:8002", Status: NodeHealthy})

	// With 64 shards and consistent hashing, both nodes should own some
	local = sm.GetLocalShards()
	stats := sm.GetShardStats()
	dist := stats["distribution"].(map[NodeID]int)
	node1Count := dist["node1"]
	node2Count := dist["node2"]

	// Both nodes should own at least 1 shard with 64 shards
	if node1Count == 0 {
		t.Error("node1 should own some shards after rebalance")
	}
	if node2Count == 0 {
		t.Error("node2 should own some shards after rebalance")
	}
	if node1Count+node2Count != 64 {
		t.Errorf("total should be 64, got %d", node1Count+node2Count)
	}
	_ = local
}

func TestShardDistribution(t *testing.T) {
	sm := NewShardManager("node1", 64, 1)
	sm.Init()

	for i := 1; i <= 4; i++ {
		sm.AddNode(&ClusterNode{
			ID:       fmt.Sprintf("node%d", i),
			Address:  fmt.Sprintf("localhost:%d", 8000+i),
			Status:   NodeHealthy,
		})
	}

	stats := sm.GetShardStats()
	dist := stats["distribution"].(map[NodeID]int)

	// Each node should own roughly 64/4 = 16 shards
	// With consistent hashing, distribution won't be perfectly even
	// but each node should own at least some shards
	for nodeID, count := range dist {
		if count == 0 {
			t.Errorf("node %s should own some shards", nodeID)
		}
	}

	total := 0
	for _, count := range dist {
		total += count
	}
	if total != 64 {
		t.Errorf("total assigned shards should be 64, got %d", total)
	}
}
