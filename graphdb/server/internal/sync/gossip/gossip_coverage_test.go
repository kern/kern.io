package gossip

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/kern/graphdb/internal/crdt"
)

func TestGetPeers(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	// No peers
	peers := relay.GetPeers()
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}

	relay.RegisterPeer("p1", "r1", func(msg Message) {})
	relay.RegisterPeer("p2", "r2", func(msg Message) {})

	peers = relay.GetPeers()
	if len(peers) != 2 {
		t.Errorf("expected 2 peers, got %d", len(peers))
	}

	// Check peer info
	found := map[string]bool{}
	for _, p := range peers {
		found[p.ID] = true
		if p.VersionVector == nil {
			t.Error("peer should have initialized version vector")
		}
	}
	if !found["p1"] || !found["p2"] {
		t.Error("expected both p1 and p2 in peer list")
	}
}

func TestDeadLettersAccessor(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	dlq := relay.DeadLetters()
	if dlq == nil {
		t.Fatal("DeadLetters() should not return nil")
	}
	if dlq.Size() != 0 {
		t.Error("dead letter queue should be empty initially")
	}

	// Enqueue something and verify
	dlq.Enqueue(Message{From: "x", SeqNo: 1}, "test", 0)
	if dlq.Size() != 1 {
		t.Errorf("expected 1 dead letter, got %d", dlq.Size())
	}
}

func TestHandleVersionVector(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})

	vv := VersionVectorPayload{
		VersionVector: map[string]uint64{"r1": 5, "r2": 10},
	}
	payloadBytes, _ := json.Marshal(vv)

	msg := Message{
		Type:    MsgVersionVector,
		From:    "peer1",
		Payload: payloadBytes,
	}

	err := relay.HandleMessage(msg)
	if err != nil {
		t.Fatalf("handleVersionVector failed: %v", err)
	}

	// Verify the version vector was updated
	peers := relay.GetPeers()
	for _, p := range peers {
		if p.ID == "peer1" {
			if p.VersionVector["r1"] != 5 {
				t.Errorf("expected r1=5, got %d", p.VersionVector["r1"])
			}
			if p.VersionVector["r2"] != 10 {
				t.Errorf("expected r2=10, got %d", p.VersionVector["r2"])
			}
		}
	}
}

func TestHandleVersionVectorInvalidPayload(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})

	msg := Message{
		Type:    MsgVersionVector,
		From:    "peer1",
		Payload: json.RawMessage(`{invalid`),
	}

	err := relay.HandleMessage(msg)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestHandleVersionVectorUnknownPeer(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	vv := VersionVectorPayload{
		VersionVector: map[string]uint64{"r1": 5},
	}
	payloadBytes, _ := json.Marshal(vv)

	msg := Message{
		Type:    MsgVersionVector,
		From:    "unknown-peer",
		Payload: payloadBytes,
	}

	// Should not error even if peer not found
	err := relay.HandleMessage(msg)
	if err != nil {
		t.Fatalf("should not error for unknown peer: %v", err)
	}
}

func TestForwardMessagePeerNotConnected(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})

	msg := Message{
		Type:    MsgOffer,
		From:    "peer1",
		To:      "nonexistent-peer",
		Payload: json.RawMessage(`{"sdp":"test"}`),
	}

	err := relay.HandleMessage(msg)
	if err == nil {
		t.Error("expected error for forwarding to non-existent peer")
	}

	// Check dead letter queue
	dlq := relay.DeadLetters()
	if dlq.Size() != 1 {
		t.Errorf("expected 1 dead letter, got %d", dlq.Size())
	}
}

func TestForwardMessageAnswer(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	var received []Message
	var mu sync.Mutex

	relay.RegisterPeer("peer1", "r1", func(msg Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	})
	relay.RegisterPeer("peer2", "r2", func(msg Message) {})

	msg := Message{
		Type:    MsgAnswer,
		From:    "peer2",
		To:      "peer1",
		Payload: json.RawMessage(`{"sdp":"answer"}`),
	}

	err := relay.HandleMessage(msg)
	if err != nil {
		t.Fatalf("forward answer failed: %v", err)
	}

	mu.Lock()
	foundAnswer := false
	for _, m := range received {
		if m.Type == MsgAnswer {
			foundAnswer = true
		}
	}
	mu.Unlock()
	if !foundAnswer {
		t.Error("peer1 should have received the answer")
	}
}

