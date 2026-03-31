// Package cluster provides horizontal scalability for GraphDB via consistent
// hashing, shard management, and inter-node replication. Each server process
// owns a set of shards and replicates to peers for high availability.
package cluster

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ShardID identifies a shard (partition of the keyspace).
type ShardID = uint32

// NodeID identifies a cluster node (server process).
type NodeID = string

// NumVirtualNodes is the number of virtual nodes per physical node
// on the consistent hash ring.
const NumVirtualNodes = 128

// ShardState represents the state of a shard.
type ShardState int

const (
	// ShardActive: shard is serving reads and writes.
	ShardActive ShardState = iota
	// ShardMigrating: shard is being transferred to another node.
	ShardMigrating
	// ShardReadOnly: shard accepts reads but not writes (during migration).
	ShardReadOnly
	// ShardInactive: shard is not active on this node.
	ShardInactive
)

// Shard represents a partition of the graph keyspace.
type Shard struct {
	ID          ShardID    `json:"id"`
	State       ShardState `json:"state"`
	Owner       NodeID     `json:"owner"`
	Replicas    []NodeID   `json:"replicas"`
	KeyRangeMin uint32     `json:"keyRangeMin"`
	KeyRangeMax uint32     `json:"keyRangeMax"`
	NodeCount   int        `json:"nodeCount"`
	LastSync    time.Time  `json:"lastSync"`
}

// ClusterNode represents a server process in the cluster.
type ClusterNode struct {
	ID            NodeID    `json:"id"`
	Address       string    `json:"address"` // host:port
	JoinedAt      time.Time `json:"joinedAt"`
	LastHeartbeat time.Time `json:"lastHeartbeat"`
	ShardCount    int       `json:"shardCount"`
	Status        NodeStatus `json:"status"`
	Weight        int       `json:"weight"` // for weighted load balancing
}

// NodeStatus represents the health of a cluster node.
type NodeStatus int

const (
	NodeHealthy NodeStatus = iota
	NodeSuspect
	NodeDead
	NodeJoining
	NodeLeaving
)

// Ring implements a consistent hash ring for shard assignment.
type Ring struct {
	mu           sync.RWMutex
	nodes        map[NodeID]*ClusterNode
	ring         []ringEntry
	shards       map[ShardID]*Shard
	replicaCount int
}

type ringEntry struct {
	hash   uint32
	nodeID NodeID
}

// NewRing creates a new consistent hash ring.
func NewRing(replicaCount int) *Ring {
	if replicaCount < 1 {
		replicaCount = 1
	}
	return &Ring{
		nodes:        make(map[NodeID]*ClusterNode),
		shards:       make(map[ShardID]*Shard),
		replicaCount: replicaCount,
	}
}

// AddNode adds a node to the consistent hash ring.
func (r *Ring) AddNode(node *ClusterNode) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nodes[node.ID] = node

	// Add virtual nodes
	for i := 0; i < NumVirtualNodes; i++ {
		hash := hashKey(fmt.Sprintf("%s-%d", node.ID, i))
		r.ring = append(r.ring, ringEntry{hash: hash, nodeID: node.ID})
	}

	sort.Slice(r.ring, func(i, j int) bool {
		return r.ring[i].hash < r.ring[j].hash
	})
}

// RemoveNode removes a node from the ring.
func (r *Ring) RemoveNode(nodeID NodeID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.nodes, nodeID)

	filtered := make([]ringEntry, 0, len(r.ring))
	for _, entry := range r.ring {
		if entry.nodeID != nodeID {
			filtered = append(filtered, entry)
		}
	}
	r.ring = filtered
}

// GetNode returns the node responsible for a given key.
func (r *Ring) GetNode(key string) NodeID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.ring) == 0 {
		return ""
	}

	hash := hashKey(key)
	idx := sort.Search(len(r.ring), func(i int) bool {
		return r.ring[i].hash >= hash
	})
	if idx >= len(r.ring) {
		idx = 0
	}
	return r.ring[idx].nodeID
}

// GetNodeForUUID returns the node responsible for a UUID-keyed entity.
func (r *Ring) GetNodeForUUID(id uuid.UUID) NodeID {
	return r.GetNode(id.String())
}

// GetReplicaNodes returns the primary + replica nodes for a key.
func (r *Ring) GetReplicaNodes(key string) []NodeID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.ring) == 0 {
		return nil
	}

	hash := hashKey(key)
	idx := sort.Search(len(r.ring), func(i int) bool {
		return r.ring[i].hash >= hash
	})
	if idx >= len(r.ring) {
		idx = 0
	}

	seen := make(map[NodeID]bool)
	var result []NodeID
	for i := 0; i < len(r.ring) && len(result) < r.replicaCount+1; i++ {
		entry := r.ring[(idx+i)%len(r.ring)]
		if !seen[entry.nodeID] {
			seen[entry.nodeID] = true
			result = append(result, entry.nodeID)
		}
	}
	return result
}

