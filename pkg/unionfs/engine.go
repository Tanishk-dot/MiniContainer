package unionfs

import (
	"context"

	"cloudforge/internal/config"
	"cloudforge/pkg/layer"
)

// Engine bundles union filesystem mounting with optional layer integration.
type Engine struct {
	Mounts   *Manager
	Resolver RootfsResolver
}

// NewEngine creates a union filesystem engine from configuration.
func NewEngine(cfg *config.Config) (*Engine, error) {
	if err := cfg.EnsureDirs(); err != nil {
		return nil, err
	}
	return &Engine{
		Mounts:   NewManager(cfg),
		Resolver: NewConfigRootfsResolver(cfg),
	}, nil
}

// MountLayerChain mounts a content-addressable layer chain using unpacked rootfs paths.
func (e *Engine) MountLayerChain(ctx context.Context, chain layer.Chain, opts MountOptions) (Handle, error) {
	return e.Mounts.MountChain(ctx, chain, e.Resolver, opts)
}
