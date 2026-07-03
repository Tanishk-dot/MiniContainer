package image

import (
    "context"
    "os"
    "path/filepath"
    "testing"
    "time"

    "cloudforge/pkg/hash"
)

func TestFileImageMetadataStore_SaveGet(t *testing.T) {
    dir := filepath.Join(os.TempDir(), "cloudforge-image-meta-test")
    _ = os.RemoveAll(dir)
    defer os.RemoveAll(dir)

    store := NewFileImageMetadataStore(dir)

    content := []byte("test-manifest")
    digest := hash.FromBytes(content)

    now := time.Now().UTC()
    manifest := &Manifest{
        Digest:       digest,
        SchemaVersion: 1,
        MediaType:    "application/vnd.test.manifest",
        Size:         int64(len(content)),
        CreatedAt:    now,
    }

    ctx := context.Background()
    if err := store.Save(ctx, manifest); err != nil {
        t.Fatalf("save metadata: %v", err)
    }

    got, err := store.Get(ctx, digest)
    if err != nil {
        t.Fatalf("get metadata: %v", err)
    }
    if got.Digest != manifest.Digest {
        t.Fatalf("digest mismatch: got %v want %v", got.Digest, manifest.Digest)
    }
    if got.Size != manifest.Size {
        t.Fatalf("size mismatch: got %d want %d", got.Size, manifest.Size)
    }
    if !got.CreatedAt.Equal(manifest.CreatedAt) {
        t.Fatalf("created at mismatch: got %v want %v", got.CreatedAt, manifest.CreatedAt)
    }
}
