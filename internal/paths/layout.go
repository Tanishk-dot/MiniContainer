package paths

import (
	"fmt"
	"path/filepath"

	"cloudforge/pkg/hash"
)

const blobDataFile = "data"

// BlobPath returns the on-disk path for a content-addressed blob.
// Layout: {root}/sha256/{first-two-hex}/{full-hex}/data
func BlobPath(root string, digest hash.Digest) (string, error) {
	if err := digest.Validate(); err != nil {
		return "", err
	}
	if len(digest.Hex) < 2 {
		return "", fmt.Errorf("paths: digest hex too short")
	}
	return filepath.Join(root, digest.Hex[:2], digest.Hex, blobDataFile), nil
}

// MetadataFileName returns a filesystem-safe metadata filename for a digest.
func MetadataFileName(digest hash.Digest) string {
	return digest.Algorithm + "-" + digest.Hex + ".json"
}
