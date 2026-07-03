package scheduler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"cloudforge/pkg/runtime"
)

// API provides HTTP endpoints for scheduler control
type API struct {
	scheduler *Scheduler
}

// NewAPI creates a new scheduler HTTP API
func NewAPI(scheduler *Scheduler) *API {
	return &API{scheduler: scheduler}
}

// Start runs the HTTP server
func (a *API) Start(addr string) error {
	mux := http.NewServeMux()

	// Deployment endpoints
	mux.HandleFunc("POST /deployments", a.createDeploymentHandler)
	mux.HandleFunc("GET /deployments", a.listDeploymentsHandler)
	mux.HandleFunc("GET /deployments/{id}", a.getDeploymentHandler)
	mux.HandleFunc("DELETE /deployments/{id}", a.deleteDeploymentHandler)

	// Scaling endpoint
	mux.HandleFunc("POST /deployments/{id}/scale", a.scaleDeploymentHandler)

	// Container endpoints
	mux.HandleFunc("GET /deployments/{id}/containers", a.listContainersHandler)
	mux.HandleFunc("GET /containers/{id}", a.getContainerHandler)

	// Health check
	mux.HandleFunc("GET /health", a.healthHandler)

	return http.ListenAndServe(addr, mux)
}

// ============================================================================
// Deployment Handlers
// ============================================================================

// DeploymentRequest represents a deployment creation request
type DeploymentRequest struct {
	Name      string                `json:"name"`
	Image     string                `json:"image"`
	Replicas  int                   `json:"replicas"`
	Resources *runtime.Resources    `json:"resources"`
	Labels    map[string]string     `json:"labels"`
}

func (a *API) createDeploymentHandler(w http.ResponseWriter, req *http.Request) {
	var deployReq DeploymentRequest
	if err := json.NewDecoder(req.Body).Decode(&deployReq); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if deployReq.Name == "" || deployReq.Image == "" {
		http.Error(w, "name and image are required", http.StatusBadRequest)
		return
	}

	if deployReq.Replicas < 1 {
		deployReq.Replicas = 1
	}

	deployment, err := a.scheduler.Deploy(req.Context(), deployReq.Name, deployReq.Image, deployReq.Replicas, deployReq.Resources, deployReq.Labels)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(deployment)
}

func (a *API) listDeploymentsHandler(w http.ResponseWriter, req *http.Request) {
	deployments := a.scheduler.ListDeployments()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deployments": deployments,
		"count":       len(deployments),
	})
}

func (a *API) getDeploymentHandler(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		http.Error(w, "deployment id required", http.StatusBadRequest)
		return
	}

	deployment, err := a.scheduler.GetStatus(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(deployment)
}

func (a *API) deleteDeploymentHandler(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		http.Error(w, "deployment id required", http.StatusBadRequest)
		return
	}

	err := a.scheduler.Delete(req.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ============================================================================
// Scaling Handler
// ============================================================================

// ScaleRequest represents a scale operation request
type ScaleRequest struct {
	Replicas int `json:"replicas"`
}

func (a *API) scaleDeploymentHandler(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		http.Error(w, "deployment id required", http.StatusBadRequest)
		return
	}

	var scaleReq ScaleRequest
	if err := json.NewDecoder(req.Body).Decode(&scaleReq); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	deployment, err := a.scheduler.Scale(req.Context(), id, scaleReq.Replicas)
	if err != nil {
		if errors.Is(err, errors.New("replicas must be >= 1")) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, err.Error(), http.StatusNotFound)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(deployment)
}

// ============================================================================
// Container Handlers
// ============================================================================

func (a *API) listContainersHandler(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		http.Error(w, "deployment id required", http.StatusBadRequest)
		return
	}

	containers, err := a.scheduler.ListContainers(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"containers": containers,
		"count":      len(containers),
	})
}

func (a *API) getContainerHandler(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	if id == "" {
		http.Error(w, "container id required", http.StatusBadRequest)
		return
	}

	container, err := a.scheduler.GetContainerStatus(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(container)
}

// ============================================================================
// Health Check
// ============================================================================

func (a *API) healthHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"timestamp": map[string]interface{}{},
	})
}
