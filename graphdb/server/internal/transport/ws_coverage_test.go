package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kern/graphdb/internal/crdt"
	"github.com/kern/graphdb/internal/function"
	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
	gsync "github.com/kern/graphdb/internal/sync"
)

// setupWS creates a WSHandler with test functions and a running httptest server.
// Returns the server, a connected websocket client, and a cleanup function.
func setupWS(t *testing.T) (*httptest.Server, *websocket.Conn, func()) {
	t.Helper()

	store := graph.NewStore("ws-test")
	validator := invariant.NewValidator()
	reg := function.NewRegistry(store, validator)
	walker := crdt.NewEGWalker("ws-test")
	reactor := gsync.NewReactor(reg, walker)

	reg.RegisterQuery("echo", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return args["msg"], nil
	})
	reg.RegisterQuery("failQuery", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, fmt.Errorf("query error")
	})
	reg.RegisterMutation("createItem", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		id, err := mctx.InsertNode("item", nil, map[string]interface{}{"name": args["name"]})
		return id, err
	})

	handler := NewWSHandler(reg, reactor, walker)
	server := httptest.NewServer(handler)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}

	cleanup := func() {
		conn.Close()
		server.Close()
	}
	return server, conn, cleanup
}

// readUntilType reads messages from the websocket, skipping subscription_update
// messages, until a message of the desired type arrives or the deadline expires.
func readUntilType(t *testing.T, conn *websocket.Conn, msgType MessageType, timeout time.Duration) ServerMessage {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		conn.SetReadDeadline(deadline)
		var resp ServerMessage
		if err := conn.ReadJSON(&resp); err != nil {
			t.Fatalf("failed to read message of type %q: %v", msgType, err)
		}
		if resp.Type == msgType {
			return resp
		}
		// Skip subscription_update messages
		if resp.Type != MsgSubscription {
			t.Fatalf("unexpected message type %q while waiting for %q", resp.Type, msgType)
		}
	}
}

