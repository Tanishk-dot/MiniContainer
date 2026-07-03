package build

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "cloudforge/internal/config"
    "cloudforge/pkg/layer"
    "cloudforge/pkg/storage"
)

func TestBuildEngine_AddCaching(t *testing.T) {
    cfg := config.WithRootDir(filepath.Join(os.TempDir(), "cloudforge-build-test"))
    _ = os.RemoveAll(cfg.RootDir)
    defer os.RemoveAll(cfg.RootDir)

    storeEngine, err := storage.NewEngine(cfg)
    if err != nil {
        t.Fatalf("storage.NewEngine: %v", err)
    }
    lm := layer.NewManager(storeEngine.Blobs, layer.NewFileLayerMetadataStore(cfg.LayerMetadataDir()))
    eng := NewEngine(lm)

    steps := []Step{{Type: StepAdd, Path: "file.txt", Content: []byte("hello")}}

    ctx := context.Background()
    res1, err := eng.Build(ctx, steps)
    if err != nil {
        t.Fatalf("first build: %v", err)
    }
    if len(res1) != 1 {
        t.Fatalf("expected 1 result, got %d", len(res1))
    }

    // Run again; should hit cache and produce same digest
    res2, err := eng.Build(ctx, steps)
    if err != nil {
        t.Fatalf("second build: %v", err)
    }
    if len(res2) != 1 {
        t.Fatalf("expected 1 result, got %d", len(res2))
    }
    if res1[0].ID != res2[0].ID {
        t.Fatalf("cached layer mismatch: %v vs %v", res1[0].ID, res2[0].ID)
    }
}
