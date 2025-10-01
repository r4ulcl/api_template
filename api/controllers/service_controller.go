package controllers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/r4ulcl/api_template/api/middlewares"
	"github.com/r4ulcl/api_template/utils/models"
)

// serviceHandlerFactory builds an http.Handler bound to a Controller instance.
type serviceHandlerFactory func(*Controller) http.Handler

var serviceHandlers = map[string]serviceHandlerFactory{
	"exampleFunction":  func(c *Controller) http.Handler { return http.HandlerFunc(c.exampleFunctionService) },
	"exampleFunction2": func(c *Controller) http.Handler { return http.HandlerFunc(c.exampleFunction2Service) },
}

// GetServiceHandler resolves a handler by name, binding it to the supplied controller instance.
func GetServiceHandler(name string, controller *Controller) (http.Handler, bool) {
	factory, ok := serviceHandlers[name]
	if !ok || controller == nil {
		return nil, false
	}
	return factory(controller), true
}

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (c *Controller) exampleFunctionService(w http.ResponseWriter, r *http.Request) {
	requesterVal := r.Context().Value(middlewares.ContextUserID)
	requester, _ := requesterVal.(string)
	if strings.TrimSpace(requester) == "" {
		respondJSON(w, http.StatusUnauthorized, models.ErrorResponse{Error: "Unauthorized"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"service":     "exampleFunction",
		"requestedBy": requester,
		"message":     "exampleFunction executed successfully (no-op)",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (c *Controller) exampleFunction2Service(w http.ResponseWriter, r *http.Request) {
	requesterVal := r.Context().Value(middlewares.ContextUserID)
	requester, _ := requesterVal.(string)
	if strings.TrimSpace(requester) == "" {
		respondJSON(w, http.StatusUnauthorized, models.ErrorResponse{Error: "Unauthorized"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"service":     "exampleFunction2",
		"requestedBy": requester,
		"message":     "exampleFunction2 responded with no side effects",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	})
}
