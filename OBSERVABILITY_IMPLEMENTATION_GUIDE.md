# IMPLEMENTATION GUIDE: Integrating Observability

This guide provides step-by-step instructions for integrating the new observability framework into existing CloudForge services.

## Overview

The observability package provides:
- **Structured Logging**: JSON and text formats, log levels
- **Metrics Collection**: Counters, gauges, histograms, timers
- **Distributed Tracing**: W3C TraceContext support
- **HTTP Middleware**: Automatic request instrumentation

## Part 1: Registry Service Integration

### Step 1: Update registry.go imports and initialization

**File**: `pkg/registry/registry.go`

Add to imports:
```go
import (
    // ... existing imports ...
    "cloudforge/pkg/observability"
)
```

Add to Registry struct:
```go
type Registry struct {
    cfg    *config.Config
    images *image.Engine
    store  *storage.Engine
    
    // NEW: Observability
    logger  *observability.Logger
    metrics *observability.MetricsRegistry
    
    // Existing fields...
    uploads map[string]*uploadSession
    umutex  sync.Mutex
}
```

Update New() function:
```go
func New(cfg *config.Config) (*Registry, error) {
    ie, err := image.NewEngine(cfg)
    if err != nil {
        return nil, err
    }
    
    se, err := storage.NewEngine(cfg)
    if err != nil {
        return nil, err
    }
    
    r := &Registry{
        cfg:    cfg,
        images: ie,
        store:  se,
        
        // NEW: Initialize observability
        logger:  observability.GlobalLogger.Get("registry"),
        metrics: observability.GlobalMetrics,
        
        uploads: make(map[string]*uploadSession),
    }
    
    r.logger.Info("registry initialized", map[string]interface{}{
        "storage_path": cfg.StoragePath(),
    })
    
    return r, nil
}
```

### Step 2: Add observability to blob handlers

**File**: `pkg/registry/registry.go`

Update `putBlobHandler()`:

```go
func (r *Registry) putBlobHandler(w http.ResponseWriter, req *http.Request) {
    // Extract trace context
    tc := observability.TraceFromContext(req.Context())
    operation := observability.NewOperation("blob_put", tc)
    operation.Start()
    defer func() {
        if operation.error != nil {
            operation.Error(operation.error)
        } else {
            operation.End()
        }
        r.logger.Info("blob_put completed", operation.ToFields())
    }()
    
    // Existing logic...
    repo := req.PathValue("repo")
    digest := req.PathValue("digest")
    
    operation.SetAttribute("repo", repo)
    operation.SetAttribute("digest", digest)
    
    // Check if already exists
    exists, err := r.store.Exists(req.Context(), digest)
    if err != nil {
        r.logger.Error("failed to check blob existence", map[string]interface{}{
            "digest": digest,
            "error":  err.Error(),
            "trace_id": tc.TraceID,
        })
        
        // Record metric
        r.metrics.RegisterCounter(observability.HTTPErrorsTotal).Inc()
        operation.Error(err)
        
        w.WriteHeader(http.StatusInternalServerError)
        return
    }
    
    if exists {
        w.WriteHeader(http.StatusConflict)
        return
    }
    
    // Read and store blob (existing logic)
    body, err := io.ReadAll(req.Body)
    if err != nil {
        operation.Error(err)
        w.WriteHeader(http.StatusBadRequest)
        return
    }
    
    // Measure operation
    timer := observability.NewTimer("blob_put_duration")
    if err := r.store.Put(req.Context(), digest, body); err != nil {
        operation.Error(err)
        w.WriteHeader(http.StatusInternalServerError)
        return
    }
    duration := timer.Stop()
    
    operation.SetAttribute("bytes_written", len(body))
    operation.SetAttribute("duration_ms", duration)
    
    // Record metrics
    r.metrics.RegisterCounter(observability.BlobPushTotal).Inc()
    r.metrics.RegisterGauge(observability.StorageSize).Set(
        calculateTotalStorageSize(r.store))
    
    w.WriteHeader(http.StatusCreated)
}
```

Update `getBlobHandler()`:

