// Package system provides a unified entry point for configuring a GraphDB
// instance. It ties together the graph store, derived layer, function
// registry, invariant system, reactive subscriptions, gossip relay, and
// cluster management into a single cohesive system.
//
// Usage:
//
//	sys := system.New("replica-1")
//	sys.Schema(func(s *system.SchemaBuilder) {
//	    s.Node("user", func(n *system.NodeBuilder) {
//	        n.String("name", true)
//	        n.String("email", true)
//	        n.Bool("active", false)
//	    })
//	    s.Edge("follows", []string{"user"}, []string{"user"})
//	    s.Invariant("unique-email", system.Unique("user", "email"))
//	    s.Derive("view-user", func(d *system.DerivationBuilder) {
//	        d.Map("user", "view:user", func(...) {...})
//	    })
//	})
//	sys.Query("getActiveUsers", func(ctx, q, args) { ... })
//	sys.Mutation("createUser", func(ctx, m, args) { ... })
//	sys.Start(":8787")
package system

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kern/graphdb/internal/cluster"
	"github.com/kern/graphdb/internal/derived"
	"github.com/kern/graphdb/internal/function"
	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
	gsync "github.com/kern/graphdb/internal/sync"
	"github.com/kern/graphdb/internal/sync/gossip"
	"github.com/kern/graphdb/internal/transport"
)

// System is the unified entry point for a GraphDB instance.
type System struct {
	ReplicaID string

	store        *graph.Store
	derivedStore *derived.Store
	registry     *function.Registry
	validator    *invariant.Validator
	incValidator *invariant.IncrementalValidator
	reactor      *gsync.Reactor
	gossipRelay  *gossip.Relay
	shardMgr     *cluster.ShardManager

	config Config
}

// Config holds system configuration.
type Config struct {
	Port         int
	ShardCount   int
	ReplicaCount int
	GossipConfig gossip.RelayConfig
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Port:         8787,
		ShardCount:   64,
		ReplicaCount: 2,
		GossipConfig: gossip.DefaultRelayConfig(),
	}
}

// New creates a new GraphDB system.
func New(replicaID string) *System {
	return NewWithConfig(replicaID, DefaultConfig())
}

// NewWithConfig creates a new GraphDB system with custom configuration.
func NewWithConfig(replicaID string, config Config) *System {
	store := graph.NewStore(replicaID)
	validator := invariant.NewValidator()
	incValidator := invariant.NewIncrementalValidator()
	registry := function.NewRegistry(store, validator)

	return &System{
		ReplicaID:    replicaID,
		store:        store,
		derivedStore: derived.NewStore(store),
		registry:     registry,
		validator:    validator,
		incValidator: incValidator,
		config:       config,
	}
}

// Store returns the underlying graph store.
func (s *System) Store() *graph.Store { return s.store }

// DerivedStore returns the derived graph store.
func (s *System) DerivedStore() *derived.Store { return s.derivedStore }

// Registry returns the function registry.
func (s *System) Registry() *function.Registry { return s.registry }

// Validator returns the invariant validator.
func (s *System) Validator() *invariant.Validator { return s.validator }

// --- Schema Definition ---

// SchemaBuilder provides a fluent API for defining the graph schema.
type SchemaBuilder struct {
	sys *System
}

// NodeBuilder provides a fluent API for defining a node type.
type NodeBuilder struct {
	def *graph.NodeTypeDef
}

// Schema configures the graph schema, invariants, and derivations.
func (s *System) Schema(fn func(sb *SchemaBuilder)) {
	sb := &SchemaBuilder{sys: s}
	fn(sb)
}

// Node defines a node type.
func (sb *SchemaBuilder) Node(name string, fn func(nb *NodeBuilder)) {
	nb := &NodeBuilder{
		def: &graph.NodeTypeDef{
			Name:       name,
			Properties: make(map[string]*graph.PropertyDef),
		},
	}
	fn(nb)
	schema := sb.sys.store.GetSchema()
	schema.DefineNode(nb.def)
}

// String adds a string property to a node type.
func (nb *NodeBuilder) String(name string, required bool) {
	nb.def.Properties[name] = &graph.PropertyDef{
		Name:     name,
		Type:     graph.PropString,
		Required: required,
	}
}

// Number adds a number property.
func (nb *NodeBuilder) Number(name string, required bool) {
	nb.def.Properties[name] = &graph.PropertyDef{
		Name:     name,
		Type:     graph.PropNumber,
		Required: required,
	}
}

// Bool adds a boolean property.
func (nb *NodeBuilder) Bool(name string, required bool) {
	nb.def.Properties[name] = &graph.PropertyDef{
		Name:     name,
		Type:     graph.PropBool,
		Required: required,
	}
}

