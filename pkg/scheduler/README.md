# CloudForge Scheduler

## Overview

The CloudForge Scheduler manages container deployments, scaling, and lifecycle. It provides:

- **Deploy**: Create new deployments with specified container replicas
- **Scale**: Dynamically adjust the number of running replicas
- **Status**: Monitor deployment and container health
- **Delete**: Remove deployments and stop all containers

## Core Components

### Scheduler Service
Manages deployments and their containers with:
- Persistent state storage
- Thread-safe operations
- Resource limit configuration
- Label management

### Deployment
A service deployment consists of:
- **ID**: Unique deployment identifier
- **Name**: Human-readable deployment name
- **Image**: Container image reference
- **Replicas**: Desired container count
- **Running**: Actual running container count
- **Containers**: List of container IDs
- **Resources**: CPU, memory, PID limits
- **Labels**: Metadata key-value pairs
- **CreatedAt/UpdatedAt**: Timestamps

### ContainerState
Tracks individual container state:
- **ID**: Unique container identifier
- **DeploymentID**: Parent deployment
- **Replica**: Replica number (0-based)
- **State**: running, stopped, pending, failed
- **Pid**: Process ID
- **StartedAt**: Container start time
- **StoppedAt**: Container stop time (if stopped)
- **ExitCode**: Process exit code (if stopped)

## API Endpoints

### Deployments

#### Create Deployment
```
POST /deployments
```

Request:
```json
{
  "name": "nginx",
  "image": "nginx:latest",
  "replicas": 3,
  "resources": {
    "memoryLimitBytes": 536870912,
    "cpuQuotaPercent": 50,
    "pidsLimit": 32
  },
  "labels": {
    "version": "1.0",
    "tier": "web"
  }
}
```

Response (201):
```json
{
  "id": "1719057600000000000",
  "name": "nginx",
  "image": "nginx:latest",
  "replicas": 3,
  "running": 3,
  "containers": ["nginx-replica-0", "nginx-replica-1", "nginx-replica-2"],
  "resources": { ... },
  "labels": { ... },
  "createdAt": "2026-06-22T10:00:00Z",
  "updatedAt": "2026-06-22T10:00:00Z"
}
```

#### List Deployments
```
GET /deployments
```

Response (200):
```json
{
  "deployments": [ ... ],
  "count": 3
}
```

#### Get Deployment
```
GET /deployments/{id}
```

Response (200): Deployment object

#### Delete Deployment
```
DELETE /deployments/{id}
```

Response (204): No content

### Scaling

#### Scale Deployment
```
POST /deployments/{id}/scale
```

Request:
```json
{
  "replicas": 5
}
```

Response (200): Updated deployment object

### Containers

#### List Deployment Containers
```
GET /deployments/{id}/containers
```

Response (200):
```json
{
  "containers": [
    {
      "id": "nginx-replica-0",
      "deploymentId": "1719057600000000000",
      "replica": 0,
      "state": "running",
      "pid": 12345,
      "startedAt": "2026-06-22T10:00:01Z",
      "stoppedAt": null,
      "exitCode": null
    }
  ],
  "count": 3
}
```

#### Get Container
```
GET /containers/{id}
```

Response (200): Container state object

### Health

#### Health Check
```
GET /health
```

Response (200):
```json
{
  "status": "healthy",
  "timestamp": {}
}
```

## CLI Commands

### Deploy

Deploy a new service with specified replicas:

```bash
cloudforge deploy myapp nginx:latest -replicas 3
```

With resource limits:
```bash
cloudforge deploy webserver nginx:latest \
  -replicas 2 \
  -memory 536870912 \
  -cpu 50 \
  -pids 32
```

With labels:
```bash
cloudforge deploy api python:3.9 \
  -replicas 4 \
  -labels "version=v1,env=prod"
```

Output:
```
✓ Deployed myapp
  ID:        1719057600000000000
  Image:     nginx:latest
  Replicas:  3
  Running:   3
  Created:   2026-06-22 10:00:00
```

### Scale

Adjust replica count for a deployment:

```bash
cloudforge scale 1719057600000000000 5
```

Output:
```
✓ Scaled myapp to 5 replicas
  ID:        1719057600000000000
  Running:   5/5
  Updated:   2026-06-22 10:05:00
```

### Status

Show deployment status:

```bash
# List all deployments
cloudforge status

# Show specific deployment
cloudforge status -deployment 1719057600000000000

# Show containers for deployment
cloudforge status -deployment 1719057600000000000 -containers
```

Output:
```
NAME            ID           IMAGE          REPLICAS  RUNNING  STATUS
nginx           1719057600   nginx:latest   3         3        healthy
api             1719057601   python:3.9     2         1        degraded
```

### Delete

Delete a deployment:

```bash
cloudforge delete 1719057600000000000
```

Output:
```
✓ Deleted deployment myapp
```

