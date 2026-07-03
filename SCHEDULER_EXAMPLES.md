# Scheduler CLI Examples

## Basic Commands

### 1. Deploy a Service

Simple deployment with default 1 replica:
```bash
./cloudforge deploy myapp nginx:latest
```

Deploy with specific replica count:
```bash
./cloudforge deploy web nginx:latest -replicas 3
```

Output:
```
✓ Deployed web
  ID:        1719057600000000000
  Image:     nginx:latest
  Replicas:  3
  Running:   3
  Created:   2026-06-22 10:00:00
```

### 2. List Deployments

```bash
./cloudforge status
```

Output:
```
NAME    ID           IMAGE          REPLICAS  RUNNING  STATUS
web     1719057600   nginx:latest   3         3        healthy
api     1719057601   python:3.9     2         2        healthy
cache   1719057602   redis:latest   1         1        healthy
```

### 3. Get Deployment Details

```bash
./cloudforge status -deployment 1719057600000000000
```

Output:
```
Deployment: web
  ID:         1719057600000000000
  Image:      nginx:latest
  Replicas:   3 desired, 3 running
  Created:    2026-06-22 10:00:00
  Updated:    2026-06-22 10:00:00
```

### 4. List Deployment Containers

```bash
./cloudforge status -deployment 1719057600000000000 -containers
```

Output:
```
Deployment: web
  ID:         1719057600000000000
  Image:      nginx:latest
  Replicas:   3 desired, 3 running
  Created:    2026-06-22 10:00:00
  Updated:    2026-06-22 10:00:00

Containers:
ID                     REPLICA  STATE    PID   STARTED       STOPPED
web-replica-0          0        running  1234  10:00:01      -
web-replica-1          1        running  1235  10:00:01      -
web-replica-2          2        running  1236  10:00:01      -
```

### 5. Scale Up

```bash
./cloudforge scale 1719057600000000000 5
```

Output:
```
✓ Scaled web to 5 replicas
  ID:        1719057600000000000
  Running:   5/5
  Updated:   2026-06-22 10:05:00
```

Verify:
```bash
./cloudforge status -deployment 1719057600000000000
```

```
Deployment: web
  ID:         1719057600000000000
  Image:      nginx:latest
  Replicas:   5 desired, 5 running
  Created:    2026-06-22 10:00:00
  Updated:    2026-06-22 10:05:00
```

### 6. Scale Down

```bash
./cloudforge scale 1719057600000000000 2
```

Output:
```
✓ Scaled web to 2 replicas
  ID:        1719057600000000000
  Running:   2/2
  Updated:   2026-06-22 10:06:00
```

### 7. Delete Deployment

```bash
./cloudforge delete 1719057600000000000
```

Output:
```
✓ Deleted deployment web
```

Verify it's gone:
```bash
./cloudforge status
```

## Advanced Examples

### Deploy with Resource Limits

Memory limit (512 MB):
```bash
./cloudforge deploy api python:3.9 \
  -replicas 2 \
  -memory 536870912
```

CPU limit (50%):
```bash
./cloudforge deploy api python:3.9 \
  -replicas 2 \
  -cpu 50
```

PID limit (32 processes):
```bash
./cloudforge deploy api python:3.9 \
  -replicas 2 \
  -pids 32
```

All together:
```bash
./cloudforge deploy api python:3.9 \
  -replicas 4 \
  -memory 536870912 \
  -cpu 50 \
  -pids 32
```

### Deploy with Labels

```bash
./cloudforge deploy webapp image:latest \
  -labels "version=v2.0,team=platform,env=production"
```

Verify:
```bash
./cloudforge status -deployment ID
```

Output:
```
Deployment: webapp
  ID:         1719057603000000000
  Image:      image:latest
  Replicas:   1 desired, 1 running
  Created:    2026-06-22 11:00:00
  Updated:    2026-06-22 11:00:00
  Labels:
    version: v2.0
    team: platform
    env: production
```

### Multi-Service Deployment

Deploy multiple services:
```bash
# Web tier
./cloudforge deploy web nginx:latest -replicas 3

# API tier
./cloudforge deploy api python:3.9 -replicas 2

# Cache tier
./cloudforge deploy cache redis:latest -replicas 1
```

Check all:
```bash
./cloudforge status
```

```
NAME    ID           IMAGE          REPLICAS  RUNNING  STATUS
web     1719057600   nginx:latest   3         3        healthy
api     1719057601   python:3.9     2         2        healthy
cache   1719057602   redis:latest   1         1        healthy
```

## Scripting Examples

### Shell Script: Deploy and Monitor

