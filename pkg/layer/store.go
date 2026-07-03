package layer

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

// FileLayerMetadataStore persists layer metadata as JSON files.
type FileLayerMetadataStore struct {
	dir string
	mu  sync.RWMutex
}

// NewFileLayerMetadataStore creates a file-backed layer metadata store.
func NewFileLayerMetadataStore(dir string) *FileLayerMetadataStore {
	return &FileLayerMetadataStore{dir: dir}
}

type layerMetadataRecord struct {
	ID        string  `json:"id"`
	ParentID  *string `json:"parent_id,omitempty"`
	Size      int64   `json:"size"`
	MediaType string  `json:"media_type"`
	CreatedAt time.Time `json:"created_at"`
}

// Save writes layer metadata to disk.
func (s *FileLayerMetadataStore) Save(ctx context.Context, layer *Layer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := layer.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("layer: create metadata dir: %w", err)
	}

	record := layerMetadataRecord{
		ID:        layer.ID.String(),
		Size:      layer.Size,
		MediaType: layer.MediaType,
		CreatedAt: layer.CreatedAt.UTC(),
	}
	if layer.ParentID != nil {
		parent := layer.ParentID.String()
		record.ParentID = &parent
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("layer: marshal metadata: %w", err)
	}

	path := filepath.Join(s.dir, paths.MetadataFileName(layer.ID))
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("layer: write metadata: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("layer: commit metadata: %w", err)
	}
	return nil
}

// Get loads layer metadata by SHA256 layer ID.
func (s *FileLayerMetadataStore) Get(ctx context.Context, id hash.Digest) (*Layer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := id.Validate(); err != nil {
		return nil, ErrInvalidLayer
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.dir, paths.MetadataFileName(id))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("layer: read metadata: %w", err)
	}

	return decodeLayerRecord(data)
}

// List returns all layer metadata records.
func (s *FileLayerMetadataStore) List(ctx context.Context) ([]*Layer, error) {
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
		return nil, fmt.Errorf("layer: list metadata: %w", err)
	}

	var layers []*Layer
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if readErr != nil {
			return nil, fmt.Errorf("layer: read metadata %s: %w", entry.Name(), readErr)
		}
		layer, decodeErr := decodeLayerRecord(data)
		if decodeErr != nil {
			return nil, fmt.Errorf("layer: parse metadata %s: %w", entry.Name(), decodeErr)
		}
		layers = append(layers, layer)
	}
	return layers, nil
}

// Delete removes layer metadata from disk.
func (s *FileLayerMetadataStore) Delete(ctx context.Context, id hash.Digest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := id.Validate(); err != nil {
		return ErrInvalidLayer
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, paths.MetadataFileName(id))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("layer: delete metadata: %w", err)
	}
	return nil
}

func decodeLayerRecord(data []byte) (*Layer, error) {
	var record layerMetadataRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("layer: unmarshal metadata: %w", err)
	}

	id, err := hash.Parse(record.ID)
	if err != nil {
		return nil, err
	}

	layer := &Layer{
		ID:        id,
		Size:      record.Size,
		MediaType: record.MediaType,
		CreatedAt: record.CreatedAt,
	}
	if record.ParentID != nil {
		parent, parseErr := hash.Parse(*record.ParentID)
		if parseErr != nil {
			return nil, parseErr
		}
		layer.ParentID = &parent
	}

	if err := layer.Validate(); err != nil {
		return nil, err
	}
	return layer, nil
}
