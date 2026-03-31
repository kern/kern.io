package system

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kern/graphdb/internal/function"
	"github.com/kern/graphdb/internal/invariant"
)

func TestNewSystem(t *testing.T) {
	sys := New("r1")
	if sys.ReplicaID != "r1" {
		t.Errorf("expected 'r1', got %s", sys.ReplicaID)
	}
	if sys.Store() == nil {
		t.Error("store should not be nil")
	}
	if sys.DerivedStore() == nil {
		t.Error("derived store should not be nil")
	}
	if sys.Registry() == nil {
		t.Error("registry should not be nil")
	}
	if sys.Validator() == nil {
		t.Error("validator should not be nil")
	}
}

func TestNewWithConfig(t *testing.T) {
	config := DefaultConfig()
	config.Port = 9999
	sys := NewWithConfig("r2", config)
	if sys.config.Port != 9999 {
		t.Errorf("expected port 9999, got %d", sys.config.Port)
	}
}

func TestSchemaDefinition(t *testing.T) {
	sys := New("r1")

	sys.Schema(func(sb *SchemaBuilder) {
		sb.Node("user", func(nb *NodeBuilder) {
			nb.String("name", true)
			nb.String("email", true)
			nb.Number("age", false)
			nb.Bool("active", false)
			nb.Any("metadata", false)
			nb.Indexed("email")
			nb.AllowedChildren("post")
			nb.AllowedParents("org")
		})

		sb.Edge("follows", []string{"user"}, []string{"user"})

		sb.Invariant(Unique("user", "email"))
		sb.Invariant(Acyclic("follows"))
		sb.Invariant(MaxCardinality("user", "follows", "out", 10))
	})

	schema := sys.Store().GetSchema()
	if schema.NodeTypes["user"] == nil {
		t.Error("user node type should be defined")
	}
	if schema.NodeTypes["user"].Properties["name"] == nil {
		t.Error("name property should be defined")
	}
	if schema.NodeTypes["user"].Properties["age"] == nil {
		t.Error("age property should be defined")
	}
	if schema.NodeTypes["user"].Properties["active"] == nil {
		t.Error("active property should be defined")
	}
	if schema.NodeTypes["user"].Properties["metadata"] == nil {
		t.Error("metadata property should be defined")
	}
	if len(schema.NodeTypes["user"].AllowedChildren) != 1 {
		t.Error("should have 1 allowed child")
	}
	if len(schema.NodeTypes["user"].AllowedParents) != 1 {
		t.Error("should have 1 allowed parent")
	}
}

func TestFunctionRegistration(t *testing.T) {
	sys := New("r1")

	sys.Query("getUsers", func(ctx context.Context, q *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return q.GetNodesByType("user"), nil
	})

	sys.Mutation("createUser", func(ctx context.Context, m *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return m.InsertNode("user", nil, args)
	})

	sys.Action("sendEmail", func(ctx context.Context, a *function.ActionCtx, args map[string]interface{}) (interface{}, error) {
		return "sent", nil
	})

	fns := sys.Registry().List()
	if len(fns) != 3 {
		t.Errorf("expected 3 functions, got %d", len(fns))
	}
}

func TestStop(t *testing.T) {
	sys := New("r1")
	// Stop should be safe to call even without Start
	sys.Stop()
}

func TestRegisterSystemBuiltins(t *testing.T) {
	sys := New("r1")
	registerSystemBuiltins(sys)

	// Verify some builtins are registered
	fns := sys.Registry().List()
	found := 0
	for _, fn := range fns {
		switch fn.Name {
		case "graphdb:getNode", "graphdb:insertNode", "graphdb:stats", "graphdb:reapOrphans":
			found++
		}
	}
	if found != 4 {
		t.Errorf("expected 4 key builtins, found %d", found)
	}
}

func TestCorsMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := corsMiddleware(inner)

	// Regular request
	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS origin header should be set")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// OPTIONS request
	req = httptest.NewRequest("OPTIONS", "/api/test", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for OPTIONS, got %d", w.Code)
	}
}

func TestUniqueFn(t *testing.T) {
	inv := Unique("user", "email")
	if inv.Name != "unique-user-email" {
		t.Errorf("expected 'unique-user-email', got %s", inv.Name)
	}
	if inv.Type != invariant.InvariantUniqueness {
		t.Error("should be uniqueness type")
	}
}

func TestAcyclicFn(t *testing.T) {
	inv := Acyclic("depends_on")
	if inv.Name != "acyclic-depends_on" {
		t.Errorf("expected 'acyclic-depends_on', got %s", inv.Name)
	}
}

func TestMaxCardinalityFn(t *testing.T) {
	inv := MaxCardinality("user", "follows", "out", 5)
	if inv.Name != "max-user-follows-out" {
		t.Errorf("expected 'max-user-follows-out', got %s", inv.Name)
	}
}

func TestSchemaDerive(t *testing.T) {
	sys := New("r1")
	sys.Schema(func(sb *SchemaBuilder) {
		// Just test that DerivedType doesn't panic with nil
		// In real usage would pass actual types
	})
}
