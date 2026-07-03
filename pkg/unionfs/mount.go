package unionfs

import (
	"context"

	"cloudforge/pkg/layer"
)

// Handle exposes a mounted union filesystem view.
type Handle interface {
	ID() string
	// MergedPath returns the unified filesystem root for reads and writes.
	MergedPath() string
	// WritablePath returns the writable (upper) layer root.
	WritablePath() string
	// ReadOnlyLayers returns read-only layer roots from bottom to top.
	ReadOnlyLayers() []string
	// CopyUp copies a path from lower layers into the writable layer if needed.
	CopyUp(relPath string) error
	// Remove deletes a path from the union view using a whiteout when required.
	Remove(relPath string) error
	// Refresh rebuilds the merged view from layers and whiteouts.
	Refresh() error
}

// MountOptions configures a union filesystem mount.
type MountOptions struct {
	// ID uniquely identifies the mount. A random ID is generated when empty.
	ID string
	// ReadOnlyLayers are layer root directories ordered from bottom to top.
	ReadOnlyLayers []string
	// WritableLayer is the upper writable layer directory. Created when empty.
	WritableLayer string
	// MountRoot is the parent directory for merged/upper/work dirs. Created when empty.
	MountRoot string
}

// MountManager mounts and unmounts union filesystems.
type MountManager interface {
	Mount(ctx context.Context, opts MountOptions) (Handle, error)
	MountChain(ctx context.Context, chain layer.Chain, resolver RootfsResolver, opts MountOptions) (Handle, error)
	Unmount(ctx context.Context, handle Handle) error
}

// RootfsResolver maps a layer to its unpacked rootfs directory on disk.
type RootfsResolver interface {
	RootfsPath(ctx context.Context, layer *layer.Layer) (string, error)
}
