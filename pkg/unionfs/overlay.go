package unionfs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"cloudforge/internal/config"
	"cloudforge/pkg/layer"
)

// Manager implements MountManager using a userspace overlay union filesystem.
type Manager struct {
	cfg    *config.Config
	mu     sync.Mutex
	mounts map[string]*overlayHandle
}

// NewManager creates a union filesystem mount manager.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		cfg:    cfg,
		mounts: make(map[string]*overlayHandle),
	}
}

// Mount creates a union filesystem from read-only layers and a writable upper layer.
func (m *Manager) Mount(ctx context.Context, opts MountOptions) (Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	id := opts.ID
	if id == "" {
		var err error
		id, err = randomID()
		if err != nil {
			return nil, err
		}
	}

	mountRoot := opts.MountRoot
	if mountRoot == "" {
		mountRoot = filepath.Join(m.cfg.MountsDir(), id)
	}

	writable := opts.WritableLayer
	if writable == "" {
		writable = filepath.Join(mountRoot, "upper")
	}

	merged := filepath.Join(mountRoot, "merged")
	work := filepath.Join(mountRoot, "work")

	if err := os.MkdirAll(work, 0o755); err != nil {
		return nil, fmt.Errorf("unionfs: create work dir: %w", err)
	}
	if err := os.MkdirAll(writable, 0o755); err != nil {
		return nil, fmt.Errorf("unionfs: create writable layer: %w", err)
	}

	readOnly := append([]string(nil), opts.ReadOnlyLayers...)
	handle := &overlayHandle{
		id:        id,
		mountRoot: mountRoot,
		merged:    merged,
		writable:  writable,
		readOnly:  readOnly,
		manager:   m,
	}

	if err := handle.Refresh(); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.mounts[id]; exists {
		return nil, ErrAlreadyMounted
	}
	m.mounts[id] = handle
	return handle, nil
}

// MountChain mounts a layer chain by resolving each layer to an unpacked rootfs path.
func (m *Manager) MountChain(ctx context.Context, chain layer.Chain, resolver RootfsResolver, opts MountOptions) (Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if resolver == nil {
		return nil, fmt.Errorf("unionfs: rootfs resolver is required")
	}

	readOnly := make([]string, 0, len(chain.Layers))
	for _, lyr := range chain.Layers {
		rootfs, err := resolver.RootfsPath(ctx, lyr)
		if err != nil {
			return nil, err
		}
		readOnly = append(readOnly, rootfs)
	}

	opts.ReadOnlyLayers = readOnly
	return m.Mount(ctx, opts)
}

// Unmount removes a mount and its merged view from the manager.
func (m *Manager) Unmount(ctx context.Context, handle Handle) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	h, ok := handle.(*overlayHandle)
	if !ok {
		return fmt.Errorf("unionfs: unknown handle type")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.mounts[h.id]; !exists {
		return ErrNotMounted
	}
	delete(m.mounts, h.id)
	return nil
}

type overlayHandle struct {
	id        string
	mountRoot string
	merged    string
	writable  string
	readOnly  []string
	manager   *Manager
}

func (h *overlayHandle) ID() string { return h.id }

func (h *overlayHandle) MergedPath() string { return h.merged }

func (h *overlayHandle) WritablePath() string { return h.writable }

func (h *overlayHandle) ReadOnlyLayers() []string {
	out := make([]string, len(h.readOnly))
	copy(out, h.readOnly)
	return out
}

func (h *overlayHandle) CopyUp(relPath string) error {
	if err := copyUp(h.writable, h.readOnly, relPath); err != nil {
		return err
	}
	return h.Refresh()
}

func (h *overlayHandle) Remove(relPath string) error {
	relPath, err := normalizeRel(relPath)
	if err != nil {
		return err
	}

	writablePath := filepath.Join(h.writable, relPath)
	if err := os.RemoveAll(writablePath); err != nil {
		return fmt.Errorf("unionfs: remove writable path: %w", err)
	}

	dirRel := filepath.Dir(relPath)
	whiteoutDir := h.writable
	if dirRel != "." {
		whiteoutDir = filepath.Join(h.writable, dirRel)
	}
	if err := CreateWhiteout(whiteoutDir, filepath.Base(relPath)); err != nil {
		return fmt.Errorf("unionfs: create whiteout: %w", err)
	}

	return h.Refresh()
}

func (h *overlayHandle) Refresh() error {
	return buildMergedView(h.merged, h.readOnly, h.writable)
}

func randomID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("unionfs: generate mount id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
