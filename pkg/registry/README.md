# CloudForge Registry Service

## Overview

The CloudForge Registry Service provides a Docker Registry V2-compatible HTTP API for pushing and pulling container images. It manages:

- **Blob Storage**: Content-addressable storage using SHA256 digests
- **Manifest Management**: Image manifests describing layers and configuration
- **Tag Management**: Human-readable references to image digests
- **Chunked Uploads**: Support for uploading large blobs in chunks

## Architecture

The Registry is composed of:

1. **HTTP Server**: RESTful API handlers for registry operations
2. **Blob Store**: Content-addressable storage with integrity verification
3. **Image Manager**: Manifest storage and retrieval with metadata tracking
4. **Tag Resolver**: Maps tags to digests and vice versa
5. **Upload Sessions**: Tracks in-progress chunked uploads

## API Endpoints

### Root API Check
```
GET /v2/
```
Verifies v2 API availability. Returns 200 OK with `Docker-Distribution-API-Version: 2.0` header.

### Blob Operations

#### Check Blob Existence
```
HEAD /v2/{repo}/blobs/{digest}
```
- **200 OK**: Blob exists
- **404 Not Found**: Blob not found

#### Download Blob
```
GET /v2/{repo}/blobs/{digest}
```
Retrieves a complete blob by digest.

**Response Headers:**
- `Docker-Content-Digest`: The blob's digest
- `Content-Length`: Size in bytes
- `Content-Type`: application/octet-stream

**Status Codes:**
- **200 OK**: Blob retrieved successfully
- **404 Not Found**: Blob not found

#### Upload Complete Blob
```
PUT /v2/{repo}/blobs/{digest}
```
Uploads a complete blob in a single request.

**Request Body**: Raw blob data

**Validation**: Digest must match SHA256 of uploaded content

**Status Codes:**
- **201 Created**: Blob stored successfully
- **400 Bad Request**: Digest mismatch or invalid format
- **500 Internal Server Error**: Storage error

**Response Headers:**
- `Docker-Content-Digest`: Confirmed digest

#### Delete Blob
```
DELETE /v2/{repo}/blobs/{digest}
```
Removes a blob from storage.

**Status Codes:**
- **202 Accepted**: Deletion initiated
- **404 Not Found**: Blob not found

### Chunked Upload (for Large Blobs)

#### Initiate Upload Session
```
POST /v2/{repo}/blobs/uploads/
```
Creates a new chunked upload session.

**Response Headers:**
- `Location`: Upload session URL (use in subsequent requests)
- `Range`: Bytes accepted (0-0 initially)

**Status:** 202 Accepted

#### Upload Chunk
```
PATCH /v2/{repo}/blobs/uploads/{uuid}
```
Appends data to an ongoing upload session.

**Request Body**: Raw chunk data

**Response Headers:**
- `Location`: Upload session URL
- `Range`: Bytes accepted so far

**Status:** 202 Accepted

#### Complete Chunked Upload
```
PUT /v2/{repo}/blobs/uploads/{uuid}?digest={digest}
```
Finalizes a chunked upload and verifies integrity.

**Query Parameters:**
- `digest`: Target digest (SHA256 format)

**Request Body**: Final chunk data (optional)

**Validation**: Combined data must match specified digest

**Status Codes:**
- **201 Created**: Upload complete
- **400 Bad Request**: Digest mismatch
- **404 Not Found**: Upload session not found

### Manifest Operations

#### Download Manifest
```
GET /v2/{repo}/manifests/{reference}
```
Retrieves a manifest by reference (tag or digest).

**Response Headers:**
- `Docker-Content-Digest`: The manifest's digest
- `Content-Type`: Manifest media type (e.g., `application/vnd.docker.distribution.manifest.v2+json`)
- `Content-Length`: Size in bytes

**Status Codes:**
- **200 OK**: Manifest retrieved
- **404 Not Found**: Reference not found

#### Check Manifest Existence
```
HEAD /v2/{repo}/manifests/{reference}
```
Verifies manifest existence and retrieves metadata.

**Status Codes:**
- **200 OK**: Manifest exists
- **404 Not Found**: Manifest not found

#### Upload Manifest
```
PUT /v2/{repo}/manifests/{reference}
```
Stores a manifest under the given reference (tag or digest).

**Request Body**: Manifest JSON data

**Features:**
- Auto-parses manifest JSON to extract config and layer digests
- If reference is a tag, creates tag mapping
- Returns digest in response headers

**Status Codes:**
- **201 Created**: Manifest stored
- **400 Bad Request**: Invalid manifest format
- **500 Internal Server Error**: Storage error

**Response Headers:**
- `Docker-Content-Digest`: Manifest's content digest

#### Delete Manifest
```
DELETE /v2/{repo}/manifests/{reference}
```
Removes a manifest and associated tag if present.

**Status Codes:**
- **202 Accepted**: Deletion initiated
- **404 Not Found**: Manifest not found

### Tag Operations

#### List Repository Tags
```
GET /v2/{repo}/tags/list
```
Lists all tags in a repository.

**Response (JSON):**
```json
{
  "name": "myapp",
  "tags": ["latest", "v1.0", "v1.1"]
}
```

**Status:** 200 OK (or empty tags array if none exist)