func TestForwardMessageICECandidate(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	var received []Message
	var mu sync.Mutex

	relay.RegisterPeer("peer1", "r1", func(msg Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	})
	relay.RegisterPeer("peer2", "r2", func(msg Message) {})

	msg := Message{
		Type:    MsgICECandidate,
		From:    "peer2",
		To:      "peer1",
		Payload: json.RawMessage(`{"candidate":"test-candidate"}`),
	}

	err := relay.HandleMessage(msg)
	if err != nil {
		t.Fatalf("forward ICE candidate failed: %v", err)
	}

	mu.Lock()
	foundICE := false
	for _, m := range received {
		if m.Type == MsgICECandidate {
			foundICE = true
		}
	}
	mu.Unlock()
	if !foundICE {
		t.Error("peer1 should have received the ICE candidate")
	}
}

func TestHandleMessageUnknownType(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})

	msg := Message{
		Type: MessageType(99),
		From: "peer1",
	}

	err := relay.HandleMessage(msg)
	if err == nil {
		t.Error("expected error for unknown message type")
	}
}

func TestHandleMessageHeartbeat(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})

	msg := Message{
		Type: MsgHeartbeat,
		From: "peer1",
	}

	err := relay.HandleMessage(msg)
	if err != nil {
		t.Fatalf("heartbeat should succeed: %v", err)
	}
}

func TestHandleDeltaInvalidPayload(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})

	msg := Message{
		Type:    MsgDelta,
		From:    "peer1",
		Payload: json.RawMessage(`{invalid`),
	}

	err := relay.HandleMessage(msg)
	if err == nil {
		t.Error("expected error for invalid delta payload")
	}
}

func TestHandleDeltaRequestInvalidPayload(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})

	msg := Message{
		Type:    MsgDeltaRequest,
		From:    "peer1",
		Payload: json.RawMessage(`{invalid`),
	}

	err := relay.HandleMessage(msg)
	if err == nil {
		t.Error("expected error for invalid delta request payload")
	}
}

func TestHandleDeltaMultiplePeersFiltering(t *testing.T) {
	w := crdt.NewEGWalker("server")
	relay := NewRelay(w.Graph(), DefaultRelayConfig())

	var peer2Messages []Message
	var peer3Messages []Message
	var mu sync.Mutex

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})
	relay.RegisterPeer("peer2", "r2", func(msg Message) {
		mu.Lock()
		peer2Messages = append(peer2Messages, msg)
		mu.Unlock()
	})
	relay.RegisterPeer("peer3", "r3", func(msg Message) {
		mu.Lock()
		peer3Messages = append(peer3Messages, msg)
		mu.Unlock()
	})

	// Give peer2 a version vector that already has the ops
	relay.mu.Lock()
	relay.peers["peer2"].Info.VersionVector = map[string]uint64{"server": 100}
	relay.mu.Unlock()

	_, op, _ := w.InsertNode("task", nil, nil)
	deltaPayload := DeltaPayload{
		Operations:    []*crdt.Operation{op},
		VersionVector: map[string]uint64{"server": 1},
	}
	payloadBytes, _ := json.Marshal(deltaPayload)

	relay.HandleMessage(Message{
		Type:    MsgDelta,
		From:    "peer1",
		Payload: payloadBytes,
	})

	mu.Lock()
	// peer2 should NOT get delta (already has it)
	peer2DeltaCount := 0
	for _, m := range peer2Messages {
		if m.Type == MsgDelta {
			peer2DeltaCount++
		}
	}
	// peer3 should get delta
	peer3DeltaCount := 0
	for _, m := range peer3Messages {
		if m.Type == MsgDelta {
			peer3DeltaCount++
		}
	}
	mu.Unlock()

	if peer2DeltaCount > 0 {
		t.Error("peer2 should not receive delta (version vector already ahead)")
	}
	if peer3DeltaCount == 0 {
		t.Error("peer3 should receive delta")
	}
}

func TestHandleDeltaRequestDisconnectedPeer(t *testing.T) {
	w := crdt.NewEGWalker("server")
	relay := NewRelay(w.Graph(), DefaultRelayConfig())

	// Add data
	w.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})

	// Register peer then immediately unregister to simulate disconnect
	relay.RegisterPeer("peer1", "r1", func(msg Message) {})

	vv := VersionVectorPayload{VersionVector: map[string]uint64{}}
	payloadBytes, _ := json.Marshal(vv)

	// Manually remove handler to simulate disconnection during delta send
	relay.mu.Lock()
	delete(relay.handlers, "peer1")
	relay.mu.Unlock()

	msg := Message{
		Type:    MsgDeltaRequest,
		From:    "peer1",
		Payload: payloadBytes,
	}

	err := relay.HandleMessage(msg)
	if err == nil {
		t.Error("expected error for disconnected peer during delta request")
	}
}

