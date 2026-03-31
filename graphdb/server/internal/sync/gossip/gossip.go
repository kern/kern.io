// Package gossip implements a WebRTC-based gossip protocol for peer-to-peer
// CRDT synchronization. The server acts as a signaling relay for WebRTC
// connections between clients. Clients can sync directly via WebRTC data
// channels, with the server providing initial signaling and fallback relay.
package gossip

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kern/graphdb/internal/crdt"
)

// PeerID uniquely identifies a connected peer.
type PeerID = string

// MessageType identifies the type of gossip message.
type MessageType int

const (
	// MsgOffer is a WebRTC SDP offer for peer connection.
	MsgOffer MessageType = iota
	// MsgAnswer is a WebRTC SDP answer.
	MsgAnswer
	// MsgICECandidate is a WebRTC ICE candidate.
	MsgICECandidate
	// MsgDelta is a CRDT delta (operations) being synced.
	MsgDelta
	// MsgVersionVector is a version vector exchange for efficient sync.
	MsgVersionVector
	// MsgDeltaRequest requests a delta from a specific version vector.
	MsgDeltaRequest
	// MsgPeerList is a list of available peers.
	MsgPeerList
	// MsgHeartbeat keeps connections alive.
	MsgHeartbeat
	// MsgDeadLetter is a notification of undeliverable messages.
	MsgDeadLetter
)

// Message is a gossip protocol message.
type Message struct {
	Type      MessageType     `json:"type"`
	From      PeerID          `json:"from"`
	To        PeerID          `json:"to,omitempty"` // empty = broadcast
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
	SeqNo     uint64          `json:"seqNo"` // for delivery tracking
}

// DeltaPayload carries CRDT operations for sync.
type DeltaPayload struct {
	Operations    []*crdt.Operation  `json:"operations"`
	VersionVector map[string]uint64  `json:"versionVector"`
}

// VersionVectorPayload carries a version vector for sync negotiation.
type VersionVectorPayload struct {
	VersionVector map[string]uint64 `json:"versionVector"`
}

// DeadLetterPayload describes an undeliverable message.
type DeadLetterPayload struct {
	OriginalSeqNo uint64 `json:"originalSeqNo"`
	Reason        string `json:"reason"`
	FailedAt      time.Time `json:"failedAt"`
	RetryCount    int    `json:"retryCount"`
}

// PeerInfo describes a connected peer.
type PeerInfo struct {
	ID            PeerID            `json:"id"`
	ReplicaID     string            `json:"replicaId"`
	ConnectedAt   time.Time         `json:"connectedAt"`
	LastHeartbeat time.Time         `json:"lastHeartbeat"`
	VersionVector map[string]uint64 `json:"versionVector"`
}

// DeadLetterQueue holds messages that could not be delivered.
type DeadLetterQueue struct {
	mu       sync.Mutex
	messages []DeadLetterEntry
	maxSize  int
}

// DeadLetterEntry is a single entry in the dead letter queue.
type DeadLetterEntry struct {
	Message    Message   `json:"message"`
	Reason     string    `json:"reason"`
	FailedAt   time.Time `json:"failedAt"`
	RetryCount int       `json:"retryCount"`
}

// NewDeadLetterQueue creates a new DLQ with the given max size.
func NewDeadLetterQueue(maxSize int) *DeadLetterQueue {
	return &DeadLetterQueue{
		maxSize: maxSize,
	}
}

// Enqueue adds a message to the dead letter queue.
func (dlq *DeadLetterQueue) Enqueue(msg Message, reason string, retryCount int) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	entry := DeadLetterEntry{
		Message:    msg,
		Reason:     reason,
		FailedAt:   time.Now(),
		RetryCount: retryCount,
	}
	dlq.messages = append(dlq.messages, entry)
	if len(dlq.messages) > dlq.maxSize {
		dlq.messages = dlq.messages[1:]
	}
}

// Drain returns and removes all dead letters.
func (dlq *DeadLetterQueue) Drain() []DeadLetterEntry {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	result := dlq.messages
	dlq.messages = nil
	return result
}

// Size returns the number of dead letters.
func (dlq *DeadLetterQueue) Size() int {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()
	return len(dlq.messages)
}

