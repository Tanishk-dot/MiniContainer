package storage

import (
	"bytes"
	"context"
	"io"
	"testing"

	"cloudforge/internal/config"
	"cloudforge/pkg/hash"
)

func newTestStore(t *testing.T) *LocalBlobStore {
	t.Helper()
	root := t.TempDir()
	cfg := config.WithRootDir(root)
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error = %v", err)
	}
	meta := NewFileBlobMetadataStore(cfg.BlobMetadataDir())
	return NewLocalBlobStore(cfg.BlobsDir(), meta)
}

func TestPutGetRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("cloudforge blob payload")

	digest, size, err := store.PutBytes(ctx, content)
	if err != nil {
		t.Fatalf("PutBytes() error = %v", err)
	}
	if size != int64(len(content)) {
		t.Fatalf("PutBytes() size = %d, want %d", size, len(content))
	}
	expected := hash.FromBytes(content)
	if digest != expected {
		t.Fatalf("PutBytes() digest = %q, want %q", digest, expected)
	}

	rc, gotSize, err := store.Get(ctx, digest)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if gotSize != int64(len(content)) {
		t.Fatalf("Get() size = %d, want %d", gotSize, len(content))
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("Get() content mismatch")
	}
}

func TestPutDeduplicates(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("deduplicated content")

	d1, size1, err := store.PutBytes(ctx, content)
	if err != nil {
		t.Fatalf("first PutBytes() error = %v", err)
	}
	d2, size2, err := store.PutBytes(ctx, content)
	if err != nil {
		t.Fatalf("second PutBytes() error = %v", err)
	}

	if d1 != d2 {
		t.Fatalf("digests differ: %q vs %q", d1, d2)
	}
	if size1 != size2 {
		t.Fatalf("sizes differ: %d vs %d", size1, size2)
	}

	info, err := store.Stat(ctx, d1)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Size != int64(len(content)) {
		t.Fatalf("Stat() size = %d, want %d", info.Size, len(content))
	}
}

func TestExistsAndNotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	missing := hash.FromBytes([]byte("missing"))
	exists, err := store.Exists(ctx, missing)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Fatal("Exists() = true for missing blob")
	}

	_, _, err = store.Get(ctx, missing)
	if err != ErrNotFound {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}

	digest, _, err := store.PutBytes(ctx, []byte("present"))
	if err != nil {
		t.Fatalf("PutBytes() error = %v", err)
	}

	exists, err = store.Exists(ctx, digest)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Fatal("Exists() = false for stored blob")
	}
}

func TestDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	digest, _, err := store.PutBytes(ctx, []byte("to delete"))
	if err != nil {
		t.Fatalf("PutBytes() error = %v", err)
	}

	if err := store.Delete(ctx, digest); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	exists, err := store.Exists(ctx, digest)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Fatal("blob still exists after Delete()")
	}
}

func TestMetadataList(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	meta := store.meta.(*FileBlobMetadataStore)

	d1, _, err := store.PutBytes(ctx, []byte("blob-one"))
	if err != nil {
		t.Fatalf("PutBytes() error = %v", err)
	}
	d2, _, err := store.PutBytes(ctx, []byte("blob-two"))
	if err != nil {
		t.Fatalf("PutBytes() error = %v", err)
	}

	list, err := meta.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List() len = %d, want 2", len(list))
	}

	found := map[string]bool{d1.String(): false, d2.String(): false}
	for _, info := range list {
		found[info.Digest.String()] = true
	}
	for digest, ok := range found {
		if !ok {
			t.Fatalf("List() missing digest %q", digest)
		}
	}
}

func TestEngineFromConfig(t *testing.T) {
	root := t.TempDir()
	cfg := config.WithRootDir(root)

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	ctx := context.Background()
	digest, _, err := engine.Blobs.Put(ctx, bytes.NewReader([]byte("engine test")))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	blobs, err := engine.ListBlobs(ctx)
	if err != nil {
		t.Fatalf("ListBlobs() error = %v", err)
	}
	if len(blobs) != 1 {
		t.Fatalf("ListBlobs() len = %d, want 1", len(blobs))
	}
	if blobs[0].Digest != digest {
		t.Fatalf("ListBlobs() digest = %q, want %q", blobs[0].Digest, digest)
	}
}

func TestPutStreamingReader(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	content := []byte("streaming reader content")

	digest, size, err := store.Put(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if size != int64(len(content)) {
		t.Fatalf("Put() size = %d, want %d", size, len(content))
	}
	if digest != hash.FromBytes(content) {
		t.Fatalf("Put() digest mismatch")
	}
}
