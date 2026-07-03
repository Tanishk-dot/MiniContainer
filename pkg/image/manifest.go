package image

import (
    "errors"
    "time"

    "cloudforge/pkg/hash"
)

var (
    // ErrNotFound when manifest doesn't exist
    ErrNotFound = errors.New("image: not found")
    // ErrInvalidManifest when manifest fails validation
    ErrInvalidManifest = errors.New("image: invalid manifest")
)

// Manifest represents a simple image manifest describing config and layer digests.
type Manifest struct {
    Digest       hash.Digest
    SchemaVersion int
    MediaType    string
    Config       *hash.Digest
    Layers       []hash.Digest
    Size         int64
    CreatedAt    time.Time
}

// Validate checks basic invariants for a manifest.
func (m *Manifest) Validate() error {
    if m == nil {
        return ErrInvalidManifest
    }
    if m.SchemaVersion <= 0 {
        return ErrInvalidManifest
    }
    if m.MediaType == "" {
        return ErrInvalidManifest
    }
    if m.Digest.Hex == "" {
        return ErrInvalidManifest
    }
    if m.Config != nil {
        if err := m.Config.Validate(); err != nil {
            return ErrInvalidManifest
        }
    }
    for _, l := range m.Layers {
        if err := l.Validate(); err != nil {
            return ErrInvalidManifest
        }
    }
    return nil
}