// Any adds an any-typed property.
func (nb *NodeBuilder) Any(name string, required bool) {
	nb.def.Properties[name] = &graph.PropertyDef{
		Name:     name,
		Type:     graph.PropAny,
		Required: required,
	}
}

// Indexed marks a property as indexed.
func (nb *NodeBuilder) Indexed(name string) {
	if p, ok := nb.def.Properties[name]; ok {
		p.Indexed = true
	}
}

// AllowedChildren sets the allowed child node types.
func (nb *NodeBuilder) AllowedChildren(types ...string) {
	nb.def.AllowedChildren = types
}

// AllowedParents sets the allowed parent node types.
func (nb *NodeBuilder) AllowedParents(types ...string) {
	nb.def.AllowedParents = types
}

// Edge defines an edge type.
func (sb *SchemaBuilder) Edge(name string, fromTypes, toTypes []string) {
	schema := sb.sys.store.GetSchema()
	schema.DefineEdge(&graph.EdgeTypeDef{
		Name:      name,
		FromTypes: fromTypes,
		ToTypes:   toTypes,
	})
}

// Invariant registers an invariant.
func (sb *SchemaBuilder) Invariant(inv *invariant.Invariant) {
	sb.sys.validator.Add(inv)
	sb.sys.incValidator.Add(inv)
}

// Unique creates a uniqueness invariant.
func Unique(nodeType, property string) *invariant.Invariant {
	return invariant.NewUniquenessInvariant(
		fmt.Sprintf("unique-%s-%s", nodeType, property),
		invariant.UniquenessConfig{
			NodeType: nodeType,
			Property: property,
		},
	)
}

// Acyclic creates an acyclicity invariant on an edge type.
func Acyclic(edgeType string) *invariant.Invariant {
	return invariant.NewAcyclicityInvariant(
		fmt.Sprintf("acyclic-%s", edgeType),
		invariant.AcyclicityConfig{
			EdgeType: edgeType,
		},
	)
}

// MaxCardinality creates a cardinality invariant.
func MaxCardinality(nodeType, edgeType, direction string, max int) *invariant.Invariant {
	return invariant.NewCardinalityInvariant(
		fmt.Sprintf("max-%s-%s-%s", nodeType, edgeType, direction),
		invariant.CardinalityConfig{
			NodeType:  nodeType,
			EdgeType:  edgeType,
			Direction: direction,
			Max:       &max,
		},
	)
}

// Derive registers a derivation pipeline.
func (sb *SchemaBuilder) Derive(pipeline *derived.Pipeline) {
	sb.sys.derivedStore.RegisterPipeline(pipeline)
}

// DerivedType registers a derived node type.
func (sb *SchemaBuilder) DerivedType(nodeType *derived.DerivedNodeType) {
	sb.sys.derivedStore.RegisterType(nodeType)
}

// --- Function Registration ---

// Query registers a query function.
func (s *System) Query(name string, handler function.QueryHandler) {
	s.registry.RegisterQuery(name, handler)
}

// Mutation registers a mutation function.
func (s *System) Mutation(name string, handler function.MutationHandler) {
	s.registry.RegisterMutation(name, handler)
}

// Action registers an action function.
func (s *System) Action(name string, handler function.ActionHandler) {
	s.registry.RegisterAction(name, handler)
}

// --- Lifecycle ---

// Start initializes all subsystems and starts the HTTP server.
func (s *System) Start(addr string) error {
	if addr == "" {
		addr = fmt.Sprintf(":%d", s.config.Port)
	}

	log.Printf("GraphDB starting (replica: %s)", s.ReplicaID)

	// Initialize reactor
	s.reactor = gsync.NewReactor(s.registry, s.store.Walker())

	// Initialize gossip relay
	s.gossipRelay = gossip.NewRelay(s.store.Walker().Graph(), s.config.GossipConfig)

	// Initialize shard manager
	s.shardMgr = cluster.NewShardManager(s.ReplicaID, s.config.ShardCount, s.config.ReplicaCount)
	s.shardMgr.Init()
	s.shardMgr.AddNode(&cluster.ClusterNode{
		ID:       s.ReplicaID,
		Address:  addr,
		JoinedAt: time.Now(),
		Status:   cluster.NodeHealthy,
		Weight:   1,
	})

	// Register built-in functions
	registerSystemBuiltins(s)

	// Set up HTTP
	mux := http.NewServeMux()

	httpHandler := transport.NewHTTPHandler(s.registry, s.store, s.validator)
	httpHandler.RegisterRoutes(mux)

	wsHandler := transport.NewWSHandler(s.registry, s.reactor, s.store.Walker())
	mux.Handle("/ws", wsHandler)

	// Cluster endpoints
	mux.HandleFunc("/api/cluster/peers", func(w http.ResponseWriter, r *http.Request) {
		transport.WriteJSONPublic(w, http.StatusOK, s.gossipRelay.GetPeers())
	})
	mux.HandleFunc("/api/cluster/shards", func(w http.ResponseWriter, r *http.Request) {
		transport.WriteJSONPublic(w, http.StatusOK, s.shardMgr.GetShardStats())
	})

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
		s.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	log.Printf("GraphDB listening on %s", addr)
	return server.ListenAndServe()
}

