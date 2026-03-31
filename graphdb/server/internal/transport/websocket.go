package transport

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	gosync "sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/kern/graphdb/internal/crdt"
	"github.com/kern/graphdb/internal/function"
	gsync "github.com/kern/graphdb/internal/sync"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WSHandler handles WebSocket connections.
type WSHandler struct {
	registry *function.Registry
	reactor  *gsync.Reactor
	walker   *crdt.EGWalker
}

// NewWSHandler creates a new WebSocket handler.
func NewWSHandler(registry *function.Registry, reactor *gsync.Reactor, walker *crdt.EGWalker) *WSHandler {
	return &WSHandler{
		registry: registry,
		reactor:  reactor,
		walker:   walker,
	}
}

// ServeHTTP upgrades the connection to WebSocket and handles messages.
func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	clientID := uuid.New().String()
	client := &wsClient{
		id:       clientID,
		conn:     conn,
		handler:  h,
		writeMu:  &gosync.Mutex{},
	}

	defer func() {
		h.reactor.UnsubscribeClient(clientID)
		conn.Close()
	}()

	client.readLoop()
}

type wsClient struct {
	id      string
	conn    *websocket.Conn
	handler *WSHandler
	writeMu *gosync.Mutex
}

func (c *wsClient) readLoop() {
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			return
		}

		var msg ClientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.sendError("", "invalid message format")
			continue
		}

		c.handleMessage(&msg)
	}
}

func (c *wsClient) handleMessage(msg *ClientMessage) {
	switch msg.Type {
	case MsgCallFunction:
		c.handleCall(msg)
	case MsgSubscribe:
		c.handleSubscribe(msg)
	case MsgUnsubscribe:
		c.handleUnsubscribe(msg)
	case MsgSync:
		c.handleSync(msg)
	default:
		c.sendError(msg.RequestID, "unknown message type")
	}
}

func (c *wsClient) handleCall(msg *ClientMessage) {
	result := c.handler.registry.Call(context.Background(), msg.FunctionName, msg.Args)

	resp := ServerMessage{
		Type:      MsgResult,
		RequestID: msg.RequestID,
	}
	if result.Error != "" {
		resp.Error = result.Error
	} else {
		resp.Value = result.Value
	}
	c.send(resp)
}

func (c *wsClient) handleSubscribe(msg *ClientMessage) {
	subID := c.handler.reactor.Subscribe(c.id, msg.FunctionName, msg.Args, func(result *function.FuncResult) {
		resp := ServerMessage{
			Type:           MsgSubscription,
			SubscriptionID: msg.SubscriptionID,
		}
		if result.Error != "" {
			resp.Error = result.Error
		} else {
			resp.Value = result.Value
		}
		c.send(resp)
	})

	// Confirm subscription
	c.send(ServerMessage{
		Type:           MsgResult,
		RequestID:      msg.RequestID,
		SubscriptionID: uint64(subID),
	})
}

func (c *wsClient) handleUnsubscribe(msg *ClientMessage) {
	c.handler.reactor.Unsubscribe(gsync.SubscriptionID(msg.SubscriptionID))
	c.send(ServerMessage{
		Type:      MsgResult,
		RequestID: msg.RequestID,
	})
}

func (c *wsClient) handleSync(msg *ClientMessage) {
	// Parse incoming operations
	if msg.Operations != nil {
		var ops []*crdt.Operation
		if err := json.Unmarshal(msg.Operations, &ops); err != nil {
			c.sendError(msg.RequestID, "invalid operations")
			return
		}
		for _, op := range ops {
			if err := c.handler.walker.ApplyRemote(op); err != nil {
				log.Printf("sync apply error: %v", err)
			}
		}
	}

	// Parse client's frontier to send back missing ops
	var clientFrontier []crdt.EventID
	if msg.Frontier != nil {
		if err := json.Unmarshal(msg.Frontier, &clientFrontier); err != nil {
			c.sendError(msg.RequestID, "invalid frontier")
			return
		}
	}

	// Send missing operations
	missingOps := c.handler.walker.Graph().EventsSince(clientFrontier)
	c.send(ServerMessage{
		Type:       MsgSyncResponse,
		RequestID:  msg.RequestID,
		Operations: missingOps,
		Frontier:   c.handler.walker.Graph().Frontier(),
	})
}

func (c *wsClient) send(msg ServerMessage) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.WriteJSON(msg); err != nil {
		log.Printf("websocket write error: %v", err)
	}
}

func (c *wsClient) sendError(requestID, errMsg string) {
	c.send(ServerMessage{
		Type:      MsgError,
		RequestID: requestID,
		Error:     errMsg,
	})
}
