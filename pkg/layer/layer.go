package layer

import (
	"errors"
	"time"

	"cloudforge/pkg/hash"
)

const (
	// MediaTypeLayer is the default media type for filesystem layer tarballs.
	MediaTypeLayer = "application/vnd.cloudforge.layer.v1+tar"
)

var (
	// ErrNotFound is returned when a layer does not exist.
	ErrNotFound = errors.New("layer: not found")
	// ErrInvalidLayer is returned when layer metadata fails validation.
	ErrInvalidLayer = errors.New("layer: invalid layer")
)

// Layer represents an immutable filesystem layer identified by SHA256 content hash.
// The layer ID equals the digest of the layer tarball stored in blob storage.
type Layer struct {
	ID        hash.Digest
	ParentID  *hash.Digest
	Size      int64
	MediaType string
	CreatedAt time.Time
}

// Validate checks layer invariants.
func (l *Layer) Validate() error {
	if err := l.ID.Validate(); err != nil {
		return ErrInvalidLayer
	}
	if l.ParentID != nil {
		if err := l.ParentID.Validate(); err != nil {
			return ErrInvalidLayer
		}
	}
	if l.Size < 0 {
		return ErrInvalidLayer
	}
	if l.MediaType == "" {
		return ErrInvalidLayer
	}
	return nil
}

// Chain represents an ordered stack of layers from base to top.
type Chain struct {
	Layers []*Layer
}

// IDs returns layer digests in order from bottom to top.
func (c Chain) IDs() []hash.Digest {
	ids := make([]hash.Digest, len(c.Layers))
	for i, layer := range c.Layers {
		ids[i] = layer.ID
	}
	return ids
}