// Stop cleanly shuts down all subsystems.
func (s *System) Stop() {
	if s.reactor != nil {
		s.reactor.Stop()
	}
	if s.derivedStore != nil {
		s.derivedStore.Stop()
	}
}

// registerSystemBuiltins registers all built-in query/mutation functions.
func registerSystemBuiltins(s *System) {
	reg := s.registry
	store := s.store

	// Queries
	reg.RegisterQuery("graphdb:getNode", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetNode(args["id"].(string))
	})
	reg.RegisterQuery("graphdb:getNodesByType", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetNodesByType(args["type"].(string)), nil
	})
	reg.RegisterQuery("graphdb:getChildren", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetChildren(args["id"].(string))
	})
	reg.RegisterQuery("graphdb:getOrderedChildren", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetOrderedChildren(args["id"].(string))
	})
	reg.RegisterQuery("graphdb:getParent", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetParent(args["id"].(string))
	})
	reg.RegisterQuery("graphdb:getRoots", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetRoots(), nil
	})
	reg.RegisterQuery("graphdb:getSubtree", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetSubtree(args["id"].(string))
	})
	reg.RegisterQuery("graphdb:getAncestors", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetAncestors(args["id"].(string))
	})
	reg.RegisterQuery("graphdb:getOutEdges", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetOutEdges(args["id"].(string))
	})
	reg.RegisterQuery("graphdb:getInEdges", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetInEdges(args["id"].(string))
	})
	reg.RegisterQuery("graphdb:stats", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.Stats(), nil
	})
	reg.RegisterQuery("graphdb:getDeletedNodes", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetDeletedNodes(), nil
	})

	// Mutations
	reg.RegisterMutation("graphdb:insertNode", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		nodeType, _ := args["type"].(string)
		props, _ := args["properties"].(map[string]interface{})
		var pid *string
		if p, ok := args["parentId"].(string); ok {
			pid = &p
		}
		return m.InsertNode(nodeType, pid, props)
	})
	reg.RegisterMutation("graphdb:deleteNode", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, m.DeleteNode(args["id"].(string))
	})
	reg.RegisterMutation("graphdb:softDeleteNode", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, m.SoftDeleteNode(args["id"].(string))
	})
	reg.RegisterMutation("graphdb:cascadeDeleteNode", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, m.CascadeDeleteNode(args["id"].(string))
	})
	reg.RegisterMutation("graphdb:restoreNode", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, m.RestoreNode(args["id"].(string))
	})
	reg.RegisterMutation("graphdb:patchNode", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		props, _ := args["properties"].(map[string]interface{})
		return nil, m.PatchNode(args["id"].(string), props)
	})
	reg.RegisterMutation("graphdb:setProperty", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, m.SetProperty(args["id"].(string), args["key"].(string), args["value"])
	})
	reg.RegisterMutation("graphdb:deleteProperty", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, m.DeleteProperty(args["id"].(string), args["key"].(string))
	})
	reg.RegisterMutation("graphdb:insertEdge", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		edgeType, _ := args["type"].(string)
		from, _ := args["from"].(string)
		to, _ := args["to"].(string)
		props, _ := args["properties"].(map[string]interface{})
		return m.InsertEdge(edgeType, from, to, props)
	})
	reg.RegisterMutation("graphdb:deleteEdge", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, m.DeleteEdge(args["id"].(string))
	})
	reg.RegisterMutation("graphdb:moveNode", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		var pid *string
		if p, ok := args["parentId"].(string); ok {
			pid = &p
		}
		return nil, m.MoveNode(args["id"].(string), pid)
	})
	reg.RegisterMutation("graphdb:reapOrphans", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		reaped, err := store.ReapOrphans()
		if err != nil {
			return nil, err
		}
		ids := make([]string, len(reaped))
		for i, id := range reaped {
			ids[i] = id.String()
		}
		return map[string]interface{}{"reapedCount": len(reaped), "reapedIds": ids}, nil
	})
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
