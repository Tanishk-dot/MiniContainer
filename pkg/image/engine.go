package image

import (
    "cloudforge/internal/config"
    "cloudforge/pkg/storage"
)

// Engine bundles image manifest management with underlying blob storage.
type Engine struct {
    Images  Manager
    Storage *storage.Engine
}

// NewEngine creates an image engine wired to a storage engine.
func NewEngine(cfg *config.Config) (*Engine, error) {
    store, err := storage.NewEngine(cfg)
    if err != nil {
        return nil, err
    }

    meta := NewFileImageMetadataStore(cfg.ImageMetadataDir())
    manager := NewManager(store.Blobs, meta)

    return &Engine{
        Images:  manager,
        Storage: store,
    }, nil
}
