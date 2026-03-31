package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kern/graphdb/internal/function"
	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
)

func setupHTTP() (*HTTPHandler, *http.ServeMux) {
	store := graph.NewStore("test-replica")
	validator := invariant.NewValidator()
	reg := function.NewRegistry(store, validator)

	reg.RegisterQuery("listUsers", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return qctx.GetNodesByType("user"), nil
	})

	reg.RegisterMutation("createUser", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		id, err := mctx.InsertNode("user", nil, map[string]interface{}{"name": args["name"]})
		return id, err
	})

	handler := NewHTTPHandler(reg, store, validator)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return handler, mux
}

func TestHealthEndpoint(t *testing.T) {
	_, mux := setupHTTP()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}

func TestMutationEndpoint(t *testing.T) {
	_, mux := setupHTTP()

	body, _ := json.Marshal(map[string]interface{}{
		"name": "createUser",
		"args": map[string]interface{}{"name": "Alice"},
	})

	req := httptest.NewRequest("POST", "/api/mutation", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestQueryEndpoint(t *testing.T) {
	_, mux := setupHTTP()

	// Create a user first
	body, _ := json.Marshal(map[string]interface{}{
		"name": "createUser",
		"args": map[string]interface{}{"name": "Alice"},
	})
	req := httptest.NewRequest("POST", "/api/mutation", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Query users
	body, _ = json.Marshal(map[string]interface{}{
		"name": "listUsers",
	})
	req = httptest.NewRequest("POST", "/api/query", bytes.NewReader(body))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListFunctionsEndpoint(t *testing.T) {
	_, mux := setupHTTP()

	req := httptest.NewRequest("GET", "/api/functions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestWrongHTTPMethod(t *testing.T) {
	_, mux := setupHTTP()

	req := httptest.NewRequest("GET", "/api/query", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 405 {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestCallWrongFunctionType(t *testing.T) {
	_, mux := setupHTTP()

	body, _ := json.Marshal(map[string]interface{}{
		"name": "listUsers",
	})
	// Try to call a query through the mutation endpoint
	req := httptest.NewRequest("POST", "/api/mutation", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
