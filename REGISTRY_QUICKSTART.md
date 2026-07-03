# CloudForge Registry - Quick Start Guide

## Overview

You've implemented a complete Docker Registry V2-compatible service with push/pull support, tag management, and chunked uploads. This guide shows how to build, run, and test it.

## Building

### Build the Registry Server
```bash
cd cloudforge
go build -o registry-server ./cmd/registry-server
```

### Build the CLI Client
```bash
go build -o registry-cli ./cmd/registry-cli
```

## Running the Registry Server

### Start with default settings (port 5000)
```bash
./registry-server
```

### Start on custom port
```bash
./registry-server -addr :8000
```

Expected output:
```
Initializing CloudForge Registry...
Blob directory: /path/to/cloudforge/.data/blobs
Metadata directory: /path/to/cloudforge/.data/metadata/images
Starting CloudForge Registry on :5000
API endpoints available at http://localhost:5000/v2/
```

## Testing the Registry

### 1. Verify API is Available
```bash
curl -X GET http://localhost:5000/v2/
# Expected: 200 OK with Docker-Distribution-API-Version: 2.0 header
```

### 2. Push a Blob (File Upload)

Create a test file:
```bash
echo "Hello, CloudForge!" > test.txt
```

Push it using the CLI:
```bash
./registry-cli push-blob localhost:5000 myapp test.txt
```

Or manually with curl:
```bash
# First calculate the digest
CONTENT="Hello, CloudForge!"
DIGEST=$(echo -n "$CONTENT" | sha256sum | awk '{print "sha256:" $1}')

# Then push
curl -X PUT http://localhost:5000/v2/myapp/blobs/$DIGEST \
  -d "$CONTENT"
```

### 3. Check if Blob Exists
```bash
./registry-cli check-blob localhost:5000 myapp sha256:abc123...
```

Or with curl:
```bash
curl -X HEAD http://localhost:5000/v2/myapp/blobs/sha256:abc123...
```

### 4. Pull a Blob (Download)
```bash
./registry-cli pull-blob localhost:5000 myapp sha256:abc123... output.txt
```

### 5. Push a Manifest

Create a manifest file (manifest.json):
```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
  "config": {
    "mediaType": "application/vnd.docker.container.image.v1+json",
    "digest": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "size": 7023
  },
  "layers": [
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "digest": "sha256:e692418e3055e5c1aab71a8b8a7f5a04ee51bf1d3e8e8b8c8d8e8f8g8h8i8j8",
      "size": 32654
    }
  ]
}
```

Push it:
```bash
./registry-cli push-manifest localhost:5000 myapp v1.0 manifest.json
```

### 6. Pull a Manifest by Tag
```bash
./registry-cli pull-manifest localhost:5000 myapp v1.0 retrieved-manifest.json
```

### 7. List All Tags
```bash
./registry-cli list-tags localhost:5000 myapp
```

Expected output:
```
Tags for myapp:
  - v1.0
  - latest
```

## Running Tests

### Run all registry tests
```bash
go test -v ./pkg/registry/
```

### Run with coverage
```bash
go test -cover ./pkg/registry/
```

### Run specific test
```bash
go test -run TestBlobPushPull -v ./pkg/registry/
```

## Complete Push/Pull Workflow Example

### Step 1: Create test layers

```bash
# Layer 1
echo "Layer content 1" > layer1.tar.gz
LAYER1_DIGEST=$(sha256sum layer1.tar.gz | awk '{print "sha256:" $1}')

# Layer 2
echo "Layer content 2" > layer2.tar.gz
LAYER2_DIGEST=$(sha256sum layer2.tar.gz | awk '{print "sha256:" $1}')

# Config
echo '{"architecture":"amd64"}' > config.json
CONFIG_DIGEST=$(sha256sum config.json | awk '{print "sha256:" $1}')
```

### Step 2: Push all blobs

```bash
./registry-cli push-blob localhost:5000 myapp layer1.tar.gz
./registry-cli push-blob localhost:5000 myapp layer2.tar.gz
./registry-cli push-blob localhost:5000 myapp config.json
```

