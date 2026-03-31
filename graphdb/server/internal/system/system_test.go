package system

import (
	"context"
	"testing"

	"github.com/kern/graphdb/internal/function"
	"github.com/kern/graphdb/internal/invariant"
)

func TestSystemSchemaAndFunctions(t *testing.T) {
	sys := New("test-replica")

	// Define schema with node types, invariants, and edges
	sys.Schema(func(sb *SchemaBuilder) {
		sb.Node("user", func(nb *NodeBuilder) {
			nb.String("name", true)
			nb.String("email", true)
			nb.Bool("active", false)
			nb.Indexed("email")
		})
		sb.Node("post", func(nb *NodeBuilder) {
			nb.String("title", true)
			nb.String("body", false)
		})
		sb.Edge("authored", []string{"user"}, []string{"post"})
		sb.Invariant(Unique("user", "email"))
		sb.Invariant(Acyclic("depends_on"))
	})

	// Register functions
	sys.Query("getActiveUsers", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		users := q.GetNodesByType("user")
		return users, nil
	})

	sys.Mutation("createUser", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		name, _ := args["name"].(string)
		email, _ := args["email"].(string)
		return m.InsertNode("user", nil, map[string]interface{}{
			"name":   name,
			"email":  email,
			"active": true,
		})
	})

	// Test store is properly initialized
	if sys.Store() == nil {
		t.Fatal("store should not be nil")
	}

	// Test function registration
	fn, ok := sys.Registry().Get("getActiveUsers")
	if !ok {
		t.Fatal("getActiveUsers should be registered")
	}
	if fn.Type != function.FuncQuery {
		t.Error("getActiveUsers should be a query")
	}

	fn, ok = sys.Registry().Get("createUser")
	if !ok {
		t.Fatal("createUser should be registered")
	}
	if fn.Type != function.FuncMutation {
		t.Error("createUser should be a mutation")
	}

	// Test calling createUser
	result := sys.Registry().Call(context.Background(), "createUser", map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
	})
	if result.Error != "" {
		t.Fatalf("createUser failed: %s", result.Error)
	}

	// Test calling getActiveUsers
	result = sys.Registry().Call(context.Background(), "getActiveUsers", nil)
	if result.Error != "" {
		t.Fatalf("getActiveUsers failed: %s", result.Error)
	}

	// Test invariant: duplicate email should fail
	result = sys.Registry().Call(context.Background(), "createUser", map[string]interface{}{
		"name":  "Bob",
		"email": "alice@example.com",
	})
	if result.Error == "" {
		t.Error("duplicate email should trigger invariant violation")
	}
}

func TestSystemInvariantHelpers(t *testing.T) {
	u := Unique("user", "email")
	if u.Type != invariant.InvariantUniqueness {
		t.Error("Unique should create a uniqueness invariant")
	}

	a := Acyclic("depends_on")
	if a.Type != invariant.InvariantAcyclicity {
		t.Error("Acyclic should create an acyclicity invariant")
	}

	mc := MaxCardinality("user", "follows", "out", 100)
	if mc.Type != invariant.InvariantCardinality {
		t.Error("MaxCardinality should create a cardinality invariant")
	}
}

func TestSystemDerivedStore(t *testing.T) {
	sys := New("test-derived")

	sys.Schema(func(sb *SchemaBuilder) {
		sb.Node("component", func(nb *NodeBuilder) {
			nb.String("name", true)
		})
	})

	if sys.DerivedStore() == nil {
		t.Fatal("derived store should not be nil")
	}
}
