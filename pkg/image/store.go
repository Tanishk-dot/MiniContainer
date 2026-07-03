package image

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"

    "cloudforge/internal/paths"
    "cloudforge/pkg/hash"
)

// ImageMetadataStore persists manifest metadata for lookup and inventory.
type ImageMetadataStore interface {
    Save(ctx context.Context, manifest *Manifest) error
    Get(ctx context.Context, digest hash.Digest) (*Manifest, error)
    List(ctx context.Context) ([]*Manifest, error)
    Delete(ctx context.Context, digest hash.Digest) error
}

// FileImageMetadataStore stores manifest metadata as JSON files.
type FileImageMetadataStore struct {
    dir string
    mu  sync.RWMutex
}

// NewFileImageMetadataStore creates a file-backed image metadata store.
func NewFileImageMetadataStore(dir string) *FileImageMetadataStore {
    return &FileImageMetadataStore{dir: dir}
}

type imageMetadataRecord struct {
    Digest        string   `json:"digest"`
    SchemaVersion int      `json:"schema_version"`
    MediaType     string   `json:"media_type"`
    Config        *string  `json:"config,omitempty"`
    Layers        []string `json:"layers"`
    Size          int64    `json:"size"`
    CreatedAt     string   `json:"created_at"`
}

// Save writes manifest metadata to disk.
func (s *FileImageMetadataStore) Save(ctx context.Context, manifest *Manifest) error {
    if err := ctx.Err(); err != nil {
        return err
    }
    if err := manifest.Validate(); err != nil {
        return err
    }

    s.mu.Lock()
    defer s.mu.Unlock()

    if err := os.MkdirAll(s.dir, 0o755); err != nil {
        return fmt.Errorf("image: create metadata dir: %w", err)
    }

    record := imageMetadataRecord{
        Digest:        manifest.Digest.String(),
        SchemaVersion: manifest.SchemaVersion,
        MediaType:     manifest.MediaType,
        Size:          manifest.Size,
        Layers:        make([]string, 0, len(manifest.Layers)),
        CreatedAt:     manifest.CreatedAt.UTC().Format(time.RFC3339Nano),
    }
    if manifest.Config != nil {
        c := manifest.Config.String()
        record.Config = &c
    }
    for _, l := range manifest.Layers {
        record.Layers = append(record.Layers, l.String())
    }

    data, err := json.MarshalIndent(record, "", "  ")
    if err != nil {
        return fmt.Errorf("image: marshal metadata: %w", err)
    }

    path := filepath.Join(s.dir, paths.MetadataFileName(manifest.Digest))
    tmpPath := path + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
        return fmt.Errorf("image: write metadata: %w", err)
    }
    if err := os.Rename(tmpPath, path); err != nil {
        _ = os.Remove(tmpPath)
        return fmt.Errorf("image: commit metadata: %w", err)
    }
    return nil
}

