// Package transport provides WebSocket and HTTP transport for the GraphDB server.
// The protocol is designed to be compatible with Convex's client protocol.
package transport

import "encoding/json"

// MessageType identifies the type of WebSocket message.
type MessageType string

const (
	// Client -> Server
	MsgCallFunction  MessageType = "call"
	MsgSubscribe     MessageType = "subscribe"
	MsgUnsubscribe   MessageType = "unsubscribe"
	MsgSync          MessageType = "sync" // send/receive CRDT operations
	MsgAuthenticate  MessageType = "authenticate"

	// Server -> Client
	MsgResult        MessageType = "result"
	MsgSubscription  MessageType = "subscription_update"
	MsgError         MessageType = "error"
	MsgSyncResponse  MessageType = "sync_response"
	MsgAuthenticated MessageType = "authenticated"
)

// ClientMessage is a message from client to server.
type ClientMessage struct {
	Type          MessageType            `json:"type"`
	RequestID     string                 `json:"requestId"`
	FunctionName  string                 `json:"functionName,omitempty"`
	Args          map[string]interface{} `json:"args,omitempty"`
	SubscriptionID uint64               `json:"subscriptionId,omitempty"`
	Token         string                 `json:"token,omitempty"`
	// For sync: operations and frontier
	Operations    json.RawMessage        `json:"operations,omitempty"`
	Frontier      json.RawMessage        `json:"frontier,omitempty"`
}

// ServerMessage is a message from server to client.
type ServerMessage struct {
	Type           MessageType   `json:"type"`
	RequestID      string        `json:"requestId,omitempty"`
	SubscriptionID uint64        `json:"subscriptionId,omitempty"`
	Value          interface{}   `json:"value,omitempty"`
	Error          string        `json:"error,omitempty"`
	Operations     interface{}   `json:"operations,omitempty"`
	Frontier       interface{}   `json:"frontier,omitempty"`
}
