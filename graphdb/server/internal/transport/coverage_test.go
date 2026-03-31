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

func setupHandler() *HTTPHandler {
	store := graph.NewStore("r1")
	validator := invariant.NewValidator()
	reg := function.NewRegistry(store, validator)

	// Register test functions
	reg.RegisterQuery("test:echo", func(ctx context.Context, qctx *function.QueryCtx, args map[string]interface{}) (interface{}, error) {
		return args["msg"], nil
	})
	reg.RegisterMutation("test:create", func(ctx context.Context, mctx *function.MutationCtx, args map[string]interface{}) (interface{}, error) {
		return "created", nil
	})
	reg.RegisterAction("test:action", func(ctx context.Context, actx *function.ActionCtx, args map[string]interface{}) (interface{}, error) {
		return "action-done", nil
	})

	return NewHTTPHandler(reg, store, validator)
}

func TestHandleAction(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "test:action", Args: map[string]interface{}{}})
	req := httptest.NewRequest("POST", "/api/action", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["value"] != "action-done" {
		t.Errorf("expected 'action-done', got %v", resp["value"])
	}
}

func TestHandleActionNotMutation(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "test:echo"})
	req := httptest.NewRequest("POST", "/api/action", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for wrong function type, got %d", w.Code)
	}
}

func TestHandleActionMethodNotAllowed(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/action", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleSchema(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/schema", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected JSON content type")
	}
}

func TestHandleSchemaMethodNotAllowed(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/api/schema", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleInvariants(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/invariants", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleInvariantsMethodNotAllowed(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/api/invariants", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleListFunctionsWithFunctions(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/functions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var funcs []map[string]string
	json.Unmarshal(w.Body.Bytes(), &funcs)
	if len(funcs) != 3 {
		t.Errorf("expected 3 functions, got %d", len(funcs))
	}
}

func TestHandleQueryInvalidJSON(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/api/query", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleQueryFunctionNotFound(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "nonexistent"})
	req := httptest.NewRequest("POST", "/api/query", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleQueryWrongType(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(callRequest{Name: "test:create"}) // mutation, not query
	req := httptest.NewRequest("POST", "/api/query", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleMutationInvalidJSON(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/api/mutation", bytes.NewReader([]byte("{bad")))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleActionInvalidJSON(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/api/action", bytes.NewReader([]byte("{bad")))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestWriteJSONPublic(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSONPublic(w, http.StatusCreated, map[string]string{"key": "value"})

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result)
	}
}

func TestHandleHealth(t *testing.T) {
	h := setupHandler()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
