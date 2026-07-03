package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloudforge/internal/config"
	"cloudforge/pkg/hash"
)

func setupTestRegistry(t *testing.T) *Registry {
	cfg := &config.Config{}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("failed to ensure dirs: %v", err)
	}

	reg, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	return reg
}

func TestCheckAPIHandler(t *testing.T) {
	reg := setupTestRegistry(t)
	req := httptest.NewRequest("GET", "/v2/", nil)
	w := httptest.NewRecorder()

	reg.checkAPIHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if v := w.Header().Get("Docker-Distribution-API-Version"); v != "2.0" {
		t.Errorf("expected API version 2.0, got %q", v)
	}
}

func TestBlobPushPull(t *testing.T) {
	reg := setupTestRegistry(t)

	// Create test blob data
	testData := []byte("hello world")
	digest := hash.FromBytes(testData)

	// Test PUT blob
	putReq := httptest.NewRequest("PUT", fmt.Sprintf("/v2/test/blobs/%s", digest.String()), bytes.NewReader(testData))
	putReq.SetPathValue("repo", "test")
	putReq.SetPathValue("digest", digest.String())
	w := httptest.NewRecorder()

	reg.putBlobHandler(w, putReq)
	if w.Code != http.StatusCreated {
		t.Errorf("PUT blob: expected status 201, got %d", w.Code)
	}

	// Test GET blob
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v2/test/blobs/%s", digest.String()), nil)
	getReq.SetPathValue("repo", "test")
	getReq.SetPathValue("digest", digest.String())
	w = httptest.NewRecorder()

	reg.getBlobHandler(w, getReq)
	if w.Code != http.StatusOK {
		t.Errorf("GET blob: expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Docker-Content-Digest") != digest.String() {
		t.Errorf("expected digest %s, got %s", digest.String(), w.Header().Get("Docker-Content-Digest"))
	}

	retrieved := w.Body.Bytes()
	if !bytes.Equal(retrieved, testData) {
		t.Errorf("blob content mismatch: expected %s, got %s", string(testData), string(retrieved))
	}
}

func TestBlobHeadCheck(t *testing.T) {
	reg := setupTestRegistry(t)

	// Create and push a test blob
	testData := []byte("test data for head check")
	digest := hash.FromBytes(testData)

	putReq := httptest.NewRequest("PUT", fmt.Sprintf("/v2/test/blobs/%s", digest.String()), bytes.NewReader(testData))
	putReq.SetPathValue("repo", "test")
	putReq.SetPathValue("digest", digest.String())
	w := httptest.NewRecorder()
	reg.putBlobHandler(w, putReq)

	// Test HEAD blob
	headReq := httptest.NewRequest("HEAD", fmt.Sprintf("/v2/test/blobs/%s", digest.String()), nil)
	headReq.SetPathValue("repo", "test")
	headReq.SetPathValue("digest", digest.String())
	w = httptest.NewRecorder()

	reg.headBlobHandler(w, headReq)
	if w.Code != http.StatusOK {
		t.Errorf("HEAD blob: expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Docker-Content-Digest") != digest.String() {
		t.Errorf("expected digest header %s, got %s", digest.String(), w.Header().Get("Docker-Content-Digest"))
	}
}

func TestBlobDigestMismatch(t *testing.T) {
	reg := setupTestRegistry(t)

	testData := []byte("hello world")
	wrongDigest := hash.FromBytes([]byte("wrong data"))

	putReq := httptest.NewRequest("PUT", fmt.Sprintf("/v2/test/blobs/%s", wrongDigest.String()), bytes.NewReader(testData))
	putReq.SetPathValue("repo", "test")
	putReq.SetPathValue("digest", wrongDigest.String())
	w := httptest.NewRecorder()

	reg.putBlobHandler(w, putReq)
	if w.Code != http.StatusBadRequest {
		t.Errorf("PUT with mismatched digest: expected status 400, got %d", w.Code)
	}
}

func TestManifestPushPull(t *testing.T) {
	reg := setupTestRegistry(t)

	// Create a test manifest
	manifest := map[string]interface{}{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.v2+json",
		"config": map[string]interface{}{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"digest":    hash.FromBytes([]byte("config")).String(),
		},
		"layers": []map[string]interface{}{
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"digest":    hash.FromBytes([]byte("layer1")).String(),
			},
		},
	}

	manifestJSON, _ := json.Marshal(manifest)

	// Test PUT manifest
	putReq := httptest.NewRequest("PUT", "/v2/test/manifests/latest", bytes.NewReader(manifestJSON))
	putReq.SetPathValue("repo", "test")
	putReq.SetPathValue("reference", "latest")
	w := httptest.NewRecorder()

	reg.putManifestHandler(w, putReq)
	if w.Code != http.StatusCreated {
		t.Errorf("PUT manifest: expected status 201, got %d", w.Code)
	}

	digest := w.Header().Get("Docker-Content-Digest")
	if digest == "" {
		t.Error("PUT manifest: expected Docker-Content-Digest header")
	}

	// Test GET manifest by tag
	getReq := httptest.NewRequest("GET", "/v2/test/manifests/latest", nil)
	getReq.SetPathValue("repo", "test")
	getReq.SetPathValue("reference", "latest")
	w = httptest.NewRecorder()

	reg.getManifestHandler(w, getReq)
	if w.Code != http.StatusOK {
		t.Errorf("GET manifest by tag: expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Docker-Content-Digest") != digest {
		t.Errorf("expected digest %s, got %s", digest, w.Header().Get("Docker-Content-Digest"))
	}

	// Test GET manifest by digest
	getReq = httptest.NewRequest("GET", fmt.Sprintf("/v2/test/manifests/%s", digest), nil)
	getReq.SetPathValue("repo", "test")
	getReq.SetPathValue("reference", digest)
	w = httptest.NewRecorder()

	reg.getManifestHandler(w, getReq)
	if w.Code != http.StatusOK {
		t.Errorf("GET manifest by digest: expected status 200, got %d", w.Code)
	}
}

func TestTagManagement(t *testing.T) {
	reg := setupTestRegistry(t)

	// Write a tag
	testDigest := hash.FromBytes([]byte("test manifest"))
	if err := reg.writeTag("myrepo", "v1.0", testDigest); err != nil {
		t.Fatalf("failed to write tag: %v", err)
	}

	// Read the tag back
	retrieved, err := reg.readTag("myrepo", "v1.0")
	if err != nil {
		t.Fatalf("failed to read tag: %v", err)
	}

	if retrieved.Hex != testDigest.Hex {
		t.Errorf("tag digest mismatch: expected %s, got %s", testDigest.Hex, retrieved.Hex)
	}

	// List tags
	tags, err := reg.listTags("myrepo")
	if err != nil {
		t.Fatalf("failed to list tags: %v", err)
	}

	found := false
	for _, tag := range tags {
		if tag == "v1.0" {
			found = true
			break
		}
	}
	if !found {
		t.Error("tag v1.0 not found in list")
	}

	// Delete tag
	if err := reg.deleteTag("myrepo", "v1.0"); err != nil {
		t.Fatalf("failed to delete tag: %v", err)
	}

	// Verify it's gone
	tags, err = reg.listTags("myrepo")
	if err != nil {
		t.Fatalf("failed to list tags after delete: %v", err)
	}

	found = false
	for _, tag := range tags {
		if tag == "v1.0" {
			found = true
			break
		}
	}
	if found {
		t.Error("tag v1.0 still found after deletion")
	}
}

func TestChunkedUpload(t *testing.T) {
	reg := setupTestRegistry(t)

	// Start upload session
	startReq := httptest.NewRequest("POST", "/v2/test/blobs/uploads/", nil)
	startReq.SetPathValue("repo", "test")
	w := httptest.NewRecorder()

	reg.startUploadHandler(w, startReq)
	if w.Code != http.StatusAccepted {
		t.Errorf("start upload: expected status 202, got %d", w.Code)
	}

	uploadURL := w.Header().Get("Location")
	if uploadURL == "" {
		t.Fatal("start upload: expected Location header")
	}

	// Extract upload ID from URL
	uploadID := uploadURL[len("/v2/test/blobs/uploads/"):]

	// Upload chunk
	chunk1 := []byte("hello ")
	patchReq := httptest.NewRequest("PATCH", uploadURL, bytes.NewReader(chunk1))
	patchReq.SetPathValue("repo", "test")
	patchReq.SetPathValue("uuid", uploadID)
	w = httptest.NewRecorder()

	reg.uploadChunkHandler(w, patchReq)
	if w.Code != http.StatusAccepted {
		t.Errorf("upload chunk: expected status 202, got %d", w.Code)
	}

	// Complete upload with second chunk
	chunk2 := []byte("world")
	completeData := append(chunk1, chunk2...)
	digest := hash.FromBytes(completeData)

	completeReq := httptest.NewRequest("PUT", fmt.Sprintf("%s?digest=%s", uploadURL, digest.String()), bytes.NewReader(chunk2))
	completeReq.SetPathValue("repo", "test")
	completeReq.SetPathValue("uuid", uploadID)
	w = httptest.NewRecorder()

	reg.completeUploadHandler(w, completeReq)
	if w.Code != http.StatusCreated {
		t.Errorf("complete upload: expected status 201, got %d", w.Code)
	}

	// Verify blob was stored
	getReq := httptest.NewRequest("GET", fmt.Sprintf("/v2/test/blobs/%s", digest.String()), nil)
	getReq.SetPathValue("repo", "test")
	getReq.SetPathValue("digest", digest.String())
	w = httptest.NewRecorder()

	reg.getBlobHandler(w, getReq)
	if w.Code != http.StatusOK {
		t.Errorf("GET uploaded blob: expected status 200, got %d", w.Code)
	}

	if !bytes.Equal(w.Body.Bytes(), completeData) {
		t.Errorf("uploaded blob content mismatch")
	}
}

func TestResolveReference(t *testing.T) {
	reg := setupTestRegistry(t)

	// Create and store a manifest
	manifest := []byte(`{"schemaVersion": 2, "mediaType": "application/vnd.docker.distribution.manifest.v2+json"}`)
	_, err := reg.images.Images.Store(context.Background(), manifest)
	if err != nil {
		t.Fatalf("failed to store manifest: %v", err)
	}

	manifestDigest := hash.FromBytes(manifest)

	// Create tag mapping
	if err := reg.writeTag("test", "latest", manifestDigest); err != nil {
		t.Fatalf("failed to write tag: %v", err)
	}

	// Resolve by digest
	d, err := reg.resolveReference("test", manifestDigest.String())
	if err != nil {
		t.Fatalf("resolve by digest failed: %v", err)
	}
	if d.Hex != manifestDigest.Hex {
		t.Errorf("resolved digest mismatch: expected %s, got %s", manifestDigest.Hex, d.Hex)
	}

	// Resolve by tag
	d, err = reg.resolveReference("test", "latest")
	if err != nil {
		t.Fatalf("resolve by tag failed: %v", err)
	}
	if d.Hex != manifestDigest.Hex {
		t.Errorf("resolved tag digest mismatch: expected %s, got %s", manifestDigest.Hex, d.Hex)
	}
}

func TestListTags(t *testing.T) {
	reg := setupTestRegistry(t)

	// Create some test manifests and tags
	digest1 := hash.FromBytes([]byte("manifest1"))
	digest2 := hash.FromBytes([]byte("manifest2"))
	digest3 := hash.FromBytes([]byte("manifest3"))

	reg.writeTag("test", "v1.0", digest1)
	reg.writeTag("test", "v2.0", digest2)
	reg.writeTag("test", "latest", digest3)

	// List tags
	tags, err := reg.listTags("test")
	if err != nil {
		t.Fatalf("failed to list tags: %v", err)
	}

	if len(tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(tags))
	}

	expectedTags := map[string]bool{"v1.0": false, "v2.0": false, "latest": false}
	for _, tag := range tags {
		if _, exists := expectedTags[tag]; exists {
			expectedTags[tag] = true
		}
	}

	for tag, found := range expectedTags {
		if !found {
			t.Errorf("expected tag %s not found", tag)
		}
	}
}
