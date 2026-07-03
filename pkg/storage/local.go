package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cloudforge/internal/paths"
	"cloudforge/pkg/hash"
)

// LocalBlobStore stores blobs on local disk using a content-addressable layout.
type LocalBlobStore struct {
	root string
	meta BlobMetadataStore
	mu   sync.RWMutex
}

// NewLocalBlobStore creates a blob store rooted at blobsDir with metadata tracking.
func NewLocalBlobStore(blobsDir string, meta BlobMetadataStore) *LocalBlobStore {
	return &LocalBlobStore{
		root: blobsDir,
		meta: meta,
	}
}

// Put stores content addressed by its SHA256 digest. Duplicate content is deduplicated.
func (s *LocalBlobStore) Put(ctx context.Context, r io.Reader) (hash.Digest, int64, error) {
	if err := ctx.Err(); err != nil {
		return hash.Digest{}, 0, err
	}

	hasher := sha256.New()
	reader := io.TeeReader(r, hasher)

	tmpPath := filepath.Join(s.root, ".staging", fmt.Sprintf("blob-%d.tmp", time.Now().UnixNano()))
	if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
		return hash.Digest{}, 0, fmt.Errorf("storage: create staging dir: %w", err)
	}

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return hash.Digest{}, 0, fmt.Errorf("storage: create temp blob: %w", err)
	}

	written, copyErr := io.Copy(f, reader)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return hash.Digest{}, 0, fmt.Errorf("storage: write blob: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return hash.Digest{}, 0, fmt.Errorf("storage: close temp blob: %w", closeErr)
	}

	digest := hash.Digest{
		Algorithm: hash.AlgorithmSHA256,
		Hex:       hex.EncodeToString(hasher.Sum(nil)),
	}

	blobPath, err := paths.BlobPath(s.root, digest)
	if err != nil {
		_ = os.Remove(tmpPath)
		return hash.Digest{}, 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, statErr := os.Stat(blobPath); statErr == nil {
		_ = os.Remove(tmpPath)
		info, metaErr := s.meta.Get(ctx, digest)
		if metaErr != nil {
			return hash.Digest{}, 0, metaErr
		}
		return digest, info.Size, nil
	} else if !os.IsNotExist(statErr) {
		_ = os.Remove(tmpPath)
		return hash.Digest{}, 0, fmt.Errorf("storage: stat blob: %w", statErr)
	}

	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		_ = os.Remove(tmpPath)
		return hash.Digest{}, 0, fmt.Errorf("storage: create blob dir: %w", err)
	}

	if err := os.Rename(tmpPath, blobPath); err != nil {
		_ = os.Remove(tmpPath)
		return hash.Digest{}, 0, fmt.Errorf("storage: commit blob: %w", err)
	}

	info := BlobInfo{
		Digest:    digest,
		Size:      written,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.meta.Save(ctx, info); err != nil {
		return hash.Digest{}, 0, fmt.Errorf("storage: save metadata: %w", err)
	}

	return digest, written, nil
}

// PutBytes is a convenience wrapper for storing in-memory content.
func (s *LocalBlobStore) PutBytes(ctx context.Context, content []byte) (hash.Digest, int64, error) {
	return s.Put(ctx, &byteReader{content: content})
}

type byteReader struct {
	content []byte
	offset  int
}

func (b *byteReader) Read(p []byte) (int, error) {
	if b.offset >= len(b.content) {
		return 0, io.EOF
	}
	n := copy(p, b.content[b.offset:])
	b.offset += n
	return n, nil
}

// Get opens a blob for reading.
func (s *LocalBlobStore) Get(ctx context.Context, digest hash.Digest) (io.ReadCloser, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if err := digest.Validate(); err != nil {
		return nil, 0, ErrInvalidDigest
	}

	info, err := s.meta.Get(ctx, digest)
	if err != nil {
		return nil, 0, err
	}

	blobPath, err := paths.BlobPath(s.root, digest)
	if err != nil {
		return nil, 0, err
	}

	f, err := os.Open(blobPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("storage: open blob: %w", err)
	}

	return f, info.Size, nil
}

// Exists reports whether a blob is present in storage.
func (s *LocalBlobStore) Exists(ctx context.Context, digest hash.Digest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := digest.Validate(); err != nil {
		return false, ErrInvalidDigest
	}

	_, err := s.meta.Get(ctx, digest)
	if err == nil {
		return true, nil
	}
	if err == ErrNotFound {
		return false, nil
	}
	return false, err
}

