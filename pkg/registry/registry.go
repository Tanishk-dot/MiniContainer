package registry

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"

    "cloudforge/internal/config"
    "cloudforge/pkg/image"
    "cloudforge/pkg/hash"
    "cloudforge/pkg/storage"
)

// Registry provides a complete HTTP service for pushing and pulling blobs and manifests.
// Implements a subset of Docker Registry V2 API.
type Registry struct {
    cfg    *config.Config
    images *image.Engine
    store  *storage.Engine

    // Track chunked uploads in progress
    uploads map[string]*uploadSession
    umutex  sync.Mutex
}

// uploadSession tracks a chunked blob upload
type uploadSession struct {
    id        string
    repo      string
    digest    hash.Digest
    data      *bytes.Buffer
    createdAt time.Time
}

// New creates a new Registry wired to the configured storage and image engines.
func New(cfg *config.Config) (*Registry, error) {
    ie, err := image.NewEngine(cfg)
    if err != nil {
        return nil, err
    }
    return &Registry{
        cfg:     cfg,
        images:  ie,
        store:   ie.Storage,
        uploads: make(map[string]*uploadSession),
    }, nil
}

// Start runs the HTTP server on the given address.
func (r *Registry) Start(addr string) error {
    mux := http.NewServeMux()

    // Root check endpoint (v2 API)
    mux.HandleFunc("/v2/", r.checkAPIHandler)

    // Blob endpoints: GET, PUT, DELETE, HEAD
    mux.HandleFunc("GET /v2/{repo}/blobs/{digest}", r.getBlobHandler)
    mux.HandleFunc("HEAD /v2/{repo}/blobs/{digest}", r.headBlobHandler)
    mux.HandleFunc("PUT /v2/{repo}/blobs/{digest}", r.putBlobHandler)
    mux.HandleFunc("DELETE /v2/{repo}/blobs/{digest}", r.deleteBlobHandler)

    // Chunked upload endpoints: POST, PATCH, PUT
    mux.HandleFunc("POST /v2/{repo}/blobs/uploads/", r.startUploadHandler)
    mux.HandleFunc("PATCH /v2/{repo}/blobs/uploads/{uuid}", r.uploadChunkHandler)
    mux.HandleFunc("PUT /v2/{repo}/blobs/uploads/{uuid}", r.completeUploadHandler)

    // Manifest endpoints: GET, PUT, DELETE, HEAD
    mux.HandleFunc("GET /v2/{repo}/manifests/{reference}", r.getManifestHandler)
    mux.HandleFunc("HEAD /v2/{repo}/manifests/{reference}", r.headManifestHandler)
    mux.HandleFunc("PUT /v2/{repo}/manifests/{reference}", r.putManifestHandler)
    mux.HandleFunc("DELETE /v2/{repo}/manifests/{reference}", r.deleteManifestHandler)

    // Tagging endpoints
    mux.HandleFunc("GET /v2/{repo}/tags/list", r.listTagsHandler)

    return http.ListenAndServe(addr, mux)
}

// checkAPIHandler verifies v2 API availability
func (r *Registry) checkAPIHandler(w http.ResponseWriter, req *http.Request) {
    if req.URL.Path == "/v2/" {
        w.Header().Set("Docker-Distribution-API-Version", "2.0")
        w.WriteHeader(http.StatusOK)
        return
    }
    http.Error(w, "not found", http.StatusNotFound)
}


// ============================================================================
// BLOB HANDLERS
// ============================================================================

// getBlobHandler retrieves a blob by digest
func (r *Registry) getBlobHandler(w http.ResponseWriter, req *http.Request) {
    _ = req.PathValue("repo") // repo name from path, used for future multi-repo support
    digestStr := req.PathValue("digest")

    d, err := hash.Parse(digestStr)
    if err != nil {
        http.Error(w, "invalid digest format", http.StatusBadRequest)
        return
    }

    rc, size, err := r.store.Blobs.Get(context.Background(), d)
    if err != nil {
        if errors.Is(err, storage.ErrNotFound) {
            http.Error(w, "blob not found", http.StatusNotFound)
            return
        }
        http.Error(w, "failed to retrieve blob", http.StatusInternalServerError)
        return
    }
    defer rc.Close()

    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
    w.Header().Set("Docker-Content-Digest", d.String())
    w.WriteHeader(http.StatusOK)
    _, _ = io.Copy(w, rc)
}

