package unionfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cloudforge/internal/config"
	"cloudforge/pkg/hash"
	"cloudforge/pkg/layer"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func readMergedFile(t *testing.T, handle Handle, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(handle.MergedPath(), rel))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", rel, err)
	}
	return string(data)
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return NewManager(config.WithRootDir(t.TempDir()))
}

func TestMountSingleReadOnlyLayer(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	layer0 := filepath.Join(t.TempDir(), "layer0")
	writeFile(t, filepath.Join(layer0, "etc", "app.conf"), "version=1")

	handle, err := mgr.Mount(ctx, MountOptions{ReadOnlyLayers: []string{layer0}})
	if err != nil {
		t.Fatalf("Mount() error = %v", err)
	}

	got := readMergedFile(t, handle, filepath.Join("etc", "app.conf"))
	if got != "version=1" {
		t.Fatalf("merged content = %q, want %q", got, "version=1")
	}
}

func TestUpperLayerOverridesLower(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	layer0 := filepath.Join(t.TempDir(), "base")
	layer1 := filepath.Join(t.TempDir(), "top")
	writeFile(t, filepath.Join(layer0, "bin", "run"), "base")
	writeFile(t, filepath.Join(layer1, "bin", "run"), "top")

	handle, err := mgr.Mount(ctx, MountOptions{ReadOnlyLayers: []string{layer0, layer1}})
	if err != nil {
		t.Fatalf("Mount() error = %v", err)
	}

	got := readMergedFile(t, handle, filepath.Join("bin", "run"))
	if got != "top" {
		t.Fatalf("merged content = %q, want %q", got, "top")
	}
}

func TestWhiteoutHidesLowerFile(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	layer0 := filepath.Join(t.TempDir(), "base")
	layer1 := filepath.Join(t.TempDir(), "top")
	writeFile(t, filepath.Join(layer0, "tmp", "secret"), "hidden")
	writeFile(t, filepath.Join(layer1, "tmp", WhiteoutPrefix+"secret"), "")

	handle, err := mgr.Mount(ctx, MountOptions{ReadOnlyLayers: []string{layer0, layer1}})
	if err != nil {
		t.Fatalf("Mount() error = %v", err)
	}

	path := filepath.Join(handle.MergedPath(), "tmp", "secret")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected whiteouted file to be absent, stat err = %v", err)
	}
}

func TestCopyOnWritePromotesLowerFile(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	layer0 := filepath.Join(t.TempDir(), "base")
	writeFile(t, filepath.Join(layer0, "data", "value"), "original")

	handle, err := mgr.Mount(ctx, MountOptions{ReadOnlyLayers: []string{layer0}})
	if err != nil {
		t.Fatalf("Mount() error = %v", err)
	}

	rel := filepath.Join("data", "value")
	if err := handle.CopyUp(rel); err != nil {
		t.Fatalf("CopyUp() error = %v", err)
	}

	writeFile(t, filepath.Join(handle.WritablePath(), rel), "modified")

	if err := handle.Refresh(); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	got := readMergedFile(t, handle, rel)
	if got != "modified" {
		t.Fatalf("merged content = %q, want %q", got, "modified")
	}

	// Lower layer remains unchanged.
	baseContent, err := os.ReadFile(filepath.Join(layer0, rel))
	if err != nil {
		t.Fatalf("ReadFile(base) error = %v", err)
	}
	if string(baseContent) != "original" {
		t.Fatalf("base layer modified by CoW, content = %q", baseContent)
	}
}

func TestWritableLayerNewFile(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	layer0 := filepath.Join(t.TempDir(), "base")
	handle, err := mgr.Mount(ctx, MountOptions{ReadOnlyLayers: []string{layer0}})
	if err != nil {
		t.Fatalf("Mount() error = %v", err)
	}

	rel := filepath.Join("new", "file.txt")
	writeFile(t, filepath.Join(handle.WritablePath(), rel), "fresh")
	if err := handle.Refresh(); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	got := readMergedFile(t, handle, rel)
	if got != "fresh" {
		t.Fatalf("merged content = %q, want %q", got, "fresh")
	}
}