## HTTP API Examples

### Using curl

Deploy a service:
```bash
curl -X POST http://localhost:5001/deployments \
  -H "Content-Type: application/json" \
  -d '{
    "name": "web",
    "image": "nginx:latest",
    "replicas": 3
  }'
```

Scale a deployment:
```bash
curl -X POST http://localhost:5001/deployments/DEPLOYMENT_ID/scale \
  -H "Content-Type: application/json" \
  -d '{"replicas": 5}'
```

Get deployment status:
```bash
curl http://localhost:5001/deployments/DEPLOYMENT_ID
```

List all deployments:
```bash
curl http://localhost:5001/deployments
```

## State Management

### Persistence

Scheduler state is persisted to disk at:
```
.data/metadata/scheduler/
├── deployments/
│   ├── DEPLOYMENT_ID.json
│   └── ...
└── containers/
    ├── CONTAINER_ID.json
    └── ...
```

State is automatically saved after:
- Deployments are created or deleted
- Scaling operations
- Container state changes

State is automatically loaded on startup.

### File Format

Deployment state (JSON):
```json
{
  "id": "1719057600000000000",
  "name": "nginx",
  "image": "nginx:latest",
  "replicas": 3,
  "running": 3,
  "containers": [...],
  "resources": {...},
  "labels": {...},
  "createdAt": "2026-06-22T10:00:00Z",
  "updatedAt": "2026-06-22T10:00:00Z"
}
```

Container state (JSON):
```json
{
  "id": "nginx-replica-0",
  "deployment_id": "1719057600000000000",
  "replica": 0,
  "state": "running",
  "pid": 12345,
  "started_at": "2026-06-22T10:00:01Z",
  "stopped_at": null,
  "exit_code": null
}
```

## Container Naming

Containers are automatically named based on deployment:
```
{deployment-name}-replica-{replica-number}
```

Example:
```
nginx-replica-0
nginx-replica-1
nginx-replica-2
```

## Resource Limits

Configure resource constraints per deployment:

### Memory
Limit memory in bytes (cgroup memory.max):
```
"resources": {
  "memoryLimitBytes": 536870912  // 512 MB
}
```

### CPU
Limit CPU usage as percentage (cgroup cpu.max):
```
"resources": {
  "cpuQuotaPercent": 50  // 50%
}
```

### PIDs
Limit process count (cgroup pids.max):
```
"resources": {
  "pidsLimit": 32
}
```

## Labels

Arbitrary metadata for deployments:
```
"labels": {
  "version": "v1.0",
  "environment": "production",
  "team": "platform"
}
```

Labels are useful for:
- Filtering and searching
- Tracking deployment versions
- Environmental tagging
- Custom organizational metadata

## Deployment Lifecycle

1. **Create**: `POST /deployments`
   - Creates deployment record
   - Starts specified number of replicas
   - Saves state to disk

2. **Running**: Containers execute with specified resources

3. **Scale**: `POST /deployments/{id}/scale`
   - Adjust replica count
   - Add or remove containers
   - Update deployment state

4. **Status Check**: `GET /deployments/{id}`
   - Query current state
   - Check running container count
   - Monitor health (running vs desired)

5. **Delete**: `DELETE /deployments/{id}`
   - Stop all containers
   - Remove deployment record
   - Clean up state files

## Status Indicators

### Deployment Health

- **Healthy**: All replicas running (`running == replicas`)
- **Degraded**: Some replicas not running (`running < replicas`)
- **Pending**: Recently created, containers starting

### Container States

- **pending**: Container being initialized
- **running**: Container actively running
- **stopped**: Container stopped normally
- **failed**: Container terminated with error

## Testing

Run the test suite:

```bash
go test -v ./pkg/scheduler/
```

Test coverage includes:
- Deployment creation with various configurations
- Scaling up and down
- Container lifecycle management
- State persistence and loading
- Resource limit configuration
- Label management
- Error handling

## Performance Characteristics

- **Deploy**: O(n) where n = replica count (creates n containers)
- **Scale**: O(k) where k = delta in replica count
- **Status**: O(1) for specific deployment, O(m) for all where m = deployment count
- **State Save**: O(m + n) where m = deployments, n = containers

All operations are thread-safe using RWMutex.

## Error Handling

- Duplicate deployment names: Returns 409 Conflict
- Invalid replica count: Returns 400 Bad Request
- Deployment not found: Returns 404 Not Found
- Server errors: Returns 500 Internal Server Error

## Future Enhancements

- [ ] Health checks for containers (liveness/readiness probes)
- [ ] Automatic scaling based on metrics (HPA)
- [ ] Rolling updates for zero-downtime deployments
- [ ] Multi-zone deployments
- [ ] Network policies and service discovery
- [ ] Persistent volume support
- [ ] Image pull policies and registries
- [ ] Event logging and audit trail