// headBlobHandler checks if a blob exists
func (r *Registry) headBlobHandler(w http.ResponseWriter, req *http.Request) {
    _ = req.PathValue("repo") // repo name from path, used for future multi-repo support
    digestStr := req.PathValue("digest")

    d, err := hash.Parse(digestStr)
    if err != nil {
        http.Error(w, "invalid digest format", http.StatusBadRequest)
        return
    }

    _, size, err := r.store.Blobs.Get(context.Background(), d)
    if err != nil {
        if errors.Is(err, storage.ErrNotFound) {
            http.Error(w, "blob not found", http.StatusNotFound)
            return
        }
        http.Error(w, "failed to check blob", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
    w.Header().Set("Docker-Content-Digest", d.String())
    w.WriteHeader(http.StatusOK)
}

// putBlobHandler stores a complete blob
func (r *Registry) putBlobHandler(w http.ResponseWriter, req *http.Request) {
    _ = req.PathValue("repo") // repo name from path, used for future multi-repo support
    digestStr := req.PathValue("digest")

    d, err := hash.Parse(digestStr)
    if err != nil {
        http.Error(w, "invalid digest format", http.StatusBadRequest)
        return
    }

    data, err := io.ReadAll(req.Body)
    if err != nil {
        http.Error(w, "failed to read request body", http.StatusBadRequest)
        return
    }

    // Calculate actual digest and verify it matches
    computed := hash.FromBytes(data)
    if computed.Hex != d.Hex {
        http.Error(w, fmt.Sprintf("digest mismatch: got %s, expected %s", computed.String(), d.String()), http.StatusBadRequest)
        return
    }

    _, _, err = r.store.Blobs.Put(context.Background(), bytes.NewReader(data))
    if err != nil {
        http.Error(w, "failed to store blob", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Docker-Content-Digest", d.String())
    w.WriteHeader(http.StatusCreated)
}

// deleteBlobHandler removes a blob
func (r *Registry) deleteBlobHandler(w http.ResponseWriter, req *http.Request) {
    _ = req.PathValue("repo") // repo name from path, used for future multi-repo support
    digestStr := req.PathValue("digest")

    d, err := hash.Parse(digestStr)
    if err != nil {
        http.Error(w, "invalid digest format", http.StatusBadRequest)
        return
    }

    // Check if blob exists
    _, _, err = r.store.Blobs.Get(context.Background(), d)
    if err != nil {
        if errors.Is(err, storage.ErrNotFound) {
            http.Error(w, "blob not found", http.StatusNotFound)
            return
        }
        http.Error(w, "failed to check blob", http.StatusInternalServerError)
        return
    }

    err = r.store.Metadata.Delete(context.Background(), d)
    if err != nil {
        http.Error(w, "failed to delete blob metadata", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusAccepted)
}

// ============================================================================
// CHUNKED UPLOAD HANDLERS (for large blobs)
// ============================================================================

// startUploadHandler initiates a chunked blob upload session
func (r *Registry) startUploadHandler(w http.ResponseWriter, req *http.Request) {
    repo := req.PathValue("repo")

    uploadID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), repo)

    r.umutex.Lock()
    r.uploads[uploadID] = &uploadSession{
        id:        uploadID,
        repo:      repo,
        data:      &bytes.Buffer{},
        createdAt: time.Now(),
    }
    r.umutex.Unlock()

    // Return upload location
    uploadURL := fmt.Sprintf("/v2/%s/blobs/uploads/%s", repo, uploadID)
    w.Header().Set("Location", uploadURL)
    w.Header().Set("Range", "0-0")
    w.WriteHeader(http.StatusAccepted)
}

// uploadChunkHandler appends data to an upload session
func (r *Registry) uploadChunkHandler(w http.ResponseWriter, req *http.Request) {
    repo := req.PathValue("repo")
    uploadID := req.PathValue("uuid")

    r.umutex.Lock()
    session, exists := r.uploads[uploadID]
    r.umutex.Unlock()

    if !exists {
        http.Error(w, "upload session not found", http.StatusNotFound)
        return
    }

    chunk, err := io.ReadAll(req.Body)
    if err != nil {
        http.Error(w, "failed to read chunk", http.StatusBadRequest)
        return
    }

    _, _ = session.data.Write(chunk)

    uploadURL := fmt.Sprintf("/v2/%s/blobs/uploads/%s", repo, uploadID)
    w.Header().Set("Location", uploadURL)
    w.Header().Set("Range", fmt.Sprintf("0-%d", session.data.Len()-1))
    w.WriteHeader(http.StatusAccepted)
}

// completeUploadHandler finalizes a chunked upload
func (r *Registry) completeUploadHandler(w http.ResponseWriter, req *http.Request) {
    _ = req.PathValue("repo") // repo name from path, used for future multi-repo support
    uploadID := req.PathValue("uuid")
    digestStr := req.URL.Query().Get("digest")

    if digestStr == "" {
        http.Error(w, "digest parameter required", http.StatusBadRequest)
        return
    }

    d, err := hash.Parse(digestStr)
    if err != nil {
        http.Error(w, "invalid digest format", http.StatusBadRequest)
        return
    }

    r.umutex.Lock()
    session, exists := r.uploads[uploadID]
    r.umutex.Unlock()

    if !exists {
        http.Error(w, "upload session not found", http.StatusNotFound)
        return
    }

    // Read any final chunk from body
    final, _ := io.ReadAll(req.Body)
    if len(final) > 0 {
        _, _ = session.data.Write(final)
    }

    // Verify digest
    data := session.data.Bytes()
    computed := hash.FromBytes(data)
    if computed.Hex != d.Hex {
        http.Error(w, fmt.Sprintf("digest mismatch: got %s, expected %s", computed.String(), d.String()), http.StatusBadRequest)
        return
    }

    // Store blob
    _, _, err = r.store.Blobs.Put(context.Background(), bytes.NewReader(data))
    if err != nil {
        http.Error(w, "failed to store blob", http.StatusInternalServerError)
        return
    }

    // Clean up upload session
    r.umutex.Lock()
    delete(r.uploads, uploadID)
    r.umutex.Unlock()

    w.Header().Set("Docker-Content-Digest", d.String())
    w.WriteHeader(http.StatusCreated)
}

// ============================================================================
// MANIFEST HANDLERS
// ============================================================================

// getManifestHandler retrieves a manifest by reference (tag or digest)
func (r *Registry) getManifestHandler(w http.ResponseWriter, req *http.Request) {
    repo := req.PathValue("repo")
    reference := req.PathValue("reference")

    d, err := r.resolveReference(repo, reference)
    if err != nil {
        http.Error(w, "manifest not found", http.StatusNotFound)
        return
    }

    rc, _, err := r.store.Blobs.Get(context.Background(), d)
    if err != nil {
        if errors.Is(err, storage.ErrNotFound) {
            http.Error(w, "manifest not found", http.StatusNotFound)
            return
        }
        http.Error(w, "failed to retrieve manifest", http.StatusInternalServerError)
        return
    }
    defer rc.Close()

    manifest, err := r.images.Images.Get(context.Background(), d)
    if err != nil {
        http.Error(w, "failed to get manifest metadata", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", manifest.MediaType)
    w.Header().Set("Docker-Content-Digest", d.String())
    w.WriteHeader(http.StatusOK)
    _, _ = io.Copy(w, rc)
}

// headManifestHandler checks if a manifest exists
func (r *Registry) headManifestHandler(w http.ResponseWriter, req *http.Request) {
    repo := req.PathValue("repo")
    reference := req.PathValue("reference")

    d, err := r.resolveReference(repo, reference)
    if err != nil {
        http.Error(w, "manifest not found", http.StatusNotFound)
        return
    }

    manifest, err := r.images.Images.Get(context.Background(), d)
    if err != nil {
        if errors.Is(err, image.ErrNotFound) {
            http.Error(w, "manifest not found", http.StatusNotFound)
            return
        }
        http.Error(w, "failed to get manifest", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", manifest.MediaType)
    w.Header().Set("Content-Length", fmt.Sprintf("%d", manifest.Size))
    w.Header().Set("Docker-Content-Digest", d.String())
    w.WriteHeader(http.StatusOK)
}

// putManifestHandler stores a manifest
func (r *Registry) putManifestHandler(w http.ResponseWriter, req *http.Request) {
    repo := req.PathValue("repo")
    reference := req.PathValue("reference")

    data, err := io.ReadAll(req.Body)
    if err != nil {
        http.Error(w, "failed to read request body", http.StatusBadRequest)
        return
    }

    // Store manifest
    m, err := r.images.Images.Store(context.Background(), data)
    if err != nil {
        http.Error(w, "failed to store manifest", http.StatusInternalServerError)
        return
    }

    // If reference is a tag (not a digest), create tag mapping
    if _, err := hash.Parse(reference); err != nil {
        if err := r.writeTag(repo, reference, m.Digest); err != nil {
            http.Error(w, "failed to write tag", http.StatusInternalServerError)
            return
        }
    }

    w.Header().Set("Docker-Content-Digest", m.Digest.String())
    w.WriteHeader(http.StatusCreated)
}

// deleteManifestHandler removes a manifest
func (r *Registry) deleteManifestHandler(w http.ResponseWriter, req *http.Request) {
    repo := req.PathValue("repo")
    reference := req.PathValue("reference")

    d, err := r.resolveReference(repo, reference)
    if err != nil {
        http.Error(w, "manifest not found", http.StatusNotFound)
        return
    }

    // Delete manifest metadata
    err = r.images.Images.Delete(context.Background(), d)
    if err != nil {
        if errors.Is(err, image.ErrNotFound) {
            http.Error(w, "manifest not found", http.StatusNotFound)
            return
        }
        http.Error(w, "failed to delete manifest", http.StatusInternalServerError)
        return
    }

    // If deleting by tag, also remove the tag mapping
    if _, err := hash.Parse(reference); err != nil {
        _ = r.deleteTag(repo, reference)
    }

    w.WriteHeader(http.StatusAccepted)
}

// ============================================================================
// TAG MANAGEMENT HANDLERS
// ============================================================================

// listTagsHandler returns tags for a repository
func (r *Registry) listTagsHandler(w http.ResponseWriter, req *http.Request) {
    repo := req.PathValue("repo")

    tags, err := r.listTags(repo)
    if err != nil {
        http.Error(w, "failed to list tags", http.StatusInternalServerError)
        return
    }

    result := map[string]interface{}{
        "name": repo,
        "tags": tags,
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(result)
}

// ============================================================================
// TAG MANAGEMENT HELPERS
// ============================================================================

// resolveReference resolves a reference (tag or digest) to a digest
func (r *Registry) resolveReference(repo, reference string) (hash.Digest, error) {
    // Try parsing as digest first
    if d, err := hash.Parse(reference); err == nil {
        return d, nil
    }

    // Try resolving as tag
    d, err := r.readTag(repo, reference)
    if err == nil {
        return d, nil
    }

    return hash.Digest{}, errors.New("reference not found")
}

// writeTag creates a tag mapping
func (r *Registry) writeTag(repo, tag string, digest hash.Digest) error {
    dir := filepath.Join(r.cfg.ImageMetadataDir(), "repos", repo, "tags")
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }

    path := filepath.Join(dir, tag+".json")
    data, _ := json.Marshal(map[string]string{"digest": digest.String()})

    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, data, 0o644); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}

// readTag retrieves a tag mapping
func (r *Registry) readTag(repo, tag string) (hash.Digest, error) {
    path := filepath.Join(r.cfg.ImageMetadataDir(), "repos", repo, "tags", tag+".json")
    data, err := os.ReadFile(path)
    if err != nil {
        return hash.Digest{}, err
    }

    var m map[string]string
    if err := json.Unmarshal(data, &m); err != nil {
        return hash.Digest{}, err
    }

    digestStr, ok := m["digest"]
    if !ok {
        return hash.Digest{}, errors.New("digest not in tag file")
    }

    return hash.Parse(digestStr)
}

// deleteTag removes a tag mapping
func (r *Registry) deleteTag(repo, tag string) error {
    path := filepath.Join(r.cfg.ImageMetadataDir(), "repos", repo, "tags", tag+".json")
    return os.Remove(path)
}

// listTags lists all tags in a repository
func (r *Registry) listTags(repo string) ([]string, error) {
    dir := filepath.Join(r.cfg.ImageMetadataDir(), "repos", repo, "tags")
    entries, err := os.ReadDir(dir)
    if err != nil {
        if os.IsNotExist(err) {
            return []string{}, nil
        }
        return nil, err
    }

    var tags []string
    for _, entry := range entries {
        if entry.IsDir() {
            continue
        }
        name := entry.Name()
        if strings.HasSuffix(name, ".json") {
            tag := strings.TrimSuffix(name, ".json")
            tags = append(tags, tag)
        }
    }

    return tags, nil
}


// bytesReader wraps a []byte as io.Reader to satisfy BlobStore.Put.
type bytesReader []byte

func (b bytesReader) Read(p []byte) (int, error) {
    if len(b) == 0 {
        return 0, io.EOF
    }
    n := copy(p, b)
    b = b[n:]
    return n, nil
}