// sendAndReceive sends a ClientMessage and reads back a ServerMessage.
func sendAndReceive(t *testing.T, conn *websocket.Conn, msg ClientMessage) ServerMessage {
	t.Helper()
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("failed to write message: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp ServerMessage
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	return resp
}

// --- NewWSHandler ---

func TestNewWSHandler(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := function.NewRegistry(store, validator)
	walker := crdt.NewEGWalker("r1")
	reactor := gsync.NewReactor(reg, walker)

	h := NewWSHandler(reg, reactor, walker)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.registry != reg {
		t.Error("registry mismatch")
	}
	if h.reactor != reactor {
		t.Error("reactor mismatch")
	}
	if h.walker != walker {
		t.Error("walker mismatch")
	}
}

// --- ServeHTTP: upgrade failure (non-websocket request) ---

func TestServeHTTP_UpgradeFails(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := function.NewRegistry(store, validator)
	walker := crdt.NewEGWalker("r1")
	reactor := gsync.NewReactor(reg, walker)
	handler := NewWSHandler(reg, reactor, walker)

	// Plain HTTP request should fail the upgrade and return an error status.
	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusSwitchingProtocols {
		t.Error("expected upgrade to fail for a plain HTTP request")
	}
}

// --- handleCall: successful function call ---

func TestWSHandleCallSuccess(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	resp := sendAndReceive(t, conn, ClientMessage{
		Type:         MsgCallFunction,
		RequestID:    "req-1",
		FunctionName: "echo",
		Args:         map[string]interface{}{"msg": "hello"},
	})

	if resp.Type != MsgResult {
		t.Errorf("expected type %q, got %q", MsgResult, resp.Type)
	}
	if resp.RequestID != "req-1" {
		t.Errorf("expected requestId req-1, got %q", resp.RequestID)
	}
	if resp.Value != "hello" {
		t.Errorf("expected value 'hello', got %v", resp.Value)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

// --- handleCall: function returns error ---

func TestWSHandleCallError(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	resp := sendAndReceive(t, conn, ClientMessage{
		Type:         MsgCallFunction,
		RequestID:    "req-2",
		FunctionName: "failQuery",
	})

	if resp.Type != MsgResult {
		t.Errorf("expected type %q, got %q", MsgResult, resp.Type)
	}
	if resp.Error == "" {
		t.Error("expected an error in response")
	}
}

// --- handleCall: function not found ---

func TestWSHandleCallNotFound(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	resp := sendAndReceive(t, conn, ClientMessage{
		Type:         MsgCallFunction,
		RequestID:    "req-3",
		FunctionName: "nonexistent",
	})

	if resp.Type != MsgResult {
		t.Errorf("expected type %q, got %q", MsgResult, resp.Type)
	}
	if resp.Error == "" {
		t.Error("expected error for nonexistent function")
	}
}

// --- handleMessage: unknown message type ---

func TestWSHandleUnknownMessageType(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	resp := sendAndReceive(t, conn, ClientMessage{
		Type:      MessageType("bogus"),
		RequestID: "req-unk",
	})

	if resp.Type != MsgError {
		t.Errorf("expected type %q, got %q", MsgError, resp.Type)
	}
	if resp.Error != "unknown message type" {
		t.Errorf("expected 'unknown message type', got %q", resp.Error)
	}
}

// --- readLoop: invalid JSON ---

func TestWSReadLoopInvalidJSON(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	// Send raw invalid JSON bytes
	err := conn.WriteMessage(websocket.TextMessage, []byte("{not valid json"))
	if err != nil {
		t.Fatalf("failed to write raw message: %v", err)
	}

	// The server should respond with an error message
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp ServerMessage
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("failed to read error response: %v", err)
	}

	if resp.Type != MsgError {
		t.Errorf("expected type %q, got %q", MsgError, resp.Type)
	}
	if resp.Error != "invalid message format" {
		t.Errorf("expected 'invalid message format', got %q", resp.Error)
	}
}

// --- handleSubscribe ---

func TestWSHandleSubscribe(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	resp := sendAndReceive(t, conn, ClientMessage{
		Type:         MsgSubscribe,
		RequestID:    "sub-1",
		FunctionName: "echo",
		Args:         map[string]interface{}{"msg": "subscribed"},
	})

	if resp.Type != MsgResult {
		t.Errorf("expected type %q, got %q", MsgResult, resp.Type)
	}
	if resp.RequestID != "sub-1" {
		t.Errorf("expected requestId sub-1, got %q", resp.RequestID)
	}
	if resp.SubscriptionID == 0 {
		t.Error("expected non-zero subscription ID")
	}
}

// --- handleUnsubscribe ---

func TestWSHandleUnsubscribe(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	// First subscribe
	resp := sendAndReceive(t, conn, ClientMessage{
		Type:         MsgSubscribe,
		RequestID:    "sub-2",
		FunctionName: "echo",
		Args:         map[string]interface{}{"msg": "test"},
	})
	subID := resp.SubscriptionID

	// Now unsubscribe
	if err := conn.WriteJSON(ClientMessage{
		Type:           MsgUnsubscribe,
		RequestID:      "unsub-1",
		SubscriptionID: subID,
	}); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Read responses, skipping any subscription_update messages
	resp = readUntilType(t, conn, MsgResult, 2*time.Second)

	if resp.RequestID != "unsub-1" {
		t.Errorf("expected requestId unsub-1, got %q", resp.RequestID)
	}
}

// --- handleSync: empty sync (no ops, no frontier) ---

func TestWSHandleSyncEmpty(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	resp := sendAndReceive(t, conn, ClientMessage{
		Type:      MsgSync,
		RequestID: "sync-1",
	})

	if resp.Type != MsgSyncResponse {
		t.Errorf("expected type %q, got %q", MsgSyncResponse, resp.Type)
	}
	if resp.RequestID != "sync-1" {
		t.Errorf("expected requestId sync-1, got %q", resp.RequestID)
	}
}

// --- handleSync: with valid frontier ---

func TestWSHandleSyncWithFrontier(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	frontierBytes, _ := json.Marshal([]crdt.EventID{})
	resp := sendAndReceive(t, conn, ClientMessage{
		Type:      MsgSync,
		RequestID: "sync-2",
		Frontier:  json.RawMessage(frontierBytes),
	})

	if resp.Type != MsgSyncResponse {
		t.Errorf("expected type %q, got %q", MsgSyncResponse, resp.Type)
	}
}

// --- handleSync: with valid operations ---

func TestWSHandleSyncWithOperations(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	ops := []*crdt.Operation{
		{
			ID:       crdt.EventID{ReplicaID: "client1", Seq: 1},
			Parents:  nil,
			Type:     crdt.OpInsertNode,
			NodeType: "test",
		},
	}
	opsBytes, _ := json.Marshal(ops)
	frontierBytes, _ := json.Marshal([]crdt.EventID{})

	resp := sendAndReceive(t, conn, ClientMessage{
		Type:       MsgSync,
		RequestID:  "sync-3",
		Operations: json.RawMessage(opsBytes),
		Frontier:   json.RawMessage(frontierBytes),
	})

	if resp.Type != MsgSyncResponse {
		t.Errorf("expected type %q, got %q", MsgSyncResponse, resp.Type)
	}
	if resp.RequestID != "sync-3" {
		t.Errorf("expected requestId sync-3, got %q", resp.RequestID)
	}
}

// --- handleSync: invalid operations JSON ---

func TestWSHandleSyncInvalidOperations(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	resp := sendAndReceive(t, conn, ClientMessage{
		Type:       MsgSync,
		RequestID:  "sync-bad-ops",
		Operations: json.RawMessage([]byte(`"not an array"`)),
	})

	if resp.Type != MsgError {
		t.Errorf("expected type %q, got %q", MsgError, resp.Type)
	}
	if resp.Error != "invalid operations" {
		t.Errorf("expected 'invalid operations', got %q", resp.Error)
	}
}

// --- handleSync: invalid frontier JSON ---

func TestWSHandleSyncInvalidFrontier(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	resp := sendAndReceive(t, conn, ClientMessage{
		Type:      MsgSync,
		RequestID: "sync-bad-frontier",
		Frontier:  json.RawMessage([]byte(`"not an array"`)),
	})

	if resp.Type != MsgError {
		t.Errorf("expected type %q, got %q", MsgError, resp.Type)
	}
	if resp.Error != "invalid frontier" {
		t.Errorf("expected 'invalid frontier', got %q", resp.Error)
	}
}

// --- readLoop: connection close (normal closure) ---

func TestWSReadLoopNormalClose(t *testing.T) {
	server, conn, _ := setupWS(t)
	defer server.Close()

	// Send a normal close message; the readLoop should exit without logging an error.
	msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye")
	conn.WriteMessage(websocket.CloseMessage, msg)

	// Give server a moment to process the close
	time.Sleep(50 * time.Millisecond)

	// Connection should be effectively closed; reading should fail.
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Error("expected error reading from closed connection")
	}
}

// --- readLoop: connection close (going away) ---

func TestWSReadLoopGoingAway(t *testing.T) {
	server, conn, _ := setupWS(t)
	defer server.Close()

	msg := websocket.FormatCloseMessage(websocket.CloseGoingAway, "going away")
	conn.WriteMessage(websocket.CloseMessage, msg)

	time.Sleep(50 * time.Millisecond)

	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Error("expected error reading from closed connection")
	}
}