// Relay is the server-side signaling and message relay for WebRTC gossip.
// It manages peer connections, routes messages, and provides fallback relay
// when direct peer-to-peer connections aren't possible.
type Relay struct {
	mu sync.RWMutex

	// Connected peers
	peers map[PeerID]*PeerState

	// Message handlers per peer (for WebSocket delivery)
	handlers map[PeerID]func(Message)

	// Dead letter queue for undeliverable messages
	dlq *DeadLetterQueue

	// Event graph for server-side state
	graph *crdt.EventGraph

	// Config
	config RelayConfig
}

// PeerState tracks the state of a connected peer.
type PeerState struct {
	Info          PeerInfo
	sendQueue     []Message
	seqCounter    uint64
	directPeers   map[PeerID]bool // peers this peer has direct WebRTC connections to
	maxRetries    int
}

// RelayConfig configures the relay behavior.
type RelayConfig struct {
	HeartbeatInterval   time.Duration
	PeerTimeout         time.Duration
	MaxDeadLetters      int
	MaxRetries          int
	DeltaBatchSize      int
	CompactAfterOps     int // compact event graph after this many ops
}

// DefaultRelayConfig returns sensible defaults.
func DefaultRelayConfig() RelayConfig {
	return RelayConfig{
		HeartbeatInterval: 30 * time.Second,
		PeerTimeout:       90 * time.Second,
		MaxDeadLetters:    1000,
		MaxRetries:        3,
		DeltaBatchSize:    100,
		CompactAfterOps:   10000,
	}
}

// NewRelay creates a new gossip relay.
func NewRelay(graph *crdt.EventGraph, config RelayConfig) *Relay {
	return &Relay{
		peers:    make(map[PeerID]*PeerState),
		handlers: make(map[PeerID]func(Message)),
		dlq:      NewDeadLetterQueue(config.MaxDeadLetters),
		graph:    graph,
		config:   config,
	}
}

// RegisterPeer adds a peer to the relay.
func (r *Relay) RegisterPeer(peerID PeerID, replicaID string, handler func(Message)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.peers[peerID] = &PeerState{
		Info: PeerInfo{
			ID:            peerID,
			ReplicaID:     replicaID,
			ConnectedAt:   now,
			LastHeartbeat: now,
			VersionVector: make(map[string]uint64),
		},
		directPeers: make(map[PeerID]bool),
		maxRetries:  r.config.MaxRetries,
	}
	r.handlers[peerID] = handler

	// Notify other peers
	peerList := r.buildPeerList()
	for id, h := range r.handlers {
		if id != peerID {
			payload, _ := json.Marshal(peerList)
			h(Message{
				Type:      MsgPeerList,
				From:      "server",
				To:        id,
				Payload:   payload,
				Timestamp: now,
			})
		}
	}
}

// UnregisterPeer removes a peer.
func (r *Relay) UnregisterPeer(peerID PeerID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.peers, peerID)
	delete(r.handlers, peerID)

	// Notify remaining peers
	now := time.Now()
	peerList := r.buildPeerList()
	for id, h := range r.handlers {
		payload, _ := json.Marshal(peerList)
		h(Message{
			Type:      MsgPeerList,
			From:      "server",
			To:        id,
			Payload:   payload,
			Timestamp: now,
		})
	}
}

// HandleMessage processes an incoming message from a peer.
func (r *Relay) HandleMessage(msg Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Update heartbeat
	if peer, ok := r.peers[msg.From]; ok {
		peer.Info.LastHeartbeat = time.Now()
	}

	switch msg.Type {
	case MsgOffer, MsgAnswer, MsgICECandidate:
		// WebRTC signaling: forward to target peer
		return r.forwardMessage(msg)

	case MsgDelta:
		// CRDT delta: apply locally and forward to peers that need it
		return r.handleDelta(msg)

	case MsgVersionVector:
		// Version vector update from a peer
		return r.handleVersionVector(msg)

	case MsgDeltaRequest:
		// Peer requesting deltas from a version vector
		return r.handleDeltaRequest(msg)

	case MsgHeartbeat:
		return nil

	default:
		return fmt.Errorf("unknown message type: %d", msg.Type)
	}
}

