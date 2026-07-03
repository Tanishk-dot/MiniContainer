package layer

import (
	"cloudforge/internal/config"
	"cloudforge/pkg/storage"
)

// Engine bundles layer management with underlying blob storage.
type Engine struct {
	Layers  Manager
	Storage *storage.Engine
}

// NewEngine creates a layer engine wired to a storage engine.
func NewEngine(cfg *config.Config) (*Engine, error) {
	store, err := storage.NewEngine(cfg)
	if err != nil {
		return nil, err
	}

	meta := NewFileLayerMetadataStore(cfg.LayerMetadataDir())
	manager := NewManager(store.Blobs, meta)

	return &Engine{
		Layers:  manager,
		Storage: store,
	}, nil
}