// Get loads manifest metadata by digest.
func (s *FileImageMetadataStore) Get(ctx context.Context, digest hash.Digest) (*Manifest, error) {
    if err := ctx.Err(); err != nil {
        return nil, err
    }
    if err := digest.Validate(); err != nil {
        return nil, ErrInvalidManifest
    }

    s.mu.RLock()
    defer s.mu.RUnlock()

    path := filepath.Join(s.dir, paths.MetadataFileName(digest))
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, ErrNotFound
        }
        return nil, fmt.Errorf("image: read metadata: %w", err)
    }

    var rec imageMetadataRecord
    if err := json.Unmarshal(data, &rec); err != nil {
        return nil, fmt.Errorf("image: unmarshal metadata: %w", err)
    }

    parsed, parseErr := hash.Parse(rec.Digest)
    if parseErr != nil {
        return nil, fmt.Errorf("image: invalid stored digest: %w", parseErr)
    }

    var cfg *hash.Digest
    if rec.Config != nil {
        p, perr := hash.Parse(*rec.Config)
        if perr != nil {
            return nil, fmt.Errorf("image: invalid stored config digest: %w", perr)
        }
        cfg = &p
    }

    layers := make([]hash.Digest, 0, len(rec.Layers))
    for _, ls := range rec.Layers {
        p, perr := hash.Parse(ls)
        if perr != nil {
            return nil, fmt.Errorf("image: invalid stored layer digest: %w", perr)
        }
        layers = append(layers, p)
    }

    // Parse created time conservatively; zero time on error.
    var createdAtTime time.Time
    if t, err := time.Parse(time.RFC3339Nano, rec.CreatedAt); err == nil {
        createdAtTime = t
    } else if t, err := time.Parse(time.RFC3339, rec.CreatedAt); err == nil {
        // Fallback for older format without nanoseconds
        createdAtTime = t
    }

    m := &Manifest{
        Digest:        parsed,
        SchemaVersion: rec.SchemaVersion,
        MediaType:     rec.MediaType,
        Config:        cfg,
        Layers:        layers,
        Size:          rec.Size,
        CreatedAt:     createdAtTime,
    }
    if err := m.Validate(); err != nil {
        return nil, err
    }
    return m, nil
}

// List returns all stored manifest metadata.
func (s *FileImageMetadataStore) List(ctx context.Context) ([]*Manifest, error) {
    if err := ctx.Err(); err != nil {
        return nil, err
    }

    s.mu.RLock()
    defer s.mu.RUnlock()

    entries, err := os.ReadDir(s.dir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil
        }
        return nil, fmt.Errorf("image: list metadata: %w", err)
    }

    var result []*Manifest
    for _, entry := range entries {
        if entry.IsDir() {
            continue
        }
        data, readErr := os.ReadFile(filepath.Join(s.dir, entry.Name()))
        if readErr != nil {
            return nil, fmt.Errorf("image: read metadata %s: %w", entry.Name(), readErr)
        }
        var rec imageMetadataRecord
        if err := json.Unmarshal(data, &rec); err != nil {
            return nil, fmt.Errorf("image: parse metadata %s: %w", entry.Name(), err)
        }
        parsed, parseErr := hash.Parse(rec.Digest)
        if parseErr != nil {
            return nil, fmt.Errorf("image: invalid digest in %s: %w", entry.Name(), parseErr)
        }
        var cfg *hash.Digest
        if rec.Config != nil {
            p, perr := hash.Parse(*rec.Config)
            if perr != nil {
                return nil, fmt.Errorf("image: invalid config digest in %s: %w", entry.Name(), perr)
            }
            cfg = &p
        }
        layers := make([]hash.Digest, 0, len(rec.Layers))
        for _, ls := range rec.Layers {
            p, perr := hash.Parse(ls)
            if perr != nil {
                return nil, fmt.Errorf("image: invalid layer digest in %s: %w", entry.Name(), perr)
            }
            layers = append(layers, p)
        }
        var createdAtTime time.Time
        if t, err := time.Parse(time.RFC3339, rec.CreatedAt); err == nil {
            createdAtTime = t
        }
        m := &Manifest{
            Digest:        parsed,
            SchemaVersion: rec.SchemaVersion,
            MediaType:     rec.MediaType,
            Config:        cfg,
            Layers:        layers,
            Size:          rec.Size,
            CreatedAt:     createdAtTime,
        }
        result = append(result, m)
    }
    return result, nil
}

// Delete removes manifest metadata from disk.
func (s *FileImageMetadataStore) Delete(ctx context.Context, digest hash.Digest) error {
    if err := ctx.Err(); err != nil {
        return err
    }
    if err := digest.Validate(); err != nil {
        return ErrInvalidManifest
    }

    s.mu.Lock()
    defer s.mu.Unlock()

    path := filepath.Join(s.dir, paths.MetadataFileName(digest))
    if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("image: delete metadata: %w", err)
    }
    return nil
}