func (r *Relay) forwardMessage(msg Message) error {
	handler, ok := r.handlers[msg.To]
	if !ok {
		r.dlq.Enqueue(msg, "peer not connected", 0)
		return fmt.Errorf("peer %s not connected", msg.To)
	}
	handler(msg)
	return nil
}

func (r *Relay) handleDelta(msg Message) error {
	var payload DeltaPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return fmt.Errorf("invalid delta payload: %w", err)
	}

	// Apply operations to server's event graph
	for _, op := range payload.Operations {
		if err := r.graph.Apply(op); err != nil {
			// Not fatal — may be duplicate or already seen
			continue
		}
	}

	// Update peer's version vector
	if peer, ok := r.peers[msg.From]; ok {
		for replica, seq := range payload.VersionVector {
			if seq > peer.Info.VersionVector[replica] {
				peer.Info.VersionVector[replica] = seq
			}
		}
	}

	// Forward delta to all other peers that need it
	for peerID, peer := range r.peers {
		if peerID == msg.From {
			continue
		}
		// Check if this peer has a direct connection to sender
		if peer.directPeers[msg.From] {
			continue // they'll get it directly
		}
		// Forward via relay
		handler, ok := r.handlers[peerID]
		if !ok {
			continue
		}
		// Filter operations to only those the peer hasn't seen
		var needed []*crdt.Operation
		for _, op := range payload.Operations {
			lastSeen := peer.Info.VersionVector[op.ID.ReplicaID]
			if op.ID.Seq > lastSeen {
				needed = append(needed, op)
			}
		}
		if len(needed) == 0 {
			continue
		}
		fwdPayload := DeltaPayload{
			Operations:    needed,
			VersionVector: r.graph.VersionVector(),
		}
		fwdBytes, _ := json.Marshal(fwdPayload)
		handler(Message{
			Type:      MsgDelta,
			From:      "server",
			To:        peerID,
			Payload:   fwdBytes,
			Timestamp: time.Now(),
		})
	}

	return nil
}

func (r *Relay) handleVersionVector(msg Message) error {
	var payload VersionVectorPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return fmt.Errorf("invalid version vector: %w", err)
	}

	if peer, ok := r.peers[msg.From]; ok {
		peer.Info.VersionVector = payload.VersionVector
	}

	return nil
}

func (r *Relay) handleDeltaRequest(msg Message) error {
	var payload VersionVectorPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return fmt.Errorf("invalid delta request: %w", err)
	}

	// Get delta from event graph
	ops := r.graph.DeltaSince(payload.VersionVector)

	// Batch the response
	for i := 0; i < len(ops); i += r.config.DeltaBatchSize {
		end := i + r.config.DeltaBatchSize
		if end > len(ops) {
			end = len(ops)
		}
		batch := ops[i:end]

		deltaPayload := DeltaPayload{
			Operations:    batch,
			VersionVector: r.graph.VersionVector(),
		}
		payloadBytes, _ := json.Marshal(deltaPayload)

		handler, ok := r.handlers[msg.From]
		if !ok {
			r.dlq.Enqueue(msg, "peer disconnected during delta send", 0)
			return fmt.Errorf("peer %s disconnected", msg.From)
		}
		handler(Message{
			Type:      MsgDelta,
			From:      "server",
			To:        msg.From,
			Payload:   payloadBytes,
			Timestamp: time.Now(),
		})
	}

	return nil
}

func (r *Relay) buildPeerList() []PeerInfo {
	list := make([]PeerInfo, 0, len(r.peers))
	for _, peer := range r.peers {
		list = append(list, peer.Info)
	}
	return list
}

// GetPeers returns the current peer list.
func (r *Relay) GetPeers() []PeerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.buildPeerList()
}

// SetDirectConnection marks that two peers have a direct WebRTC connection.
func (r *Relay) SetDirectConnection(peer1, peer2 PeerID, connected bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p, ok := r.peers[peer1]; ok {
		p.directPeers[peer2] = connected
	}
	if p, ok := r.peers[peer2]; ok {
		p.directPeers[peer1] = connected
	}
}

// DeadLetters returns the dead letter queue.
func (r *Relay) DeadLetters() *DeadLetterQueue {
	return r.dlq
}

// PeerCount returns the number of connected peers.
func (r *Relay) PeerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.peers)
}

// ensure imports used
var _ = uuid.Nil
