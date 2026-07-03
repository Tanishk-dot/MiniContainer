package layer

import (
	"context"
	"testing"

	"cloudforge/internal/config"
	"cloudforge/pkg/hash"
	"cloudforge/pkg/storage"
)

func newTestManager(t *testing.T) Manager {
	t.Helper()
	root := t.TempDir()
	cfg := config.WithRootDir(root)
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error = %v", err)
	}

	meta := storage.NewFileBlobMetadataStore(cfg.BlobMetadataDir())
	blobs := storage.NewLocalBlobStore(cfg.BlobsDir(), meta)
	return NewManager(blobs, NewFileLayerMetadataStore(cfg.LayerMetadataDir()))
}

func TestStoreAndGetLayer(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	content := []byte("layer tarball bytes")

	layer, err := mgr.Store(ctx, content, nil)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	expectedID := hash.FromBytes(content)
	if layer.ID != expectedID {
		t.Fatalf("Store() ID = %q, want %q", layer.ID, expectedID)
	}
	if layer.ParentID != nil {
		t.Fatal("Store() ParentID should be nil for base layer")
	}
	if layer.Size != int64(len(content)) {
		t.Fatalf("Store() Size = %d, want %d", layer.Size, len(content))
	}
	if layer.MediaType != MediaTypeLayer {
		t.Fatalf("Store() MediaType = %q, want %q", layer.MediaType, MediaTypeLayer)
	}

	got, err := mgr.Get(ctx, layer.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != layer.ID {
		t.Fatalf("Get() ID = %q, want %q", got.ID, layer.ID)
	}
}

func TestStoreWithParent(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	base, err := mgr.Store(ctx, []byte("base layer"), nil)
	if err != nil {
		t.Fatalf("Store(base) error = %v", err)
	}

	child, err := mgr.Store(ctx, []byte("child layer"), &base.ID)
	if err != nil {
		t.Fatalf("Store(child) error = %v", err)
	}

	if child.ParentID == nil {
		t.Fatal("child ParentID is nil")
	}
	if *child.ParentID != base.ID {
		t.Fatalf("child ParentID = %q, want %q", *child.ParentID, base.ID)
	}
}

func TestStoreDeduplicatesMetadata(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	content := []byte("shared layer content")

	l1, err := mgr.Store(ctx, content, nil)
	if err != nil {
		t.Fatalf("first Store() error = %v", err)
	}
	l2, err := mgr.Store(ctx, content, nil)
	if err != nil {
		t.Fatalf("second Store() error = %v", err)
	}

	if l1.ID != l2.ID {
		t.Fatalf("layer IDs differ: %q vs %q", l1.ID, l2.ID)
	}

	list, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}
}

func TestChain(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	l1, err := mgr.Store(ctx, []byte("chain-layer-1"), nil)
	if err != nil {
		t.Fatalf("Store(l1) error = %v", err)
	}
	l2, err := mgr.Store(ctx, []byte("chain-layer-2"), &l1.ID)
	if err != nil {
		t.Fatalf("Store(l2) error = %v", err)
	}
	l3, err := mgr.Store(ctx, []byte("chain-layer-3"), &l2.ID)
	if err != nil {
		t.Fatalf("Store(l3) error = %v", err)
	}

	chain, err := mgr.Chain(ctx, []hash.Digest{l1.ID, l2.ID, l3.ID})
	if err != nil {
		t.Fatalf("Chain() error = %v", err)
	}
	if len(chain.Layers) != 3 {
		t.Fatalf("Chain() len = %d, want 3", len(chain.Layers))
	}
	if chain.Layers[0].ID != l1.ID || chain.Layers[2].ID != l3.ID {
		t.Fatal("Chain() layer order incorrect")
	}
}

func TestGetNotFound(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	missing := hash.FromBytes([]byte("missing layer"))
	_, err := mgr.Get(ctx, missing)
	if err != ErrNotFound {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestDeleteLayer(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	layer, err := mgr.Store(ctx, []byte("delete me"), nil)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	if err := mgr.Delete(ctx, layer.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = mgr.Get(ctx, layer.ID)
	if err != ErrNotFound {
		t.Fatalf("Get() after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestLayerEngine(t *testing.T) {
	root := t.TempDir()
	cfg := config.WithRootDir(root)

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}

	ctx := context.Background()
	layer, err := engine.Layers.Store(ctx, []byte("engine layer"), nil)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	exists, err := engine.Storage.Blobs.Exists(ctx, layer.ID)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Fatal("underlying blob not stored")
	}
}