func TestRemoveCreatesWhiteout(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	layer0 := filepath.Join(t.TempDir(), "base")
	writeFile(t, filepath.Join(layer0, "keep.txt"), "stay")
	writeFile(t, filepath.Join(layer0, "remove.txt"), "go")

	handle, err := mgr.Mount(ctx, MountOptions{ReadOnlyLayers: []string{layer0}})
	if err != nil {
		t.Fatalf("Mount() error = %v", err)
	}

	if err := handle.Remove("remove.txt"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(handle.MergedPath(), "remove.txt")); !os.IsNotExist(err) {
		t.Fatalf("removed file still visible in merged view")
	}

	whiteout := filepath.Join(handle.WritablePath(), WhiteoutPrefix+"remove.txt")
	if _, err := os.Stat(whiteout); err != nil {
		t.Fatalf("whiteout not created: %v", err)
	}

	got := readMergedFile(t, handle, "keep.txt")
	if got != "stay" {
		t.Fatalf("keep.txt = %q, want %q", got, "stay")
	}
}

func TestOpaqueDirectoryWhiteout(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	layer0 := filepath.Join(t.TempDir(), "base")
	layer1 := filepath.Join(t.TempDir(), "top")
	writeFile(t, filepath.Join(layer0, "app", "old"), "old-value")
	writeFile(t, filepath.Join(layer1, "app", OpaqueWhiteout), "")
	writeFile(t, filepath.Join(layer1, "app", "new"), "new-value")

	handle, err := mgr.Mount(ctx, MountOptions{ReadOnlyLayers: []string{layer0, layer1}})
	if err != nil {
		t.Fatalf("Mount() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(handle.MergedPath(), "app", "old")); !os.IsNotExist(err) {
		t.Fatalf("opaque dir should hide old file")
	}

	got := readMergedFile(t, handle, filepath.Join("app", "new"))
	if got != "new-value" {
		t.Fatalf("merged new = %q, want %q", got, "new-value")
	}
}

func TestMountAndUnmount(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	layer0 := filepath.Join(t.TempDir(), "base")
	writeFile(t, filepath.Join(layer0, "file"), "data")

	handle, err := mgr.Mount(ctx, MountOptions{ID: "test-mount", ReadOnlyLayers: []string{layer0}})
	if err != nil {
		t.Fatalf("Mount() error = %v", err)
	}
	if handle.ID() != "test-mount" {
		t.Fatalf("handle ID = %q, want %q", handle.ID(), "test-mount")
	}

	if err := mgr.Unmount(ctx, handle); err != nil {
		t.Fatalf("Unmount() error = %v", err)
	}
	if err := mgr.Unmount(ctx, handle); err != ErrNotMounted {
		t.Fatalf("second Unmount() error = %v, want ErrNotMounted", err)
	}
}

func TestMountChainWithResolver(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	cfg := config.WithRootDir(t.TempDir())

	layerA := filepath.Join(cfg.LayersDir(), "aaa", "rootfs")
	layerB := filepath.Join(cfg.LayersDir(), "bbb", "rootfs")
	writeFile(t, filepath.Join(layerA, "shared"), "base")
	writeFile(t, filepath.Join(layerB, "shared"), "override")

	digestA := hash.FromBytes([]byte("layer-a"))
	digestB := hash.FromBytes([]byte("layer-b"))
	chain := layer.Chain{Layers: []*layer.Layer{
		{ID: digestA, MediaType: layer.MediaTypeLayer, CreatedAt: time.Now().UTC()},
		{ID: digestB, ParentID: &digestA, MediaType: layer.MediaTypeLayer, CreatedAt: time.Now().UTC()},
	}}

	resolver := &staticResolver{paths: map[string]string{
		digestA.Hex: layerA,
		digestB.Hex: layerB,
	}}

	handle, err := mgr.MountChain(ctx, chain, resolver, MountOptions{})
	if err != nil {
		t.Fatalf("MountChain() error = %v", err)
	}

	got := readMergedFile(t, handle, "shared")
	if got != "override" {
		t.Fatalf("merged content = %q, want %q", got, "override")
	}
}

type staticResolver struct {
	paths map[string]string
}

func (r *staticResolver) RootfsPath(_ context.Context, lyr *layer.Layer) (string, error) {
	path, ok := r.paths[lyr.ID.Hex]
	if !ok {
		return "", ErrNotFound
	}
	return path, nil
}
