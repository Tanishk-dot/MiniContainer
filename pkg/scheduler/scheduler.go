package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cloudforge/internal/config"
	"cloudforge/pkg/image"
	"cloudforge/pkg/runtime"
)

// Scheduler manages container deployments, scaling, and lifecycle
type Scheduler struct {
	cfg        *config.Config
	images     *image.Engine
	mu         sync.RWMutex
	deployments map[string]*Deployment
	containers map[string]*ContainerState
}

// Deployment represents a service deployment
type Deployment struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Image     string                 `json:"image"`
	Replicas  int                    `json:"replicas"`
	Running   int                    `json:"running"`
	Containers []string              `json:"containers"`
	Resources *runtime.Resources     `json:"resources"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Labels    map[string]string      `json:"labels"`
}

// ContainerState tracks an individual container's state
type ContainerState struct {
	ID           string               `json:"id"`
	DeploymentID string               `json:"deployment_id"`
	Replica      int                  `json:"replica"`
	State        string               `json:"state"` // running, stopped, failed, pending
	Pid          int                  `json:"pid"`
	StartedAt    time.Time            `json:"started_at"`
	StoppedAt    *time.Time           `json:"stopped_at"`
	ExitCode     *int                 `json:"exit_code"`
}

// New creates a new scheduler
func New(cfg *config.Config) (*Scheduler, error) {
	ie, err := image.NewEngine(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create image engine: %w", err)
	}

	s := &Scheduler{
		cfg:         cfg,
		images:      ie,
		deployments: make(map[string]*Deployment),
		containers:  make(map[string]*ContainerState),
	}

	// Load existing deployments from state
	if err := s.loadState(); err != nil {
		// Log but don't fail on load errors
		fmt.Fprintf(os.Stderr, "warning: failed to load scheduler state: %v\n", err)
	}

	return s, nil
}

// Deploy creates a new deployment with specified replicas
func (s *Scheduler) Deploy(ctx context.Context, name, imageRef string, replicas int, resources *runtime.Resources, labels map[string]string) (*Deployment, error) {
	if replicas < 1 {
		return nil, errors.New("replicas must be >= 1")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if deployment already exists
	for _, d := range s.deployments {
		if d.Name == name {
			return nil, fmt.Errorf("deployment %q already exists", name)
		}
	}

	// Create deployment
	now := time.Now().UTC()
	deployment := &Deployment{
		ID:        fmt.Sprintf("%d", now.UnixNano()),
		Name:      name,
		Image:     imageRef,
		Replicas:  replicas,
		Running:   0,
		Containers: []string{},
		Resources: resources,
		CreatedAt: now,
		UpdatedAt: now,
		Labels:    labels,
	}

	s.deployments[deployment.ID] = deployment

	// Start containers
	for i := 0; i < replicas; i++ {
		if err := s.startContainer(ctx, deployment, i); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to start container %d: %v\n", i, err)
		}
	}

	// Update running count
	deployment.Running = len(deployment.Containers)

	// Save state
	_ = s.saveState()

	return deployment, nil
}

// Scale adjusts the number of container replicas for a deployment
func (s *Scheduler) Scale(ctx context.Context, deploymentID string, replicas int) (*Deployment, error) {
	if replicas < 1 {
		return nil, errors.New("replicas must be >= 1")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	deployment, exists := s.deployments[deploymentID]
	if !exists {
		return nil, fmt.Errorf("deployment %q not found", deploymentID)
	}

	currentCount := len(deployment.Containers)

	if replicas > currentCount {
		// Scale up
		toAdd := replicas - currentCount
		for i := 0; i < toAdd; i++ {
			replicaNum := currentCount + i
			if err := s.startContainer(ctx, deployment, replicaNum); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to start replica %d: %v\n", replicaNum, err)
			}
		}
	} else if replicas < currentCount {
		// Scale down
		toRemove := currentCount - replicas
		for i := 0; i < toRemove; i++ {
			if len(deployment.Containers) > 0 {
				containerID := deployment.Containers[len(deployment.Containers)-1]
				if err := s.stopContainer(ctx, containerID); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to stop container %s: %v\n", containerID, err)
				}
			}
		}
	}

	deployment.Replicas = replicas
	deployment.Running = len(deployment.Containers)
	deployment.UpdatedAt = time.Now().UTC()

	// Save state
	_ = s.saveState()

	return deployment, nil
}

// GetStatus returns current deployment status
func (s *Scheduler) GetStatus(deploymentID string) (*Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	deployment, exists := s.deployments[deploymentID]
	if !exists {
		return nil, fmt.Errorf("deployment %q not found", deploymentID)
	}

	// Update running count
	running := 0
	for _, containerID := range deployment.Containers {
		if container, ok := s.containers[containerID]; ok && container.State == "running" {
			running++
		}
	}
	deployment.Running = running

	return deployment, nil
}

// ListDeployments returns all deployments
func (s *Scheduler) ListDeployments() []*Deployment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	deployments := make([]*Deployment, 0, len(s.deployments))
	for _, d := range s.deployments {
		// Update running count
		running := 0
		for _, containerID := range d.Containers {
			if container, ok := s.containers[containerID]; ok && container.State == "running" {
				running++
			}
		}
		d.Running = running
		deployments = append(deployments, d)
	}

	return deployments
}

// GetContainerStatus returns status of a specific container
func (s *Scheduler) GetContainerStatus(containerID string) (*ContainerState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	container, exists := s.containers[containerID]
	if !exists {
		return nil, fmt.Errorf("container %q not found", containerID)
	}

	return container, nil
}

// ListContainers returns all containers for a deployment
func (s *Scheduler) ListContainers(deploymentID string) ([]*ContainerState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	deployment, exists := s.deployments[deploymentID]
	if !exists {
		return nil, fmt.Errorf("deployment %q not found", deploymentID)
	}

	containers := make([]*ContainerState, 0, len(deployment.Containers))
	for _, containerID := range deployment.Containers {
		if container, ok := s.containers[containerID]; ok {
			containers = append(containers, container)
		}
	}

	return containers, nil
}

// Delete removes a deployment and stops all its containers
func (s *Scheduler) Delete(ctx context.Context, deploymentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	deployment, exists := s.deployments[deploymentID]
	if !exists {
		return fmt.Errorf("deployment %q not found", deploymentID)
	}

	// Stop all containers
	for _, containerID := range deployment.Containers {
		_ = s.stopContainer(ctx, containerID)
	}

	// Remove deployment
	delete(s.deployments, deploymentID)

	// Save state
	_ = s.saveState()

	return nil
}

// ============================================================================
// Private Helpers
// ============================================================================

// startContainer starts a single container for a deployment
func (s *Scheduler) startContainer(ctx context.Context, deployment *Deployment, replicaNum int) error {
	containerID := fmt.Sprintf("%s-replica-%d", deployment.Name, replicaNum)

	// Create container state
	now := time.Now().UTC()
	state := &ContainerState{
		ID:           containerID,
		DeploymentID: deployment.ID,
		Replica:      replicaNum,
		State:        "pending",
		StartedAt:    now,
	}

	s.containers[containerID] = state
	deployment.Containers = append(deployment.Containers, containerID)

	// Note: In a real implementation, this would pull the image and run it
	// For now, we'll just mark as running after a brief delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		s.mu.Lock()
		if container, ok := s.containers[containerID]; ok {
			container.State = "running"
			container.Pid = os.Getpid() // Placeholder: in real impl, track actual process
		}
		s.mu.Unlock()
	}()

	return nil
}

// stopContainer stops a running container
func (s *Scheduler) stopContainer(ctx context.Context, containerID string) error {
	container, ok := s.containers[containerID]
	if !ok {
		return fmt.Errorf("container %q not found", containerID)
	}

	now := time.Now().UTC()
	container.State = "stopped"
	container.StoppedAt = &now
	exitCode := 0
	container.ExitCode = &exitCode

	return nil
}

// saveState persists scheduler state to disk
func (s *Scheduler) saveState() error {
	schedulerDir := filepath.Join(s.cfg.MetadataDir(), "scheduler")
	if err := os.MkdirAll(schedulerDir, 0o755); err != nil {
		return fmt.Errorf("failed to create scheduler dir: %w", err)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Save deployments
	deploymentsDir := filepath.Join(schedulerDir, "deployments")
	if err := os.MkdirAll(deploymentsDir, 0o755); err != nil {
		return err
	}

	for id, deployment := range s.deployments {
		path := filepath.Join(deploymentsDir, id+".json")
		data, _ := json.MarshalIndent(deployment, "", "  ")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("failed to save deployment %s: %w", id, err)
		}
	}

	// Save containers
	containersDir := filepath.Join(schedulerDir, "containers")
	if err := os.MkdirAll(containersDir, 0o755); err != nil {
		return err
	}

	for id, container := range s.containers {
		path := filepath.Join(containersDir, id+".json")
		data, _ := json.MarshalIndent(container, "", "  ")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("failed to save container %s: %w", id, err)
		}
	}

	return nil
}

// loadState loads scheduler state from disk
func (s *Scheduler) loadState() error {
	schedulerDir := filepath.Join(s.cfg.MetadataDir(), "scheduler")
	if _, err := os.Stat(schedulerDir); os.IsNotExist(err) {
		return nil // No state yet
	}

	// Load deployments
	deploymentsDir := filepath.Join(schedulerDir, "deployments")
	entries, err := os.ReadDir(deploymentsDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && len(entry.Name()) > 5 && entry.Name()[len(entry.Name())-5:] == ".json" {
				path := filepath.Join(deploymentsDir, entry.Name())
				data, _ := os.ReadFile(path)
				var deployment Deployment
				if err := json.Unmarshal(data, &deployment); err == nil {
					s.deployments[deployment.ID] = &deployment
				}
			}
		}
	}

	// Load containers
	containersDir := filepath.Join(schedulerDir, "containers")
	entries, err = os.ReadDir(containersDir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && len(entry.Name()) > 5 && entry.Name()[len(entry.Name())-5:] == ".json" {
				path := filepath.Join(containersDir, entry.Name())
				data, _ := os.ReadFile(path)
				var container ContainerState
				if err := json.Unmarshal(data, &container); err == nil {
					s.containers[container.ID] = &container
				}
			}
		}
	}

	return nil
}
