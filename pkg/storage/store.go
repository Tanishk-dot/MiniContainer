package storage

import (
	"context"
	"io"
	"time"

	"cloudforge/pkg/hash"
)

// BlobInfo describes a stored content-addressed blob.
type BlobInfo struct {
	Digest    hash.Digest
	Size      int64
	CreatedAt time.Time
}

// BlobStore provides content-addressable blob storage keyed by SHA256 digest.
type BlobStore interface {
	Put(ctx context.Context, r io.Reader) (hash.Digest, int64, error)
	Get(ctx context.Context, digest hash.Digest) (io.ReadCloser, int64, error)
	Exists(ctx context.Context, digest hash.Digest) (bool, error)
	Stat(ctx context.Context, digest hash.Digest) (BlobInfo, error)
	Delete(ctx context.Context, digest hash.Digest) error
}

// BlobMetadataStore tracks blob metadata for lookup and inventory.
type BlobMetadataStore interface {
	Save(ctx context.Context, info BlobInfo) error
	Get(ctx context.Context, digest hash.Digest) (BlobInfo, error)
	List(ctx context.Context) ([]BlobInfo, error)
	Delete(ctx context.Context, digest hash.Digest) error
}