// GetNodes returns all registered nodes.
func (r *Ring) GetNodes() []*ClusterNode {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*ClusterNode, 0, len(r.nodes))
	for _, n := range r.nodes {
		result = append(result, n)
	}
	return result
}

// NodeCount returns the number of nodes in the ring.
func (r *Ring) NodeCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.nodes)
}

func hashKey(key string) uint32 {
	h := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint32(h[:4])
}

// ShardManager manages shard assignment and migration.
type ShardManager struct {
	mu sync.RWMutex

	ring   *Ring
	shards map[ShardID]*Shard
	localNodeID NodeID

	// Shard count determines how many shards the keyspace is divided into.
	shardCount int

	// Migration state
	migrations map[ShardID]*Migration
}

// Migration tracks an in-progress shard migration.
type Migration struct {
	ShardID    ShardID   `json:"shardId"`
	FromNode   NodeID    `json:"fromNode"`
	ToNode     NodeID    `json:"toNode"`
	StartedAt  time.Time `json:"startedAt"`
	Progress   float64   `json:"progress"` // 0.0 to 1.0
	Status     string    `json:"status"`
}

// NewShardManager creates a new shard manager.
func NewShardManager(localNodeID NodeID, shardCount int, replicaCount int) *ShardManager {
	return &ShardManager{
		ring:        NewRing(replicaCount),
		shards:      make(map[ShardID]*Shard),
		localNodeID: localNodeID,
		shardCount:  shardCount,
		migrations:  make(map[ShardID]*Migration),
	}
}

// Init initializes shards evenly across the ring.
func (sm *ShardManager) Init() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	rangeSize := uint32(0xFFFFFFFF) / uint32(sm.shardCount)
	for i := 0; i < sm.shardCount; i++ {
		sid := ShardID(i)
		min := uint32(i) * rangeSize
		max := min + rangeSize - 1
		if i == sm.shardCount-1 {
			max = 0xFFFFFFFF
		}
		sm.shards[sid] = &Shard{
			ID:          sid,
			State:       ShardActive,
			KeyRangeMin: min,
			KeyRangeMax: max,
		}
	}
}

// AssignShards assigns shards to nodes based on the consistent hash ring.
func (sm *ShardManager) AssignShards() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for sid, shard := range sm.shards {
		key := fmt.Sprintf("shard-%d", sid)
		nodes := sm.ring.GetReplicaNodes(key)
		if len(nodes) > 0 {
			shard.Owner = nodes[0]
			if len(nodes) > 1 {
				shard.Replicas = nodes[1:]
			}
		}
	}
}

// GetLocalShards returns shards owned by or replicated to this node.
func (sm *ShardManager) GetLocalShards() []*Shard {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var result []*Shard
	for _, shard := range sm.shards {
		if shard.Owner == sm.localNodeID {
			result = append(result, shard)
			continue
		}
		for _, replica := range shard.Replicas {
			if replica == sm.localNodeID {
				result = append(result, shard)
				break
			}
		}
	}
	return result
}

// IsLocalShard checks if a given UUID belongs to a shard owned by this node.
func (sm *ShardManager) IsLocalShard(id uuid.UUID) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	hash := hashKey(id.String())
	for _, shard := range sm.shards {
		if hash >= shard.KeyRangeMin && hash <= shard.KeyRangeMax {
			if shard.Owner == sm.localNodeID {
				return true
			}
			for _, replica := range shard.Replicas {
				if replica == sm.localNodeID {
					return true
				}
			}
			return false
		}
	}
	return false
}

// RouteNode returns the node that should handle a specific UUID.
func (sm *ShardManager) RouteNode(id uuid.UUID) NodeID {
	return sm.ring.GetNodeForUUID(id)
}

// AddNode registers a new node and rebalances shards.
func (sm *ShardManager) AddNode(node *ClusterNode) {
	sm.ring.AddNode(node)
	sm.AssignShards()
}

// RemoveNode removes a node and rebalances.
func (sm *ShardManager) RemoveNode(nodeID NodeID) {
	sm.ring.RemoveNode(nodeID)
	sm.AssignShards()
}

// GetShardStats returns statistics about shard distribution.
func (sm *ShardManager) GetShardStats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	ownerCounts := make(map[NodeID]int)
	for _, shard := range sm.shards {
		ownerCounts[shard.Owner]++
	}

	return map[string]interface{}{
		"totalShards":  sm.shardCount,
		"nodeCount":    sm.ring.NodeCount(),
		"distribution": ownerCounts,
		"migrations":   len(sm.migrations),
	}
}
