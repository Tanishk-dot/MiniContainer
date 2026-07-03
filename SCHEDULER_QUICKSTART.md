# CloudForge Scheduler - Quick Start Guide

## Overview

The CloudForge Scheduler enables:
- **Deploy**: Create services with multiple replicas
- **Scale**: Dynamically adjust replica count
- **Status**: Monitor deployment health
- **Delete**: Clean up deployments

## Building

```bash
cd cloudforge
go build -o cloudforge ./cmd/cloudforge
```

## Testing

```bash
# Run all tests
go test -v ./pkg/scheduler/

# Run specific test
go test -run TestScheduler_Deploy -v ./pkg/scheduler/
```

## CLI Usage

### 1. Deploy a Service

Deploy with 3 replicas:
```bash
./cloudforge deploy myapp nginx:latest -replicas 3
```

Expected output:
```
✓ Deployed myapp
  ID:        1719057600000000000
  Image:     nginx:latest
  Replicas:  3
  Running:   3
  Created:   2026-06-22 10:00:00
```

With resource limits:
```bash
./cloudforge deploy webserver nginx:latest \
  -replicas 2 \
  -memory 536870912 \
  -cpu 50 \
  -pids 32
```

With labels:
```bash
./cloudforge deploy api python:3.9 \
  -replicas 4 \
  -labels "version=v1.0,env=production"
```

### 2. Check Status

List all deployments:
```bash
./cloudforge status
```

Get specific deployment:
```bash
./cloudforge status -deployment 1719057600000000000
```

Show containers:
```bash
./cloudforge status -deployment 1719057600000000000 -containers
```

Expected output:
```
NAME            ID           IMAGE          REPLICAS  RUNNING  STATUS
myapp           1719057600   nginx:latest   3         3        healthy
webserver       1719057601   nginx:latest   2         2        healthy
api             1719057602   python:3.9     4         3        degraded
```

### 3. Scale a Deployment

Scale up:
```bash
./cloudforge scale 1719057600000000000 5
```

Scale down:
```bash
./cloudforge scale 1719057600000000000 1
```

Expected output:
```
✓ Scaled myapp to 5 replicas
  ID:        1719057600000000000
  Running:   5/5
  Updated:   2026-06-22 10:05:00
```

### 4. Delete a Deployment

```bash
./cloudforge delete 1719057600000000000
```

Expected output:
```
✓ Deleted deployment myapp
```

## HTTP API Usage

### Start Scheduler API Server

Create a simple server program:

```go
package main

import (
	"log"
	
	"cloudforge/internal/config"
	"cloudforge/pkg/scheduler"
)

func main() {
	cfg := &config.Config{}
	cfg.EnsureDirs()
	
	sched, err := scheduler.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	
	api := scheduler.NewAPI(sched)
	log.Println("Starting scheduler API on :5001")
	log.Fatal(api.Start(":5001"))
}
```

Build and run:
```bash
go build -o scheduler-api server.go
./scheduler-api
```

### API Examples

#### Deploy with curl:
```bash
curl -X POST http://localhost:5001/deployments \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nginx",
    "image": "nginx:latest",
    "replicas": 3
  }'
```

Response:
```json
{
  "id": "1719057600000000000",
  "name": "nginx",
  "image": "nginx:latest",
  "replicas": 3,
  "running": 3,
  "containers": [...],
  "createdAt": "2026-06-22T10:00:00Z",
  "updatedAt": "2026-06-22T10:00:00Z"
}
```

#### List deployments:
```bash
curl http://localhost:5001/deployments
```

#### Get deployment:
```bash
curl http://localhost:5001/deployments/1719057600000000000
```

#### Scale deployment:
```bash
curl -X POST http://localhost:5001/deployments/1719057600000000000/scale \
  -H "Content-Type: application/json" \
  -d '{"replicas": 5}'
```

#### Delete deployment:
```bash
curl -X DELETE http://localhost:5001/deployments/1719057600000000000
```

#### Health check:
```bash
curl http://localhost:5001/health
```

Response:
```json
{
  "status": "healthy"
}
```

## Common Workflows

### Workflow 1: Deploy and Monitor

