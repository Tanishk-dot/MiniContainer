package storage

import (
	"context"

	"cloudforge/internal/config"
)

// Engine bundles a content-addressable blob store with metadata tracking.
type Engine struct {
	Blobs    BlobStore
	Metadata BlobMetadataStore
}

// NewEngine creates a fully wired storage engine from configuration.
func NewEngine(cfg *config.Config) (*Engine, error) {
	if err := cfg.EnsureDirs(); err != nil {
		return nil, err
	}

	meta := NewFileBlobMetadataStore(cfg.BlobMetadataDir())
	blobs := NewLocalBlobStore(cfg.BlobsDir(), meta)

	return &Engine{
		Blobs:    blobs,
		Metadata: meta,
	}, nil
}

// ListBlobs returns all stored blob metadata records.
func (e *Engine) ListBlobs(ctx context.Context) ([]BlobInfo, error) {
	return e.Metadata.List(ctx)
}
