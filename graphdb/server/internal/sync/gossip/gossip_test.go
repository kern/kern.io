package gossip

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/kern/graphdb/internal/crdt"
)

func TestRelayPeerRegistration(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	var received []Message
	var mu sync.Mutex
	handler := func(msg Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	relay.RegisterPeer("peer1", "r1", handler)
	if relay.PeerCount() != 1 {
		t.Errorf("expected 1 peer, got %d", relay.PeerCount())
	}

	relay.RegisterPeer("peer2", "r2", handler)
	if relay.PeerCount() != 2 {
		t.Errorf("expected 2 peers, got %d", relay.PeerCount())
	}

	// peer1 should have received a peer list update when peer2 joined
	mu.Lock()
	if len(received) != 1 {
		t.Errorf("expected 1 message (peer list update), got %d", len(received))
	}
	if len(received) > 0 && received[0].Type != MsgPeerList {
		t.Errorf("expected MsgPeerList, got %d", received[0].Type)
	}
	mu.Unlock()

	relay.UnregisterPeer("peer1")
	if relay.PeerCount() != 1 {
		t.Errorf("expected 1 peer after unregister, got %d", relay.PeerCount())
	}
}

func TestRelaySignalingForward(t *testing.T) {
	graph := crdt.NewEventGraph()
	relay := NewRelay(graph, DefaultRelayConfig())

	var peer2Received []Message
	var mu sync.Mutex

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})
	relay.RegisterPeer("peer2", "r2", func(msg Message) {
		mu.Lock()
		peer2Received = append(peer2Received, msg)
		mu.Unlock()
	})

	// Peer1 sends SDP offer to peer2
	offer := Message{
		Type:    MsgOffer,
		From:    "peer1",
		To:      "peer2",
		Payload: json.RawMessage(`{"sdp":"test-offer"}`),
	}

	if err := relay.HandleMessage(offer); err != nil {
		t.Fatalf("forward failed: %v", err)
	}

	mu.Lock()
	// peer2 should have received: 1 peer list update + 1 offer
	found := false
	for _, msg := range peer2Received {
		if msg.Type == MsgOffer {
			found = true
		}
	}
	mu.Unlock()
	if !found {
		t.Error("peer2 should have received the offer")
	}
}

func TestRelayDeltaSync(t *testing.T) {
	graph := crdt.NewEventGraph()
	w := crdt.NewEGWalker("server")

	relay := NewRelay(w.Graph(), DefaultRelayConfig())

	var peer2Messages []Message
	var mu sync.Mutex

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})
	relay.RegisterPeer("peer2", "r2", func(msg Message) {
		mu.Lock()
		peer2Messages = append(peer2Messages, msg)
		mu.Unlock()
	})
	_ = graph

	// Peer1 sends a delta with operations
	_, op1, _ := w.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})

	deltaPayload := DeltaPayload{
		Operations:    []*crdt.Operation{op1},
		VersionVector: map[string]uint64{"server": 1},
	}
	payloadBytes, _ := json.Marshal(deltaPayload)

	msg := Message{
		Type:    MsgDelta,
		From:    "peer1",
		Payload: payloadBytes,
	}

	if err := relay.HandleMessage(msg); err != nil {
		t.Fatalf("delta handling failed: %v", err)
	}

	// peer2 should have received the delta (relayed from server)
	mu.Lock()
	foundDelta := false
	for _, m := range peer2Messages {
		if m.Type == MsgDelta {
			foundDelta = true
		}
	}
	mu.Unlock()
	if !foundDelta {
		t.Error("peer2 should have received relayed delta")
	}
}

func TestRelayDeltaRequest(t *testing.T) {
	w := crdt.NewEGWalker("server")
	relay := NewRelay(w.Graph(), DefaultRelayConfig())

	// Add some data
	w.InsertNode("user", nil, map[string]interface{}{"name": "Alice"})
	w.InsertNode("user", nil, map[string]interface{}{"name": "Bob"})

	var peerMessages []Message
	var mu sync.Mutex

	relay.RegisterPeer("peer1", "r1", func(msg Message) {
		mu.Lock()
		peerMessages = append(peerMessages, msg)
		mu.Unlock()
	})

	// Peer requests delta from empty version vector (full sync)
	vv := VersionVectorPayload{VersionVector: map[string]uint64{}}
	payloadBytes, _ := json.Marshal(vv)

	msg := Message{
		Type:    MsgDeltaRequest,
		From:    "peer1",
		Payload: payloadBytes,
	}

	if err := relay.HandleMessage(msg); err != nil {
		t.Fatalf("delta request failed: %v", err)
	}

	mu.Lock()
	foundDelta := false
	for _, m := range peerMessages {
		if m.Type == MsgDelta {
			foundDelta = true
			var dp DeltaPayload
			json.Unmarshal(m.Payload, &dp)
			if len(dp.Operations) < 2 {
				t.Errorf("expected at least 2 operations, got %d", len(dp.Operations))
			}
		}
	}
	mu.Unlock()
	if !foundDelta {
		t.Error("peer should have received delta response")
	}
}

func TestDeadLetterQueue(t *testing.T) {
	dlq := NewDeadLetterQueue(3)

	dlq.Enqueue(Message{From: "p1", SeqNo: 1}, "disconnected", 0)
	dlq.Enqueue(Message{From: "p1", SeqNo: 2}, "timeout", 1)

	if dlq.Size() != 2 {
		t.Errorf("expected 2 dead letters, got %d", dlq.Size())
	}

	// Add more than max
	dlq.Enqueue(Message{From: "p2", SeqNo: 3}, "error", 0)
	dlq.Enqueue(Message{From: "p2", SeqNo: 4}, "error", 0)

	if dlq.Size() != 3 {
		t.Errorf("expected max 3 dead letters, got %d", dlq.Size())
	}

	entries := dlq.Drain()
	if len(entries) != 3 {
		t.Errorf("expected 3 drained entries, got %d", len(entries))
	}
	if dlq.Size() != 0 {
		t.Error("DLQ should be empty after drain")
	}

	// First entry should be the second one (first was evicted)
	if entries[0].Message.SeqNo != 2 {
		t.Errorf("expected first entry seqNo=2, got %d", entries[0].Message.SeqNo)
	}
}

func TestRelayDirectConnectionSkipsRelay(t *testing.T) {
	w := crdt.NewEGWalker("server")
	relay := NewRelay(w.Graph(), DefaultRelayConfig())

	var peer2Messages []Message
	var mu sync.Mutex

	relay.RegisterPeer("peer1", "r1", func(msg Message) {})
	relay.RegisterPeer("peer2", "r2", func(msg Message) {
		mu.Lock()
		peer2Messages = append(peer2Messages, msg)
		mu.Unlock()
	})

	// Mark peer1 and peer2 as having a direct connection
	relay.SetDirectConnection("peer1", "peer2", true)

	// Send a delta from peer1
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

	// peer2 should NOT get the relay (they have a direct connection)
	mu.Lock()
	deltaCount := 0
	for _, m := range peer2Messages {
		if m.Type == MsgDelta {
			deltaCount++
		}
	}
	mu.Unlock()
	if deltaCount > 0 {
		t.Error("peer2 should not receive relayed delta when direct connection exists")
	}
}