```bash
# Deploy service
./cloudforge deploy api python:3.9 -replicas 3

# Check status
./cloudforge status

# List containers
./cloudforge status -deployment ID -containers
```

### Workflow 2: Scale Based on Load

```bash
# Start with 2 replicas
./cloudforge deploy web nginx:latest -replicas 2

# Check current status
./cloudforge status -deployment ID

# Scale up during high load
./cloudforge scale ID 5

# Scale down during low load
./cloudforge scale ID 2
```

### Workflow 3: Clean Up

```bash
# List deployments
./cloudforge status

# Delete specific deployment
./cloudforge delete ID

# Verify it's gone
./cloudforge status
```

## Configuration

### Environment Variables

Not required; uses default CloudForge config directories:
```
.data/metadata/scheduler/deployments/
.data/metadata/scheduler/containers/
```

### State Files

Deployments are persisted as JSON:
```
.data/metadata/scheduler/deployments/ID.json
.data/metadata/scheduler/containers/CONTAINER_ID.json
```

## Troubleshooting

### Issue: Deployment not found
```
Error: deployment "ID" not found
```

Solution: Use `cloudforge status` to list deployment IDs.

### Issue: Invalid replica count
```
Error: replicas must be >= 1
```

Solution: Ensure replica count is at least 1.

### Issue: Duplicate deployment name
```
Error: deployment "name" already exists
```

Solution: Use a different deployment name or delete existing one first.

### Issue: Scale operation fails
```
Error: scale failed: deployment not found
```

Solution: Verify deployment ID with `cloudforge status`.

## State Persistence

The scheduler automatically:
- Saves state after deploy/scale/delete operations
- Loads state on startup
- Recovers from disk on restart

State is stored in JSON format for easy inspection:
```bash
ls .data/metadata/scheduler/deployments/
cat .data/metadata/scheduler/deployments/ID.json
```

## Resource Limits

### Memory (in bytes)

```bash
./cloudforge deploy app image:tag \
  -memory 536870912  # 512 MB
```

### CPU (percentage 1-100)

```bash
./cloudforge deploy app image:tag \
  -cpu 50  # 50% of CPU
```

### Process Limit

```bash
./cloudforge deploy app image:tag \
  -pids 32  # Max 32 processes
```

## Labels Example

```bash
./cloudforge deploy webapp image:tag \
  -labels "version=v2.0,team=platform,env=prod"
```

## Advanced Usage

### Get Container Details

```bash
# List containers for deployment
./cloudforge status -deployment DEPLOYMENT_ID -containers
```

Output:
```
Containers:
ID                      REPLICA  STATE    PID   STARTED      STOPPED
myapp-replica-0         0        running  1234  10:00:01     -
myapp-replica-1         1        running  1235  10:00:02     -
myapp-replica-2         2        running  1236  10:00:03     -
```

### HTTP API with Resource Limits

```bash
curl -X POST http://localhost:5001/deployments \
  -H "Content-Type: application/json" \
  -d '{
    "name": "limited-app",
    "image": "app:latest",
    "replicas": 2,
    "resources": {
      "memoryLimitBytes": 268435456,
      "cpuQuotaPercent": 25,
      "pidsLimit": 16
    }
  }'
```

## Performance Notes

- Deployments with many replicas (100+) may take a few seconds
- Scaling operations are asynchronous
- Container startup time depends on image size and system load
- State file operations are fast (< 100ms typically)

## Next Steps

1. **Integrate with Image Registry**: Pull images from the CloudForge Registry
2. **Add Health Checks**: Monitor container liveness
3. **Implement Auto-scaling**: Scale based on metrics
4. **Add Service Discovery**: Route traffic to healthy containers
5. **Support Rolling Updates**: Deploy new versions with zero downtime

## Related Components

- **Registry**: [REGISTRY_QUICKSTART.md](../REGISTRY_QUICKSTART.md) for image management
- **Runtime**: Container execution with cgroups
- **Storage**: Persistent state backend

## Files

- **Implementation**: `pkg/scheduler/scheduler.go`
- **API**: `pkg/scheduler/api.go`
- **CLI**: `cmd/cloudforge/scheduler.go`
- **Tests**: `pkg/scheduler/scheduler_test.go`
- **Documentation**: `pkg/scheduler/README.md` (this file)