```go
func (r *Registry) getBlobHandler(w http.ResponseWriter, req *http.Request) {
    tc := observability.TraceFromContext(req.Context())
    operation := observability.NewOperation("blob_get", tc)
    operation.Start()
    
    repo := req.PathValue("repo")
    digest := req.PathValue("digest")
    
    operation.SetAttribute("repo", repo)
    operation.SetAttribute("digest", digest)
    
    // Measure retrieval
    timer := observability.NewTimer("blob_get_duration")
    blob, err := r.store.Get(req.Context(), digest)
    if err != nil {
        r.logger.Warn("blob not found", map[string]interface{}{
            "digest":   digest,
            "trace_id": tc.TraceID,
        })
        operation.Error(err)
        w.WriteHeader(http.StatusNotFound)
        return
    }
    duration := timer.Stop()
    
    operation.SetAttribute("bytes_read", len(blob))
    operation.SetAttribute("duration_ms", duration)
    operation.End()
    
    // Record metrics
    r.metrics.RegisterCounter(observability.BlobPullTotal).Inc()
    
    r.logger.Info("blob retrieved", operation.ToFields())
    
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Length", fmt.Sprintf("%d", len(blob)))
    w.WriteHeader(http.StatusOK)
    w.Write(blob)
}
```

### Step 3: Create registry HTTP server with middleware

**File**: `cmd/registry-server/main.go`

```go
package main

import (
    "flag"
    "fmt"
    "log"
    "net/http"
    
    "cloudforge/internal/config"
    "cloudforge/pkg/observability"
    "cloudforge/pkg/registry"
)

func main() {
    addr := flag.String("addr", ":5000", "listen address")
    debug := flag.Bool("debug", false, "enable debug logging")
    
    flag.Parse()
    
    // Configure observability
    logLevel := observability.LogLevelInfo
    if *debug {
        logLevel = observability.LogLevelDebug
    }
    observability.GlobalLogger.SetLevel(logLevel)
    
    logger := observability.GlobalLogger.Get("registry-server")
    
    // Initialize config
    cfg := &config.Config{}
    if err := cfg.EnsureDirs(); err != nil {
        logger.Fatal("failed to initialize config", map[string]interface{}{
            "error": err.Error(),
        })
    }
    
    // Create registry
    reg, err := registry.New(cfg)
    if err != nil {
        logger.Fatal("failed to create registry", map[string]interface{}{
            "error": err.Error(),
        })
    }
    
    // Create router with middleware
    mux := http.NewServeMux()
    
    // Add observability middleware
    middleware := observability.NewHTTPMiddleware(logger, observability.GlobalMetrics)
    
    // Add health and metrics endpoints
    mux.HandleFunc("GET /health", observability.HealthCheckHandler)
    mux.HandleFunc("GET /metrics", observability.MetricsHandler(observability.GlobalMetrics))
    mux.HandleFunc("GET /debug", observability.DebugHandler)
    
    // Register registry handlers with middleware
    mux.Handle("GET /v2/", middleware.Middleware(http.HandlerFunc(reg.checkAPIHandler)))
    // ... other registry endpoints wrapped with middleware
    
    // Create HTTP server
    server := &http.Server{
        Addr:           *addr,
        Handler:        mux,
        MaxHeaderBytes: 1 << 20, // 1MB
        ReadTimeout:    30,
        WriteTimeout:   30,
    }
    
    logger.Info("starting registry server", map[string]interface{}{
        "addr": *addr,
    })
    
    if err := server.ListenAndServe(); err != nil {
        logger.Fatal("server error", map[string]interface{}{
            "error": err.Error(),
        })
    }
}
```

## Part 2: Scheduler Service Integration

### Step 1: Update scheduler.go

**File**: `pkg/scheduler/scheduler.go`

Add to Scheduler struct:
```go
type Scheduler struct {
    cfg   *config.Config
    rt    *runtime.Runtime
    
    // NEW: Observability
    logger  *observability.Logger
    metrics *observability.MetricsRegistry
    
    // Existing fields...
    mu          sync.RWMutex
    deployments map[string]*Deployment
    containers  map[string]*ContainerState
}
```

Update New() function:
```go
func New(cfg *config.Config) (*Scheduler, error) {
    rt, err := runtime.New(cfg)
    if err != nil {
        return nil, err
    }
    
    s := &Scheduler{
        cfg: cfg,
        rt:  rt,
        
        // NEW: Initialize observability
        logger:  observability.GlobalLogger.Get("scheduler"),
        metrics: observability.GlobalMetrics,
        
        deployments: make(map[string]*Deployment),
        containers:  make(map[string]*ContainerState),
    }
    
    s.logger.Info("scheduler initialized", nil)
    
    if err := s.loadState(); err != nil {
        s.logger.Warn("failed to load previous state", map[string]interface{}{
            "error": err.Error(),
        })
    }
    
    return s, nil
}
```

