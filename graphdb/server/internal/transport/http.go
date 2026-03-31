package transport

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/kern/graphdb/internal/function"
	"github.com/kern/graphdb/internal/graph"
	"github.com/kern/graphdb/internal/invariant"
)

// HTTPHandler provides REST-style HTTP endpoints for the graph database.
type HTTPHandler struct {
	registry  *function.Registry
	store     *graph.Store
	validator *invariant.Validator
}

// NewHTTPHandler creates a new HTTP handler.
func NewHTTPHandler(registry *function.Registry, store *graph.Store, validator *invariant.Validator) *HTTPHandler {
	return &HTTPHandler{
		registry:  registry,
		store:     store,
		validator: validator,
	}
}

// RegisterRoutes registers all HTTP routes on the given mux.
func (h *HTTPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/query", h.handleQuery)
	mux.HandleFunc("/api/mutation", h.handleMutation)
	mux.HandleFunc("/api/action", h.handleAction)
	mux.HandleFunc("/api/functions", h.handleListFunctions)
	mux.HandleFunc("/api/schema", h.handleSchema)
	mux.HandleFunc("/api/invariants", h.handleInvariants)
	mux.HandleFunc("/health", h.handleHealth)
}

type callRequest struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

func (h *HTTPHandler) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req callRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Verify the function is a query
	fn, ok := h.registry.Get(req.Name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "function not found"})
		return
	}
	if fn.Type != function.FuncQuery {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "function is not a query"})
		return
	}

	result := h.registry.Call(context.Background(), req.Name, req.Args)
	if result.Error != "" {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error":    result.Error,
			"duration": result.Duration.String(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"value":    result.Value,
		"duration": result.Duration.String(),
	})
}

func (h *HTTPHandler) handleMutation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req callRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	fn, ok := h.registry.Get(req.Name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "function not found"})
		return
	}
	if fn.Type != function.FuncMutation {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "function is not a mutation"})
		return
	}

	result := h.registry.Call(context.Background(), req.Name, req.Args)
	if result.Error != "" {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error":    result.Error,
			"duration": result.Duration.String(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"value":    result.Value,
		"duration": result.Duration.String(),
	})
}

func (h *HTTPHandler) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req callRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	fn, ok := h.registry.Get(req.Name)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "function not found"})
		return
	}
	if fn.Type != function.FuncAction {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "function is not an action"})
		return
	}

	result := h.registry.Call(context.Background(), req.Name, req.Args)
	if result.Error != "" {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error":    result.Error,
			"duration": result.Duration.String(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"value":    result.Value,
		"duration": result.Duration.String(),
	})
}

func (h *HTTPHandler) handleListFunctions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fns := h.registry.List()
	type funcInfo struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	var result []funcInfo
	for _, fn := range fns {
		result = append(result, funcInfo{Name: fn.Name, Type: string(fn.Type)})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *HTTPHandler) handleSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, h.store.GetSchema())
}

func (h *HTTPHandler) handleInvariants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	invs := h.validator.List()
	type invInfo struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Type        string `json:"type"`
		Description string `json:"description"`
	}
	var result []invInfo
	for _, inv := range invs {
		result = append(result, invInfo{
			ID:          inv.ID,
			Name:        inv.Name,
			Type:        string(inv.Type),
			Description: inv.Description,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *HTTPHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// WriteJSONPublic is the exported version of writeJSON for use by main.
func WriteJSONPublic(w http.ResponseWriter, status int, v interface{}) {
	writeJSON(w, status, v)
}
