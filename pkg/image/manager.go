package image

import (
    "bytes"
    "context"
    "encoding/json"
    "time"

    "cloudforge/pkg/hash"
    "cloudforge/pkg/storage"
)

// Manager provides manifest registration and lookup.
type Manager interface {
    Store(ctx context.Context, raw []byte) (*Manifest, error)
    Get(ctx context.Context, id hash.Digest) (*Manifest, error)
    List(ctx context.Context) ([]*Manifest, error)
    Delete(ctx context.Context, id hash.Digest) error
}

type imageManager struct {
    blobs storage.BlobStore
    store ImageMetadataStore
}

// NewManager creates an image manager backed by blob storage and metadata tracking.
func NewManager(blobs storage.BlobStore, store ImageMetadataStore) Manager {
    return &imageManager{blobs: blobs, store: store}
}

// Store saves manifest bytes to blob storage and records metadata.
func (m *imageManager) Store(ctx context.Context, raw []byte) (*Manifest, error) {
    if err := ctx.Err(); err != nil {
        return nil, err
    }

    digest, size, err := m.blobs.Put(ctx, bytes.NewReader(raw))
    if err != nil {
        return nil, err
    }

    manifest := &Manifest{
        Digest:       digest,
        SchemaVersion: 1,
        MediaType:    "application/vnd.cloudforge.image.manifest.v1+json",
        Size:         size,
        CreatedAt:    time.Now().UTC(),
    }

    // Best-effort: try to parse a simple manifest JSON structure to populate config/layers.
    // Parsing is optional; metadata store still records the manifest blob digest and size.
    var parsed struct {
        SchemaVersion int `json:"schemaVersion"`
        MediaType     string `json:"mediaType"`
        Config struct {
            MediaType string `json:"mediaType"`
            Digest    string `json:"digest"`
            Size      int64  `json:"size"`
        } `json:"config"`
        Layers []struct {
            MediaType string `json:"mediaType"`
            Digest    string `json:"digest"`
            Size      int64  `json:"size"`
        } `json:"layers"`
    }
    _ = json.Unmarshal(raw, &parsed) // ignore errors; fill what we can
    if parsed.SchemaVersion != 0 {
        manifest.SchemaVersion = parsed.SchemaVersion
    }
    if parsed.MediaType != "" {
        manifest.MediaType = parsed.MediaType
    }
    if parsed.Config.Digest != "" {
        if d, err := hash.Parse(parsed.Config.Digest); err == nil {
            manifest.Config = &d
        }
    }
    if len(parsed.Layers) > 0 {
        manifest.Layers = make([]hash.Digest, 0, len(parsed.Layers))
        for _, l := range parsed.Layers {
            if d, err := hash.Parse(l.Digest); err == nil {
                manifest.Layers = append(manifest.Layers, d)
            }
        }
    }

    if err := manifest.Validate(); err != nil {
        return nil, err
    }

    if existing, getErr := m.store.Get(ctx, digest); getErr == nil {
        return existing, nil
    } else if getErr != ErrNotFound {
        return nil, getErr
    }

    if err := m.store.Save(ctx, manifest); err != nil {
        return nil, err
    }
    return manifest, nil
}

// Get returns manifest metadata by digest.
func (m *imageManager) Get(ctx context.Context, id hash.Digest) (*Manifest, error) {
    if err := ctx.Err(); err != nil {
        return nil, err
    }
    if err := id.Validate(); err != nil {
        return nil, ErrInvalidManifest
    }
    return m.store.Get(ctx, id)
}

// List returns all manifests.
func (m *imageManager) List(ctx context.Context) ([]*Manifest, error) {
    if err := ctx.Err(); err != nil {
        return nil, err
    }
    return m.store.List(ctx)
}

// Delete removes manifest metadata (does not delete blob bytes).
func (m *imageManager) Delete(ctx context.Context, id hash.Digest) error {
    if err := ctx.Err(); err != nil {
        return err
    }
    if err := id.Validate(); err != nil {
        return ErrInvalidManifest
    }
    return m.store.Delete(ctx, id)
}