Update Deploy() with observability:
```go
func (s *Scheduler) Deploy(ctx context.Context, name, image string, replicas int, 
    resources *runtime.Resources, labels map[string]string) (*Deployment, error) {
    
    tc := observability.TraceFromContext(ctx)
    operation := observability.NewOperation("deploy", tc)
    operation.Start()
    
    operation.SetAttribute("deployment_name", name)
    operation.SetAttribute("image", image)
    operation.SetAttribute("replicas", replicas)
    
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Check for duplicates
    for _, d := range s.deployments {
        if d.Name == name {
            err := fmt.Errorf("deployment %q already exists", name)
            operation.Error(err)
            s.logger.Warn("deploy failed - duplicate", operation.ToFields())
            s.metrics.RegisterCounter(observability.HTTPErrorsTotal).Inc()
            return nil, err
        }
    }
    
    // Create deployment
    timer := observability.NewTimer("deploy_duration")
    
    deployment := &Deployment{
        ID:        generateID(),
        Name:      name,
        Image:     image,
        Replicas:  replicas,
        Running:   0,
        Containers: []string{},
        Resources: resources,
        Labels:    labels,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }
    
    // Start container replicas
    for i := 0; i < replicas; i++ {
        containerName := fmt.Sprintf("%s-replica-%d", name, i)
        
        container, err := s.startContainer(ctx, containerName, image, i, deployment)
        if err != nil {
            s.logger.Error("failed to start container", map[string]interface{}{
                "container": containerName,
                "error":     err.Error(),
                "trace_id":  tc.TraceID,
            })
            continue
        }
        
        deployment.Containers = append(deployment.Containers, container.ID)
        deployment.Running++
        s.containers[container.ID] = container
        
        // Record metric
        s.metrics.RegisterCounter(observability.ContainerStarts).Inc()
    }
    
    // Save state
    s.deployments[deployment.ID] = deployment
    s.saveState()
    
    duration := timer.Stop()
    operation.SetAttribute("duration_ms", duration)
    operation.SetAttribute("containers_started", deployment.Running)
    operation.End()
    
    // Record metrics
    s.metrics.RegisterCounter(observability.DeploymentTotal).Inc()
    s.metrics.RegisterGauge(observability.ContainerRunning).Set(int64(deployment.Running))
    
    s.logger.Info("deployment created", operation.ToFields())
    
    return deployment, nil
}
```

### Step 2: Update API handler

**File**: `pkg/scheduler/api.go`

```go
package scheduler

import (
    "encoding/json"
    "fmt"
    "net/http"
    
    "cloudforge/pkg/observability"
)

// API provides HTTP endpoints for the scheduler
type API struct {
    scheduler *Scheduler
    logger    *observability.Logger
    middleware *observability.HTTPMiddleware
}

// NewAPI creates a new scheduler API
func NewAPI(scheduler *Scheduler) *API {
    logger := observability.GlobalLogger.Get("scheduler-api")
    middleware := observability.NewHTTPMiddleware(logger, observability.GlobalMetrics)
    
    return &API{
        scheduler: scheduler,
        logger:    logger,
        middleware: middleware,
    }
}

// Start starts the HTTP API server
func (a *API) Start(addr string) error {
    mux := http.NewServeMux()
    
    // Health and metrics endpoints
    mux.HandleFunc("GET /health", observability.HealthCheckHandler)
    mux.HandleFunc("GET /metrics", observability.MetricsHandler(observability.GlobalMetrics))
    mux.HandleFunc("GET /debug", observability.DebugHandler)
    
    // Scheduler endpoints with middleware
    mux.Handle("POST /deployments", a.middleware.Middleware(
        http.HandlerFunc(a.createDeploymentHandler)))
    mux.Handle("GET /deployments", a.middleware.Middleware(
        http.HandlerFunc(a.listDeploymentsHandler)))
    mux.Handle("GET /deployments/{id}", a.middleware.Middleware(
        http.HandlerFunc(a.getDeploymentHandler)))
    mux.Handle("DELETE /deployments/{id}", a.middleware.Middleware(
        http.HandlerFunc(a.deleteDeploymentHandler)))
    mux.Handle("POST /deployments/{id}/scale", a.middleware.Middleware(
        http.HandlerFunc(a.scaleDeploymentHandler)))
    
    server := &http.Server{
        Addr:    addr,
        Handler: mux,
    }
    
    a.logger.Info("starting scheduler API", map[string]interface{}{
        "addr": addr,
    })
    
    return server.ListenAndServe()
}

func (a *API) createDeploymentHandler(w http.ResponseWriter, r *http.Request) {
    tc := observability.TraceFromContext(r.Context())
    
    var req DeploymentRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        observability.ErrorHandler(w, r, http.StatusBadRequest, "invalid request")
        return
    }
    
    deployment, err := a.scheduler.Deploy(r.Context(), req.Name, req.Image, 
        req.Replicas, req.Resources, req.Labels)
    if err != nil {
        a.logger.Error("deploy failed", map[string]interface{}{
            "error":    err.Error(),
            "trace_id": tc.TraceID,
        })
        observability.ErrorHandler(w, r, http.StatusConflict, err.Error())
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(deployment)
}

// ... other handlers similar pattern
```

