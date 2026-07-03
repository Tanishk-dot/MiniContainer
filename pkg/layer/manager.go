package layer

import (
	"bytes"
	"context"
	"time"

	"cloudforge/pkg/hash"
	"cloudforge/pkg/storage"
)

// LayerMetadataStore persists layer metadata for lookup and inventory.
type LayerMetadataStore interface {
	Save(ctx context.Context, layer *Layer) error
	Get(ctx context.Context, id hash.Digest) (*Layer, error)
	List(ctx context.Context) ([]*Layer, error)
	Delete(ctx context.Context, id hash.Digest) error
}

// Manager provides layer registration and lookup over content-addressable storage.
type Manager interface {
	Store(ctx context.Context, content []byte, parent *hash.Digest) (*Layer, error)
	Get(ctx context.Context, id hash.Digest) (*Layer, error)
	List(ctx context.Context) ([]*Layer, error)
	Chain(ctx context.Context, ids []hash.Digest) (Chain, error)
	Delete(ctx context.Context, id hash.Digest) error
}

// layerManager implements Manager using blob storage and a metadata store.
type layerManager struct {
	blobs storage.BlobStore
	store LayerMetadataStore
}

// NewManager creates a layer manager backed by blob storage and metadata tracking.
func NewManager(blobs storage.BlobStore, store LayerMetadataStore) Manager {
	return &layerManager{
		blobs: blobs,
		store: store,
	}
}

// Store writes layer content to blob storage and records layer metadata.
// The layer ID is the SHA256 digest of the content (content-addressable layer ID).
func (m *layerManager) Store(ctx context.Context, content []byte, parent *hash.Digest) (*Layer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if parent != nil {
		if err := parent.Validate(); err != nil {
			return nil, ErrInvalidLayer
		}
	}

	digest, size, err := m.blobs.Put(ctx, bytes.NewReader(content))
	if err != nil {
		return nil, err
	}

	layer := &Layer{
		ID:        digest,
		ParentID:  parent,
		Size:      size,
		MediaType: MediaTypeLayer,
		CreatedAt: time.Now().UTC(),
	}
	if err := layer.Validate(); err != nil {
		return nil, err
	}

	if existing, getErr := m.store.Get(ctx, digest); getErr == nil {
		return existing, nil
	} else if getErr != ErrNotFound {
		return nil, getErr
	}

	if err := m.store.Save(ctx, layer); err != nil {
		return nil, err
	}
	return layer, nil
}

// Get retrieves a layer by its SHA256 ID.
func (m *layerManager) Get(ctx context.Context, id hash.Digest) (*Layer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := id.Validate(); err != nil {
		return nil, ErrInvalidLayer
	}
	return m.store.Get(ctx, id)
}

// List returns all registered layers.
func (m *layerManager) List(ctx context.Context) ([]*Layer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return m.store.List(ctx)
}

// Chain resolves an ordered list of layer IDs into a Layer chain.
func (m *layerManager) Chain(ctx context.Context, ids []hash.Digest) (Chain, error) {
	if err := ctx.Err(); err != nil {
		return Chain{}, err
	}
	if len(ids) == 0 {
		return Chain{}, nil
	}

	layers := make([]*Layer, 0, len(ids))
	for _, id := range ids {
		layer, err := m.Get(ctx, id)
		if err != nil {
			return Chain{}, err
		}
		layers = append(layers, layer)
	}
	return Chain{Layers: layers}, nil
}

// Delete removes layer metadata. Blob bytes are not deleted (shared CAS).
func (m *layerManager) Delete(ctx context.Context, id hash.Digest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := id.Validate(); err != nil {
		return ErrInvalidLayer
	}
	return m.store.Delete(ctx, id)
}