// --- Multiple messages on same connection ---

func TestWSMultipleMessagesOnSameConnection(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	// First call
	resp1 := sendAndReceive(t, conn, ClientMessage{
		Type:         MsgCallFunction,
		RequestID:    "multi-1",
		FunctionName: "echo",
		Args:         map[string]interface{}{"msg": "first"},
	})
	if resp1.Value != "first" {
		t.Errorf("expected 'first', got %v", resp1.Value)
	}

	// Second call
	resp2 := sendAndReceive(t, conn, ClientMessage{
		Type:         MsgCallFunction,
		RequestID:    "multi-2",
		FunctionName: "echo",
		Args:         map[string]interface{}{"msg": "second"},
	})
	if resp2.Value != "second" {
		t.Errorf("expected 'second', got %v", resp2.Value)
	}
}

// --- Multiple concurrent WebSocket clients ---

func TestWSMultipleClients(t *testing.T) {
	store := graph.NewStore("mc-test")
	validator := invariant.NewValidator()
	reg := function.NewRegistry(store, validator)
	walker := crdt.NewEGWalker("mc-test")
	reactor := gsync.NewReactor(reg, walker)

	reg.RegisterQuery("ping", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return "pong", nil
	})

	handler := NewWSHandler(reg, reactor, walker)
	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	dialer := websocket.Dialer{}

	// Connect two clients
	conn1, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("client 1 dial failed: %v", err)
	}
	defer conn1.Close()

	conn2, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("client 2 dial failed: %v", err)
	}
	defer conn2.Close()

	resp1 := sendAndReceive(t, conn1, ClientMessage{
		Type:         MsgCallFunction,
		RequestID:    "c1-req",
		FunctionName: "ping",
	})
	if resp1.Value != "pong" {
		t.Errorf("client 1: expected 'pong', got %v", resp1.Value)
	}

	resp2 := sendAndReceive(t, conn2, ClientMessage{
		Type:         MsgCallFunction,
		RequestID:    "c2-req",
		FunctionName: "ping",
	})
	if resp2.Value != "pong" {
		t.Errorf("client 2: expected 'pong', got %v", resp2.Value)
	}
}