func TestUnregisterPeerNotifiesOthers(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	var peer2Messages []Message
	var mu sync.Mutex

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})
	relay.RegisterPeer("peer2", "r2", func(msg Message) {
		mu.Lock()
		peer2Messages = append(peer2Messages, msg)
		mu.Unlock()
	})

	// Reset to only track unregister notifications
	mu.Lock()
	peer2Messages = nil
	mu.Unlock()

	relay.UnregisterPeer("peer1")

	mu.Lock()
	foundPeerList := false
	for _, m := range peer2Messages {
		if m.Type == MsgPeerList {
			foundPeerList = true
		}
	}
	mu.Unlock()
	if !foundPeerList {
		t.Error("peer2 should receive peer list update after peer1 unregisters")
	}
}

func TestSetDirectConnectionBothWays(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})
	relay.RegisterPeer("peer2", "r2", func(msg Message) {})

	relay.SetDirectConnection("peer1", "peer2", true)

	relay.mu.RLock()
	if !relay.peers["peer1"].directPeers["peer2"] {
		t.Error("peer1 should know about direct connection to peer2")
	}
	if !relay.peers["peer2"].directPeers["peer1"] {
		t.Error("peer2 should know about direct connection to peer1")
	}
	relay.mu.RUnlock()

	// Disconnect
	relay.SetDirectConnection("peer1", "peer2", false)

	relay.mu.RLock()
	if relay.peers["peer1"].directPeers["peer2"] {
		t.Error("peer1 should no longer have direct connection to peer2")
	}
	relay.mu.RUnlock()
}

func TestSetDirectConnectionUnknownPeer(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})

	// Should not panic for unknown peer
	relay.SetDirectConnection("peer1", "unknown", true)
	relay.SetDirectConnection("unknown", "peer1", true)
}

func TestPeerCountAfterOperations(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	if relay.PeerCount() != 0 {
		t.Error("expected 0 peers initially")
	}

	relay.RegisterPeer("p1", "r1", func(msg Message) {})
	relay.RegisterPeer("p2", "r2", func(msg Message) {})
	relay.RegisterPeer("p3", "r3", func(msg Message) {})

	if relay.PeerCount() != 3 {
		t.Errorf("expected 3 peers, got %d", relay.PeerCount())
	}

	relay.UnregisterPeer("p2")
	if relay.PeerCount() != 2 {
		t.Errorf("expected 2 peers after unregister, got %d", relay.PeerCount())
	}
}

func TestDeadLetterQueueOverflow(t *testing.T) {
	dlq := NewDeadLetterQueue(2)

	dlq.Enqueue(Message{SeqNo: 1}, "r1", 0)
	dlq.Enqueue(Message{SeqNo: 2}, "r2", 0)
	dlq.Enqueue(Message{SeqNo: 3}, "r3", 0)

	if dlq.Size() != 2 {
		t.Errorf("expected 2 (capped), got %d", dlq.Size())
	}

	entries := dlq.Drain()
	// First entry should be SeqNo 2 (1 was evicted)
	if entries[0].Message.SeqNo != 2 {
		t.Errorf("expected seqNo=2 first, got %d", entries[0].Message.SeqNo)
	}
	if entries[1].Message.SeqNo != 3 {
		t.Errorf("expected seqNo=3 second, got %d", entries[1].Message.SeqNo)
	}
}

func TestDefaultRelayConfig(t *testing.T) {
	cfg := DefaultRelayConfig()
	if cfg.MaxDeadLetters != 1000 {
		t.Errorf("expected MaxDeadLetters=1000, got %d", cfg.MaxDeadLetters)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", cfg.MaxRetries)
	}
	if cfg.DeltaBatchSize != 100 {
		t.Errorf("expected DeltaBatchSize=100, got %d", cfg.DeltaBatchSize)
	}
}

func TestHandleMessageUpdatesHeartbeat(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})

	// Get initial heartbeat
	peers := relay.GetPeers()
	initialHB := peers[0].LastHeartbeat

	// Send a heartbeat message
	relay.HandleMessage(Message{
		Type: MsgHeartbeat,
		From: "peer1",
	})

	peers = relay.GetPeers()
	if peers[0].LastHeartbeat.Before(initialHB) {
		t.Error("heartbeat timestamp should be updated")
	}
}
