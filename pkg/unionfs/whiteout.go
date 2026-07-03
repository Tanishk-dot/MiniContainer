package unionfs

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// WhiteoutPrefix marks deletion whiteout files in upper/writable layers.
	WhiteoutPrefix = ".wh."
	// OpaqueWhiteout marks a directory as opaque, hiding lower-layer entries.
	OpaqueWhiteout = ".wh..wh..opq"
)

// IsWhiteout reports whether name is a whiteout marker file.
func IsWhiteout(name string) bool {
	return strings.HasPrefix(name, WhiteoutPrefix)
}

// WhiteoutTarget returns the hidden filename for a whiteout marker.
func WhiteoutTarget(name string) (string, bool) {
	if !IsWhiteout(name) {
		return "", false
	}
	target := strings.TrimPrefix(name, WhiteoutPrefix)
	if target == "" {
		return "", false
	}
	return target, true
}

// IsOpaqueWhiteout reports whether name is the opaque directory marker.
func IsOpaqueWhiteout(name string) bool {
	return name == OpaqueWhiteout
}

// CreateWhiteout creates a whiteout marker hiding targetName in dir.
func CreateWhiteout(dir, targetName string) error {
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, WhiteoutPrefix+targetName)
	return os.WriteFile(path, []byte{}, 0o644)
}