// --- handleCall with mutation ---

func TestWSHandleCallMutation(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	resp := sendAndReceive(t, conn, ClientMessage{
		Type:         MsgCallFunction,
		RequestID:    "mut-1",
		FunctionName: "createItem",
		Args:         map[string]interface{}{"name": "widget"},
	})

	if resp.Type != MsgResult {
		t.Errorf("expected type %q, got %q", MsgResult, resp.Type)
	}
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	// createItem returns a UUID string
	if resp.Value == nil {
		t.Error("expected non-nil value from mutation")
	}
}

// --- handleSync: operations with frontier requesting missing ops ---

func TestWSHandleSyncRoundTrip(t *testing.T) {
	store := graph.NewStore("sync-test")
	validator := invariant.NewValidator()
	reg := function.NewRegistry(store, validator)
	walker := crdt.NewEGWalker("sync-test")
	reactor := gsync.NewReactor(reg, walker)

	reg.RegisterMutation("addNode", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		id, err := mctx.InsertNode("item", nil, nil)
		return id, err
	})

	handler := NewWSHandler(reg, reactor, walker)
	server := httptest.NewServer(handler)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// First, get current frontier (empty graph)
	frontierBytes, _ := json.Marshal([]crdt.EventID{})
	resp := sendAndReceive(t, conn, ClientMessage{
		Type:      MsgSync,
		RequestID: "sync-rt-1",
		Frontier:  json.RawMessage(frontierBytes),
	})

	if resp.Type != MsgSyncResponse {
		t.Errorf("expected sync_response, got %q", resp.Type)
	}

	// Add a node via mutation (which writes to the walker's event graph)
	mutResp := sendAndReceive(t, conn, ClientMessage{
		Type:         MsgCallFunction,
		RequestID:    "sync-rt-mut",
		FunctionName: "addNode",
	})
	if mutResp.Error != "" {
		t.Fatalf("mutation failed: %s", mutResp.Error)
	}

	// Now sync again with empty frontier - should get the operation back
	resp2 := sendAndReceive(t, conn, ClientMessage{
		Type:      MsgSync,
		RequestID: "sync-rt-2",
		Frontier:  json.RawMessage(frontierBytes),
	})
	if resp2.Type != MsgSyncResponse {
		t.Errorf("expected sync_response, got %q", resp2.Type)
	}
	// The response should have operations and a frontier
	if resp2.Frontier == nil {
		t.Error("expected non-nil frontier in sync response")
	}
}

// --- subscribe then unsubscribe cycle ---

func TestWSSubscribeUnsubscribeCycle(t *testing.T) {
	_, conn, cleanup := setupWS(t)
	defer cleanup()

	// Subscribe
	subResp := sendAndReceive(t, conn, ClientMessage{
		Type:           MsgSubscribe,
		RequestID:      "cycle-sub",
		FunctionName:   "echo",
		Args:           map[string]interface{}{"msg": "sub-value"},
		SubscriptionID: 0,
	})
	if subResp.Type != MsgResult {
		t.Fatalf("expected result, got %q", subResp.Type)
	}
	subID := subResp.SubscriptionID

	// Unsubscribe with the received ID
	if err := conn.WriteJSON(ClientMessage{
		Type:           MsgUnsubscribe,
		RequestID:      "cycle-unsub",
		SubscriptionID: subID,
	}); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	unsubResp := readUntilType(t, conn, MsgResult, 2*time.Second)
	if unsubResp.RequestID != "cycle-unsub" {
		t.Errorf("expected requestId cycle-unsub, got %q", unsubResp.RequestID)
	}

	// Unsubscribe again (idempotent - should still return result)
	unsubResp2 := sendAndReceive(t, conn, ClientMessage{
		Type:           MsgUnsubscribe,
		RequestID:      "cycle-unsub-2",
		SubscriptionID: subID,
	})
	if unsubResp2.Type != MsgResult {
		t.Errorf("expected result for double-unsubscribe, got %q", unsubResp2.Type)
	}
}