```bash
#!/bin/bash

DEPLOYMENT_ID=$(./cloudforge deploy app image:tag -replicas 3 | grep "ID:" | awk '{print $NF}')
echo "Deployed: $DEPLOYMENT_ID"

# Wait for containers to start
sleep 2

# Check status
./cloudforge status -deployment $DEPLOYMENT_ID

# List containers
./cloudforge status -deployment $DEPLOYMENT_ID -containers
```

### Shell Script: Scale Based on Time

```bash
#!/bin/bash

DEPLOYMENT_ID="1719057600000000000"

# Scale up during business hours (9 AM)
HOUR=$(date +%H)
if [ $HOUR -ge 9 ] && [ $HOUR -lt 17 ]; then
  echo "Business hours: scaling to 5 replicas"
  ./cloudforge scale $DEPLOYMENT_ID 5
else
  echo "After hours: scaling to 1 replica"
  ./cloudforge scale $DEPLOYMENT_ID 1
fi

./cloudforge status -deployment $DEPLOYMENT_ID
```

### Shell Script: Cleanup All

```bash
#!/bin/bash

./cloudforge status | tail -n +2 | while read line; do
  ID=$(echo $line | awk '{print $2}')
  NAME=$(echo $line | awk '{print $1}')
  echo "Deleting $NAME..."
  ./cloudforge delete $ID
done

echo "Cleanup complete"
```

## HTTP API Examples

### Using curl

#### Deploy:
```bash
curl -X POST http://localhost:5001/deployments \
  -H "Content-Type: application/json" \
  -d '{
    "name": "web",
    "image": "nginx:latest",
    "replicas": 3
  }'
```

#### List:
```bash
curl http://localhost:5001/deployments
```

#### Get:
```bash
curl http://localhost:5001/deployments/1719057600000000000
```

#### Scale:
```bash
curl -X POST http://localhost:5001/deployments/1719057600000000000/scale \
  -H "Content-Type: application/json" \
  -d '{"replicas": 5}'
```

#### Delete:
```bash
curl -X DELETE http://localhost:5001/deployments/1719057600000000000
```

#### Health:
```bash
curl http://localhost:5001/health
```

### Using wget

```bash
wget -O - http://localhost:5001/deployments
```

### Using httpie (http)

```bash
# Deploy
http POST localhost:5001/deployments \
  name=web image=nginx:latest replicas:=3

# List
http GET localhost:5001/deployments

# Scale
http POST localhost:5001/deployments/1719057600000000000/scale \
  replicas:=5

# Delete
http DELETE localhost:5001/deployments/1719057600000000000
```

## Error Scenarios

### Duplicate Name

```bash
./cloudforge deploy myapp image:tag
./cloudforge deploy myapp image:tag  # Error!
```

```
Error: deployment "myapp" already exists
```

### Invalid Replica Count

```bash
./cloudforge scale 1719057600000000000 0  # Error!
```

```
Error: replicas must be >= 1
```

### Deployment Not Found

```bash
./cloudforge delete invalid-id  # Error!
```

```
Error: deployment not found
```

## Typical Workflow

1. **Deploy services**:
   ```bash
   ./cloudforge deploy web nginx:latest -replicas 3
   ./cloudforge deploy api app:latest -replicas 2
   ```

2. **Verify deployment**:
   ```bash
   ./cloudforge status
   ```

3. **Monitor specific deployment**:
   ```bash
   ./cloudforge status -deployment ID -containers
   ```

4. **Scale based on demand**:
   ```bash
   ./cloudforge scale ID 5  # Scale up
   ```

5. **Update containers** (typically via new image):
   ```bash
   ./cloudforge scale ID 0      # Scale to 0
   # Update image in registry
   ./cloudforge scale ID 3      # Scale back up
   ```

6. **Cleanup when done**:
   ```bash
   ./cloudforge delete ID
   ```

## Performance Tuning

### Large Deployments

For many replicas, deployment may take a few seconds:
```bash
# This will take a moment
./cloudforge deploy large-service image:tag -replicas 100
```

### Batch Operations

Script to deploy multiple services:
```bash
for i in {1..10}; do
  ./cloudforge deploy "service-$i" image:tag -replicas 3 &
done
wait
```

## State Inspection

View deployment state files directly:
```bash
# List all deployments
ls .data/metadata/scheduler/deployments/

# View specific deployment
cat .data/metadata/scheduler/deployments/1719057600000000000.json

# List all containers
ls .data/metadata/scheduler/containers/
```

## Integration with Registry

Deploy images from CloudForge Registry:
```bash
# Assuming registry at localhost:5000
./cloudforge deploy app localhost:5000/myimage:latest -replicas 3
```

## Integration with Build

Deploy recently built image:
```bash
# Build image
./cloudforge build -file build.json

# Deploy built image
./cloudforge deploy myapp myimage:latest -replicas 3
```
