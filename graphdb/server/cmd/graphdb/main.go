// GraphDB Server - A reactive graph database with CRDT support.
//
// This is the main entry point for the GraphDB server. It sets up the
// graph store, CRDT engine, function registry, reactivity system, and
// exposes both WebSocket and HTTP transports.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kern/graphdb/internal/function"
	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
	gsync "github.com/kern/graphdb/internal/sync"
	"github.com/kern/graphdb/internal/transport"
)

func main() {
	port := flag.Int("port", 8787, "port to listen on")
	replicaID := flag.String("replica-id", "", "unique replica ID (auto-generated if empty)")
	flag.Parse()

	if *replicaID == "" {
		hostname, _ := os.Hostname()
		*replicaID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}

	log.Printf("GraphDB starting (replica: %s)", *replicaID)

	// --- Initialize core components ---

	// Graph store with CRDT
	store := graph.NewStore(*replicaID)

	// Invariant validator
	validator := invariant.NewValidator()

	// Function registry
	registry := function.NewRegistry(store, validator)

	// Register built-in functions
	registerBuiltins(registry)

	// Reactive subscription engine
	reactor := gsync.NewReactor(registry, store.Walker())

	// --- Set up HTTP server ---

	mux := http.NewServeMux()

	// REST API
	httpHandler := transport.NewHTTPHandler(registry, store, validator)
	httpHandler.RegisterRoutes(mux)

	// WebSocket endpoint
	wsHandler := transport.NewWSHandler(registry, reactor, store.Walker())
	mux.Handle("/ws", wsHandler)

	// Serve
	addr := fmt.Sprintf(":%d", *port)
	server := &http.Server{
		Addr:    addr,
		Handler: corsMiddleware(mux),
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		reactor.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	log.Printf("GraphDB listening on %s", addr)
	log.Printf("  HTTP API: http://localhost:%d/api", *port)
	log.Printf("  WebSocket: ws://localhost:%d/ws", *port)
	log.Printf("  Health: http://localhost:%d/health", *port)

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// registerBuiltins registers built-in query/mutation functions that
// provide the core graph API (similar to Convex's db.* methods).
func registerBuiltins(registry *function.Registry) {
	// --- Built-in Queries ---

	registry.RegisterQuery("graphdb:getNode", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("missing required arg 'id'")
		}
		return qctx.GetNode(id)
	})

	registry.RegisterQuery("graphdb:getNodesByType", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		nodeType, ok := args["type"].(string)
		if !ok {
			return nil, fmt.Errorf("missing required arg 'type'")
		}
		return qctx.GetNodesByType(nodeType), nil
	})

	registry.RegisterQuery("graphdb:getChildren", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("missing required arg 'id'")
		}
		return qctx.GetChildren(id)
	})

	registry.RegisterQuery("graphdb:getParent", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("missing required arg 'id'")
		}
		return qctx.GetParent(id)
	})

	registry.RegisterQuery("graphdb:getSubtree", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("missing required arg 'id'")
		}
		return qctx.GetSubtree(id)
	})

	registry.RegisterQuery("graphdb:getAncestors", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("missing required arg 'id'")
		}
		return qctx.GetAncestors(id)
	})

	registry.RegisterQuery("graphdb:getOutEdges", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("missing required arg 'id'")
		}
		return qctx.GetOutEdges(id)
	})

	registry.RegisterQuery("graphdb:getInEdges", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("missing required arg 'id'")
		}
		return qctx.GetInEdges(id)
	})

	registry.RegisterQuery("graphdb:getRoots", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return qctx.GetRoots(), nil
	})

	registry.RegisterQuery("graphdb:traverse", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		id, _ := args["id"].(string)
		edgeType, _ := args["edgeType"].(string)
		direction, _ := args["direction"].(string)
		if direction == "" {
			direction = "out"
		}
		maxDepth := 10
		if md, ok := args["maxDepth"].(float64); ok {
			maxDepth = int(md)
		}
		return qctx.Traverse(id, edgeType, direction, maxDepth)
	})

	registry.RegisterQuery("graphdb:findByIndex", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		nodeType, _ := args["type"].(string)
		property, _ := args["property"].(string)
		value := args["value"]
		return qctx.FindByIndex(nodeType, property, value), nil
	})

	// --- Built-in Mutations ---

	registry.RegisterMutation("graphdb:insertNode", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		nodeType, _ := args["type"].(string)
		props, _ := args["properties"].(map[string]interface{})
		var parentID *string
		if pid, ok := args["parentId"].(string); ok {
			parentID = &pid
		}
		return mctx.InsertNode(nodeType, parentID, props)
	})

	registry.RegisterMutation("graphdb:deleteNode", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		id, _ := args["id"].(string)
		return nil, mctx.DeleteNode(id)
	})

	registry.RegisterMutation("graphdb:patchNode", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		id, _ := args["id"].(string)
		props, _ := args["properties"].(map[string]interface{})
		return nil, mctx.PatchNode(id, props)
	})

	registry.RegisterMutation("graphdb:setProperty", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		id, _ := args["id"].(string)
		key, _ := args["key"].(string)
		value := args["value"]
		return nil, mctx.SetProperty(id, key, value)
	})

	registry.RegisterMutation("graphdb:deleteProperty", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		id, _ := args["id"].(string)
		key, _ := args["key"].(string)
		return nil, mctx.DeleteProperty(id, key)
	})

	registry.RegisterMutation("graphdb:insertEdge", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		edgeType, _ := args["type"].(string)
		fromID, _ := args["from"].(string)
		toID, _ := args["to"].(string)
		props, _ := args["properties"].(map[string]interface{})
		return mctx.InsertEdge(edgeType, fromID, toID, props)
	})

	registry.RegisterMutation("graphdb:deleteEdge", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		id, _ := args["id"].(string)
		return nil, mctx.DeleteEdge(id)
	})

	registry.RegisterMutation("graphdb:moveNode", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		id, _ := args["id"].(string)
		var newParentID *string
		if pid, ok := args["parentId"].(string); ok {
			newParentID = &pid
		}
		return nil, mctx.MoveNode(id, newParentID)
	})
}