### Step 3: Create manifest

manifest.json:
```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
  "config": {
    "mediaType": "application/vnd.docker.container.image.v1+json",
    "digest": "sha256:ACTUAL_CONFIG_DIGEST",
    "size": 21
  },
  "layers": [
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "digest": "sha256:ACTUAL_LAYER1_DIGEST",
      "size": 18
    },
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "digest": "sha256:ACTUAL_LAYER2_DIGEST",
      "size": 18
    }
  ]
}
```

### Step 4: Push manifest with tag

```bash
./registry-cli push-manifest localhost:5000 myapp latest manifest.json
```

### Step 5: Verify and pull

```bash
# List tags
./registry-cli list-tags localhost:5000 myapp

# Pull manifest
./registry-cli pull-manifest localhost:5000 myapp latest my-retrieved-manifest.json

# Pull layers
./registry-cli pull-blob localhost:5000 myapp sha256:LAYER1_DIGEST retrieved-layer1.tar.gz
./registry-cli pull-blob localhost:5000 myapp sha256:LAYER2_DIGEST retrieved-layer2.tar.gz
```

## Common Operations

### Delete a blob
```bash
curl -X DELETE http://localhost:5000/v2/myapp/blobs/sha256:abc123...
```

### Delete a manifest
```bash
curl -X DELETE http://localhost:5000/v2/myapp/manifests/latest
```

### Get manifest metadata
```bash
curl -X HEAD http://localhost:5000/v2/myapp/manifests/latest
```

### Chunked upload for large files

```bash
# 1. Start upload session
UPLOAD_URL=$(curl -s -X POST http://localhost:5000/v2/myapp/blobs/uploads/ \
  -i 2>&1 | grep Location | cut -d' ' -f2)

# 2. Upload first chunk
curl -X PATCH "$UPLOAD_URL" --data-binary @chunk1.bin

# 3. Upload second chunk  
curl -X PATCH "$UPLOAD_URL" --data-binary @chunk2.bin

# 4. Complete upload with digest verification
FINAL_DIGEST="sha256:..."  # Calculate digest of combined chunks
curl -X PUT "$UPLOAD_URL?digest=$FINAL_DIGEST" --data-binary @chunk3.bin
```

## Directory Structure

After running the registry, you'll see:

```
cloudforge/
  .data/
    blobs/
      sha256/
        ab/
          cdef123.../
            data           # Raw blob content
            metadata       # Metadata JSON
    metadata/
      images/
        repos/
          myapp/
            tags/
              latest.json  # {"digest": "sha256:..."}
              v1.0.json
```

## Troubleshooting

### Registry won't start
- Check port 5000 is available: `netstat -an | grep 5000`
- Use `-addr` flag for different port: `./registry-server -addr :8000`

### Digest mismatch errors
- Ensure file content hasn't changed between push and digest calculation
- Use `sha256sum file` to recalculate digest

### Tags not persisting
- Check `.data/metadata/images/repos/` directory has write permissions
- Ensure `EnsureDirs()` was called successfully

### Cannot find blob after push
- Verify push returned 201 Created status
- Check digest was calculated correctly
- Verify blob exists: `curl -X HEAD http://localhost:5000/v2/{repo}/blobs/{digest}`

## Next Steps in CloudForge Development

Based on the photos, the next phases are:

- **Phase 3**: Union Filesystem layer manager
- **Phase 4**: Image manifest and metadata management
- **Phase 5**: Dockerfile-like build engine
- **Phase 6**: Container runtime
- **Phase 7**: cgroup resource limits
- **Phase 8**: CLI commands (build, run, stop, rm, images)

The registry service is now ready to support these phases!

## Further Enhancements

Consider implementing:

1. **Authentication**: Bearer token support
2. **Repository ACLs**: Per-repo access control
3. **Rate Limiting**: Prevent abuse
4. **Garbage Collection**: Clean up unreferenced blobs
5. **Signature Verification**: Sign and verify manifests
6. **Layer Deduplication**: Save space
7. **Streaming**: Optimize large blob transfers
