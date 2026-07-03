package unionfs

import (
	"context"
	"fmt"
	"os"

	"cloudforge/internal/config"
	"cloudforge/pkg/layer"
)

// ConfigRootfsResolver resolves layers to unpacked rootfs directories under config.LayersDir().
type ConfigRootfsResolver struct {
	cfg *config.Config
}

// NewConfigRootfsResolver creates a resolver using the standard layer rootfs layout.
func NewConfigRootfsResolver(cfg *config.Config) *ConfigRootfsResolver {
	return &ConfigRootfsResolver{cfg: cfg}
}

// RootfsPath returns the unpacked rootfs directory for a layer.
func (r *ConfigRootfsResolver) RootfsPath(_ context.Context, lyr *layer.Layer) (string, error) {
	if lyr == nil {
		return "", fmt.Errorf("unionfs: layer is nil")
	}
	path := r.cfg.LayerRootfsPath(lyr.ID.Hex)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("unionfs: layer rootfs not found at %s", path)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("unionfs: layer rootfs is not a directory: %s", path)
	}
	return path, nil
}