## Part 3: CLI Integration

### Update cmd/cloudforge/main.go

Add observability initialization in main():

```go
import (
    "cloudforge/pkg/observability"
)

func main() {
    debug := flag.Bool("debug", false, "enable debug logging")
    json := flag.Bool("json", false, "use JSON logging format")
    flag.Parse()
    
    // Configure observability
    logLevel := observability.LogLevelInfo
    if *debug {
        logLevel = observability.LogLevelDebug
    }
    
    observability.GlobalLogger = observability.NewLoggerPool(logLevel, *json)
    
    // ... rest of main
}
```

Add new commands:

```go
case "health":
    healthStatus(cfg)
case "metrics":
    showMetrics()
case "debug":
    showDebugInfo()
default:
    usage()
```

Implement handlers:

```go
func healthStatus(cfg *config.Config) {
    resp, err := http.Get("http://localhost:5000/health")
    if err != nil {
        fmt.Fprintf(os.Stderr, "failed to check health: %v\n", err)
        os.Exit(1)
    }
    defer resp.Body.Close()
    
    var data interface{}
    json.NewDecoder(resp.Body).Decode(&data)
    fmt.Println(prettyJSON(data))
}

func showMetrics() {
    resp, err := http.Get("http://localhost:5000/metrics")
    if err != nil {
        fmt.Fprintf(os.Stderr, "failed to get metrics: %v\n", err)
        os.Exit(1)
    }
    defer resp.Body.Close()
    
    var metrics []map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&metrics)
    
    for _, m := range metrics {
        fmt.Printf("%s: %v (%s)\n", m["name"], m["value"], m["unit"])
    }
}
```

## Testing

### Unit tests with observability

**File**: `pkg/registry/registry_test.go`

```go
func TestRegistryWithObservability(t *testing.T) {
    cfg := &config.Config{}
    cfg.EnsureDirs()
    
    reg, err := registry.New(cfg)
    if err != nil {
        t.Fatal(err)
    }
    
    // Verify logger and metrics initialized
    if reg.logger == nil {
        t.Fatal("logger not initialized")
    }
    
    if reg.metrics == nil {
        t.Fatal("metrics not initialized")
    }
    
    // Test blob push with metrics
    ctx := observability.ContextWithTrace(context.Background(), 
        observability.NewTraceContext())
    
    blob := []byte("test blob data")
    err = reg.store.Put(ctx, "sha256:abc123", blob)
    if err != nil {
        t.Fatal(err)
    }
    
    // Verify metrics recorded
    metrics := reg.metrics.GetMetrics()
    if len(metrics) == 0 {
        t.Fatal("no metrics recorded")
    }
}
```

## Deployment

### Docker build with observability

```dockerfile
FROM golang:1.22 AS builder
WORKDIR /build
COPY . .
RUN go build -o cloudforge ./cmd/cloudforge

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/cloudforge /usr/local/bin/
EXPOSE 5000 5001 6060
ENTRYPOINT ["cloudforge"]
CMD ["-addr", ":5000", "-debug=false", "-json=true"]
```

### Prometheus configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'cloudforge-registry'
    static_configs:
      - targets: ['localhost:5000']
    metrics_path: '/metrics'
    
  - job_name: 'cloudforge-scheduler'
    static_configs:
      - targets: ['localhost:5001']
    metrics_path: '/metrics'
```

### Grafana dashboard

Create dashboard panels for:
- HTTP Request Rate (requests/sec)
- HTTP Request Latency (p50, p95, p99)
- Blob Transfer Volume (bytes/sec)
- Container Count (running vs desired)
- System Memory Usage
- Goroutine Count
- Error Rate

## Verification

### End-to-end trace

```bash
# 1. Start services
./cloudforge registry-server &
./cloudforge scheduler-api &

# 2. Make request with trace logging
curl -H "X-Trace-ID: test-trace-1" \
  -X POST http://localhost:5000/v2/test/blobs/uploads \
  -d @blob.tar.gz

# 3. View metrics
curl http://localhost:5000/metrics | jq '.[] | select(.name | contains("blob"))'

# 4. Check logs
tail -f .data/logs/cloudforge.log | grep "test-trace-1"
```

---

This implementation guide provides a complete path to instrumenting CloudForge with production-grade observability.