## Push/Pull Workflow

### Push Workflow (Client to Registry)

1. **Push Blob(s)**
   - Send layer data via `PUT /v2/{repo}/blobs/{digest}` or chunked upload
   - Registry validates digest and stores content

2. **Push Config Blob** (optional separate blob)
   - Similar to layer upload

3. **Push Manifest**
   - Send manifest JSON describing config and layers
   - Reference it by tag: `PUT /v2/{repo}/manifests/{tag}`

Example:
```bash
# Push layer blob
curl -X PUT https://registry.example.com/v2/myapp/blobs/sha256:abc123... -d @layer.tar.gz

# Push config blob
curl -X PUT https://registry.example.com/v2/myapp/blobs/sha256:def456... -d @config.json

# Push manifest with tag
curl -X PUT https://registry.example.com/v2/myapp/manifests/latest \
  -H "Content-Type: application/vnd.docker.distribution.manifest.v2+json" \
  -d '{
    "schemaVersion": 2,
    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
    "config": {
      "mediaType": "application/vnd.docker.container.image.v1+json",
      "digest": "sha256:def456..."
    },
    "layers": [{
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "digest": "sha256:abc123..."
    }]
  }'
```

### Pull Workflow (Client from Registry)

1. **Resolve Reference**
   - If using tag, first resolve it to a digest
   - `GET /v2/{repo}/manifests/{tag}` returns manifest with digest in headers

2. **Download Manifest**
   - `GET /v2/{repo}/manifests/{digest}` by digest for guaranteed consistency

3. **Download Blobs**
   - For each layer: `GET /v2/{repo}/blobs/{layer-digest}`
   - For config: `GET /v2/{repo}/blobs/{config-digest}`

Example:
```bash
# Get manifest by tag
MANIFEST=$(curl -s https://registry.example.com/v2/myapp/manifests/latest)
DIGEST=$(echo $MANIFEST | jq -r '.config.digest')

# Get config blob
curl -s https://registry.example.com/v2/myapp/blobs/$DIGEST -o config.json

# Get each layer
curl -s https://registry.example.com/v2/myapp/blobs/sha256:abc123... -o layer.tar.gz
```

## Digest Format

Digests follow the format: `algorithm:hex`

- **Algorithm**: Currently only `sha256` is supported
- **Hex**: 64-character hexadecimal string (SHA256 hash)

Example: `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`

## Error Handling

The registry returns standard HTTP status codes:

| Code | Meaning |
|------|---------|
| 200 | OK - Request succeeded |
| 201 | Created - Resource created |
| 202 | Accepted - Operation accepted (async) |
| 204 | No Content - Success with no body |
| 400 | Bad Request - Invalid input or digest mismatch |
| 404 | Not Found - Resource doesn't exist |
| 500 | Internal Server Error - Server error |

Error responses include a message in the body and/or `www-authenticate` headers.

## Storage Layout

```
metadata/
  images/
    repos/
      {repo}/
        tags/
          {tag}.json         # {"digest": "sha256:..."}
blobs/
  sha256/
    {first-two-chars}/
      {remaining-chars}/
        data                 # Raw blob content
        metadata             # Blob metadata (JSON)
```

## Configuration

Registry requires configuration from the main `config.Config` object:

- `config.BlobsDir()`: Directory for storing blob data
- `config.BlobMetadataDir()`: Directory for blob metadata
- `config.ImageMetadataDir()`: Directory for image/tag metadata

## Starting the Registry

```go
package main

import (
    "log"
    "cloudforge/internal/config"
    "cloudforge/pkg/registry"
)

func main() {
    cfg := &config.Config{}
    if err := cfg.EnsureDirs(); err != nil {
        log.Fatal(err)
    }

    reg, err := registry.New(cfg)
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Registry listening on :5000")
    log.Fatal(reg.Start(":5000"))
}
```

## Testing

Run the comprehensive test suite:

```bash
go test -v ./pkg/registry/
```

Tests cover:
- API availability check
- Blob push/pull/delete
- Blob digest validation
- Manifest push/pull/delete
- Tag management (write/read/delete/list)
- Chunked uploads
- Reference resolution (digest vs tag)

## Content-Type Negotiation

The registry currently uses these media types:

- **Blobs**: `application/octet-stream`
- **Manifest v2**: `application/vnd.docker.distribution.manifest.v2+json`
- **OCI Manifest**: `application/vnd.oci.image.manifest.v1+json`
- **Config**: `application/vnd.docker.container.image.v1+json`
- **Layer**: `application/vnd.docker.image.rootfs.diff.tar.gzip`

## Security Considerations

1. **Digest Verification**: All pushed blobs are verified against their claimed digest
2. **Atomic Writes**: Tag mappings use temp files and atomic renames
3. **No Authentication**: Current implementation has no auth (add Bearer token support for production)
4. **No Authorization**: All users can access all images (add ACLs for production)
5. **No Rate Limiting**: Consider adding for production deployments

## Future Enhancements

- [ ] Bearer token authentication
- [ ] Repository-level authorization
- [ ] Rate limiting
- [ ] Image garbage collection
- [ ] Manifest validation with signature verification
- [ ] Blob deduplication across repositories
- [ ] Image layer caching optimization
- [ ] Streaming responses for large blobs
