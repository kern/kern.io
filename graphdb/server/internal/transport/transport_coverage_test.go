package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kern/graphdb/internal/function"
	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
)

func setupFullHandler() *HTTPHandler {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := function.NewRegistry(store, validator)

	reg.RegisterQuery("q:echo", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return args["msg"], nil
	})
	reg.RegisterQuery("q:fail", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return nil, fmt.Errorf("query failed")
	})
	reg.RegisterMutation("m:create", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return "created", nil
	})
	reg.RegisterMutation("m:fail", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return nil, fmt.Errorf("mutation failed")
	})
	reg.RegisterAction("a:run", func(ctx context.Context, actx *function.ActionCtx, args map[string]interface{}) (interface{}, error) {
		return "action-done", nil
	})
	reg.RegisterAction("a:fail", func(ctx context.Context, actx *function.ActionCtx, args map[string]interface{}) (interface{}, error) {
		return nil, fmt.Errorf("action failed")
	})

	validator.Add(invariant.NewUniquenessInvariant("test-unique", invariant.UniquenessConfig{
		NodeType: "user",
		Property: "email",
	}))

	return NewHTTPHandler(reg, store, validator)
}

func TestHandleQuerySuccess(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "q:echo", Args: map[string]interface{}{"msg": "hello"}})
	req := httptest.NewRequest("POST", "/api/query", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["value"] != "hello" {
		t.Errorf("expected 'hello', got %v", resp["value"])
	}
}

func TestHandleQueryError(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "q:fail"})
	req := httptest.NewRequest("POST", "/api/query", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleQueryMethodNotAllowed(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/query", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleMutationSuccess(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "m:create"})
	req := httptest.NewRequest("POST", "/api/mutation", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleMutationError(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "m:fail"})
	req := httptest.NewRequest("POST", "/api/mutation", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleMutationNotFound(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "nonexistent"})
	req := httptest.NewRequest("POST", "/api/mutation", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleMutationWrongType(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "q:echo"}) // query, not mutation
	req := httptest.NewRequest("POST", "/api/mutation", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleMutationMethodNotAllowed(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/mutation", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleActionError(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "a:fail"})
	req := httptest.NewRequest("POST", "/api/action", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleActionNotFound(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "nonexistent"})
	req := httptest.NewRequest("POST", "/api/action", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleListFunctionsMethodNotAllowed(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/api/functions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleInvariantsWithData(t *testing.T) {
	h := setupFullHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/invariants", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var invs []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &invs)
	if len(invs) != 1 {
		t.Errorf("expected 1 invariant, got %d", len(invs))
	}
}

func TestHandleListFunctionsEmpty(t *testing.T) {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := function.NewRegistry(store, validator)
	h := NewHTTPHandler(reg, store, validator)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/functions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
