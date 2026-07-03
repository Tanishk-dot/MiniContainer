package config

import (
	"os"
	"path/filepath"
)

const (
	DefaultRootDirName = ".cloudforge"
)

// Config holds filesystem paths for the CloudForge data directory.
type Config struct {
	RootDir string
}

// Default returns a Config rooted at ~/.cloudforge (or %USERPROFILE%\.cloudforge on Windows).
func Default() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &Config{RootDir: filepath.Join(home, DefaultRootDirName)}, nil
}

// WithRootDir returns a Config using an explicit root directory.
func WithRootDir(root string) *Config {
	return &Config{RootDir: root}
}

// BlobsDir returns the content-addressable blob store root.
func (c *Config) BlobsDir() string {
	return filepath.Join(c.RootDir, "blobs", "sha256")
}

// MetadataDir returns the directory for JSON metadata indexes.
func (c *Config) MetadataDir() string {
	return filepath.Join(c.RootDir, "metadata")
}

// BlobMetadataDir returns blob metadata file location root.
func (c *Config) BlobMetadataDir() string {
	return filepath.Join(c.MetadataDir(), "blobs")
}

// LayerMetadataDir returns layer metadata file location root.
func (c *Config) LayerMetadataDir() string {
	return filepath.Join(c.MetadataDir(), "layers")
}

// ImageMetadataDir returns image manifest metadata file location root.
func (c *Config) ImageMetadataDir() string {
	return filepath.Join(c.MetadataDir(), "images")
}

// TmpDir returns a scratch directory for staging writes.
func (c *Config) TmpDir() string {
	return filepath.Join(c.RootDir, "tmp")
}

// LayersDir returns the root for unpacked layer rootfs trees.
func (c *Config) LayersDir() string {
	return filepath.Join(c.RootDir, "layers")
}

// LayerRootfsPath returns the unpacked rootfs directory for a layer digest.
func (c *Config) LayerRootfsPath(digest string) string {
	return filepath.Join(c.LayersDir(), digest, "rootfs")
}

// MountsDir returns the root for active union filesystem mounts.
func (c *Config) MountsDir() string {
	return filepath.Join(c.RootDir, "mounts")
}

// EnsureDirs creates all required on-disk directories.
func (c *Config) EnsureDirs() error {
	dirs := []string{
		c.BlobsDir(),
		c.BlobMetadataDir(),
		c.LayerMetadataDir(),
		c.ImageMetadataDir(),
		c.LayersDir(),
		c.MountsDir(),
		c.TmpDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