// Stat returns metadata for a blob.
func (s *LocalBlobStore) Stat(ctx context.Context, digest hash.Digest) (BlobInfo, error) {
	if err := ctx.Err(); err != nil {
		return BlobInfo{}, err
	}
	if err := digest.Validate(); err != nil {
		return BlobInfo{}, ErrInvalidDigest
	}
	return s.meta.Get(ctx, digest)
}

// Delete removes a blob and its metadata. Intended for garbage collection.
func (s *LocalBlobStore) Delete(ctx context.Context, digest hash.Digest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := digest.Validate(); err != nil {
		return ErrInvalidDigest
	}

	blobPath, err := paths.BlobPath(s.root, digest)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(blobPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: remove blob: %w", err)
	}

	// Remove empty parent directories best-effort.
	_ = os.Remove(filepath.Dir(blobPath))
	_ = os.Remove(filepath.Dir(filepath.Dir(blobPath)))

	return s.meta.Delete(ctx, digest)
}

// FileBlobMetadataStore persists blob metadata as JSON files.
type FileBlobMetadataStore struct {
	dir string
	mu  sync.RWMutex
}

// NewFileBlobMetadataStore creates a file-backed blob metadata store.
func NewFileBlobMetadataStore(dir string) *FileBlobMetadataStore {
	return &FileBlobMetadataStore{dir: dir}
}

type blobMetadataRecord struct {
	Digest    string    `json:"digest"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// Save writes blob metadata to disk.
func (m *FileBlobMetadataStore) Save(ctx context.Context, info BlobInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := info.Digest.Validate(); err != nil {
		return ErrInvalidDigest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return fmt.Errorf("storage: create metadata dir: %w", err)
	}

	record := blobMetadataRecord{
		Digest:    info.Digest.String(),
		Size:      info.Size,
		CreatedAt: info.CreatedAt.UTC(),
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("storage: marshal blob metadata: %w", err)
	}

	path := filepath.Join(m.dir, paths.MetadataFileName(info.Digest))
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("storage: write blob metadata: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("storage: commit blob metadata: %w", err)
	}
	return nil
}

// Get loads blob metadata from disk.
func (m *FileBlobMetadataStore) Get(ctx context.Context, digest hash.Digest) (BlobInfo, error) {
	if err := ctx.Err(); err != nil {
		return BlobInfo{}, err
	}
	if err := digest.Validate(); err != nil {
		return BlobInfo{}, ErrInvalidDigest
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	path := filepath.Join(m.dir, paths.MetadataFileName(digest))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BlobInfo{}, ErrNotFound
		}
		return BlobInfo{}, fmt.Errorf("storage: read blob metadata: %w", err)
	}

	var record blobMetadataRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return BlobInfo{}, fmt.Errorf("storage: parse blob metadata: %w", err)
	}

	parsed, err := hash.Parse(record.Digest)
	if err != nil {
		return BlobInfo{}, fmt.Errorf("storage: invalid stored digest: %w", err)
	}

	return BlobInfo{
		Digest:    parsed,
		Size:      record.Size,
		CreatedAt: record.CreatedAt,
	}, nil
}

// List returns all tracked blob metadata records.
func (m *FileBlobMetadataStore) List(ctx context.Context) ([]BlobInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("storage: list blob metadata: %w", err)
	}

	var result []BlobInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(m.dir, entry.Name()))
		if readErr != nil {
			return nil, fmt.Errorf("storage: read blob metadata %s: %w", entry.Name(), readErr)
		}
		var record blobMetadataRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("storage: parse blob metadata %s: %w", entry.Name(), err)
		}
		parsed, parseErr := hash.Parse(record.Digest)
		if parseErr != nil {
			return nil, fmt.Errorf("storage: invalid digest in %s: %w", entry.Name(), parseErr)
		}
		result = append(result, BlobInfo{
			Digest:    parsed,
			Size:      record.Size,
			CreatedAt: record.CreatedAt,
		})
	}
	return result, nil
}

// Delete removes blob metadata from disk.
func (m *FileBlobMetadataStore) Delete(ctx context.Context, digest hash.Digest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := digest.Validate(); err != nil {
		return ErrInvalidDigest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.dir, paths.MetadataFileName(digest))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete blob metadata: %w", err)
	}
	return nil
}
